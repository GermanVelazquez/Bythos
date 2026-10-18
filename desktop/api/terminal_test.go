package api

// terminal_test.go — ticket + WebSocket de la Terminal. Primero la matriz de
// guardas (design D9, Threat Matrix); luego los flujos con un PTY falso y un
// gestor real; al final el ConPTY de verdad (se salta con -short).

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"bythos-desktop/db"
	"bythos-desktop/terminal"
)

// ptyFalso: la salida entra por emitir; Read bloquea hasta que llega o se cierra.
type ptyFalso struct {
	sal, fin    chan []byte
	finCodigo   chan int
	once        sync.Once
	mu          sync.Mutex
	entrada     []byte
	cols, filas uint16
	cerrado     bool
}

func nuevoPTYFalso() *ptyFalso {
	return &ptyFalso{sal: make(chan []byte, 64), finCodigo: make(chan int, 1)}
}
func (p *ptyFalso) emitir(b string) { p.sal <- []byte(b) }
func (p *ptyFalso) Read(b []byte) (int, error) {
	d, ok := <-p.sal
	if !ok {
		return 0, io.EOF
	}
	return copy(b, d), nil
}
func (p *ptyFalso) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entrada = append(p.entrada, b...)
	return len(b), nil
}
func (p *ptyFalso) Redimensionar(c, f uint16) error {
	p.mu.Lock()
	p.cols, p.filas = c, f
	p.mu.Unlock()
	return nil
}
func (p *ptyFalso) Fin() <-chan int { return p.finCodigo }
func (p *ptyFalso) Cerrar() error {
	p.once.Do(func() {
		p.mu.Lock()
		p.cerrado = true
		p.mu.Unlock()
		close(p.sal)
		close(p.finCodigo)
	})
	return nil
}
func (p *ptyFalso) leer() (entrada string, cols, filas uint16) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return string(p.entrada), p.cols, p.filas
}

// entornoWS arma un Servidor con gestor real (PTYs falsos), reloj falso y espía de Accept.
type entornoWS struct {
	s       *Servidor
	g       *terminal.Gestor
	ahora   time.Time
	accepts int
	cfgs    []terminal.Config
	ptys    []*ptyFalso
	iniciar func(terminal.Config) (terminal.PTY, error)
	dir     string
}

func nuevoEntornoWS(t *testing.T) *entornoWS {
	t.Helper()
	e := &entornoWS{s: basePrueba(t), ahora: time.Now(), dir: t.TempDir()}
	e.iniciar = func(c terminal.Config) (terminal.PTY, error) {
		p := nuevoPTYFalso()
		e.cfgs, e.ptys = append(e.cfgs, c), append(e.ptys, p)
		return p, nil
	}
	e.g = terminal.NuevoGestor(terminal.Opciones{Iniciar: func(c terminal.Config) (terminal.PTY, error) { return e.iniciar(c) }})
	t.Cleanup(e.g.CerrarTodo)
	e.s.Terminal = e.g
	e.s.tickets.ahora = func() time.Time { return e.ahora }

	aceptarOrig, prepararOrig := aceptarWS, prepararWorkspaceAgente
	aceptarWS = func(w http.ResponseWriter, r *http.Request, o *websocket.AcceptOptions) (*websocket.Conn, error) {
		e.accepts++
		return aceptarOrig(w, r, o)
	}
	prepararWorkspaceAgente = func(string) (string, error) { return e.dir, nil }
	t.Cleanup(func() { aceptarWS, prepararWorkspaceAgente = aceptarOrig, prepararOrig })
	return e
}

func (e *entornoWS) sesion(t *testing.T, nombre string) int64 {
	t.Helper()
	return crearSesionT(t, e.s, `{"nombre":`+strconv.Quote(nombre)+`,"modo":"practica"}`).ID
}

// ticket pide uno por el endpoint real, como lo hace la ventana.
func (e *entornoWS) ticket(t *testing.T, id int64) string {
	t.Helper()
	rec := ventanaT(e.s, "POST", "sesiones/"+strconv.FormatInt(id, 10)+"/ticket", "")
	var r struct{ Ticket string }
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &r) != nil || r.Ticket == "" {
		t.Fatalf("ticket: %d %s", rec.Code, rec.Body.String())
	}
	return r.Ticket
}

