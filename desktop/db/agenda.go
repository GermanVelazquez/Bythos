package db

// agenda.go — La AGENDA. Tus notas en el calendario ("repasar hooks el viernes"...).
// ¿Por qué archivo propio y no en resources.go?
// Porque responde otra pregunta: resources = "qué tengo guardado",
// agenda = "qué hago cada día". Pregunta distinta, archivo distinto.
//
// ¿Por qué fecha como TEXT 'YYYY-MM-DD' y no DATETIME?
// Porque el calendario piensa en DÍAS, no en instantes.
// 'YYYY-MM-DD' se ordena y se compara como string sin parsear nada,
// y DATE(created_at) de las otras tablas ya habla ese idioma.

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Agenda es una nota en un día. Sin json tags a propósito:
// como Carpeta y Recurso, las claves viajan Capitalizadas
// y la UI las normaliza (ver normAgenda en App.jsx).
type Agenda struct {
	ID            int64
	Fecha         string // 'YYYY-MM-DD', día real de calendario
	HoraInicio    string // 'HH:MM' o "" = sin hora
	HoraFin       string // 'HH:MM' o ""
	Texto         string
	CarpetaID     *int64 // nil = nota suelta, sin carpeta
	CarpetaNombre string // JOIN con folders, "" si no hay carpeta
}

// Actividad es un punto del calendario: cuántas cosas CREASTE ese día.
// Suma recursos + carpetas (DATE(created_at) de ambas) porque las dos
// son "ruido de estudio": guardar un link o abrir un tema cuentan igual.
type Actividad struct {
	Fecha string // 'YYYY-MM-DD'
	Total int
}

// CrearAgenda guarda una nota en un día.
// Valida AQUÍ y no solo en la UI, ¿por qué?
// Misma razón que CrearCarpeta: la UI miente, la BD es la última defensa.
// Devuelve errores en español listos para mostrar (api los pasa tal cual).
func CrearAgenda(base *sql.DB, fecha, horaInicio, horaFin, texto string, carpetaID *int64) (Agenda, error) {
	return CrearAgendaEnLote(base, fecha, horaInicio, horaFin, texto, carpetaID, nil)
}

// CrearAgendaEnLote es CrearAgenda con dueño: la nota nace atada a un
// lote de importación (loteID nil = nota suelta, a mano).
// El lote debe existir si lo das: mismo trato que la carpeta,
// error en español en vez de un "FOREIGN KEY" en inglés.
func CrearAgendaEnLote(base *sql.DB, fecha, horaInicio, horaFin, texto string, carpetaID, loteID *int64) (Agenda, error) {
	fecha = strings.TrimSpace(fecha)
	horaInicio = strings.TrimSpace(horaInicio)
	horaFin = strings.TrimSpace(horaFin)
	texto = strings.TrimSpace(texto)

	if err := validarAgenda(fecha, horaInicio, horaFin, texto, carpetaID); err != nil {
		return Agenda{}, err
	}
	// La carpeta debe existir SI la diste: el FK la exige igual,
	// pero este SELECT da un error en español en vez de un "FOREIGN KEY" en inglés.
	if carpetaID != nil {
		var n int
		if err := base.QueryRow(`SELECT COUNT(*) FROM folders WHERE id = ?`, *carpetaID).Scan(&n); err != nil {
			return Agenda{}, err
		}
		if n == 0 {
			return Agenda{}, errors.New("La carpeta no existe")
		}
	}

	// "" en Go sería "" en SQLite, y queremos NULL de verdad
	// (sin hora = NULL, no string vacío: el ORDER BY y la UI distinguen).
	var inicioArg, finArg, carpetaArg, loteArg interface{}
	if horaInicio != "" {
		inicioArg = horaInicio
	}
	if horaFin != "" {
		finArg = horaFin
	}
	if carpetaID != nil {
		carpetaArg = *carpetaID
	}
	if loteID != nil {
		var n int
		if err := base.QueryRow(`SELECT COUNT(*) FROM import_lotes WHERE id = ?`, *loteID).Scan(&n); err != nil {
			return Agenda{}, err
		}
		if n == 0 {
			return Agenda{}, errors.New("El lote de importación no existe")
		}
		loteArg = *loteID
	}

	// QueryRow + RETURNING evita 2 viajes a disco (igual que CrearCarpeta).
	var a Agenda
	var hi, hf sql.NullString
	var cid sql.NullInt64
	err := base.QueryRow(
		`INSERT INTO agenda (fecha, hora_inicio, hora_fin, texto, carpeta_id, lote_id)
		 VALUES (?, ?, ?, ?, ?, ?)
		 RETURNING id, fecha, hora_inicio, hora_fin, texto, carpeta_id`,
		fecha, inicioArg, finArg, texto, carpetaArg, loteArg,
	).Scan(&a.ID, &a.Fecha, &hi, &hf, &a.Texto, &cid)
	if err != nil {
		return Agenda{}, err
	}
	a.HoraInicio = hi.String // Null → "" = "Sin hora" en la UI
	a.HoraFin = hf.String
	if cid.Valid {
		id := cid.Int64
		a.CarpetaID = &id
		a.CarpetaNombre = nombreCarpeta(base, id)
	}
	return a, nil
}

