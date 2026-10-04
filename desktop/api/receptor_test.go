package api

// receptor_test.go — API de control del receptor LAN: Origin EXACTO (403
// para cualquier otro), desactivar idempotente (camino keepalive del cierre
// de pantalla), activar con el mapeo de errores y revocar dispositivos.

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"bythos-desktop/db"
	"bythos-desktop/lan"
)

// receptorFalso cuenta Start/Stop sin abrir puertos TLS.
type receptorFalso struct {
	activo      bool
	errStart    error
	stops       int
	ultimaIface lan.Interfaz
}

func (f *receptorFalso) Start(i lan.Interfaz) error {
	if f.errStart != nil {
		return f.errStart
	}
	f.activo, f.ultimaIface = true, i
	return nil
}
func (f *receptorFalso) Stop(context.Context) error { f.stops++; f.activo = false; return nil }
func (f *receptorFalso) Activo() bool               { return f.activo }
func (f *receptorFalso) RedDesconocida() bool       { return false }
func (f *receptorFalso) Direccion() string          { return "192.168.1.10:48080" }
func (f *receptorFalso) Huella() (string, error)    { return "huella", nil }

func controlDePrueba(t *testing.T) (*ControlReceptor, *receptorFalso, http.Handler) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	s := basePrueba(t)
	f := &receptorFalso{}
	reloj := lan.NuevoReceptor().Reloj
	c := &ControlReceptor{
		s: s, rc: f,
		emp:     lan.NuevoEmparejador(s.Base, reloj),
		almacen: lan.NuevoAlmacenSubidas(reloj),
		interfaces: func() ([]lan.Interfaz, error) {
			ip := net.ParseIP("192.168.1.10").To4()
			return []lan.Interfaz{{IP: ip, Nombre: "Wi-Fi", Rango: &net.IPNet{IP: ip.Mask(net.CIDRMask(24, 32)), Mask: net.CIDRMask(24, 32)}}}, nil
		},
		nombrePC: func() (string, error) { return "PC", nil },
	}
	c.servicio = lan.NuevoServicio(s.Base, NuevoPuenteLAN(s), c.almacen)
	s.Receptor = c
	return c, f, s.Rutas()
}

