package archivos

// archivos_test.go — El almacén bajo lupa: detección por contenido,
// dedupe, y que un nombre de cliente hostil ("../../evil") jamás toque
// una ruta en disco. Cada test apunta Base() a su propio t.TempDir()
// (ver usarBasePrueba) para no tocar el %APPDATA% real de quien corre
// `go test`.

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// usarBasePrueba redirige el almacén a un t.TempDir() propio SIN llamar
// nunca a Base() con su cálculo perezoso real (ver comentario de baseDir
// en archivos.go): restaura a "" al terminar, no al valor real de
// antes, para que ningún test de este paquete termine creando
// %APPDATA%\Bythos en la máquina de quien corre `go test`.
func usarBasePrueba(t *testing.T) {
	t.Helper()
	UsarBase(t.TempDir())
	t.Cleanup(func() { UsarBase("") })
}

// pdfMinimo es lo mínimo que detectarTipo necesita: el prefijo mágico.
// No hace falta un PDF válido de verdad, la lista blanca solo mira bytes.
const pdfMinimo = "%PDF-1.4\n%âãÏÓ\n1 0 obj<</Type/Catalog>>endobj\n%%EOF"

func TestGuardarPDFOk(t *testing.T) {
	usarBasePrueba(t)
	g, err := Guardar(strings.NewReader(pdfMinimo), "apuntes.pdf")
	if err != nil {
		t.Fatalf("Guardar: %v", err)
	}
	if g.Mime != "application/pdf" {
		t.Fatalf("mime mal detectado: %q", g.Mime)
	}
	if g.Tamano != int64(len(pdfMinimo)) {
		t.Fatalf("tamaño mal: %d, quería %d", g.Tamano, len(pdfMinimo))
	}
	if !strings.HasSuffix(g.RutaRelativa, ".pdf") {
		t.Fatalf("ruta debió terminar en .pdf: %q", g.RutaRelativa)
	}
	if g.Reusado {
		t.Fatal("primera subida no debió marcarse como reusada")
	}
	if _, err := os.Stat(Absoluta(g.RutaRelativa)); err != nil {
		t.Fatalf("el blob debió existir en disco: %v", err)
	}
}

func TestGuardarDedupePorContenido(t *testing.T) {
	usarBasePrueba(t)
	g1, err := Guardar(strings.NewReader(pdfMinimo), "a.pdf")
	if err != nil {
		t.Fatalf("primera subida: %v", err)
	}
	g2, err := Guardar(strings.NewReader(pdfMinimo), "copia-con-otro-nombre.pdf")
	if err != nil {
		t.Fatalf("segunda subida: %v", err)
	}
	if g1.SHA256 != g2.SHA256 || g1.RutaRelativa != g2.RutaRelativa {
		t.Fatalf("mismo contenido debió dar el mismo hash/ruta: %+v vs %+v", g1, g2)
	}
	if !g2.Reusado {
		t.Fatal("la segunda subida (mismo contenido) debió marcarse como reusada")
	}
}

func TestGuardarRechazaHTML(t *testing.T) {
	usarBasePrueba(t)
	_, err := Guardar(strings.NewReader("<!DOCTYPE html><html><body><script>alert(1)</script></body></html>"), "pagina.html")
	if err == nil {
		t.Fatal("HTML debió rechazarse")
	}
	if !strings.Contains(err.Error(), "no permitido") {
		t.Fatalf("error debió explicar el rechazo: %v", err)
	}
}

func TestGuardarRechazaSVG(t *testing.T) {
	usarBasePrueba(t)
	_, err := Guardar(strings.NewReader(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), "icono.svg")
	if err == nil {
		t.Fatal("SVG debió rechazarse (es XML/markup, corre script en el origen si se sirve tal cual)")
	}
}

func TestGuardarRechazaEjecutable(t *testing.T) {
	usarBasePrueba(t)
	// Cabecera MZ de un .exe de Windows: binario, no matchea ningún
	// formato de la lista blanca y no es texto (bytes fuera de rango,
	// probablemente algún NUL).
	cabeceraEXE := []byte{'M', 'Z', 0x90, 0x00, 0x03, 0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0xff, 0xff, 0x00, 0x00}
	_, err := Guardar(bytes.NewReader(cabeceraEXE), "programa.exe")
	if err == nil {
		t.Fatal(".exe debió rechazarse")
	}
}

