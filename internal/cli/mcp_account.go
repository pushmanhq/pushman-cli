package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/text/unicode/norm"
)

type mcpAccountResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}
type mcpRenameInput struct {
	Nickname string `json:"nickname" jsonschema:"New sender nickname, 1 through 64 Unicode scalar values."`
}
type mcpAuthorizationInput struct {
	SuggestedName string `json:"suggestedName,omitempty" jsonschema:"Optional sender nickname. Defaults to this computer's hostname."`
}
type mcpAuthorizationHandle struct {
	AuthorizationID string `json:"authorizationId" jsonschema:"Opaque progress handle returned by this MCP process, never a device code or bearer token."`
}
type mcpAuthorizationOutput struct {
	AuthorizationID string `json:"authorizationId"`
	Status          string `json:"status"`
	UserCode        string `json:"userCode,omitempty"`
	VerificationURL string `json:"verificationUrl,omitempty"`
	ExpiresAt       string `json:"expiresAt,omitempty"`
	Nickname        string `json:"nickname,omitempty"`
	Message         string `json:"message,omitempty"`
}

type mcpPendingAuthorization struct {
	output mcpAuthorizationOutput
	owner  *mcp.ServerSession
	cancel context.CancelFunc
	ready  chan struct{}
	done   chan struct{}
	err    error
}
type mcpAuthorizations struct {
	mu      sync.Mutex
	pending *mcpPendingAuthorization
}

