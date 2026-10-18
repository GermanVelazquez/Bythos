package db

// adjuntos.go — Archivos que el usuario adjunta a la pizarra de una sesión
// de Terminal. Un adjunto es un VÍNCULO (sesión, archivo): el blob vive una
// sola vez en `archivos` (dedupe por sha256) y varias sesiones pueden
// apuntar al mismo. Borrar la sesión borra sus vínculos (CASCADE) pero
// nunca el archivo; y un archivo con vínculos no se puede borrar de
// `archivos` (la FK no lleva CASCADE a propósito, design D1).

import "database/sql"

// Adjunto es un vínculo sesión-archivo con los metadatos del blob.
type Adjunto struct {
	ID        int64  `json:"id"`
	SesionID  int64  `json:"sesion_id"`
	ArchivoID int64  `json:"archivo_id"`
	Nombre    string `json:"nombre"`
	Creado    string `json:"creado"`
	SHA256    string `json:"sha256"`
	Mime      string `json:"mime"`
	Tamano    int64  `json:"tamano"`
}

// crearTablaAdjuntos: tabla nueva, CREATE TABLE IF NOT EXISTS alcanza.
// Va DESPUÉS de crearTablaSesiones y crearTablaArchivos (las referencia).
func crearTablaAdjuntos(base *sql.DB) error {
	_, err := base.Exec(`CREATE TABLE IF NOT EXISTS terminal_adjuntos (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		sesion_id INTEGER NOT NULL REFERENCES terminal_sesiones(id) ON DELETE CASCADE,
		archivo_id INTEGER NOT NULL REFERENCES archivos(id),
		nombre TEXT NOT NULL DEFAULT '',
		creado TEXT NOT NULL DEFAULT (datetime('now','localtime')),
		UNIQUE(sesion_id, archivo_id)
	);`)
	return err
}

const columnasAdjunto = `t.id, t.sesion_id, t.archivo_id, t.nombre, t.creado, a.sha256, a.mime, a.tamano`

const desdeAdjuntos = ` FROM terminal_adjuntos t JOIN archivos a ON a.id = t.archivo_id`

func leerAdjunto(r escaner) (Adjunto, error) {
	var a Adjunto
	err := r.Scan(&a.ID, &a.SesionID, &a.ArchivoID, &a.Nombre, &a.Creado, &a.SHA256, &a.Mime, &a.Tamano)
	return a, err
}

// CrearAdjunto vincula el archivo a la sesión. Si ya estaba vinculado
// devuelve el vínculo existente (UNIQUE(sesion_id, archivo_id)); el nombre
// de la primera vez gana. Una sesión o archivo inexistente falla por la FK.
func CrearAdjunto(base *sql.DB, sesionID, archivoID int64, nombre string) (Adjunto, error) {
	if _, err := base.Exec(
		`INSERT INTO terminal_adjuntos (sesion_id, archivo_id, nombre) VALUES (?, ?, ?)
		 ON CONFLICT(sesion_id, archivo_id) DO NOTHING`,
		sesionID, archivoID, nombre); err != nil {
		return Adjunto{}, err
	}
	return leerAdjunto(base.QueryRow(
		`SELECT `+columnasAdjunto+desdeAdjuntos+` WHERE t.sesion_id = ? AND t.archivo_id = ?`,
		sesionID, archivoID))
}

// ListarAdjuntos trae solo los vínculos de esa sesión, en orden de creación.
func ListarAdjuntos(base *sql.DB, sesionID int64) ([]Adjunto, error) {
	rows, err := base.Query(
		`SELECT `+columnasAdjunto+desdeAdjuntos+` WHERE t.sesion_id = ? ORDER BY t.id`, sesionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Adjunto{}
	for rows.Next() {
		a, err := leerAdjunto(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ObtenerAdjunto busca por (sesión, id): un adjunto de OTRA sesión da ok
// false, igual que uno inexistente (privacidad entre sesiones).
func ObtenerAdjunto(base *sql.DB, sesionID, id int64) (Adjunto, bool, error) {
	a, err := leerAdjunto(base.QueryRow(
		`SELECT `+columnasAdjunto+desdeAdjuntos+` WHERE t.sesion_id = ? AND t.id = ?`, sesionID, id))
	if err == sql.ErrNoRows {
		return Adjunto{}, false, nil
	}
	return a, err == nil, err
}
