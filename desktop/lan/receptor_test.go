package lan

// receptor_test.go — Start/Stop de punta a punta: TLS con cliente
// pineado, guardia de subred real, apagado por inactividad (reloj
// falso), Stop espera una request en vuelo, Stop es idempotente, y la
// huella se mantiene entre arranques (clave persistida).

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

// conCarpetaLANTemporal apunta CarpetaLAN() a t.TempDir(), nunca al
// %APPDATA% real de quien corre `go test` (mismo patrón que
// workspace_test.go).
func conCarpetaLANTemporal(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("APPDATA", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)
}

func ifazLoopback(cidr string) Interfaz {
	_, red, _ := net.ParseCIDR(cidr)
	return Interfaz{IP: net.ParseIP("127.0.0.1"), Nombre: "loopback-test", Rango: red}
}

// clientePineado fija el fingerprint SPKI en vez de validar contra una
// CA, igual que el celular real (PinnedTrustManager, unidad 7a).
func clientePineado(huella string) *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				VerifyPeerCertificate: func(raw [][]byte, _ [][]*x509.Certificate) error {
					if len(raw) == 0 {
						return errors.New("sin certificado")
					}
					hoja, err := x509.ParseCertificate(raw[0])
					if err != nil {
						return err
					}
					got, err := FingerprintSPKI(tls.Certificate{Leaf: hoja})
					if err != nil {
						return err
					}
					if got != huella {
						return errors.New("fingerprint no coincide")
					}
					return nil
				},
			},
		},
	}
}

type relojFalso struct {
	mu    sync.Mutex
	ahora time.Time
}

func (r *relojFalso) Ahora() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ahora
}

func (r *relojFalso) avanzar(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ahora = r.ahora.Add(d)
}

func TestReceptorTLSPinnedClientOK(t *testing.T) {
	conCarpetaLANTemporal(t)
	rc := NuevoReceptor()
	rc.Puerto = 0
	if err := rc.Start(ifazLoopback("127.0.0.1/32")); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer rc.Stop(context.Background())

	huella, err := rc.Huella()
	if err != nil {
		t.Fatalf("Huella: %v", err)
	}
	resp, err := clientePineado(huella).Get("https://" + rc.Direccion() + "/v1/salud")
	if err != nil {
		t.Fatalf("GET /v1/salud: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, quería 200", resp.StatusCode)
	}
}

func TestReceptorRechazaFueraDeSubred(t *testing.T) {
	conCarpetaLANTemporal(t)
	rc := NuevoReceptor()
	rc.Puerto = 0
	// 127.0.0.1 (el remoto real del cliente de test) NO cae en esta
	// subred: Private address, different subnet -> 403.
	if err := rc.Start(ifazLoopback("10.0.0.0/24")); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer rc.Stop(context.Background())

	huella, err := rc.Huella()
	if err != nil {
		t.Fatalf("Huella: %v", err)
	}
	resp, err := clientePineado(huella).Get("https://" + rc.Direccion() + "/v1/salud")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, quería 403", resp.StatusCode)
	}
}

func TestReceptorStopEsRefusaYEsIdempotente(t *testing.T) {
	conCarpetaLANTemporal(t)
	rc := NuevoReceptor()
	rc.Puerto = 0
	if err := rc.Start(ifazLoopback("127.0.0.1/32")); err != nil {
		t.Fatalf("Start: %v", err)
	}
	direccion := rc.Direccion()

	if err := rc.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := rc.Stop(context.Background()); err != nil {
		t.Fatalf("Stop repetido no debía fallar: %v", err)
	}
	if rc.Activo() {
		t.Fatalf("Activo() debía ser false tras Stop")
	}
	if _, err := net.DialTimeout("tcp", direccion, 500*time.Millisecond); err == nil {
		t.Fatalf("se esperaba conexión rechazada tras Stop (reception off)")
	}
}