func registerMCPAccountTools(server *mcp.Server, deps Dependencies) {
	read := &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: boolPointer(false), IdempotentHint: true, OpenWorldHint: boolPointer(false)}
	change := &mcp.ToolAnnotations{DestructiveHint: boolPointer(false), OpenWorldHint: boolPointer(true)}
	revoke := &mcp.ToolAnnotations{DestructiveHint: boolPointer(true), OpenWorldHint: boolPointer(true)}
	mcp.AddTool(server, &mcp.Tool{Name: "pushman_rename_sender", Title: "Rename this Pushman sender", Description: "Rename only this authorized sender. Requires an explicit user request; does not send a notification.", Annotations: change}, func(ctx context.Context, _ *mcp.CallToolRequest, in mcpRenameInput) (*mcp.CallToolResult, mcpAccountResult, error) {
		name := norm.NFC.String(strings.TrimSpace(in.Nickname))
		if !utf8.ValidString(name) || countScalars(name) < 1 || countScalars(name) > 64 {
			return nil, mcpAccountResult{}, usagef("nickname must contain 1 through 64 Unicode scalar values")
		}
		err := deps.Service.Rename(ctx, name)
		return nil, mcpAccountResult{OK: err == nil}, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "pushman_logout", Title: "Disconnect this Pushman sender", Description: "Revoke the current local sender credential and remove it from this computer's credential store. Requires an explicit user request.", Annotations: revoke}, func(ctx context.Context, _ *mcp.CallToolRequest, _ mcpEmptyInput) (*mcp.CallToolResult, mcpAccountResult, error) {
		err := deps.Service.Logout(ctx)
		return nil, mcpAccountResult{OK: err == nil}, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "pushman_get_version", Title: "Get Pushman version", Description: "Read the installed CLI build version without changing it.", Annotations: read}, func(context.Context, *mcp.CallToolRequest, mcpEmptyInput) (*mcp.CallToolResult, VersionInfo, error) {
		return nil, deps.Version, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "pushman_get_support_bundle", Title: "Get privacy-safe Pushman diagnostics", Description: "Return version, platform, generation time and named boolean doctor checks. Excludes tokens, messages, account IDs, endpoints and diagnostic text. Does not write or upload a file.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPointer(true)}}, func(ctx context.Context, _ *mcp.CallToolRequest, _ mcpEmptyInput) (*mcp.CallToolResult, supportBundle, error) {
		checks, err := deps.Service.Doctor(ctx)
		if err != nil {
			return nil, supportBundle{}, err
		}
		safe := make([]supportBundleCheck, 0, len(checks))
		for _, check := range checks {
			safe = append(safe, supportBundleCheck{Name: check.Name, OK: check.OK})
		}
		return nil, supportBundle{Schema: 1, GeneratedAt: deps.Now().UTC().Format(time.RFC3339), Version: deps.Version, Runtime: supportBundleRuntime{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}, Checks: safe, Privacy: "Returned only to this requesting client; no file or support upload. No tokens, message content, account IDs, endpoints or diagnostic messages."}, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "pushman_self_update", Title: "Update an eligible Pushman Homebrew installation", Description: "Run the existing ownership-checked Homebrew self-update on this computer. Requires an explicit request to update the CLI. May replace the executable. Never retries or bypasses an ownership refusal.", Annotations: change}, func(ctx context.Context, _ *mcp.CallToolRequest, _ mcpEmptyInput) (*mcp.CallToolResult, mcpAccountResult, error) {
		output, err := deps.SelfUpdate(ctx)
		return nil, mcpAccountResult{OK: err == nil, Message: output}, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "pushman_get_capabilities", Title: "Get Pushman command help and capabilities", Description: "Describe every CLI workflow, its MCP equivalent and this transport's authorization boundary.", Annotations: read}, func(context.Context, *mcp.CallToolRequest, mcpEmptyInput) (*mcp.CallToolResult, mcpCapabilitiesOutput, error) {
		return nil, localMCPCapabilities(), nil
	})
	authorizations := &mcpAuthorizations{}
	for _, method := range []string{"login", "pair"} {
		mcp.AddTool(server, &mcp.Tool{Name: "pushman_" + method, Title: "Start Pushman " + method, Description: "Start explicitly requested local account authorization. Returns the public verification URL and code; the user signs in or approves on iPhone. Credentials and polling secrets stay private. Use pushman_get_authorization to inspect progress or pushman_cancel_authorization to cancel.", Annotations: change}, func(ctx context.Context, req *mcp.CallToolRequest, in mcpAuthorizationInput) (*mcp.CallToolResult, mcpAuthorizationOutput, error) {
			out, err := authorizations.begin(ctx, req.Session, deps, method, in.SuggestedName)
			return nil, out, err
		})
	}
	mcp.AddTool(server, &mcp.Tool{Name: "pushman_get_authorization", Title: "Read Pushman authorization progress", Description: "Read the status of one authorization started by this MCP session. Does not create or retry an authorization request.", Annotations: read}, func(_ context.Context, req *mcp.CallToolRequest, in mcpAuthorizationHandle) (*mcp.CallToolResult, mcpAuthorizationOutput, error) {
		out, err := authorizations.get(req.Session, in.AuthorizationID, false)
		return nil, out, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "pushman_cancel_authorization", Title: "Cancel pending Pushman authorization", Description: "Cancel the named pending authorization from this MCP session. Does not revoke an already issued credential.", Annotations: change}, func(_ context.Context, req *mcp.CallToolRequest, in mcpAuthorizationHandle) (*mcp.CallToolResult, mcpAuthorizationOutput, error) {
		out, err := authorizations.get(req.Session, in.AuthorizationID, true)
		return nil, out, err
	})
}

