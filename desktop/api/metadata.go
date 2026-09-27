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
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"syscall"
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

// esquemaSeguro exige http/https ANTES de intentar cualquier red.
// Corta en seco file://, ftp:// y cualquier otro esquema raro: ni siquiera
// vale la pena resolverlo, y esYouTube/porHTML tampoco saben qué hacer con eso.
func esquemaSeguro(link string) bool {
	u, err := url.Parse(link)
	if err != nil {
		return false
	}
	esquema := strings.ToLower(u.Scheme)
	return esquema == "http" || esquema == "https"
}

// --- Guard SSRF: nunca conectar a la red interna de la PC del usuario ---
//
// El link de un recurso es 100% controlado por quien lo guarda (usuario o
// extensión). Sin este guard, pedir metadata de "http://169.254.169.254/"
// o "http://localhost:<puerto interno>" convierte a Bythos en un proxy
// hacia la red local de quien lo corre (SSRF clásico).
//
// direccionesPermitidasTest es el ÚNICO hook de test: en producción está
// vacío y CUALQUIER IP loopback/privada/link-local se bloquea sin
// excepción. Los tests que necesitan pegarle a su propio httptest.Server
// (que escucha en 127.0.0.1) agregan ahí su "ip:puerto" EXACTO -nunca un
// permitirRedLocal=true genérico- así una prueba de redirección puede
// demostrar que el primer salto (el httptest.Server legítimo) pasa pero el
// destino de la redirección (otro 127.0.0.1 distinto, el "atacante") sigue
// bloqueado por no estar en este mapa.
var direccionesPermitidasTest = map[string]bool{}

// ipBloqueada decide si una IP YA RESUELTA no debe recibir conexiones
// salientes: loopback, privada RFC1918 + ULA IPv6 fc00::/7 (IsPrivate),
// link-local incluido el metadata endpoint de nube 169.254.169.254 y
// fe80::/10 (IsLinkLocalUnicast), sin especificar, multicast y CGNAT
// 100.64.0.0/10 (sin método propio en net.IP, se chequea a mano).
// Los métodos de net.IP ya resuelven variantes IPv4-mapeadas en IPv6
// (::ffff:127.0.0.1) internamente via To4(), no hace falta duplicarlo.
func ipBloqueada(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
		return true // CGNAT 100.64.0.0/10
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

// dialerSeguro usa net.Dialer.Control, que Go llama con la IP YA RESUELTA
// justo antes de conectar (una vez por cada IP que intenta). Chequear ahí
// y no antes (ej. resolviendo el host a mano con net.LookupIP) es lo que
// cierra el TOCTOU de DNS rebinding: un dominio que resuelve distinto entre
// "cuando lo miramos" y "cuando conectamos" no sirve de nada si el chequeo
// corre siempre contra la IP real del connect.
func dialerSeguro() *net.Dialer {
	return &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			if direccionesPermitidasTest[address] {
				return nil
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("dirección no soportada: %s", address)
			}
			if ipBloqueada(ip) {
				return fmt.Errorf("bloqueado por seguridad: %s no es una dirección pública", ip)
			}
			return nil
		},
	}
}

// limitarRedirects tope 5 saltos y exige http/https en cada uno. El guard
// de IP de arriba ya corre en el Dial de cada salto nuevo (incluidos los de
// una redirección); esto solo cierra el hueco de esquemas raros que un 3xx
// podría intentar meter.
func limitarRedirects(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return fmt.Errorf("demasiadas redirecciones (máximo 5)")
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return fmt.Errorf("esquema no permitido en redirección: %s", req.URL.Scheme)
	}
	return nil
}

// clientSeguro es el único http.Client que usan porYouTube y porHTML para
// resolver URLs del usuario. Transport propio (no DefaultTransport) para
// que dialerSeguro corra en TODA conexión saliente, con Proxy: nil para que
// un HTTP_PROXY/HTTPS_PROXY de entorno no pueda saltarse el guard dialando
// directo a donde el proxy diga en vez de a la IP que el guard revisó.
func clientSeguro(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:               nil,
			DialContext:         dialerSeguro().DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
		},
		CheckRedirect: limitarRedirects,
	}
}

// obtenerMetadata es la única que el resto llama. Orden:
// 1. YouTube (oEmbed, sin API key) → 2. HTML genérico → 3. fallback digno.
// Nunca falla: en el peor caso devuelve {Titulo: url, Tipo: "otro"}
// (esquema no http/https incluido: ni se intenta la red, ver esquemaSeguro).
func obtenerMetadata(link string) Metadata {
	link = strings.TrimSpace(link)
	if !esquemaSeguro(link) {
		return Metadata{Titulo: link, Tipo: "otro"}
	}
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
	client := clientSeguro(8 * time.Second)
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
	client := clientSeguro(10 * time.Second)
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