func pedir(h http.Handler, metodo, ruta, origin, cuerpo string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
	r.Host = "localhost:8080"
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestReceptorControlOrigenExacto403(t *testing.T) {
	_, f, h := controlDePrueba(t)
	rutas := []struct{ metodo, ruta string }{
		{"POST", "/api/receptor/estado"}, {"POST", "/api/receptor/activar"},
		{"POST", "/api/receptor/desactivar"}, {"POST", "/api/receptor/codigo/renovar"},
		{"POST", "/api/receptor/solicitudes/x/aprobar"}, {"POST", "/api/receptor/solicitudes/x/rechazar"},
		{"DELETE", "/api/receptor/dispositivos/1"},
	}
	// Sin Origin, la extensión y un origen parecido: todos 403.
	for _, origin := range []string{"", "chrome-extension://abc", "http://localhost:8080.evil.com", "http://localhost:5173"} {
		for _, r := range rutas {
			if w := pedir(h, r.metodo, r.ruta, origin, ""); w.Code != http.StatusForbidden {
				t.Errorf("%s %s con Origin %q: %d, quería 403", r.metodo, r.ruta, origin, w.Code)
			}
		}
	}
	if f.activo || f.stops != 0 {
		t.Fatalf("un 403 no debe tocar el receptor: %+v", f)
	}
	if w := pedir(h, "POST", "/api/receptor/estado", "http://localhost:8080", ""); w.Code != http.StatusOK {
		t.Fatalf("Origin exacto: %d %s", w.Code, w.Body.String())
	}
}

func TestReceptorControlActivarYDesactivarIdempotente(t *testing.T) {
	_, f, h := controlDePrueba(t)
	const origen = "http://localhost:8080"

	w := pedir(h, "POST", "/api/receptor/activar", origen, "")
	if w.Code != 200 || !f.activo || f.ultimaIface.IP.String() != "192.168.1.10" {
		t.Fatalf("activar: %d %s %+v", w.Code, w.Body.String(), f)
	}
	var est map[string]any
	json.Unmarshal(w.Body.Bytes(), &est)
	if est["activo"] != true || est["ip"] != "192.168.1.10" || est["puerto"] != float64(48080) ||
		!strings.HasPrefix(est["qr_texto"].(string), "bythos://pair") || est["qr_png"] == "" {
		t.Fatalf("estado tras activar: %v", est)
	}

	// Camino keepalive de pagehide: sin body, dos veces, siempre ok.
	for i := 0; i < 2; i++ {
		if w := pedir(h, "POST", "/api/receptor/desactivar", origen, ""); w.Code != 200 || f.activo {
			t.Fatalf("desactivar #%d: %d activo=%v", i, w.Code, f.activo)
		}
	}
	if f.stops != 2 {
		t.Fatalf("Stop llamado %d veces, quería 2 (idempotente en lan)", f.stops)
	}
}

func TestReceptorControlActivarMapeaErrores(t *testing.T) {
	_, f, h := controlDePrueba(t)
	const origen = "http://localhost:8080"
	casos := []struct {
		err    error
		status int
		codigo string
	}{
		{lan.ErrRedNoPrivada, 503, "red_no_privada"},
		{lan.ErrPuertoOcupado, 409, "puerto_ocupado"},
	}
	for _, c := range casos {
		f.errStart = c.err
		w := pedir(h, "POST", "/api/receptor/activar", origen, "")
		var cuerpo map[string]string
		json.Unmarshal(w.Body.Bytes(), &cuerpo)
		if w.Code != c.status || cuerpo["error"] != c.codigo {
			t.Errorf("%v: %d %v", c.err, w.Code, cuerpo)
		}
	}
	f.errStart = nil
	if w := pedir(h, "POST", "/api/receptor/activar", origen, `{"ip":"10.9.9.9"}`); w.Code != 409 {
		t.Fatalf("ip inexistente: %d, quería 409 sin_interfaz", w.Code)
	}
}

func TestReceptorControlRevocarCortaAlDispositivo(t *testing.T) {
	c, _, h := controlDePrueba(t)
	d, err := db.CrearDispositivo(c.s.Base, "Pixel de Ana", "hash-1")
	if err != nil {
		t.Fatal(err)
	}
	const origen = "http://localhost:8080"
	if w := pedir(h, "DELETE", "/api/receptor/dispositivos/abc", origen, ""); w.Code != 400 {
		t.Fatalf("id inválido: %d", w.Code)
	}
	if w := pedir(h, "DELETE", "/api/receptor/dispositivos/"+strconv.FormatInt(d.ID, 10), origen, ""); w.Code != 200 {
		t.Fatalf("revocar: %d %s", w.Code, w.Body.String())
	}
	if _, ok, _ := db.DispositivoPorTokenHash(c.s.Base, "hash-1"); ok {
		t.Fatal("el token del dispositivo revocado todavía resuelve")
	}
	w := pedir(h, "POST", "/api/receptor/estado", origen, "")
	var est struct {
		Dispositivos []any `json:"dispositivos"`
	}
	json.Unmarshal(w.Body.Bytes(), &est)
	if len(est.Dispositivos) != 0 {
		t.Fatalf("el estado no debe listar revocados: %v", est.Dispositivos)
	}
}

func TestReceptorControlSolicitudInexistente404(t *testing.T) {
	_, _, h := controlDePrueba(t)
	for _, accion := range []string{"aprobar", "rechazar"} {
		if w := pedir(h, "POST", "/api/receptor/solicitudes/nada/"+accion, "http://localhost:8080", ""); w.Code != 404 {
			t.Errorf("%s: %d, quería 404", accion, w.Code)
		}
	}
}
