package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

const mcpServerInstructions = "Pushman sends notifications to the user's own paired iPhone devices. Ask for explicit user confirmation before calling pushman_send_notification unless the user already gave a direct instruction to send that exact notification. Never invent notification content, target devices, URLs, or update keys."

type writeNopCloser struct{ io.Writer }

func (writeNopCloser) Close() error { return nil }

func newMCPCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve the Model Context Protocol over stdio",
		Args:  func(_ *cobra.Command, args []string) error { return noArgs(args) },
		RunE: func(cmd *cobra.Command, _ []string) error {
			server := newMCPServer(deps)
			transport := &mcp.IOTransport{
				Reader: mcpReadCloser(cmd.InOrStdin()),
				Writer: writeNopCloser{cmd.OutOrStdout()},
			}
			if err := server.Run(cmd.Context(), transport); err != nil && !errors.Is(err, context.Canceled) {
				return fmt.Errorf("run MCP server: %w", err)
			}
			return nil
		},
	}
}

func mcpReadCloser(reader io.Reader) io.ReadCloser {
	if closer, ok := reader.(io.ReadCloser); ok {
		return closer
	}
	return io.NopCloser(reader)
}

func newMCPServer(deps Dependencies) *mcp.Server {
	version := deps.Version.Version
	if version == "" {
		version = "dev"
	}
	server := mcp.NewServer(&mcp.Implementation{
		Name:        "pushman",
		Title:       "Pushman",
		Description: "Send and inspect Pushman notifications using the locally configured CLI credential.",
		Version:     version,
		WebsiteURL:  "https://github.com/pushmanhq/pushman",
	}, &mcp.ServerOptions{
		Instructions: mcpServerInstructions,
		Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
	})

	registerMCPTools(server, deps.Service)
	return server
}

func registerMCPTools(server *mcp.Server, service Service) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPointer(true)}
	send := &mcp.ToolAnnotations{
		ReadOnlyHint:    false,
		DestructiveHint: boolPointer(false),
		IdempotentHint:  false,
		OpenWorldHint:   boolPointer(true),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "pushman_send_notification",
		Title:       "Send a Pushman notification",
		Description: "Send one notification to the user's eligible Pushman devices. This consumes quota and has an external side effect. Call only after the user explicitly requests or confirms the exact notification.",
		Annotations: send,
		InputSchema: mcpSendInputSchema(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input mcpSendInput) (*mcp.CallToolResult, mcpSendOutput, error) {
		request, err := buildMCPPushRequest(input)
		if err != nil {
			return nil, mcpSendOutput{}, err
		}
		result, err := service.Push(ctx, request)
		if err != nil {
			return nil, mcpSendOutput{}, err
		}
		return nil, mcpSendOutput{
			ID: result.ID, Status: result.Status, DeviceCount: result.DeviceCount,
			AcceptedAt: result.AcceptedAt.Format(time.RFC3339Nano),
		}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "pushman_list_devices",
		Title:       "List Pushman devices",
		Description: "List receiving device nicknames and notification eligibility states for the paired Pushman account.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ mcpEmptyInput) (*mcp.CallToolResult, mcpDevicesOutput, error) {
		devices, err := service.Devices(ctx)
		if err != nil {
			return nil, mcpDevicesOutput{}, err
		}
		output := mcpDevicesOutput{Devices: make([]mcpDevice, 0, len(devices))}
		for _, device := range devices {
			output.Devices = append(output.Devices, mcpDevice{Nickname: device.Nickname, Status: device.Status})
		}
		return nil, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "pushman_list_history",
		Title:       "List Pushman history",
		Description: "List the paired sender's retained Pushman messages from the seven-day server history.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ mcpEmptyInput) (*mcp.CallToolResult, mcpHistoryOutput, error) {
		items, err := service.History(ctx)
		if err != nil {
			return nil, mcpHistoryOutput{}, err
		}
		output := mcpHistoryOutput{Messages: make([]mcpHistoryItem, 0, len(items))}
		for _, item := range items {
			output.Messages = append(output.Messages, mcpHistoryItem{
				ID: item.ID, Title: item.Title, UpdatedAt: item.UpdatedAt.Format(time.RFC3339Nano),
				UpdateCount: item.UpdateCount, DeliveryState: item.DeliveryState,
			})
		}
		return nil, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "pushman_get_message",
		Title:       "Get a Pushman message",
		Description: "Get one retained logical Pushman message, its revisions, read state, and per-device delivery states by message or revision ID.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input mcpGetMessageInput) (*mcp.CallToolResult, mcpMessageOutput, error) {
		id := strings.TrimSpace(input.MessageID)
		if id == "" {
			return nil, mcpMessageOutput{}, usagef("messageId must contain a non-whitespace value")
		}
		detail, err := service.HistoryShow(ctx, id)
		if err != nil {
			return nil, mcpMessageOutput{}, err
		}
		return nil, mapMCPMessage(detail), nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "pushman_get_usage",
		Title:       "Get Pushman usage",
		Description: "Get the paired Pushman account's current monthly accepted-send usage, limit, and reset time.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ mcpEmptyInput) (*mcp.CallToolResult, mcpUsageOutput, error) {
		usage, err := service.Usage(ctx)
		if err != nil {
			return nil, mcpUsageOutput{}, err
		}
		result := mcpUsageOutput{Used: usage.Used, Limit: usage.Limit, ResetsAt: usage.ResetsAt.Format(time.RFC3339Nano)}
		if usage.Plan == "free" || usage.Plan == "pro" {
			result.Plan = usage.Plan
		}
		switch usage.BillingState {
		case "disabled", "synchronized", "unavailable":
			result.BillingState = usage.BillingState
		}
		return nil, result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "pushman_get_status",
		Title:       "Get Pushman pairing status",
		Description: "Check whether this local Pushman CLI is paired and return its sender nickname when paired.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ mcpEmptyInput) (*mcp.CallToolResult, mcpStatusOutput, error) {
		status, err := service.Status(ctx)
		if err != nil {
			return nil, mcpStatusOutput{}, err
		}
		return nil, mcpStatusOutput{Paired: status.Paired, Nickname: status.Nickname}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "pushman_doctor",
		Title:       "Diagnose Pushman",
		Description: "Run non-mutating local credential and Pushman service connectivity checks.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ mcpEmptyInput) (*mcp.CallToolResult, mcpDoctorOutput, error) {
		checks, err := service.Doctor(ctx)
		if err != nil {
			return nil, mcpDoctorOutput{}, err
		}
		output := mcpDoctorOutput{OK: true, Checks: make([]mcpDoctorCheck, 0, len(checks))}
		for _, check := range checks {
			output.Checks = append(output.Checks, mcpDoctorCheck{Name: check.Name, OK: check.OK, Message: check.Message})
			output.OK = output.OK && check.OK
		}
		return nil, output, nil
	})
}

