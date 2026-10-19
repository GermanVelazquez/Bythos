package api

// adjuntos_agente_test.go — Rutas del agente para leer adjuntos (design D5,
// D13, D14): solo lo que está en la escena GUARDADA, con la UI cerrada.

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"net/http/httptest"
	"strings"
	"testing"

	"bythos-desktop/db"
)

// adjEnPizarra sube un archivo y guarda una escena con un elemento vivo que lo
// referencia (imagen por fileId, tarjeta por customData.sha256).
func adjEnPizarra(t *testing.T, s *Servidor, sesionID int64, nombre, contenido string) (adj int64, sha string) {
	t.Helper()
	m := decodificarAdj(t, subirAdj(t, s, sesionID, origenUI, nombre, contenido))
	adj, sha = int64(m["id"].(float64)), m["sha256"].(string)
	el := fmt.Sprintf(`{"id":"el%d","type":"rectangle","customData":{"sha256":%q}}`, adj, sha)
	if m["tipo"] == "imagen" {
		el = fmt.Sprintf(`{"id":"el%d","type":"image","fileId":%q}`, adj, sha)
	}
	guardarEscenaT(t, s, sesionID, el)
	return adj, sha
}

func guardarEscenaT(t *testing.T, s *Servidor, sesionID int64, elementos string) {
	t.Helper()
	_, ver, _, _ := db.ObtenerPizarra(s.Base, sesionID)
	if _, err := db.GuardarPizarra(s.Base, sesionID, `{"elements":[`+elementos+`],"appState":{},"files":{}}`, ver); err != nil {
		t.Fatal(err)
	}
}

func pngAncho(t *testing.T, ancho int) string {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, ancho, 10))); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func agenteAdj(s *Servidor, sesionID, adj int64, ruta, origen string) *httptest.ResponseRecorder {
	return pedirTerminal(s, "GET", fmt.Sprintf("adjuntos/%d/%s?sesion_id=%d", adj, ruta, sesionID), "", origen)
}

func shaDeAdj(t *testing.T, s *Servidor, sesionID, adj int64) string {
	t.Helper()
	a, _, err := db.ObtenerAdjunto(s.Base, sesionID, adj)
	if err != nil {
		t.Fatal(err)
	}
	return a.SHA256
}

func TestAgenteImagenEscalaYLlevaCabeceras(t *testing.T) {
	s, se := prepararAdjuntos(t)
	adj, _ := adjEnPizarra(t, s, se.ID, "ancha.png", pngAncho(t, 1440)) // UI cerrada: sin latido
	w := agenteAdj(s, se.ID, adj, "imagen", "")
	if w.Code != 200 || w.Result().Header.Get("Content-Type") != "image/png" {
		t.Fatalf("imagen: %d %s", w.Code, w.Header())
	}
	cfg, _, err := image.DecodeConfig(w.Body)
	if err != nil || cfg.Width != 1280 {
		t.Fatalf("ancho %d (%v), quería 1280", cfg.Width, err)
	}
	h := w.Result().Header
	if h.Get("X-Bythos-Adjunto") != fmt.Sprint(adj) || h.Get("X-Bythos-Sesion") != fmt.Sprint(se.ID) ||
		h.Get("X-Bythos-Resolucion") != "argumento" || h.Get("X-Bythos-Ruta") == "" || h.Get("X-Bythos-Tipo") != "imagen" {
		t.Fatalf("cabeceras: %v", h)
	}
}

func TestAgenteTextoDeTarjetaYTruncado(t *testing.T) {
	s, se := prepararAdjuntos(t)
	adj, _ := adjEnPizarra(t, s, se.ID, "nota.txt", "hola pizarra")
	m := decodificarAdj(t, agenteAdj(s, se.ID, adj, "texto", ""))
	if m["texto"] != "hola pizarra" || m["truncado"] != false || m["caracteres_totales"] != float64(12) ||
		m["resolucion"] != "argumento" || m["adjunto"] != float64(adj) {
		t.Fatalf("texto: %v", m)
	}

	largo, _ := adjEnPizarra(t, s, se.ID, "largo.txt", strings.Repeat("a", 40_500))
	m = decodificarAdj(t, agenteAdj(s, se.ID, largo, "texto", ""))
	if m["truncado"] != true || m["caracteres_totales"] != float64(40_500) || len(m["texto"].(string)) != 40_000 {
		t.Fatalf("truncado: %v %v", m["truncado"], m["caracteres_totales"])
	}
}

func TestAgentePDFSoloDaMetadataYRuta(t *testing.T) {
	s, se := prepararAdjuntos(t)
	adj, _ := adjEnPizarra(t, s, se.ID, "doc.pdf", pdfMinimo)
	m := decodificarAdj(t, agenteAdj(s, se.ID, adj, "texto", ""))
	if m["ruta"] == "" || m["aviso"] == "" || m["adjunto"] != float64(adj) || m["texto"] != nil {
		t.Fatalf("pdf: %v", m)
	}
}

func TestAgenteTipoEquivocado(t *testing.T) {
	s, se := prepararAdjuntos(t)
	img, _ := adjEnPizarra(t, s, se.ID, "a.png", pngAdj+"x")
	pdf, _ := adjEnPizarra(t, s, se.ID, "doc.pdf", pdfMinimo)
	// adjEnPizarra deja solo el último elemento: dejar ambos vivos
	guardarEscenaT(t, s, se.ID, fmt.Sprintf(`{"id":"i","type":"image","fileId":%q},{"id":"p","type":"rectangle","customData":{"sha256":%q}}`,
		shaDeAdj(t, s, se.ID, img), shaDeAdj(t, s, se.ID, pdf)))
	if w := agenteAdj(s, se.ID, img, "texto", ""); w.Code != 400 || !strings.Contains(w.Body.String(), "leer_imagen") {
		t.Fatalf("imagen en texto: %d %s", w.Code, w.Body.String())
	}
	if w := agenteAdj(s, se.ID, pdf, "imagen", ""); w.Code != 400 || !strings.Contains(w.Body.String(), "leer_archivo") {
		t.Fatalf("pdf en imagen: %d %s", w.Code, w.Body.String())
	}
}

