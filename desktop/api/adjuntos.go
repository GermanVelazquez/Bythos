package api

// adjuntos.go — Archivos que el usuario adjunta a la pizarra de una sesión de
// Terminal. El blob se guarda una sola vez (archivos.Guardar, dedupe por
// sha256) y cada sesión lo ve a través de su propio vínculo (db/adjuntos.go):
// todas las rutas resuelven el adjunto por (sesión, id), así que el de otra
// sesión es un 404 igual que uno inexistente.
//
// POST  .../sesiones/{id}/adjuntos                    (Ventana)
// GET   .../sesiones/{id}/adjuntos                    (Ventana-lectura)
// GET   .../sesiones/{id}/adjuntos/{adj}/contenido    (Ventana-lectura)
// GET   .../sesiones/{id}/adjuntos/{adj}/texto        (Ventana-lectura)
// POST  .../sesiones/{id}/adjuntos/{adj}/guardar      (Ventana)
// POST  .../sesiones/{id}/adjuntos/desde-recurso      (Ventana)
// GET   .../adjuntos/{adj}/imagen|texto             (Agente: solo lo que está en la pizarra guardada)

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"bythos-desktop/archivos"
	"bythos-desktop/db"
	"bythos-desktop/pizarra"
)

// limiteAdjunto es el techo de un adjunto de pizarra: 25 MiB. Es var para que
// los tests lo bajen y disparen el 413 sin generar megabytes (mismo criterio
// que archivos.TamanoMaximo).
var limiteAdjunto int64 = 25 << 20

func (s *Servidor) rutasAdjuntos(mux *http.ServeMux) {
	const p = "/api/terminal/sesiones/{id}/adjuntos"
	mux.HandleFunc("POST "+p, conVentana(s.subirAdjunto))
	mux.HandleFunc("GET "+p, conVentanaLectura(s.listarAdjuntos))
	mux.HandleFunc("GET "+p+"/{adj}/contenido", conVentanaLectura(s.contenidoAdjunto))
	mux.HandleFunc("GET "+p+"/{adj}/texto", conVentanaLectura(s.textoAdjunto))
	mux.HandleFunc("POST "+p+"/desde-recurso", conVentana(s.adjuntarDesdeRecurso))
	mux.HandleFunc("POST "+p+"/{adj}/guardar", conVentana(s.guardarAdjuntoEnCarpeta))
}

// lectorAcotado falla con *http.MaxBytesError apenas el contenido pasa del
// límite: MaxBytesReader acota el body entero (con margen para el multipart),
// pero el archivo en sí debe cortar exactamente en limiteAdjunto.
type lectorAcotado struct {
	r      io.Reader
	resta  int64
	limite int64
}

func (l *lectorAcotado) Read(p []byte) (int, error) {
	if l.resta <= 0 {
		var extra [1]byte
		if n, _ := io.ReadFull(l.r, extra[:]); n > 0 {
			return 0, &http.MaxBytesError{Limit: l.limite}
		}
		return 0, io.EOF
	}
	if int64(len(p)) > l.resta {
		p = p[:l.resta]
	}
	n, err := l.r.Read(p)
	l.resta -= int64(n)
	return n, err
}

func mensajeAdjuntoDemasiadoGrande() string {
	return fmt.Sprintf("El archivo es demasiado grande (máximo %d MiB)", limiteAdjunto>>20)
}

// sesionDeRuta lee {id} y comprueba que la sesión exista; ok false ya
// respondió el 404.
func (s *Servidor) sesionDeRuta(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, ok := idSesion(w, r)
	if !ok {
		return 0, false
	}
	_, existe, err := db.ObtenerSesion(s.Base, id)
	if err != nil {
		responderError(w, 500, "No se pudo leer la sesión")
		return 0, false
	}
	if !existe {
		noExiste(w, id)
		return 0, false
	}
	return id, true
}

