param(
    [switch]$Direct,
    [switch]$NoInstaller
)

$ErrorActionPreference = 'Stop'

$brunoGoCommand = Get-Command go -ErrorAction SilentlyContinue
$brunoGoCandidates = @(
    'C:\Program Files\Go\bin\go.exe',
    (Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) 'Programs\BrunoGoMSI\bin\go.exe')
)

$brunoGoPath = if ($brunoGoCommand) {
    $brunoGoCommand.Source
} else {
    $brunoGoCandidates | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
}

if (-not $brunoGoPath) {
    throw 'Go 1.26+ não foi encontrado. Instale o Go oficial em https://go.dev/dl/ e execute novamente.'
}

$brunoGoBin = Split-Path -Parent $brunoGoPath
$env:Path = $brunoGoBin + [IO.Path]::PathSeparator + $env:Path
$brunoWorkspace = Split-Path -Parent $PSScriptRoot
$brunoNsisBin = Join-Path $brunoWorkspace '.tools\nsis\nsis-3.12\Bin'
if (Test-Path -LiteralPath (Join-Path $brunoNsisBin 'makensis.exe')) {
    $env:Path = $brunoNsisBin + [IO.Path]::PathSeparator + $env:Path
}
$env:GOCACHE = Join-Path $brunoWorkspace '.cache\go-build'
$env:GOMODCACHE = Join-Path $brunoWorkspace '.cache\go-mod'
$env:GOTMPDIR = Join-Path $brunoWorkspace '.cache\go-tmp'
$env:NPM_CONFIG_CACHE = Join-Path $brunoWorkspace '.cache\npm'
$env:BRUNO_BROWSER_DATA_DIR = Join-Path $brunoWorkspace '.cache\build-data'
New-Item -ItemType Directory -Force -Path $env:GOTMPDIR | Out-Null

Push-Location (Join-Path $brunoWorkspace 'frontend')
try {
    if (-not (Test-Path -LiteralPath 'node_modules\vite\package.json')) {
        node (Join-Path $brunoWorkspace 'scripts\npm-runner.mjs') ci --include=dev
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    }

    node (Join-Path $brunoWorkspace 'scripts\npm-runner.mjs') run build
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally {
    Pop-Location
}

if ($Direct) {
    $brunoOutputDirectory = Join-Path $brunoWorkspace 'build\bin'
    New-Item -ItemType Directory -Force -Path $brunoOutputDirectory | Out-Null
    & $brunoGoPath build -trimpath -tags 'desktop,production' -ldflags '-w -s -H windowsgui' -o (Join-Path $brunoOutputDirectory 'Bruno Browser.exe') .
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    Write-Host "Aplicativo gerado em $brunoOutputDirectory\Bruno Browser.exe"
    exit 0
}

$brunoWailsPath = Join-Path $brunoWorkspace '.tools\wails.exe'
$brunoWailsArguments = @('build', '-clean', '-s')
if (-not $NoInstaller) {
    $brunoWailsArguments += @('-platform', 'windows/amd64,windows/arm64', '-nsis', '-webview2', 'embed')
}

if (Test-Path -LiteralPath $brunoWailsPath) {
    & $brunoWailsPath @brunoWailsArguments
} else {
    & $brunoGoPath run github.com/wailsapp/wails/v2/cmd/wails@v2.12.0 @brunoWailsArguments
}
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$brunoAmd64Executable = Join-Path $brunoWorkspace 'build\bin\Bruno Browser-amd64.exe'
$brunoPortableExecutable = Join-Path $brunoWorkspace 'build\bin\Bruno Browser.exe'
if (Test-Path -LiteralPath $brunoAmd64Executable) {
    Copy-Item -LiteralPath $brunoAmd64Executable -Destination $brunoPortableExecutable -Force
}
