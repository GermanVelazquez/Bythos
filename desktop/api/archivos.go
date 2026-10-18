package api

// archivos.go — Paso 0 del roadmap: Bythos guarda ARCHIVOS enteros (PDF,
// video, imagen, documento) como recursos, no solo links a ellos. Base
// para una futura app móvil: los bytes viven en disco (ver
// desktop/archivos), los metadatos en SQLite (ver db/archivos.go), y esta
// ruta HTTP es la única puerta para subirlos y servirlos — igual que el
// resto de la API, detrás de conGuardia (ver server.go).
//
// POST /api/archivos: multipart, streamea el part "archivo" directo a
// disco vía r.MultipartReader() (nunca vuelca el archivo entero a
// memoria) y crea un recurso en la carpeta indicada.
// GET /api/archivos/{id}/contenido: sirve el blob con Range (para que un
// <video> pueda hacer seek) y headers que evitan que el navegador lo
// ejecute como HTML/script.

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"bythos-desktop/archivos"
	"bythos-desktop/db"
)

// logAvisoArchivo es el único punto de log de este archivo: separado
// para que quede claro que ningún error acá vuelve al cliente HTTP (el
// borrado del recurso, que es lo que el usuario pidió, ya sucedió).
func logAvisoArchivo(mensaje string, err error) {
	log.Println(mensaje, err)
}

// margenMultipart es lo que se le suma a archivos.TamanoMaximo para el
// techo TOTAL del body multipart: boundaries, headers de cada part y el
// campo carpeta_id son unos pocos bytes, 4KB sobra de sobra. Chico a
// propósito: así un test puede bajar archivos.TamanoMaximo a unos pocos
// bytes y seguir dando 413 con un payload de prueba chico (si el margen
// fuera 1MB, un archivo de prueba de unos KB nunca lo superaría).
const margenMultipart = 4 << 10 // 4 KiB

// subirArchivo atiende POST /api/archivos.
func (s *Servidor) subirArchivo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, archivos.TamanoMaximo+margenMultipart)

	mr, err := r.MultipartReader()
	if err != nil {
		responderError(w, 400, "Petición inválida: se esperaba multipart/form-data")
		return
	}

	var carpetaID int64
	var guardado *archivos.Guardado

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			if esBodyDemasiadoGrande(err) {
				responderError(w, http.StatusRequestEntityTooLarge, mensajeArchivoDemasiadoGrande())
				return
			}
			responderError(w, 400, "Petición multipart inválida")
			return
		}

		switch part.FormName() {
		case "carpeta_id":
			// Campo de texto chico: 32 bytes sobran de sobra para un int64.
			datos, _ := io.ReadAll(io.LimitReader(part, 32))
			carpetaID, _ = strconv.ParseInt(strings.TrimSpace(string(datos)), 10, 64)
		case "archivo":
			g, err := archivos.Guardar(part, part.FileName())
			if err != nil {
				if esBodyDemasiadoGrande(err) {
					responderError(w, http.StatusRequestEntityTooLarge, mensajeArchivoDemasiadoGrande())
					return
				}
				if errors.Is(err, archivos.ErrTipoNoPermitido) {
					responderError(w, http.StatusUnsupportedMediaType, err.Error())
					return
				}
				responderError(w, 500, "No se pudo guardar el archivo")
				return
			}
			guardado = &g
		}
		part.Close()
	}

	if carpetaID == 0 {
		responderError(w, 400, "Falta carpeta_id")
		return
	}
	if guardado == nil {
		responderError(w, 400, "Falta el archivo (campo \"archivo\")")
		return
	}

	a, err := db.CrearArchivo(s.Base, guardado.SHA256, guardado.NombreOriginal, guardado.Mime, guardado.Tamano, guardado.RutaRelativa)
	if err != nil {
		responderError(w, 500, "No se pudo registrar el archivo")
		return
	}

	rec, err := db.GuardarArchivo(s.Base, carpetaID, a.ID, tituloDeArchivo(guardado.NombreOriginal), archivos.TipoRecurso(guardado.Mime))
	if err != nil {
		responderError(w, 400, "No se pudo crear el recurso (¿la carpeta existe?)")
		return
	}

	s.registrarEvento(r, db.AccionArchivoSubido, rec.ID, tituloOUrl(rec))
	w.WriteHeader(http.StatusCreated)
	responder(w, recursoConArchivo(s.Base, rec))
}

