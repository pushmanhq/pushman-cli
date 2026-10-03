package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type supportBundleService struct{ UnconfiguredService }

func (supportBundleService) Doctor(context.Context) ([]DoctorCheck, error) {
	return []DoctorCheck{
		{Name: "api-url", OK: true, Message: "https://private.example/v1?token=secret"},
		{Name: "credential", OK: false, Message: "account acct_secret used pm_cli_secret"},
	}, nil
}

func TestSupportBundleRequiresExplicitLocalPathAndOmitsSensitiveMessages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "support.json")
	out := new(strings.Builder)
	cmd := New(Dependencies{
		Out: out,
		Service: supportBundleService{},
		Version: VersionInfo{Version: "v-test", Commit: "abc123", Date: "2026-10-03"},
		Now: func() time.Time { return time.Date(2026, 10, 3, 3, 30, 0, 0, time.UTC) },
	})
	cmd.SetArgs([]string{"support-bundle", "--output", path})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var bundle supportBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.Schema != 1 || bundle.GeneratedAt != "2026-10-03T03:30:00Z" || len(bundle.Checks) != 2 {
		t.Fatalf("bundle = %#v", bundle)
	}
	text := string(data)
	for _, secret := range []string{"private.example", "token=secret", "acct_secret", "pm_cli_secret"} {
		if strings.Contains(text, secret) {
			t.Fatalf("bundle leaked %q: %s", secret, text)
		}
	}
	if !strings.Contains(out.String(), "written locally") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestSupportBundleDoesNotOverwriteExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "support.json")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := New(Dependencies{Service: supportBundleService{}})
	cmd.SetArgs([]string{"support-bundle", "--output", path})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected existing file to be rejected")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "keep" {
		t.Fatalf("existing file changed: %q", data)
	}
}