// validarAgenda es la regla de negocio en un solo lugar:
// la usan CrearAgenda (backend) y los tests; la UI la espeja para avisar rápido.
func validarAgenda(fecha, horaInicio, horaFin, texto string, carpetaID *int64) error {
	if fecha == "" {
		return errors.New("La fecha es obligatoria (YYYY-MM-DD)")
	}
	if !fechaValida(fecha) {
		return errors.New("Fecha inválida. Usa un día real en formato YYYY-MM-DD")
	}
	sinCarpeta := carpetaID == nil
	if sinCarpeta && texto == "" {
		return errors.New("Escribí una nota o elegí una carpeta")
	}
	if carpetaID != nil && *carpetaID <= 0 {
		return errors.New("La carpeta no existe")
	}
	if horaInicio != "" && !horaValida(horaInicio) {
		return errors.New("Hora de inicio inválida. Usa HH:MM")
	}
	if horaFin != "" && !horaValida(horaFin) {
		return errors.New("Hora de fin inválida. Usa HH:MM")
	}
	if horaFin != "" && horaInicio == "" {
		return errors.New("Si pones hora de fin, poné también la de inicio")
	}
	// 'HH:MM' con ceros se compara como string: "09:30" < "10:00" siempre.
	if horaInicio != "" && horaFin != "" && horaFin <= horaInicio {
		return errors.New("La hora de fin debe ser posterior a la de inicio")
	}
	return nil
}

// fechaValida exige calendario real, no solo forma linda:
// "2026-02-30" parsea la forma pero no existe, y el round-trip lo delata.
func fechaValida(f string) bool {
	t, err := time.Parse("2006-01-02", f)
	if err != nil {
		return false
	}
	return t.Format("2006-01-02") == f
}

// horaValida exige 'HH:MM' de 00:00 a 23:59.
func horaValida(h string) bool {
	if len(h) != 5 || h[2] != ':' {
		return false
	}
	_, err := time.Parse("15:04", h)
	return err == nil
}

// nombreCarpeta resuelve el JOIN en Go para 1 fila.
// Para listas usa ListarAgenda (trae el JOIN en SQL, 1 viaje).
func nombreCarpeta(base *sql.DB, id int64) string {
	var nombre string
	if err := base.QueryRow(`SELECT name FROM folders WHERE id = ?`, id).Scan(&nombre); err != nil {
		return ""
	}
	return nombre
}

// ListarAgenda trae las notas en un rango, ordenadas por día y hora.
// desde/hasta vacíos = sin filtro (la UI siempre los manda, es para tests).
// El LEFT JOIN trae el nombre de la carpeta sin segundo viaje.
func ListarAgenda(base *sql.DB, desde, hasta string) ([]Agenda, error) {
	query := `SELECT a.id, a.fecha, a.hora_inicio, a.hora_fin, a.texto, a.carpeta_id, COALESCE(f.name, '')
	          FROM agenda a LEFT JOIN folders f ON f.id = a.carpeta_id`
	args := []interface{}{}
	filtros := []string{}
	if desde != "" {
		filtros = append(filtros, `a.fecha >= ?`)
		args = append(args, desde)
	}
	if hasta != "" {
		filtros = append(filtros, `a.fecha <= ?`)
		args = append(args, hasta)
	}
	if len(filtros) > 0 {
		query += ` WHERE ` + strings.Join(filtros, ` AND `)
	}
	// Sin hora (NULL) primero: es el plan del día antes que los horarios.
	query += ` ORDER BY a.fecha, a.hora_inicio`

	rows, err := base.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // SIEMPRE cerrar rows o bloqueas el archivo .db

	out := []Agenda{} // slice vacío (no nil) para que el JSON sea [] y no null
	for rows.Next() {
		var a Agenda
		var hi, hf sql.NullString
		var cid sql.NullInt64
		if err := rows.Scan(&a.ID, &a.Fecha, &hi, &hf, &a.Texto, &cid, &a.CarpetaNombre); err != nil {
			return nil, err
		}
		a.HoraInicio = hi.String
		a.HoraFin = hf.String
		if cid.Valid {
			id := cid.Int64
			a.CarpetaID = &id
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// BorrarAgenda elimina una nota por id.
// Devuelve true si existía, false si no había nada que borrar (igual que BorrarCarpeta).
func BorrarAgenda(base *sql.DB, id int64) (bool, error) {
	res, err := base.Exec(`DELETE FROM agenda WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ActividadPorDia cuenta creaciones por día (recursos + carpetas).
// UNION ALL y no 2 queries: un solo viaje agrupa todo por fecha.
// desde/hasta vacíos = todo el historial.
func ActividadPorDia(base *sql.DB, desde, hasta string) ([]Actividad, error) {
	query := `SELECT fecha, SUM(n) AS total FROM (
	            SELECT DATE(created_at) AS fecha, COUNT(*) AS n FROM resources GROUP BY DATE(created_at)
	            UNION ALL
	            SELECT DATE(created_at) AS fecha, COUNT(*) AS n FROM folders GROUP BY DATE(created_at)
	          ) WHERE fecha IS NOT NULL`
	args := []interface{}{}
	if desde != "" {
		query += ` AND fecha >= ?`
		args = append(args, desde)
	}
	if hasta != "" {
		query += ` AND fecha <= ?`
		args = append(args, hasta)
	}
	query += ` GROUP BY fecha ORDER BY fecha`

	rows, err := base.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Actividad{}
	for rows.Next() {
		var a Actividad
		if err := rows.Scan(&a.Fecha, &a.Total); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
