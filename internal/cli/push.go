package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

type pushOptions struct {
	title     string
	subtitle  string
	url       string
	group     string
	image     string
	sound     string
	key       string
	monospace bool
	devices   []string
	json      bool
	quiet     bool
}

func newPushCommand(deps Dependencies) *cobra.Command {
	var opts pushOptions
	cmd := &cobra.Command{
		Use:   "push [body|-]",
		Short: "Send a notification",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usagef("push accepts at most one body argument")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			request, err := buildPushRequest(cmd.InOrStdin(), deps.IsTerminal(), args, opts)
			if err != nil {
				return err
			}
			result, err := deps.Service.Push(cmd.Context(), request)
			if err != nil {
				return err
			}
			if opts.quiet {
				return nil
			}
			if opts.json {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetEscapeHTML(false)
				return encoder.Encode(result)
			}
			deviceWord := "devices"
			if result.DeviceCount == 1 {
				deviceWord = "device"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Accepted %s for %d %s\n", result.ID, result.DeviceCount, deviceWord)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVarP(&opts.title, "title", "t", "", "notification title")
	f.StringVar(&opts.subtitle, "subtitle", "", "notification subtitle")
	f.StringVar(&opts.url, "url", "", "URL opened when the notification is tapped")
	f.StringVar(&opts.group, "group", "", "group related notifications")
	f.StringVar(&opts.image, "image", "", "HTTPS image URL")
	f.StringVar(&opts.sound, "sound", "default", "sound behavior: default or none")
	f.StringVar(&opts.key, "key", "", "stable key used to update a notification")
	f.BoolVar(&opts.monospace, "monospace", false, "use monospace body presentation")
	f.StringArrayVarP(&opts.devices, "device", "d", nil, "target device nickname (repeatable)")
	f.BoolVar(&opts.json, "json", false, "write a stable JSON result")
	f.BoolVar(&opts.quiet, "quiet", false, "suppress success output")
	return cmd
}

func buildPushRequest(stdin io.Reader, stdinIsTerminal bool, args []string, opts pushOptions) (PushRequest, error) {
	body, err := readBody(stdin, stdinIsTerminal, args)
	if err != nil {
		return PushRequest{}, err
	}
	if err := validatePushOptions(opts); err != nil {
		return PushRequest{}, err
	}
	devices, _ := normalizeDevices(opts.devices)
	format := "plain"
	if opts.monospace {
		format = "monospace"
	}
	return PushRequest{Body: body, Title: opts.title, Subtitle: opts.subtitle, URL: opts.url, Group: opts.group, Image: opts.image, Sound: opts.sound, Key: opts.key, Format: format, Devices: devices}, nil
}

func readBody(stdin io.Reader, stdinIsTerminal bool, args []string) (string, error) {
	if len(args) > 1 {
		return "", usagef("push accepts at most one body argument")
	}
	readStdin := len(args) == 1 && args[0] == "-"
	if len(args) == 0 {
		if stdinIsTerminal {
			return "", usagef("provide a body argument or pipe body text on standard input")
		}
		readStdin = true
	}
	if len(args) == 1 && args[0] != "-" {
		if !stdinIsTerminal {
			return "", usagef("cannot combine a body argument with piped standard input")
		}
		return validateBody(args[0])
	}
	if !readStdin {
		return "", usagef("provide a notification body")
	}
	const maxBodyBytes = 4096 * utf8.UTFMax
	// A shell may append LF or CRLF to a body already at the scalar limit.
	// Include that removable suffix while keeping oversized streams bounded.
	const maxStdinBytes = maxBodyBytes + len("\r\n")
	data, err := io.ReadAll(io.LimitReader(stdin, int64(maxStdinBytes)+1))
	if err != nil {
		return "", fmt.Errorf("read body from standard input: %w", err)
	}
	if len(data) > maxStdinBytes {
		return "", usagef("body must not exceed 4096 Unicode scalar values")
	}
	if !utf8.Valid(data) {
		return "", usagef("body must be valid UTF-8")
	}
	body := string(data)
	if strings.HasSuffix(body, "\r\n") {
		body = strings.TrimSuffix(body, "\r\n")
	} else {
		body = strings.TrimSuffix(body, "\n")
	}
	return validateBody(body)
}

func validateBody(body string) (string, error) {
	if !utf8.ValidString(body) {
		return "", usagef("body must be valid UTF-8")
	}
	if strings.TrimSpace(body) == "" {
		return "", usagef("body must contain a non-whitespace character")
	}
	if countScalars(body) > 4096 {
		return "", usagef("body must not exceed 4096 Unicode scalar values")
	}
	return body, nil
}

func validatePushOptions(opts pushOptions) error {
	if opts.json && opts.quiet {
		return usagef("--json and --quiet are mutually exclusive")
	}
	if !utf8.ValidString(opts.title) || countScalars(opts.title) > 250 {
		return usagef("title must be valid UTF-8 and at most 250 Unicode scalar values")
	}
	if !utf8.ValidString(opts.subtitle) || countScalars(opts.subtitle) > 250 {
		return usagef("subtitle must be valid UTF-8 and at most 250 Unicode scalar values")
	}
	if err := validateTargetURL(opts.url); err != nil {
		return err
	}
	if err := validateImageURL(opts.image); err != nil {
		return err
	}
	if opts.group != "" && !validRestrictedASCII(opts.group) {
		return usagef("group must contain 1 through 64 characters from A-Z, a-z, 0-9, ., _, :, and -")
	}
	if opts.key != "" && !validRestrictedASCII(opts.key) {
		return usagef("key must contain 1 through 64 characters from A-Z, a-z, 0-9, ., _, :, and -")
	}
	if opts.sound != "default" && opts.sound != "none" {
		return usagef("sound must be default or none")
	}
	_, err := normalizeDevices(opts.devices)
	return err
}

func validateTargetURL(raw string) error {
	if raw == "" {
		return nil
	}
	if !utf8.ValidString(raw) || countScalars(raw) > 2048 {
		return usagef("URL must be valid UTF-8 and at most 2048 Unicode scalar values")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" {
		return usagef("URL must be an absolute web or app URI")
	}
	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "javascript", "data", "file", "intent":
		return usagef("URL scheme %q is not allowed", parsed.Scheme)
	}
	if (scheme == "http" || scheme == "https") && parsed.Host == "" {
		return usagef("HTTP URL must include a host")
	}
	return nil
}

func validateImageURL(raw string) error {
	if raw == "" {
		return nil
	}
	if !utf8.ValidString(raw) || countScalars(raw) > 2048 {
		return usagef("image URL must be valid UTF-8 and at most 2048 Unicode scalar values")
	}
	parsed, err := url.Parse(raw)
	if err != nil || strings.ToLower(parsed.Scheme) != "https" || parsed.Host == "" {
		return usagef("image must be an absolute HTTPS URL")
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "localhost" || strings.HasSuffix(hostname, ".local") || net.ParseIP(hostname) != nil {
		return usagef("image host must not be localhost, a .local name, or a literal IP address")
	}
	return nil
}

func normalizeDevices(devices []string) ([]string, error) {
	normalized := make([]string, 0, len(devices))
	seen := make(map[string]struct{}, len(devices))
	for _, device := range devices {
		name := norm.NFC.String(strings.TrimSpace(device))
		if name == "" || countScalars(name) > 64 {
			return nil, usagef("device nickname must contain 1 through 64 Unicode scalar values")
		}
		folded := cases.Fold().String(name)
		if _, exists := seen[folded]; exists {
			return nil, usagef("device %q is repeated", device)
		}
		seen[folded] = struct{}{}
		normalized = append(normalized, name)
	}
	return normalized, nil
}

func validRestrictedASCII(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || strings.ContainsRune("._:-", r) {
			continue
		}
		return false
	}
	return true
}

func countScalars(value string) int { return utf8.RuneCountInString(value) }
