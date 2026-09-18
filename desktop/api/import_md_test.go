package api

// import_md_test.go — Pure parser plus import handler (httptest, no network).
// Table-driven per go-testing skill: cases named by scenario.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParsePuntualOK(t *testing.T) {
	md := "## Compromiso: Parcial\n" +
		"- Tipo: puntual\n" +
		"- Fecha: 2026-04-10\n" +
		"- Horario: 10-12\n" +
		"- Aula: 101\n" +
		"- Docente: García\n"
	occs, avisos := parseCompromisosMD(md)
	if len(avisos) != 0 {
		t.Fatalf("avisos inesperados: %v", avisos)
	}
	if len(occs) != 1 {
		t.Fatalf("queria 1 ocurrencia, salieron %d", len(occs))
	}
	o := occs[0]
	if o.Fecha != "2026-04-10" || o.HoraInicio != "10:00" || o.HoraFin != "12:00" {
		t.Fatalf("fecha/horas mal: %+v", o)
	}
	if o.Texto != "Parcial [Aula 101] (García)" {
		t.Fatalf("texto mal: %q", o.Texto)
	}
}

func TestParseSemanalExpande(t *testing.T) {
	casos := []struct {
		nombre string
		md     string
		total  int
		fechas []string
	}{
		{
			"dos dias por dos semanas son cuatro",
			"## Compromiso: Física I\n" +
				"- Tipo: semanal\n" +
				"- Desde: 2026-03-02\n" +
				"- Hasta: 2026-03-15\n" +
				"- Horario: lunes 19-20, miércoles 19-20\n",
			4,
			[]string{"2026-03-02", "2026-03-04", "2026-03-09", "2026-03-11"},
		},
		{
			"claves insensibles y sin acento",
			"## Compromiso: Química\n" +
				"- TIPO: Semanal\n" +
				"- DESDE: 2026-03-02\n" +
				"- HASTA: 2026-03-08\n" +
				"- HORARIO: miercoles 09:30-11:00\n",
			1,
			[]string{"2026-03-04"},
		},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			occs, avisos := parseCompromisosMD(mdWrap(tt.md))
			if len(avisos) != 0 {
				t.Fatalf("avisos inesperados: %v", avisos)
			}
			if len(occs) != tt.total {
				t.Fatalf("queria %d, salieron %d (%+v)", tt.total, len(occs), occs)
			}
			for i, f := range tt.fechas {
				if occs[i].Fecha != f {
					t.Fatalf("fecha %d mal: %q, queria %q", i, occs[i].Fecha, f)
				}
			}
		})
	}
}

func mdWrap(s string) string { return s }

func TestParseCuatrimestralSinFechasAvisa(t *testing.T) {
	md := "## Compromiso: Física I\n" +
		"- Tipo: semanal\n" +
		"- Modalidad: 1er cuatrimestre\n" +
		"- Horario: lunes 19-20\n"
	occs, avisos := parseCompromisosMD(md)
	if len(occs) != 0 {
		t.Fatalf("queria 0 ocurrencias, salieron %+v", occs)
	}
	if len(avisos) != 1 || !strings.Contains(avisos[0], "Bloque 1 (Física I)") {
		t.Fatalf("aviso con bloque mal: %v", avisos)
	}
	if !strings.Contains(avisos[0], "1er cuatrimestre") || !strings.Contains(avisos[0], "Desde/Hasta") {
		t.Fatalf("aviso debe pedir Desde/Hasta para el cuatrimestre: %v", avisos)
	}
}

func TestParseHoraInvalidaAvisa(t *testing.T) {
	casos := []struct {
		nombre string
		md     string
		frag   string
	}{
		{
			"rango invertido en puntual",
			"## Compromiso: Parcial\n- Tipo: puntual\n- Fecha: 2026-04-10\n- Horario: 12-10\n",
			"horario inválido",
		},
		{
			"dia desconocido en semanal",
			"## Compromiso: Física\n- Tipo: semanal\n- Desde: 2026-03-02\n- Hasta: 2026-03-15\n- Horario: lunes 19-20, funday 19-20\n",
			"día desconocido",
		},
		{
			"falta tipo",
			"## Compromiso: X\n- Fecha: 2026-04-10\n- Horario: 10-11\n",
			"falta Tipo",
		},
		{
			"desde posterior a hasta",
			"## Compromiso: X\n- Tipo: semanal\n- Desde: 2026-03-15\n- Hasta: 2026-03-02\n- Horario: lunes 19-20\n",
			"posterior",
		},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			_, avisos := parseCompromisosMD(tt.md)
			if len(avisos) == 0 {
				t.Fatal("queria al menos 1 aviso y no hubo")
			}
			juntos := strings.Join(avisos, " | ")
			if !strings.Contains(juntos, tt.frag) {
				t.Fatalf("falta %q en avisos: %v", tt.frag, avisos)
			}
		})
	}
}

