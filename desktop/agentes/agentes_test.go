package agentes

// agentes_test.go — helpers compartidos por los tests del paquete:
// un Bythos real (httptest.Server + SQLite temporal) y una sesión MCP
// conectada en memoria, para probar las tools de punta a punta como las
// usaría un agente real (initialize -> tools/call), sin mockear nada.

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"bythos-desktop/api"
	"bythos-desktop/db"
)

// bythosDePrueba levanta un api.Servidor real sobre una SQLite temporal,
// detrás de un httptest.Server. Devuelve un *Cliente que le manda el
// Host: localhost:8080 que exige conGuardia (ver api/server.go), aunque
// httptest asigne otro puerto, y la *sql.DB subyacente para plantar datos
// de semilla directo (más simple que ir tool por tool para armar el
// escenario de un test de lectura).
func bythosDePrueba(t *testing.T) (*Cliente, *sql.DB) {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), "bythos.db")
	base, err := db.Abrir(ruta)
	if err != nil {
		t.Fatalf("db.Abrir: %v", err)
	}
	t.Cleanup(func() { base.Close() })

	srv := httptest.NewServer((&api.Servidor{Base: base}).Rutas())
	t.Cleanup(srv.Close)

	c := &Cliente{BaseURL: srv.URL, Host: "localhost:8080", HTTP: srv.Client()}
	return c, base
}

// sesionDePrueba conecta un mcp.Server (con las tools de c) y un
// mcp.Client sobre transportes en memoria, e inicializa la sesión.
// Devuelve la ClientSession lista para CallTool.
func sesionDePrueba(t *testing.T, c *Cliente) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	servidor := NuevoServidor(c, "test")

	ct, st := mcp.NewInMemoryTransports()
	if _, err := servidor.Connect(ctx, st, nil); err != nil {
		t.Fatalf("servidor.Connect: %v", err)
	}

	cliente := mcp.NewClient(&mcp.Implementation{Name: "test-agent", Version: "0.0.0"}, nil)
	sesion, err := cliente.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("cliente.Connect: %v", err)
	}
	t.Cleanup(func() { sesion.Close() })
	return sesion
}

// llamar invoca una tool y devuelve su CallToolResult ya decodificado:
// si IsError, el texto del error (para assertar el mensaje en español);
// si no, el StructuredContent volcado en salida (si salida no es nil).
func llamar(t *testing.T, sesion *mcp.ClientSession, nombre string, args map[string]any, salida any) (isError bool, textoError string) {
	t.Helper()
	res, err := sesion.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      nombre,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s): error de protocolo inesperado: %v", nombre, err)
	}
	if res.IsError {
		return true, textoDe(res)
	}
	if salida != nil {
		datos, err := json.Marshal(res.StructuredContent)
		if err != nil {
			t.Fatalf("CallTool(%s): no se pudo re-serializar StructuredContent: %v", nombre, err)
		}
		if err := json.Unmarshal(datos, salida); err != nil {
			t.Fatalf("CallTool(%s): no se pudo decodificar salida: %v", nombre, err)
		}
	}
	return false, ""
}

// crearCarpetaSemilla y guardarRecursoSemilla plantan datos directo por
// db/ (evitan pasar por una tool cuando el test solo necesita un
// escenario previo, no está probando esa tool en particular).
func crearCarpetaSemilla(t *testing.T, base *sql.DB, nombre string) int64 {
	t.Helper()
	c, err := db.CrearCarpeta(base, nombre)
	if err != nil {
		t.Fatalf("semilla CrearCarpeta: %v", err)
	}
	return c.ID
}

func guardarRecursoSemilla(t *testing.T, base *sql.DB, carpetaID int64, url string) int64 {
	t.Helper()
	r, err := db.Guardar(base, carpetaID, url, "Título", "", "", "articulo")
	if err != nil {
		t.Fatalf("semilla Guardar: %v", err)
	}
	return r.ID
}

func textoDe(res *mcp.CallToolResult) string {
	if len(res.Content) == 0 {
		return ""
	}
	if tc, ok := res.Content[0].(*mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}
