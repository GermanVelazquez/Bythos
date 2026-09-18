package api

// import_md.go — Import commitments from Markdown into agenda.
// Pure parser (parseCompromisosMD) is unit-tested without DB; the handler
// adds idempotency counting and a dry-run preview flag.
//
// Grammar (keys case-insensitive, spaces tolerated):
//   ## Compromiso: <title>
//   - Tipo: puntual | semanal
//   - Fecha: YYYY-MM-DD (puntual only)
//   - Desde: YYYY-MM-DD (semanal only)
//   - Hasta: YYYY-MM-DD (semanal only)
//   - Horario: lunes 19-20, miercoles 19-20 | 10-11 (puntual HH or HH:MM range)
//   - Aula: optional, appended as [Aula X]
//   - Docente: optional, appended as (Name)
//   - Carpeta: optional, links rows to an existing folder by name
//   - Modalidad: legacy alias, anual -> semanal March-December of Desde year;
//     1er/2do cuatrimestre require explicit Desde/Hasta or it is a warning.
// Weekly entries expand to one row per matching weekday (no schema change).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"bythos-desktop/db"
)

// ocurrenciaMD is one agenda row produced by the parser.
// CarpetaNombre keeps the raw "- Carpeta:" value ("" = no line).
// CarpetaID + Estado are filled by vincularCarpetas against live folders:
// vinculada = matched, suelta = named but missing, sin-carpeta = no line.
// Bloque carries "Bloque N (Title)" so the UI can group by source block.
type ocurrenciaMD struct {
	Fecha         string `json:"fecha"`
	HoraInicio    string `json:"hora_inicio"`
	HoraFin       string `json:"hora_fin"`
	Texto         string `json:"texto"`
	CarpetaNombre string `json:"carpeta_nombre"`
	CarpetaID     *int64 `json:"carpeta_id"`
	Estado        string `json:"estado"`
	Bloque        string `json:"bloque"`
}

// bloqueMD is one ## Compromiso block in MD order for the per-block menu.
// CarpetaPedida is the raw "- Carpeta:" value ("" = no line).
// CarpetaID + Estado mirror ocurrenciaMD semantics against live folders.
type bloqueMD struct {
	Indice        int    `json:"indice"`
	Titulo        string `json:"titulo"`
	CarpetaPedida string `json:"carpeta_pedida"`
	CarpetaID     *int64 `json:"carpeta_id"`
	Estado        string `json:"estado"`
}

// decisionCarpeta is one per-block folder choice from the UI.
// Bloque is 1-based in MD order; Accion is auto|usar|crear|suelta ("" = auto).
type decisionCarpeta struct {
	Bloque    int    `json:"bloque"`
	Accion    string `json:"accion"`
	CarpetaID *int64 `json:"carpeta_id"`
	Nombre    string `json:"nombre"`
}

// carpetaCreada reports a folder created during confirm with the blocks linked to it.
type carpetaCreada struct {
	ID      int64  `json:"id"`
	Nombre  string `json:"nombre"`
	Bloques []int  `json:"bloques"`
}

type horarioEntry struct {
	diaSemana int // Sunday=0 like time.Weekday
	inicio    string
	fin       string
}

var compromisoRe = regexp.MustCompile(`(?im)^##\s*compromiso\s*:\s*(.*)\s*$`)

// maxImportDays caps absurd ranges (annual typo) before expansion.
const maxImportDays = 366

