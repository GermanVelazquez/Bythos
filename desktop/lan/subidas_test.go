package lan

// subidas_test.go — Las rutas de subida y de links de punta a punta contra
// una Biblioteca falsa: crear/estado/partes/cancelar, 409/413/422, carpeta
// inexistente al crear y borrada antes de finalizar, hash completo, tipo
// rechazado, y links que validan la carpeta antes de pedir metadata.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"bythos-desktop/archivos"
	"bythos-desktop/db"
)

// bibFalsa registra qué se guardó; no toca disco ni red.
type bibFalsa struct {
	mu       sync.Mutex
	carpetas map[int64]bool
	archivos [][]byte
	links    int
	errTipo  bool
}

func (b *bibFalsa) ListarCarpetas() ([]Carpeta, error) { return []Carpeta{{ID: 1, Nombre: "Go"}}, nil }
func (b *bibFalsa) ExisteCarpeta(id int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.carpetas[id]
}
func (b *bibFalsa) GuardarLink(carpetaID int64, url, titulo, actor string) (int64, string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.links++
	return 7, "T:" + titulo, nil
}
func (b *bibFalsa) GuardarArchivo(carpetaID int64, r io.Reader, nombre, actor string) (int64, error) {
	datos, _ := io.ReadAll(r)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.errTipo {
		return 0, archivos.ErrTipoNoPermitido
	}
	b.archivos = append(b.archivos, datos)
	return 42, nil
}

type entornoLAN struct {
	t     *testing.T
	base  *sql.DB
	srv   *httptest.Server
	svc   *Servicio
	bib   *bibFalsa
	dir   string
	token string // del dispositivo "Pixel"
}

