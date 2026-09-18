package api

// import_lotes_test.go — Import batches over HTTP (httptest, no network).
// Covers the lote lifecycle: POST creates source + rows, PUT re-expands,
// DELETE removes only its own rows. Table-driven per go-testing skill.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bythos-desktop/db"
)

const loteMDUno = "## Compromiso: Parcial\n- Tipo: puntual\n- Fecha: 2026-04-10\n- Horario: 10-12\n"

const loteMDDos = "## Compromiso: Parcial\n- Tipo: puntual\n- Fecha: 2026-04-10\n- Horario: 10-12\n" +
	"\n## Compromiso: Final\n- Tipo: puntual\n- Fecha: 2026-06-20\n- Horario: 9-11\n"

// postLote imports markdown through the real handler and returns the body.
func postLote(t *testing.T, s *Servidor, markdown, nombre string) map[string]interface{} {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"markdown": markdown, "nombre": nombre})
	req := httptest.NewRequest(http.MethodPost, "/api/agenda/import", strings.NewReader(string(payload)))
	rec := httptest.NewRecorder()
	s.importarAgenda(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST import: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("POST import no JSON: %v", err)
	}
	return resp
}

func contarAgenda(t *testing.T, s *Servidor) int {
	t.Helper()
	rows, err := db.ListarAgenda(s.Base, "", "")
	if err != nil {
		t.Fatalf("ListarAgenda: %v", err)
	}
	return len(rows)
}

func TestImportCreaLoteConNombre(t *testing.T) {
	s := basePrueba(t)
	resp := postLote(t, s, loteMDUno, "Cursada 2026")

	loteID, ok := resp["lote_id"].(float64)
	if !ok || loteID == 0 {
		t.Fatalf("falta lote_id en: %v", resp)
	}
	if resp["nombre"] != "Cursada 2026" {
		t.Fatalf("nombre mal en: %v", resp)
	}
	if int(resp["creadas"].(float64)) != 1 || int(resp["total"].(float64)) != 1 {
		t.Fatalf("creadas/total mal en: %v", resp)
	}

	// List shows it with its live count.
	reqList := httptest.NewRequest(http.MethodGet, "/api/agenda/imports", nil)
	recList := httptest.NewRecorder()
	s.listarLotes(recList, reqList)
	var lista []map[string]interface{}
	if err := json.Unmarshal(recList.Body.Bytes(), &lista); err != nil || len(lista) != 1 {
		t.Fatalf("lista mal: %s err=%v", recList.Body.String(), err)
	}
	if lista[0]["nombre"] != "Cursada 2026" || int(lista[0]["total"].(float64)) != 1 {
		t.Fatalf("item mal: %v", lista[0])
	}

	// Detail carries the source markdown for editing.
	reqGet := httptest.NewRequest(http.MethodGet, "/api/agenda/imports/1", nil)
	reqGet.SetPathValue("id", "1")
	recGet := httptest.NewRecorder()
	s.obtenerLote(recGet, reqGet)
	var detalle map[string]interface{}
	if err := json.Unmarshal(recGet.Body.Bytes(), &detalle); err != nil {
		t.Fatalf("detalle no JSON: %v", err)
	}
	if detalle["markdown"] != loteMDUno {
		t.Fatalf("markdown mal: %v", detalle)
	}

	// Unknown id is 404 in Spanish.
	req404 := httptest.NewRequest(http.MethodGet, "/api/agenda/imports/9999", nil)
	req404.SetPathValue("id", "9999")
	rec404 := httptest.NewRecorder()
	s.obtenerLote(rec404, req404)
	if rec404.Code != http.StatusNotFound || !strings.Contains(rec404.Body.String(), "Lote") {
		t.Fatalf("ausente debió ser 404 en español: %d %s", rec404.Code, rec404.Body.String())
	}
}

