package lan

// subidas.go — Las rutas de subida resumible (todas POST) sobre el
// almacén: crear, estado, partes y cancelar, más el worker asíncrono que
// finaliza (hash completo, carpeta, archivos.Guardar y evento). Ninguna
// ruta loguea tokens ni contenido.

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"bythos-desktop/archivos"
	"bythos-desktop/db"
)

const (
	// ChunkMax es el tope de una parte (lo que el celular debe respetar).
	ChunkMax = 8 << 20
	// MaxSubidasActivas por dispositivo.
	MaxSubidasActivas = 3
)

// Estados de una subida y errores de finalización (campo "error" del estado).
const (
	estadoRecibiendo  = "recibiendo"
	estadoVerificando = "verificando"
	estadoListo       = "listo"
	estadoError       = "error"

	errHashArchivo = "hash_archivo_invalido"
	errInterno     = "error_interno"
)

func (s *Servicio) manejarCrearSubida(w http.ResponseWriter, r *http.Request, d db.Dispositivo) {
	var c struct {
		ClienteID string `json:"cliente_id"`
		CarpetaID int64  `json:"carpeta_id"`
		Nombre    string `json:"nombre"`
		Tamano    int64  `json:"tamano"`
		SHA256    string `json:"sha256"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if json.NewDecoder(r.Body).Decode(&c) != nil || c.ClienteID == "" || len(c.ClienteID) > 64 ||
		c.CarpetaID == 0 || strings.TrimSpace(c.Nombre) == "" || c.Tamano <= 0 || len(c.SHA256) != 64 {
		responderError(w, ErrCuerpoInvalido)
		return
	}
	if !s.bib.ExisteCarpeta(c.CarpetaID) { // antes de crear sidecar o .parte
		responderError(w, ErrCarpetaNoEncontrada)
		return
	}
	if c.Tamano > archivos.TamanoMaximo {
		responderError(w, ErrTamanoExcedido)
		return
	}
	s.creacion.Lock()
	defer s.creacion.Unlock()
	activas := 0
	for _, e := range s.almacen.ListarConsulta() {
		if e.DispositivoID != d.ID {
			continue
		}
		if e.ClienteID == c.ClienteID { // idempotente: devuelve la misma subida
			responderJSON(w, http.StatusCreated, respuestaCrear(e))
			return
		}
		if e.Estado == estadoRecibiendo || e.Estado == estadoVerificando {
			activas++
		}
	}
	if activas >= MaxSubidasActivas {
		responderError(w, ErrDemasiadasSubidas)
		return
	}
	nueva, err := s.almacen.Crear(Subida{DispositivoID: d.ID, ClienteID: c.ClienteID, CarpetaID: c.CarpetaID,
		Nombre: c.Nombre, Tamano: c.Tamano, SHA256: strings.ToLower(c.SHA256)})
	if err != nil {
		responderErrorLAN(w, http.StatusInternalServerError, errInterno, "Error interno")
		return
	}
	responderJSON(w, http.StatusCreated, respuestaCrear(nueva))
}

func respuestaCrear(e *Subida) map[string]any {
	return map[string]any{"id": e.ID, "offset": offsetVisible(e), "chunk_max": ChunkMax}
}

// offsetVisible: una subida "lista" ya recibió todo (su .parte se vació).
func offsetVisible(e *Subida) int64 {
	if e.Estado == estadoListo {
		return e.Tamano
	}
	return e.Offset
}

// propia carga la subida y exige que sea del dispositivo; si no, 404.
func (s *Servicio) propia(w http.ResponseWriter, r *http.Request, d db.Dispositivo) (*Subida, bool) {
	e, err := s.almacen.Consultar(r.PathValue("id")) // sin truncar: puede haber una escritura en curso
	if err != nil || e.DispositivoID != d.ID {
		responderError(w, ErrSubidaNoEncontrada)
		return nil, false
	}
	return e, true
}

func (s *Servicio) manejarEstadoSubida(w http.ResponseWriter, r *http.Request, d db.Dispositivo) {
	e, ok := s.propia(w, r, d)
	if !ok {
		return
	}
	resp := map[string]any{"estado": e.Estado, "offset": offsetVisible(e), "tamano": e.Tamano}
	if e.RecursoID != 0 {
		resp["recurso_id"] = e.RecursoID
	}
	if e.Error != "" {
		resp["error"] = e.Error
	}
	responderJSON(w, http.StatusOK, resp)
}

func (s *Servicio) manejarCancelar(w http.ResponseWriter, r *http.Request, d db.Dispositivo) {
	e, ok := s.propia(w, r, d)
	if !ok {
		return
	}
	liberar, libre := s.almacen.Bloquear(e.ID)
	if !libre {
		responderError(w, ErrSubidaOcupada)
		return
	}
	defer liberar()
	s.almacen.Eliminar(e.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servicio) manejarParte(w http.ResponseWriter, r *http.Request, d db.Dispositivo) {
	if _, ok := s.propia(w, r, d); !ok {
		return
	}
	id := r.PathValue("id")
	offset, err := strconv.ParseInt(r.Header.Get("X-Offset"), 10, 64)
	hash := r.Header.Get("X-Chunk-SHA256")
	if err != nil || offset < 0 || hash == "" || r.ContentLength <= 0 {
		responderError(w, ErrCuerpoInvalido)
		return
	}
	if r.ContentLength > ChunkMax {
		responderError(w, ErrTamanoExcedido)
		return
	}
	liberar, libre := s.almacen.Bloquear(id)
	if !libre {
		responderError(w, ErrSubidaOcupada)
		return
	}
	traspasado := false // la finalización asume el candado
	defer func() {
		if !traspasado {
			liberar()
		}
	}()
	e, err := s.almacen.Cargar(id) // estado fresco bajo el candado
	if err != nil {
		responderError(w, ErrSubidaNoEncontrada)
		return
	}
	if e.Estado != estadoRecibiendo {
		responderError(w, ErrSubidaOcupada)
		return
	}
	if offset != e.Offset {
		respuestaOffset(w, e.Offset)
		return
	}
	if e.Offset+r.ContentLength > e.Tamano { // supera lo declarado: se descarta todo
		s.almacen.Eliminar(id)
		responderError(w, ErrTamanoExcedido)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, ChunkMax)
	e, err = s.almacen.Anexar(id, offset, r.Body, hash)
	var excedido *http.MaxBytesError
	switch {
	case errors.Is(err, ErrOffsetParte):
		respuestaOffset(w, e.Offset)
	case errors.Is(err, ErrHashParte):
		responderError(w, ErrHashParteInvalido)
	case errors.As(err, &excedido):
		responderError(w, ErrTamanoExcedido)
	case err != nil:
		responderErrorLAN(w, http.StatusInternalServerError, errInterno, "Error interno")
	case e.Offset < e.Tamano:
		responderJSON(w, http.StatusOK, map[string]any{"offset": e.Offset})
	default:
		e.Estado = estadoVerificando
		if s.almacen.Actualizar(e) != nil {
			responderErrorLAN(w, http.StatusInternalServerError, errInterno, "Error interno")
			return
		}
		traspasado = true
		s.trabajos.Add(1)
		go s.finalizar(e, d.Nombre, liberar)
		responderJSON(w, http.StatusAccepted, map[string]any{"offset": e.Offset, "estado": estadoVerificando})
	}
}

// respuestaOffset: 409 offset_invalido con el offset actual en el cuerpo,
// para que el celular retome desde ahí.
func respuestaOffset(w http.ResponseWriter, offset int64) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	json.NewEncoder(w).Encode(map[string]any{"error": ErrOffsetInvalido, "mensaje": errorCable[ErrOffsetInvalido].Mensaje, "offset": offset})
}

// finalizar es el worker: hash completo, carpeta, archivos.Guardar y evento
// (todo dentro de GuardarArchivo). Cualquier error deja el temp vacío y no
// registra evento.
func (s *Servicio) finalizar(e *Subida, actor string, liberar func()) {
	defer s.trabajos.Done()
	defer liberar()
	if suma, err := e.SumaHex(); err != nil || suma != e.SHA256 {
		s.terminar(e, estadoError, errHashArchivo, 0)
		return
	}
	if !s.bib.ExisteCarpeta(e.CarpetaID) {
		s.terminar(e, estadoError, ErrCarpetaNoEncontrada, 0)
		return
	}
	f, err := s.almacen.AbrirParte(e.ID)
	if err != nil {
		s.terminar(e, estadoError, errInterno, 0)
		return
	}
	recurso, err := s.bib.GuardarArchivo(e.CarpetaID, f, e.Nombre, actor)
	f.Close()
	switch {
	case errors.Is(err, archivos.ErrTipoNoPermitido):
		s.terminar(e, estadoError, ErrTipoNoPermitido, 0)
	case errors.Is(err, ErrCarpetaNoExiste):
		s.terminar(e, estadoError, ErrCarpetaNoEncontrada, 0)
	case err != nil:
		s.terminar(e, estadoError, errInterno, 0)
	default:
		s.terminar(e, estadoListo, "", recurso)
	}
}

// terminar deja la subida en un estado final visible para el celular:
// vacía el .parte (el temp se descarta) pero conserva el sidecar para que
// el celular consulte el resultado; Limpiar lo borra a las 24 h.
func (s *Servicio) terminar(e *Subida, estado, errCodigo string, recurso int64) {
	os.Truncate(s.almacen.rutaParte(e.ID), 0)
	e.Offset, e.Estado, e.Error, e.RecursoID = 0, estado, errCodigo, recurso
	e.EstadoHash = nil
	if s.almacen.Actualizar(e) != nil {
		s.almacen.Eliminar(e.ID)
	}
}
