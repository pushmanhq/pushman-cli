param(
    [Parameter(Mandatory)][ValidatePattern('^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$')][string]$Version,
    [Parameter(Mandatory)][ValidateSet('x86_64', 'arm64')][string]$Architecture,
    [Parameter(Mandatory)][string]$Executable,
    [string]$OutputDirectory = 'dist\windows',
    [string]$Compiler,
    [ValidatePattern('^[0-9a-fA-F-]{36}$')][string]$AppIdentity = '222c0b32-df1d-468b-a624-913a96355115',
    [ValidatePattern('^Software\\Pushman\\[A-Za-z0-9-]+$')][string]$OwnerKey = 'Software\Pushman\Installer'
)
$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSScriptRoot 'windows-packaging.psm1') -Force
$executablePath = (Resolve-Path -LiteralPath $Executable).Path
Assert-PackagingExecutable $executablePath $Architecture $Version
if (-not $Compiler) { $Compiler = & (Join-Path $PSScriptRoot 'ensure-inno-setup.ps1') }
$Compiler = (Resolve-Path -LiteralPath $Compiler).Path
New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
$outputPath = (Resolve-Path -LiteralPath $OutputDirectory).Path
$script = Join-Path $PSScriptRoot '..\packaging\windows\pushman.iss'
$arguments = @('/Qp', "/DVersion=$Version", "/DArchitecture=$Architecture", "/DExecutable=$executablePath",
    "/DOutputDirectory=$outputPath", "/DAppIdentity=$AppIdentity", "/DOwnerKey=$OwnerKey")
$arguments += $script
$result = Invoke-PackagingProcess $Compiler $arguments 180
if ($result.ExitCode -ne 0) { throw ("Installer compilation failed:`n" + $result.Stdout + $result.Stderr) }
$installer = Join-Path $outputPath "pushman_${Version}_windows_${Architecture}_setup.exe"
if (-not (Test-Path -LiteralPath $installer -PathType Leaf)) { throw 'Compiler produced no installer' }
[pscustomobject]@{Installer=$installer;Version=$Version;Architecture=$Architecture;Signed=$false;SHA256=(Get-FileHash -LiteralPath $installer -Algorithm SHA256).Hash.ToLowerInvariant()}
