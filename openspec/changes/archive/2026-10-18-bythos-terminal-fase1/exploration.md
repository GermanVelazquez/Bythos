# Exploration: bythos-terminal-fase1

Phase 1 of the Terminal spec (`Bythos — Terminal decisiones.pdf`, 2026-10-16): a new
Terminal screen with an Excalidraw whiteboard and an embedded terminal, persisted
sessions and scenes, and the `pizarra_leer`, `pizarra_captura` and `pizarra_dibujar`
MCP tools. Bythos ships no AI: each user runs their own CLI agent (claude, opencode,
gemini, codex or a local one) inside the embedded terminal.

## Spec details from the PDF

- **Placement:** a new menu item, Terminal, sits below Exportar and carries a "NUEVO"
  badge. The whiteboard is in the center and the terminal in a right panel.
- **Top bar:** session tabs, each with a mode label (Práctica, Revisión, Prototipo,
  Dibujo, Páginas), and a `+` that opens a new session.
- **Status chips:** `Agente: <cli | sin iniciar>`, `Memoria: Engram` and
  `Corrige según: <folder>`.
- **New-session dialog:**
  - Fields: name, folder ("Corregir según la carpeta") and mode ("Práctica paso a paso").
  - Three hint steps: type the CLI command; paste or drop content; ask for a review or tap
    Revisar.
  - Buttons: "Abrir sesión de ejemplo" and "Crear sesión".
  - Chips: "Engram detectado" and "Sesiones anteriores: N".
- **Terminal header:** `Terminal %APPDATA%\Bythos\agente` with an agent badge.
- **Terminal banner:** lists what Bythos prepared: AGENTS.md / CLAUDE.md, .mcp.json,
  opencode.json, .gemini, .codex, and the MCP servers bythos and engram.
- **Footer:** "Bythos nunca escribe el comando por vos; el agente lo elegís y lo abrís
  vos."
- **Phase 1 toolset:** free draw, colors, stroke width, eraser, shapes and text. The
  highlighter, formula tool, pen pressure and live page nodes belong to phase 4, and
  paste/attach to phase 2.
- **Resolved decisions:**
  - The embedded terminal is the default, and the external "Abrir agente" stays.
  - The agent looks only when the user taps Revisar.
  - The agent sees the whole whiteboard.
  - The agent has full whiteboard permissions; undo history comes in phase 3.
  - The agent still cannot delete anything in the library.
  - Sessions can be loose or inside a folder.
  - Windows only, local-first.

## Current state (verified in code)

- **App shell:** `desktop/` is one Go binary. `main.go` serves the API plus the embedded
  `ui/dist` with `http.ListenAndServe("localhost:8080", ...)` and no timeouts. It opens
  an Edge/Chrome `--app=` window (not WebView2) with the profile in
  `%APPDATA%\Bythos\Ventana`. The process outlives the window.
- **Guard (`api/server.go` `conGuardia`):**
  - The Host must be `localhost:8080` or `127.0.0.1:8080`.
  - The Origin must be exact `http://localhost:8080`, OR any `chrome-extension://`
    prefix (line 460), OR `:5173` under `BYTHOS_DEV=1`.
  - A request with no Origin passes.
  - Responses carry `X-Frame-Options: DENY` and `frame-ancestors 'none'`.
- **Strict-origin pattern (`api/agente.go` `origenesAgentePermitidos`):** exact Origin
  only; it rejects an empty Origin and the extension prefix. It is used by
  `POST /api/agente/terminal` and `/api/receptor/*` (`conOrigen`). Test seams:
  `lanzadorAgente`, `prepararWorkspaceAgente` and `exeActual`.
- **Abrir agente:** `workspace.PrepararWorkspace(exe)` writes AGENTS.md, CLAUDE.md,
  LEEME.txt, `.mcp.json`, `opencode.json`, `.gemini/settings.json` and
  `.codex/config.toml` in `%APPDATA%\Bythos\agente`. All of them point the MCP server at
  `<exe> mcp`. `terminal.Lanzador` opens `wt.exe -d dir`, or falls back to `cmd.exe`.
  `terminal/` already uses the `_windows.go` / `_other.go` split.
- **MCP (`desktop/agentes/`):**
  - `bythos.exe mcp` is a separate stdio process on go-sdk v1.8.0.
  - It never opens the DB; it calls the running app over HTTP through `agentes.Cliente`
    with a 15 s timeout.
  - Tools are split into `lectura.go` and `escritura.go`, and none of them deletes.
  - `mcp.ImageContent` is available.
