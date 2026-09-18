package db

// import_lotes_test.go — Import batches under the lens, never your real .db.
// Each test opens its own file in t.TempDir() (see basePrueba in db_test.go).
// Table-driven per go-testing skill: cases named by scenario, not input.

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestCrearListarContarLotes(t *testing.T) {
	base := basePrueba(t)

	l1, err := CrearLote(base, "Cursada 2026", "## Compromiso: Física I\n- Tipo: puntual\n- Fecha: 2026-04-10\n- Horario: 10-11\n")
	if err != nil || l1.ID == 0 {
		t.Fatalf("CrearLote: %+v err=%v", l1, err)
	}
	if l1.Nombre != "Cursada 2026" || l1.Creado == "" {
		t.Fatalf("lote mal: %+v", l1)
	}
	l2, err := CrearLote(base, "  Parciales  ", "## Compromiso: Parcial\n- Tipo: puntual\n- Fecha: 2026-05-01\n- Horario: 9-10\n")
	if err != nil {
		t.Fatalf("CrearLote 2: %v", err)
	}
	// Name is trimmed on the way in.
	if l2.Nombre != "Parciales" {
		t.Fatalf("nombre debió recortarse, salió %q", l2.Nombre)
	}

	// Two rows owned by l1, one by l2, one loose (by hand, lote_id NULL).
	id1 := l1.ID
	if _, err := CrearAgendaEnLote(base, "2026-04-10", "10:00", "11:00", "Física I", nil, &id1); err != nil {
		t.Fatalf("fila lote 1: %v", err)
	}
	if _, err := CrearAgendaEnLote(base, "2026-04-11", "10:00", "11:00", "Física I", nil, &id1); err != nil {
		t.Fatalf("fila lote 1: %v", err)
	}
	id2 := l2.ID
	if _, err := CrearAgendaEnLote(base, "2026-05-01", "09:00", "10:00", "Parcial", nil, &id2); err != nil {
		t.Fatalf("fila lote 2: %v", err)
	}
	if _, err := CrearAgenda(base, "2026-05-02", "", "", "Nota suelta", nil); err != nil {
		t.Fatalf("fila suelta: %v", err)
	}

	if n, _ := ContarPorLote(base, l1.ID); n != 2 {
		t.Fatalf("lote 1 debió contar 2, contó %d", n)
	}
	if n, _ := ContarPorLote(base, l2.ID); n != 1 {
		t.Fatalf("lote 2 debió contar 1, contó %d", n)
	}

	lotes, err := ListarLotes(base)
	if err != nil || len(lotes) != 2 {
		t.Fatalf("listar mal: %+v err=%v", lotes, err)
	}
	// Newest first: l2 before l1.
	if lotes[0].ID != l2.ID || lotes[0].Total != 1 {
		t.Fatalf("orden/cuenta mal en [0]: %+v", lotes[0])
	}
	if lotes[1].ID != l1.ID || lotes[1].Total != 2 {
		t.Fatalf("orden/cuenta mal en [1]: %+v", lotes[1])
	}
	// Detail carries markdown + live count.
	uno, existe, err := ObtenerLote(base, l1.ID)
	if err != nil || !existe {
		t.Fatalf("obtener mal: %+v existe=%v err=%v", uno, existe, err)
	}
	if uno.Total != 2 || uno.Markdown == "" || uno.Nombre != "Cursada 2026" {
		t.Fatalf("detalle mal: %+v", uno)
	}
	if _, existe, _ := ObtenerLote(base, 9999); existe {
		t.Fatal("lote ausente debió dar existe=false")
	}
}

func TestCrearLoteValidacion(t *testing.T) {
	base := basePrueba(t)
	casos := []struct {
		nombre     string
		lote       string
		markdown   string
		debeFallar bool
	}{
		{"nombre y markdown sanos", "Cursada", "## Compromiso: X", false},
		{"nombre vacío", "   ", "## Compromiso: X", true},
		{"markdown vacío", "Cursada", "   ", true},
		{"ambos vacíos", "", "", true},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			_, err := CrearLote(base, tt.lote, tt.markdown)
			if tt.debeFallar && err == nil {
				t.Fatal("debió fallar y pasó")
			}
			if !tt.debeFallar && err != nil {
				t.Fatalf("debió pasar y falló: %v", err)
			}
		})
	}
}

