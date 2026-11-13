$ErrorActionPreference = 'Stop'

$repoRoot = $PSScriptRoot
$artifactRoot = Join-Path $repoRoot 'tvrockgo'
$displayVersion = if ($env:APP_VERSION) { $env:APP_VERSION } elseif ($env:CI_COMMIT_TAG) { $env:CI_COMMIT_TAG } elseif ($env:GITHUB_REF_TYPE -eq 'tag') { $env:GITHUB_REF_NAME } else { 'dev' }
$env:APP_VERSION = $displayVersion
$env:VUE_APP_VERSION = $displayVersion
# Electron downloads its distribution on first use; npm ci alone does not fetch it.
& node (Join-Path $repoRoot 'rockbar/node_modules/electron/install.js')
if ($LASTEXITCODE -ne 0) { throw 'Electron distribution installation failed.' }
& node (Join-Path $repoRoot 'scripts/generate-third-party-licenses.cjs') --check
if ($LASTEXITCODE -ne 0) { throw 'Third-party license notices are out of date. See docs/releases.md.' }
if ($displayVersion -ne 'dev') {
    $version = & node (Join-Path $repoRoot 'rockbar/scripts/set-release-version.cjs') $displayVersion
    if ($LASTEXITCODE -ne 0) { throw 'Rockbar release version update failed.' }
    $env:ROCKBAR_VERSION = $version
} else {
    $env:ROCKBAR_VERSION = (Get-Content -LiteralPath (Join-Path $repoRoot 'rockbar/package.json') -Raw | ConvertFrom-Json).version
}

function Reset-BuildDirectory([string]$Path) {
    $resolved = [IO.Path]::GetFullPath($Path)
    $root = [IO.Path]::GetFullPath($repoRoot).TrimEnd('\', '/') + [IO.Path]::DirectorySeparatorChar
    if (-not $resolved.StartsWith($root, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Build output is outside the repository: $resolved"
    }
    if (Test-Path -LiteralPath $resolved) { Remove-Item -LiteralPath $resolved -Recurse -Force }
}

function Invoke-NpmBuild([string]$Directory, [string[]]$Arguments) {
    Push-Location (Join-Path $repoRoot $Directory)
    try {
        & npm @Arguments
        if ($LASTEXITCODE -ne 0) { throw "npm $($Arguments -join ' ') failed in $Directory (exit code $LASTEXITCODE)" }
    }
    finally { Pop-Location }
}

Invoke-NpmBuild 'scripts/release-manual' @('ci', '--ignore-scripts', '--no-audit', '--no-fund')
& (Join-Path $repoRoot 'build.ps1')
if ($LASTEXITCODE -ne 0) { throw "build.ps1 failed with exit code $LASTEXITCODE" }
Invoke-NpmBuild 'app' @('run', 'build')
Reset-BuildDirectory (Join-Path $repoRoot 'rockbar/dist')
Invoke-NpmBuild 'rockbar' @('run', 'build:all')

Reset-BuildDirectory $artifactRoot
New-Item -ItemType Directory -Path $artifactRoot -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $artifactRoot 'conf') -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $artifactRoot 'data') -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $artifactRoot 'log') -Force | Out-Null

Copy-Item -LiteralPath (Join-Path $repoRoot 'tvrockgo.exe') -Destination (Join-Path $artifactRoot 'tvrockgo.exe')
foreach ($document in @('README.md', 'LICENSE', 'THIRD_PARTY_LICENSES.md')) {
    Copy-Item -LiteralPath (Join-Path $repoRoot $document) -Destination $artifactRoot
}
$electronLicenses = Join-Path $artifactRoot 'licenses/electron'
New-Item -ItemType Directory -Path $electronLicenses -Force | Out-Null
foreach ($document in @('LICENSE', 'LICENSES.chromium.html')) {
    Copy-Item -LiteralPath (Join-Path $repoRoot "rockbar/node_modules/electron/dist/$document") -Destination $electronLicenses
}
New-Item -ItemType Directory -Path (Join-Path $artifactRoot 'docs') -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $repoRoot 'docs/images') -Destination (Join-Path $artifactRoot 'docs/images') -Recurse
# Keep release documentation on an explicit allowlist; do not copy docs wholesale.
$manualChapters = @('docs/startup.md', 'docs/configuration.md', 'docs/tvguide.md', 'docs/rockbar.md', 'docs/remote-viewing.md', 'docs/playback.md', 'docs/custom-commands.md')
foreach ($document in $manualChapters) {
    Copy-Item -LiteralPath (Join-Path $repoRoot $document) -Destination (Join-Path $artifactRoot $document)
}
Copy-Item -LiteralPath (Join-Path $repoRoot 'docs/custom-command-examples') -Destination (Join-Path $artifactRoot 'docs/custom-command-examples') -Recurse
Copy-Item -LiteralPath (Join-Path $repoRoot 'deploy') -Destination (Join-Path $artifactRoot 'deploy') -Recurse
Copy-Item -LiteralPath (Join-Path $repoRoot 'static') -Destination (Join-Path $artifactRoot 'static') -Recurse
Copy-Item -LiteralPath (Join-Path $repoRoot 'conf/tvrock.org.yml') -Destination (Join-Path $artifactRoot 'conf/tvrock.org.yml')
New-Item -ItemType Directory -Path (Join-Path $artifactRoot 'rockbar') -Force | Out-Null
$rockbarInstaller = Join-Path $repoRoot "rockbar/dist/Rockbar-Setup-$env:ROCKBAR_VERSION-x64.exe"
Copy-Item -LiteralPath $rockbarInstaller -Destination (Join-Path $artifactRoot 'rockbar')
Copy-Item -LiteralPath (Join-Path $repoRoot 'template') -Destination (Join-Path $artifactRoot 'template') -Recurse

& node (Join-Path $repoRoot 'scripts/release-manual/generate.cjs') $artifactRoot @manualChapters
if ($LASTEXITCODE -ne 0) { throw "Release manual generation failed with exit code $LASTEXITCODE" }

$archiveRoot = Join-Path $repoRoot '.tmp/release'
New-Item -ItemType Directory -Path $archiveRoot -Force | Out-Null
$archive = Join-Path $archiveRoot 'tvrockgo.zip'
Compress-Archive -LiteralPath $artifactRoot -DestinationPath $archive -Force

Write-Host "Build complete: $artifactRoot"
Write-Host "Release archive: $archive"