func TestReceptorStopEsperaRequestEnVuelo(t *testing.T) {
	conCarpetaLANTemporal(t)
	rc := NuevoReceptor()
	rc.Puerto = 0
	arranco := make(chan struct{})
	rc.Mux.HandleFunc("GET /v1/lento", func(w http.ResponseWriter, r *http.Request) {
		close(arranco)
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	if err := rc.Start(ifazLoopback("127.0.0.1/32")); err != nil {
		t.Fatalf("Start: %v", err)
	}
	huella, err := rc.Huella()
	if err != nil {
		t.Fatalf("Huella: %v", err)
	}

	var status int
	var errGet error
	hecho := make(chan struct{})
	go func() {
		defer close(hecho)
		resp, err := clientePineado(huella).Get("https://" + rc.Direccion() + "/v1/lento")
		errGet = err
		if resp != nil {
			status = resp.StatusCode
			resp.Body.Close()
		}
	}()

	<-arranco
	if err := rc.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	<-hecho
	if errGet != nil || status != http.StatusOK {
		t.Fatalf("la request en vuelo debía completar 200 pese a Stop: err=%v status=%d", errGet, status)
	}
	if _, err := net.DialTimeout("tcp", rc.Direccion(), 500*time.Millisecond); err == nil {
		t.Fatalf("tras Stop, nuevas conexiones debían rechazarse")
	}
}

// esperarApagado sondea hasta que el receptor se apaga (poll corto, el
// reloj es falso: nunca se duerme el lapso real).
func esperarApagado(rc *Receptor) bool {
	for i := 0; i < 100; i++ {
		if !rc.Activo() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func receptorConRelojFalso(t *testing.T) (*Receptor, *relojFalso) {
	t.Helper()
	conCarpetaLANTemporal(t)
	reloj := &relojFalso{ahora: time.Now()}
	rc := NuevoReceptor()
	rc.Puerto = 0
	rc.Reloj = reloj
	rc.IntervaloRevision = 10 * time.Millisecond
	if err := rc.Start(ifazLoopback("127.0.0.1/32")); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { rc.Stop(context.Background()) })
	return rc, reloj
}

func TestReceptorSinUsoApagaA10Min(t *testing.T) {
	rc, reloj := receptorConRelojFalso(t)
	reloj.avanzar(VentanaSinUso - time.Minute)
	time.Sleep(60 * time.Millisecond)
	if !rc.Activo() {
		t.Fatalf("no debía apagarse antes de VentanaSinUso")
	}
	reloj.avanzar(2 * time.Minute)
	if !esperarApagado(rc) {
		t.Fatalf("debía apagarse tras VentanaSinUso sin requests exitosas")
	}
}

func TestReceptorTrasUsoApagaA2Min(t *testing.T) {
	rc, reloj := receptorConRelojFalso(t)
	huella, _ := rc.Huella()
	resp, err := clientePineado(huella).Get("https://" + rc.Direccion() + "/v1/salud")
	if err != nil {
		t.Fatalf("GET salud: %v", err)
	}
	resp.Body.Close()

	reloj.avanzar(VentanaTrasUso - 30*time.Second)
	time.Sleep(60 * time.Millisecond)
	if !rc.Activo() {
		t.Fatalf("no debía apagarse antes de VentanaTrasUso")
	}
	reloj.avanzar(time.Minute)
	if !esperarApagado(rc) {
		t.Fatalf("debía apagarse tras VentanaTrasUso sin requests exitosas")
	}
}

func TestReceptorErroresNoExtiendenVentana(t *testing.T) {
	rc, reloj := receptorConRelojFalso(t)
	huella, _ := rc.Huella()
	reloj.avanzar(VentanaSinUso - time.Minute)
	// 404 (ruta inexistente) es error: no cuenta como actividad.
	resp, err := clientePineado(huella).Get("https://" + rc.Direccion() + "/v1/no-existe")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatalf("se esperaba error, status=%d", resp.StatusCode)
	}
	reloj.avanzar(2 * time.Minute)
	if !esperarApagado(rc) {
		t.Fatalf("una respuesta de error no debía extender la ventana")
	}
}

// La estabilidad de la huella entre arranques (misma clave persistida)
// se prueba directo sobre certificadoParaIPs en cert_test.go — Receptor
// solo delega en esa misma función, no hay lógica propia que retestear
// acá.
