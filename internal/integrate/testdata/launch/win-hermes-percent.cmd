@echo off
chcp 65001 >nul
title Hermes ^(EZLM^)
set "PATH=C:\Users\100%%Sam\AppData\Local\hermes\bin;%PATH%"
cd /d "C:\Users\100%%Sam\EZLM Workspace" || (>"C:\Users\100%%Sam\.ezlm\launch\hermes.exit" echo 1& exit /b 1)
(
"C:\Users\100%%Sam\AppData\Local\hermes\bin\hermes.exe" chat --provider "custom:ezlm" -m "qwen%%2"
if errorlevel 1 (
(call echo %%ERRORLEVEL%%)>"C:\Users\100%%Sam\.ezlm\launch\hermes.exit"
echo.
echo Hermes stopped. EZLM shows what went wrong.
pause
) else if not errorlevel 0 (
(call echo %%ERRORLEVEL%%)>"C:\Users\100%%Sam\.ezlm\launch\hermes.exit"
echo.
echo Hermes stopped. EZLM shows what went wrong.
pause
) else (
>"C:\Users\100%%Sam\.ezlm\launch\hermes.exit" echo 0
)
exit /b
)