// upgrade manda el GET del WS sin red (solo sirve para respuestas que NO suben a WS).
func (e *entornoWS) upgrade(id int64, ticket, host, origen string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/api/terminal/sesiones/"+strconv.FormatInt(id, 10)+"/ws?ticket="+ticket, nil)
	req.Host = host
	if origen != "" {
		req.Header.Set("Origin", origen)
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	rec := httptest.NewRecorder()
	e.s.Rutas().ServeHTTP(rec, req)
	return rec
}

// dial abre un WS de verdad contra un servidor de prueba, haciéndose pasar por la ventana.
func (e *entornoWS) dial(t *testing.T, srv *httptest.Server, id int64, query, host string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	c, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/api/terminal/sesiones/"+strconv.FormatInt(id, 10)+"/ws?"+query,
		&websocket.DialOptions{Host: host, HTTPHeader: http.Header{"Origin": {origenUI}}})
	if c != nil {
		t.Cleanup(func() { c.CloseNow() })
	}
	return c, resp, err
}

func (e *entornoWS) conectar(t *testing.T, srv *httptest.Server, id int64) *websocket.Conn {
	t.Helper()
	c, _, err := e.dial(t, srv, id, "ticket="+e.ticket(t, id), "localhost:8080")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return c
}

func leerBin(t *testing.T, c *websocket.Conn) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tipo, d, err := c.Read(ctx)
	if err != nil || tipo != websocket.MessageBinary {
		t.Fatalf("leer binario: tipo=%v err=%v", tipo, err)
	}
	return string(d)
}

