@echo off
chcp 65001 >nul
title OpenCode ^(EZLM^)
cd /d "C:\Users\Sam & Co\EZLM Workspace" || (>"C:\Users\Sam & Co\.ezlm\launch\opencode.exit" echo 1& exit /b 1)
(
call "C:\Users\Sam & Co\AppData\Roaming\npm\opencode.cmd" --model "ezlm/qwen%%%%x"
if errorlevel 1 (
(call echo %%ERRORLEVEL%%)>"C:\Users\Sam & Co\.ezlm\launch\opencode.exit"
echo.
echo OpenCode stopped. EZLM shows what went wrong.
pause
) else if not errorlevel 0 (
(call echo %%ERRORLEVEL%%)>"C:\Users\Sam & Co\.ezlm\launch\opencode.exit"
echo.
echo OpenCode stopped. EZLM shows what went wrong.
pause
) else (
>"C:\Users\Sam & Co\.ezlm\launch\opencode.exit" echo 0
)
exit /b
)
