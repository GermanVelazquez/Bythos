package api

// export_test.go — Pruebas del repaso SIN base de datos.
// ¿Por qué se puede? Porque markdownCarpeta/geminiRepaso/guiaNotebookLM
// son funciones PURAS: mismo input → mismo output, sin disco ni red.
// Lección senior: lo puro se testea en milisegundos, lo impuro (SQL/HTTP)
// necesita .db temporal y httptest. Separa ambos y testeas 10x más.

import (
	"strings"
	"testing"

	"bythos-desktop/db"
)

func recursosPrueba() []db.Recurso {
	return []db.Recurso{
		{ID: 1, URL: "http://a/1", Titulo: "Intro React", Descripcion: "Desc A", Tipo: "articulo", Estado: "en_curso", Progreso: 40},
		{ID: 2, URL: "http://a/2", Titulo: "", Descripcion: "", Tipo: "youtube", Estado: "pendiente", Progreso: 0},
	}
}

func TestMarkdown(t *testing.T) {
	out := markdownCarpeta("React", recursosPrueba())
	if !strings.Contains(out, "# React") {
		t.Fatal("falta encabezado con nombre")
	}
	// New per-link format carries % and id for the import round-trip.
	if !strings.Contains(out, "- [40%] Intro React <!-- id:1 -->") {
		t.Fatalf("falta linea con %% e id, salio:\n%s", out)
	}
	// Without title the URL is the fallback, progress still present.
	if !strings.Contains(out, "- [0%] http://a/2 <!-- id:2 -->") {
		t.Fatalf("sin título debe usar la URL con %% e id, salio:\n%s", out)
	}
	if v := markdownCarpeta("Vacia", nil); !strings.Contains(v, "vacía") {
		t.Fatal("carpeta vacía debe avisarlo")
	}
}

func TestMarkdownClamp(t *testing.T) {
	casos := []struct {
		nombre   string
		progreso int
		esperado string
	}{
		{"negative clamps to zero", -5, "- [0%] T <!-- id:9 -->"},
		{"over one hundred clamps", 150, "- [100%] T <!-- id:9 -->"},
		{"exact hundred", 100, "- [100%] T <!-- id:9 -->"},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			out := markdownCarpeta("C", []db.Recurso{{ID: 9, Titulo: "T", Progreso: tt.progreso}})
			if !strings.Contains(out, tt.esperado) {
				t.Fatalf("esperaba %q, salio:\n%s", tt.esperado, out)
			}
		})
	}
}

func TestGemini(t *testing.T) {
	out := geminiRepaso("React", recursosPrueba())
	if !strings.Contains(out, "Gemini") || !strings.Contains(out, "Intro React") {
		t.Fatal("debe servir para pegar en Gemini con tus links")
	}
	if !strings.Contains(out, "10 preguntas") {
		t.Fatal("debe pedir preguntas de repaso")
	}
}

func TestNotebookLM(t *testing.T) {
	out := guiaNotebookLM("React", recursosPrueba())
	if !strings.Contains(out, "notebooklm.google.com") {
		t.Fatal("debe decir dónde importar")
	}
	if !strings.Contains(out, "http://a/1") || !strings.Contains(out, "http://a/2") {
		t.Fatal("debe listar cada URL para pegar una por una")
	}
}
