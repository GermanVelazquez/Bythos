package api

// export.go — El REPASO. Convierte tu carpeta en texto para estudiar fuera:
// - markdown: lista formateada (para tus notas u Obsidian)
// - gemini: prompt listo para pegar en la IA de Google (resumen + preguntas)
// - notebooklm: instrucciones + links para importar uno por uno
// - drive: mismo markdown pero como ARCHIVO descargable (.md para subir)
//
// ¿Por qué texto y no PDFs ni zips? Porque texto se copia, se versiona
// y se pega en cualquier IA. Formato simple = compatible con todo.
// Dormido hasta cablearlo en Rutas() la próxima clase.

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"bythos-desktop/db"
)

// exportarCarpeta atiende GET /api/carpetas/{id}/export?format=...
// Busca el nombre con ListarCarpetas (sin SQL nuevo: reutiliza lo que hay).
// Para <100 carpetas el loop en Go es instantáneo; si tuvieras 10k,
// ahí sí agregarías un SELECT por id en db/.
func (s *Servidor) exportarCarpeta(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	formato := strings.ToLower(r.URL.Query().Get("format"))
	if formato == "" {
		formato = "markdown"
	}

	// Nombre de la carpeta (404 si no es tuya/existe)
	nombre := ""
	carpetas, err := db.ListarCarpetas(s.Base)
	if err != nil {
		responderError(w, 500, "No se pudieron leer las carpetas")
		return
	}
	for _, c := range carpetas {
		if c.ID == id {
			nombre = c.Nombre
			break
		}
	}
	if nombre == "" {
		responderError(w, 404, "Carpeta no encontrada")
		return
	}

	items, err := db.ListarRecursos(s.Base, id)
	if err != nil {
		responderError(w, 500, "No se pudo leer la carpeta")
		return
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	switch formato {
	case "markdown":
		fmt.Fprint(w, markdownCarpeta(nombre, items))
	case "gemini":
		fmt.Fprint(w, geminiRepaso(nombre, items))
	case "notebooklm":
		fmt.Fprint(w, guiaNotebookLM(nombre, items))
	case "drive":
		// Mismo contenido, pero el navegador lo DESCARGA como archivo listo
		w.Header().Set("Content-Disposition", `attachment; filename="bythos-`+nombre+`.md"`)
		fmt.Fprint(w, markdownCarpeta(nombre, items))
	default:
		responderError(w, 400, "Formato inválido. Usa: markdown, gemini, notebooklm o drive")
	}
}

func tituloDe(r db.Recurso) string {
	if r.Titulo != "" {
		return r.Titulo
	}
	return r.URL
}

func markdownCarpeta(nombre string, items []db.Recurso) string {
	var b strings.Builder
	b.WriteString("# " + nombre + " (exportado desde Bythos)\n\n")
	b.WriteString("Pega este texto en tu IA para repasar tus recursos.\n\n")
	if len(items) == 0 {
		return b.String() + "_Carpeta vacía._\n"
	}
	b.WriteString("## Mis recursos\n")
	for _, it := range items {
		fmt.Fprintf(&b, "- [%d%%] %s <!-- id:%d -->\n", clampExport(it.Progreso), tituloDe(it), it.ID)
	}
	ejemplo := fmt.Sprintf("- [%d%%] %s <!-- id:%d -->", clampExport(items[0].Progreso), tituloDe(items[0]), items[0].ID)
	b.WriteString(bloqueActualizacion(ejemplo))
	return b.String()
}

func clampExport(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// bloqueActualizacion son las instrucciones estrictas para que la IA
// devuelva avance parseable por parseAvance (ver avance.go).
// Ejemplo usa un id real de la carpeta para que el modelo copie el shape.
// Mantiene los marcadores que exige TestExportPromptShape.
func bloqueActualizacion(ejemplo string) string {
	var b strings.Builder
	b.WriteString("\n## Para actualizar tu avance en Bythos\n" +
		"Copia el mensaje de abajo y pégalo en tu IA junto con la lista de arriba.\n" +
		"Tu IA debe devolver SOLO las líneas actualizadas con este formato exacto, " +
		"una por línea, sin texto extra:\n" +
		"- [N%] Título <!-- id:X -->\n" +
		"Donde N es un número de 0 a 100 y <!-- id:X --> no se borra ni se cambia.\n" +
		"Reglas: una línea por recurso, sin encabezados, sin explicaciones, " +
		"sin bloques de código; conserva cada <!-- id:X --> tal cual; " +
		"si no sabes el avance de uno, repite su línea sin cambios.\n")
	if ejemplo != "" {
		b.WriteString("Ejemplo con un id real de esta carpeta (cambia solo N):\n" + ejemplo + "\n")
	}
	b.WriteString("Mensaje listo para pegar:\n" +
		"\"Actualiza mi avance y devuélveme SOLO las líneas en formato " +
		"- [N%] Título <!-- id:X -->, una por línea, sin texto extra. " +
		"N de 0 a 100. No cambies ni borres ningún <!-- id:X -->.\"\n" +
		"Luego pega esa respuesta en Bythos → Importar avance.\n")
	return b.String()
}

func geminiRepaso(nombre string, items []db.Recurso) string {
	var b strings.Builder
	b.WriteString("# Repaso con Gemini — " + nombre + "\n\n")
	b.WriteString("Pega este texto en Gemini para resumen y preguntas.\n\n## Mis recursos\n")
	for i, it := range items {
		fmt.Fprintf(&b, "%d. %s — %s\n", i+1, tituloDe(it), it.URL)
	}
	b.WriteString("\n## Pídele a Gemini:\n" +
		"- Resúmeme por temas estos recursos.\n" +
		"- Genera 10 preguntas de repaso con respuestas.\n" +
		"- Arma un plan de 7 días con este material.\n")
	ejemplo := ""
	if len(items) > 0 {
		ejemplo = fmt.Sprintf("- [%d%%] %s <!-- id:%d -->", clampExport(items[0].Progreso), tituloDe(items[0]), items[0].ID)
	}
	b.WriteString(bloqueActualizacion(ejemplo))
	b.WriteString("\n## Recursos con avance (para que la IA conserve cada id)\n")
	for _, it := range items {
		fmt.Fprintf(&b, "- [%d%%] %s <!-- id:%d -->\n", clampExport(it.Progreso), tituloDe(it), it.ID)
	}
	return b.String()
}

func guiaNotebookLM(nombre string, items []db.Recurso) string {
	var b strings.Builder
	b.WriteString("# Importar a NotebookLM — " + nombre + "\n\n")
	b.WriteString("1. Entra a notebooklm.google.com y crea un cuaderno.\n" +
		"2. Agregar fuente → Sitio web o YouTube.\n" +
		"3. Pega cada enlace (Bythos ya te ahorró copiarlos a mano):\n\n")
	for i, it := range items {
		fmt.Fprintf(&b, "%d. %s\n", i+1, it.URL)
	}
	b.WriteString("\n4. Pide resumen y guía de estudio.\n")
	return b.String()
}
