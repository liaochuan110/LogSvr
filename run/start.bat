@echo off
echo Starting logsvr server...
if not exist logsvr.exe (
    echo Error: logsvr.exe not found. Please run makesvr.bat first.
    pause
    exit /b 1
)
start "Logsvr Server" logsvr.exe
echo Logsvr server started in background
echo You can check the server status in Task Manager