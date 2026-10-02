package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestLoginBrowserFailureKeepsManualApproval(t *testing.T) {
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	command := New(Dependencies{
		Out: out, ErrOut: errOut, IsTerminal: func() bool { return true },
		Hostname:    func() (string, error) { return "Test sender", nil },
		Service:     stubService{loginResult: PairResult{Nickname: "Test sender"}},
		OpenBrowser: func(string) error { return errors.New("synthetic launch failure") },
	})
	command.SetArgs([]string{"login"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Verify at:") || !strings.Contains(out.String(), "Logged in as Test sender") {
		t.Fatal("browser failure prevented the manual approval path")
	}
	if !strings.Contains(errOut.String(), "Could not open a browser; use the URL above:") {
		t.Fatal("browser failure diagnostic missing from stderr")
	}
	if strings.Contains(out.String(), "synthetic launch failure") {
		t.Fatal("browser diagnostic leaked onto stdout")
	}
}

func TestLoginSkipsBrowserWhenDisabledOrRedirected(t *testing.T) {
	for _, test := range []struct {
		name       string
		args       []string
		isTerminal bool
	}{
		{name: "explicit no-browser", args: []string{"login", "--no-browser"}, isTerminal: true},
		{name: "noninteractive", args: []string{"login"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			out, errOut := new(bytes.Buffer), new(bytes.Buffer)
			command := New(Dependencies{
				Out: out, ErrOut: errOut, IsTerminal: func() bool { return test.isTerminal },
				Hostname: func() (string, error) { return "Test sender", nil },
				Service:  stubService{loginResult: PairResult{Nickname: "Test sender"}},
				OpenBrowser: func(string) error {
					t.Error("disabled/noninteractive login opened a browser")
					return nil
				},
			})
			command.SetArgs(test.args)
			if err := command.Execute(); err != nil || !strings.Contains(out.String(), "Verify at:") || errOut.Len() != 0 {
				t.Fatal("disabled/noninteractive browser lost the manual approval path")
			}
		})
	}
}
