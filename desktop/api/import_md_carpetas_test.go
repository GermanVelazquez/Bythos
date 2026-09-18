package api

// import_md_carpetas_test.go — Folder alignment for MD imports.
// Covers parsing "- Carpeta:", normalized resolution, warnings and
// persistence across POST/PUT. Table-driven per go-testing skill.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bythos-desktop/db"
)

func TestParseCarpetaNombrePorBloque(t *testing.T) {
	casos := []struct {
		nombre      string
		md          string
		carpeta     string
		estado      string
		quiereBloque string
	}{
		{
			"exact folder line is kept raw",
			"## Compromiso: Parcial\n- Tipo: puntual\n- Fecha: 2026-04-10\n- Horario: 10-12\n- Carpeta: Programación II\n",
			"Programación II", "suelta", "Bloque 1 (Parcial)",
		},
		{
			"key is case-insensitive",
			"## Compromiso: Parcial\n- Tipo: puntual\n- Fecha: 2026-04-10\n- Horario: 10-12\n- CARPETA: Fisica\n",
			"Fisica", "suelta", "Bloque 1 (Parcial)",
		},
		{
			"missing line stays loose without folder",
			"## Compromiso: Parcial\n- Tipo: puntual\n- Fecha: 2026-04-10\n- Horario: 10-12\n",
			"", "sin-carpeta", "Bloque 1 (Parcial)",
		},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			occs, avisos := parseCompromisosMD(tt.md)
			if len(avisos) != 0 {
				t.Fatalf("avisos inesperados: %v", avisos)
			}
			if len(occs) != 1 {
				t.Fatalf("queria 1 ocurrencia, salieron %+v", occs)
			}
			o := occs[0]
			if o.CarpetaNombre != tt.carpeta {
				t.Fatalf("carpeta=%q, queria %q", o.CarpetaNombre, tt.carpeta)
			}
			if o.Estado != tt.estado {
				t.Fatalf("estado=%q, queria %q", o.Estado, tt.estado)
			}
			if o.Bloque != tt.quiereBloque {
				t.Fatalf("bloque=%q, queria %q", o.Bloque, tt.quiereBloque)
			}
			if o.CarpetaID != nil {
				t.Fatalf("el parser puro no resuelve IDs, salio %+v", o.CarpetaID)
			}
		})
	}
}

func TestVincularCarpetasResolucion(t *testing.T) {
	carpetas := []db.Carpeta{{ID: 7, Nombre: "Programación II"}}
	nueva := func(carpeta string, bloque string) []ocurrenciaMD {
		estado := "sin-carpeta"
		if carpeta != "" {
			estado = "suelta"
		}
		return []ocurrenciaMD{{Fecha: "2026-04-10", HoraInicio: "10:00", HoraFin: "11:00", Texto: "X", CarpetaNombre: carpeta, Estado: estado, Bloque: bloque}}
	}

	casos := []struct {
		nombre       string
		carpetaMD    string
		quiereID     bool
		quiereEstado string
		quiereAviso  string
	}{
		{"exact match links", "Programación II", true, "vinculada", ""},
		{"case-insensitive without accents links", "programacion ii", true, "vinculada", ""},
		{"surrounding spaces are trimmed", "  Programación II  ", true, "vinculada", ""},
		{"unknown folder stays loose with warning", "Física III", false, "suelta", `Bloque 1 (Parcial): carpeta "Física III" no existe, queda suelto`},
		{"no line stays loose without warning", "", false, "sin-carpeta", ""},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			occs, avisos := vincularCarpetas(nueva(tt.carpetaMD, "Bloque 1 (Parcial)"), carpetas)
			if len(occs) != 1 {
				t.Fatalf("queria 1 ocurrencia, salieron %d", len(occs))
			}
			o := occs[0]
			if tt.quiereID && o.CarpetaID == nil {
				t.Fatalf("debió vincularse, salio %+v", o)
			}
			if !tt.quiereID && o.CarpetaID != nil {
				t.Fatalf("debió quedar NULL, salio %+v", o.CarpetaID)
			}
			if tt.quiereID && *o.CarpetaID != 7 {
				t.Fatalf("carpeta_id=%d, queria 7", *o.CarpetaID)
			}
			if o.Estado != tt.quiereEstado {
				t.Fatalf("estado=%q, queria %q", o.Estado, tt.quiereEstado)
			}
			juntos := strings.Join(avisos, " | ")
			if tt.quiereAviso == "" {
				for _, a := range avisos {
					if strings.Contains(a, "no existe") {
						t.Fatalf("sin aviso esperado, salio: %v", avisos)
					}
				}
				return
			}
			if len(avisos) != 1 || avisos[0] != tt.quiereAviso {
				t.Fatalf("aviso=%v, queria %q", avisos, tt.quiereAviso)
			}
			if !strings.Contains(juntos, "queda suelto") {
				t.Fatalf("el aviso debe decir que queda suelto: %v", avisos)
			}
		})
	}
}

