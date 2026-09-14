package db

// folders.go — Operaciones de CARPETAS ("Go Backend", "React"...).
// ¿Por qué un archivo solo para esto y no todo en db.go?
// Principio senior: un archivo = una responsabilidad.
// db.go = "cómo me conecto". folders.go = "qué hago con carpetas".
// Cuando busques un bug de carpetas, ya sabes dónde mirar.

import (
	"database/sql"
	"strings"
)

// Carpeta es lo que el resto de la app ve.
// No exponemos created_at porque la UI no la necesita hoy.
// Si mañana la pides, la agregas aquí sin romper nada.
type Carpeta struct {
	ID     int64
	Nombre string
}

// CrearCarpeta guarda una carpeta nueva.
// Recibe el *sql.DB abierto (no abre otro: reutiliza la única conexión).
// Devuelve la Carpeta con su ID recién asignado.
func CrearCarpeta(base *sql.DB, nombre string) (Carpeta, error) {
	// Validar AQUÍ y no en la UI, ¿por qué?
	// Porque la UI miente: el usuario puede mandar "" desde la extensión,
	// desde un test o desde Postman. La BD es la última defensa.
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return Carpeta{}, sql.ErrNoRows // usamos un error conocido para "dato inválido"
	}

	// QueryRow + RETURNING evita 2 viajes a disco (INSERT y luego SELECT).
	// SQLite moderno sí soporta RETURNING.
	var c Carpeta
	err := base.QueryRow(
		`INSERT INTO folders (name) VALUES (?) RETURNING id, name`,
		nombre,
	).Scan(&c.ID, &c.Nombre)
	if err != nil {
		return Carpeta{}, err
	}
	return c, nil
}

// BorrarCarpeta elimina una carpeta por id.
// Los recursos se borran solos por ON DELETE CASCADE (ver db.go).
// Devuelve true si la carpeta existía, false si no había nada que borrar.
func BorrarCarpeta(base *sql.DB, id int64) (bool, error) {
	res, err := base.Exec(`DELETE FROM folders WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ListarCarpetas trae todas ordenadas por creación.
// ¿Por qué ORDER BY id y no por name?
// Porque el usuario espera ver primero lo que creó primero.
// Ordenar por nombre confunde ("¿dónde quedó la que acabo de crear?").
func ListarCarpetas(base *sql.DB) ([]Carpeta, error) {
	rows, err := base.Query(`SELECT id, name FROM folders ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // SIEMPRE cerrar rows o bloqueas el archivo .db

	out := []Carpeta{} // slice vacío (no nil) para que el JSON sea [] y no null
	for rows.Next() {
		var c Carpeta
		if err := rows.Scan(&c.ID, &c.Nombre); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