func TestGuardarTxtYMarkdown(t *testing.T) {
	usarBasePrueba(t)
	gt, err := Guardar(strings.NewReader("hola mundo, esto es texto plano"), "nota.txt")
	if err != nil || gt.Mime != "text/plain" || !strings.HasSuffix(gt.RutaRelativa, ".txt") {
		t.Fatalf("txt mal: %+v err=%v", gt, err)
	}
	gm, err := Guardar(strings.NewReader("# Título\n\nUn markdown corto."), "notas.md")
	if err != nil || gm.Mime != "text/plain" || !strings.HasSuffix(gm.RutaRelativa, ".md") {
		t.Fatalf("md mal: %+v err=%v", gm, err)
	}
}

func TestGuardarPNG(t *testing.T) {
	usarBasePrueba(t)
	firma := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0}
	g, err := Guardar(bytes.NewReader(firma), "foto.png")
	if err != nil || g.Mime != "image/png" {
		t.Fatalf("png mal: %+v err=%v", g, err)
	}
}

func TestGuardarDocx(t *testing.T) {
	usarBasePrueba(t)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	escribir := func(nombre, contenido string) {
		w, err := zw.Create(nombre)
		if err != nil {
			t.Fatalf("zip.Create: %v", err)
		}
		if _, err := w.Write([]byte(contenido)); err != nil {
			t.Fatalf("zip write: %v", err)
		}
	}
	escribir("[Content_Types].xml", "<Types/>")
	escribir("word/document.xml", "<w:document/>")
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}

	g, err := Guardar(bytes.NewReader(buf.Bytes()), "informe.docx")
	if err != nil {
		t.Fatalf("Guardar docx: %v", err)
	}
	if g.Mime != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Fatalf("mime docx mal detectado: %q", g.Mime)
	}
	if !strings.HasSuffix(g.RutaRelativa, ".docx") {
		t.Fatalf("ruta debió terminar en .docx: %q", g.RutaRelativa)
	}
}

// zipOfficeConContentTypes arma un ZIP mínimo con forma de Office
// (carpeta principal + [Content_Types].xml con el texto dado) para probar
// detectarZipOffice sin tener que generar un .docm/.xlsm/.pptm real.
func zipOfficeConContentTypes(t *testing.T, carpeta, contentTypes string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	escribir := func(nombre, contenido string) {
		w, err := zw.Create(nombre)
		if err != nil {
			t.Fatalf("zip.Create(%q): %v", nombre, err)
		}
		if _, err := w.Write([]byte(contenido)); err != nil {
			t.Fatalf("zip write(%q): %v", nombre, err)
		}
	}
	escribir("[Content_Types].xml", contentTypes)
	escribir(carpeta+"/document.xml", "<contenido/>")
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
	return buf.Bytes()
}

