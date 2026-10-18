package db

// pizarra_ops.go — cola de operaciones de pizarra_dibujar (design D12).
// La escena la posee la UI: el agente encola un lote, la UI lo recoge por
// latido, lo aplica y responde (ack). Una fila que sigue pendiente después
// de 30 s pasa a "vencida" y nunca se entrega (barrido perezoso: no hay
// goroutine, cada lectura o ack barre primero).

import (
	"database/sql"
	"errors"
)

// ErrOpNoPendiente: el ack llegó para una op que ya no está pendiente
// (ya respondida o vencida).
var ErrOpNoPendiente = errors.New("la operación ya no está pendiente")

// OpPendiente es un lote de ops tal como lo recibe la UI. Op es JSON crudo.
type OpPendiente struct {
	ID int64
	Op string
}

func crearTablaOps(base *sql.DB) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS terminal_pizarra_ops (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sesion_id INTEGER NOT NULL REFERENCES terminal_sesiones(id) ON DELETE CASCADE,
			op TEXT NOT NULL DEFAULT '',
			estado TEXT NOT NULL DEFAULT 'pendiente' CHECK(estado IN ('pendiente','aplicada','fallida','vencida')),
			resultado TEXT NOT NULL DEFAULT '',
			creado TEXT NOT NULL DEFAULT (datetime('now','localtime')),
			aplicado TEXT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_pizarra_ops_sesion_estado ON terminal_pizarra_ops(sesion_id, estado);`,
	} {
		if _, err := base.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// barrerOps vence las pendientes de más de 30 s.
func barrerOps(base *sql.DB) error {
	_, err := base.Exec(`UPDATE terminal_pizarra_ops SET estado = 'vencida'
		WHERE estado = 'pendiente' AND creado <= datetime('now','localtime','-30 seconds')`)
	return err
}

// EncolarOp inserta el lote como pendiente y devuelve su id.
func EncolarOp(base *sql.DB, sesionID int64, op string) (int64, error) {
	var id int64
	err := base.QueryRow(`INSERT INTO terminal_pizarra_ops (sesion_id, op) VALUES (?, ?) RETURNING id`, sesionID, op).Scan(&id)
	return id, err
}

// OpsPendientes devuelve los lotes por entregar, más viejo primero. Se
// siguen devolviendo hasta el ack: si el ack se pierde, la UI los reaplica
// y el merge por id los deja idempotentes.
func OpsPendientes(base *sql.DB, sesionID int64) ([]OpPendiente, error) {
	if err := barrerOps(base); err != nil {
		return nil, err
	}
	rows, err := base.Query(`SELECT id, op FROM terminal_pizarra_ops WHERE sesion_id = ? AND estado = 'pendiente' ORDER BY id`, sesionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OpPendiente{}
	for rows.Next() {
		var o OpPendiente
		if err := rows.Scan(&o.ID, &o.Op); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ContarOpsPendientes cuenta los lotes aún por aplicar de la sesión.
func ContarOpsPendientes(base *sql.DB, sesionID int64) (n int, err error) {
	if err = barrerOps(base); err != nil {
		return 0, err
	}
	err = base.QueryRow(`SELECT COUNT(*) FROM terminal_pizarra_ops WHERE sesion_id = ? AND estado = 'pendiente'`, sesionID).Scan(&n)
	return n, err
}

// ResponderOp registra el ack (aplicada o fallida) y devuelve la sesión de la
// op. ErrOpNoPendiente si ya no estaba pendiente; sql.ErrNoRows si no existe.
func ResponderOp(base *sql.DB, id int64, aplicada bool, resultado string) (int64, error) {
	if err := barrerOps(base); err != nil {
		return 0, err
	}
	estado := "fallida"
	if aplicada {
		estado = "aplicada"
	}
	var sesion int64
	err := base.QueryRow(`UPDATE terminal_pizarra_ops SET estado = ?, resultado = ?, aplicado = datetime('now','localtime')
		WHERE id = ? AND estado = 'pendiente' RETURNING sesion_id`, estado, resultado, id).Scan(&sesion)
	if err != sql.ErrNoRows {
		return sesion, err
	}
	var existe int
	if err := base.QueryRow(`SELECT 1 FROM terminal_pizarra_ops WHERE id = ?`, id).Scan(&existe); err != nil {
		return 0, err // sql.ErrNoRows = no existe
	}
	return 0, ErrOpNoPendiente
}

// EstadoOp lee el estado y el resultado guardados de una op (sin barrer:
// solo lo usa quien acaba de encolarla, para no perder un ack temprano).
func EstadoOp(base *sql.DB, id int64) (estado, resultado string, err error) {
	err = base.QueryRow(`SELECT estado, resultado FROM terminal_pizarra_ops WHERE id = ?`, id).Scan(&estado, &resultado)
	return estado, resultado, err
}
