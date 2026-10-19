package api

// sesiones_test.go — /api/terminal/sesiones* y /api/terminal/entorno, más las
// guardas Ventana / Ventana-lectura (design D8). basePrueba: avance_test.go.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"bythos-desktop/db"
	"bythos-desktop/terminal"
)

const origenUI = "http://localhost:8080"

// gestorFalso registra los Cerrar y simula PTYs vivos.
type gestorFalso struct{ cerradas []int64 }

func (g *gestorFalso) Cerrar(id int64) error { g.cerradas = append(g.cerradas, id); return nil }
func (g *gestorFalso) Vivas() []int64        { return []int64{1, 2} }
func (g *gestorFalso) Adjuntar(int64, func() (terminal.Config, error), uint16, uint16) (*terminal.Adjunto, error) {
	return nil, terminal.ErrLimite
}

// pedirTerminal manda una request como la ventana: Host localhost:8080 y el
// Origin dado ("" = sin Origin). cabeceras extra en pares clave, valor.
func pedirTerminal(s *Servidor, metodo, ruta, cuerpo, origen string, extra ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(metodo, "/api/terminal/"+ruta, strings.NewReader(cuerpo))
	req.Host = "localhost:8080"
	if origen != "" {
		req.Header.Set("Origin", origen)
	}
	for i := 0; i+1 < len(extra); i += 2 {
		req.Header.Set(extra[i], extra[i+1])
	}
	rec := httptest.NewRecorder()
	s.Rutas().ServeHTTP(rec, req)
	return rec
}

func ventanaT(s *Servidor, metodo, ruta, cuerpo string) *httptest.ResponseRecorder {
	return pedirTerminal(s, metodo, ruta, cuerpo, origenUI)
}

