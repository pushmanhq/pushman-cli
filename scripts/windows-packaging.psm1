Set-StrictMode -Version Latest

function Invoke-PackagingProcess {
    param([string]$File, [string[]]$Arguments, [int]$TimeoutSeconds = 60)
    $start = [Diagnostics.ProcessStartInfo]::new()
    $start.FileName = $File
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    foreach ($argument in $Arguments) { $start.ArgumentList.Add($argument) }
    $process = [Diagnostics.Process]::new()
    $process.StartInfo = $start
    try {
        if (-not $process.Start()) { throw 'Could not start packaging process' }
        $stdout = $process.StandardOutput.ReadToEndAsync()
        $stderr = $process.StandardError.ReadToEndAsync()
        if (-not $process.WaitForExit($TimeoutSeconds * 1000)) {
            $process.Kill($true)
            $process.WaitForExit()
            throw 'Packaging process timed out'
        }
        [pscustomobject]@{
            ExitCode = $process.ExitCode
            Stdout = $stdout.GetAwaiter().GetResult()
            Stderr = $stderr.GetAwaiter().GetResult()
        }
    } finally { $process.Dispose() }
}

function Assert-PackagingExecutable {
    param([string]$Path, [string]$Architecture, [string]$Version)
    $reader = [IO.BinaryReader]::new([IO.File]::OpenRead($Path))
    try {
        if ($reader.ReadUInt16() -ne 0x5A4D) { throw 'Missing executable DOS header' }
        $reader.BaseStream.Position = 0x3C
        $offset = $reader.ReadInt32()
        if ($offset -lt 64 -or $offset -gt $reader.BaseStream.Length - 6) { throw 'Invalid PE header offset' }
        $reader.BaseStream.Position = $offset
        if ($reader.ReadUInt32() -ne 0x4550) { throw 'Missing PE signature' }
        $expected = if ($Architecture -eq 'arm64') { 0xAA64 } else { 0x8664 }
        if ($reader.ReadUInt16() -ne $expected) { throw 'Executable architecture mismatch' }
    } finally { $reader.Dispose() }
    $result = Invoke-PackagingProcess $Path @('version')
    if ($result.ExitCode -ne 0 -or $result.Stderr -ne '' -or
        $result.Stdout -notmatch ('^pushman ' + [regex]::Escape($Version) + ' (\(|\r?\n|$)')) {
        throw 'Executable version does not match the installer version'
    }
}

function ConvertTo-ManifestYaml {
    param($Value, [int]$Indent = 0)
    $prefix = ' ' * $Indent
    if ($Value -is [Collections.IDictionary]) {
        foreach ($key in $Value.Keys) {
            $item = $Value[$key]
            if ($item -is [Collections.IDictionary] -or ($item -is [array])) {
                "$prefix${key}:"
                ConvertTo-ManifestYaml $item ($Indent + 2)
            } else {
                "$prefix${key}: " + (ConvertTo-Json -InputObject $item -Compress)
            }
        }
    } elseif ($Value -is [array]) {
        foreach ($item in $Value) {
            if ($item -is [Collections.IDictionary]) {
                "$prefix-"
                ConvertTo-ManifestYaml $item ($Indent + 2)
            } else { "$prefix- " + (ConvertTo-Json -InputObject $item -Compress) }
        }
    } else { throw 'Unsupported manifest node' }
}

Export-ModuleMember -Function Invoke-PackagingProcess, Assert-PackagingExecutable, ConvertTo-ManifestYaml
