# Installing Pushman CLI

Homebrew is the recommended installation method on macOS and Linux. Go and verified release archives are supported when Homebrew is unavailable or inappropriate for the environment.

Pushman for iPhone is currently in private beta. Installing the CLI does not install the iPhone app, create an account, authorize automatically, start a background service, or send a notification.

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

The [GitHub Releases](https://github.com/pushmanhq/pushman-cli/releases) page provides prebuilt archives for macOS, Linux, and Windows on arm64 and x86_64. Every release includes `checksums.txt`, and every archive has GitHub build provenance.

Download the archive for the machine together with `checksums.txt`, then verify both before extracting it:

```sh
release_asset=pushman_<version>_<platform>_<arch>.<archive>
awk -v name="$release_asset" '$2 == name' checksums.txt | shasum -a 256 -c -
gh attestation verify "$release_asset" -R pushmanhq/pushman-cli
```

Move the extracted `pushman` or `pushman.exe` into a directory already on `PATH`. The archive does not modify shell startup files or require administrator access.

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

## Windows support roadmap

Windows release builds and portability CI are already configured. The [Windows support plan](WINDOWS_SUPPORT_PLAN.md) tracks remaining shell, credential, MCP, packaging, and validation work; it does not change current installation or update behavior.
