# Windows distribution

The Windows channel uses Inno Setup 6.7.3 with a pinned bootstrap checksum, per-user installation, native system appearance, a stable app identity, and owned PATH registration. The setup assets are architecture-specific; users need neither Go nor elevation. Signing is deferred; no signing credentials or integration are required by these workflows.

Manifest-validation jobs prepare WinGet 1.29.380 with Microsoft's pinned `Microsoft.WinGet.Client` module and `Repair-WinGetPackageManager` when the disposable runner lacks App Installer. This is a CI prerequisite, not part of the Pushman installer. Microsoft schemas and native `winget validate` remain mandatory.

## Release gates

Push a new immutable `v<major>.<minor>.<patch>` tag after review and merge, or dispatch Release with an existing unpublished tag. Published releases cannot be rebuilt or have their assets replaced. A correction gets a new version.

1. Resolve and validate the tag's exact commit. Refuse an already public release.
2. Run the reusable full CI at that commit: Linux formatting/generation/race/vet/vulnerability checks, macOS tests/build, native Windows x64/ARM64 credential/shell/console/MCP/ZIP checks, installer lifecycle, and WinGet manifest validation.
3. Build both final installers on native Windows. Test those exact bytes through install, reinstall, update/rollback fixtures, PATH opt-out and preexisting-entry handling, and uninstall. No installer is rebuilt after qualification.
4. Generate and validate the WinGet submission bundle from the final files and immutable tag URLs.
5. GoReleaser builds the six existing archives and adds both installers and the bundle to a **draft** release. All final files enter `checksums.txt`.
6. Verify installer checksum coverage, attest the checksum subjects, then publish the draft. Update the Homebrew tap using its existing deploy key when configured.

An interrupted run before publication leaves the release unpublished. Resume an unpublished draft only at the same tag; investigate failures rather than editing or replacing public files. CI candidate artifacts use `0.0.1-ci` and are temporary verification artifacts, not published versions. Update/rollback fixtures test replacement behavior, not the validity of an older public release. Human UI checks for appearance, keyboard navigation and screen readers remain part of the [Windows verification record](WINDOWS_VERIFICATION.md).

## WinGet catalog submission

Catalog submission happens **after** a stable release is public. Microsoft validates the package independently; a release bundle or PR does not make `winget install` available. See [Microsoft's submission requirements](https://learn.microsoft.com/en-us/windows/package-manager/package/repository).

On Windows with PowerShell 7 and GitHub CLI, download the release's two `setup.exe` files, `checksums.txt`, and `pushman_<version>_winget.zip` into a fresh directory. Verify each file's exact checksum and `gh attestation verify <file> -R pushmanhq/pushman-cli` before extraction. Confirm both immutable installer URLs are publicly downloadable, without login. Recreate and validate manifests from those final files:

```powershell
# Set $tag and $downloadDir to that verified, published stable release.
$manifest = .\scripts\new-winget-manifests.ps1 -Tag $tag -InstallerDirectory $downloadDir -RequireWinget
```

The generator uses `PushmanHQ.Pushman`, official pinned Microsoft schemas, user scope, Inno silent switches, the installed-app product code, and exact installer SHA-256 values. It never uploads manifests or invokes an installer.

Install Microsoft's manifest tool with `winget install --id Microsoft.WingetCreate --exact`. Authenticate it with `wingetcreate token --store` using its browser flow; avoid putting tokens in command arguments or scripts. Submit the generated version directory with `wingetcreate submit $manifest.Directory`. Alternatively, copy the verified bundle's `manifests/p/PushmanHQ/Pushman/<version>` directory into a fork of `microsoft/winget-pkgs` and open a focused PR.

Record the catalog PR, resolve its installation/validation results, and wait for acceptance. Then check `winget show --id PushmanHQ.Pushman --exact`, and verify clean install, detection, upgrade, and uninstall before documenting catalog availability. No new repository token is needed for building or publishing the submission bundle.

## Ownership and rollback

The app identity is `222c0b32-df1d-468b-a624-913a96355115`; keep it stable across versions and architectures. HKCU `Software\Pushman\Installer` records only installer-owned PATH state. No machine PATH, account authorization, keyring data or server state is changed by packaging. The uninstaller deletes only files recorded by Inno and the owned PATH occurrence; preexisting matching entries are retained. Stop CLI/MCP processes before replacing executable files.

Rollback uses an older verified installer with the same destination and identity. Store it outside the installed directory. Logout/revocation is a separate user action; installation, update, rollback and removal must never invoke it.