// parseCompromisosMD scans markdown and returns occurrences plus warnings.
// It never aborts the whole file: every block contributes its valid rows
// and accumulates Spanish warnings shaped as "Bloque N (Title): message".
func parseCompromisosMD(markdown string) ([]ocurrenciaMD, []string) {
	ocurrencias := []ocurrenciaMD{}
	avisos := []string{}

	matches := compromisoRe.FindAllStringSubmatchIndex(markdown, -1)
	if len(matches) == 0 {
		if strings.TrimSpace(markdown) != "" {
			avisos = append(avisos, "No se encontraron bloques ## Compromiso: <título>")
		}
		return ocurrencias, avisos
	}

	for n, m := range matches {
		num := n + 1
		titulo := ""
		if len(m) >= 4 && m[2] >= 0 {
			titulo = strings.TrimSpace(markdown[m[2]:m[3]])
		}
		finBloque := len(markdown)
		if n+1 < len(matches) {
			finBloque = matches[n+1][0]
		}
		cuerpo := markdown[m[1]:finBloque]
		etiqueta := etiquetaBloque(num, titulo)
		avisar := func(msg string) {
			avisos = append(avisos, etiqueta+": "+msg)
		}

		campos := leerCampos(cuerpo)
		tipo := strings.ToLower(strings.TrimSpace(campos["tipo"]))
		fechaRaw := strings.TrimSpace(campos["fecha"])
		desdeRaw := strings.TrimSpace(campos["desde"])
		hastaRaw := strings.TrimSpace(campos["hasta"])
		horarioRaw := strings.TrimSpace(campos["horario"])
		aulaRaw := strings.TrimSpace(campos["aula"])
		docenteRaw := strings.TrimSpace(campos["docente"])
		carpetaRaw := strings.TrimSpace(campos["carpeta"])
		modalidadRaw := strings.TrimSpace(campos["modalidad"])
		// Provisional folder state: parse keeps the raw name, vincularCarpetas
		// resolves the ID later. Empty = sin-carpeta, named = suelta until matched.
		estadoCarpeta := "sin-carpeta"
		if carpetaRaw != "" {
			estadoCarpeta = "suelta"
		}
		modalidadNorm := strings.ToLower(modalidadRaw)
		esAnual := strings.Contains(modalidadNorm, "anual")
		esCuatri := strings.Contains(modalidadNorm, "cuatri")

		// Legacy alias implies weekly even when Tipo was omitted.
		if tipo == "" && (esAnual || esCuatri) {
			tipo = "semanal"
		}
		if tipo == "" {
			avisar("falta Tipo (usa puntual o semanal)")
			continue
		}
		if tipo != "puntual" && tipo != "semanal" {
			avisar(fmt.Sprintf("Tipo inválido %q (usa puntual o semanal)", campos["tipo"]))
			continue
		}

		texto := construirTexto(titulo, aulaRaw, docenteRaw)
		if strings.TrimSpace(texto) == "" {
			avisar("falta título en ## Compromiso:")
			continue
		}
		if horarioRaw == "" {
			avisar("falta Horario")
			continue
		}

		if tipo == "puntual" {
			if fechaRaw == "" {
				avisar("falta Fecha (usa YYYY-MM-DD)")
				continue
			}
			if !fechaValidaMD(fechaRaw) {
				avisar(fmt.Sprintf("formato fecha inválido %q (usa YYYY-MM-DD)", fechaRaw))
				continue
			}
			rangos, avisosHor := parseHorarioPuntual(horarioRaw)
			for _, a := range avisosHor {
				avisar(a)
			}
			if len(rangos) == 0 {
				continue
			}
			for _, rg := range rangos {
				ocurrencias = append(ocurrencias, ocurrenciaMD{
					Fecha: fechaRaw, HoraInicio: rg[0], HoraFin: rg[1], Texto: texto,
					CarpetaNombre: carpetaRaw, Estado: estadoCarpeta, Bloque: etiqueta,
				})
			}
			continue
		}

		// Tipo semanal (anual and cuatrimestral fold here).
		desde, hasta := desdeRaw, hastaRaw
		if esAnual {
			if desdeRaw == "" {
				avisar("falta Desde para modalidad anual (usa YYYY-MM-DD)")
				continue
			}
			if !fechaValidaMD(desdeRaw) {
				avisar(fmt.Sprintf("formato Desde inválido %q (usa YYYY-MM-DD)", desdeRaw))
				continue
			}
			anio := desdeRaw[:4]
			desde = anio + "-03-01"
			hasta = anio + "-12-31"
		} else if esCuatri {
			if desdeRaw == "" || hastaRaw == "" {
				nombre := modalidadRaw
				if nombre == "" {
					nombre = "cuatrimestral"
				}
				avisar(fmt.Sprintf("falta Desde/Hasta para %s", nombre))
				continue
			}
		} else {
			if desdeRaw == "" && hastaRaw == "" {
				avisar("falta Desde/Hasta (usa YYYY-MM-DD)")
				continue
			}
			if desdeRaw == "" {
				avisar("falta Desde (usa YYYY-MM-DD)")
				continue
			}
			if hastaRaw == "" {
				avisar("falta Hasta (usa YYYY-MM-DD)")
				continue
			}
		}
		if !fechaValidaMD(desde) {
			avisar(fmt.Sprintf("formato Desde inválido %q (usa YYYY-MM-DD)", desde))
			continue
		}
		if !fechaValidaMD(hasta) {
			avisar(fmt.Sprintf("formato Hasta inválido %q (usa YYYY-MM-DD)", hasta))
			continue
		}
		if hasta < desde {
			avisar(fmt.Sprintf("Desde %s es posterior a Hasta %s", desde, hasta))
			continue
		}
		// Cap absurd ranges before fan-out.
		d0, _ := time.Parse("2006-01-02", desde)
		d1, _ := time.Parse("2006-01-02", hasta)
		if d1.Sub(d0).Hours() > float64(maxImportDays*24) {
			hasta = d0.AddDate(0, 0, maxImportDays).Format("2006-01-02")
			d1, _ = time.Parse("2006-01-02", hasta)
			avisar(fmt.Sprintf("rango muy amplio, limitado a %d días (Desde %s Hasta %s)", maxImportDays, desde, hasta))
		}

		entradas, avisosHor := parseHorarioSemanal(horarioRaw)
		for _, a := range avisosHor {
			avisar(a)
		}
		if len(entradas) == 0 {
			continue
		}
		porDia := map[int][]horarioEntry{}
		for _, e := range entradas {
			porDia[e.diaSemana] = append(porDia[e.diaSemana], e)
		}
		for d := d0; !d.After(d1); d = d.AddDate(0, 0, 1) {
			for _, e := range porDia[int(d.Weekday())] {
				ocurrencias = append(ocurrencias, ocurrenciaMD{
					Fecha: d.Format("2006-01-02"), HoraInicio: e.inicio, HoraFin: e.fin, Texto: texto,
					CarpetaNombre: carpetaRaw, Estado: estadoCarpeta, Bloque: etiqueta,
				})
			}
		}
	}

	sort.Slice(ocurrencias, func(i, j int) bool {
		if ocurrencias[i].Fecha != ocurrencias[j].Fecha {
			return ocurrencias[i].Fecha < ocurrencias[j].Fecha
		}
		if ocurrencias[i].HoraInicio != ocurrencias[j].HoraInicio {
			return ocurrencias[i].HoraInicio < ocurrencias[j].HoraInicio
		}
		if ocurrencias[i].HoraFin != ocurrencias[j].HoraFin {
			return ocurrencias[i].HoraFin < ocurrencias[j].HoraFin
		}
		if ocurrencias[i].Texto != ocurrencias[j].Texto {
			return ocurrencias[i].Texto < ocurrencias[j].Texto
		}
		return ocurrencias[i].CarpetaNombre < ocurrencias[j].CarpetaNombre
	})
	return ocurrencias, avisos
}

