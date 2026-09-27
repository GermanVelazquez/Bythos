package api

// body_test.go — Techo de tamaño para bodies JSON (leerJSON en server.go).
// Antes crearCarpeta/guardarRecurso/cambiarEstado/crearAgenda decodificaban
// r.Body sin límite: un body de gigabytes se comía RAM antes de que el JSON
// se validara. Ahora http.MaxBytesReader corta en 1MB y el handler responde
// 413 en español; bodies normales siguen igual.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bythos-desktop/db"
)

// bodyGigante supera el techo de 1MB (limiteBodyJSON) con margen.
func bodyGigante() string {
	return `{"nombre":"` + strings.Repeat("a", 2<<20) + `"}`
}

func TestLeerJSONBodyDemasiadoGrandeCrearCarpeta(t *testing.T) {
	s := basePrueba(t)
	req := httptest.NewRequest(http.MethodPost, "/api/carpetas", strings.NewReader(bodyGigante()))
	rec := httptest.NewRecorder()
	s.crearCarpeta(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s, quería 413", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp["error"] == "" {
		t.Fatalf("413 debe traer {error} en español, salió: %s", rec.Body.String())
	}
}

func TestLeerJSONBodyDemasiadoGrandeGuardarRecurso(t *testing.T) {
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	body := `{"carpeta_id":` + itoa(c.ID) + `,"url":"http://a/1","titulo":"` + strings.Repeat("a", 2<<20) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/recursos", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.guardarRecurso(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s, quería 413", rec.Code, rec.Body.String())
	}
}

func TestLeerJSONBodyDemasiadoGrandeCambiarEstado(t *testing.T) {
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "React")
	r, _ := db.Guardar(s.Base, c.ID, "http://a/1", "A", "", "", "otro")
	body := `{"estado":"en_curso","relleno":"` + strings.Repeat("a", 2<<20) + `"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/recursos/"+itoa(r.ID), strings.NewReader(body))
	req.SetPathValue("id", itoa(r.ID))
	rec := httptest.NewRecorder()
	s.cambiarEstado(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s, quería 413", rec.Code, rec.Body.String())
	}
}

func TestLeerJSONBodyDemasiadoGrandeCrearAgenda(t *testing.T) {
	s := basePrueba(t)
	body := `{"fecha":"2026-09-12","texto":"` + strings.Repeat("a", 2<<20) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/agenda", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.crearAgenda(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s, quería 413", rec.Code, rec.Body.String())
	}
}

// Body normal (chico) sigue funcionando igual que antes del límite.
func TestLeerJSONBodyNormalSigueFuncionando(t *testing.T) {
	s := basePrueba(t)
	req := httptest.NewRequest(http.MethodPost, "/api/carpetas", strings.NewReader(`{"nombre":"React"}`))
	rec := httptest.NewRecorder()
	s.crearCarpeta(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s, quería 201", rec.Code, rec.Body.String())
	}
}