func TestVincularCarpetasDuplicadasUsaLaPrimera(t *testing.T) {
	carpetas := []db.Carpeta{{ID: 1, Nombre: "Programación II"}, {ID: 2, Nombre: "programacion ii"}}
	occs := []ocurrenciaMD{{Fecha: "2026-04-10", HoraInicio: "10:00", HoraFin: "11:00", Texto: "X", CarpetaNombre: "PROGRAMACION II", Estado: "suelta", Bloque: "Bloque 1 (X)"}}
	got, avisos := vincularCarpetas(occs, carpetas)
	if got[0].CarpetaID == nil || *got[0].CarpetaID != 1 {
		t.Fatalf("debió usarse la primera (id 1), salio %+v", got[0].CarpetaID)
	}
	if got[0].Estado != "vinculada" {
		t.Fatalf("estado=%q, queria vinculada", got[0].Estado)
	}
	juntos := strings.Join(avisos, " | ")
	if !strings.Contains(juntos, "se normalizan igual") || !strings.Contains(juntos, "se usa") {
		t.Fatalf("falta aviso de duplicadas: %v", avisos)
	}
}

func TestImportPreviewIncluyeMapeoCarpeta(t *testing.T) {
	s := basePrueba(t)
	carpeta, err := db.CrearCarpeta(s.Base, "Programación II")
	if err != nil {
		t.Fatalf("CrearCarpeta: %v", err)
	}
	md := "## Compromiso: Parcial\n- Tipo: puntual\n- Fecha: 2026-04-10\n- Horario: 10-12\n- Carpeta: programacion ii\n" +
		"\n## Compromiso: Final\n- Tipo: puntual\n- Fecha: 2026-06-20\n- Horario: 9-11\n- Carpeta: Física III\n"
	payload, _ := json.Marshal(map[string]string{"markdown": md})
	req := httptest.NewRequest(http.MethodPost, "/api/agenda/import?dry=true", strings.NewReader(string(payload)))
	rec := httptest.NewRecorder()
	s.importarAgenda(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Total       int            `json:"total"`
		Ocurrencias []ocurrenciaMD `json:"ocurrencias"`
		Avisos      []string       `json:"avisos"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("preview no JSON: %v", err)
	}
	if resp.Total != 2 || len(resp.Ocurrencias) != 2 {
		t.Fatalf("preview mal: %+v", resp)
	}
	// Sorted by date: Parcial first (linked), Final second (loose).
	primera := resp.Ocurrencias[0]
	if primera.CarpetaID == nil || *primera.CarpetaID != carpeta.ID {
		t.Fatalf("la primera debió vincularse a %d: %+v", carpeta.ID, primera)
	}
	if primera.Estado != "vinculada" || primera.CarpetaNombre != "programacion ii" {
		t.Fatalf("mapeo mal: %+v", primera)
	}
	segunda := resp.Ocurrencias[1]
	if segunda.CarpetaID != nil || segunda.Estado != "suelta" {
		t.Fatalf("la desconocida debió quedar suelta: %+v", segunda)
	}
	juntos := strings.Join(resp.Avisos, " | ")
	if !strings.Contains(juntos, `Bloque 2 (Final): carpeta "Física III" no existe, queda suelto`) {
		t.Fatalf("falta aviso del bloque 2: %v", resp.Avisos)
	}
	// Dry-run must not persist.
	if contarAgenda(t, s) != 0 {
		t.Fatal("dry=true no debe guardar filas")
	}
}

func TestImportPostPersisteCarpetaID(t *testing.T) {
	s := basePrueba(t)
	carpeta, _ := db.CrearCarpeta(s.Base, "Programación II")
	md := "## Compromiso: Parcial\n- Tipo: puntual\n- Fecha: 2026-04-10\n- Horario: 10-12\n- Carpeta: PROGRAMACION II\n"
	resp := postLote(t, s, md, "Cursada")
	if int(resp["creadas"].(float64)) != 1 {
		t.Fatalf("creadas mal: %v", resp)
	}
	filas, err := db.ListarAgenda(s.Base, "2026-04-10", "2026-04-10")
	if err != nil || len(filas) != 1 {
		t.Fatalf("filas=%+v err=%v", filas, err)
	}
	if filas[0].CarpetaID == nil || *filas[0].CarpetaID != carpeta.ID {
		t.Fatalf("carpeta_id no persistió: %+v", filas[0])
	}
	if filas[0].CarpetaNombre != "Programación II" {
		t.Fatalf("el JOIN debió traer el nombre: %+v", filas[0])
	}

	// List carries live folders for MD cargados badges.
	reqList := httptest.NewRequest(http.MethodGet, "/api/agenda/imports", nil)
	recList := httptest.NewRecorder()
	s.listarLotes(recList, reqList)
	var lista []map[string]interface{}
	if err := json.Unmarshal(recList.Body.Bytes(), &lista); err != nil || len(lista) != 1 {
		t.Fatalf("lista mal: %s err=%v", recList.Body.String(), err)
	}
	carps, _ := lista[0]["carpetas"].([]interface{})
	if len(carps) != 1 {
		t.Fatalf("la lista debió traer 1 carpeta: %v", lista[0])
	}
	primera, _ := carps[0].(map[string]interface{})
	if primera["nombre"] != "Programación II" {
		t.Fatalf("carpeta mal en lista: %v", primera)
	}

	// Detail re-resolves the same mapping for the edit view.
	id := fmt.Sprintf("%.0f", resp["lote_id"].(float64))
	reqGet := httptest.NewRequest(http.MethodGet, "/api/agenda/imports/"+id, nil)
	reqGet.SetPathValue("id", id)
	recGet := httptest.NewRecorder()
	s.obtenerLote(recGet, reqGet)
	var detalle struct {
		Ocurrencias []ocurrenciaMD `json:"ocurrencias"`
		Avisos      []string       `json:"avisos"`
	}
	if err := json.Unmarshal(recGet.Body.Bytes(), &detalle); err != nil {
		t.Fatalf("detalle no JSON: %v", err)
	}
	if len(detalle.Ocurrencias) != 1 || detalle.Ocurrencias[0].Estado != "vinculada" {
		t.Fatalf("detalle mal: %+v", detalle)
	}
}

func TestPutReresuelveCarpetaCreadaDespues(t *testing.T) {
	s := basePrueba(t)
	md := "## Compromiso: Parcial\n- Tipo: puntual\n- Fecha: 2026-04-10\n- Horario: 10-12\n- Carpeta: Programación II\n"
	resp := postLote(t, s, md, "Cursada")
	id := fmt.Sprintf("%.0f", resp["lote_id"].(float64))
	// Unknown folder at POST time: loose row + warning, never auto-created.
	filas, _ := db.ListarAgenda(s.Base, "", "")
	if len(filas) != 1 || filas[0].CarpetaID != nil {
		t.Fatalf("debió quedar suelta: %+v", filas)
	}
	juntos := fmt.Sprintf("%v", resp["avisos"])
	if !strings.Contains(juntos, "no existe") {
		t.Fatalf("el POST debió avisar carpeta faltante: %v", resp)
	}
	sueltas, _ := db.ListarCarpetas(s.Base)
	if len(sueltas) != 0 {
		t.Fatalf("nunca crear carpetas solo: %+v", sueltas)
	}

	// Creating the folder later links it alone on PUT with the same markdown.
	carpeta, err := db.CrearCarpeta(s.Base, "Programación II")
	if err != nil {
		t.Fatalf("CrearCarpeta: %v", err)
	}
	payload, _ := json.Marshal(map[string]string{"markdown": md})
	req := httptest.NewRequest(http.MethodPut, "/api/agenda/imports/"+id, strings.NewReader(string(payload)))
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	s.actualizarLote(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT: status=%d body=%s", rec.Code, rec.Body.String())
	}
	filas, _ = db.ListarAgenda(s.Base, "", "")
	if len(filas) != 1 || filas[0].CarpetaID == nil || *filas[0].CarpetaID != carpeta.ID {
		t.Fatalf("el PUT debió vincular solo: %+v", filas)
	}
	var r map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &r)
	for _, a := range r["avisos"].([]interface{}) {
		if strings.Contains(a.(string), "no existe") {
			t.Fatalf("ya existe, no debió avisar: %v", r["avisos"])
		}
	}
}
