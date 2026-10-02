# PowerShell 7+. Generate from final qualified setup files, never rebuilt copies.
[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidatePattern('^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$')][string]$Tag,
    [Parameter(Mandatory)][string]$InstallerDirectory,
    [string]$OutputDirectory = 'dist\winget',
    [switch]$RequireWinget
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
Import-Module (Join-Path $PSScriptRoot 'windows-packaging.psm1') -Force
$version = $Tag.Substring(1)
$identifier = 'PushmanHQ.Pushman'
$schemaVersion = '1.12.0'
# Pin Microsoft's schema source so a mutable branch cannot change validation.
$schemaBase = 'https://raw.githubusercontent.com/microsoft/winget-cli/b0f6209cc58841c19e432325b127ab3c33a8f6f8/schemas/JSON/manifests/v1.12.0'
$releaseBase = "https://github.com/pushmanhq/pushman-cli/releases/download/$Tag"
$installers = @(
    foreach ($arch in @('x86_64', 'arm64')) {
        $file = "pushman_${version}_windows_${arch}_setup.exe"
        $path = (Resolve-Path -LiteralPath (Join-Path $InstallerDirectory $file)).Path
        [ordered]@{
            Architecture = $(if ($arch -eq 'x86_64') { 'x64' } else { 'arm64' })
            InstallerUrl = "$releaseBase/$file"
            InstallerSha256 = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
        }
    }
)
$manifests = @(
    [ordered]@{
        PackageIdentifier=$identifier;PackageVersion=$version;DefaultLocale='en-US'
        ManifestType='version';ManifestVersion=$schemaVersion
    },
    [ordered]@{
        PackageIdentifier=$identifier;PackageVersion=$version;PackageLocale='en-US'
        Publisher='Pushman';PublisherUrl='https://github.com/pushmanhq'
        PublisherSupportUrl='https://github.com/pushmanhq/pushman-cli/issues'
        PackageName='Pushman CLI';PackageUrl='https://github.com/pushmanhq/pushman-cli'
        License='MIT';LicenseUrl='https://github.com/pushmanhq/pushman-cli/blob/main/LICENSE'
        ShortDescription='Send notifications to your iPhone from the terminal and MCP clients.'
        Moniker='pushman';Tags=@('cli','mcp','notifications','iphone')
        ReleaseNotesUrl="https://github.com/pushmanhq/pushman-cli/releases/tag/$Tag"
        Documentations=@([ordered]@{DocumentLabel='Installation guide';DocumentUrl='https://github.com/pushmanhq/pushman-cli/blob/main/docs/INSTALL.md'})
        ManifestType='defaultLocale';ManifestVersion=$schemaVersion
    },
    [ordered]@{
        PackageIdentifier=$identifier;PackageVersion=$version
        InstallerType='inno';Scope='user';UpgradeBehavior='install'
        InstallModes=@('interactive','silent','silentWithProgress');Commands=@('pushman')
        InstallerSwitches=[ordered]@{
            Silent='/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /SP-'
            SilentWithProgress='/SILENT /SUPPRESSMSGBOXES /NORESTART /SP-'
            InstallLocation='/DIR="<INSTALLPATH>"';Log='/LOG="<LOGPATH>"'
        }
        AppsAndFeaturesEntries=@([ordered]@{
            DisplayName='Pushman CLI';Publisher='Pushman'
            ProductCode='222c0b32-df1d-468b-a624-913a96355115_is1'
        })
        ElevationRequirement='elevationProhibited'
        InstallationMetadata=[ordered]@{DefaultInstallLocation='%LOCALAPPDATA%\Programs\Pushman'}
        Installers=$installers;ManifestType='installer';ManifestVersion=$schemaVersion
    }
)
$destination = Join-Path $OutputDirectory "manifests\p\PushmanHQ\Pushman\$version"
New-Item -ItemType Directory -Force -Path $destination | Out-Null
foreach ($manifest in $manifests) {
    $type = $manifest.ManifestType
    $schemaUri = "$schemaBase/manifest.$type.$schemaVersion.json"
    $schema = (Invoke-WebRequest -Uri $schemaUri).Content
    if (-not (Test-Json -Json (ConvertTo-Json -InputObject $manifest -Depth 15) -Schema $schema)) {
        throw "Invalid $type manifest"
    }
    $suffix = switch ($type) { 'version' { '' }; 'defaultLocale' { '.locale.en-US' }; 'installer' { '.installer' } }
    $yaml = @("# yaml-language-server: `$schema=https://aka.ms/winget-manifest.$type.$schemaVersion.schema.json")
    $yaml += ConvertTo-ManifestYaml $manifest
    [IO.File]::WriteAllText((Join-Path $destination "$identifier$suffix.yaml"), ($yaml -join "`n") + "`n", [Text.UTF8Encoding]::new($false))
}
$winget = Get-Command winget.exe -ErrorAction SilentlyContinue
if ($winget) {
    $result = Invoke-PackagingProcess $winget.Source @('validate', '--manifest', $destination, '--disable-interactivity')
    if ($result.ExitCode -ne 0) { throw ("winget validate failed:`n" + $result.Stdout + $result.Stderr) }
} elseif ($RequireWinget) { throw 'winget is required for catalog submission validation' }
[pscustomobject]@{Directory=(Resolve-Path -LiteralPath $destination).Path;PackageIdentifier=$identifier;
    PackageVersion=$version;SchemaValidation='PASS';WingetValidation=$(if ($winget) { 'PASS' } else { 'NOT RUN (WinGet unavailable)' })}
