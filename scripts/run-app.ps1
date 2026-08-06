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
$env:GOCACHE = Join-Path $brunoWorkspace '.cache\go-build'
$env:GOMODCACHE = Join-Path $brunoWorkspace '.cache\go-mod'
$env:GOTMPDIR = Join-Path $brunoWorkspace '.cache\go-tmp'
New-Item -ItemType Directory -Force -Path $env:GOTMPDIR | Out-Null
& $brunoGoPath run github.com/wailsapp/wails/v2/cmd/wails@v2.12.0 dev
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
