@echo off
setlocal DisableDelayedExpansion
"%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -NonInteractive -File "%~dp0WriteRecordInfo.ps1" -Mode Tweet
exit /b %errorlevel%
