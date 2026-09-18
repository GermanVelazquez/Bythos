package api

// agenda_test.go — Handlers de agenda + actividad por HTTP (httptest, sin red).
// Usa basePrueba/itoa de avance_test.go (mismo package, sin duplicar).
// Table-driven por go-testing skill: cada caso dice su escenario.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bythos-desktop/db"
)

func TestCrearAgendaHandler(t *testing.T) {
	casos := []struct {
		nombre   string
		body     string
		status   int
		quiere   string // fragmento esperado en la respuesta
		noQuiere string
	}{
		{"nota completa con carpeta", `{"fecha":"2026-09-12","texto":"Repaso","hora_inicio":"10:00","hora_fin":"11:30","carpeta_id":0}`, http.StatusCreated, `"ID"`, ""},
		{"nota suelta sin carpeta", `{"fecha":"2026-09-12","texto":"Leer tranqui"}`, http.StatusCreated, `"ID"`, ""},
		{"fecha imposible", `{"fecha":"2026-02-30","texto":"X"}`, http.StatusBadRequest, "Fecha inválida", ""},
		{"sin fecha", `{"texto":"X"}`, http.StatusBadRequest, "fecha", ""},
		{"sin texto ni carpeta", `{"fecha":"2026-09-12"}`, http.StatusBadRequest, "nota o eleg", ""},
		{"fin sin inicio", `{"fecha":"2026-09-12","texto":"X","hora_fin":"11:00"}`, http.StatusBadRequest, "inicio", ""},
		{"fin antes que inicio", `{"fecha":"2026-09-12","texto":"X","hora_inicio":"12:00","hora_fin":"11:00"}`, http.StatusBadRequest, "posterior", ""},
		{"carpeta inexistente", `{"fecha":"2026-09-12","texto":"","carpeta_id":9999}`, http.StatusBadRequest, "carpeta", ""},
		{"json roto", `{fecha:`, http.StatusBadRequest, "JSON", ""},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			s := basePrueba(t)
			// carpeta_id 0 del primer caso se reemplaza por una real:
			// 0 significa "sin carpeta" en db y fallaría con texto... no:
			// el caso 1 manda texto + carpeta real para probar el JOIN.
			body := tt.body
			if strings.Contains(body, `"carpeta_id":0}`) {
				c, _ := db.CrearCarpeta(s.Base, "React")
				body = strings.Replace(body, `"carpeta_id":0}`, `"carpeta_id":`+itoa(c.ID)+`}`, 1)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/agenda", strings.NewReader(body))
			rec := httptest.NewRecorder()
			s.crearAgenda(rec, req)
			if rec.Code != tt.status {
				t.Fatalf("status=%d body=%s, quería %d", rec.Code, rec.Body.String(), tt.status)
			}
			if tt.quiere != "" && !strings.Contains(rec.Body.String(), tt.quiere) {
				t.Fatalf("falta %q en: %s", tt.quiere, rec.Body.String())
			}
			if tt.status >= 400 {
				var resp map[string]string
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp["error"] == "" {
					t.Fatalf("el error debe ser {error} en español, salió: %s", rec.Body.String())
				}
			}
		})
	}
}

