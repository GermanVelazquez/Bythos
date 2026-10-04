package lan

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// IdleSubida es cuánto puede estar una subida sin recibir partes antes de
// que la limpieza la descarte.
const IdleSubida = 24 * time.Hour

var (
	ErrSubidaNoExiste = errors.New("lan: la subida no existe")
	ErrHashParte      = errors.New("lan: el hash de la parte no coincide")
	ErrOffsetParte    = errors.New("lan: la parte no empieza en el offset actual")
)

// Subida es el sidecar JSON de una subida en curso. El nombre del archivo
// del cliente es solo metadato: jamás se usa como ruta, solo el ID.
type Subida struct {
	ID            string    `json:"id"`
	DispositivoID int64     `json:"dispositivo_id"`
	ClienteID     string    `json:"cliente_id"`
	CarpetaID     int64     `json:"carpeta_id"`
	Nombre        string    `json:"nombre"`
	Tamano        int64     `json:"tamano"`
	SHA256        string    `json:"sha256"` // hash declarado del archivo completo (hex)
	Offset        int64     `json:"offset"`
	EstadoHash    []byte    `json:"estado_hash"` // MarshalBinary del sha256 en curso
	Estado        string    `json:"estado"`
	Error         string    `json:"error,omitempty"`
	RecursoID     int64     `json:"recurso_id,omitempty"`
	Actualizada   time.Time `json:"actualizada"`
}

// AlmacenSubidas guarda sidecar + .parte por subida en una carpeta.
type AlmacenSubidas struct {
	carpeta string
	reloj   Reloj

	mu       sync.Mutex
	bloqueos map[string]*sync.Mutex
}

// NuevoAlmacenSubidas usa CarpetaLAN()\subidas y el reloj indicado.
func NuevoAlmacenSubidas(reloj Reloj) *AlmacenSubidas {
	return nuevoAlmacenEn(filepath.Join(CarpetaLAN(), "subidas"), reloj)
}

func nuevoAlmacenEn(carpeta string, reloj Reloj) *AlmacenSubidas {
	return &AlmacenSubidas{carpeta: carpeta, reloj: reloj, bloqueos: map[string]*sync.Mutex{}}
}

