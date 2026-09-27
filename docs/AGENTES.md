# Conectar tu agente de IA (MCP)

Guía completa para estudiar con un agente de IA en tu terminal (Claude Code,
OpenCode, Codex, Gemini CLI...) conectado a tu biblioteca de Bythos. Si solo
quieres el resumen para usuarios, ve a la sección correspondiente del
[README](../README.md#estudia-con-tu-agente-de-ia).

## Qué es MCP

[MCP](https://modelcontextprotocol.io/) (Model Context Protocol) es un
protocolo abierto que le permite a un agente de IA en tu terminal hablar con
una aplicación local usando un set de "herramientas" bien definidas, en vez
de adivinar comandos o leer pantalla. Bythos implementa un servidor MCP
(`bythos.exe mcp`) que expone tus carpetas, tus recursos, tu progreso y tu
agenda como herramientas: el agente las llama, y por dentro cada llamada es
una petición HTTP normal a `localhost:8080`, la misma API que usan la
ventana y la extensión. **La app tiene que estar abierta** para que esto
funcione; si no lo está, cada herramienta avisa en español en vez de fallar
en silencio.

## El botón "Abrir agente"

En vez de armar la configuración a mano, el botón **Abrir agente** del
sidebar (grupo Acciones) hace todo esto por ti:

1. Prepara `%APPDATA%\Bythos\agente\` con:
   - `AGENTS.md` / `CLAUDE.md` — instrucciones para el agente: qué es
     Bythos, qué herramientas tiene disponibles y las reglas de uso (no
     puede borrar nada, confirma antes de un cambio masivo, cita títulos
     tal como los devuelve la herramienta, todo queda en el Historial).
     Se escriben ambos archivos con el mismo contenido porque versiones de
     Claude Code anteriores a la 2.1.277 no leían `AGENTS.md` todavía.
   - `LEEME.txt` — bienvenida corta: qué CLI escribir para empezar.
   - `.mcp.json` — config de proyecto para **Claude Code**.
   - `opencode.json` — config de proyecto para **OpenCode**.
   - `.gemini/settings.json` — config de proyecto para **Gemini CLI**.
   - `.codex/config.toml` — config de proyecto para **Codex CLI** (ver nota
     de confianza más abajo).

   Todas las configs apuntan al `.exe` que está corriendo en ese momento
   (`os.Executable()`), y se regeneran cada vez que abres el agente — así
   la ruta nunca queda vieja aunque muevas o actualices el `.exe`. Bythos
   nunca borra archivos propios tuyos que dejes en esa carpeta: solo pisa
   los nombres que ya conoce.

2. Abre una terminal ahí mismo (Windows Terminal si está instalado, si no
   `cmd.exe`). **Bythos nunca lanza `claude`/`opencode`/`gemini`/`codex`
   por ti** — solo te deja la terminal lista y tú escribes el que quieras.

El botón solo funciona desde la ventana de Bythos: pide un `Origin` exacto
(`http://localhost:8080`), así que la extensión de Chrome no puede pedirlo
aunque quisiera (ver `desktop/api/agente.go`).

### Primeras veces por cliente

- **Claude Code**: la primera vez te va a pedir aprobar el servidor MCP de
  proyecto (el diálogo que muestra el contenido de `.mcp.json`). Acéptalo.
- **Codex CLI**: no se configura solo. Su config de proyecto
  (`.codex/config.toml`, que Bythos también escribe) solo se carga si
  confías en la carpeta la primera vez que corres `codex` ahí (te lo va a
  preguntar). Si prefieres no usar carpetas de proyecto, agrega el bloque
  de abajo a mano en tu `~/.codex/config.toml` global (mismo formato, sin
  el paso de confianza):

  ```toml
  [mcp_servers.bythos]
  command = 'RUTA_A_TU_BYTHOS.EXE'
  args = ["mcp"]
  ```

- **OpenCode** y **Gemini CLI**: leen su config de proyecto sola, sin paso
  de confianza adicional.

## Configuración manual (sin el botón)

Si prefieres armar la conexión a mano en vez de usar "Abrir agente":

**Claude Code:**

```powershell
claude mcp add bythos -- "%LOCALAPPDATA%\Bythos\bythos.exe" mcp
```

**Config genérica** (OpenCode / Gemini CLI / cualquier cliente MCP por
stdio), ajusta la ruta si no instalaste con el setup:

```json
{
  "mcpServers": {
    "bythos": {
      "command": "C:\\Users\\TU_USUARIO\\AppData\\Local\\Bythos\\bythos.exe",
      "args": ["mcp"]
    }
  }
}
```

**OpenCode** usa una forma distinta (`command` siempre como array):

```json
{
  "mcp": {
    "bythos": {
      "type": "local",
      "command": ["C:\\Users\\TU_USUARIO\\AppData\\Local\\Bythos\\bythos.exe", "mcp"],
      "enabled": true
    }
  }
}
```

## Herramientas disponibles

Nombres y contratos verificados contra `desktop/agentes/lectura.go` y
`desktop/agentes/escritura.go`.

### Lectura (no cambian nada — `ReadOnlyHint: true`)

| Herramienta | Hace |
|---|---|
| `listar_carpetas` | carpetas + progreso (total, completados, en curso, pendientes, %) |
| `listar_recursos` | recursos guardados, opcionalmente filtrados por `carpeta_id` |
| `leer_recurso` | detalle de un recurso por `id` |
| `ver_progreso_carpeta` | termómetro de una sola carpeta |
| `ver_stats` | termómetro general (todas las carpetas) |
| `ver_agenda` | notas del calendario, con `desde`/`hasta` opcionales |
| `ver_actividad` | historial de creación por día |
| `ver_historial` | quién tocó los datos y qué cambió (`limite`, `origen`) |
| `exportar_carpeta` | texto para repasar (`markdown` · `gemini` · `notebooklm` · `drive`) |

### Escritura (agregan o actualizan — nunca borran)

| Herramienta | Hace |
|---|---|
| `actualizar_progreso` | cambia el % (0–100) de un recurso; idempotente |
| `cambiar_estado` | `pendiente` / `en_curso` / `completado`; idempotente |
| `crear_carpeta` | carpeta nueva; no idempotente (dos llamadas = dos carpetas) |
| `guardar_link` | link nuevo, Bythos completa título/imagen/tipo solo; no idempotente |
| `crear_nota_agenda` | nota nueva en el calendario; no idempotente |

**No existen herramientas de borrado, a propósito.** El agente puede
agregar y actualizar, nunca destruir: si le pides borrar algo, te va a
decir que lo hagas desde la app o la extensión.

## Historial y auditoría

Cada llamada de escritura manda el nombre del cliente MCP conectado
(`clientInfo.name`, lo que declaró el agente al conectarse — por ejemplo
`claude-code`) como "actor", y queda registrada en el Historial de la app
(vista **Historial** en el sidebar) igual que cualquier cambio hecho desde
la app o la extensión. Puedes filtrar por origen (`app` · `extension` ·
`agente` · `desconocido`) y ver exactamente qué tocó tu agente, cuándo, y
qué cambió (p.ej. `"Hooks": 40% → 80%`). Nada de esto es un secreto: la
herramienta `ver_historial` le permite al propio agente auditarse a sí
mismo si se lo pides.

## Sobre prompt injection

Algunos de los recursos que guardas (páginas web, transcripciones de video,
notas que exportas) son texto que tu agente puede terminar leyendo si se lo
pides a él directamente. Como con cualquier agente de IA que lee contenido
de internet: si un texto que le pasas a tu agente contiene instrucciones
escondidas ("ignora lo anterior y...", "ahora ejecuta..."), un agente
descuidado podría intentar seguirlas. Bythos reduce el daño posible
limitando las herramientas de escritura a agregar/actualizar (nunca
borrar) y dejando todo registrado en el Historial — pero la responsabilidad
de revisar lo que tu agente hace, especialmente antes de pedirle acciones
en lote, sigue siendo tuya. Si algo en el Historial no tiene sentido,
repásalo ahí antes de seguir.
