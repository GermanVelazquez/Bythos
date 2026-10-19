package api

// adjuntos_test.go — Rutas de adjuntos de la pizarra (sesiones/{id}/adjuntos):
// guardas, tipos, tamaño y privacidad entre sesiones. Reusa construirZip y
// usarArchivosPrueba de los tests de archivos.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"mime/multipart"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"bythos-desktop/archivos"
	"bythos-desktop/db"
)

const (
	pngAdj  = "\x89PNG\r\n\x1a\n"
	mp4Adj  = "\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00"
	macroCT = `<Types><Override ContentType="application/vnd.ms-word.document.macroEnabled.main+xml"/></Types>`
)

func prepararAdjuntos(t *testing.T) (*Servidor, db.Sesion) {
	t.Helper()
	usarArchivosPrueba(t)
	s := basePrueba(t)
	return s, crearSesionT(t, s, `{"nombre":"Pizarra","modo":"dibujo"}`)
}

func rutaAdj(sesionID int64, resto string) string {
	return fmt.Sprintf("sesiones/%d/adjuntos%s", sesionID, resto)
}

// subirAdj sube un archivo con el Origin dado ("" = sin Origin).
func subirAdj(t *testing.T, s *Servidor, sesionID int64, origen, nombre, contenido string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("archivo", nombre)
	fw.Write([]byte(contenido))
	mw.Close()
	return pedirTerminal(s, "POST", rutaAdj(sesionID, ""), buf.String(), origen, "Content-Type", mw.FormDataContentType())
}

