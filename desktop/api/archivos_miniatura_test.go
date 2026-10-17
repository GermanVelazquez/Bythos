package api

// archivos_miniatura_test.go — GET /api/archivos/{id}/miniatura: preview
// chico (JPEG) para las tarjetas de recurso (ver
// desktop/archivos/miniaturas.go). Mismo criterio de aislamiento que
// archivos_test.go: usarArchivosPrueba redirige el almacén a un
// t.TempDir(), nunca toca el %APPDATA%\Bythos real.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bythos-desktop/archivos"
	"bythos-desktop/db"
)

// pngPrueba/jpegPrueba generan una imagen real (no solo magic bytes) con
// image/png y image/jpeg de la std lib: Miniatura() necesita decodificar
// los píxeles de verdad, así que a diferencia de otros tests de este
// paquete acá no alcanza con una cabecera de mentira.
func pngPrueba(t *testing.T, ancho, alto int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, ancho, alto))
	for y := 0; y < alto; y++ {
		for x := 0; x < ancho; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.String()
}

func jpegPrueba(t *testing.T, ancho, alto int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, ancho, alto))
	for y := 0; y < alto; y++ {
		for x := 0; x < ancho; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 64, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	return buf.String()
}

// pngConDimensionesFalsas arma solo la firma PNG + un chunk IHDR válido
// (CRC correcto) que declara un ancho×alto gigante. image.DecodeConfig
// para un PNG no paletizado (ver image/png.DecodeConfig) corta la
// lectura apenas terminó de parsear IHDR — no necesita PLTE/IDAT/IEND
// para devolver las dimensiones — así que esto alcanza para que
// Miniatura() vea "100 millones de píxeles declarados" y rechace por el
// guardia de decompression bomb, SIN tener que generar en el test una
// imagen real de varios GB.
func pngConDimensionesFalsas(t *testing.T, ancho, alto uint32) string {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("\x89PNG\r\n\x1a\n")

	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], ancho)
	binary.BigEndian.PutUint32(ihdr[4:8], alto)
	ihdr[8] = 8 // profundidad de bits
	ihdr[9] = 0 // tipo de color: escala de grises (no necesita PLTE)
	// ihdr[10..12] (compresión/filtro/interlace) quedan en 0, únicos
	// valores válidos para esos campos.

	var l [4]byte
	binary.BigEndian.PutUint32(l[:], uint32(len(ihdr)))
	buf.Write(l[:])
	buf.WriteString("IHDR")
	buf.Write(ihdr)
	crc := crc32.NewIEEE()
	crc.Write([]byte("IHDR"))
	crc.Write(ihdr)
	var c [4]byte
	binary.BigEndian.PutUint32(c[:], crc.Sum32())
	buf.Write(c[:])

	return buf.String()
}

// webpMinimo es la cabecera RIFF/WEBP mínima para que detectarTipo (ver
// archivos/archivos.go) acepte la subida como image/webp — la std lib de
// Go no trae decoder de WEBP, así que Miniatura() la rechaza siempre (ver
// mimeSoportaMiniatura).
const webpMinimo = "RIFF\x00\x00\x00\x00WEBP"

func pedirMiniatura(t *testing.T, s *Servidor, archivoID int64) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/archivos/%d/miniatura", archivoID), nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	s.Rutas().ServeHTTP(w, req)
	return w
}

func TestMiniaturaPNGGeneradaYCacheada(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	rec := decodificarRecurso(t, subirMultipart(t, s, c.ID, "foto.png", pngPrueba(t, 400, 300)))

	w := pedirMiniatura(t, s, rec.Archivo.ID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, quería 200: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("Content-Type = %q, quería image/jpeg", ct)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("faltó X-Content-Type-Options: nosniff")
	}
	if w.Header().Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Fatalf("X-Frame-Options = %q, quería SAMEORIGIN", w.Header().Get("X-Frame-Options"))
	}
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") || !strings.Contains(csp, "frame-ancestors 'self'") {
		t.Fatalf("CSP mal armada: %q", csp)
	}

	cfg, err := jpeg.DecodeConfig(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("la miniatura no es un JPEG válido: %v", err)
	}
	if cfg.Width > 320 || cfg.Height > 320 {
		t.Fatalf("miniatura mayor a 320px en el lado largo: %dx%d", cfg.Width, cfg.Height)
	}
	if cfg.Width != 320 {
		t.Fatalf("el lado más largo (ancho, 400 original) debió quedar en 320, salió %d", cfg.Width)
	}

	a, ok, err := db.ObtenerArchivo(s.Base, rec.Archivo.ID)
	if err != nil || !ok {
		t.Fatalf("ObtenerArchivo: ok=%v err=%v", ok, err)
	}
	rutaCache := filepath.Join(archivos.Base(), "miniaturas", a.SHA256+".jpg")
	info1, err := os.Stat(rutaCache)
	if err != nil {
		t.Fatalf("la miniatura debió quedar cacheada en disco: %v", err)
	}

	// Segundo pedido del mismo archivo: no debió regenerarse (mismo
	// ModTime que la primera vez).
	w2 := pedirMiniatura(t, s, rec.Archivo.ID)
	if w2.Code != http.StatusOK {
		t.Fatalf("segundo pedido: status = %d", w2.Code)
	}
	info2, err := os.Stat(rutaCache)
	if err != nil {
		t.Fatalf("Stat tras segundo pedido: %v", err)
	}
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Fatal("la miniatura se regeneró en el segundo pedido: debió reusar la cacheada")
	}
}