// TestGuardarRechazaOfficeMacroHabilitado cubre el MINOR: un
// .docm/.xlsm/.pptm es un ZIP con la MISMA forma (word/, xl/, ppt/ +
// [Content_Types].xml) que su equivalente sin macros — antes del fix,
// detectarZipOffice solo miraba esa forma y dejaba pasar el docm/xlsm/pptm
// disfrazado de docx/xlsx/pptx. El content type
// "...macroEnabled.main+xml" en [Content_Types].xml es la señal real de
// que hay macros.
func TestGuardarRechazaOfficeMacroHabilitado(t *testing.T) {
	casos := []struct {
		nombre, carpeta, extCliente, contentType string
	}{
		{"docm", "word", "informe.docm", "vnd.ms-word.document.macroEnabled.main+xml"},
		{"xlsm", "xl", "planilla.xlsm", "vnd.ms-excel.sheet.macroEnabled.main+xml"},
		{"pptm", "ppt", "diapos.pptm", "vnd.ms-powerpoint.presentation.macroEnabled.main+xml"},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			usarBasePrueba(t)
			contentTypes := `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
				`<Override PartName="/` + tt.carpeta + `/document.xml" ContentType="application/` + tt.contentType + `"/></Types>`
			datos := zipOfficeConContentTypes(t, tt.carpeta, contentTypes)
			_, err := Guardar(bytes.NewReader(datos), tt.extCliente)
			if err == nil {
				t.Fatalf("un %s (macro-habilitado) debió rechazarse, no pasar como Office sin macros", tt.nombre)
			}
			if !strings.Contains(err.Error(), "no permitido") {
				t.Fatalf("el error debió explicar el rechazo: %v", err)
			}
		})
	}
}

// TestGuardarRechazaVbaProjectSinDeclararEnContentTypes cubre el caso
// límite donde [Content_Types].xml NO declara ningún content type
// macroEnabled (por ejemplo, un .docm armado a mano o con metadatos
// manipulados) pero el paquete igual trae vbaProject.bin: la sola
// presencia del binario de macros alcanza para rechazarlo.
func TestGuardarRechazaVbaProjectSinDeclararEnContentTypes(t *testing.T) {
	usarBasePrueba(t)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	escribir := func(nombre, contenido string) {
		w, err := zw.Create(nombre)
		if err != nil {
			t.Fatalf("zip.Create: %v", err)
		}
		w.Write([]byte(contenido))
	}
	escribir("[Content_Types].xml", "<Types/>") // sin mención de macroEnabled
	escribir("word/document.xml", "<w:document/>")
	escribir("word/vbaProject.bin", "binario-de-macros-falso")
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}

	_, err := Guardar(bytes.NewReader(buf.Bytes()), "informe.docm")
	if err == nil {
		t.Fatal("un paquete con vbaProject.bin debió rechazarse aunque Content_Types no lo declare")
	}
}

func TestGuardarZipGenericoRechazado(t *testing.T) {
	usarBasePrueba(t)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("cualquier-cosa.txt")
	w.Write([]byte("no es un documento de Office"))
	zw.Close()

	_, err := Guardar(bytes.NewReader(buf.Bytes()), "archivo.zip")
	if err == nil {
		t.Fatal("un ZIP genérico (no Office) debió rechazarse")
	}
}

// TestGuardarNombreClienteNuncaAfectaLaRuta es el test de seguridad
// central de este paquete: un nombre de cliente con "../" (path
// traversal) o separadores no debe poder escapar Base() ni cambiar dónde
// termina el blob — porque el nombre NUNCA se usa para construir la
// ruta en disco (ver comentario de Guardar), solo el sha256.
func TestGuardarNombreClienteNuncaAfectaLaRuta(t *testing.T) {
	usarBasePrueba(t)
	nombresHostiles := []string{
		"../../../etc/passwd.pdf",
		"..\\..\\windows\\system32\\evil.pdf",
		"/etc/passwd",
		"",
		strings.Repeat("a", 500) + ".pdf",
	}
	for _, nombre := range nombresHostiles {
		t.Run(nombre, func(t *testing.T) {
			g, err := Guardar(strings.NewReader(pdfMinimo), nombre)
			if err != nil {
				t.Fatalf("Guardar no debió fallar por el nombre: %v", err)
			}
			abs := Absoluta(g.RutaRelativa)
			rel, err := filepath.Rel(Base(), abs)
			if err != nil || strings.HasPrefix(rel, "..") {
				t.Fatalf("la ruta final escapó de Base(): abs=%q rel=%q err=%v", abs, rel, err)
			}
			if g.NombreOriginal == nombre && (strings.Contains(nombre, "/") || strings.Contains(nombre, "\\")) {
				t.Fatalf("NombreOriginal debió sanearse (sin separadores): %q", g.NombreOriginal)
			}
		})
	}
}

func TestGuardarRechazaArchivoVacio(t *testing.T) {
	usarBasePrueba(t)
	if _, err := Guardar(strings.NewReader(""), "vacio.pdf"); err == nil {
		t.Fatal("archivo vacío debió rechazarse")
	}
}

func TestGuardarNoAutolimitaSinWrapperHTTP(t *testing.T) {
	usarBasePrueba(t)
	anterior := TamanoMaximo
	TamanoMaximo = 8 // téco chico: cualquier cosa de más de 8 bytes ya no entra
	t.Cleanup(func() { TamanoMaximo = anterior })

	// io.Copy no conoce TamanoMaximo (ese techo lo aplica el caller HTTP
	// con http.MaxBytesReader sobre r.Body, ver api/archivos.go); este
	// test solo confirma que Guardar no impone SU PROPIO techo por
	// error — sigue aceptando más de 8 bytes cuando nadie envuelve el
	// reader. La prueba real de 413 vive en api/archivos_test.go.
	g, err := Guardar(strings.NewReader(pdfMinimo), "grande.pdf")
	if err != nil {
		t.Fatalf("Guardar sin wrapper HTTP no debe autolimitarse: %v", err)
	}
	if g.Tamano != int64(len(pdfMinimo)) {
		t.Fatalf("tamaño mal: %d", g.Tamano)
	}
}

// TestGuardarDedupeYAbortadoNoDejanTemporalHuerfano cubre el BLOCKER: la
// rama de dedupe marcaba huboExito=true sin haber tocado tmpPath, así que
// el defer de limpieza nunca corría y el temporal (tamaño completo del
// archivo) quedaba huérfano en el almacén en CADA subida duplicada. Antes
// del fix este test falla porque queda un "tmp-subida-*" en Base().
// También confirma que una subida cortada a mitad de camino (io.Copy con
// error) sigue sin dejar basura, para que no se rompa mientras se toca
// esta zona.
func TestGuardarDedupeYAbortadoNoDejanTemporalHuerfano(t *testing.T) {
	usarBasePrueba(t)

	if _, err := Guardar(strings.NewReader(pdfMinimo), "a.pdf"); err != nil {
		t.Fatalf("primera subida: %v", err)
	}
	if _, err := Guardar(strings.NewReader(pdfMinimo), "b.pdf"); err != nil {
		t.Fatalf("segunda subida (dedupe): %v", err)
	}
	if _, err := Guardar(&lectorQueFalla{restante: 5}, "c.pdf"); err == nil {
		t.Fatal("la subida cortada a mitad de camino debió fallar")
	}

	huerfanos, err := filepath.Glob(filepath.Join(Base(), "tmp-subida-*"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(huerfanos) != 0 {
		t.Fatalf("quedaron temporales huérfanos tras el dedupe/aborto: %v", huerfanos)
	}
	blobs, err := filepath.Glob(filepath.Join(Base(), "*", "*.pdf"))
	if err != nil {
		t.Fatalf("Glob blobs: %v", err)
	}
	if len(blobs) != 1 {
		t.Fatalf("debió quedar exactamente un blob final (dedupe): %v", blobs)
	}
}

// lectorQueFalla simula una conexión cortada a mitad de una subida:
// entrega `restante` bytes y a partir de ahí siempre devuelve error, para
// que io.Copy dentro de Guardar aborte y ejercite el defer de limpieza.
type lectorQueFalla struct{ restante int }

func (l *lectorQueFalla) Read(p []byte) (int, error) {
	if l.restante <= 0 {
		return 0, errors.New("conexión cortada a mitad de la subida")
	}
	n := len(p)
	if n > l.restante {
		n = l.restante
	}
	l.restante -= n
	return n, nil
}

func TestEliminarEsIdempotente(t *testing.T) {
	usarBasePrueba(t)
	g, err := Guardar(strings.NewReader(pdfMinimo), "a.pdf")
	if err != nil {
		t.Fatalf("Guardar: %v", err)
	}
	if err := Eliminar(g.RutaRelativa); err != nil {
		t.Fatalf("primer Eliminar: %v", err)
	}
	if _, err := os.Stat(Absoluta(g.RutaRelativa)); !os.IsNotExist(err) {
		t.Fatalf("el blob debió desaparecer: err=%v", err)
	}
	if err := Eliminar(g.RutaRelativa); err != nil {
		t.Fatalf("segundo Eliminar (ya borrado) no debió fallar: %v", err)
	}
}

func TestTipoRecurso(t *testing.T) {
	casos := map[string]string{
		"application/pdf": "pdf",
		"video/mp4":       "video",
		"video/webm":      "video",
		"image/png":       "imagen",
		"image/jpeg":      "imagen",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": "documento",
		"text/plain": "documento",
	}
	for mime, esperado := range casos {
		if got := TipoRecurso(mime); got != esperado {
			t.Errorf("TipoRecurso(%q) = %q, quería %q", mime, got, esperado)
		}
	}
}