// leerCampos extracts "- Clave: valor" lines, keys lowercased.
// Leading bullets (-, *, middle dot) and extra spaces are tolerated.
func leerCampos(cuerpo string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(cuerpo, "\n") {
		t := strings.TrimSpace(line)
		t = strings.TrimLeft(t, "-*•")
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		// Skip the heading itself if it leaked into the slice.
		if compromisoRe.MatchString(t) {
			continue
		}
		idx := strings.Index(t, ":")
		if idx < 0 {
			continue
		}
		clave := strings.ToLower(strings.TrimSpace(t[:idx]))
		valor := strings.TrimSpace(t[idx+1:])
		switch clave {
		case "tipo", "fecha", "desde", "hasta", "horario", "aula", "docente", "carpeta", "modalidad":
			if _, existe := out[clave]; !existe {
				out[clave] = valor
			}
		}
	}
	return out
}

// construirTexto appends optional Aula and Docente to the title.
func construirTexto(titulo, aula, docente string) string {
	texto := strings.TrimSpace(titulo)
	if aula != "" {
		if strings.HasPrefix(strings.ToLower(aula), "aula") {
			texto = strings.TrimSpace(texto + " [" + aula + "]")
		} else {
			texto = strings.TrimSpace(texto + " [Aula " + aula + "]")
		}
	}
	if docente != "" {
		if strings.HasPrefix(docente, "(") && strings.HasSuffix(docente, ")") {
			texto = strings.TrimSpace(texto + " " + docente)
		} else {
			texto = strings.TrimSpace(texto + " (" + docente + ")")
		}
	}
	return texto
}

// fechaValidaMD mirrors db validation: real calendar day, not just shape.
func fechaValidaMD(f string) bool {
	t, err := time.Parse("2006-01-02", f)
	if err != nil {
		return false
	}
	return t.Format("2006-01-02") == f
}

// normalizarCarpeta trims, lowercases and strips accents for folder matching.
// Exact normalized equality decides the link; accents and case never block it.
func normalizarCarpeta(s string) string {
	t := strings.ToLower(strings.TrimSpace(s))
	r := strings.NewReplacer(
		"á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "å", "a",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o", "ø", "o",
		"ú", "u", "ù", "u", "û", "u", "ü", "u",
		"ñ", "n", "ç", "c", "ý", "y", "ÿ", "y",
	)
	return r.Replace(t)
}

// etiquetaBloque shapes "Bloque N (Title)" so parser, block menu and
// warnings never drift apart.
func etiquetaBloque(num int, titulo string) string {
	if titulo != "" {
		return fmt.Sprintf("Bloque %d (%s)", num, titulo)
	}
	return fmt.Sprintf("Bloque %d", num)
}

// extraerBloques lists every ## Compromiso block in MD order (1-based).
// Blocks with zero occurrences still show up: the UI menu needs them
// even when the parser rejected their rows.
func extraerBloques(markdown string) []bloqueMD {
	out := []bloqueMD{}
	matches := compromisoRe.FindAllStringSubmatchIndex(markdown, -1)
	for n, m := range matches {
		titulo := ""
		if len(m) >= 4 && m[2] >= 0 {
			titulo = strings.TrimSpace(markdown[m[2]:m[3]])
		}
		finBloque := len(markdown)
		if n+1 < len(matches) {
			finBloque = matches[n+1][0]
		}
		campos := leerCampos(markdown[m[1]:finBloque])
		out = append(out, bloqueMD{
			Indice: n + 1, Titulo: titulo,
			CarpetaPedida: strings.TrimSpace(campos["carpeta"]),
		})
	}
	return out
}

// mapaCarpetas indexes folders by normalized name (first wins) and warns
// about normalized duplicates, so imports link deterministically.
func mapaCarpetas(carpetas []db.Carpeta) (map[string]db.Carpeta, []string) {
	avisos := []string{}
	porNorma := map[string]db.Carpeta{}
	for _, c := range carpetas {
		norma := normalizarCarpeta(c.Nombre)
		if norma == "" {
			continue
		}
		if previa, existe := porNorma[norma]; existe {
			if previa.Nombre != c.Nombre {
				avisos = append(avisos, fmt.Sprintf("Las carpetas %q y %q se normalizan igual, se usa %q", previa.Nombre, c.Nombre, previa.Nombre))
			}
			continue
		}
		porNorma[norma] = c
	}
	return porNorma, avisos
}

// resolverBloques fills CarpetaID/Estado of every block against folders.
// Pure (no DB writes): the confirm path reuses it after creating folders.
func resolverBloques(bloques []bloqueMD, carpetas []db.Carpeta) []bloqueMD {
	out := make([]bloqueMD, len(bloques))
	copy(out, bloques)
	porNorma, _ := mapaCarpetas(carpetas)
	for i := range out {
		if out[i].CarpetaPedida == "" {
			out[i].Estado = "sin-carpeta"
			continue
		}
		if c, ok := porNorma[normalizarCarpeta(out[i].CarpetaPedida)]; ok {
			id := c.ID
			out[i].CarpetaID = &id
			out[i].Estado = "vinculada"
			continue
		}
		out[i].CarpetaID = nil
		out[i].Estado = "suelta"
	}
	return out
}

