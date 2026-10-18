package api

// terminal.go — ticket de un solo uso + WebSocket del PTY (design D2, D7, D9).
// Orden del upgrade: Host (conGuardia) -> Origin EXACTO (conVentana) -> ticket
// -> websocket.Accept. Un rechazo en Host u Origin nunca toca los tickets.
// El comando es fijo: nada de la query ni del body llega a terminal.Config.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"bythos-desktop/db"
	"bythos-desktop/terminal"
)

const (
	ttlTicket         = 30 * time.Second
	limiteLecturaWS   = 1 << 20
	colsIni, filasIni = 80, 24
	shellPorDefecto   = `C:\Windows\System32\cmd.exe`
	msgSinConPTY      = "La terminal requiere Windows 10 versión 1809 o superior."
)

// aceptarWS es var para que los tests espíen que Accept no se llama si una guarda rechaza.
var aceptarWS = websocket.Accept

type ticket struct {
	sesion int64
	vence  time.Time
}

// almacenTickets: valor cero listo para usar; ahora nil = time.Now.
type almacenTickets struct {
	mu    sync.Mutex
	m     map[string]ticket
	ahora func() time.Time
}

func (a *almacenTickets) hora() time.Time {
	if a.ahora != nil {
		return a.ahora()
	}
	return time.Now()
}

// emitir crea un ticket (16 bytes de crypto/rand) y barre los vencidos.
func (a *almacenTickets) emitir(sesion int64) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	t, ahora := base64.RawURLEncoding.EncodeToString(b), a.hora()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.m == nil {
		a.m = map[string]ticket{}
	}
	for k, v := range a.m {
		if !ahora.Before(v.vence) {
			delete(a.m, k)
		}
	}
	a.m[t] = ticket{sesion, ahora.Add(ttlTicket)}
	return t, nil
}

// consumir borra el ticket pase lo que pase; vale solo si existe, no venció y es de esa sesión.
func (a *almacenTickets) consumir(t string, sesion int64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	v, ok := a.m[t]
	delete(a.m, t)
	return ok && v.sesion == sesion && a.hora().Before(v.vence)
}

// patronesOrigen: defensa en profundidad de Accept, solo host (la autoridad es conVentana).
func patronesOrigen() []string {
	p := []string{"localhost:8080"}
	if modoDev() {
		p = append(p, hostVite)
	}
	return p
}

// rutaShell: %ComSpec% o cmd.exe del sistema.
func rutaShell(getenv func(string) string) string {
	if s := getenv("ComSpec"); s != "" {
		return s
	}
	return shellPorDefecto
}

func (s *Servidor) rutasTerminal(mux *http.ServeMux) {
	const p = "/api/terminal/sesiones/{id}/"
	mux.HandleFunc("POST "+p+"ticket", conVentana(s.pedirTicket))
	mux.HandleFunc("GET "+p+"ws", conVentana(s.wsTerminal))
	mux.HandleFunc("DELETE "+p+"pty", conVentana(s.matarPTY))
}

func (s *Servidor) pedirTicket(w http.ResponseWriter, r *http.Request) {
	id, ok := s.sesionExistente(w, r)
	if !ok {
		return
	}
	t, err := s.tickets.emitir(id)
	if err != nil {
		responderError(w, 500, "No se pudo crear el ticket")
		return
	}
	responder(w, map[string]string{"ticket": t})
}

func (s *Servidor) sesionExistente(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, ok := idSesion(w, r)
	if !ok {
		return 0, false
	}
	if _, existe, err := db.ObtenerSesion(s.Base, id); err != nil {
		responderError(w, 500, "No se pudo leer la sesión")
		return 0, false
	} else if !existe {
		noExiste(w, id)
		return 0, false
	}
	return id, true
}

// configPTY arma el Config con valores fijos: solo el id (numérico) entra al entorno.
func configPTY(id int64) func() (terminal.Config, error) {
	return func() (terminal.Config, error) {
		exe, err := exeActual()
		if err != nil {
			return terminal.Config{}, err
		}
		dir, err := prepararWorkspaceAgente(exe)
		if err != nil {
			return terminal.Config{}, err
		}
		return terminal.Config{
			Comando: rutaShell(os.Getenv),
			Dir:     dir,
			Env:     append(os.Environ(), "BYTHOS_SESION="+strconv.FormatInt(id, 10)),
		}, nil
	}
}

