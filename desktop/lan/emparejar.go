package lan

// emparejar.go — El emparejamiento por QR: un código de un solo uso en
// memoria (nunca en la base), solicitudes que el usuario aprueba desde la
// PC con un SAS de 6 dígitos que ambos lados muestran, y el token que se
// entrega UNA sola vez. Nada de esto se loguea: ni códigos ni tokens.

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"bythos-desktop/db"
)

const (
	TTLCodigo           = 10 * time.Minute
	MaxSolicitudes      = 3
	MaxCodigosMalos     = 5 // por ventana de un minuto
	VentanaCodigosMalos = time.Minute
	TiempoBloqueo       = 30 * time.Second
)

// Estados de una solicitud de emparejamiento.
const (
	EstadoPendiente = "pendiente"
	EstadoAprobado  = "aprobado"
	EstadoRechazado = "rechazado"
)

// ErrSolicitudNoEncontrada: id desconocido, expirado o ya resuelto.
var ErrSolicitudNoEncontrada = errors.New("solicitud_no_encontrada")

// Pendiente es lo que la UI de la PC muestra para aprobar o rechazar.
type Pendiente struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
	SAS    string `json:"sas"`
}

type solicitud struct {
	nombre      string
	sas         string
	estado      string
	token       string // en claro hasta que se entrega; luego ""
	dispositivo int64
	creada      time.Time
}

// Emparejador guarda el código activo y las solicitudes (todo en memoria).
type Emparejador struct {
	base  *sql.DB
	reloj Reloj

	mu          sync.Mutex
	codigo      string
	expira      time.Time
	malos       []time.Time
	bloqueado   time.Time
	solicitudes map[string]*solicitud
}

// NuevoEmparejador arranca con un código vigente.
func NuevoEmparejador(base *sql.DB, reloj Reloj) *Emparejador {
	e := &Emparejador{base: base, reloj: reloj, solicitudes: map[string]*solicitud{}}
	e.renovar()
	return e
}