// adjuntoDeRuta resuelve {id}/{adj} y devuelve el archivo del vínculo; ok
// false ya respondió (404 si el adjunto no es de esta sesión).
func (s *Servidor) adjuntoDeRuta(w http.ResponseWriter, r *http.Request) (db.Archivo, bool) {
	sesionID, ok := s.sesionDeRuta(w, r)
	if !ok {
		return db.Archivo{}, false
	}
	raw := r.PathValue("adj")
	adjID, _ := strconv.ParseInt(raw, 10, 64)
	adj, existe, err := db.ObtenerAdjunto(s.Base, sesionID, adjID)
	if err != nil {
		responderError(w, 500, "No se pudo leer el adjunto")
		return db.Archivo{}, false
	}
	if !existe {
		responderError(w, http.StatusNotFound, "El adjunto "+raw+" no existe")
		return db.Archivo{}, false
	}
	// La FK garantiza que el archivo existe mientras el vínculo exista.
	a, _, err := db.ObtenerArchivo(s.Base, adj.ArchivoID)
	if err != nil {
		responderError(w, 500, "No se pudo leer el adjunto")
		return db.Archivo{}, false
	}
	return a, true
}

// descartarBlob borra el blob recién escrito, salvo que ya existiera antes
// (Reusado): ese pertenece a otros vínculos o recursos.
func descartarBlob(g archivos.Guardado) {
	if !g.Reusado {
		if err := archivos.Eliminar(g.RutaRelativa); err != nil {
			logAvisoArchivo("No se pudo borrar el blob descartado:", err)
		}
	}
}

// subirAdjunto atiende POST .../sesiones/{id}/adjuntos.
func (s *Servidor) subirAdjunto(w http.ResponseWriter, r *http.Request) {
	sesionID, ok := s.sesionDeRuta(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, limiteAdjunto+margenMultipart)
	mr, err := r.MultipartReader()
	if err != nil {
		responderError(w, 400, "Petición inválida: se esperaba multipart/form-data")
		return
	}

	var guardado *archivos.Guardado
	for guardado == nil {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			if esBodyDemasiadoGrande(err) {
				responderError(w, http.StatusRequestEntityTooLarge, mensajeAdjuntoDemasiadoGrande())
				return
			}
			responderError(w, 400, "Petición multipart inválida")
			return
		}
		if part.FormName() == "archivo" {
			g, err := archivos.Guardar(&lectorAcotado{r: part, resta: limiteAdjunto, limite: limiteAdjunto}, part.FileName())
			if err != nil {
				switch {
				case esBodyDemasiadoGrande(err):
					responderError(w, http.StatusRequestEntityTooLarge, mensajeAdjuntoDemasiadoGrande())
				case errors.Is(err, archivos.ErrTipoNoPermitido):
					responderError(w, http.StatusUnsupportedMediaType, err.Error())
				default:
					responderError(w, 500, "No se pudo guardar el archivo")
				}
				return
			}
			guardado = &g
		}
		part.Close()
	}
	if guardado == nil {
		responderError(w, 400, "Falta el archivo (campo \"archivo\")")
		return
	}

	// El video entra en la lista blanca de la biblioteca, no en la pizarra.
	if archivos.TipoRecurso(guardado.Mime) == "video" {
		descartarBlob(*guardado)
		responderError(w, http.StatusUnsupportedMediaType, "Los videos no se pueden adjuntar a la pizarra")
		return
	}

	a, err := db.CrearArchivo(s.Base, guardado.SHA256, guardado.NombreOriginal, guardado.Mime, guardado.Tamano, guardado.RutaRelativa)
	if err != nil {
		descartarBlob(*guardado)
		responderError(w, 500, "No se pudo registrar el archivo")
		return
	}
	adj, err := db.CrearAdjunto(s.Base, sesionID, a.ID, guardado.NombreOriginal)
	if err != nil {
		descartarBlob(*guardado)
		responderError(w, 500, "No se pudo adjuntar el archivo")
		return
	}
	responderCreado(w, fichaAdjunto(adj))
}

