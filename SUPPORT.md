# Support

## Getting help

Start with these commands:

```sh
pushman doctor
pushman status
pushman help
```

Search existing [issues](https://github.com/pushmanhq/pushman-cli/issues) before opening a new one. Use the provided bug-report form and include the CLI version, operating system, command shape, expected result, and redacted output.

This repository supports the open-source CLI. For the iPhone app, accounts, quota, hosted-service behavior, or general product help, use the [Pushman support guide](https://github.com/pushmanhq/pushman/blob/main/SUPPORT.md).

For Windows reports, include the Windows build, x64/ARM64 architecture, shell name/version, installation method, CLI version, and exit code. Describe whether the executable path contains spaces/non-ASCII characters or shadows another installation; redact the username and actual private path. Use `Get-Command pushman.exe -All` locally to check discovery and report only the relevant path characteristics.

Windows CLI/MCP sends to the iPhone app; it does not provide a Windows notification receiving app. Native x64/ARM64 archive availability, automated runtime coverage, and user-session/desktop-host acceptance are recorded separately in [Windows verification](docs/WINDOWS_VERIFICATION.md). WSL using a Linux executable is a separate environment.

When browser launch is restricted, use `pushman login --no-browser`. Credential availability can differ between interactive, elevated, different-user, SSH, CI, and scheduled-task sessions. Follow organization-approved proxy/certificate policy for connectivity failures and keep TLS verification enabled. Do not export the credential store or share an environment-variable dump for diagnosis.

## Protect your data

Never share a `PUSHMAN_TOKEN`, account CLI credential, OAuth assertion, notification content, private hostname, or unredacted diagnostic output. If you believe you found a security issue, follow [SECURITY.md](SECURITY.md).
