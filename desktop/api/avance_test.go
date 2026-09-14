package api

// avance_test.go — Parser puro + progreso en DB temporal + handler HTTP.
// Pure parser runs in milliseconds; DB/handler use t.TempDir() so no real
// home directory is touched. Table-driven per go-testing skill.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"bythos-desktop/db"
)

func TestParseAvanceLine(t *testing.T) {
	casos := []struct {
		nombre   string
		linea    string
		ok       bool
		id       int64
		progreso int
	}{
		{"exact spec format", "- [40%] Intro React <!-- id:7 -->", true, 7, 40},
		{"zero percent", "- [0%] Nada <!-- id:1 -->", true, 1, 0},
		{"hundred percent", "- [100%] Todo <!-- id:2 -->", true, 2, 100},
		{"over clamps to hundred", "- [150%] AI invento <!-- id:3 -->", true, 3, 100},
		{"extra spaces in comment", "- [25%] T <!--  id:9  -->", true, 9, 25},
		{"no marker ignored", "just a title without progress", false, 0, 0},
		{"missing id ignored", "- [40%] Sin id", false, 0, 0},
		{"header ignored", "# React (exportado desde Bythos)", false, 0, 0},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			id, prog, ok := parseAvanceLine(tt.linea)
			if ok != tt.ok {
				t.Fatalf("ok=%v, queria %v", ok, tt.ok)
			}
			if !ok {
				return
			}
			if id != tt.id || prog != tt.progreso {
				t.Fatalf("salio id=%d prog=%d, queria id=%d prog=%d", id, prog, tt.id, tt.progreso)
			}
		})
	}
}

func TestParseAvanceCounts(t *testing.T) {
	texto := "# React (exportado desde Bythos)\n\n" +
		"- [40%] Intro React <!-- id:7 -->\n" +
		"- [100%] Hooks <!-- id:8 -->\n" +
		"linea rota sin marcador\n"
	items, omitidos := parseAvance(texto)
	if len(items) != 2 {
		t.Fatalf("queria 2 items, salieron %d", len(items))
	}
	if omitidos != 1 {
		t.Fatalf("queria 1 omitido, salieron %d", omitidos)
	}
	if items[0].ID != 7 || items[0].Progreso != 40 {
		t.Fatalf("primer item mal: %+v", items[0])
	}
}

// basePrueba opens an isolated .db with the real migration path.
func basePrueba(t *testing.T) *Servidor {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), "bythos.db")
	base, err := db.Abrir(ruta)
	if err != nil {
		t.Fatalf("Abrir: %v", err)
	}
	t.Cleanup(func() { base.Close() })
	return &Servidor{Base: base}
}

func TestActualizarProgresoSyncsEstado(t *testing.T) {
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	r, _ := db.Guardar(s.Base, c.ID, "http://a/1", "Intro", "", "", "articulo")

	casos := []struct {
		nombre   string
		valor    int
		progreso int
		estado   string
	}{
		{"partial moves to en_curso", 40, 40, db.EstadoEnCurso},
		{"zero back to pendiente", 0, 0, db.EstadoPendiente},
		{"hundred completes", 100, 100, db.EstadoCompletado},
		{"over clamps to hundred", 150, 100, db.EstadoCompletado},
		{"negative clamps to zero", -10, 0, db.EstadoPendiente},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			if err := db.ActualizarProgreso(s.Base, r.ID, tt.valor); err != nil {
				t.Fatalf("ActualizarProgreso: %v", err)
			}
			got, _ := db.ListarRecursos(s.Base, c.ID)
			if got[0].Progreso != tt.progreso || got[0].Estado != tt.estado {
				t.Fatalf("salio prog=%d estado=%s, queria %d/%s",
					got[0].Progreso, got[0].Estado, tt.progreso, tt.estado)
			}
		})
	}
}

func TestPromedioProgreso(t *testing.T) {
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	a, _ := db.Guardar(s.Base, c.ID, "http://a/1", "A", "", "", "otro")
	b, _ := db.Guardar(s.Base, c.ID, "http://a/2", "B", "", "", "otro")
	_ = db.ActualizarProgreso(s.Base, a.ID, 40)
	_ = db.ActualizarProgreso(s.Base, b.ID, 80)

	p, err := db.ProgresoCarpeta(s.Base, c.ID)
	if err != nil {
		t.Fatalf("ProgresoCarpeta: %v", err)
	}
	if p.Promedio != 60 {
		t.Fatalf("promedio=%d, queria 60", p.Promedio)
	}
	// Porcentaje counts completados only, so it stays 0 here by design.
	if p.Porcentaje != 0 {
		t.Fatalf("porcentaje=%d, queria 0 (compat con completados)", p.Porcentaje)
	}

	g, err := db.ProgresoGeneral(s.Base)
	if err != nil {
		t.Fatalf("ProgresoGeneral: %v", err)
	}
	if g.Promedio != 60 || g.Total != 2 {
		t.Fatalf("general mal: %+v", g)
	}
}

func TestImportAvanceHandlerMarkdown(t *testing.T) {
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	a, _ := db.Guardar(s.Base, c.ID, "http://a/1", "Intro React", "", "", "articulo")
	b, _ := db.Guardar(s.Base, c.ID, "http://a/2", "Hooks", "", "", "articulo")

	md := "# React (exportado desde Bythos)\n" +
		"- [40%] Intro React <!-- id:" + itoa(a.ID) + " -->\n" +
		"- [100%] Hooks <!-- id:" + itoa(b.ID) + " -->\n" +
		"linea rota\n"
	body, _ := json.Marshal(map[string]string{"markdown": md})
	req := httptest.NewRequest(http.MethodPost, "/api/carpetas/"+itoa(c.ID)+"/import-avance", strings.NewReader(string(body)))
	req.SetPathValue("id", itoa(c.ID))
	rec := httptest.NewRecorder()
	s.importarAvance(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("respuesta no JSON: %v", err)
	}
	if resp["actualizados"] != 2 || resp["omitidos"] != 1 {
		t.Fatalf("respuesta mal: %v", resp)
	}
	got, _ := db.ListarRecursos(s.Base, c.ID)
	porID := map[int64]db.Recurso{}
	for _, r := range got {
		porID[r.ID] = r
	}
	if porID[a.ID].Progreso != 40 || porID[a.ID].Estado != db.EstadoEnCurso {
		t.Fatalf("recurso A mal: %+v", porID[a.ID])
	}
	if porID[b.ID].Progreso != 100 || porID[b.ID].Estado != db.EstadoCompletado {
		t.Fatalf("recurso B mal: %+v", porID[b.ID])
	}
}

func TestImportAvanceHandlerJSONArray(t *testing.T) {
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	a, _ := db.Guardar(s.Base, c.ID, "http://a/1", "A", "", "", "otro")
	payload := `[{"id":` + itoa(a.ID) + `,"progreso":250},{"id":9999,"progreso":10}]`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	req.SetPathValue("id", itoa(c.ID))
	rec := httptest.NewRecorder()
	s.importarAvance(rec, req)

	var resp map[string]int
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["actualizados"] != 1 || resp["omitidos"] != 1 {
		t.Fatalf("clamp+omitidos mal: %v", resp)
	}
	got, _ := db.ListarRecursos(s.Base, c.ID)
	if got[0].Progreso != 100 {
		t.Fatalf("250 debio clampear a 100, salio %d", got[0].Progreso)
	}
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
