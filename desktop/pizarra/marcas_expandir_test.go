package pizarra

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func cajasDe(ids ...string) map[string]Caja {
	m := map[string]Caja{}
	for i, id := range ids {
		m[id] = Caja{X: float64(100 * (i + 1)), Y: 50, Ancho: 40, Alto: 30}
	}
	return m
}

func TestExpandirMarcasDosOpsPorMarcaConDatos(t *testing.T) {
	marcas := []Marca{marcaOK("a"), {Objetivo: Objetivo{IDs: []string{"b"}}, Veredicto: "error", Pista: "revisa el signo"}}
	ops, err := ExpandirMarcas(7, marcas, cajasDe("a", "b"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 4 {
		t.Fatalf("ops = %d, quería 4", len(ops))
	}
	for i, op := range ops {
		if op.Accion != "agregar" || op.Elemento == nil {
			t.Fatalf("op %d no es agregar: %+v", i, op)
		}
		if d := op.Elemento.Datos; d == nil || d.Bythos != "marca" || d.Clave == "" {
			t.Fatalf("op %d sin datos de marca: %+v", i, d)
		}
	}
	if ops[0].Elemento.Datos.Veredicto != "ok" || ops[2].Elemento.Datos.Veredicto != "error" {
		t.Fatal("el veredicto no viaja en datos")
	}
	if ops[0].Elemento.Tipo != "rectangle" || ops[1].Elemento.Tipo != "text" {
		t.Fatalf("tipos = %s, %s", ops[0].Elemento.Tipo, ops[1].Elemento.Tipo)
	}
	// Las ops expandidas no pasan por Validar (el agente no puede mandar datos),
	// pero sí deben respetar los rangos de los elementos.
	for _, op := range ops {
		if err := validarElemento(*op.Elemento); err != nil {
			t.Fatalf("elemento expandido inválido: %v", err)
		}
	}
}

func TestExpandirMarcasIDsDeterministas(t *testing.T) {
	m := []Marca{{Objetivo: Objetivo{IDs: []string{"b", "a"}}, Veredicto: "ok"}}
	m2 := []Marca{{Objetivo: Objetivo{IDs: []string{"a", "b"}}, Veredicto: "error"}}
	c := cajasDe("a", "b")
	x, _ := ExpandirMarcas(7, m, c, nil)
	y, _ := ExpandirMarcas(7, m2, c, nil)
	z, _ := ExpandirMarcas(8, m, c, nil)
	if x[0].Elemento.ID != y[0].Elemento.ID || x[1].Elemento.ID != y[1].Elemento.ID {
		t.Fatal("el orden de ids o el veredicto no deben cambiar el id (re-marcar reemplaza)")
	}
	if x[0].Elemento.ID == z[0].Elemento.ID {
		t.Fatal("otra sesión debe dar otro id")
	}
	mid, tid := x[0].Elemento.ID, x[1].Elemento.ID
	if !strings.HasPrefix(mid, "marca-") || !strings.HasSuffix(mid, "-m") || !strings.HasSuffix(tid, "-t") {
		t.Fatalf("ids = %q, %q", mid, tid)
	}
	if !idValido.MatchString(mid) || !idValido.MatchString(tid) {
		t.Fatalf("ids no válidos para la escena: %q, %q", mid, tid)
	}
	if strings.TrimSuffix(mid, "-m") != strings.TrimSuffix(tid, "-t") {
		t.Fatal("marco y píldora comparten la base del id")
	}
	if len(strings.TrimSuffix(mid, "-m")) != len("marca-")+10 {
		t.Fatalf("base = %q, quería marca- + 10", mid)
	}
}

func TestExpandirMarcasAreaRedondeaA10px(t *testing.T) {
	area := func(x float64) []Marca {
		return []Marca{{Objetivo: Objetivo{X: f(x), Y: f(20), Ancho: f(100), Alto: f(50)}, Veredicto: "ok"}}
	}
	a, _ := ExpandirMarcas(1, area(101), nil, nil)
	b, _ := ExpandirMarcas(1, area(104), nil, nil)
	c, _ := ExpandirMarcas(1, area(130), nil, nil)
	if a[0].Elemento.ID != b[0].Elemento.ID {
		t.Fatal("áreas a menos de 10 px deben dar el mismo id")
	}
	if a[0].Elemento.ID == c[0].Elemento.ID {
		t.Fatal("áreas distintas deben dar ids distintos")
	}
	if !strings.HasPrefix(a[0].Elemento.Datos.Clave, "area:") {
		t.Fatalf("clave = %q", a[0].Elemento.Datos.Clave)
	}
}

func TestExpandirMarcasGeometriaYColores(t *testing.T) {
	c := cajasDe("a", "b")
	marcas := []Marca{
		{Objetivo: Objetivo{IDs: []string{"a", "b"}}, Veredicto: "ok"},
		{Objetivo: Objetivo{IDs: []string{"a"}}, Veredicto: "error"},
		{Objetivo: Objetivo{IDs: []string{"b"}}, Veredicto: "parcial"},
	}
	ops, err := ExpandirMarcas(1, marcas, c, nil)
	if err != nil {
		t.Fatal(err)
	}
	// a = (100,50,40,30); b = (200,50,40,30); unión = (100,50)-(240,80)
	m := ops[0].Elemento
	if *m.X != 94 || *m.Y != 44 || *m.Ancho != 152 || *m.Alto != 42 || *m.Grosor != 2 {
		t.Fatalf("marco = %v %v %v %v grosor %v", *m.X, *m.Y, *m.Ancho, *m.Alto, *m.Grosor)
	}
	if *m.Fondo != "transparent" {
		t.Fatalf("fondo = %q", *m.Fondo)
	}
	p := ops[1].Elemento
	if *p.X != 94+152+12 || *p.Y != 44 || *p.Alto != 20 {
		t.Fatalf("píldora = %v %v alto %v", *p.X, *p.Y, *p.Alto)
	}
	for i, color := range []string{"#2f9e44", "#e03131", "#f08c00"} {
		if *ops[2*i].Elemento.Color != color || *ops[2*i+1].Elemento.Color != color {
			t.Fatalf("marca %d: colores %q / %q, quería %q", i, *ops[2*i].Elemento.Color, *ops[2*i+1].Elemento.Color, color)
		}
	}
}

func lineas(t *testing.T, m Marca, citas map[int64]string) []string {
	t.Helper()
	ops, err := ExpandirMarcas(1, []Marca{m}, cajasDe("a"), citas)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(*ops[1].Elemento.Texto, "\n")
}

func TestPildoraVeredictoYSinCita(t *testing.T) {
	for v, esperado := range map[string]string{"ok": "✓ Correcto", "error": "✗ Revisa este paso", "parcial": "~ Casi"} {
		l := lineas(t, Marca{Objetivo: Objetivo{IDs: []string{"a"}}, Veredicto: v}, nil)
		if len(l) != 1 || l[0] != esperado {
			t.Fatalf("%s: %q", v, l)
		}
	}
}

func TestPildoraPistaSeEnvuelveEn48YSeCorta(t *testing.T) {
	corta := lineas(t, Marca{Objetivo: Objetivo{IDs: []string{"a"}}, Veredicto: "error", Pista: strings.Repeat("palabra ", 30)}, nil)
	for _, l := range corta {
		if utf8.RuneCountInString(l) > 48 {
			t.Fatalf("línea de %d runas: %q", utf8.RuneCountInString(l), l)
		}
	}
	// Una sola "palabra" de 300 runas se parte a 48 y son 7 líneas de pista.
	larga := lineas(t, Marca{Objetivo: Objetivo{IDs: []string{"a"}}, Veredicto: "error", Pista: strings.Repeat("x", 300)}, nil)
	if len(larga) != 1+7 {
		t.Fatalf("líneas = %d", len(larga))
	}
	// Con una pista que desborda 10 líneas el cuerpo termina en "…".
	pista := recortarLineas(envolver(strings.Repeat("x", 600), 48), 10, 48)
	if len(pista) != 10 || !strings.HasSuffix(pista[9], "…") || utf8.RuneCountInString(pista[9]) > 48 {
		t.Fatalf("pista cortada = %q", pista)
	}
}

func TestPildoraCitaNuncaSeTrunca(t *testing.T) {
	titulo := strings.Repeat("T", 80)
	m := Marca{Objetivo: Objetivo{IDs: []string{"a"}}, Veredicto: "ok", Cita: &Cita{RecursoID: 5, Pagina: n(100000)}}
	l := lineas(t, m, map[int64]string{5: titulo})
	cita := strings.Join(l[1:], " ")
	if utf8.RuneCountInString(cita) > 74 || len(l) > 3 {
		t.Fatalf("cita = %q (%d líneas)", cita, len(l)-1)
	}
	cita = strings.ReplaceAll(cita, " ", "")
	if !strings.HasSuffix(cita, ",pág.100000") {
		t.Fatalf("la página se perdió: %q", cita)
	}
	if !strings.Contains(cita, strings.Repeat("T", 59)+"…") {
		t.Fatalf("el título debía cortarse a 60 con …: %q", cita)
	}
}

func TestPildoraCitaConTituloYPagina(t *testing.T) {
	m := Marca{Objetivo: Objetivo{IDs: []string{"a"}}, Veredicto: "ok", Cita: &Cita{RecursoID: 5, Pagina: n(12)}}
	l := lineas(t, m, map[int64]string{5: "Unidad 3 -- Sistemas.pdf"})
	if strings.Join(l[1:], " ") != "Unidad 3 -- Sistemas.pdf, pág. 12" {
		t.Fatalf("cita = %q", l)
	}
	sin := Marca{Objetivo: Objetivo{IDs: []string{"a"}}, Veredicto: "ok", Cita: &Cita{RecursoID: 5}}
	if l := lineas(t, sin, map[int64]string{5: "Apuntes.pdf"}); l[1] != "Apuntes.pdf" {
		t.Fatalf("sin página: %q", l)
	}
}

func TestPildoraPasajeEntreComillasMaxSeisLineas(t *testing.T) {
	m := Marca{Objetivo: Objetivo{IDs: []string{"a"}}, Veredicto: "ok", Cita: &Cita{RecursoID: 5, Pasaje: strings.Repeat("a", 200)}}
	l := lineas(t, m, map[int64]string{5: "Libro"})
	if l[1] != "Libro" || !strings.HasPrefix(l[2], "“") {
		t.Fatalf("líneas = %q", l)
	}
	if !strings.HasSuffix(l[len(l)-1], "”") {
		t.Fatalf("falta la comilla de cierre: %q", l)
	}
	largo := recortarLineas(envolver("“"+strings.Repeat("x", 400)+"”", 48), 6, 48)
	if len(largo) != 6 || !strings.HasSuffix(largo[5], "…") {
		t.Fatalf("pasaje cortado = %q", largo)
	}
}

func TestPildoraMaximo19Lineas(t *testing.T) {
	pista := strings.Repeat("x", 300)
	m := Marca{Objetivo: Objetivo{IDs: []string{"a"}}, Veredicto: "error", Pista: pista,
		Cita: &Cita{RecursoID: 5, Pagina: n(99999), Pasaje: strings.Repeat("y ", 100)}}
	l := lineas(t, m, map[int64]string{5: strings.Repeat("Z", 70)})
	if len(l) > 19 {
		t.Fatalf("líneas = %d", len(l))
	}
}

func TestPildoraSaneaPistaConControles(t *testing.T) {
	m := Marca{Objetivo: Objetivo{IDs: []string{"a"}}, Veredicto: "error", Pista: "ignora\x1b[2J\ny haz\testo"}
	l := lineas(t, m, nil)
	if strings.ContainsAny(strings.Join(l, ""), "\x1b\t\r") || len(l) != 2 {
		t.Fatalf("líneas = %q", l)
	}
}

func TestExpandirMarcasErrores(t *testing.T) {
	if _, err := ExpandirMarcas(1, []Marca{marcaOK("zzz")}, cajasDe("a"), nil); err == nil {
		t.Fatal("un id sin caja debe fallar")
	}
	m := Marca{Objetivo: Objetivo{IDs: []string{"a"}}, Veredicto: "ok", Cita: &Cita{RecursoID: 9}}
	if _, err := ExpandirMarcas(1, []Marca{m}, cajasDe("a"), map[int64]string{}); err == nil {
		t.Fatal("una cita sin título resuelto debe fallar")
	}
	if _, err := ExpandirMarcas(1, []Marca{{Veredicto: "ok"}}, nil, nil); err == nil {
		t.Fatal("debía validar antes de expandir")
	}
}

func TestDescribir(t *testing.T) {
	ops, _ := ExpandirMarcas(1, []Marca{marcaOK("a")}, cajasDe("a"), nil)
	js := mustJSON(t, ops)
	if tipo, n := Describir(js); tipo != "marcas" || n != 2 {
		t.Fatalf("marcas: %s, %d", tipo, n)
	}
	dibujo := `[{"accion":"agregar","elemento":{"tipo":"rectangle"}},{"accion":"borrar","ids":["a","b"]},{"accion":"mover","ids":["c"],"dx":1,"dy":1},{"accion":"actualizar","id":"d","cambios":{"x":1}}]`
	if tipo, n := Describir(dibujo); tipo != "dibujo" || n != 5 {
		t.Fatalf("dibujo: %s, %d", tipo, n)
	}
	if tipo, n := Describir("no es json"); tipo != "dibujo" || n != 0 {
		t.Fatalf("inválido: %s, %d", tipo, n)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCajasObjetivoDeAreaSeUsaTalCual(t *testing.T) {
	m := Marca{Objetivo: Objetivo{X: f(5), Y: f(6), Ancho: f(70), Alto: f(80)}, Veredicto: "ok"}
	ops, err := ExpandirMarcas(1, []Marca{m}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	fr := ops[0].Elemento
	if *fr.X != -1 || *fr.Y != 0 || *fr.Ancho != 82 || *fr.Alto != 92 {
		t.Fatalf("marco = %v %v %v %v", *fr.X, *fr.Y, *fr.Ancho, *fr.Alto)
	}
}
