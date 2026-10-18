package lan

// interfaces_test.go — filtrarYOrdenar con una lista falsa: descarta
// inactivas, públicas y VPN; ordena Wi-Fi antes que Ethernet.

import (
	"net"
	"testing"
)

func TestFiltrarYOrdenar(t *testing.T) {
	crudas := []candidatoCrudo{
		{Nombre: "Ethernet", Activa: true, IP: net.ParseIP("192.168.1.5"), Mascara: net.CIDRMask(24, 32)},
		{Nombre: "Wi-Fi", Activa: true, IP: net.ParseIP("192.168.1.6"), Mascara: net.CIDRMask(24, 32)},
		{Nombre: "vEthernet (WSL)", Activa: true, IP: net.ParseIP("172.20.0.1"), Mascara: net.CIDRMask(20, 32)},
		{Nombre: "Ethernet inactiva", Activa: false, IP: net.ParseIP("192.168.2.5"), Mascara: net.CIDRMask(24, 32)},
		{Nombre: "Ethernet pública", Activa: true, IP: net.ParseIP("8.8.8.8"), Mascara: net.CIDRMask(24, 32)},
	}
	out := filtrarYOrdenar(crudas)
	if len(out) != 2 {
		t.Fatalf("esperaba 2 candidatas (Wi-Fi + Ethernet), llegaron %d: %+v", len(out), out)
	}
	if out[0].Nombre != "Wi-Fi" || out[1].Nombre != "Ethernet" {
		t.Fatalf("Wi-Fi debía ir antes que Ethernet: %+v", out)
	}
	if !out[0].Rango.Contains(net.ParseIP("192.168.1.9")) {
		t.Fatalf("rango de subred mal calculado: %+v", out[0].Rango)
	}
}