func TestActualizarLote(t *testing.T) {
	base := basePrueba(t)
	l, err := CrearLote(base, "Viejo", "## Compromiso: A\n- Tipo: puntual\n- Fecha: 2026-04-10\n- Horario: 10-11\n")
	if err != nil {
		t.Fatalf("CrearLote: %v", err)
	}
	id := l.ID
	if _, err := CrearAgendaEnLote(base, "2026-04-10", "10:00", "11:00", "A", nil, &id); err != nil {
		t.Fatalf("fila: %v", err)
	}

	// Rename + new markdown keeps the id and its rows.
	act, existe, err := ActualizarLote(base, l.ID, "Nuevo", "## Compromiso: B\n- Tipo: puntual\n- Fecha: 2026-06-01\n- Horario: 8-9\n")
	if err != nil || !existe {
		t.Fatalf("actualizar mal: %+v existe=%v err=%v", act, existe, err)
	}
	if act.ID != l.ID || act.Nombre != "Nuevo" || act.Total != 1 {
		t.Fatalf("actualizado mal: %+v", act)
	}
	if act.Markdown == l.Markdown {
		t.Fatal("el markdown debió cambiar")
	}

	// Missing id reports existe=false, empty name is an error.
	if _, existe, _ := ActualizarLote(base, 9999, "X", "## Compromiso: X"); existe {
		t.Fatal("lote ausente debió dar existe=false")
	}
	if _, _, err := ActualizarLote(base, l.ID, "  ", "## Compromiso: X"); err == nil {
		t.Fatal("nombre vacío debió fallar")
	}
}

func TestBorrarLoteSoloTocaLoSuyo(t *testing.T) {
	base := basePrueba(t)
	l1, _ := CrearLote(base, "Uno", "## Compromiso: A")
	l2, _ := CrearLote(base, "Dos", "## Compromiso: B")
	id1, id2 := l1.ID, l2.ID
	_, _ = CrearAgendaEnLote(base, "2026-04-10", "10:00", "11:00", "A1", nil, &id1)
	_, _ = CrearAgendaEnLote(base, "2026-04-11", "10:00", "11:00", "A2", nil, &id1)
	_, _ = CrearAgendaEnLote(base, "2026-04-12", "10:00", "11:00", "B1", nil, &id2)
	suelta, _ := CrearAgenda(base, "2026-04-13", "", "", "Suelta", nil)

	// Explicit per-batch wipe (what PUT uses before re-expanding).
	if n, err := BorrarAgendaPorLote(base, l1.ID); err != nil || n != 2 {
		t.Fatalf("borrado por lote mal: n=%d err=%v", n, err)
	}
	if n, _ := ContarPorLote(base, l1.ID); n != 0 {
		t.Fatalf("lote 1 debió quedar en 0, quedó en %d", n)
	}
	// The lote itself survives the wipe; other rows are intact.
	if _, existe, _ := ObtenerLote(base, l1.ID); !existe {
		t.Fatal("el lote debió sobrevivir al borrado de sus filas")
	}
	quedan, _ := ListarAgenda(base, "", "")
	if len(quedan) != 2 {
		t.Fatalf("debieron quedar 2 (lote 2 + suelta), quedaron %+v", quedan)
	}

	// Full delete removes lote + its rows, nothing else.
	borradas, existe, err := BorrarLote(base, l2.ID)
	if err != nil || !existe || borradas != 1 {
		t.Fatalf("borrar mal: borradas=%d existe=%v err=%v", borradas, existe, err)
	}
	quedan, _ = ListarAgenda(base, "", "")
	if len(quedan) != 1 || quedan[0].ID != suelta.ID {
		t.Fatalf("solo la suelta debió sobrevivir: %+v", quedan)
	}
	if _, existe, _ := ObtenerLote(base, l2.ID); existe {
		t.Fatal("el lote debió desaparecer")
	}
	// Second delete reports existe=false, no error.
	if _, existe, _ := BorrarLote(base, l2.ID); existe {
		t.Fatal("segundo borrar debió dar existe=false")
	}
	// Deleting an empty lote still removes it with borradas=0.
	l3, _ := CrearLote(base, "Vacío", "## Compromiso: C")
	if n, existe, _ := BorrarLote(base, l3.ID); !existe || n != 0 {
		t.Fatalf("lote vacío mal: n=%d existe=%v", n, existe)
	}
}

