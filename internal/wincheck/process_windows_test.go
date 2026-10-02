//go:build windows

package wincheck

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf16"
)

const syntheticToken = "synthetic-windows-test-token"

var executable string

func TestMain(m *testing.M) {
	if mode := os.Getenv("PUSHMAN_WIN_CONSOLE_HELPER"); mode != "" {
		os.Exit(consoleHelper(mode))
	}
	directory, err := os.MkdirTemp("", "pushman-windows-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create Windows test directory:", err)
		os.Exit(1)
	}
	code := func() int {
		defer os.RemoveAll(directory)
		// Exercise the actual main package, including a Unicode/spaced path,
		// with an isolated credential namespace and no installed CLI changes.
		binDir := filepath.Join(directory, "CLI path 한글")
		if err := os.Mkdir(binDir, 0700); err != nil {
			fmt.Fprintln(os.Stderr, "create executable directory:", err)
			return 1
		}
		executable = filepath.Join(binDir, "pushman.exe")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		build := exec.CommandContext(ctx, "go", "build", "-trimpath",
			"-ldflags", "-X main.version=windows-test -X main.credentialNamespace=windows-test",
			"-o", executable, "./cmd/pushman")
		build.Dir = filepath.Join("..", "..")
		if output, err := build.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "build Windows test CLI: %v\n%s", err, output)
			return 1
		}
		fmt.Printf("Native CLI process tests: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		return m.Run()
	}()
	os.Exit(code)
}

func isolatedEnvironment(apiURL string) []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "PUSHMAN_API_URL", "PUSHMAN_TOKEN", "PUSHMAN_DEV_TOKEN":
			continue
		}
		env = append(env, entry)
	}
	return append(env, "PUSHMAN_API_URL="+apiURL+"/v1", "PUSHMAN_TOKEN="+syntheticToken)
}

func runCommand(t *testing.T, command *exec.Cmd) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	code := 0
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else {
			t.Fatalf("could not run test process: %v", err)
		}
	}
	return stdout.String(), stderr.String(), code
}

func cliCommand(t *testing.T, apiURL string, args ...string) *exec.Cmd {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	t.Cleanup(cancel)
	command := exec.CommandContext(ctx, executable, args...)
	command.Env = isolatedEnvironment(apiURL)
	return command
}

type capturedPush struct {
	Body, Title, URL string
}

func pushServer(t *testing.T) (*httptest.Server, <-chan capturedPush) {
	t.Helper()
	requests := make(chan capturedPush, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" || r.Header.Get("Authorization") != "Bearer "+syntheticToken {
			t.Error("unexpected loopback API request or synthetic authorization")
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		var input struct {
			Body  string `json:"body"`
			Title string `json:"title"`
			URL   string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error("could not decode loopback push body")
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		requests <- capturedPush{input.Body, input.Title, input.URL}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, `{"id":"msg_test","logicalMessageId":"lmsg_test","acceptedAt":"2026-10-02T00:00:00Z","targetCount":1}`)
	}))
	t.Cleanup(server.Close)
	return server, requests
}

func assertAcceptedJSON(t *testing.T, stdout, stderr string, code int) {
	t.Helper()
	var result struct {
		ID, Status  string
		DeviceCount int
	}
	if code != 0 || stderr != "" || !json.Valid([]byte(stdout)) || json.Unmarshal([]byte(stdout), &result) != nil ||
		result.ID != "msg_test" || result.Status != "accepted" || result.DeviceCount != 1 {
		t.Fatalf("JSON success contract failed: exit=%d validJSON=%v stderrEmpty=%v", code, json.Valid([]byte(stdout)), stderr == "")
	}
}

func psLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func powerShellCommand(t *testing.T, shell, script, apiURL string) *exec.Cmd {
	t.Helper()
	words := utf16.Encode([]rune(script))
	data := make([]byte, 2*len(words))
	for index, word := range words {
		binary.LittleEndian.PutUint16(data[2*index:], word)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	t.Cleanup(cancel)
	command := exec.CommandContext(ctx, shell, "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(data))
	command.Env = isolatedEnvironment(apiURL)
	return command
}

func TestWindowsPowerShellUTF8AndQuoting(t *testing.T) {
	body := "한글 😀 spaces \"quotes\" `ticks` $dollar & amp\nsecond line"
	title := "한글 title 😀"
	targetURL := "https://example.com/?a=1&b=$two"
	for _, shell := range []string{"pwsh", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			if _, err := exec.LookPath(shell); err != nil {
				t.Skipf("NOT RUN: %s unavailable", shell)
			}
			server, requests := pushServer(t)
			script := "$ProgressPreference = 'SilentlyContinue'\n$OutputEncoding = [System.Text.UTF8Encoding]::new($false)\n" +
				psLiteral(body) + " | & " + psLiteral(executable) + " push - --json --title " + psLiteral(title) + " --url " + psLiteral(targetURL) + "\nexit $LASTEXITCODE"
			stdout, stderr, code := runCommand(t, powerShellCommand(t, shell, script, server.URL))
			assertAcceptedJSON(t, stdout, stderr, code)
			select {
			case got := <-requests:
				if got != (capturedPush{body, title, targetURL}) {
					t.Fatal("shell changed the Unicode body, title, or quoted URL")
				}
			default:
				t.Fatal("shell did not reach the loopback service")
			}
		})
	}
}

