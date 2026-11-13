$ErrorActionPreference = 'Stop'
try {
    if (-not $env:TVROCK_CONTEXT_FILE) { throw 'TVROCK_CONTEXT_FILE is required.' }
    $info = Get-Content -LiteralPath $env:TVROCK_CONTEXT_FILE -Raw -Encoding UTF8 | ConvertFrom-Json
    if ($info.recordFile) { Get-Item -LiteralPath $info.recordFile }
}
catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