// blobsEnDisco cuenta los archivos reales del almacén (sin temporales).
func blobsEnDisco(t *testing.T) int {
	t.Helper()
	n := 0
	filepath.WalkDir(archivos.Base(), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func decodificarAdj(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("respuesta no es JSON: %v (%s)", err, w.Body.String())
	}
	return m
}

func TestAdjuntoGuardas(t *testing.T) {
	s, se := prepararAdjuntos(t)
	for _, origen := range []string{"chrome-extension://abc", ""} {
		if w := subirAdj(t, s, se.ID, origen, "a.png", pngAdj+"x"); w.Code != 403 {
			t.Fatalf("subida con Origin %q: %d, quería 403", origen, w.Code)
		}
	}
	if n := blobsEnDisco(t); n != 0 {
		t.Fatalf("la subida rechazada guardó %d blobs", n)
	}
	w := subirAdj(t, s, se.ID, origenUI, "a.png", pngAdj+"x")
	adj := int64(decodificarAdj(t, w)["id"].(float64))
	for _, resto := range []string{"", fmt.Sprintf("/%d/contenido", adj), fmt.Sprintf("/%d/texto", adj)} {
		if w := pedirTerminal(s, "GET", rutaAdj(se.ID, resto), "", "", "Sec-Fetch-Site", "cross-site"); w.Code != 403 {
			t.Fatalf("GET %q cross-site: %d, quería 403", resto, w.Code)
		}
	}
}

func TestAdjuntoSesionAjenaODesconocida(t *testing.T) {
	s, a := prepararAdjuntos(t)
	b := crearSesionT(t, s, `{"nombre":"Otra","modo":"dibujo"}`)
	adj := int64(decodificarAdj(t, subirAdj(t, s, a.ID, origenUI, "a.png", pngAdj+"x"))["id"].(float64))

	for _, resto := range []string{fmt.Sprintf("/%d/contenido", adj), fmt.Sprintf("/%d/texto", adj)} {
		if w := ventanaT(s, "GET", rutaAdj(b.ID, resto), ""); w.Code != 404 {
			t.Fatalf("adjunto de otra sesión %q: %d, quería 404", resto, w.Code)
		}
	}
	if w := ventanaT(s, "GET", rutaAdj(9999, ""), ""); w.Code != 404 {
		t.Fatalf("listar sesión desconocida: %d", w.Code)
	}
	if w := subirAdj(t, s, 9999, origenUI, "a.png", pngAdj+"x"); w.Code != 404 {
		t.Fatalf("subir a sesión desconocida: %d", w.Code)
	}
	if n := blobsEnDisco(t); n != 1 {
		t.Fatalf("blobs = %d, quería 1 (el de la sesión A)", n)
	}
}

func TestAdjuntoTiposPermitidosYRechazados(t *testing.T) {
	s, se := prepararAdjuntos(t)
	docx := string(construirZip(t, map[string]string{"word/document.xml": "<w/>"}))
	xlsx := string(construirZip(t, map[string]string{"xl/workbook.xml": "<w/>"}))
	pptx := string(construirZip(t, map[string]string{"ppt/presentation.xml": "<w/>"}))
	docm := string(construirZip(t, map[string]string{"word/document.xml": "<w/>", "[Content_Types].xml": macroCT}))

	permitidos := []struct{ nombre, contenido, mime, tipo string }{
		{"a.png", pngAdj + "1", "image/png", "imagen"},
		{"a.jpg", "\xff\xd8\xff\xe0" + "2", "image/jpeg", "imagen"},
		{"a.gif", "GIF89a3", "image/gif", "imagen"},
		{"a.webp", webpMinimo + "4", "image/webp", "imagen"},
		{"a.pdf", pdfMinimo, "application/pdf", "pdf"},
		{"a.docx", docx, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "documento"},
		{"a.xlsx", xlsx, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "documento"},
		{"a.pptx", pptx, "application/vnd.openxmlformats-officedocument.presentationml.presentation", "documento"},
		{"a.txt", "hola", "text/plain", "documento"},
		{"a.md", "# hola", "text/plain", "documento"},
		{"README.sh", "echo hola\n", "text/plain", "documento"},
	}
	for _, c := range permitidos {
		w := subirAdj(t, s, se.ID, origenUI, c.nombre, c.contenido)
		if w.Code != 201 {
			t.Fatalf("%s: %d, quería 201 (%s)", c.nombre, w.Code, w.Body.String())
		}
		m := decodificarAdj(t, w)
		if m["mime"] != c.mime || m["tipo"] != c.tipo || m["nombre"] != c.nombre || len(m["sha256"].(string)) != 64 || m["id"] == nil {
			t.Fatalf("%s: respuesta mal armada: %v", c.nombre, m)
		}
	}
	antes := blobsEnDisco(t)

	rechazados := map[string][2]string{
		"svg":          {"a.svg", `<svg xmlns="http://www.w3.org/2000/svg"/>`},
		"bmp":          {"a.bmp", "BM\x00\x00\x00\x00"},
		"html":         {"a.html", "<html><script>alert(1)</script></html>"},
		"docm":         {"a.docm", docm},
		"md con tag":   {"a.md", "<script>alert(1)</script>"},
		"exe como png": {"foto.png", "MZ\x90\x00\x03\x00\x00\x00\x04\x00"},
		"mp4":          {"a.mp4", mp4Adj + "5"},
	}
	for nombre, c := range rechazados {
		if w := subirAdj(t, s, se.ID, origenUI, c[0], c[1]); w.Code != 415 {
			t.Fatalf("%s: %d, quería 415 (%s)", nombre, w.Code, w.Body.String())
		}
	}
	if n := blobsEnDisco(t); n != antes {
		t.Fatalf("los rechazados dejaron blobs: %d -> %d", antes, n)
	}
	if lista, _ := db.ListarAdjuntos(s.Base, se.ID); len(lista) != len(permitidos) {
		t.Fatalf("vínculos = %d, quería %d", len(lista), len(permitidos))
	}
}

func TestAdjuntoVideoReusadoNoBorraElBlob(t *testing.T) {
	s, se := prepararAdjuntos(t)
	// El mismo video ya vive en la biblioteca: el blob es de ese recurso.
	c, _ := db.CrearCarpeta(s.Base, "Videos")
	if w := subirMultipart(t, s, c.ID, "v.mp4", mp4Adj+"6"); w.Code != 201 {
		t.Fatalf("subida a la biblioteca: %d %s", w.Code, w.Body.String())
	}
	if w := subirAdj(t, s, se.ID, origenUI, "v.mp4", mp4Adj+"6"); w.Code != 415 {
		t.Fatalf("video como adjunto: %d, quería 415", w.Code)
	}
	if n := blobsEnDisco(t); n != 1 {
		t.Fatalf("el blob de la biblioteca debía seguir: %d", n)
	}
}

func TestAdjuntoLimiteDeTamano(t *testing.T) {
	s, se := prepararAdjuntos(t)
	previo := limiteAdjunto
	limiteAdjunto = 100
	t.Cleanup(func() { limiteAdjunto = previo })

	relleno := func(n int) string { return pngAdj + strings.Repeat("a", n-len(pngAdj)) }
	if w := subirAdj(t, s, se.ID, origenUI, "justo.png", relleno(100)); w.Code != 201 {
		t.Fatalf("en el límite: %d, quería 201", w.Code)
	}
	w := subirAdj(t, s, se.ID, origenUI, "grande.png", relleno(101))
	if w.Code != 413 || !strings.Contains(w.Body.String(), "máximo") {
		t.Fatalf("sobre el límite: %d %s, quería 413", w.Code, w.Body.String())
	}
	if lista, _ := db.ListarAdjuntos(s.Base, se.ID); len(lista) != 1 || blobsEnDisco(t) != 1 {
		t.Fatalf("el 413 dejó rastro: %d vínculos, %d blobs", len(lista), blobsEnDisco(t))
	}
}

func TestAdjuntoLimiteReal25MiB(t *testing.T) {
	if testing.Short() {
		t.Skip("sube 2 x 25 MiB")
	}
	s, se := prepararAdjuntos(t)
	relleno := func(n int) string { return pngAdj + strings.Repeat("a", n-len(pngAdj)) }
	if w := subirAdj(t, s, se.ID, origenUI, "justo.png", relleno(25<<20)); w.Code != 201 {
		t.Fatalf("25 MiB exactos: %d, quería 201", w.Code)
	}
	w := subirAdj(t, s, se.ID, origenUI, "grande.png", relleno(25<<20+1))
	if w.Code != 413 || !strings.Contains(w.Body.String(), "25 MiB") {
		t.Fatalf("25 MiB + 1: %d %s, quería 413 con el límite", w.Code, w.Body.String())
	}
	if blobsEnDisco(t) != 1 {
		t.Fatalf("el 413 dejó un blob")
	}
}

func TestAdjuntoDuplicadoYSesiones(t *testing.T) {
	s, a := prepararAdjuntos(t)
	b := crearSesionT(t, s, `{"nombre":"Otra","modo":"dibujo"}`)
	contenido := pngAdj + "mismo"

	id1 := decodificarAdj(t, subirAdj(t, s, a.ID, origenUI, "uno.png", contenido))["id"]
	id2 := decodificarAdj(t, subirAdj(t, s, a.ID, origenUI, "dos.png", contenido))["id"]
	if id1 != id2 {
		t.Fatalf("mismo contenido en una sesión debía dar el mismo vínculo: %v != %v", id1, id2)
	}
	idB := decodificarAdj(t, subirAdj(t, s, b.ID, origenUI, "uno.png", contenido))["id"]
	if idB == id1 {
		t.Fatal("sesiones distintas deben tener vínculos distintos")
	}
	if n := blobsEnDisco(t); n != 1 {
		t.Fatalf("blobs = %d, quería 1 (guardado una sola vez)", n)
	}

	var listaA []db.Adjunto
	json.Unmarshal(ventanaT(s, "GET", rutaAdj(a.ID, ""), "").Body.Bytes(), &listaA)
	if len(listaA) != 1 || listaA[0].SesionID != a.ID {
		t.Fatalf("la lista de A debía tener solo su vínculo: %+v", listaA)
	}
}

func TestAdjuntoContenidoYTexto(t *testing.T) {
	s, se := prepararAdjuntos(t)
	adj := int64(decodificarAdj(t, subirAdj(t, s, se.ID, origenUI, "nota.txt", "hola pizarra"))["id"].(float64))

	w := ventanaT(s, "GET", rutaAdj(se.ID, fmt.Sprintf("/%d/contenido", adj)), "")
	if w.Code != 200 || w.Body.String() != "hola pizarra" {
		t.Fatalf("contenido: %d %q", w.Code, w.Body.String())
	}
	h := w.Result().Header
	if h.Get("X-Content-Type-Options") != "nosniff" ||
		!strings.Contains(h.Get("Content-Security-Policy"), "sandbox") ||
		!strings.Contains(h.Get("Content-Security-Policy"), "frame-ancestors 'self'") {
		t.Fatalf("headers de contenido inseguros: %v", h)
	}

	w = ventanaT(s, "GET", rutaAdj(se.ID, fmt.Sprintf("/%d/texto", adj)), "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "hola pizarra") {
		t.Fatalf("texto: %d %s", w.Code, w.Body.String())
	}
}

// sesionConCarpeta crea una carpeta y una sesión vinculada a ella.
func sesionConCarpeta(t *testing.T, s *Servidor) (db.Sesion, db.Carpeta) {
	t.Helper()
	c, _ := db.CrearCarpeta(s.Base, "Mates")
	se := crearSesionT(t, s, fmt.Sprintf(`{"nombre":"P","modo":"dibujo","carpeta_id":%d}`, c.ID))
	return se, c
}

func contarRecursos(t *testing.T, s *Servidor, carpetaID int64) int {
	t.Helper()
	rs, err := db.ListarRecursos(s.Base, carpetaID)
	if err != nil {
		t.Fatal(err)
	}
	return len(rs)
}

func TestAdjuntoGuardarEnCarpeta(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	se, c := sesionConCarpeta(t, s)
	adj := int64(decodificarAdj(t, subirAdj(t, s, se.ID, origenUI, "foto.png", pngAdj+"g"))["id"].(float64))
	ruta := rutaAdj(se.ID, fmt.Sprintf("/%d/guardar", adj))

	// Subir nunca crea recurso (sin guardado automático).
	if n := contarRecursos(t, s, c.ID); n != 0 {
		t.Fatalf("subir creó %d recursos", n)
	}
	for _, origen := range []string{"chrome-extension://abc", ""} {
		if w := pedirTerminal(s, "POST", ruta, "", origen); w.Code != 403 {
			t.Fatalf("guardar con Origin %q: %d, quería 403", origen, w.Code)
		}
	}

	w := ventanaT(s, "POST", ruta, "")
	if w.Code != 201 {
		t.Fatalf("guardar: %d %s", w.Code, w.Body.String())
	}
	r1 := decodificarRecurso(t, w)
	if r1.CarpetaID != c.ID || r1.Tipo != "imagen" || r1.ArchivoID == 0 {
		t.Fatalf("recurso mal armado: %+v", r1)
	}
	if n := blobsEnDisco(t); n != 1 {
		t.Fatalf("blobs = %d, quería 1 (sin copiar bytes)", n)
	}
	// El adjunto sigue en la pizarra.
	if w := ventanaT(s, "GET", rutaAdj(se.ID, fmt.Sprintf("/%d/contenido", adj)), ""); w.Code != 200 {
		t.Fatalf("el adjunto debía seguir: %d", w.Code)
	}

	// Mismo contenido ya en la carpeta: 200 con el recurso existente.
	w = ventanaT(s, "POST", ruta, "")
	if w.Code != 200 || decodificarRecurso(t, w).ID != r1.ID {
		t.Fatalf("repetido: %d %s", w.Code, w.Body.String())
	}
	if n := contarRecursos(t, s, c.ID); n != 1 {
		t.Fatalf("recursos = %d, quería 1", n)
	}
}

func TestAdjuntoGuardarSinCarpetaOAjeno(t *testing.T) {
	s, se := prepararAdjuntos(t)
	adj := int64(decodificarAdj(t, subirAdj(t, s, se.ID, origenUI, "foto.png", pngAdj+"g"))["id"].(float64))
	w := ventanaT(s, "POST", rutaAdj(se.ID, fmt.Sprintf("/%d/guardar", adj)), "")
	if w.Code != 409 || !strings.Contains(w.Body.String(), "La sesión no tiene carpeta") {
		t.Fatalf("sin carpeta: %d %s", w.Code, w.Body.String())
	}
	if w := ventanaT(s, "POST", rutaAdj(se.ID, "/9999/guardar"), ""); w.Code != 404 {
		t.Fatalf("adjunto inexistente: %d, quería 404", w.Code)
	}
}

func TestAdjuntoDesdeRecurso(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	se, c := sesionConCarpeta(t, s)
	otra, _ := db.CrearCarpeta(s.Base, "Otra")
	ruta := rutaAdj(se.ID, "/desde-recurso")
	cuerpo := func(id int64) string { return fmt.Sprintf(`{"recurso_id":%d}`, id) }
	sembrar := func(carpeta int64, sha, mime, tipo string) db.Recurso {
		a, _ := db.CrearArchivo(s.Base, sha, "x", mime, 5, sha[:2]+"/"+sha)
		r, err := db.GuardarArchivo(s.Base, carpeta, a.ID, "R", tipo)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	img := sembrar(c.ID, "aa11", "image/png", "imagen")

	for _, origen := range []string{"chrome-extension://abc", ""} {
		if w := pedirTerminal(s, "POST", ruta, cuerpo(img.ID), origen); w.Code != 403 {
			t.Fatalf("Origin %q: %d, quería 403", origen, w.Code)
		}
	}
	if w := ventanaT(s, "POST", ruta, cuerpo(img.ID)); w.Code != 201 {
		t.Fatalf("recurso de la carpeta: %d %s", w.Code, w.Body.String())
	}
	if w := ventanaT(s, "POST", ruta, cuerpo(img.ID)); w.Code != 201 {
		t.Fatalf("segunda vez: %d", w.Code)
	}
	var lista []db.Adjunto
	json.Unmarshal(ventanaT(s, "GET", rutaAdj(se.ID, ""), "").Body.Bytes(), &lista)
	if len(lista) != 1 {
		t.Fatalf("el mismo recurso dos veces debía dar un vínculo: %+v", lista)
	}

	ajeno := sembrar(otra.ID, "bb22", "image/png", "imagen")
	video := sembrar(c.ID, "cc33", "video/mp4", "video")
	enlace, _ := db.Guardar(s.Base, c.ID, "https://x.dev", "L", "", "", "otro")
	for nombre, id := range map[string]int64{"otra carpeta": ajeno.ID, "video": video.ID, "enlace": enlace.ID, "inexistente": 9999} {
		if w := ventanaT(s, "POST", ruta, cuerpo(id)); w.Code != 400 {
			t.Fatalf("%s: %d, quería 400", nombre, w.Code)
		}
	}
	if w := ventanaT(s, "POST", ruta, `no json`); w.Code != 400 {
		t.Fatalf("JSON inválido: %d", w.Code)
	}

	sinCarpeta := crearSesionT(t, s, `{"nombre":"S","modo":"dibujo"}`)
	w := ventanaT(s, "POST", rutaAdj(sinCarpeta.ID, "/desde-recurso"), cuerpo(img.ID))
	if w.Code != 409 || !strings.Contains(w.Body.String(), "La sesión no tiene carpeta") {
		t.Fatalf("sin carpeta: %d %s", w.Code, w.Body.String())
	}
}
