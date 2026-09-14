package api

// avance.go — Import progress back from corrected markdown.
// The user exports "- [40%] Title <!-- id:7 -->", lets their AI update the
// percentages, pastes the text back, and we parse id+% and save it.
// Pure parser (parseAvance) is unit-tested without DB; the handler adds
// folder scope and counting.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"bythos-desktop/db"
)

// avanceRe matches "[N%] ... <!-- id:X -->". Middle is free-form so the
// AI can reorder or reword titles without breaking the import.
var avanceRe = regexp.MustCompile(`\[(\d{1,3})%\].*<!--\s*id:(\d+)\s*-->`)

type avanceItem struct {
	ID       int64
	Progreso int
}

// parseAvanceLine extracts one (id, progreso) from a single line.
// ok=false means the line carries no progress marker and must be ignored.
func parseAvanceLine(line string) (id int64, progreso int, ok bool) {
	m := avanceRe.FindStringSubmatch(line)
	if m == nil {
		return 0, 0, false
	}
	p, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, 0, false
	}
	n, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil || n <= 0 {
		return 0, 0, false
	}
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return n, p, true
}

// parseAvance scans whole markdown text line by line.
// Header lines ("# ...") and empty lines are skipped silently;
// any other line without a match counts as omitted.
func parseAvance(texto string) (items []avanceItem, omitidos int) {
	for _, line := range strings.Split(texto, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		id, prog, ok := parseAvanceLine(t)
		if !ok {
			omitidos++
			continue
		}
		items = append(items, avanceItem{ID: id, Progreso: prog})
	}
	return items, omitidos
}

func carpetaExiste(s *Servidor, id int64) (bool, error) {
	carpetas, err := db.ListarCarpetas(s.Base)
	if err != nil {
		return false, err
	}
	for _, c := range carpetas {
		if c.ID == id {
			return true, nil
		}
	}
	return false, nil
}

// importarAvance handles POST /api/carpetas/{id}/import-avance.
// Body is either {"markdown":"..."} or [{"id":7,"progreso":40}].
// It clamps 0-100, ignores lines without a match, updates only rows
// of this folder, and replies {"actualizados":N,"omitidos":M}.
func (s *Servidor) importarAvance(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	ok, err := carpetaExiste(s, id)
	if err != nil {
		responderError(w, 500, "No se pudieron leer las carpetas")
		return
	}
	if !ok {
		responderError(w, 404, "Carpeta no encontrada")
		return
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || len(bytes.TrimSpace(raw)) == 0 {
		responderError(w, 400, "Manda {\"markdown\":\"...\"}")
		return
	}
	trimmed := bytes.TrimSpace(raw)

	// JSON array variant: [{id, progreso}]
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var arr []struct {
			ID       int64 `json:"id"`
			Progreso *int  `json:"progreso"`
		}
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			responderError(w, 400, "JSON inválido. Manda {\"markdown\":\"...\"}")
			return
		}
		actualizados, omitidos := 0, 0
		for _, it := range arr {
			if it.ID <= 0 || it.Progreso == nil {
				omitidos++
				continue
			}
			p := *it.Progreso
			if p < 0 {
				p = 0
			}
			if p > 100 {
				p = 100
			}
			done, err := db.ActualizarProgresoEnCarpeta(s.Base, id, it.ID, p)
			if err != nil || !done {
				omitidos++
				continue
			}
			actualizados++
		}
		responder(w, map[string]int{"actualizados": actualizados, "omitidos": omitidos})
		return
	}

	var body struct {
		Markdown string `json:"markdown"`
	}
	if err := json.Unmarshal(trimmed, &body); err != nil || body.Markdown == "" {
		responderError(w, 400, "Manda {\"markdown\":\"...\"}")
		return
	}

	items, omitidos := parseAvance(body.Markdown)
	actualizados := 0
	for _, it := range items {
		done, err := db.ActualizarProgresoEnCarpeta(s.Base, id, it.ID, it.Progreso)
		if err != nil || !done {
			omitidos++
			continue
		}
		actualizados++
	}
	responder(w, map[string]int{"actualizados": actualizados, "omitidos": omitidos})
}
