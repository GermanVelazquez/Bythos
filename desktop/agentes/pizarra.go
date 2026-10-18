package agentes

// pizarra.go — Las herramientas de la pizarra del agente (Terminal):
// leer la escena guardada, dibujar con operaciones y capturar una imagen.
// La sesión se resuelve en la app (api/pizarra.go): acá solo se manda el
// argumento sesion_id y, aparte, la pista BYTHOS_SESION del entorno.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"bythos-desktop/pizarra"
	"bythos-desktop/workspace"
)

const rutaPizarra = "/api/terminal/pizarra"

// pistaEntorno es el id de la sesión que la terminal le pasó al agente.
func pistaEntorno() string { return os.Getenv("BYTHOS_SESION") }

type entradaPizarraSesion struct {
	SesionID *int64 `json:"sesion_id,omitempty" jsonschema:"id de la sesión de Terminal (opcional; sin él se usa la sesión activa)"`
}

type entradaPizarraDibujar struct {
	SesionID *int64       `json:"sesion_id,omitempty" jsonschema:"id de la sesión de Terminal (opcional; sin él se usa la sesión activa)"`
	Ops      []pizarra.Op `json:"ops" jsonschema:"operaciones en orden: agregar{elemento}, actualizar{id,cambios}, borrar{ids}, mover{ids,dx,dy}"`
}

// --- pizarra_leer ---

func registrarPizarraLeer(s *mcp.Server, c *Cliente) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "pizarra_leer",
		Description: "Lee la pizarra guardada de una sesión de Terminal: elementos, resumen por tipo, " +
			"versión y operaciones pendientes. Funciona aunque la pantalla Terminal esté cerrada.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in entradaPizarraSesion) (*mcp.CallToolResult, map[string]any, error) {
		ctx = conActor(ctx, actorDeClientInfo(req))
		q := url.Values{}
		if in.SesionID != nil {
			q.Set("sesion_id", strconv.FormatInt(*in.SesionID, 10))
		}
		if p := pistaEntorno(); p != "" {
			q.Set("pista", p)
		}
		var salida map[string]any
		if err := c.obtenerJSON(ctx, rutaPizarra+"?"+q.Encode(), &salida); err != nil {
			return nil, nil, err
		}
		return nil, salida, nil
	})
}

// --- pizarra_dibujar ---

func registrarPizarraDibujar(s *mcp.Server, c *Cliente) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "pizarra_dibujar",
		Description: "Dibuja en la pizarra de una sesión de Terminal con operaciones (agregar, actualizar, " +
			"borrar, mover). Puede borrar elementos: usala solo cuando el usuario lo pida. Requiere la " +
			"pantalla Terminal abierta; si la UI no confirma a tiempo, el estado queda pendiente. " +
			"En tipo text, alto fija el tamaño de letra (alto ≈ 1,25 × letra) y el ancho se calcula solo; " +
			"en line, arrow y freedraw, puntos son coordenadas relativas a x,y.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(true)},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in entradaPizarraDibujar) (*mcp.CallToolResult, map[string]any, error) {
		if err := pizarra.Validar(in.Ops); err != nil {
			return nil, nil, err
		}
		ctx = conActor(ctx, actorDeClientInfo(req))
		cuerpo := map[string]any{"ops": in.Ops}
		if in.SesionID != nil {
			cuerpo["sesion_id"] = *in.SesionID
		}
		if p := pistaEntorno(); p != "" {
			cuerpo["pista"] = p
		}
		var salida map[string]any
		if err := c.enviarJSON(ctx, http.MethodPost, rutaPizarra+"/ops", cuerpo, &salida); err != nil {
			return nil, nil, err
		}
		return nil, salida, nil
	})
}

// --- pizarra_captura ---

// registrarPizarraCaptura va por Server.AddTool crudo: el AddTool tipado
// siempre rellena StructuredContent y Claude Code descarta la imagen cuando
// está presente.
func registrarPizarraCaptura(s *mcp.Server, c *Cliente) {
	esquema, err := jsonschema.For[entradaPizarraSesion](nil)
	if err != nil {
		panic(err)
	}
	s.AddTool(&mcp.Tool{
		Name: "pizarra_captura",
		Description: "Captura la pizarra de una sesión de Terminal como imagen PNG (1280 px máx.) y la " +
			"guarda en un archivo cuya ruta se cita en el texto. Requiere la pantalla Terminal abierta.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		InputSchema: esquema,
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		res, err := capturarPizarra(ctx, c, req)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
			}, nil
		}
		return res, nil
	})
}

func capturarPizarra(ctx context.Context, c *Cliente, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var in entradaPizarraSesion
	if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &in); err != nil {
			return nil, fmt.Errorf("argumentos inválidos: %w", err)
		}
	}
	ctx = conActor(ctx, actorDeClientInfo(req))
	cuerpo := map[string]any{}
	if in.SesionID != nil {
		cuerpo["sesion_id"] = *in.SesionID
	}
	if p := pistaEntorno(); p != "" {
		cuerpo["pista"] = p
	}
	png, cab, err := c.hacer(ctx, c.httpLargo(), http.MethodPost, rutaPizarra+"/captura", cuerpo)
	if err != nil {
		return nil, err
	}
	sesionID := cab.Get("X-Bythos-Sesion")
	if sesionID == "" {
		sesionID = "sesion"
	}
	ruta, errArchivo := guardarCaptura(c, sesionID, png)
	texto := fmt.Sprintf("Captura de la pizarra de la sesión %s (resolución: %s).", sesionID, cab.Get("X-Bythos-Resolucion"))
	if errArchivo != nil {
		texto += " No se pudo guardar el archivo: " + errArchivo.Error()
	} else {
		texto += " Archivo: " + ruta
	}
	return &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.ImageContent{Data: png, MIMEType: "image/png"},
		&mcp.TextContent{Text: texto},
	}}, nil
}

// guardarCaptura escribe capturas/pizarra-<sesión>.png (nombre fijo, se pisa).
func guardarCaptura(c *Cliente, sesionID string, png []byte) (string, error) {
	if filepath.Base(sesionID) != sesionID {
		return "", errors.New("id de sesión inválido")
	}
	base := c.DirAgente
	if base == "" {
		base = workspace.RutaWorkspace()
	}
	dir := filepath.Join(base, "capturas")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	ruta := filepath.Join(dir, "pizarra-"+sesionID+".png")
	return ruta, os.WriteFile(ruta, png, 0644)
}

// RegistrarPizarra suma las tres herramientas de la pizarra al servidor.
func RegistrarPizarra(s *mcp.Server, c *Cliente) {
	registrarPizarraLeer(s, c)
	registrarPizarraDibujar(s, c)
	registrarPizarraCaptura(s, c)
}
