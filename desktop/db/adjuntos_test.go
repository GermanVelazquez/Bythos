package db

// adjuntos_test.go — Vínculos sesión-archivo: dedupe por sesión, privacidad
// entre sesiones y que borrar la sesión nunca se lleve el archivo.

import "testing"

func TestAdjuntosCrearReAdjuntarYListar(t *testing.T) {
	base := basePrueba(t)
	s1, _ := CrearSesion(base, "uno", "dibujo", nil)
	s2, _ := CrearSesion(base, "dos", "dibujo", nil)
	a, _ := CrearArchivo(base, "aaa", "foto.png", "image/png", 10, "aa/aaa.png")

	v1, err := CrearAdjunto(base, s1.ID, a.ID, "foto.png")
	if err != nil || v1.ID == 0 || v1.SHA256 != "aaa" || v1.Mime != "image/png" {
		t.Fatalf("CrearAdjunto: %+v err=%v", v1, err)
	}

	// Re-adjuntar el mismo archivo a la misma sesión devuelve el vínculo existente.
	v2, err := CrearAdjunto(base, s1.ID, a.ID, "otro-nombre.png")
	if err != nil || v2.ID != v1.ID || v2.Nombre != "foto.png" {
		t.Fatalf("re-adjuntar debió devolver el vínculo existente: %+v err=%v", v2, err)
	}

	// Otra sesión, mismo archivo: vínculo propio, un solo blob.
	v3, err := CrearAdjunto(base, s2.ID, a.ID, "foto.png")
	if err != nil || v3.ID == v1.ID {
		t.Fatalf("otra sesión debió tener su propio vínculo: %+v err=%v", v3, err)
	}

	lista, err := ListarAdjuntos(base, s1.ID)
	if err != nil || len(lista) != 1 || lista[0].ID != v1.ID {
		t.Fatalf("la lista de s1 debió tener solo su vínculo: %+v err=%v", lista, err)
	}
	vacia, err := ListarAdjuntos(base, 9999)
	if err != nil || vacia == nil || len(vacia) != 0 {
		t.Fatalf("sesión sin adjuntos debió dar lista vacía no nula: %#v err=%v", vacia, err)
	}
}

func TestObtenerAdjuntoEsPorSesion(t *testing.T) {
	base := basePrueba(t)
	s1, _ := CrearSesion(base, "uno", "dibujo", nil)
	s2, _ := CrearSesion(base, "dos", "dibujo", nil)
	a, _ := CrearArchivo(base, "aaa", "foto.png", "image/png", 10, "aa/aaa.png")
	v, _ := CrearAdjunto(base, s1.ID, a.ID, "foto.png")

	if got, ok, err := ObtenerAdjunto(base, s1.ID, v.ID); err != nil || !ok || got.ID != v.ID {
		t.Fatalf("la sesión dueña debió verlo: %+v ok=%v err=%v", got, ok, err)
	}
	if _, ok, err := ObtenerAdjunto(base, s2.ID, v.ID); err != nil || ok {
		t.Fatalf("otra sesión no debió verlo: ok=%v err=%v", ok, err)
	}
}

func TestBorrarSesionCascadeaVinculosYConservaArchivo(t *testing.T) {
	base := basePrueba(t)
	s, _ := CrearSesion(base, "uno", "dibujo", nil)
	a, _ := CrearArchivo(base, "aaa", "foto.png", "image/png", 10, "aa/aaa.png")
	CrearAdjunto(base, s.ID, a.ID, "foto.png")

	if ok, err := BorrarSesion(base, s.ID); err != nil || !ok {
		t.Fatalf("BorrarSesion: ok=%v err=%v", ok, err)
	}
	if lista, _ := ListarAdjuntos(base, s.ID); len(lista) != 0 {
		t.Fatalf("los vínculos debieron caer con la sesión: %+v", lista)
	}
	if _, ok, _ := ObtenerArchivo(base, a.ID); !ok {
		t.Fatal("la fila de archivos debió sobrevivir al borrado de la sesión")
	}
}

func TestArchivoConVinculoNoSePuedeBorrar(t *testing.T) {
	base := basePrueba(t)
	s, _ := CrearSesion(base, "uno", "dibujo", nil)
	a, _ := CrearArchivo(base, "aaa", "foto.png", "image/png", 10, "aa/aaa.png")
	CrearAdjunto(base, s.ID, a.ID, "foto.png")

	if err := BorrarArchivo(base, a.ID); err == nil {
		t.Fatal("borrar un archivo vinculado debió fallar por la FK (sin CASCADE)")
	}
	if _, ok, _ := ObtenerArchivo(base, a.ID); !ok {
		t.Fatal("el archivo vinculado debió seguir existiendo")
	}
}
