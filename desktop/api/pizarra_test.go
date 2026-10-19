package api

// pizarra_test.go — latido, ops, capturas y resolución de sesión del agente
// (design D8 Agente, D12, D13). Los tiempos de espera son campos inyectables:
// ningún test duerme segundos de verdad.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bythos-desktop/db"
)

const (
	textoPizarraCerrada  = "La pizarra de esta sesión no está abierta: abre la pizarra en la pantalla Terminal de Bythos y vuelve a intentarlo."
	textoTerminalCerrada = "La pantalla Terminal no está abierta: ábrela en Bythos para capturar la pizarra."
	pngMagic             = "\x89PNG\r\n\x1a\n"
	opBorrar             = `{"ops":[{"accion":"agregar","elemento":{"tipo":"rectangle","x":1,"y":2}}]}`
)

type latidoResp struct {
	Ops []struct {
		ID int64           `json:"id"`
		Op json.RawMessage `json:"op"`
	} `json:"ops"`
	Capturas []int64 `json:"capturas"`
}

func agenteT(s *Servidor, metodo, ruta, cuerpo string) *httptest.ResponseRecorder {
	return pedirTerminal(s, metodo, ruta, cuerpo, "")
}

func sesionMontada(t *testing.T, s *Servidor, nombre string) db.Sesion {
	t.Helper()
	se, err := db.CrearSesion(s.Base, nombre, "dibujo", nil)
	if err != nil {
		t.Fatal(err)
	}
	latir(t, s, se.ID)
	return se
}

func latir(t *testing.T, s *Servidor, id int64) latidoResp {
	t.Helper()
	rec := ventanaT(s, "POST", "latido", fmt.Sprintf(`{"sesion_id":%d}`, id))
	if rec.Code != 200 {
		t.Fatalf("latido: %d %s", rec.Code, rec.Body.String())
	}
	var r latidoResp
	json.Unmarshal(rec.Body.Bytes(), &r)
	return r
}