- **UI:**
  - React 18.3 and Vite 5, with `react` and `react-dom` as the only dependencies.
  - `App.jsx` is ~2,700 lines and one component owns all state. The view is chosen by a
    `useState` plus a render ternary.
  - The sidebar `Acciones` group lists export, historial, recibir, agente-terminal and
    settings.
  - `api.js` is the only fetch module.
  - Polling is the live-update idiom (1.5 to 2.5 s).
  - Styling is one `styles.css`.
  - The `.content` wrapper has a drag-and-drop handler that opens the upload modal.
- **DB:** `crearTablas` runs `CREATE TABLE IF NOT EXISTS` plus guarded `ALTER`s, with
  one file per table. `leerJSON` caps bodies at 1 MiB.
- **Dependencies:** there is no WebSocket or PTY library. `golang.org/x/sys v0.47.0` is
  already direct and exposes `CreatePseudoConsole`, `ResizePseudoConsole` and the
  proc-thread attribute APIs.
- **Tests:** `go test ./...` in `desktop/` uses table-driven httptest with injected
  fakes. CI runs `go vet` and `go test` on windows-latest. There are no UI tests.
- **Related defects:**
  - There is no single-instance guard. A stale instance on :8080 serves an old UI, and
    `esperarSalud` can accept the stale 200.
  - `installer/bythos.iss` has no AppMutex.

## Approaches and recommendations

| Fork | Options | Recommendation |
|---|---|---|
| ConPTY | A. hand-rolled on `x/sys/windows`; B. `UserExistsError/conpty`; C. `aymanbagabas/go-pty` | **A**, about 200 lines behind a `PTY` interface (Read/Write/Resize/Close/Wait). A Job Object with `KILL_ON_JOB_CLOSE` kills children together with `bythos.exe`. No new dependency. |
| WebSocket | `coder/websocket`, gorilla, or hand-rolled | **`github.com/coder/websocket`** (ISC). Binary frames carry PTY data and JSON text frames carry control (resize). |
| Scene ownership | A. UI-owned plus debounced autosave and an agent ops queue; B. Go-authoritative; C. live-only | **A**. Excalidraw logic stays in JS. `pizarra_leer` reads the last saved scene even with the UI closed. |
| Capture | A. UI renders on demand (`exportToBlob`) and POSTs a PNG; B. a PNG on every autosave; C. Go renders | **A**. It returns a clear error when the view is not mounted. The PNG goes back as `mcp.ImageContent` plus text. |
| Tool addressing | env var, per-session workspace, or active session | **Active session.** Tools take an optional `sesion_id` and default to the active session, which the UI reports. `BYTHOS_SESION` is also set in the PTY env as a hint. |

Notes on scene ownership:
- `pizarra_dibujar` takes simplified skeleton shapes plus add/update/delete/move verbs.
- Go validates the op, caps sizes and enqueues it.
- The UI polls, applies the op with `convertToExcalidrawElements` + `updateScene`
  (merging by id), and acks.
- The tool waits about 8 s for the ack.

## Terminal and WebSocket security

- Reuse the loopback listener; do not add a second one.
- The upgrade requires the exact Origin (`origenesAgentePermitidos`; `:5173` only in dev)
  and the Host check.
- The upgrade also requires a one-time ticket: 128-bit, ~30 s TTL, bound to the session,
  and consumed on upgrade.
- The ticket endpoint must use the strict Origin check, because `conGuardia` would
  otherwise allow extension origins to read it.
- Limits: about 8 concurrent PTYs, a 64 KiB read limit, and kill on session close and on
  app exit.
- The command is fixed: the user's shell, with cwd set to the agent workspace. Nothing
  from HTTP is interpolated, and the AI CLI is never auto-run.

## Window and xterm

- **Clipboard:** `http://localhost` is a secure context, so the clipboard works. Ctrl+C
  copies when there is a selection and otherwise sends `^C`. Ctrl+Shift+V pastes.
- **Shortcuts:** Ctrl+W, Ctrl+T and Ctrl+N cannot be blocked, and Ctrl+W closes the app
  window; this is a known limitation. Ctrl+R and F5 are prevented.
- **Reload:** a reload must not kill the PTY. The manager keeps PTYs independent of
  connections and replays a ~256 KiB ring buffer on re-attach.
- **Fit:** `@xterm/addon-fit` plus a ResizeObserver drives `ResizePseudoConsole`.
- **Drops:** render the screen outside the `.content` drop wrapper, or stop drag
  propagation.

## UI and bundle

- Excalidraw 0.18.x is MIT and peers with React 17 to 19.
  - It grows the embedded UI from tiny to several MB. Measure it in the PR.
  - It loads fonts from a CDN by default, so self-hosting the fonts (`ui/public` +
    `window.EXCALIDRAW_ASSET_PATH`) is mandatory for local-first.
  - Pin the version and load it with `React.lazy`.
  - Disable the image tool until phase 2.
