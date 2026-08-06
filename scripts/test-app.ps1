param(
    [switch]$CompileOnly
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

$brunoWorkspace = Split-Path -Parent $PSScriptRoot
$env:Path = (Split-Path -Parent $brunoGoPath) + [IO.Path]::PathSeparator + $env:Path
$env:GOCACHE = Join-Path $brunoWorkspace '.cache\go-build'
$env:GOMODCACHE = Join-Path $brunoWorkspace '.cache\go-mod'
$env:GOTMPDIR = Join-Path $brunoWorkspace '.cache\go-tmp'
$env:NPM_CONFIG_CACHE = Join-Path $brunoWorkspace '.cache\npm'
New-Item -ItemType Directory -Force -Path $env:GOTMPDIR | Out-Null

Push-Location (Join-Path $brunoWorkspace 'frontend')
try {
    node (Join-Path $brunoWorkspace 'scripts\npm-runner.mjs') run build
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally {
    Pop-Location
}

Push-Location $brunoWorkspace
try {
    & $brunoGoPath vet ./...
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    if (-not $CompileOnly) {
        & $brunoGoPath test ./...
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        Write-Host 'Frontend, análise estática e testes Go concluídos com sucesso.'
        exit 0
    }

    $brunoTestOutput = Join-Path $brunoWorkspace '.cache\test-bin'
    New-Item -ItemType Directory -Force -Path $brunoTestOutput | Out-Null
    $brunoPackages = & $brunoGoPath list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./internal/... |
        Where-Object { $_ }
    $brunoCompiledCount = 0
    foreach ($brunoPackage in $brunoPackages) {
        $brunoBinaryName = ($brunoPackage -replace '[^A-Za-z0-9._-]', '_') + '.test.exe'
        & $brunoGoPath test -c -vet=off -o (Join-Path $brunoTestOutput $brunoBinaryName) $brunoPackage
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        $brunoCompiledCount++
    }
    Write-Host "$brunoCompiledCount pacotes de teste compilados. A execução foi omitida por solicitação."
} finally {
    Pop-Location
}