func aleatorioB64(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // sin entropía no hay seguridad posible
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// renovar rota el código (16 bytes, 22 chars base64url). Con el lock tomado.
func (e *Emparejador) renovar() {
	e.codigo = aleatorioB64(16)
	e.expira = e.reloj.Ahora().Add(TTLCodigo)
}

// Renovar rota el código a pedido del usuario (botón "renovar" de la UI).
func (e *Emparejador) Renovar() (string, time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.renovar()
	return e.codigo, e.expira
}

// Codigo es el código vigente y su vencimiento (para armar el QR).
func (e *Emparejador) Codigo() (string, time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.codigo, e.expira
}

// SAS deriva los 6 dígitos de confirmación: primeros 20 bits de
// SHA-256(codigo‖idSolicitud) mod 10^6. Ambas pantallas muestran lo mismo.
func SAS(codigo, idSolicitud string) string {
	h := sha256.Sum256([]byte(codigo + idSolicitud))
	n := binary.BigEndian.Uint32(h[:4]) >> 12 // 20 bits
	return fmt.Sprintf("%06d", n%1000000)
}

// podarPendientes descarta solicitudes sin resolver tras TTLCodigo
// ("ignorar" = rechazar). Con el lock tomado.
func (e *Emparejador) podarPendientes() {
	for id, s := range e.solicitudes {
		if s.estado == EstadoPendiente && e.reloj.Ahora().Sub(s.creada) >= TTLCodigo {
			delete(e.solicitudes, id)
		}
	}
}

// Solicitar valida el código y crea una solicitud pendiente. El código se
// consume (rota) al primer uso válido. Devuelve el id secreto y el SAS, o
// uno de los códigos de cable (ErrCodigoInvalido/Expirado/Bloqueado).
func (e *Emparejador) Solicitar(codigo, nombre string) (id, sas, errCable string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ahora := e.reloj.Ahora()
	if ahora.Before(e.bloqueado) {
		return "", "", ErrBloqueado
	}
	if subtle.ConstantTimeCompare([]byte(codigo), []byte(e.codigo)) != 1 {
		// Ventana deslizante de intentos malos; al llegar al tope se rota
		// el código (el QR filtrado queda inútil) y se bloquea 30 s.
		vivos := e.malos[:0]
		for _, t := range e.malos {
			if ahora.Sub(t) < VentanaCodigosMalos {
				vivos = append(vivos, t)
			}
		}
		e.malos = append(vivos, ahora)
		if len(e.malos) >= MaxCodigosMalos {
			e.renovar()
			e.malos = nil
			e.bloqueado = ahora.Add(TiempoBloqueo)
			return "", "", ErrBloqueado
		}
		return "", "", ErrCodigoInvalido
	}
	if !ahora.Before(e.expira) {
		return "", "", ErrCodigoExpirado
	}
	e.podarPendientes()
	pendientes := 0
	for _, s := range e.solicitudes {
		if s.estado == EstadoPendiente {
			pendientes++
		}
	}
	if pendientes >= MaxSolicitudes {
		return "", "", ErrBloqueado
	}
	id = aleatorioB64(16)
	sas = SAS(e.codigo, id)
	e.solicitudes[id] = &solicitud{nombre: nombre, sas: sas, estado: EstadoPendiente, creada: ahora}
	e.renovar() // un solo uso
	return id, sas, ""
}

// Pendientes lista las solicitudes por aprobar (UI de la PC).
func (e *Emparejador) Pendientes() []Pendiente {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.podarPendientes()
	out := []Pendiente{}
	for id, s := range e.solicitudes {
		if s.estado == EstadoPendiente {
			out = append(out, Pendiente{ID: id, Nombre: s.nombre, SAS: s.sas})
		}
	}
	return out
}

// Aprobar emite el token, guarda solo su hash y deja el token listo para
// entregarse una vez en el próximo GET de estado.
func (e *Emparejador) Aprobar(id string) (db.Dispositivo, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.solicitudes[id]
	if !ok || s.estado != EstadoPendiente {
		return db.Dispositivo{}, ErrSolicitudNoEncontrada
	}
	token, hash, err := GenerarToken()
	if err != nil {
		return db.Dispositivo{}, err
	}
	d, err := db.CrearDispositivo(e.base, s.nombre, hash)
	if err != nil {
		return db.Dispositivo{}, err
	}
	s.estado, s.token, s.dispositivo = EstadoAprobado, token, d.ID
	return d, nil
}

// Rechazar niega la solicitud: nunca se emite token.
func (e *Emparejador) Rechazar(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.solicitudes[id]
	if !ok || s.estado != EstadoPendiente {
		return ErrSolicitudNoEncontrada
	}
	s.estado = EstadoRechazado
	return nil
}

// Registrar monta POST /v1/emparejar y GET /v1/emparejar/{solicitud}.
func (e *Emparejador) Registrar(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/emparejar", e.manejarSolicitar)
	mux.HandleFunc("GET /v1/emparejar/{solicitud}", e.manejarEstado)
}

func (e *Emparejador) manejarSolicitar(w http.ResponseWriter, r *http.Request) {
	var cuerpo struct{ Codigo, Nombre string }
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if json.NewDecoder(r.Body).Decode(&cuerpo) != nil || strings.TrimSpace(cuerpo.Nombre) == "" {
		responderError(w, ErrCuerpoInvalido)
		return
	}
	id, sas, errCable := e.Solicitar(cuerpo.Codigo, strings.TrimSpace(cuerpo.Nombre))
	if errCable != "" {
		responderError(w, errCable)
		return
	}
	responderJSON(w, http.StatusAccepted, map[string]string{"solicitud": id, "sas": sas})
}

// manejarEstado entrega el token una única vez: después de leerlo se borra.
func (e *Emparejador) manejarEstado(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("solicitud")
	e.mu.Lock()
	defer e.mu.Unlock()
	e.podarPendientes()
	s, ok := e.solicitudes[id]
	if !ok {
		responderError(w, ErrCodigoExpirado)
		return
	}
	resp := map[string]any{"estado": s.estado}
	if s.estado == EstadoAprobado {
		resp["dispositivo_id"] = s.dispositivo
		if s.token != "" {
			resp["token"] = s.token
			s.token = ""
		}
	}
	responderJSON(w, http.StatusOK, resp)
}
