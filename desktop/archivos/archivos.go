// Package archivos — el ALMACÉN de los archivos que sube el usuario (PDF,
// video, imagen, documento). "Paso 0" del roadmap: Bythos guarda el
// archivo entero, no solo un link a él (ver desktop/db/archivos.go para
// los metadatos y api/archivos.go para las rutas HTTP).
//
// Regla de oro de este paquete: SOLO toca el disco. Nunca abre bythos.db
// ni sabe de recursos/carpetas (eso es db/ y api/). Misma separación
// senior que terminal/ y workspace/: un paquete, una responsabilidad.
//
// Direccionado por contenido (content-addressed): cada archivo vive en
// Base()/<sha256[:2]>/<sha256>.<ext>. El nombre en disco NUNCA sale del
// nombre que mandó el cliente — eso cierra cualquier path traversal
// ("../../windows/system32/...") de raíz, porque el nombre del cliente ni
// siquiera se USA para construir una ruta, solo se guarda como texto
// (NombreOriginal, sanitizado y acotado) para mostrarlo en la UI.
//
// Dedupe automático: si el hash ya existe en disco, el temporal se borra
// y se reusa el blob de siempre. Subir el mismo PDF desde 2 carpetas no
// duplica el archivo en disco.
package archivos

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"bythos-desktop/db"
)

// TamanoMaximo es el techo de subida: 2 GiB por default. Es var (no
// const) para que los tests lo bajen a un valor chico y puedan disparar
// el 413 sin tener que generar gigabytes de verdad; en producción nadie
// la toca. El handler HTTP (ver api/archivos.go) la aplica con
// http.MaxBytesReader ANTES de llamar a Guardar, así un cliente hostil no
// puede llenar el disco del usuario ni tirar el proceso mandando
// gigabytes sin límite.
var TamanoMaximo int64 = 2 << 30 // 2 GiB

// ErrTipoNoPermitido es el error que ve el handler cuando el contenido no
// matchea ningún formato de la lista blanca (ver detectarTipo). El
// mensaje ya viene en español, listo para responderError.
var ErrTipoNoPermitido = errors.New("tipo de archivo no permitido. Bythos acepta PDF, video (mp4/webm/mkv/mov), imagen (png/jpg/gif/webp), documentos de Office (docx/xlsx/pptx) y texto plano (txt/md)")

// baseDir empieza vacío A PROPÓSITO: nunca se calcula en la
// inicialización del paquete (un `var baseDir = filepath.Join(db.CarpetaDatos(), ...)`
// llamaría a os.UserConfigDir()+MkdirAll ni bien alguien IMPORTA este
// paquete — incluido un test que ni siquiera sube un archivo — y crearía
// la carpeta %APPDATA%\Bythos real en la máquina de quien corre `go test`.
// Base() la calcula perezosamente, solo la primera vez que alguien
// realmente la necesita; UsarBase (tests) la fija ANTES de esa primera
// llamada, así el default nunca llega a tocar disco en un test.
var baseDir string

// Base devuelve la carpeta raíz de almacenamiento (calculada la primera
// vez que se pide, ver comentario de baseDir). api/ la usa para armar la
// ruta absoluta que expone a la UI y al agente MCP (ver Absoluta).
func Base() string {
	if baseDir == "" {
		baseDir = filepath.Join(db.CarpetaDatos(), "archivos")
	}
	return baseDir
}

// UsarBase cambia la carpeta raíz. Solo para tests: nunca se llama desde
// código de producción (main.go no la toca, usa el default perezoso de
// arriba).
func UsarBase(dir string) { baseDir = dir }

// Absoluta arma la ruta absoluta en disco de un archivo a partir de su
// ruta relativa guardada en db.Archivo.Ruta (ej. "ab/ab12...ef.pdf").
func Absoluta(rutaRelativa string) string {
	return filepath.Join(Base(), filepath.FromSlash(rutaRelativa))
}

// Guardado es lo que Guardar devuelve: todo lo que api/ necesita para
// llamar db.CrearArchivo (nombre en disco, hash, mime detectado, tamaño)
// más el nombre original ya sanitizado para mostrar en la UI.
type Guardado struct {
	SHA256         string
	Mime           string
	Tamano         int64
	RutaRelativa   string // "<sha[:2]>/<sha>.<ext>", con / (ToSlash)
	NombreOriginal string // sanitizado y acotado, SOLO para mostrar
	Reusado        bool   // true si el hash ya existía (dedupe: no se escribió nada nuevo)
}