func crearSesionT(t *testing.T, s *Servidor, cuerpo string) db.Sesion {
	t.Helper()
	rec := ventanaT(s, "POST", "sesiones", cuerpo)
	if rec.Code != http.StatusCreated {
		t.Fatalf("crear sesión: %d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Result().Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("crear sesión: Content-Type %q, la UI necesita JSON", ct)
	}
	var se db.Sesion
	json.Unmarshal(rec.Body.Bytes(), &se)
	return se
}

func TestSesionesCrearListarPatchBorrar(t *testing.T) {
	s := basePrueba(t)
	g := &gestorFalso{}
	s.Terminal = g
	c, _ := db.CrearCarpeta(s.Base, "Mates")

	se := crearSesionT(t, s, `{"nombre":"Álgebra","modo":"practica","carpeta_id":`+strconv.FormatInt(c.ID, 10)+`}`)
	if se.CarpetaID == nil || *se.CarpetaID != c.ID {
		t.Fatalf("carpeta no vinculada: %+v", se)
	}

	rec := pedirTerminal(s, "GET", "sesiones", "", "")
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "escena") || !strings.Contains(rec.Body.String(), "Álgebra") {
		t.Fatalf("lista mal (sin blobs): %d %s", rec.Code, rec.Body.String())
	}

	id := strconv.FormatInt(se.ID, 10)
	if rec := ventanaT(s, "PATCH", "sesiones/"+id, `{"nombre":"Cálculo","carpeta_id":null}`); rec.Code != 200 {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	got, _, _ := db.ObtenerSesion(s.Base, se.ID)
	if got.Nombre != "Cálculo" || got.CarpetaID != nil || got.Modo != "practica" {
		t.Fatalf("patch no soltó la carpeta o pisó el modo: %+v", got)
	}

	if rec := ventanaT(s, "DELETE", "sesiones/"+id, ""); rec.Code != 200 {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if len(g.cerradas) != 1 || g.cerradas[0] != se.ID {
		t.Fatalf("DELETE debía cerrar el PTY primero: %v", g.cerradas)
	}
	if _, ok, _ := db.ObtenerSesion(s.Base, se.ID); ok {
		t.Fatal("la fila sigue tras el DELETE")
	}
	ev, _ := db.ListarEventos(s.Base, db.FiltroEventos{})
	if len(ev) == 0 || ev[0].Accion != db.AccionSesionTerminalBorrada {
		t.Fatalf("faltó el evento de borrado: %+v", ev)
	}
	rec = ventanaT(s, "DELETE", "sesiones/"+id, "")
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), "La sesión "+id+" no existe") {
		t.Fatalf("borrar inexistente: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSesionesValidacion(t *testing.T) {
	s := basePrueba(t) // s.Terminal nil: el DELETE tampoco debe panicar sin gestor
	se := crearSesionT(t, s, `{"nombre":"A","modo":"dibujo"}`)
	if rec := ventanaT(s, "DELETE", "sesiones/"+strconv.FormatInt(se.ID, 10), ""); rec.Code != 200 {
		t.Fatalf("delete sin gestor: %d %s", rec.Code, rec.Body.String())
	}
	rec := ventanaT(s, "POST", "sesiones", `{"nombre":"  ","modo":"practica"}`)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "Escribe un nombre para la sesión.") {
		t.Fatalf("nombre vacío: %d %s", rec.Code, rec.Body.String())
	}
	if rec := ventanaT(s, "POST", "sesiones", `{"nombre":"A","modo":"inventado"}`); rec.Code != 400 {
		t.Fatalf("modo desconocido: %d", rec.Code)
	}
	if rec := ventanaT(s, "POST", "sesiones", `{"nombre":"A","modo":"practica","carpeta_id":999}`); rec.Code != 400 {
		t.Fatalf("carpeta inexistente: %d %s", rec.Code, rec.Body.String())
	}
	if rec := ventanaT(s, "PATCH", "sesiones/999", `{"nombre":"x"}`); rec.Code != 404 {
		t.Fatalf("patch inexistente: %d", rec.Code)
	}
}

func TestPizarraEscenaGetPutYConflicto(t *testing.T) {
	s := basePrueba(t)
	se := crearSesionT(t, s, `{"nombre":"A","modo":"dibujo"}`)
	ruta := "sesiones/" + strconv.FormatInt(se.ID, 10) + "/pizarra"

	rec := pedirTerminal(s, "GET", ruta, "", "")
	var got struct {
		Escena  map[string]json.RawMessage `json:"escena"`
		Version int64                      `json:"version"`
	}
	if json.Unmarshal(rec.Body.Bytes(), &got); rec.Code != 200 || got.Version != 0 || got.Escena["elements"] == nil {
		t.Fatalf("GET pizarra: %d %s", rec.Code, rec.Body.String())
	}

	// Reducción: sin isDeleted, appState solo con el fondo, files vacío.
	cuerpo := `{"version":0,"escena":{"elements":[{"id":"a","type":"rectangle"},{"id":"b","type":"text","isDeleted":true}],
		"appState":{"viewBackgroundColor":"#112233","zoom":3},"files":{"f":{"dataURL":"x"}}}}`
	if rec := ventanaT(s, "PUT", ruta, cuerpo); rec.Code != 200 {
		t.Fatalf("PUT: %d %s", rec.Code, rec.Body.String())
	}
	escena, ver, _, _ := db.ObtenerPizarra(s.Base, se.ID)
	if ver != 1 || strings.Contains(escena, `"b"`) || strings.Contains(escena, "zoom") || strings.Contains(escena, "dataURL") ||
		!strings.Contains(escena, `"viewBackgroundColor":"#112233"`) || !strings.Contains(escena, `"files":{}`) {
		t.Fatalf("escena sin reducir: v%d %s", ver, escena)
	}

	rec = ventanaT(s, "PUT", ruta, `{"version":0,"escena":{"elements":[]}}`)
	var conf struct {
		Version int64 `json:"version"`
	}
	json.Unmarshal(rec.Body.Bytes(), &conf)
	if rec.Code != 409 || conf.Version != 1 {
		t.Fatalf("conflicto: %d %s", rec.Code, rec.Body.String())
	}

	if rec := ventanaT(s, "PUT", ruta, `{"version":1,"escena":{"elements":[{"id":"i","type":"image"}]}}`); rec.Code != 400 {
		t.Fatalf("imagen debía dar 400, dio %d", rec.Code)
	}
	if rec := ventanaT(s, "PUT", "sesiones/999/pizarra", `{"version":0,"escena":{"elements":[]}}`); rec.Code != 404 {
		t.Fatalf("sesión inexistente: %d", rec.Code)
	}
}

func TestPizarraCuerpoGrandeDa413(t *testing.T) {
	s := basePrueba(t)
	se := crearSesionT(t, s, `{"nombre":"A","modo":"dibujo"}`)
	ruta := "sesiones/" + strconv.FormatInt(se.ID, 10) + "/pizarra"
	antes, _, _, _ := db.ObtenerPizarra(s.Base, se.ID)

	grande := `{"version":0,"escena":{"elements":[],"relleno":"` + strings.Repeat("a", 21<<20) + `"}}`
	if rec := ventanaT(s, "PUT", ruta, grande); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("21 MiB debía dar 413, dio %d", rec.Code)
	}
	if despues, ver, _, _ := db.ObtenerPizarra(s.Base, se.ID); despues != antes || ver != 0 {
		t.Fatal("el 413 no debía tocar la escena guardada")
	}
}

func TestEntorno(t *testing.T) {
	s := basePrueba(t)
	s.Terminal = &gestorFalso{}
	rec := pedirTerminal(s, "GET", "entorno", "", "")
	var e map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &e)
	if rec.Code != 200 || e["sesiones"] != float64(2) {
		t.Fatalf("entorno: %d %s", rec.Code, rec.Body.String())
	}
	if _, ok := e["conpty"]; !ok {
		t.Fatalf("falta conpty: %v", e)
	}
	if _, ok := e["engram"]; !ok {
		t.Fatalf("falta engram: %v", e)
	}
}

