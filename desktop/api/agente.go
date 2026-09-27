package api

// agente.go — POST /api/agente/terminal: abre una terminal del sistema
// en el escritorio del agente (ver workspace.PrepararWorkspace), lista
// para que el usuario escriba claude/opencode/gemini a mano. Bythos
// NUNCA lanza ese CLI, solo la terminal (ver desktop/terminal/).
//
// Más estricto que conGuardia a propósito: conGuardia deja pasar a la
// extensión (prefijo chrome-extension://) porque hay rutas legítimas que
// necesita (guardar links). Esta ruta LANZA UN PROCESO en la PC del
// usuario: la extensión jamás debe poder pedir eso, así que el Origin
// tiene que ser EXACTO a la ventana de Bythos, nunca un prefijo ni "sin
// Origin" (a diferencia del resto de la API, que sí deja pasar peticiones
// same-origin sin esa cabecera).
//
// localhost:5173 (Vite dev) se acepta solo con BYTHOS_DEV=1, igual que en
// conGuardia (ver modoDev en server.go).

import (
	"net/http"
	"os"
	"sync"
	"time"

	"bythos-desktop/db"
	"bythos-desktop/terminal"
	"bythos-desktop/workspace"
)

// origenesAgentePermitidos es EXACTO (sin el prefijo chrome-extension://
// que sí acepta origenPermitido para el resto de la API).
var origenesAgentePermitidos = map[string]bool{
	"http://localhost:8080": true, // ventana de Bythos
}

// ventanaAgente es la cortesía anti-doble-click: como mucho 1 apertura
// cada 3s. No es la defensa real (Origin exacto ya filtra quién puede
// llamar); sin esto, un doble clic en el botón abriría 2 terminales.
const ventanaAgente = 3 * time.Second

// agenteMu + agenteUltimo implementan esa ventana: 1 mutex, sin tabla ni
// archivo. Se resetea si reinicias Bythos, a propósito: no hace falta
// persistir un cooldown de 3 segundos.
var (
	agenteMu     sync.Mutex
	agenteUltimo time.Time
)

// lanzadorAgente es var (no una llamada directa a terminal.NuevoLanzador
// en el handler) para que los tests de este paquete inyecten un Lanzador
// falso y jamás abran una terminal de verdad. Ver TestAbrirTerminalAgente*.
var lanzadorAgente terminal.Lanzador = terminal.NuevoLanzador()

// prepararWorkspaceAgente es var por la misma razón: los tests de api/
// no deben tocar el %APPDATA% real de quien corre `go test`.
var prepararWorkspaceAgente = workspace.PrepararWorkspace

// exeActual es var para poder simular un fallo de os.Executable en tests
// sin depender de condiciones del SO que casi nunca fallan de verdad.
var exeActual = os.Executable

func (s *Servidor) abrirTerminalAgente(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if !origenesAgentePermitidos[origin] && !(modoDev() && origin == origenVite) {
		responderError(w, http.StatusForbidden, "Esta acción solo se puede pedir desde la ventana de Bythos")
		return
	}

	agenteMu.Lock()
	espera := agenteUltimo.Add(ventanaAgente).Sub(time.Now())
	if espera > 0 {
		agenteMu.Unlock()
		responderError(w, http.StatusTooManyRequests, "Esperá unos segundos antes de abrir otra terminal")
		return
	}
	agenteUltimo = time.Now()
	agenteMu.Unlock()

	exe, err := exeActual()
	if err != nil {
		responderError(w, http.StatusInternalServerError, "No se pudo ubicar bythos.exe")
		return
	}

	dir, err := prepararWorkspaceAgente(exe)
	if err != nil {
		responderError(w, http.StatusInternalServerError, "No se pudo preparar la carpeta del agente")
		return
	}

	terminalUsada, err := lanzadorAgente.Abrir(dir)
	if err != nil {
		responderError(w, http.StatusInternalServerError, "No se pudo abrir la terminal")
		return
	}

	s.registrarEvento(r, db.AccionTerminalAgenteAbierta, 0, terminalUsada+" · "+dir)
	responder(w, map[string]any{"ok": true, "terminal": terminalUsada, "workspace": dir})
}