func TestImportNombrePorDefecto(t *testing.T) {
	casos := []struct {
		nombre   string
		markdown string
		quiere   string // expected nombre (exact or prefix with "*")
		creadas  int
	}{
		{"usa el primer compromiso", loteMDUno, "Parcial", 1},
		{"sin bloques usa timestamp", "- Tipo: puntual\n- Fecha: 2026-04-10\n", "Import *", 0},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			s := basePrueba(t)
			resp := postLote(t, s, tt.markdown, "")
			if int(resp["creadas"].(float64)) != tt.creadas {
				t.Fatalf("creadas mal en: %v", resp)
			}
			got, _ := resp["nombre"].(string)
			if strings.HasSuffix(tt.quiere, "*") {
				if !strings.HasPrefix(got, strings.TrimSuffix(tt.quiere, "*")) {
					t.Fatalf("nombre %q debió empezar con %q", got, tt.quiere)
				}
				return
			}
			if got != tt.quiere {
				t.Fatalf("nombre=%q, quería %q", got, tt.quiere)
			}
		})
	}
}

func TestImportRepetidoCreaLoteVacio(t *testing.T) {
	s := basePrueba(t)
	primera := postLote(t, s, loteMDUno, "Uno")
	segunda := postLote(t, s, loteMDUno, "Dos")
	if int(segunda["creadas"].(float64)) != 0 || int(segunda["omitidas"].(float64)) != 1 {
		t.Fatalf("la repetida debió omitirse contra filas globales: %v", segunda)
	}
	if primera["lote_id"] == segunda["lote_id"] {
		t.Fatal("cada import es su propio lote")
	}
	// Second lote exists but owns zero rows.
	req := httptest.NewRequest(http.MethodGet, "/api/agenda/imports", nil)
	rec := httptest.NewRecorder()
	s.listarLotes(rec, req)
	var lista []map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &lista)
	if len(lista) != 2 {
		t.Fatalf("debió haber 2 lotes: %v", lista)
	}
	if int(lista[0]["total"].(float64)) != 0 || int(lista[1]["total"].(float64)) != 1 {
		t.Fatalf("cuentas mal (nuevo primero): %v", lista)
	}
}

