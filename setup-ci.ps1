$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$toolsRoot = Join-Path $PSScriptRoot '.tmp/ci-tools'
New-Item -ItemType Directory -Path $toolsRoot -Force | Out-Null

function Install-CIZip([string]$Url, [string]$Checksum, [string]$Directory) {
    $archive = "$Directory.zip"
    Invoke-WebRequest -Uri $Url -OutFile $archive -UseBasicParsing
    if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash -ne $Checksum) {
        throw "Checksum mismatch: $Url"
    }
    Expand-Archive -LiteralPath $archive -DestinationPath $Directory -Force
}

$goMatch = [regex]::Match((Get-Content -LiteralPath (Join-Path $PSScriptRoot 'go.mod') -Raw), '(?m)^go\s+(\d+\.\d+\.\d+)')
if (-not $goMatch.Success) { throw 'go.mod must specify a Go version.' }
$requiredGoVersion = $goMatch.Groups[1].Value
$installedGoVersion = if (Get-Command go -ErrorAction SilentlyContinue) { (& go version) } else { '' }
if ($installedGoVersion -notmatch "go$([regex]::Escape($requiredGoVersion))\s") {
    $releases = Invoke-RestMethod -Uri 'https://go.dev/dl/?mode=json&include=all'
    $file = $releases | Where-Object version -eq "go$requiredGoVersion" | ForEach-Object files |
        Where-Object { $_.os -eq 'windows' -and $_.arch -eq 'amd64' -and $_.kind -eq 'archive' } | Select-Object -First 1
    if (-not $file) { throw "Go Windows archive not found: $requiredGoVersion" }
    $goDirectory = Join-Path $toolsRoot "go-$requiredGoVersion"
    Install-CIZip "https://go.dev/dl/$($file.filename)" $file.sha256 $goDirectory
    $env:PATH = "$(Join-Path $goDirectory 'go/bin');$env:PATH"
    $env:GOROOT = Join-Path $goDirectory 'go'
}

$installedNodeVersion = if (Get-Command node -ErrorAction SilentlyContinue) { (& node --version).TrimStart('v') } else { '0.0.0' }
if (([version]$installedNodeVersion).Major -ne 22 -or [version]$installedNodeVersion -lt [version]'22.12.0') {
    $baseUri = 'https://nodejs.org/dist/latest-v22.x'
    $checksums = (Invoke-WebRequest -Uri "$baseUri/SHASUMS256.txt" -UseBasicParsing).Content
    $match = [regex]::Match($checksums, '(?m)^([a-f0-9]{64})\s+(node-v22\.\d+\.\d+-win-x64\.zip)\s*$')
    if (-not $match.Success) { throw 'Node.js 22 Windows checksum was not found.' }
    $nodeArchive = $match.Groups[2].Value
    $nodeDirectory = Join-Path $toolsRoot 'node22'
    Install-CIZip "$baseUri/$nodeArchive" $match.Groups[1].Value $nodeDirectory
    $env:PATH = "$(Join-Path $nodeDirectory ($nodeArchive -replace '\.zip$', ''));$env:PATH"
}
$env:CGO_ENABLED = '0'
$env:GOTOOLCHAIN = 'auto'
& go version
if ($LASTEXITCODE -ne 0) { throw 'Go setup failed.' }
& node --version
if ($LASTEXITCODE -ne 0) { throw 'Node.js setup failed.' }