// esperarLatido repite el latido (como la UI, cada 1 s) hasta que llegue lo que se espera.
func esperarLatido(t *testing.T, s *Servidor, id int64, listo func(latidoResp) bool) latidoResp {
	t.Helper()
	for i := 0; i < 300; i++ {
		if r := latir(t, s, id); listo(r) {
			return r
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("el latido nunca trajo lo esperado")
	return latidoResp{}
}

// asincrono lanza la llamada del agente y devuelve el canal de su respuesta.
func asincrono(s *Servidor, ruta, cuerpo string) <-chan *httptest.ResponseRecorder {
	c := make(chan *httptest.ResponseRecorder, 1)
	go func() { c <- agenteT(s, "POST", ruta, cuerpo) }()
	return c
}

func json2(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("respuesta no es JSON: %v %q", err, rec.Body.String())
	}
	return m
}

func contarOps(s *Servidor) (n int) {
	s.Base.QueryRow(`SELECT COUNT(*) FROM terminal_pizarra_ops`).Scan(&n)
	return n
}

func ack(s *Servidor, id int64, cuerpo string) *httptest.ResponseRecorder {
	return ventanaT(s, "POST", fmt.Sprintf("pizarra/ops/%d/ack", id), cuerpo)
}

func TestOpsAckDentroDeLaEsperaDevuelveAplicadaConIds(t *testing.T) {
	s := basePrueba(t)
	se := sesionMontada(t, s, "A")
	resp := asincrono(s, "pizarra/ops", fmt.Sprintf(`{"sesion_id":%d,%s`, se.ID, opBorrar[1:]))

	l := esperarLatido(t, s, se.ID, func(r latidoResp) bool { return len(r.Ops) == 1 })
	if rec := ack(s, l.Ops[0].ID, `{"ok":true,"ids":["e1"]}`); rec.Code != 200 {
		t.Fatalf("ack: %d %s", rec.Code, rec.Body.String())
	}
	rec := <-resp
	m := json2(t, rec)
	if rec.Code != 200 || m["estado"] != "aplicada" || m["resolucion"] != "argumento" || fmt.Sprint(m["ids"]) != "[e1]" {
		t.Fatalf("respuesta: %d %v", rec.Code, m)
	}
	if rec.Result().Header.Get("Content-Type") != "application/json" {
		t.Fatal("Content-Type debía ser JSON")
	}
	ev, _ := db.ListarEventos(s.Base, db.FiltroEventos{})
	if len(ev) != 1 || ev[0].Accion != db.AccionPizarraEditada || ev[0].EntidadID != se.ID || ev[0].Origen != db.OrigenAgente {
		t.Fatalf("eventos: %+v", ev)
	}
}

// Aviso perdido: el ack se guarda en la BD sin llegar al canal (como si la UI
// respondiera antes de esperarOp). Al vencer la espera manda la fila.
func TestOpsAckSinAvisoSeLeeDeLaBD(t *testing.T) {
	s := basePrueba(t)
	s.pizarra.esperaOps = 300 * time.Millisecond
	se := sesionMontada(t, s, "A")
	resp := asincrono(s, "pizarra/ops", fmt.Sprintf(`{"sesion_id":%d,%s`, se.ID, opBorrar[1:]))
	l := esperarLatido(t, s, se.ID, func(r latidoResp) bool { return len(r.Ops) == 1 })
	if _, err := db.ResponderOp(s.Base, l.Ops[0].ID, true, `{"ids":["e9"]}`); err != nil {
		t.Fatal(err)
	}
	m := json2(t, <-resp)
	if m["estado"] != "aplicada" || fmt.Sprint(m["ids"]) != "[e9]" || m["mensaje"] != nil {
		t.Fatalf("debía leer la aplicada de la BD: %v", m)
	}
}

func TestOpsFallidaSeReportaAlAgente(t *testing.T) {
	s := basePrueba(t)
	se := sesionMontada(t, s, "A")
	resp := asincrono(s, "pizarra/ops", fmt.Sprintf(`{"sesion_id":%d,%s`, se.ID, opBorrar[1:]))
	l := esperarLatido(t, s, se.ID, func(r latidoResp) bool { return len(r.Ops) == 1 })
	ack(s, l.Ops[0].ID, `{"ok":false,"error":"no existe el id z"}`)
	m := json2(t, <-resp)
	if m["estado"] != "fallida" || m["error"] != "no existe el id z" {
		t.Fatalf("respuesta: %v", m)
	}
	if ev, _ := db.ListarEventos(s.Base, db.FiltroEventos{}); len(ev) != 0 {
		t.Fatalf("una fallida no registra pizarra_editada: %+v", ev)
	}
}

func TestOpsSinAckDevuelvePendienteYLaReentregaEsIdempotente(t *testing.T) {
	s := basePrueba(t)
	s.pizarra.esperaOps = 30 * time.Millisecond
	se := sesionMontada(t, s, "A")

	rec := agenteT(s, "POST", "pizarra/ops", fmt.Sprintf(`{"sesion_id":%d,%s`, se.ID, opBorrar[1:]))
	m := json2(t, rec)
	if rec.Code != 200 || m["estado"] != "pendiente" || !strings.Contains(fmt.Sprint(m["mensaje"]), "se aplicará en segundos si la pizarra sigue abierta") {
		t.Fatalf("respuesta: %d %v", rec.Code, m)
	}
	// Ack perdido: el latido sigue entregando el MISMO lote con los MISMOS ids.
	a, b := latir(t, s, se.ID), latir(t, s, se.ID)
	if len(a.Ops) != 1 || len(b.Ops) != 1 || a.Ops[0].ID != b.Ops[0].ID || string(a.Ops[0].Op) != string(b.Ops[0].Op) {
		t.Fatalf("reentrega distinta: %+v %+v", a, b)
	}
	if !strings.Contains(string(a.Ops[0].Op), `"id":`) {
		t.Fatalf("el servidor debía asignar el id del elemento: %s", a.Ops[0].Op)
	}
	// El ack tardío (dentro de los 30 s) aún aplica y registra el evento.
	if rec := ack(s, a.Ops[0].ID, `{"ok":true,"ids":["x"]}`); rec.Code != 200 {
		t.Fatalf("ack tardío: %d", rec.Code)
	}
	if ev, _ := db.ListarEventos(s.Base, db.FiltroEventos{}); len(ev) != 1 {
		t.Fatalf("eventos: %+v", ev)
	}
}

func TestOpsVencidaNuncaSeEntrega(t *testing.T) {
	s := basePrueba(t)
	s.pizarra.esperaOps = 10 * time.Millisecond
	se := sesionMontada(t, s, "A")
	agenteT(s, "POST", "pizarra/ops", fmt.Sprintf(`{"sesion_id":%d,%s`, se.ID, opBorrar[1:]))
	s.Base.Exec(`UPDATE terminal_pizarra_ops SET creado = datetime('now','localtime','-31 seconds')`)

	if l := latir(t, s, se.ID); len(l.Ops) != 0 {
		t.Fatalf("una vencida no se entrega: %+v", l)
	}
	if rec := ack(s, 1, `{"ok":true}`); rec.Code != http.StatusConflict {
		t.Fatalf("ack de una vencida = %d, debía ser 409", rec.Code)
	}
	if rec := ack(s, 999, `{"ok":true}`); rec.Code != http.StatusNotFound {
		t.Fatalf("ack de una op inexistente = %d, debía ser 404", rec.Code)
	}
}

func TestOpsSesionNoMontadaDa409YNoEncola(t *testing.T) {
	s := basePrueba(t)
	ahora := time.Now()
	s.pizarra.ahora = func() time.Time { return ahora }
	se := sesionMontada(t, s, "A")
	cuerpo := fmt.Sprintf(`{"sesion_id":%d,%s`, se.ID, opBorrar[1:])

	ahora = ahora.Add(4 * time.Second) // el último latido tiene 4 s: ya no está montada
	rec := agenteT(s, "POST", "pizarra/ops", cuerpo)
	if rec.Code != 409 || json2(t, rec)["error"] != textoPizarraCerrada || contarOps(s) != 0 {
		t.Fatalf("respuesta: %d %s, filas=%d", rec.Code, rec.Body.String(), contarOps(s))
	}
	// Otra sesión sin latido propio tampoco cuenta como montada.
	otra, _ := db.CrearSesion(s.Base, "B", "dibujo", nil)
	latir(t, s, se.ID)
	if rec := agenteT(s, "POST", "pizarra/ops", fmt.Sprintf(`{"sesion_id":%d,%s`, otra.ID, opBorrar[1:])); rec.Code != 409 {
		t.Fatalf("sesión sin latido = %d", rec.Code)
	}
}

func TestOps201OperacionesDa400YNoEncola(t *testing.T) {
	s := basePrueba(t)
	se := sesionMontada(t, s, "A")
	ops := strings.Repeat(`{"accion":"borrar","ids":["a"]},`, 200) + `{"accion":"borrar","ids":["a"]}`
	rec := agenteT(s, "POST", "pizarra/ops", fmt.Sprintf(`{"sesion_id":%d,"ops":[%s]}`, se.ID, ops))
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "200") || contarOps(s) != 0 {
		t.Fatalf("respuesta: %d %s, filas=%d", rec.Code, rec.Body.String(), contarOps(s))
	}
}

