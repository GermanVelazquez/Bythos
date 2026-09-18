package api

// import_md_decisiones_test.go — Per-block folder assignment on MD import.
// Covers preview bloques menu data, confirm decisiones (auto/usar/crear/
// suelta), normalized dedupe, race-safe creation and PUT parity.
// Table-driven per go-testing skill; httptest, no network.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bythos-desktop/db"
)

const decisionesMD = "## Compromiso: Parcial\n- Tipo: puntual\n- Fecha: 2026-04-10\n- Horario: 10-12\n- Carpeta: Programación II\n" +
	"\n## Compromiso: Final\n- Tipo: puntual\n- Fecha: 2026-06-20\n- Horario: 9-11\n- Carpeta: Física III\n" +
	"\n## Compromiso: Libre\n- Tipo: puntual\n- Fecha: 2026-07-01\n- Horario: 9-11\n"

// postImportRaw sends an import payload and returns status + decoded body.
func postImportRaw(t *testing.T, s *Servidor, payload map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	raw, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/agenda/import", strings.NewReader(string(raw)))
	rec := httptest.NewRecorder()
	s.importarAgenda(rec, req)
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("respuesta no JSON: %v (%s)", err, rec.Body.String())
	}
	return rec.Code, resp
}

// putImportRaw sends a lote update and returns status + decoded body.
func putImportRaw(t *testing.T, s *Servidor, id string, payload map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	raw, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPut, "/api/agenda/imports/"+id, strings.NewReader(string(raw)))
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	s.actualizarLote(rec, req)
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("respuesta no JSON: %v (%s)", err, rec.Body.String())
	}
	return rec.Code, resp
}

func carpetaIDDeFila(t *testing.T, s *Servidor, fecha string) *int64 {
	t.Helper()
	filas, err := db.ListarAgenda(s.Base, fecha, fecha)
	if err != nil || len(filas) != 1 {
		t.Fatalf("filas %s=%+v err=%v", fecha, filas, err)
	}
	return filas[0].CarpetaID
}

func TestPreviewTraeBloquesEnOrden(t *testing.T) {
	s := basePrueba(t)
	existente, err := db.CrearCarpeta(s.Base, "Programación II")
	if err != nil {
		t.Fatalf("CrearCarpeta: %v", err)
	}
	raw, _ := json.Marshal(map[string]string{"markdown": decisionesMD})
	req := httptest.NewRequest(http.MethodPost, "/api/agenda/import?dry=true", strings.NewReader(string(raw)))
	rec := httptest.NewRecorder()
	s.importarAgenda(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Total    int       `json:"total"`
		Bloques  []bloqueMD `json:"bloques"`
		Existentes []db.Carpeta `json:"carpetas_existentes"`
		Avisos   []string  `json:"avisos"`
	}
	// db.Carpeta has no json tags (ID/Nombre capitalized): decode leniently.
	var crudo map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &crudo); err != nil {
		t.Fatalf("preview no JSON: %v", err)
	}
	if err := json.Unmarshal(crudo["bloques"], &resp.Bloques); err != nil {
		t.Fatalf("bloques no JSON: %v (%s)", err, string(crudo["bloques"]))
	}
	if err := json.Unmarshal(crudo["avisos"], &resp.Avisos); err != nil {
		t.Fatalf("avisos no JSON: %v", err)
	}
	var existentes []map[string]interface{}
	if err := json.Unmarshal(crudo["carpetas_existentes"], &existentes); err != nil {
		t.Fatalf("carpetas_existentes no JSON: %v", err)
	}
	if len(resp.Bloques) != 3 {
		t.Fatalf("quería 3 bloques en orden MD, salieron %+v", resp.Bloques)
	}
	casos := []struct {
		indice   int
		titulo   string
		pedida   string
		estado   string
		conID    bool
	}{
		{1, "Parcial", "Programación II", "vinculada", true},
		{2, "Final", "Física III", "suelta", false},
		{3, "Libre", "", "sin-carpeta", false},
	}
	for i, tt := range casos {
		b := resp.Bloques[i]
		if b.Indice != tt.indice || b.Titulo != tt.titulo || b.CarpetaPedida != tt.pedida || b.Estado != tt.estado {
			t.Fatalf("bloque %d mal: %+v", i, b)
		}
		if tt.conID && (b.CarpetaID == nil || *b.CarpetaID != existente.ID) {
			t.Fatalf("bloque %d debió apuntar a %d: %+v", i, existente.ID, b.CarpetaID)
		}
		if !tt.conID && b.CarpetaID != nil {
			t.Fatalf("bloque %d debió traer null: %+v", i, b.CarpetaID)
		}
	}
	if len(existentes) != 1 || existentes[0]["nombre"] != "Programación II" {
		t.Fatalf("carpetas_existentes mal: %v", existentes)
	}
	if id, ok := existentes[0]["id"].(float64); !ok || int64(id) != existente.ID {
		t.Fatalf("carpetas_existentes id mal: %v", existentes)
	}
	if contarAgenda(t, s) != 0 {
		t.Fatal("dry=true no debe guardar filas")
	}
}

