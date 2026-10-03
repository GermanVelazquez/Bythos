package lan

// links.go — El puerto Biblioteca (lo que la LAN necesita de la app) y las
// rutas de lectura/links. lan es dueño del puerto y api lo implementa
// (api/puente_lan.go): así lan nunca importa api. Nada de lo que llega del
// celular se loguea.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"bythos-desktop/db"
)

// ErrCarpetaNoExiste la devuelve la Biblioteca cuando la carpeta destino
// no existe (o se borró en el medio): el cable la traduce a 404.
var ErrCarpetaNoExiste = errors.New("lan: la carpeta no existe")

// Carpeta es lo que el celular ve de una carpeta.
type Carpeta struct {
	ID     int64  `json:"id"`
	Nombre string `json:"nombre"`
}

// Biblioteca es el puerto hacia la biblioteca de la app. actor es el nombre
// del dispositivo: el puente registra el evento con origen "celular".
type Biblioteca interface {
	ListarCarpetas() ([]Carpeta, error)
	ExisteCarpeta(id int64) bool
	GuardarLink(carpetaID int64, url, titulo, actor string) (recursoID int64, tituloFinal string, err error)
	GuardarArchivo(carpetaID int64, r io.Reader, nombre, actor string) (recursoID int64, err error)
}

// Servicio agrupa las rutas autenticadas de la LAN que tocan la biblioteca.
type Servicio struct {
	base     *sql.DB
	bib      Biblioteca
	almacen  *AlmacenSubidas
	creacion sync.Mutex // serializa crear (idempotencia + tope de 3 activas)
	trabajos sync.WaitGroup
}

// NuevoServicio arma el servicio; 5a lo construye y llama Registrar(rc.Mux).
func NuevoServicio(base *sql.DB, bib Biblioteca, almacen *AlmacenSubidas) *Servicio {
	return &Servicio{base: base, bib: bib, almacen: almacen}
}

// Esperar bloquea hasta que terminen las finalizaciones en curso
// (apagado ordenado y tests).
func (s *Servicio) Esperar() { s.trabajos.Wait() }

// Registrar monta carpetas, links y subidas. Todo exige Bearer.
func (s *Servicio) Registrar(mux *http.ServeMux) {
	auth := func(h func(http.ResponseWriter, *http.Request, db.Dispositivo)) http.Handler {
		return ConToken(s.base, h)
	}
	mux.Handle("GET /v1/carpetas", auth(s.manejarCarpetas))
	mux.Handle("POST /v1/links", auth(s.manejarLink))
	mux.Handle("POST /v1/subidas", auth(s.manejarCrearSubida))
	mux.Handle("POST /v1/subidas/{id}/estado", auth(s.manejarEstadoSubida))
	mux.Handle("POST /v1/subidas/{id}/partes", auth(s.manejarParte))
	mux.Handle("POST /v1/subidas/{id}/cancelar", auth(s.manejarCancelar))
}

func (s *Servicio) manejarCarpetas(w http.ResponseWriter, r *http.Request, _ db.Dispositivo) {
	carpetas, err := s.bib.ListarCarpetas()
	if err != nil {
		responderErrorLAN(w, http.StatusInternalServerError, "error_interno", "Error interno")
		return
	}
	responderJSON(w, http.StatusOK, carpetas)
}

// manejarLink: la carpeta se valida PRIMERO, antes de pedir metadata a la
// red: una carpeta inexistente no escribe nada ni toca internet.
func (s *Servicio) manejarLink(w http.ResponseWriter, r *http.Request, d db.Dispositivo) {
	var cuerpo struct {
		CarpetaID int64  `json:"carpeta_id"`
		URL       string `json:"url"`
		Titulo    string `json:"titulo"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if json.NewDecoder(r.Body).Decode(&cuerpo) != nil || cuerpo.CarpetaID == 0 {
		responderError(w, ErrCuerpoInvalido)
		return
	}
	u, err := url.Parse(strings.TrimSpace(cuerpo.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		responderError(w, ErrCuerpoInvalido)
		return
	}
	if !s.bib.ExisteCarpeta(cuerpo.CarpetaID) {
		responderError(w, ErrCarpetaNoEncontrada)
		return
	}
	id, titulo, err := s.bib.GuardarLink(cuerpo.CarpetaID, u.String(), strings.TrimSpace(cuerpo.Titulo), d.Nombre)
	if errors.Is(err, ErrCarpetaNoExiste) {
		responderError(w, ErrCarpetaNoEncontrada)
		return
	}
	if err != nil {
		responderErrorLAN(w, http.StatusInternalServerError, "error_interno", "Error interno")
		return
	}
	responderJSON(w, http.StatusCreated, map[string]any{"recurso_id": id, "titulo": titulo})
}
