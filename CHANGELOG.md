# Changelog

Notable changes to Pushman CLI will be documented here. This project follows [Semantic Versioning](https://semver.org/) once public releases begin.

## [Unreleased]

## [0.3.0] - 2026-09-27

### Added

- Build an isolated `pushman-dev` / `pdev` command for contributing, with a separate credential namespace and local API default.
- Display optional plan and billing status when supplied by the server, without enabling purchases or exposing provider identifiers.

### Fixed

- Show message revisions newest first in CLI text output while preserving API/MCP ordering and revision numbers.
- Accept additive API response fields for forward-compatible clients.

### Changed

- Use the canonical `github.com/pushmanhq/pushman-cli` Go installation path and link supported product documentation from the product hub.
- Update dependencies and the MCP SDK.

This release includes the repository/module migration from the earlier `v0.2.0` tag,
whose release archives were not published. Existing tags are preserved; use `v0.3.0`
for a complete checksummed release and the matching Homebrew update.

## [0.2.0] - 2026-08-27

### Changed

- Move the canonical repository and Go module to `github.com/pushmanhq/pushman-cli`; existing GitHub repository and release URLs continue to redirect from the former owner.

## [0.1.1] - 2026-08-26

### Added

- Update Homebrew-managed installations safely with `pushman self-update`.

## [0.1.0] - 2026-08-26

### Added

- Authorize the CLI with Google or Apple through the OAuth Device Authorization Grant using `pushman login`.
- Keep SSH and headless login browser-free with `pushman login --no-browser`.

### Changed

- Report whether the current account CLI credential came from browser login or app pairing.

## [0.1.0-beta.4] - 2026-08-25

### Added

- Serve seven typed notification, device, history, usage, status, and diagnostic tools over stdio with `pushman mcp`.

## [0.1.0-beta.3] - 2026-08-25

### Added

- Install and upgrade Pushman through `whitekiwi/tap` on macOS and Linux.
- Publish future Homebrew Formula updates from the release workflow with a tap-scoped deploy key.

## [0.1.0-beta.2] - 2026-08-25

### Fixed

- Report the tagged module version and available VCS metadata when installed with `go install`.

## [0.1.0-beta.1] - 2026-08-25

### Added

- Pairing with native operating-system credential storage.
- Rich push notifications, explicit device targeting, and update keys.
- Device listing, seven-day message history, usage, status, and diagnostics.
- Stable JSON output and environment-only automation credentials.
- Checksummed macOS, Linux, and Windows archives with GitHub artifact attestations.

[Unreleased]: https://github.com/pushmanhq/pushman-cli/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/pushmanhq/pushman-cli/compare/v0.1.1...v0.3.0
[0.2.0]: https://github.com/pushmanhq/pushman-cli/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/pushmanhq/pushman-cli/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/pushmanhq/pushman-cli/compare/v0.1.0-beta.4...v0.1.0
[0.1.0-beta.4]: https://github.com/pushmanhq/pushman-cli/compare/v0.1.0-beta.3...v0.1.0-beta.4
[0.1.0-beta.3]: https://github.com/pushmanhq/pushman-cli/compare/v0.1.0-beta.2...v0.1.0-beta.3
[0.1.0-beta.2]: https://github.com/pushmanhq/pushman-cli/compare/v0.1.0-beta.1...v0.1.0-beta.2
[0.1.0-beta.1]: https://github.com/pushmanhq/pushman-cli/releases/tag/v0.1.0-beta.1
