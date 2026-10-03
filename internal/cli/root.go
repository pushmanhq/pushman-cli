package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
)

type Dependencies struct {
	In          io.Reader
	Out         io.Writer
	ErrOut      io.Writer
	IsTerminal  func() bool
	Hostname    func() (string, error)
	OpenBrowser func(string) error
	SelfUpdate  func(context.Context) (string, error)
	Now         func() time.Time
	Service     Service
	Version     VersionInfo
}

func New(deps Dependencies) *cobra.Command {
	deps = withDefaults(deps)
	root := &cobra.Command{
		Use:           "pushman",
		Short:         "Send push notifications to your phone",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.SetIn(deps.In)
	root.SetOut(deps.Out)
	root.SetErr(deps.ErrOut)
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &UsageError{Err: err}
	})
	root.AddCommand(
		newPairCommand(deps),
		newLoginCommand(deps),
		newStatusCommand(deps),
		newRenameCommand(deps),
		newLogoutCommand(deps),
		newPushCommand(deps),
		newDevicesCommand(deps),
		newHistoryCommand(deps),
		newUsageCommand(deps),
		newDoctorCommand(deps),
		newSupportBundleCommand(deps),
		newMCPCommand(deps),
		newSelfUpdateCommand(deps),
		newVersionCommand(deps),
	)
	root.InitDefaultHelpCmd()
	return root
}

func withDefaults(deps Dependencies) Dependencies {
	if deps.In == nil {
		deps.In = os.Stdin
	}
	if deps.Out == nil {
		deps.Out = io.Discard
	}
	if deps.ErrOut == nil {
		deps.ErrOut = io.Discard
	}
	if deps.IsTerminal == nil {
		deps.IsTerminal = func() bool { return true }
	}
	if deps.Hostname == nil {
		deps.Hostname = os.Hostname
	}
	if deps.OpenBrowser == nil {
		deps.OpenBrowser = func(string) error { return nil }
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.SelfUpdate == nil {
		deps.SelfUpdate = func(context.Context) (string, error) {
			return "", fmt.Errorf("self-update is not configured")
		}
	}
	if deps.Service == nil {
		deps.Service = UnconfiguredService{}
	}
	return deps
}

func noArgs(args []string) error {
	if len(args) != 0 {
		return usagef("expected no arguments")
	}
	return nil
}

func exactlyOne(args []string, name string) error {
	if len(args) != 1 {
		return usagef("expected exactly one %s", name)
	}
	return nil
}
