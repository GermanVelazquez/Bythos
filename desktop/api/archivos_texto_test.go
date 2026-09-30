package api

// archivos_texto_test.go — GET /api/archivos/{id}/texto: vista de texto
// de documentos de Office (docx/xlsx/pptx) y texto plano, para el visor
// (ver desktop/archivos/texto.go). Los docx/xlsx/pptx de prueba se arman
// a mano como ZIP mínimo (archive/zip), sin depender de Word/Excel/
// PowerPoint reales — alcanza con la forma que detectarZipOffice y los
// parsers de texto.go esperan.

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bythos-desktop/db"
)

// contentTypesGenerico es el [Content_Types].xml mínimo que
// detectarZipOffice (ver archivos/archivos.go) necesita para aceptar la
// subida como Office moderno: no declara ningún content type de macro
// (.docm/.xlsm/.pptm), así que nunca dispara el rechazo de
// zipEsMacroHabilitado.
const contentTypesGenerico = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="xml" ContentType="application/xml"/>
</Types>`

// construirZip arma un ZIP en memoria a partir de nombre->contenido,
// incluyendo SIEMPRE "[Content_Types].xml" (lo que detectarZipOffice
// exige para reconocer el paquete como Office).
func construirZip(t *testing.T, entradas map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	todas := map[string]string{"[Content_Types].xml": contentTypesGenerico}
	for k, v := range entradas {
		todas[k] = v
	}
	for nombre, contenido := range todas {
		fw, err := zw.Create(nombre)
		if err != nil {
			t.Fatalf("zip.Create(%q): %v", nombre, err)
		}
		if _, err := io.WriteString(fw, contenido); err != nil {
			t.Fatalf("escribir %q: %v", nombre, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zw.Close: %v", err)
	}
	return buf.Bytes()
}

func pedirTexto(t *testing.T, s *Servidor, archivoID int64) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/archivos/%d/texto", archivoID), nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	s.Rutas().ServeHTTP(w, req)
	return w
}

// textoExtraidoJSON refleja el contrato JSON de archivos.TextoExtraido
// (claves en minúscula, ver el comentario de ese tipo en texto.go).
type textoExtraidoJSON struct {
	Tipo    string `json:"tipo"`
	Bloques []struct {
		Tipo  string `json:"tipo"`
		Nivel int    `json:"nivel"`
		Texto string `json:"texto"`
	} `json:"bloques"`
	Diapositivas []struct {
		Numero   int      `json:"numero"`
		Parrafos []string `json:"parrafos"`
	} `json:"diapositivas"`
	Filas    [][]string `json:"filas"`
	Texto    string     `json:"texto"`
	Truncado bool       `json:"truncado"`
}

func decodificarTexto(t *testing.T, w *httptest.ResponseRecorder) textoExtraidoJSON {
	t.Helper()
	var te textoExtraidoJSON
	if err := json.Unmarshal(w.Body.Bytes(), &te); err != nil {
		t.Fatalf("decodificar /texto: %v (body=%s)", err, w.Body.String())
	}
	return te
}

// --- DOCX ---

const documentoDocxPrueba = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Título de prueba</w:t></w:r></w:p>
<w:p><w:r><w:t>Este es un párrafo normal.</w:t></w:r></w:p>
<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr><w:r><w:t>Ítem de lista</w:t></w:r></w:p>
</w:body>
</w:document>`