func mensajeArchivoDemasiadoGrande() string {
	return fmt.Sprintf("El archivo es demasiado grande (máximo %d MB)", archivos.TamanoMaximo/(1<<20))
}

// tituloDeArchivo es el título del recurso al subir un archivo: el
// nombre original sin su extensión ("Apuntes de Go.pdf" → "Apuntes de
// Go"). Si no queda nada legible, se usa el nombre tal cual.
func tituloDeArchivo(nombreOriginal string) string {
	sinExt := nombreOriginal
	if i := strings.LastIndex(nombreOriginal, "."); i > 0 {
		sinExt = nombreOriginal[:i]
	}
	sinExt = strings.TrimSpace(sinExt)
	if sinExt == "" {
		return nombreOriginal
	}
	return sinExt
}

// esOrigenCruzado es el mismo chequeo que usan /contenido, /miniatura y
// /texto: conGuardia exige Origin solo cuando el navegador lo manda, y un
// <img>/<video src="http://localhost:8080/...">  incrustado desde una
// página ajena NO manda Origin en esa carga (no es fetch/XHR) — así que
// ese guardia solo no alcanza acá, y los IDs de archivo son secuenciales
// (fáciles de adivinar). Sec-Fetch-Site sí lo manda TODO navegador
// moderno en cada request, incluida esa carga de <img>, y ahí SÍ dice
// "cross-site" sin depender de Origin. Clientes sin este header (curl, el
// proceso MCP) no mandan Sec-Fetch-Site y siguen funcionando igual que
// siempre; solo cortamos cuando el navegador ya nos avisó explícitamente
// que es un embed de otro origen.
func esOrigenCruzado(r *http.Request) bool {
	valor := r.Header.Get("Sec-Fetch-Site")
	return valor != "" && valor != "same-origin" && valor != "none"
}

// contenidoArchivo atiende GET /api/archivos/{id}/contenido.
func (s *Servidor) contenidoArchivo(w http.ResponseWriter, r *http.Request) {
	if esOrigenCruzado(r) {
		responderError(w, http.StatusForbidden, "Origen no permitido")
		return
	}

	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	a, ok, err := db.ObtenerArchivo(s.Base, id)
	if err != nil {
		responderError(w, 500, "No se pudo leer el archivo")
		return
	}
	if !ok {
		responderError(w, 404, "Archivo no encontrado")
		return
	}

	f, err := archivos.Abrir(a.Ruta)
	if err != nil {
		responderError(w, 404, "El archivo no está en el disco")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		responderError(w, 500, "No se pudo leer el archivo")
		return
	}

	w.Header().Set("Content-Type", a.Mime)
	// nosniff: el navegador nunca debe "adivinar" un tipo distinto al
	// que Bythos ya detectó por contenido (ver desktop/archivos).
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// conGuardia (server.go) ya puso X-Frame-Options: DENY + CSP
	// "frame-ancestors 'none'" en TODA respuesta, pero esta ruta es la
	// única excepción real: el visor de PDF propio de la UI la embebe en
	// un <iframe> same-origin (ver Archivos.jsx), así que DENY/'none' la
	// rompería. frame-ancestors 'self' + SAMEORIGIN es el equivalente
	// correcto: deja enmarcar desde el propio origen de Bythos y sigue
	// bloqueando cualquier otro (frame-ancestors ya cubre esto en todo
	// navegador moderno; SAMEORIGIN queda de respaldo consistente para el
	// resto).
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	if a.Mime == "application/pdf" {
		// PDF: la CSP NO lleva "sandbox". El visor nativo de PDF de
		// Chromium (el que abre este <iframe>) es un plugin/MimeHandler
		// interno, no HTML del origen de Bythos — un CSP con "sandbox"
		// se lo come entero y el <iframe> queda en blanco. Confirmado por
		// dos bugs de Chromium: "Sandbox breaks PDF rendering"
		// (issues.chromium.org/issues/41131921) y "Chrome does not
		// display PDF content if Content Security Policy (CSP) is in
		// effect" (issues.chromium.org/issues/40328564). No hace falta
		// sandbox para estar seguros igual: un PDF con este Content-Type
		// no corre script del origen de Bythos en ese visor, y nosniff
		// (arriba) ya impide que el navegador lo reinterprete como otra
		// cosa.
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'self'")
	} else {
		// Todo lo demás (video/imagen/documento/texto) sí soporta
		// sandbox sin romper el render: no dependen de un visor-plugin
		// del navegador, son <video>/<img> o descarga directa.
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; frame-ancestors 'self'")
	}
	w.Header().Set("Content-Disposition", `inline; filename*=UTF-8''`+url.PathEscape(nombreParaDisposition(a.NombreOriginal)))

	// http.ServeContent maneja Range (206) solo, ETag/Last-Modified y el
	// resto de la negociación HTTP — no hay que reinventar nada de eso.
	http.ServeContent(w, r, a.NombreOriginal, info.ModTime(), f)
}

