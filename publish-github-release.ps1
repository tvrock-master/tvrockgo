$ErrorActionPreference = 'Stop'

foreach ($name in @('GH_TOKEN', 'GH_REPO', 'RELEASE_TAG')) {
    if (-not [Environment]::GetEnvironmentVariable($name)) { throw "Required GitHub variable is missing: $name" }
}
$archive = Join-Path $PSScriptRoot '.tmp/release/tvrockgo.zip'
if (-not (Test-Path -LiteralPath $archive -PathType Leaf)) { throw 'Release archive is missing. Run buildall.ps1 first.' }

$tag = [Uri]::EscapeDataString($env:RELEASE_TAG)
$headers = @{ Authorization = "Bearer $env:GH_TOKEN"; Accept = 'application/vnd.github+json' }
$release = $null
try {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$env:GH_REPO/releases/tags/$tag" -Headers $headers -Method Get
} catch {
    # Authentication, rate limit and server errors must not create a new release.
    if (-not $_.Exception.Response -or [int]$_.Exception.Response.StatusCode -ne 404) { throw }
}

if ($null -eq $release) {
    gh release create $env:RELEASE_TAG $archive --verify-tag --title $env:RELEASE_TAG --generate-notes
} else {
    gh release upload $env:RELEASE_TAG $archive --clobber
}
if ($LASTEXITCODE -ne 0) { throw 'GitHub Release publication failed.' }
Write-Output "Published GitHub Release: $env:RELEASE_TAG (tvrockgo.zip)"