func TestMiniaturaJPEGDimensionesDentroDelLimite(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	rec := decodificarRecurso(t, subirMultipart(t, s, c.ID, "foto.jpg", jpegPrueba(t, 100, 80)))

	w := pedirMiniatura(t, s, rec.Archivo.ID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, quería 200: %s", w.Code, w.Body.String())
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("la miniatura no es un JPEG válido: %v", err)
	}
	if cfg.Width > 320 || cfg.Height > 320 {
		t.Fatalf("miniatura mayor a 320px en el lado largo: %dx%d", cfg.Width, cfg.Height)
	}
}

func TestMiniaturaCrossSite403(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	rec := decodificarRecurso(t, subirMultipart(t, s, c.ID, "foto.png", pngPrueba(t, 40, 40)))

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/archivos/%d/miniatura", rec.Archivo.ID), nil)
	req.Host = "localhost:8080"
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	s.Rutas().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, quería 403: %s", w.Code, w.Body.String())
	}
}

func TestMiniaturaImagenDemasiadoGrandeDa404(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	// 10000x10000 = 100 millones de píxeles declarados, supera el techo
	// de 50 megapíxeles del guardia contra decompression bombs.
	rec := decodificarRecurso(t, subirMultipart(t, s, c.ID, "bomba.png", pngConDimensionesFalsas(t, 10000, 10000)))

	w := pedirMiniatura(t, s, rec.Archivo.ID)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, quería 404 (guardia de decompression bomb): %s", w.Code, w.Body.String())
	}
}

func TestMiniaturaWebpDa404(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	rec := decodificarRecurso(t, subirMultipart(t, s, c.ID, "foto.webp", webpMinimo))

	w := pedirMiniatura(t, s, rec.Archivo.ID)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, quería 404 (webp sin decoder en la std lib): %s", w.Code, w.Body.String())
	}
}

// TestMiniaturaSeBorraAlBorrarBlob cubre el refcount cleanup: cuando el
// último recurso que referencia un archivo se borra, la miniatura
// cacheada tiene que desaparecer junto con el blob (ver EliminarMiniatura
// y limpiarArchivoSiHuerfano) — si no, quedaría un .jpg huérfano en disco
// para siempre por cada archivo borrado que alguna vez tuvo preview.
func TestMiniaturaSeBorraAlBorrarBlob(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	rec := decodificarRecurso(t, subirMultipart(t, s, c.ID, "foto.png", pngPrueba(t, 400, 300)))

	if w := pedirMiniatura(t, s, rec.Archivo.ID); w.Code != http.StatusOK {
		t.Fatalf("preparación: status = %d", w.Code)
	}
	a, ok, err := db.ObtenerArchivo(s.Base, rec.Archivo.ID)
	if err != nil || !ok {
		t.Fatalf("ObtenerArchivo: ok=%v err=%v", ok, err)
	}
	rutaCache := filepath.Join(archivos.Base(), "miniaturas", a.SHA256+".jpg")
	if _, err := os.Stat(rutaCache); err != nil {
		t.Fatalf("preparación: la miniatura debió existir: %v", err)
	}

	reqDel := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/recursos/%d", rec.ID), nil)
	reqDel.Host = "localhost:8080"
	wDel := httptest.NewRecorder()
	s.Rutas().ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("borrar recurso: status %d: %s", wDel.Code, wDel.Body.String())
	}

	if _, err := os.Stat(rutaCache); !os.IsNotExist(err) {
		t.Fatalf("la miniatura cacheada debió borrarse junto con el blob: err=%v", err)
	}
}