func TestImportPreviewNoCreaLote(t *testing.T) {
	s := basePrueba(t)
	payload, _ := json.Marshal(map[string]string{"markdown": loteMDUno, "nombre": "X"})
	req := httptest.NewRequest(http.MethodPost, "/api/agenda/import?dry=true", strings.NewReader(string(payload)))
	rec := httptest.NewRecorder()
	s.importarAgenda(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if contarAgenda(t, s) != 0 {
		t.Fatal("dry=true no debe guardar filas")
	}
	reqList := httptest.NewRequest(http.MethodGet, "/api/agenda/imports", nil)
	recList := httptest.NewRecorder()
	s.listarLotes(recList, reqList)
	if strings.TrimSpace(recList.Body.String()) != "[]" {
		t.Fatalf("dry=true no debe crear lotes: %s", recList.Body.String())
	}
}

func TestPutReexpandeCambiaTotal(t *testing.T) {
	s := basePrueba(t)
	resp := postLote(t, s, loteMDUno, "Cursada")
	id := fmt.Sprintf("%.0f", resp["lote_id"].(float64))

	// Same markdown re-saved restores the same single row (idempotent).
	mismo, _ := json.Marshal(map[string]string{"markdown": loteMDUno})
	reqMismo := httptest.NewRequest(http.MethodPut, "/api/agenda/imports/"+id, strings.NewReader(string(mismo)))
	reqMismo.SetPathValue("id", id)
	recMismo := httptest.NewRecorder()
	s.actualizarLote(recMismo, reqMismo)
	var rMismo map[string]interface{}
	_ = json.Unmarshal(recMismo.Body.Bytes(), &rMismo)
	if recMismo.Code != http.StatusOK || int(rMismo["creadas"].(float64)) != 1 {
		t.Fatalf("re-guardar igual debió recrear 1: %d %v", recMismo.Code, rMismo)
	}
	if contarAgenda(t, s) != 1 {
		t.Fatalf("debió quedar 1 fila, hay %d", contarAgenda(t, s))
	}

	// New markdown with two blocks changes the total; old rows are gone.
	nuevo, _ := json.Marshal(map[string]string{"markdown": loteMDDos, "nombre": "Cursada final"})
	req := httptest.NewRequest(http.MethodPut, "/api/agenda/imports/"+id, strings.NewReader(string(nuevo)))
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	s.actualizarLote(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var r map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &r)
	if int(r["creadas"].(float64)) != 2 || int(r["total"].(float64)) != 2 {
		t.Fatalf("PUT debió crear 2: %v", r)
	}
	if r["nombre"] != "Cursada final" {
		t.Fatalf("nombre no se actualizó: %v", r)
	}
	if contarAgenda(t, s) != 2 {
		t.Fatalf("debieron quedar 2 filas, hay %d", contarAgenda(t, s))
	}
	// Old date is gone, new date exists.
	viejas, _ := db.ListarAgenda(s.Base, "2026-04-10", "2026-04-10")
	if len(viejas) != 1 {
		t.Fatalf("el 10/4 debió seguir con 1 fila: %+v", viejas)
	}
	nuevas, _ := db.ListarAgenda(s.Base, "2026-06-20", "2026-06-20")
	if len(nuevas) != 1 || nuevas[0].Texto != "Final" {
		t.Fatalf("el 20/6 debió traer a Final: %+v", nuevas)
	}
	// Lote detail kept the new source.
	detalle, existe, err := db.ObtenerLote(s.Base, 1)
	if err != nil || !existe || detalle.Markdown != loteMDDos || detalle.Nombre != "Cursada final" {
		t.Fatalf("detalle post-PUT mal: %+v existe=%v err=%v", detalle, existe, err)
	}
}

func TestPutSinEventosEs400SinTocar(t *testing.T) {
	s := basePrueba(t)
	resp := postLote(t, s, loteMDUno, "Cursada")
	id := fmt.Sprintf("%.0f", resp["lote_id"].(float64))

	malo, _ := json.Marshal(map[string]string{"markdown": "## Compromiso: Roto\n- Tipo: sin-esto\n"})
	req := httptest.NewRequest(http.MethodPut, "/api/agenda/imports/"+id, strings.NewReader(string(malo)))
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	s.actualizarLote(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT roto debió ser 400, salió %d: %s", rec.Code, rec.Body.String())
	}
	var r map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &r)
	if _, ok := r["avisos"]; !ok {
		t.Fatalf("el 400 debe traer avisos: %s", rec.Body.String())
	}
	// Nothing was touched: row and source intact.
	if contarAgenda(t, s) != 1 {
		t.Fatalf("la fila debió sobrevivir, hay %d", contarAgenda(t, s))
	}
	detalle, _, _ := db.ObtenerLote(s.Base, 1)
	if detalle.Markdown != loteMDUno {
		t.Fatal("el markdown debió quedar intacto")
	}
}

func TestPutCasosBorde(t *testing.T) {
	casos := []struct {
		nombre  string
		id      string
		payload string
		status  int
		frag    string
	}{
		{"lote ausente es 404", "9999", `{"markdown":"## Compromiso: X\n- Tipo: puntual\n- Fecha: 2026-04-10\n- Horario: 10-11\n"}`, http.StatusNotFound, "Lote"},
		{"markdown vacío es 400", "1", `{"markdown":"   "}`, http.StatusBadRequest, "vacío"},
		{"json roto es 400", "1", `{markdown:`, http.StatusBadRequest, "vacío"},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			s := basePrueba(t)
			_ = postLote(t, s, loteMDUno, "Cursada")
			req := httptest.NewRequest(http.MethodPut, "/api/agenda/imports/"+tt.id, strings.NewReader(tt.payload))
			req.SetPathValue("id", tt.id)
			rec := httptest.NewRecorder()
			s.actualizarLote(rec, req)
			if rec.Code != tt.status || !strings.Contains(rec.Body.String(), tt.frag) {
				t.Fatalf("status=%d body=%s, quería %d con %q", rec.Code, rec.Body.String(), tt.status, tt.frag)
			}
		})
	}
}