// miniaturaArchivo atiende GET /api/archivos/{id}/miniatura: un JPEG
// chico (máx. 320px en el lado largo, ver desktop/archivos/miniaturas.go)
// para el preview de la tarjeta en la grilla de recursos. 404 para
// CUALQUIER caso en que no corresponda mostrar miniatura (tipo sin
// soporte como webp/video/pdf/documento, imagen demasiado grande,
// decodificación fallida): la UI no necesita distinguir el motivo, cae
// sola al ícono de tipo (ver ArchivoPreview en Archivos.jsx).
func (s *Servidor) miniaturaArchivo(w http.ResponseWriter, r *http.Request) {
	if esOrigenCruzado(r) {
		responderError(w, http.StatusForbidden, "Origen no permitido")
		return
	}

	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	a, ok, err := db.ObtenerArchivo(s.Base, id)
	if err != nil {
		responderError(w, 500, "No se pudo leer el archivo")
		return
	}
	if !ok {
		responderError(w, 404, "Archivo no encontrado")
		return
	}

	ruta, err := archivos.Miniatura(a.SHA256, a.Ruta, a.Mime)
	if err != nil {
		responderError(w, 404, "No hay miniatura disponible para este archivo")
		return
	}
	f, err := os.Open(ruta)
	if err != nil {
		responderError(w, 404, "No hay miniatura disponible para este archivo")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		responderError(w, 500, "No se pudo leer la miniatura")
		return
	}

	// Mismos headers de sandboxing que /contenido (nunca es PDF, así que
	// siempre lleva "sandbox" — ver el comentario de contenidoArchivo
	// sobre por qué PDF es la única excepción).
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; frame-ancestors 'self'")
	http.ServeContent(w, r, "miniatura.jpg", info.ModTime(), f)
}

// textoArchivo atiende GET /api/archivos/{id}/texto: una vista de texto
// simple de un documento de Office (docx/xlsx/pptx) o texto plano
// (txt/md), para que el visor de la UI muestre contenido en vez de solo
// "Descargar" (ver desktop/archivos/texto.go y DocPreview en
// Archivos.jsx). 422 si el mime no es soportado o no se pudo extraer
// nada — la UI cae al botón Descargar de siempre.
func (s *Servidor) textoArchivo(w http.ResponseWriter, r *http.Request) {
	if esOrigenCruzado(r) {
		responderError(w, http.StatusForbidden, "Origen no permitido")
		return
	}

	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	a, ok, err := db.ObtenerArchivo(s.Base, id)
	if err != nil {
		responderError(w, 500, "No se pudo leer el archivo")
		return
	}
	if !ok {
		responderError(w, 404, "Archivo no encontrado")
		return
	}

	texto, err := archivos.ExtraerTexto(a.Ruta, a.Mime)
	if err != nil {
		responderError(w, http.StatusUnprocessableEntity, "Bythos no pudo generar una vista de texto de este documento")
		return
	}
	responder(w, texto)
}

// nombreParaDisposition evita que un nombre original vacío (edge case:
// archivo subido sin nombre) deje un Content-Disposition roto.
func nombreParaDisposition(nombre string) string {
	if strings.TrimSpace(nombre) == "" {
		return "archivo"
	}
	return nombre
}

// --- JSON enriquecido: recursos + su archivo (si lo tienen) ---
//
// db.Recurso no lleva `json:` tags (ver comentario en resources.go): sus
// claves viajan Capitalizadas. archivoInfoJSON sigue esa misma
// convención para que la UI (normRecurso en App.jsx) y el MCP
// (agentes/tipos.go) lean todo con el mismo criterio de siempre.

