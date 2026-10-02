# Contributing to Pushman CLI

Thanks for helping improve Pushman CLI. Bug reports, focused fixes, tests, documentation, and usability feedback are welcome.

## Before you start

- Search existing issues before opening a new one.
- Use GitHub Discussions for support and open an issue for reproducible bugs or concrete proposals.
- Never include credentials, tokens, private notification content, or unredacted logs.
- For vulnerabilities, follow [SECURITY.md](SECURITY.md) instead of opening a public issue.

For a substantial feature or public command change, open a proposal first in the [Pushman product repository](https://github.com/pushmanhq/pushman). The hosted API and product behavior are maintained outside this repository, so a CLI change may require an accepted contract change before implementation.

## Development

Pushman CLI requires Go 1.27 or newer.

```sh
git clone https://github.com/pushmanhq/pushman-cli.git
cd pushman-cli
go mod download
go generate ./internal/api
go test -race ./...
go vet ./...
make pdev ARGS=help
```

The development target builds `.bin/pushman-dev` with a loopback API default, a separate Keychain namespace, `PUSHMAN_DEV_TOKEN`, and self-update disabled. It never replaces or reads credentials from an installed release command named `pushman`. `make install-dev` optionally installs the isolated shortcut as `~/.local/bin/pdev`.

### Native Windows development

Go 1.27+ and Git are sufficient; Bash, GNU Make, and Homebrew are not required. In PowerShell, run native tests without the race flag unless the additional Windows race compiler/runtime prerequisites have been provisioned:

```powershell
go mod download
if ($LASTEXITCODE -ne 0) { throw 'Dependency download failed' }
go generate ./internal/api
if ($LASTEXITCODE -ne 0) { throw 'Generation failed' }
git diff --exit-code -- internal/api/api.gen.go
if ($LASTEXITCODE -ne 0) { throw 'Generated API client changed' }
go test ./...
if ($LASTEXITCODE -ne 0) { throw 'Tests failed' }
go vet ./...
if ($LASTEXITCODE -ne 0) { throw 'Vet failed' }

$ldflags = '-X main.version=dev-local -X main.commit=working-tree -X main.date=local -X main.defaultBaseURL=http://127.0.0.1:8080/v1 -X main.credentialNamespace=dev -X main.automationTokenEnvironment=PUSHMAN_DEV_TOKEN'
go build -trimpath -ldflags $ldflags -o .bin/pushman-dev.exe ./cmd/pushman
if ($LASTEXITCODE -ne 0) { throw 'Development build failed' }
.\.bin\pushman-dev.exe help
if ($LASTEXITCODE -ne 0) { throw 'Development smoke failed' }
```

This is the same isolated configuration as `make build-dev`. `PUSHMAN_API_URL` can override its loopback default; `PUSHMAN_DEV_TOKEN` is process-scoped and independent from release `PUSHMAN_TOKEN`. Self-update is disabled. Use the explicit development path instead of replacing a release installation or sharing its credential namespace.

`go test -v ./internal/wincheck` exercises Windows shells, real console/redirected handles, native console interruption, and an actual CLI stdio MCP process using only loopback services and synthetic values. Tests create hidden, owned console processes; they do not interrupt the user's console or open a browser.

Native Credential Manager tests are opt-in and live in a separate package/process from the mock-backed unit tests:

```powershell
$previousNativeTest = $env:PUSHMAN_TEST_NATIVE_KEYRING
try {
    $env:PUSHMAN_TEST_NATIVE_KEYRING = '1'
    go test -v ./internal/credential/native
    if ($LASTEXITCODE -ne 0) { throw 'Native credential test failed' }
} finally {
    if ($null -eq $previousNativeTest) {
        Remove-Item Env:PUSHMAN_TEST_NATIVE_KEYRING -ErrorAction SilentlyContinue
    } else {
        $env:PUSHMAN_TEST_NATIVE_KEYRING = $previousNativeTest
    }
}
```

The suite writes only a unique `com.pushman.test.*` entry with a synthetic value, registers cleanup before writing, and checks restart persistence/namespace isolation/deletion. A skipped suite is `NOT RUN`; an unavailable store in an opted-in run fails verification.

CI keeps Linux race/generated-client/script/vulnerability checks, macOS portability, and native Windows x64/ARM64 jobs. The Windows jobs record their image and native architecture and run credential/process tests, vet, an explicit `.exe` build, three-shell smoke, and checksum/provenance/version/help checks on the pinned published ZIP. Use the [verification record](docs/WINDOWS_VERIFICATION.md) for release and desktop-host evidence. A PowerShell script policy may disallow a local `.ps1` test wrapper; native Go tests and typed `.exe` commands do not require changing that policy.

Before submitting a pull request:

```sh
test -z "$(gofmt -l .)"
go generate ./internal/api
git diff --exit-code -- internal/api/api.gen.go
go test -race ./...
go vet ./...
```

Keep changes focused. Add or update tests for user-visible behavior, preserve the stdout/stderr contract, and do not hand-edit the generated API client. Commit messages use `type: imperative summary`, for example `fix: reject an empty notification body`.

By contributing, you agree that your contributions are licensed under the repository's [MIT License](LICENSE). All participants must follow the [Code of Conduct](CODE_OF_CONDUCT.md).
