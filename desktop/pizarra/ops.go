// Package pizarra es el dominio puro de la pizarra del agente: validar las
// operaciones de pizarra_dibujar y resumir la escena para pizarra_leer.
// No toca HTTP ni SQLite (design D1): el mismo validador sirve al proceso MCP
// (validación temprana) y a la API (la que manda).
package pizarra

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Límites (design, tabla Limits). Fuente única, espejada en las specs.
const (
	MaxOps        = 200
	MaxTexto      = 10000
	MaxCoord      = 100000
	MaxTamano     = 20000
	MaxPuntos     = 2000
	MaxElementos  = 2000 // pizarra_leer
	MaxTextoLeido = 2000 // caracteres de texto por elemento en pizarra_leer
)

var (
	idValido    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	colorValido = regexp.MustCompile(`^(#[0-9a-fA-F]{6}|transparent)$`)
	tipos       = map[string]bool{"rectangle": true, "ellipse": true, "diamond": true, "arrow": true, "line": true, "text": true, "freedraw": true}
)

// Elemento son los campos que el agente puede fijar. Los punteros distinguen
// "no mandado" de cero (en "actualizar" solo cambia lo presente).
type Elemento struct {
	ID     string       `json:"id,omitempty"`
	Tipo   string       `json:"tipo,omitempty"`
	X      *float64     `json:"x,omitempty"`
	Y      *float64     `json:"y,omitempty"`
	Ancho  *float64     `json:"ancho,omitempty"`
	Alto   *float64     `json:"alto,omitempty"`
	Texto  *string      `json:"texto,omitempty"`
	Color  *string      `json:"color,omitempty"`
	Fondo  *string      `json:"fondo,omitempty"`
	Grosor *float64     `json:"grosor,omitempty"`
	Puntos [][2]float64 `json:"puntos,omitempty"`
	Desde  string       `json:"desde,omitempty"`
	Hasta  string       `json:"hasta,omitempty"`
	// Datos solo lo fija el servidor (marcas, design D4): Validar lo rechaza si lo manda el agente.
	Datos *DatosMarca `json:"datos,omitempty"`
}

// Op es un verbo de pizarra_dibujar: agregar{elemento}, actualizar{id,cambios},
// borrar{ids}, mover{ids,dx,dy}.
type Op struct {
	Accion   string    `json:"accion"`
	Elemento *Elemento `json:"elemento,omitempty"`
	ID       string    `json:"id,omitempty"`
	Cambios  *Elemento `json:"cambios,omitempty"`
	IDs      []string  `json:"ids,omitempty"`
	DX       *float64  `json:"dx,omitempty"`
	DY       *float64  `json:"dy,omitempty"`
}

// Validar devuelve el primer problema (mensaje para el agente) o nil.
func Validar(ops []Op) error {
	if len(ops) == 0 {
		return fmt.Errorf("manda al menos una operación")
	}
	if len(ops) > MaxOps {
		return fmt.Errorf("máximo %d operaciones por llamada (llegaron %d)", MaxOps, len(ops))
	}
	for i, op := range ops {
		if err := validarOp(op); err != nil {
			return fmt.Errorf("operación %d: %w", i+1, err)
		}
	}
	return nil
}

func validarOp(op Op) error {
	switch op.Accion {
	case "agregar":
		if op.Elemento == nil {
			return fmt.Errorf("agregar necesita un elemento")
		}
		if op.Elemento.Tipo == "" {
			return fmt.Errorf("el elemento necesita tipo")
		}
		if err := rechazarReservado(*op.Elemento); err != nil {
			return err
		}
		return validarElemento(*op.Elemento)
	case "actualizar":
		if !idValido.MatchString(op.ID) {
			return fmt.Errorf("id inválido")
		}
		if op.Cambios == nil {
			return fmt.Errorf("actualizar necesita cambios")
		}
		if err := rechazarReservado(*op.Cambios); err != nil {
			return err
		}
		return validarElemento(*op.Cambios)
	case "borrar":
		return validarIDs(op.IDs)
	case "mover":
		if err := validarIDs(op.IDs); err != nil {
			return err
		}
		if op.DX == nil || op.DY == nil {
			return fmt.Errorf("mover necesita dx y dy")
		}
		if err := rango("dx", *op.DX, -MaxCoord, MaxCoord); err != nil {
			return err
		}
		return rango("dy", *op.DY, -MaxCoord, MaxCoord)
	}
	return fmt.Errorf("acción desconocida %q (usa agregar, actualizar, borrar o mover)", op.Accion)
}

