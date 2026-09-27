package db

// eventos.go — El HISTORIAL. Quién tocó tus datos, cuándo y qué cambió.
// ¿Por qué tabla propia y no un archivo de log?
// Porque queremos filtrar por origen y limitar sin parsear texto: mismo
// trato que folders/resources/agenda, la BD es la última defensa y la
// única fuente de verdad (ver db.go).
//
// ¿Para qué sirve? Bythos ahora lo puede tocar un agente de IA por MCP
// (ver desktop/agentes/), no solo tú desde la UI o la extensión. Esta
// tabla es cómo auditás qué hizo el agente sin tener que confiar a ciegas.

import (
	"database/sql"
	"strings"
)

// Orígenes válidos de un evento: quién hizo el cambio.
// Cualquier valor que no sea uno de estos tres cae en OrigenDesconocido
// (ver normalizarOrigen); la fila del historial nunca se pierde por esto.
const (
	OrigenApp         = "app"
	OrigenExtension   = "extension"
	OrigenAgente      = "agente"
	OrigenDesconocido = "desconocido"
)

// Acciones registradas. Nombres estables: son datos que ya viven en el
// .db del usuario, cambiarlos rompe el historial de instalaciones viejas.
const (
	AccionCarpetaCreada     = "carpeta_creada"
	AccionCarpetaBorrada    = "carpeta_borrada"
	AccionRecursoGuardado   = "recurso_guardado"
	AccionRecursoBorrado    = "recurso_borrado"
	AccionProgresoCambiado  = "progreso_cambiado"
	AccionEstadoCambiado    = "estado_cambiado"
	AccionNotaAgendaCreada  = "nota_agenda_creada"
	AccionNotaAgendaBorrada = "nota_agenda_borrada"
	AccionLoteImportado     = "lote_importado"
	AccionLoteActualizado   = "lote_actualizado"
	AccionLoteBorrado       = "lote_borrado"
	AccionAvanceImportado   = "avance_importado"
	// AccionTerminalAgenteAbierta: el usuario pidió el botón "Abrir agente"
	// (ver api/agente.go) y Bythos abrió una terminal en el workspace del
	// agente. detalle trae qué terminal se usó y la carpeta.
	AccionTerminalAgenteAbierta = "terminal_agente_abierta"
)

// limiteEventosPorDefecto/Maximo acotan ListarEventos: sin límite pedido
// trae poco (la UI no necesita miles de filas), y un límite absurdo que
// mande el cliente no puede tirar abajo la consulta.
const (
	limiteEventosPorDefecto = 50
	limiteEventosMaximo     = 500
)

// Evento es una fila del historial: quién, qué y cuándo.
// EntidadID es el id de la carpeta/recurso/nota/lote afectado (0 si no aplica).
type Evento struct {
	ID        int64
	Creado    string
	Origen    string
	Actor     string
	Accion    string
	EntidadID int64
	Detalle   string
}

// FiltroEventos acota ListarEventos. Limite <= 0 usa el default;
// Origen vacío no filtra por origen.
type FiltroEventos struct {
	Limite int
	Origen string
}

// crearTablaEventos define la tabla. CREATE TABLE IF NOT EXISTS ya es
// idempotente por sí solo (no hace falta ALTER TABLE como migrarProgreso):
// una instalación vieja sin esta tabla la gana en el próximo Abrir, una
// instalación nueva la trae desde el primer arranque. Ver crearTablas en db.go.
func crearTablaEventos(base *sql.DB) error {
	_, err := base.Exec(`CREATE TABLE IF NOT EXISTS eventos (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		creado TEXT NOT NULL DEFAULT (datetime('now','localtime')),
		origen TEXT NOT NULL DEFAULT 'desconocido',
		actor TEXT NOT NULL DEFAULT '',
		accion TEXT NOT NULL,
		entidad_id INTEGER NOT NULL DEFAULT 0,
		detalle TEXT NOT NULL DEFAULT ''
	);`)
	return err
}

// normalizarOrigen fuerza uno de los 3 orígenes válidos; cualquier otra
// cosa (o vacío) es OrigenDesconocido. Misma regla que aplica api/origen.go
// sobre la cabecera X-Bythos-Origen, repetida aquí porque la BD es la
// última defensa (no confía en que el caller ya validó).
func normalizarOrigen(o string) string {
	switch strings.ToLower(strings.TrimSpace(o)) {
	case OrigenApp:
		return OrigenApp
	case OrigenExtension:
		return OrigenExtension
	case OrigenAgente:
		return OrigenAgente
	default:
		return OrigenDesconocido
	}
}

// RegistrarEvento guarda una fila del historial.
// actor puede ir vacío (no todo origen manda uno). Nunca falla por un
// origen raro: cae a desconocido en vez de rechazar el evento.
func RegistrarEvento(base *sql.DB, origen, actor, accion string, entidadID int64, detalle string) error {
	_, err := base.Exec(
		`INSERT INTO eventos (origen, actor, accion, entidad_id, detalle) VALUES (?, ?, ?, ?, ?)`,
		normalizarOrigen(origen), actor, accion, entidadID, detalle,
	)
	return err
}

// ListarEventos trae el historial, más nuevo primero.
func ListarEventos(base *sql.DB, filtro FiltroEventos) ([]Evento, error) {
	limite := filtro.Limite
	if limite <= 0 {
		limite = limiteEventosPorDefecto
	}
	if limite > limiteEventosMaximo {
		limite = limiteEventosMaximo
	}

	query := `SELECT id, creado, origen, actor, accion, entidad_id, detalle FROM eventos`
	args := []interface{}{}
	if filtro.Origen != "" {
		query += ` WHERE origen = ?`
		args = append(args, normalizarOrigen(filtro.Origen))
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limite)

	rows, err := base.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // SIEMPRE cerrar rows o bloqueas el archivo .db

	out := []Evento{} // slice vacío (no nil) para que el JSON sea [] y no null
	for rows.Next() {
		var e Evento
		if err := rows.Scan(&e.ID, &e.Creado, &e.Origen, &e.Actor, &e.Accion, &e.EntidadID, &e.Detalle); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
