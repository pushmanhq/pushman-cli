# Pushman MCP Guide

`pushman mcp` serves the Model Context Protocol over standard input and output from the installed Pushman CLI binary. It is intended for local AI clients and uses the same Pushman API origin, operating-system credential, validation rules, and monthly allowance as normal CLI commands.

No additional daemon or plaintext token file is required for local stdio. A hosted OAuth plugin is also available when its connection has been deployed; see the [hosted MCP guide](https://github.com/pushmanhq/pushman/blob/main/docs/MCP.md).

## Prerequisites

1. Install a release containing `pushman mcp`.
2. Run `pushman login` and authorize the CLI with Google or Apple in a browser, or use `pushman pair` with the signed-in iPhone app.
3. Confirm `pushman status` and `pushman devices` work before adding the MCP server.

Browser login and app pairing issue the same account CLI permission set, support every MCP tool, and store the credential in the native operating-system keyring. A process-scoped `PUSHMAN_TOKEN` can send notifications but intentionally cannot read devices, history, usage, status, or diagnostics. Never paste a token into a checked-in MCP configuration.

## Connect a client

### Codex

```sh
codex mcp add pushman -- pushman mcp
```

### Claude Code

```sh
claude mcp add --scope user pushman -- pushman mcp
```

### JSON-based clients

Clients that accept the common `mcpServers` JSON shape can launch Pushman like this:

```json
{
  "mcpServers": {
    "pushman": {
      "command": "/opt/homebrew/bin/pushman",
      "args": ["mcp"]
    }
  }
}
```

Replace the example command with the result of `command -v pushman`. An absolute path is recommended for desktop apps because their executable search path can differ from an interactive terminal. On Intel macOS Homebrew commonly uses `/usr/local/bin/pushman`; Linux locations vary.

On Windows, resolve the native executable in PowerShell:

```powershell
(Get-Command pushman.exe).Source
```

Use that absolute `.exe` path as `command`, with `args: ["mcp"]`. JSON requires doubled backslashes; spaces and non-ASCII characters stay inside the same string:

```json
{
  "mcpServers": {
    "pushman": {
      "command": "C:\\Users\\you\\AppData\\Local\\Programs\\Pushman\\pushman.exe",
      "args": ["mcp"]
    }
  }
}
```

You can generate correctly escaped JSON without manually editing the path:

```powershell
@{
    mcpServers = @{
        pushman = @{
            command = (Get-Command pushman.exe).Source
            args = @('mcp')
        }
    }
} | ConvertTo-Json -Depth 4
```

Restart desktop clients after a user PATH change, and run the host under the same Windows identity/session that authorized the CLI. Otherwise, an absolute path fixes executable discovery but does not grant access to another account's Credential Manager. Keep tokens out of this configuration. See [Windows installation and session guidance](INSTALL.md#windows-zip-installation).

The server is a long-running stdio subprocess. Do not start it manually and expect a terminal interface: a compatible MCP client launches it and exchanges protocol frames. Stdout is reserved exclusively for MCP; Pushman emits no routine diagnostic logs or notification content on stderr.

## Tools and permissions

| Tool | Effect | Authentication |
| --- | --- | --- |
| `pushman_send_notification` | Sends one notification and consumes one accepted-send allowance | Account CLI credential or `PUSHMAN_TOKEN` |
| `pushman_list_devices` | Reads receiving device names and eligibility states | Account CLI credential |
| `pushman_list_history` | Reads the sender's retained seven-day history | Account CLI credential |
| `pushman_get_message` | Reads one retained message, revisions, and delivery states | Account CLI credential |
| `pushman_get_usage` | Reads monthly usage, limit, and reset time | Account CLI credential |
| `pushman_get_status` | Reads local authorization state and sender nickname | Account CLI credential |
| `pushman_doctor` | Runs non-mutating credential and service connectivity checks | Account CLI credential |

The send tool is explicitly described to clients as non-read-only, non-idempotent, non-destructive/additive, and open-world. The six original read tools are read-only and open-world because they contact the hosted Pushman service. The additional account and local workflows have their own annotations. Tool annotations are safety hints, not an authorization boundary; the hosted API still enforces the credential's capabilities and account quota.

Local login/pair, sender rename/logout, diagnostics and eligible binary update
are described in [CLI workflow parity](#cli-workflow-parity-v050) below. Account
administration, billing, MCP resources and MCP prompts remain outside this local
tool surface. Hosted Streamable HTTP uses the separate OAuth endpoint and sender.

Usage results may include server-provided `plan` and `billingState` metadata. Older servers
omit them; clients must not infer a plan from the numerical limit. `billingState: unavailable`
means the paid decision could not be checked, not that a subscription was canceled. Existing
`used`, `limit`, and `resetsAt` fields are unchanged. There are no purchase or cancellation tools.

## Safe sending

An AI client should ask before calling `pushman_send_notification` unless the user has already directly requested that exact send. Review the body, title, targets, URL, and update key when they matter. In particular:

- Omitting `devices` targets every currently eligible receiving device.
- Repeating a send without a `key` creates another notification and consumes another allowance.
- Reusing a `key` updates the matching logical notification but still counts as an accepted send.
- The server does not automatically retry a rate-limited or ambiguous send.
- Notification content and URLs are sent to the hosted Pushman service and synchronized for seven days.

The MCP input schema exposes the same fields and limits as `pushman push`: `body`, `title`, `subtitle`, `url`, `group`, `image`, `sound`, `key`, `format`, and `devices`. Validation remains shared with the CLI, including HTTPS-only public image URLs and blocked dangerous URL schemes.

## Troubleshooting

Run these in a normal terminal, not through the MCP client:

```sh
pushman version
pushman status
pushman devices
pushman doctor
```

If the client reports that `pushman` cannot be found, configure the absolute path printed by `command -v pushman`. If read tools report an authorization error, run `pushman login` or `pushman pair` rather than supplying an automation token. On macOS, approve a Keychain access prompt if the MCP host causes one to appear.

Closing the client's stdio connection stops the server cleanly. `Ctrl-C` or client cancellation also stops it. Protocol or tool errors are returned to the MCP client; Pushman does not upload CLI crash reports or MCP telemetry.

Before replacing a Windows ZIP-installed executable, have the desktop client stop its Pushman subprocess. Start it again after checking the replacement version. See the [Windows verification record](WINDOWS_VERIFICATION.md) for automated process coverage and desktop-host acceptance status.

## Development verification

```sh
go test -race ./...
go vet ./...
make build-dev
.bin/pushman-dev mcp
```

Tests negotiate with the official MCP Go SDK client, verify all schemas and safety annotations, exercise successful and failed tool calls, and run the Cobra command in a separate process to prove protocol-only stdout and clean EOF shutdown.

On Windows, `go test -v ./internal/wincheck` builds the real main package into a path with spaces and Korean characters, then exercises the official SDK client against that executable and a loopback API. It verifies negotiation, tool schemas, success/error, send-only authorization, protocol-only stdout, EOF, restart, and cancellation of an in-flight API call. This does not substitute for acceptance in a specific desktop host.

## CLI workflow parity (v0.5.0)

Local MCP exposes 17 tools. The original seven tools retain their notification
fields, output contract and explicit-send policy. The additional workflows are:

| CLI command | Local MCP tool | Behavior |
| --- | --- | --- |
| `login`, `pair` | `pushman_login`, `pushman_pair` | Start explicitly requested human-approved authorization; return a public URL/code and progress handle. |
| Authorization progress | `pushman_get_authorization` | Read the exact process-local handle. |
| Cancel pending authorization | `pushman_cancel_authorization` | Stop polling; does not revoke a completed credential. |
| `rename` | `pushman_rename_sender` | Rename only the current sender. |
| `logout` | `pushman_logout` | Revoke the current credential and remove it locally. |
| `support-bundle` | `pushman_get_support_bundle` | Return privacy-safe JSON to the client; no local file or support upload. |
| `version` | `pushman_get_version` | Report this process's loaded build. |
| `help` | `pushman_get_capabilities` | Map commands to MCP workflows and explain transport limits. |
| `self-update` | `pushman_self_update` | Reuse the eligible Homebrew ownership check; requires an explicit binary-update request. |

Do not invent an authorization handle, restart a pending attempt, auto-retry a
denial/expiry, or submit provider credentials for the user. Client disconnect
stops pending polling. Verify status after completion; no test send is implied.

Account-management tools do not change the send-only automation-token boundary.
After self-update, the existing MCP process still runs its loaded version.
Restart the client's Pushman subprocess and verify version/tool discovery before
claiming updated tools are active. Hosted OAuth has its own sender and token
family; a hosted service cannot modify this computer's keyring or executable.