func TestDeleteBorraLoteYFilasSinTocarSueltas(t *testing.T) {
	s := basePrueba(t)
	resp := postLote(t, s, loteMDDos, "Cursada")
	id := fmt.Sprintf("%.0f", resp["lote_id"].(float64))
	suelta, err := db.CrearAgenda(s.Base, "2026-07-01", "", "", "Nota a mano", nil)
	if err != nil {
		t.Fatalf("suelta: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/agenda/imports/"+id, nil)
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	s.borrarLote(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var r map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &r)
	if int(r["borradas"].(float64)) != 2 {
		t.Fatalf("borradas debió ser 2: %v", r)
	}
	// Only the hand-made row survives.
	quedan, _ := db.ListarAgenda(s.Base, "", "")
	if len(quedan) != 1 || quedan[0].ID != suelta.ID {
		t.Fatalf("solo la suelta debió sobrevivir: %+v", quedan)
	}
	// Gone lote reads 404; deleting twice reports 404.
	reqGet := httptest.NewRequest(http.MethodGet, "/api/agenda/imports/"+id, nil)
	reqGet.SetPathValue("id", id)
	recGet := httptest.NewRecorder()
	s.obtenerLote(recGet, reqGet)
	if recGet.Code != http.StatusNotFound {
		t.Fatalf("detalle post-borrado debió ser 404: %d", recGet.Code)
	}
	reqDel2 := httptest.NewRequest(http.MethodDelete, "/api/agenda/imports/"+id, nil)
	reqDel2.SetPathValue("id", id)
	recDel2 := httptest.NewRecorder()
	s.borrarLote(recDel2, reqDel2)
	if recDel2.Code != http.StatusNotFound {
		t.Fatalf("segundo DELETE debió ser 404: %d %s", recDel2.Code, recDel2.Body.String())
	}
	reqDel3 := httptest.NewRequest(http.MethodDelete, "/api/agenda/imports/9999", nil)
	reqDel3.SetPathValue("id", "9999")
	recDel3 := httptest.NewRecorder()
	s.borrarLote(recDel3, reqDel3)
	if recDel3.Code != http.StatusNotFound {
		t.Fatalf("DELETE ausente debió ser 404: %d", recDel3.Code)
	}
}

func TestLotesRutasWire(t *testing.T) {
	s := basePrueba(t)
	payload, _ := json.Marshal(map[string]string{"markdown": loteMDUno, "nombre": "Wire"})

	// POST through the real mux (no :8080, no network).
	req := httptest.NewRequest(http.MethodPost, "/api/agenda/import", strings.NewReader(string(payload)))
	rec := httptest.NewRecorder()
	s.Rutas().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "lote_id") {
		t.Fatalf("Rutas POST import mal: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var creado map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &creado)
	id := fmt.Sprintf("%.0f", creado["lote_id"].(float64))

	// GET list through the mux.
	reqList := httptest.NewRequest(http.MethodGet, "/api/agenda/imports", nil)
	recList := httptest.NewRecorder()
	s.Rutas().ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK || !strings.Contains(recList.Body.String(), "Wire") {
		t.Fatalf("Rutas GET imports mal: status=%d body=%s", recList.Code, recList.Body.String())
	}

	// GET detail through the mux.
	reqGet := httptest.NewRequest(http.MethodGet, "/api/agenda/imports/"+id, nil)
	recGet := httptest.NewRecorder()
	s.Rutas().ServeHTTP(recGet, reqGet)
	if recGet.Code != http.StatusOK || !strings.Contains(recGet.Body.String(), "markdown") {
		t.Fatalf("Rutas GET detalle mal: status=%d body=%s", recGet.Code, recGet.Body.String())
	}

	// PUT through the mux (proves the method is registered + CORS allows it).
	nuevo, _ := json.Marshal(map[string]string{"markdown": loteMDUno})
	reqPut := httptest.NewRequest(http.MethodPut, "/api/agenda/imports/"+id, strings.NewReader(string(nuevo)))
	recPut := httptest.NewRecorder()
	s.Rutas().ServeHTTP(recPut, reqPut)
	if recPut.Code != http.StatusOK || !strings.Contains(recPut.Body.String(), "creadas") {
		t.Fatalf("Rutas PUT mal: status=%d body=%s", recPut.Code, recPut.Body.String())
	}

	// DELETE through the mux.
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/agenda/imports/"+id, nil)
	recDel := httptest.NewRecorder()
	s.Rutas().ServeHTTP(recDel, reqDel)
	if recDel.Code != http.StatusOK || !strings.Contains(recDel.Body.String(), "borradas") {
		t.Fatalf("Rutas DELETE mal: status=%d body=%s", recDel.Code, recDel.Body.String())
	}
}
