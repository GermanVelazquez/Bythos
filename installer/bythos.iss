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
; - Per-machine install ({autopf}\Bythos, PrivilegesRequired=admin): needed
;   to add the firewall rule for phone reception (Private profile + local
;   subnet only; removed on uninstall). A previous per-user (HKCU) install
;   is silently uninstalled in InitializeSetup; %APPDATA% is preserved.
;   {userdesktop} (never {commondesktop}) resolves the real Desktop incl.
;   OneDrive redirects (under over-the-shoulder elevation it is the admin's).
; - Output goes to installer\Output (git-ignored); release copies the
;   compiled setup elsewhere, never into the repo.

#define AppVersion "1.1.0"

[Setup]
AppId={{AEEA0123-59D0-4FFD-89D0-13AA666C70C4}}
AppName=Bythos
; Mismo nombre que el mutex de main.go (Local\Bythos.Instancia): el instalador
; y el desinstalador piden cerrar Bythos si está abierto.
AppMutex=Bythos.Instancia
AppVersion={#AppVersion}
AppVerName=Bythos {#AppVersion}
UninstallDisplayName=Bythos
VersionInfoVersion={#AppVersion}
DefaultDirName={autopf}\Bythos
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64compatible
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
Source: "{#SourcePath}\..\extension\background.js"; DestDir: "{userdesktop}\Bythos-Extension"; Flags: ignoreversion restartreplace uninsneveruninstall
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

[Run]
; Regla de firewall para la recepción desde el celular: SOLO perfil privado y
; SOLO la subred local; nunca se crea una regla para perfil público o de
; dominio. Primero se borra (idempotente en reinstalaciones) y luego se agrega.
; La app nunca modifica el firewall: lo hace únicamente este instalador.
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall delete rule name=""Bythos (celular)"""; Flags: runhidden; StatusMsg: "Configurando el firewall..."
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall add rule name=""Bythos (celular)"" dir=in action=allow program=""{app}\bythos.exe"" protocol=TCP localport=48080 profile=private remoteip=localsubnet"; Flags: runhidden; StatusMsg: "Configurando el firewall..."
; runasoriginaluser: la app se abre con el usuario que lanzó el setup, no con
; la cuenta elevada, para que %APPDATA% apunte a sus datos.
Filename: "{app}\bythos.exe"; Description: "{cm:LaunchProgram,Bythos}"; Flags: nowait postinstall skipifsilent runasoriginaluser

[UninstallRun]
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall delete rule name=""Bythos (celular)"""; Flags: runhidden; RunOnceId: "QuitarReglaFirewall"

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

// Migración per-usuario (HKCU) -> per-máquina. Las versiones anteriores
// se instalaban con PrivilegesRequired=lowest y quedaron registradas en
// HKCU con este mismo AppId. Se desinstalan en silencio ANTES de instalar
// (sin tocar %APPDATA%\Bythos: la base de datos del usuario se conserva).
// /SUPPRESSMSGBOXES hace que la pregunta de borrar la extensión se responda
// con su botón por defecto ("No"), así la carpeta del Escritorio se conserva.
// Si no hay registro previo o la desinstalación falla, la instalación sigue.
function InitializeSetup(): Boolean;
var
  Clave, Cadena: String;
  Codigo: Integer;
begin
  Result := True;
  // AppId={{...}} registra la clave con la llave final doble: "{{" escapa
  // la primera llave, pero "}}" queda literal. La clave real es "...}}_is1".
  Clave := 'Software\Microsoft\Windows\CurrentVersion\Uninstall\{AEEA0123-59D0-4FFD-89D0-13AA666C70C4}}_is1';
  Log('Migración HKCU: usuario=' + GetUserNameString + ', admin=' + IntToStr(Ord(IsAdmin)));
  if RegQueryStringValue(HKCU, Clave, 'UninstallString', Cadena) then
  begin
    Log('Migración HKCU: UninstallString leído=' + Cadena);
    Cadena := RemoveQuotes(Trim(Cadena));
    if (Cadena <> '') and FileExists(Cadena) then
    begin
      if Exec(Cadena, '/VERYSILENT /SUPPRESSMSGBOXES /NORESTART', '', SW_HIDE, ewWaitUntilTerminated, Codigo) then
        Log('Migración HKCU: desinstalador terminó con código=' + IntToStr(Codigo))
      else
        Log('Migración HKCU: no se pudo ejecutar el desinstalador, error=' + IntToStr(Codigo) + ' (' + SysErrorMessage(Codigo) + ')');
      if RegValueExists(HKCU, Clave, 'UninstallString') then
        Log('Migración HKCU: la entrada sigue en HKCU después de desinstalar')
      else
        Log('Migración HKCU: la entrada ya no está en HKCU');
    end
    else
      Log('Migración HKCU: el desinstalador no existe en la ruta=' + Cadena);
  end
  else
    Log('Migración HKCU: no hay instalación previa por usuario');
end;