func TestWindowsCmdUTF8FileAndQuoting(t *testing.T) {
	server, requests := pushServer(t)
	body := "한글 😀 \"quotes\" `ticks` $dollar & amp\r\nsecond line"
	input := filepath.Join(t.TempDir(), "input file 한글.txt")
	if err := os.WriteFile(input, []byte(body+"\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	line := `""` + executable + `" push - --json --title "한글 title 😀" --url "https://example.com/?a=1&b=$two" < "` + input + `""`
	// cmd has its own command-line quoting rules; bypass Go's CommandLineToArgvW
	// quoting rather than escaping quotes with backslashes for this shell.
	command := exec.CommandContext(ctx, "cmd")
	command.SysProcAttr = &syscall.SysProcAttr{CmdLine: "cmd.exe /d /s /c " + line}
	command.Env = isolatedEnvironment(server.URL)
	stdout, stderr, code := runCommand(t, command)
	assertAcceptedJSON(t, stdout, stderr, code)
	got := <-requests
	if got != (capturedPush{body, "한글 title 😀", "https://example.com/?a=1&b=$two"}) {
		t.Fatal("cmd changed UTF-8 file input or quoted arguments")
	}
}

func TestWindowsProcessInputAndOutput(t *testing.T) {
	fullBody := strings.Repeat("😀", 4096)
	for _, test := range []struct {
		name, input, want string
		args              []string
		wantExit          int
		quiet             bool
	}{
		{name: "maximum LF", input: fullBody + "\n", want: fullBody},
		{name: "maximum CRLF", input: fullBody + "\r\n", want: fullBody},
		{name: "one LF only", input: "one\n\n", want: "one\n"},
		{name: "BOM remains content", input: "\ufeffhello\r\n", want: "\ufeffhello"},
		{name: "quiet", input: "hello\r\n", want: "hello", quiet: true},
		{name: "too many scalars", input: fullBody + "x", wantExit: 2},
		{name: "invalid UTF-8", input: "hello\xff", wantExit: 2},
		{name: "UTF-16 file", input: "\xff\xfeh\x00i\x00", wantExit: 2},
		{name: "empty EOF", wantExit: 2},
		{name: "body and pipe", input: "piped", args: []string{"push", "argument"}, wantExit: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, requests := pushServer(t)
			args := test.args
			if len(args) == 0 {
				args = []string{"push", "-", "--json"}
				if test.quiet {
					args = []string{"push", "-", "--quiet"}
				}
			}
			command := cliCommand(t, server.URL, args...)
			command.Stdin = strings.NewReader(test.input)
			stdout, stderr, code := runCommand(t, command)
			if test.wantExit != 0 {
				if code != test.wantExit || stdout != "" || stderr == "" || len(requests) != 0 {
					t.Fatalf("invalid input contract failed: exit=%d API calls=%d", code, len(requests))
				}
				return
			}
			if test.quiet {
				if code != 0 || stdout != "" || stderr != "" {
					t.Fatal("quiet success produced output or failed")
				}
			} else {
				assertAcceptedJSON(t, stdout, stderr, code)
			}
			if got := <-requests; got.Body != test.want {
				t.Fatal("process changed the normalized body")
			}
		})
	}
}

func TestWindowsFileAndNULHandles(t *testing.T) {
	server, requests := pushServer(t)
	path := filepath.Join(t.TempDir(), "stdin.txt")
	if err := os.WriteFile(path, []byte("file body\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	command := cliCommand(t, server.URL, "push", "--json")
	command.Stdin = file
	stdout, stderr, code := runCommand(t, command)
	assertAcceptedJSON(t, stdout, stderr, code)
	if got := <-requests; got.Body != "file body" {
		t.Fatal("redirected file input changed")
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	command = cliCommand(t, server.URL, "push", "argument body", "--json")
	command.Stdin = null
	stdout, stderr, code = runCommand(t, command)
	assertAcceptedJSON(t, stdout, stderr, code)
	if got := <-requests; got.Body != "argument body" {
		t.Fatal("NUL prevented the body argument")
	}
}
