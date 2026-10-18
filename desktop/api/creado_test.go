package api

// creado_test.go — Toda creación responde 201 con Content-Type JSON: si
// WriteHeader va antes que la cabecera, Go la ignora y llega text/plain.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bythos-desktop/db"
)

func TestCrearRespondeJSON(t *testing.T) {
	usarArchivosPrueba(t)
	s := basePrueba(t)
	c, err := db.CrearCarpeta(s.Base, "Base")
	if err != nil {
		t.Fatalf("CrearCarpeta: %v", err)
	}

	pedirJSON := func(ruta, cuerpo string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, ruta, strings.NewReader(cuerpo))
		req.Host = "localhost:8080"
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.Rutas().ServeHTTP(rec, req)
		return rec
	}

	casos := []struct {
		nombre string
		pedir  func() *httptest.ResponseRecorder
	}{
		{"carpeta", func() *httptest.ResponseRecorder {
			return pedirJSON("/api/carpetas", `{"nombre":"React"}`)
		}},
		{"recurso", func() *httptest.ResponseRecorder {
			// Con título y tipo no se busca metadata (sin red).
			return pedirJSON("/api/recursos", fmt.Sprintf(`{"carpeta_id":%d,"url":"http://a/1","titulo":"Intro","tipo":"articulo"}`, c.ID))
		}},
		{"nota de agenda", func() *httptest.ResponseRecorder {
			return pedirJSON("/api/agenda", `{"fecha":"2026-09-12","texto":"Repaso"}`)
		}},
		{"archivo", func() *httptest.ResponseRecorder {
			return subirMultipart(t, s, c.ID, "doc.pdf", pdfMinimo)
		}},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			rec := tt.pedir()
			if rec.Code != http.StatusCreated {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if ct := rec.Result().Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type %q, la UI necesita JSON", ct)
			}
		})
	}
}
