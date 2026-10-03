package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestReadBody(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, stdin string
		terminal    bool
		args        []string
		want        string
		wantErr     bool
	}{
		{name: "argument", terminal: true, args: []string{"done"}, want: "done"},
		{name: "automatic stdin", stdin: "line one\nline two\n", args: nil, want: "line one\nline two"},
		{name: "explicit stdin", stdin: "done\r\n", terminal: true, args: []string{"-"}, want: "done"},
		{name: "remove only one newline", stdin: "done\n\n", args: []string{"-"}, want: "done\n"},
		{name: "missing interactive body", terminal: true, wantErr: true},
		{name: "argument and pipe", stdin: "ignored", args: []string{"argument"}, wantErr: true},
		{name: "whitespace", terminal: true, args: []string{" \t"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readBody(strings.NewReader(tt.stdin), tt.terminal, tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("readBody() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("readBody() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidatePushOptions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		opts    pushOptions
		wantErr bool
	}{
		{name: "defaults", opts: pushOptions{sound: "default"}},
		{name: "valid", opts: pushOptions{sound: "none", url: "myapp://message/1", image: "https://example.com/a.png", group: "deploy:prod", key: "build-1", devices: []string{"iPhone", "iPad"}}},
		{name: "json quiet", opts: pushOptions{sound: "default", json: true, quiet: true}, wantErr: true},
		{name: "invalid sound", opts: pushOptions{sound: "loud"}, wantErr: true},
		{name: "invalid key", opts: pushOptions{sound: "default", key: "space here"}, wantErr: true},
		{name: "insecure image", opts: pushOptions{sound: "default", image: "http://example.com/a.png"}, wantErr: true},
		{name: "blocked URL", opts: pushOptions{sound: "default", url: "javascript:alert(1)"}, wantErr: true},
		{name: "hostless web URL", opts: pushOptions{sound: "default", url: "https:/message/1"}, wantErr: true},
		{name: "local image", opts: pushOptions{sound: "default", image: "https://localhost/a.png"}, wantErr: true},
		{name: "duplicate device", opts: pushOptions{sound: "default", devices: []string{"Phone", "phone"}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validatePushOptions(tt.opts); (got != nil) != tt.wantErr {
				t.Fatalf("validatePushOptions() error = %v, wantErr %v", got, tt.wantErr)
			}
		})
	}
}

func TestPushOutput(t *testing.T) {
	t.Parallel()
	service := stubService{pushResult: PushResult{ID: "msg_123", Status: "accepted", DeviceCount: 1, AcceptedAt: time.Date(2026, 8, 25, 1, 2, 3, 0, time.UTC)}}
	out := new(bytes.Buffer)
	cmd := New(Dependencies{In: strings.NewReader(""), Out: out, IsTerminal: func() bool { return true }, Service: service})
	cmd.SetArgs([]string{"push", "complete"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "Accepted msg_123 for 1 device\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestPairOutput(t *testing.T) {
	t.Parallel()
	service := stubService{pairResult: PairResult{Nickname: "Build Mac"}}
	out := new(bytes.Buffer)
	cmd := New(Dependencies{Out: out, Hostname: func() (string, error) { return "Build Mac", nil }, Service: service})
	cmd.SetArgs([]string{"pair"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	want := "Pairing code: ABCD-EFGH\nVerify at: https://app.pushman.example/pair/?code=ABCD-EFGH\nPaired as Build Mac\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestLoginOpensCompleteURLOnlyForInteractiveTerminal(t *testing.T) {
	t.Parallel()
	service := stubService{loginResult: PairResult{Nickname: "Build Mac"}}
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	var opened string
	cmd := New(Dependencies{
		Out: out, ErrOut: errOut, IsTerminal: func() bool { return true },
		Hostname: func() (string, error) { return "Build Mac", nil }, Service: service,
		OpenBrowser: func(value string) error { opened = value; return nil },
	})
	cmd.SetArgs([]string{"login"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	want := "Login code: ABCD-EFGH\nVerify at: https://app.pushman.example/activate/\nLogged in as Build Mac\n"
	if out.String() != want || opened != "https://app.pushman.example/activate/?user_code=ABCD-EFGH" {
		t.Fatalf("output/opened = %q / %q", out.String(), opened)
	}
}

func TestLoginDoesNotOpenBrowserWhenDisabledOrNonInteractive(t *testing.T) {
	for _, test := range []struct {
		name     string
		terminal bool
		args     []string
	}{
		{name: "disabled", terminal: true, args: []string{"login", "--no-browser"}},
		{name: "non-interactive", terminal: false, args: []string{"login"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			opened := false
			cmd := New(Dependencies{
				Out: new(bytes.Buffer), IsTerminal: func() bool { return test.terminal },
				Hostname:    func() (string, error) { return "Build Mac", nil },
				Service:     stubService{loginResult: PairResult{Nickname: "Build Mac"}},
				OpenBrowser: func(string) error { opened = true; return nil },
			})
			cmd.SetArgs(test.args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if opened {
				t.Fatal("browser was opened")
			}
		})
	}
}

func TestPairingVerificationURL(t *testing.T) {
	t.Parallel()
	got, err := pairingVerificationURL("https://app.pushman.example/pair/?source=cli#ignored", "ABCD-EFGH")
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://app.pushman.example/pair/?code=ABCD-EFGH&source=cli"; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
	if _, err := pairingVerificationURL("/pair", "ABCD-EFGH"); err == nil {
		t.Fatal("relative verification URL accepted")
	}
}

func TestCommandSurface(t *testing.T) {
	t.Parallel()
	root := New(Dependencies{})
	for _, path := range [][]string{{"pair"}, {"login"}, {"status"}, {"rename"}, {"logout"}, {"push"}, {"devices"}, {"history"}, {"history", "show"}, {"usage"}, {"doctor"}, {"support-bundle"}, {"mcp"}, {"self-update"}, {"version"}, {"help"}} {
		if _, _, err := root.Find(path); err != nil {
			t.Errorf("command %q missing: %v", strings.Join(path, " "), err)
		}
	}
	if command, _, err := root.Find([]string{"completion"}); err == nil && command.Name() == "completion" {
		t.Fatal("unexpected completion command")
	}
}

func TestSelfUpdateOutput(t *testing.T) {
	t.Parallel()
	out := new(bytes.Buffer)
	cmd := New(Dependencies{
		Out: out,
		SelfUpdate: func(context.Context) (string, error) {
			return "Pushman is already up to date.", nil
		},
	})
	cmd.SetArgs([]string{"self-update"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "Pushman is already up to date.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestSelfUpdateError(t *testing.T) {
	t.Parallel()
	want := errors.New("upgrade failed")
	cmd := New(Dependencies{
		SelfUpdate: func(context.Context) (string, error) { return "", want },
	})
	cmd.SetArgs([]string{"self-update"})
	if err := cmd.Execute(); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestNormalizeSuggestedName(t *testing.T) {
	t.Parallel()
	got, err := normalizeSuggestedName("  " + strings.Repeat("가", 70) + "  ")
	if err != nil {
		t.Fatal(err)
	}
	if countScalars(got) != 64 {
		t.Fatalf("scalar count = %d", countScalars(got))
	}
	if _, err := normalizeSuggestedName(" \t "); err == nil {
		t.Fatal("expected empty hostname to fail")
	}
}

func TestExitCode(t *testing.T) {
	t.Parallel()
	if got := ExitCode(usagef("bad input")); got != 2 {
		t.Fatalf("usage exit = %d", got)
	}
	if got := ExitCode(&AuthorizationError{Err: context.Canceled}); got != 4 {
		t.Fatalf("auth exit = %d", got)
	}
	if got := ExitCode(context.Canceled); got != 130 {
		t.Fatalf("interrupt exit = %d", got)
	}
}

type stubService struct {
	UnconfiguredService
	pushResult  PushResult
	pairResult  PairResult
	loginResult PairResult
}

func (s stubService) Login(_ context.Context, request LoginRequest) (PairResult, error) {
	if request.OnChallenge != nil {
		if err := request.OnChallenge(LoginChallenge{
			UserCode: "ABCD-EFGH", VerificationURL: "https://app.pushman.example/activate/",
			CompleteURL: "https://app.pushman.example/activate/?user_code=ABCD-EFGH",
		}); err != nil {
			return PairResult{}, err
		}
	}
	return s.loginResult, nil
}

func (s stubService) Push(context.Context, PushRequest) (PushResult, error) { return s.pushResult, nil }
func (s stubService) Pair(_ context.Context, request PairRequest) (PairResult, error) {
	if request.OnChallenge != nil {
		if err := request.OnChallenge(PairChallenge{UserCode: "ABCD-EFGH", VerificationURL: "https://app.pushman.example/pair"}); err != nil {
			return PairResult{}, err
		}
	}
	return s.pairResult, nil
}
