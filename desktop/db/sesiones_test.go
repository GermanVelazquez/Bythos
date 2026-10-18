package db

// sesiones_test.go — sesiones de Terminal y su pizarra: CRUD, CHECK de modo,
// carpeta suelta, cascade y conflicto de versión. basePrueba viene de db_test.go.

import (
	"errors"
	"testing"
)

func TestSesionCRUD(t *testing.T) {
	base := basePrueba(t)
	c, _ := CrearCarpeta(base, "Mates")
	s, err := CrearSesion(base, "  Álgebra ", "practica", &c.ID)
	if err != nil {
		t.Fatalf("CrearSesion: %v", err)
	}
	if s.Nombre != "Álgebra" || s.Modo != "practica" || s.CarpetaID == nil || *s.CarpetaID != c.ID {
		t.Fatalf("sesión mal: %+v", s)
	}
	lista, err := ListarSesiones(base)
	if err != nil || len(lista) != 1 || lista[0].ID != s.ID {
		t.Fatalf("listar mal: %+v err=%v", lista, err)
	}
	nuevo, modo := "Cálculo", "revision"
	ok, err := ActualizarSesion(base, s.ID, CambiosSesion{Nombre: &nuevo, Modo: &modo, CarpetaSet: true})
	if err != nil || !ok {
		t.Fatalf("ActualizarSesion: ok=%v err=%v", ok, err)
	}
	got, ok, _ := ObtenerSesion(base, s.ID)
	if !ok || got.Nombre != "Cálculo" || got.Modo != "revision" || got.CarpetaID != nil {
		t.Fatalf("patch mal (carpeta debía quedar suelta): %+v", got)
	}
	if ok, _ := ActualizarSesion(base, 999, CambiosSesion{Nombre: &nuevo}); ok {
		t.Fatal("actualizar una sesión inexistente no debe reportar ok")
	}
	if ok, err := BorrarSesion(base, s.ID); err != nil || !ok {
		t.Fatalf("BorrarSesion: ok=%v err=%v", ok, err)
	}
	if ok, _ := BorrarSesion(base, s.ID); ok {
		t.Fatal("borrar dos veces no debe reportar ok")
	}
}

func TestSesionValidaciones(t *testing.T) {
	base := basePrueba(t)
	if _, err := CrearSesion(base, "x", "inventado", nil); err == nil {
		t.Fatal("el CHECK debía rechazar un modo desconocido")
	}
	for _, n := range []string{"", "   "} {
		if _, err := CrearSesion(base, n, "practica", nil); err == nil {
			t.Fatalf("nombre %q debía rechazarse", n)
		}
	}
	otra := int64(999)
	if _, err := CrearSesion(base, "x", "practica", &otra); err == nil {
		t.Fatal("una carpeta inexistente debía rechazarse (llave foránea)")
	}
}

func TestSesionBorrarCarpetaLaSuelta(t *testing.T) {
	base := basePrueba(t)
	c, _ := CrearCarpeta(base, "Mates")
	s, _ := CrearSesion(base, "A", "dibujo", &c.ID)
	if _, err := GuardarPizarra(base, s.ID, `{"elements":[1]}`, 0); err != nil {
		t.Fatalf("GuardarPizarra: %v", err)
	}
	if _, err := BorrarCarpeta(base, c.ID); err != nil {
		t.Fatalf("BorrarCarpeta: %v", err)
	}
	got, ok, _ := ObtenerSesion(base, s.ID)
	if !ok || got.CarpetaID != nil {
		t.Fatalf("la sesión debía sobrevivir suelta: %+v ok=%v", got, ok)
	}
	if escena, ver, ok, _ := ObtenerPizarra(base, s.ID); !ok || ver != 1 || escena != `{"elements":[1]}` {
		t.Fatalf("la escena debía sobrevivir: %q v%d ok=%v", escena, ver, ok)
	}
}

func TestSesionBorrarCascadaEscenaYOps(t *testing.T) {
	base := basePrueba(t)
	s, _ := CrearSesion(base, "A", "paginas", nil)
	// Tabla real (db.go la crea): el cascade tiene a quién llevarse.
	if _, err := EncolarOp(base, s.ID, `[]`); err != nil {
		t.Fatal(err)
	}
	if _, err := BorrarSesion(base, s.ID); err != nil {
		t.Fatalf("BorrarSesion: %v", err)
	}
	for _, q := range []string{`SELECT COUNT(*) FROM terminal_pizarras`, `SELECT COUNT(*) FROM terminal_pizarra_ops`} {
		var n int
		base.QueryRow(q).Scan(&n)
		if n != 0 {
			t.Fatalf("%s = %d, el cascade debía vaciarla", q, n)
		}
	}
}

func TestPizarraConflictoDeVersion(t *testing.T) {
	base := basePrueba(t)
	s, _ := CrearSesion(base, "A", "practica", nil)
	v, err := GuardarPizarra(base, s.ID, `{"a":1}`, 0)
	if err != nil || v != 1 {
		t.Fatalf("primer guardado: v=%d err=%v", v, err)
	}
	_, err = GuardarPizarra(base, s.ID, `{"a":2}`, 0) // versión vieja
	var conf *ConflictoVersion
	if !errors.As(err, &conf) || conf.Actual != 1 {
		t.Fatalf("debía ser conflicto con versión actual 1, salió %v", err)
	}
	if escena, _, _, _ := ObtenerPizarra(base, s.ID); escena != `{"a":1}` {
		t.Fatalf("el conflicto no debía tocar la escena: %q", escena)
	}
	if _, err := GuardarPizarra(base, 999, `{}`, 0); err == nil || errors.As(err, &conf) {
		t.Fatalf("sesión inexistente debía fallar sin ser conflicto: %v", err)
	}
}
