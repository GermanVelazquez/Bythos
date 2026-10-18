package archivos

// texto.go — Vista de texto simple de documentos de Office (docx/xlsx/
// pptx) y texto plano (txt/md), para el visor de la UI en vez de solo
// "Descargar" (ver api/archivos.go, ruta /texto, y Archivos.jsx,
// DocPreview). NO es un conversor fiel: es una lectura best-effort del
// XML interno de cada formato (Office moderno = ZIP de XML, ver
// detectarZipOffice en archivos.go) que alcanza para leer el contenido
// sin abrir Word/Excel/PowerPoint.
//
// Misma defensa que el resto del paquete contra archivos hostiles
// ("zip bombs" y XML gigante armado a propósito): cada parte del ZIP se
// lee a través de io.LimitReader (maxBytesPorParte) ANTES de dársela al
// decoder XML, y encoding/xml.Decoder nunca resuelve entidades externas
// (no hay "billion laughs" ni XXE posible con este decoder — es una
// garantía del paquete estándar, no algo que este archivo deba
// configurar). Además se acotan la cantidad de bloques/diapositivas/
// filas y el total de caracteres: al llegar a cualquiera de esos techos,
// se corta la lectura y se devuelve Truncado: true en vez de seguir
// consumiendo memoria.

import (
	"archive/zip"
	"encoding/xml"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// --- Límites compartidos ---

// maxBytesPorParte es cuánto lee ExtraerTexto de CADA entrada del ZIP
// (document.xml, cada slideN.xml, sheet1.xml, sharedStrings.xml) antes
// de darla por leída: un documento de Office real, aunque tenga cientos
// de páginas de texto, no pasa los 20 MiB de XML sin comprimir para una
// sola parte. Esto es lo que corta un "zip bomb" (una entrada que
// descomprime a gigabytes): io.LimitReader nunca lee más de esto, sin
// importar cuánto diga el ZIP que mide la entrada real.
const maxBytesPorParte = 20 << 20 // 20 MiB

// maxBloques acota bloques de texto (docx) y diapositivas (pptx).
const maxBloques = 1000

// maxDiapositivas acota cuántas diapositivas de un pptx se procesan.
const maxDiapositivas = 200

// maxCaracteresTexto acota el total de texto extraído (docx/pptx/txt):
// más que esto y la vista de texto ya no sirve como preview, solo carga
// memoria de más.
const maxCaracteresTexto = 300_000

// maxFilasXLSX / maxColsXLSX: "primeras 200 filas × 30 columnas" pedido
// en el enunciado — un preview de hoja de cálculo, no un lector completo.
const (
	maxFilasXLSX = 200
	maxColsXLSX  = 30
)

// maxSharedStrings acota cuántas entradas de xl/sharedStrings.xml se
// resuelven: una hoja de cálculo real no tiene más de unas pocas miles
// de strings únicas.
const maxSharedStrings = 20_000

// ErrTextoNoDisponible es lo que ve el handler HTTP (que lo traduce a
// 422) cuando el mime no es soportado, el ZIP/XML no tiene la forma
// esperada, o no se pudo extraer NADA de texto.
var ErrTextoNoDisponible = errNoDisponible("no se pudo generar una vista de texto para este archivo")

type errNoDisponible string

func (e errNoDisponible) Error() string { return string(e) }

// --- Tipos de salida (JSON, ver api/archivos.go) ---
//
// A diferencia del resto de la API Go (structs sin `json:` tags, claves
// Capitalizadas — ver comentario de archivoInfoJSON), estos tags van en
// minúscula A PROPÓSITO: es el contrato pedido para esta vista
// (Archivos.jsx/DocPreview lee exactamente estas claves).

// BloqueTexto es un párrafo/título/ítem de lista de un docx, en el orden
// en que aparecen en el documento.
type BloqueTexto struct {
	Tipo  string `json:"tipo"` // "titulo" | "parrafo" | "lista"
	Nivel int    `json:"nivel,omitempty"`
	Texto string `json:"texto"`
}

// Diapositiva es una slide de un pptx con sus párrafos de texto, en
// orden de lectura dentro de la diapositiva.
type Diapositiva struct {
	Numero   int      `json:"numero"`
	Parrafos []string `json:"parrafos"`
}

// TextoExtraido es la respuesta de ExtraerTexto / GET /texto. Solo uno
// de Bloques/Diapositivas/Filas/Texto viene poblado, según Tipo.
type TextoExtraido struct {
	Tipo         string        `json:"tipo"` // "docx" | "pptx" | "xlsx" | "texto"
	Bloques      []BloqueTexto `json:"bloques,omitempty"`
	Diapositivas []Diapositiva `json:"diapositivas,omitempty"`
	Filas        [][]string    `json:"filas,omitempty"`
	Texto        string        `json:"texto,omitempty"`
	Truncado     bool          `json:"truncado"`
}

// mimeDocx/mimePptx/mimeXlsx: mismos strings que TipoRecurso/
// detectarZipOffice en archivos.go — la fuente de verdad de esos mimes
// es esa función, acá solo los repetimos para el switch de ExtraerTexto.
const (
	mimeDocx = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	mimePptx = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	mimeXlsx = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)

// ExtraerTexto arma la vista de texto de un archivo ya guardado, según
// su mime detectado (ver detectarTipo en archivos.go). Cualquier mime
// fuera de esta lista (pdf, imagen, video) devuelve ErrTextoNoDisponible
// de entrada: esos tipos ya tienen su propio visor (iframe/video/img),
// no necesitan vista de texto.
func ExtraerTexto(rutaRelativa, mime string) (TextoExtraido, error) {
	switch mime {
	case mimeDocx:
		return extraerDocx(rutaRelativa)
	case mimePptx:
		return extraerPptx(rutaRelativa)
	case mimeXlsx:
		return extraerXlsx(rutaRelativa)
	case "text/plain":
		return extraerTextoPlano(rutaRelativa)
	default:
		return TextoExtraido{}, ErrTextoNoDisponible
	}
}

func abrirZip(rutaRelativa string) (*zip.ReadCloser, error) {
	return zip.OpenReader(Absoluta(rutaRelativa))
}

func buscarEntry(zr *zip.ReadCloser, nombre string) *zip.File {
	for _, f := range zr.File {
		if f.Name == nombre {
			return f
		}
	}
	return nil
}

func atributo(t xml.StartElement, local string) string {
	for _, a := range t.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// --- DOCX ---

func extraerDocx(rutaRelativa string) (TextoExtraido, error) {
	zr, err := abrirZip(rutaRelativa)
	if err != nil {
		return TextoExtraido{}, ErrTextoNoDisponible
	}
	defer zr.Close()

	f := buscarEntry(zr, "word/document.xml")
	if f == nil {
		return TextoExtraido{}, ErrTextoNoDisponible
	}
	rc, err := f.Open()
	if err != nil {
		return TextoExtraido{}, ErrTextoNoDisponible
	}
	defer rc.Close()

	bloques, truncado, err := parsearDocumentoDocx(io.LimitReader(rc, maxBytesPorParte))
	if err != nil && len(bloques) == 0 {
		return TextoExtraido{}, ErrTextoNoDisponible
	}
	return TextoExtraido{Tipo: "docx", Bloques: bloques, Truncado: truncado}, nil
}

// nivelDeEstilo traduce el styleId de w:pStyle a un nivel de encabezado
// (1-9), o 0 si el estilo no es un encabezado. Cubre los nombres que
// Word usa según el idioma de instalación: "Heading1".."Heading9" (EN),
// "Titulo1"/"Título1" (ES, con o sin tilde, que a veces se pierde en el
// styleId interno) y "Title"/el título del documento (nivel 1).
func nivelDeEstilo(estilo string) int {
	e := strings.ToLower(strings.TrimSpace(estilo))
	if e == "" {
		return 0
	}
	switch {
	case strings.HasPrefix(e, "heading"):
		return nivelNumerico(e, "heading")
	case strings.HasPrefix(e, "título"):
		return nivelNumerico(e, "título")
	case strings.HasPrefix(e, "titulo"):
		return nivelNumerico(e, "titulo")
	case e == "title":
		return 1
	}
	return 0
}

func nivelNumerico(estilo, prefijo string) int {
	resto := strings.TrimSpace(strings.TrimPrefix(estilo, prefijo))
	if resto == "" {
		return 1
	}
	if n, err := strconv.Atoi(resto); err == nil && n > 0 {
		return n
	}
	return 1
}

// parsearDocumentoDocx recorre word/document.xml con un decoder XML
// streaming (nunca carga el árbol completo en memoria) y arma los
// bloques en orden de aparición. w:p = párrafo, w:pStyle/@w:val = estilo
// (para distinguir título de párrafo normal), w:numPr presente en w:pPr
// = ítem de lista, w:t = texto visible, w:tab/w:br = tab/salto de línea
// DENTRO del párrafo (se preservan como caracteres literales).
func parsearDocumentoDocx(r io.Reader) ([]BloqueTexto, bool, error) {
	dec := xml.NewDecoder(r)

	var bloques []BloqueTexto
	var truncado bool
	var enParrafo, dentroT bool
	var estilo string
	var esLista bool
	var texto strings.Builder
	var totalChars int

	cerrarParrafo := func() {
		contenido := strings.TrimSpace(texto.String())
		texto.Reset()
		if contenido == "" {
			return
		}
		tipo := "parrafo"
		nivel := 0
		if esLista {
			tipo = "lista"
		} else if n := nivelDeEstilo(estilo); n > 0 {
			tipo = "titulo"
			nivel = n
		}
		bloques = append(bloques, BloqueTexto{Tipo: tipo, Nivel: nivel, Texto: contenido})
		totalChars += len(contenido)
	}

	for {
		if len(bloques) >= maxBloques || totalChars >= maxCaracteresTexto {
			truncado = true
			break
		}
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Corte a mitad del XML (tocó el techo de maxBytesPorParte, o
			// el archivo viene mal formado): si ya sacamos algo, se
			// devuelve truncado en vez de tirar todo el resultado.
			if len(bloques) > 0 {
				truncado = true
				break
			}
			return nil, false, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				enParrafo = true
				estilo = ""
				esLista = false
				texto.Reset()
			case "pStyle":
				estilo = atributo(t, "val")
			case "numPr":
				esLista = true
			case "t":
				dentroT = true
			case "tab":
				if enParrafo {
					texto.WriteByte('\t')
				}
			case "br":
				if enParrafo {
					texto.WriteByte('\n')
				}
			}
		case xml.CharData:
			if enParrafo && dentroT {
				texto.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				dentroT = false
			case "p":
				if enParrafo {
					cerrarParrafo()
					enParrafo = false
				}
			}
		}
	}
	return bloques, truncado, nil
}

// --- PPTX ---

// reSlide matcha exactamente "ppt/slides/slideN.xml" (no
// slideLayouts/slideMasters, que viven en otras carpetas, ni los
// _rels/slideN.xml.rels de cada slide, que terminan en ".rels" y no en
// ".xml").
var reSlide = regexp.MustCompile(`^ppt/slides/slide([0-9]+)\.xml$`)

func extraerPptx(rutaRelativa string) (TextoExtraido, error) {
	zr, err := abrirZip(rutaRelativa)
	if err != nil {
		return TextoExtraido{}, ErrTextoNoDisponible
	}
	defer zr.Close()

	type entradaSlide struct {
		numero int
		file   *zip.File
	}
	var entradas []entradaSlide
	for _, f := range zr.File {
		m := reSlide.FindStringSubmatch(f.Name)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		entradas = append(entradas, entradaSlide{numero: n, file: f})
	}
	if len(entradas) == 0 {
		return TextoExtraido{}, ErrTextoNoDisponible
	}
	sort.Slice(entradas, func(i, j int) bool { return entradas[i].numero < entradas[j].numero })

	var diapositivas []Diapositiva
	var truncado bool
	for _, e := range entradas {
		if len(diapositivas) >= maxDiapositivas {
			truncado = true
			break
		}
		rc, err := e.file.Open()
		if err != nil {
			continue
		}
		parrafos, perr := parsearDiapositivaPptx(io.LimitReader(rc, maxBytesPorParte))
		rc.Close()
		if perr != nil && len(parrafos) == 0 {
			continue
		}
		diapositivas = append(diapositivas, Diapositiva{Numero: e.numero, Parrafos: parrafos})
	}
	if len(diapositivas) == 0 {
		return TextoExtraido{}, ErrTextoNoDisponible
	}
	return TextoExtraido{Tipo: "pptx", Diapositivas: diapositivas, Truncado: truncado}, nil
}

// parsearDiapositivaPptx lee un slideN.xml y junta el texto de cada
// párrafo (a:p) concatenando sus runs (a:t) — una diapositiva puede
// tener varios "a:t" seguidos dentro del mismo "a:p" por formato mixto
// (negrita en una parte, normal en otra), y acá los tratamos como un
// solo párrafo de texto plano.
func parsearDiapositivaPptx(r io.Reader) ([]string, error) {
	dec := xml.NewDecoder(r)
	var parrafos []string
	var dentroT bool
	var actual strings.Builder
	var totalChars int

	for {
		if len(parrafos) >= maxBloques || totalChars >= maxCaracteresTexto {
			break
		}
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			if len(parrafos) > 0 {
				break
			}
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				actual.Reset()
			case "t":
				dentroT = true
			}
		case xml.CharData:
			if dentroT {
				actual.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				dentroT = false
			case "p":
				contenido := strings.TrimSpace(actual.String())
				actual.Reset()
				if contenido != "" {
					parrafos = append(parrafos, contenido)
					totalChars += len(contenido)
				}
			}
		}
	}
	return parrafos, nil
}

