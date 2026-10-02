# Windows support completion plan

Status: proposed implementation roadmap; no runtime, workflow, or release change.
Reviewed on 2026-10-02 against `main` at `a2e62d62cae854695c0b00d7565bd6098e7b678f`.

Windows is not a greenfield port. The repository already contains Windows build targets, a native CI job, browser launching, and credential-store integration. This plan closes verification and documentation gaps without treating untested scenarios as confirmed defects or claiming that Windows acceptance testing has passed.

## Existing baseline

| Area | Evidence in this repository | Remaining work |
| --- | --- | --- |
| Release builds | [GoReleaser](../.goreleaser.yaml) includes `windows/amd64` and `windows/arm64`, `CGO_ENABLED=0`, and ZIP archives. | Verify the actual archive contents and executable on each claimed runtime architecture. |
| Native CI | [CI](../.github/workflows/ci.yml) already runs `go test ./...` and `go build -trimpath ./cmd/pushman` on `windows-latest`. | Add shell/process smoke tests and identify the actual runner image and architecture; do not add a duplicate Windows build job. |
| Credentials | [Keyring adapter](../internal/credential/store.go) uses `go-keyring`; the [README](../README.md) names Windows Credential Manager. | Exercise native read/write/delete under an isolated test namespace and document session restrictions. |
| Browser authorization | [Browser launcher](../internal/browser/open.go) has a Windows `rundll32` branch; [installation guidance](INSTALL.md) documents `login --no-browser`. | Verify launch success, launch failure, cancellation, and the manual URL/code path. |
| Terminal and input | [Windows terminal detection](../internal/cli/terminal_windows.go) checks `os.ModeCharDevice`; [push input](../internal/cli/push.go) validates UTF-8 and removes one trailing LF or CRLF. | Test real console, pipe, file, and null-device handles, plus shell encoding and quoting. |
| Updates | [Updater](../internal/selfupdate/homebrew.go) deliberately restricts `self-update` to Homebrew on macOS/Linux. | Keep that boundary and document Windows updates through the original installation method. |
| MCP and installation docs | [MCP guide](MCP.md) demonstrates Unix executable paths; [installation guide](INSTALL.md) verifies archives using Unix shell utilities. | Add tested PowerShell examples, Windows executable discovery, and JSON path escaping. |
| Release provenance | [Release workflow](../.github/workflows/release.yml) publishes archives and attests `dist/checksums.txt` subjects. | Verify provenance on the downloaded Windows archive; do not equate this with Windows executable signing. |

The table records configuration and source inspection, not a fresh CI result or a completed interactive Windows test.

## Scope and boundaries

Prioritize a currently serviced Windows 11 x64 installation with PowerShell 7 for the first complete user journey. Validate Windows PowerShell 5.1 and `cmd.exe` separately. Continue producing the existing ARM64 archive, but record native ARM64 runtime evidence separately from x64 tests or cross-compilation. These are proposed validation priorities, not a unilateral removal of current compatibility or a new support guarantee.

The scope is the existing `pushman.exe` sender and stdio MCP server. It does not add a Windows notification receiver, Android support, a Windows service, scheduled-task registration, new API behavior, or new authentication permissions. WSL running the Linux binary is a separate environment; it is not proof that native Windows works. Maintainer decisions beyond implementation are recorded in the PR description.

## P0 — Native regression coverage and shell correctness

Keep the current portability job and extend it with focused tests rather than rewriting the pipeline.

