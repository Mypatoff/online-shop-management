; ShopKeeper setup. Build it with installer\build-installer.ps1, or by hand with
;   ISCC.exe /DAppVersion=1.0.0 installer\ShopKeeper.iss
; after building dist\ShopKeeper.exe (build.bat / build.sh). Nothing else has to be installed on the
; shop PC except Microsoft's Edge WebView2 Runtime, which Windows 11 and an updated Windows 10 have.
;
; To make the setup work on a PC with no internet and no WebView2 either, put Microsoft's
; "MicrosoftEdgeWebview2Setup.exe" (the Evergreen bootstrapper, needs internet) or the standalone
; installer renamed to that name in this folder before compiling: it is then bundled and run only
; when the runtime is missing.

#define AppName "ShopKeeper"
#ifndef AppVersion
  #define AppVersion "1.0.0"
#endif

[Setup]
AppId={{B5E3C1A8-6F24-4D7B-9A10-3C8E52D7F4A6}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher={#AppName}
VersionInfoVersion={#AppVersion}
VersionInfoDescription={#AppName} Setup
; The database and the backups are kept next to the program, so it must be a folder the Windows user
; can write to: not Program Files.
DefaultDirName=C:\ShopKeeper
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
OutputDir=..\dist
OutputBaseFilename=ShopKeeper-Setup
UninstallDisplayName={#AppName}
UninstallDisplayIcon={app}\ShopKeeper.exe
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
; A running ShopKeeper is asked to close (it takes its last backup then) before it is replaced.
CloseApplications=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "russian"; MessagesFile: "compiler:Languages\Russian.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"

[Files]
Source: "..\dist\ShopKeeper.exe"; DestDir: "{app}"; Flags: ignoreversion
#ifexist "MicrosoftEdgeWebview2Setup.exe"
Source: "MicrosoftEdgeWebview2Setup.exe"; DestDir: "{tmp}"; Flags: deleteafterinstall; Check: not WebView2Installed
#endif

[Icons]
Name: "{group}\{#AppName}"; Filename: "{app}\ShopKeeper.exe"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\ShopKeeper.exe"; Tasks: desktopicon

[Run]
#ifexist "MicrosoftEdgeWebview2Setup.exe"
Filename: "{tmp}\MicrosoftEdgeWebview2Setup.exe"; Parameters: "/silent /install"; StatusMsg: "Installing Microsoft Edge WebView2 Runtime..."; Check: not WebView2Installed; Flags: waituntilterminated
#endif
Filename: "{app}\ShopKeeper.exe"; Description: "Start {#AppName} now"; Flags: nowait postinstall skipifsilent

; The uninstaller removes only what this setup installed: shop.db, backups\ and shop.log stay.

[Code]
// Microsoft Edge WebView2 Runtime registers its version under this client id, machine-wide or per user.
function WebView2Installed: Boolean;
var
  Version: String;
begin
  Result :=
    (RegQueryStringValue(HKLM32, 'SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}', 'pv', Version) and (Version <> '') and (Version <> '0.0.0.0')) or
    (RegQueryStringValue(HKLM64, 'SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}', 'pv', Version) and (Version <> '') and (Version <> '0.0.0.0')) or
    (RegQueryStringValue(HKCU, 'Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}', 'pv', Version) and (Version <> '') and (Version <> '0.0.0.0'));
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if (CurStep = ssPostInstall) and (not WebView2Installed) then
    MsgBox('ShopKeeper needs "Microsoft Edge WebView2 Runtime" to show its window, and this PC does not have it yet.' + #13#10 + #13#10 +
           'Download and install it once (free): https://go.microsoft.com/fwlink/p/?LinkId=2124703', mbInformation, MB_OK);
end;