func TestTextoDocxTituloParrafoLista(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")

	zipBytes := construirZip(t, map[string]string{"word/document.xml": documentoDocxPrueba})
	rec := decodificarRecurso(t, subirMultipart(t, s, c.ID, "apuntes.docx", string(zipBytes)))
	if rec.Archivo == nil {
		t.Fatalf("la subida del docx de prueba falló: %+v", rec)
	}

	w := pedirTexto(t, s, rec.Archivo.ID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, quería 200: %s", w.Code, w.Body.String())
	}
	te := decodificarTexto(t, w)
	if te.Tipo != "docx" {
		t.Fatalf("tipo = %q, quería docx", te.Tipo)
	}
	if len(te.Bloques) != 3 {
		t.Fatalf("bloques = %d, quería 3: %+v", len(te.Bloques), te.Bloques)
	}
	if te.Bloques[0].Tipo != "titulo" || te.Bloques[0].Nivel != 1 || te.Bloques[0].Texto != "Título de prueba" {
		t.Fatalf("bloque 0 (título) mal: %+v", te.Bloques[0])
	}
	if te.Bloques[1].Tipo != "parrafo" || te.Bloques[1].Texto != "Este es un párrafo normal." {
		t.Fatalf("bloque 1 (párrafo) mal: %+v", te.Bloques[1])
	}
	if te.Bloques[2].Tipo != "lista" || te.Bloques[2].Texto != "Ítem de lista" {
		t.Fatalf("bloque 2 (lista) mal: %+v", te.Bloques[2])
	}
	if te.Truncado {
		t.Fatal("un docx chico no debió venir truncado")
	}
}

// --- PPTX ---

