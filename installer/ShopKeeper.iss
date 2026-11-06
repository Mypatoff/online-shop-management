; ShopKeeper setup. Build it with installer\build-installer.ps1, or by hand with
;   ISCC.exe /DAppVersion=1.0.0 installer\ShopKeeper.iss
; after building dist\ShopKeeper.exe (build.bat / build.sh). Nothing else has to be installed on the
; shop PC: the program draws its own window (no browser engine needed).

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
SetupIconFile=..\assets\icon.ico
UninstallDisplayIcon={app}\ShopKeeper.ico
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
Source: "..\assets\icon.ico"; DestDir: "{app}"; DestName: "ShopKeeper.ico"; Flags: ignoreversion

[Icons]
Name: "{group}\{#AppName}"; Filename: "{app}\ShopKeeper.exe"; IconFilename: "{app}\ShopKeeper.ico"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\ShopKeeper.exe"; IconFilename: "{app}\ShopKeeper.ico"; Tasks: desktopicon

[Run]
Filename: "{app}\ShopKeeper.exe"; Description: "Start {#AppName} now"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
; Left beside the program by the in-app updater.
Type: files; Name: "{app}\ShopKeeper.exe.old"
Type: files; Name: "{app}\ShopKeeper.exe.new"

; The uninstaller removes only what this setup installed: shop.db, backups\ and shop.log stay.
