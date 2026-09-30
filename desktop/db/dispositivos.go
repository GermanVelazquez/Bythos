package db

// dispositivos.go — Los TELÉFONOS emparejados con Bythos (ver
// openspec/changes/bythos-movil-qr). Cada fila es un celular al que el
// usuario le aprobó el emparejamiento por QR: guarda el hash del token
// (nunca el token en claro, ver desktop/lan/auth.go), no el propio celular.
//
// Mismo trato que archivos.go/eventos.go: tabla nueva (bythos-movil-qr),
// CREATE TABLE IF NOT EXISTS alcanza, sin ALTER TABLE.

import (
	"database/sql"
	"strings"
)

// nombreDispositivoLargoMaximo evita que un nombre de celular gigantesco
// (a propósito o no) infle la fila; 80 caracteres alcanza de sobra para
// el nombre de un teléfono, mismo límite que actorLargoMaximo en
// api/origen.go.
const nombreDispositivoLargoMaximo = 80

// Dispositivo es una fila de dispositivos: un celular emparejado.
// UltimoUso y RevocadoEn van vacíos ("") cuando nunca se tocó o nunca se
// revocó (NULL en la tabla), mismo criterio que Agenda.HoraInicio.
type Dispositivo struct {
	ID         int64
	Nombre     string
	TokenHash  string
	CreadoEn   string
	UltimoUso  string
	RevocadoEn string
}

// crearTablaDispositivos define la tabla. CREATE TABLE IF NOT EXISTS ya
// es idempotente por sí solo (tabla nueva, igual que archivos/eventos):
// una instalación vieja la gana en el próximo Abrir, una nueva la trae
// desde el primer arranque. Va ANTES de crearTablaEventos en crearTablas
// (db.go) porque no depende de ella, y así mantiene el mismo orden que
// documenta el diseño.
func crearTablaDispositivos(base *sql.DB) error {
	_, err := base.Exec(`CREATE TABLE IF NOT EXISTS dispositivos (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		nombre TEXT NOT NULL,
		token_hash TEXT NOT NULL UNIQUE,
		creado_en TEXT NOT NULL DEFAULT (datetime('now','localtime')),
		ultimo_uso TEXT,
		revocado_en TEXT
	);`)
	return err
}

// CrearDispositivo guarda un celular recién aprobado. tokenHash es el
// SHA-256 hex del token (32 bytes crypto/rand, ver desktop/lan/auth.go):
// esta capa nunca ve ni guarda el token en claro, solo su hash.
// nombre se recorta a nombreDispositivoLargoMaximo, mismo criterio que
// actorDeCabeceras en api/origen.go.
func CrearDispositivo(base *sql.DB, nombre, tokenHash string) (Dispositivo, error) {
	nombre = strings.TrimSpace(nombre)
	tokenHash = strings.TrimSpace(tokenHash)
	if nombre == "" || tokenHash == "" {
		return Dispositivo{}, sql.ErrNoRows
	}
	if len(nombre) > nombreDispositivoLargoMaximo {
		nombre = nombre[:nombreDispositivoLargoMaximo]
	}
	var d Dispositivo
	var ultimoUso, revocadoEn sql.NullString
	err := base.QueryRow(
		`INSERT INTO dispositivos (nombre, token_hash) VALUES (?, ?)
		 RETURNING id, nombre, token_hash, creado_en, ultimo_uso, revocado_en`,
		nombre, tokenHash,
	).Scan(&d.ID, &d.Nombre, &d.TokenHash, &d.CreadoEn, &ultimoUso, &revocadoEn)
	if err != nil {
		return Dispositivo{}, err
	}
	d.UltimoUso = ultimoUso.String
	d.RevocadoEn = revocadoEn.String
	return d, nil
}

// DispositivoPorTokenHash busca el celular dueño de un token ya
// hasheado. Excluye revocados a propósito: es lo que usa el middleware
// Bearer (desktop/lan/auth.go) para autenticar cada request, y un
// dispositivo revocado debe fallar como si el token no existiera.
// Segundo valor es false si no hay match (sin error), mismo criterio que
// ObtenerArchivo.
func DispositivoPorTokenHash(base *sql.DB, tokenHash string) (Dispositivo, bool, error) {
	var d Dispositivo
	var ultimoUso, revocadoEn sql.NullString
	err := base.QueryRow(
		`SELECT id, nombre, token_hash, creado_en, ultimo_uso, revocado_en
		   FROM dispositivos WHERE token_hash = ? AND revocado_en IS NULL`,
		tokenHash,
	).Scan(&d.ID, &d.Nombre, &d.TokenHash, &d.CreadoEn, &ultimoUso, &revocadoEn)
	if err == sql.ErrNoRows {
		return Dispositivo{}, false, nil
	}
	if err != nil {
		return Dispositivo{}, false, err
	}
	d.UltimoUso = ultimoUso.String
	d.RevocadoEn = revocadoEn.String
	return d, true, nil
}

// ListarDispositivos trae todos los celulares (emparejados y revocados,
// la UI decide cómo mostrarlos), más nuevo primero, mismo orden que
// ListarEventos.
func ListarDispositivos(base *sql.DB) ([]Dispositivo, error) {
	rows, err := base.Query(
		`SELECT id, nombre, token_hash, creado_en, ultimo_uso, revocado_en
		   FROM dispositivos ORDER BY id DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // SIEMPRE cerrar rows o bloqueas el archivo .db

	out := []Dispositivo{} // slice vacío (no nil) para que el JSON sea [] y no null
	for rows.Next() {
		var d Dispositivo
		var ultimoUso, revocadoEn sql.NullString
		if err := rows.Scan(&d.ID, &d.Nombre, &d.TokenHash, &d.CreadoEn, &ultimoUso, &revocadoEn); err != nil {
			return nil, err
		}
		d.UltimoUso = ultimoUso.String
		d.RevocadoEn = revocadoEn.String
		out = append(out, d)
	}
	return out, rows.Err()
}

// RevocarDispositivo marca un celular como revocado, efectivo de
// inmediato: DispositivoPorTokenHash deja de encontrarlo en la próxima
// consulta. Idempotente a propósito (WHERE revocado_en IS NULL): revocar
// dos veces no pisa la fecha de la primera revocación ni falla.
func RevocarDispositivo(base *sql.DB, id int64) error {
	_, err := base.Exec(
		`UPDATE dispositivos SET revocado_en = datetime('now','localtime')
		  WHERE id = ? AND revocado_en IS NULL`,
		id,
	)
	return err
}

// TocarDispositivo actualiza ultimo_uso, pero como mucho 1 vez por
// minuto: cada request autenticado del celular llamaría esto, y escribir
// a disco en CADA request sería ruido innecesario (SetMaxOpenConns(1) en
// db.go ya serializa cada Exec, no hace falta además golpear WAL a cada
// rato). El throttle vive en el propio WHERE (una sola sentencia,
// atómica) en vez de leer-y-decidir en Go: sin ventana de carrera y sin
// necesitar un reloj falso para testear "todavía no pasó 1 minuto"
// (dispositivos_test.go mueve ultimo_uso al pasado a mano para probar el
// otro lado del throttle).
func TocarDispositivo(base *sql.DB, id int64) error {
	_, err := base.Exec(
		`UPDATE dispositivos SET ultimo_uso = datetime('now','localtime')
		  WHERE id = ? AND (ultimo_uso IS NULL OR ultimo_uso <= datetime('now','localtime','-1 minutes'))`,
		id,
	)
	return err
}
