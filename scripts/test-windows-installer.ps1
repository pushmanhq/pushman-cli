# Run on a disposable native Windows account. Never replace an existing install.
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$Installer,
    [Parameter(Mandatory)][string]$Executable,
    [Parameter(Mandatory)][string]$Version,
    [Parameter(Mandatory)][ValidateSet('x86_64', 'arm64')][string]$Architecture,
    [Parameter(Mandatory)][string]$Compiler,
    [string]$AppIdentity = '222c0b32-df1d-468b-a624-913a96355115',
    [string]$OwnerKey = 'Software\Pushman\Installer'
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
Import-Module (Join-Path $PSScriptRoot 'windows-packaging.psm1') -Force
$Installer = (Resolve-Path -LiteralPath $Installer).Path
$Executable = (Resolve-Path -LiteralPath $Executable).Path
Assert-PackagingExecutable $Executable $Architecture $Version
$native = [Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
if (($Architecture -eq 'arm64' -and $native -ne 'Arm64') -or
    ($Architecture -eq 'x86_64' -and $native -ne 'X64')) { throw 'Native architecture required' }
$appKey = "Software\Microsoft\Windows\CurrentVersion\Uninstall\${AppIdentity}_is1"
$userRegistry = [Microsoft.Win32.RegistryKey]::OpenBaseKey('CurrentUser', 'Registry32')
foreach ($view in @('Registry32', 'Registry64')) {
    foreach ($hive in @('CurrentUser', 'LocalMachine')) {
        $key = [Microsoft.Win32.RegistryKey]::OpenBaseKey($hive, $view)
        try {
            $existing = $key.OpenSubKey($appKey)
            if ($existing) { $existing.Dispose(); throw 'Refusing to touch an existing Pushman installation' }
        } finally { $key.Dispose() }
    }
}
$existingOwner = $userRegistry.OpenSubKey($OwnerKey)
if ($existingOwner) { $existingOwner.Dispose(); throw 'Refusing to touch existing PATH ownership' }

function Read-UserPath {
    $key = $userRegistry.OpenSubKey('Environment')
    try {
        $exists = $key -and ($key.GetValueNames() -contains 'Path')
        if ($exists) {
            return @{Exists=$true;Value=$key.GetValue('Path', '', 'DoNotExpandEnvironmentNames');Kind=$key.GetValueKind('Path')}
        }
        return @{Exists=$false;Value='';Kind=[Microsoft.Win32.RegistryValueKind]::ExpandString}
    } finally { if ($key) { $key.Dispose() } }
}
function Write-UserPath($State) {
    $key = $userRegistry.CreateSubKey('Environment')
    try {
        if ($State.Exists) { $key.SetValue('Path', $State.Value, $State.Kind) }
        else { $key.DeleteValue('Path', $false) }
    } finally { $key.Dispose() }
}
function Assert-Path($Expected) {
    $actual = Read-UserPath
    if ($actual.Exists -ne $Expected.Exists -or $actual.Value -cne $Expected.Value -or
        ($Expected.Exists -and $actual.Kind -ne $Expected.Kind)) {
        throw "User PATH was not preserved exactly (setup $sequence; exists=$($actual.Exists)/$($Expected.Exists), kind=$($actual.Kind)/$($Expected.Kind), length=$($actual.Value.Length)/$($Expected.Value.Length))"
    }
}
function Assert-NoOwner {
    $key = $userRegistry.OpenSubKey($OwnerKey)
    if ($key) { $key.Dispose(); throw 'Unexpected PATH ownership remains' }
}
function Run-Setup([string]$File, [string]$Tasks = 'addtopath') {
    $script:sequence++
    $log = Join-Path $root ("setup-$sequence.log")
    $arguments = @('/SP-', '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/NOICONS', "/DIR=$installDir", "/LOG=$log")
    if ($Tasks -ne 'default') { $arguments += "/TASKS=$Tasks" }
    $result = Invoke-PackagingProcess $File $arguments 180
    if ($result.ExitCode -ne 0) { throw "Setup failed ($($result.ExitCode)); log: $log" }
}
function Assert-Installed([string]$ExpectedExecutable, [string]$ExpectedVersion) {
    $installed = Join-Path $installDir 'pushman.exe'
    if ((Get-FileHash -LiteralPath $installed).Hash -ne (Get-FileHash -LiteralPath $ExpectedExecutable).Hash) {
        throw 'Installed executable differs from qualified payload'
    }
    Assert-PackagingExecutable $installed $Architecture $ExpectedVersion
    $help = Invoke-PackagingProcess $installed @('help')
    if ($help.ExitCode -ne 0 -or $help.Stderr -ne '' -or $help.Stdout -notmatch 'Usage:') { throw 'Installed help failed' }
    $key = $userRegistry.OpenSubKey($appKey)
    if (-not $key) { throw 'Missing per-user Installed Apps registration' }
    try {
        if ($key.GetValue('DisplayName') -ne 'Pushman CLI' -or $key.GetValue('Publisher') -ne 'Pushman' -or
            $key.GetValue('DisplayVersion') -ne $ExpectedVersion -or
            $key.GetValue('InstallLocation').TrimEnd('\') -ne $installDir) { throw 'Incorrect Installed Apps metadata' }
    } finally { $key.Dispose() }
}
function Run-Uninstall {
    $app = $userRegistry.OpenSubKey($appKey)
    if (-not $app) { throw 'Missing uninstall registration' }
    try { $uninstaller = $app.GetValue('UninstallString').Trim('"') }
    finally { $app.Dispose() }
    if ([IO.Path]::GetDirectoryName($uninstaller) -ne $installDir -or
        [IO.Path]::GetFileName($uninstaller) -notmatch '^unins[0-9]+\.exe$') { throw 'Unexpected uninstall command' }
    if (-not (Test-Path -LiteralPath $uninstaller)) { throw 'Missing uninstaller' }
    $result = Invoke-PackagingProcess $uninstaller @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART') 180
    if ($result.ExitCode -ne 0) { throw 'Uninstall failed' }
    $deadline = [DateTime]::UtcNow.AddSeconds(15)
    # Inno's bootstrap process can exit before its temporary uninstaller.
    do {
        $app = $userRegistry.OpenSubKey($appKey)
        $owner = $userRegistry.OpenSubKey($OwnerKey)
        $pending = (Test-Path -LiteralPath (Join-Path $installDir 'pushman.exe')) -or
            (Test-Path -LiteralPath $uninstaller) -or (Test-Path -LiteralPath ([IO.Path]::ChangeExtension($uninstaller, '.dat'))) -or $app -or $owner
        if ($app) { $app.Dispose() }
        if ($owner) { $owner.Dispose() }
        if (-not $pending) { break }
        if ([DateTime]::UtcNow -gt $deadline) { throw 'Uninstall did not finish' }
        Start-Sleep -Milliseconds 100
    } while ($true)
    $key = $userRegistry.OpenSubKey($appKey)
    if ($key) { $key.Dispose(); throw 'Installed Apps entry survived uninstall' }
    Assert-NoOwner
}

$originalPath = Read-UserPath
$processPath = $env:Path
$installerHash = (Get-FileHash -LiteralPath $Installer).Hash
$root = Join-Path ([IO.Path]::GetTempPath()) ('pushman-installer-test-' + [guid]::NewGuid())
$installDir = Join-Path $root '설치 폴더 with spaces'
$sequence = 0
$succeeded = $false
try {
    New-Item -ItemType Directory -Path $root | Out-Null
    # An older build of this source exercises real payload/metadata replacement.
    # It is a fixture, not evidence that a historical public release is verified.
    $oldVersion = '0.0.0-installer-test'
    $oldExe = Join-Path $root 'old.exe'
    $build = Invoke-PackagingProcess 'go' @('build', '-trimpath', '-ldflags',
        "-s -w -X main.version=$oldVersion -X main.credentialNamespace=installer-test", '-o', $oldExe, './cmd/pushman') 180
    if ($build.ExitCode -ne 0) { throw ("Old fixture build failed: " + $build.Stderr) }
    $old = & (Join-Path $PSScriptRoot 'build-windows-installer.ps1') -Version $oldVersion -Architecture $Architecture `
        -Executable $oldExe -OutputDirectory $root -Compiler $Compiler -AppIdentity $AppIdentity -OwnerKey $OwnerKey

    Run-Setup $old.Installer
    Assert-Installed $oldExe $oldVersion
    Run-Setup $Installer
    Assert-Installed $Executable $Version
    $afterInstall = Read-UserPath
    $expected = if ($originalPath.Value -eq '') { $installDir } else { "$installDir;$($originalPath.Value)" }
    Assert-Path @{Exists=$true;Value=$expected;Kind=$originalPath.Kind}
    # Resolve through the registry PATH in a fresh child process, without setx.
    $env:Path = [Environment]::ExpandEnvironmentVariables($expected) + ';' + $processPath
    if ((Get-Command pushman.exe).Source -ne (Join-Path $installDir 'pushman.exe')) { throw 'PATH discovery failed' }
    $discovered = Invoke-PackagingProcess 'pushman.exe' @('version')
    if ($discovered.ExitCode -ne 0) { throw 'PATH invocation failed' }
    $env:Path = $processPath
    Run-Setup $Installer
    Assert-Path $afterInstall
    Run-Setup $old.Installer
    Assert-Installed $oldExe $oldVersion
    Run-Setup $Installer
    Assert-Installed $Executable $Version
    Run-Setup $Installer ''
    Assert-Installed $Executable $Version
    Assert-Path $originalPath
    Assert-NoOwner
    Run-Setup $Installer 'default'
    Assert-Path $originalPath
    Assert-NoOwner
    Run-Setup $Installer
    $sentinel = Join-Path $installDir 'user-file.txt'
    [IO.File]::WriteAllText($sentinel, 'must survive uninstall')
    Run-Uninstall
    Assert-Path $originalPath
    if (-not (Test-Path -LiteralPath $sentinel)) { throw 'Uninstaller removed an unrelated file' }

    # Check raw value kind, empty/missing state, quotes, variable expansion and
    # trailing empty tokens. Only test-owned directories are used in PATH.
    $cases = @(
        @{Exists=$false;Value='';Kind=[Microsoft.Win32.RegistryValueKind]::ExpandString},
        @{Exists=$true;Value='';Kind=[Microsoft.Win32.RegistryValueKind]::String},
        @{Exists=$true;Value=('"' + $root + '\other\";;');Kind=[Microsoft.Win32.RegistryValueKind]::String},
        @{Exists=$true;Value='%LOCALAPPDATA%\unrelated;;';Kind=[Microsoft.Win32.RegistryValueKind]::ExpandString}
    )
    foreach ($state in $cases) {
        Write-UserPath $state
        Run-Setup $Installer
        Run-Setup $Installer
        Run-Uninstall
        Assert-Path $state
    }
    foreach ($entry in @(($installDir.ToUpperInvariant() + '\'), ('"' + $installDir + '"'),
        $installDir.Replace($env:TEMP, '%TEMP%'))) {
        $state = @{Exists=$true;Value="$root\other;$entry;;";Kind=[Microsoft.Win32.RegistryValueKind]::ExpandString}
        Write-UserPath $state
        Run-Setup $Installer
        Assert-Path $state
        Assert-NoOwner
        Run-Uninstall
        Assert-Path $state
    }
    Write-UserPath $originalPath
    if ((Get-FileHash -LiteralPath $Installer).Hash -ne $installerHash) { throw 'Candidate installer changed during qualification' }
    $succeeded = $true
    [pscustomobject]@{Version=$Version;Architecture=$Architecture;InstallerSHA256=$installerHash.ToLowerInvariant();
        Result='PASS';Scenarios='install/reinstall/update/rollback/opt-out/preexisting-PATH/uninstall/exact-PATH-restoration'}
} finally {
    $env:Path = $processPath
    try {
        $remaining = $userRegistry.OpenSubKey($appKey)
        if ($remaining) { $remaining.Dispose(); Run-Uninstall }
    } finally {
        Write-UserPath $originalPath
        $userRegistry.Dispose()
    }
    if ($succeeded) {
        $resolvedRoot = [IO.Path]::GetFullPath($root)
        $tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\'
        if (-not $resolvedRoot.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase) -or
            [IO.Path]::GetFileName($resolvedRoot) -notlike 'pushman-installer-test-*') { throw 'Unsafe test cleanup path' }
        Remove-Item -LiteralPath $resolvedRoot -Recurse -Force
    } else { Write-Warning "Installer test logs retained: $root" }
}
