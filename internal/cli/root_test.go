package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestVersionUsesInjectedOutput(t *testing.T) {
	t.Parallel()
	out := new(bytes.Buffer)
	root := New(Dependencies{Out: out, Version: VersionInfo{Version: "1.2.3", Commit: "abc", Date: "2026-08-25"}})
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "1.2.3") || !strings.Contains(got, "abc") {
		t.Fatalf("version output = %q", got)
	}
}

func TestUsageKeepsLegacyLineAndSeparatesBillingWarning(t *testing.T) {
	for _, plan := range []string{"", "free", "pro", "bad\x1b[2J"} {
		out, diagnostics := new(bytes.Buffer), new(bytes.Buffer)
		svc := &mcpStubService{usage: UsageResult{Used: 9000, Limit: 200, ResetsAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Plan: plan, BillingState: "unavailable"}}
		root := New(Dependencies{Service: svc, Out: out, ErrOut: diagnostics})
		root.SetArgs([]string{"usage"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(out.String(), "9000 of 200 messages used; resets 2026-10-01T00:00:00Z\n") {
			t.Fatal(out.String())
		}
		if strings.Contains(out.String(), "could not") || strings.Contains(out.String(), "\x1b") {
			t.Fatal("unsafe stdout")
		}
		if !strings.Contains(diagnostics.String(), "does not mean a subscription was canceled") {
			t.Fatal(diagnostics.String())
		}
	}
}

func TestInvalidArgumentIsUsageError(t *testing.T) {
	t.Parallel()
	root := New(Dependencies{})
	root.SetArgs([]string{"status", "extra"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("exit code = %d, want 2", got)
	}
}
