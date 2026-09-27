package agentes

// escritura.go — Herramientas que CAMBIAN datos en Bythos: progreso,
// estado, nuevas carpetas, nuevos links, nuevas notas de agenda.
// Ninguna borra nada (a propósito: NO hay delete tools todavía) y
// ninguna es DestructiveHint (solo agregan o actualizan filas existentes).

import (
	"context"
	"fmt"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func boolPtr(b bool) *bool { return &b }

// actorDeClientInfo lee el nombre del cliente MCP conectado (clientInfo.name
// del initialize, ver ClientInfo() en el SDK) para que el historial de
// Bythos registre QUIÉN hizo el cambio, no solo "un agente". Sin nombre
// (cliente viejo, o simplemente no lo mandó), "agente" es el actor genérico.
func actorDeClientInfo(req *mcp.CallToolRequest) string {
	if req == nil {
		return "agente"
	}
	info := req.ClientInfo()
	if info == nil || info.Name == "" {
		return "agente"
	}
	return info.Name
}

func noDestructiva(idempotente bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    false,
		DestructiveHint: boolPtr(false),
		IdempotentHint:  idempotente,
	}
}

var estadosValidos = map[string]bool{
	"pendiente":  true,
	"en_curso":   true,
	"completado": true,
}

// --- actualizar_progreso ---

type entradaActualizarProgreso struct {
	RecursoID int64 `json:"recurso_id" jsonschema:"id del recurso a actualizar"`
	Progreso  int   `json:"progreso" jsonschema:"progreso de 0 a 100 (porcentaje estudiado); fuera de rango se rechaza"`
}

type salidaOK struct {
	OK bool `json:"ok" jsonschema:"true si el cambio se guardó"`
}

func registrarActualizarProgreso(s *mcp.Server, c *Cliente) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "actualizar_progreso",
		Description: "Actualiza el progreso (0 a 100) de un recurso. Bythos sincroniza " +
			"el estado automáticamente: 0 = pendiente, 100 = completado, 1-99 = en_curso. " +
			"Es idempotente: llamarla dos veces con el mismo valor deja el mismo resultado.",
		Annotations: noDestructiva(true),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in entradaActualizarProgreso) (*mcp.CallToolResult, salidaOK, error) {
		if in.Progreso < 0 || in.Progreso > 100 {
			return nil, salidaOK{}, fmt.Errorf("el progreso debe estar entre 0 y 100, llegó %d", in.Progreso)
		}
		ctx = conActor(ctx, actorDeClientInfo(req))
		body := map[string]any{"progreso": in.Progreso}
		if err := c.enviarJSON(ctx, "PATCH", "/api/recursos/"+strconv.FormatInt(in.RecursoID, 10), body, nil); err != nil {
			return nil, salidaOK{}, err
		}
		return nil, salidaOK{OK: true}, nil
	})
}

// --- cambiar_estado ---

type entradaCambiarEstado struct {
	RecursoID int64  `json:"recurso_id" jsonschema:"id del recurso"`
	Estado    string `json:"estado" jsonschema:"pendiente, en_curso o completado"`
}

func registrarCambiarEstado(s *mcp.Server, c *Cliente) {
	entrada, err := esquemaEnum[entradaCambiarEstado]("estado", "pendiente", "en_curso", "completado")
	if err != nil {
		panic(err)
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "cambiar_estado",
		Description: "Cambia el estado de un recurso a pendiente, en_curso o completado. " +
			"pendiente fuerza el progreso a 0 y completado lo fuerza a 100; en_curso " +
			"conserva el progreso actual. Es idempotente.",
		Annotations: noDestructiva(true),
		InputSchema: entrada,
	}, func(ctx context.Context, req *mcp.CallToolRequest, in entradaCambiarEstado) (*mcp.CallToolResult, salidaOK, error) {
		if !estadosValidos[in.Estado] {
			return nil, salidaOK{}, fmt.Errorf("estado inválido %q. Usá: pendiente, en_curso o completado", in.Estado)
		}
		ctx = conActor(ctx, actorDeClientInfo(req))
		body := map[string]any{"estado": in.Estado}
		if err := c.enviarJSON(ctx, "PATCH", "/api/recursos/"+strconv.FormatInt(in.RecursoID, 10), body, nil); err != nil {
			return nil, salidaOK{}, err
		}
		return nil, salidaOK{OK: true}, nil
	})
}

// --- crear_carpeta ---

