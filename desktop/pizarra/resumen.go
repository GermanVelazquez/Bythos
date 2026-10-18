package pizarra

import "encoding/json"

// ElementoResumen es un elemento de Excalidraw en el vocabulario del agente.
// Puntos va como conteo (los puntos crudos de un trazo pesan mucho).
type ElementoResumen struct {
	ID     string  `json:"id"`
	Tipo   string  `json:"tipo"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Ancho  float64 `json:"ancho"`
	Alto   float64 `json:"alto"`
	Texto  string  `json:"texto,omitempty"`
	Color  string  `json:"color"`
	Fondo  string  `json:"fondo,omitempty"`
	Grosor float64 `json:"grosor,omitempty"`
	Puntos int     `json:"puntos,omitempty"`
	Desde  string  `json:"desde,omitempty"`
	Hasta  string  `json:"hasta,omitempty"`
	Grupo  string  `json:"grupo,omitempty"`
}

// Conteo cubre TODOS los elementos vivos, también los que el tope dejó fuera.
type Conteo struct {
	Total   int            `json:"total"`
	PorTipo map[string]int `json:"por_tipo"`
}

type Resultado struct {
	Elementos []ElementoResumen `json:"elementos"`
	Resumen   Conteo            `json:"resumen"`
	Truncado  bool              `json:"truncado"`
}

type vinculo struct {
	ElementID string `json:"elementId"`
}

// Resumir convierte la escena guardada en lo que devuelve pizarra_leer: como
// mucho MaxElementos elementos y MaxTextoLeido caracteres de texto cada uno.
// Función pura. Escena vacía = pizarra vacía; JSON inválido = error.
func Resumir(escena []byte) (Resultado, error) {
	r := Resultado{Elementos: []ElementoResumen{}, Resumen: Conteo{PorTipo: map[string]int{}}}
	if len(escena) == 0 {
		return r, nil
	}
	var e struct {
		Elements []struct {
			ID       string       `json:"id"`
			Type     string       `json:"type"`
			Borrado  bool         `json:"isDeleted"`
			X        float64      `json:"x"`
			Y        float64      `json:"y"`
			Width    float64      `json:"width"`
			Height   float64      `json:"height"`
			Text     string       `json:"text"`
			Stroke   string       `json:"strokeColor"`
			Fondo    string       `json:"backgroundColor"`
			Grosor   float64      `json:"strokeWidth"`
			Points   [][2]float64 `json:"points"`
			Inicio   *vinculo     `json:"startBinding"`
			Fin      *vinculo     `json:"endBinding"`
			GroupIDs []string     `json:"groupIds"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(escena, &e); err != nil {
		return r, err
	}
	for _, el := range e.Elements {
		if el.Borrado {
			continue
		}
		r.Resumen.Total++
		r.Resumen.PorTipo[el.Type]++
		if len(r.Elementos) >= MaxElementos {
			r.Truncado = true
			continue
		}
		out := ElementoResumen{
			ID: el.ID, Tipo: el.Type, X: el.X, Y: el.Y, Ancho: el.Width, Alto: el.Height,
			Texto: recortar(el.Text, MaxTextoLeido), Color: el.Stroke, Fondo: el.Fondo,
			Grosor: el.Grosor, Puntos: len(el.Points),
		}
		if el.Inicio != nil {
			out.Desde = el.Inicio.ElementID
		}
		if el.Fin != nil {
			out.Hasta = el.Fin.ElementID
		}
		if len(el.GroupIDs) > 0 {
			out.Grupo = el.GroupIDs[0]
		}
		r.Elementos = append(r.Elementos, out)
	}
	return r, nil
}

func recortar(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max])
	}
	return s
}