func TestOpsNoRetieneLaConexionMientrasEspera(t *testing.T) {
	s := basePrueba(t) // SetMaxOpenConns(1)
	s.pizarra.esperaOps = 3 * time.Second
	se := sesionMontada(t, s, "A")
	resp := asincrono(s, "pizarra/ops", fmt.Sprintf(`{"sesion_id":%d,%s`, se.ID, opBorrar[1:]))
	l := esperarLatido(t, s, se.ID, func(r latidoResp) bool { return len(r.Ops) == 1 })

	inicio := time.Now()
	if rec := agenteT(s, "GET", fmt.Sprintf("pizarra?sesion_id=%d", se.ID), ""); rec.Code != 200 || time.Since(inicio) > time.Second {
		t.Fatalf("la lectura concurrente quedó bloqueada: %d en %v", rec.Code, time.Since(inicio))
	}
	ack(s, l.Ops[0].ID, `{"ok":true,"ids":["e"]}`)
	<-resp
}

// --- capturas (design D13) ---

func TestCapturaNoMontadaDa409Inmediato(t *testing.T) {
	s := basePrueba(t)
	se, _ := db.CrearSesion(s.Base, "A", "dibujo", nil)
	inicio := time.Now()
	rec := agenteT(s, "POST", "pizarra/captura", fmt.Sprintf(`{"sesion_id":%d}`, se.ID))
	if rec.Code != 409 || json2(t, rec)["error"] != textoTerminalCerrada || time.Since(inicio) > time.Second {
		t.Fatalf("respuesta: %d %s", rec.Code, rec.Body.String())
	}
}

func subirCaptura(s *Servidor, id int64, cuerpo string) *httptest.ResponseRecorder {
	return ventanaT(s, "POST", fmt.Sprintf("pizarra/capturas/%d", id), cuerpo)
}

