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
; - The Chrome extension (extension/ + installer/LEEME-Extension.txt)
;   is installed as loose files in {userdesktop}\Bythos-Extension, so the
;   user loads it with "Load unpacked" and reads LEEME.txt right there.
;   The file list below is EXPLICIT on purpose: a stray .pem/.crx in
;   extension/ is never packaged (exclusion by construction).
; - Per-user install ({localappdata}\Bythos, PrivilegesRequired=lowest),
;   so no admin rights are needed and the uninstaller is registered
;   in the Control Panel for the installing user. {userdesktop} (never
;   {commondesktop}) resolves the real Desktop incl. OneDrive redirects.
; - Output goes to installer\Output (git-ignored); release copies the
;   compiled setup elsewhere, never into the repo.

#define AppVersion "1.0.2"

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

; Extensión para Chrome -> carpeta Bythos-Extension en el Escritorio.
; uninsneveruninstall: el desinstalador NO la borra en silencio; pregunta
; primero (ver InitializeUninstall abajo). restartreplace: si Chrome tiene
; la extensión cargada, los archivos pueden estar bloqueados y el reemplazo
; se reintenta al reiniciar en vez de fallar la instalación.
Source: "{#SourcePath}\..\extension\manifest.json"; DestDir: "{userdesktop}\Bythos-Extension"; Flags: ignoreversion restartreplace uninsneveruninstall
Source: "{#SourcePath}\..\extension\popup.html"; DestDir: "{userdesktop}\Bythos-Extension"; Flags: ignoreversion restartreplace uninsneveruninstall
Source: "{#SourcePath}\..\extension\popup.js"; DestDir: "{userdesktop}\Bythos-Extension"; Flags: ignoreversion restartreplace uninsneveruninstall
Source: "{#SourcePath}\..\extension\content.js"; DestDir: "{userdesktop}\Bythos-Extension"; Flags: ignoreversion restartreplace uninsneveruninstall
Source: "{#SourcePath}\..\extension\icons\icon16.png"; DestDir: "{userdesktop}\Bythos-Extension\icons"; Flags: ignoreversion restartreplace uninsneveruninstall
Source: "{#SourcePath}\..\extension\icons\icon48.png"; DestDir: "{userdesktop}\Bythos-Extension\icons"; Flags: ignoreversion restartreplace uninsneveruninstall
Source: "{#SourcePath}\..\extension\icons\icon128.png"; DestDir: "{userdesktop}\Bythos-Extension\icons"; Flags: ignoreversion restartreplace uninsneveruninstall
Source: "{#SourcePath}\LEEME-Extension.txt"; DestDir: "{userdesktop}\Bythos-Extension"; DestName: "LEEME.txt"; Flags: ignoreversion uninsneveruninstall

[InstallDelete]
; Migración 1.0.2 -> 1.0.3: "Bythos" en el menú inicio era un acceso directo
; suelto y ahora es carpeta (app + ayuda de la extensión). Se borra el suelto
; viejo para no duplicar; si el usuario lo había fijado a la barra, Windows
; conserva su fijado aunque el .lnk original cambie de lugar.
Type: files; Name: "{autoprograms}\Bythos.lnk"

[Icons]
Name: "{autoprograms}\Bythos\Bythos"; Filename: "{app}\bythos.exe"; WorkingDir: "{app}"; IconFilename: "{app}\bythos.exe"
Name: "{autoprograms}\Bythos\Extensión para Chrome"; Filename: "{userdesktop}\Bythos-Extension"
Name: "{autodesktop}\Bythos"; Filename: "{app}\bythos.exe"; WorkingDir: "{app}"; Tasks: desktopicon; IconFilename: "{app}\bythos.exe"

[Code]
// La carpeta del Escritorio se borra SOLO con permiso explícito.
// Decisión documentada: se pregunta en InitializeUninstall con MsgBox en vez
// de una tarea [Tasks] desmarcada, porque:
//  1. Preguntar al desinstalar decide con contexto actual (el usuario pudo
//     haber movido o renombrado la carpeta; si ya no existe, ni se pregunta).
//  2. El mensaje explica QUÉ se borra (carpeta + guía LEEME.txt) y qué pasa
//     si responde "No" (la conserva para reinstalar o cargar a mano).
//  3. DelTree ignora errores puntuales (archivos bloqueados por Chrome con la
//     extensión cargada) devolviendo False en vez de lanzar excepción: el ciclo
//     del desinstalador nunca falla por esto. Su resultado se ignora a propósito.
var
  BorrarExtension: Boolean;

function InitializeUninstall(): Boolean;
begin
  BorrarExtension := False;
  if DirExists(ExpandConstant('{userdesktop}\Bythos-Extension')) then
    BorrarExtension := MsgBox('¿Eliminar también la carpeta de la extensión del Escritorio?' + #13#10 + #13#10 +
      'Bythos-Extension (con su guía LEEME.txt).' + #13#10 +
      'Responde "No" si quieres conservarla para reinstalar o cargarla a mano.',
      mbConfirmation, MB_YESNO or MB_DEFBUTTON2) = IDYES;
  Result := True;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if (CurUninstallStep = usUninstall) and BorrarExtension then
    DelTree(ExpandConstant('{userdesktop}\Bythos-Extension'), True, True, True);
end;