// --- XLSX ---

func extraerXlsx(rutaRelativa string) (TextoExtraido, error) {
	zr, err := abrirZip(rutaRelativa)
	if err != nil {
		return TextoExtraido{}, ErrTextoNoDisponible
	}
	defer zr.Close()

	var compartidas []string
	if f := buscarEntry(zr, "xl/sharedStrings.xml"); f != nil {
		if rc, err := f.Open(); err == nil {
			compartidas, _ = parsearSharedStrings(io.LimitReader(rc, maxBytesPorParte))
			rc.Close()
		}
	}

	// "Primera hoja": xl/worksheets/sheet1.xml es la convención estándar
	// de OOXML para la primera hoja definida en el libro. Simplificación
	// consciente y documentada: no resolvemos el mapeo completo vía
	// xl/workbook.xml + xl/_rels/workbook.xml.rels (que sería necesario
	// solo si alguien reordenó las pestañas a mano de forma que el
	// nombre de archivo interno ya no coincide con el orden visible) —
	// el enunciado pide "primera hoja" como preview, no un lector fiel
	// completo, y Descargar sigue disponible como respaldo exacto.
	f := buscarEntry(zr, "xl/worksheets/sheet1.xml")
	if f == nil {
		return TextoExtraido{}, ErrTextoNoDisponible
	}
	rc, err := f.Open()
	if err != nil {
		return TextoExtraido{}, ErrTextoNoDisponible
	}
	defer rc.Close()

	filas, truncado, err := parsearHojaXLSX(io.LimitReader(rc, maxBytesPorParte), compartidas)
	if err != nil && len(filas) == 0 {
		return TextoExtraido{}, ErrTextoNoDisponible
	}
	return TextoExtraido{Tipo: "xlsx", Filas: filas, Truncado: truncado}, nil
}