// vincularCarpetas resolves CarpetaNombre against existing folders.
// It never creates folders (explicit owner decision).
// Match = exact normalized equality; first folder wins on collisions.
// Missing = NULL + one Spanish warning per block:
// `Bloque N (Title): carpeta "X" no existe, queda suelto`.
// Empty = NULL + sin-carpeta, no warning.
func vincularCarpetas(ocurrencias []ocurrenciaMD, carpetas []db.Carpeta) ([]ocurrenciaMD, []string) {
	if len(ocurrencias) == 0 {
		return ocurrencias, []string{}
	}
	porNorma, avisos := mapaCarpetas(carpetas)

	vistas := map[string]bool{} // bloque etiqueta -> warning already emitted
	ordenBloques := []string{}
	faltantes := map[string]string{} // bloque etiqueta -> raw folder name
	for i := range ocurrencias {
		nombre := strings.TrimSpace(ocurrencias[i].CarpetaNombre)
		ocurrencias[i].CarpetaNombre = nombre
		if nombre == "" {
			ocurrencias[i].CarpetaID = nil
			ocurrencias[i].Estado = "sin-carpeta"
			continue
		}
		if c, ok := porNorma[normalizarCarpeta(nombre)]; ok {
			id := c.ID
			ocurrencias[i].CarpetaID = &id
			ocurrencias[i].Estado = "vinculada"
			continue
		}
		ocurrencias[i].CarpetaID = nil
		ocurrencias[i].Estado = "suelta"
		bloque := ocurrencias[i].Bloque
		if bloque == "" {
			bloque = "Bloque"
		}
		if !vistas[bloque] {
			vistas[bloque] = true
			ordenBloques = append(ordenBloques, bloque)
			faltantes[bloque] = nombre
		}
	}
	// Warnings in block order (Bloque N), not in date-sorted order.
	sort.Slice(ordenBloques, func(i, j int) bool {
		return numBloque(ordenBloques[i]) < numBloque(ordenBloques[j])
	})
	for _, b := range ordenBloques {
		avisos = append(avisos, fmt.Sprintf("%s: carpeta %q no existe, queda suelto", b, faltantes[b]))
	}
	return ocurrencias, avisos
}

// numBloque extracts N from "Bloque N (Title)" for warning ordering.
// Unknown shapes sort last without breaking the output.
func numBloque(bloque string) int {
	var n int
	if _, err := fmt.Sscanf(bloque, "Bloque %d", &n); err == nil {
		return n
	}
	return 1 << 30
}

// carpetasActuales lists folders for resolution; on DB error it returns
// empty (rows stay loose) so a folder read failure never blocks an import.
func (s *Servidor) carpetasActuales() []db.Carpeta {
	carpetas, err := db.ListarCarpetas(s.Base)
	if err != nil {
		return []db.Carpeta{}
	}
	return carpetas
}

// resolverImport parses markdown and links folders in one step.
// It returns occurrences with CarpetaID/Estado filled plus parser warnings
// followed by folder warnings (duplicates first, then missing per block).
func (s *Servidor) resolverImport(markdown string) ([]ocurrenciaMD, []string) {
	ocurrencias, avisos := parseCompromisosMD(markdown)
	if avisos == nil {
		avisos = []string{}
	}
	if ocurrencias == nil {
		ocurrencias = []ocurrenciaMD{}
	}
	ocurrencias, avisosCarpetas := vincularCarpetas(ocurrencias, s.carpetasActuales())
	avisos = append(avisos, avisosCarpetas...)
	return ocurrencias, avisos
}

