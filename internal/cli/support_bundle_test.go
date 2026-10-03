package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type supportBundleService struct {
	UnconfiguredService
	doctorCalls int
}

func (s *supportBundleService) Doctor(context.Context) ([]DoctorCheck, error) {
	s.doctorCalls++
	return []DoctorCheck{
		{Name: "api-url", OK: true, Message: "https://private.example/v1?token=secret"},
		{Name: "credential", OK: false, Message: "account acct_secret used pm_cli_secret"},
	}, nil
}

func TestSupportBundleRequiresExplicitLocalPathAndOmitsSensitiveMessages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "support.json")
	out := new(strings.Builder)
	cmd := New(Dependencies{
		Out:     out,
		Service: &supportBundleService{},
		Version: VersionInfo{Version: "v-test", Commit: "abc123", Date: "2026-10-03"},
		Now:     func() time.Time { return time.Date(2026, 10, 3, 3, 30, 0, 0, time.UTC) },
	})
	cmd.SetArgs([]string{"support-bundle", "--output", path})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
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
	if runtime.GOOS == "windows" && !strings.Contains(bundle.Privacy, "destination directory ACL") {
		t.Fatalf("Windows privacy contract missing: %q", bundle.Privacy)
	}
	text := string(data)
	for _, secret := range []string{"private.example", "token=secret", "acct_secret", "pm_cli_secret"} {
		if strings.Contains(text, secret) {
			t.Fatalf("bundle leaked %q: %s", secret, text)
		}
	}
	if out.String() != fmt.Sprintf("Support bundle written locally to %s\n", path) {
		t.Fatalf("output = %q", out.String())
	}
}

func TestSupportBundleDoesNotOverwriteExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "support.json")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := New(Dependencies{Service: &supportBundleService{}})
	cmd.SetArgs([]string{"support-bundle", "--output", path})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected existing file to be rejected")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "keep" {
		t.Fatalf("existing file changed: %q", data)
	}
}

type supportBundleWriterFunc func([]byte) (int, error)

func (f supportBundleWriterFunc) Write(p []byte) (int, error) { return f(p) }

func TestSupportBundleOutputFailurePreservesCompletedFile(t *testing.T) {
	writeErr := errors.New("output unavailable")
	for _, tt := range []struct {
		name   string
		writer func(*testing.T) io.Writer
		want   error
	}{
		{
			name: "failed write",
			writer: func(*testing.T) io.Writer {
				return supportBundleWriterFunc(func([]byte) (int, error) { return 0, writeErr })
			},
			want: writeErr,
		},
		{
			name: "partial write with error",
			writer: func(*testing.T) io.Writer {
				return supportBundleWriterFunc(func(p []byte) (int, error) { return len(p) / 2, writeErr })
			},
			want: writeErr,
		},
		{
			name: "short write without error",
			writer: func(*testing.T) io.Writer {
				// Deliberately violate io.Writer's contract to verify defensive detection.
				return supportBundleWriterFunc(func(p []byte) (int, error) { return len(p) - 1, nil })
			},
			want: io.ErrShortWrite,
		},
		{
			name: "zero write without error",
			writer: func(*testing.T) io.Writer {
				return supportBundleWriterFunc(func([]byte) (int, error) { return 0, nil })
			},
			want: io.ErrShortWrite,
		},
		{
			name: "full write with error",
			writer: func(*testing.T) io.Writer {
				return supportBundleWriterFunc(func(p []byte) (int, error) { return len(p), writeErr })
			},
			want: writeErr,
		},
		{
			name: "closed file",
			writer: func(t *testing.T) io.Writer {
				file, err := os.CreateTemp(t.TempDir(), "closed-output")
				if err != nil {
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
				return file
			},
			want: os.ErrClosed,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "support.json")
			service := &supportBundleService{}
			writer := tt.writer(t)
			var saved []byte
			outputCalls := 0
			cmd := New(Dependencies{
				Service: service,
				Out: supportBundleWriterFunc(func(p []byte) (int, error) {
					outputCalls++
					var err error
					saved, err = os.ReadFile(path)
					if err != nil {
						t.Fatalf("read completed bundle before confirmation: %v", err)
					}
					return writer.Write(p)
				}),
			})
			cmd.SetArgs([]string{"support-bundle", "--output", path})
			err := cmd.Execute()
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if ExitCode(err) != 1 || !strings.Contains(err.Error(), "support bundle saved locally to "+path) {
				t.Fatalf("error must distinguish a saved bundle from failed confirmation: %v", err)
			}
			if service.doctorCalls != 1 || outputCalls != 1 {
				t.Fatalf("Doctor calls = %d, output attempts = %d; want one each", service.doctorCalls, outputCalls)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("completed bundle was not preserved: %v", err)
			}
			if !bytes.Equal(data, saved) {
				t.Fatal("completed bundle changed after failed confirmation")
			}
			var bundle supportBundle
			if err := json.Unmarshal(data, &bundle); err != nil {
				t.Fatalf("completed bundle is not valid JSON: %v", err)
			}
			if bundle.Schema != 1 || len(bundle.Checks) != 2 {
				t.Fatalf("bundle = %#v", bundle)
			}
			for _, secret := range []string{"private.example", "token=secret", "acct_secret", "pm_cli_secret"} {
				if bytes.Contains(data, []byte(secret)) {
					t.Fatalf("bundle leaked %q", secret)
				}
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
				t.Fatalf("mode = %o", info.Mode().Perm())
			}
		})
	}
}