// parsearSharedStrings lee xl/sharedStrings.xml: cada <si> es una string
// única referenciada por índice desde las celdas tipo "s" de la hoja.
// Un <si> puede tener texto directo (<si><t>...</t></si>) o varios runs
// con formato mixto (<si><r><t>...</t></r><r><t>...</t></r></si>) — en
// ambos casos juntamos todo el texto de los <t> que caen dentro del
// mismo <si>.
func parsearSharedStrings(r io.Reader) ([]string, error) {
	dec := xml.NewDecoder(r)
	var out []string
	var dentroSI, dentroT bool
	var actual strings.Builder

	for len(out) < maxSharedStrings {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break // best-effort: lo que se pudo resolver hasta el corte
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "si":
				dentroSI = true
				actual.Reset()
			case "t":
				dentroT = true
			}
		case xml.CharData:
			if dentroSI && dentroT {
				actual.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				dentroT = false
			case "si":
				dentroSI = false
				out = append(out, actual.String())
			}
		}
	}
	return out, nil
}

// parsearHojaXLSX lee un sheetN.xml (<sheetData><row><c><v>...) y arma
// filas de celdas como texto, resolviendo "shared strings" (celdas
// t="s", donde <v> es un índice a compartidas) y usando el valor de <v>
// tal cual para todo lo demás (número, booleano, fórmula con resultado
// string). Respeta la posición real de columna del atributo r="C5" de
// cada celda (una hoja real suele venir con huecos: celdas vacías no
// tienen <c>), acotado a maxColsXLSX.
func parsearHojaXLSX(r io.Reader, compartidas []string) ([][]string, bool, error) {
	dec := xml.NewDecoder(r)

	var filas [][]string
	var truncado bool
	var enFila bool
	var filaActual []celdaXLSX
	var colActual int
	var tipoActual string
	var dentroV bool
	var valorV strings.Builder

	flushFila := func() {
		if len(filas) >= maxFilasXLSX {
			truncado = true
			return
		}
		maxCol := 0
		for _, c := range filaActual {
			if c.col > maxCol {
				maxCol = c.col
			}
		}
		if maxCol > maxColsXLSX-1 {
			maxCol = maxColsXLSX - 1
			truncado = true
		}
		fila := make([]string, maxCol+1)
		for _, c := range filaActual {
			if c.col <= maxCol {
				fila[c.col] = c.val
			} else {
				truncado = true
			}
		}
		filas = append(filas, fila)
		filaActual = nil
	}

	for {
		if len(filas) >= maxFilasXLSX {
			truncado = true
			break
		}
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			if len(filas) > 0 {
				truncado = true
				break
			}
			return nil, false, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "row":
				enFila = true
				filaActual = nil
			case "c":
				tipoActual = atributo(t, "t")
				colActual = colDeReferencia(atributo(t, "r"))
			case "v":
				dentroV = true
				valorV.Reset()
			}
		case xml.CharData:
			if dentroV {
				valorV.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v":
				dentroV = false
				valor := valorV.String()
				if tipoActual == "s" {
					if idx, err := strconv.Atoi(valor); err == nil && idx >= 0 && idx < len(compartidas) {
						valor = compartidas[idx]
					}
				}
				filaActual = append(filaActual, celdaXLSX{col: colActual, val: valor})
			case "row":
				if enFila {
					flushFila()
					enFila = false
				}
			}
		}
	}
	return filas, truncado, nil
}