func TestCapturaMontadaRecibeElPNGYValidaMagicYTamano(t *testing.T) {
	s := basePrueba(t)
	se := sesionMontada(t, s, "A")
	resp := asincrono(s, "pizarra/captura", fmt.Sprintf(`{"sesion_id":%d}`, se.ID))
	l := esperarLatido(t, s, se.ID, func(r latidoResp) bool { return len(r.Capturas) == 1 })
	id := l.Capturas[0]
	if otra := latir(t, s, se.ID); len(otra.Capturas) != 0 {
		t.Fatalf("la captura se entrega una sola vez: %+v", otra)
	}

	if rec := subirCaptura(s, id, "<html>no soy png"); rec.Code != 400 {
		t.Fatalf("no-PNG = %d, debía ser 400", rec.Code)
	}
	if rec := subirCaptura(s, id, pngMagic+strings.Repeat("0", 4<<20)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("PNG de más de 4 MiB = %d, debía ser 413", rec.Code)
	}
	png := pngMagic + "datos"
	if rec := subirCaptura(s, id, png); rec.Code != 200 {
		t.Fatalf("PNG válido: %d %s", rec.Code, rec.Body.String())
	}
	rec := <-resp
	r := rec.Result()
	if rec.Code != 200 || r.Header.Get("Content-Type") != "image/png" || !bytes.Equal(rec.Body.Bytes(), []byte(png)) ||
		r.Header.Get("X-Bythos-Resolucion") != "argumento" || r.Header.Get("X-Bythos-Sesion") != fmt.Sprint(se.ID) || r.Header.Get("X-Bythos-Captura") != fmt.Sprint(id) {
		t.Fatalf("respuesta: %d %v %q", rec.Code, r.Header, rec.Body.String())
	}
	if rec := subirCaptura(s, id, png); rec.Code != http.StatusNotFound {
		t.Fatalf("segunda subida = %d, la captura ya se entregó", rec.Code)
	}
}

func TestCapturaSinRespuestaVenceYLimpia(t *testing.T) {
	s := basePrueba(t)
	s.pizarra.esperaCaptura = 30 * time.Millisecond
	se := sesionMontada(t, s, "A")
	rec := agenteT(s, "POST", "pizarra/captura", fmt.Sprintf(`{"sesion_id":%d}`, se.ID))
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("respuesta: %d %s", rec.Code, rec.Body.String())
	}
	if l := latir(t, s, se.ID); len(l.Capturas) != 0 {
		t.Fatalf("una captura vencida no se entrega: %+v", l)
	}
	if rec := subirCaptura(s, 1, pngMagic); rec.Code != http.StatusNotFound {
		t.Fatalf("subida tardía = %d", rec.Code)
	}
}

func TestCapturaCuerpoGrandeDa413(t *testing.T) {
	s := basePrueba(t)
	se := sesionMontada(t, s, "A")
	cuerpo := fmt.Sprintf(`{"sesion_id":%d,"pista":"%s"}`, se.ID, strings.Repeat("a", 2<<20))
	if rec := agenteT(s, "POST", "pizarra/captura", cuerpo); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("cuerpo de 2 MiB = %d, debía ser 413", rec.Code)
	}
}

// --- resolución de sesión (design: argumento|entorno|activa|reciente) ---

func leerResolucion(t *testing.T, s *Servidor, query string) (int, map[string]interface{}) {
	t.Helper()
	rec := agenteT(s, "GET", "pizarra?"+query, "")
	return rec.Code, json2(t, rec)
}

func TestResolucionDeSesion(t *testing.T) {
	s := basePrueba(t)
	if c, m := leerResolucion(t, s, ""); c != 404 || m["error"] != "No hay sesiones de Terminal" {
		t.Fatalf("sin sesiones: %d %v", c, m)
	}
	a, _ := db.CrearSesion(s.Base, "A", "dibujo", nil)
	b, _ := db.CrearSesion(s.Base, "B", "dibujo", nil)
	borrada, _ := db.CrearSesion(s.Base, "C", "dibujo", nil)
	db.BorrarSesion(s.Base, borrada.ID)
	s.Base.Exec(`UPDATE terminal_sesiones SET actualizado = datetime('now','localtime','-1 hour') WHERE id = ?`, b.ID)
	n := func(id int64) string { return fmt.Sprint(id) }

	casos := []struct {
		nombre, query, resolucion string
		sesion                    int64
	}{
		{"reciente (sin latido ni pista)", "", "reciente", a.ID},
		{"pista numérica", "pista=" + n(b.ID), "entorno", b.ID},
		{"argumento gana a la pista", "sesion_id=" + n(a.ID) + "&pista=" + n(b.ID), "argumento", a.ID},
		{"pista de sesión borrada se ignora", "pista=" + n(borrada.ID), "reciente", a.ID},
		{"pista no numérica se ignora", "pista=abc", "reciente", a.ID},
	}
	for _, c := range casos {
		code, m := leerResolucion(t, s, c.query)
		se, _ := m["sesion"].(map[string]interface{})
		if code != 200 || m["resolucion"] != c.resolucion || se == nil || int64(se["id"].(float64)) != c.sesion {
			t.Fatalf("%s: %d %v", c.nombre, code, m)
		}
	}

	// Id explícito inexistente: 404 y nunca cae a otra sesión, aunque haya pista y latido.
	latir(t, s, b.ID)
	for _, q := range []string{"sesion_id=999&pista=" + n(a.ID), "sesion_id=abc", "sesion_id=" + n(borrada.ID)} {
		if c, m := leerResolucion(t, s, q); c != 404 || !strings.HasPrefix(fmt.Sprint(m["error"]), "La sesión ") || !strings.HasSuffix(fmt.Sprint(m["error"]), " no existe") {
			t.Fatalf("%s: %d %v", q, c, m)
		}
	}
	if _, m := leerResolucion(t, s, "sesion_id=999"); m["error"] != "La sesión 999 no existe" {
		t.Fatalf("mensaje: %v", m)
	}

	// Pista vieja (borrada) con latido: cae a la sesión del último latido.
	if c, m := leerResolucion(t, s, "pista="+n(borrada.ID)); c != 200 || m["resolucion"] != "activa" || int64(m["sesion"].(map[string]interface{})["id"].(float64)) != b.ID {
		t.Fatalf("pista vieja con latido: %d %v", c, m)
	}
	// Y la sesión del latido, si se borra, deja de ser "activa".
	db.BorrarSesion(s.Base, b.ID)
	if _, m := leerResolucion(t, s, ""); m["resolucion"] != "reciente" {
		t.Fatalf("latido de sesión borrada: %v", m)
	}
}

