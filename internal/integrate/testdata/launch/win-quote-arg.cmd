@echo off
chcp 65001 >nul
title Hermes ^(EZLM^)
cd /d "C:\Users\Sam\EZLM Workspace" || (>"C:\Users\Sam\.ezlm\launch\hermes.exit" echo 1& exit /b 1)
(
"C:\Users\Sam\AppData\Local\Microsoft\WindowsApps\hermes.exe" chat -q "say \"hi\" & (bye) ^ <ok>" "C:\dir with space\\"
if errorlevel 1 (
(call echo %%ERRORLEVEL%%)>"C:\Users\Sam\.ezlm\launch\hermes.exit"
echo.
echo Hermes stopped. EZLM shows what went wrong.
pause
) else if not errorlevel 0 (
(call echo %%ERRORLEVEL%%)>"C:\Users\Sam\.ezlm\launch\hermes.exit"
echo.
echo Hermes stopped. EZLM shows what went wrong.
pause
) else (
>"C:\Users\Sam\.ezlm\launch\hermes.exit" echo 0
)
exit /b
)