// rechazarReservado impide que el agente falsifique marcas: ni `datos` ni ids con
// el prefijo reservado (design D4).
func rechazarReservado(e Elemento) error {
	if e.Datos != nil {
		return fmt.Errorf("el campo datos no está permitido")
	}
	if strings.HasPrefix(e.ID, prefijoMarca) {
		return fmt.Errorf("los ids que empiezan con %q están reservados", prefijoMarca)
	}
	return nil
}

func validarIDs(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("faltan ids")
	}
	for _, id := range ids {
		if !idValido.MatchString(id) {
			return fmt.Errorf("id inválido %q", id)
		}
	}
	return nil
}

func rango(campo string, v, min, max float64) error {
	if v < min || v > max {
		return fmt.Errorf("%s fuera de rango (%g a %g)", campo, min, max)
	}
	return nil
}

func validarElemento(e Elemento) error {
	if e.Tipo == "image" {
		return fmt.Errorf("no se admiten elementos de tipo imagen")
	}
	if e.Tipo != "" && !tipos[e.Tipo] {
		return fmt.Errorf("tipo desconocido %q", e.Tipo)
	}
	if e.ID != "" && !idValido.MatchString(e.ID) {
		return fmt.Errorf("id inválido %q", e.ID)
	}
	for campo, id := range map[string]string{"desde": e.Desde, "hasta": e.Hasta} {
		if id != "" && !idValido.MatchString(id) {
			return fmt.Errorf("%s inválido %q", campo, id)
		}
	}
	if e.Texto != nil && utf8.RuneCountInString(*e.Texto) > MaxTexto {
		return fmt.Errorf("texto de más de %d caracteres", MaxTexto)
	}
	if len(e.Puntos) > MaxPuntos {
		return fmt.Errorf("más de %d puntos", MaxPuntos)
	}
	for _, p := range e.Puntos {
		if rango("puntos", p[0], -MaxCoord, MaxCoord) != nil || rango("puntos", p[1], -MaxCoord, MaxCoord) != nil {
			return fmt.Errorf("puntos fuera de rango (±%d)", MaxCoord)
		}
	}
	for campo, c := range map[string]*string{"color": e.Color, "fondo": e.Fondo} {
		if c != nil && !colorValido.MatchString(*c) {
			return fmt.Errorf("%s inválido: usa #rrggbb o transparent", campo)
		}
	}
	for _, c := range []struct {
		campo string
		v     *float64
		min   float64
		max   float64
	}{
		{"x", e.X, -MaxCoord, MaxCoord}, {"y", e.Y, -MaxCoord, MaxCoord},
		{"ancho", e.Ancho, 0, MaxTamano}, {"alto", e.Alto, 0, MaxTamano}, {"grosor", e.Grosor, 0, MaxTamano},
	} {
		if c.v != nil {
			if err := rango(c.campo, *c.v, c.min, c.max); err != nil {
				return err
			}
		}
	}
	return nil
}

// AsignarIDs da un id a cada elemento de "agregar" que no lo trae. Con ids
// fijados por el servidor, reaplicar un lote tras un ack perdido hace merge
// por id en vez de duplicar (design D12).
func AsignarIDs(ops []Op) {
	for i := range ops {
		if e := ops[i].Elemento; ops[i].Accion == "agregar" && e != nil && e.ID == "" {
			b := make([]byte, 9)
			rand.Read(b)
			e.ID = base64.RawURLEncoding.EncodeToString(b)
		}
	}
}
