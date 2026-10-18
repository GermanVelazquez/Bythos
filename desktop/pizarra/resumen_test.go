package pizarra

import (
	"fmt"
	"strings"
	"testing"
)

func escenaConElementos(n int, tipo string) string {
	els := make([]string, n)
	for i := range els {
		els[i] = fmt.Sprintf(`{"id":"e%d","type":%q,"x":1,"y":2,"width":3,"height":4,"strokeColor":"#000000"}`, i, tipo)
	}
	return `{"elements":[` + strings.Join(els, ",") + `],"appState":{},"files":{}}`
}

func TestResumirTruncaA2000ElementosYMarcaTruncado(t *testing.T) {
	r, err := Resumir([]byte(escenaConElementos(2500, "rectangle")))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Elementos) != MaxElementos || !r.Truncado {
		t.Fatalf("elementos=%d truncado=%v", len(r.Elementos), r.Truncado)
	}
	if r.Resumen.Total != 2500 || r.Resumen.PorTipo["rectangle"] != 2500 {
		t.Fatalf("el resumen cuenta TODOS los elementos, no solo los devueltos: %+v", r.Resumen)
	}
}

func TestResumirBajoElLimiteNoTrunca(t *testing.T) {
	r, _ := Resumir([]byte(escenaConElementos(3, "ellipse")))
	if len(r.Elementos) != 3 || r.Truncado {
		t.Fatalf("elementos=%d truncado=%v", len(r.Elementos), r.Truncado)
	}
}

func TestResumirCamposYReglas(t *testing.T) {
	largo := strings.Repeat("ñ", 2500)
	escena := `{"elements":[
		{"id":"t","type":"text","x":1,"y":2,"width":30,"height":10,"text":"` + largo + `","strokeColor":"#111111","backgroundColor":"transparent","strokeWidth":2,"groupIds":["g1"]},
		{"id":"f","type":"freedraw","x":0,"y":0,"width":5,"height":5,"points":[[0,0],[1,1],[2,2]],"strokeColor":"#222222"},
		{"id":"a","type":"arrow","x":0,"y":0,"width":9,"height":9,"strokeColor":"#333333","startBinding":{"elementId":"t"},"endBinding":{"elementId":"f"}},
		{"id":"b","type":"rectangle","isDeleted":true,"x":0,"y":0,"width":1,"height":1,"strokeColor":"#444444"}
	],"appState":{},"files":{}}`
	r, err := Resumir([]byte(escena))
	if err != nil {
		t.Fatal(err)
	}
	if r.Resumen.Total != 3 || r.Resumen.PorTipo["rectangle"] != 0 {
		t.Fatalf("los borrados no cuentan: %+v", r.Resumen)
	}
	t0, f, a := r.Elementos[0], r.Elementos[1], r.Elementos[2]
	if n := len([]rune(t0.Texto)); n != MaxTextoLeido {
		t.Fatalf("texto de %d caracteres, el tope es %d", n, MaxTextoLeido)
	}
	if t0.Tipo != "text" || t0.Color != "#111111" || t0.Fondo != "transparent" || t0.Grosor != 2 || t0.Grupo != "g1" || t0.Ancho != 30 {
		t.Fatalf("texto mal mapeado: %+v", t0)
	}
	if f.Puntos != 3 {
		t.Fatalf("puntos solo como conteo, llegó %d", f.Puntos)
	}
	if a.Desde != "t" || a.Hasta != "f" {
		t.Fatalf("flecha mal mapeada: %+v", a)
	}
}

func TestResumirEscenaInvalida(t *testing.T) {
	if _, err := Resumir([]byte(`no es json`)); err == nil {
		t.Fatal("JSON inválido debía fallar")
	}
	if r, err := Resumir(nil); err != nil || r.Resumen.Total != 0 || r.Elementos == nil {
		t.Fatalf("escena vacía: %+v %v", r, err)
	}
}