type entradaCrearCarpeta struct {
	Nombre string `json:"nombre" jsonschema:"nombre de la carpeta/tema a crear, no puede estar vacío"`
}

func registrarCrearCarpeta(s *mcp.Server, c *Cliente) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "crear_carpeta",
		Description: "Crea una carpeta (tema de estudio) nueva en Bythos, ej. \"Go Backend\". No es idempotente: llamarla dos veces crea dos carpetas.",
		Annotations: noDestructiva(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in entradaCrearCarpeta) (*mcp.CallToolResult, CarpetaSalida, error) {
		ctx = conActor(ctx, actorDeClientInfo(req))
		body := map[string]any{"nombre": in.Nombre}
		var carpeta wireCarpeta
		if err := c.enviarJSON(ctx, "POST", "/api/carpetas", body, &carpeta); err != nil {
			return nil, CarpetaSalida{}, err
		}
		return nil, CarpetaSalida{ID: carpeta.ID, Nombre: carpeta.Nombre}, nil
	})
}

// --- guardar_link ---

type entradaGuardarLink struct {
	CarpetaID int64  `json:"carpeta_id" jsonschema:"id de la carpeta donde guardar el link"`
	URL       string `json:"url" jsonschema:"URL del recurso a guardar"`
}

func registrarGuardarLink(s *mcp.Server, c *Cliente) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "guardar_link",
		Description: "Guarda un link nuevo en una carpeta. Bythos completa automáticamente " +
			"título, imagen, descripción y tipo (youtube/articulo/otro) leyendo la URL; " +
			"no hace falta mandarlos. El recurso nace en estado pendiente con progreso 0. " +
			"No es idempotente: llamarla dos veces con la misma URL crea dos recursos.",
		Annotations: noDestructiva(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in entradaGuardarLink) (*mcp.CallToolResult, RecursoSalida, error) {
		ctx = conActor(ctx, actorDeClientInfo(req))
		body := map[string]any{"carpeta_id": in.CarpetaID, "url": in.URL}
		var recurso wireRecurso
		if err := c.enviarJSON(ctx, "POST", "/api/recursos", body, &recurso); err != nil {
			return nil, RecursoSalida{}, err
		}
		return nil, aRecursoSalida(recurso), nil
	})
}

// --- crear_nota_agenda ---

type entradaCrearNotaAgenda struct {
	Fecha      string `json:"fecha" jsonschema:"fecha YYYY-MM-DD de la nota (obligatoria, tiene que ser un día real)"`
	Texto      string `json:"texto,omitempty" jsonschema:"texto de la nota; obligatorio si no mandás carpeta_id"`
	HoraInicio string `json:"hora_inicio,omitempty" jsonschema:"hora de inicio HH:MM (opcional)"`
	HoraFin    string `json:"hora_fin,omitempty" jsonschema:"hora de fin HH:MM (opcional, requiere hora_inicio y debe ser posterior)"`
	CarpetaID  *int64 `json:"carpeta_id,omitempty" jsonschema:"id de una carpeta existente para asociar la nota (opcional)"`
}

func registrarCrearNotaAgenda(s *mcp.Server, c *Cliente) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "crear_nota_agenda",
		Description: "Crea una nota en la agenda (calendario) de Bythos en una fecha dada, " +
			"con hora opcional y carpeta opcional asociada. No es idempotente: llamarla " +
			"dos veces con los mismos datos crea dos notas.",
		Annotations: noDestructiva(false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, in entradaCrearNotaAgenda) (*mcp.CallToolResult, AgendaSalida, error) {
		ctx = conActor(ctx, actorDeClientInfo(req))
		body := map[string]any{
			"fecha":       in.Fecha,
			"texto":       in.Texto,
			"hora_inicio": in.HoraInicio,
			"hora_fin":    in.HoraFin,
			"carpeta_id":  in.CarpetaID,
		}
		var nota wireAgenda
		if err := c.enviarJSON(ctx, "POST", "/api/agenda", body, &nota); err != nil {
			return nil, AgendaSalida{}, err
		}
		return nil, aAgendaSalida(nota), nil
	})
}

// RegistrarEscritura suma todas las herramientas de escritura al servidor.
func RegistrarEscritura(s *mcp.Server, c *Cliente) {
	registrarActualizarProgreso(s, c)
	registrarCambiarEstado(s, c)
	registrarCrearCarpeta(s, c)
	registrarGuardarLink(s, c)
	registrarCrearNotaAgenda(s, c)
}
