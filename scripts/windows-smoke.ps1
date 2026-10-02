param(
    [Parameter(Mandatory = $true)]
    [string]$Executable
)

$ErrorActionPreference = 'Stop'
$Executable = (Resolve-Path -LiteralPath $Executable).Path

foreach ($command in @('version', 'help')) {
    & $Executable $command
    if ($LASTEXITCODE -ne 0) {
        throw "$command returned exit $LASTEXITCODE"
    }
}

# Expected failures must be checked explicitly in both PowerShell versions.
# Process pipes also let us check stdout/stderr without PS 5.1 converting
# native stderr into ErrorRecords or changing the captured bytes.
foreach ($case in @(
    @{ Argument = '--not-a-pushman-flag'; Exit = 2; Error = 'unknown flag' },
    @{ Argument = 'self-update'; Exit = 1; Error = 'self-update supports Homebrew installations' }
)) {
    $start = [System.Diagnostics.ProcessStartInfo]::new()
    $start.FileName = $Executable
    $start.Arguments = $case.Argument
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $start
    try {
        if (-not $process.Start()) { throw 'Could not start CLI smoke process' }
        $stdout = $process.StandardOutput.ReadToEndAsync()
        $stderr = $process.StandardError.ReadToEndAsync()
        if (-not $process.WaitForExit(10000)) {
            $process.Kill()
            throw 'CLI smoke timed out'
        }
        $output = $stdout.GetAwaiter().GetResult()
        $diagnostic = $stderr.GetAwaiter().GetResult()
        if ($process.ExitCode -ne $case.Exit -or $output -ne '' -or
            -not $diagnostic.Contains($case.Error)) {
            throw "$($case.Argument) violated its exit/stdout/stderr contract"
        }
    } finally {
        $process.Dispose()
    }
}

Write-Output "PASS: PowerShell $($PSVersionTable.PSVersion) native CLI smoke"
