# Design: Terminal Phase 1 (bythos-terminal-fase1)

## Technical Approach

Two new pure-Go packages hold the Windows-specific and domain logic. `terminal/` gets the ConPTY, the Job Object, the session manager and the ring buffer. The new `pizarra/` package holds the op validation and the scene summary. `api/` stays thin HTTP over them, following the existing seam style (`lanzadorAgente`, `prepararWorkspaceAgente`).

The UI owns the scene. Agent ops and capture requests travel over a 1 s `latido` poll, which only runs while the Terminal view is mounted. Every browser-facing mutation uses the existing exact-Origin pattern (`origenesAgentePermitidos`). The agent (stdio MCP, which sends no Origin) gets its own narrow guard.

Confirmed proposal decisions are not reopened. The open forks below are resolved with rationale.

## Architecture Decisions

| # | Decision | Choice | Rejected | Rationale |
|---|---|---|---|---|
| D1 | Package layout | `terminal/pty.go` (interface, `Config`, `ErrNoSoportado`), `conpty_windows.go`, `conpty_other.go`, `anillo.go`, `gestor.go`. These files sit next to the existing Lanzador files (`terminal.go`, `terminal_windows.go`, `terminal_other.go`) with no file or identifier collisions: the new code must not redeclare `Lanzador`, `NuevoLanzador`, `lanzadorSistema`, `createNewConsole` or `sysProcAttrConsola`. New domain package `pizarra/` (`ops.go`, `resumen.go`). `api/terminal.go` (ticket, ws, pty kill), `api/sesiones.go` (CRUD, scene), `api/pizarra.go` (latido, ops, capture). `db/sesiones.go`, `db/pizarra_ops.go`. `agentes/pizarra.go`. | Validation duplicated in `agentes` and `api`; logic in `api/` | `agentes` must not import `api` (it is a separate process). A shared pure package gives one validator, and table tests need no HTTP. |
| D2 | Shell | `%ComSpec%`, falling back to `C:\Windows\System32\cmd.exe`, with cwd = `workspace.PrepararWorkspace(exe)`. Env = `os.Environ()` plus `BYTHOS_SESION=<id>` (an integer formatted by Go). No HTTP input reaches the command. | `powershell.exe` | With a Restricted execution policy, PowerShell resolves npm `claude.ps1` shims and fails. `cmd` runs `.cmd` shims, and the user can still type `powershell`. |
| D3 | Job scope | One Job **per PTY session**, with `KILL_ON_JOB_CLOSE`. | One global job | `TerminateJobObject` kills the whole tree (cmd → node → claude) before `ClosePseudoConsole`, which avoids orphans and the 0xc0000142 dialog. When bythos.exe exits, even by TerminateProcess, the OS closes every job handle and every child dies. |
| D4 | Window close | PTYs survive the window. A reaper kills a PTY that has been **detached for 10 min** (`Gracia`). Every child dies when the exe exits (D3), and a tab's "Cerrar terminal" kills that PTY. | Kill on detach; never reap | A reload and a window close look the same to the server, and reload must reattach. Combined with single-instance relaunch (D14), the user can come back within 10 min. The grace limit bounds how long an unattended agent keeps running. |
| D5 | Attach model | One attachment per session; a new attach replaces the old one (close code 4000). Snapshot and subscribe happen atomically under the session mutex. The subscriber channel holds 256 chunks; **if it is full the connection is dropped** and the PTY reader never blocks. | Fan-out to N clients; a blocking send | A blocked reader stops draining the output pipe and deadlocks ConPTY. A dropped client reattaches and gets the replay. |
| D6 | Ring | 256 KiB per session, overwrite-oldest. The client calls `term.reset()` before writing the replay. | Unbounded log | Bounded memory (8 × 256 KiB). A replay can start in the middle of an escape sequence; the reset limits the garbage to the first line. |
| D7 | WS library and limits | coder/websocket v1.8.15. **The authority is our own exact-Origin pre-check (D9 order).** `OriginPatterns` is defense in depth only, with host-only patterns: `["localhost:8080"]`, plus `"localhost:5173"` only when `modoDev()`. We do not rely on its scheme matching. Never `InsecureSkipVerify`. `SetReadLimit` and the PTY cap follow the Limits table. | gorilla; trusting `Accept` alone | `Accept` treats a request with no Origin as allowed, so on its own it would admit an empty Origin. The pre-check rejects that. |
| D8 | Guards | Every route first passes `conGuardia` (Host).<br>**Ventana** (POST, PUT, PATCH, DELETE and WS): exact Origin only; rejects an empty Origin and the extension prefix.<br>**Ventana-lectura** (UI GETs): rejects any Origin that is not exact, which includes the extension, and rejects a `Sec-Fetch-Site` other than `same-origin`.<br>**Agente**: empty or exact Origin; rejects `chrome-extension://`. | Reusing `conGuardia` alone; exact Origin on GETs | `conGuardia` admits the extension prefix. Browsers send no Origin on same-origin GETs, so an exact-Origin check would break the UI's own reads; `Sec-Fetch-Site` blocks cross-site reads instead, like `esOrigenCruzado` in `archivos.go`. The MCP process sends no Origin, and a local process already owns the user's files. |
| D9 | Ticket and guard order | **Order:** (1) Host via `conGuardia`, (2) exact Origin, (3) consume the ticket, (4) `websocket.Accept`. A request rejected at (1) or (2) never touches the ticket store, so the ticket stays valid.<br>**Ticket:** 16 bytes from `crypto/rand`, base64url, stored in `map[ticket]{sesion, vence}`. TTL 30 s. Deleted at (3) **whatever the outcome**, and the session must match the path. Expired entries are swept on issue. A failure returns plain 403 before the upgrade. | A cookie | The ticket is one-time and session-bound, and only the exact origin can read it through POST. A same-origin GET carries no Origin, which is why the ticket endpoint is a POST. Checking Host and Origin first stops a rebinding page from burning the user's tickets. |
| D10 | Scene save | `PUT` with `{escena, version}` runs `UPDATE … SET version=version+1 WHERE version=?`; zero rows returns 409 with the current version. **On 409 the UI reloads the server scene** and shows "Actualizada desde otra ventana". Body limit 20 MiB (its own `MaxBytesReader`). `escena` is reduced to `{elements (no isDeleted), appState:{viewBackgroundColor}, files:{}}`; image elements are rejected with 400. | Local-wins retry | A stale second window must not overwrite the newer scene. At most ~1.5 s of local edits is lost. |
| D11 | Autosave | `onChange` checks whether `getSceneVersion` changed, then debounces 1.5 s before the PUT. **On a failed save, the scene stays in memory, the chip shows "Error al guardar", and the save is retried on the next change or flush trigger.** Flushes on tab switch, Revisar, unmount and `visibilitychange:hidden`. On `pagehide`, a `keepalive` fetch is used **only if the body is under 60 KiB** (the browser keepalive cap). | sendBeacon | The keepalive and beacon cap is 64 KiB, so `visibilitychange` carries the large-scene flush. |
| D12 | Ops delivery | The agent calls `POST ops`. Go validates (`pizarra.Validar`), **assigns missing element ids**, inserts a `pendiente` row, and waits up to 8 s on an in-memory channel. **No DB connection is held while waiting** (`SetMaxOpenConns(1)`). The UI receives the op through latido, applies the batch **atomically** (it is rejected if any target id is missing), then acks `aplicada` or `fallida`. A row still `pendiente` after 30 s becomes `vencida` (lazy sweep) and is never applied. **If the target session is not mounted** (no latido for it in 3 s), the call fails fast with 409 and the Spanish message from the UI strings table, and nothing is enqueued (ratified by the orchestrator; the spec has been updated to match). | Go applies to the stored scene; long-lived queue | The scene is UI-owned (confirmed). Fail-fast is honest, and server-assigned ids make a redelivery after a lost ack idempotent (merge by id). |
| D13 | Capture | Capture requests are in memory only (`map[id]chan []byte`) and surface through latido. The UI runs `exportToBlob({mimeType:'image/png', maxWidthOrHeight:1280})` and POSTs the raw PNG (at most 4 MiB, magic bytes checked). The server waits 15 s; the MCP client uses a 25 s timeout. The MCP process also writes `capturas/pizarra-<id>.png` (fixed name, overwritten) in the agent workspace, and the text part cites it. | A PNG per autosave | The file path is the fallback for CLIs that drop images (OpenCode is unverified). |
| D14 | Single instance | `CreateMutexW("Local\Bythos.Instancia")` right after the `mcp` branch. Inno uses `AppMutex=Bythos.Instancia` (the session namespace, the same as `Local\`). `/api/salud` adds `pid`; `esperarSalud` accepts only `pid == os.Getpid()`. | `Global\` | The port is machine-wide anyway, and the existing port-busy dialog covers another user's session. The pid check fixes the stale-200 race. |

### Second launch and stale-server detection

- **Mutex already exists:** probe `/api/salud`, retrying for 10 s.
  - 200 with the same `version` → `ventana.Abrir` and exit 0.
  - A different version → dialog: "Hay otra versión de Bythos abierta; cerrala".
  - No answer → dialog: "Bythos ya está abierto pero no responde" (exit 1).
- **Mutex taken, but :8080 belongs to an older build without the mutex:** the pid mismatch makes `esperarSalud` keep waiting until `errServidor` reports the port-busy error.

### ConPTY lifecycle (`conpty_windows.go`)

1. **Version check.** Require `windows.RtlGetVersion().BuildNumber >= 17763`; otherwise return `ErrNoSoportado`. The API answers 501 and the UI offers "Abrir agente" (external).
2. **Create the pseudoconsole.** Two `CreatePipe` pairs, then `CreatePseudoConsole(size, inR, outW, 0)`. Flags are 0: never `INHERIT_CURSOR`.
3. **Prepare the attributes.** Build the attribute list with `PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE`.
4. **Create the child suspended.** `CreateProcess` with `CREATE_SUSPENDED | EXTENDED_STARTUPINFO_PRESENT | CREATE_UNICODE_ENVIRONMENT` and `bInheritHandles=false`.
5. **Assign the job and resume.** `AssignProcessToJobObject`, then `ResumeThread`, then close the thread handle. **Close `inR` and `outW`.**
6. **Pipe goroutines.** The gestor reader drains `outR` until EOF. Writes to `inW` go through a mutex. A waiter goroutine on the process handle reports the exit code.
7. **Cerrar** is idempotent through `sync.Once`, in this order:
   1. `TerminateJobObject`.
   2. `ClosePseudoConsole` **while the reader is still draining**.
   3. Wait for the reader to reach EOF (2 s cap).
   4. Close `inW`, `outR`, the job and the process handles.

## Data Flow

Ticket, upgrade, attach and reattach:

```
UI(Terminal.jsx)        api/terminal.go            terminal.Gestor         ConPTY
 |-POST /sesiones/7/ticket (Origin exact)->|                                  |
 |<-------------- {ticket} (30 s) ---------|                                  |
 |-GET /sesiones/7/ws?ticket=T (Upgrade)-->| Host, Origin exact, consume T    |
 |                                         |-Accept(OriginPatterns)           |
 |                                         |-Adjuntar(7,cfg,cols,filas)------>| start if absent
 |                                         |<- snapshot(ring)+chan (atomic) --|
 |<===== binary: replay, then live ========|<========= reader goroutine ======|
 |== binary input / text {"tipo":"resize"} ===>| Escribir / Redimensionar --->|
 reload: socket closes -> Soltar (ultimoDetach=now) ... new ticket -> same path,
         replay of 256 KiB; prior attach (if any) closed with 4000
 child exits -> text {"tipo":"salida","codigo":N}; reaper: detached >10 min -> Cerrar
```

`pizarra_dibujar`:

```
Agent CLI   bythos mcp (agentes)      api/pizarra.go                  UI (latido 1 s)
 |-call---->| pizarra.Validar (early)  |                                 |
 |          |-POST /pizarra/ops ------>| resolve session; mounted? no -> 409, nothing enqueued
 |          |                          | Validar, assign ids, INSERT pendiente, wait<=8 s
 |          |                          |<-POST /latido {sesion_id} ------|
 |          |                          |-- {ops:[{id,op}],capturas:[]} ->| adapter.aplicar (atomic,
 |          |                          |                                 |  merge by id, captureUpdate
 |          |                          |<-POST /ops/{id}/ack {ok,ids} ---|  IMMEDIATELY -> user Ctrl+Z)
 |          |<-{estado:aplicada,ids} --| UPDATE estado; registrarEvento(pizarra_editada)
 |<-text----|   timeout -> {estado:pendiente} "se aplicará en segundos si la pizarra sigue abierta"
                                         autosave later persists the result
```

`pizarra_captura`:

```
Agent  agentes(25 s client)   api/pizarra.go              UI
 |----->|-POST /pizarra/captura->| mounted? no -> 409 immediately
 |      |                       | new chan id, wait<=15 s
 |      |                       |-- latido: {capturas:[id]} -->| exportToBlob(png, 1280)
 |      |                       |<-POST /capturas/{id} (PNG) --|
 |      |<-- image/png ---------| check magic, <=4 MiB
 |<-ImageContent + TextContent (+ file path); no structuredContent
```

Active session resolution is done server-side for every agent route. The MCP process sends the argument as `sesion_id` and the env hint as a separate `pista` field.

1. **`sesion_id` argument.** Authoritative: an unknown id returns 404 "La sesión N no existe" and never falls through.
2. **`pista` (`BYTHOS_SESION`).** Only a hint. If it is non-numeric or the session was deleted, it is ignored and resolution falls through.
3. **The last latido session** in this process lifetime, if it still exists.
4. **The most recently `actualizado` session.**
5. **Otherwise 404:** "No hay sesiones de Terminal".

Every response carries `resolucion` with one of `argumento`, `entorno`, `activa` or `reciente`. The spike uses it as evidence.

## Interfaces / Contracts

```go
type PTY interface { io.ReadWriter; Redimensionar(cols, filas uint16) error; Esperar() (uint32, error); Cerrar() error }
type Opciones struct { Iniciar func(Config) (PTY, error); Max int; Gracia time.Duration; Ahora func() time.Time }
func (g *Gestor) Adjuntar(id int64, cfg func() (Config, error), cols, filas uint16) (*Adjunto, error) // Repeticion []byte; Salida <-chan []byte; Fin <-chan uint32; Escribir; Redimensionar; Soltar
func (g *Gestor) Cerrar(id int64) error; func (g *Gestor) CerrarTodo()
```

**Schema (`db/sesiones.go`, `CREATE TABLE IF NOT EXISTS`):**

- **`terminal_sesiones`:** `id` PK AUTOINCREMENT, `nombre` TEXT NOT NULL, `modo` TEXT CHECK IN (`practica`, `revision`, `prototipo`, `dibujo`, `paginas`), `carpeta_id` NULL REFERENCES `folders(id)` ON DELETE SET NULL, `creado`, `actualizado` (localtime).
- **`terminal_pizarras`:** `sesion_id` PK REFERENCES `terminal_sesiones` ON DELETE CASCADE, `escena` TEXT, `version` INTEGER DEFAULT 0, `actualizado`.
- **`terminal_pizarra_ops`:** `id` PK, `sesion_id` FK CASCADE, `op` TEXT (JSON), `estado` CHECK IN (`pendiente`, `aplicada`, `fallida`, `vencida`), `resultado` TEXT, `creado`, `aplicado` NULL. Index on (`sesion_id`, `estado`).

**Routes, all under `/api/terminal`:**

| Guard | Routes |
|---|---|
| Ventana | `POST sesiones`, `PATCH sesiones/{id}` (`carpeta_id: null` loosens the session), `DELETE sesiones/{id}`, `PUT sesiones/{id}/pizarra`, `POST sesiones/{id}/ticket`, `GET sesiones/{id}/ws` (WS order in D9), `DELETE sesiones/{id}/pty`, `POST latido`, `POST pizarra/ops/{id}/ack`, `POST pizarra/capturas/{id}` |
| Ventana-lectura | `GET sesiones` (list without blobs), `GET sesiones/{id}/pizarra` (returns `{escena, version}`, the raw scene used on load and on reload after a 409), `GET entorno` (`{conpty, engram, sesiones}`) |
| Agente | `GET pizarra?sesion_id=&pista=`, `POST pizarra/ops`, `POST pizarra/captura` |

**Deleting a session.** `DELETE sesiones/{id}` runs only after the UI confirms. It calls `Gestor.Cerrar(id)` first, then deletes the row; the CASCADE clears the scene and ops. It logs `sesion_terminal_borrada`. Closing a tab only removes it from `localStorage` and deletes nothing.

**Limits (single source of truth, mirrored in the specs):**

| Limit | Value | Over the limit |
|---|---|---|
| Concurrent PTYs | 8 | 429 |
| WS read limit | 1 MiB | connection closed; PTY survives |
| Ring replay | 256 KiB per session | oldest bytes dropped |
| Scene body | 20 MiB | 413; stored scene unchanged |
| `pizarra_leer` output | 2,000 elements; 2,000 chars of text each | `truncado: true` |
| `pizarra_dibujar` ops per call | 200 | 400; nothing enqueued |
| Capture PNG | 4 MiB; 1280 px on the longest side | 413 |
| Detached PTY grace | 10 min | PTY reaped |
| Ticket TTL | 30 s | 403 |

**Frames:**

- Client to server: binary frames carry input bytes; text frames carry `{"tipo":"resize","cols","filas"}`, clamped to 2..500 × 1..300.
- Server to client: binary frames carry output; text frames carry `{"tipo":"salida"|"error",...}`.

**MCP tools (`agentes/pizarra.go`):**

- **`pizarra_leer`** is read-only and returns `{sesion:{id,nombre,modo,carpeta}, resolucion, version, actualizado, elementos:[{id,tipo,x,y,ancho,alto,texto?,color,fondo?,grosor?,puntos?(count only),desde?,hasta?,grupo?}], resumen:{total,por_tipo}, limites, ops_pendientes, truncado}`.
  - It returns at most 2,000 elements and at most 2,000 characters of text per element.
  - `pizarra.Resumir` is a pure function.
- **`pizarra_dibujar`** takes `{sesion_id?, ops:[…]}` with these verbs:
  - `{accion:"agregar", elemento:{id?, tipo: rectangle|ellipse|diamond|arrow|line|text|freedraw, x, y, ancho, alto, texto, color, fondo, grosor, puntos, desde, hasta}}`
  - `{accion:"actualizar", id, cambios:{…same fields}}`
  - `{accion:"borrar", ids}`
  - `{accion:"mover", ids, dx, dy}`
  - **Caps:**
    - At most 200 ops per call.
    - Text up to 10,000 characters.
    - Coordinates within ±100,000; sizes from 0 to 20,000.
    - `puntos` up to 2,000 per element.
    - Colors match `#rrggbb` or `transparent`.
    - Ids match `^[A-Za-z0-9_-]{1,64}$`.
    - No `image` elements.
  - Annotation: `DestructiveHint:true` (it can delete).
- **`pizarra_captura`** is registered through raw `Server.AddTool`, with an `InputSchema` built by `jsonschema.For`.
  - It returns `Content:[ImageContent{png}, TextContent{dimensions, session, file path}]` and **never sets `StructuredContent`**.
  - Typed `mcp.AddTool` would set `StructuredContent`, and Claude Code has been reported to drop images when it is present.
- **`Cliente`** gets a `TimeoutLargo` field (25 s) for capture; everything else keeps 15 s.

**Workspace (`workspace.go`):**

- **Rule 1** becomes: the library still has no delete tools. On the whiteboard the agent may add, edit, move and delete with `pizarra_dibujar` when the user asks. It looks at the board only when asked (Revisar).
- **The same sentence goes into the MCP `Instructions`.**
- **Env forwarding:**
  - `.mcp.json` adds `env:{"BYTHOS_SESION":"${BYTHOS_SESION:-}"}`.
  - `.codex/config.toml` adds `env_vars = ["BYTHOS_SESION"]`.
  - Gemini inherits the environment.
  - OpenCode is left to the spike.
- **Approval guidance** (in `LEEME.txt` and the UI banner):
  - Claude: approve the project server.
  - Codex: trust the folder.
  - Gemini: trust the folder.
  - OpenCode: no approval step.

## UI Strings (verbatim)

| Where | String |
|---|---|
| Nav item and badge | "Terminal", "NUEVO" |
| Dialog, empty name | "Escribe un nombre para la sesión." |
| Session with no folder | "Suelta" |
| Save chip | "Guardando…", "Guardado", "Error al guardar" |
| Image paste or drop blocked (shown by the guard) | "Las imágenes llegan en una fase posterior." |
| API 501 `ErrNoSoportado` | "La terminal requiere Windows 10 versión 1809 o superior." |
| Scene 409 reload | "Actualizada desde otra ventana" |
| `pizarra_dibujar` 409 | "La pizarra de esta sesión no está abierta: abre la pizarra en la pantalla Terminal de Bythos y vuelve a intentarlo." |
| `pizarra_captura` 409 | "La pantalla Terminal no está abierta: ábrela en Bythos para capturar la pizarra." |
| Unknown explicit session | "La sesión N no existe" |
| No sessions | "No hay sesiones de Terminal" |

## UI Architecture

- **`App.jsx` changes (~25 lines):**
  - A nav item `{id:'terminal', label:'Terminal', icon:'terminal', badge:'NUEVO'}` below Exportar, with badge rendering.
  - `const Terminal = lazy(() => import('./terminal/Terminal.jsx'))`.
  - When `view==='terminal'`, render `<Suspense><Terminal carpetas={carpetas}/></Suspense>` **instead of the `.content` div**, with no PageHeader, no Topbar and no drop handler.
- **Container `terminal/Terminal.jsx`:** owns sessions, open tabs (`localStorage`), the active tab, latido, autosave and Revisar. It sets `window.EXCALIDRAW_ASSET_PATH='/excalidraw/'` at module top, before the lazy Excalidraw import.
- **Presentational components:** `PestanasSesiones`, `DialogoNuevaSesion`, `ChipsEstado`, `PanelTerminal`, `Pizarra`.
- **`terminal/excalidrawAdapter.js`** is the only importer of `@excalidraw/excalidraw` 0.18.1, which is pinned exactly with no `^`. It wraps:
  - `convertToExcalidrawElements(..., {regenerateIds:false})`
  - `updateScene({elements, captureUpdate})`
  - `exportToBlob`
  - `getSceneVersion`
  - `sanearEscena`, which strips images and deleted elements.
- **Excalidraw props:**
  - `UIOptions={{tools:{image:false}, canvasActions:{loadScene:false, saveToActiveFile:false, export:false, saveAsImage:false}}}`
  - `aiEnabled={false}`, because Bythos ships no AI.
  - An `onPaste` guard that returns false when files are present.
  - A capture-phase `onDropCapture`/`onPasteCapture` wrapper that blocks `Files`.
- **`terminal/conexionPty.js`:** handles ticket, ws and reconnect (backoff 0.5 to 5 s), using `TextEncoder` for binary input.
- **xterm:** `@xterm/xterm` 6.0.0 with `@xterm/addon-fit` 0.11.0, driven by a ResizeObserver (100 ms debounce).
  - **Key handler** (`attachCustomKeyEventHandler`):
    - Ctrl+C with a selection, or Ctrl+Shift+C, writes to the clipboard and returns false.
    - Ctrl+V and Ctrl+Shift+V return false **without** `preventDefault`, so the native paste event reaches xterm once (bracketed).
    - Ctrl+R calls `preventDefault` and returns true.
  - **Window listener:** a capture-phase keydown listener blocks F5 and Ctrl+R while the view is mounted.
- **Fonts:**
  - **Copy script:** `ui/scripts/copiar-fuentes.mjs` uses Node built-ins only (`fs.cpSync` with a filter that excludes `Xiaolai`). It copies `node_modules/@excalidraw/excalidraw/dist/prod/fonts` to `public/excalidraw/fonts`.
  - **Wiring:** the `dev` and `build` scripts run it first, because pnpm skips `pre*` scripts. The output folder is git-ignored and not committed: it is reproducible from the lockfile and keeps binaries out of git and out of PR line counts.
  - **Content type:** Go registers `mime.AddExtensionType(".woff2","font/woff2")`.
- **`vite.config.js`:** `'/api': {target:'http://localhost:8080', ws:true}`.

## File Changes

| File | Action | PR |
|---|---|---|
| `desktop/main.go`, `instancia_windows.go`, `instancia_other.go`, `arranque.go` (+test) | Modify/Create | 0 |
| `installer/bythos.iss` (`AppMutex`), `api/server.go` (`salud` pid) | Modify | 0 |
| `desktop/terminal/pty.go`, `conpty_windows.go`, `conpty_other.go`, `conpty_windows_test.go` | Create | 1a |
| `desktop/terminal/anillo.go`, `gestor.go` (+tests, fake PTY) | Create | 1b |
| `desktop/db/sesiones.go`, `db.go` (`crearTablaSesiones`), `api/sesiones.go` (+tests); `db/eventos.go`: `AccionSesionTerminalCreada="sesion_terminal_creada"`, `AccionSesionTerminalBorrada="sesion_terminal_borrada"` | Create/Modify | 2 |
| `desktop/api/terminal.go` (+guard matrix tests), `go.mod` | Create/Modify | 3 |
| `ui/src/App.jsx`, `terminal/Terminal.jsx`, `PestanasSesiones`, `DialogoNuevaSesion`, `PanelTerminal`, `conexionPty.js`, `api.js`, `vite.config.js`, `package.json`, `styles.css` | Modify/Create | 4 |
| `ui/src/terminal/Pizarra.jsx`, `excalidrawAdapter.js`, `ChipsEstado.jsx`, `scripts/copiar-fuentes.mjs`, `.gitignore`, `api/ui.go` (mime) | Create/Modify | 5 |
| `desktop/pizarra/*`, `db/pizarra_ops.go`, `api/pizarra.go`, `agentes/pizarra.go`, `cliente.go`, `servidor.go` (+tests); `db/eventos.go`: `AccionPizarraEditada="pizarra_editada"` | Create/Modify | 6a |
| `Terminal.jsx` (latido, apply, capture), `workspace/workspace.go` (+test) | Modify | 6b |

## Testing Strategy

| PR | Go tests (table-driven, `t.TempDir`, fakes) |
|---|---|
| 0 | `esperarSalud` rejects a foreign pid and accepts its own; mutex acquire/release (Windows) |
| 1a | Real ConPTY: `cmd.exe /c echo hola` emits the output and an exit code, and `Cerrar` after start leaves no process.<br>**Process death:** the test binary re-executes itself as a helper host (`TestMain` with an env flag). The helper opens a PTY running `cmd /c ping -n 60 127.0.0.1` and reports the grandchild PID. The test kills the helper with `TerminateProcess` and asserts that the grandchild is gone within 5 s, which proves `KILL_ON_JOB_CLOSE`.<br>Both tests are skipped with `-short` and on non-Windows. |
| 1b | Fake PTY: replay equals the last 256 KiB; atomic snapshot (no gap or duplicate); a new attach replaces the old one; a slow client is dropped without blocking the reader; the cap of 8 returns an error; reaper with a fake clock; `Cerrar` is idempotent; resize is forwarded |
| 2 | CRUD; mode CHECK; folder delete sets `carpeta_id` NULL; `DELETE sesiones/{id}` cascades scene and ops; version conflict returns 409; 21 MiB returns 413; images rejected; scene reduction; the `GET sesiones/{id}/pizarra` shape; Ventana-lectura rejects an extension Origin and `Sec-Fetch-Site: cross-site` |
| 3 | Guard matrix (see the threat matrix); round trip; resize; dev `:5173` only with `BYTHOS_DEV=1`. `DELETE sesiones/{id}` with a fake live PTY: the PTY is closed and absent from `Vivas()`. |
| 6a | `pizarra.Validar` caps per field; `Resumir` truncation; the ops lifecycle (ack, timeout to pendiente, vencida, not mounted returns 409, redelivery idempotent); capture 409, timeout and magic check; resolution order, including an explicit unknown id → 404 "La sesión N no existe" and a stale `pista` falling through to the active session; 201 ops → 400 with nothing enqueued; the tool result has `StructuredContent == nil` and contains Image plus Text; ack timeout through httptest |
| 6b | `workspace_test`: AGENTS.md contains the pizarra rule and still keeps the library no-delete rule; `.mcp.json` env and Codex `env_vars` |

The UI has no test runner; check it with `pnpm build` plus a manual checklist. That checklist includes zero non-localhost requests in the DevTools Network panel, and pastes or drops of images being refused.

**Per-CLI spike (between 6a and 6b, on Windows):**

1. In an embedded terminal of session S, run `claude`, `opencode`, `gemini` and `codex` in turn.
2. For each one, record:
   - whether an approval or trust prompt appears;
   - `pizarra_leer` → `resolucion` (shows whether `entorno` reached the server);
   - `pizarra_captura`: does the model describe the drawing? If not, can it read the cited file?
3. Record the results in `tasks.md`; 6b adjusts the configs and the text.

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | N/A: no file classification or execution of repository files | — | — |
| Git repository selection, commit state, push state, PR commands | N/A: no VCS or PR automation | — | — |
| Process integration: WS upgrade (design-specific) | Applicable | D7, D8, D9 (order: Host, exact Origin, ticket, Accept) | Origin wrong, empty, `chrome-extension://`, or `http://localhost:8080.evil.com` → 403.<br>**Pre-check gates `Accept`:** an empty Origin with a valid ticket → 403 from the pre-check, `Accept` is never called (spy seam), and the ticket is not consumed.<br>**Bad Host leaves the ticket valid:** a bad Host with a valid ticket → 403, then a correct request with the same ticket succeeds.<br>Ticket missing, expired, reused, or bound to another session → 403.<br>`:5173` without dev → 403. |
| Process integration: PTY spawn | Applicable | D2: fixed program, cwd computed in Go, numeric `BYTHOS_SESION` | The `Config` that `api` builds ignores query and body. A ninth PTY → 429. |
| Process integration: child lifetime | Applicable | D3, D4 | Real test: no child after `Cerrar`. Helper host killed with `TerminateProcess` → grandchild gone. Reaper with a fake clock. `DELETE sesiones/{id}` → PTY gone. |
| Agent routes | Applicable | D8 Agente | An extension origin on ops or captura → 403; a foreign origin on ack or upload → 403 |

## Migration / Rollout

The tables are additive (`IF NOT EXISTS`). Delivery is a Feature Branch Chain of **PR0, 1a, 1b, 2, 3, 4, 5, 6a, 6b**. Two adjustments to the proposal:

- **PR1 is split into 1a and 1b.** ConPTY plus the gestor estimates at ~700 lines.
- **DB moves ahead of WS (PR2 ↔ PR3).** The WS handler validates the session through `db`.

The lockfile and generated fonts stay excluded from the line budget. Rollback is reverse order. If CI fails to build Excalidraw on Node 18, PR5 bumps `setup-node` to 20.

## Open Questions

- [ ] Whether `aiEnabled`, the `onPaste` file data and `regenerateIds:false` behave as expected in 0.18.1. Verify in PR5 and PR6b; the adapter isolates any fix.
- [ ] OpenCode env and image behavior. The spike decides.
- [ ] Whether the 10 min reaper grace is acceptable to the user.
