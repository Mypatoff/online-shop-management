<#
    Builds dist\ShopKeeper-Setup.exe: builds dist\ShopKeeper.exe with build.bat, then compiles
    installer\ShopKeeper.iss with Inno Setup 6.

    Usage:
      powershell -File installer\build-installer.ps1
      powershell -File installer\build-installer.ps1 -Version 1.2.0
      powershell -File installer\build-installer.ps1 -Version 1.3.0 -AppVersion 1.3.0-beta.1

    -Version is the number the installer shows; Inno Setup needs it numeric (1.3.0). -AppVersion is
    the version built into the app, which the Update button compares against GitHub: it defaults to
    -Version, but for a pre-release it must keep the suffix (1.3.0-beta.1), or the app would call
    itself 1.3.0 and never be offered the real 1.3.0.
#>
param(
    [string]$Version = "1.0.0",
    [string]$AppVersion = "",
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

if (-not $AppVersion) { $AppVersion = $Version }
$env:VERSION = $AppVersion   # build.bat embeds it, so the app knows its own version
& cmd /c build.bat
if ($LASTEXITCODE -ne 0) { throw "build.bat failed" }

& $InnoCompiler "/DAppVersion=$Version" "installer\ShopKeeper.iss"
if ($LASTEXITCODE -ne 0) { throw "Inno Setup failed" }

Write-Host "Done: dist\ShopKeeper-Setup.exe"
