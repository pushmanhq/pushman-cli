package cli

import (
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"golang.org/x/text/unicode/norm"
)

func newPairCommand(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{Use: "pair", Short: "Pair this CLI with Pushman", Args: func(_ *cobra.Command, args []string) error { return noArgs(args) }, RunE: func(cmd *cobra.Command, _ []string) error {
		hostname, err := deps.Hostname()
		if err != nil {
			return fmt.Errorf("read hostname for pairing: %w", err)
		}
		suggestedName, err := normalizeSuggestedName(hostname)
		if err != nil {
			return err
		}
		result, err := deps.Service.Pair(cmd.Context(), PairRequest{
			Platform: runtime.GOOS, SuggestedName: suggestedName,
			OnChallenge: func(challenge PairChallenge) error {
				verificationURL, err := pairingVerificationURL(challenge.VerificationURL, challenge.UserCode)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Pairing code: %s\nVerify at: %s\n", challenge.UserCode, verificationURL)
				return err
			},
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Paired as %s\n", result.Nickname)
		return nil
	}}
	return cmd
}

func newLoginCommand(deps Dependencies) *cobra.Command {
	var noBrowser bool
	cmd := &cobra.Command{Use: "login", Short: "Log this CLI into a Pushman account", Args: func(_ *cobra.Command, args []string) error { return noArgs(args) }, RunE: func(cmd *cobra.Command, _ []string) error {
		hostname, err := deps.Hostname()
		if err != nil {
			return fmt.Errorf("read hostname for login: %w", err)
		}
		suggestedName, err := normalizeSuggestedName(hostname)
		if err != nil {
			return err
		}
		result, err := deps.Service.Login(cmd.Context(), LoginRequest{
			Platform: runtime.GOOS, SuggestedName: suggestedName,
			OnChallenge: func(challenge LoginChallenge) error {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Login code: %s\nVerify at: %s\n", challenge.UserCode, challenge.VerificationURL); err != nil {
					return err
				}
				if noBrowser || !deps.IsTerminal() {
					return nil
				}
				if err := deps.OpenBrowser(challenge.CompleteURL); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Could not open a browser; use the URL above: %v\n", err)
				}
				return nil
			},
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Logged in as %s\n", result.Nickname)
		return nil
	}}
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "Do not open the verification page")
	return cmd
}

func pairingVerificationURL(rawURL, code string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return "", fmt.Errorf("invalid pairing verification URL")
	}
	query := parsed.Query()
	query.Set("code", code)
	if parsed.Path == "/pair" {
		parsed.Path = "/pair/"
	}
	parsed.RawQuery = query.Encode()
	parsed.Fragment = ""
	return parsed.String(), nil
}

func normalizeSuggestedName(value string) (string, error) {
	name := norm.NFC.String(strings.TrimSpace(value))
	if !utf8.ValidString(name) || name == "" {
		return "", usagef("hostname must contain valid non-whitespace UTF-8")
	}
	runes := []rune(name)
	if len(runes) > 64 {
		name = string(runes[:64])
	}
	return name, nil
}

func newStatusCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show pairing status", Args: func(_ *cobra.Command, args []string) error { return noArgs(args) }, RunE: func(cmd *cobra.Command, _ []string) error {
		result, err := deps.Service.Status(cmd.Context())
		if err != nil {
			return err
		}
		if !result.Paired {
			fmt.Fprintln(cmd.OutOrStdout(), "Not paired")
			return nil
		}
		method := result.Method
		if method == "web_login" {
			method = "web login"
		} else if method == "app_pair" {
			method = "app pairing"
		} else {
			method = "account authorization"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Authorized as %s (%s)\n", result.Nickname, method)
		return nil
	}}
}

func newRenameCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "rename <nickname>", Short: "Rename this paired CLI", Args: func(_ *cobra.Command, args []string) error { return exactlyOne(args, "nickname") }, RunE: func(cmd *cobra.Command, args []string) error {
		name := norm.NFC.String(strings.TrimSpace(args[0]))
		if !utf8.ValidString(name) || countScalars(name) < 1 || countScalars(name) > 64 {
			return usagef("nickname must contain 1 through 64 Unicode scalar values")
		}
		if err := deps.Service.Rename(cmd.Context(), name); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Renamed to %s\n", name)
		return nil
	}}
}

func newLogoutCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "logout", Short: "Revoke this CLI credential", Args: func(_ *cobra.Command, args []string) error { return noArgs(args) }, RunE: func(cmd *cobra.Command, _ []string) error {
		if err := deps.Service.Logout(cmd.Context()); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Logged out")
		return nil
	}}
}

func newDevicesCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "devices", Short: "List receiving devices", Args: func(_ *cobra.Command, args []string) error { return noArgs(args) }, RunE: func(cmd *cobra.Command, _ []string) error {
		devices, err := deps.Service.Devices(cmd.Context())
		if err != nil {
			return err
		}
		for _, device := range devices {
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", device.Nickname, device.Status)
		}
		return nil
	}}
}

func newHistoryCommand(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{Use: "history", Short: "List message history", Args: func(_ *cobra.Command, args []string) error { return noArgs(args) }, RunE: func(cmd *cobra.Command, _ []string) error {
		items, err := deps.Service.History(cmd.Context())
		if err != nil {
			return err
		}
		for _, item := range items {
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%d revision(s)\t%s\n", item.ID, item.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"), item.Title, item.UpdateCount, item.DeliveryState)
		}
		return nil
	}}
	cmd.AddCommand(&cobra.Command{Use: "show <message-id>", Short: "Show a message and its revisions", Args: func(_ *cobra.Command, args []string) error { return exactlyOne(args, "message ID") }, RunE: func(cmd *cobra.Command, args []string) error {
		detail, err := deps.Service.HistoryShow(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Logical message: %s\nRead: %t\n", detail.LogicalMessageID, detail.Read)
		// Keep API/MCP order chronological; only human-readable output is newest first.
		for index := len(detail.Revisions) - 1; index >= 0; index-- {
			revision := detail.Revisions[index]
			fmt.Fprintf(cmd.OutOrStdout(), "\nRevision %d/%d: %s\nAccepted: %s\nSender: %s\nTitle: %s\n", index+1, len(detail.Revisions), revision.ID, revision.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"), revision.SenderName, revision.Title)
			if revision.Subtitle != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Subtitle: %s\n", revision.Subtitle)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Sound: %s\nFormat: %s\n", revision.Sound, revision.Format)
			if revision.URL != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "URL: %s\n", revision.URL)
			}
			if revision.Image != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Image: %s\n", revision.Image)
			}
			for _, delivery := range revision.Deliveries {
				fmt.Fprintf(cmd.OutOrStdout(), "Delivery: %s — %s", delivery.DeviceName, delivery.State)
				if delivery.Failure != "" {
					fmt.Fprintf(cmd.OutOrStdout(), " (%s)", delivery.Failure)
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%s\n", revision.Body)
		}
		return nil
	}})
	return cmd
}

func newUsageCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "usage", Short: "Show monthly usage", Args: func(_ *cobra.Command, args []string) error { return noArgs(args) }, RunE: func(cmd *cobra.Command, _ []string) error {
		usage, err := deps.Service.Usage(cmd.Context())
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%d of %d messages used; resets %s\n", usage.Used, usage.Limit, usage.ResetsAt.Format("2006-01-02T15:04:05Z07:00"))
		return nil
	}}
}

func newDoctorCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "Diagnose local and service connectivity", Args: func(_ *cobra.Command, args []string) error { return noArgs(args) }, RunE: func(cmd *cobra.Command, _ []string) error {
		checks, err := deps.Service.Doctor(cmd.Context())
		if err != nil {
			return err
		}
		for _, check := range checks {
			state := "ok"
			if !check.OK {
				state = "failed"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", state, check.Name, check.Message)
		}
		return nil
	}}
}

func newSelfUpdateCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "self-update",
		Short: "Update Pushman through Homebrew",
		Args:  func(_ *cobra.Command, args []string) error { return noArgs(args) },
		RunE: func(cmd *cobra.Command, _ []string) error {
			output, err := deps.SelfUpdate(cmd.Context())
			if err != nil {
				return err
			}
			if output != "" {
				fmt.Fprintln(cmd.OutOrStdout(), output)
			}
			return nil
		},
	}
}

func newVersionCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "version", Short: "Print version information", Args: func(_ *cobra.Command, args []string) error { return noArgs(args) }, RunE: func(cmd *cobra.Command, _ []string) error {
		fmt.Fprintf(cmd.OutOrStdout(), "pushman %s (commit %s, built %s)\n", deps.Version.Version, deps.Version.Commit, deps.Version.Date)
		return nil
	}}
}
