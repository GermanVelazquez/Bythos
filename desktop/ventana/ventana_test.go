package ventana

// ventana_test.go — El fallback NUNCA abre el navegador: avisa y da error.
// Los tests usan abrir() con candidatos falsos y aviso espiado,
// así no lanzan Edge ni muestran diálogos de verdad.

import (
	"os"
	"strings"
	"testing"
)

// Sin navegador compatible: 1 aviso nativo + error que nombra a Edge.
func TestAbrirSinNavegadorAvisaYDevuelveError(t *testing.T) {
	casos := []struct {
		nombre string
		exes   []string
	}{
		{"sin candidatos", nil},
		{"rutas inexistentes", []string{`C:\NoExiste\navegador.exe`, `Z:\Nada\otro.exe`}},
		{"ruta existente pero no ejecutable", []string{t.TempDir()}},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			avisos := 0
			err := abrir("http://localhost:8080", tt.exes, func() { avisos++ })
			if err == nil {
				t.Fatal("se esperaba error sin navegador compatible, fue nil")
			}
			if avisos != 1 {
				t.Fatalf("se esperaba 1 aviso nativo, fueron %d", avisos)
			}
			if !strings.Contains(err.Error(), "Microsoft Edge") {
				t.Fatalf("el error debe nombrar a Microsoft Edge, fue: %q", err.Error())
			}
		})
	}
}

// El diálogo le dice al usuario qué hacer: Edge + cómo recuperarlo.
func TestTextoAvisoPideEdge(t *testing.T) {
	titulo, mensaje := textoAviso()
	if titulo == "" {
		t.Fatal("el diálogo necesita título")
	}
	for _, quiere := range []string{"Microsoft Edge", "Windows"} {
		if !strings.Contains(mensaje, quiere) {
			t.Fatalf("el aviso debe mencionar %q, fue: %q", quiere, mensaje)
		}
	}
}

// Con un ejecutable real, la ventana arranca y no hay aviso ni error.
// Toca el sistema (lanza un proceso), así que se salta en -short.
func TestAbrirConEjecutableArrancaVentana(t *testing.T) {
	if testing.Short() {
		t.Skip("lanza un proceso real, solo en corrida completa")
	}
	// El propio binario de test: abrir solo hace Start (no espera salida),
	// así que basta que el proceso nazca. El hijo sale solo al fallar
	// sus flags con --app=..., sin dejar procesos colgados.
	yo, err := os.Executable()
	if err != nil {
		t.Fatalf("no se pudo ubicar el binario de test: %v", err)
	}
	avisos := 0
	if err := abrir("http://localhost:8080", []string{yo}, func() { avisos++ }); err != nil {
		t.Fatalf("con ejecutable válido no hay error, fue: %v", err)
	}
	if avisos != 0 {
		t.Fatalf("con ejecutable válido no hay aviso, hubo %d", avisos)
	}
}
