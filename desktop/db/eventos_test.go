package db

// eventos_test.go — El historial bajo lupa: tabla fresca, tabla sobre un
// .db viejo (sin eventos, camino de actualización) y Registrar/Listar
// con límite y filtro de origen. Mismo estilo que import_lotes_test.go.

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestEventosTablaFresca(t *testing.T) {
	base := basePrueba(t)
	if err := RegistrarEvento(base, OrigenApp, "", AccionCarpetaCreada, 1, "React"); err != nil {
		t.Fatalf("RegistrarEvento en DB fresca: %v", err)
	}
	eventos, err := ListarEventos(base, FiltroEventos{})
	if err != nil || len(eventos) != 1 {
		t.Fatalf("listar mal: %+v err=%v", eventos, err)
	}
	if eventos[0].Origen != OrigenApp || eventos[0].Accion != AccionCarpetaCreada || eventos[0].Detalle != "React" {
		t.Fatalf("evento mal: %+v", eventos[0])
	}
}

// TestEventosMigracionDBVieja replays a .db without the eventos table
// (pre-historial installs, solo folders) through Abrir: la tabla debe
// aparecer lista para usar en el mismo Abrir, sin tocar lo que ya había.
func TestEventosMigracionDBVieja(t *testing.T) {
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

	// La carpeta vieja sobrevivió a la migración.
	carpetas, err := ListarCarpetas(base)
	if err != nil || len(carpetas) != 1 || carpetas[0].Nombre != "React" {
		t.Fatalf("la carpeta vieja debió sobrevivir: %+v err=%v", carpetas, err)
	}

	// Mundo nuevo funciona: se puede registrar y listar sin ALTER a mano.
	if err := RegistrarEvento(base, OrigenAgente, "claude-code", AccionRecursoGuardado, 5, "Hooks"); err != nil {
		t.Fatalf("RegistrarEvento post-migración: %v", err)
	}
	eventos, err := ListarEventos(base, FiltroEventos{})
	if err != nil || len(eventos) != 1 {
		t.Fatalf("listar post-migración mal: %+v err=%v", eventos, err)
	}
	if eventos[0].Origen != OrigenAgente || eventos[0].Actor != "claude-code" {
		t.Fatalf("evento mal: %+v", eventos[0])
	}

	// Idempotente: reabrir el mismo .db no debe fallar (CREATE TABLE IF
	// NOT EXISTS, misma regla que crearTablaEventos exige).
	base.Close()
	base2, err := Abrir(ruta)
	if err != nil {
		t.Fatalf("segundo Abrir debió ser no-op: %v", err)
	}
	defer base2.Close()
	otra, err := ListarEventos(base2, FiltroEventos{})
	if err != nil || len(otra) != 1 {
		t.Fatalf("el evento debió sobrevivir al segundo Abrir: %+v err=%v", otra, err)
	}
}

func TestListarEventosLimiteYOrigen(t *testing.T) {
	base := basePrueba(t)
	for i := 0; i < 5; i++ {
		origen := OrigenApp
		if i%2 == 0 {
			origen = OrigenAgente
		}
		if err := RegistrarEvento(base, origen, "", AccionEstadoCambiado, int64(i), "x"); err != nil {
			t.Fatalf("RegistrarEvento: %v", err)
		}
	}
	todos, err := ListarEventos(base, FiltroEventos{})
	if err != nil || len(todos) != 5 {
		t.Fatalf("sin filtro debía traer 5: %+v err=%v", todos, err)
	}
	// Más nuevo primero: el último insertado (i=4) va en [0].
	if todos[0].EntidadID != 4 {
		t.Fatalf("orden mal, primero debía ser el último insertado: %+v", todos[0])
	}

	limitados, err := ListarEventos(base, FiltroEventos{Limite: 2})
	if err != nil || len(limitados) != 2 {
		t.Fatalf("límite mal: %+v err=%v", limitados, err)
	}

	agentes, err := ListarEventos(base, FiltroEventos{Origen: OrigenAgente})
	if err != nil || len(agentes) != 3 {
		t.Fatalf("filtro origen mal: %+v err=%v", agentes, err)
	}
	for _, e := range agentes {
		if e.Origen != OrigenAgente {
			t.Fatalf("filtro trajo origen ajeno: %+v", e)
		}
	}
}

func TestRegistrarEventoOrigenDesconocido(t *testing.T) {
	base := basePrueba(t)
	if err := RegistrarEvento(base, "quien-sabe", "", AccionCarpetaCreada, 1, "x"); err != nil {
		t.Fatalf("RegistrarEvento: %v", err)
	}
	eventos, err := ListarEventos(base, FiltroEventos{})
	if err != nil || len(eventos) != 1 || eventos[0].Origen != OrigenDesconocido {
		t.Fatalf("origen ajeno debía caer a desconocido: %+v err=%v", eventos, err)
	}
}

// TestNormalizarOrigenAceptaCelular cubre bythos-movil-qr: la BD (última
// defensa) acepta "celular" igual que app/extension/agente, aunque
// api/origen.go nunca lo mande desde una cabecera (ver comentario en
// OrigenCelular). Cualquier variante de mayúsculas/espacios normaliza
// igual que los otros tres orígenes.
func TestNormalizarOrigenAceptaCelular(t *testing.T) {
	base := basePrueba(t)
	if err := RegistrarEvento(base, "  CELULAR  ", "Pixel de Ana", AccionArchivoSubido, 1, "video.mp4"); err != nil {
		t.Fatalf("RegistrarEvento: %v", err)
	}
	eventos, err := ListarEventos(base, FiltroEventos{})
	if err != nil || len(eventos) != 1 {
		t.Fatalf("listar mal: %+v err=%v", eventos, err)
	}
	if eventos[0].Origen != OrigenCelular || eventos[0].Actor != "Pixel de Ana" {
		t.Fatalf("origen celular mal normalizado: %+v", eventos[0])
	}

	filtrados, err := ListarEventos(base, FiltroEventos{Origen: "celular"})
	if err != nil || len(filtrados) != 1 {
		t.Fatalf("filtro por origen=celular mal: %+v err=%v", filtrados, err)
	}
}

// TestNormalizarOrigenCeceraMalformadaCaeADesconocido cubre el segundo
// escenario del spec activity-history (Unknown origin header): una
// cabecera malformada o ausente normaliza a desconocido y el evento se
// guarda igual, nunca se pierde la fila del historial.
func TestNormalizarOrigenCeceraMalformadaCaeADesconocido(t *testing.T) {
	base := basePrueba(t)
	casos := []string{"", "  ", "celular-falso", "CELULAR2"}
	for _, o := range casos {
		if err := RegistrarEvento(base, o, "", AccionCarpetaCreada, 1, "x"); err != nil {
			t.Fatalf("RegistrarEvento(%q): %v", o, err)
		}
	}
	eventos, err := ListarEventos(base, FiltroEventos{})
	if err != nil || len(eventos) != len(casos) {
		t.Fatalf("listar mal: %+v err=%v", eventos, err)
	}
	for _, e := range eventos {
		if e.Origen != OrigenDesconocido {
			t.Fatalf("cabecera malformada debía caer a desconocido: %+v", e)
		}
	}
}
