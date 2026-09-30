package lan

// guardia_test.go — subredPermite y conGuardia: solo la misma subred
// pasa, y el handler de negocio nunca corre en el caso rechazado.

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSubredPermite(t *testing.T) {
	_, rango, _ := net.ParseCIDR("192.168.1.0/24")
	casos := []struct {
		nombre string
		remoto string
		quiere bool
	}{
		{"misma subred", "192.168.1.50", true},
		{"otra subred privada", "192.168.2.50", false},
		{"pública", "8.8.8.8", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := subredPermite(net.ParseIP(c.remoto), rango); got != c.quiere {
				t.Fatalf("subredPermite(%s) = %v, quería %v", c.remoto, got, c.quiere)
			}
		})
	}
}

func TestConGuardiaRechazaFueraDeSubred(t *testing.T) {
	_, rango, _ := net.ParseCIDR("10.0.0.0/24")
	tocado := false
	h := conGuardia(rango, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { tocado = true }))
	req := httptest.NewRequest("GET", "/v1/salud", nil)
	req.RemoteAddr = "192.168.1.5:1234" // fuera de 10.0.0.0/24
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("código = %d, quería 403", rr.Code)
	}
	if tocado {
		t.Fatalf("el handler de negocio no debía ejecutarse")
	}
}