// aplicarDecisiones validates per-block folder choices and creates folders
// in block order. It returns the occurrences with their final carpeta_id,
// the folders it created and fresh folder warnings.
// Validation runs BEFORE any write: on error it returns a Spanish message
// and touches nothing (no folders, no lotes, no rows).
// Semantics per block (last decision wins when repeated):
//   auto (or no decision) = current behavior, resolved against live folders
//     including the ones just created below.
//   usar = link every row of the block to carpeta_id (must exist now).
//   crear = create nombre once per normalized name (first raw spelling wins)
//     and link every requesting block to it; when the normalized name
//     already exists (race between preview and confirm) it links instead.
//   suelta = force NULL for every row of the block.
// Blocks with explicit usar/crear/suelta decisions never emit "no existe"
// warnings: the user already chose what to do with them.
func (s *Servidor) aplicarDecisiones(markdown string, ocurrencias []ocurrenciaMD, decisiones []decisionCarpeta) ([]ocurrenciaMD, []carpetaCreada, []string, error) {
	bloques := extraerBloques(markdown)
	porBloque := map[int]decisionCarpeta{}
	for _, d := range decisiones {
		if d.Bloque < 1 {
			return nil, nil, nil, fmt.Errorf("Bloque %d inválido en decisiones (usa 1 en adelante)", d.Bloque)
		}
		if d.Bloque > len(bloques) {
			continue // unknown block: no rows to affect, ignore
		}
		accion := strings.ToLower(strings.TrimSpace(d.Accion))
		if accion == "" {
			accion = "auto"
		}
		if accion != "auto" && accion != "usar" && accion != "crear" && accion != "suelta" {
			return nil, nil, nil, fmt.Errorf("Bloque %d: acción inválida %q (usa auto, usar, crear o suelta)", d.Bloque, d.Accion)
		}
		d.Accion = accion
		porBloque[d.Bloque] = d
	}

	frescas := s.carpetasActuales()
	porID := map[int64]db.Carpeta{}
	for _, c := range frescas {
		porID[c.ID] = c
	}

	// Validate every explicit choice before creating anything.
	for num := 1; num <= len(bloques); num++ {
		d, hay := porBloque[num]
		if !hay || d.Accion == "auto" || d.Accion == "suelta" {
			continue
		}
		if d.Accion == "usar" {
			if d.CarpetaID == nil || *d.CarpetaID <= 0 {
				return nil, nil, nil, fmt.Errorf("Bloque %d: elegí una carpeta existente o dejalo suelto", num)
			}
			if _, ok := porID[*d.CarpetaID]; !ok {
				return nil, nil, nil, fmt.Errorf("Bloque %d: la carpeta elegida no existe", num)
			}
			continue
		}
		if strings.TrimSpace(d.Nombre) == "" {
			return nil, nil, nil, fmt.Errorf("Bloque %d: el nombre de la carpeta es obligatorio para crearla", num)
		}
	}

	// Create requested folders in block order, deduped by normalized name.
	idPorNorma := map[string]int64{}
	for _, c := range frescas {
		norma := normalizarCarpeta(c.Nombre)
		if norma == "" {
			continue
		}
		if _, vista := idPorNorma[norma]; !vista {
			idPorNorma[norma] = c.ID
		}
	}
	creadas := []carpetaCreada{}
	grupoPorNorma := map[string]int{} // normalized name -> index in creadas
	for num := 1; num <= len(bloques); num++ {
		d, hay := porBloque[num]
		if !hay || d.Accion != "crear" {
			continue
		}
		norma := normalizarCarpeta(d.Nombre)
		if _, ok := idPorNorma[norma]; ok {
			// Race (created between preview and confirm) or shared with a
			// folder created above: link below, nothing to create. When the
			// group was created above, record the extra block.
			if idx, dup := grupoPorNorma[norma]; dup {
				creadas[idx].Bloques = append(creadas[idx].Bloques, num)
			}
			continue
		}
		if idx, dup := grupoPorNorma[norma]; dup {
			creadas[idx].Bloques = append(creadas[idx].Bloques, num)
			continue
		}
		nombre := strings.TrimSpace(d.Nombre)
		c, err := db.CrearCarpeta(s.Base, nombre)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("Bloque %d: no se pudo crear la carpeta %q", num, nombre)
		}
		frescas = append(frescas, c)
		porID[c.ID] = c
		idPorNorma[norma] = c.ID
		grupoPorNorma[norma] = len(creadas)
		creadas = append(creadas, carpetaCreada{ID: c.ID, Nombre: c.Nombre, Bloques: []int{num}})
	}

	// Final folder per block: explicit choices win, auto resolves against
	// the fresh list (so auto blocks link to folders just created above).
	finalPorBloque := map[int]*int64{}
	for num := 1; num <= len(bloques); num++ {
		d, hay := porBloque[num]
		if hay && d.Accion == "usar" {
			id := *d.CarpetaID
			finalPorBloque[num] = &id
			continue
		}
		if hay && d.Accion == "crear" {
			id := idPorNorma[normalizarCarpeta(d.Nombre)]
			finalPorBloque[num] = &id
			continue
		}
		if hay && d.Accion == "suelta" {
			finalPorBloque[num] = nil
			continue
		}
		pedida := bloques[num-1].CarpetaPedida
		if pedida == "" {
			finalPorBloque[num] = nil
			continue
		}
		if id, ok := idPorNorma[normalizarCarpeta(pedida)]; ok {
			id := id
			finalPorBloque[num] = &id
			continue
		}
		finalPorBloque[num] = nil
	}

	for i := range ocurrencias {
		num := numBloque(ocurrencias[i].Bloque)
		id, hay := finalPorBloque[num]
		if !hay {
			continue // unknown label shape: keep parser state
		}
		ocurrencias[i].CarpetaID = id
		switch {
		case id != nil:
			ocurrencias[i].Estado = "vinculada"
		case strings.TrimSpace(ocurrencias[i].CarpetaNombre) != "":
			ocurrencias[i].Estado = "suelta"
		default:
			ocurrencias[i].Estado = "sin-carpeta"
		}
	}

	// Folder warnings: duplicates first (same rule as vincularCarpetas),
	// then one missing warning per auto block still loose, in block order.
	// Only blocks with rows warn, like vincularCarpetas does.
	_, avisosDup := mapaCarpetas(frescas)
	avisos := avisosDup
	if avisos == nil {
		avisos = []string{}
	}
	conFilas := map[int]bool{}
	for _, o := range ocurrencias {
		conFilas[numBloque(o.Bloque)] = true
	}
	for num := 1; num <= len(bloques); num++ {
		d, hay := porBloque[num]
		if hay && d.Accion != "auto" {
			continue
		}
		if !conFilas[num] {
			continue
		}
		if finalPorBloque[num] != nil || bloques[num-1].CarpetaPedida == "" {
			continue
		}
		avisos = append(avisos, fmt.Sprintf("%s: carpeta %q no existe, queda suelto",
			etiquetaBloque(num, bloques[num-1].Titulo), bloques[num-1].CarpetaPedida))
	}
	return ocurrencias, creadas, avisos, nil
}

// parseHoraMD accepts H or HH:MM and normalizes to HH:MM.
func parseHoraMD(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if t == "" {
		return "", false
	}
	var h, m int
	if strings.Contains(t, ":") {
		partes := strings.SplitN(t, ":", 2)
		if len(partes) != 2 {
			return "", false
		}
		var errH, errM error
		h, errH = atoiEstricto(strings.TrimSpace(partes[0]))
		m, errM = atoiEstricto(strings.TrimSpace(partes[1]))
		if errH != nil || errM != nil {
			return "", false
		}
	} else {
		var err error
		h, err = atoiEstricto(t)
		if err != nil {
			return "", false
		}
		m = 0
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return "", false
	}
	return fmt.Sprintf("%02d:%02d", h, m), true
}