func TestAgenteSoloLeeLoQueEstaEnLaPizarraGuardada(t *testing.T) {
	s, se := prepararAdjuntos(t)
	otra := crearSesionT(t, s, `{"nombre":"Otra","modo":"dibujo"}`)
	ajeno, _ := adjEnPizarra(t, s, otra.ID, "ajeno.txt", "secreto")
	if w := agenteAdj(s, se.ID, ajeno, "texto", ""); w.Code != 404 || strings.Contains(w.Body.String(), "secreto") {
		t.Fatalf("adjunto de otra sesión: %d %s", w.Code, w.Body.String())
	}
	if w := agenteAdj(s, se.ID, 9999, "texto", ""); w.Code != 404 {
		t.Fatalf("id desconocido: %d", w.Code)
	}

	// vinculado pero sin elemento en la escena guardada
	suelto := int64(decodificarAdj(t, subirAdj(t, s, se.ID, origenUI, "suelto.txt", "x"))["id"].(float64))
	w := agenteAdj(s, se.ID, suelto, "texto", "")
	if w.Code != 404 || !strings.Contains(w.Body.String(), fmt.Sprintf("El adjunto %d no está en la pizarra", suelto)) {
		t.Fatalf("no está en la pizarra: %d %s", w.Code, w.Body.String())
	}

	// el elemento existe pero está borrado en la escena guardada
	adj, sha := adjEnPizarra(t, s, se.ID, "borrado.txt", "y")
	guardarEscenaT(t, s, se.ID, fmt.Sprintf(`{"id":"b","type":"rectangle","isDeleted":true,"customData":{"sha256":%q}}`, sha))
	if w := agenteAdj(s, se.ID, adj, "texto", ""); w.Code != 404 {
		t.Fatalf("elemento borrado: %d", w.Code)
	}
	guardarEscenaT(t, s, se.ID, "")
	if w := agenteAdj(s, se.ID, adj, "texto", ""); w.Code != 404 {
		t.Fatalf("quitado de la escena: %d", w.Code)
	}
}

func TestAgenteRutasDeAdjuntosGuardaDeOrigin(t *testing.T) {
	s, se := prepararAdjuntos(t)
	adj, _ := adjEnPizarra(t, s, se.ID, "nota.txt", "hola")
	for _, ruta := range []string{"texto", "imagen"} {
		for _, origen := range []string{"http://evil.example", "chrome-extension://abc", "http://localhost:8080.evil.com"} {
			if w := agenteAdj(s, se.ID, adj, ruta, origen); w.Code != 403 {
				t.Errorf("%s con Origin %q = %d, debía ser 403", ruta, origen, w.Code)
			}
		}
	}
	if w := agenteAdj(s, se.ID, adj, "texto", origenUI); w.Code != 200 {
		t.Fatalf("Origin de la ventana: %d", w.Code)
	}
}

func TestPizarraLeerListaSoloAdjuntosVivos(t *testing.T) {
	s, se := prepararAdjuntos(t)
	w := pedirTerminal(s, "GET", fmt.Sprintf("pizarra?sesion_id=%d", se.ID), "", "")
	m := decodificarAdj(t, w)
	if lista, ok := m["adjuntos"].([]any); !ok || len(lista) != 0 || m["truncado"] != false {
		t.Fatalf("sin adjuntos: %v", m["adjuntos"])
	}

	img, _ := adjEnPizarra(t, s, se.ID, "a.png", pngAdj+"x")
	pdf, _ := adjEnPizarra(t, s, se.ID, "doc.pdf", pdfMinimo)
	subirAdj(t, s, se.ID, origenUI, "suelto.txt", "x") // vinculado, sin elemento
	guardarEscenaT(t, s, se.ID, fmt.Sprintf(`{"id":"i","type":"image","fileId":%q},{"id":"p","type":"rectangle","customData":{"sha256":%q}}`,
		shaDeAdj(t, s, se.ID, img), shaDeAdj(t, s, se.ID, pdf)))
	w = pedirTerminal(s, "GET", fmt.Sprintf("pizarra?sesion_id=%d", se.ID), "", "")
	m = decodificarAdj(t, w)
	lista, _ := m["adjuntos"].([]any)
	if len(lista) != 2 || strings.Contains(w.Body.String(), "suelto.txt") || strings.Contains(w.Body.String(), "PDF-1.4") {
		t.Fatalf("adjuntos: %s", w.Body.String())
	}
	a := lista[0].(map[string]any)
	if a["id"] != float64(img) || a["nombre"] != "a.png" || a["tipo"] != "imagen" || a["mime"] != "image/png" ||
		fmt.Sprint(a["elementos"]) != "[i]" || a["tamano"] == nil || lista[1].(map[string]any)["id"] != float64(pdf) {
		t.Fatalf("primer adjunto: %v", a)
	}
	for _, e := range m["elementos"].([]any) {
		if e.(map[string]any)["id"] == "i" && e.(map[string]any)["adjunto"] != float64(img) {
			t.Fatalf("el elemento imagen debe llevar su adjunto: %v", e)
		}
	}
}
