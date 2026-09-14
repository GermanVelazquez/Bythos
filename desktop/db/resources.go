package db

// resources.go — Operaciones de RECURSOS (tus links guardados).
// ¿Por qué separado de folders.go?
// Porque cambian por motivos distintos: carpetas casi no cambian,
// recursos cambian cada día (guardas, avanzas, completas).
// Si todo estuviera junto, el archivo sería gigante y frágil.

import (
	"database/sql"
	"strings"
)

// Los 3 únicos estados válidos en Bythos.
// Constantes y no strings sueltos: si escribes "completado" mal,
// el compilador te avisa. Con strings sueltos el bug llega al usuario.
const (
	EstadoPendiente  = "pendiente"
	EstadoEnCurso    = "en_curso"
	EstadoCompletado = "completado"
)

// Recurso es lo que la UI muestra en cada tarjeta.
// Fíjate: db NO descarga nada de internet.
// Solo guarda los strings que api/ ya detectó (título, imagen...).
// Separación senior: db = memoria, api = cerebro que piensa.
type Recurso struct {
	ID          int64
	CarpetaID   int64
	URL         string
	Titulo      string
	Imagen      string
	Descripcion string
	Tipo        string // "youtube" | "articulo" | "otro"
	Estado      string // "pendiente" | "en_curso" | "completado"
	Progreso    int    // 0-100, percentage studied per link
}

// Guardar crea un recurso en estado pendiente.
// ¿Por qué siempre pendiente al nacer?
// Porque acabas de pegarlo, aún no lo estudiaste. Regla de negocio,
// no capricho: nace pendiente, luego CambiarEstado lo mueve.
func Guardar(base *sql.DB, carpetaID int64, url, titulo, imagen, descripcion, tipo string) (Recurso, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return Recurso{}, sql.ErrNoRows
	}
	if carpetaID == 0 {
		return Recurso{}, sql.ErrNoRows
	}
	if tipo == "" {
		tipo = "otro"
	}

	var r Recurso
	err := base.QueryRow(
		`INSERT INTO resources (folder_id, url, title, image, description, content_type, status, progreso)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0)
		 RETURNING id, folder_id, url, title, image, description, content_type, status, progreso`,
		carpetaID, url, titulo, imagen, descripcion, tipo, EstadoPendiente,
	).Scan(&r.ID, &r.CarpetaID, &r.URL, &r.Titulo, &r.Imagen, &r.Descripcion, &r.Tipo, &r.Estado, &r.Progreso)
	if err != nil {
		return Recurso{}, err
	}
	return r, nil
}

// ListarRecursos trae recursos. Si carpetaID es 0, trae TODOS.
// ¿Por qué un solo ListarRecursos con filtro y no dos funciones?
// Porque la query solo cambia en un WHERE. Dos funciones duplicarían
// el Scan de 9 campos y duplicar es deuda técnica.
func ListarRecursos(base *sql.DB, carpetaID int64) ([]Recurso, error) {
	query := `SELECT id, folder_id, url, title, image, description, content_type, status, progreso
	          FROM resources`
	args := []interface{}{}
	if carpetaID != 0 {
		query += ` WHERE folder_id = ?`
		args = append(args, carpetaID)
	}
	query += ` ORDER BY id DESC` // DESC: lo recién guardado primero (lo buscas arriba)

	rows, err := base.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Recurso{}
	for rows.Next() {
		var r Recurso
		if err := rows.Scan(&r.ID, &r.CarpetaID, &r.URL, &r.Titulo, &r.Imagen, &r.Descripcion, &r.Tipo, &r.Estado, &r.Progreso); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ActualizarProgreso sets 0-100 progress and syncs status for compatibility:
// 0 -> pendiente, 100 -> completado, 1-99 -> en_curso.
// Values outside range are clamped, never rejected.
func ActualizarProgreso(base *sql.DB, id int64, valor int) error {
	valor = clampProgreso(valor)
	var estado string
	switch {
	case valor == 0:
		estado = EstadoPendiente
	case valor == 100:
		estado = EstadoCompletado
	default:
		estado = EstadoEnCurso
	}
	_, err := base.Exec(`UPDATE resources SET progreso = ?, status = ? WHERE id = ?`, valor, estado, id)
	return err
}

// ActualizarProgresoEnCarpeta is the folder-scoped variant used by the
// import-avance endpoint: it only touches the row when it belongs to the
// given folder. It reports whether a row was actually updated.
func ActualizarProgresoEnCarpeta(base *sql.DB, carpetaID, id int64, valor int) (bool, error) {
	valor = clampProgreso(valor)
	var estado string
	switch {
	case valor == 0:
		estado = EstadoPendiente
	case valor == 100:
		estado = EstadoCompletado
	default:
		estado = EstadoEnCurso
	}
	res, err := base.Exec(`UPDATE resources SET progreso = ?, status = ? WHERE id = ? AND folder_id = ?`, valor, estado, id, carpetaID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func clampProgreso(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// CambiarEstado mueve pendiente -> en_curso -> completado.
// Valida ANTES de tocar disco: si el estado es inventado ("viendo"),
// falla rápido sin UPDATE a medias.
// Compat: pendiente forces progreso 0 and completado forces 100 so the
// per-link bar stays honest; en_curso keeps the current progreso.
func CambiarEstado(base *sql.DB, id int64, nuevo string) error {
	nuevo = normalizarEstado(nuevo)
	if !estadoValido(nuevo) {
		return sql.ErrNoRows
	}
	switch nuevo {
	case EstadoPendiente:
		_, err := base.Exec(`UPDATE resources SET status = ?, progreso = 0 WHERE id = ?`, nuevo, id)
		return err
	case EstadoCompletado:
		_, err := base.Exec(`UPDATE resources SET status = ?, progreso = 100 WHERE id = ?`, nuevo, id)
		return err
	default:
		_, err := base.Exec(`UPDATE resources SET status = ? WHERE id = ?`, nuevo, id)
		return err
	}
}

// Borrar elimina un recurso. No pide carpeta: el id ya es único.
func Borrar(base *sql.DB, id int64) error {
	_, err := base.Exec(`DELETE FROM resources WHERE id = ?`, id)
	return err
}

// normalizarEstado acepta lo que el humano escribe y lo vuelve canónico:
// "en curso", "en-curso", "EN_CURSO" -> "en_curso".
// ¿Por qué aquí y no en la UI? Porque la extensión y la UI pueden
// mandar formatos distintos. La BD unifica una sola vez.
func normalizarEstado(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, " ", "_")
	return s
}

func estadoValido(s string) bool {
	return s == EstadoPendiente || s == EstadoEnCurso || s == EstadoCompletado
}
