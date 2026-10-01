package lan

// emparejar_test.go — Emparejamiento, SAS, token y middleware Bearer:
// TTL del código, un solo uso, bloqueo por intentos malos, aprobar/rechazar,
// token entregado una vez y dispositivo revocado -> 401.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bythos-desktop/db"
)

func baseEmparejar(t *testing.T) *sql.DB {
	t.Helper()
	base, err := db.Abrir(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("db.Abrir: %v", err)
	}
	t.Cleanup(func() { base.Close() })
	return base
}

func nuevoEmparejadorTest(t *testing.T) (*Emparejador, *relojFalso) {
	t.Helper()
	reloj := &relojFalso{ahora: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	return NuevoEmparejador(baseEmparejar(t), reloj), reloj
}

func TestEmparejarCodigoValidoExpiradoYReusado(t *testing.T) {
	e, reloj := nuevoEmparejadorTest(t)
	codigo, _ := e.Codigo()
	if len(codigo) != 22 {
		t.Fatalf("código de %d chars, quería 22 (16 bytes base64url)", len(codigo))
	}
	id, sas, err := e.Solicitar(codigo, "Pixel")
	if err != "" || id == "" || sas != SAS(codigo, id) {
		t.Fatalf("solicitud válida falló: id=%q sas=%q err=%q", id, sas, err)
	}
	if _, _, err := e.Solicitar(codigo, "Otro"); err != ErrCodigoInvalido {
		t.Fatalf("código reusado: err=%q, quería %q", err, ErrCodigoInvalido)
	}

	vigente, _ := e.Codigo()
	reloj.avanzar(TTLCodigo)
	if _, _, err := e.Solicitar(vigente, "Tarde"); err != ErrCodigoExpirado {
		t.Fatalf("código vencido: err=%q, quería %q", err, ErrCodigoExpirado)
	}
}

func TestEmparejarBloqueaTrasCincoMalos(t *testing.T) {
	e, reloj := nuevoEmparejadorTest(t)
	original, _ := e.Codigo()
	for i := 0; i < MaxCodigosMalos-1; i++ {
		if _, _, err := e.Solicitar("mal", "x"); err != ErrCodigoInvalido {
			t.Fatalf("intento %d: err=%q", i, err)
		}
	}
	if _, _, err := e.Solicitar("mal", "x"); err != ErrBloqueado {
		t.Fatalf("5.º intento malo: err=%q, quería %q", err, ErrBloqueado)
	}
	if rotado, _ := e.Codigo(); rotado == original {
		t.Fatal("el código debía rotar tras el bloqueo")
	}
	nuevo, _ := e.Codigo()
	if _, _, err := e.Solicitar(nuevo, "x"); err != ErrBloqueado {
		t.Fatalf("durante el bloqueo ni el código bueno pasa: err=%q", err)
	}
	reloj.avanzar(TiempoBloqueo)
	if _, _, err := e.Solicitar(nuevo, "x"); err != "" {
		t.Fatalf("pasado el bloqueo debía aceptar: err=%q", err)
	}
}

func TestEmparejarMaxTresPendientes(t *testing.T) {
	e, _ := nuevoEmparejadorTest(t)
	for i := 0; i < MaxSolicitudes; i++ {
		c, _ := e.Codigo()
		if _, _, err := e.Solicitar(c, "d"); err != "" {
			t.Fatalf("solicitud %d: err=%q", i, err)
		}
	}
	c, _ := e.Codigo()
	if _, _, err := e.Solicitar(c, "d"); err != ErrBloqueado {
		t.Fatalf("4.ª pendiente: err=%q, quería %q", err, ErrBloqueado)
	}
}

func TestEmparejarSASDeterministaDe6Digitos(t *testing.T) {
	a, b := SAS("codigo", "req-1"), SAS("codigo", "req-1")
	if a != b || len(a) != 6 || a == SAS("codigo", "req-2") {
		t.Fatalf("SAS no determinista o mal formado: %q %q", a, b)
	}
}

// Aprobar emite el token una vez por HTTP; rechazar nunca; revocado -> 401.
func TestEmparejarFlujoHTTPTokenUnaVezYRevocado(t *testing.T) {
	e, _ := nuevoEmparejadorTest(t)
	mux := http.NewServeMux()
	e.Registrar(mux)
	mux.Handle("GET /v1/protegida", ConToken(e.base, func(w http.ResponseWriter, _ *http.Request, d db.Dispositivo) {
		w.Write([]byte(d.Nombre))
	}))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	pedir := func(cuerpo string) (int, map[string]any) {
		resp, err := http.Post(srv.URL+"/v1/emparejar", "application/json", strings.NewReader(cuerpo))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		return resp.StatusCode, m
	}
	estado := func(id string) map[string]any {
		resp, err := http.Get(srv.URL + "/v1/emparejar/" + id)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		return m
	}

	codigo, _ := e.Codigo()
	status, m := pedir(`{"codigo":"` + codigo + `","nombre":"Pixel de Ana"}`)
	if status != http.StatusAccepted || m["sas"] == "" {
		t.Fatalf("POST emparejar: %d %v", status, m)
	}
	id := m["solicitud"].(string)
	if s := estado(id); s["estado"] != EstadoPendiente || s["token"] != nil {
		t.Fatalf("pendiente no debe traer token: %v", s)
	}

	d, err := e.Aprobar(id)
	if err != nil {
		t.Fatalf("Aprobar: %v", err)
	}
	s := estado(id)
	token, _ := s["token"].(string)
	if s["estado"] != EstadoAprobado || token == "" {
		t.Fatalf("aprobado debe traer token: %v", s)
	}
	if again := estado(id); again["token"] != nil {
		t.Fatalf("el token se entrega UNA vez: %v", again)
	}
	if d.TokenHash == token || d.TokenHash != HashToken(token) {
		t.Fatal("la base debe guardar solo el hash del token")
	}

	llamar := func(auth string) int {
		req, _ := http.NewRequest("GET", srv.URL+"/v1/protegida", nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if c := llamar("Bearer " + token); c != http.StatusOK {
		t.Fatalf("token válido: %d", c)
	}
	if c := llamar(""); c != http.StatusUnauthorized {
		t.Fatalf("sin token: %d", c)
	}
	if c := llamar("Bearer desconocido"); c != http.StatusUnauthorized {
		t.Fatalf("token desconocido: %d", c)
	}
	if err := db.RevocarDispositivo(e.base, d.ID); err != nil {
		t.Fatal(err)
	}
	if c := llamar("Bearer " + token); c != http.StatusUnauthorized {
		t.Fatalf("token revocado: %d, quería 401", c)
	}
}

func TestEmparejarRechazadaNoEmiteToken(t *testing.T) {
	e, _ := nuevoEmparejadorTest(t)
	c, _ := e.Codigo()
	id, _, _ := e.Solicitar(c, "Intruso")
	if err := e.Rechazar(id); err != nil {
		t.Fatalf("Rechazar: %v", err)
	}
	if _, err := e.Aprobar(id); err != ErrSolicitudNoEncontrada {
		t.Fatalf("aprobar una rechazada: err=%v", err)
	}
	if lista, _ := db.ListarDispositivos(e.base); len(lista) != 0 {
		t.Fatalf("rechazar no debe crear dispositivo: %+v", lista)
	}
	if len(e.Pendientes()) != 0 {
		t.Fatal("una rechazada no debe seguir pendiente")
	}
}

func TestEmparejarQR(t *testing.T) {
	texto := QRTexto("192.168.1.5", 48080, "HUELLA", "COD", "Mi PC")
	if texto != "bythos://pair?v=1&h=192.168.1.5&p=48080&k=HUELLA&c=COD&n=Mi+PC" {
		t.Fatalf("texto QR inesperado: %s", texto)
	}
	uri, err := QRDataURI(texto)
	if err != nil || !strings.HasPrefix(uri, "data:image/png;base64,") {
		t.Fatalf("QRDataURI: %q err=%v", uri, err)
	}
}
