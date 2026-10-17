package db

// sesiones.go — Sesiones de Terminal y su pizarra (escena de Excalidraw).
// La escena es un blob que la UI posee: acá solo se guarda con control de
// versión para que una ventana vieja no pise a una más nueva (design D10).

import (
	"database/sql"
	"fmt"
	"strings"
)

// EscenaVacia es la escena inicial de toda sesión nueva.
const EscenaVacia = `{"elements":[],"appState":{"viewBackgroundColor":"#ffffff"},"files":{}}`

// Sesion es una fila de terminal_sesiones. CarpetaID nil = sesión "suelta".
type Sesion struct {
	ID          int64  `json:"id"`
	Nombre      string `json:"nombre"`
	Modo        string `json:"modo"`
	CarpetaID   *int64 `json:"carpeta_id"`
	Creado      string `json:"creado"`
	Actualizado string `json:"actualizado"`
}

// CambiosSesion es un PATCH parcial. CarpetaSet distingue "no tocar la
// carpeta" de "soltarla" (CarpetaSet con CarpetaID nil).
type CambiosSesion struct {
	Nombre, Modo *string
	CarpetaSet   bool
	CarpetaID    *int64
}

// ConflictoVersion: la versión no coincidía; Actual es la vigente.
type ConflictoVersion struct{ Actual int64 }

func (c *ConflictoVersion) Error() string {
	return fmt.Sprintf("la pizarra ya está en la versión %d", c.Actual)
}

func crearTablaSesiones(base *sql.DB) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS terminal_sesiones (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			nombre TEXT NOT NULL CHECK(length(trim(nombre)) > 0),
			modo TEXT NOT NULL CHECK(modo IN ('practica','revision','prototipo','dibujo','paginas')),
			carpeta_id INTEGER NULL REFERENCES folders(id) ON DELETE SET NULL,
			creado TEXT NOT NULL DEFAULT (datetime('now','localtime')),
			actualizado TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		);`,
		`CREATE TABLE IF NOT EXISTS terminal_pizarras (
			sesion_id INTEGER PRIMARY KEY REFERENCES terminal_sesiones(id) ON DELETE CASCADE,
			escena TEXT NOT NULL DEFAULT '',
			version INTEGER NOT NULL DEFAULT 0,
			actualizado TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		);`,
	} {
		if _, err := base.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

const columnasSesion = `id, nombre, modo, carpeta_id, creado, actualizado`

type escaner interface{ Scan(...interface{}) error }

func leerSesion(r escaner) (Sesion, error) {
	var s Sesion
	var carpeta sql.NullInt64
	if err := r.Scan(&s.ID, &s.Nombre, &s.Modo, &carpeta, &s.Creado, &s.Actualizado); err != nil {
		return Sesion{}, err
	}
	if carpeta.Valid {
		s.CarpetaID = &carpeta.Int64
	}
	return s, nil
}

// CrearSesion inserta la sesión y su fila de pizarra vacía en una transacción.
func CrearSesion(base *sql.DB, nombre, modo string, carpetaID *int64) (Sesion, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return Sesion{}, sql.ErrNoRows // mismo "dato inválido" que CrearCarpeta
	}
	tx, err := base.Begin()
	if err != nil {
		return Sesion{}, err
	}
	defer tx.Rollback()
	s, err := leerSesion(tx.QueryRow(
		`INSERT INTO terminal_sesiones (nombre, modo, carpeta_id) VALUES (?, ?, ?) RETURNING `+columnasSesion,
		nombre, modo, carpetaID))
	if err != nil {
		return Sesion{}, err
	}
	if _, err := tx.Exec(`INSERT INTO terminal_pizarras (sesion_id, escena) VALUES (?, ?)`, s.ID, EscenaVacia); err != nil {
		return Sesion{}, err
	}
	return s, tx.Commit()
}

// ListarSesiones trae las sesiones SIN la escena (puede pesar MiB).
func ListarSesiones(base *sql.DB) ([]Sesion, error) {
	rows, err := base.Query(`SELECT ` + columnasSesion + ` FROM terminal_sesiones ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Sesion{}
	for rows.Next() {
		s, err := leerSesion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ObtenerSesion devuelve la sesión; ok false si no existe.
func ObtenerSesion(base *sql.DB, id int64) (Sesion, bool, error) {
	s, err := leerSesion(base.QueryRow(`SELECT `+columnasSesion+` FROM terminal_sesiones WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return Sesion{}, false, nil
	}
	return s, err == nil, err
}

// ActualizarSesion aplica solo los campos presentes. false si no existe.
func ActualizarSesion(base *sql.DB, id int64, c CambiosSesion) (bool, error) {
	sets, args := []string{`actualizado = datetime('now','localtime')`}, []interface{}{}
	if c.Nombre != nil {
		sets, args = append(sets, `nombre = ?`), append(args, strings.TrimSpace(*c.Nombre))
	}
	if c.Modo != nil {
		sets, args = append(sets, `modo = ?`), append(args, *c.Modo)
	}
	if c.CarpetaSet {
		sets, args = append(sets, `carpeta_id = ?`), append(args, c.CarpetaID)
	}
	res, err := base.Exec(`UPDATE terminal_sesiones SET `+strings.Join(sets, ", ")+` WHERE id = ?`, append(args, id)...)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// BorrarSesion borra la fila; el CASCADE se lleva escena y ops.
func BorrarSesion(base *sql.DB, id int64) (bool, error) {
	res, err := base.Exec(`DELETE FROM terminal_sesiones WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// ObtenerPizarra devuelve la escena cruda y su versión; ok false si no hay sesión.
func ObtenerPizarra(base *sql.DB, id int64) (escena string, version int64, ok bool, err error) {
	err = base.QueryRow(`SELECT escena, version FROM terminal_pizarras WHERE sesion_id = ?`, id).Scan(&escena, &version)
	if err == sql.ErrNoRows {
		return "", 0, false, nil
	}
	return escena, version, err == nil, err
}

// GuardarPizarra guarda la escena solo si version coincide con la vigente y
// devuelve la nueva. Si no, *ConflictoVersion con la versión actual; si la
// sesión no existe, sql.ErrNoRows.
func GuardarPizarra(base *sql.DB, id int64, escena string, version int64) (int64, error) {
	res, err := base.Exec(
		`UPDATE terminal_pizarras SET escena = ?, version = version + 1, actualizado = datetime('now','localtime')
		 WHERE sesion_id = ? AND version = ?`, escena, id, version)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		base.Exec(`UPDATE terminal_sesiones SET actualizado = datetime('now','localtime') WHERE id = ?`, id)
		return version + 1, nil
	}
	_, actual, ok, err := ObtenerPizarra(base, id)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, sql.ErrNoRows
	}
	return 0, &ConflictoVersion{Actual: actual}
}