// listarAdjuntos atiende GET .../sesiones/{id}/adjuntos.
func (s *Servidor) listarAdjuntos(w http.ResponseWriter, r *http.Request) {
	sesionID, ok := s.sesionDeRuta(w, r)
	if !ok {
		return
	}
	lista, err := db.ListarAdjuntos(s.Base, sesionID)
	if err != nil {
		responderError(w, 500, "No se pudieron leer los adjuntos")
		return
	}
	responder(w, lista)
}

// contenidoAdjunto atiende GET .../adjuntos/{adj}/contenido.
func (s *Servidor) contenidoAdjunto(w http.ResponseWriter, r *http.Request) {
	if a, ok := s.adjuntoDeRuta(w, r); ok {
		servirArchivo(w, r, a)
	}
}

// textoAdjunto atiende GET .../adjuntos/{adj}/texto (misma vista de texto
// que /api/archivos/{id}/texto).
func (s *Servidor) textoAdjunto(w http.ResponseWriter, r *http.Request) {
	a, ok := s.adjuntoDeRuta(w, r)
	if !ok {
		return
	}
	texto, err := archivos.ExtraerTexto(a.Ruta, a.Mime)
	if err != nil {
		responderError(w, http.StatusUnprocessableEntity, "Bythos no pudo generar una vista de texto de este documento")
		return
	}
	responder(w, texto)
}

// --- Rutas del agente (guarda Agente): leen la escena GUARDADA, sin latido ---

func (s *Servidor) rutasAdjuntosAgente(mux *http.ServeMux) {
	const p = "/api/terminal/adjuntos/{adj}/"
	mux.HandleFunc("GET "+p+"imagen", conAgente(s.imagenAdjuntoAgente))
	mux.HandleFunc("GET "+p+"texto", conAgente(s.textoAdjuntoAgente))
}

// adjuntoAgente es el adjunto ya resuelto y comprobado contra la pizarra.
type adjuntoAgente struct {
	adj        db.Adjunto
	ruta       string // relativa al almacén
	sesion     db.Sesion
	resolucion string
}

// ficha son los datos comunes que el agente recibe de cualquier adjunto.
func (a adjuntoAgente) ficha() map[string]any {
	return map[string]any{
		"adjunto": a.adj.ID, "nombre": a.adj.Nombre, "tipo": archivos.TipoRecurso(a.adj.Mime),
		"mime": a.adj.Mime, "tamano": a.adj.Tamano, "ruta": archivos.Absoluta(a.ruta),
		"sesion": a.sesion.ID, "resolucion": a.resolucion,
	}
}

// adjuntoParaAgente resuelve la sesión (argumento > entorno > activa > reciente)
// y el adjunto. Solo se entrega si un elemento vivo de la escena GUARDADA lo
// referencia (design D5); si no, 404. ok false ya respondió.
func (s *Servidor) adjuntoParaAgente(w http.ResponseWriter, r *http.Request) (adjuntoAgente, bool) {
	q := r.URL.Query()
	se, resolucion, ok := s.resolverSesion(w, q.Get("sesion_id"), q.Get("pista"))
	if !ok {
		return adjuntoAgente{}, false
	}
	raw := r.PathValue("adj")
	adjID, _ := strconv.ParseInt(raw, 10, 64)
	adj, existe, err := db.ObtenerAdjunto(s.Base, se.ID, adjID)
	if err != nil {
		responderError(w, 500, "No se pudo leer el adjunto")
		return adjuntoAgente{}, false
	}
	if !existe {
		responderError(w, http.StatusNotFound, "El adjunto "+raw+" no existe")
		return adjuntoAgente{}, false
	}
	escena, _, _, err := db.ObtenerPizarra(s.Base, se.ID)
	if err != nil {
		responderError(w, 500, "No se pudo leer la pizarra")
		return adjuntoAgente{}, false
	}
	refs, err := pizarra.Referencias([]byte(escena))
	if err != nil {
		responderError(w, 500, "La pizarra guardada está dañada")
		return adjuntoAgente{}, false
	}
	if len(refs[adj.SHA256]) == 0 {
		responderError(w, http.StatusNotFound, "El adjunto "+raw+" no está en la pizarra")
		return adjuntoAgente{}, false
	}
	a, _, err := db.ObtenerArchivo(s.Base, adj.ArchivoID)
	if err != nil {
		responderError(w, 500, "No se pudo leer el adjunto")
		return adjuntoAgente{}, false
	}
	return adjuntoAgente{adj: adj, ruta: a.Ruta, sesion: se, resolucion: resolucion}, true
}

