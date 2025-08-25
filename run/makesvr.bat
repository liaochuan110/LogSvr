@echo off
echo Building logsvr...
if exist logsvr.exe del logsvr.exe
cd ..\core
go build -o ..\run\logsvr.exe .
cd ..\run
if exist logsvr.exe (
    echo logsvr built successfully
) else (
    echo Failed to build logsvr
    pause
)