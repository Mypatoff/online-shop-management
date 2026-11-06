<#
    Builds dist\ShopKeeper-Setup.exe: builds dist\ShopKeeper.exe with build.bat, then compiles
    installer\ShopKeeper.iss with Inno Setup 6.

    Usage:
      powershell -File installer\build-installer.ps1
      powershell -File installer\build-installer.ps1 -Version 1.2.0
#>
param(
    [string]$Version = "1.0.0",
    [string]$InnoCompiler = ""
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

if (-not $InnoCompiler) {
    $candidates = @(
        "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
        "$env:ProgramFiles\Inno Setup 6\ISCC.exe",
        "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe"
    )
    $InnoCompiler = $candidates | Where-Object { Test-Path $_ } | Select-Object -First 1
}
if (-not $InnoCompiler -or -not (Test-Path $InnoCompiler)) {
    throw "Inno Setup 6 not found. Install it from https://jrsoftware.org/isdl.php or pass -InnoCompiler <path to ISCC.exe>."
}

& cmd /c build.bat
if ($LASTEXITCODE -ne 0) { throw "build.bat failed" }

& $InnoCompiler "/DAppVersion=$Version" "installer\ShopKeeper.iss"
if ($LASTEXITCODE -ne 0) { throw "Inno Setup failed" }

Write-Host "Done: dist\ShopKeeper-Setup.exe"
