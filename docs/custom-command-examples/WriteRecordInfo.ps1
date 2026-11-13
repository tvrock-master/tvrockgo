param(
    [Parameter(Mandatory)][ValidateSet('Touch', 'Tweet')][string]$Mode,
    [string]$OutputDirectory = 'C:\Temp'
)
$ErrorActionPreference = 'Stop'
try {
    if (-not $env:TVROCK_CONTEXT_FILE) { throw 'TVROCK_CONTEXT_FILE is required.' }
    $info = Get-Content -LiteralPath $env:TVROCK_CONTEXT_FILE -Raw -Encoding UTF8 | ConvertFrom-Json
    if ($Mode -eq 'Touch') {
        $file = Join-Path $OutputDirectory 'record.txt'
        $line = '{0} {1}:{2} {3} {4} {5}' -f @(
            $info.date, $info.startTime, $info.startSeconds,
            $info.title, $info.subtitle, $info.recordFile
        )
    }
    else {
        $file = Join-Path $OutputDirectory 'tweet.txt'
        # Unicode escape keeps this script ASCII-compatible with Windows PowerShell 5.1.
        $line = '{0}~{1}({2}{3}) {4} {5} {6} {7}' -f @(
            $info.startTime, $info.endTime, $info.durationMinutes, [char]0x5206,
            $info.title, $info.subtitle, $info.keywordHashtags, $info.stationHashtags
        )
    }
    Add-Content -LiteralPath $file -Value $line -Encoding UTF8
}
catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
