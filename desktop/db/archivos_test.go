package db

// archivos_test.go — Metadatos de archivos bajo lupa: dedupe por sha256,
// refcount para saber cuándo borrar el blob, y que una .db vieja (sin
// archivo_id) migre sin romper nada. Mismo estilo que db_test.go: cada
// test abre su .db aislado en t.TempDir().

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

func TestCrearArchivoDedupePorSHA256(t *testing.T) {
	base := basePrueba(t)

	a, err := CrearArchivo(base, "abc123", "notas.pdf", "application/pdf", 1024, "ab/abc123.pdf")
	if err != nil {
		t.Fatalf("CrearArchivo: %v", err)
	}
	if a.ID == 0 || a.SHA256 != "abc123" {
		t.Fatalf("archivo mal creado: %+v", a)
	}

	// Mismo sha256, otro nombre/carpeta: dedupe, misma fila (mismo ID),
	// el nombre/mime originales de la PRIMERA subida ganan.
	b, err := CrearArchivo(base, "abc123", "copia.pdf", "application/pdf", 1024, "ab/abc123.pdf")
	if err != nil {
		t.Fatalf("CrearArchivo dedupe: %v", err)
	}
	if b.ID != a.ID || b.NombreOriginal != "notas.pdf" {
		t.Fatalf("dedupe debió reusar la fila: primero=%+v segundo=%+v", a, b)
	}

	// Sha256 distinto: fila nueva.
	c, err := CrearArchivo(base, "def456", "video.mp4", "video/mp4", 2048, "de/def456.mp4")
	if err != nil {
		t.Fatalf("CrearArchivo distinto: %v", err)
	}
	if c.ID == a.ID {
		t.Fatal("sha256 distinto debió crear una fila nueva")
	}
}

// TestCrearArchivoConcurrenteMismoSHA256NoRompeElUnique cubre el MAJOR:
// el viejo CrearArchivo hacía un SELECT y, si no encontraba nada, un
// INSERT — dos llamadas SEPARADAS. SetMaxOpenConns(1) (ver comentario en
// archivos.go) serializa cada Query/Exec individual, pero NO la secuencia
// completa: la conexión se libera entre el SELECT y el INSERT, así que 30
// goroutines podían intercalar sus SELECT (ninguna ve la fila de la otra
// todavía) y terminar todas haciendo INSERT del mismo sha256, reventando
// contra "UNIQUE constraint failed: archivos.sha256". Con INSERT ... ON
// CONFLICT DO NOTHING RETURNING en un solo statement, esa ventana
// desaparece. Antes del fix, este test es flaky-pero-reproducible: con 30
// goroutines casi siempre dispara el UNIQUE al menos una vez.
func TestCrearArchivoConcurrenteMismoSHA256NoRompeElUnique(t *testing.T) {
	base := basePrueba(t)
	const n = 30

	var wg sync.WaitGroup
	ids := make([]int64, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a, err := CrearArchivo(base, "sha-concurrente", fmt.Sprintf("archivo-%d.pdf", i), "application/pdf", 100, "sh/sha-concurrente.pdf")
			ids[i] = a.ID
			errs[i] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("CrearArchivo concurrente #%d falló: %v", i, err)
		}
	}
	primero := ids[0]
	if primero == 0 {
		t.Fatal("ID devuelto fue 0")
	}
	for i, id := range ids {
		if id != primero {
			t.Fatalf("todas las llamadas debieron devolver el mismo ID: #%d dio %d, esperaba %d", i, id, primero)
		}
	}

	var total int
	if err := base.QueryRow(`SELECT COUNT(*) FROM archivos WHERE sha256 = ?`, "sha-concurrente").Scan(&total); err != nil {
		t.Fatalf("contar filas: %v", err)
	}
	if total != 1 {
		t.Fatalf("debió quedar exactamente 1 fila para el sha256, quedaron %d", total)
	}
}

func TestCrearArchivoDatosInvalidos(t *testing.T) {
	base := basePrueba(t)
	casos := []struct {
		nombre                             string
		sha256, mime, nombreOriginal, ruta string
	}{
		{"sin sha256", "", "application/pdf", "a.pdf", "ab/x.pdf"},
		{"sin mime", "abc", "", "a.pdf", "ab/x.pdf"},
		{"sin ruta", "abc", "application/pdf", "a.pdf", ""},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			if _, err := CrearArchivo(base, tt.sha256, tt.nombreOriginal, tt.mime, 10, tt.ruta); err == nil {
				t.Fatal("debió fallar y pasó")
			}
		})
	}
}

func TestObtenerArchivoInexistente(t *testing.T) {
	base := basePrueba(t)
	_, ok, err := ObtenerArchivo(base, 9999)
	if err != nil || ok {
		t.Fatalf("archivo inexistente debió dar ok=false sin error: ok=%v err=%v", ok, err)
	}
}

