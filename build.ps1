param([string]$OutputPath = (Join-Path $PSScriptRoot 'tvrockgo.exe'))
$ErrorActionPreference = 'Stop'

$repoRoot = $PSScriptRoot
$tempRoot = Join-Path $repoRoot '.tmp'
$env:GOOS = 'windows'
$env:CGO_ENABLED = '0'
$env:GOARCH = 'amd64'
$env:CGO_CFLAGS = '-Wno-return-local-addr'
$env:TMP = $tempRoot
$env:TEMP = $tempRoot
$env:GOCACHE = Join-Path $tempRoot 'gocache'

$version = if ($env:APP_VERSION) { $env:APP_VERSION } else { 'dev' }
$version = $version -replace '^v', ''

New-Item -ItemType Directory -Path $env:GOCACHE -Force | Out-Null
Push-Location $repoRoot
try {
    $iconPath = Join-Path $repoRoot 'app/public/image/tvrockgo.png'
    if (-not (Test-Path -LiteralPath $iconPath -PathType Leaf)) {
        throw "Icon file not found: $iconPath"
    }
    & go run github.com/tc-hib/go-winres@v0.3.3 simply --icon $iconPath --arch amd64 --manifest none
    if ($LASTEXITCODE -ne 0) { throw "Icon resource generation failed with exit code $LASTEXITCODE" }

    $outputFullPath = [IO.Path]::GetFullPath($OutputPath)
    New-Item -ItemType Directory -Path ([IO.Path]::GetDirectoryName($outputFullPath)) -Force | Out-Null
    & go build -trimpath -ldflags="-H=windowsgui -s -w -X github.com/tvrock-master/tvrockgo/tvrock.Version=$version" -o $outputFullPath .
    if ($LASTEXITCODE -ne 0) { throw "go build failed with exit code $LASTEXITCODE" }
}
finally {
    Pop-Location
}