func atoiEstricto(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int(r-'0')
	}
	if len(s) > 2 {
		return 0, fmt.Errorf("too long")
	}
	return n, nil
}

// parseRangoMD splits "HH-HH" with hyphen or dash variants.
func parseRangoMD(s string) (string, string, bool) {
	t := strings.ReplaceAll(s, "–", "-")
	t = strings.ReplaceAll(t, "—", "-")
	partes := strings.SplitN(t, "-", 2)
	if len(partes) != 2 {
		return "", "", false
	}
	ini, ok1 := parseHoraMD(partes[0])
	fin, ok2 := parseHoraMD(partes[1])
	if !ok1 || !ok2 {
		return "", "", false
	}
	if fin <= ini {
		return "", "", false
	}
	return ini, fin, true
}

// diaAWeekday maps Spanish day names (accent tolerant) to time.Weekday ints.
func diaAWeekday(s string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "lunes":
		return 1, true
	case "martes":
		return 2, true
	case "miercoles", "miércoles":
		return 3, true
	case "jueves":
		return 4, true
	case "viernes":
		return 5, true
	case "sabado", "sábado":
		return 6, true
	case "domingo":
		return 0, true
	}
	return 0, false
}

func contieneLetra(s string) bool {
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || r == 'á' || r == 'é' || r == 'í' || r == 'ó' || r == 'ú' || r == 'ñ' || r == 'ü' {
			return true
		}
	}
	return false
}

// parseHorarioSemanal parses "lunes 19-20, miércoles 19:00-20:30".
func parseHorarioSemanal(raw string) ([]horarioEntry, []string) {
	entradas := []horarioEntry{}
	avisos := []string{}
	for _, parte := range strings.Split(raw, ",") {
		p := strings.TrimSpace(parte)
		if p == "" {
			continue
		}
		idx := strings.IndexFunc(p, func(r rune) bool { return r == ' ' || r == '\t' })
		if idx < 0 {
			avisos = append(avisos, fmt.Sprintf("día desconocido %q (usa lunes, martes, miércoles, jueves, viernes, sábado, domingo)", p))
			continue
		}
		diaRaw := strings.TrimSpace(p[:idx])
		rangoRaw := strings.TrimSpace(p[idx+1:])
		dia, ok := diaAWeekday(diaRaw)
		if !ok {
			avisos = append(avisos, fmt.Sprintf("día desconocido %q (usa lunes, martes, miércoles, jueves, viernes, sábado, domingo)", diaRaw))
			continue
		}
		ini, fin, ok := parseRangoMD(rangoRaw)
		if !ok {
			avisos = append(avisos, fmt.Sprintf("horario inválido %q (usa HH-HH o HH:MM-HH:MM)", p))
			continue
		}
		entradas = append(entradas, horarioEntry{diaSemana: dia, inicio: ini, fin: fin})
	}
	return entradas, avisos
}

// parseHorarioPuntual parses "10-11" (one or comma-separated ranges, no day).
func parseHorarioPuntual(raw string) ([][2]string, []string) {
	rangos := [][2]string{}
	avisos := []string{}
	for _, parte := range strings.Split(raw, ",") {
		p := strings.TrimSpace(parte)
		if p == "" {
			continue
		}
		if contieneLetra(p) {
			avisos = append(avisos, fmt.Sprintf("horario inválido %q (usa HH-HH o HH:MM-HH:MM)", p))
			continue
		}
		ini, fin, ok := parseRangoMD(p)
		if !ok {
			avisos = append(avisos, fmt.Sprintf("horario inválido %q (usa HH-HH o HH:MM-HH:MM)", p))
			continue
		}
		rangos = append(rangos, [2]string{ini, fin})
	}
	return rangos, avisos
}

