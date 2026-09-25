package api

// guard_test.go — conGuardia (Host+Origin) en desktop/api/server.go.
// Sin esto el server confiaba en CORS "*"; ahora rechaza Host ajeno y
// Origin ajeno ANTES de tocar la DB (defiende contra CSRF y DNS rebinding).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGuardiaHostNoPermitido(t *testing.T) {
	s := basePrueba(t)
	req := httptest.NewRequest(http.MethodGet, "/api/carpetas", nil)
	req.Host = "evil.com:8080" // ej. DNS rebinding a 127.0.0.1
	rec := httptest.NewRecorder()
	s.Rutas().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("Host ajeno debía ser 403, salió %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp["error"] == "" {
		t.Fatalf("403 debe traer {error} en español, salió: %s", rec.Body.String())
	}
}

func TestGuardiaHostPermitidoSinOrigin(t *testing.T) {
	// curl, esperarSalud, navegación same-origin: sin Origin, pasa.
	s := basePrueba(t)
	req := httptest.NewRequest(http.MethodGet, "/api/carpetas", nil)
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()
	s.Rutas().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Host permitido sin Origin debía pasar, salió %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("sin Origin no debe haber ACAO, salió: %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestGuardiaOrigenAjenoRechazado(t *testing.T) {
	casos := []struct {
		nombre string
		method string
		target string
		ctype  string
		body   string
	}{
		{"GET simple", http.MethodGet, "/api/carpetas", "", ""},
		{"POST text/plain con JSON (salta preflight)", http.MethodPost, "/api/carpetas", "text/plain", `{"nombre":"X"}`},
		{"OPTIONS preflight", http.MethodOptions, "/api/carpetas", "", ""},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			s := basePrueba(t)
			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			} else {
				req = httptest.NewRequest(tt.method, tt.target, nil)
			}
			req.Host = "localhost:8080"
			req.Header.Set("Origin", "https://evil.example")
			if tt.ctype != "" {
				req.Header.Set("Content-Type", tt.ctype)
			}
			rec := httptest.NewRecorder()
			s.Rutas().ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("Origin ajeno debía ser 403, salió %d: %s", rec.Code, rec.Body.String())
			}
			if rec.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatalf("Origin ajeno no debe traer ACAO, salió: %q", rec.Header().Get("Access-Control-Allow-Origin"))
			}
		})
	}
}

func TestGuardiaOrigenesPermitidos(t *testing.T) {
	origenes := []string{
		"http://localhost:8080",      // ventana embebida
		"chrome-extension://abcdefg", // extensión (id no estable en unpacked)
		"http://localhost:5173",      // Vite dev
	}
	for _, origen := range origenes {
		t.Run(origen, func(t *testing.T) {
			s := basePrueba(t)
			req := httptest.NewRequest(http.MethodGet, "/api/carpetas", nil)
			req.Host = "localhost:8080"
			req.Header.Set("Origin", origen)
			rec := httptest.NewRecorder()
			s.Rutas().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("Origin permitido debía pasar, salió %d: %s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origen {
				t.Fatalf("ACAO debía eco exacto %q, salió %q", origen, got)
			}
			if rec.Header().Get("Vary") != "Origin" {
				t.Fatalf("faltó Vary: Origin, salió %q", rec.Header().Get("Vary"))
			}
		})
	}
}