func diapositivaPptxPrueba(texto string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
<p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>%s</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld>
</p:sld>`, texto)
}

func TestTextoPptxDosSlidesEnOrden(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")

	zipBytes := construirZip(t, map[string]string{
		"ppt/slides/slide1.xml": diapositivaPptxPrueba("Texto de la diapositiva 1"),
		"ppt/slides/slide2.xml": diapositivaPptxPrueba("Texto de la diapositiva 2"),
	})
	rec := decodificarRecurso(t, subirMultipart(t, s, c.ID, "charla.pptx", string(zipBytes)))
	if rec.Archivo == nil {
		t.Fatalf("la subida del pptx de prueba falló: %+v", rec)
	}

	w := pedirTexto(t, s, rec.Archivo.ID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, quería 200: %s", w.Code, w.Body.String())
	}
	te := decodificarTexto(t, w)
	if te.Tipo != "pptx" {
		t.Fatalf("tipo = %q, quería pptx", te.Tipo)
	}
	if len(te.Diapositivas) != 2 {
		t.Fatalf("diapositivas = %d, quería 2: %+v", len(te.Diapositivas), te.Diapositivas)
	}
	if te.Diapositivas[0].Numero != 1 || len(te.Diapositivas[0].Parrafos) != 1 || te.Diapositivas[0].Parrafos[0] != "Texto de la diapositiva 1" {
		t.Fatalf("diapositiva 1 mal: %+v", te.Diapositivas[0])
	}
	if te.Diapositivas[1].Numero != 2 || len(te.Diapositivas[1].Parrafos) != 1 || te.Diapositivas[1].Parrafos[0] != "Texto de la diapositiva 2" {
		t.Fatalf("diapositiva 2 mal: %+v", te.Diapositivas[1])
	}
}

// --- XLSX ---

const sharedStringsPrueba = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="2" uniqueCount="2">
<si><t>Hola</t></si>
<si><t>Mundo</t></si>
</sst>`

const hojaXlsxPrueba = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<sheetData>
<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row>
<row r="2"><c r="A2"><v>42</v></c></row>
</sheetData>
</worksheet>`

func TestTextoXlsxResuelveSharedStrings(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")

	zipBytes := construirZip(t, map[string]string{
		"xl/worksheets/sheet1.xml": hojaXlsxPrueba,
		"xl/sharedStrings.xml":     sharedStringsPrueba,
	})
	rec := decodificarRecurso(t, subirMultipart(t, s, c.ID, "planilla.xlsx", string(zipBytes)))
	if rec.Archivo == nil {
		t.Fatalf("la subida del xlsx de prueba falló: %+v", rec)
	}

	w := pedirTexto(t, s, rec.Archivo.ID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, quería 200: %s", w.Code, w.Body.String())
	}
	te := decodificarTexto(t, w)
	if te.Tipo != "xlsx" {
		t.Fatalf("tipo = %q, quería xlsx", te.Tipo)
	}
	if len(te.Filas) != 2 {
		t.Fatalf("filas = %d, quería 2: %+v", len(te.Filas), te.Filas)
	}
	if len(te.Filas[0]) != 2 || te.Filas[0][0] != "Hola" || te.Filas[0][1] != "Mundo" {
		t.Fatalf("fila 0 (shared strings) mal: %+v", te.Filas[0])
	}
	if len(te.Filas[1]) != 1 || te.Filas[1][0] != "42" {
		t.Fatalf("fila 1 (número directo) mal: %+v", te.Filas[1])
	}
}

// --- TXT ---

func TestTextoPlano(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")

	contenido := "Hola en texto plano\nSegunda línea"
	rec := decodificarRecurso(t, subirMultipart(t, s, c.ID, "notas.txt", contenido))
	if rec.Archivo == nil {
		t.Fatalf("la subida del txt de prueba falló: %+v", rec)
	}

	w := pedirTexto(t, s, rec.Archivo.ID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, quería 200: %s", w.Code, w.Body.String())
	}
	te := decodificarTexto(t, w)
	if te.Tipo != "texto" {
		t.Fatalf("tipo = %q, quería texto", te.Tipo)
	}
	if te.Texto != contenido {
		t.Fatalf("texto = %q, quería %q", te.Texto, contenido)
	}
	if te.Truncado {
		t.Fatal("un txt chico no debió venir truncado")
	}
}

// --- No-office → 422 ---

func TestTextoNoOfficeDa422(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	rec := decodificarRecurso(t, subirMultipart(t, s, c.ID, "apuntes.pdf", pdfMinimo))

	w := pedirTexto(t, s, rec.Archivo.ID)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, quería 422: %s", w.Code, w.Body.String())
	}
	var cuerpo map[string]string
	json.Unmarshal(w.Body.Bytes(), &cuerpo)
	if !strings.Contains(cuerpo["error"], "vista de texto") {
		t.Fatalf("el error debió estar en español y explicar el rechazo: %+v", cuerpo)
	}
}

// --- zip bomb-ish: parte grande sin comprimir, debe truncarse sin
// cargar todo en memoria ---

// TestTextoDocxParteGrandeSeTrunca cubre la defensa contra "zip bomb":
// un word/document.xml que, sin comprimir, pesa más que
// maxBytesPorParte (20 MiB) pero comprime a casi nada (texto repetido) —
// el ZIP que viaja en la subida es chico, pero el contenido real que
// intentaría leerse es grande. Confirma que la respuesta vuelve
// truncada, con MENOS bloques que el techo por cantidad (1000): si el
// corte fuera por cantidad de bloques y no por bytes, este test no
// probaría nada distinto del límite de bloques.
func TestTextoDocxParteGrandeSeTrunca(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")

	// ~24 MiB de XML sin comprimir (800 párrafos de 30.000 'x' cada uno),
	// muy por encima de maxBytesPorParte (20 MiB) pero con < 1000 bloques
	// si se leyera entero — así que el truncado que importa acá es el de
	// BYTES, no el de cantidad.
	var cuerpo strings.Builder
	cuerpo.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	cuerpo.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	parrafoGrande := "<w:p><w:r><w:t>" + strings.Repeat("x", 30000) + "</w:t></w:r></w:p>"
	for i := 0; i < 800; i++ {
		cuerpo.WriteString(parrafoGrande)
	}
	cuerpo.WriteString(`</w:body></w:document>`)

	zipBytes := construirZip(t, map[string]string{"word/document.xml": cuerpo.String()})
	rec := decodificarRecurso(t, subirMultipart(t, s, c.ID, "gigante.docx", string(zipBytes)))
	if rec.Archivo == nil {
		t.Fatalf("la subida del docx gigante falló: %+v", rec)
	}

	w := pedirTexto(t, s, rec.Archivo.ID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, quería 200 (parcial, truncado): %s", w.Code, w.Body.String())
	}
	te := decodificarTexto(t, w)
	if !te.Truncado {
		t.Fatal("un documento que supera maxBytesPorParte debió volver truncado: true")
	}
	if len(te.Bloques) == 0 {
		t.Fatal("debió devolver los bloques que llegó a leer antes del corte, no una lista vacía")
	}
	if len(te.Bloques) >= 1000 {
		t.Fatalf("bloques = %d: el corte debió ser por BYTES (antes del techo de 1000 bloques), no por cantidad", len(te.Bloques))
	}
}
