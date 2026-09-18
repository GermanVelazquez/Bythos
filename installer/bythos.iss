; Bythos Windows setup built with Inno Setup 6.
;
; Wizard flow (Spanish): welcome -> license -> destination folder ->
; desktop-icon task -> ready -> install -> finish.
;
; Layout notes:
; - The app binary is built fresh before compiling this script
;   (see README "Instalador Windows"): npm run build in desktop/ui,
;   then go build in desktop/. Only bythos.exe is packaged.
; - The user database is NEVER packaged: the app creates
;   %APPDATA%\Bythos\bythos.db itself on first launch.
; - Per-user install ({localappdata}\Bythos, PrivilegesRequired=lowest),
;   so no admin rights are needed and the uninstaller is registered
;   in the Control Panel for the installing user.
; - Output goes to installer\Output (git-ignored); release copies the
;   compiled setup elsewhere, never into the repo.

#define AppVersion "1.0.1"

[Setup]
AppId={{AEEA0123-59D0-4FFD-89D0-13AA666C70C4}}
AppName=Bythos
AppVersion={#AppVersion}
AppVerName=Bythos {#AppVersion}
UninstallDisplayName=Bythos
VersionInfoVersion={#AppVersion}
DefaultDirName={localappdata}\Bythos
PrivilegesRequired=lowest
DisableProgramGroupPage=yes
LicenseFile={#SourcePath}\..\LICENSE
OutputDir={#SourcePath}\Output
OutputBaseFilename=Bythos-Setup
SolidCompression=yes
WizardStyle=modern
ArchitecturesAllowed=x64compatible
; Icono del setup y del Panel de control: el .ico fuente vive en assets/.
; Los accesos directos usan el icono EMBEBIDO del .exe (IconFilename abajo),
; así no se instala ningún archivo extra de icono junto a la app.
SetupIconFile={#SourcePath}\..\assets\bythos.ico
UninstallDisplayIcon={app}\bythos.exe

[Languages]
Name: "spanish"; MessagesFile: "compiler:Languages\Spanish.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"

[Files]
Source: "{#SourcePath}\..\desktop\bythos.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\Bythos"; Filename: "{app}\bythos.exe"; WorkingDir: "{app}"; IconFilename: "{app}\bythos.exe"
Name: "{autodesktop}\Bythos"; Filename: "{app}\bythos.exe"; WorkingDir: "{app}"; Tasks: desktopicon; IconFilename: "{app}\bythos.exe"