// importarAgenda handles POST /api/agenda/import {markdown, nombre?, decisiones?}.
// ?dry=true (or ?preview=true) returns a preview without persisting:
// {total, ocurrencias (max 20), avisos, bloques (MD order), carpetas_existentes}.
// Each occurrence carries {carpeta_nombre, carpeta_id, estado, bloque}
// resolved against live folders; each bloque carries
// {indice, titulo, carpeta_pedida, carpeta_id, estado} for the per-block menu.
// Otherwise it validates decisiones (400 in Spanish, touching nothing),
// creates the requested folders in block order (deduped, race-safe),
// stores the markdown as a lote (nombre defaults to the first
// "## Compromiso: X" title), expands it, links every row via lote_id plus
// its final carpeta_id, and replies {lote_id, nombre, creadas, omitidas,
// avisos, total, carpetas_creadas}. Skips exact duplicates
// (fecha+horas+texto) like always.
func (s *Servidor) importarAgenda(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || len(bytes.TrimSpace(raw)) == 0 {
		responderError(w, 400, "El markdown está vacío. Manda {\"markdown\":\"...\"}")
		return
	}
	var body struct {
		Markdown   string            `json:"markdown"`
		Nombre     string            `json:"nombre"`
		Decisiones []decisionCarpeta `json:"decisiones"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(raw), &body); err != nil || strings.TrimSpace(body.Markdown) == "" {
		responderError(w, 400, "El markdown está vacío. Manda {\"markdown\":\"...\"}")
		return
	}

	if esPreview(r) {
		ocurrencias, avisos := s.resolverImport(body.Markdown)
		carpetas := s.carpetasActuales()
		preview := ocurrencias
		if len(preview) > 20 {
			preview = preview[:20]
		}
		responder(w, map[string]interface{}{
			"total":               len(ocurrencias),
			"ocurrencias":         preview,
			"avisos":              avisos,
			"creadas":             0,
			"omitidas":            0,
			"bloques":             resolverBloques(extraerBloques(body.Markdown), carpetas),
			"carpetas_existentes": carpetasLoteJSON(carpetas),
		})
		return
	}

	ocurrencias, avisosParser := parseCompromisosMD(body.Markdown)
	if avisosParser == nil {
		avisosParser = []string{}
	}
	if ocurrencias == nil {
		ocurrencias = []ocurrenciaMD{}
	}
	ocurrencias, creadas, avisosCarpetas, err := s.aplicarDecisiones(body.Markdown, ocurrencias, body.Decisiones)
	if err != nil {
		responderError(w, 400, err.Error())
		return
	}
	avisos := append(avisosParser, avisosCarpetas...)

	// The lote is the source of truth: it is stored even when nothing
	// expands, so a bad paste can be fixed later with PUT instead of lost.
	nombre := strings.TrimSpace(body.Nombre)
	if nombre == "" {
		nombre = nombreLotePorDefecto(body.Markdown)
	}
	lote, err := db.CrearLote(s.Base, nombre, body.Markdown)
	if err != nil {
		responderError(w, 400, err.Error())
		return
	}

	creadasN, omitidas, avisos := s.guardarOcurrencias(ocurrencias, &lote.ID, avisos)
	responder(w, map[string]interface{}{
		"lote_id": lote.ID, "nombre": lote.Nombre,
		"creadas": creadasN, "omitidas": omitidas, "avisos": avisos, "total": len(ocurrencias),
		"carpetas_creadas": creadas,
	})
}

// listarLotes handles GET /api/agenda/imports.
// Replies [{id, nombre, total, creado, carpetas}] newest first, [] when empty.
// carpetas lists the distinct live folders linked from each lote's rows
// (empty when the lote is loose), so MD cargados can show its badges.
func (s *Servidor) listarLotes(w http.ResponseWriter, r *http.Request) {
	lotes, err := db.ListarLotes(s.Base)
	if err != nil {
		responderError(w, 500, "No se pudieron leer los lotes")
		return
	}
	out := []map[string]interface{}{}
	for _, l := range lotes {
		carpetas, err := db.ListarCarpetasPorLote(s.Base, l.ID)
		if err != nil {
			responderError(w, 500, "No se pudieron leer los lotes")
			return
		}
		out = append(out, map[string]interface{}{
			"id": l.ID, "nombre": l.Nombre, "total": l.Total, "creado": l.Creado,
			"carpetas": carpetasLoteJSON(carpetas),
		})
	}
	responder(w, out)
}

// obtenerLote handles GET /api/agenda/imports/{id}.
// Replies {id, nombre, markdown, total, creado, carpetas, ocurrencias,
// avisos, bloques, carpetas_existentes} or 404 in Spanish. ocurrencias
// re-resolves the stored markdown against live folders, so creating a
// folder later shows as vinculada here and on PUT.
func (s *Servidor) obtenerLote(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	lote, existe, err := db.ObtenerLote(s.Base, id)
	if err != nil {
		responderError(w, 500, "No se pudo leer el lote")
		return
	}
	if !existe {
		responderError(w, 404, "Lote no encontrado")
		return
	}
	carpetas, err := db.ListarCarpetasPorLote(s.Base, id)
	if err != nil {
		responderError(w, 500, "No se pudo leer el lote")
		return
	}
	ocurrencias, avisos := s.resolverImport(lote.Markdown)
	carpetasVivas := s.carpetasActuales()
	responder(w, map[string]interface{}{
		"id": lote.ID, "nombre": lote.Nombre, "markdown": lote.Markdown,
		"total": lote.Total, "creado": lote.Creado,
		"carpetas": carpetasLoteJSON(carpetas), "ocurrencias": ocurrencias, "avisos": avisos,
		"bloques": resolverBloques(extraerBloques(lote.Markdown), carpetasVivas),
		"carpetas_existentes": carpetasLoteJSON(carpetasVivas),
	})
}

// carpetasLoteJSON shapes live folders for list/detail responses.
// Slice is never nil so the JSON is [] and not null when loose.
func carpetasLoteJSON(carpetas []db.Carpeta) []map[string]interface{} {
	out := []map[string]interface{}{}
	for _, c := range carpetas {
		out = append(out, map[string]interface{}{"id": c.ID, "nombre": c.Nombre})
	}
	return out
}

// actualizarLote handles PUT /api/agenda/imports/{id} {nombre?, markdown, decisiones?}.
// It dry-parses first: zero occurrences is a hard 400 with avisos and
// nothing is touched. Then it validates decisiones (400 in Spanish, still
// touching nothing) and creates the requested folders in block order with
// the same semantics as POST. Otherwise old rows are wiped, the new markdown
// is expanded under the same lote_id with its final carpeta_id per block
// (creating the folder between edits links it alone), name + source are
// updated and it replies {lote_id, nombre, creadas, omitidas, avisos, total,
// carpetas_creadas}.
func (s *Servidor) actualizarLote(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	lote, existe, err := db.ObtenerLote(s.Base, id)
	if err != nil {
		responderError(w, 500, "No se pudo leer el lote")
		return
	}
	if !existe {
		responderError(w, 404, "Lote no encontrado")
		return
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || len(bytes.TrimSpace(raw)) == 0 {
		responderError(w, 400, "El markdown está vacío. Manda {\"markdown\":\"...\"}")
		return
	}
	var body struct {
		Markdown   string            `json:"markdown"`
		Nombre     string            `json:"nombre"`
		Decisiones []decisionCarpeta `json:"decisiones"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(raw), &body); err != nil || strings.TrimSpace(body.Markdown) == "" {
		responderError(w, 400, "El markdown está vacío. Manda {\"markdown\":\"...\"}")
		return
	}

	ocurrencias, avisosParser := parseCompromisosMD(body.Markdown)
	if avisosParser == nil {
		avisosParser = []string{}
	}
	if ocurrencias == nil {
		ocurrencias = []ocurrenciaMD{}
	}
	if len(ocurrencias) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":  "El markdown no produjo eventos. Revisá los avisos y corregilo sin salir de acá",
			"avisos": avisosParser,
		})
		return
	}

	// Validate + create folders BEFORE wiping: a bad decision leaves
	// every row and folder untouched.
	ocurrencias, creadas, avisosCarpetas, err := s.aplicarDecisiones(body.Markdown, ocurrencias, body.Decisiones)
	if err != nil {
		responderError(w, 400, err.Error())
		return
	}
	avisos := append(avisosParser, avisosCarpetas...)

	// Wipe first: no self-collision, so re-saving untouched markdown
	// restores the exact same rows (PUT is idempotent by content).
	if _, err := db.BorrarAgendaPorLote(s.Base, id); err != nil {
		responderError(w, 500, "No se pudo actualizar el lote")
		return
	}
	creadasN, omitidas, avisos := s.guardarOcurrencias(ocurrencias, &id, avisos)

	nombre := strings.TrimSpace(body.Nombre)
	if nombre == "" {
		nombre = lote.Nombre
	}
	act, _, err := db.ActualizarLote(s.Base, id, nombre, body.Markdown)
	if err != nil {
		responderError(w, 400, err.Error())
		return
	}
	responder(w, map[string]interface{}{
		"lote_id": act.ID, "nombre": act.Nombre,
		"creadas": creadasN, "omitidas": omitidas, "avisos": avisos, "total": len(ocurrencias),
		"carpetas_creadas": creadas,
	})
}

