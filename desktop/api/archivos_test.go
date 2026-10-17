package api

// archivos_test.go — Subida y servido de archivos por HTTP, de punta a
// punta (multipart → disco → SQLite → JSON), sin tocar el %APPDATA% real
// de quien corre `go test` (ver usarArchivosPrueba).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bythos-desktop/archivos"
	"bythos-desktop/db"
)

// usarArchivosPrueba redirige el almacén de archivos.Guardar a un
// t.TempDir() propio: sin esto, cualquier test que suba un archivo
// escribiría en el %APPDATA%\Bythos\archivos real de la máquina.
// usarArchivosPrueba redirige el almacén SIN llamar nunca a
// archivos.Base() con su cálculo perezoso real (ver comentario de
// baseDir en archivos/archivos.go): restaura a "" al terminar, no al
// valor real de antes, para que ningún test de este paquete termine
// creando %APPDATA%\Bythos en la máquina de quien corre `go test`.
func usarArchivosPrueba(t *testing.T) {
	t.Helper()
	archivos.UsarBase(t.TempDir())
	t.Cleanup(func() { archivos.UsarBase("") })
}

const pdfMinimo = "%PDF-1.4\n%âãÏÓ\n1 0 obj<</Type/Catalog>>endobj\n%%EOF"

// subirMultipart arma y ejecuta un POST /api/archivos con carpetaID +
// contenido/nombre de archivo dados, y devuelve la respuesta ya
// decodificada como mapa genérico (para no atarse a un struct por test).
func subirMultipart(t *testing.T, s *Servidor, carpetaID int64, nombreArchivo, contenido string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if carpetaID != 0 {
		if err := mw.WriteField("carpeta_id", fmt.Sprintf("%d", carpetaID)); err != nil {
			t.Fatalf("WriteField: %v", err)
		}
	}
	fw, err := mw.CreateFormFile("archivo", nombreArchivo)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := io.WriteString(fw, contenido); err != nil {
		t.Fatalf("write contenido: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("mw.Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/archivos", &buf)
	req.Host = "localhost:8080" // conGuardia exige Host permitido
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	s.Rutas().ServeHTTP(w, req)
	return w
}

func decodificarRecurso(t *testing.T, w *httptest.ResponseRecorder) recursoJSON {
	t.Helper()
	var rec recursoJSON
	if err := json.Unmarshal(w.Body.Bytes(), &rec); err != nil {
		t.Fatalf("decodificar respuesta: %v (body=%s)", err, w.Body.String())
	}
	return rec
}

func TestSubirArchivoPDFOk(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")

	w := subirMultipart(t, s, c.ID, "apuntes.pdf", pdfMinimo)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, quería 201: %s", w.Code, w.Body.String())
	}
	rec := decodificarRecurso(t, w)
	if rec.Tipo != "pdf" || rec.ArchivoID == 0 {
		t.Fatalf("recurso mal armado: %+v", rec)
	}
	if rec.Archivo == nil || rec.Archivo.Mime != "application/pdf" || rec.Archivo.NombreOriginal != "apuntes.pdf" {
		t.Fatalf("Archivo mal armado: %+v", rec.Archivo)
	}
	if rec.Titulo != "apuntes" {
		t.Fatalf("título debió ser el nombre sin extensión: %q", rec.Titulo)
	}
	if rec.Estado != db.EstadoPendiente {
		t.Fatalf("nace pendiente, salió %q", rec.Estado)
	}

	// El evento quedó en el historial.
	eventos, err := db.ListarEventos(s.Base, db.FiltroEventos{})
	if err != nil {
		t.Fatalf("ListarEventos: %v", err)
	}
	encontrado := false
	for _, e := range eventos {
		if e.Accion == db.AccionArchivoSubido && e.EntidadID == rec.ID {
			encontrado = true
		}
	}
	if !encontrado {
		t.Fatal("no quedó registrado el evento archivo_subido")
	}
}

func TestSubirArchivoDedupe(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c1, _ := db.CrearCarpeta(s.Base, "React")
	c2, _ := db.CrearCarpeta(s.Base, "Go")

	w1 := subirMultipart(t, s, c1.ID, "a.pdf", pdfMinimo)
	w2 := subirMultipart(t, s, c2.ID, "copia-otro-nombre.pdf", pdfMinimo)
	if w1.Code != http.StatusCreated || w2.Code != http.StatusCreated {
		t.Fatalf("ambas subidas debieron dar 201: %d / %d", w1.Code, w2.Code)
	}
	r1 := decodificarRecurso(t, w1)
	r2 := decodificarRecurso(t, w2)
	if r1.ArchivoID != r2.ArchivoID {
		t.Fatalf("mismo contenido debió compartir archivo_id: %d vs %d", r1.ArchivoID, r2.ArchivoID)
	}
	if n, err := db.ContarRecursosPorArchivo(s.Base, r1.ArchivoID); err != nil || n != 2 {
		t.Fatalf("refcount debió ser 2 tras el dedupe: n=%d err=%v", n, err)
	}
}

func TestSubirArchivoTipoRechazado(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")

	casos := []struct {
		nombre, archivo, contenido string
	}{
		{"html", "pagina.html", "<!DOCTYPE html><html><body><script>alert(1)</script></body></html>"},
		{"svg", "icono.svg", `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`},
		{"exe", "programa.exe", "MZ\x90\x00\x03\x00\x00\x00\x04\x00\x00\x00\xff\xff\x00\x00"},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			w := subirMultipart(t, s, c.ID, tt.archivo, tt.contenido)
			if w.Code != http.StatusUnsupportedMediaType {
				t.Fatalf("status = %d, quería 415: %s", w.Code, w.Body.String())
			}
			var cuerpo map[string]string
			json.Unmarshal(w.Body.Bytes(), &cuerpo)
			if !strings.Contains(cuerpo["error"], "no permitido") {
				t.Fatalf("el error debió estar en español y explicar el rechazo: %+v", cuerpo)
			}
		})
	}
}

