package pizarra

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Límites de pizarra_marcar (design D9).
const (
	MaxMarcas    = 20
	MaxPista     = 300
	MaxPasaje    = 200
	MaxPagina    = 100000
	prefijoMarca = "marca-" // prefijo reservado de los ids de marca (D4)
)

// veredictos admitidos en una marca.
var veredictos = map[string]bool{"ok": true, "error": true, "parcial": true}

// Objetivo es lo que se marca: ids de elementos o un rectángulo (x, y, ancho, alto).
type Objetivo struct {
	IDs   []string `json:"ids,omitempty"`
	X     *float64 `json:"x,omitempty"`
	Y     *float64 `json:"y,omitempty"`
	Ancho *float64 `json:"ancho,omitempty"`
	Alto  *float64 `json:"alto,omitempty"`
}

// Cita apunta a un recurso de la carpeta de la sesión. El título que se ve lo
// pone Bythos desde la base de datos, nunca el agente (D8).
type Cita struct {
	RecursoID int64  `json:"recurso_id"`
	Pagina    *int   `json:"pagina,omitempty"`
	Pasaje    string `json:"pasaje,omitempty"`
}

// Marca es un paso del alumno con su veredicto, una pista opcional y una cita opcional.
type Marca struct {
	Objetivo  Objetivo `json:"objetivo"`
	Veredicto string   `json:"veredicto"`
	Pista     string   `json:"pista,omitempty"`
	Cita      *Cita    `json:"cita,omitempty"`
}

// DatosMarca es el customData que identifica a un elemento como marca (D4).
type DatosMarca struct {
	Bythos    string `json:"bythos"`
	Veredicto string `json:"veredicto"`
	Clave     string `json:"clave"`
}

// ValidarMarcas devuelve el primer problema de la llamada (mensaje para el
// agente) o nil. Una marca inválida rechaza la llamada entera.
func ValidarMarcas(marcas []Marca) error {
	if len(marcas) == 0 {
		return fmt.Errorf("manda al menos una marca")
	}
	if len(marcas) > MaxMarcas {
		return fmt.Errorf("máximo %d marcas por llamada (llegaron %d)", MaxMarcas, len(marcas))
	}
	for i, m := range marcas {
		if err := validarMarca(m); err != nil {
			return fmt.Errorf("marca %d: %w", i+1, err)
		}
	}
	return nil
}

func validarMarca(m Marca) error {
	if !veredictos[m.Veredicto] {
		return fmt.Errorf("veredicto desconocido %q (usa ok, error o parcial)", m.Veredicto)
	}
	if utf8.RuneCountInString(m.Pista) > MaxPista {
		return fmt.Errorf("pista de más de %d caracteres", MaxPista)
	}
	if err := validarObjetivo(m.Objetivo); err != nil {
		return err
	}
	if c := m.Cita; c != nil {
		if c.RecursoID <= 0 {
			return fmt.Errorf("cita: recurso_id inválido")
		}
		if c.Pagina != nil && (*c.Pagina < 1 || *c.Pagina > MaxPagina) {
			return fmt.Errorf("cita: página fuera de rango (1 a %d)", MaxPagina)
		}
		if utf8.RuneCountInString(c.Pasaje) > MaxPasaje {
			return fmt.Errorf("cita: pasaje de más de %d caracteres", MaxPasaje)
		}
	}
	return nil
}

func validarObjetivo(o Objetivo) error {
	area := []*float64{o.X, o.Y, o.Ancho, o.Alto}
	dadas := 0
	for _, v := range area {
		if v != nil {
			dadas++
		}
	}
	switch {
	case len(o.IDs) > 0 && dadas > 0:
		return fmt.Errorf("objetivo: manda ids o un área, no ambos")
	case len(o.IDs) > 0:
		return validarIDs(o.IDs)
	case dadas != 4:
		return fmt.Errorf("objetivo: manda ids o un área completa (x, y, ancho, alto)")
	}
	for _, c := range []struct {
		campo string
		v     float64
		min   float64
		max   float64
	}{
		{"x", *o.X, -MaxCoord, MaxCoord}, {"y", *o.Y, -MaxCoord, MaxCoord},
		{"ancho", *o.Ancho, 1, MaxTamano}, {"alto", *o.Alto, 1, MaxTamano},
	} {
		if err := rango(c.campo, c.v, c.min, c.max); err != nil {
			return fmt.Errorf("objetivo: %w", err)
		}
	}
	return nil
}

// limpiarTexto vuelve espacios los caracteres de control y los saltos de línea,
// colapsa los espacios y recorta. Es la defensa contra texto del agente con
// secuencias de escape o formato (design D9).
func limpiarTexto(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}