// borrarLote handles DELETE /api/agenda/imports/{id}.
// Removes the lote and its rows (loose rows survive) and replies {borradas}.
func (s *Servidor) borrarLote(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	borradas, existe, err := db.BorrarLote(s.Base, id)
	if err != nil {
		responderError(w, 500, "No se pudo borrar el lote")
		return
	}
	if !existe {
		responderError(w, 404, "Lote no encontrado")
		return
	}
	responder(w, map[string]interface{}{"borradas": borradas})
}

// guardarOcurrencias persists expanded rows under loteID (nil = loose).
// Each row keeps its resolved carpeta_id (nil = loose row).
// Exact duplicates (fecha+horas+texto) against existing rows are skipped
// and counted as omitidas; insert failures join avisos in Spanish.
// The duplicate key stays folder-blind on purpose: re-importing the same
// time+text with a folder still counts as the same event.
func (s *Servidor) guardarOcurrencias(ocurrencias []ocurrenciaMD, loteID *int64, avisos []string) (int, int, []string) {
	creadas, omitidas := 0, 0
	if len(ocurrencias) == 0 {
		return creadas, omitidas, avisos
	}

	desdeMin, hastaMax := ocurrencias[0].Fecha, ocurrencias[0].Fecha
	for _, o := range ocurrencias[1:] {
		if o.Fecha < desdeMin {
			desdeMin = o.Fecha
		}
		if o.Fecha > hastaMax {
			hastaMax = o.Fecha
		}
	}
	existentes, err := db.ListarAgenda(s.Base, desdeMin, hastaMax)
	if err != nil {
		return creadas, len(ocurrencias), append(avisos, "No se pudo leer la agenda para verificar duplicados")
	}
	vistas := map[string]bool{}
	for _, e := range existentes {
		vistas[claveOcurrencia(e.Fecha, e.HoraInicio, e.HoraFin, e.Texto)] = true
	}

	for _, o := range ocurrencias {
		clave := claveOcurrencia(o.Fecha, o.HoraInicio, o.HoraFin, o.Texto)
		if vistas[clave] {
			omitidas++
			continue
		}
		if _, err := db.CrearAgendaEnLote(s.Base, o.Fecha, o.HoraInicio, o.HoraFin, o.Texto, o.CarpetaID, loteID); err != nil {
			omitidas++
			avisos = append(avisos, fmt.Sprintf("No se pudo guardar %s %s", o.Fecha, o.Texto))
			continue
		}
		vistas[clave] = true
		creadas++
	}
	return creadas, omitidas, avisos
}

// esPreview reads ?dry=true (or ?preview=true) from the import endpoint.
func esPreview(r *http.Request) bool {
	q := r.URL.Query()
	return q.Get("dry") == "true" || q.Get("dry") == "1" || q.Get("preview") == "true" || q.Get("preview") == "1"
}

// nombreLotePorDefecto is the first "## Compromiso: <title>" or a timestamp.
// It reuses compromisoRe so the rule never drifts from the parser.
func nombreLotePorDefecto(markdown string) string {
	if m := compromisoRe.FindStringSubmatch(markdown); len(m) >= 2 {
		if titulo := strings.TrimSpace(m[1]); titulo != "" {
			return titulo
		}
	}
	return "Import " + time.Now().Format("2006-01-02 15:04")
}

func claveOcurrencia(fecha, inicio, fin, texto string) string {
	return fecha + "\x00" + inicio + "\x00" + fin + "\x00" + texto
}
