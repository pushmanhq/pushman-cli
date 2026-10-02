# Windows verification

Windows ZIPs for x64 and ARM64 already exist. This record separates distribution, automated tests, and user acceptance; a green build or a roadmap merge does not prove every Windows session or desktop MCP host works.

## Repeatable automated checks

The portability matrix keeps macOS and Windows x64 and adds native `windows-11-arm`. Windows jobs assert the default Go architecture rather than setting `GOARCH` to cross-compile. [GitHub's runner reference](https://docs.github.com/en/actions/reference/runners/github-hosted-runners) lists the ARM64 label.

- `go test ./...`, `go vet ./...`, and a native `pushman.exe` build.
- `PUSHMAN_TEST_NATIVE_KEYRING=1 go test -v ./internal/credential/native`: unique synthetic native entry, restart, namespace separation, delete, cleanup. The ordinary mock suite runs in a separate process.
- `go test -v ./internal/wincheck`: built main package in a spaced/Korean path; PowerShell 7/5.1 and cmd; UTF-8/CRLF/length boundary; JSON/quiet/errors; file/pipe/EOF/NUL; MCP initialize/tools/call/error/EOF/restart/in-flight cancellation.
- A hidden real console validates `CONIN$` detection. A directed `CTRL_BREAK_EVENT` to an owned test process exercises Go's `os.Interrupt` and login exit 130. This does not claim a human keyboard Ctrl-C test. [Windows console event semantics](https://learn.microsoft.com/en-us/windows/console/generateconsolectrlevent), [Go Windows signal behavior](https://pkg.go.dev/os/signal#hdr-Windows).
- PowerShell 7/5.1 and cmd run the production-style binary for version/help and expected usage/self-update failure exit codes.
- `scripts/test-windows-release.ps1 -Tag v0.3.0 -Architecture x86_64` (or `arm64`) downloads a matching published ZIP, requires exactly one checksum entry, verifies provenance and the PE machine architecture, and executes version/help. With `-CandidateExecutable .\.bin\pushman.exe`, it also verifies temporary user-directory installation/discovery, replacement with the source build, byte-exact rollback, and binary removal, restoring session PATH afterward. It does not change persistent PATH or authorization. The pinned tag is independent of the source being tested. Advance it deliberately after validating a new release.

The process-test binary uses a test credential namespace. API requests use a loopback endpoint and synthetic token. Tests never access production credentials or send real notifications. Windows race prerequisites are not silently assumed; Linux's existing race and other checks remain mandatory.

## Evidence and remaining acceptance

For every implementation/release record include commit and CI URL, runner image, Windows build, native OS/executable architecture, shell/client versions, exit codes, exact asset/tag/hash, and `PASS`, `FAIL`, `NOT RUN`, or `N/A` with a reason. Never upload tokens, codes, user/host names, actual private paths, account/device identifiers, or notification content.

| Check | Current local evidence | Remaining acceptance |
| --- | --- | --- |
| Native source tests/vet/build | Windows 11 x64 build 26200; Go 1.27.1. Results attach to the implementation PR. | Native CI results for the exact commit, including ARM64. |
| Shell/input | PowerShell 7.6.5, Windows PowerShell 5.1, cmd; Unicode, quoting, UTF-8, CRLF, maximum body, output/exit contracts pass against a loopback API. | Human keyboard input/Ctrl-C and other advertised Windows builds. |
| Console interrupt | Hidden real console handle and directed CTRL-BREAK login cancellation pass. | Keyboard Ctrl-C in an ordinary user console. |
| Native credentials | Non-administrator execution; isolated set/get/restart/separation/delete passes. | Different identity/elevated/SSH/Task Scheduler and unavailable-store session results. |
| Actual CLI MCP process | SDK initialize/tools/send/error/read denial/EOF/restart/client cancellation passes from a spaced/Korean path. | Acceptance in a named desktop host/version using account authorization. |
| Published x64 ZIP | v0.3.0 checksum/provenance/version/help pass; SHA-256 `d622adf0bb0ec9dce6a7e0c9e776a70268f018cfb75f033cfd00bdd9b7615319`. Temporary non-administrator install/source replacement/byte-exact rollback/removal is checked by the release probe. | Final distributed bytes for the release containing the input fix; persistent user PATH/fresh terminal and authorized uninstall acceptance. |
| Published ARM64 ZIP | `NOT RUN` locally; no local ARM64 hardware. | Native ARM64 CI source and pinned-archive results, then claimed user journeys. |
| Browser/account/iPhone | Injected browser failure and mocked approval/denial/expiry/slow-down/cancellation/store failure pass. | Actual login/pair/send/logout with available iPhone app access and synthetic notification content. |

The older v0.1.1 x64 archive passed its SHA-256 comparison but could not be provenance-verified under the current canonical repository (attestation lookup returned 404). The probe stopped before extraction/execution; it is not a verified rollback candidate. The v0.3.0 archive passed both checks.

## Support and release decisions

Keep ZIP and Go as the Windows installation channels. Manual ZIP replacement/rollback and Go updates use the original channel; Homebrew self-update remains macOS/Linux only. Authenticode, WinGet/Scoop/MSI, a native updater, services, and new authentication permissions are separate decisions, not prerequisites silently introduced here.

Prioritize a serviced Windows 11 x64/PowerShell 7 user journey and record PowerShell 5.1/cmd/ARM64 separately. Do not infer Windows 10/Server/WSL or enterprise proxy/security-policy acceptance from those results, or remove existing compatibility merely because it has not been tested.

The maintainer must record QA/desktop/iPhone/public-docs/support ownership and review unverified combinations before broadening support claims or publishing a release. Signing purchases and package registrations remain separate work. Public product docs should describe supported installation and PC-to-iPhone sending, not unreleased operational decisions.
