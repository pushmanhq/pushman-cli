param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$')]
    [string]$Tag,
    [Parameter(Mandatory = $true)]
    [ValidateSet('x86_64', 'arm64')]
    [string]$Architecture,
    [string]$CandidateExecutable
)

$ErrorActionPreference = 'Stop'
$version = $Tag.Substring(1)
$asset = "pushman_${version}_windows_${Architecture}.zip"
$directory = Join-Path ([System.IO.Path]::GetTempPath()) ('pushman-release-test-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $directory | Out-Null

& gh release download $Tag -R pushmanhq/pushman-cli --pattern $asset --pattern checksums.txt --dir $directory
if ($LASTEXITCODE -ne 0) { throw 'Release download failed' }
$archive = Join-Path $directory $asset
$pattern = '^([0-9a-fA-F]{64})\s+\*?' + [regex]::Escape($asset) + '$'
$entries = @(Get-Content -LiteralPath (Join-Path $directory 'checksums.txt') | Where-Object { $_ -match $pattern })
if ($entries.Count -ne 1) { throw 'Expected exactly one checksum for the selected archive' }
$expected = ($entries[0] -split '\s+')[0]
$actual = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash
if ($expected -ne $actual) { throw 'Archive checksum mismatch' }
& gh attestation verify $archive -R pushmanhq/pushman-cli
if ($LASTEXITCODE -ne 0) { throw 'Archive provenance verification failed' }

$extracted = Join-Path $directory 'extracted'
Expand-Archive -LiteralPath $archive -DestinationPath $extracted
$executable = Join-Path $extracted 'pushman.exe'
if (-not (Test-Path -LiteralPath $executable -PathType Leaf)) { throw 'Archive has no pushman.exe' }
$reader = [System.IO.BinaryReader]::new([System.IO.File]::OpenRead($executable))
try {
    if ($reader.ReadUInt16() -ne 0x5A4D) { throw 'Executable has no DOS header' }
    $reader.BaseStream.Position = 0x3C
    $peOffset = $reader.ReadInt32()
    if ($peOffset -lt 64 -or $peOffset -gt $reader.BaseStream.Length - 6) { throw 'Invalid PE header offset' }
    $reader.BaseStream.Position = $peOffset
    if ($reader.ReadUInt32() -ne 0x00004550) { throw 'Executable has no PE signature' }
    $machine = $reader.ReadUInt16()
    $expectedMachine = if ($Architecture -eq 'arm64') { 0xAA64 } else { 0x8664 }
    if ($machine -ne $expectedMachine) { throw 'Executable architecture does not match the selected archive' }
} finally {
    $reader.Dispose()
}
$output = & $executable version
if ($LASTEXITCODE -ne 0 -or ($output -join ' ') -notmatch ('^pushman ' + [regex]::Escape($version) + ' (\(|$)')) {
    throw 'Extracted release version does not match the selected tag'
}
& $executable help | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Extracted release help failed' }

if ($CandidateExecutable) {
    $candidate = (Resolve-Path -LiteralPath $CandidateExecutable).Path
    $candidateVersion = & $candidate version
    if ($LASTEXITCODE -ne 0 -or ($candidateVersion -join ' ') -notmatch '^pushman ') {
        throw 'Source candidate version failed'
    }
    $candidateHash = (Get-FileHash -LiteralPath $candidate -Algorithm SHA256).Hash
    $releaseHash = (Get-FileHash -LiteralPath $executable -Algorithm SHA256).Hash
    $installDir = Join-Path $directory 'User install path'
    $installed = Join-Path $installDir 'pushman.exe'
    $backup = Join-Path $directory 'previous-verified.exe'
    New-Item -ItemType Directory -Path $installDir | Out-Null
    $previousPath = $env:Path
    try {
        Copy-Item -LiteralPath $executable -Destination $installed
        $env:Path = "$installDir;$previousPath"
        if ((Get-Command pushman.exe).Source -ne $installed) { throw 'Installed executable was shadowed' }
        $installedVersion = & $installed version
        if ($LASTEXITCODE -ne 0 -or ($installedVersion -join ' ') -ne ($output -join ' ')) {
            throw 'Installed release version failed'
        }
        Copy-Item -LiteralPath $installed -Destination $backup
        Copy-Item -LiteralPath $candidate -Destination $installed -Force
        if ((Get-FileHash -LiteralPath $installed -Algorithm SHA256).Hash -ne $candidateHash) {
            throw 'Source candidate replacement changed bytes'
        }
        $updatedVersion = & $installed version
        if ($LASTEXITCODE -ne 0 -or ($updatedVersion -join ' ') -ne ($candidateVersion -join ' ')) {
            throw 'Replacement version failed'
        }
        & $installed help | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Replacement help failed' }
        Copy-Item -LiteralPath $backup -Destination $installed -Force
        if ((Get-FileHash -LiteralPath $installed -Algorithm SHA256).Hash -ne $releaseHash) {
            throw 'Rollback changed the verified release bytes'
        }
        $restoredVersion = & $installed version
        if ($LASTEXITCODE -ne 0 -or ($restoredVersion -join ' ') -ne ($output -join ' ')) {
            throw 'Rollback version failed'
        }
        Remove-Item -LiteralPath $installed
        if (Test-Path -LiteralPath $installed) { throw 'Binary removal failed' }
    } finally {
        $env:Path = $previousPath
    }
    Write-Output 'PASS: temporary user-directory install/discovery/source replacement/verified rollback/binary removal; session PATH restored'
}

# Keep the verified bytes in the unique scratch directory for investigation.
# Installation is confined to the owned scratch directory and session PATH.
# This probe does not authorize, revoke, or send a notification.
[pscustomobject]@{
    status = 'PASS'
    tag = $Tag
    asset = $asset
    sha256 = $actual.ToLowerInvariant()
    architecture = $Architecture
    peMachine = ('0x{0:X4}' -f $machine)
    windows = [System.Environment]::OSVersion.Version.ToString()
    shell = $PSVersionTable.PSVersion.ToString()
}
