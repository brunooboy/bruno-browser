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

function Get-BrunoEnginePlatform {
    param(
        [Parameter(Mandatory = $true)][string]$Architecture,
        [Parameter(Mandatory = $true)]$Definition
    )

    $brunoEngineCache = Join-Path $brunoWorkspace '.cache\bruno-engine'
    $brunoArchivePath = Join-Path $brunoEngineCache "chromium-$Architecture-$($Definition.revision).zip"
    $brunoExtractPath = Join-Path $brunoEngineCache "extracted\$Architecture-$($Definition.revision)"
    $brunoMarkerPath = Join-Path $brunoExtractPath '.bruno-engine-sha256'
    New-Item -ItemType Directory -Force -Path $brunoEngineCache | Out-Null

    $brunoArchiveValid = $false
    if (Test-Path -LiteralPath $brunoArchivePath) {
        $brunoArchive = Get-Item -LiteralPath $brunoArchivePath
        if ($brunoArchive.Length -eq [int64]$Definition.bytes) {
            $brunoActualHash = (Get-FileHash -LiteralPath $brunoArchivePath -Algorithm SHA256).Hash
            $brunoArchiveValid = $brunoActualHash -eq ([string]$Definition.sha256).ToUpperInvariant()
        }
    }

    if (-not $brunoArchiveValid) {
        if (Test-Path -LiteralPath $brunoArchivePath) {
            Remove-Item -LiteralPath $brunoArchivePath -Force
        }
        Write-Host "Baixando Bruno Engine $Architecture (Chromium $($Definition.revision))..."
        curl.exe -L --fail --retry 3 --output $brunoArchivePath ([string]$Definition.url)
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        $brunoArchive = Get-Item -LiteralPath $brunoArchivePath
        $brunoActualHash = (Get-FileHash -LiteralPath $brunoArchivePath -Algorithm SHA256).Hash
        if ($brunoArchive.Length -ne [int64]$Definition.bytes -or $brunoActualHash -ne ([string]$Definition.sha256).ToUpperInvariant()) {
            Remove-Item -LiteralPath $brunoArchivePath -Force
            throw "Falha na verificação de integridade do Bruno Engine $Architecture."
        }
    }

    $brunoExtractValid = (Test-Path -LiteralPath (Join-Path $brunoExtractPath 'chrome-win\chrome.exe')) -and
        (Test-Path -LiteralPath (Join-Path $brunoExtractPath 'chrome-win\chrome.dll')) -and
        (Test-Path -LiteralPath (Join-Path $brunoExtractPath 'chrome-win\resources.pak')) -and
        (Test-Path -LiteralPath (Join-Path $brunoExtractPath 'chrome-win\locales\en-US.pak')) -and
        (Test-Path -LiteralPath $brunoMarkerPath) -and
        ((Get-Content -LiteralPath $brunoMarkerPath -Raw).Trim() -eq ([string]$Definition.sha256).ToUpperInvariant())

    if (-not $brunoExtractValid) {
        if (Test-Path -LiteralPath $brunoExtractPath) {
            $brunoResolvedCache = [IO.Path]::GetFullPath($brunoEngineCache).TrimEnd('\') + '\'
            $brunoResolvedExtract = [IO.Path]::GetFullPath($brunoExtractPath).TrimEnd('\') + '\'
            if (-not $brunoResolvedExtract.StartsWith($brunoResolvedCache, [StringComparison]::OrdinalIgnoreCase)) {
                throw 'O diretório de extração do motor saiu da área de cache permitida.'
            }
            Remove-Item -LiteralPath $brunoExtractPath -Recurse -Force
        }
        New-Item -ItemType Directory -Force -Path $brunoExtractPath | Out-Null
        Write-Host "Preparando Bruno Engine $Architecture..."
        Expand-Archive -LiteralPath $brunoArchivePath -DestinationPath $brunoExtractPath -Force
        Set-Content -LiteralPath $brunoMarkerPath -Value ([string]$Definition.sha256).ToUpperInvariant() -Encoding ascii
    }

    return $brunoExtractPath
}

function Get-BrunoBrandingTool {
    param([Parameter(Mandatory = $true)]$Definition)
    $brunoToolRoot = Join-Path $brunoWorkspace '.cache\tools'
    $brunoToolPath = Join-Path $brunoToolRoot 'rcedit-x64-v2.0.0.exe'
    New-Item -ItemType Directory -Force -Path $brunoToolRoot | Out-Null
    $brunoToolValid = $false
    if (Test-Path -LiteralPath $brunoToolPath) {
        $brunoTool = Get-Item -LiteralPath $brunoToolPath
        if ($brunoTool.Length -eq [int64]$Definition.toolBytes) {
            $brunoToolValid = (Get-FileHash -LiteralPath $brunoToolPath -Algorithm SHA256).Hash -eq ([string]$Definition.toolSha256).ToUpperInvariant()
        }
    }
    if (-not $brunoToolValid) {
        if (Test-Path -LiteralPath $brunoToolPath) { Remove-Item -LiteralPath $brunoToolPath -Force }
        Write-Host 'Baixando ferramenta de identidade visual do Bruno Engine...'
        curl.exe -L --fail --retry 3 --output $brunoToolPath ([string]$Definition.toolUrl)
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        $brunoTool = Get-Item -LiteralPath $brunoToolPath
        $brunoToolHash = (Get-FileHash -LiteralPath $brunoToolPath -Algorithm SHA256).Hash
        if ($brunoTool.Length -ne [int64]$Definition.toolBytes -or $brunoToolHash -ne ([string]$Definition.toolSha256).ToUpperInvariant()) {
            Remove-Item -LiteralPath $brunoToolPath -Force
            throw 'Falha na verificação de integridade da ferramenta de branding.'
        }
    }
    return $brunoToolPath
}

function Set-BrunoEngineBranding {
    param(
        [Parameter(Mandatory = $true)][string]$EngineRoot,
        [Parameter(Mandatory = $true)][string]$ToolPath,
        [Parameter(Mandatory = $true)]$Definition
    )
    $brunoChromePath = Join-Path $EngineRoot 'chrome-win\chrome.exe'
    $brunoChromeDllPath = Join-Path $EngineRoot 'chrome-win\chrome.dll'
    $brunoLocalesPath = Join-Path $EngineRoot 'chrome-win\locales'
    $brunoIconPath = Join-Path $brunoWorkspace 'build\windows\icon.ico'
    $brunoBrandScriptPath = Join-Path $brunoWorkspace 'scripts\brand-engine.mjs'
    $brunoBrandMarker = Join-Path $EngineRoot '.bruno-engine-brand'
    $brunoIconHash = (Get-FileHash -LiteralPath $brunoIconPath -Algorithm SHA256).Hash
    $brunoBrandScriptHash = (Get-FileHash -LiteralPath $brunoBrandScriptPath -Algorithm SHA256).Hash
    $brunoBrandKey = "$($Definition.version)|$brunoIconHash|$($Definition.toolSha256)|$brunoBrandScriptHash"
    $brunoCurrentProduct = (Get-Item -LiteralPath $brunoChromePath).VersionInfo.ProductName
    $brunoCurrentDllProduct = (Get-Item -LiteralPath $brunoChromeDllPath).VersionInfo.ProductName
    $brunoAlreadyBranded = (Test-Path -LiteralPath $brunoBrandMarker) -and
        ((Get-Content -LiteralPath $brunoBrandMarker -Raw).Trim() -eq $brunoBrandKey) -and
        ($brunoCurrentProduct -eq 'Bruno Engine') -and
        ($brunoCurrentDllProduct -eq 'Bruno Engine')
    if ($brunoAlreadyBranded) { return }

    Write-Host "Aplicando logo e identidade Bruno Engine em $EngineRoot..."
    & $ToolPath $brunoChromePath `
        --set-icon $brunoIconPath `
        --set-version-string ProductName 'Bruno Engine' `
        --set-version-string FileDescription 'Bruno Engine (Chromium)' `
        --set-version-string CompanyName 'Bruno Browser' `
        --set-version-string InternalName 'Bruno Engine'
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    & $ToolPath $brunoChromeDllPath `
        --set-icon $brunoIconPath `
        --set-version-string ProductName 'Bruno Engine' `
        --set-version-string FileDescription 'Bruno Engine Core' `
        --set-version-string CompanyName 'Bruno Browser' `
        --set-version-string InternalName 'Bruno Engine'
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    node $brunoBrandScriptPath patch-locales $brunoLocalesPath
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    $brunoVersionInfo = (Get-Item -LiteralPath $brunoChromePath).VersionInfo
    $brunoDllVersionInfo = (Get-Item -LiteralPath $brunoChromeDllPath).VersionInfo
    if ($brunoVersionInfo.ProductName -ne 'Bruno Engine' -or $brunoVersionInfo.CompanyName -ne 'Bruno Browser' -or
        $brunoDllVersionInfo.ProductName -ne 'Bruno Engine' -or $brunoDllVersionInfo.CompanyName -ne 'Bruno Browser') {
        throw "A identidade do Bruno Engine não foi aplicada em $brunoChromePath."
    }
    Set-Content -LiteralPath $brunoBrandMarker -Value $brunoBrandKey -Encoding ascii
}

function Set-BrunoApplicationBranding {
    param(
        [Parameter(Mandatory = $true)][string]$ExecutablePath,
        [Parameter(Mandatory = $true)][string]$ToolPath,
        [Parameter(Mandatory = $true)][string]$Version
    )
    if (-not (Test-Path -LiteralPath $ExecutablePath)) { return }

    $brunoIconPath = Join-Path $brunoWorkspace 'build\windows\icon.ico'
    Write-Host "Aplicando identidade Bruno Browser em $ExecutablePath..."
    & $ToolPath $ExecutablePath `
        --set-icon $brunoIconPath `
        --set-file-version $Version `
        --set-product-version $Version `
        --set-version-string ProductName 'Bruno Browser' `
        --set-version-string FileDescription 'Bruno Browser' `
        --set-version-string CompanyName 'Bruno Browser' `
        --set-version-string InternalName 'Bruno Browser' `
        --set-version-string OriginalFilename 'Bruno Browser.exe'
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    $brunoVersionInfo = (Get-Item -LiteralPath $ExecutablePath).VersionInfo
    if ($brunoVersionInfo.ProductName -ne 'Bruno Browser' -or
        $brunoVersionInfo.CompanyName -ne 'Bruno Browser' -or
        $brunoVersionInfo.ProductVersion -notlike "$Version*") {
        throw "A identidade do Bruno Browser não foi aplicada em $ExecutablePath."
    }
}

function Copy-BrunoStartExtension {
    param([Parameter(Mandatory = $true)][string]$EngineDestination)
    $brunoStartDestination = Join-Path $EngineDestination 'bruno-start'
    if (Test-Path -LiteralPath $brunoStartDestination) {
        Remove-Item -LiteralPath $brunoStartDestination -Recurse -Force
    }
    New-Item -ItemType Directory -Force -Path $brunoStartDestination | Out-Null
    Copy-Item -Path (Join-Path $brunoWorkspace 'assets\bruno-start\*') -Destination $brunoStartDestination -Recurse -Force
    Copy-Item -LiteralPath (Join-Path $brunoWorkspace 'docs\assets\bruno-browser-icon.png') -Destination (Join-Path $brunoStartDestination 'icon.png') -Force
    Copy-Item -LiteralPath (Join-Path $brunoWorkspace 'scripts\engine-manifest.json') -Destination (Join-Path $EngineDestination 'manifest.json') -Force
}

function Copy-BrunoNativeExtensions {
    param([Parameter(Mandatory = $true)][string]$ApplicationDestination)
    $brunoNativeSource = Join-Path $brunoWorkspace 'assets\native-extensions'
    $brunoNativeDestination = Join-Path $ApplicationDestination 'native-extensions'
    if (Test-Path -LiteralPath $brunoNativeDestination) {
        Remove-Item -LiteralPath $brunoNativeDestination -Recurse -Force
    }
    New-Item -ItemType Directory -Force -Path $brunoNativeDestination | Out-Null
    Copy-Item -Path (Join-Path $brunoNativeSource '*') -Destination $brunoNativeDestination -Recurse -Force
}

$brunoEngineManifestPath = Join-Path $brunoWorkspace 'scripts\engine-manifest.json'
$brunoEngineManifest = Get-Content -LiteralPath $brunoEngineManifestPath -Raw | ConvertFrom-Json
$brunoApplicationVersion = [string]((Get-Content -LiteralPath (Join-Path $brunoWorkspace 'wails.json') -Raw | ConvertFrom-Json).info.productVersion)
$brunoEngineAmd64 = Get-BrunoEnginePlatform -Architecture 'amd64' -Definition $brunoEngineManifest.platforms.amd64
$brunoEngineArm64 = Get-BrunoEnginePlatform -Architecture 'arm64' -Definition $brunoEngineManifest.platforms.arm64
$brunoBrandingTool = Get-BrunoBrandingTool -Definition $brunoEngineManifest.branding
Set-BrunoEngineBranding -EngineRoot $brunoEngineAmd64 -ToolPath $brunoBrandingTool -Definition $brunoEngineManifest.branding
Set-BrunoEngineBranding -EngineRoot $brunoEngineArm64 -ToolPath $brunoBrandingTool -Definition $brunoEngineManifest.branding

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
    Set-BrunoApplicationBranding -ExecutablePath (Join-Path $brunoOutputDirectory 'Bruno Browser.exe') -ToolPath $brunoBrandingTool -Version $brunoApplicationVersion
    $brunoDirectEngine = Join-Path $brunoOutputDirectory 'engine'
    if (Test-Path -LiteralPath $brunoDirectEngine) {
        Remove-Item -LiteralPath $brunoDirectEngine -Recurse -Force
    }
    New-Item -ItemType Directory -Force -Path $brunoDirectEngine | Out-Null
    Copy-Item -LiteralPath (Join-Path $brunoEngineAmd64 'chrome-win') -Destination $brunoDirectEngine -Recurse -Force
    Copy-BrunoStartExtension -EngineDestination $brunoDirectEngine
    Copy-BrunoNativeExtensions -ApplicationDestination $brunoOutputDirectory
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
$brunoArm64Executable = Join-Path $brunoWorkspace 'build\bin\Bruno Browser-arm64.exe'
$brunoPortableExecutable = Join-Path $brunoWorkspace 'build\bin\Bruno Browser.exe'
Set-BrunoApplicationBranding -ExecutablePath $brunoAmd64Executable -ToolPath $brunoBrandingTool -Version $brunoApplicationVersion
Set-BrunoApplicationBranding -ExecutablePath $brunoArm64Executable -ToolPath $brunoBrandingTool -Version $brunoApplicationVersion
if (Test-Path -LiteralPath $brunoAmd64Executable) {
    Copy-Item -LiteralPath $brunoAmd64Executable -Destination $brunoPortableExecutable -Force
}

if (-not $NoInstaller) {
    $brunoInstallerDirectory = Join-Path $brunoWorkspace 'build\windows\installer'
    $brunoMakeNsis = Join-Path $brunoNsisBin 'makensis.exe'
    if (-not (Test-Path -LiteralPath $brunoMakeNsis)) {
        throw 'O compilador NSIS não foi encontrado para finalizar o instalador com a identidade do aplicativo.'
    }
    Write-Host 'Recompilando o instalador com os executáveis identificados...'
    Push-Location $brunoInstallerDirectory
    try {
        & $brunoMakeNsis '-DBRUNO_FINAL_PACKAGE=1' "-DARG_WAILS_AMD64_BINARY=$brunoAmd64Executable" "-DARG_WAILS_ARM64_BINARY=$brunoArm64Executable" 'project.nsi'
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    } finally {
        Pop-Location
    }
}

$brunoLocalEngine = Join-Path $brunoWorkspace 'build\bin\engine'
if (Test-Path -LiteralPath $brunoLocalEngine) {
    Remove-Item -LiteralPath $brunoLocalEngine -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $brunoLocalEngine | Out-Null
Copy-Item -LiteralPath (Join-Path $brunoEngineAmd64 'chrome-win') -Destination $brunoLocalEngine -Recurse -Force
Copy-BrunoStartExtension -EngineDestination $brunoLocalEngine
Copy-BrunoNativeExtensions -ApplicationDestination (Join-Path $brunoWorkspace 'build\bin')

if (-not $NoInstaller) {
    foreach ($brunoPortable in @(
        @{ Architecture = 'amd64'; Executable = $brunoAmd64Executable; Engine = $brunoEngineAmd64 },
        @{ Architecture = 'arm64'; Executable = $brunoArm64Executable; Engine = $brunoEngineArm64 }
    )) {
        if (-not (Test-Path -LiteralPath $brunoPortable.Executable)) { continue }
        $brunoPortableRoot = Join-Path $brunoWorkspace ".cache\portable\$($brunoPortable.Architecture)"
        if (Test-Path -LiteralPath $brunoPortableRoot) {
            Remove-Item -LiteralPath $brunoPortableRoot -Recurse -Force
        }
        New-Item -ItemType Directory -Force -Path (Join-Path $brunoPortableRoot 'engine') | Out-Null
        Copy-Item -LiteralPath $brunoPortable.Executable -Destination (Join-Path $brunoPortableRoot 'Bruno Browser.exe') -Force
        Copy-Item -LiteralPath (Join-Path $brunoPortable.Engine 'chrome-win') -Destination (Join-Path $brunoPortableRoot 'engine') -Recurse -Force
        Copy-BrunoStartExtension -EngineDestination (Join-Path $brunoPortableRoot 'engine')
        Copy-BrunoNativeExtensions -ApplicationDestination $brunoPortableRoot
        Copy-Item -LiteralPath (Join-Path $brunoWorkspace 'docs\THIRD_PARTY_NOTICES.txt') -Destination $brunoPortableRoot -Force
        $brunoPortableArchive = Join-Path $brunoWorkspace "build\bin\Bruno-Browser-portable-$($brunoPortable.Architecture).zip"
        if (Test-Path -LiteralPath $brunoPortableArchive) {
            Remove-Item -LiteralPath $brunoPortableArchive -Force
        }
        Compress-Archive -Path (Join-Path $brunoPortableRoot '*') -DestinationPath $brunoPortableArchive -CompressionLevel Optimal
    }
}
