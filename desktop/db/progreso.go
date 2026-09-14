package db

// progreso.go — El TERMÓMETRO. ¿Cuánto ya estudiaste?
// Solo CUENTA filas, no las trae. Barato aunque tengas 10.000 links.
//
// ¿Por qué archivo propio y no dentro de resources.go?
// Porque responde otra pregunta: resources = "qué tengo",
// progreso = "cómo voy". Pregunta distinta, archivo distinto.

import "database/sql"

// Progreso sirve para la barra por carpeta Y para el gráfico general.
// Un solo struct para ambos: la UI pinta igual (ancho = Porcentaje).
type Progreso struct {
	Total       int
	Completados int
	Pendientes  int
	EnCurso     int
	Porcentaje  int // 0-100, entero: la barra CSS lo usa directo
	Promedio    int // 0-100, AVG(progreso) per link: dashboard real
}

// ProgresoCarpeta cuenta solo una carpeta.
// Carpeta vacía = todo en 0 (sin dividir por cero, sin NaN en la UI).
func ProgresoCarpeta(base *sql.DB, carpetaID int64) (Progreso, error) {
	var p Progreso
	consultas := []struct {
		dest *int
		sql  string
		args []interface{}
	}{
		{&p.Total, `SELECT COUNT(*) FROM resources WHERE folder_id = ?`, []interface{}{carpetaID}},
		{&p.Completados, `SELECT COUNT(*) FROM resources WHERE folder_id = ? AND status = ?`, []interface{}{carpetaID, EstadoCompletado}},
		{&p.Pendientes, `SELECT COUNT(*) FROM resources WHERE folder_id = ? AND status = ?`, []interface{}{carpetaID, EstadoPendiente}},
		{&p.EnCurso, `SELECT COUNT(*) FROM resources WHERE folder_id = ? AND status = ?`, []interface{}{carpetaID, EstadoEnCurso}},
	}
	for _, c := range consultas {
		if err := base.QueryRow(c.sql, c.args...).Scan(c.dest); err != nil {
			return Progreso{}, err
		}
	}
	// Entero a propósito: 1/3 = 33, no 33.333. La barra no necesita decimales
	// y el JSON viaja más limpio. El redondeo hacia abajo es honesto.
	if p.Total > 0 {
		p.Porcentaje = p.Completados * 100 / p.Total
	}
	if prom, err := promedioProgreso(base, `SELECT COALESCE(AVG(progreso), 0) FROM resources WHERE folder_id = ?`, carpetaID); err != nil {
		return Progreso{}, err
	} else {
		p.Promedio = prom
	}
	return p, nil
}

// ProgresoGeneral cuenta TODO (para el gráfico de la portada).
// Misma forma que el de carpeta: la UI reutiliza el pintado.
func ProgresoGeneral(base *sql.DB) (Progreso, error) {
	var p Progreso
	consultas := []struct {
		dest *int
		sql  string
	}{
		{&p.Total, `SELECT COUNT(*) FROM resources`},
		{&p.Completados, `SELECT COUNT(*) FROM resources WHERE status = '` + EstadoCompletado + `'`},
		{&p.Pendientes, `SELECT COUNT(*) FROM resources WHERE status = '` + EstadoPendiente + `'`},
		{&p.EnCurso, `SELECT COUNT(*) FROM resources WHERE status = '` + EstadoEnCurso + `'`},
	}
	for _, c := range consultas {
		if err := base.QueryRow(c.sql).Scan(c.dest); err != nil {
			return Progreso{}, err
		}
	}
	if p.Total > 0 {
		p.Porcentaje = p.Completados * 100 / p.Total
	}
	if prom, err := promedioProgreso(base, `SELECT COALESCE(AVG(progreso), 0) FROM resources`); err != nil {
		return Progreso{}, err
	} else {
		p.Promedio = prom
	}
	return p, nil
}

func promedioProgreso(base *sql.DB, query string, args ...interface{}) (int, error) {
	var avg sql.NullFloat64
	if err := base.QueryRow(query, args...).Scan(&avg); err != nil {
		return 0, err
	}
	if !avg.Valid {
		return 0, nil
	}
	return int(avg.Float64), nil
}