func TestOpsYCapturaTambienResuelvenConPista(t *testing.T) {
	s := basePrueba(t)
	s.pizarra.esperaOps = 10 * time.Millisecond
	se := sesionMontada(t, s, "A")
	m := json2(t, agenteT(s, "POST", "pizarra/ops", fmt.Sprintf(`{"pista":"%d",%s`, se.ID, opBorrar[1:])))
	if m["resolucion"] != "entorno" || m["estado"] != "pendiente" {
		t.Fatalf("ops: %v", m)
	}
}

func TestPizarraLeerSinLatidoDevuelveLaEscenaGuardada(t *testing.T) {
	s := basePrueba(t) // UI cerrada: nadie hizo latido
	se, _ := db.CrearSesion(s.Base, "Mapa", "dibujo", nil)
	els := make([]string, 2500)
	for i := range els {
		els[i] = fmt.Sprintf(`{"id":"e%d","type":"rectangle","x":0,"y":0,"width":1,"height":1,"strokeColor":"#000000"}`, i)
	}
	db.GuardarPizarra(s.Base, se.ID, `{"elements":[`+strings.Join(els, ",")+`],"appState":{},"files":{}}`, 0)
	db.EncolarOp(s.Base, se.ID, `[]`)

	code, m := leerResolucion(t, s, fmt.Sprintf("sesion_id=%d", se.ID))
	resumen, _ := m["resumen"].(map[string]interface{})
	if code != 200 || len(m["elementos"].([]interface{})) != 2000 || m["truncado"] != true || resumen["total"] != float64(2500) ||
		m["version"] != float64(1) || m["ops_pendientes"] != float64(1) || m["limites"] == nil || m["actualizado"] == "" ||
		m["sesion"].(map[string]interface{})["nombre"] != "Mapa" {
		t.Fatalf("lectura: %d %v", code, m)
	}
}

// --- guardas (design D8; Threat Matrix, Agent routes) ---

func TestGuardasDeLasRutasDeLaPizarra(t *testing.T) {
	s := basePrueba(t)
	s.pizarra.esperaOps, s.pizarra.esperaCaptura = 10*time.Millisecond, 10*time.Millisecond
	se := sesionMontada(t, s, "A")
	const extension, ajeno = "chrome-extension://abcdef", "http://evil.example"
	id := fmt.Sprint(se.ID)
	rutas := []struct{ metodo, ruta, cuerpo string }{
		{"POST", "pizarra/ops", fmt.Sprintf(`{"sesion_id":%s,%s`, id, opBorrar[1:])},
		{"POST", "pizarra/captura", `{"sesion_id":` + id + `}`},
		{"GET", "pizarra?sesion_id=" + id, ""},
		{"POST", "latido", `{"sesion_id":` + id + `}`},
		{"POST", "pizarra/ops/1/ack", `{"ok":true}`},
		{"POST", "pizarra/capturas/1", pngMagic},
	}
	agente := map[string]bool{"pizarra/ops": true, "pizarra/captura": true, "pizarra?sesion_id=" + id: true}
	for _, r := range rutas {
		for _, origen := range []string{extension, ajeno, "http://localhost:8080.evil.com"} {
			if rec := pedirTerminal(s, r.metodo, r.ruta, r.cuerpo, origen); rec.Code != http.StatusForbidden {
				t.Errorf("%s %s con Origin %q = %d, debía ser 403", r.metodo, r.ruta, origen, rec.Code)
			}
		}
		vacio := pedirTerminal(s, r.metodo, r.ruta, r.cuerpo, "")
		if agente[r.ruta] == (vacio.Code == http.StatusForbidden) {
			t.Errorf("%s %s sin Origin = %d: solo las rutas Agente lo admiten", r.metodo, r.ruta, vacio.Code)
		}
		if rec := pedirTerminal(s, r.metodo, r.ruta, r.cuerpo, origenUI); rec.Code == http.StatusForbidden {
			t.Errorf("%s %s con el Origin exacto de la ventana = 403", r.metodo, r.ruta)
		}
	}
}

