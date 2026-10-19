package pizarra

import (
	"strings"
	"testing"
)

func f(v float64) *float64 { return &v }
func n(v int) *int         { return &v }

func marcaOK(id string) Marca {
	return Marca{Objetivo: Objetivo{IDs: []string{id}}, Veredicto: "ok"}
}

func TestValidarMarcasCasos(t *testing.T) {
	veintiuna := make([]Marca, 21)
	for i := range veintiuna {
		veintiuna[i] = marcaOK("a")
	}
	casos := []struct {
		nombre string
		marcas []Marca
		error  string // "" = válido
	}{
		{"una marca por ids", []Marca{marcaOK("a")}, ""},
		{"20 marcas", veintiuna[:20], ""},
		{"21 marcas", veintiuna, "20"},
		{"sin marcas", nil, "al menos"},
		{"pista de 300", []Marca{{Objetivo: Objetivo{IDs: []string{"a"}}, Veredicto: "error", Pista: strings.Repeat("ñ", 300)}}, ""},
		{"pista de 301", []Marca{{Objetivo: Objetivo{IDs: []string{"a"}}, Veredicto: "error", Pista: strings.Repeat("ñ", 301)}}, "pista"},
		{"pasaje de 200", []Marca{{Objetivo: Objetivo{IDs: []string{"a"}}, Veredicto: "ok", Cita: &Cita{RecursoID: 1, Pasaje: strings.Repeat("a", 200)}}}, ""},
		{"pasaje de 201", []Marca{{Objetivo: Objetivo{IDs: []string{"a"}}, Veredicto: "ok", Cita: &Cita{RecursoID: 1, Pasaje: strings.Repeat("a", 201)}}}, "pasaje"},
		{"página 0", []Marca{{Objetivo: Objetivo{IDs: []string{"a"}}, Veredicto: "ok", Cita: &Cita{RecursoID: 1, Pagina: n(0)}}}, "página"},
		{"página 100001", []Marca{{Objetivo: Objetivo{IDs: []string{"a"}}, Veredicto: "ok", Cita: &Cita{RecursoID: 1, Pagina: n(100001)}}}, "página"},
		{"página 100000", []Marca{{Objetivo: Objetivo{IDs: []string{"a"}}, Veredicto: "ok", Cita: &Cita{RecursoID: 1, Pagina: n(100000)}}}, ""},
		{"recurso inválido", []Marca{{Objetivo: Objetivo{IDs: []string{"a"}}, Veredicto: "ok", Cita: &Cita{RecursoID: 0}}}, "recurso"},
		{"veredicto desconocido", []Marca{{Objetivo: Objetivo{IDs: []string{"a"}}, Veredicto: "bien"}}, "veredicto"},
		{"objetivo vacío", []Marca{{Veredicto: "ok"}}, "objetivo"},
		{"id inválido", []Marca{marcaOK("a b")}, "id"},
		{"área completa", []Marca{{Objetivo: Objetivo{X: f(0), Y: f(0), Ancho: f(10), Alto: f(10)}, Veredicto: "parcial"}}, ""},
		{"área incompleta", []Marca{{Objetivo: Objetivo{X: f(0), Y: f(0), Ancho: f(10)}, Veredicto: "ok"}}, "objetivo"},
		{"área sin tamaño", []Marca{{Objetivo: Objetivo{X: f(0), Y: f(0), Ancho: f(0), Alto: f(10)}, Veredicto: "ok"}}, "objetivo"},
		{"ids y área", []Marca{{Objetivo: Objetivo{IDs: []string{"a"}, X: f(0), Y: f(0), Ancho: f(1), Alto: f(1)}, Veredicto: "ok"}}, "objetivo"},
		{"área fuera de rango", []Marca{{Objetivo: Objetivo{X: f(100001), Y: f(0), Ancho: f(1), Alto: f(1)}, Veredicto: "ok"}}, "x"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := ValidarMarcas(c.marcas)
			switch {
			case c.error == "" && err != nil:
				t.Fatalf("debía ser válido: %v", err)
			case c.error != "" && (err == nil || !strings.Contains(err.Error(), c.error)):
				t.Fatalf("error = %v, debía mencionar %q", err, c.error)
			}
		})
	}
}

func TestLimpiarTextoQuitaControlesYColapsa(t *testing.T) {
	got := limpiarTexto("  hola\x1b[31m\n\tmundo\r\n  fin\x7f ")
	if got != "hola [31m mundo fin" {
		t.Fatalf("limpiarTexto = %q", got)
	}
	if strings.ContainsAny(got, "\x1b\n\t\r\x7f") {
		t.Fatalf("quedaron controles: %q", got)
	}
}
