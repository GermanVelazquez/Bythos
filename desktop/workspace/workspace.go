package workspace

// workspace.go — El escritorio de trabajo del agente: %APPDATA%\Bythos\agente\.
// Bythos NUNCA lanza un CLI de IA (eso lo hace el usuario a mano desde la
// terminal, ver desktop/terminal/), pero le deja el terreno listo:
// instrucciones (AGENTS.md/CLAUDE.md) + config de MCP para cada cliente
// que soporta config a nivel de proyecto, apuntando siempre al .exe que
// está corriendo (os.Executable(), lo pasa api/agente.go) para que
// sobreviva a que el usuario mueva o actualice el .exe.
//
// Se REGENERA en cada apertura: pisa los archivos que Bythos controla
// (para que la ruta del .exe nunca quede vieja) pero jamás borra nada.
// Si el usuario deja sus propios archivos en esta carpeta, Bythos no los
// toca: solo escribe los nombres que conoce.
//
// Formato de cada config, investigado y con su fuente (todo a nivel de
// PROYECTO: ningún archivo global del usuario se toca):
//
//   - Claude Code: .mcp.json en la raíz, {"mcpServers": {nombre: {command,
//     args}}}. La primera vez pide aprobar el servidor de proyecto
//     (claude mcp list / diálogo interactivo). No lee AGENTS.md en
//     versiones anteriores a la 2.1.277 (18-sep-2026): por eso este
//     archivo escribe TAMBIÉN CLAUDE.md con el mismo contenido.
//     https://code.claude.com/docs/en/mcp
//   - OpenCode: opencode.json en la raíz, {"mcp": {nombre: {type:"local",
//     command:[...], enabled}}}. command SIEMPRE array, nunca un string
//     con espacios. Un opencode.json en la raíz del proyecto se lee solo.
//     https://opencode.ai/docs/mcp-servers/
//   - Gemini CLI: .gemini/settings.json en la raíz, {"mcpServers":
//     {nombre: {command, args}}}. Se lee solo para esa carpeta.
//     https://google-gemini.github.io/gemini-cli/docs/tools/mcp-server.html
//   - Codex CLI: SÍ tiene config de proyecto (.codex/config.toml), pero
//     solo se carga si el usuario "confía" en la carpeta (workspace
//     trust) la primera vez que corre Codex ahí; no hay forma de otorgar
//     esa confianza desde afuera, así que este archivo escribe el TOML
//     igual (no toca ~/.codex/config.toml global) y LEEME.txt explica el
//     paso de confianza + una alternativa manual.
//     https://developers.openai.com/codex/config-reference

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// RutaWorkspace es la carpeta del agente. Hermana de db.RutaPorDefecto():
// mismo %APPDATA%/Bythos, subcarpeta "agente" para no mezclar con bythos.db.
func RutaWorkspace() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return "agente" // plan B, igual criterio que db.RutaPorDefecto
	}
	return filepath.Join(base, "Bythos", "agente")
}

// PrepararWorkspace crea (si falta) la carpeta del agente y escribe/pisa
// los archivos que Bythos controla. exe es la ruta al .exe corriendo
// (os.Executable(), inyectada por el caller para no atar este paquete a
// os.Executable real en los tests). Devuelve la carpeta lista para abrir
// una terminal ahí.
func PrepararWorkspace(exe string) (string, error) {
	dir := RutaWorkspace()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	instrucciones := textoInstrucciones()
	if err := escribirTexto(filepath.Join(dir, "AGENTS.md"), instrucciones); err != nil {
		return "", err
	}
	// Mismo contenido en CLAUDE.md: así da igual si la versión de Claude
	// Code instalada todavía no lee AGENTS.md (ver comentario de arriba).
	if err := escribirTexto(filepath.Join(dir, "CLAUDE.md"), instrucciones); err != nil {
		return "", err
	}
	if err := escribirTexto(filepath.Join(dir, "LEEME.txt"), textoBienvenida()); err != nil {
		return "", err
	}

	if err := escribirJSON(filepath.Join(dir, ".mcp.json"), mcpJSONClaudeCode(exe)); err != nil {
		return "", err
	}
	if err := escribirJSON(filepath.Join(dir, "opencode.json"), mcpJSONOpenCode(exe)); err != nil {
		return "", err
	}

	dirGemini := filepath.Join(dir, ".gemini")
	if err := os.MkdirAll(dirGemini, 0755); err != nil {
		return "", err
	}
	if err := escribirJSON(filepath.Join(dirGemini, "settings.json"), mcpJSONGemini(exe)); err != nil {
		return "", err
	}

	dirCodex := filepath.Join(dir, ".codex")
	if err := os.MkdirAll(dirCodex, 0755); err != nil {
		return "", err
	}
	if err := escribirTexto(filepath.Join(dirCodex, "config.toml"), tomlCodex(exe)); err != nil {
		return "", err
	}

	return dir, nil
}

