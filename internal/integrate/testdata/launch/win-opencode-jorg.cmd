@echo off
chcp 65001 >nul
title OpenCode ^(EZLM^)
cd /d "C:\Users\Jörg\EZLM Workspace" || (>"C:\Users\Jörg\.ezlm\launch\opencode.exit" echo 1& exit /b 1)
(
"C:\Users\Jörg\.ezlm\tools\opencode\2.2.5\opencode.exe" --standalone --model "ezlm/qwen3.8-27b"
if errorlevel 1 (
(call echo %%ERRORLEVEL%%)>"C:\Users\Jörg\.ezlm\launch\opencode.exit"
echo.
echo OpenCode stopped. EZLM shows what went wrong.
pause
) else if not errorlevel 0 (
(call echo %%ERRORLEVEL%%)>"C:\Users\Jörg\.ezlm\launch\opencode.exit"
echo.
echo OpenCode stopped. EZLM shows what went wrong.
pause
) else (
>"C:\Users\Jörg\.ezlm\launch\opencode.exit" echo 0
)
exit /b
)