func esperar(t *testing.T, msg string, ok func() bool) {
	t.Helper()
	for fin := time.Now().Add(5 * time.Second); time.Now().Before(fin); time.Sleep(10 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatal("no se cumplió a tiempo: " + msg)
}

// --- 3.1 Matriz de guardas ---

func TestGuardiaOrigenEnTicketYUpgrade(t *testing.T) {
	e := nuevoEntornoWS(t)
	id := e.sesion(t, "uno")
	malos := map[string]string{
		"vacío": "", "extensión": "chrome-extension://abc", "sufijo evil": "http://localhost:8080.evil.com",
		"prefijo": "http://localhost:8080evil", "ajeno": "http://evil.com", "vite sin dev": "http://localhost:5173",
	}
	for nombre, origen := range malos {
		t.Run(nombre, func(t *testing.T) {
			if rec := pedirTerminal(e.s, "POST", "sesiones/"+strconv.FormatInt(id, 10)+"/ticket", "", origen); rec.Code != 403 {
				t.Fatalf("ticket con Origin %q: %d", origen, rec.Code)
			}
			tk, _ := e.s.tickets.emitir(id)
			if rec := e.upgrade(id, tk, "localhost:8080", origen); rec.Code != 403 {
				t.Fatalf("upgrade con Origin %q: %d", origen, rec.Code)
			}
			if e.accepts != 0 || len(e.g.Vivas()) != 0 {
				t.Fatalf("hubo Accept (%d) o PTY (%v) con Origin %q", e.accepts, e.g.Vivas(), origen)
			}
			if !e.s.tickets.consumir(tk, id) {
				t.Fatalf("el pre-check de Origin gastó el ticket (Origin %q)", origen)
			}
		})
	}
}

func TestGuardiaTicketViteSoloEnModoDev(t *testing.T) {
	e := nuevoEntornoWS(t)
	id := e.sesion(t, "uno")
	ruta := "sesiones/" + strconv.FormatInt(id, 10) + "/ticket"
	pedir := func() int {
		req := httptest.NewRequest("POST", "/api/terminal/"+ruta, nil)
		req.Host, req.Header["Origin"] = "localhost:5173", []string{"http://localhost:5173"}
		rec := httptest.NewRecorder()
		e.s.Rutas().ServeHTTP(rec, req)
		return rec.Code
	}
	if c := pedir(); c != 403 {
		t.Fatalf("sin BYTHOS_DEV: %d", c)
	}
	t.Setenv("BYTHOS_DEV", "1")
	if c := pedir(); c != 200 {
		t.Fatalf("con BYTHOS_DEV=1: %d", c)
	}
	if got := patronesOrigen(); len(got) != 2 || got[1] != "localhost:5173" {
		t.Fatalf("patrones en dev: %v", got)
	}
	t.Setenv("BYTHOS_DEV", "")
	if got := patronesOrigen(); len(got) != 1 || got[0] != "localhost:8080" {
		t.Fatalf("patrones sin dev: %v", got)
	}
}

func TestGuardiaHostMaloNoGastaElTicket(t *testing.T) {
	e := nuevoEntornoWS(t)
	id := e.sesion(t, "uno")
	tk := e.ticket(t, id)
	if rec := e.upgrade(id, tk, "evil.com:8080", origenUI); rec.Code != 403 {
		t.Fatalf("Host malo: %d", rec.Code)
	}
	srv := httptest.NewServer(e.s.Rutas())
	defer srv.Close()
	if _, _, err := e.dial(t, srv, id, "ticket="+tk, "localhost:8080"); err != nil {
		t.Fatalf("el mismo ticket debía servir tras el Host malo: %v", err)
	}
}

func TestGuardiaTicketInvalido(t *testing.T) {
	e := nuevoEntornoWS(t)
	a, b := e.sesion(t, "a"), e.sesion(t, "b")
	srv := httptest.NewServer(e.s.Rutas())
	defer srv.Close()

	vencido := e.ticket(t, a)
	e.ahora = e.ahora.Add(31 * time.Second)
	otraSesion := e.ticket(t, b)
	usado := e.ticket(t, a)
	e.conectar2(t, srv, a, usado)

	casos := map[string]struct {
		id int64
		tk string
	}{
		"falta": {a, ""}, "desconocido": {a, "zzz"}, "vencido": {a, vencido},
		"reusado": {a, usado}, "otra sesión": {a, otraSesion},
	}
	for nombre, c := range casos {
		if rec := e.upgrade(c.id, c.tk, "localhost:8080", origenUI); rec.Code != 403 {
			t.Errorf("%s: %d", nombre, rec.Code)
		}
	}
	if e.accepts != 1 { // solo el del ticket bueno de arriba
		t.Fatalf("Accept llamado %d veces, esperaba 1", e.accepts)
	}
}

func (e *entornoWS) conectar2(t *testing.T, srv *httptest.Server, id int64, tk string) {
	t.Helper()
	if _, _, err := e.dial(t, srv, id, "ticket="+tk, "localhost:8080"); err != nil {
		t.Fatalf("dial con ticket bueno: %v", err)
	}
}

// --- 3.2 Flujos ---

func TestWSRepeticionYReemplazo(t *testing.T) {
	e := nuevoEntornoWS(t)
	id := e.sesion(t, "uno")
	srv := httptest.NewServer(e.s.Rutas())
	defer srv.Close()

	c1 := e.conectar(t, srv, id)
	e.ptys[0].emitir("hello")
	if got := leerBin(t, c1); got != "hello" {
		t.Fatalf("salida en vivo: %q", got)
	}
	c2 := e.conectar(t, srv, id)
	if got := leerBin(t, c2); got != "hello" {
		t.Fatalf("repetición al reconectar: %q", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, err := c1.Read(ctx)
	if websocket.CloseStatus(err) != 4000 {
		t.Fatalf("el adjunto reemplazado debía cerrar con 4000: %v", err)
	}
	if len(e.ptys) != 1 {
		t.Fatalf("reconectar no debe crear otro PTY: %d", len(e.ptys))
	}

	c2.Write(ctx, websocket.MessageBinary, []byte("ls\r"))
	esperar(t, "entrada al PTY", func() bool { in, _, _ := e.ptys[0].leer(); return in == "ls\r" })
}

func TestWSRedimensionaConLimites(t *testing.T) {
	e := nuevoEntornoWS(t)
	id := e.sesion(t, "uno")
	srv := httptest.NewServer(e.s.Rutas())
	defer srv.Close()
	c := e.conectar(t, srv, id)
	ctx := context.Background()
	for _, cs := range []struct {
		msg         string
		cols, filas uint16
	}{
		{`{"tipo":"resize","cols":9999,"filas":0}`, 500, 1},
		{`{"tipo":"resize","cols":0,"filas":9999}`, 2, 300},
		{`{"tipo":"resize","cols":120,"filas":40}`, 120, 40},
	} {
		c.Write(ctx, websocket.MessageText, []byte(cs.msg))
		esperar(t, cs.msg, func() bool { _, c2, f := e.ptys[0].leer(); return c2 == cs.cols && f == cs.filas })
	}
}

func TestWSMensajeGrandeCierraPeroElPTYSobrevive(t *testing.T) {
	e := nuevoEntornoWS(t)
	id := e.sesion(t, "uno")
	srv := httptest.NewServer(e.s.Rutas())
	defer srv.Close()
	c := e.conectar(t, srv, id)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c.Write(ctx, websocket.MessageBinary, make([]byte, 1<<20+1))
	if _, _, err := c.Read(ctx); err == nil || websocket.CloseStatus(err) != websocket.StatusMessageTooBig {
		t.Fatalf("esperaba cierre 1009: %v", err)
	}
	if v := e.g.Vivas(); len(v) != 1 || v[0] != id {
		t.Fatalf("el PTY debía sobrevivir: %v", v)
	}
}

func TestWSNovenoPTYEs429(t *testing.T) {
	e := nuevoEntornoWS(t)
	srv := httptest.NewServer(e.s.Rutas())
	defer srv.Close()
	for i := 0; i < 8; i++ {
		e.conectar(t, srv, e.sesion(t, "s"+strconv.Itoa(i)))
	}
	id := e.sesion(t, "novena")
	_, resp, err := e.dial(t, srv, id, "ticket="+e.ticket(t, id), "localhost:8080")
	if err == nil || resp == nil || resp.StatusCode != 429 {
		t.Fatalf("esperaba 429: %v %v", err, resp)
	}
	if len(e.ptys) != 8 {
		t.Fatalf("la novena no debía arrancar nada: %d", len(e.ptys))
	}
}

func TestWSConfigIgnoraQueryYNombre(t *testing.T) {
	t.Setenv("ComSpec", `X:\falso\cmd.exe`)
	e := nuevoEntornoWS(t)
	id := e.sesion(t, "& calc")
	srv := httptest.NewServer(e.s.Rutas())
	defer srv.Close()
	if _, _, err := e.dial(t, srv, id, "ticket="+e.ticket(t, id)+"&comando=calc&cmd=calc&args=%26+calc&dir=C:\\", "localhost:8080"); err != nil {
		t.Fatal(err)
	}
	c := e.cfgs[0]
	idTxt := strconv.FormatInt(id, 10)
	if c.Comando != `X:\falso\cmd.exe` || len(c.Args) != 0 || c.Dir != e.dir {
		t.Fatalf("Config inesperada: %+v", c)
	}
	if ult := c.Env[len(c.Env)-1]; ult != "BYTHOS_SESION="+idTxt {
		t.Fatalf("falta BYTHOS_SESION: %q", ult)
	}
	for _, v := range c.Env {
		if strings.Contains(v, "calc") {
			t.Fatalf("algo del request llegó al entorno: %q", v)
		}
	}
	if rutaShell(func(string) string { return "" }) != `C:\Windows\System32\cmd.exe` {
		t.Fatal("sin ComSpec debe caer a cmd.exe del sistema")
	}
}

func TestWSSinSoporteEs501(t *testing.T) {
	e := nuevoEntornoWS(t)
	e.iniciar = func(terminal.Config) (terminal.PTY, error) { return nil, terminal.ErrNoSoportado }
	id := e.sesion(t, "uno")
	srv := httptest.NewServer(e.s.Rutas())
	defer srv.Close()
	_, resp, err := e.dial(t, srv, id, "ticket="+e.ticket(t, id), "localhost:8080")
	if err == nil || resp == nil || resp.StatusCode != 501 {
		t.Fatalf("esperaba 501: %v %v", err, resp)
	}
	cuerpo, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(cuerpo), "La terminal requiere Windows 10 versión 1809 o superior.") {
		t.Fatalf("mensaje: %s", cuerpo)
	}
}

func TestBorrarSesionYMatarPTY(t *testing.T) {
	e := nuevoEntornoWS(t)
	a, b := e.sesion(t, "a"), e.sesion(t, "b")
	srv := httptest.NewServer(e.s.Rutas())
	defer srv.Close()
	e.conectar(t, srv, a)
	e.conectar(t, srv, b)

	if rec := ventanaT(e.s, "DELETE", "sesiones/"+strconv.FormatInt(a, 10)+"/pty", ""); rec.Code != 200 {
		t.Fatalf("matar pty: %d", rec.Code)
	}
	if _, ok, _ := db.ObtenerSesion(e.s.Base, a); !ok || len(e.g.Vivas()) != 1 || !e.ptys[0].cerrado {
		t.Fatalf("matar el PTY debía conservar la fila: vivas=%v", e.g.Vivas())
	}
	if rec := ventanaT(e.s, "DELETE", "sesiones/"+strconv.FormatInt(b, 10), ""); rec.Code != 200 {
		t.Fatalf("borrar: %d", rec.Code)
	}
	if v := e.g.Vivas(); len(v) != 0 || !e.ptys[1].cerrado {
		t.Fatalf("borrar la sesión debía cerrar su PTY: %v", v)
	}
	if rec := pedirTerminal(e.s, "DELETE", "sesiones/"+strconv.FormatInt(b, 10)+"/pty", "", ""); rec.Code != 403 {
		t.Fatalf("DELETE pty sin Origin debe ser 403: %d", rec.Code)
	}
}

// Seguimiento del verificador de PR2: validaciones del PATCH.
func TestPatchSesionValidaciones(t *testing.T) {
	s := basePrueba(t)
	id := strconv.FormatInt(crearSesionT(t, s, `{"nombre":"x","modo":"practica"}`).ID, 10)
	for nombre, cuerpo := range map[string]string{
		"modo inválido": `{"modo":"nope"}`, "nombre vacío": `{"nombre":"   "}`,
		"carpeta_id string": `{"carpeta_id":"abc"}`, "carpeta_id decimal": `{"carpeta_id":1.5}`,
		"nombre numérico": `{"nombre":5}`,
	} {
		if rec := ventanaT(s, "PATCH", "sesiones/"+id, cuerpo); rec.Code != 400 {
			t.Errorf("%s: esperaba 400, fue %d", nombre, rec.Code)
		}
	}
}

// --- 3.3 ConPTY real ---

func TestTerminalConPTYReal(t *testing.T) {
	if testing.Short() || runtime.GOOS != "windows" {
		t.Skip("ConPTY real: solo Windows y sin -short")
	}
	e := nuevoEntornoWS(t)
	e.g.CerrarTodo()
	e.g = terminal.NuevoGestor(terminal.Opciones{Iniciar: terminal.NuevoPTY})
	t.Cleanup(e.g.CerrarTodo)
	e.s.Terminal = e.g
	id := e.sesion(t, "real")
	srv := httptest.NewServer(e.s.Rutas())
	defer srv.Close()
	c := e.conectar(t, srv, id)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageBinary, []byte("echo hola\r\n")); err != nil {
		t.Fatal(err)
	}
	var salida string
	for strings.Count(salida, "hola") < 2 { // el eco de lo tecleado y la salida del echo
		tipo, d, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("sin salida de cmd.exe (%q): %v", salida, err)
		}
		if tipo == websocket.MessageBinary {
			salida += string(d)
		}
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatal("timeout")
	}
}