func TestConfirmCrearCarpetaLinkeaBloque(t *testing.T) {
	s := basePrueba(t)
	status, resp := postImportRaw(t, s, map[string]interface{}{
		"markdown": decisionesMD[:strings.Index(decisionesMD, "## Compromiso: Final")],
		"decisiones": []map[string]interface{}{
			{"bloque": 1, "accion": "crear", "nombre": "Programación II"},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("confirm: status=%d body=%v", status, resp)
	}
	id := carpetaIDDeFila(t, s, "2026-04-10")
	if id == nil {
		t.Fatal("la fila debió quedar vinculada a la carpeta creada")
	}
	carpetas, _ := db.ListarCarpetas(s.Base)
	if len(carpetas) != 1 || carpetas[0].Nombre != "Programación II" || carpetas[0].ID != *id {
		t.Fatalf("carpetas mal: %+v", carpetas)
	}
	creadas, _ := resp["carpetas_creadas"].([]interface{})
	if len(creadas) != 1 {
		t.Fatalf("carpetas_creadas debió traer 1: %v", resp)
	}
	primera, _ := creadas[0].(map[string]interface{})
	if primera["nombre"] != "Programación II" {
		t.Fatalf("creada mal: %v", primera)
	}
	bloques, _ := primera["bloques"].([]interface{})
	if len(bloques) != 1 || int(bloques[0].(float64)) != 1 {
		t.Fatalf("bloques de la creada mal: %v", primera)
	}
	for _, a := range resp["avisos"].([]interface{}) {
		if strings.Contains(a.(string), "no existe") {
			t.Fatalf("creada la carpeta, no debió avisar faltante: %v", resp["avisos"])
		}
	}
}

func TestConfirmCrearMismoNombreUnaSolaCarpeta(t *testing.T) {
	s := basePrueba(t)
	md := "## Compromiso: Uno\n- Tipo: puntual\n- Fecha: 2026-04-10\n- Horario: 10-11\n- Carpeta: Física\n" +
		"\n## Compromiso: Dos\n- Tipo: puntual\n- Fecha: 2026-04-11\n- Horario: 10-11\n- Carpeta: fisica\n"
	status, resp := postImportRaw(t, s, map[string]interface{}{
		"markdown": md,
		"decisiones": []map[string]interface{}{
			{"bloque": 1, "accion": "crear", "nombre": "Física"},
			{"bloque": 2, "accion": "crear", "nombre": "fisica"},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("confirm: status=%d body=%v", status, resp)
	}
	carpetas, _ := db.ListarCarpetas(s.Base)
	if len(carpetas) != 1 {
		t.Fatalf("dedupe normalizado debió crear 1 sola: %+v", carpetas)
	}
	id1 := carpetaIDDeFila(t, s, "2026-04-10")
	id2 := carpetaIDDeFila(t, s, "2026-04-11")
	if id1 == nil || id2 == nil || *id1 != *id2 || *id1 != carpetas[0].ID {
		t.Fatalf("ambos bloques debieron linkear a %d: %v %v", carpetas[0].ID, id1, id2)
	}
	creadas, _ := resp["carpetas_creadas"].([]interface{})
	if len(creadas) != 1 {
		t.Fatalf("carpetas_creadas debió traer 1 entrada: %v", resp)
	}
	primera, _ := creadas[0].(map[string]interface{})
	bloques, _ := primera["bloques"].([]interface{})
	if len(bloques) != 2 || int(bloques[0].(float64)) != 1 || int(bloques[1].(float64)) != 2 {
		t.Fatalf("la creada debió listar bloques [1 2]: %v", primera)
	}
}

func TestConfirmUsarExistenteLinkea(t *testing.T) {
	s := basePrueba(t)
	existente, _ := db.CrearCarpeta(s.Base, "Historia")
	md := "## Compromiso: Libre\n- Tipo: puntual\n- Fecha: 2026-07-01\n- Horario: 9-11\n"
	status, resp := postImportRaw(t, s, map[string]interface{}{
		"markdown": md,
		"decisiones": []map[string]interface{}{
			{"bloque": 1, "accion": "usar", "carpeta_id": existente.ID},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("confirm: status=%d body=%v", status, resp)
	}
	id := carpetaIDDeFila(t, s, "2026-07-01")
	if id == nil || *id != existente.ID {
		t.Fatalf("usar debió vincular a %d: %+v", existente.ID, id)
	}
	if creadas, _ := resp["carpetas_creadas"].([]interface{}); len(creadas) != 0 {
		t.Fatalf("usar no crea carpetas: %v", resp)
	}
}

func TestConfirmSueltaFuerzaNULL(t *testing.T) {
	s := basePrueba(t)
	_, _ = db.CrearCarpeta(s.Base, "Programación II")
	md := "## Compromiso: Parcial\n- Tipo: puntual\n- Fecha: 2026-04-10\n- Horario: 10-12\n- Carpeta: Programación II\n"
	status, resp := postImportRaw(t, s, map[string]interface{}{
		"markdown": md,
		"decisiones": []map[string]interface{}{
			{"bloque": 1, "accion": "suelta"},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("confirm: status=%d body=%v", status, resp)
	}
	if id := carpetaIDDeFila(t, s, "2026-04-10"); id != nil {
		t.Fatalf("suelta debió dejar NULL: %+v", id)
	}
	for _, a := range resp["avisos"].([]interface{}) {
		if strings.Contains(a.(string), "no existe") {
			t.Fatalf("decisión explícita no avisa: %v", resp["avisos"])
		}
	}
}

func TestConfirmCarreraNoDuplica(t *testing.T) {
	s := basePrueba(t)
	md := "## Compromiso: Parcial\n- Tipo: puntual\n- Fecha: 2026-04-10\n- Horario: 10-12\n- Carpeta: Física III\n"
	// La carpeta nace ENTRE el preview y el confirm (otra pestaña).
	externa, err := db.CrearCarpeta(s.Base, "Física III")
	if err != nil {
		t.Fatalf("CrearCarpeta: %v", err)
	}
	status, resp := postImportRaw(t, s, map[string]interface{}{
		"markdown": md,
		"decisiones": []map[string]interface{}{
			{"bloque": 1, "accion": "crear", "nombre": "fisica iii"},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("confirm: status=%d body=%v", status, resp)
	}
	carpetas, _ := db.ListarCarpetas(s.Base)
	if len(carpetas) != 1 || carpetas[0].ID != externa.ID {
		t.Fatalf("la carrera no debió duplicar: %+v", carpetas)
	}
	id := carpetaIDDeFila(t, s, "2026-04-10")
	if id == nil || *id != externa.ID {
		t.Fatalf("debió linkear a la existente %d: %+v", externa.ID, id)
	}
	if creadas, _ := resp["carpetas_creadas"].([]interface{}); len(creadas) != 0 {
		t.Fatalf("nada creado, lista vacía esperada: %v", resp)
	}
}

func TestConfirmDecisionesInvalidasSon400SinEfectos(t *testing.T) {
	md := "## Compromiso: Parcial\n- Tipo: puntual\n- Fecha: 2026-04-10\n- Horario: 10-12\n- Carpeta: Física III\n"
	casos := []struct {
		nombre     string
		decisiones []map[string]interface{}
		frag       string
	}{
		{"crear con nombre vacío no toca nada", []map[string]interface{}{{"bloque": 1, "accion": "crear", "nombre": ""}}, "obligatorio"},
		{"crear con solo espacios no toca nada", []map[string]interface{}{{"bloque": 1, "accion": "crear", "nombre": "   "}}, "obligatorio"},
		{"acción desconocida no toca nada", []map[string]interface{}{{"bloque": 1, "accion": "mover"}}, "inválida"},
		{"usar sin carpeta no toca nada", []map[string]interface{}{{"bloque": 1, "accion": "usar"}}, "elegí una carpeta"},
		{"usar carpeta ausente no toca nada", []map[string]interface{}{{"bloque": 1, "accion": "usar", "carpeta_id": 9999}}, "no existe"},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			s := basePrueba(t)
			status, resp := postImportRaw(t, s, map[string]interface{}{
				"markdown": md, "nombre": "Lote", "decisiones": tt.decisiones,
			})
			if status != http.StatusBadRequest {
				t.Fatalf("status=%d body=%v, quería 400", status, resp)
			}
			msg, _ := resp["error"].(string)
			if !strings.Contains(msg, tt.frag) {
				t.Fatalf("error=%q, quería que contenga %q", msg, tt.frag)
			}
			if carpetas, _ := db.ListarCarpetas(s.Base); len(carpetas) != 0 {
				t.Fatalf("sin carpetas nuevas: %+v", carpetas)
			}
			if contarAgenda(t, s) != 0 {
				t.Fatal("sin filas nuevas")
			}
			reqList := httptest.NewRequest(http.MethodGet, "/api/agenda/imports", nil)
			recList := httptest.NewRecorder()
			s.listarLotes(recList, reqList)
			if strings.TrimSpace(recList.Body.String()) != "[]" {
				t.Fatalf("sin lotes nuevos: %s", recList.Body.String())
			}
		})
	}
}

func TestPutAceptaDecisiones(t *testing.T) {
	s := basePrueba(t)
	md := "## Compromiso: Parcial\n- Tipo: puntual\n- Fecha: 2026-04-10\n- Horario: 10-12\n- Carpeta: Física III\n"
	creado := postLote(t, s, md, "Cursada")
	id := fmt.Sprintf("%.0f", creado["lote_id"].(float64))

	status, resp := putImportRaw(t, s, id, map[string]interface{}{
		"markdown": md,
		"decisiones": []map[string]interface{}{
			{"bloque": 1, "accion": "crear", "nombre": "Física III"},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("PUT: status=%d body=%v", status, resp)
	}
	carpetas, _ := db.ListarCarpetas(s.Base)
	if len(carpetas) != 1 {
		t.Fatalf("el PUT debió crear 1: %+v", carpetas)
	}
	fila := carpetaIDDeFila(t, s, "2026-04-10")
	if fila == nil || *fila != carpetas[0].ID {
		t.Fatalf("el PUT debió vincular a %d: %+v", carpetas[0].ID, fila)
	}
	creadas, _ := resp["carpetas_creadas"].([]interface{})
	if len(creadas) != 1 {
		t.Fatalf("carpetas_creadas debió traer 1: %v", resp)
	}

	// PUT con decisión inválida: 400 y todo intacto.
	malo, _ := json.Marshal(map[string]interface{}{
		"markdown": md,
		"decisiones": []map[string]interface{}{
			{"bloque": 1, "accion": "crear", "nombre": ""},
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/agenda/imports/"+id, strings.NewReader(string(malo)))
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	s.actualizarLote(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "obligatorio") {
		t.Fatalf("PUT inválido debió ser 400 en español: %d %s", rec.Code, rec.Body.String())
	}
	if contarAgenda(t, s) != 1 {
		t.Fatal("la fila debió sobrevivir al 400")
	}
	if carpetas, _ := db.ListarCarpetas(s.Base); len(carpetas) != 1 {
		t.Fatalf("la carpeta debió sobrevivir al 400: %+v", carpetas)
	}
	detalle, _, _ := db.ObtenerLote(s.Base, 1)
	if detalle.Markdown != md {
		t.Fatal("el markdown debió quedar intacto")
	}
}

func TestAutoSinDecisionesNoCambia(t *testing.T) {
	s := basePrueba(t)
	_, _ = db.CrearCarpeta(s.Base, "Programación II")
	// Sin decisiones (o auto explícito) rige lo de siempre: existe vincula,
	// faltante queda suelto con aviso.
	status, resp := postImportRaw(t, s, map[string]interface{}{
		"markdown": decisionesMD,
		"decisiones": []map[string]interface{}{
			{"bloque": 1, "accion": "auto"},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("confirm: status=%d body=%v", status, resp)
	}
	if id := carpetaIDDeFila(t, s, "2026-04-10"); id == nil {
		t.Fatal("el bloque 1 debió vincularse solo")
	}
	if id := carpetaIDDeFila(t, s, "2026-06-20"); id != nil {
		t.Fatalf("el bloque 2 debió quedar suelto: %+v", id)
	}
	juntos := fmt.Sprintf("%v", resp["avisos"])
	if !strings.Contains(juntos, `Bloque 2 (Final): carpeta "Física III" no existe, queda suelto`) {
		t.Fatalf("el auto debió avisar el bloque 2: %v", resp["avisos"])
	}
}