func nuevoEntornoLAN(t *testing.T) *entornoLAN {
	t.Helper()
	base := baseEmparejar(t)
	bib := &bibFalsa{carpetas: map[int64]bool{1: true}}
	dir := filepath.Join(t.TempDir(), "subidas")
	svc := NuevoServicio(base, bib, nuevoAlmacenEn(dir, &relojFalso{ahora: time.Now()}))
	mux := http.NewServeMux()
	svc.Registrar(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	e := &entornoLAN{t: t, base: base, srv: srv, svc: svc, bib: bib, dir: dir}
	e.token = e.dispositivo("Pixel")
	return e
}

func (e *entornoLAN) dispositivo(nombre string) string {
	token, hash, _ := GenerarToken()
	if _, err := db.CrearDispositivo(e.base, nombre, hash); err != nil {
		e.t.Fatal(err)
	}
	return token
}

func (e *entornoLAN) post(token, ruta string, cab map[string]string, cuerpo []byte) (int, map[string]any) {
	e.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+ruta, bytes.NewReader(cuerpo))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range cab {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	json.NewDecoder(resp.Body).Decode(&m)
	return resp.StatusCode, m
}

// crear crea una subida de ese contenido y devuelve su id.
func (e *entornoLAN) crear(cliente string, datos []byte) string {
	e.t.Helper()
	cuerpo := fmt.Sprintf(`{"cliente_id":%q,"carpeta_id":1,"nombre":"a.txt","tamano":%d,"sha256":%q}`, cliente, len(datos), hexDe(datos))
	status, m := e.post(e.token, "/v1/subidas", nil, []byte(cuerpo))
	if status != http.StatusCreated {
		e.t.Fatalf("crear: status %d %v", status, m)
	}
	return m["id"].(string)
}

func (e *entornoLAN) parte(id string, offset int, datos []byte) (int, map[string]any) {
	return e.post(e.token, "/v1/subidas/"+id+"/partes",
		map[string]string{"X-Offset": fmt.Sprint(offset), "X-Chunk-SHA256": hexDe(datos)}, datos)
}

func (e *entornoLAN) estado(id string) map[string]any {
	_, m := e.post(e.token, "/v1/subidas/"+id+"/estado", nil, nil)
	return m
}

func TestSubidasFlujoCompletoYEstado(t *testing.T) {
	e := nuevoEntornoLAN(t)
	datos := []byte("0123456789abcdefghij")
	id := e.crear("c1", datos)

	if status, m := e.parte(id, 0, datos[:8]); status != 200 || m["offset"] != float64(8) {
		t.Fatalf("parte 1: %d %v", status, m)
	}
	if m := e.estado(id); m["estado"] != "recibiendo" || m["offset"] != float64(8) {
		t.Fatalf("estado: %v", m)
	}
	if status, m := e.parte(id, 8, datos[8:]); status != 202 || m["estado"] != "verificando" {
		t.Fatalf("parte final: %d %v", status, m)
	}
	e.svc.Esperar()
	if m := e.estado(id); m["estado"] != "listo" || m["recurso_id"] != float64(42) || m["offset"] != float64(len(datos)) {
		t.Fatalf("estado final: %v", m)
	}
	if len(e.bib.archivos) != 1 || !bytes.Equal(e.bib.archivos[0], datos) {
		t.Fatalf("la biblioteca no recibió el archivo íntegro")
	}
	if len(e.svc.almacen.Listar()) != 1 { // el sidecar queda para consultar el resultado
		t.Fatalf("debía quedar el sidecar de la subida lista")
	}
}

func TestSubidasCrearErroresEIdempotencia(t *testing.T) {
	e := nuevoEntornoLAN(t)
	hash := hexDe([]byte("x"))
	crear := func(cliente string, carpeta int, tamano int64) (int, map[string]any) {
		return e.post(e.token, "/v1/subidas", nil, []byte(fmt.Sprintf(
			`{"cliente_id":%q,"carpeta_id":%d,"nombre":"a.txt","tamano":%d,"sha256":%q}`, cliente, carpeta, tamano, hash)))
	}
	if status, m := crear("c1", 99, 1); status != 404 || m["error"] != ErrCarpetaNoEncontrada {
		t.Fatalf("carpeta inexistente: %d %v", status, m)
	}
	if entradas, _ := os.ReadDir(e.dir); len(entradas) != 0 {
		t.Fatalf("carpeta inexistente no debía crear sidecar ni .parte: %v", entradas)
	}
	anterior := archivos.TamanoMaximo
	archivos.TamanoMaximo = 10
	t.Cleanup(func() { archivos.TamanoMaximo = anterior })
	if status, m := crear("c1", 1, 11); status != 413 || m["error"] != ErrTamanoExcedido {
		t.Fatalf("oversize: %d %v", status, m)
	}
	_, a := crear("c1", 1, 5)
	_, b := crear("c1", 1, 5)
	if a["id"] == nil || a["id"] != b["id"] {
		t.Fatalf("cliente_id repetido debía ser idempotente: %v vs %v", a, b)
	}
	crear("c2", 1, 5)
	crear("c3", 1, 5)
	if status, m := crear("c4", 1, 5); status != 429 || m["error"] != ErrDemasiadasSubidas {
		t.Fatalf("cuarta activa: %d %v", status, m)
	}
}

func TestSubidasPartesConflictos(t *testing.T) {
	e := nuevoEntornoLAN(t)
	datos := []byte("0123456789")
	id := e.crear("c1", datos)
	e.parte(id, 0, datos[:4])

	if status, m := e.parte(id, 0, datos[4:6]); status != 409 || m["error"] != ErrOffsetInvalido || m["offset"] != float64(4) {
		t.Fatalf("offset viejo: %d %v", status, m)
	}
	status, m := e.post(e.token, "/v1/subidas/"+id+"/partes",
		map[string]string{"X-Offset": "4", "X-Chunk-SHA256": hexDe([]byte("otra cosa"))}, datos[4:6])
	if status != 422 || m["error"] != ErrHashParteInvalido {
		t.Fatalf("hash malo: %d %v", status, m)
	}
	if m := e.estado(id); m["offset"] != float64(4) {
		t.Fatalf("tras 422 el offset debía seguir en 4: %v", m)
	}
	liberar, _ := e.svc.almacen.Bloquear(id)
	if status, m := e.parte(id, 4, datos[4:6]); status != 409 || m["error"] != ErrSubidaOcupada {
		t.Fatalf("ocupada: %d %v", status, m)
	}
	liberar()

	otro := e.dispositivo("Otro")
	if status, _ := e.post(otro, "/v1/subidas/"+id+"/estado", nil, nil); status != 404 {
		t.Fatalf("subida ajena: status %d, quería 404", status)
	}
	if status, m := e.parte(id, 4, append(datos[4:], 'x', 'y')); status != 413 || m["error"] != ErrTamanoExcedido {
		t.Fatalf("más de lo declarado: %d %v", status, m)
	}
	if status, _ := e.post(e.token, "/v1/subidas/"+id+"/estado", nil, nil); status != 404 {
		t.Fatalf("la subida excedida debía descartarse, status %d", status)
	}
}

func TestSubidasCancelar(t *testing.T) {
	e := nuevoEntornoLAN(t)
	id := e.crear("c1", []byte("abc"))
	if status, _ := e.post(e.token, "/v1/subidas/"+id+"/cancelar", nil, nil); status != http.StatusNoContent {
		t.Fatalf("cancelar: status %d", status)
	}
	if entradas, _ := os.ReadDir(e.dir); len(entradas) != 0 {
		t.Fatalf("cancelar debía borrar sidecar y .parte: %v", entradas)
	}
	if status, _ := e.post("", "/v1/subidas/"+id+"/estado", nil, nil); status != 401 {
		t.Fatalf("sin token: status %d, quería 401", status)
	}
}

func TestSubidasFinalizacionErrores(t *testing.T) {
	casos := []struct {
		nombre string
		ajusta func(e *entornoLAN, datos []byte) []byte // devuelve lo que se envía
		quiere string
	}{
		{"hash completo distinto", func(e *entornoLAN, d []byte) []byte { return []byte("ABCDEFGHIJ") }, errHashArchivo},
		{"carpeta borrada antes de finalizar", func(e *entornoLAN, d []byte) []byte { e.bib.carpetas[1] = false; return d }, ErrCarpetaNoEncontrada},
		{"tipo no permitido", func(e *entornoLAN, d []byte) []byte { e.bib.errTipo = true; return d }, ErrTipoNoPermitido},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			e := nuevoEntornoLAN(t)
			declarado := []byte("0123456789")
			id := e.crear("c1", declarado)
			enviado := c.ajusta(e, declarado)
			if status, _ := e.parte(id, 0, enviado); status != 202 {
				t.Fatalf("última parte: status %d", status)
			}
			e.svc.Esperar()
			if m := e.estado(id); m["estado"] != "error" || m["error"] != c.quiere {
				t.Fatalf("estado: %v, quería error=%s", m, c.quiere)
			}
			if len(e.bib.archivos) != 0 {
				t.Fatalf("no debía guardarse nada")
			}
			if info, err := os.Stat(filepath.Join(e.dir, id+".parte")); err != nil || info.Size() != 0 {
				t.Fatalf("el temporal debía quedar descartado (vacío): %v %v", info, err)
			}
		})
	}
}

