package pizarra

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

// Geometría y texto de la píldora (design D6).
const (
	anchoPildora    = 48 // runas por línea de la píldora
	maxLineasPista  = 10
	maxLineasPasaje = 6
	maxTituloCita   = 60
	margenMarco     = 6
)

// Colores del marco y la píldora por veredicto (D6) y su texto de cabecera.
var (
	colorVeredicto = map[string]string{"ok": "#2f9e44", "error": "#e03131", "parcial": "#f08c00"}
	textoVeredicto = map[string]string{"ok": "✓ Correcto", "error": "✗ Revisa este paso", "parcial": "~ Casi"}
)

// ExpandirMarcas convierte cada marca en dos ops "agregar": el marco y la
// píldora de texto (D3). cajas resuelve los ids de los objetivos (ver Cajas) y
// citas da el título ya resuelto de cada recurso citado (lo resuelve la API
// contra la carpeta de la sesión). Las ops salen etiquetadas con Datos, que
// Validar rechaza si las manda el agente.
func ExpandirMarcas(sesion int64, marcas []Marca, cajas map[string]Caja, citas map[int64]string) ([]Op, error) {
	if err := ValidarMarcas(marcas); err != nil {
		return nil, err
	}
	ops := make([]Op, 0, 2*len(marcas))
	for i, m := range marcas {
		caja, clave, err := resolverObjetivo(m.Objetivo, cajas)
		if err != nil {
			return nil, fmt.Errorf("marca %d: %w", i+1, err)
		}
		cita := ""
		if m.Cita != nil {
			titulo, ok := citas[m.Cita.RecursoID]
			if !ok {
				return nil, fmt.Errorf("marca %d: cita sin recurso resuelto", i+1)
			}
			cita = textoCita(titulo, m.Cita.Pagina)
		}
		base := idMarca(sesion, clave)
		color := colorVeredicto[m.Veredicto]
		datos := func() *DatosMarca { return &DatosMarca{Bythos: "marca", Veredicto: m.Veredicto, Clave: clave} }
		marcoX, marcoY := caja.X-margenMarco, caja.Y-margenMarco
		marcoAncho, marcoAlto := caja.Ancho+2*margenMarco, caja.Alto+2*margenMarco
		texto := textoPildora(m, cita)
		ops = append(ops,
			Op{Accion: "agregar", Elemento: &Elemento{
				ID: base + "-m", Tipo: "rectangle", X: &marcoX, Y: &marcoY, Ancho: &marcoAncho, Alto: &marcoAlto,
				Color: &color, Fondo: ptr("transparent"), Grosor: ptr(2.0), Datos: datos(),
			}},
			Op{Accion: "agregar", Elemento: &Elemento{
				ID: base + "-t", Tipo: "text", X: ptr(marcoX + marcoAncho + 12), Y: &marcoY, Alto: ptr(20.0),
				Texto: &texto, Color: &color, Datos: datos(),
			}},
		)
	}
	return ops, nil
}

func ptr[T any](v T) *T { return &v }

// idMarca es determinista por sesión y objetivo: re-marcar reemplaza (D5).
func idMarca(sesion int64, clave string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%d|%s", sesion, clave)))
	return prefijoMarca + base64.RawURLEncoding.EncodeToString(h[:])[:10]
}

// resolverObjetivo devuelve la caja del objetivo y su clave estable (D5, D7).
func resolverObjetivo(o Objetivo, cajas map[string]Caja) (Caja, string, error) {
	if len(o.IDs) == 0 {
		redondeo := func(v float64) int { return int(math.Round(v/10)) * 10 }
		clave := fmt.Sprintf("area:%d,%d,%d,%d", redondeo(*o.X), redondeo(*o.Y), redondeo(*o.Ancho), redondeo(*o.Alto))
		return Caja{X: *o.X, Y: *o.Y, Ancho: *o.Ancho, Alto: *o.Alto}, clave, nil
	}
	ids := append([]string(nil), o.IDs...)
	sort.Strings(ids)
	var u Caja
	for i, id := range ids {
		c, ok := cajas[id]
		if !ok {
			return Caja{}, "", fmt.Errorf("el elemento %q no existe en la pizarra", id)
		}
		if i == 0 {
			u = c
			continue
		}
		x0, y0 := math.Min(u.X, c.X), math.Min(u.Y, c.Y)
		x1, y1 := math.Max(u.X+u.Ancho, c.X+c.Ancho), math.Max(u.Y+u.Alto, c.Y+c.Alto)
		u = Caja{X: x0, Y: y0, Ancho: x1 - x0, Alto: y1 - y0}
	}
	return u, "ids:" + strings.Join(ids, ","), nil
}