func TestGuardaVentanaLectura(t *testing.T) {
	s := basePrueba(t)
	casos := []struct {
		origen, fetch string
		quiero        int
	}{
		{"", "", 200}, {"", "same-origin", 200}, {origenUI, "", 200},
		{"chrome-extension://abc", "", 403}, {"", "cross-site", 403},
		{"", "same-site", 403}, {origenUI, "cross-site", 403},
	}
	for _, c := range casos {
		for _, ruta := range []string{"sesiones", "entorno"} {
			if rec := pedirTerminal(s, "GET", ruta, "", c.origen, "Sec-Fetch-Site", c.fetch); rec.Code != c.quiero {
				t.Errorf("%s Origin=%q Sec-Fetch-Site=%q: %d, quería %d", ruta, c.origen, c.fetch, rec.Code, c.quiero)
			}
		}
	}
}

func TestGuardaVentanaMutaciones(t *testing.T) {
	s := basePrueba(t)
	for _, origen := range []string{"", "chrome-extension://abc", "http://localhost:8080.evil.com"} {
		rec := pedirTerminal(s, "POST", "sesiones", `{"nombre":"A","modo":"dibujo"}`, origen)
		if rec.Code != 403 {
			t.Errorf("POST con Origin %q: %d, quería 403", origen, rec.Code)
		}
	}
	if l, _ := db.ListarSesiones(s.Base); len(l) != 0 {
		t.Fatalf("una mutación rechazada creó sesiones: %+v", l)
	}
}

func TestReducirEscenaValidaReferenciasContraAdjuntos(t *testing.T) {
	propio := strings.Repeat("a", 64)
	ajeno := strings.Repeat("b", 64)
	permitidos := map[string]bool{propio: true}
	casos := []struct {
		nombre, escena string
		ok             bool
	}{
		{"imagen propia", `{"type":"image","fileId":"` + propio + `"}`, true},
		{"tarjeta propia", `{"type":"rectangle","customData":{"bythos":"adjunto","sha256":"` + propio + `"}}`, true},
		{"imagen ajena", `{"type":"image","fileId":"` + ajeno + `"}`, false},
		{"imagen sin fileId", `{"type":"image"}`, false},
		{"tarjeta ajena", `{"type":"rectangle","customData":{"sha256":"` + ajeno + `"}}`, false},
		{"embeddable", `{"type":"embeddable","link":"https://ejemplo.com"}`, false},
		{"iframe", `{"type":"iframe"}`, false},
		{"escena heredada", `{"type":"rectangle","customData":{"otro":1}}`, true},
		{"type duplicado en mayúsculas", `{"type":"embeddable","Type":"rectangle"}`, false},
		{"fileId duplicado en mayúsculas", `{"type":"image","fileId":"` + ajeno + `","FILEID":"` + propio + `"}`, false},
		{"sha256 duplicado en mayúsculas", `{"type":"rectangle","customData":{"sha256":"` + ajeno + `","SHA256":"` + propio + `"}}`, false},
		{"rectángulo con fileId ajeno", `{"type":"rectangle","fileId":"` + ajeno + `"}`, false},
		{"sha256 vacío", `{"type":"rectangle","customData":{"sha256":""}}`, false},
		{"fileId no string", `{"type":"image","fileId":5}`, false},
		{"sha256 no string", `{"type":"rectangle","customData":{"sha256":5}}`, false},
		{"magicframe", `{"type":"magicframe"}`, false},
		{"enlace y frame heredados", `{"type":"frame","link":"https://ejemplo.com"}`, true},
		{"marca legítima sin sha256", `{"type":"rectangle","customData":{"bythos":"marca","veredicto":"ok","clave":"area:x"}}`, true},
		{"marca con sha256 ajeno", `{"type":"rectangle","customData":{"bythos":"marca","sha256":"` + ajeno + `"}}`, false},
		{"marca con sha256 de adjunto propio", `{"type":"rectangle","customData":{"bythos":"marca","sha256":"` + propio + `"}}`, false},
		{"marca con fileId de adjunto propio", `{"type":"image","fileId":"` + propio + `","customData":{"bythos":"marca"}}`, false},
		{"marca con Bythos en mayúsculas", `{"type":"rectangle","customData":{"Bythos":"marca","sha256":"` + propio + `"}}`, false},
		{"marca con BYTHOS y bythos duplicados", `{"type":"rectangle","customData":{"bythos":"adjunto","BYTHOS":"marca","sha256":"` + propio + `"}}`, false},
		{"tarjeta de adjunto heredada", `{"type":"rectangle","customData":{"bythos":"adjunto","sha256":"` + propio + `"}}`, true},
	}
	for _, c := range casos {
		salida, ok := reducirEscena(json.RawMessage(`{"elements":[`+c.escena+`]}`), permitidos)
		if ok != c.ok {
			t.Errorf("%s: ok = %v, quería %v", c.nombre, ok, c.ok)
		}
		if ok && !strings.Contains(salida, `"files":{}`) {
			t.Errorf("%s: la escena guardada debe llevar files vacío: %s", c.nombre, salida)
		}
	}
	// Elementos borrados no cuentan: una imagen ajena ya borrada se descarta sin error.
	if salida, ok := reducirEscena(json.RawMessage(`{"elements":[{"type":"image","fileId":"`+ajeno+`","isDeleted":true}]}`), permitidos); !ok || strings.Contains(salida, ajeno) {
		t.Errorf("imagen borrada: ok=%v %s", ok, salida)
	}
}

