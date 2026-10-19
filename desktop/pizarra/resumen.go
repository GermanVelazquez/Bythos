package pizarra

import (
	"encoding/json"
	"fmt"
	"math"
)

// ElementoResumen es un elemento de Excalidraw en el vocabulario del agente.
// Puntos va como conteo (los puntos crudos de un trazo pesan mucho).
type ElementoResumen struct {
	ID      string  `json:"id"`
	Tipo    string  `json:"tipo"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Ancho   float64 `json:"ancho"`
	Alto    float64 `json:"alto"`
	Texto   string  `json:"texto,omitempty"`
	Color   string  `json:"color"`
	Fondo   string  `json:"fondo,omitempty"`
	Grosor  float64 `json:"grosor,omitempty"`
	Puntos  int     `json:"puntos,omitempty"`
	Desde   string  `json:"desde,omitempty"`
	Hasta   string  `json:"hasta,omitempty"`
	Grupo   string  `json:"grupo,omitempty"`
	Adjunto int64   `json:"adjunto,omitempty"` // id del adjunto (lo completa la api con el sha)
	SHA     string  `json:"-"`                 // fileId de la imagen o customData.sha256 de la tarjeta
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
			FileID   string       `json:"fileId"`
			Datos    *struct {
				SHA string `json:"sha256"`
			} `json:"customData"`
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
		out.SHA = el.FileID
		if out.SHA == "" && el.Datos != nil {
			out.SHA = el.Datos.SHA
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

// Referencias devuelve, por sha256, los ids de los elementos VIVOS de la escena
// que apuntan a ese adjunto: fileId de una imagen o customData.sha256 de una
// tarjeta. Mira todos los elementos (Resumir se corta en MaxElementos). Es lo
// que decide qué adjuntos puede leer el agente (design D5).
func Referencias(escena []byte) (map[string][]string, error) {
	refs := map[string][]string{}
	if len(escena) == 0 {
		return refs, nil
	}
	var e struct {
		Elements []struct {
			ID      string `json:"id"`
			Borrado bool   `json:"isDeleted"`
			FileID  string `json:"fileId"`
			Datos   *struct {
				SHA string `json:"sha256"`
			} `json:"customData"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(escena, &e); err != nil {
		return nil, err
	}
	for _, el := range e.Elements {
		sha := el.FileID
		if sha == "" && el.Datos != nil {
			sha = el.Datos.SHA
		}
		if !el.Borrado && sha != "" {
			refs[sha] = append(refs[sha], el.ID)
		}
	}
	return refs, nil
}

// Caja es el rectángulo de un elemento de la escena guardada.
type Caja struct{ X, Y, Ancho, Alto float64 }

// Cajas resuelve cada id contra la escena guardada (design D7). Un elemento
// lineal usa la caja de sus puntos (relativos a x, y). Un id que no existe o
// está borrado es un error: el agente no puede marcar lo que no hay.
func Cajas(escena []byte, ids []string) (map[string]Caja, error) {
	cajas := map[string]Caja{}
	if len(ids) == 0 {
		return cajas, nil
	}
	var e struct {
		Elements []struct {
			ID      string       `json:"id"`
			Borrado bool         `json:"isDeleted"`
			X       float64      `json:"x"`
			Y       float64      `json:"y"`
			Width   float64      `json:"width"`
			Height  float64      `json:"height"`
			Points  [][2]float64 `json:"points"`
		} `json:"elements"`
	}
	if len(escena) > 0 {
		if err := json.Unmarshal(escena, &e); err != nil {
			return nil, err
		}
	}
	vivos := map[string]int{}
	for i, el := range e.Elements {
		if !el.Borrado {
			vivos[el.ID] = i
		}
	}
	for _, id := range ids {
		i, ok := vivos[id]
		if !ok {
			return nil, fmt.Errorf("el elemento %q no existe en la pizarra", id)
		}
		el := e.Elements[i]
		c := Caja{X: el.X, Y: el.Y, Ancho: el.Width, Alto: el.Height}
		if len(el.Points) > 0 {
			minX, minY, maxX, maxY := el.Points[0][0], el.Points[0][1], el.Points[0][0], el.Points[0][1]
			for _, p := range el.Points[1:] {
				minX, maxX = math.Min(minX, p[0]), math.Max(maxX, p[0])
				minY, maxY = math.Min(minY, p[1]), math.Max(maxY, p[1])
			}
			c = Caja{X: el.X + minX, Y: el.Y + minY, Ancho: maxX - minX, Alto: maxY - minY}
		}
		cajas[id] = c
	}
	return cajas, nil
}
