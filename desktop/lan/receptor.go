package lan

// receptor.go — El listener HTTPS LAN: arranca/para, aplica el cert
// propio, la guardia de subred y el apagado por inactividad. Mux solo
// trae /v1/salud (placeholder): las rutas de negocio (unidades 3, 4a,
// 4b) se registran en rc.Mux antes de Start. La categoría de red de
// Windows (unidad 2b) y los otros 3 disparadores de parada (unidad 5a)
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

// TiempoInactividad apaga el listener si nadie lo tocó en este lapso.
const TiempoInactividad = 30 * time.Minute

// Reloj abstrae time.Now para testear el apagado por inactividad sin
// esperar 30 minutos de verdad.
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
	actividad  time.Time
	direccion  string // ln.Addr().String(): el puerto real, útil si Puerto=0
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
	rc.actividad = rc.Reloj.Ahora()
	// Cualquier request (aceptada o rechazada por la guardia) cuenta como
	// actividad: un intento de conexión ya demuestra uso ("Idle auto-off"
	// es sin requests, no sin requests válidas).
	marcaActividad := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rc.mu.Lock()
			rc.actividad = rc.Reloj.Ahora()
			rc.mu.Unlock()
			next.ServeHTTP(w, r)
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

// vigilarInactividad revisa cada IntervaloRevision (1 min por defecto) si
// pasaron TiempoInactividad desde la última actividad, y se para sola.
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
			inactivo := rc.servidor != nil && rc.Reloj.Ahora().Sub(rc.actividad) >= TiempoInactividad
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
