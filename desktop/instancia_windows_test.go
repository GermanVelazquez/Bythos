//go:build windows

package main

// instancia_windows_test.go — el mutex de instancia única.
// Usa un nombre propio del test para no chocar con un Bythos real abierto.

import (
	"fmt"
	"os"
	"testing"
)

func TestInstanciaMutex(t *testing.T) {
	nombre := fmt.Sprintf(`Local\Bythos.Test.%d`, os.Getpid())

	liberar1, existe, err := adquirirMutex(nombre)
	if err != nil || existe {
		t.Fatalf("primera adquisición: existe=%v err=%v, quería éxito limpio", existe, err)
	}
	liberar2, existe, err := adquirirMutex(nombre)
	if err != nil || !existe {
		t.Fatalf("segunda adquisición: existe=%v err=%v, quería existe=true", existe, err)
	}
	liberar2()
	liberar1()

	// Soltados todos los handles, el nombre queda libre otra vez.
	liberar3, existe, err := adquirirMutex(nombre)
	if err != nil || existe {
		t.Fatalf("re-adquisición: existe=%v err=%v, quería éxito limpio", existe, err)
	}
	liberar3()
}
