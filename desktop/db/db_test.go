package db

// db_test.go — La memoria bajo lupa, sin tocar tu .db real.
// Cada test abre su propio archivo en t.TempDir() y lo cierra solo.
// Si un test falla, tu Bythos de verdad ni se entera.

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// basePrueba abre un .db aislado con las tablas reales (mismo camino que Abrir).
func basePrueba(t *testing.T) *sql.DB {
	t.Helper()
	base, err := Abrir(filepath.Join(t.TempDir(), "bythos.db"))
	if err != nil {
		t.Fatalf("Abrir: %v", err)
	}
	t.Cleanup(func() { base.Close() })
	return base
}

func TestCreateAndListFolders(t *testing.T) {
	base := basePrueba(t)
	// Nombre con espacios se guarda recortado, vacío se rechaza.
	c, err := CrearCarpeta(base, "  React  ")
	if err != nil || c.Nombre != "React" {
		t.Fatalf("crear con trim mal: %+v err=%v", c, err)
	}
	if _, err := CrearCarpeta(base, "   "); err == nil {
		t.Fatal("nombre vacío debió fallar")
	}
	lista, err := ListarCarpetas(base)
	if err != nil || len(lista) != 1 || lista[0].ID != c.ID {
		t.Fatalf("listar mal: %+v err=%v", lista, err)
	}
}

func TestDeleteFolderCascadesResources(t *testing.T) {
	base := basePrueba(t)
	c, _ := CrearCarpeta(base, "React")
	a, _ := Guardar(base, c.ID, "http://a/1", "A", "", "", "articulo")
	if _, err := Guardar(base, c.ID, "http://a/2", "B", "", "", "otro"); err != nil {
		t.Fatalf("Guardar: %v", err)
	}
	borrada, err := BorrarCarpeta(base, c.ID)
	if err != nil || !borrada {
		t.Fatalf("borrar mal: borrada=%v err=%v", borrada, err)
	}
	// Los recursos huérfanos no existen: el cascade los llevó puestos.
	quedan, _ := ListarRecursos(base, c.ID)
	if len(quedan) != 0 {
		t.Fatalf("cascade falló: quedan %d", len(quedan))
	}
	todos, _ := ListarRecursos(base, 0)
	for _, r := range todos {
		if r.ID == a.ID {
			t.Fatalf("recurso huérfano: %+v", r)
		}
	}
	// Borrar lo que no existe avisa con false, sin error.
	if borrada, _ := BorrarCarpeta(base, 9999); borrada {
		t.Fatal("borrar ausente debió dar false")
	}
}

func TestSaveAndListResources(t *testing.T) {
	base := basePrueba(t)
	c1, _ := CrearCarpeta(base, "React")
	c2, _ := CrearCarpeta(base, "Go")
	// Sin url o sin carpeta no se guarda nada.
	if _, err := Guardar(base, c1.ID, "  ", "X", "", "", ""); err == nil {
		t.Fatal("url vacía debió fallar")
	}
	if _, err := Guardar(base, 0, "http://a/0", "X", "", "", ""); err == nil {
		t.Fatal("carpeta 0 debió fallar")
	}
	r, err := Guardar(base, c1.ID, "http://a/1", "Intro", "", "", "")
	if err != nil {
		t.Fatalf("Guardar: %v", err)
	}
	// Nace pendiente en 0 y el tipo vacío cae a "otro".
	if r.Estado != EstadoPendiente || r.Progreso != 0 || r.Tipo != "otro" {
		t.Fatalf("recién nacido mal: %+v", r)
	}
	if _, err := Guardar(base, c2.ID, "http://a/2", "Goroutines", "", "", "articulo"); err != nil {
		t.Fatalf("Guardar: %v", err)
	}
	// Filtro por carpeta trae 1, sin filtro trae todos (lo nuevo primero).
	if unos, _ := ListarRecursos(base, c1.ID); len(unos) != 1 || unos[0].URL != "http://a/1" {
		t.Fatalf("filtro mal: %+v", unos)
	}
	if todos, _ := ListarRecursos(base, 0); len(todos) != 2 {
		t.Fatalf("todos mal: %+v", todos)
	}
}

func TestUpdateProgressClamps(t *testing.T) {
	base := basePrueba(t)
	c, _ := CrearCarpeta(base, "React")
	r, _ := Guardar(base, c.ID, "http://a/1", "A", "", "", "otro")
	casos := []struct {
		nombre   string
		valor    int
		progreso int
		estado   string
	}{
		{"negative clamps to zero", -30, 0, EstadoPendiente},
		{"zero stays pendiente", 0, 0, EstadoPendiente},
		{"partial moves to en_curso", 40, 40, EstadoEnCurso},
		{"hundred completes", 100, 100, EstadoCompletado},
		{"over clamps to hundred", 250, 100, EstadoCompletado},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			if err := ActualizarProgreso(base, r.ID, tt.valor); err != nil {
				t.Fatalf("ActualizarProgreso: %v", err)
			}
			got, _ := ListarRecursos(base, c.ID)
			if got[0].Progreso != tt.progreso || got[0].Estado != tt.estado {
				t.Fatalf("salio %d/%s, queria %d/%s",
					got[0].Progreso, got[0].Estado, tt.progreso, tt.estado)
			}
		})
	}
}

func TestFolderProgressMath(t *testing.T) {
	base := basePrueba(t)
	c, _ := CrearCarpeta(base, "React")
	// Carpeta vacía: todo en 0, sin dividir por cero.
	vacia, err := ProgresoCarpeta(base, c.ID)
	if err != nil || vacia != (Progreso{}) {
		t.Fatalf("vacía debió ser ceros: %+v err=%v", vacia, err)
	}
	a, _ := Guardar(base, c.ID, "http://a/1", "A", "", "", "otro")
	b, _ := Guardar(base, c.ID, "http://a/2", "B", "", "", "otro")
	_ = ActualizarProgreso(base, a.ID, 40)
	_ = ActualizarProgreso(base, b.ID, 100) // solo este cuenta como completado
	p, err := ProgresoCarpeta(base, c.ID)
	if err != nil {
		t.Fatalf("ProgresoCarpeta: %v", err)
	}
	// Porcentaje = completados/total (1/2 = 50); Promedio = AVG(40,100) = 70.
	if p.Total != 2 || p.Completados != 1 || p.Porcentaje != 50 || p.Promedio != 70 {
		t.Fatalf("cuentas mal: %+v", p)
	}
	if p.EnCurso != 1 || p.Pendientes != 0 {
		t.Fatalf("estados mal: %+v", p)
	}
}