func TestParseAnualExpandeMarzoDiciembre(t *testing.T) {
	md := "## Compromiso: Anual\n" +
		"- Tipo: semanal\n" +
		"- Desde: 2026-05-01\n" +
		"- Modalidad: anual\n" +
		"- Horario: lunes 10-11\n"
	occs, avisos := parseCompromisosMD(md)
	if len(avisos) != 0 {
		t.Fatalf("avisos inesperados: %v", avisos)
	}
	if len(occs) == 0 {
		t.Fatal("anual debio expandir ocurrencias")
	}
	if occs[0].Fecha != "2026-03-02" {
		t.Fatalf("anual debe arrancar en marzo, salio %q", occs[0].Fecha)
	}
	ultima := occs[len(occs)-1].Fecha
	if ultima != "2026-12-28" {
		t.Fatalf("anual debe cerrar en diciembre, salio %q", ultima)
	}
}

func TestImportAgendaHandlerIdempotente(t *testing.T) {
	s := basePrueba(t)
	md := "## Compromiso: Parcial\n- Tipo: puntual\n- Fecha: 2026-04-10\n- Horario: 10-12\n"
	cuerpo, _ := json.Marshal(map[string]string{"markdown": md})

	llamar := func() (int, map[string]interface{}) {
		req := httptest.NewRequest(http.MethodPost, "/api/agenda/import", strings.NewReader(string(cuerpo)))
		rec := httptest.NewRecorder()
		s.importarAgenda(rec, req)
		var resp map[string]interface{}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		return rec.Code, resp
	}

	codigo, primera := llamar()
	if codigo != http.StatusOK || int(primera["creadas"].(float64)) != 1 {
		t.Fatalf("primera import mal: codigo=%d resp=%v", codigo, primera)
	}
	codigo, segunda := llamar()
	if codigo != http.StatusOK {
		t.Fatalf("segunda import status=%d", codigo)
	}
	if int(segunda["creadas"].(float64)) != 0 || int(segunda["omitidas"].(float64)) != 1 {
		t.Fatalf("la repetida debio omitirse: %v", segunda)
	}
}

func TestImportAgendaHandlerPreviewYVacio(t *testing.T) {
	s := basePrueba(t)

	vacio, _ := json.Marshal(map[string]string{"markdown": "   "})
	reqVacio := httptest.NewRequest(http.MethodPost, "/api/agenda/import", strings.NewReader(string(vacio)))
	recVacio := httptest.NewRecorder()
	s.importarAgenda(recVacio, reqVacio)
	if recVacio.Code != http.StatusBadRequest {
		t.Fatalf("vacio debio ser 400, salio %d: %s", recVacio.Code, recVacio.Body.String())
	}
	var errResp map[string]string
	_ = json.Unmarshal(recVacio.Body.Bytes(), &errResp)
	if errResp["error"] == "" {
		t.Fatalf("400 debe traer {error} en español: %s", recVacio.Body.String())
	}

	md := "## Compromiso: Física I\n- Tipo: semanal\n- Desde: 2026-03-02\n- Hasta: 2026-03-15\n- Horario: lunes 19-20, miércoles 19-20\n"
	cuerpo, _ := json.Marshal(map[string]string{"markdown": md})
	req := httptest.NewRequest(http.MethodPost, "/api/agenda/import?dry=true", strings.NewReader(string(cuerpo)))
	rec := httptest.NewRecorder()
	s.importarAgenda(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Total       int             `json:"total"`
		Ocurrencias []ocurrenciaMD `json:"ocurrencias"`
		Avisos      []string        `json:"avisos"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("preview no JSON: %v", err)
	}
	if resp.Total != 4 || len(resp.Ocurrencias) != 4 {
		t.Fatalf("preview mal: %+v", resp)
	}
	// Dry-run must not persist.
	quedan, _ := s.Base.Query(`SELECT COUNT(*) FROM agenda`)
	var n int
	if quedan.Next() {
		_ = quedan.Scan(&n)
	}
	quedan.Close()
	if n != 0 {
		t.Fatalf("dry=true no debe guardar, hay %d filas", n)
	}
}

func TestImportAgendaRutasWire(t *testing.T) {
	s := basePrueba(t)
	cuerpo, _ := json.Marshal(map[string]string{"markdown": "## Compromiso: X\n- Tipo: puntual\n- Fecha: 2026-04-10\n- Horario: 10-11\n"})
	req := httptest.NewRequest(http.MethodPost, "/api/agenda/import", strings.NewReader(string(cuerpo)))
	rec := httptest.NewRecorder()
	s.Rutas().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "creadas") {
		t.Fatalf("Rutas import mal: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
