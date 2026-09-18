package db

// import_lotes.go — Import batches: the SOURCE behind agenda rows.
// Every Markdown import is stored as a lote (nombre + raw markdown) and
// each expanded agenda row points back via agenda.lote_id.
// Editing a lote re-expands its markdown; deleting it removes its rows.
// Loose rows (lote_id NULL, created by hand) are never touched.
//
// Why a separate file and not inside agenda.go?
// Different question: agenda.go = "what do I do each day",
// this file = "where did those rows come from". Question differs,
// file differs. Same rule as folders vs resources.

import (
	"database/sql"
	"errors"
	"strings"
)

// Lote is one stored Markdown import with its live row count.
// No json tags on purpose: like Agenda, keys travel Capitalized
// for direct db responses; the import handlers re-shape to lowercase.
type Lote struct {
	ID       int64
	Nombre   string
	Markdown string
	Creado   string // local 'YYYY-MM-DD HH:MM:SS' via datetime('now','localtime')
	Total    int    // live COUNT of agenda rows pointing at this lote
}

// nombreLoteValido trims and rejects empties in Spanish, ready for the UI.
func nombreLoteValido(nombre string) (string, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return "", errors.New("El nombre del lote es obligatorio")
	}
	return nombre, nil
}

// CrearLote stores the import source and returns its id.
// Markdown may be any non-empty text; the parser decides what it means.
func CrearLote(base *sql.DB, nombre, markdown string) (Lote, error) {
	nombre, err := nombreLoteValido(nombre)
	if err != nil {
		return Lote{}, err
	}
	if strings.TrimSpace(markdown) == "" {
		return Lote{}, errors.New("El markdown está vacío. Manda {\"markdown\":\"...\"}")
	}
	var l Lote
	err = base.QueryRow(
		`INSERT INTO import_lotes (nombre, markdown) VALUES (?, ?)
		 RETURNING id, nombre, markdown, creado`,
		nombre, markdown,
	).Scan(&l.ID, &l.Nombre, &l.Markdown, &l.Creado)
	if err != nil {
		return Lote{}, err
	}
	return l, nil
}

// ObtenerLote fetches one batch with its live row count.
// Second return is false when the id does not exist (no error).
func ObtenerLote(base *sql.DB, id int64) (Lote, bool, error) {
	var l Lote
	err := base.QueryRow(
		`SELECT id, nombre, markdown, creado FROM import_lotes WHERE id = ?`, id,
	).Scan(&l.ID, &l.Nombre, &l.Markdown, &l.Creado)
	if err == sql.ErrNoRows {
		return Lote{}, false, nil
	}
	if err != nil {
		return Lote{}, false, err
	}
	total, err := ContarPorLote(base, id)
	if err != nil {
		return Lote{}, false, err
	}
	l.Total = total
	return l, true, nil
}

// ListarLotes returns every batch newest first with its row count.
// Slice is never nil so the JSON is [] and not null when empty.
func ListarLotes(base *sql.DB) ([]Lote, error) {
	rows, err := base.Query(
		`SELECT l.id, l.nombre, l.markdown, l.creado, COUNT(a.id) AS total
		   FROM import_lotes l LEFT JOIN agenda a ON a.lote_id = l.id
		  GROUP BY l.id ORDER BY l.creado DESC, l.id DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // ALWAYS close rows or the .db file locks

	out := []Lote{}
	for rows.Next() {
		var l Lote
		if err := rows.Scan(&l.ID, &l.Nombre, &l.Markdown, &l.Creado, &l.Total); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ActualizarLote rewrites name + markdown of an existing batch.
// It does NOT touch agenda rows: the caller re-expands first.
// Returns false when the id does not exist.
func ActualizarLote(base *sql.DB, id int64, nombre, markdown string) (Lote, bool, error) {
	nombre, err := nombreLoteValido(nombre)
	if err != nil {
		return Lote{}, false, err
	}
	if strings.TrimSpace(markdown) == "" {
		return Lote{}, false, errors.New("El markdown está vacío. Manda {\"markdown\":\"...\"}")
	}
	res, err := base.Exec(
		`UPDATE import_lotes SET nombre = ?, markdown = ? WHERE id = ?`,
		nombre, markdown, id,
	)
	if err != nil {
		return Lote{}, false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Lote{}, false, err
	}
	if n == 0 {
		return Lote{}, false, nil
	}
	return ObtenerLoteFull(base, id)
}

// ObtenerLoteFull is ObtenerLote for ids known to exist.
func ObtenerLoteFull(base *sql.DB, id int64) (Lote, bool, error) {
	return ObtenerLote(base, id)
}

// ContarPorLote counts agenda rows born from one batch.
func ContarPorLote(base *sql.DB, loteID int64) (int, error) {
	var n int
	if err := base.QueryRow(`SELECT COUNT(*) FROM agenda WHERE lote_id = ?`, loteID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// ListarCarpetasPorLote returns the distinct live folders linked from one
// batch's rows, ordered by creation (same as ListarCarpetas).
// Empty slice (never nil) when the lote is loose or its folder was deleted
// (agenda.carpeta_id is ON DELETE SET NULL, so the link just vanishes).
func ListarCarpetasPorLote(base *sql.DB, loteID int64) ([]Carpeta, error) {
	rows, err := base.Query(
		`SELECT DISTINCT f.id, f.name
		   FROM agenda a JOIN folders f ON f.id = a.carpeta_id
		  WHERE a.lote_id = ? ORDER BY f.id`,
		loteID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Carpeta{}
	for rows.Next() {
		var c Carpeta
		if err := rows.Scan(&c.ID, &c.Nombre); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// BorrarAgendaPorLote removes every agenda row of one batch.
// Loose rows (lote_id NULL) and other batches are untouched.
// Returns how many rows were removed.
func BorrarAgendaPorLote(base *sql.DB, loteID int64) (int64, error) {
	res, err := base.Exec(`DELETE FROM agenda WHERE lote_id = ?`, loteID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// BorrarLote removes the batch and its agenda rows, nothing else.
// Rows are deleted explicitly BEFORE the lote: FK cascade is the
// seatbelt, this DELETE is the brake (works even if a connection
// ever opens the .db with foreign_keys off).
// Returns (agenda rows removed, lote existed, error).
func BorrarLote(base *sql.DB, id int64) (int64, bool, error) {
	borradas, err := BorrarAgendaPorLote(base, id)
	if err != nil {
		return 0, false, err
	}
	res, err := base.Exec(`DELETE FROM import_lotes WHERE id = ?`, id)
	if err != nil {
		return borradas, false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return borradas, false, err
	}
	if n == 0 {
		return borradas, false, nil
	}
	return borradas, true, nil
}