func escribirTexto(ruta, contenido string) error {
	return os.WriteFile(ruta, []byte(contenido), 0644)
}

// escribirJSON serializa con encoding/json, NUNCA concatenación de texto:
// así una ruta de Windows con \ se escapa sola ("C:\\Users\\...") sin
// arriesgar un JSON roto a mano.
func escribirJSON(ruta string, dato any) error {
	b, err := json.MarshalIndent(dato, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ruta, b, 0644)
}

// --- Config MCP por cliente ---

// mcpServidorStdio es la forma común de un server MCP por stdio en Claude
// Code y Gemini CLI: {"command": "...", "args": [...]}.
type mcpServidorStdio struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// mcpJSONClaudeCode arma .mcp.json.
func mcpJSONClaudeCode(exe string) map[string]any {
	return map[string]any{
		"mcpServers": map[string]any{
			"bythos": mcpServidorStdio{Command: exe, Args: []string{"mcp"}},
		},
	}
}

type opencodeServidorLocal struct {
	Type    string   `json:"type"`
	Command []string `json:"command"`
	Enabled bool     `json:"enabled"`
}

func mcpJSONOpenCode(exe string) map[string]any {
	return map[string]any{
		"mcp": map[string]any{
			"bythos": opencodeServidorLocal{Type: "local", Command: []string{exe, "mcp"}, Enabled: true},
		},
	}
}

func mcpJSONGemini(exe string) map[string]any {
	return map[string]any{
		"mcpServers": map[string]any{
			"bythos": mcpServidorStdio{Command: exe, Args: []string{"mcp"}},
		},
	}
}

// tomlCodex arma .codex/config.toml. command va como TOML literal string
// (comillas simples): TOML no procesa escapes ahí adentro, así que una
// ruta de Windows con \ viaja tal cual, sin duplicar cada barra a mano.
func tomlCodex(exe string) string {
	return "# Config de PROYECTO para Codex CLI (ver LEEME.txt en esta carpeta).\n" +
		"# Solo se carga si confiás en esta carpeta (Codex te lo pregunta la\n" +
		"# primera vez que corre acá); no toca ~/.codex/config.toml global.\n" +
		"# Fuente: https://developers.openai.com/codex/config-reference\n" +
		"[mcp_servers.bythos]\n" +
		"command = " + tomlLiteral(exe) + "\n" +
		"args = [\"mcp\"]\n"
}

