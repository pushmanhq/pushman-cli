# Pushman MCP Guide

`pushman mcp` serves the Model Context Protocol over standard input and output from the installed Pushman CLI binary. It is intended for local AI clients and uses the same Pushman API origin, operating-system credential, validation rules, and monthly allowance as normal CLI commands.

No additional daemon, remote MCP endpoint, MCP account, or plaintext token file is required.

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

The send tool is explicitly described to clients as non-read-only, non-idempotent, non-destructive/additive, and open-world. All other tools are read-only and open-world because they contact the hosted Pushman service. Tool annotations are safety hints, not an authorization boundary; the hosted API still enforces the credential's capabilities and account quota.

Pairing, logout, sender rename, credential creation or revocation, account changes, and billing are deliberately not exposed through MCP. Remote Streamable HTTP, MCP resources, and MCP prompts are not part of this version.

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

## Development verification

```sh
go test -race ./...
go vet ./...
make build-dev
.bin/pushman-dev mcp
```

Tests negotiate with the official MCP Go SDK client, verify all schemas and safety annotations, exercise successful and failed tool calls, and run the Cobra command in a separate process to prove protocol-only stdout and clean EOF shutdown.
