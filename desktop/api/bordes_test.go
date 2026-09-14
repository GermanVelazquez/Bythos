package api

// bordes_test.go — Dos bordes baratos sin red ni mocks.
// Si el mock de red se pone feo, estos dos igual corren en milisegundos.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Sin esquema no hay ni intento de red: cae al fallback digno.
func TestMetadataFallbackSinRed(t *testing.T) {
	link := "nota-url-sin-esquema"
	m := obtenerMetadata(link)
	if m.Titulo != link || m.Tipo != "otro" {
		t.Fatalf("fallback mal: %+v", m)
	}
}

// En desktop/api no hay ui/dist: "/" debe avisar en español, no dar 404 mudo.
func TestMontarUIAvisaSinDist(t *testing.T) {
	s := basePrueba(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.Rutas().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, queria 200 con aviso", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Falta la UI") {
		t.Fatalf("sin aviso en español, salio: %s", rec.Body.String())
	}
}
