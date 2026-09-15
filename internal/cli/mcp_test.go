package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPToolSurfaceAndCalls(t *testing.T) {
	t.Parallel()
	acceptedAt := time.Date(2026, 8, 25, 1, 2, 3, 456, time.UTC)
	service := &mcpStubService{
		pushResult: PushResult{ID: "msg_123", Status: "accepted", DeviceCount: 1, AcceptedAt: acceptedAt},
		devices:    []Device{{Nickname: "iPhone", Status: "eligible"}},
		history: []HistoryItem{{
			ID: "msg_123", Title: "Deploy", UpdatedAt: acceptedAt, UpdateCount: 1, DeliveryState: "delivered",
		}},
		detail: HistoryDetail{LogicalMessageID: "logical_123", Read: true, Revisions: []HistoryRevision{{
			ID: "msg_123", Title: "Deploy", Body: "complete", SenderName: "Build Mac",
			Sound: "default", Format: "plain", UpdatedAt: acceptedAt,
			Deliveries: []HistoryDelivery{{DeviceName: "iPhone", State: "delivered"}},
		}}},
		usage:  UsageResult{Used: 12, Limit: 200, ResetsAt: acceptedAt, Plan: "free", BillingState: "unavailable"},
		status: StatusResult{Paired: true, Nickname: "Build Mac"},
		checks: []DoctorCheck{{Name: "credential", OK: true, Message: "available"}},
	}

	session, closeSession := connectMCPTestClient(t, service)
	defer closeSession()

	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(listed.Tools), 7; got != want {
		t.Fatalf("tool count = %d, want %d", got, want)
	}
	tools := make(map[string]*mcp.Tool, len(listed.Tools))
	for _, tool := range listed.Tools {
		tools[tool.Name] = tool
	}
	for _, name := range []string{
		"pushman_send_notification", "pushman_list_devices", "pushman_list_history",
		"pushman_get_message", "pushman_get_usage", "pushman_get_status", "pushman_doctor",
	} {
		tool := tools[name]
		if tool == nil {
			t.Fatalf("tool %q missing", name)
		}
		if tool.InputSchema == nil || tool.OutputSchema == nil {
			t.Fatalf("tool %q is missing an input or output schema", name)
		}
		if tool.Annotations == nil || tool.Annotations.OpenWorldHint == nil || !*tool.Annotations.OpenWorldHint {
			t.Fatalf("tool %q has incomplete open-world annotations: %#v", name, tool.Annotations)
		}
	}
	sendTool := tools["pushman_send_notification"]
	if sendTool.Annotations.ReadOnlyHint || sendTool.Annotations.DestructiveHint == nil || *sendTool.Annotations.DestructiveHint || sendTool.Annotations.IdempotentHint {
		t.Fatalf("send annotations = %#v", sendTool.Annotations)
	}
	schemaJSON, err := json.Marshal(sendTool.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	for _, contract := range []string{`"required":["body"]`, `"enum":["default","none"]`, `"enum":["plain","monospace"]`, `"maxLength":4096`} {
		if !strings.Contains(string(schemaJSON), contract) {
			t.Fatalf("send schema %s is missing %s", schemaJSON, contract)
		}
	}
	for name, tool := range tools {
		if name != "pushman_send_notification" && !tool.Annotations.ReadOnlyHint {
			t.Fatalf("read tool %q is not marked read-only", name)
		}
	}

	callToolOK(t, session, "pushman_send_notification", map[string]any{
		"body": "complete", "title": "Deploy", "sound": "none", "format": "monospace", "devices": []string{"iPhone"},
	}, "id", "msg_123")
	if service.lastPush.Body != "complete" || service.lastPush.Sound != "none" || service.lastPush.Format != "monospace" {
		t.Fatalf("push request = %#v", service.lastPush)
	}
	callToolOK(t, session, "pushman_list_devices", map[string]any{}, "devices", nil)
	callToolOK(t, session, "pushman_list_history", map[string]any{}, "messages", nil)
	callToolOK(t, session, "pushman_get_message", map[string]any{"messageId": "msg_123"}, "logicalMessageId", "logical_123")
	callToolOK(t, session, "pushman_get_usage", map[string]any{}, "limit", float64(200))
	callToolOK(t, session, "pushman_get_usage", map[string]any{}, "plan", "free")
	callToolOK(t, session, "pushman_get_usage", map[string]any{}, "billingState", "unavailable")
	callToolOK(t, session, "pushman_get_status", map[string]any{}, "nickname", "Build Mac")
	callToolOK(t, session, "pushman_doctor", map[string]any{}, "ok", true)
}

func TestMCPSendUsesCLIValidationAndToolErrors(t *testing.T) {
	t.Parallel()
	service := &mcpStubService{}
	session, closeSession := connectMCPTestClient(t, service)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "pushman_send_notification", Arguments: map[string]any{"body": "hello", "image": "http://example.com/a.png"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !toolTextContains(result, "image must be an absolute HTTPS URL") {
		t.Fatalf("validation result = %#v", result)
	}
	if service.pushCalls != 0 {
		t.Fatalf("Push called %d times after validation failure", service.pushCalls)
	}

	service.pushErr = errors.New("service unavailable")
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "pushman_send_notification", Arguments: map[string]any{"body": "hello"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !toolTextContains(result, "service unavailable") {
		t.Fatalf("service error result = %#v", result)
	}
}

func TestMCPStdioSubprocessNegotiatesAndStopsAtEOF(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=TestMCPHelperProcess")
	command.Env = append(os.Environ(), "PUSHMAN_MCP_HELPER=1")
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}

	request := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28","capabilities":{},"clientInfo":{"name":"pushman-test","version":"1"}}}` + "\n"
	if _, err := stdin.Write([]byte(request)); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(line) {
		t.Fatalf("stdout is not protocol JSON: %q", line)
	}
	var response struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			ServerInfo struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	if err := json.Unmarshal(line, &response); err != nil {
		t.Fatal(err)
	}
	if response.JSONRPC != "2.0" || response.ID != 1 || response.Result.ServerInfo.Name != "pushman" {
		t.Fatalf("initialize response = %s", line)
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	diagnostic, err := io.ReadAll(stderr)
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	if len(diagnostic) != 0 {
		t.Fatalf("unexpected stderr: %q", diagnostic)
	}
}

func TestMCPCommandStopsOnCancellation(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	root := New(Dependencies{In: reader, Service: &mcpStubService{}, Version: VersionInfo{Version: "test"}})
	root.SetArgs([]string{"mcp"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- root.ExecuteContext(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("MCP command did not stop after cancellation")
	}
}

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("PUSHMAN_MCP_HELPER") != "1" {
		return
	}
	root := New(Dependencies{
		In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr,
		Service: &mcpStubService{}, Version: VersionInfo{Version: "test"},
	})
	root.SetArgs([]string{"mcp"})
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func connectMCPTestClient(t *testing.T, service Service) (*mcp.ClientSession, func()) {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	server := newMCPServer(Dependencies{Service: service, Version: VersionInfo{Version: "test"}})
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "pushman-test", Version: "1"}, nil)
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		serverSession.Close()
		t.Fatal(err)
	}
	return clientSession, func() {
		clientSession.Close()
		serverSession.Close()
	}
}

func callToolOK(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any, field string, want any) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("tool %q failed: %#v", name, result.Content)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var output map[string]any
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatal(err)
	}
	if _, ok := output[field]; !ok {
		t.Fatalf("tool %q output %s is missing %q", name, encoded, field)
	}
	if want != nil && output[field] != want {
		t.Fatalf("tool %q output[%q] = %#v, want %#v", name, field, output[field], want)
	}
}

