package api

// metadata.go — El OLFATO. Dado un link, adivina título/imagen/descripción/tipo.
// ¿Por qué aquí y no en db? db = memoria tonta (guarda strings).
// api = cerebro (piensa: descarga, parsea, decide). Separación que ya conoces.
//
// ¿Por qué sin librerías externas (sin goquery, sin colly)?
// Cada dependencia engorda tu .exe y es código que no dominas.
// Con stdlib + 5 regex cubres YouTube + 90% de artículos. Suficiente para v1.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Metadata es lo que Guardar necesita para no pedirle nada al usuario.
// La extensión manda SOLO {url}; esto rellena el resto.
type Metadata struct {
	Titulo      string
	Imagen      string
	Descripcion string
	Tipo        string // "youtube" | "articulo" | "otro"
}

var (
	reTitle   = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reOGTitle = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:title["'][^>]*>`)
	reOGDesc  = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:description["'][^>]*>`)
	reOGImage = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:image["'][^>]*>`)
	reMetaDes = regexp.MustCompile(`(?is)<meta[^>]+name=["']description["'][^>]*>`)
	reContent = regexp.MustCompile(`(?is)content=["'](.*?)["']`)
)

// esYouTube decide por HOST, no por regex loco.
// youtube.com + youtu.be cubren web y compartir de móvil.
func esYouTube(link string) bool {
	u, err := url.Parse(link)
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Host)
	return strings.Contains(h, "youtube.com") || strings.Contains(h, "youtu.be")
}

// obtenerMetadata es la única que el resto llama. Orden:
// 1. YouTube (oEmbed, sin API key) → 2. HTML genérico → 3. fallback digno.
// Nunca falla: en el peor caso devuelve {Titulo: url, Tipo: "otro"}.
func obtenerMetadata(link string) Metadata {
	link = strings.TrimSpace(link)
	if esYouTube(link) {
		if m := porYouTube(link); m.Titulo != "" {
			return m
		}
	}
	if m := porHTML(link); m.Titulo != "" {
		m.Tipo = "articulo"
		return m
	}
	return Metadata{Titulo: link, Tipo: "otro"}
}

// porYouTube usa oEmbed público (sin key, sin cuota).
// Devuelve título + miniatura + "Video de YouTube por X".
func porYouTube(link string) Metadata {
	client := http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get("https://www.youtube.com/oembed?url=" + url.QueryEscape(link) + "&format=json")
	if err != nil {
		return Metadata{}
	}
	defer resp.Body.Close()

	var d struct {
		Title     string `json:"title"`
		Author    string `json:"author_name"`
		Miniatura string `json:"thumbnail_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return Metadata{}
	}
	desc := ""
	if d.Author != "" {
		desc = "Video de YouTube por " + d.Author
	}
	return Metadata{Titulo: d.Title, Imagen: d.Miniatura, Descripcion: desc, Tipo: "youtube"}
}

// porHTML descarga solo 500KB (basta el <head>) con timeout de 10s.
// Lee <title>, og:title, og:description, og:image, meta description.
// og:* gana a <title> porque el autor lo escribió para compartir.
func porHTML(link string) Metadata {
	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(link)
	if err != nil {
		return Metadata{}
	}
	defer resp.Body.Close()

	cuerpo, err := io.ReadAll(io.LimitReader(resp.Body, 500*1024))
	if err != nil {
		return Metadata{}
	}
	html := string(cuerpo)

	titulo := primerGrupo(reTitle, html)
	if og := contenidoMeta(reOGTitle, html); og != "" {
		titulo = og
	}
	desc := contenidoMeta(reOGDesc, html)
	if desc == "" {
		desc = contenidoMeta(reMetaDes, html)
	}
	return Metadata{Titulo: limpiar(titulo), Imagen: strings.TrimSpace(contenidoMeta(reOGImage, html)), Descripcion: limpiar(desc)}
}

func primerGrupo(re *regexp.Regexp, html string) string {
	m := re.FindStringSubmatch(html)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

func contenidoMeta(re *regexp.Regexp, html string) string {
	return primerGrupo(reContent, re.FindString(html))
}

// limpiar aplasta espacios/saltos y corta a 300 (descripciones eternas
// rompen tus tarjetas). El "…" avisa que hubo corte.
func limpiar(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}