func TestSubirArchivoDemasiadoGrande(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")

	anterior := archivos.TamanoMaximo
	archivos.TamanoMaximo = 100 // techo chico para no tener que mandar gigabytes de verdad
	t.Cleanup(func() { archivos.TamanoMaximo = anterior })

	// 6000 bytes supera de sobra TamanoMaximo + el margen del multipart
	// (ver margenMultipart en archivos.go); no necesita magic bytes
	// reales, el 413 dispara ANTES de llegar a detectar el tipo.
	contenidoGrande := strings.Repeat("A", 6000)
	w := subirMultipart(t, s, c.ID, "grande.pdf", contenidoGrande)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, quería 413: %s", w.Code, w.Body.String())
	}
}

func TestSubirArchivoNombreConPathTraversalNoAfectaRuta(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")

	w := subirMultipart(t, s, c.ID, "../../../etc/passwd.pdf", pdfMinimo)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, quería 201: %s", w.Code, w.Body.String())
	}
	rec := decodificarRecurso(t, w)
	if rec.Archivo == nil {
		t.Fatal("faltó Archivo en la respuesta")
	}
	if !strings.HasPrefix(rec.Archivo.RutaLocal, archivos.Base()) {
		t.Fatalf("la ruta absoluta debió quedar DENTRO del almacén: %q (base %q)", rec.Archivo.RutaLocal, archivos.Base())
	}
	if strings.Contains(rec.Archivo.RutaLocal, "etc") || strings.Contains(rec.Archivo.RutaLocal, "passwd") {
		t.Fatalf("el nombre hostil no debió filtrarse a la ruta en disco: %q", rec.Archivo.RutaLocal)
	}
}

func TestSubirArchivoSinCarpetaOSinArchivo(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")

	if w := subirMultipart(t, s, 0, "a.pdf", pdfMinimo); w.Code != 400 {
		t.Fatalf("sin carpeta_id debió dar 400, dio %d", w.Code)
	}

	// Multipart sin el part "archivo": solo carpeta_id.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("carpeta_id", fmt.Sprintf("%d", c.ID))
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/archivos", &buf)
	req.Host = "localhost:8080"
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	s.Rutas().ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("sin archivo debió dar 400, dio %d: %s", w.Code, w.Body.String())
	}
}

