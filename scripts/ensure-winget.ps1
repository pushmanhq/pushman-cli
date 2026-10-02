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
    Repair-WinGetPackageManager -Version $version -Force | Out-Null
    $windowsApps = Join-Path $env:LOCALAPPDATA 'Microsoft\WindowsApps'
    $env:Path = "$windowsApps;$env:Path"
    $winget = Get-Command winget.exe -ErrorAction Stop
}
$result = Invoke-PackagingProcess $winget.Source @('--version')
if ($result.ExitCode -ne 0 -or $result.Stdout.Trim() -ne "v$version") {
    throw "Expected working WinGet $version for reproducible manifest validation"
}
$winget.Source
