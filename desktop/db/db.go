package db

// db.go — La MEMORIA de Bythos.
// Todo lo que sabe SQLite vive aquí y en ningún otro lado.
//
// Regla senior: si ves SQL fuera de esta carpeta, está mal.
// api/ y main.go NUNCA escriben SQL directo, siempre llaman a estas funciones.

import (
	"database/sql"
	"os"
	"path/filepath"

	// Driver SQLite puro en Go (sin CGO ni gcc).
	// ¿Por qué este y no mattn/go-sqlite3?
	// Porque en Windows mattn te pide instalar gcc y rompe el .exe portable.
	// modernc.org/sqlite compila en cualquier PC con solo `go build`.
	_ "modernc.org/sqlite"
)

// RutaPorDefecto devuelve dónde vive bythos.db.
// ¿Por qué NO al lado del .exe?
// Porque si instalas en "Archivos de Programa", Windows bloquea escritura ahí.
// AppData (%APPDATA%/Bythos/bythos.db) SIEMPRE se puede escribir.
// En Linux/Mac usa ~/.config/bythos/bythos.db automáticamente.
func RutaPorDefecto() string {
	base, err := os.UserConfigDir()
	if err != nil {
		// Plan B: carpeta actual (solo para no romper en PCs raros)
		return "bythos.db"
	}
	dir := filepath.Join(base, "Bythos")
	// MkdirAll no falla si ya existe, así que es seguro llamarlo siempre
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "bythos.db")
}

// Abrir conecta al archivo SQLite y deja las tablas listas.
// Lógica en 4 pasos, siempre en este orden:
//  1. sql.Open (no conecta aún, solo prepara)
//  2. Ping (aquí sí toca el archivo y lo crea si no existe)
//  3. PRAGMAs (reglas de SQLite: llaves foráneas + modo WAL)
//  4. crearTablas (CREATE TABLE IF NOT EXISTS, seguro de repetir)
func Abrir(ruta string) (*sql.DB, error) {
	base, err := sql.Open("sqlite", ruta)
	if err != nil {
		return nil, err
	}

	// SQLite en un .exe de escritorio solo lo usa 1 persona a la vez,
	// con 1 conexión basta y evitas bloqueos del archivo.
	base.SetMaxOpenConns(1)

	if err := base.Ping(); err != nil {
		base.Close()
		return nil, err
	}

	// Activa ON DELETE CASCADE (si borras carpeta, se borran sus recursos)
	if _, err := base.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		base.Close()
		return nil, err
	}
	// WAL = mejor contra apagones y no bloquea lecturas mientras escribes
	if _, err := base.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		base.Close()
		return nil, err
	}

	if err := crearTablas(base); err != nil {
		base.Close()
		return nil, err
	}
	return base, nil
}

// crearTablas define el modelo mínimo. Solo 2 tablas, a propósito:
// - folders: tus temas ("Go Backend", "React"...)
// - resources: tus links con su estado de estudio
//
// ¿Por qué NO hay tabla users como antes con Postgres?
// Porque es app de escritorio de 1 usuario: TÚ. No hay login,
// no hay JWT, no hay bcrypt. El .db ya es tuyo en tu disco.
func crearTablas(base *sql.DB) error {
	tablas := []string{
		`CREATE TABLE IF NOT EXISTS folders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS resources (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			folder_id INTEGER NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
			url TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			image TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			content_type TEXT NOT NULL DEFAULT 'otro',
			status TEXT NOT NULL DEFAULT 'pendiente',
			progreso INTEGER NOT NULL DEFAULT 0 CHECK(progreso BETWEEN 0 AND 100),
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
	}
	for _, q := range tablas {
		if _, err := base.Exec(q); err != nil {
			return err
		}
	}
	return migrarProgreso(base)
}

// migrarProgreso adds resources.progreso on databases created before v0.2.
// New installs already have the column via CREATE TABLE above.
// Check PRAGMA table_info first: ALTER TABLE fails if the column exists,
// so the check keeps Abrir idempotent.
func migrarProgreso(base *sql.DB) error {
	rows, err := base.Query(`PRAGMA table_info(resources)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == "progreso" {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = base.Exec(`ALTER TABLE resources ADD COLUMN progreso INTEGER NOT NULL DEFAULT 0 CHECK(progreso BETWEEN 0 AND 100)`)
	if err != nil {
		return err
	}
	// Backfill: rows completed before the column existed are 100% by definition.
	_, err = base.Exec(`UPDATE resources SET progreso = 100 WHERE status = 'completado'`)
	return err
}
