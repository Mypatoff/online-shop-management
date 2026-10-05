@echo off
rem Builds double-clickable ShopKeeper binaries for Windows and Linux into dist\.
setlocal
cd /d "%~dp0"

if not exist dist mkdir dist

set CGO_ENABLED=0

echo Building dist\ShopKeeper.exe (windows/amd64)...
set GOOS=windows
set GOARCH=amd64
go build -ldflags "-s -w" -o dist\ShopKeeper.exe .\cmd\server
if errorlevel 1 exit /b 1

echo Building dist\shopkeeper-linux (linux/amd64)...
set GOOS=linux
set GOARCH=amd64
go build -ldflags "-s -w" -o dist\shopkeeper-linux .\cmd\server
if errorlevel 1 exit /b 1

echo Done. Binaries in dist\
endlocal
