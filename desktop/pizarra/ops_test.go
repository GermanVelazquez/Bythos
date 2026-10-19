package pizarra

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func opsDe(t *testing.T, js string) []Op {
	t.Helper()
	var ops []Op
	if err := json.Unmarshal([]byte(js), &ops); err != nil {
		t.Fatalf("JSON de prueba inválido: %v", err)
	}
	return ops
}

func agregar(elemento string) string { return `[{"accion":"agregar","elemento":` + elemento + `}]` }

func TestValidarCasos(t *testing.T) {
	largo := strings.Repeat("a", 10001)
	puntos := "[" + strings.Repeat("[0,0],", 2000) + "[0,0]]" // 2001 puntos
	casos := []struct {
		nombre, js, error string // error "" = válido
	}{
		{"rectángulo válido", agregar(`{"tipo":"rectangle","x":10,"y":-5,"ancho":100,"alto":20000,"color":"#a1B2c3","fondo":"transparent"}`), ""},
		{"texto con id válido", agregar(`{"id":"nota_1-A","tipo":"text","texto":"hola","x":0,"y":0}`), ""},
		{"actualizar", `[{"accion":"actualizar","id":"a1","cambios":{"x":5,"color":"#000000"}}]`, ""},
		{"borrar", `[{"accion":"borrar","ids":["a1","b2"]}]`, ""},
		{"mover", `[{"accion":"mover","ids":["a1"],"dx":-100000,"dy":100000}]`, ""},
		{"verbo desconocido", `[{"accion":"explotar"}]`, "acción"},
		{"tipo desconocido", agregar(`{"tipo":"hexagon","x":0,"y":0}`), "tipo"},
		{"imagen no se admite", agregar(`{"tipo":"image","x":0,"y":0}`), "imagen"},
		{"agregar sin elemento", `[{"accion":"agregar"}]`, "elemento"},
		{"texto de 10001", agregar(`{"tipo":"text","texto":"` + largo + `","x":0,"y":0}`), "texto"},
		{"x fuera de rango", agregar(`{"tipo":"rectangle","x":100001,"y":0}`), "x"},
		{"y fuera de rango", agregar(`{"tipo":"rectangle","x":0,"y":-100001}`), "y"},
		{"ancho negativo", agregar(`{"tipo":"rectangle","x":0,"y":0,"ancho":-1}`), "ancho"},
		{"alto de 20001", agregar(`{"tipo":"rectangle","x":0,"y":0,"alto":20001}`), "alto"},
		{"2001 puntos", agregar(`{"tipo":"freedraw","x":0,"y":0,"puntos":` + puntos + `}`), "puntos"},
		{"color inválido", agregar(`{"tipo":"rectangle","x":0,"y":0,"color":"red"}`), "color"},
		{"fondo inválido", agregar(`{"tipo":"rectangle","x":0,"y":0,"fondo":"#fff"}`), "fondo"},
		{"id con espacio", agregar(`{"id":"a b","tipo":"rectangle","x":0,"y":0}`), "id"},
		{"id de 65", agregar(`{"id":"` + strings.Repeat("a", 65) + `","tipo":"rectangle","x":0,"y":0}`), "id"},
		{"desde inválido", agregar(`{"tipo":"arrow","x":0,"y":0,"desde":"a/b"}`), "desde"},
		{"borrar sin ids", `[{"accion":"borrar","ids":[]}]`, "ids"},
		{"mover sin dx", `[{"accion":"mover","ids":["a"],"dy":1}]`, "dx"},
		{"actualizar sin id", `[{"accion":"actualizar","cambios":{"x":1}}]`, "id"},
		{"actualizar tipo imagen", `[{"accion":"actualizar","id":"a","cambios":{"tipo":"image"}}]`, "imagen"},
		{"sin operaciones", `[]`, "al menos"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := Validar(opsDe(t, c.js))
			switch {
			case c.error == "" && err != nil:
				t.Fatalf("debía ser válido: %v", err)
			case c.error != "" && (err == nil || !strings.Contains(err.Error(), c.error)):
				t.Fatalf("error = %v, debía mencionar %q", err, c.error)
			}
		})
	}
}

func TestValidarLimiteDeOperaciones(t *testing.T) {
	op := func(n int) []Op {
		ops := make([]Op, n)
		for i := range ops {
			ops[i] = Op{Accion: "borrar", IDs: []string{fmt.Sprint("e", i)}}
		}
		return ops
	}
	if err := Validar(op(200)); err != nil {
		t.Fatalf("200 operaciones debían pasar: %v", err)
	}
	if err := Validar(op(201)); err == nil || !strings.Contains(err.Error(), "200") {
		t.Fatalf("201 operaciones debían fallar mencionando el máximo: %v", err)
	}
}

func TestAsignarIDsSoloALosFaltantes(t *testing.T) {
	ops := opsDe(t, `[{"accion":"agregar","elemento":{"tipo":"text","x":0,"y":0}},
		{"accion":"agregar","elemento":{"id":"fijo","tipo":"ellipse","x":0,"y":0}},
		{"accion":"borrar","ids":["x"]}]`)
	AsignarIDs(ops)
	if ops[1].Elemento.ID != "fijo" {
		t.Fatalf("un id dado no se toca: %q", ops[1].Elemento.ID)
	}
	if id := ops[0].Elemento.ID; !idValido.MatchString(id) {
		t.Fatalf("id asignado inválido: %q", id)
	}
	if err := Validar(ops); err != nil {
		t.Fatalf("tras asignar ids debe seguir válido: %v", err)
	}
}

func TestValidarRechazaDatosYPrefijoReservado(t *testing.T) {
	casos := []struct{ nombre, js, error string }{
		{"datos en agregar", agregar(`{"tipo":"rectangle","x":0,"y":0,"datos":{"bythos":"marca","veredicto":"ok","clave":"k"}}`), "datos"},
		{"datos en actualizar", `[{"accion":"actualizar","id":"a","cambios":{"datos":{"bythos":"marca"}}}]`, "datos"},
		{"id marca- en agregar", agregar(`{"id":"marca-abc-m","tipo":"rectangle","x":0,"y":0}`), "marca-"},
		{"id parecido sin guion", agregar(`{"id":"marcador","tipo":"rectangle","x":0,"y":0}`), ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := Validar(opsDe(t, c.js))
			switch {
			case c.error == "" && err != nil:
				t.Fatalf("debía ser válido: %v", err)
			case c.error != "" && (err == nil || !strings.Contains(err.Error(), c.error)):
				t.Fatalf("error = %v, debía mencionar %q", err, c.error)
			}
		})
	}
}
