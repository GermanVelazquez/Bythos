package api

// carpetas_test.go — DELETE /api/carpetas/{id} + export prompt shape.
// Uses basePrueba/itoa from avance_test.go (same package, no duplication).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bythos-desktop/db"
)

func TestBorrarCarpetaCascade(t *testing.T) {
	s := basePrueba(t)
	c, err := db.CrearCarpeta(s.Base, "React")
	if err != nil {
		t.Fatalf("CrearCarpeta: %v", err)
	}
	a, _ := db.Guardar(s.Base, c.ID, "http://a/1", "A", "", "", "articulo")
	b, _ := db.Guardar(s.Base, c.ID, "http://a/2", "B", "", "", "youtube")

	borrada, err := db.BorrarCarpeta(s.Base, c.ID)
	if err != nil {
		t.Fatalf("BorrarCarpeta: %v", err)
	}
	if !borrada {
		t.Fatal("debía reportar carpeta borrada")
	}
	restantes, _ := db.ListarRecursos(s.Base, c.ID)
	if len(restantes) != 0 {
		t.Fatalf("cascade falló: quedan %d recursos", len(restantes))
	}
	todos, _ := db.ListarRecursos(s.Base, 0)
	for _, r := range todos {
		if r.ID == a.ID || r.ID == b.ID {
			t.Fatalf("recurso huérfano tras cascade: %+v", r)
		}
	}
	carpetas, _ := db.ListarCarpetas(s.Base)
	for _, f := range carpetas {
		if f.ID == c.ID {
			t.Fatal("la carpeta sigue listada tras borrar")
		}
	}
}

func TestBorrarCarpetaHandler(t *testing.T) {
	casos := []struct {
		nombre string
		existe bool
		status int
	}{
		{"existing folder returns ok", true, http.StatusOK},
		{"missing folder returns 404", false, http.StatusNotFound},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			s := basePrueba(t)
			id := int64(9999)
			if tt.existe {
				c, _ := db.CrearCarpeta(s.Base, "React")
				id = c.ID
			}
			req := httptest.NewRequest(http.MethodDelete, "/api/carpetas/"+itoa(id), nil)
			req.SetPathValue("id", itoa(id))
			rec := httptest.NewRecorder()
			s.borrarCarpeta(rec, req)
			if rec.Code != tt.status {
				t.Fatalf("status=%d body=%s, quería %d", rec.Code, rec.Body.String(), tt.status)
			}
			if tt.existe {
				var resp map[string]bool
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || !resp["ok"] {
					t.Fatalf("respuesta mal: %s", rec.Body.String())
				}
			} else {
				var resp map[string]string
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp["error"] == "" {
					t.Fatalf("404 debe traer {error} en español, salió: %s", rec.Body.String())
				}
			}
		})
	}
}

func TestBorrarCarpetaRutaWire(t *testing.T) {
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")

	// DELETE an existing folder through the real mux (no :8080, no network).
	req := httptest.NewRequest(http.MethodDelete, "/api/carpetas/"+itoa(c.ID), nil)
	rec := httptest.NewRecorder()
	s.Rutas().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Rutas DELETE existente: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Same mux, missing id → 404 Spanish {error}.
	req2 := httptest.NewRequest(http.MethodDelete, "/api/carpetas/9999", nil)
	rec2 := httptest.NewRecorder()
	s.Rutas().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("Rutas DELETE ausente: status=%d body=%s", rec2.Code, rec2.Body.String())
	}
}

func TestExportPromptShape(t *testing.T) {
	items := recursosPrueba()
	for _, tt := range []struct {
		nombre string
		out    string
	}{
		{"markdown carries prompt block", markdownCarpeta("React", items)},
		{"gemini carries prompt block", geminiRepaso("React", items)},
	} {
		t.Run(tt.nombre, func(t *testing.T) {
			for _, want := range []string{
				"Para actualizar tu avance",
				"- [N%]",
				"<!-- id:X -->",
				"0 a 100",
				"sin texto extra",
				"Importar avance",
			} {
				if !strings.Contains(tt.out, want) {
					t.Fatalf("falta %q en:\n%s", want, tt.out)
				}
			}
		})
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	a, _ := db.Guardar(s.Base, c.ID, "http://a/1", "Intro React", "", "", "articulo")
	b, _ := db.Guardar(s.Base, c.ID, "http://a/2", "Hooks", "", "", "articulo")

	// Export still carries parseable lines for each resource.
	// (The strict-prompt block adds one ejemplo line duplicating the first
	// id, so count can exceed resources; what matters is both ids parse.)
	md := markdownCarpeta("React", []db.Recurso{
		{ID: a.ID, Titulo: "Intro React", Progreso: 10},
		{ID: b.ID, Titulo: "Hooks", Progreso: 20},
	})
	got, _ := parseAvance(md)
	vistos := map[int64]bool{}
	for _, it := range got {
		vistos[it.ID] = true
	}
	if !vistos[a.ID] || !vistos[b.ID] {
		t.Fatalf("export debe aportar líneas parseables para %d y %d, salieron %+v", a.ID, b.ID, got)
	}

	// Simulated AI answer: strict shape only, no prose.
	ia := "- [80%] Intro React <!-- id:" + itoa(a.ID) + " -->\n" +
		"- [100%] Hooks <!-- id:" + itoa(b.ID) + " -->\n"
	body, _ := json.Marshal(map[string]string{"markdown": ia})
	req := httptest.NewRequest(http.MethodPost, "/api/carpetas/"+itoa(c.ID)+"/import-avance", strings.NewReader(string(body)))
	req.SetPathValue("id", itoa(c.ID))
	rec := httptest.NewRecorder()
	s.importarAvance(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["actualizados"] != 2 || resp["omitidos"] != 0 {
		t.Fatalf("respuesta mal: %v", resp)
	}
	actual, _ := db.ListarRecursos(s.Base, c.ID)
	porID := map[int64]db.Recurso{}
	for _, r := range actual {
		porID[r.ID] = r
	}
	if porID[a.ID].Progreso != 80 || porID[b.ID].Progreso != 100 {
		t.Fatalf("progreso no aplicado: %+v", porID)
	}

	// Gemini export keeps the same contract for its progress block.
	gm := geminiRepaso("React", []db.Recurso{
		{ID: a.ID, Titulo: "Intro React", URL: "http://a/1", Progreso: 80},
		{ID: b.ID, Titulo: "Hooks", URL: "http://a/2", Progreso: 100},
	})
	if !strings.Contains(gm, "- [80%] Intro React <!-- id:"+itoa(a.ID)+" -->") {
		t.Fatalf("gemini debe incluir bloque con id+%%, salió:\n%s", gm)
	}
}
