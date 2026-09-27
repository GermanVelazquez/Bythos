package api

// metadata_ssrf_test.go — Guard SSRF de metadata.go: ningún link de usuario
// puede hacer que Bythos hable con la red interna de su propia PC.
// direccionesPermitidasTest es el ÚNICO hook: vacío = producción real
// (bloquea TODO lo local), y cada test que lo usa lo limpia con t.Cleanup.

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"bythos-desktop/db"
)

// nuevoServidorQueCuenta arma un httptest.Server que cuenta cuántas veces
// lo golpearon. Sirve para probar un NEGATIVO real: no alcanza con que
// obtenerMetadata devuelva el fallback, hay que ver que la red ni se tocó.
func nuevoServidorQueCuenta(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write([]byte("<html><head><title>no debería llegar</title></head></html>"))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func puertoDe(t *testing.T, urlServidor string) string {
	t.Helper()
	_, puerto, err := net.SplitHostPort(strings.TrimPrefix(urlServidor, "http://"))
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	return puerto
}

// TestMetadataSSRFBloqueaRedesInternas cubre el catálogo del guard: loopback
// (127.0.0.1, localhost), metadata endpoint de nube, RFC1918 y esquemas que
// ni deberían intentar red (file://, ftp://). El hook permanece OFF (mapa
// vacío, comportamiento real de producción) en todo este test.
func TestMetadataSSRFBloqueaRedesInternas(t *testing.T) {
	srv, hits := nuevoServidorQueCuenta(t)
	puerto := puertoDe(t, srv.URL)

	casos := []struct {
		nombre string
		link   string
	}{
		{"127.0.0.1 directo", srv.URL},
		{"localhost", "http://localhost:" + puerto},
		{"metadata endpoint de nube", "http://169.254.169.254/"},
		{"RFC1918 10.0.0.1", "http://10.0.0.1/"},
		{"esquema file", "file:///C:/Windows/win.ini"},
		{"esquema ftp", "ftp://127.0.0.1/algo"},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			m := obtenerMetadata(tt.link)
			if m.Titulo != tt.link || m.Tipo != "otro" {
				t.Fatalf("debía caer al fallback digno (sin red), salió %+v", m)
			}
		})
	}
	if got := atomic.LoadInt32(hits); got != 0 {
		t.Fatalf("el guard debía bloquear TODO intento a la red local, pero el server recibió %d hits", got)
	}
}

// TestMetadataSSRFPermiteDireccionAutorizada prueba el hook en sentido
// positivo: con la IP:puerto exacta del test en el allowlist, el fetch
// funciona normal (el guard no rompe el camino feliz).
func TestMetadataSSRFPermiteDireccionAutorizada(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><title>Hola Bythos</title></head></html>`))
	}))
	t.Cleanup(srv.Close)
	direccion := "127.0.0.1:" + puertoDe(t, srv.URL)
	direccionesPermitidasTest[direccion] = true
	t.Cleanup(func() { delete(direccionesPermitidasTest, direccion) })

	m := porHTML(srv.URL)
	if m.Titulo != "Hola Bythos" {
		t.Fatalf("con la dirección autorizada, el fetch debía funcionar normal, salió %+v", m)
	}
}

// TestMetadataSSRFRedirectAOtroLocalBloqueado es el caso fino: el primer
// salto (servidor "legítimo") está autorizado por su dirección EXACTA, pero
// redirige a otro 127.0.0.1 distinto (el "atacante") que NO está en el
// allowlist. El guard corre en el Dial de CADA salto, así que debe bloquear
// el segundo aunque el primero haya pasado.
func TestMetadataSSRFRedirectAOtroLocalBloqueado(t *testing.T) {
	atacante, hitsAtacante := nuevoServidorQueCuenta(t)

	legitimo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, atacante.URL, http.StatusFound)
	}))
	t.Cleanup(legitimo.Close)

	direccion := "127.0.0.1:" + puertoDe(t, legitimo.URL)
	direccionesPermitidasTest[direccion] = true
	t.Cleanup(func() { delete(direccionesPermitidasTest, direccion) })

	m := porHTML(legitimo.URL)
	if m.Titulo != "" {
		t.Fatalf("la redirección al atacante debía fallar bloqueada, salió %+v", m)
	}
	if got := atomic.LoadInt32(hitsAtacante); got != 0 {
		t.Fatalf("el atacante NO debía recibir el hit tras la redirección, recibió %d", got)
	}
}

// TestMetadataSSRFRecursoIgualSeGuarda confirma la UX pedida: aunque el
// olfato no pueda (o no deba) traer metadata, el recurso se guarda igual
// con lo mínimo (la propia URL como título).
func TestMetadataSSRFRecursoIgualSeGuarda(t *testing.T) {
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	body := `{"carpeta_id":` + itoa(c.ID) + `,"url":"http://169.254.169.254/"}`
	req := httptest.NewRequest(http.MethodPost, "/api/recursos", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.guardarRecurso(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s, quería 201 (se guarda igual aunque falle metadata)", rec.Code, rec.Body.String())
	}
	var creado db.Recurso
	if err := json.Unmarshal(rec.Body.Bytes(), &creado); err != nil {
		t.Fatalf("respuesta mal: %v", err)
	}
	if creado.URL != "http://169.254.169.254/" || creado.Tipo != "otro" {
		t.Fatalf("recurso mal guardado: %+v", creado)
	}
}