type mcpEmptyInput struct{}

type mcpSendInput struct {
	Body     string   `json:"body" jsonschema:"Notification body; 1 through 4096 Unicode scalar values and not only whitespace."`
	Title    string   `json:"title,omitempty" jsonschema:"Optional notification title; at most 250 Unicode scalar values."`
	Subtitle string   `json:"subtitle,omitempty" jsonschema:"Optional notification subtitle; at most 250 Unicode scalar values."`
	URL      string   `json:"url,omitempty" jsonschema:"Optional absolute web or app URI opened when the notification is tapped."`
	Group    string   `json:"group,omitempty" jsonschema:"Optional 1 through 64 character notification group identifier using letters, digits, dot, underscore, colon, or hyphen."`
	Image    string   `json:"image,omitempty" jsonschema:"Optional absolute public HTTPS image URL; localhost, .local names, and literal IP addresses are rejected."`
	Sound    string   `json:"sound,omitempty" jsonschema:"Sound behavior: default or none. Defaults to default."`
	Key      string   `json:"key,omitempty" jsonschema:"Optional stable 1 through 64 character identifier used to update a prior notification."`
	Format   string   `json:"format,omitempty" jsonschema:"Body presentation: plain or monospace. Defaults to plain."`
	Devices  []string `json:"devices,omitempty" jsonschema:"Optional receiving device nicknames. Omit to target every eligible device."`
}

type mcpSendOutput struct {
	ID          string `json:"id" jsonschema:"Accepted immutable message revision ID."`
	Status      string `json:"status" jsonschema:"Acceptance state."`
	DeviceCount int    `json:"deviceCount" jsonschema:"Number of targeted eligible receiving devices."`
	AcceptedAt  string `json:"acceptedAt" jsonschema:"UTC RFC 3339 acceptance timestamp."`
}

type mcpDevice struct {
	Nickname string `json:"nickname"`
	Status   string `json:"status"`
}

type mcpDevicesOutput struct {
	Devices []mcpDevice `json:"devices"`
}

type mcpHistoryItem struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	UpdatedAt     string `json:"updatedAt"`
	UpdateCount   int    `json:"updateCount"`
	DeliveryState string `json:"deliveryState"`
}

type mcpHistoryOutput struct {
	Messages []mcpHistoryItem `json:"messages"`
}

type mcpGetMessageInput struct {
	MessageID string `json:"messageId" jsonschema:"Retained logical message or revision ID."`
}

type mcpMessageOutput struct {
	LogicalMessageID string               `json:"logicalMessageId"`
	Read             bool                 `json:"read"`
	Revisions        []mcpMessageRevision `json:"revisions"`
}

