[CmdletBinding()]
param(
    [switch]$CheckOnly,
    [switch]$Devtools
)

$ErrorActionPreference = 'Stop'
$repoRoot = $PSScriptRoot
$appDirectory = Join-Path $repoRoot 'cmd\jide'
$goCommand = Get-Command go.exe -ErrorAction Stop

# Wails v2.12's type/binding tooling is verified with the repository's Go 1.24
# baseline. GOTOOLCHAIN downloads that toolchain automatically if needed.
$env:GOTOOLCHAIN = 'go1.24.0'
$env:GOFLAGS = ''
$goBin = (& $goCommand.Source env GOBIN).Trim()
if (-not $goBin) {
    $goPath = (& $goCommand.Source env GOPATH).Trim()
    $goBin = Join-Path $goPath 'bin'
}
if (-not (Test-Path $goBin)) {
    New-Item -ItemType Directory -Force -Path $goBin | Out-Null
}
$env:Path = "$goBin;$env:Path"

$nodeCommand = Get-Command node.exe -ErrorAction SilentlyContinue
$npmCommand = Get-Command npm.cmd -ErrorAction SilentlyContinue
if (-not $nodeCommand -or -not $npmCommand) {
    throw 'Node.js and npm are required. Install Node.js 24, then rerun this script.'
}
$nodeVersion = (& $nodeCommand.Source --version).Trim()
$nodeMatch = [regex]::Match($nodeVersion, '^v(?<major>\d+)\.(?<minor>\d+)')
$nodeMajor = [int]$nodeMatch.Groups['major'].Value
$nodeMinor = [int]$nodeMatch.Groups['minor'].Value
$nodeSupported = $nodeMajor -ge 24 -or
    ($nodeMajor -eq 22 -and $nodeMinor -ge 12) -or
    ($nodeMajor -eq 20 -and $nodeMinor -ge 19)
if (-not $nodeSupported) {
    throw "Vite needs Node.js 20.19+, 22.12+, or 24+. Found $nodeVersion."
}

$gccCandidates = [System.Collections.Generic.List[string]]::new()
if ($env:CC) {
    $configuredCompiler = Get-Command $env:CC -ErrorAction SilentlyContinue
    if ($configuredCompiler) {
        $gccCandidates.Add($configuredCompiler.Source)
    } elseif (Test-Path $env:CC) {
        $gccCandidates.Add($env:CC)
    }
}
$gccOnPath = Get-Command gcc.exe -ErrorAction SilentlyContinue
if ($gccOnPath) {
    $gccCandidates.Add($gccOnPath.Source)
}
$gccCandidates.Add('C:\msys64\mingw64\bin\gcc.exe')
$gccCandidates.Add('C:\msys64\ucrt64\bin\gcc.exe')
$gccCandidates.Add('C:\mingw64\bin\gcc.exe')
if ($env:ProgramFiles) {
    $octaveRoot = Join-Path $env:ProgramFiles 'GNU Octave'
    if (Test-Path $octaveRoot) {
        Get-ChildItem -Path $octaveRoot -Directory -Filter 'Octave-*' -ErrorAction SilentlyContinue |
            ForEach-Object { $gccCandidates.Add((Join-Path $_.FullName 'mingw64\bin\gcc.exe')) }
    }
}
$gccPath = $gccCandidates | Where-Object { Test-Path $_ } | Select-Object -First 1
if (-not $gccPath) {
    throw 'A GCC C compiler is required for Tree-sitter. Install MSYS2 MinGW-w64 GCC or set up GCC on PATH.'
}
$env:CC = 'gcc'
$env:CGO_ENABLED = '1'
$env:Path = "$(Split-Path -Parent $gccPath);$env:Path"

$wailsPath = Join-Path $goBin 'wails.exe'
$installWails = -not (Test-Path $wailsPath)
if (-not $installWails) {
    $wailsVersion = (& $wailsPath version 2>&1 | Out-String)
    $installWails = $wailsVersion -notmatch 'v2\.12\.0'
}
if ($installWails) {
    Write-Host 'Installing the pinned Wails v2.12.0 CLI...'
    & $goCommand.Source install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
    if ($LASTEXITCODE -ne 0) {
        throw "Installing Wails failed with exit code $LASTEXITCODE."
    }
}

Write-Host "Go: $(& $goCommand.Source version)"
Write-Host "C compiler: $gccPath"
Write-Host "Wails: $wailsPath"
if ($CheckOnly) {
    Write-Host 'Toolchain check passed.'
    exit 0
}

$wailsArguments = @('dev', '-skipbindings', '-skipembedcreate')
if ($Devtools) {
    $wailsArguments += '-devtools'
}
Push-Location $appDirectory
try {
    # The frontend calls Wails' injected window.go bindings directly, and the
    # embedded dist directory is checked in. Skip Wails' package type-analysis
    # passes; they are unnecessary here and can fail with newer Go toolchains.
    & $wailsPath @wailsArguments
    if ($LASTEXITCODE -ne 0) {
        throw "Wails dev exited with code $LASTEXITCODE."
    }
}
finally {
    Pop-Location
}
