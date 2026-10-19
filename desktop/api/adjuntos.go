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

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"bythos-desktop/archivos"
	"bythos-desktop/db"
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
	responderCreado(w, map[string]any{
		"id": adj.ID, "sha256": adj.SHA256, "nombre": adj.Nombre,
		"mime": adj.Mime, "tipo": archivos.TipoRecurso(adj.Mime),
	})
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
