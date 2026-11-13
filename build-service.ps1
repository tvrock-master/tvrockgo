param(
    [ValidateSet('linux', 'darwin')][string]$TargetOS = 'linux',
    [ValidateSet('amd64', 'arm64')][string]$Arch = 'amd64'
)
$ErrorActionPreference = 'Stop'
$repoRoot = $PSScriptRoot
$tempRoot = Join-Path $repoRoot '.tmp'
$outputRoot = Join-Path $tempRoot "service/$TargetOS-$Arch"
$version = if ($env:APP_VERSION) { $env:APP_VERSION -replace '^v', '' } else { 'dev' }
$savedEnvironment = @{}
foreach ($key in @('GOOS', 'GOARCH', 'CGO_ENABLED', 'GOCACHE', 'TMP', 'TEMP')) {
    $savedEnvironment[$key] = [Environment]::GetEnvironmentVariable($key, 'Process')
}
New-Item -ItemType Directory -Path $outputRoot -Force | Out-Null
Push-Location $repoRoot
try {
    $env:GOOS = $TargetOS
    $env:GOARCH = $Arch
    $env:CGO_ENABLED = '0'
    $env:GOCACHE = Join-Path $tempRoot 'gocache'
    $env:TMP = $tempRoot
    $env:TEMP = $tempRoot
    New-Item -ItemType Directory -Path $env:GOCACHE -Force | Out-Null
    & go build -trimpath -ldflags="-s -w -X github.com/tvrock-master/tvrockgo/tvrock.Version=$version" -o (Join-Path $outputRoot 'tvrockgo') .
    if ($LASTEXITCODE -ne 0) { throw "Service build failed with exit code $LASTEXITCODE" }
    Write-Host "Service binary: $(Join-Path $outputRoot 'tvrockgo')"
}
finally {
    Pop-Location
    foreach ($key in $savedEnvironment.Keys) {
        [Environment]::SetEnvironmentVariable($key, $savedEnvironment[$key], 'Process')
    }
}
