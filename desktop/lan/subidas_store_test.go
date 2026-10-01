package lan

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func hexDe(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func almacenDePrueba(t *testing.T) (*AlmacenSubidas, *relojFalso) {
	t.Helper()
	reloj := &relojFalso{ahora: time.Now()}
	return nuevoAlmacenEn(filepath.Join(t.TempDir(), "subidas"), reloj), reloj
}

func TestSubidasStoreReanudaTrasReinicio(t *testing.T) {
	a, reloj := almacenDePrueba(t)
	datos := []byte("hola mundo, esto es un archivo en dos partes")
	s, err := a.Crear(Subida{DispositivoID: 1, ClienteID: "c1", Nombre: "../../evil.txt", Tamano: int64(len(datos))})
	if err != nil {
		t.Fatal(err)
	}
	corte := 10
	if _, err := a.Anexar(s.ID, 0, bytes.NewReader(datos[:corte]), hexDe(datos[:corte])); err != nil {
		t.Fatal(err)
	}
	// "Reinicio": un almacén nuevo sobre la misma carpeta.
	b := nuevoAlmacenEn(a.carpeta, reloj)
	cargada, err := b.Cargar(s.ID)
	if err != nil || cargada.Offset != int64(corte) {
		t.Fatalf("Cargar = %+v, %v", cargada, err)
	}
	if _, err := b.Anexar(s.ID, int64(corte), bytes.NewReader(datos[corte:]), hexDe(datos[corte:])); err != nil {
		t.Fatal(err)
	}
	final, _ := b.Cargar(s.ID)
	if suma, _ := final.SumaHex(); suma != hexDe(datos) {
		t.Fatalf("hash acumulado %s != %s", suma, hexDe(datos))
	}
	// El nombre del cliente nunca es ruta: solo ID.json y ID.parte.
	entradas, _ := os.ReadDir(a.carpeta)
	for _, e := range entradas {
		if e.Name() != s.ID+".json" && e.Name() != s.ID+".parte" {
			t.Errorf("archivo inesperado %q", e.Name())
		}
	}
}

func TestSubidasStoreTruncaPorCrash(t *testing.T) {
	a, _ := almacenDePrueba(t)
	s, _ := a.Crear(Subida{Tamano: 100})
	a.Anexar(s.ID, 0, bytes.NewReader([]byte("12345")), hexDe([]byte("12345")))
	// Crash: bytes escritos al .parte sin que el sidecar avance.
	f, _ := os.OpenFile(a.rutaParte(s.ID), os.O_APPEND|os.O_WRONLY, 0600)
	f.WriteString("basura")
	f.Close()
	if _, err := a.Cargar(s.ID); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(a.rutaParte(s.ID)); info.Size() != 5 {
		t.Fatalf(".parte mide %d, quería 5", info.Size())
	}
}

func TestSubidasStoreHashMalaRestauraEstado(t *testing.T) {
	a, _ := almacenDePrueba(t)
	s, _ := a.Crear(Subida{Tamano: 100})
	a.Anexar(s.ID, 0, bytes.NewReader([]byte("abc")), hexDe([]byte("abc")))
	_, err := a.Anexar(s.ID, 3, bytes.NewReader([]byte("def")), hexDe([]byte("otra cosa")))
	if !errors.Is(err, ErrHashParte) {
		t.Fatalf("err = %v, quería ErrHashParte", err)
	}
	if _, err := a.Anexar(s.ID, 0, bytes.NewReader([]byte("x")), hexDe([]byte("x"))); !errors.Is(err, ErrOffsetParte) {
		t.Fatalf("err = %v, quería ErrOffsetParte", err)
	}
	// Reintento correcto desde el offset previo: el hash acumulado sigue limpio.
	if _, err := a.Anexar(s.ID, 3, bytes.NewReader([]byte("def")), hexDe([]byte("def"))); err != nil {
		t.Fatal(err)
	}
	final, _ := a.Cargar(s.ID)
	if suma, _ := final.SumaHex(); suma != hexDe([]byte("abcdef")) {
		t.Fatal("el hash acumulado quedó contaminado")
	}
}

func TestSubidasStoreIDInvalidoNoTocaDisco(t *testing.T) {
	a, _ := almacenDePrueba(t)
	for _, id := range []string{"", "../x", `..\x`, "ABCDEF0123456789ABCDEF0123456789", "a/b"} {
		if _, err := a.Cargar(id); !errors.Is(err, ErrSubidaNoExiste) {
			t.Errorf("Cargar(%q) = %v", id, err)
		}
		a.Eliminar(id) // no debe borrar nada ni entrar en pánico
	}
}

func TestSubidasStoreLimpiezaIdle24h(t *testing.T) {
	a, reloj := almacenDePrueba(t)
	vieja, _ := a.Crear(Subida{DispositivoID: 1})
	reloj.avanzar(23 * time.Hour)
	reciente, _ := a.Crear(Subida{DispositivoID: 1})
	reloj.avanzar(2 * time.Hour) // vieja: 25 h sin actividad, reciente: 2 h
	os.WriteFile(filepath.Join(a.carpeta, hexDe([]byte("huerfano"))[:32]+".parte"), []byte("x"), 0600)

	a.Limpiar()
	if _, err := a.Cargar(vieja.ID); !errors.Is(err, ErrSubidaNoExiste) {
		t.Error("la subida de 25 h debía limpiarse")
	}
	if _, err := a.Cargar(reciente.ID); err != nil {
		t.Errorf("la reciente debía quedar: %v", err)
	}
	if entradas, _ := os.ReadDir(a.carpeta); len(entradas) != 2 {
		t.Errorf("quedaron %d archivos, quería 2 (sin huérfanos)", len(entradas))
	}
}

func TestSubidasStoreVigilarLimpiaCadaIntervalo(t *testing.T) {
	a, reloj := almacenDePrueba(t)
	s, _ := a.Crear(Subida{})
	parar := make(chan struct{})
	hecho := make(chan struct{})
	go func() { a.Vigilar(parar, 10*time.Millisecond); close(hecho) }()
	reloj.avanzar(25 * time.Hour)
	limite := time.Now().Add(2 * time.Second)
	for time.Now().Before(limite) {
		if _, err := a.Cargar(s.ID); errors.Is(err, ErrSubidaNoExiste) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(parar)
	<-hecho
	if _, err := a.Cargar(s.ID); !errors.Is(err, ErrSubidaNoExiste) {
		t.Fatal("la limpieza periódica no descartó la subida idle")
	}
}

func TestSubidasStoreRevocarYBloqueo(t *testing.T) {
	a, _ := almacenDePrueba(t)
	s1, _ := a.Crear(Subida{DispositivoID: 1})
	s2, _ := a.Crear(Subida{DispositivoID: 2})
	a.EliminarDispositivo(1)
	if _, err := a.Cargar(s1.ID); err == nil {
		t.Error("las subidas del dispositivo revocado debían borrarse")
	}
	if _, err := a.Cargar(s2.ID); err != nil {
		t.Error("las de otro dispositivo debían quedar")
	}
	liberar, ok := a.Bloquear(s2.ID)
	if !ok {
		t.Fatal("primer Bloquear debía ganar")
	}
	if _, ok2 := a.Bloquear(s2.ID); ok2 {
		t.Error("segundo Bloquear debía fallar (TryLock)")
	}
	liberar()
}
