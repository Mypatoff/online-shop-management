@echo off
rem Builds ShopKeeper for Windows (a window, no console) and Linux (opens the browser) into dist\.
setlocal
cd /d "%~dp0"

if not exist dist mkdir dist

rem The version shown in the app and compared by its "Update" button: set VERSION=1.2.0 before running.
if "%VERSION%"=="" set VERSION=dev
set VERSION_FLAG=-X shop/internal/update.Version=%VERSION%

set CGO_ENABLED=0

echo Building dist\ShopKeeper.exe (windows/amd64)...
set GOOS=windows
set GOARCH=amd64
go build -ldflags "-s -w -H=windowsgui %VERSION_FLAG%" -o dist\ShopKeeper.exe .\cmd\server
if errorlevel 1 exit /b 1

echo Building dist\shopkeeper-linux (linux/amd64)...
set GOOS=linux
set GOARCH=amd64
go build -ldflags "-s -w %VERSION_FLAG%" -o dist\shopkeeper-linux .\cmd\server
if errorlevel 1 exit /b 1

echo Done. Binaries in dist\
endlocal