- Run native Windows `go test ./...`, `go vet ./...`, and an explicit `pushman.exe` build. Keep the existing Linux race, generated-client, shell-script, and vulnerability checks. Add a Windows race job only after its compiler/runtime prerequisites are deliberately provisioned; cross-compilation is not a runtime test.
- Add process-level smoke coverage with explicit `pwsh` and `powershell` shells and a separate `cmd.exe` invocation. Check native exit codes explicitly, including `$LASTEXITCODE` in PowerShell; a command that prints an error must not accidentally leave a green smoke step.
- Cover console stdin, an ordinary pipe, redirected files, EOF, and `NUL` in Windows-specific terminal tests. Preserve injectable I/O and the existing rule that a body argument plus piped stdin is rejected. Investigate a console-handle API only if a reproducible test demonstrates that the current character-device heuristic is insufficient.
- Exercise Korean text, emoji, spaces, quotes, backticks, dollar signs, ampersands in URLs, multiline input, and exactly one terminal LF/CRLF. Preserve internal newlines and the 4,096-Unicode-scalar limit. Reject invalid UTF-8 rather than silently guessing a legacy encoding or changing the public input contract.
- Use an injected service or local mock HTTP server for send assertions. Check successful JSON-only stdout, diagnostics on stderr, quiet output, and nonzero failures without production credentials or real notifications.

PowerShell string-to-native-program encoding and file-output encoding are different concerns. Windows PowerShell 5.1 also has different defaults from modern PowerShell. Test both, document any session-scoped UTF-8 setup actually required, and restore settings rather than editing a user's profile or global code page. See Microsoft's [character-encoding documentation](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_character_encoding).

Acceptance: native smoke tests and regression fixtures pass on the recorded Windows image; console-only cases have a separate interactive result. A green unit-test job alone does not satisfy the interactive cases.

## P1 — Authorization, credential lifecycle, and MCP

Review `internal/browser`, `internal/credential`, `internal/client`, `internal/cli`, and `cmd/pushman/main.go` together; keep the current service contract and credential namespace separation.

- Cover browser launch failure, `login --no-browser`, expired/denied authorization, and Ctrl-C during polling without changing provider behavior. Keep browser launch injectable so automated tests do not open a real browser.
- Add opt-in Windows Credential Manager integration tests with a unique test service name and synthetic secret. Register cleanup before writing, exercise set/get/delete and missing entries, and never access or delete the production credential. A skipped or unavailable native store test must be reported as unverified, not passed.
- Verify persistence across CLI restarts, production/development namespace isolation, and local cleanup separately from server-side revocation. Test the current logout flow with a mock service before a human-approved account test.
- Exercise an ordinary interactive account, a different execution identity, and an unavailable credential store. Do not promise that a desktop login works in every SSH, CI, or Task Scheduler session: Windows credential reads use the current token's logon session ([Microsoft reference](https://learn.microsoft.com/en-us/windows/win32/api/wincred/nf-wincred-credreadw)). Keep automation secrets process-scoped and injected by the caller; `PUSHMAN_TOKEN` remains send-only, not a substitute for account-authorized MCP read tools.
- Launch MCP using an absolute executable path and `args: ["mcp"]`, including a path with spaces and non-ASCII characters. Verify initialization, a mocked tool call, protocol-only stdout, EOF shutdown, client cancellation, and a clean restart. Do not add a shell wrapper, background service, or token-bearing config as the default workaround.

Acceptance: isolated native credential evidence and a desktop MCP-client smoke result are recorded. End-to-end login/pair/send/logout on a real account is a separate, explicitly approved manual test using synthetic notification content; it is never triggered by CI or installation.

## P2 — Windows installation, archive validation, and development docs

Use the existing ZIP and Go installation channels first. Keep WinGet, Scoop, MSI, an automatic installer, and a native self-updater outside the initial implementation unless separately approved.

