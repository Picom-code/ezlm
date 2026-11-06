@echo off
chcp 65001 >nul
title DeepSeek Harness ^(EZLM^)
set "DSH_TELEMETRY_MODE=DISABLED"
set "PATH=C:\Users\Sam\.ezlm\tools\node\24.11.1;%PATH%"
cd /d "C:\Users\Sam\EZLM Workspace" || (>"C:\Users\Sam\.ezlm\launch\dsh.exit" echo 1& exit /b 1)
(
"C:\Users\Sam\.ezlm\tools\node\24.11.1\node.exe" "C:\Users\Sam\.ezlm\tools\dsh\0.2.1-rc.5\node_modules\@deepseek-ai\dsh\bin\dsh.js" web --port 3081
if errorlevel 1 (
(call echo %%ERRORLEVEL%%)>"C:\Users\Sam\.ezlm\launch\dsh.exit"
echo.
echo DeepSeek Harness stopped. EZLM shows what went wrong.
pause
) else if not errorlevel 0 (
(call echo %%ERRORLEVEL%%)>"C:\Users\Sam\.ezlm\launch\dsh.exit"
echo.
echo DeepSeek Harness stopped. EZLM shows what went wrong.
pause
) else (
>"C:\Users\Sam\.ezlm\launch\dsh.exit" echo 0
)
exit /b
)