func TestPizarraPutConAdjuntosPropiosAjenosYBytes(t *testing.T) {
	s, se := prepararAdjuntos(t)
	otra := crearSesionT(t, s, `{"nombre":"B","modo":"dibujo"}`)
	propio := decodificarAdj(t, subirAdj(t, s, se.ID, origenUI, "a.png", pngAdj+"propio"))["sha256"].(string)
	ajeno := decodificarAdj(t, subirAdj(t, s, otra.ID, origenUI, "b.png", pngAdj+"ajeno"))["sha256"].(string)
	ruta := "sesiones/" + strconv.FormatInt(se.ID, 10) + "/pizarra"

	ok := `{"version":0,"escena":{"elements":[{"id":"i","type":"image","fileId":"` + propio + `"}],"files":{"` + propio + `":{"dataURL":"data:x"}}}}`
	if rec := ventanaT(s, "PUT", ruta, ok); rec.Code != 200 {
		t.Fatalf("imagen propia: %d %s", rec.Code, rec.Body.String())
	}
	guardada, _, _, _ := db.ObtenerPizarra(s.Base, se.ID)
	if strings.Contains(guardada, "dataURL") || !strings.Contains(guardada, `"files":{}`) {
		t.Fatalf("los bytes no se descartaron: %s", guardada)
	}
	mal := `{"version":1,"escena":{"elements":[{"id":"i","type":"image","fileId":"` + ajeno + `"}]}}`
	if rec := ventanaT(s, "PUT", ruta, mal); rec.Code != 400 {
		t.Fatalf("imagen ajena debía dar 400, dio %d", rec.Code)
	}
	if despues, ver, _, _ := db.ObtenerPizarra(s.Base, se.ID); despues != guardada || ver != 1 {
		t.Fatal("el 400 no debía tocar la escena guardada")
	}
}

func TestBorrarSesionConservaArchivosYOtrasSesiones(t *testing.T) {
	s, a := prepararAdjuntos(t)
	b := crearSesionT(t, s, `{"nombre":"B","modo":"dibujo"}`)
	subirAdj(t, s, a.ID, origenUI, "a.png", pngAdj+"comun")
	subirAdj(t, s, b.ID, origenUI, "b.png", pngAdj+"comun")
	solo := decodificarAdj(t, subirAdj(t, s, a.ID, origenUI, "solo.png", pngAdj+"solo de A"))["sha256"].(string)
	if rec := ventanaT(s, "DELETE", "sesiones/"+strconv.FormatInt(a.ID, 10), ""); rec.Code != 200 {
		t.Fatalf("borrar: %d %s", rec.Code, rec.Body.String())
	}
	if l, _ := db.ListarAdjuntos(s.Base, a.ID); len(l) != 0 {
		t.Fatalf("quedaron %d vínculos de la sesión borrada", len(l))
	}
	lista, _ := db.ListarAdjuntos(s.Base, b.ID)
	if len(lista) != 1 || blobsEnDisco(t) != 2 {
		t.Fatalf("la otra sesión perdió el adjunto o se borró un blob: %d vínculos, %d blobs", len(lista), blobsEnDisco(t))
	}
	if _, ok, _ := db.ObtenerArchivo(s.Base, lista[0].ArchivoID); !ok {
		t.Fatal("la fila de archivos debía quedar")
	}
	if _, ok, _ := db.ObtenerArchivoPorSHA256(s.Base, solo); !ok {
		t.Fatal("la fila del adjunto exclusivo de A debía quedar")
	}
	rec := pedirTerminal(s, "GET", rutaAdj(b.ID, "/"+strconv.FormatInt(lista[0].ID, 10)+"/contenido"), "", "", "Sec-Fetch-Site", "same-origin")
	if rec.Code != 200 {
		t.Fatalf("contenido de la otra sesión: %d", rec.Code)
	}
}
