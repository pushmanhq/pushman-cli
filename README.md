<p align="center">
  <img src="docs/assets/pushman-icon.png" alt="Pushman" width="128" height="128">
</p>

<h1 align="center">Pushman CLI</h1>

<p align="center">
  Send push notifications to your iPhone from any terminal, script, server, or CI job.
</p>

<p align="center">
  <a href="https://github.com/pushmanhq/pushman">Pushman product home</a> ·
  <a href="https://app.pushman.whitekiwi.link/support/">Support</a>
</p>

<p align="center">
  <a href="https://github.com/pushmanhq/pushman-cli/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/pushmanhq/pushman-cli/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://go.dev/"><img alt="Go 1.27+" src="https://img.shields.io/badge/Go-1.27%2B-00ADD8?logo=go&logoColor=white"></a>
  <a href="LICENSE"><img alt="MIT License" src="https://img.shields.io/badge/license-MIT-151515"></a>
  <a href="https://github.com/pushmanhq/pushman-cli/releases"><img alt="GitHub release" src="https://img.shields.io/github/v/release/pushmanhq/pushman-cli?display_name=tag&include_prereleases"></a>
</p>

```console
$ pushman push "Production deploy finished" --title "Acme API"
Accepted msg_01M0W2RDPVGEVX7D6ZWFK907B2 for 1 device
```

Pushman is a small, script-friendly companion for getting your own operational messages onto your iPhone. Authorize once through a browser or the iPhone app, then use the same command interactively or in automation. Credentials live in the operating-system keyring, not a plaintext config file.

> [!NOTE]
> Pushman for iPhone is preparing for its first App Store release. Follow the [product repository](https://github.com/pushmanhq/pushman) for availability and product documentation.

## Highlights

- One command for terminals, shell scripts, servers, and CI
- Rich notifications with title, subtitle, URL, image, sound, group, and update key
- Multiple receiving devices and explicit device targeting
- Seven-day history plus usage and delivery diagnostics
- Native credential storage through macOS Keychain, Windows Credential Manager, or Secret Service
- Browser-assisted account login for local terminals and headless machines
- Stable JSON output and exit codes for automation
- A local stdio MCP server for Codex, Claude Code, and other compatible clients
- Safe in-place updates for Homebrew-managed installations
- Signed GitHub release provenance and published SHA-256 checksums

## Install

**Homebrew (macOS and Linux):**

```sh
brew install whitekiwi/tap/pushman
```

**Or ask Claude Code:**

```sh
claude "Install Pushman CLI from https://github.com/pushmanhq/pushman-cli using the safest supported method for this machine, verify it, then guide me through login. Ask before sending a test notification."
```

Review and approve each command it proposes. See the [Installation Guide](docs/INSTALL.md) for Go installs, verified release archives, updates, uninstalling, and troubleshooting.

**Windows:** use a verified x64 or ARM64 ZIP with the [PowerShell installation steps](docs/INSTALL.md#windows-zip-installation). The same executable provides the CLI and local stdio MCP server; it sends notifications to the iPhone app.

Then authorize the CLI in a browser:

```sh
pushman login
```

## Quick start

Authorize the CLI with Google or Apple in your browser:

```sh
pushman login
```

Use `pushman pair` instead when you want to approve from the signed-in iPhone app.

Then send a notification:

```sh
pushman push "Database backup completed"
pushman push "Deploy completed" --title "Production" --url "https://example.com/runs/42"
printf '%s\n' "Build failed" | pushman push --title "CI"
```

Use `pushman help push` to see every notification field and output option.

## Commands

| Command | Purpose |
| --- | --- |
| `pushman login` | Authorize this CLI through a browser |
| `pushman pair` | Pair this CLI and assign its default nickname |
| `pushman push <body>` | Send a notification |
| `pushman devices` | List receiving devices |
| `pushman history` | Show recent messages |
| `pushman usage` | Show the current monthly allowance |
| `pushman status` | Show authorization and account state |
| `pushman rename <nickname>` | Rename this CLI |
| `pushman doctor` | Diagnose configuration and connectivity |
| `pushman mcp` | Serve Pushman's MCP tools over stdio |
| `pushman self-update` | Update a Homebrew-managed installation |
| `pushman logout` | Revoke and remove the local CLI credential |

## AI clients and MCP

`pushman mcp` lets a local AI client send notifications and inspect devices, seven-day history, usage, authorization state, and diagnostics through the same credential and API client as the CLI.

```sh
codex mcp add pushman -- pushman mcp
# or
claude mcp add --scope user pushman -- pushman mcp
```

Authorize with `pushman login` or `pushman pair` before connecting. Sending consumes quota and changes external state, so clients should ask for confirmation unless you already gave a direct instruction containing the exact notification. See the [MCP Guide](docs/MCP.md) for every tool, generic client configuration, permissions, and troubleshooting.

## Automation

Pass reusable automation credentials only through `PUSHMAN_TOKEN`. Never place a token in command arguments, logs, or source control.

```sh
export PUSHMAN_TOKEN="..."
pushman push "Release $GITHUB_REF_NAME is live" --title "Deploy" --json
```

Successful command output is written to stdout; errors and diagnostics are written to stderr. Use `--json` for machine-readable output and `--quiet` when only the exit status matters.

## Service and future plans

The CLI source is MIT licensed. The hosted Pushman service, iPhone app, monthly allowance, and any future paid plan are separate from the source-code license. During beta, an account can submit up to 200 accepted push requests per month. The CLI discovers service capabilities at runtime so future plan and quota changes do not require embedding billing logic or secrets in this repository.

## Configuration

The production API is `https://api.pushman.whitekiwi.link/v1`. For local development, an explicit loopback override is supported:

```sh
make pdev ARGS=status
```

This builds `.bin/pushman-dev`, defaults it to `http://127.0.0.1:8080/v1`, uses a separate development Keychain namespace, and reads automation credentials only from `PUSHMAN_DEV_TOKEN`. It cannot self-update. `PUSHMAN_API_URL` can still override the endpoint explicitly; overrides must use HTTPS except for loopback development and must end in `/v1`. Redirects are rejected to prevent credentials from being forwarded to another origin.

For an optional global development shortcut, run `make install-dev`. It writes `pdev` to `~/.local/bin`, leaving a Homebrew or release installation named `pushman` untouched. Use `pushman` for the installed product and `pdev` only for a development checkout.

## Development

```sh
go mod download
go generate ./internal/api
go test -race ./...
go vet ./...
make pdev ARGS=help
make pdev ARGS=mcp
```

`api/openapi.yaml` is a bundled snapshot of the authoritative public contract. After updating it, run `go generate ./internal/api` and commit the generated client. CI rejects stale generated code.

See [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request. For help, read [SUPPORT.md](SUPPORT.md). Please report vulnerabilities according to [SECURITY.md](SECURITY.md), never in a public issue.

## License

Pushman CLI is available under the [MIT License](LICENSE).