func (a *mcpAuthorizations) begin(ctx context.Context, owner *mcp.ServerSession, deps Dependencies, method, name string) (mcpAuthorizationOutput, error) {
	if name == "" {
		var err error
		name, err = deps.Hostname()
		if err != nil {
			return mcpAuthorizationOutput{}, fmt.Errorf("read hostname for authorization: %w", err)
		}
	}
	if !utf8.ValidString(name) || countScalars(strings.TrimSpace(name)) > 64 {
		return mcpAuthorizationOutput{}, usagef("suggestedName must contain 1 through 64 Unicode scalar values")
	}
	name, err := normalizeSuggestedName(name)
	if err != nil {
		return mcpAuthorizationOutput{}, err
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return mcpAuthorizationOutput{}, err
	}
	a.mu.Lock()
	if a.pending != nil {
		select {
		case <-a.pending.done:
		default:
			a.mu.Unlock()
			return mcpAuthorizationOutput{}, fmt.Errorf("an authorization is already pending; inspect or cancel it")
		}
	}
	runCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	p := &mcpPendingAuthorization{output: mcpAuthorizationOutput{AuthorizationID: hex.EncodeToString(id[:]), Status: "pending"}, owner: owner, cancel: cancel, ready: make(chan struct{}, 1), done: make(chan struct{})}
	a.pending = p
	a.mu.Unlock()
	challenge := func(code, url string, expiry time.Time) error {
		a.mu.Lock()
		p.output.UserCode = code
		p.output.VerificationURL = url
		p.output.ExpiresAt = expiry.UTC().Format(time.RFC3339)
		a.mu.Unlock()
		select {
		case p.ready <- struct{}{}:
		default:
		}
		return nil
	}
	go func() {
		defer cancel()
		var result PairResult
		var err error
		if method == "login" {
			result, err = deps.Service.Login(runCtx, LoginRequest{Platform: runtime.GOOS, SuggestedName: name, OnChallenge: func(c LoginChallenge) error { return challenge(c.UserCode, c.CompleteURL, deps.Now().Add(c.ExpiresIn)) }})
		} else {
			result, err = deps.Service.Pair(runCtx, PairRequest{Platform: runtime.GOOS, SuggestedName: name, OnChallenge: func(c PairChallenge) error {
				url, err := pairingVerificationURL(c.VerificationURL, c.UserCode)
				if err != nil {
					return err
				}
				return challenge(c.UserCode, url, c.ExpiresAt)
			}})
		}
		a.mu.Lock()
		p.err = err
		if err == nil {
			p.output.Status = "authorized"
			p.output.Nickname = result.Nickname
		} else {
			p.output.Status = "failed"
			p.output.Message = "Authorization did not complete; inspect local status before starting another attempt."
		}
		// Clear public challenge details once the attempt ends.
		p.output.UserCode = ""
		p.output.VerificationURL = ""
		close(p.done)
		a.mu.Unlock()
	}()
	// A subprocess/client disconnect must stop polling rather than leave an
	// unattended authorization capable of changing the keyring later.
	if owner != nil {
		sessionClosed := make(chan struct{})
		go func() { _ = owner.Wait(); close(sessionClosed) }()
		go func() {
			select {
			case <-sessionClosed:
				cancel()
			case <-p.done:
			}
		}()
	}
	select {
	case <-p.ready:
	case <-p.done:
	case <-ctx.Done():
		cancel()
		return mcpAuthorizationOutput{}, ctx.Err()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if p.err != nil {
		return mcpAuthorizationOutput{}, p.err
	}
	return p.output, nil
}

func (a *mcpAuthorizations) get(owner *mcp.ServerSession, id string, cancel bool) (mcpAuthorizationOutput, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	p := a.pending
	if p == nil || p.owner != owner || p.output.AuthorizationID != id {
		return mcpAuthorizationOutput{}, usagef("authorizationId is not available in this MCP session")
	}
	if cancel {
		select {
		case <-p.done:
		default:
			p.cancel()
			p.output.Status = "canceling"
		}
	}
	return p.output, nil
}

type mcpCapabilitiesOutput struct {
	Transport     string            `json:"transport"`
	Commands      map[string]string `json:"commands"`
	Authorization string            `json:"authorization"`
}

func localMCPCapabilities() mcpCapabilitiesOutput {
	return mcpCapabilitiesOutput{Transport: "stdio", Authorization: "Uses this computer's account credential; PUSHMAN_TOKEN remains send-only. Mutations require explicit user intent.", Commands: map[string]string{
		"push": "pushman_send_notification", "devices": "pushman_list_devices", "history": "pushman_list_history", "history show": "pushman_get_message", "usage": "pushman_get_usage", "status": "pushman_get_status", "doctor": "pushman_doctor", "rename": "pushman_rename_sender", "logout": "pushman_logout", "login": "pushman_login then pushman_get_authorization", "pair": "pushman_pair then pushman_get_authorization", "support-bundle": "pushman_get_support_bundle returns a bundle without writing a file", "version": "pushman_get_version", "help": "pushman_get_capabilities and each tool's schema", "self-update": "pushman_self_update (eligible Homebrew installation only)", "mcp": "This active MCP transport"}}
}
