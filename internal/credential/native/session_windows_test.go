//go:build windows

package native

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pushmanhq/pushman-cli/internal/cli"
	"github.com/pushmanhq/pushman-cli/internal/client"
	"github.com/pushmanhq/pushman-cli/internal/credential"
	"golang.org/x/sys/windows"
)

const syntheticSessionCredential = "synthetic-unavailable-session-credential"

func TestNativeKeyringAnonymousToken(t *testing.T) {
	if os.Getenv("PUSHMAN_TEST_NATIVE_KEYRING") != "1" {
		t.Skip("NOT RUN: set PUSHMAN_TEST_NATIVE_KEYRING=1 to test an isolated unavailable Windows credential set")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	service := "com.pushman.test.unavailable." + hex.EncodeToString(nonce[:])
	store := credential.NewKeyring(service)
	t.Cleanup(func() {
		if err := store.Delete(); err != nil {
			t.Errorf("isolated session entry cleanup failed: %v", err)
		}
	})
	if _, err := store.Get(); !errors.Is(err, credential.ErrNotFound) {
		t.Fatalf("isolated entry missing lookup: %v", err)
	}
	if err := store.Set(syntheticSessionCredential); err != nil {
		t.Fatalf("native write failed (unverified store): %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.v", "-test.run=^TestNativeAnonymousTokenHelper$")
	command.Env = append(os.Environ(), "PUSHMAN_TEST_UNAVAILABLE_SERVICE="+service)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("isolated unavailable-session child failed: %v\n%s", err, output)
	} else {
		t.Logf("native child evidence:\n%s", output)
	}
	if got, err := store.Get(); err != nil || got != syntheticSessionCredential {
		t.Fatal("restricted child changed the interactive session's synthetic credential")
	}
	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(); !errors.Is(err, credential.ErrNotFound) {
		t.Fatal("isolated session entry remains after cleanup")
	}
	t.Log("PASS: native unavailable credential set; account commands fail before transport; send-only automation remains separate; interactive entry preserved and removed")
}

func TestNativeAnonymousTokenHelper(t *testing.T) {
	service := os.Getenv("PUSHMAN_TEST_UNAVAILABLE_SERVICE")
	if service == "" {
		t.Skip("native unavailable-session helper")
	}
	if os.Getenv("PUSHMAN_TEST_NATIVE_KEYRING") != "1" || !strings.HasPrefix(service, "com.pushman.test.unavailable.") {
		t.Fatal("refuse a non-test credential namespace")
	}
	store := credential.NewKeyring(service)
	if got, err := store.Get(); err != nil || got != syntheticSessionCredential {
		t.Fatal("owned child could not read its isolated entry before restricting its token")
	}
	// Impersonation is confined to a locked thread in this disposable child.
	// It neither changes host policy nor creates or uses another user's login.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	thread, err := windows.OpenThread(windows.THREAD_IMPERSONATE, false, windows.GetCurrentThreadId())
	if err != nil {
		t.Fatalf("open owned thread for anonymous impersonation: %v", err)
	}
	defer windows.CloseHandle(thread)
	advapi := windows.NewLazySystemDLL("advapi32.dll")
	impersonate := advapi.NewProc("ImpersonateAnonymousToken")
	revert := advapi.NewProc("RevertToSelf")
	if ok, _, err := impersonate.Call(uintptr(thread)); ok == 0 {
		t.Fatalf("anonymous session setup failed (NOT RUN): %v", err)
	}
	defer func() {
		if ok, _, err := revert.Call(); ok == 0 {
			fmt.Fprintln(os.Stderr, "owned child could not revert anonymous impersonation:", err)
			// Never return an impersonating thread to the Go runtime.
			os.Exit(1)
		}
	}()
	_, readErr := store.Get()
	readCode := nativeCredentialFailureCode(t, readErr)
	if readCode != windows.ERROR_NO_SUCH_LOGON_SESSION && readCode != windows.ERROR_ACCESS_DENIED && readCode != windows.Errno(1702) {
		t.Fatalf("anonymous credential read returned an unexpected native failure: %d", readCode)
	}
	// The same valid Set inputs succeeded before impersonation. Different
	// credential APIs can report different native failures for an anonymous
	// token; preserve those errors rather than recasting them as missing data.
	writeCode := nativeCredentialFailureCode(t, store.Set(syntheticSessionCredential))
	deleteCode := nativeCredentialFailureCode(t, store.Delete())
	t.Logf("native restricted token errors: read=%d write=%d delete=%d", readCode, writeCode, deleteCode)
	transport := &unavailableSessionTransport{}
	apiClient := &http.Client{Transport: transport, Timeout: time.Second}
	accountService, err := client.New("http://127.0.0.1:1/v1", store, "", apiClient)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"status"}, {"devices"}, {"login", "--no-browser"}, {"logout"}} {
		var stdout, stderr bytes.Buffer
		app := cli.New(cli.Dependencies{Service: accountService, In: strings.NewReader(""), Out: &stdout, ErrOut: &stderr,
			IsTerminal: func() bool { return false }, Hostname: func() (string, error) { return "synthetic-native-host", nil }})
		app.SetArgs(args)
		err := app.ExecuteContext(t.Context())
		if cli.ExitCode(err) != 1 || !errors.Is(err, readCode) || stdout.Len() != 0 {
			t.Fatalf("%s did not preserve the native unavailable-store failure", args[0])
		}
	}
	if transport.requests.Load() != 0 {
		t.Fatal("account command attempted transport after the native credential error")
	}
	const automationToken = "synthetic-session-send-only-token"
	transport.acceptSend = true
	automationService, err := client.New("http://127.0.0.1:1/v1", store, automationToken, apiClient)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	app := cli.New(cli.Dependencies{Service: automationService, In: strings.NewReader(""), Out: &stdout, ErrOut: &stderr,
		IsTerminal: func() bool { return true }})
	app.SetArgs([]string{"push", "synthetic native session fixture", "--json"})
	if err := app.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("send-only automation depended on the unavailable account store: %v", err)
	}
	var result cli.PushResult
	if json.Unmarshal(stdout.Bytes(), &result) != nil || result.Status != "accepted" || result.DeviceCount != 1 ||
		stderr.Len() != 0 || transport.requests.Load() != 1 || bytes.Contains(stdout.Bytes(), []byte(automationToken)) {
		t.Fatal("send-only automation did not preserve the JSON/output/transport boundary")
	}
}

func nativeCredentialFailureCode(t *testing.T, err error) windows.Errno {
	t.Helper()
	var code windows.Errno
	if errors.Is(err, credential.ErrNotFound) || !errors.As(err, &code) || code == 0 || code == windows.ERROR_NOT_FOUND {
		t.Fatalf("expected a native credential failure, not missing data or success: %v", err)
	}
	return code
}

type unavailableSessionTransport struct {
	requests   atomic.Int32
	acceptSend bool
}

func (transport *unavailableSessionTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.requests.Add(1)
	if !transport.acceptSend || request.Method != http.MethodPost || request.URL.Path != "/v1/messages" ||
		request.Header.Get("Authorization") != "Bearer synthetic-session-send-only-token" {
		return nil, errors.New("refuse unexpected synthetic transport request")
	}
	// The stub never opens a socket, even if a command erroneously reaches it.
	return &http.Response{StatusCode: http.StatusAccepted, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"id":"msg_test","logicalMessageId":"lmsg_test","acceptedAt":"2026-10-02T00:00:00Z","targetCount":1}`))}, nil
}
