// Package terminal abre una terminal del sistema en el workspace del
// agente (ver workspace.PrepararWorkspace), lista para que el usuario
// escriba claude/opencode/gemini a mano. Bythos NUNCA lanza esos CLIs:
// solo abre la terminal, el usuario decide qué escribir.
//
// ¿Por qué paquete propio y no código directo en api/agente.go? Porque
// api/ tiene que quedar libre de código atado al SO (exec.Command,
// SysProcAttr de Windows): así sus tests siguen siendo puro HTTP+SQLite,
// sin depender de que haya una terminal real para correr. api/ inyecta
// un Lanzador falso (interfaz de abajo) en sus tests.
//
// Seguridad: exec.Command SIEMPRE con args fijos (el ejecutable y "mcp"
// literal, o "-d"/dir para wt.exe); dir es la carpeta que calculó Go
// (workspace.RutaWorkspace()), nunca algo que viaje por HTTP o de un
// usuario. Nada de fmt.Sprintf armando el comando como texto: eso es
// justo la clase de bug que abre una inyección de comandos.
package terminal

import (
	"os/exec"
)

// Lanzador abre una terminal en dir y NO espera a que termine (Start, no
// Run): si esperáramos, colgaría el handler HTTP mientras el usuario
// tiene la terminal abierta. Devuelve qué terminal se usó (para el
// historial, ver api/agente.go).
type Lanzador interface {
	Abrir(dir string) (terminalUsada string, err error)
}

// lanzadorSistema es el Lanzador real.
type lanzadorSistema struct{}

// NuevoLanzador construye el lanzador real (Windows Terminal si está
// instalado, si no cmd.exe).
func NuevoLanzador() Lanzador { return lanzadorSistema{} }

// Abrir intenta Windows Terminal primero (respeta el tema/perfil del
// usuario y no viene con Windows 10, pero sí suele venir con Windows 11);
// si no está en PATH o falla al arrancar, cae a cmd.exe, que existe en
// cualquier Windows. El cmd.exe de fallback muestra LEEME.txt (creado por
// workspace.PrepararWorkspace) con /k: se queda abierto con la bienvenida
// arriba en vez de cerrarse solo.
func (lanzadorSistema) Abrir(dir string) (string, error) {
	if wt, err := exec.LookPath("wt.exe"); err == nil {
		cmd := exec.Command(wt, "-d", dir)
		if err := cmd.Start(); err == nil {
			return "Windows Terminal", nil
		}
		// wt.exe está pero no pudo arrancar (perfil roto, etc.): seguir
		// al fallback en vez de devolver error, cmd.exe casi nunca falla.
	}

	cmd := exec.Command("cmd.exe", "/k", "type", "LEEME.txt")
	cmd.Dir = dir
	cmd.SysProcAttr = sysProcAttrConsola()
	if err := cmd.Start(); err != nil {
		return "", err
	}
	return "cmd.exe", nil
}