// imagenAdjuntoAgente atiende GET adjuntos/{adj}/imagen: bytes de la imagen a
// 1280px como máximo, con los datos en cabeceras X-Bythos-*.
func (s *Servidor) imagenAdjuntoAgente(w http.ResponseWriter, r *http.Request) {
	a, ok := s.adjuntoParaAgente(w, r)
	if !ok {
		return
	}
	if archivos.TipoRecurso(a.adj.Mime) != "imagen" {
		responderError(w, http.StatusBadRequest, "El adjunto "+r.PathValue("adj")+" no es una imagen: usa leer_archivo")
		return
	}
	img, err := archivos.ImagenParaAgente(a.ruta, a.adj.Mime)
	if err != nil {
		responderError(w, 500, "No se pudo leer la imagen")
		return
	}
	if img.Datos == nil {
		ficha := a.ficha()
		ficha["aviso"] = img.Aviso
		responder(w, ficha)
		return
	}
	h := w.Header()
	h.Set("Content-Type", img.Mime)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Bythos-Adjunto", strconv.FormatInt(a.adj.ID, 10))
	h.Set("X-Bythos-Nombre", url.QueryEscape(a.adj.Nombre))
	h.Set("X-Bythos-Tipo", archivos.TipoRecurso(a.adj.Mime))
	h.Set("X-Bythos-Tamano", strconv.FormatInt(a.adj.Tamano, 10))
	h.Set("X-Bythos-Ruta", url.QueryEscape(archivos.Absoluta(a.ruta)))
	h.Set("X-Bythos-Sesion", strconv.FormatInt(a.sesion.ID, 10))
	h.Set("X-Bythos-Resolucion", a.resolucion)
	w.Write(img.Datos)
}

// textoAdjuntoAgente atiende GET adjuntos/{adj}/texto: texto plano con tope; el
// PDF no tiene extractor y devuelve solo metadatos y ruta.
func (s *Servidor) textoAdjuntoAgente(w http.ResponseWriter, r *http.Request) {
	a, ok := s.adjuntoParaAgente(w, r)
	if !ok {
		return
	}
	ficha := a.ficha()
	switch archivos.TipoRecurso(a.adj.Mime) {
	case "imagen":
		responderError(w, http.StatusBadRequest, "El adjunto "+r.PathValue("adj")+" es una imagen: usa leer_imagen")
		return
	case "pdf":
		ficha["aviso"] = "Bythos no extrae el texto de los PDF: ábrelo desde la ruta."
		responder(w, ficha)
		return
	}
	extraido, err := archivos.ExtraerTexto(a.ruta, a.adj.Mime)
	if err != nil {
		responderError(w, http.StatusUnprocessableEntity, "Bythos no pudo extraer el texto de este documento")
		return
	}
	plano := extraido.Plano()
	ficha["texto"], ficha["truncado"], ficha["caracteres_totales"] = plano.Texto, plano.Truncado, plano.CaracteresTotales
	responder(w, ficha)
}

// fichaAdjunto es la respuesta de subir o adjuntar: lo que la UI necesita
// para pintar la tarjeta.
func fichaAdjunto(adj db.Adjunto) map[string]any {
	return map[string]any{
		"id": adj.ID, "sha256": adj.SHA256, "nombre": adj.Nombre,
		"mime": adj.Mime, "tipo": archivos.TipoRecurso(adj.Mime),
	}
}

// carpetaDeSesion devuelve la carpeta vinculada a la sesión; ok false ya
// respondió 409 (sin carpeta) o 500.
func (s *Servidor) carpetaDeSesion(w http.ResponseWriter, sesionID int64) (int64, bool) {
	se, _, err := db.ObtenerSesion(s.Base, sesionID)
	if err != nil {
		responderError(w, 500, "No se pudo leer la sesión")
		return 0, false
	}
	if se.CarpetaID == nil {
		responderError(w, http.StatusConflict, "La sesión no tiene carpeta")
		return 0, false
	}
	return *se.CarpetaID, true
}