// Guardar consume r (el multipart.Part del archivo subido, ya envuelto
// por el caller en http.MaxBytesReader para el techo de tamaño) y lo
// escribe SIN pasar por memoria completa: io.Copy a un temporal dentro de
// esta misma carpeta base (para que el rename final sea atómico, mismo
// volumen) mientras calcula el sha256 al vuelo. Solo al terminar de
// escribir sabe el hash, así que el temporal nace con nombre random y se
// renombra (o se descarta, si dedupe) al final.
//
// nombreCliente es el nombre de archivo tal como lo mandó el navegador
// (Content-Disposition del part); se usa SOLO para: (1) decidir txt vs md
// cuando el contenido es texto plano indistinguible por bytes, y (2)
// como NombreOriginal de display. NUNCA se usa para construir una ruta.
func Guardar(r io.Reader, nombreCliente string) (Guardado, error) {
	base := Base()
	if err := os.MkdirAll(base, 0755); err != nil {
		return Guardado{}, fmt.Errorf("no se pudo preparar el almacén de archivos: %w", err)
	}

	tmp, err := os.CreateTemp(base, "tmp-subida-*")
	if err != nil {
		return Guardado{}, fmt.Errorf("no se pudo crear el archivo temporal: %w", err)
	}
	tmpPath := tmp.Name()
	// Si algo falla antes del rename final, no dejamos basura en el
	// almacén: el defer solo borra si tmpPath sigue existiendo (el
	// rename exitoso lo mueve, así que el segundo Remove da IsNotExist y
	// lo ignoramos sin problema). huboExito SOLO lo pone en true la rama
	// que efectivamente consume tmpPath (el rename); la rama de dedupe
	// borra el temporal ella misma más abajo, así que si algo raro pasa y
	// tmpPath sigue vivo, este defer es la red de seguridad.
	huboExito := false
	defer func() {
		if huboExito {
			return
		}
		if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
			// Best-effort: estamos en un defer, ya no hay a quién
			// devolverle este error, pero tampoco lo perdemos en
			// silencio (mismo criterio que logAvisoArchivo en api/).
			fmt.Fprintln(os.Stderr, "archivos: no se pudo limpiar el temporal", tmpPath, "-", err)
		}
	}()

	hasher := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, hasher), r)
	cerrarErr := tmp.Close()
	if err != nil {
		return Guardado{}, fmt.Errorf("no se pudo guardar el archivo: %w", err)
	}
	if cerrarErr != nil {
		return Guardado{}, fmt.Errorf("no se pudo cerrar el archivo temporal: %w", cerrarErr)
	}
	if n == 0 {
		return Guardado{}, errors.New("el archivo está vacío")
	}

	tipo, err := detectarTipo(tmpPath, nombreCliente)
	if err != nil {
		return Guardado{}, err
	}

	shaHex := hex.EncodeToString(hasher.Sum(nil))
	subcarpeta := shaHex[:2]
	nombreFinal := shaHex + "." + tipo.ext
	rutaRelativa := subcarpeta + "/" + nombreFinal
	destinoDir := filepath.Join(base, subcarpeta)
	if err := os.MkdirAll(destinoDir, 0755); err != nil {
		return Guardado{}, fmt.Errorf("no se pudo preparar el almacén de archivos: %w", err)
	}
	destino := filepath.Join(destinoDir, nombreFinal)

	reusado := false
	if _, err := os.Stat(destino); err == nil {
		// Dedupe: mismo contenido ya vive en disco. Acá es donde antes se
		// marcaba huboExito=true sin haber usado tmpPath para nada — el
		// defer nunca corría y el temporal completo (mismo tamaño que el
		// archivo subido) quedaba huérfano en el almacén para siempre en
		// CADA subida duplicada. Por eso esta rama borra el temporal ELLA
		// MISMA (tmp ya está cerrado desde el Close() de arriba, así que
		// en Windows el Remove no choca con "archivo en uso") y NUNCA
		// pone huboExito en true: si este Remove fallara, el defer de
		// arriba reintenta como red de seguridad.
		if err := os.Remove(tmpPath); err != nil {
			return Guardado{}, fmt.Errorf("no se pudo limpiar el temporal tras el dedupe: %w", err)
		}
		reusado = true
	} else {
		if err := os.Rename(tmpPath, destino); err != nil {
			return Guardado{}, fmt.Errorf("no se pudo guardar el archivo: %w", err)
		}
		// Solo esta rama consume tmpPath (lo mueve a destino): es la
		// ÚNICA que debe desactivar el cleanup del defer.
		huboExito = true
	}

	return Guardado{
		SHA256:         shaHex,
		Mime:           tipo.mime,
		Tamano:         n,
		RutaRelativa:   rutaRelativa,
		NombreOriginal: sanearNombre(nombreCliente),
		Reusado:        reusado,
	}, nil
}

