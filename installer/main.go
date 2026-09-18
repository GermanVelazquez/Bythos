// Command bythos-setup installs Bythos on Windows.
//
// Double click (or `Bythos-Setup.exe /S` for silent) copies the embedded
// bythos.exe into %LocalAppData%\Bythos, creates a "Bythos" shortcut on the
// user's Desktop plus a Start Menu entry, and drops an uninstall.bat next
// to the app. The user database is NEVER shipped: the app creates
// %APPDATA%\Bythos\bythos.db itself on first launch (see db.RutaPorDefecto).
//
// Why a Go installer instead of Inno Setup? The release machine had no
// Inno/NSIS and installing new system software was out of scope, so the
// installer is plain Go with zero dependencies: shortcuts are created
// through a throwaway WSH/VBS script, which resolves the real Desktop
// folder (including OneDrive-redirected Desktops) via SpecialFolders.
//
// Flags (also accept /S and /D=dir installer-style on Windows):
//
//	-dir <path>     install directory (default %LocalAppData%\Bythos)
//	-silent, /S     no popup at the end, stdout only
//	-no-shortcuts   copy files only, skip Desktop/Start Menu links
//	                (used to test the installer against a temp dir)
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// payload is the application binary. It is copied next to this source as
// payload/bythos.exe at release time (see README "Instalador Windows") and
// is deliberately git-ignored: large binaries never go into the repo.
//
//go:embed payload/bythos.exe
var payload []byte

const appName = "Bythos"

// vbsMakeShortcut creates a .lnk through WScript.Shell. location is either
// "Desktop" (a SpecialFolder) or a path relative to the Programs folder in
// the Start Menu. Using SpecialFolders keeps OneDrive-redirected Desktops
// working, where %USERPROFILE%\Desktop would point nowhere.
const vbsMakeShortcut = `Set sh = CreateObject("WScript.Shell")
loc = WScript.Arguments(0)
name = WScript.Arguments(1)
target = WScript.Arguments(2)
workdir = WScript.Arguments(3)
If loc = "Desktop" Then
  dir = sh.SpecialFolders("Desktop")
Else
  dir = sh.SpecialFolders("Programs") & "\" & loc
  Set fso = CreateObject("Scripting.FileSystemObject")
  If Not fso.FolderExists(dir) Then fso.CreateFolder(dir)
End If
Set lnk = sh.CreateShortcut(dir & "\" & name & ".lnk")
lnk.TargetPath = target
lnk.WorkingDirectory = workdir
lnk.Description = "Bythos - tu biblioteca personal, en tu PC"
lnk.IconLocation = target & ",0"
lnk.Save
`

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Bythos Setup error:", err)
		os.Exit(1)
	}
}

func run() error {
	// Accept installer-style /S and /D=... too: on Windows, "/flag" is
	// idiomatic for setups, but Go's flag package only knows "-flag".
	for i, a := range os.Args {
		if len(a) > 1 && a[0] == '/' {
			os.Args[i] = "-" + a[1:]
		}
	}
	dir := flag.String("dir", defaultInstallDir(), "install directory")
	silent := flag.Bool("silent", false, "no popup at the end")
	shortS := flag.Bool("S", false, "alias for -silent")
	noLinks := flag.Bool("no-shortcuts", false, "copy files only, skip shortcuts")
	flag.Parse()
	if *shortS {
		*silent = true
	}
	installDir := *dir

	// 1. Copy the app binary. MkdirAll is a no-op when it already exists,
	// so reinstalling over an old version is just an overwrite.
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("create install dir: %w", err)
	}
	exePath := filepath.Join(installDir, "bythos.exe")
	if err := os.WriteFile(exePath, payload, 0755); err != nil {
		return fmt.Errorf("write bythos.exe: %w", err)
	}

	// 2. Drop an uninstaller next to the app. Paths are baked in at
	// install time so a custom -dir keeps working on removal.
	uninstall := "@echo off\r\n" +
		"taskkill /F /IM bythos.exe 2>nul\r\n" +
		"powershell -NoProfile -Command \"$s=New-Object -ComObject WScript.Shell; " +
		"Remove-Item -LiteralPath ($s.SpecialFolders('Desktop')+'\\Bythos.lnk') -Force -ErrorAction SilentlyContinue; " +
		"Remove-Item -LiteralPath ($s.SpecialFolders('Programs')+'\\Bythos') -Recurse -Force -ErrorAction SilentlyContinue\"\r\n" +
		"rmdir /S /Q \"" + installDir + "\"\r\n"
	if err := os.WriteFile(filepath.Join(installDir, "uninstall.bat"), []byte(uninstall), 0755); err != nil {
		return fmt.Errorf("write uninstall.bat: %w", err)
	}

	// 3. Shortcuts: Desktop "Bythos" + Start Menu "Bythos" (+ uninstall entry).
	if !*noLinks {
		if err := makeShortcut("Desktop", appName, exePath, installDir); err != nil {
			return fmt.Errorf("desktop shortcut: %w", err)
		}
		if err := makeShortcut(appName, appName, exePath, installDir); err != nil {
			return fmt.Errorf("start menu shortcut: %w", err)
		}
		if err := makeShortcut(appName, "Desinstalar "+appName,
			filepath.Join(installDir, "uninstall.bat"), installDir); err != nil {
			return fmt.Errorf("uninstall shortcut: %w", err)
		}
	}

	done := "Bythos instalado en " + installDir + ".\nAbrelo desde el acceso directo del Escritorio."
	fmt.Println(done)
	if !*silent && !*noLinks {
		// Best effort popup; a missing cscript never fails the install.
		_ = popup(done)
	}
	return nil
}

// defaultInstallDir is %LocalAppData%\Bythos. LocalAppData (not Program
// Files) needs no admin rights and is always writable, which is also why
// the app database lives in %APPDATA%\Bythos instead of next to the .exe.
func defaultInstallDir() string {
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		return filepath.Join(local, appName)
	}
	return filepath.Join(".", appName)
}

// makeShortcut writes the VBS helper to a temp file, runs it with cscript,
// and removes it. cscript (not wscript) keeps the run console-friendly.
func makeShortcut(location, name, target, workdir string) error {
	vbs := filepath.Join(os.TempDir(), "bythos-shortcut.vbs")
	if err := os.WriteFile(vbs, []byte(vbsMakeShortcut), 0600); err != nil {
		return err
	}
	defer os.Remove(vbs)
	cmd := exec.Command("cscript", "//Nologo", vbs, location, name, target, workdir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cscript: %w (%s)", err, string(out))
	}
	return nil
}

// popup shows a final message box through a throwaway VBS.
func popup(msg string) error {
	vbs := filepath.Join(os.TempDir(), "bythos-done.vbs")
	script := "MsgBox \"" + vbsEscape(msg) + "\", 64, \"Bythos\""
	if err := os.WriteFile(vbs, []byte(script), 0600); err != nil {
		return err
	}
	defer os.Remove(vbs)
	return exec.Command("wscript", vbs).Run()
}

func vbsEscape(s string) string {
	out := ""
	for _, r := range s {
		switch r {
		case '"':
			out += "\"\""
		case '\n':
			out += "\" & vbCrLf & \""
		default:
			out += string(r)
		}
	}
	return out
}
