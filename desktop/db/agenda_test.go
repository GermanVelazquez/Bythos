package db

// agenda_test.go — La agenda bajo lupa, sin tocar tu .db real.
// Cada test abre su propio archivo en t.TempDir() (ver basePrueba en db_test.go).
// Table-driven por go-testing skill: casos con nombre de escenario, no de input.

import (
	"testing"
)

func TestCrearYListarAgenda(t *testing.T) {
	base := basePrueba(t)
	c, _ := CrearCarpeta(base, "React")

	cid := c.ID
	a, err := CrearAgenda(base, "2026-09-12", "10:00", "11:30", "Repasar hooks", &cid)
	if err != nil {
		t.Fatalf("CrearAgenda: %v", err)
	}
	// El JOIN trae el nombre sin segundo viaje.
	if a.Fecha != "2026-09-12" || a.HoraInicio != "10:00" || a.HoraFin != "11:30" {
		t.Fatalf("fecha/horas mal: %+v", a)
	}
	if a.Texto != "Repasar hooks" || a.CarpetaNombre != "React" {
		t.Fatalf("texto/carpeta mal: %+v", a)
	}
	if a.CarpetaID == nil || *a.CarpetaID != c.ID {
		t.Fatalf("carpeta_id mal: %+v", a.CarpetaID)
	}

	// Nota suelta: sin horas ni carpeta, con texto.
	suelta, err := CrearAgenda(base, "2026-09-13", "", "", "Leer tranqui", nil)
	if err != nil {
		t.Fatalf("nota suelta: %v", err)
	}
	if suelta.HoraInicio != "" || suelta.HoraFin != "" || suelta.CarpetaID != nil {
		t.Fatalf("suelta debió ser sin hora ni carpeta: %+v", suelta)
	}

	// Rango ordenado por fecha y hora (lo nuevo-last primero por día).
	radas, err := ListarAgenda(base, "2026-09-01", "2026-09-30")
	if err != nil || len(radas) != 2 {
		t.Fatalf("rango mal: %+v err=%v", radas, err)
	}
	if radas[0].Fecha != "2026-09-12" || radas[1].Fecha != "2026-09-13" {
		t.Fatalf("orden mal: %+v", radas)
	}
	// Fuera de rango no aparece nada (slice vacío, no nil: JSON []).
	fuera, _ := ListarAgenda(base, "2026-10-01", "2026-10-31")
	if len(fuera) != 0 {
		t.Fatalf("fuera de rango debió ser vacío: %+v", fuera)
	}

	// Borrar avisa true una vez y false la segunda.
	if ok, _ := BorrarAgenda(base, a.ID); !ok {
		t.Fatal("borrar existente debió dar true")
	}
	if ok, _ := BorrarAgenda(base, a.ID); ok {
		t.Fatal("borrar ausente debió dar false")
	}
}

func TestCrearAgendaValidacion(t *testing.T) {
	base := basePrueba(t)
	c, _ := CrearCarpeta(base, "React")
	buena := c.ID
	mala := int64(9999)
	cero := int64(0)

	casos := []struct {
		nombre     string
		fecha      string
		inicio     string
		fin        string
		texto      string
		carpetaID  *int64
		debeFallar bool
	}{
		{"texto solo sin carpeta", "2026-09-12", "", "", "Leer", nil, false},
		{"carpeta sola sin texto", "2026-09-12", "", "", "   ", &buena, false},
		{"todo completo", "2026-09-12", "10:00", "11:30", "Repaso", &buena, false},
		{"solo inicio sin fin", "2026-09-12", "10:00", "", "X", nil, false},
		{"fecha vacía", "", "", "", "X", nil, true},
		{"fecha con otro formato", "12/09/2026", "", "", "X", nil, true},
		{"fecha imposible", "2026-02-30", "", "", "X", nil, true},
		{"mes 13", "2026-13-01", "", "", "X", nil, true},
		{"sin texto ni carpeta", "2026-09-12", "", "", "   ", nil, true},
		{"carpeta inexistente", "2026-09-12", "", "", "", &mala, true},
		{"carpeta cero", "2026-09-12", "", "", "X", &cero, true},
		{"fin sin inicio", "2026-09-12", "", "11:30", "X", nil, true},
		{"fin antes que inicio", "2026-09-12", "11:30", "10:00", "X", nil, true},
		{"fin igual que inicio", "2026-09-12", "10:00", "10:00", "X", nil, true},
		{"hora imposible", "2026-09-12", "25:00", "", "X", nil, true},
		{"hora sin ceros", "2026-09-12", "9:00", "", "X", nil, true},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			_, err := CrearAgenda(base, tt.fecha, tt.inicio, tt.fin, tt.texto, tt.carpetaID)
			if tt.debeFallar && err == nil {
				t.Fatal("debió fallar y pasó")
			}
			if !tt.debeFallar && err != nil {
				t.Fatalf("debió pasar y falló: %v", err)
			}
		})
	}
}

