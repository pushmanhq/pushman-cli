# Intended for disposable CI accounts lacking App Installer, not end-user setup.
$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSScriptRoot 'windows-packaging.psm1') -Force
$version = '1.29.380'
$winget = Get-Command winget.exe -ErrorAction SilentlyContinue
if (-not $winget) {
    # Microsoft's supported bootstrap installs dependencies and registers the
    # signed App Installer for this account; no machine policy changes.
    Install-Module Microsoft.WinGet.Client -Repository PSGallery -RequiredVersion $version -Scope CurrentUser -Force
    Import-Module Microsoft.WinGet.Client -RequiredVersion $version
    # Exact versions are passed to the GitHub release API as tag names; the
    # wildcard example in Microsoft's help resolves its own v-prefixed tag.
    Repair-WinGetPackageManager -Version "v$version" -Force | Out-Null
    $windowsApps = Join-Path $env:LOCALAPPDATA 'Microsoft\WindowsApps'
    $env:Path = "$windowsApps;$env:Path"
    $winget = Get-Command winget.exe -ErrorAction Stop
}
$result = Invoke-PackagingProcess $winget.Source @('--version')
if ($result.ExitCode -ne 0 -or $result.Stdout.Trim() -ne "v$version") {
    throw "Expected working WinGet $version for reproducible manifest validation"
}
if ($env:GITHUB_PATH) {
    # Each Actions step starts a fresh shell; persist discovery to later steps.
    [IO.File]::AppendAllText($env:GITHUB_PATH, (Split-Path $winget.Source -Parent) + "`n", [Text.UTF8Encoding]::new($false))
}
$winget.Source