type archivoInfoJSON struct {
	ID             int64  `json:"ID"`
	NombreOriginal string `json:"NombreOriginal"`
	Mime           string `json:"Mime"`
	Tamano         int64  `json:"Tamano"`
	// RutaLocal es la ruta ABSOLUTA en el disco de esta PC. La UI no la
	// usa (pide el contenido por HTTP, ver /contenido arriba), pero el
	// agente MCP sí: corre en la misma PC y puede abrir el archivo
	// directo con sus propias herramientas (ver docs/AGENTES.md).
	RutaLocal string `json:"RutaLocal"`
}

type recursoJSON struct {
	db.Recurso
	Archivo *archivoInfoJSON `json:"Archivo"`
}

// recursoConArchivo enriquece un Recurso con los datos de su archivo (si
// ArchivoID != 0). Para <100 recursos por carpeta esta consulta extra por
// fila es instantánea (mismo criterio que nombreCarpetaPorID en server.go
// y el loop de exportarCarpeta); si Bythos alguna vez maneja miles, ahí sí
// se justifica un JOIN en db/.
func recursoConArchivo(base *sql.DB, r db.Recurso) recursoJSON {
	out := recursoJSON{Recurso: r}
	if r.ArchivoID == 0 {
		return out
	}
	a, ok, err := db.ObtenerArchivo(base, r.ArchivoID)
	if err != nil || !ok {
		return out
	}
	out.Archivo = &archivoInfoJSON{
		ID:             a.ID,
		NombreOriginal: a.NombreOriginal,
		Mime:           a.Mime,
		Tamano:         a.Tamano,
		RutaLocal:      archivos.Absoluta(a.Ruta),
	}
	return out
}

func recursosConArchivo(base *sql.DB, rs []db.Recurso) []recursoJSON {
	out := make([]recursoJSON, 0, len(rs))
	for _, r := range rs {
		out = append(out, recursoConArchivo(base, r))
	}
	return out
}

// --- Limpieza de archivos huérfanos ---

// limpiarArchivoSiHuerfano borra el blob de disco + su fila de metadatos
// SOLO si, después de borrar el/los recurso(s) que lo referenciaban,
// ningún otro recurso sigue apuntando a este archivo (dos recursos
// pueden compartir un mismo archivo_id por el dedupe de CrearArchivo).
// La llaman borrarRecurso y borrarCarpeta (el cascade de SQL borra las
// filas de resources solo, nunca el archivo). Un fallo acá NUNCA rompe
// el borrado que ya sucedió: solo queda log, mismo criterio que
// registrarEvento.
func (s *Servidor) limpiarArchivoSiHuerfano(archivoID int64) {
	n, err := db.ContarRecursosPorArchivo(s.Base, archivoID)
	if err != nil {
		logAvisoArchivo("No se pudo contar referencias del archivo:", err)
		return
	}
	if n > 0 {
		return
	}
	a, ok, err := db.ObtenerArchivo(s.Base, archivoID)
	if err != nil || !ok {
		return
	}
	if err := archivos.Eliminar(a.Ruta); err != nil {
		logAvisoArchivo("No se pudo borrar el blob del archivo:", err)
	}
	if err := archivos.EliminarMiniatura(a.SHA256); err != nil {
		logAvisoArchivo("No se pudo borrar la miniatura cacheada del archivo:", err)
	}
	if err := db.BorrarArchivo(s.Base, archivoID); err != nil {
		logAvisoArchivo("No se pudo borrar los metadatos del archivo:", err)
	}
}

// archivosReferenciadosEnCarpeta lista, ANTES de borrar la carpeta (el
// cascade de SQL se lleva sus recursos), los archivo_id únicos que hay
// que revisar con limpiarArchivoSiHuerfano después.
func archivosReferenciadosEnCarpeta(s *Servidor, carpetaID int64) []int64 {
	recursos, err := db.ListarRecursos(s.Base, carpetaID)
	if err != nil {
		return nil
	}
	vistos := map[int64]bool{}
	var out []int64
	for _, r := range recursos {
		if r.ArchivoID != 0 && !vistos[r.ArchivoID] {
			vistos[r.ArchivoID] = true
			out = append(out, r.ArchivoID)
		}
	}
	return out
}