1. Extend `docs/INSTALL.md` with tested PowerShell instructions for the existing `pushman_<version>_windows_x86_64.zip` and `pushman_<version>_windows_arm64.zip` naming convention. Select a matching architecture explicitly; do not advertise a 32-bit archive.
2. Show exact-filename checksum matching with `Get-FileHash -Algorithm SHA256`, followed by `gh attestation verify` against `pushmanhq/pushman-cli`. Stop on a missing/mismatched checksum or failed provenance verification before extraction/execution; document the GitHub CLI prerequisite. Extract with `Expand-Archive` only after validation.
3. Use a user-writable installation directory. Explain temporary versus persistent user `PATH`, a fresh terminal after PATH changes, and `Get-Command pushman.exe` to detect an older/shadowing installation. Do not require elevation, edit the machine PATH, or execute downloaded script text automatically.
4. Document ZIP updates by closing running CLI/MCP processes, verifying the replacement archive, replacing the executable, and checking `version`; retain a previously verified binary for rollback. Go installations update through `go install`. Keep the deliberate Windows `self-update` refusal and never claim it manages these installations.
5. Distinguish uninstalling the binary from revoking authorization with `logout`. Explain the credential-retention behavior already documented for other installation methods; never silently remove unrelated Credential Manager entries.
6. Extend `docs/MCP.md` with `(Get-Command pushman.exe).Source`, a properly JSON-escaped absolute path, and restarting a desktop client after PATH changes. Preserve the send-only automation-token limitation.
7. Add native PowerShell build/test instructions to `CONTRIBUTING.md`. Keep the existing Go requirement from `go.mod` and avoid making GNU Make, Bash, or Homebrew prerequisites for Windows. Any development shortcut must retain the existing isolated endpoint, credential namespace, and token-environment behavior.

Archive verification must target the actual release candidate bytes. If signing or packaging is added later, checksums/provenance must cover the final distributed bytes. No release is published by this planning PR.

Acceptance: a clean non-administrator Windows account can follow the documented install, version/help, update, rollback, and uninstall procedures. Record ARM64 execution separately; a successful x64 run or successful ARM64 build does not satisfy that row.

## Verification record for the implementation PRs

Attach results in this shape; use `PASS`, `FAIL`, `NOT RUN`, or `N/A` with a reason. Do not mark this checklist complete merely because the plan was merged.

| Verification | Required evidence |
| --- | --- |
| Native Windows unit tests, vet, build, and shell smoke | Commit, CI run, runner image, Windows version, architecture, shell versions, and exit codes. |
| Console and input behavior | Interactive and redirected-input cases, UTF-8/CRLF fixtures, quoting, JSON/stderr separation, and cancellation results. |
| Native credentials | Isolated namespace set/get/delete, cleanup, restart persistence, and unavailable-store behavior; no secret values in evidence. |
| Browser login and iPhone pairing | Human-approved account flow, no-browser fallback, denial/expiry/cancellation, and revocation/cleanup results. |
| Desktop MCP host | Client/version, resolved executable, initialization/tool call, stdout purity, EOF/cancellation, and restart result. |
| Windows release ZIP | Exact tag, architecture, asset name/hash, provenance verification, extracted binary version, and non-admin lifecycle test. |
| Native ARM64 | Actual native host and executable architecture; explicitly `NOT RUN` until hardware or a verified native runner is available. |
| macOS/Linux regression | Existing CI remains green; generated API code and public behavior remain unchanged. |

Use mocks and synthetic values by default. Redact local usernames/paths, pairing codes, account/device identifiers, and notification content from screenshots and logs. Do not upload an environment dump, credential-store export, or raw tokens.

## Completion gate

- [ ] P0 native CI and interactive shell evidence is attached.
- [ ] P1 authorization, isolated credential, and MCP evidence is attached.
- [ ] P2 exact-archive installation/update/uninstall evidence is attached.
- [ ] ARM64 runtime status is stated independently of build availability.
- [ ] Every advertised environment is either verified or explicitly scoped with a documented limitation.
- [ ] Public installation, MCP, contribution, support, and product documentation agree.
- [ ] The maintainer has resolved the support/operations decisions in the PR description before broadening Windows support claims.

Merging this roadmap satisfies none of the runtime gates by itself. Implementation, release publication, package-manager registration, and any paid signing service remain separate work.
