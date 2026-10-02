# Installing Pushman CLI

Homebrew is the recommended installation method on macOS and Linux. Go and verified release archives are supported when Homebrew is unavailable or inappropriate for the environment.

Pushman for iPhone is preparing for its first public App Store release. Check the [product repository](https://github.com/pushmanhq/pushman) for app access. Installing the CLI does not install the iPhone app, create an account, authorize automatically, start a background service, or send a notification.

## Homebrew

Install the fully qualified Formula:

```sh
brew install whitekiwi/tap/pushman
pushman version
```

Homebrew adds `whitekiwi/tap` automatically and trusts only the Pushman Formula. You do not need to trust every package in the tap.

Update an existing installation:

```sh
pushman self-update
```

`pushman self-update` verifies that the running executable belongs to the Pushman Homebrew Formula before asking Homebrew to upgrade it. It refuses Go installs, release archives, and other unowned executables; update those with their original installation method. Installations older than v0.1.1 need one manual `brew upgrade whitekiwi/tap/pushman` to gain this command.

To remove Pushman and revoke this CLI's server credential:

```sh
pushman logout
brew uninstall pushman
```

Uninstalling without `pushman logout` removes the executable but intentionally leaves its Keychain or credential-store entry and server authorization intact. Reinstalling the CLI can use that authorization again.

## Go

Go 1.27 or newer can install the latest tagged version from source:

```sh
go install github.com/pushmanhq/pushman-cli/cmd/pushman@latest
pushman version
```

The canonical Go module moved to `github.com/pushmanhq/pushman-cli` before v1.0. Existing version-pinned installs from the former owner continue to resolve through GitHub's repository redirect, but new installs and upgrades should use the command above.

The binary is normally written to `GOBIN`, or to the `bin` directory under `GOPATH` when `GOBIN` is empty. Add that directory to `PATH` if the shell cannot find `pushman`.

Update by running the same `go install` command. Before deleting a Go-installed binary, run `pushman logout` if you also want to revoke the authorization.

## Release archives

The [GitHub Releases](https://github.com/pushmanhq/pushman-cli/releases) page provides prebuilt archives for macOS, Linux, and Windows on arm64 and x86_64, with `checksums.txt`. Verify GitHub build provenance for the archive you selected as well; an older release without verifiable provenance does not pass this installation method.

Download the archive for the machine together with `checksums.txt`, then verify both before extracting it:

```sh
release_asset=pushman_<version>_<platform>_<arch>.<archive>
awk -v name="$release_asset" '$2 == name' checksums.txt | shasum -a 256 -c -
gh attestation verify "$release_asset" -R pushmanhq/pushman-cli
```

Move the extracted `pushman` or `pushman.exe` into a directory already on `PATH`. The archive does not modify shell startup files or require administrator access.

### Windows ZIP installation

Use PowerShell 7 for the examples below. Select the release tag and the architecture that matches Windows: `x86_64` for x64, or `arm64` for Windows on ARM. There is no 32-bit Windows archive. Native Windows is a CLI/MCP sender to an iPhone; it does not install a Windows notification receiver.

Install the [GitHub CLI](https://cli.github.com/) first and confirm `gh --version` works. Download and validate an archive before extracting or running it:

```powershell
$ErrorActionPreference = 'Stop'
$tag = 'v0.3.0' # Replace with the release you selected.
$arch = 'x86_64' # Use 'arm64' on Windows ARM64.
$version = $tag.Substring(1)
$asset = "pushman_${version}_windows_${arch}.zip"
$downloadDir = Join-Path $env:TEMP ('pushman-download-' + [guid]::NewGuid())
New-Item -ItemType Directory -Path $downloadDir | Out-Null

gh release download $tag -R pushmanhq/pushman-cli --pattern $asset --pattern checksums.txt --dir $downloadDir
if ($LASTEXITCODE -ne 0) { throw 'Download failed' }
$archive = Join-Path $downloadDir $asset
$pattern = '^([0-9a-fA-F]{64})\s+\*?' + [regex]::Escape($asset) + '$'
$entries = @(Get-Content -LiteralPath (Join-Path $downloadDir 'checksums.txt') |
    Where-Object { $_ -match $pattern })
if ($entries.Count -ne 1) { throw 'Expected exactly one matching checksum' }
$expected = ($entries[0] -split '\s+')[0]
$actual = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash
if ($expected -ne $actual) { throw 'Checksum mismatch' }
gh attestation verify $archive -R pushmanhq/pushman-cli
if ($LASTEXITCODE -ne 0) { throw 'Provenance verification failed' }

Expand-Archive -LiteralPath $archive -DestinationPath (Join-Path $downloadDir 'extracted')
$installDir = Join-Path $env:LOCALAPPDATA 'Programs\Pushman'
$executable = Join-Path $installDir 'pushman.exe'
if (Test-Path -LiteralPath $executable) { throw 'An installation exists; follow the update steps below' }
New-Item -ItemType Directory -Force -Path $installDir | Out-Null
Copy-Item -LiteralPath (Join-Path $downloadDir 'extracted\pushman.exe') -Destination $executable
$env:Path = "$installDir;$env:Path"
Get-Command pushman.exe -All | Select-Object Source
& $executable version
if ($LASTEXITCODE -ne 0) { throw 'Version check failed' }
```

The PATH change above applies only to this terminal. Optionally add the same directory to the **user** PATH for future terminals:

```powershell
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$parts = @($userPath -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
if ($parts -notcontains $installDir) {
    [Environment]::SetEnvironmentVariable('Path', (($parts + $installDir) -join ';'), 'User')
}
```

Open a new terminal and restart desktop MCP clients afterward. `Get-Command pushman.exe -All` identifies older or shadowing executables; use the intended absolute path if another installation wins. These steps do not require elevation or change the machine PATH. Checksums and GitHub provenance are separate from Windows Authenticode signing.

### Windows updates, rollback, and removal

For a ZIP installation, close running CLI commands and have MCP clients stop their Pushman subprocesses. Download the replacement ZIP into a new directory and repeat the checksum/provenance checks above. Keep the previous verified `pushman.exe` outside the installation directory, replace the installed executable with the newly verified one, then check its absolute-path `version` and `help`. If the new version cannot run, stop its processes and restore that previous verified executable. Windows does not allow replacing an executable while it is running.

A Go installation updates with the original `go install` command. Windows `pushman self-update` deliberately refuses these installations; it does not manage ZIP files or Go installs. WinGet, Scoop, and MSI are not installation channels documented here.

Keep a rollback binary only from an archive that passed both verification steps. Do not substitute a historical archive merely because its checksum matches if its provenance cannot be verified under `pushmanhq/pushman-cli`.

If you want to revoke this CLI's authorization, run `pushman logout` **before** deleting its executable. Logout contacts the service and removes the local credential after revocation succeeds. A failed logout is not confirmation of revocation. Then remove only the installed `pushman.exe` and its own user PATH entry, if added. Deleting the executable alone retains authorization and its credential-store entry; reinstalling can use it again.

### Windows input and execution sessions

PowerShell 7 uses UTF-8 for native pipelines. Windows PowerShell 5.1 needs an explicit `$OutputEncoding` when sending non-ASCII pipeline text to a native program. Change it only for the current operation and restore it afterward:

```powershell
$previousOutputEncoding = $OutputEncoding
try {
    $OutputEncoding = [System.Text.UTF8Encoding]::new($false)
    'Example notification 한글 😀 "quotes" $dollar & ampersand' | pushman.exe push - --json
    if ($LASTEXITCODE -ne 0) { throw 'Pushman failed' }
} finally {
    $OutputEncoding = $previousOutputEncoding
}
```

Run a send example only when you intend to send that notification after authorization. Piping the body also avoids legacy native-argument quoting differences for embedded quotes. Quote URL arguments containing `&`, and use single-quoted PowerShell literals when dollar signs/backticks should remain literal.

For `cmd.exe`, write a UTF-8 body file without a BOM and redirect its bytes:

```bat
pushman.exe push - --json < body.txt
```

Windows PowerShell 5.1 `>` and `Out-File` normally create UTF-16LE files, and `-Encoding UTF8` adds a BOM. To create a BOM-free UTF-8 body file in either PowerShell version, use `[IO.File]::WriteAllText('body.txt', $body, [Text.UTF8Encoding]::new($false))`. A UTF-8 BOM is retained as body content; it is not an encoding guess or silently removed. Invalid UTF-8 is rejected. Pushman removes exactly one terminal LF/CRLF and preserves internal newlines. See [Microsoft's encoding reference](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_character_encoding).

Credential Manager entries belong to the execution identity and logon session. An elevated/different user, SSH, CI, or Task Scheduler session may not see a desktop login. Use `login --no-browser` where browser launch is unavailable. Callers can inject a process-scoped `PUSHMAN_TOKEN` for send-only automation; it does not grant MCP read access or replace account authorization. Do not put a token in MCP JSON, command arguments, `setx`, or a plaintext token file.

[Windows verification](WINDOWS_VERIFICATION.md) records automated coverage and the remaining user-session, desktop-host, and release checks.

## Ask Claude Code

Claude Code can choose a supported method, verify the installed version, and walk through login:

```sh
claude "Install Pushman CLI from https://github.com/pushmanhq/pushman-cli using the safest supported method for this machine, verify it, then guide me through login. Ask before sending a test notification."
```

Review and approve each proposed command. The agent should not need a Pushman token, Apple credential, Google credential, or Keychain export. Browser login keeps provider credentials in the provider page; app pairing still requires approval from the signed-in Pushman iPhone app.

## Authorize and verify

After installation:

```sh
pushman version
pushman login
pushman status
```

`pushman login --no-browser` prints a code and URL without launching a browser, which is useful over SSH. `pushman pair` remains available when you prefer approval in the signed-in iPhone app. Both methods create the same account-scoped CLI authorization.

Run `pushman doctor` if authorization, local credential storage, or service connectivity fails. Never paste a CLI credential, `PUSHMAN_TOKEN`, OAuth assertion, or unredacted diagnostic output into an issue or agent conversation. Follow [SECURITY.md](../SECURITY.md) for suspected vulnerabilities and [SUPPORT.md](../SUPPORT.md) for bug-report guidance.