// --- pizarra/marcas (design D3, D7, D8, D19) ---

const escenaConPaso = `{"elements":[{"id":"paso1","type":"rectangle","x":10,"y":20,"width":100,"height":40}],"appState":{},"files":{}}`

// sesionParaMarcar crea una sesión montada con carpeta, un recurso en ella y
// otro recurso en una carpeta ajena, con un paso dibujado en la escena.
func sesionParaMarcar(t *testing.T, s *Servidor) (se db.Sesion, propio, ajeno int64) {
	t.Helper()
	c, _ := db.CrearCarpeta(s.Base, "Sistemas")
	otra, _ := db.CrearCarpeta(s.Base, "Otra")
	r1, err := db.Guardar(s.Base, c.ID, "https://a.example/u3", "Unidad 3 -- Sistemas.pdf", "", "", "otro")
	if err != nil {
		t.Fatal(err)
	}
	r2, _ := db.Guardar(s.Base, otra.ID, "https://b.example/x", "Secreto", "", "", "otro")
	se, err = db.CrearSesion(s.Base, "A", "dibujo", &c.ID)
	if err != nil {
		t.Fatal(err)
	}
	db.GuardarPizarra(s.Base, se.ID, escenaConPaso, 0)
	latir(t, s, se.ID)
	return se, r1.ID, r2.ID
}

func cuerpoMarca(sesion int64, marca string) string {
	return fmt.Sprintf(`{"sesion_id":%d,"marcas":[%s]}`, sesion, marca)
}

const marcaPaso = `{"objetivo":{"ids":["paso1"]},"veredicto":"error","pista":"revisa el signo"`

func TestMarcasAplicaElLoteDeDosOpsPorMarca(t *testing.T) {
	s := basePrueba(t)
	se, propio, _ := sesionParaMarcar(t, s)
	cuerpo := cuerpoMarca(se.ID, marcaPaso+fmt.Sprintf(`,"cita":{"recurso_id":%d,"pagina":12}}`, propio))
	resp := asincrono(s, "pizarra/marcas", cuerpo)

	l := esperarLatido(t, s, se.ID, func(r latidoResp) bool { return len(r.Ops) == 1 })
	var ops []map[string]any
	json.Unmarshal(l.Ops[0].Op, &ops)
	if len(ops) != 2 || ops[0]["accion"] != "agregar" || ops[1]["accion"] != "agregar" {
		t.Fatalf("el lote debía ser 2 ops agregar: %s", l.Ops[0].Op)
	}
	if !strings.Contains(string(l.Ops[0].Op), "Unidad 3 -- Sistemas.pdf, pág. 12") || !strings.Contains(string(l.Ops[0].Op), `"bythos":"marca"`) {
		t.Fatalf("falta la cita armada desde la BD o el customData: %s", l.Ops[0].Op)
	}
	ack(s, l.Ops[0].ID, `{"ok":true,"ids":["marca-x-m","marca-x-t"]}`)
	rec := <-resp
	m := json2(t, rec)
	if rec.Code != 200 || m["estado"] != "aplicada" || m["resolucion"] != "argumento" {
		t.Fatalf("respuesta: %d %v", rec.Code, m)
	}
}

func TestMarcasRechazosDan400YNoEncolan(t *testing.T) {
	s := basePrueba(t)
	se, propio, ajeno := sesionParaMarcar(t, s)
	sinCarpeta := sesionMontada(t, s, "Suelta")
	veintiuna := strings.TrimSuffix(strings.Repeat(marcaPaso+`},`, 21), ",")
	casos := []struct{ nombre, cuerpo, texto string }{
		{"cita fuera de la carpeta", cuerpoMarca(se.ID, marcaPaso+fmt.Sprintf(`,"cita":{"recurso_id":%d}}`, ajeno)), "no está en la carpeta de la sesión"},
		{"recurso inexistente", cuerpoMarca(se.ID, marcaPaso+`,"cita":{"recurso_id":9999}}`), "no está en la carpeta de la sesión"},
		{"sesión sin carpeta", cuerpoMarca(sinCarpeta.ID, marcaPaso+fmt.Sprintf(`,"cita":{"recurso_id":%d}}`, propio)), "La sesión no tiene carpeta"},
		{"más de 20 marcas", cuerpoMarca(se.ID, veintiuna), "20"},
		{"id desconocido", cuerpoMarca(se.ID, `{"objetivo":{"ids":["nope"]},"veredicto":"ok"}`), "nope"},
		{"veredicto desconocido", cuerpoMarca(se.ID, `{"objetivo":{"ids":["paso1"]},"veredicto":"quizá"}`), "veredicto"},
		{"JSON inválido", `{"marcas":`, "JSON inválido"},
	}
	for _, c := range casos {
		rec := agenteT(s, "POST", "pizarra/marcas", c.cuerpo)
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), c.texto) {
			t.Errorf("%s: %d %s, debía ser 400 con %q", c.nombre, rec.Code, rec.Body.String(), c.texto)
		}
	}
	if contarOps(s) != 0 {
		t.Fatalf("un rechazo no encola: filas=%d", contarOps(s))
	}
}