type mcpMessageRevision struct {
	ID         string               `json:"id"`
	Title      string               `json:"title"`
	Subtitle   string               `json:"subtitle,omitempty"`
	Body       string               `json:"body"`
	SenderName string               `json:"senderName"`
	URL        string               `json:"url,omitempty"`
	Image      string               `json:"image,omitempty"`
	Sound      string               `json:"sound"`
	Format     string               `json:"format"`
	UpdatedAt  string               `json:"updatedAt"`
	Deliveries []mcpMessageDelivery `json:"deliveries"`
}

type mcpMessageDelivery struct {
	DeviceName string `json:"deviceName"`
	State      string `json:"state"`
	Failure    string `json:"failure,omitempty"`
}

type mcpUsageOutput struct {
	Used         int    `json:"used"`
	Limit        int    `json:"limit"`
	ResetsAt     string `json:"resetsAt"`
	Plan         string `json:"plan,omitempty" jsonschema:"Currently verified capacity tier; not proof of subscription cancellation"`
	BillingState string `json:"billingState,omitempty" jsonschema:"Billing synchronization status; unavailable does not mean canceled"`
}

type mcpStatusOutput struct {
	Paired   bool   `json:"paired"`
	Nickname string `json:"nickname,omitempty"`
}

type mcpDoctorCheck struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type mcpDoctorOutput struct {
	OK     bool             `json:"ok"`
	Checks []mcpDoctorCheck `json:"checks"`
}

func buildMCPPushRequest(input mcpSendInput) (PushRequest, error) {
	body, err := validateBody(input.Body)
	if err != nil {
		return PushRequest{}, err
	}
	format := input.Format
	if format == "" {
		format = "plain"
	}
	if format != "plain" && format != "monospace" {
		return PushRequest{}, usagef("format must be plain or monospace")
	}
	sound := input.Sound
	if sound == "" {
		sound = "default"
	}
	opts := pushOptions{
		title: input.Title, subtitle: input.Subtitle, url: input.URL, group: input.Group,
		image: input.Image, sound: sound, key: input.Key, monospace: format == "monospace", devices: input.Devices,
	}
	if err := validatePushOptions(opts); err != nil {
		return PushRequest{}, err
	}
	devices, _ := normalizeDevices(input.Devices)
	return PushRequest{
		Body: body, Title: input.Title, Subtitle: input.Subtitle, URL: input.URL,
		Group: input.Group, Image: input.Image, Sound: sound, Key: input.Key,
		Format: format, Devices: devices,
	}, nil
}

func mapMCPMessage(detail HistoryDetail) mcpMessageOutput {
	output := mcpMessageOutput{
		LogicalMessageID: detail.LogicalMessageID,
		Read:             detail.Read,
		Revisions:        make([]mcpMessageRevision, 0, len(detail.Revisions)),
	}
	for _, revision := range detail.Revisions {
		mapped := mcpMessageRevision{
			ID: revision.ID, Title: revision.Title, Subtitle: revision.Subtitle, Body: revision.Body,
			SenderName: revision.SenderName, URL: revision.URL, Image: revision.Image,
			Sound: revision.Sound, Format: revision.Format, UpdatedAt: revision.UpdatedAt.Format(time.RFC3339Nano),
			Deliveries: make([]mcpMessageDelivery, 0, len(revision.Deliveries)),
		}
		for _, delivery := range revision.Deliveries {
			mapped.Deliveries = append(mapped.Deliveries, mcpMessageDelivery{
				DeviceName: delivery.DeviceName, State: delivery.State, Failure: delivery.Failure,
			})
		}
		output.Revisions = append(output.Revisions, mapped)
	}
	return output
}

func boolPointer(value bool) *bool { return &value }

func mcpSendInputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[mcpSendInput](nil)
	if err != nil {
		panic(fmt.Sprintf("build Pushman MCP send schema: %v", err))
	}
	maxBody, maxTitle, maxURL, maxIdentifier, maxDevice := 4096, 250, 2048, 64, 64
	minBody, minIdentifier, minDevice := 1, 1, 1
	schema.Properties["body"].MinLength = &minBody
	schema.Properties["body"].MaxLength = &maxBody
	schema.Properties["title"].MaxLength = &maxTitle
	schema.Properties["subtitle"].MaxLength = &maxTitle
	schema.Properties["url"].MaxLength = &maxURL
	schema.Properties["image"].MaxLength = &maxURL
	for _, name := range []string{"group", "key"} {
		schema.Properties[name].MinLength = &minIdentifier
		schema.Properties[name].MaxLength = &maxIdentifier
		schema.Properties[name].Pattern = `^[A-Za-z0-9._:-]+$`
	}
	schema.Properties["sound"].Enum = []any{"default", "none"}
	schema.Properties["sound"].Default = []byte(`"default"`)
	schema.Properties["format"].Enum = []any{"plain", "monospace"}
	schema.Properties["format"].Default = []byte(`"plain"`)
	schema.Properties["devices"].UniqueItems = true
	schema.Properties["devices"].Items.MinLength = &minDevice
	schema.Properties["devices"].Items.MaxLength = &maxDevice
	return schema
}
