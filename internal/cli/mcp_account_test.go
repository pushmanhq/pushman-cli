package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type accountMCPService struct {
	mcpStubService
	authorized chan struct{}
	stopped    chan struct{}
	renamed    string
	loggedOut  bool
}

func (s *accountMCPService) Login(ctx context.Context, r LoginRequest) (PairResult, error) {
	defer close(s.stopped)
	if err := r.OnChallenge(LoginChallenge{UserCode: "ABCD-EFGH", VerificationURL: "https://example.com/activate/", CompleteURL: "https://example.com/activate/?user_code=ABCD-EFGH", ExpiresIn: 10 * time.Minute}); err != nil {
		return PairResult{}, err
	}
	select {
	case <-ctx.Done():
		return PairResult{}, ctx.Err()
	case <-s.authorized:
		return PairResult{Nickname: r.SuggestedName}, nil
	}
}
func (s *accountMCPService) Pair(ctx context.Context, r PairRequest) (PairResult, error) {
	defer close(s.stopped)
	if err := r.OnChallenge(PairChallenge{UserCode: "ABCD-EFGH", VerificationURL: "https://example.com/pair/", ExpiresAt: time.Now().Add(10 * time.Minute)}); err != nil {
		return PairResult{}, err
	}
	select {
	case <-ctx.Done():
		return PairResult{}, ctx.Err()
	case <-s.authorized:
		return PairResult{Nickname: r.SuggestedName}, nil
	}
}
func (s *accountMCPService) Rename(_ context.Context, name string) error {
	s.renamed = name
	return nil
}
func (s *accountMCPService) Logout(context.Context) error { s.loggedOut = true; return nil }
func decodeAuthorization(t *testing.T, result *mcp.CallToolResult) mcpAuthorizationOutput {
	t.Helper()
	if result.IsError {
		t.Fatal(result.Content)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out mcpAuthorizationOutput
	if json.Unmarshal(raw, &out) != nil {
		t.Fatal("invalid authorization result")
	}
	return out
}
func TestMCPLocalAuthorizationProgressAndCancellation(t *testing.T) {
	for _, method := range []string{"login", "pair"} {
		t.Run(method, func(t *testing.T) {
			service := &accountMCPService{authorized: make(chan struct{}), stopped: make(chan struct{})}
			session, closeSession := connectMCPTestClient(t, service)
			defer closeSession()
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pushman_" + method, Arguments: map[string]any{"suggestedName": "Synthetic sender"}})
			if err != nil {
				t.Fatal(err)
			}
			out := decodeAuthorization(t, result)
			if out.Status != "pending" || out.UserCode != "ABCD-EFGH" || out.AuthorizationID == "" || !strings.HasPrefix(out.VerificationURL, "https://example.com/") {
				t.Fatal("public challenge missing")
			}
			callToolOK(t, session, "pushman_get_authorization", map[string]any{"authorizationId": out.AuthorizationID}, "status", "pending")
			invalid, _ := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pushman_get_authorization", Arguments: map[string]any{"authorizationId": "other"}})
			if !invalid.IsError {
				t.Fatal("unknown handle accepted")
			}
			callToolOK(t, session, "pushman_cancel_authorization", map[string]any{"authorizationId": out.AuthorizationID}, "status", nil)
			select {
			case <-service.stopped:
			case <-time.After(time.Second):
				t.Fatal("authorization did not stop")
			}
			callToolOK(t, session, "pushman_get_authorization", map[string]any{"authorizationId": out.AuthorizationID}, "status", "failed")
		})
	}
}
func TestMCPLocalAuthorizationCompletesAndDisconnectStopsPending(t *testing.T) {
	service := &accountMCPService{authorized: make(chan struct{}), stopped: make(chan struct{})}
	session, closeSession := connectMCPTestClient(t, service)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pushman_login", Arguments: map[string]any{"suggestedName": "Synthetic sender"}})
	if err != nil {
		t.Fatal(err)
	}
	out := decodeAuthorization(t, result)
	closeSession()
	select {
	case <-service.stopped:
	case <-time.After(time.Second):
		t.Fatal("client disconnect left polling active")
	}
	service = &accountMCPService{authorized: make(chan struct{}), stopped: make(chan struct{})}
	session, closeSession = connectMCPTestClient(t, service)
	defer closeSession()
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pushman_pair", Arguments: map[string]any{"suggestedName": "Synthetic sender"}})
	if err != nil {
		t.Fatal(err)
	}
	out = decodeAuthorization(t, result)
	close(service.authorized)
	<-service.stopped
	callToolOK(t, session, "pushman_get_authorization", map[string]any{"authorizationId": out.AuthorizationID}, "nickname", "Synthetic sender")
	callToolOK(t, session, "pushman_get_authorization", map[string]any{"authorizationId": out.AuthorizationID}, "status", "authorized")
}
func TestMCPAccountMutationValidationAndBundlePrivacy(t *testing.T) {
	service := &accountMCPService{mcpStubService: mcpStubService{checks: []DoctorCheck{{Name: "credential", OK: true, Message: "synthetic-private-context"}}}}
	updates := 0
	session, closeSession := connectMCPDependencies(t, Dependencies{Service: service, Version: VersionInfo{Version: "test"}, SelfUpdate: func(context.Context) (string, error) { updates++; return "already up to date", nil }})
	defer closeSession()
	bad, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pushman_rename_sender", Arguments: map[string]any{"nickname": " "}})
	if err != nil || !bad.IsError || service.renamed != "" {
		t.Fatal("invalid rename reached service")
	}
	callToolOK(t, session, "pushman_rename_sender", map[string]any{"nickname": " Renamed "}, "ok", true)
	if service.renamed != "Renamed" {
		t.Fatal("rename normalization changed")
	}
	bundle, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pushman_get_support_bundle", Arguments: map[string]any{}})
	if err != nil || bundle.IsError || toolTextContains(bundle, "synthetic-private-context") {
		t.Fatal("support diagnostics leaked")
	}
	callToolOK(t, session, "pushman_get_version", map[string]any{}, "Version", "test")
	callToolOK(t, session, "pushman_self_update", map[string]any{}, "ok", true)
	if updates != 1 {
		t.Fatal("update repeated")
	}
	callToolOK(t, session, "pushman_logout", map[string]any{}, "ok", true)
	if !service.loggedOut {
		t.Fatal("logout not called")
	}
}
