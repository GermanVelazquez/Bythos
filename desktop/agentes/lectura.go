package agentes

// lectura.go — Herramientas de SOLO LECTURA: listar carpetas y recursos,
// leer un recurso puntual, ver el termómetro y la agenda/actividad,
// y exportar una carpeta para repasar. Todas anotadas ReadOnlyHint:true
// para que el agente sepa que puede llamarlas sin pedir permiso.

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func soloLectura() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{ReadOnlyHint: true}
}

// --- listar_carpetas ---

type salidaListarCarpetas struct {
	Carpetas []CarpetaConProgreso `json:"carpetas" jsonschema:"carpetas existentes, con su progreso"`
}

func (c *Cliente) listarCarpetasConProgreso(ctx context.Context) ([]CarpetaConProgreso, error) {
	var carpetas []wireCarpeta
	if err := c.obtenerJSON(ctx, "/api/carpetas", &carpetas); err != nil {
		return nil, err
	}
	out := make([]CarpetaConProgreso, 0, len(carpetas))
	for _, cp := range carpetas {
		var prog wireProgreso
		if err := c.obtenerJSON(ctx, "/api/carpetas/"+strconv.FormatInt(cp.ID, 10)+"/progreso", &prog); err != nil {
			return nil, err
		}
		out = append(out, CarpetaConProgreso{
			ID:          cp.ID,
			Nombre:      cp.Nombre,
			Total:       prog.Total,
			Completados: prog.Completados,
			EnCurso:     prog.EnCurso,
			Pendientes:  prog.Pendientes,
			Porcentaje:  prog.Porcentaje,
		})
	}
	return out, nil
}

func registrarListarCarpetas(s *mcp.Server, c *Cliente) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "listar_carpetas",
		Description: "Lista todas las carpetas (temas de estudio) de Bythos, con su " +
			"progreso: total de recursos, cuántos están completados/en curso/pendientes " +
			"y el porcentaje completado (0 a 100). Sin parámetros.",
		Annotations: soloLectura(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, salidaListarCarpetas, error) {
		carpetas, err := c.listarCarpetasConProgreso(ctx)
		if err != nil {
			return nil, salidaListarCarpetas{}, err
		}
		return nil, salidaListarCarpetas{Carpetas: carpetas}, nil
	})
}

// --- listar_recursos ---

type entradaListarRecursos struct {
	CarpetaID int64 `json:"carpeta_id,omitempty" jsonschema:"id de la carpeta a filtrar; 0 u omitido lista los recursos de TODAS las carpetas"`
}

type salidaListarRecursos struct {
	Recursos []RecursoSalida `json:"recursos" jsonschema:"recursos (links guardados) que cumplen el filtro"`
}

func registrarListarRecursos(s *mcp.Server, c *Cliente) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "listar_recursos",
		Description: "Lista los recursos (links) guardados en Bythos. Si mandás " +
			"carpeta_id, filtra solo esa carpeta; si lo omitís (o mandás 0), trae los " +
			"de todas las carpetas. Cada recurso trae su progreso (0 a 100) y estado " +
			"(pendiente, en_curso, completado).",
		Annotations: soloLectura(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in entradaListarRecursos) (*mcp.CallToolResult, salidaListarRecursos, error) {
		ruta := "/api/recursos"
		if in.CarpetaID > 0 {
			ruta += "?carpeta_id=" + strconv.FormatInt(in.CarpetaID, 10)
		}
		var recursos []wireRecurso
		if err := c.obtenerJSON(ctx, ruta, &recursos); err != nil {
			return nil, salidaListarRecursos{}, err
		}
		out := make([]RecursoSalida, 0, len(recursos))
		for _, r := range recursos {
			out = append(out, aRecursoSalida(r))
		}
		return nil, salidaListarRecursos{Recursos: out}, nil
	})
}

// --- leer_recurso ---

type entradaLeerRecurso struct {
	ID int64 `json:"id" jsonschema:"id del recurso a leer"`
}