// tomlLiteral encierra s como TOML literal string. Fallback a basic
// string con escape manual solo si s trae una comilla simple (poco
// probable en una ruta de .exe, pero mejor no romper el TOML si pasa).
func tomlLiteral(s string) string {
	if !strings.Contains(s, "'") {
		return "'" + s + "'"
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}

// --- Textos en español (instrucciones + bienvenida) ---

// textoInstrucciones documenta Bythos, las herramientas MCP disponibles
// (mismos nombres/orden que RegistrarLectura + RegistrarEscritura, ver
// lectura.go y escritura.go) y las reglas de uso. Contenido manual: si
// agregás o cambiás una herramienta, actualizá también esta lista.
func textoInstrucciones() string {
	return `# Bythos — instrucciones para el agente

Bythos es un gestor de recursos de estudio: carpetas (temas), links
guardados con su progreso (0 a 100%) y una agenda/calendario. Vos (el
agente conectado por MCP) hablás con la app real por HTTP a través de
estas herramientas: si Bythos no está abierto en la PC del usuario, cada
herramienta te va a avisar en vez de fallar en silencio.

## Herramientas disponibles

Lectura (no cambian nada):
- listar_carpetas: carpetas con su progreso (total, completados, en
  curso, pendientes, %).
- listar_recursos: recursos guardados, opcionalmente filtrados por
  carpeta_id.
- leer_recurso: detalle completo de un recurso por id.
- ver_progreso_carpeta: termómetro de UNA carpeta.
- ver_stats: termómetro general (todas las carpetas).
- ver_agenda: notas del calendario, con desde/hasta opcionales.
- ver_actividad: historial de creación por día.
- ver_historial: quién tocó los datos y qué cambió (limite, origen).
- exportar_carpeta: texto para repasar (markdown, gemini, notebooklm,
  drive).

Escritura (agregan o actualizan, nunca destruyen):
- actualizar_progreso: cambia el % (0-100) de un recurso.
- cambiar_estado: pendiente / en_curso / completado.
- crear_carpeta: carpeta nueva.
- guardar_link: link nuevo (Bythos completa título/imagen/tipo solo).
- crear_nota_agenda: nota nueva en el calendario.

## Reglas

1. No existen herramientas para BORRAR nada (ni carpetas, ni recursos,
   ni notas). Si el usuario te pide borrar algo, decile que lo haga
   desde la app o la extensión: vos no tenés esa herramienta a
   propósito.
2. Antes de hacer un cambio MASIVO (varias carpetas, varios links o
   varias notas de una sola vez), confirmá primero con el usuario qué
   vas a hacer. Una herramienta a la vez para un pedido puntual está
   bien sin preguntar; un lote grande no.
3. Cuando menciones un recurso o carpeta en tu respuesta, citá su
   título (o el de la carpeta) tal como lo devuelve la herramienta, no
   inventes uno ni lo resumas de más: el usuario tiene que poder
   reconocerlo en la app.
4. Todo cambio que hagas queda en el Historial de la app (vista
   Historial), con tu nombre de cliente MCP si lo mandaste al conectar.
   No es un secreto: el usuario puede auditar todo lo que hiciste.

## Sobre esta carpeta

Esta carpeta (` + "`%APPDATA%\\Bythos\\agente`" + ` en Windows) es tu
espacio de trabajo con Bythos. Bythos regenera AGENTS.md, CLAUDE.md,
LEEME.txt, .mcp.json, opencode.json, .gemini/settings.json y
.codex/config.toml cada vez que se abre esta terminal desde el botón
"Abrir agente" de la app, para que la ruta al .exe nunca quede vieja.
Cualquier otro archivo que dejes acá, Bythos no lo toca.
`
}

// textoBienvenida es LEEME.txt: lo primero que ve el usuario en la
// terminal (cmd.exe /k type LEEME.txt en el fallback sin Windows
// Terminal). Corto a propósito.
func textoBienvenida() string {
	return `Bythos preparó esta carpeta para tu agente de IA.

Escribí uno de estos para empezar (Bythos NO los lanza por vos):

    claude
    opencode
    gemini

El servidor MCP de Bythos ya está configurado como proyecto para los
tres. Si tu terminal soporta otro cliente MCP, mirá AGENTS.md.

- Claude Code: la primera vez te va a pedir aprobar el servidor de
  proyecto (.mcp.json). Aceptalo desde el diálogo de Claude.
- Codex CLI: no está en la lista de arriba porque necesita que confíes
  en esta carpeta la primera vez que corrés "codex" acá (te lo va a
  preguntar). Si preferís no usar carpetas de proyecto, agregá esto a
  mano en tu config global (~/.codex/config.toml):
      [mcp_servers.bythos]
      command = 'RUTA_A_TU_BYTHOS.EXE'
      args = ["mcp"]

Bythos no toca ningún archivo tuyo en esta carpeta: solo escribe los
que ya conoce (los de arriba). Podés agregar los tuyos sin miedo.
`
}
