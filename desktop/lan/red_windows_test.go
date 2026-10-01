//go:build windows

package lan

import (
	"net"
	"testing"
)

// TestRedHumoWindows corre la detección COM real contra la primera
// interfaz candidata; se saltea si no hay red privada o COM no responde.
func TestRedHumoWindows(t *testing.T) {
	ifaces, err := InterfacesDisponibles()
	if err != nil || len(ifaces) == 0 {
		t.Skip("sin interfaz candidata")
	}
	cat, err := categoriaRedSO(ifaces[0].IP)
	if err != nil {
		t.Skipf("COM/NLM no disponible: %v", err)
	}
	switch cat {
	case CategoriaPrivada, CategoriaPublica, CategoriaDominio, CategoriaDesconocida:
		t.Logf("%s -> %s", net.IP(ifaces[0].IP), cat)
	default:
		t.Fatalf("categoría inválida: %q", cat)
	}
}
