package db

// archivos.go — Metadatos de los ARCHIVOS que sube el usuario (PDF, video,
// imagen, documento), a diferencia de resources.go que guarda LINKS.
// Este archivo solo sabe de SQL (tabla archivos): el byte a byte en disco
// vive en desktop/archivos (paquete separado, sin SQL). Misma separación
// senior que el resto de db/: memoria tonta acá, lógica de negocio afuera.
//
// Dedupe por sha256: dos subidas del mismo archivo (mismo hash) comparten
// una sola fila y un solo blob en disco. OJO: SetMaxOpenConns(1) (ver
// db.go) NO alcanza para volver esto atómico — sí serializa cada Query/Exec
// individual, pero CrearArchivo hacía dos llamadas separadas (un SELECT y
// después un INSERT) y la conexión se libera entre una y otra. Dos
// goroutines podían intercalar sus SELECT (ninguna ve todavía la fila del
// otro) y las dos terminar haciendo INSERT, la segunda reventando contra el
// UNIQUE de sha256. La solución real es un solo statement atómico: ver
// INSERT ... ON CONFLICT DO NOTHING RETURNING en CrearArchivo.

import (
	"database/sql"
	"strings"
)

// Archivo es una fila de la tabla archivos: el blob que uno o más
// recursos pueden referenciar (ver resources.ArchivoID).
type Archivo struct {
	ID             int64
	SHA256         string
	NombreOriginal string
	Mime           string
	Tamano         int64
	Ruta           string // relativa a archivos.Base(), ej. "ab/ab12...ef.pdf"
	Creado         string
}

// crearTablaArchivos define la tabla. CREATE TABLE IF NOT EXISTS ya es
// idempotente por sí solo (tabla nueva de v1.2, igual que eventos): una
// instalación vieja la gana en el próximo Abrir, una nueva la trae desde
// el primer arranque.
func crearTablaArchivos(base *sql.DB) error {
	_, err := base.Exec(`CREATE TABLE IF NOT EXISTS archivos (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		sha256 TEXT NOT NULL UNIQUE,
		nombre_original TEXT NOT NULL DEFAULT '',
		mime TEXT NOT NULL,
		tamano INTEGER NOT NULL DEFAULT 0,
		ruta TEXT NOT NULL,
		creado TEXT NOT NULL DEFAULT (datetime('now','localtime'))
	);`)
	return err
}

// CrearArchivo guarda los metadatos de un archivo ya escrito en disco
// (ver archivos.Guardar). Si el sha256 ya existe, NO inserta de nuevo:
// devuelve la fila existente (dedupe). El caller (api/) es quien ya sabe,
// por el resultado de archivos.Guardar, si el blob se escribió o se
// reusó; esta función solo asegura que la fila de metadatos sea una sola
// por contenido.
//
// Un solo INSERT ... ON CONFLICT(sha256) DO NOTHING RETURNING resuelve
// esto en UN statement: si el sha256 ya existía, el INSERT no inserta
// nada y el RETURNING no devuelve fila (sql.ErrNoRows), así que caemos al
// SELECT normal para traer la fila existente. Ninguna otra goroutine
// puede colarse ENTRE el intento de insertar y saber si chocó, porque es
// la misma sentencia — a diferencia del SELECT-luego-INSERT viejo (ver
// comentario de arriba).
func CrearArchivo(base *sql.DB, sha256, nombreOriginal, mime string, tamano int64, ruta string) (Archivo, error) {
	sha256 = strings.TrimSpace(sha256)
	ruta = strings.TrimSpace(ruta)
	if sha256 == "" || mime == "" || ruta == "" {
		return Archivo{}, sql.ErrNoRows
	}
	var a Archivo
	err := base.QueryRow(
		`INSERT INTO archivos (sha256, nombre_original, mime, tamano, ruta)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(sha256) DO NOTHING
		 RETURNING id, sha256, nombre_original, mime, tamano, ruta, creado`,
		sha256, nombreOriginal, mime, tamano, ruta,
	).Scan(&a.ID, &a.SHA256, &a.NombreOriginal, &a.Mime, &a.Tamano, &a.Ruta, &a.Creado)
	if err == nil {
		return a, nil
	}
	if err != sql.ErrNoRows {
		return Archivo{}, err
	}
	// ErrNoRows acá significa DO NOTHING: alguien (otra goroutine, u otra
	// subida anterior) ya tiene este sha256. Traemos la fila existente,
	// mismo resultado que el dedupe de siempre.
	existente, ok, err := ObtenerArchivoPorSHA256(base, sha256)
	if err != nil {
		return Archivo{}, err
	}
	if !ok {
		// No debería pasar (el conflicto implica que la fila existe), pero
		// mejor un error claro que un Archivo{} vacío silencioso.
		return Archivo{}, sql.ErrNoRows
	}
	return existente, nil
}

// ObtenerArchivo trae un archivo por id. Segundo valor es false si no
// existe (sin error), mismo criterio que ObtenerRecurso.
func ObtenerArchivo(base *sql.DB, id int64) (Archivo, bool, error) {
	var a Archivo
	err := base.QueryRow(
		`SELECT id, sha256, nombre_original, mime, tamano, ruta, creado FROM archivos WHERE id = ?`, id,
	).Scan(&a.ID, &a.SHA256, &a.NombreOriginal, &a.Mime, &a.Tamano, &a.Ruta, &a.Creado)
	if err == sql.ErrNoRows {
		return Archivo{}, false, nil
	}
	if err != nil {
		return Archivo{}, false, err
	}
	return a, true, nil
}

// ObtenerArchivoPorSHA256 es lo que usa CrearArchivo para decidir
// dedupe: mismo hash, misma fila.
func ObtenerArchivoPorSHA256(base *sql.DB, sha256 string) (Archivo, bool, error) {
	var a Archivo
	err := base.QueryRow(
		`SELECT id, sha256, nombre_original, mime, tamano, ruta, creado FROM archivos WHERE sha256 = ?`, sha256,
	).Scan(&a.ID, &a.SHA256, &a.NombreOriginal, &a.Mime, &a.Tamano, &a.Ruta, &a.Creado)
	if err == sql.ErrNoRows {
		return Archivo{}, false, nil
	}
	if err != nil {
		return Archivo{}, false, err
	}
	return a, true, nil
}

// ContarRecursosPorArchivo cuenta cuántos recursos apuntan a este
// archivo. El caller (api/) lo usa ANTES de borrar un recurso para saber
// si, después de borrarlo, el archivo se queda sin referencias y hay que
// borrar también el blob de disco + esta fila (ver BorrarArchivo).
func ContarRecursosPorArchivo(base *sql.DB, archivoID int64) (int, error) {
	var n int
	err := base.QueryRow(`SELECT COUNT(*) FROM resources WHERE archivo_id = ?`, archivoID).Scan(&n)
	return n, err
}

// BorrarArchivo elimina la fila de metadatos. NO toca el disco: eso es
// archivos.Eliminar, que el caller llama por separado (mismo principio
// que todo db/: memoria tonta, nunca I/O de archivos).
func BorrarArchivo(base *sql.DB, id int64) error {
	_, err := base.Exec(`DELETE FROM archivos WHERE id = ?`, id)
	return err
}