// textoCita arma "<título>, pág. N" con el título de la base de datos cortado
// a 60 runas (con "…"); sin página va solo el título (D8).
func textoCita(titulo string, pagina *int) string {
	titulo = limpiarTexto(titulo)
	if r := []rune(titulo); len(r) > maxTituloCita {
		titulo = string(r[:maxTituloCita-1]) + "…"
	}
	if pagina == nil {
		return titulo
	}
	return fmt.Sprintf("%s, pág. %d", titulo, *pagina)
}

// textoPildora arma las líneas de la píldora (D6): veredicto, pista (≤10),
// cita (≤2, nunca cortada) y pasaje entre comillas (≤6). Máximo 19 líneas.
func textoPildora(m Marca, cita string) string {
	lineas := []string{textoVeredicto[m.Veredicto]}
	if pista := limpiarTexto(m.Pista); pista != "" {
		lineas = append(lineas, recortarLineas(envolver(pista, anchoPildora), maxLineasPista, anchoPildora)...)
	}
	if cita != "" {
		lineas = append(lineas, envolver(cita, anchoPildora)...)
	}
	if m.Cita != nil {
		if pasaje := limpiarTexto(m.Cita.Pasaje); pasaje != "" {
			lineas = append(lineas, recortarLineas(envolver("“"+pasaje+"”", anchoPildora), maxLineasPasaje, anchoPildora)...)
		}
	}
	return strings.Join(lineas, "\n")
}

// envolver parte el texto en líneas de hasta ancho runas, por palabras; una
// palabra más larga que la línea se parte.
func envolver(texto string, ancho int) []string {
	var lineas []string
	actual := ""
	cierra := func() {
		if actual != "" {
			lineas = append(lineas, actual)
			actual = ""
		}
	}
	for _, palabra := range strings.Fields(texto) {
		for utf8.RuneCountInString(palabra) > ancho {
			cierra()
			r := []rune(palabra)
			lineas = append(lineas, string(r[:ancho]))
			palabra = string(r[ancho:])
		}
		switch {
		case actual == "":
			actual = palabra
		case utf8.RuneCountInString(actual)+1+utf8.RuneCountInString(palabra) <= ancho:
			actual += " " + palabra
		default:
			cierra()
			actual = palabra
		}
	}
	cierra()
	return lineas
}

// recortarLineas deja como mucho max líneas; si sobra texto, la última acaba en "…".
func recortarLineas(lineas []string, max, ancho int) []string {
	if len(lineas) <= max {
		return lineas
	}
	out := append([]string(nil), lineas[:max]...)
	r := []rune(out[max-1])
	if len(r) > ancho-1 {
		r = r[:ancho-1]
	}
	out[max-1] = strings.TrimRight(string(r), " ") + "…"
	return out
}

// Describir resume un lote guardado (JSON de []Op) para el historial: su tipo
// ("marcas" si agrega elementos de marca, si no "dibujo") y cuántos elementos
// toca. Un JSON ilegible cuenta como dibujo vacío.
func Describir(op string) (tipo string, elementos int) {
	var ops []Op
	if err := json.Unmarshal([]byte(op), &ops); err != nil {
		return "dibujo", 0
	}
	tipo = "dibujo"
	for _, o := range ops {
		switch o.Accion {
		case "agregar", "actualizar":
			elementos++
			if e := o.Elemento; e != nil && (e.Datos != nil || strings.HasPrefix(e.ID, prefijoMarca)) {
				tipo = "marcas"
			}
		case "borrar", "mover":
			elementos += len(o.IDs)
		}
	}
	return tipo, elementos
}
