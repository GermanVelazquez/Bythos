package api

// agente_test.go — POST /api/agente/terminal sin abrir una terminal real:
// lanzadorAgente y prepararWorkspaceAgente se reemplazan por dobles de
// prueba (ver agente.go). basePrueba viene de avance_test.go.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"bythos-desktop/db"
)

// lanzadorFalso cuenta cuántas veces se llamó Abrir y nunca toca el SO.
type lanzadorFalso struct {
	mu          sync.Mutex
	llamadas    int
	dirRecibido string
	terminal    string
	err         error
}

func (l *lanzadorFalso) Abrir(dir string) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.llamadas++
	l.dirRecibido = dir
	if l.err != nil {
		return "", l.err
	}
	if l.terminal == "" {
		return "cmd.exe (prueba)", nil
	}
	return l.terminal, nil
}

func (l *lanzadorFalso) contador() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.llamadas
}

// conDoblesDeAgente reemplaza lanzadorAgente/prepararWorkspaceAgente/
// exeActual por dobles de prueba y los restaura al terminar el test, así
// ningún test de este archivo abre una terminal ni toca el %APPDATA% real.
func conDoblesDeAgente(t *testing.T, lanzador *lanzadorFalso) {
	t.Helper()
	lanzadorOriginal := lanzadorAgente
	prepararOriginal := prepararWorkspaceAgente
	exeOriginal := exeActual
	t.Cleanup(func() {
		lanzadorAgente = lanzadorOriginal
		prepararWorkspaceAgente = prepararOriginal
		exeActual = exeOriginal
		agenteMu.Lock()
		agenteUltimo = time.Time{}
		agenteMu.Unlock()
	})
	lanzadorAgente = lanzador
	prepararWorkspaceAgente = func(exe string) (string, error) {
		return t.TempDir(), nil
	}
	exeActual = func() (string, error) { return `C:\bythos.exe`, nil }
	agenteMu.Lock()
	agenteUltimo = time.Time{}
	agenteMu.Unlock()
}

func peticionAgente(origin string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/agente/terminal", nil)
	req.Host = "localhost:8080"
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	return req
}

func TestAbrirTerminalAgenteRechazaOrigenesNoPermitidos(t *testing.T) {
	casos := []struct {
		nombre string
		origin string
	}{
		{"extensión", "chrome-extension://abcdefg"},
		{"sin Origin", ""},
		{"origen ajeno", "https://evil.com"},
		{"http en vez de https con dominio ajeno", "http://evil.com:8080"},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			s := basePrueba(t)
			lanzador := &lanzadorFalso{}
			conDoblesDeAgente(t, lanzador)

			rec := httptest.NewRecorder()
			s.Rutas().ServeHTTP(rec, peticionAgente(tt.origin))

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status=%d, quería 403. body=%s", rec.Code, rec.Body.String())
			}
			if lanzador.contador() != 0 {
				t.Fatalf("no debía abrirse ninguna terminal, se abrieron %d", lanzador.contador())
			}
		})
	}
}

func TestAbrirTerminalAgenteOkConOrigenDeLaVentana(t *testing.T) {
	for _, origin := range []string{"http://localhost:8080", "http://localhost:5173"} {
		t.Run(origin, func(t *testing.T) {
			t.Setenv("BYTHOS_DEV", "1") // :5173 solo vale en modo dev
			s := basePrueba(t)
			lanzador := &lanzadorFalso{terminal: "Windows Terminal"}
			conDoblesDeAgente(t, lanzador)

			rec := httptest.NewRecorder()
			s.Rutas().ServeHTTP(rec, peticionAgente(origin))

			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d, quería 200. body=%s", rec.Code, rec.Body.String())
			}
			if lanzador.contador() != 1 {
				t.Fatalf("quería exactamente 1 apertura, hubo %d", lanzador.contador())
			}

			var resp map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("respuesta no JSON: %v", err)
			}
			if resp["terminal"] != "Windows Terminal" {
				t.Fatalf("terminal = %v, quería Windows Terminal", resp["terminal"])
			}

			eventos, err := db.ListarEventos(s.Base, db.FiltroEventos{})
			if err != nil || len(eventos) != 1 {
				t.Fatalf("listar eventos: %+v err=%v", eventos, err)
			}
			if eventos[0].Accion != db.AccionTerminalAgenteAbierta {
				t.Fatalf("accion = %q, quería %q", eventos[0].Accion, db.AccionTerminalAgenteAbierta)
			}
			if !strings.Contains(eventos[0].Detalle, "Windows Terminal") {
				t.Fatalf("detalle debía nombrar la terminal usada, salió: %q", eventos[0].Detalle)
			}
		})
	}
}

func TestAbrirTerminalAgenteRateLimit429EnLaSegundaLlamadaInmediata(t *testing.T) {
	s := basePrueba(t)
	lanzador := &lanzadorFalso{}
	conDoblesDeAgente(t, lanzador)

	rec1 := httptest.NewRecorder()
	s.Rutas().ServeHTTP(rec1, peticionAgente("http://localhost:8080"))
	if rec1.Code != http.StatusOK {
		t.Fatalf("primera llamada: status=%d body=%s", rec1.Code, rec1.Body.String())
	}

	rec2 := httptest.NewRecorder()
	s.Rutas().ServeHTTP(rec2, peticionAgente("http://localhost:8080"))
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("segunda llamada inmediata: status=%d, quería 429. body=%s", rec2.Code, rec2.Body.String())
	}

	if lanzador.contador() != 1 {
		t.Fatalf("la terminal debía abrirse una sola vez, se abrió %d veces", lanzador.contador())
	}
	eventos, err := db.ListarEventos(s.Base, db.FiltroEventos{})
	if err != nil || len(eventos) != 1 {
		t.Fatalf("el intento bloqueado por rate limit no debía registrar evento: %+v err=%v", eventos, err)
	}
}

func TestAbrirTerminalAgenteHeadersAntiClickjacking(t *testing.T) {
	s := basePrueba(t)
	req := httptest.NewRequest(http.MethodGet, "/api/salud", nil)
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()
	s.Rutas().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, quería 200", rec.Code)
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, quería DENY", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != "frame-ancestors 'none'" {
		t.Fatalf("Content-Security-Policy = %q, quería frame-ancestors 'none'", got)
	}
}