// idValido: solo 32 hex minúsculos. Es lo único que llega a un nombre de
// archivo, así que cierra cualquier path traversal.
func idValido(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func (a *AlmacenSubidas) rutaSidecar(id string) string { return filepath.Join(a.carpeta, id+".json") }
func (a *AlmacenSubidas) rutaParte(id string) string   { return filepath.Join(a.carpeta, id+".parte") }

// Crear registra una subida nueva con sidecar y .parte vacío.
func (a *AlmacenSubidas) Crear(s Subida) (*Subida, error) {
	crudo := make([]byte, 16)
	if _, err := rand.Read(crudo); err != nil {
		return nil, err
	}
	s.ID = hex.EncodeToString(crudo)
	s.Offset = 0
	s.Estado = "recibiendo"
	estado, err := marshalHash(sha256.New())
	if err != nil {
		return nil, err
	}
	s.EstadoHash = estado
	if err := os.MkdirAll(a.carpeta, 0700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(a.rutaParte(s.ID), nil, 0600); err != nil {
		return nil, err
	}
	if err := a.guardar(&s); err != nil {
		os.Remove(a.rutaParte(s.ID))
		return nil, err
	}
	return &s, nil
}

func marshalHash(h hash.Hash) ([]byte, error) {
	return h.(encoding.BinaryMarshaler).MarshalBinary()
}

func hashDesde(estado []byte) (hash.Hash, error) {
	h := sha256.New()
	if err := h.(encoding.BinaryUnmarshaler).UnmarshalBinary(estado); err != nil {
		return nil, err
	}
	return h, nil
}

// guardar escribe el sidecar con temp+rename: nunca queda a medias.
func (a *AlmacenSubidas) guardar(s *Subida) error {
	s.Actualizada = a.reloj.Ahora()
	crudo, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := a.rutaSidecar(s.ID) + ".tmp"
	if err := os.WriteFile(tmp, crudo, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, a.rutaSidecar(s.ID))
}

// Cargar lee el sidecar y recupera de un crash: trunca el .parte al offset
// del sidecar (los bytes de más son de una parte que no llegó a verificarse).
func (a *AlmacenSubidas) Cargar(id string) (*Subida, error) {
	if !idValido(id) {
		return nil, ErrSubidaNoExiste
	}
	crudo, err := os.ReadFile(a.rutaSidecar(id))
	if err != nil {
		return nil, ErrSubidaNoExiste
	}
	var s Subida
	if err := json.Unmarshal(crudo, &s); err != nil || s.ID != id {
		return nil, ErrSubidaNoExiste
	}
	if info, err := os.Stat(a.rutaParte(id)); err != nil || info.Size() < s.Offset {
		a.Eliminar(id) // sidecar sin datos: irrecuperable
		return nil, ErrSubidaNoExiste
	}
	if err := os.Truncate(a.rutaParte(id), s.Offset); err != nil {
		return nil, err
	}
	return &s, nil
}

// Anexar escribe una parte en el offset indicado y verifica su SHA-256
// (hex). Si el hash no coincide, trunca al offset previo y restaura el
// estado del hash: la subida queda como estaba.
func (a *AlmacenSubidas) Anexar(id string, offset int64, r io.Reader, hashHex string) (*Subida, error) {
	s, err := a.Cargar(id)
	if err != nil {
		return nil, err
	}
	if offset != s.Offset {
		return s, ErrOffsetParte
	}
	total, err := hashDesde(s.EstadoHash)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(a.rutaParte(id), os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	parte := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, parte, total), r)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	esperado, derr := hex.DecodeString(hashHex)
	if err == nil && (derr != nil || subtle.ConstantTimeCompare(parte.Sum(nil), esperado) != 1) {
		err = ErrHashParte
	}
	if err != nil {
		os.Truncate(a.rutaParte(id), s.Offset)
		return s, err
	}
	estado, err := marshalHash(total)
	if err != nil {
		os.Truncate(a.rutaParte(id), s.Offset)
		return s, err
	}
	previo := s.Offset
	s.Offset += n
	s.EstadoHash = estado
	if err := a.guardar(s); err != nil {
		os.Truncate(a.rutaParte(id), previo)
		return nil, err
	}
	return s, nil
}

// SumaHex devuelve el SHA-256 acumulado de lo recibido hasta ahora.
func (s *Subida) SumaHex() (string, error) {
	h, err := hashDesde(s.EstadoHash)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Actualizar persiste cambios de estado/error/recurso de una subida.
func (a *AlmacenSubidas) Actualizar(s *Subida) error {
	if !idValido(s.ID) {
		return ErrSubidaNoExiste
	}
	return a.guardar(s)
}

// AbrirParte abre el .parte para leerlo al finalizar.
func (a *AlmacenSubidas) AbrirParte(id string) (*os.File, error) {
	if !idValido(id) {
		return nil, ErrSubidaNoExiste
	}
	return os.Open(a.rutaParte(id))
}

// Bloquear toma el candado de una subida sin esperar (TryLock): ok=false si
// otra request ya la está escribiendo.
func (a *AlmacenSubidas) Bloquear(id string) (liberar func(), ok bool) {
	a.mu.Lock()
	m, existe := a.bloqueos[id]
	if !existe {
		m = &sync.Mutex{}
		a.bloqueos[id] = m
	}
	a.mu.Unlock()
	if !m.TryLock() {
		return nil, false
	}
	return m.Unlock, true
}

// Listar devuelve las subidas válidas (aplica la recuperación de crash).
func (a *AlmacenSubidas) Listar() []*Subida {
	entradas, _ := os.ReadDir(a.carpeta)
	var out []*Subida
	for _, e := range entradas {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok {
			continue
		}
		if s, err := a.Cargar(id); err == nil {
			out = append(out, s)
		}
	}
	return out
}

// Eliminar borra sidecar y .parte (cancelación, error, completada).
func (a *AlmacenSubidas) Eliminar(id string) {
	if !idValido(id) {
		return
	}
	os.Remove(a.rutaSidecar(id))
	os.Remove(a.rutaSidecar(id) + ".tmp")
	os.Remove(a.rutaParte(id))
	a.mu.Lock()
	delete(a.bloqueos, id)
	a.mu.Unlock()
}

// EliminarDispositivo descarta todas las subidas de un dispositivo. La
// unidad 5a lo llama al revocar un celular.
func (a *AlmacenSubidas) EliminarDispositivo(dispositivoID int64) {
	for _, s := range a.Listar() {
		if s.DispositivoID == dispositivoID {
			a.Eliminar(s.ID)
		}
	}
}

// Limpiar descarta subidas sin actividad por más de IdleSubida, y
// huérfanos (.parte sin sidecar).
func (a *AlmacenSubidas) Limpiar() {
	limite := a.reloj.Ahora().Add(-IdleSubida)
	for _, s := range a.Listar() {
		if s.Actualizada.Before(limite) {
			a.Eliminar(s.ID)
		}
	}
	entradas, _ := os.ReadDir(a.carpeta)
	for _, e := range entradas {
		if id, ok := strings.CutSuffix(e.Name(), ".parte"); ok {
			if _, err := os.Stat(a.rutaSidecar(id)); err != nil {
				os.Remove(filepath.Join(a.carpeta, e.Name()))
			}
		}
	}
}

// Vigilar limpia al arrancar y luego cada intervalo (1 h en producción)
// hasta que parar se cierre. Bloquea: llamarlo en una goroutine.
func (a *AlmacenSubidas) Vigilar(parar <-chan struct{}, intervalo time.Duration) {
	a.Limpiar()
	t := time.NewTicker(intervalo)
	defer t.Stop()
	for {
		select {
		case <-parar:
			return
		case <-t.C:
			a.Limpiar()
		}
	}
}
