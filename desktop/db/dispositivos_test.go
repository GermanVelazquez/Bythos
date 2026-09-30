package db

// dispositivos_test.go — Los celulares emparejados bajo lupa: creación
// idempotente de la tabla, que el lookup por hash excluya revocados, y
// el throttle de 1/min en TocarDispositivo. Mismo estilo que
// archivos_test.go: cada test abre su .db aislado en t.TempDir().

import (
	"testing"
)

func TestCrearDispositivoYListar(t *testing.T) {
	base := basePrueba(t)
	d, err := CrearDispositivo(base, "  Pixel de Ana  ", "hash-abc")
	if err != nil {
		t.Fatalf("CrearDispositivo: %v", err)
	}
	if d.ID == 0 || d.Nombre != "Pixel de Ana" || d.TokenHash != "hash-abc" {
		t.Fatalf("dispositivo mal creado: %+v", d)
	}
	if d.UltimoUso != "" || d.RevocadoEn != "" {
		t.Fatalf("recién creado no debía tener ultimo_uso ni revocado_en: %+v", d)
	}

	lista, err := ListarDispositivos(base)
	if err != nil || len(lista) != 1 || lista[0].ID != d.ID {
		t.Fatalf("listar mal: %+v err=%v", lista, err)
	}
}

// TestCrearTablaDispositivosIdempotente cubre que reabrir el mismo .db
// (crearTablaDispositivos vía Abrir) no rompe nada, mismo criterio que
// exige crearTablas para toda tabla nueva.
func TestCrearTablaDispositivosIdempotente(t *testing.T) {
	base := basePrueba(t)
	if _, err := CrearDispositivo(base, "Pixel", "hash-1"); err != nil {
		t.Fatalf("CrearDispositivo: %v", err)
	}
	if err := crearTablaDispositivos(base); err != nil {
		t.Fatalf("crearTablaDispositivos repetido debió ser no-op: %v", err)
	}
	lista, err := ListarDispositivos(base)
	if err != nil || len(lista) != 1 {
		t.Fatalf("el dispositivo debió sobrevivir: %+v err=%v", lista, err)
	}
}

func TestCrearDispositivoDatosInvalidos(t *testing.T) {
	base := basePrueba(t)
	if _, err := CrearDispositivo(base, "", "hash"); err == nil {
		t.Fatal("nombre vacío debió fallar")
	}
	if _, err := CrearDispositivo(base, "Pixel", ""); err == nil {
		t.Fatal("token_hash vacío debió fallar")
	}
}

func TestCrearDispositivoNombreLargoSeRecorta(t *testing.T) {
	base := basePrueba(t)
	largo := ""
	for i := 0; i < 200; i++ {
		largo += "x"
	}
	d, err := CrearDispositivo(base, largo, "hash-largo")
	if err != nil {
		t.Fatalf("CrearDispositivo: %v", err)
	}
	if len(d.Nombre) != nombreDispositivoLargoMaximo {
		t.Fatalf("nombre debió recortarse a %d, quedó en %d", nombreDispositivoLargoMaximo, len(d.Nombre))
	}
}

func TestDispositivoPorTokenHashExcluyeRevocados(t *testing.T) {
	base := basePrueba(t)
	d, err := CrearDispositivo(base, "Pixel", "hash-xyz")
	if err != nil {
		t.Fatalf("CrearDispositivo: %v", err)
	}

	encontrado, ok, err := DispositivoPorTokenHash(base, "hash-xyz")
	if err != nil || !ok || encontrado.ID != d.ID {
		t.Fatalf("debía encontrar el dispositivo activo: %+v ok=%v err=%v", encontrado, ok, err)
	}

	if err := RevocarDispositivo(base, d.ID); err != nil {
		t.Fatalf("RevocarDispositivo: %v", err)
	}
	_, ok, err = DispositivoPorTokenHash(base, "hash-xyz")
	if err != nil || ok {
		t.Fatalf("un dispositivo revocado no debía encontrarse: ok=%v err=%v", ok, err)
	}

	// Idempotente: revocar dos veces no debe fallar.
	if err := RevocarDispositivo(base, d.ID); err != nil {
		t.Fatalf("revocar dos veces no debió fallar: %v", err)
	}
}

func TestDispositivoPorTokenHashInexistente(t *testing.T) {
	base := basePrueba(t)
	_, ok, err := DispositivoPorTokenHash(base, "no-existe")
	if err != nil || ok {
		t.Fatalf("hash inexistente debió dar ok=false sin error: ok=%v err=%v", ok, err)
	}
}

// TestTocarDispositivoThrottle cubre las dos puntas del throttle: la
// primera llamada (ultimo_uso NULL) siempre actualiza, y una segunda
// llamada inmediata NO reescribe ultimo_uso (menos de 1 minuto). Para
// probar el otro lado (ya pasó 1 minuto) sin dormir el test, movemos
// ultimo_uso al pasado a mano por SQL y confirmamos que el próximo toque
// sí actualiza.
func TestTocarDispositivoThrottle(t *testing.T) {
	base := basePrueba(t)
	d, err := CrearDispositivo(base, "Pixel", "hash-touch")
	if err != nil {
		t.Fatalf("CrearDispositivo: %v", err)
	}

	if err := TocarDispositivo(base, d.ID); err != nil {
		t.Fatalf("primer TocarDispositivo: %v", err)
	}
	lista, _ := ListarDispositivos(base)
	primerToque := lista[0].UltimoUso
	if primerToque == "" {
		t.Fatal("el primer toque debió setear ultimo_uso")
	}

	// Segundo toque inmediato: dentro del minuto, no debe cambiar.
	if err := TocarDispositivo(base, d.ID); err != nil {
		t.Fatalf("segundo TocarDispositivo: %v", err)
	}
	lista, _ = ListarDispositivos(base)
	if lista[0].UltimoUso != primerToque {
		t.Fatalf("el throttle debió dejar ultimo_uso igual: antes=%q después=%q", primerToque, lista[0].UltimoUso)
	}

	// Forzamos que el último toque quede "hace más de 1 minuto" a mano,
	// sin depender de un reloj falso ni de dormir el test.
	if _, err := base.Exec(`UPDATE dispositivos SET ultimo_uso = datetime('now','localtime','-2 minutes') WHERE id = ?`, d.ID); err != nil {
		t.Fatalf("forzar ultimo_uso viejo: %v", err)
	}
	lista, _ = ListarDispositivos(base)
	viejo := lista[0].UltimoUso

	if err := TocarDispositivo(base, d.ID); err != nil {
		t.Fatalf("tercer TocarDispositivo: %v", err)
	}
	lista, _ = ListarDispositivos(base)
	if lista[0].UltimoUso == viejo {
		t.Fatalf("después de pasar el minuto, el toque debió actualizar ultimo_uso (siguió en %q)", viejo)
	}
}