func TestActividadPorDia(t *testing.T) {
	base := basePrueba(t)
	c, _ := CrearCarpeta(base, "React")
	a, _ := Guardar(base, c.ID, "http://a/1", "A", "", "", "otro")
	b, _ := Guardar(base, c.ID, "http://a/2", "B", "", "", "otro")

	// Fechas fijas: el historial debe ser determinista, no "hoy".
	// (created_at nace con CURRENT_TIMESTAMP; lo pisamos por SQL.)
	fijarFecha := func(tabla string, id int64, cuando string) {
		t.Helper()
		if _, err := base.Exec(`UPDATE `+tabla+` SET created_at = ? WHERE id = ?`, cuando, id); err != nil {
			t.Fatalf("fijar fecha: %v", err)
		}
	}
	fijarFecha("folders", c.ID, "2026-09-10 08:00:00")
	fijarFecha("resources", a.ID, "2026-09-10 09:00:00")
	fijarFecha("resources", b.ID, "2026-09-12 09:00:00")
	// La agenda NO cuenta: son planes, no creaciones.
	cid := c.ID
	if _, err := CrearAgenda(base, "2026-09-10", "", "", "Plan", &cid); err != nil {
		t.Fatalf("CrearAgenda: %v", err)
	}

	got, err := ActividadPorDia(base, "2026-09-01", "2026-09-30")
	if err != nil {
		t.Fatalf("ActividadPorDia: %v", err)
	}
	porFecha := map[string]int{}
	for _, x := range got {
		porFecha[x.Fecha] = x.Total
	}
	// 10/9: 1 carpeta + 1 recurso = 2. 12/9: 1 recurso = 1.
	if porFecha["2026-09-10"] != 2 {
		t.Fatalf("10/9 debió ser 2, salió %d (%+v)", porFecha["2026-09-10"], got)
	}
	if porFecha["2026-09-12"] != 1 {
		t.Fatalf("12/9 debió ser 1, salió %d (%+v)", porFecha["2026-09-12"], got)
	}
	// El rango recorta: solo el 12.
	recorte, _ := ActividadPorDia(base, "2026-09-12", "2026-09-12")
	if len(recorte) != 1 || recorte[0].Total != 1 {
		t.Fatalf("recorte mal: %+v", recorte)
	}
}

func TestBorrarCarpetaConservaAgenda(t *testing.T) {
	base := basePrueba(t)
	c, _ := CrearCarpeta(base, "React")
	cid := c.ID
	a, err := CrearAgenda(base, "2026-09-12", "10:00", "", "Repasar hooks", &cid)
	if err != nil {
		t.Fatalf("CrearAgenda: %v", err)
	}

	// Borrar la carpeta NO borra la nota (ON DELETE SET NULL, no CASCADE).
	borrada, err := BorrarCarpeta(base, c.ID)
	if err != nil || !borrada {
		t.Fatalf("borrar mal: borrada=%v err=%v", borrada, err)
	}
	quedan, err := ListarAgenda(base, "", "")
	if err != nil || len(quedan) != 1 || quedan[0].ID != a.ID {
		t.Fatalf("la nota debió sobrevivir: %+v err=%v", quedan, err)
	}
	if quedan[0].CarpetaID != nil {
		t.Fatalf("carpeta_id debió quedar NULL, salió %d", *quedan[0].CarpetaID)
	}
	if quedan[0].CarpetaNombre != "" {
		t.Fatalf("nombre debió quedar vacío, salió %q", quedan[0].CarpetaNombre)
	}
	// Y el texto y la hora siguen intactos.
	if quedan[0].Texto != "Repasar hooks" || quedan[0].HoraInicio != "10:00" {
		t.Fatalf("la nota perdió datos: %+v", quedan[0])
	}
}