func TestCrearAgendaEnLoteValida(t *testing.T) {
	base := basePrueba(t)
	l, _ := CrearLote(base, "Uno", "## Compromiso: A")
	bueno := l.ID
	malo := int64(9999)

	casos := []struct {
		nombre     string
		loteID     *int64
		debeFallar bool
	}{
		{"nota suelta sin lote", nil, false},
		{"lote existente", &bueno, false},
		{"lote inexistente", &malo, true},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			_, err := CrearAgendaEnLote(base, "2026-04-10", "10:00", "11:00", "X", nil, tt.loteID)
			if tt.debeFallar && err == nil {
				t.Fatal("debió fallar y pasó")
			}
			if !tt.debeFallar && err != nil {
				t.Fatalf("debió pasar y falló: %v", err)
			}
		})
	}
}

// TestMigracionNoRompeDBVieja replays an old .db (no progreso, no
// import_lotes, no lote_id) through Abrir: old rows must survive and
// the new columns must work right after.
func TestMigracionNoRompeDBVieja(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "bythos.db")
	vieja, err := sql.Open("sqlite", ruta)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	viejas := []string{
		`CREATE TABLE folders (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, created_at DATETIME DEFAULT CURRENT_TIMESTAMP);`,
		`CREATE TABLE resources (id INTEGER PRIMARY KEY AUTOINCREMENT, folder_id INTEGER NOT NULL REFERENCES folders(id) ON DELETE CASCADE, url TEXT NOT NULL, title TEXT NOT NULL DEFAULT '', image TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', content_type TEXT NOT NULL DEFAULT 'otro', status TEXT NOT NULL DEFAULT 'pendiente', created_at DATETIME DEFAULT CURRENT_TIMESTAMP);`,
		`CREATE TABLE agenda (id INTEGER PRIMARY KEY AUTOINCREMENT, fecha TEXT NOT NULL, hora_inicio TEXT NULL, hora_fin TEXT NULL, texto TEXT NOT NULL DEFAULT '', carpeta_id INTEGER NULL REFERENCES folders(id) ON DELETE SET NULL);`,
		`INSERT INTO folders (name) VALUES ('React');`,
		`INSERT INTO agenda (fecha, hora_inicio, texto) VALUES ('2026-04-10', '10:00', 'Nota vieja');`,
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

	// Old agenda row survived the migration.
	notas, err := ListarAgenda(base, "", "")
	if err != nil || len(notas) != 1 || notas[0].Texto != "Nota vieja" {
		t.Fatalf("la nota vieja debió sobrevivir: %+v err=%v", notas, err)
	}
	// New world works: lote + linked row, and Abrir stays idempotent.
	l, err := CrearLote(base, "Nuevo", "## Compromiso: X")
	if err != nil {
		t.Fatalf("CrearLote post-migración: %v", err)
	}
	id := l.ID
	if _, err := CrearAgendaEnLote(base, "2026-05-01", "09:00", "10:00", "Nueva", nil, &id); err != nil {
		t.Fatalf("fila con lote post-migración: %v", err)
	}
	if err := migrarImportLotes(base); err != nil {
		t.Fatalf("migración repetida debió ser no-op: %v", err)
	}
	lotes, err := ListarLotes(base)
	if err != nil || len(lotes) != 1 || lotes[0].Total != 1 {
		t.Fatalf("lotes post-migración mal: %+v err=%v", lotes, err)
	}
}