func TestContenidoArchivoSirveTipoYRange(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	rec := decodificarRecurso(t, subirMultipart(t, s, c.ID, "apuntes.pdf", pdfMinimo))

	ruta := fmt.Sprintf("/api/archivos/%d/contenido", rec.Archivo.ID)

	req := httptest.NewRequest(http.MethodGet, ruta, nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	s.Rutas().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, quería 200: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("faltó X-Content-Type-Options: nosniff")
	}
	// PDF: sin "sandbox" a propósito (rompe el visor nativo de Chromium,
	// ver comentario de contenidoArchivo en api/archivos.go), pero SÍ con
	// frame-ancestors 'self' para permitir el <iframe> same-origin de la
	// UI.
	if csp := w.Header().Get("Content-Security-Policy"); strings.Contains(csp, "sandbox") || !strings.Contains(csp, "frame-ancestors 'self'") {
		t.Fatalf("CSP de PDF mal armada: %q", csp)
	}
	if w.Body.String() != pdfMinimo {
		t.Fatal("el contenido servido no coincide con lo subido")
	}

	// Range: bytes=0-3 debe dar 206 con el pedazo correcto.
	req2 := httptest.NewRequest(http.MethodGet, ruta, nil)
	req2.Host = "localhost:8080"
	req2.Header.Set("Range", "bytes=0-3")
	w2 := httptest.NewRecorder()
	s.Rutas().ServeHTTP(w2, req2)
	if w2.Code != http.StatusPartialContent {
		t.Fatalf("status con Range = %d, quería 206: %s", w2.Code, w2.Body.String())
	}
	if w2.Body.String() != pdfMinimo[:4] {
		t.Fatalf("body parcial mal: %q", w2.Body.String())
	}
}

// TestContenidoArchivoFrameAncestorsSelfYSinDeny cubre el MAJOR: antes del
// fix, /contenido heredaba de conGuardia X-Frame-Options: DENY y pisaba
// la CSP con "frame-ancestors" ausente (solo "sandbox; default-src
// 'none'"), así que el <iframe> propio de la UI para ver el PDF quedaba
// bloqueado por su propio backend. Este test falla antes del fix porque
// encuentra DENY y no encuentra frame-ancestors 'self'.
func TestContenidoArchivoFrameAncestorsSelfYSinDeny(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	rec := decodificarRecurso(t, subirMultipart(t, s, c.ID, "apuntes.pdf", pdfMinimo))

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/archivos/%d/contenido", rec.Archivo.ID), nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	s.Rutas().ServeHTTP(w, req)

	if w.Header().Get("X-Frame-Options") == "DENY" {
		t.Fatalf("X-Frame-Options no debió ser DENY en /contenido (bloquea el <iframe> propio de la UI): %q", w.Header().Get("X-Frame-Options"))
	}
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'self'") {
		t.Fatalf("CSP debió incluir frame-ancestors 'self': %q", csp)
	}
}

// TestContenidoArchivoNoPDFMantieneSandbox confirma que la excepción de
// sandbox (ver comentario en contenidoArchivo) es SOLO para PDF: cualquier
// otro tipo servido por esta ruta sigue con "sandbox" en la CSP.
func TestContenidoArchivoNoPDFMantieneSandbox(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	firmaPNG := "\x89PNG\r\n\x1a\n" + strings.Repeat("x", 10)
	rec := decodificarRecurso(t, subirMultipart(t, s, c.ID, "foto.png", firmaPNG))

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/archivos/%d/contenido", rec.Archivo.ID), nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	s.Rutas().ServeHTTP(w, req)

	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") || !strings.Contains(csp, "frame-ancestors 'self'") {
		t.Fatalf("CSP de un no-PDF debió llevar sandbox + frame-ancestors 'self': %q", csp)
	}
}

// TestOtrasRutasMantienenDenyYFrameAncestorsNone confirma que el fix de
// /contenido no aflojó nada para el resto de la API: siguen con el DENY +
// frame-ancestors 'none' de conGuardia de siempre.
func TestOtrasRutasMantienenDenyYFrameAncestorsNone(t *testing.T) {
	s := basePrueba(t)
	req := httptest.NewRequest(http.MethodGet, "/api/carpetas", nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	s.Rutas().ServeHTTP(w, req)

	if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, quería DENY", got)
	}
	if got := w.Header().Get("Content-Security-Policy"); got != "frame-ancestors 'none'" {
		t.Fatalf("Content-Security-Policy = %q, quería frame-ancestors 'none'", got)
	}
}

