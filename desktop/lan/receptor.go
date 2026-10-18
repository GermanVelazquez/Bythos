package lan

// receptor.go — El listener HTTPS LAN: arranca/para, aplica el cert
// propio, la guardia de subred y el apagado por inactividad. Mux solo
// trae /v1/salud (placeholder): las rutas de negocio (unidades 3, 4a,
// 4b) se registran en rc.Mux antes de Start. La categoría de red de
// Windows se valida en red.go (unidad 2b); los otros 3 disparadores de parada (unidad 5a)
// no viven acá todavía.

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// PuertoPorDefecto es el puerto fijo del diseño, SIN fallback (el
// firewall del instalador fija localport=48080). NuevoReceptor lo usa
// por defecto; un test que pisa Puerto=0 deja que el SO elija uno libre
// (idioma estándar de net.Listen), útil para no pelear por el 48080 real
// de la máquina que corre `go test`.
const PuertoPorDefecto = 48080

// ErrPuertoOcupado: Start cuando el puerto ya está en uso.
var ErrPuertoOcupado = errors.New("puerto_ocupado")

// Ventana de recepción corta (enmienda 3b): el listener solo está abierto
// mientras se usa de verdad.
// VentanaSinUso: si nadie completó una request exitosa desde Start, se
// apaga (iguala el TTL del código de emparejamiento).
// VentanaTrasUso: con al menos una request exitosa, se apaga tras este
// lapso sin otra exitosa.
const (
	VentanaSinUso  = 10 * time.Minute
	VentanaTrasUso = 2 * time.Minute
)

// registraEstado captura el status de la respuesta para decidir si la
// request cuenta como actividad (solo < 400).
type registraEstado struct {
	http.ResponseWriter
	status int
}

func (w *registraEstado) WriteHeader(c int) {
	if w.status == 0 {
		w.status = c
	}
	w.ResponseWriter.WriteHeader(c)
}

func (w *registraEstado) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *registraEstado) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *registraEstado) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Reloj abstrae time.Now para testear el apagado por inactividad sin
// esperar minutos de verdad.
type Reloj interface{ Ahora() time.Time }

type relojReal struct{}

func (relojReal) Ahora() time.Time { return time.Now() }

// Receptor arma y controla el listener HTTPS LAN.
type Receptor struct {
	Puerto            int
	Reloj             Reloj
	IntervaloRevision time.Duration
	Mux               *http.ServeMux // rutas de negocio de unidades futuras

	mu         sync.Mutex
	servidor   *http.Server
	pararIdle  chan struct{}
	interfaz   Interfaz
	inicio     time.Time // momento del Start
	actividad  time.Time // última request exitosa
	huboUso    bool      // ya hubo al menos una request exitosa
	direccion  string    // ln.Addr().String(): el puerto real, útil si Puerto=0
	categoria  Categoria
	certActual tls.Certificate
}

// NuevoReceptor arma un Receptor con defaults de producción y /v1/salud.
func NuevoReceptor() *Receptor {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/salud", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"app":"bythos","api":1}`))
	})
	return &Receptor{Puerto: PuertoPorDefecto, Reloj: relojReal{}, Mux: mux}
}

// Start bindea iface.IP:Puerto en TLS (SAN = iface.IP), aplica la
// guardia de subred y arranca el monitor de inactividad. Idempotente.
func (rc *Receptor) Start(iface Interfaz) error {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.servidor != nil {
		return nil
	}

	// Compuerta de perfil de red: antes de tocar cert o puerto. Pública o
	// de Dominio se rechaza; desconocida arranca y queda el aviso.
	cat := categoriaDe(iface.IP)
	if cat == CategoriaPublica || cat == CategoriaDominio {
		return ErrRedNoPrivada
	}
	rc.categoria = cat

	cert, err := certificadoParaIPs(CarpetaLAN(), []net.IP{iface.IP})
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(iface.IP.String(), strconv.Itoa(rc.Puerto)))
	if err != nil {
		return ErrPuertoOcupado
	}
	lnTLS := tls.NewListener(ln, &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}})

	rc.interfaz = iface
	rc.direccion = ln.Addr().String()
	rc.certActual = cert
	rc.inicio = rc.Reloj.Ahora()
	rc.huboUso = false
	// Solo cuenta como actividad una respuesta con status < 400: los 403
	// de la guardia, 401 y demás errores NO extienden la ventana, así un
	// extraño en la LAN no puede mantener el puerto abierto.
	marcaActividad := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &registraEstado{ResponseWriter: w}
			next.ServeHTTP(rw, r)
			if rw.status < 400 {
				rc.mu.Lock()
				rc.actividad = rc.Reloj.Ahora()
				rc.huboUso = true
				rc.mu.Unlock()
			}
		})
	}
	rc.servidor = &http.Server{
		Handler:           marcaActividad(conGuardia(iface.Rango, rc.Mux)),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	go rc.servidor.Serve(lnTLS)

	rc.pararIdle = make(chan struct{})
	go rc.vigilarInactividad(rc.pararIdle)
	return nil
}

// vigilarInactividad revisa cada IntervaloRevision (1 min por defecto) las
// dos ventanas (VentanaSinUso desde Start, VentanaTrasUso desde la última
// request exitosa) y se para sola.
// parar corta el loop desde un Stop externo sin esperar la revisión.
func (rc *Receptor) vigilarInactividad(parar chan struct{}) {
	intervalo := rc.IntervaloRevision
	if intervalo <= 0 {
		intervalo = time.Minute
	}
	ticker := time.NewTicker(intervalo)
	defer ticker.Stop()
	for {
		select {
		case <-parar:
			return
		case <-ticker.C:
			rc.mu.Lock()
			inactivo := false
			if rc.servidor != nil {
				ahora := rc.Reloj.Ahora()
				if rc.huboUso {
					inactivo = ahora.Sub(rc.actividad) >= VentanaTrasUso
				} else {
					inactivo = ahora.Sub(rc.inicio) >= VentanaSinUso
				}
			}
			rc.mu.Unlock()
			if inactivo {
				_ = rc.Stop(context.Background())
				return
			}
		}
	}
}

// Stop apaga con gracia: Shutdown da 10s para que una request en vuelo
// termine, y recién ahí Close si no llegó a tiempo. Idempotente.
func (rc *Receptor) Stop(ctx context.Context) error {
	rc.mu.Lock()
	servidor := rc.servidor
	pararIdle := rc.pararIdle
	rc.servidor = nil
	rc.pararIdle = nil
	rc.mu.Unlock()

	if servidor == nil {
		return nil
	}
	if pararIdle != nil {
		close(pararIdle)
	}
	apagarCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := servidor.Shutdown(apagarCtx); err != nil {
		return servidor.Close()
	}
	return nil
}

// Activo dice si el listener está corriendo (unidad 5a: /api/receptor/estado).
func (rc *Receptor) Activo() bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.servidor != nil
}

// RedDesconocida dice si el último Start no pudo confirmar que la red es
// Privada; la UI (unidad 5b) muestra el aviso.
func (rc *Receptor) RedDesconocida() bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.categoria == CategoriaDesconocida
}

// Direccion es host:puerto real del último Start (útil con Puerto=0).
func (rc *Receptor) Direccion() string {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.direccion
}

// Huella es el FingerprintSPKI del cert servido en el último Start; lo
// usa el QR (unidad 3) y los tests para armar el cliente pineado.
func (rc *Receptor) Huella() (string, error) {
	rc.mu.Lock()
	cert := rc.certActual
	rc.mu.Unlock()
	return FingerprintSPKI(cert)
}