func (s *Servidor) wsTerminal(w http.ResponseWriter, r *http.Request) {
	id, ok := idSesion(w, r)
	if !ok {
		return
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") || r.Header.Get("Sec-WebSocket-Key") == "" {
		responderError(w, http.StatusBadRequest, "Se esperaba un WebSocket")
		return
	}
	if s.Terminal == nil {
		responderError(w, http.StatusServiceUnavailable, "La terminal no está disponible")
		return
	}
	if !s.tickets.consumir(r.URL.Query().Get("ticket"), id) {
		responderError(w, http.StatusForbidden, "Ticket inválido o vencido")
		return
	}
	adj, err := s.Terminal.Adjuntar(id, configPTY(id), colsIni, filasIni)
	switch {
	case errors.Is(err, terminal.ErrLimite):
		responderError(w, http.StatusTooManyRequests, "Hay demasiadas terminales abiertas (máximo 8)")
		return
	case errors.Is(err, terminal.ErrNoSoportado):
		responderError(w, http.StatusNotImplemented, msgSinConPTY)
		return
	case err != nil:
		responderError(w, 500, "No se pudo abrir la terminal")
		return
	}
	c, err := aceptarWS(w, r, &websocket.AcceptOptions{OriginPatterns: patronesOrigen()})
	if err != nil {
		adj.Soltar() // Accept ya respondió
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(limiteLecturaWS)
	bombear(r.Context(), c, adj)
}

// bombear une el WS con el adjunto hasta que cualquiera de los dos termine.
// Al terminar el cliente solo se suelta el adjunto: el PTY sigue vivo (D4).
func bombear(ctx context.Context, c *websocket.Conn, adj *terminal.Adjunto) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer adj.Soltar()
	if len(adj.Repeticion) > 0 && c.Write(ctx, websocket.MessageBinary, adj.Repeticion) != nil {
		return
	}
	cerrado := make(chan struct{})
	go func() {
		defer close(cerrado)
		for {
			tipo, d, err := c.Read(ctx)
			if err != nil {
				return
			}
			if tipo == websocket.MessageBinary {
				adj.Escribir(d)
			} else if cols, filas, ok := leerResize(d); ok {
				adj.Redimensionar(cols, filas)
			}
		}
	}()
	fin := adj.Fin
	for {
		select {
		case b, abierto := <-adj.Salida:
			if !abierto {
				codigo := websocket.StatusNormalClosure
				if m := adj.Motivo(); m != 0 {
					codigo = websocket.StatusCode(m)
				}
				c.Close(codigo, "")
				return
			}
			if c.Write(ctx, websocket.MessageBinary, b) != nil {
				return
			}
		case cod := <-fin:
			fin = nil
			msg, _ := json.Marshal(map[string]interface{}{"tipo": "salida", "codigo": cod})
			if c.Write(ctx, websocket.MessageText, msg) != nil {
				return
			}
		case <-cerrado:
			return
		}
	}
}

// leerResize acota el tamaño a 2..500 x 1..300 (design, frames).
func leerResize(d []byte) (cols, filas uint16, ok bool) {
	var m struct {
		Tipo        string `json:"tipo"`
		Cols, Filas int
	}
	if json.Unmarshal(d, &m) != nil || m.Tipo != "resize" {
		return 0, 0, false
	}
	return uint16(min(max(m.Cols, 2), 500)), uint16(min(max(m.Filas, 1), 300)), true
}

// matarPTY mata el proceso de la sesión y conserva la fila.
func (s *Servidor) matarPTY(w http.ResponseWriter, r *http.Request) {
	id, ok := idSesion(w, r)
	if !ok {
		return
	}
	if s.Terminal != nil {
		if err := s.Terminal.Cerrar(id); err != nil {
			responderError(w, 500, "No se pudo cerrar la terminal de la sesión")
			return
		}
	}
	responder(w, map[string]bool{"ok": true})
}
