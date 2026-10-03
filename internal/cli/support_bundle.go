package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
)

type supportBundle struct {
	Schema      int                  `json:"schema"`
	GeneratedAt string               `json:"generatedAt"`
	Version     VersionInfo          `json:"version"`
	Runtime     supportBundleRuntime `json:"runtime"`
	Checks      []supportBundleCheck `json:"checks"`
	Privacy     string               `json:"privacy"`
}

type supportBundleRuntime struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
}

type supportBundleCheck struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
}

func newSupportBundleCommand(deps Dependencies) *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "support-bundle",
		Short: "Create a local privacy-safe support bundle",
		Args: func(_ *cobra.Command, args []string) error {
			return noArgs(args)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if output == "" {
				return usagef("--output is required")
			}
			checks, err := deps.Service.Doctor(cmd.Context())
			if err != nil {
				return fmt.Errorf("collect support diagnostics: %w", err)
			}
			safeChecks := make([]supportBundleCheck, 0, len(checks))
			for _, check := range checks {
				// Doctor messages can contain endpoint or account-adjacent context.
				// Support bundles intentionally keep only the check name and boolean result.
				safeChecks = append(safeChecks, supportBundleCheck{Name: check.Name, OK: check.OK})
			}
			privacy := "Local-only. No tokens, message content, account IDs, endpoints, or diagnostic messages are included or uploaded."
			if runtime.GOOS == "windows" {
				privacy += " File access follows the destination directory ACL on Windows."
			}
			bundle := supportBundle{
				Schema:      1,
				GeneratedAt: deps.Now().UTC().Format("2006-01-02T15:04:05Z"),
				Version:     deps.Version,
				Runtime:     supportBundleRuntime{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH},
				Checks:      safeChecks,
				Privacy:     privacy,
			}
			file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err != nil {
				return fmt.Errorf("create support bundle: %w", err)
			}
			encoder := json.NewEncoder(file)
			encoder.SetIndent("", "  ")
			writeErr := encoder.Encode(bundle)
			closeErr := file.Close()
			if writeErr != nil {
				_ = os.Remove(output)
				return fmt.Errorf("write support bundle: %w", writeErr)
			}
			if closeErr != nil {
				_ = os.Remove(output)
				return fmt.Errorf("close support bundle: %w", closeErr)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Support bundle written locally to %s\n", output)
			return nil
		},
	}
	cmd.Flags().StringVar(&output, "output", "", "local path for the new support bundle JSON file")
	return cmd
}