func TestMarcasSesionNoMontadaDa409(t *testing.T) {
	s := basePrueba(t)
	se, _, _ := sesionParaMarcar(t, s)
	ahora := time.Now().Add(10 * time.Second)
	s.pizarra.ahora = func() time.Time { return ahora }
	rec := agenteT(s, "POST", "pizarra/marcas", cuerpoMarca(se.ID, marcaPaso+`}`))
	if rec.Code != 409 || json2(t, rec)["error"] != textoPizarraCerrada || contarOps(s) != 0 {
		t.Fatalf("respuesta: %d %s", rec.Code, rec.Body.String())
	}
}

func TestMarcasResuelveSesionPorPista(t *testing.T) {
	s := basePrueba(t)
	s.pizarra.esperaOps = 10 * time.Millisecond
	se, _, _ := sesionParaMarcar(t, s)
	rec := agenteT(s, "POST", "pizarra/marcas", fmt.Sprintf(`{"pista":"%d","marcas":[%s}]}`, se.ID, marcaPaso))
	if m := json2(t, rec); rec.Code != 200 || m["resolucion"] != "entorno" || m["estado"] != "pendiente" {
		t.Fatalf("respuesta: %d %v", rec.Code, m)
	}
}

func TestMarcasGuardaAgente(t *testing.T) {
	s := basePrueba(t)
	s.pizarra.esperaOps = 10 * time.Millisecond
	se, _, _ := sesionParaMarcar(t, s)
	cuerpo := cuerpoMarca(se.ID, marcaPaso+`}`)
	for _, origen := range []string{"chrome-extension://abcdef", "http://evil.example", "http://localhost:8080.evil.com"} {
		if rec := pedirTerminal(s, "POST", "pizarra/marcas", cuerpo, origen); rec.Code != http.StatusForbidden {
			t.Errorf("Origin %q = %d, debía ser 403", origen, rec.Code)
		}
	}
	if rec := pedirTerminal(s, "POST", "pizarra/marcas", cuerpo, ""); rec.Code != 200 {
		t.Errorf("sin Origin = %d, el agente lo admite", rec.Code)
	}
}

// --- historial del agente: ack con inversa y GET cambios ---

func columnasOp(s *Servidor, id int64) (inversa, versiones sql.NullString, estado string) {
	s.Base.QueryRow(`SELECT inversa, versiones, estado FROM terminal_pizarra_ops WHERE id = ?`, id).Scan(&inversa, &versiones, &estado)
	return
}

func TestAckGuardaInversaYVersiones(t *testing.T) {
	s := basePrueba(t)
	se, _ := db.CrearSesion(s.Base, "A", "dibujo", nil)
	id, _ := db.EncolarOp(s.Base, se.ID, `[{"accion":"borrar","ids":["a"]}]`)
	cuerpo := `{"ok":true,"ids":["a"],"inversa":{"restaurar":[{"id":"a"}],"quitar":[]},"versiones":{"a":[2,9]}}`
	if rec := ack(s, id, cuerpo); rec.Code != 200 {
		t.Fatalf("ack: %d %s", rec.Code, rec.Body.String())
	}
	inv, ver, _ := columnasOp(s, id)
	if inv.String != `{"restaurar":[{"id":"a"}],"quitar":[]}` || ver.String != `{"a":[2,9]}` {
		t.Fatalf("guardado = %q %q", inv.String, ver.String)
	}
}

func TestAckSinInversaOSinDeshacerGuardaNull(t *testing.T) {
	s := basePrueba(t)
	se, _ := db.CrearSesion(s.Base, "A", "dibujo", nil)
	for _, cuerpo := range []string{
		`{"ok":true,"ids":["a"]}`, // ack de la versión anterior de la UI
		`{"ok":true,"ids":["a"],"inversa":null,"sin_deshacer":true}`,
		`{"ok":true,"ids":["a"],"inversa":{"quitar":["a"]},"sin_deshacer":true}`,
	} {
		id, _ := db.EncolarOp(s.Base, se.ID, `[]`)
		if rec := ack(s, id, cuerpo); rec.Code != 200 {
			t.Fatalf("ack %s: %d", cuerpo, rec.Code)
		}
		if inv, ver, est := columnasOp(s, id); inv.Valid || ver.Valid || est != "aplicada" {
			t.Errorf("%s: inversa=%v versiones=%v estado=%s", cuerpo, inv, ver, est)
		}
	}
}

