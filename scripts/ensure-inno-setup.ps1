param([string]$Directory = (Join-Path ([IO.Path]::GetTempPath()) 'pushman-inno-6.7.3'))
$ErrorActionPreference = 'Stop'
$compiler = Join-Path $Directory 'compiler\ISCC.exe'
if (-not (Test-Path -LiteralPath $compiler -PathType Leaf)) {
    New-Item -ItemType Directory -Force -Path $Directory | Out-Null
    $download = Join-Path $Directory 'innosetup-6.7.3.exe'
    Invoke-WebRequest -Uri 'https://github.com/jrsoftware/issrc/releases/download/is-6_7_3/innosetup-6.7.3.exe' -OutFile $download
    $expected = '9c73c3bae7ed48d44112a0f48e66742c00090bdb5bef71d9d3c056c66e97b732'
    if ((Get-FileHash -LiteralPath $download -Algorithm SHA256).Hash -ne $expected) {
        throw 'Inno Setup download checksum mismatch'
    }
    $arguments = @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/SP-', '/NORESTART', '/CURRENTUSER', '/NOICONS', ('/DIR="' + (Join-Path $Directory 'compiler') + '"'))
    $process = Start-Process -FilePath $download -ArgumentList $arguments -Wait -PassThru -WindowStyle Hidden
    try { if ($process.ExitCode -ne 0) { throw 'Inno Setup installation failed' } }
    finally { $process.Dispose() }
}
# The installer source checks the compiler's built-in Ver value. ISCC and
# ISCmplr.dll do not expose a usable FileVersion resource.
$compiler
