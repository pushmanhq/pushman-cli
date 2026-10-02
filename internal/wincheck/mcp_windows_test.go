//go:build windows

package wincheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type processSession struct {
	session               *mcp.ClientSession
	command               *exec.Cmd
	protocol, diagnostics *bytes.Buffer
}

func openProcessSession(t *testing.T, apiURL string) *processSession {
	t.Helper()
	command := cliCommand(t, apiURL, "mcp")
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: newHiddenConsole, HideWindow: true}
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	protocol, diagnostics := new(bytes.Buffer), new(bytes.Buffer)
	command.Stderr = diagnostics
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "windows-process-test", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), &mcp.IOTransport{
		Reader: io.NopCloser(io.TeeReader(stdout, protocol)), Writer: stdin,
	}, nil)
	if err != nil {
		stdin.Close()
		command.Wait()
		t.Fatal(err)
	}
	return &processSession{session, command, protocol, diagnostics}
}

func (p *processSession) close(t *testing.T) {
	t.Helper()
	if err := p.session.Close(); err != nil {
		t.Errorf("MCP EOF close: %v", err)
	}
	if err := p.command.Wait(); err != nil {
		t.Errorf("MCP process did not exit cleanly: %v", err)
	}
	if p.diagnostics.Len() != 0 {
		t.Error("MCP emitted unexpected routine diagnostics")
	}
	p.assertProtocol(t)
}

func (p *processSession) assertProtocol(t *testing.T) {
	t.Helper()
	for _, frame := range bytes.Split(p.protocol.Bytes(), []byte{'\n'}) {
		if len(bytes.TrimSpace(frame)) == 0 {
			continue
		}
		var message struct {
			JSONRPC string `json:"jsonrpc"`
		}
		if json.Unmarshal(frame, &message) != nil || message.JSONRPC != "2.0" {
			t.Error("MCP stdout contained non-protocol output")
		}
	}
}

func TestWindowsMCPProcessInterrupt(t *testing.T) {
	server, _ := pushServer(t)
	process := openProcessSession(t, server.URL)
	t.Cleanup(func() {
		process.session.Close()
		if process.command.ProcessState == nil {
			process.command.Process.Kill()
			process.command.Wait()
		}
	})
	if err := interruptOwnedProcess(t, process.command); err != nil {
		t.Fatalf("could not interrupt owned MCP process: %v", err)
	}
	err := process.command.Wait()
	process.session.Close()
	// MCP treats cancellation as clean server shutdown, unlike an interrupted
	// login command. Preserve that existing quiet exit-0 contract.
	if err != nil || process.diagnostics.Len() != 0 {
		t.Fatal("native MCP console interrupt did not shut down cleanly")
	}
	process.assertProtocol(t)
}

func TestWindowsMCPExecutableAndRestart(t *testing.T) {
	server, requests := pushServer(t)
	for run := 0; run < 2; run++ {
		process := openProcessSession(t, server.URL)
		func() {
			defer process.close(t)
			tools, err := process.session.ListTools(t.Context(), nil)
			if err != nil || len(tools.Tools) != 7 {
				t.Fatalf("actual CLI tools/list failed: %v", err)
			}
			for _, tool := range tools.Tools {
				if tool.InputSchema == nil || tool.OutputSchema == nil || tool.Annotations == nil {
					t.Fatal("actual executable did not expose schemas and annotations")
				}
			}
			result, err := process.session.CallTool(t.Context(), &mcp.CallToolParams{
				Name: "pushman_send_notification", Arguments: map[string]any{"body": "synthetic MCP 한글 😀"},
			})
			if err != nil || result.IsError {
				t.Fatalf("actual CLI send tool failed: %v", err)
			}
			if got := <-requests; got.Body != "synthetic MCP 한글 😀" {
				t.Fatal("MCP changed the synthetic body")
			}
			result, err = process.session.CallTool(t.Context(), &mcp.CallToolParams{
				Name: "pushman_send_notification", Arguments: map[string]any{"body": "synthetic", "image": "http://example.com/a.png"},
			})
			if err != nil || !result.IsError || len(requests) != 0 {
				t.Fatal("MCP validation did not reject input before the service")
			}
			result, err = process.session.CallTool(t.Context(), &mcp.CallToolParams{Name: "pushman_list_devices"})
			if err != nil || !result.IsError || len(requests) != 0 {
				t.Fatal("automation token unexpectedly authorized an account read tool")
			}
		}()
	}
}

func TestWindowsMCPClientCancellation(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	defer server.Close()
	process := openProcessSession(t, server.URL)
	defer process.close(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := process.session.CallTool(ctx, &mcp.CallToolParams{
			Name: "pushman_send_notification", Arguments: map[string]any{"body": "synthetic cancel"},
		})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("MCP call never reached loopback service")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("client cancellation returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("MCP call did not return after client cancellation")
	}
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("MCP cancellation did not cancel the API request")
	}
}