- `@xterm/xterm` plus the fit addon add about 300 KB.
- `Terminal.jsx` is the container (tabs and state) with presentational children, so
  `App.jsx` gets only a nav item, a title and a lazy branch.
- `vite.config.js` needs `ws: true` on the `/api` proxy.

## DB

- `terminal_sesiones(id, nombre, modo, carpeta_id NULL REFERENCES folders ON DELETE SET
  NULL, creado, actualizado)`. A loose session has a NULL folder; saving it into a folder
  is an UPDATE.
- `terminal_pizarras(sesion_id PK FK CASCADE, escena, version, actualizado)`. It is
  separate so list queries never load blobs.
- `terminal_pizarra_ops(id, sesion_id FK CASCADE, op, estado, creado)` is the agent
  queue.
- **Autosave:**
  - Debounce ~1.5 s, skip unchanged `getSceneVersion`, and use an optimistic `version`.
  - Flush on visibilitychange, pagehide and tab switch.
  - Prune `isDeleted` elements.
  - The scene route gets its own ~20 MiB limit.
- Log agent edits with `registrarEvento`, but not every autosave.

## Testing

- **Plain `go test`:**
  - PTY manager on a fake: ring buffer, reattach, cap, resize, idempotent close.
  - WebSocket guard matrix via httptest: wrong, empty or extension Origin; missing,
    expired or reused ticket; wrong Host; round trip; resize; dev-only `:5173`.
  - DB: CRUD, cascade, folder delete loosens sessions, version conflict.
  - API: 413 on oversize scenes and the ops lifecycle.
  - MCP tools: ack timeout and image content shape.
  - `workspace_test` checks the pizarra tools in AGENTS.md.
- **Real ConPTY:** one integration test (`cmd.exe /c echo`), skipped under `-short`.
- **UI:** there are no UI tests, so keep JS thin and put validation in Go.

## Proposed slicing (400-line budget, excluding the lockfile and font binaries)

| PR | Scope | Est. lines |
|---|---|---|
| PR0 (optional) | Single-instance mutex in `main.go` + `AppMutex` in `bythos.iss` | ~60 |
| PR1 | ConPTY, Job Object, session manager, ring buffer, fakes and tests | ~350 |
| PR2 | WebSocket endpoint, ticket, exact-origin guard, `coder/websocket`, tests | ~350 |
| PR3 | DB and API: sessions, scenes, body limit, folder link, tests | ~380 |
| PR4 | UI: nav item, lazy `Terminal.jsx`, tabs, xterm over WS, proxy `ws:true` | ~400 |
| PR5 | UI: Excalidraw (lazy), self-hosted fonts, autosave, split layout, status chips | ~350 |
| PR6 | MCP `pizarra_*`, ops endpoints, UI poller/apply/capture, active session, AGENTS.md | ~400 (may split 6a/6b) |

Total: about 2,200 to 2,600 changed lines (~1,300 Go including tests, ~900 JS/CSS).

## Risks

- **ConPTY lifecycle:** `ClosePseudoConsole` can deadlock if output is not drained, and
  children or handles leak without the Job Object.
- **Shell WebSocket:** an Origin or ticket flaw would give any page a shell. The guard
  tests are the top priority.
- **Excalidraw:** bundle size, CDN fonts, API churn (pin the version), and Node 18 CI
  compatibility is unverified.
- **MCP:** env propagation to MCP stdio servers is unverified per CLI; validate the
  active-session fallback manually with each CLI. Image-result support in Gemini and
  Codex is unverified.
- **Races:** user strokes and agent ops can collide. Mitigate by merging by element id.
- **Keyboard:** Ctrl+W closes the window.
- **Delete rule:** the "no delete tools" text in AGENTS.md and the MCP instructions must
  change consistently (whiteboard: allowed; library: still forbidden).
- **Layout:** the `.content` wrapper (PageHeader and drop handler) interferes with a
  full-height split layout.
- **Process lifetime:** the exe outlives the window, so PTYs keep running. Decide whether
  closing the window ends sessions.
- **Builds:** Windows-only code must keep non-Windows builds and `go vet` clean.

## Decisions to confirm before the proposal

1. Hand-rolled ConPTY on `x/sys/windows`.
2. `coder/websocket` as the only new Go dependency.
3. A UI-owned scene plus an agent ops queue.
4. The capture rendered by the UI on demand.
5. Active-session addressing for the MCP tools.
6. Image insertion disabled until phase 2.
7. Folder deletion loosens sessions instead of deleting them.
8. The lockfile and font binaries excluded from the 400-line budget.
9. The single-instance mutex shipped as PR0.