func TestGuardarArchivoYRefcount(t *testing.T) {
	base := basePrueba(t)
	c, _ := CrearCarpeta(base, "React")
	a, err := CrearArchivo(base, "sha-1", "apuntes.pdf", "application/pdf", 500, "sh/sha-1.pdf")
	if err != nil {
		t.Fatalf("CrearArchivo: %v", err)
	}

	// Tipo inválido para un recurso de archivo (esos 4 valores están
	// reservados, "youtube"/"articulo"/"otro" son de LINKS).
	if _, err := GuardarArchivo(base, c.ID, a.ID, "Apuntes", "youtube"); err == nil {
		t.Fatal("tipo de link no debió aceptarse para un archivo")
	}

	r1, err := GuardarArchivo(base, c.ID, a.ID, "Apuntes", "pdf")
	if err != nil {
		t.Fatalf("GuardarArchivo: %v", err)
	}
	if r1.ArchivoID != a.ID || r1.URL != "archivo:"+strconv.FormatInt(a.ID, 10) || r1.Estado != EstadoPendiente {
		t.Fatalf("recurso de archivo mal creado: %+v", r1)
	}

	// Sin referencias todavía sería 0; con 1 recurso, 1.
	n, err := ContarRecursosPorArchivo(base, a.ID)
	if err != nil || n != 1 {
		t.Fatalf("refcount debió ser 1: n=%d err=%v", n, err)
	}

	// Un segundo recurso puede compartir el mismo archivo (dedupe real:
	// dos carpetas distintas subiendo el mismo PDF).
	r2, err := GuardarArchivo(base, c.ID, a.ID, "Apuntes (copia)", "pdf")
	if err != nil {
		t.Fatalf("segundo GuardarArchivo: %v", err)
	}
	n, err = ContarRecursosPorArchivo(base, a.ID)
	if err != nil || n != 2 {
		t.Fatalf("refcount debió ser 2: n=%d err=%v", n, err)
	}

	// Borrar uno de los dos: sigue habiendo 1 referencia, el archivo NO
	// está huérfano todavía.
	if err := Borrar(base, r1.ID); err != nil {
		t.Fatalf("Borrar: %v", err)
	}
	n, err = ContarRecursosPorArchivo(base, a.ID)
	if err != nil || n != 1 {
		t.Fatalf("refcount tras borrar uno debió ser 1: n=%d err=%v", n, err)
	}

	// Borrar el último: ahora sí, 0 referencias (el caller, en api/, es
	// quien decide borrar el blob + la fila en este punto).
	if err := Borrar(base, r2.ID); err != nil {
		t.Fatalf("Borrar: %v", err)
	}
	n, err = ContarRecursosPorArchivo(base, a.ID)
	if err != nil || n != 0 {
		t.Fatalf("refcount tras borrar todos debió ser 0: n=%d err=%v", n, err)
	}

	// BorrarArchivo limpia la fila de metadatos (el blob en disco lo
	// borra el paquete archivos, sin SQL de por medio).
	if err := BorrarArchivo(base, a.ID); err != nil {
		t.Fatalf("BorrarArchivo: %v", err)
	}
	if _, ok, _ := ObtenerArchivo(base, a.ID); ok {
		t.Fatal("archivo debió desaparecer tras BorrarArchivo")
	}
}

// TestMigracionArchivoIDNoRompeDBVieja replays a .db from before v1.2
// (sin la tabla archivos ni resources.archivo_id) through Abrir: filas
// viejas sobreviven, y el mundo nuevo (archivo_id) funciona después.
// Mismo patrón que TestMigracionNoRompeDBVieja en import_lotes_test.go.
func TestMigracionArchivoIDNoRompeDBVieja(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "bythos.db")
	vieja, err := sql.Open("sqlite", ruta)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	viejas := []string{
		`CREATE TABLE folders (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, created_at DATETIME DEFAULT CURRENT_TIMESTAMP);`,
		`CREATE TABLE resources (id INTEGER PRIMARY KEY AUTOINCREMENT, folder_id INTEGER NOT NULL REFERENCES folders(id) ON DELETE CASCADE, url TEXT NOT NULL, title TEXT NOT NULL DEFAULT '', image TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', content_type TEXT NOT NULL DEFAULT 'otro', status TEXT NOT NULL DEFAULT 'pendiente', progreso INTEGER NOT NULL DEFAULT 0, created_at DATETIME DEFAULT CURRENT_TIMESTAMP);`,
		`CREATE TABLE agenda (id INTEGER PRIMARY KEY AUTOINCREMENT, fecha TEXT NOT NULL, hora_inicio TEXT NULL, hora_fin TEXT NULL, texto TEXT NOT NULL DEFAULT '', carpeta_id INTEGER NULL REFERENCES folders(id) ON DELETE SET NULL);`,
		`INSERT INTO folders (name) VALUES ('React');`,
		`INSERT INTO resources (folder_id, url, title) VALUES (1, 'http://a/1', 'Link viejo');`,
	}
	for _, q := range viejas {
		if _, err := vieja.Exec(q); err != nil {
			vieja.Close()
			t.Fatalf("esquema viejo: %v", err)
		}
	}
	vieja.Close()

	base, err := Abrir(ruta)
	if err != nil {
		t.Fatalf("Abrir sobre .db viejo: %v", err)
	}
	defer base.Close()

	// El link viejo sobrevivió, y su archivo_id lee 0 (NULL): sigue
	// siendo un link, no un archivo.
	viejos, err := ListarRecursos(base, 0)
	if err != nil || len(viejos) != 1 || viejos[0].Titulo != "Link viejo" || viejos[0].ArchivoID != 0 {
		t.Fatalf("el recurso viejo debió sobrevivir con ArchivoID=0: %+v err=%v", viejos, err)
	}

	// El mundo nuevo funciona post-migración.
	a, err := CrearArchivo(base, "post-mig", "nuevo.pdf", "application/pdf", 10, "po/post-mig.pdf")
	if err != nil {
		t.Fatalf("CrearArchivo post-migración: %v", err)
	}
	r, err := GuardarArchivo(base, 1, a.ID, "Nuevo", "pdf")
	if err != nil || r.ArchivoID != a.ID {
		t.Fatalf("GuardarArchivo post-migración: %+v err=%v", r, err)
	}

	// La migración repetida es no-op (idempotente).
	if err := migrarArchivoID(base); err != nil {
		t.Fatalf("migración repetida debió ser no-op: %v", err)
	}
}