// TestContenidoArchivoSecFetchSite cubre el hardening MAJOR: un
// <img>/<video src="http://localhost:8080/api/archivos/N/contenido">
// desde una página ajena no manda Origin (no es fetch/XHR), así que
// conGuardia lo deja pasar — pero todo navegador moderno sí manda
// Sec-Fetch-Site en esa carga, y ahí "cross-site" lo delata. Antes del
// fix, este header se ignoraba por completo: el caso "cross-site" daba
// 200 en vez de 403.
func TestContenidoArchivoSecFetchSite(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	rec := decodificarRecurso(t, subirMultipart(t, s, c.ID, "apuntes.pdf", pdfMinimo))
	ruta := fmt.Sprintf("/api/archivos/%d/contenido", rec.Archivo.ID)

	casos := []struct {
		nombre        string
		secFetchSite  string
		conHeader     bool
		statusQuerido int
	}{
		{"cross-site: 403", "cross-site", true, http.StatusForbidden},
		{"same-site: 403 (solo same-origin/none confían)", "same-site", true, http.StatusForbidden},
		{"same-origin: 200", "same-origin", true, http.StatusOK},
		{"none: 200 (navegación directa/marcador)", "none", true, http.StatusOK},
		{"ausente: 200 (curl, proceso MCP)", "", false, http.StatusOK},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, ruta, nil)
			req.Host = "localhost:8080"
			if tt.conHeader {
				req.Header.Set("Sec-Fetch-Site", tt.secFetchSite)
			}
			w := httptest.NewRecorder()
			s.Rutas().ServeHTTP(w, req)
			if w.Code != tt.statusQuerido {
				t.Fatalf("Sec-Fetch-Site=%q: status = %d, quería %d: %s", tt.secFetchSite, w.Code, tt.statusQuerido, w.Body.String())
			}
		})
	}
}

func TestContenidoArchivoInexistente(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	req := httptest.NewRequest(http.MethodGet, "/api/archivos/9999/contenido", nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	s.Rutas().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, quería 404", w.Code)
	}
}

// TestBorrarRecursoArchivoRefcount confirma el ciclo completo: el blob
// solo se borra de disco cuando el ÚLTIMO recurso que lo referencia
// desaparece (dos recursos pueden compartir un archivo por el dedupe).
func TestBorrarRecursoArchivoRefcount(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c1, _ := db.CrearCarpeta(s.Base, "React")
	c2, _ := db.CrearCarpeta(s.Base, "Go")

	r1 := decodificarRecurso(t, subirMultipart(t, s, c1.ID, "a.pdf", pdfMinimo))
	r2 := decodificarRecurso(t, subirMultipart(t, s, c2.ID, "b.pdf", pdfMinimo)) // mismo contenido → mismo archivo (dedupe)

	// Borrar el primero: el blob sigue en disco (r2 lo sigue usando).
	reqDel1 := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/recursos/%d", r1.ID), nil)
	reqDel1.Host = "localhost:8080"
	wDel1 := httptest.NewRecorder()
	s.Rutas().ServeHTTP(wDel1, reqDel1)
	if wDel1.Code != http.StatusOK {
		t.Fatalf("borrar r1: status %d: %s", wDel1.Code, wDel1.Body.String())
	}
	if _, ok, _ := db.ObtenerArchivo(s.Base, r1.ArchivoID); !ok {
		t.Fatal("el archivo no debió borrarse: r2 lo sigue referenciando")
	}

	// Borrar el segundo (el último): ahora sí, blob + fila desaparecen.
	reqDel2 := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/recursos/%d", r2.ID), nil)
	reqDel2.Host = "localhost:8080"
	wDel2 := httptest.NewRecorder()
	s.Rutas().ServeHTTP(wDel2, reqDel2)
	if wDel2.Code != http.StatusOK {
		t.Fatalf("borrar r2: status %d: %s", wDel2.Code, wDel2.Body.String())
	}
	if _, ok, _ := db.ObtenerArchivo(s.Base, r2.ArchivoID); ok {
		t.Fatal("el archivo debió borrarse tras quedar sin referencias")
	}
}

func TestExportarCarpetaMuestraArchivoLocal(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	decodificarRecurso(t, subirMultipart(t, s, c.ID, "apuntes.pdf", pdfMinimo))

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/carpetas/%d/export?format=gemini", c.ID), nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	s.Rutas().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "archivo local") {
		t.Fatalf("el export debió avisar que es un archivo local, no un link roto: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "archivo:") {
		t.Fatalf("el export nunca debió mostrar la forma interna \"archivo:<id>\": %s", w.Body.String())
	}
}
