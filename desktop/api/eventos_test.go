package api

// eventos_test.go — El historial visto desde la API: origen leído de
// X-Bythos-Origen (con fallback a desconocido), detalle antes → después
// en progreso/estado, y GET /api/eventos. basePrueba viene de avance_test.go.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bythos-desktop/db"
)

func TestCrearCarpetaRegistraEventoConOrigenDeCabecera(t *testing.T) {
	s := basePrueba(t)
	body := strings.NewReader(`{"nombre":"React"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/carpetas", body)
	req.Host = "localhost:8080"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Bythos-Origen", "agente")
	req.Header.Set("X-Bythos-Actor", "claude-code")
	rec := httptest.NewRecorder()
	s.Rutas().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("crear carpeta: status=%d body=%s", rec.Code, rec.Body.String())
	}

	eventos, err := db.ListarEventos(s.Base, db.FiltroEventos{})
	if err != nil || len(eventos) != 1 {
		t.Fatalf("listar eventos mal: %+v err=%v", eventos, err)
	}
	e := eventos[0]
	if e.Origen != db.OrigenAgente {
		t.Fatalf("origen mal: %q, quería %q", e.Origen, db.OrigenAgente)
	}
	if e.Actor != "claude-code" {
		t.Fatalf("actor mal: %q", e.Actor)
	}
	if e.Accion != db.AccionCarpetaCreada {
		t.Fatalf("accion mal: %q", e.Accion)
	}
	if e.Detalle != "React" {
		t.Fatalf("detalle mal: %q", e.Detalle)
	}
}

func TestOrigenAjenoOSinCabeceraCaeADesconocido(t *testing.T) {
	casos := []struct {
		nombre string
		origen string // "" = sin cabecera
	}{
		{"sin cabecera", ""},
		{"valor inventado", "quien-sabe"},
		{"valor de otra API", "web"},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			s := basePrueba(t)
			req := httptest.NewRequest(http.MethodPost, "/api/carpetas", strings.NewReader(`{"nombre":"X"}`))
			req.Host = "localhost:8080"
			req.Header.Set("Content-Type", "application/json")
			if tt.origen != "" {
				req.Header.Set("X-Bythos-Origen", tt.origen)
			}
			rec := httptest.NewRecorder()
			s.Rutas().ServeHTTP(rec, req)
			if rec.Code != http.StatusCreated {
				t.Fatalf("crear carpeta: status=%d body=%s", rec.Code, rec.Body.String())
			}
			eventos, err := db.ListarEventos(s.Base, db.FiltroEventos{})
			if err != nil || len(eventos) != 1 {
				t.Fatalf("listar eventos mal: %+v err=%v", eventos, err)
			}
			if eventos[0].Origen != db.OrigenDesconocido {
				t.Fatalf("origen debía caer a desconocido, salió %q", eventos[0].Origen)
			}
		})
	}
}

func TestCambiarEstadoRegistraDetalleAntesDespues(t *testing.T) {
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	r, _ := db.Guardar(s.Base, c.ID, "http://a/1", "Hooks", "", "", "articulo")

	// Progreso: pendiente (0%) -> 80%.
	req := httptest.NewRequest(http.MethodPatch, "/api/recursos/"+itoa(r.ID), strings.NewReader(`{"progreso":80}`))
	req.Host = "localhost:8080"
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", itoa(r.ID))
	rec := httptest.NewRecorder()
	s.cambiarEstado(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cambiarEstado: status=%d body=%s", rec.Code, rec.Body.String())
	}

	eventos, err := db.ListarEventos(s.Base, db.FiltroEventos{})
	if err != nil || len(eventos) != 1 {
		t.Fatalf("listar eventos mal: %+v err=%v", eventos, err)
	}
	e := eventos[0]
	if e.Accion != db.AccionProgresoCambiado {
		t.Fatalf("accion mal: %q", e.Accion)
	}
	if !strings.Contains(e.Detalle, "Hooks") || !strings.Contains(e.Detalle, "0%") || !strings.Contains(e.Detalle, "80%") {
		t.Fatalf("detalle debía traer título + antes + después, salió: %q", e.Detalle)
	}

	// Estado: en_curso (por el progreso recién puesto) -> completado.
	req2 := httptest.NewRequest(http.MethodPatch, "/api/recursos/"+itoa(r.ID), strings.NewReader(`{"estado":"completado"}`))
	req2.Host = "localhost:8080"
	req2.Header.Set("Content-Type", "application/json")
	req2.SetPathValue("id", itoa(r.ID))
	rec2 := httptest.NewRecorder()
	s.cambiarEstado(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("cambiarEstado (estado): status=%d body=%s", rec2.Code, rec2.Body.String())
	}
	eventos2, err := db.ListarEventos(s.Base, db.FiltroEventos{})
	if err != nil || len(eventos2) != 2 {
		t.Fatalf("listar eventos mal tras 2do cambio: %+v err=%v", eventos2, err)
	}
	ultimo := eventos2[0] // más nuevo primero
	if ultimo.Accion != db.AccionEstadoCambiado {
		t.Fatalf("accion mal: %q", ultimo.Accion)
	}
	if !strings.Contains(ultimo.Detalle, "en_curso") || !strings.Contains(ultimo.Detalle, "completado") {
		t.Fatalf("detalle debía traer estado antes y después, salió: %q", ultimo.Detalle)
	}
}

func TestBorrarRecursoYCarpetaRegistranEvento(t *testing.T) {
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	r, _ := db.Guardar(s.Base, c.ID, "http://a/1", "Hooks", "", "", "articulo")

	req := httptest.NewRequest(http.MethodDelete, "/api/recursos/"+itoa(r.ID), nil)
	req.SetPathValue("id", itoa(r.ID))
	rec := httptest.NewRecorder()
	s.borrarRecurso(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("borrarRecurso: status=%d body=%s", rec.Code, rec.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodDelete, "/api/carpetas/"+itoa(c.ID), nil)
	req2.SetPathValue("id", itoa(c.ID))
	rec2 := httptest.NewRecorder()
	s.borrarCarpeta(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("borrarCarpeta: status=%d body=%s", rec2.Code, rec2.Body.String())
	}

	eventos, err := db.ListarEventos(s.Base, db.FiltroEventos{})
	if err != nil || len(eventos) != 2 {
		t.Fatalf("listar eventos mal: %+v err=%v", eventos, err)
	}
	// Más nuevo primero: la carpeta se borró después del recurso.
	if eventos[0].Accion != db.AccionCarpetaBorrada || eventos[0].Detalle != "React" {
		t.Fatalf("evento carpeta mal: %+v", eventos[0])
	}
	if eventos[1].Accion != db.AccionRecursoBorrado || eventos[1].Detalle != "Hooks" {
		t.Fatalf("evento recurso mal: %+v", eventos[1])
	}
}

func TestGetEventosHandler(t *testing.T) {
	s := basePrueba(t)
	if err := db.RegistrarEvento(s.Base, db.OrigenApp, "", db.AccionCarpetaCreada, 1, "Uno"); err != nil {
		t.Fatalf("semilla: %v", err)
	}
	if err := db.RegistrarEvento(s.Base, db.OrigenAgente, "claude-code", db.AccionRecursoGuardado, 2, "Dos"); err != nil {
		t.Fatalf("semilla: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/eventos", nil)
	rec := httptest.NewRecorder()
	s.listarEventos(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("respuesta no JSON: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("quería 2 eventos, salieron %d: %v", len(resp), resp)
	}
	// Claves en minúscula (a diferencia de carpetas/recursos/agenda).
	if _, ok := resp[0]["detalle"]; !ok {
		t.Fatalf("clave 'detalle' faltante, salió: %v", resp[0])
	}
	if _, ok := resp[0]["entidad_id"]; !ok {
		t.Fatalf("clave 'entidad_id' faltante, salió: %v", resp[0])
	}

	// Filtro por origen.
	req2 := httptest.NewRequest(http.MethodGet, "/api/eventos?origen=agente", nil)
	rec2 := httptest.NewRecorder()
	s.listarEventos(rec2, req2)
	var resp2 []map[string]interface{}
	_ = json.Unmarshal(rec2.Body.Bytes(), &resp2)
	if len(resp2) != 1 || resp2[0]["origen"] != "agente" {
		t.Fatalf("filtro origen mal: %v", resp2)
	}

	// limite=1.
	req3 := httptest.NewRequest(http.MethodGet, "/api/eventos?limite=1", nil)
	rec3 := httptest.NewRecorder()
	s.listarEventos(rec3, req3)
	var resp3 []map[string]interface{}
	_ = json.Unmarshal(rec3.Body.Bytes(), &resp3)
	if len(resp3) != 1 {
		t.Fatalf("limite mal: %v", resp3)
	}
}