func registrarLeerRecurso(s *mcp.Server, c *Cliente) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "leer_recurso",
		Description: "Trae el detalle completo de un recurso por su id: url, título, " +
			"tipo, descripción, imagen, progreso (0 a 100), estado y la carpeta que lo " +
			"contiene. Bythos no tiene un \"buscar por id\" propio; esta herramienta " +
			"filtra la lista completa, así que es más lenta que listar_recursos si ya " +
			"tenés el recurso a mano.",
		Annotations: soloLectura(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in entradaLeerRecurso) (*mcp.CallToolResult, RecursoDetalle, error) {
		var recursos []wireRecurso
		if err := c.obtenerJSON(ctx, "/api/recursos", &recursos); err != nil {
			return nil, RecursoDetalle{}, err
		}
		for _, r := range recursos {
			if r.ID != in.ID {
				continue
			}
			carpeta := ""
			var carpetas []wireCarpeta
			if err := c.obtenerJSON(ctx, "/api/carpetas", &carpetas); err == nil {
				for _, cp := range carpetas {
					if cp.ID == r.CarpetaID {
						carpeta = cp.Nombre
						break
					}
				}
			}
			return nil, RecursoDetalle{
				ID:          r.ID,
				URL:         r.URL,
				Titulo:      r.Titulo,
				Tipo:        r.Tipo,
				Descripcion: r.Descripcion,
				Imagen:      r.Imagen,
				Progreso:    r.Progreso,
				Estado:      r.Estado,
				CarpetaID:   r.CarpetaID,
				Carpeta:     carpeta,
			}, nil
		}
		return nil, RecursoDetalle{}, fmt.Errorf("no existe un recurso con id %d", in.ID)
	})
}

// --- ver_progreso_carpeta ---

type entradaCarpetaID struct {
	CarpetaID int64 `json:"carpeta_id" jsonschema:"id de la carpeta"`
}

func registrarVerProgresoCarpeta(s *mcp.Server, c *Cliente) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "ver_progreso_carpeta",
		Description: "Devuelve el termómetro de UNA carpeta: total de recursos, " +
			"cuántos completados/en curso/pendientes, porcentaje (0 a 100, " +
			"completados/total) y promedio de progreso (0 a 100) de todos sus recursos.",
		Annotations: soloLectura(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in entradaCarpetaID) (*mcp.CallToolResult, ProgresoSalida, error) {
		var prog wireProgreso
		if err := c.obtenerJSON(ctx, "/api/carpetas/"+strconv.FormatInt(in.CarpetaID, 10)+"/progreso", &prog); err != nil {
			return nil, ProgresoSalida{}, err
		}
		return nil, aProgresoSalida(prog), nil
	})
}

// --- ver_stats ---

func registrarVerStats(s *mcp.Server, c *Cliente) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "ver_stats",
		Description: "Devuelve el termómetro GENERAL de todo Bythos (todas las " +
			"carpetas juntas): total, completados/en_curso/pendientes, porcentaje " +
			"(0 a 100) y promedio de progreso (0 a 100). Sin parámetros.",
		Annotations: soloLectura(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, ProgresoSalida, error) {
		var prog wireProgreso
		if err := c.obtenerJSON(ctx, "/api/stats", &prog); err != nil {
			return nil, ProgresoSalida{}, err
		}
		return nil, aProgresoSalida(prog), nil
	})
}

// --- ver_agenda / ver_actividad: comparten el filtro desde/hasta ---

type entradaRangoFechas struct {
	Desde string `json:"desde,omitempty" jsonschema:"fecha inicial YYYY-MM-DD (inclusive); vacío = sin límite inferior"`
	Hasta string `json:"hasta,omitempty" jsonschema:"fecha final YYYY-MM-DD (inclusive); vacío = sin límite superior"`
}

func rangoFechasQuery(desde, hasta string) string {
	q := url.Values{}
	if desde != "" {
		q.Set("desde", desde)
	}
	if hasta != "" {
		q.Set("hasta", hasta)
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

type salidaVerAgenda struct {
	Notas []AgendaSalida `json:"notas" jsonschema:"notas de la agenda en el rango pedido"`
}

func registrarVerAgenda(s *mcp.Server, c *Cliente) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "ver_agenda",
		Description: "Lista las notas de la agenda (calendario) de Bythos, opcionalmente " +
			"filtradas por rango de fechas (YYYY-MM-DD, ambos límites inclusive). Sin " +
			"desde/hasta trae toda la agenda.",
		Annotations: soloLectura(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in entradaRangoFechas) (*mcp.CallToolResult, salidaVerAgenda, error) {
		var notas []wireAgenda
		if err := c.obtenerJSON(ctx, "/api/agenda"+rangoFechasQuery(in.Desde, in.Hasta), &notas); err != nil {
			return nil, salidaVerAgenda{}, err
		}
		out := make([]AgendaSalida, 0, len(notas))
		for _, n := range notas {
			out = append(out, aAgendaSalida(n))
		}
		return nil, salidaVerAgenda{Notas: out}, nil
	})
}

