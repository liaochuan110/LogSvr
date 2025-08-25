@echo off
echo Stopping logsvr server...
tasklist /FI "IMAGENAME eq logsvr.exe" | find /I "logsvr.exe" >nul
if %errorlevel% neq 0 (
    echo No logsvr.exe process found
    exit /b 0
)
taskkill /f /im logsvr.exe >nul 2>&1
if %errorlevel% equ 0 (
    echo Logsvr server stopped successfully
) else (
    echo Failed to stop logsvr server
)