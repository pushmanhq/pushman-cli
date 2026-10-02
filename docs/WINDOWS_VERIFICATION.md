# Windows verification

Windows releases include per-user x64/ARM64 installers and optional ZIPs. This record separates distribution, automated tests, and user acceptance; a green build or a roadmap merge does not prove every Windows session or desktop MCP host works.

## Repeatable automated checks

The portability matrix keeps macOS and Windows x64 and adds native `windows-11-arm`. Windows jobs assert the default Go architecture rather than setting `GOARCH` to cross-compile. [GitHub's runner reference](https://docs.github.com/en/actions/reference/runners/github-hosted-runners) lists the ARM64 label.

- `go test ./...`, `go vet ./...`, and a native `pushman.exe` build.
- `PUSHMAN_TEST_NATIVE_KEYRING=1 go test -v ./internal/credential/native`: unique synthetic native entry, restart, namespace separation, delete, cleanup. The ordinary mock suite runs in a separate process.
- `go test -v ./internal/wincheck`: built main package in a spaced/Korean path; PowerShell 7/5.1 and cmd; UTF-8/CRLF/length boundary; JSON/quiet/errors; file/pipe/EOF/NUL; MCP initialize/tools/call/error/EOF/restart/in-flight cancellation.
- A hidden real console validates `CONIN$` detection. A directed `CTRL_BREAK_EVENT` to an owned test process exercises Go's `os.Interrupt` and login exit 130. This does not claim a human keyboard Ctrl-C test. [Windows console event semantics](https://learn.microsoft.com/en-us/windows/console/generateconsolectrlevent), [Go Windows signal behavior](https://pkg.go.dev/os/signal#hdr-Windows).
- PowerShell 7/5.1 and cmd run the production-style binary for version/help and expected usage/self-update failure exit codes.
- `scripts/test-windows-release.ps1 -Tag v0.4.2 -Architecture x86_64` (or `arm64`) downloads a matching published ZIP, requires exactly one checksum entry, verifies provenance and the PE machine architecture, and executes version/help. With `-CandidateExecutable .\.bin\pushman.exe`, it also verifies temporary user-directory installation/discovery, replacement with the source build, byte-exact rollback, and binary removal, restoring session PATH afterward. It does not change persistent PATH or authorization. The pinned tag is independent of the source being tested. Advance it deliberately after validating a new release.
- `scripts/test-windows-installer.ps1` qualifies an actual candidate setup on a disposable native account: install/reinstall/update/rollback fixtures, installed-app registration, executable bytes/version/help, PATH discovery/opt-out/preexisting entries, unrelated-file preservation, uninstall, and exact persistent user PATH value/type/existence restoration. The test refuses an existing registered installation. Both native CI architectures run it; WinGet manifests use the two qualified files and pass official schema/native validation.

The process-test binary uses a test credential namespace. API requests use a loopback endpoint and synthetic token. Tests never access production credentials or send real notifications. Windows race prerequisites are not silently assumed; Linux's existing race and other checks remain mandatory.

## Evidence and remaining acceptance

For every implementation/release record include commit and CI URL, runner image, Windows build, native OS/executable architecture, shell/client versions, exit codes, exact asset/tag/hash, and `PASS`, `FAIL`, `NOT RUN`, or `N/A` with a reason. Never upload tokens, codes, user/host names, actual private paths, account/device identifiers, or notification content.

| Check | Current evidence | Remaining acceptance |
| --- | --- | --- |
| Native source tests/vet/build | Windows 11 x64 build 26200 / Go 1.27.1 locally; [PR #28 CI](https://github.com/pushmanhq/pushman-cli/actions/runs/37038327397) passes Linux/macOS/native Windows x64/ARM64 and WinGet validation at `5945365`. The reviewed and merged `a97403a` trees match. | Other claimed environments; tagged-file results are recorded separately below. |
| Shell/input | Local PowerShell 7.6.5 / Windows PowerShell 5.1 / cmd and both CI architectures pass Unicode, quoting, UTF-8, CRLF, maximum body, output/exit contracts against a loopback API. CI also passes full PS7/5.1 script wrappers. | Human keyboard input/Ctrl-C and other advertised Windows builds. |
| Console interrupt | Hidden real console handle and directed CTRL-BREAK pass: login exits 130; MCP stops quietly with exit 0. | Keyboard Ctrl-C in an ordinary user console. |
| Native credentials | Non-administrator local execution and both CI architectures pass isolated set/get/restart/separation/delete. | Different identity/elevated/SSH/Task Scheduler and unavailable-store session results. |
| Actual CLI MCP process | SDK initialize/tools/send/error/read denial/EOF/restart/client cancellation and native process interruption pass from a spaced/Korean path on both CI architectures. | Desktop-host manual acceptance is **WAIVED / NOT RUN** by the maintainer. This is not a desktop-host PASS; automated native MCP checks remain mandatory. |
| Published x64 ZIP | Verified public v0.4.1 -> public v0.4.2 replacement -> byte-exact v0.4.1 rollback -> removal passes locally without elevation. The latest release's exact files are recorded below. | Fresh Explorer-launched terminal inheritance and the authorized account journey. |
| Published ARM64 ZIP | Native CI passes checksum/provenance/PE `0xAA64`/version/help and temporary installation/replacement/rollback/removal. The latest release's exact files are recorded below. | `NOT RUN` locally: no ARM64 hardware. Non-administrator ARM64 account acceptance remains separate; desktop-host manual acceptance is waived. |
| Native installer and PATH | Both native CI architectures pass full installer lifecycle. Actual public v0.4.2 x64 setup passes non-administrator GUI installation, Installed apps/version/help, fresh registry-derived child-process discovery with Go absent and standard GUI uninstall. User and machine PATH value/type/existence exactly match the pre-install snapshot. The v0.4.1 public setup also preserved an unrelated file. | Fresh Explorer-launched terminal and screen-reader acceptance are `NOT RUN`. The child process rebuilt PATH from registry; it is not an Explorer-terminal test. |
| Browser/account/iPhone | Injected browser failure and mocked approval/denial/expiry/slow-down/cancellation/store failure pass. The maintainer confirms app/account readiness. An unapproved real login challenge expired with exit 1 and stored no authorization. | Actual approved login/pair/send/iPhone receipt/logout. An expired challenge does not satisfy account acceptance. |
| WinGet catalog | [Submission #445791](https://github.com/microsoft/winget-pkgs/pull/445791) is open. Official schemas and native `winget validate` pass; provider checks and personal CLA approval are separate gates. | Catalog acceptance and `show/install/upgrade/uninstall` remain pending. Local-manifest install is `NOT RUN` because it is disabled on the non-administrator host; no security setting was changed. |

## Published release evidence

[v0.4.2](https://github.com/pushmanhq/pushman-cli/releases/tag/v0.4.2) was published on 2026-10-02 at 17:22:33 UTC from immutable commit `a97403a0c7495ca90d50b9bfd795a59e2a1f97f5`. [Release CI](https://github.com/pushmanhq/pushman-cli/actions/runs/37038942003), [merged-main CI](https://github.com/pushmanhq/pushman-cli/actions/runs/37038893139), and [CodeQL](https://github.com/pushmanhq/pushman-cli/actions/runs/37038893115) are PASS. Full quality, both final native installer lifecycles, mandatory WinGet bundle validation, publication and Homebrew tap update succeeded.

All nine public archive/setup/bundle files passed their single exact checksum entry and provenance verification pinned to the release tag, commit and `.github/workflows/release.yml`, with self-hosted signers denied. Both anonymous HTTPS setup downloads exactly matched the files qualified by native CI; no installer was rebuilt after qualification. The installers are unsigned. The [release checksums](https://github.com/pushmanhq/pushman-cli/releases/download/v0.4.2/checksums.txt) cover all files; Windows hashes are recorded here for comparison.

| Public asset | SHA-256 |
| --- | --- |
| `pushman_0.4.2_windows_x86_64_setup.exe` | `96d2f682f26a9146be6d9804aa1d81738b8d5c83e6cc395d4f66b6fd3f85d095` |
| `pushman_0.4.2_windows_arm64_setup.exe` | `d3ba55d85b483af63a8e8d72f413b8db978b8faaf245e73c18acdecf7bbcb60f` |
| `pushman_0.4.2_windows_x86_64.zip` | `d61f09b1080932842321440523791ae76443d05f19ab5487de67cce74ec4818b` |
| `pushman_0.4.2_windows_arm64.zip` | `2fb5dac25ba464a4b84a6df4ddd18340733f7b29c3e192de6d2e34f8f85caa7d` |
| `pushman_0.4.2_winget.zip` | `aae01af0d0e3ffe23c6460d5d515c3878579df45745f282f333dd1af98efef38` |

| Release quality job | Native environment |
| --- | --- |
| [x64](https://github.com/pushmanhq/pushman-cli/actions/runs/37038942003/job/110944041020) | `win25-vs2026` / `20260925.250.1`, Windows build 26100, OS X64 / Go 1.27.0 amd64, PowerShell 7.6.6 / 5.1.26100.33438 |
| [ARM64](https://github.com/pushmanhq/pushman-cli/actions/runs/37038942003/job/110944040990) | `win11-vs2026-arm64` / `20260924.168.1`, Windows build 26200, OS Arm64 / Go 1.27.0 arm64, PowerShell 7.6.6 / 5.1.26100.9457 |

The exact public x64 v0.4.2 setup was installed locally without elevation on Windows 11 build 26200. Installed apps metadata, version/help and a fresh registry-derived PowerShell child process passed with Go absent and administrator status false. Its English completed page showed the login/help/restart text and square brand image; UI Automation exposed the text and keyboard Finish focus. The Korean completed-page preview was inspected separately. Standard GUI uninstall succeeded and removed the executable, app registration and owned PATH state; raw user/machine PATH value/type/existence exactly matched the pre-install snapshot. The setup payload reports `a97403a` and build time 17:16:45 UTC; the public ZIP reports the same commit with build time 17:21:03 UTC. Those separate builds are not compared as identical executable payloads.

Local verified public v0.4.1 ZIP -> public v0.4.2 ZIP replacement -> byte-exact v0.4.1 rollback -> removal passed, with session PATH restored. CI now pins its published-ZIP probe to v0.4.2 on each native architecture. Existing v0.4.1 release files and the unpublished v0.4.0 tag remain unchanged.

[Public product documentation](https://github.com/pushmanhq/pushman/pull/3) points installation examples at v0.4.2. [WinGet submission](https://github.com/microsoft/winget-pkgs/pull/445791) contains only the v0.4.2 three-file manifest set: local official schemas/native validation pass, and regenerated manifests byte-match the attested bundle before repository CRLF formatting. Personal CLA agreement, provider acceptance and catalog lifecycle remain pending; neither the bundle nor the PR establishes WinGet availability.

## Historical source qualification

Source commit `dc86303745b9c0058498ed0848ac317df5ab2632` passed [CI](https://github.com/pushmanhq/pushman-cli/actions/runs/37008806009) and [CodeQL](https://github.com/pushmanhq/pushman-cli/actions/runs/37008805932). The [x64 job](https://github.com/pushmanhq/pushman-cli/actions/runs/37008806009/job/110843311146) recorded image `win25-vs2026` / `20260925.250.1`, Windows build 26100, native amd64, PowerShell 7.6.6 and Windows PowerShell 5.1.26100.33438. The [ARM64 job](https://github.com/pushmanhq/pushman-cli/actions/runs/37008806009/job/110843310838) recorded `win11-vs2026-arm64` / `20260924.168.1`, Windows build 26200, native arm64, PowerShell 7.6.6 and Windows PowerShell 5.1.26100.9457. This qualifies the tested runtime combinations; it does not establish full user journeys on every Windows Server or ARM64 session.

The documented isolated development build also passed version/help and disabled self-update locally. Go 1.27.1 installed v0.4.1 and then v0.4.2 into the same isolated temporary `GOBIN`; both versions passed version/help without changing persistent PATH or credentials. The earlier v0.3.0 same-version reinstall was not used as release-upgrade evidence.

The first native CI run found that Windows PowerShell 5.1 prepended a UTF-8 BOM with only `$OutputEncoding` set to BOM-free UTF-8. Local behavior differed. The console example now aligns console input encoding for the operation and restores both settings; the process test uses an owned hidden console so it cannot alter the runner's shared console. Pushman's BOM/content policy is unchanged.

The older v0.1.1 x64 archive passed its SHA-256 comparison but could not be provenance-verified under the current canonical repository (attestation lookup returned 404). The probe stopped before extraction/execution; it is not a verified rollback candidate. The v0.3.0 archive passed both checks.

## Support and release decisions

The per-user installer is the Windows default for releases containing setup assets; existing v0.3.0 downloads remain ZIP-only. ZIP replacement/rollback and Go updates use their original channel; Homebrew self-update remains macOS/Linux only. [Windows distribution](WINDOWS_DISTRIBUTION.md) defines full CI, native installer qualification, checksums/provenance, and draft-before-publication gates. WinGet manifests and catalog acceptance are separate stages. Authenticode is explicitly backlog only; Scoop/MSI, a native updater, services, and new authentication permissions remain outside this change.

The local native x64 installer lifecycle passed with the v0.4.2 UI correction, including Korean/space paths and exact PATH restoration. The completed Korean dark-mode preview displayed the canonical square logo and login/help/restart instructions, exposed the text through UI Automation, and kept keyboard focus on Finish. That preview was removed through its standard uninstaller. The inspected v0.4.1 final installer exposed the completion-page bug fixed by [PR #28](https://github.com/pushmanhq/pushman-cli/pull/28); final-file English inspection is recorded above. Other theme/scale combinations and actual screen-reader acceptance remain `NOT RUN`. A synthetic source-version rollback does not qualify an older public release.

Prioritize a serviced Windows 11 x64/PowerShell 7 user journey and record PowerShell 5.1/cmd/ARM64 separately. Do not infer Windows 10/Server/WSL or enterprise proxy/security-policy acceptance from those results, or remove existing compatibility merely because it has not been tested.

The repository maintainer owns release approval, public documentation and support intake. Native x64/ARM64 CI owns repeatable source and packaging qualification; the maintainer performs account/iPhone and any additional human session/UI acceptance. The maintainer authorized unsigned release/catalog work, deferred signing to backlog and waived desktop-host manual acceptance. Other unverified combinations remain explicit limitations before support is broadened. Public product docs describe supported installation and PC-to-iPhone sending; provider acceptance and publisher signing remain separate gates.