// guardarAdjuntoEnCarpeta atiende POST .../adjuntos/{adj}/guardar: crea en la
// carpeta de la sesión un recurso que apunta al mismo archivo (sin copiar
// bytes). Si la carpeta ya lo tiene, 200 con el recurso existente.
func (s *Servidor) guardarAdjuntoEnCarpeta(w http.ResponseWriter, r *http.Request) {
	sesionID, ok := s.sesionDeRuta(w, r)
	if !ok {
		return
	}
	raw := r.PathValue("adj")
	adjID, _ := strconv.ParseInt(raw, 10, 64)
	adj, existe, err := db.ObtenerAdjunto(s.Base, sesionID, adjID)
	if err != nil {
		responderError(w, 500, "No se pudo leer el adjunto")
		return
	}
	if !existe {
		responderError(w, http.StatusNotFound, "El adjunto "+raw+" no existe")
		return
	}
	carpetaID, ok := s.carpetaDeSesion(w, sesionID)
	if !ok {
		return
	}
	previo, hay, err := db.ObtenerRecursoPorCarpetaYArchivo(s.Base, carpetaID, adj.ArchivoID)
	if err != nil {
		responderError(w, 500, "No se pudo leer la carpeta")
		return
	}
	if hay {
		responder(w, recursoConArchivo(s.Base, previo))
		return
	}
	rec, err := db.GuardarArchivo(s.Base, carpetaID, adj.ArchivoID, tituloDeArchivo(adj.Nombre), archivos.TipoRecurso(adj.Mime))
	if err != nil {
		responderError(w, 500, "No se pudo guardar en la carpeta")
		return
	}
	s.registrarEvento(r, db.AccionArchivoSubido, rec.ID, tituloOUrl(rec))
	responderCreado(w, recursoConArchivo(s.Base, rec))
}

// adjuntarDesdeRecurso atiende POST .../adjuntos/desde-recurso {recurso_id}:
// vincula a la pizarra un recurso de archivo de la carpeta de la sesión.
func (s *Servidor) adjuntarDesdeRecurso(w http.ResponseWriter, r *http.Request) {
	sesionID, ok := s.sesionDeRuta(w, r)
	if !ok {
		return
	}
	var cuerpo struct {
		RecursoID int64 `json:"recurso_id"`
	}
	if err := leerJSON(w, r, &cuerpo); err != nil {
		if esBodyDemasiadoGrande(err) {
			responderError(w, http.StatusRequestEntityTooLarge, "Petición demasiado grande")
			return
		}
		responderError(w, 400, "JSON inválido")
		return
	}
	carpetaID, ok := s.carpetaDeSesion(w, sesionID)
	if !ok {
		return
	}
	rec, existe, err := db.ObtenerRecurso(s.Base, cuerpo.RecursoID)
	if err != nil {
		responderError(w, 500, "No se pudo leer el recurso")
		return
	}
	if !existe || rec.CarpetaID != carpetaID || rec.ArchivoID == 0 {
		responderError(w, 400, "El recurso no es un archivo de la carpeta de la sesión")
		return
	}
	a, hay, err := db.ObtenerArchivo(s.Base, rec.ArchivoID)
	if err != nil {
		responderError(w, 500, "No se pudo leer el archivo")
		return
	}
	if !hay {
		responderError(w, 400, "El recurso no es un archivo de la carpeta de la sesión")
		return
	}
	if rec.Tipo == "video" || archivos.TipoRecurso(a.Mime) == "video" {
		responderError(w, 400, "Los videos no se pueden adjuntar a la pizarra")
		return
	}
	adj, err := db.CrearAdjunto(s.Base, sesionID, a.ID, a.NombreOriginal)
	if err != nil {
		responderError(w, 500, "No se pudo adjuntar el archivo")
		return
	}
	responderCreado(w, fichaAdjunto(adj))
}