func toolTextContains(result *mcp.CallToolResult, substring string) bool {
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok && strings.Contains(text.Text, substring) {
			return true
		}
	}
	return false
}

type mcpStubService struct {
	UnconfiguredService
	pushResult PushResult
	pushErr    error
	pushCalls  int
	lastPush   PushRequest
	devices    []Device
	history    []HistoryItem
	detail     HistoryDetail
	usage      UsageResult
	status     StatusResult
	checks     []DoctorCheck
}

func (s *mcpStubService) Push(_ context.Context, request PushRequest) (PushResult, error) {
	s.pushCalls++
	s.lastPush = request
	return s.pushResult, s.pushErr
}

func (s *mcpStubService) Devices(context.Context) ([]Device, error)      { return s.devices, nil }
func (s *mcpStubService) History(context.Context) ([]HistoryItem, error) { return s.history, nil }
func (s *mcpStubService) HistoryShow(context.Context, string) (HistoryDetail, error) {
	return s.detail, nil
}
func (s *mcpStubService) Usage(context.Context) (UsageResult, error)    { return s.usage, nil }
func (s *mcpStubService) Status(context.Context) (StatusResult, error)  { return s.status, nil }
func (s *mcpStubService) Doctor(context.Context) ([]DoctorCheck, error) { return s.checks, nil }