func TestAckConInversaGrandeGuardaNullYCompleta(t *testing.T) {
	s := basePrueba(t)
	se, _ := db.CrearSesion(s.Base, "A", "dibujo", nil)
	// 600 KiB de inversa: pasa el lector de 2 MiB pero supera el tope de 512 KiB.
	grande := fmt.Sprintf(`{"ok":true,"ids":["a"],"inversa":{"quitar":["%s"]},"versiones":{"a":[1,1]}}`, strings.Repeat("x", 600<<10))
	id, _ := db.EncolarOp(s.Base, se.ID, `[]`)
	if rec := ack(s, id, grande); rec.Code != 200 {
		t.Fatalf("ack grande: %d", rec.Code)
	}
	if inv, ver, est := columnasOp(s, id); inv.Valid || ver.Valid || est != "aplicada" {
		t.Fatalf("inversa=%v versiones=%v estado=%s", inv.Valid, ver.Valid, est)
	}
	// Body de más de 2 MiB: tampoco se rechaza (la op vencería); queda sin inversa.
	enorme := fmt.Sprintf(`{"ok":true,"ids":["a"],"inversa":{"quitar":["%s"]}}`, strings.Repeat("x", 3<<20))
	id2, _ := db.EncolarOp(s.Base, se.ID, `[]`)
	if rec := ack(s, id2, enorme); rec.Code != 200 {
		t.Fatalf("ack enorme: %d", rec.Code)
	}
	if inv, _, est := columnasOp(s, id2); inv.Valid || est != "aplicada" {
		t.Fatalf("inversa=%v estado=%s", inv.Valid, est)
	}
}

func TestAckDeMarcasRegistraEventoDeMarcas(t *testing.T) {
	s := basePrueba(t)
	se, _ := db.CrearSesion(s.Base, "A", "dibujo", nil)
	id, _ := db.EncolarOp(s.Base, se.ID, `[{"accion":"agregar","elemento":{"id":"marca-x-m"}},{"accion":"agregar","elemento":{"id":"marca-x-t"}}]`)
	ack(s, id, `{"ok":true,"ids":["marca-x-m","marca-x-t"]}`)
	ev, _ := db.ListarEventos(s.Base, db.FiltroEventos{})
	if len(ev) != 1 || !strings.Contains(ev[0].Detalle, "pizarra_marcar") || strings.Contains(ev[0].Detalle, "pizarra_dibujar") {
		t.Fatalf("un lote de marcas no es un dibujo: %+v", ev)
	}
}

func TestGetCambiosListaYGuarda(t *testing.T) {
	s := basePrueba(t)
	se, _ := db.CrearSesion(s.Base, "A", "dibujo", nil)
	id, _ := db.EncolarOp(s.Base, se.ID, `[{"accion":"borrar","ids":["a","b"]}]`)
	ack(s, id, `{"ok":true,"ids":["a","b"],"inversa":{"quitar":[]},"versiones":{}}`)
	ruta := fmt.Sprintf("sesiones/%d/cambios", se.ID)
	rec := pedirTerminal(s, "GET", ruta, "", "", "Sec-Fetch-Site", "same-origin")
	var lista []map[string]any
	json.Unmarshal(rec.Body.Bytes(), &lista)
	if rec.Code != 200 || len(lista) != 1 || lista[0]["estado"] != "deshacible" || lista[0]["tipo"] != "dibujo" || lista[0]["elementos"] != float64(2) || lista[0]["inversa"] != nil {
		t.Fatalf("cambios: %d %s", rec.Code, rec.Body.String())
	}
	if rec := pedirTerminal(s, "GET", ruta, "", "", "Sec-Fetch-Site", "cross-site"); rec.Code != 403 {
		t.Errorf("cross-site = %d, debía ser 403", rec.Code)
	}
	if rec := pedirTerminal(s, "GET", ruta, "", "chrome-extension://abcdef"); rec.Code != 403 {
		t.Errorf("Origin de extensión = %d, debía ser 403", rec.Code)
	}
	if rec := pedirTerminal(s, "GET", "sesiones/999/cambios", "", ""); rec.Code != 404 {
		t.Errorf("sesión inexistente = %d, debía ser 404", rec.Code)
	}
}