type celdaXLSX struct {
	col int
	val string
}

// colDeReferencia traduce la parte de letras de una referencia de celda
// ("C5" -> "C", "AA12" -> "AA") a un índice de columna 0-based (C -> 2,
// AA -> 26). Ignora cualquier caracter que no sea letra A-Z (mayúscula o
// minúscula, aunque en un xlsx real siempre viene en mayúscula).
func colDeReferencia(ref string) int {
	col := 0
	for _, r := range ref {
		switch {
		case r >= 'A' && r <= 'Z':
			col = col*26 + int(r-'A'+1)
		case r >= 'a' && r <= 'z':
			col = col*26 + int(r-'a'+1)
		default:
			if col > 0 {
				return col - 1
			}
			return 0
		}
	}
	if col == 0 {
		return 0
	}
	return col - 1
}

// --- Texto plano (txt/md) ---

func extraerTextoPlano(rutaRelativa string) (TextoExtraido, error) {
	f, err := Abrir(rutaRelativa)
	if err != nil {
		return TextoExtraido{}, ErrTextoNoDisponible
	}
	defer f.Close()

	datos, err := io.ReadAll(io.LimitReader(f, int64(maxCaracteresTexto)+1))
	if err != nil {
		return TextoExtraido{}, ErrTextoNoDisponible
	}

	truncado := false
	if len(datos) > maxCaracteresTexto {
		datos = datos[:maxCaracteresTexto]
		// Recortar al límite de una rune UTF-8 válida: LimitReader corta a
		// mitad de byte sin avisar, y un multi-byte partido rompería
		// json.Marshal (o al menos ensuciaría el último caracter mostrado).
		for len(datos) > 0 && !utf8.Valid(datos) {
			datos = datos[:len(datos)-1]
		}
		truncado = true
	}
	return TextoExtraido{Tipo: "texto", Texto: string(datos), Truncado: truncado}, nil
}