func TestLinksValidaCarpetaAntesDeMetadata(t *testing.T) {
	e := nuevoEntornoLAN(t)
	enviar := func(cuerpo string) (int, map[string]any) {
		return e.post(e.token, "/v1/links", nil, []byte(cuerpo))
	}
	if status, m := enviar(`{"carpeta_id":99,"url":"https://example.com"}`); status != 404 || m["error"] != ErrCarpetaNoEncontrada {
		t.Fatalf("carpeta inexistente: %d %v", status, m)
	}
	if e.bib.links != 0 {
		t.Fatalf("GuardarLink (metadata) se llamó %d veces con carpeta inexistente", e.bib.links)
	}
	if status, _ := enviar(`{"carpeta_id":1,"url":"javascript:alert(1)"}`); status != 400 {
		t.Fatalf("esquema no http(s): status %d, quería 400", status)
	}
	if status, m := enviar(`{"carpeta_id":1,"url":"https://example.com","titulo":"Hola"}`); status != 201 || m["recurso_id"] != float64(7) || m["titulo"] != "T:Hola" {
		t.Fatalf("link válido: %d %v", status, m)
	}
	if e.bib.links != 1 {
		t.Fatalf("GuardarLink debía llamarse una vez, fue %d", e.bib.links)
	}
}
