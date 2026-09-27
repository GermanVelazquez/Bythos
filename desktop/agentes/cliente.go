// Package agentes — el puente MCP para que un agente de IA (Claude Code,
// OpenCode, Codex, Gemini CLI...) maneje Bythos por su cuenta.
//
// ¿Por qué "agentes" y no "mcp"? Porque el SDK oficial YA se llama paquete
// "mcp" (github.com/modelcontextprotocol/go-sdk/mcp). Si este paquete se
// llamara igual, cada archivo tendría que aliasar uno de los dos imports.
// Nombre distinto = cero ambigüedad.
//
// Regla de oro de todo el paquete: este proceso NO abre bythos.db.
// Habla con la app real por HTTP en http://localhost:8080, exactamente
// como la ventana y la extensión (ver api/server.go). Si la app no está
// abierta, las herramientas devuelven un error en español explicando eso,
// no un panic ni un stacktrace en inglés.
package agentes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// mensajeAppCerrada es el error que ve el agente cuando Bythos no está
// corriendo (conexión rechazada, DNS, timeout...). Un solo texto en
// español, reusado por toda petición fallida a nivel de transporte.
const mensajeAppCerrada = "Bythos no está abierto: abrí la app y volvé a intentar"

// claveActor es la key del context.Context donde viaja el nombre del
// cliente MCP conectado (ver conActor/actorDelContexto), para que
// peticion() lo mande como X-Bythos-Actor sin que cada tool tenga que
// pasarlo a mano en cada llamada.
type claveActor struct{}

// conActor agrega el actor (clientInfo.Name del agente conectado, ver
// actorDeClientInfo en escritura.go) al contexto de una llamada de
// escritura, para que el historial de Bythos (db/eventos.go) sepa QUIÉN
// hizo el cambio, no solo que "un agente" lo hizo.
func conActor(ctx context.Context, actor string) context.Context {
	return context.WithValue(ctx, claveActor{}, actor)
}

func actorDelContexto(ctx context.Context) string {
	actor, _ := ctx.Value(claveActor{}).(string)
	return actor
}

// Cliente es el puente HTTP delgado hacia Bythos.
// BaseURL y Host van SEPARADOS a propósito: BaseURL es a dónde se conecta
// (real: localhost:8080; en tests: el puerto que arma httptest.Server),
// y Host es la cabecera Host: que exige conGuardia en api/server.go.
// Sin esta separación, los tests no podrían pasar la guardia: el puerto
// que asigna httptest nunca es ":8080".
type Cliente struct {
	BaseURL string
	Host    string
	HTTP    *http.Client
}

// NuevoCliente apunta al Bythos real. Es lo que usa el servidor MCP en
// producción; los tests construyen su propio *Cliente con BaseURL/Host
// del httptest.Server.
func NuevoCliente() *Cliente {
	return &Cliente{
		BaseURL: "http://localhost:8080",
		Host:    "localhost:8080",
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

// peticion hace la llamada HTTP cruda: devuelve el cuerpo de la respuesta
// si status < 400, o un error en español si algo salió mal (red caída,
// o la API respondió con {"error": "..."}).
func (c *Cliente) peticion(ctx context.Context, metodo, ruta string, cuerpo any) ([]byte, error) {
	var lector io.Reader
	if cuerpo != nil {
		datos, err := json.Marshal(cuerpo)
		if err != nil {
			return nil, fmt.Errorf("no se pudo armar la petición: %w", err)
		}
		lector = bytes.NewReader(datos)
	}

	req, err := http.NewRequestWithContext(ctx, metodo, c.BaseURL+ruta, lector)
	if err != nil {
		return nil, fmt.Errorf("no se pudo armar la petición: %w", err)
	}
	if cuerpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// Toda petición de este paquete es de un agente de IA (ver el
	// comentario del paquete arriba): el historial de Bythos (ver
	// db/eventos.go y api/origen.go) necesita saberlo para poder auditar
	// qué hizo el agente. El actor (nombre del cliente MCP conectado) es
	// opcional: sin él, la fila queda con origen "agente" y actor vacío.
	req.Header.Set("X-Bythos-Origen", "agente")
	if actor := actorDelContexto(ctx); actor != "" {
		req.Header.Set("X-Bythos-Actor", actor)
	}
	// El Host: es lo que conGuardia revisa (ver hostsPermitidos en
	// api/server.go); NO es la dirección real de conexión, que sigue
	// siendo BaseURL. Así los tests hablan con httptest.Server pero
	// mandan el Host: que la guardia acepta.
	if c.Host != "" {
		req.Host = c.Host
	}

	resp, err := c.http().Do(req)
	if err != nil {
		// Cualquier error de transporte (puerto cerrado, DNS, timeout)
		// significa lo mismo para el agente: la app no está corriendo.
		return nil, errors.New(mensajeAppCerrada)
	}
	defer resp.Body.Close()

	datos, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer la respuesta de Bythos: %w", err)
	}

	if resp.StatusCode >= 400 {
		var carga struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(datos, &carga) == nil && carga.Error != "" {
			return nil, errors.New(carga.Error)
		}
		return nil, fmt.Errorf("Bythos respondió con error %d", resp.StatusCode)
	}
	return datos, nil
}

func (c *Cliente) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// obtenerJSON hace un GET y decodifica el JSON de respuesta en salida.
func (c *Cliente) obtenerJSON(ctx context.Context, ruta string, salida any) error {
	datos, err := c.peticion(ctx, http.MethodGet, ruta, nil)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(datos)) == 0 {
		return nil
	}
	return json.Unmarshal(datos, salida)
}

// enviarJSON manda cuerpo como JSON (POST/PATCH/PUT) y decodifica la
// respuesta en salida. salida puede ser nil si no importa la respuesta.
func (c *Cliente) enviarJSON(ctx context.Context, metodo, ruta string, cuerpo, salida any) error {
	datos, err := c.peticion(ctx, metodo, ruta, cuerpo)
	if err != nil {
		return err
	}
	if salida == nil || len(bytes.TrimSpace(datos)) == 0 {
		return nil
	}
	return json.Unmarshal(datos, salida)
}

// obtenerTexto hace un GET y devuelve el cuerpo tal cual (para /export,
// que responde text/markdown y no JSON).
func (c *Cliente) obtenerTexto(ctx context.Context, ruta string) (string, error) {
	datos, err := c.peticion(ctx, http.MethodGet, ruta, nil)
	if err != nil {
		return "", err
	}
	return string(datos), nil
}