// Abrir devuelve un *os.File listo para http.ServeContent (necesita
// io.ReadSeeker). El caller es dueño de cerrarlo.
func Abrir(rutaRelativa string) (*os.File, error) {
	return os.Open(Absoluta(rutaRelativa))
}

// Eliminar borra el blob de disco. Idempotente: si ya no está, no es
// error (el caller puede llamar esto después de una carrera rara sin
// tener que chequear existencia antes).
func Eliminar(rutaRelativa string) error {
	if rutaRelativa == "" {
		return nil
	}
	err := os.Remove(Absoluta(rutaRelativa))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// nombreLargoMaximo acota NombreOriginal: 150 runas alcanza de sobra para
// cualquier nombre de archivo real, y evita que un nombre gigantesco (a
// propósito o no) infle una fila o una tarjeta de la UI.
const nombreLargoMaximo = 150

// sanearNombre limpia el nombre que mandó el cliente para guardarlo SOLO
// como texto de display (nunca como parte de una ruta en disco, ver
// comentario del paquete). filepath.Base tira cualquier "carpeta/" o
// "../" que traiga; luego se recortan caracteres de control y se acota el
// largo.
func sanearNombre(nombreCliente string) string {
	nombre := filepath.Base(strings.TrimSpace(nombreCliente))
	if nombre == "." || nombre == string(filepath.Separator) || nombre == "" {
		return "archivo"
	}
	var b strings.Builder
	for _, r := range nombre {
		if r < 0x20 || r == 0x7f {
			continue // caracteres de control fuera, incluido NUL
		}
		b.WriteRune(r)
	}
	limpio := strings.TrimSpace(b.String())
	if limpio == "" {
		return "archivo"
	}
	runas := []rune(limpio)
	if len(runas) > nombreLargoMaximo {
		runas = runas[:nombreLargoMaximo]
	}
	return string(runas)
}

// --- Detección de tipo por contenido (magic bytes), lista blanca ---
//
// ¿Por qué por contenido y no por extensión/Content-Type del cliente?
// Porque ambos los controla quien sube el archivo: un .pdf falso que en
// realidad es un .html con <script> pasaría cualquier chequeo de
// extensión. La lista blanca mira los bytes reales; todo lo que no
// matchea se rechaza, INCLUIDO HTML y SVG a propósito (ver
// parecemarkup): si Bythos los sirviera con Content-Type correcto,
// correrían script en el origen de la app (ver api/archivos.go, la ruta
// /contenido).

type tipoDetectado struct {
	mime string
	ext  string
}

// cabeceraMax es cuánto lee detectarTipo del arranque del archivo para
// los magic bytes de PDF/imagen/video. 4096 sobra para todos los formatos
// de esta lista (el más exigente es MP4, que necesita los primeros ~12
// bytes del ftyp box; EBML necesita bastante menos).
const cabeceraMax = 4096

func detectarTipo(path, nombreCliente string) (tipoDetectado, error) {
	f, err := os.Open(path)
	if err != nil {
		return tipoDetectado{}, fmt.Errorf("no se pudo leer el archivo para detectar su tipo: %w", err)
	}
	defer f.Close()

	cabecera := make([]byte, cabeceraMax)
	n, _ := io.ReadFull(f, cabecera)
	cabecera = cabecera[:n]

	switch {
	case bytes.HasPrefix(cabecera, []byte("%PDF-")):
		return tipoDetectado{"application/pdf", "pdf"}, nil
	case esPNG(cabecera):
		return tipoDetectado{"image/png", "png"}, nil
	case esJPEG(cabecera):
		return tipoDetectado{"image/jpeg", "jpg"}, nil
	case esGIF(cabecera):
		return tipoDetectado{"image/gif", "gif"}, nil
	case esWEBP(cabecera):
		return tipoDetectado{"image/webp", "webp"}, nil
	case esFtypBox(cabecera):
		return tipoDeFtyp(cabecera), nil
	case esEBML(cabecera):
		return tipoDeEBML(cabecera), nil
	case bytes.HasPrefix(cabecera, []byte("PK\x03\x04")):
		return detectarZipOffice(path)
	}

	if tipo, ok := detectarTexto(path, nombreCliente); ok {
		return tipo, nil
	}
	return tipoDetectado{}, ErrTipoNoPermitido
}

func esPNG(b []byte) bool {
	firma := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	return bytes.HasPrefix(b, firma)
}

func esJPEG(b []byte) bool {
	return len(b) >= 3 && b[0] == 0xff && b[1] == 0xd8 && b[2] == 0xff
}

func esGIF(b []byte) bool {
	return bytes.HasPrefix(b, []byte("GIF87a")) || bytes.HasPrefix(b, []byte("GIF89a"))
}

func esWEBP(b []byte) bool {
	return len(b) >= 12 && bytes.Equal(b[0:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP"))
}

// esFtypBox detecta la familia MP4/M4V/MOV: caja "ftyp" en el offset 4
// (formato ISO base media, ver ISO/IEC 14496-12).
func esFtypBox(b []byte) bool {
	return len(b) >= 12 && bytes.Equal(b[4:8], []byte("ftyp"))
}

func tipoDeFtyp(b []byte) tipoDetectado {
	brand := string(b[8:12])
	if brand == "qt  " {
		return tipoDetectado{"video/quicktime", "mov"}
	}
	return tipoDetectado{"video/mp4", "mp4"}
}

// esEBML detecta el contenedor de WebM/Matroska: magic number 0x1A45DFA3.
func esEBML(b []byte) bool {
	firma := []byte{0x1a, 0x45, 0xdf, 0xa3}
	return bytes.HasPrefix(b, firma)
}

// tipoDeEBML distingue WebM de Matroska buscando el DocType en la
// cabecera ya leída (string ASCII "webm" o "matroska" cerca del
// arranque). Si no se puede distinguir, cae a Matroska por ser el
// contenedor más general.
func tipoDeEBML(b []byte) tipoDetectado {
	if bytes.Contains(b, []byte("webm")) {
		return tipoDetectado{"video/webm", "webm"}
	}
	return tipoDetectado{"video/x-matroska", "mkv"}
}

// detectarZipOffice abre el ZIP completo (ya escrito en disco) y decide
// si es un documento de Office moderno mirando sus entradas: todo
// .docx/.xlsx/.pptx es un ZIP con "[Content_Types].xml" en la raíz y una
// carpeta principal (word/, xl/ o ppt/) que lo identifica. Un ZIP
// cualquiera que no tenga esa forma se rechaza (la lista blanca no
// incluye archivos comprimidos genéricos). Además rechaza la variante CON
// macros (.docm/.xlsm/.pptm, ver zipEsMacroHabilitado): esas también son
// ZIPs con exactamente la misma forma word/xl/ppt, así que sin este
// chequeo pasarían disfrazadas de docx/xlsx/pptx — y Bythos las serviría
// con ese Content-Type "seguro" cuando en realidad llevan VBA.
func detectarZipOffice(path string) (tipoDetectado, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return tipoDetectado{}, ErrTipoNoPermitido
	}
	defer zr.Close()

	tieneContentTypes := false
	var archivoContentTypes *zip.File
	var carpetaPrincipal string
	for _, f := range zr.File {
		nombre := f.Name
		if nombre == "[Content_Types].xml" {
			tieneContentTypes = true
			archivoContentTypes = f
		}
		// vbaProject.bin es el binario compilado de las macros VBA. Si
		// está en CUALQUIER carpeta del paquete hay macros, sin importar
		// lo que diga (o no diga) [Content_Types].xml.
		if strings.EqualFold(filepath.Base(nombre), "vbaProject.bin") {
			return tipoDetectado{}, ErrTipoNoPermitido
		}
		switch {
		case strings.HasPrefix(nombre, "word/"):
			carpetaPrincipal = "word"
		case strings.HasPrefix(nombre, "xl/"):
			carpetaPrincipal = "xl"
		case strings.HasPrefix(nombre, "ppt/"):
			carpetaPrincipal = "ppt"
		}
	}
	if !tieneContentTypes || carpetaPrincipal == "" {
		return tipoDetectado{}, ErrTipoNoPermitido
	}
	macroHabilitado, err := zipEsMacroHabilitado(archivoContentTypes)
	if err != nil || macroHabilitado {
		// Un [Content_Types].xml ilegible es tan sospechoso como uno que
		// SÍ declara macros: mismo criterio "todo lo que no matchea
		// limpio se rechaza" que el resto de este archivo.
		return tipoDetectado{}, ErrTipoNoPermitido
	}
	switch carpetaPrincipal {
	case "word":
		return tipoDetectado{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "docx"}, nil
	case "xl":
		return tipoDetectado{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "xlsx"}, nil
	default:
		return tipoDetectado{"application/vnd.openxmlformats-officedocument.presentationml.presentation", "pptx"}, nil
	}
}

// limiteContentTypes acota cuánto lee zipEsMacroHabilitado de
// "[Content_Types].xml": ese XML lista los tipos de contenido del
// paquete y para un documento real no pasa de unos pocos KB. 64 KiB sobra
// de sobra y evita cargar en memoria un XML gigante armado a propósito.
const limiteContentTypes = 64 << 10 // 64 KiB

// contentTypesMacroHabilitado son los content types que Office usa en
// [Content_Types].xml para declarar el documento PRINCIPAL de un archivo
// con macros (.docm/.xlsm/.pptm) — ver ECMA-376 parte 2. Encontrar
// cualquiera de estos es la señal inequívoca de "esto tiene macros",
// mismo ZIP y mismas carpetas word/xl/ppt que su equivalente sin macros.
var contentTypesMacroHabilitado = []string{
	"vnd.ms-word.document.macroEnabled.main+xml",
	"vnd.ms-excel.sheet.macroEnabled.main+xml",
	"vnd.ms-powerpoint.presentation.macroEnabled.main+xml",
}

// zipEsMacroHabilitado lee (acotado) el entry [Content_Types].xml ya
// localizado por detectarZipOffice y busca alguno de los content types de
// contentTypesMacroHabilitado.
func zipEsMacroHabilitado(f *zip.File) (bool, error) {
	rc, err := f.Open()
	if err != nil {
		return false, err
	}
	defer rc.Close()
	datos, err := io.ReadAll(io.LimitReader(rc, limiteContentTypes))
	if err != nil {
		return false, err
	}
	for _, ct := range contentTypesMacroHabilitado {
		if bytes.Contains(datos, []byte(ct)) {
			return true, nil
		}
	}
	return false, nil
}

// limiteValidacionTexto acota cuánto lee detectarTexto para validar
// UTF-8/NUL: 20 MiB alcanza de sobra para cualquier .txt/.md real. Con
// archivos de texto más grandes que eso, el chequeo solo mira el
// arranque (compromiso documentado: prioriza no cargar el archivo entero
// en memoria sobre validar cada byte de un texto gigante, que no es el
// caso de uso esperado para notas o documentación).
const limiteValidacionTexto = 20 << 20 // 20 MiB

// detectarTexto acepta SOLO texto plano de verdad: UTF-8 válido, sin
// bytes NUL (binarios), y que NO empiece pareciendo markup (HTML/XML/SVG)
// una vez descontado espacio en blanco y BOM. Este último chequeo es a
// propósito: un .html o un .svg son UTF-8 válido sin NUL, así que sin
// esta regla pasarían como "texto plano" — y Bythos los serviría con
// Content-Type text/html o image/svg+xml más adelante, corriendo script
// en el origen de la app. Ver ErrTipoNoPermitido.
func detectarTexto(path, nombreCliente string) (tipoDetectado, bool) {
	f, err := os.Open(path)
	if err != nil {
		return tipoDetectado{}, false
	}
	defer f.Close()

	datos, err := io.ReadAll(io.LimitReader(f, limiteValidacionTexto))
	if err != nil {
		return tipoDetectado{}, false
	}
	if bytes.IndexByte(datos, 0) != -1 {
		return tipoDetectado{}, false
	}
	if !utf8.Valid(datos) {
		return tipoDetectado{}, false
	}
	if pareceMarkup(datos) {
		return tipoDetectado{}, false
	}

	ext := "txt"
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(nombreCliente)), ".md") {
		ext = "md"
	}
	return tipoDetectado{"text/plain", ext}, true
}

// pareceMarkup mira el primer caracter no-blanco (después de tirar BOM y
// espacios): si es '<', asumimos HTML/XML/SVG y lo rechazamos. Es una
// regla simple a propósito — sobra para bloquear <!DOCTYPE html>, <html>,
// <svg> y cualquier XML, que es exactamente lo que hay que bloquear.
func pareceMarkup(datos []byte) bool {
	recortado := bytes.TrimPrefix(datos, []byte{0xef, 0xbb, 0xbf}) // BOM UTF-8
	recortado = bytes.TrimLeft(recortado, " \t\r\n")
	return len(recortado) > 0 && recortado[0] == '<'
}

// TipoRecurso traduce un mime detectado al content_type de 4 valores que
// usa resources.tipo para archivos (ver db.TiposArchivoValidos):
// pdf | video | imagen | documento. api/ la usa justo después de Guardar.
func TipoRecurso(mime string) string {
	switch {
	case mime == "application/pdf":
		return "pdf"
	case strings.HasPrefix(mime, "video/"):
		return "video"
	case strings.HasPrefix(mime, "image/"):
		return "imagen"
	default:
		return "documento" // office (docx/xlsx/pptx) y texto (txt/md)
	}
}