type salidaVerActividad struct {
	Actividad []ActividadSalida `json:"actividad" jsonschema:"puntos de historia: cuántas carpetas+recursos se crearon cada día"`
}

func registrarVerActividad(s *mcp.Server, c *Cliente) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "ver_actividad",
		Description: "Devuelve la actividad (historial de creación) de Bythos por día: " +
			"cuántas carpetas y recursos se crearon cada fecha, opcionalmente filtrado " +
			"por rango desde/hasta (YYYY-MM-DD, ambos inclusive). Sin desde/hasta trae " +
			"todo el historial.",
		Annotations: soloLectura(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in entradaRangoFechas) (*mcp.CallToolResult, salidaVerActividad, error) {
		var actividad []wireActividad
		if err := c.obtenerJSON(ctx, "/api/actividad"+rangoFechasQuery(in.Desde, in.Hasta), &actividad); err != nil {
			return nil, salidaVerActividad{}, err
		}
		out := make([]ActividadSalida, 0, len(actividad))
		for _, a := range actividad {
			out = append(out, aActividadSalida(a))
		}
		return nil, salidaVerActividad{Actividad: out}, nil
	})
}

// --- exportar_carpeta ---

var formatosExport = map[string]bool{
	"markdown":   true,
	"gemini":     true,
	"notebooklm": true,
	"drive":      true,
}

type entradaExportarCarpeta struct {
	CarpetaID int64  `json:"carpeta_id" jsonschema:"id de la carpeta a exportar"`
	Formato   string `json:"formato,omitempty" jsonschema:"markdown (default), gemini, notebooklm o drive"`
}

type salidaExportarCarpeta struct {
	Contenido string `json:"contenido" jsonschema:"texto exportado, listo para pegar en un chat de IA o guardar como .md"`
}

func registrarExportarCarpeta(s *mcp.Server, c *Cliente) {
	entrada, err := esquemaEnum[entradaExportarCarpeta]("formato", "markdown", "gemini", "notebooklm", "drive")
	if err != nil {
		panic(err)
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "exportar_carpeta",
		Description: "Exporta una carpeta como texto para repasar afuera de Bythos. " +
			"formato: \"markdown\" (lista lista para notas/Obsidian, default), " +
			"\"gemini\" (prompt para pegar en Gemini con resumen y preguntas), " +
			"\"notebooklm\" (guía de importación a NotebookLM), o " +
			"\"drive\" (mismo markdown, pensado para subir como archivo). " +
			"El texto incluye instrucciones para que una IA devuelva el avance " +
			"actualizado y lo importes de nuevo con actualizar_progreso.",
		Annotations: soloLectura(),
		InputSchema: entrada,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in entradaExportarCarpeta) (*mcp.CallToolResult, salidaExportarCarpeta, error) {
		formato := in.Formato
		if formato == "" {
			formato = "markdown"
		}
		if !formatosExport[formato] {
			return nil, salidaExportarCarpeta{}, fmt.Errorf("formato inválido %q. Usá: markdown, gemini, notebooklm o drive", formato)
		}
		q := url.Values{"format": {formato}}
		texto, err := c.obtenerTexto(ctx, "/api/carpetas/"+strconv.FormatInt(in.CarpetaID, 10)+"/export?"+q.Encode())
		if err != nil {
			return nil, salidaExportarCarpeta{}, err
		}
		return nil, salidaExportarCarpeta{Contenido: texto}, nil
	})
}

// RegistrarLectura suma todas las herramientas de solo lectura al servidor.
func RegistrarLectura(s *mcp.Server, c *Cliente) {
	registrarListarCarpetas(s, c)
	registrarListarRecursos(s, c)
	registrarLeerRecurso(s, c)
	registrarVerProgresoCarpeta(s, c)
	registrarVerStats(s, c)
	registrarVerAgenda(s, c)
	registrarVerActividad(s, c)
	registrarExportarCarpeta(s, c)
}