func TestAgendaRangoYBorrarHandler(t *testing.T) {
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")

	// POST directo al handler.
	req := httptest.NewRequest(http.MethodPost, "/api/agenda",
		strings.NewReader(`{"fecha":"2026-09-12","texto":"Repaso","hora_inicio":"10:00","carpeta_id":`+itoa(c.ID)+`}`))
	rec := httptest.NewRecorder()
	s.crearAgenda(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("crear: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var creada map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &creada); err != nil {
		t.Fatalf("crear no devolvió JSON: %v", err)
	}

	// GET con rango la trae con el nombre de la carpeta (JOIN).
	reqGet := httptest.NewRequest(http.MethodGet, "/api/agenda?desde=2026-09-01&hasta=2026-09-30", nil)
	recGet := httptest.NewRecorder()
	s.listarAgenda(recGet, reqGet)
	if recGet.Code != http.StatusOK || !strings.Contains(recGet.Body.String(), "Repaso") {
		t.Fatalf("rango mal: status=%d body=%s", recGet.Code, recGet.Body.String())
	}
	if !strings.Contains(recGet.Body.String(), "React") {
		t.Fatalf("falta el nombre de la carpeta en: %s", recGet.Body.String())
	}
	// GET fuera de rango no la trae.
	reqFuera := httptest.NewRequest(http.MethodGet, "/api/agenda?desde=2026-10-01&hasta=2026-10-31", nil)
	recFuera := httptest.NewRecorder()
	s.listarAgenda(recFuera, reqFuera)
	if recFuera.Code != http.StatusOK || strings.Contains(recFuera.Body.String(), "Repaso") {
		t.Fatalf("fuera de rango debió ser vacío: %s", recFuera.Body.String())
	}

	// DELETE la borra; el segundo intento es 404 en español.
	id := itoa(int64(creada["ID"].(float64)))
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/agenda/"+id, nil)
	reqDel.SetPathValue("id", id)
	recDel := httptest.NewRecorder()
	s.borrarAgenda(recDel, reqDel)
	if recDel.Code != http.StatusOK {
		t.Fatalf("borrar: status=%d body=%s", recDel.Code, recDel.Body.String())
	}
	reqDel2 := httptest.NewRequest(http.MethodDelete, "/api/agenda/"+id, nil)
	reqDel2.SetPathValue("id", id)
	recDel2 := httptest.NewRecorder()
	s.borrarAgenda(recDel2, reqDel2)
	if recDel2.Code != http.StatusNotFound {
		t.Fatalf("segundo borrar debió ser 404, salió %d: %s", recDel2.Code, recDel2.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(recDel2.Body.Bytes(), &resp); err != nil || resp["error"] == "" {
		t.Fatalf("404 debe traer {error} en español, salió: %s", recDel2.Body.String())
	}
}

func TestActividadHandler(t *testing.T) {
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	r, _ := db.Guardar(s.Base, c.ID, "http://a/1", "A", "", "", "otro")
	s.Base.Exec(`UPDATE folders SET created_at = '2026-09-10 08:00:00' WHERE id = ?`, c.ID)
	s.Base.Exec(`UPDATE resources SET created_at = '2026-09-10 09:00:00' WHERE id = ?`, r.ID)

	req := httptest.NewRequest(http.MethodGet, "/api/actividad?desde=2026-09-01&hasta=2026-09-30", nil)
	rec := httptest.NewRecorder()
	s.actividad(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	// 10/9 combina 1 carpeta + 1 recurso = 2.
	var got []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("actividad no devolvió arreglo JSON: %v", err)
	}
	visto := false
	for _, x := range got {
		if x["Fecha"] == "2026-09-10" && x["Total"] == float64(2) {
			visto = true
		}
	}
	if !visto {
		t.Fatalf("falta {2026-09-10, 2} en: %s", rec.Body.String())
	}
}

func TestAgendaRutasWire(t *testing.T) {
	s := basePrueba(t)

	// POST por el mux real (sin :8080, sin red).
	req := httptest.NewRequest(http.MethodPost, "/api/agenda",
		strings.NewReader(`{"fecha":"2026-09-12","texto":"Rutas"}`))
	rec := httptest.NewRecorder()
	s.Rutas().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Rutas POST: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// GET actividad por el mux real.
	req2 := httptest.NewRequest(http.MethodGet, "/api/actividad?desde=2026-09-01&hasta=2026-09-30", nil)
	rec2 := httptest.NewRecorder()
	s.Rutas().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("Rutas actividad: status=%d body=%s", rec2.Code, rec2.Body.String())
	}

	// GET agenda por el mux real trae la nota creada.
	req3 := httptest.NewRequest(http.MethodGet, "/api/agenda?desde=2026-09-01&hasta=2026-09-30", nil)
	rec3 := httptest.NewRecorder()
	s.Rutas().ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK || !strings.Contains(rec3.Body.String(), "Rutas") {
		t.Fatalf("Rutas GET agenda mal: status=%d body=%s", rec3.Code, rec3.Body.String())
	}

	// DELETE ausente por el mux real es 404.
	req4 := httptest.NewRequest(http.MethodDelete, "/api/agenda/9999", nil)
	rec4 := httptest.NewRecorder()
	s.Rutas().ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusNotFound {
		t.Fatalf("Rutas DELETE ausente debió ser 404, salió %d: %s", rec4.Code, rec4.Body.String())
	}
}

func TestBorrarCarpetaConservaAgendaHandler(t *testing.T) {
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	if _, err := db.CrearAgenda(s.Base, "2026-09-12", "", "", "Plan", &c.ID); err != nil {
		t.Fatalf("CrearAgenda: %v", err)
	}

	// DELETE de la carpeta por el handler (el camino real de la UI).
	req := httptest.NewRequest(http.MethodDelete, "/api/carpetas/"+itoa(c.ID), nil)
	req.SetPathValue("id", itoa(c.ID))
	rec := httptest.NewRecorder()
	s.borrarCarpeta(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("borrar carpeta: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// La nota sigue, con carpeta NULL.
	quedan, err := db.ListarAgenda(s.Base, "", "")
	if err != nil || len(quedan) != 1 || quedan[0].Texto != "Plan" {
		t.Fatalf("la nota debió sobrevivir: %+v err=%v", quedan, err)
	}
	if quedan[0].CarpetaID != nil {
		t.Fatalf("carpeta_id debió quedar NULL")
	}
}
