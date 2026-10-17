# Tasks: Terminal Phase 1 (bythos-terminal-fase1)

Paths: `D` = `desktop`, `UI` = `desktop/ui`, `S` = `UI/src`, `T` = `S/terminal`. Specs: `ET` = embedded-terminal, `TS` = terminal-sessions, `WB` = whiteboard, `WT` = whiteboard-agent-tools, `SI` = single-instance. Design refs (D1..D14, Limits table, UI Strings table) point to `design.md`, which is authoritative.

Verification commands per PR (all must be green before opening the PR):

- Go PRs: from `desktop/`: `go vet ./...` and `go test ./...`.
- Real ConPTY (PR1a, PR1b, PR3, PR6a, run at least once locally on Windows, never with `-short`): from `desktop/`: `go test ./terminal/... -count=1 -run "ConPTY|Job"` (no `-short`).
- UI PRs (4, 5, 6b): from `desktop/ui`: `pnpm build`.
- Strings that appear in code or tests are copied verbatim from the design "UI Strings" table.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | PR0 ~230; PR1a ~380; PR1b ~420; PR2 ~400; PR3 ~420; PR4 ~430; PR5 ~380; PR6a ~900; PR6b ~220. Total ~3,800 text lines. Excluded from the count: `pnpm-lock.yaml` and copied font binaries (the fonts are not committed at all, see D tasks in PR5). |
| 400-line budget risk | PR0 Low; PR1a Medium; PR1b Medium-High; PR2 Medium-High; PR3 Medium-High; PR4 High; PR5 Medium; PR6a **High**; PR6b Low |
| Chained PRs recommended | Yes |
| Suggested split | PR0, 1a, 1b, 2, 3, 4, 5, 6a, 6b (design Migration). Optional further split of 6a into 6a-i (pizarra pkg + db + api) and 6a-ii (agentes MCP + cliente) |
| Delivery strategy | auto-chain |
| Chain strategy | stacked-to-main (repo precedent: Android PRs #11..#16 stacked to master) |

Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High (PR6a; PR4, PR1b, PR2, PR3 Medium-High)

Decision needed before apply: Yes. (1) PR6a is forecast at ~900 lines, more than double the budget. Recommendation: split it into 6a-i and 6a-ii at the boundary marked in the PR6a section (branches `feat/bythos-terminal-fase1-08a-pizarra-api` and `feat/bythos-terminal-fase1-08b-pizarra-mcp`); the alternative is a `size:exception` from the user. (2) PR4 (~430) and PR1b/PR2/PR3 (~400-420) are at the edge: apply measures the real diff after each PR and the user is asked for `size:exception` only if it ends above 400 with no cohesive cut left (one honest slicing pass, per chained-pr).

### Suggested Work Units

| Unit | Goal | PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|----|----------------------|-----------------|-------------------|
| A | Single instance, pid-checked health, installer mutex | PR0 | `go test ./ -run "Instancia|Arranque|Salud"` | Windows: launch twice | `main.go`, `instancia_*.go`, `arranque.go`, `bythos.iss`, `api/server.go` |
| B | ConPTY + Job Object | PR1a | `go test ./terminal -run "ConPTY|Job" -count=1` | Real `cmd.exe` | `terminal/pty.go`, `conpty_*.go` |
| C | Ring buffer + session manager | PR1b | `go test ./terminal` | Fake PTY | `terminal/anillo.go`, `gestor.go` |
| D | Sessions DB + CRUD + scene API | PR2 | `go test ./db ./api -run "Sesion|Pizarra"` | httptest | `db/sesiones.go`, `api/sesiones.go` |
| E | Ticket + WS + guards | PR3 | `go test ./api -run "Terminal|Guardia"` | httptest + real WS | `api/terminal.go` |
| F | Terminal screen + xterm | PR4 | `pnpm build` | Real app | `S/terminal/*`, `App.jsx` |
| G | Whiteboard + autosave + fonts | PR5 | `pnpm build` | Real app, DevTools Network | `Pizarra.jsx`, adapter, font script |
| H | Pizarra domain, ops, routes | PR6a | `go test ./pizarra ./db ./api ./agentes` | httptest | `pizarra/`, `db/pizarra_ops.go`, `api/pizarra.go`, `agentes/pizarra.go` |
| I | Per-CLI spike | between 6a and 6b | manual | Real app + 4 CLIs | none (records results) |
| J | UI latido/apply/capture + workspace text | PR6b | `go test ./workspace` + `pnpm build` | Real app + agent | `Terminal.jsx`, `workspace.go` |

## PR0: Single instance (branch `feat/bythos-terminal-fase1-00-instancia`, base `master`)

Satisfies: SI Single Server, Stale state, Installer Detection (design D14).

- [x] 0.1 RED: create `D/arranque_test.go` with table tests: `esperarSalud` rejects a response whose `pid` differs from `os.Getpid()` and accepts its own; second-launch probe returns "mismo", "otra versión" and "sin respuesta" outcomes (fake HTTP). (SI: Second launch, Stale state)
- [x] 0.2 Modify `D/api/server.go`: `/api/salud` response adds `pid`. Add or extend a test in `D/api/` asserting `pid` is present. (SI: Second launch)
- [x] 0.3 Modify `D/arranque.go`: `esperarSalud` accepts only `pid == os.Getpid()`; add the second-launch probe (retry 10 s): same `version` -> `ventana.Abrir` and exit 0; different version -> dialog "Hay otra versión de Bythos abierta; cerrala"; no answer -> dialog "Bythos ya está abierto pero no responde" (exit 1). (SI: Second launch)
- [x] 0.4 Create `D/instancia_windows.go` (`CreateMutexW("Local\Bythos.Instancia")`, release on exit) and `D/instancia_other.go` (no-op acquire). Add `D/instancia_windows_test.go`: first acquire succeeds, second reports existing, release allows re-acquire; skipped on non-Windows. (SI: Second launch, Stale state)
- [x] 0.5 Modify `D/main.go`: acquire the mutex right after the `mcp` branch; on "already exists" run the second-launch flow. Do not touch the `mcp` path. (SI: Second launch)
- [x] 0.6 Modify `D/installer/bythos.iss` (path as in repo): add `AppMutex=Bythos.Instancia`. (SI: Upgrade while running, Uninstall while running)
- [x] 0.7 Verify: `go vet ./...` and `go test ./...` from `desktop/`.
- [x] 0.8 Open PR0 (base `master`) with the chain diagram and manual items M.8 and M.9 in the description.

## PR1a: ConPTY and Job Object (branch `feat/bythos-terminal-fase1-01a-conpty`, base `feat/bythos-terminal-fase1-00-instancia`)

Satisfies: ET Process Cleanup, Platform Guard (design D1, D3, ConPTY lifecycle).

- [x] 1a.1 Create `D/terminal/pty.go`: `PTY` interface, `Config`, `ErrNoSoportado`. No redeclaration of `Lanzador`, `NuevoLanzador`, `lanzadorSistema`, `createNewConsole`, `sysProcAttrConsola` (check against `terminal.go`, `terminal_windows.go`, `terminal_other.go`). (ET: Platform Guard)
- [x] 1a.2 RED: create `D/terminal/conpty_windows_test.go` (skipped with `-short` and on non-Windows): (a) `cmd.exe /c echo hola` emits the output and an exit code; (b) `Cerrar` after start leaves no process. (ET: Process Cleanup)
- [x] 1a.3 RED: in the same test file add the helper-host test: `TestMain` with an env flag re-executes the binary as a host that opens a PTY running `cmd /c ping -n 60 127.0.0.1` and reports the grandchild PID; the test kills the host with `TerminateProcess` and asserts the grandchild is gone within 5 s (proves `KILL_ON_JOB_CLOSE`). (ET: App exit)
- [x] 1a.4 Create `D/terminal/conpty_windows.go` following the design lifecycle: build >= 17763 check via `RtlGetVersion` else `ErrNoSoportado`; two `CreatePipe` pairs; `CreatePseudoConsole` flags 0; attribute list; `CreateProcess` with `CREATE_SUSPENDED | EXTENDED_STARTUPINFO_PRESENT | CREATE_UNICODE_ENVIRONMENT`, `bInheritHandles=false`; per-PTY Job with `KILL_ON_JOB_CLOSE`; assign, resume, close thread handle, close `inR` and `outW`; write mutex; waiter goroutine for the exit code; `Cerrar` via `sync.Once` (TerminateJobObject, ClosePseudoConsole while draining, reader EOF with 2 s cap, close handles). Add `golang.org/x/sys/windows` to `go.mod` only if not already present. (ET: Process Cleanup, Platform Guard)
- [x] 1a.5 Create `D/terminal/conpty_other.go` (build tag `!windows`): the constructor returns `ErrNoSoportado`. (ET: Platform Guard)
- [x] 1a.6 Add a unit test for the build-number gate (injectable version func) returning `ErrNoSoportado` below 17763. (ET: Old Windows)
- [x] 1a.7 Verify: `go vet ./...`, `go test ./...` from `desktop/`, and the real ConPTY run without `-short` (see header).
- [x] 1a.8 Open PR1a (base PR0 branch). Opened as #18 with size:exception (~750 lines) accepted by the maintainer.

## PR1b: Ring buffer and session manager (branch `feat/bythos-terminal-fase1-01b-gestor`, base `feat/bythos-terminal-fase1-01a-conpty`)

Satisfies: ET Reattach and Replay, Fixed Command and Limits (PTY cap), Process Cleanup (design D4, D5, D6).

- [x] 1b.1 RED: create `D/terminal/anillo_test.go`: replay equals the last 256 KiB after overflow; overwrite-oldest; small writes below capacity return everything. (ET: Reattach and Replay)
- [x] 1b.2 Create `D/terminal/anillo.go` (256 KiB, overwrite-oldest, snapshot copy). (ET: Reattach and Replay)
- [x] 1b.3 RED: create `D/terminal/gestor_test.go` with a fake `PTY` and fake clock: atomic snapshot+subscribe (no gap, no duplicate under concurrent output); a new attach replaces the old one (old gets closed with code 4000 signal); slow client (256 chunks full) is dropped and the reader never blocks; the 9th PTY returns a cap error and starts nothing; reaper closes a PTY detached >10 min and keeps one detached <10 min; `Cerrar` idempotent; resize forwarded; `Fin` channel carries the exit code; `Vivas()` lists live ids. (ET: PTY cap, Reload; TS: Delete)
- [x] 1b.4 Create `D/terminal/gestor.go`: `Opciones{Iniciar, Max, Gracia, Ahora}`, `Adjuntar`, `Adjunto{Repeticion, Salida, Fin, Escribir, Redimensionar, Soltar}`, `Cerrar`, `CerrarTodo`, `Vivas`; reaper goroutine; constants from the Limits table (Max 8, Gracia 10 min). (ET: Reattach and Replay, PTY cap)
- [x] 1b.5 Verify: `go vet ./...`, `go test ./...` from `desktop/`; run `go test ./terminal -race` once if the toolchain allows.
- [x] 1b.6 Open PR1b (base PR1a branch). Opened as #19 with size:exception (~665 lines); budget raised to 800 for the rest of the chain.

## PR2: Sessions DB and scene API (branch `feat/bythos-terminal-fase1-02-sesiones`, base `feat/bythos-terminal-fase1-01b-gestor`)

Satisfies: TS Create Session, Folder Link, Close and Delete Session; WB Autosave (server side), Scene Limits (design D8, D10, schema, routes).

- [x] 2.1 RED: create `D/db/sesiones_test.go` (`t.TempDir`): CRUD; mode CHECK rejects an unknown mode; empty-name rejected; deleting a folder sets `carpeta_id` NULL and keeps session, scene and ops; deleting a session cascades scene and ops; `version` conflict (`UPDATE ... WHERE version=?` with zero rows) reports conflict with the current version. (TS: Create, Folder deleted, Delete)
- [x] 2.2 Create `D/db/sesiones.go` and modify `D/db/db.go` (`crearTablaSesiones` with `CREATE TABLE IF NOT EXISTS` for `terminal_sesiones` and `terminal_pizarras`; the ops table stays for PR6a). Check that foreign keys are enabled on the connection. (TS: Create Session, Folder Link)
- [x] 2.3 Modify `D/db/eventos.go`: `AccionSesionTerminalCreada="sesion_terminal_creada"`, `AccionSesionTerminalBorrada="sesion_terminal_borrada"`. (TS: Close and Delete Session)
- [x] 2.4 RED: create `D/api/sesiones_test.go`: list without blobs; create/patch (`carpeta_id: null` loosens)/delete; scene PUT 409 with current version; 21 MiB body returns 413 and the stored scene is unchanged; image elements rejected with 400; scene reduction (`elements` without `isDeleted`, `appState:{viewBackgroundColor}`, `files:{}`); `GET sesiones/{id}/pizarra` returns `{escena, version}`; Ventana-lectura rejects an extension Origin and `Sec-Fetch-Site: cross-site`; Ventana rejects empty and extension Origin on mutations. (TS: Create, Empty name; WB: Oversized)
- [x] 2.5 Create `D/api/sesiones.go`: routes `GET/POST sesiones`, `PATCH/DELETE sesiones/{id}`, `GET/PUT sesiones/{id}/pizarra`, `GET entorno` (`{conpty, engram, sesiones}`), each under `conGuardia` plus the Ventana or Ventana-lectura guard from D8 (implement the two guards here as shared helpers, reused by PR3 and PR6a). Scene PUT has its own 20 MiB `MaxBytesReader`. `DELETE sesiones/{id}` calls `Gestor.Cerrar(id)` before deleting the row and logs `sesion_terminal_borrada` (use a nil-safe gestor seam so PR2 compiles before PR3 wires the real one). Wire routes in `D/api/server.go`. (TS: Delete; WB: Oversized)
- [x] 2.6 Verify: `go vet ./...`, `go test ./...` from `desktop/`.
- [x] 2.7 Open PR2 (base PR1b branch). Opened as #20 with size:exception (~880 lines).

## PR3: Ticket and WebSocket (branch `feat/bythos-terminal-fase1-03-ws`, base `feat/bythos-terminal-fase1-02-sesiones`)

Satisfies: ET Origin Check, Host Check, One-Time Ticket, Fixed Command and Limits, Reattach and Replay (design D2, D7, D9, Threat Matrix). **The guard test matrix is written first (RED) before any handler code.**

- [x] 3.1 RED (first): create `D/api/terminal_test.go` guard matrix, with a spy seam on `websocket.Accept` and a fake `Gestor`:
  - wrong Origin, empty Origin, `chrome-extension://abc`, `http://localhost:8080.evil.com` and a prefix variant -> 403 at the ticket endpoint and at the upgrade; no PTY created. (ET: Empty or extension origin, Wrong origin, Ticket endpoint under Origin check)
  - empty Origin with a valid ticket -> 403 from the pre-check, `Accept` never called, ticket not consumed. (ET: Origin Check)
  - bad Host with a valid ticket -> 403, then a correct request with the same ticket succeeds. (ET: Rebinding host)
  - ticket missing, expired (fake clock), reused, unknown, bound to another session -> 403 before the upgrade. (ET: Reused or expired, Wrong session)
  - `:5173` origin -> 403 unless `BYTHOS_DEV=1`. (ET: Dev origin gating)
- [x] 3.2 RED: functional tests: ticket + upgrade round trip with replay of "hello" on reattach and close code 4000 for the replaced attachment; resize frame clamped to 2..500 x 1..300; message above 1 MiB closes the connection and the PTY survives; ninth PTY returns 429; the `Config` built by `api` ignores query and body (`& calc` in name or query leaves the command unchanged); 501 with "La terminal requiere Windows 10 versión 1809 o superior." on `ErrNoSoportado`; `DELETE sesiones/{id}` with a fake live PTY closes it and it is absent from `Vivas()`; `DELETE sesiones/{id}/pty` kills the PTY and keeps the row. (ET: Valid ticket, Reload, Read limit, PTY cap, No injection, Old Windows; TS: Delete)
- [x] 3.3 Add the real-ConPTY integration test (skipped with `-short`): ticket -> WS -> `echo hola` over a real `cmd.exe`. (ET: Reload)
- [x] 3.4 Create `D/api/terminal.go`: `POST sesiones/{id}/ticket` (16 bytes `crypto/rand`, base64url, TTL 30 s, sweep on issue, delete on consume whatever the outcome, session must match); `GET sesiones/{id}/ws` in the order Host, exact Origin, ticket, `websocket.Accept` with host-only `OriginPatterns` (`localhost:8080`, plus `localhost:5173` only when `modoDev()`), never `InsecureSkipVerify`, `SetReadLimit(1 MiB)`; frames per design (binary input/output, text `resize`, `salida`, `error`); `DELETE sesiones/{id}/pty`; shell `%ComSpec%` falling back to `C:\Windows\System32\cmd.exe`, cwd from `workspace.PrepararWorkspace(exe)`, env `os.Environ()` + `BYTHOS_SESION=<id>`; Gestor 429 maps to HTTP 429. Wire the real Gestor into the server (and `CerrarTodo` on shutdown). (ET: all requirements)
- [x] 3.5 Modify `D/go.mod` / `go.sum`: add `github.com/coder/websocket v1.8.15`.
- [x] 3.6 Verify: `go vet ./...`, `go test ./...` from `desktop/`, and the real ConPTY test without `-short`.
- [x] 3.7 Open PR3 (base PR2 branch). Opened as #21 with size:exception (805 counted lines).

## PR4: Terminal screen and xterm (branch `feat/bythos-terminal-fase1-04-ui-terminal`, base `feat/bythos-terminal-fase1-03-ws`)

Satisfies: TS Navigation Entry, Create Session, Close and Delete Session, Status Chips (terminal chip); ET Reattach and Replay (client side) (design UI Architecture, D5, D6).

- [ ] 4.1 Modify `UI/package.json`: add `@xterm/xterm` 6.0.0 and `@xterm/addon-fit` 0.11.0 (exact versions, no `^`); update `pnpm-lock.yaml` (excluded from the budget). Modify `UI/vite.config.js`: `'/api': {target:'http://localhost:8080', ws:true}`.
- [ ] 4.2 Modify `S/api.js`: functions for sessions CRUD, ticket, entorno. Add the strings from the design UI Strings table used in this PR: "Terminal", "NUEVO", "Escribe un nombre para la sesión.", "Suelta", "La terminal requiere Windows 10 versión 1809 o superior.". (TS: Entry present, Empty name, Folder deleted)
- [ ] 4.3 Modify `S/App.jsx` (~25 lines): nav item `{id:'terminal', label:'Terminal', icon:'terminal', badge:'NUEVO'}` directly after Exportar with badge rendering; `lazy(() => import('./terminal/Terminal.jsx'))`; when `view==='terminal'` render `<Suspense><Terminal carpetas={carpetas}/></Suspense>` instead of the `.content` div (no PageHeader, no Topbar, no drop handler). (TS: Entry present)
- [ ] 4.4 Create `T/Terminal.jsx` (container): sessions list, open tabs and active tab in `localStorage`, create/close/delete flows (delete after confirmation; close only removes from `localStorage`). Sets `window.EXCALIDRAW_ASSET_PATH='/excalidraw/'` is deferred to PR5. (TS: Create, Delete)
- [ ] 4.5 Create presentational components `T/PestanasSesiones.jsx` (tabs, mode, "Suelta" for folderless), `T/DialogoNuevaSesion.jsx` (nombre, carpeta optional, modo; blank name shows "Escribe un nombre para la sesión."), `T/PanelTerminal.jsx` (xterm + fit addon, ResizeObserver with 100 ms debounce, "Abrir agente" fallback on 501). (TS: Create, Empty name)
- [ ] 4.6 Create `T/conexionPty.js`: ticket + ws, reconnect backoff 0.5 to 5 s, `TextEncoder` for binary input, resize text frames, handle close 4000 (no reconnect storm) and `salida` frames; `term.reset()` before writing the replay. (ET: Reload)
- [ ] 4.7 In `PanelTerminal.jsx`, add `attachCustomKeyEventHandler`: Ctrl+C with selection or Ctrl+Shift+C copies and returns false; Ctrl+V / Ctrl+Shift+V return false without `preventDefault`; Ctrl+R calls `preventDefault` and returns true. Add a capture-phase window keydown listener blocking F5 and Ctrl+R while the view is mounted (removed on unmount). (ET: Reload)
- [ ] 4.8 Modify `S/styles.css` for the terminal layout (tabs, panel, nav badge).
- [ ] 4.9 Verify: `pnpm build` from `desktop/ui`; `go vet ./...` and `go test ./...` from `desktop/` still green (embedded `dist` unaffected); manual checks M.1 to M.4 on the real app before the PR is opened.
- [ ] 4.10 Open PR4 (base PR3 branch) with M.1 to M.4 in the description.

## PR5: Whiteboard, autosave and offline fonts (branch `feat/bythos-terminal-fase1-05-pizarra-ui`, base `feat/bythos-terminal-fase1-04-ui-terminal`)

Satisfies: WB Toolset, Autosave, Offline Assets, Lazy Load; TS Status Chips (save chip), Revisar (flush part) (design D10, D11, UI Architecture, Fonts).

- [ ] 5.1 Modify `UI/package.json`: add `@excalidraw/excalidraw` 0.18.1 pinned exactly (no `^`); update the lockfile (excluded from the budget). (WB: Lazy Load)
- [ ] 5.2 Create `UI/scripts/copiar-fuentes.mjs` (Node built-ins only; `fs.cpSync` with a filter excluding `Xiaolai`; source `node_modules/@excalidraw/excalidraw/dist/prod/fonts`, target `public/excalidraw/fonts`). Wire it ahead of `dev` and `build` in `package.json` (not via `pre*`). Add the output folder to `.gitignore`; fonts are not committed. (WB: No external requests)
- [ ] 5.3 Modify `D/api/ui.go`: `mime.AddExtensionType(".woff2","font/woff2")` with a test that a `.woff2` is served as `font/woff2`. (WB: No external requests)
- [ ] 5.4 Create `T/excalidrawAdapter.js` (the only importer of `@excalidraw/excalidraw`): wrappers for `convertToExcalidrawElements(..., {regenerateIds:false})`, `updateScene({elements, captureUpdate})`, `exportToBlob`, `getSceneVersion`, `sanearEscena` (strips images and deleted elements). (WB: Lazy Load)
- [ ] 5.5 Create `T/Pizarra.jsx`: props `UIOptions={{tools:{image:false}, canvasActions:{loadScene:false, saveToActiveFile:false, export:false, saveAsImage:false}}}`, `aiEnabled={false}`, `onPaste` guard returning false when files are present, capture-phase `onDropCapture`/`onPasteCapture` wrapper blocking `Files` and showing "Las imágenes llegan en una fase posterior.". (WB: Image blocked, Toolset)
- [ ] 5.6 Create `T/ChipsEstado.jsx` with "Guardando…", "Guardado", "Error al guardar" plus the terminal-state chip. (TS: Save state)
- [ ] 5.7 Modify `T/Terminal.jsx`: set `window.EXCALIDRAW_ASSET_PATH='/excalidraw/'` at module top before the lazy Excalidraw import; autosave: `onChange` checks `getSceneVersion`, 1.5 s debounce, `PUT {escena, version}`; on 409 reload the server scene and show "Actualizada desde otra ventana"; on failure keep the scene in memory, show "Error al guardar", retry on next change or flush; flush on tab switch, unmount, `visibilitychange:hidden`, and `pagehide` with `keepalive` only if the body is under 60 KiB; add the "Revisar" button that flushes and focuses the terminal without writing any bytes to the PTY. (WB: Restart, Save failure; TS: Revisar, Save state)
- [ ] 5.8 Verify: `pnpm build` from `desktop/ui` (the Excalidraw chunk is separate and not loaded by other screens); `go vet ./...` and `go test ./...` from `desktop/`. If CI fails to build Excalidraw on Node 18, bump `setup-node` to 20 in the workflow in this PR. Resolve the open question on `aiEnabled`, `onPaste` file data and `regenerateIds:false` in 0.18.1 and note the result in the PR.
- [ ] 5.9 Manual checks before opening: M.5, M.6, M.7, M.10 on the real app.
- [ ] 5.10 Open PR5 (base PR4 branch).

## PR6a: Pizarra domain, ops, routes and MCP tools (branch `feat/bythos-terminal-fase1-06a-pizarra-agente`, base `feat/bythos-terminal-fase1-05-pizarra-ui`)

Satisfies: WT Active Session, pizarra_leer, pizarra_captura, pizarra_dibujar (design D1, D8 Agente, D12, D13, MCP tools). Forecast ~900 lines: see "Decision needed before apply". If the user accepts the split, the cut is after task 6a.9 (6a-i = tasks 6a.1 to 6a.9 on branch `feat/bythos-terminal-fase1-06a-pizarra-api`; 6a-ii = tasks 6a.10 to 6a.14 on branch `feat/bythos-terminal-fase1-06b-pizarra-mcp`, renumbering the later branch to `07` is not needed: 6b keeps its name and is based on the last 6a branch).

- [ ] 6a.1 RED: create `D/pizarra/ops_test.go` (table-driven): `Validar` caps per field (200 ops/call, text 10,000, coords ±100,000, sizes 0..20,000, `puntos` 2,000, colors `#rrggbb` or `transparent`, ids `^[A-Za-z0-9_-]{1,64}$`, no `image` elements, unknown verb/type rejected). (WT: Too many ops)
- [ ] 6a.2 Create `D/pizarra/ops.go`: `Validar` for the verbs `agregar`, `actualizar`, `borrar`, `mover`; id assignment helper for missing element ids. (WT: Draw, Too many ops)
- [ ] 6a.3 RED then create `D/pizarra/resumen_test.go` / `resumen.go`: `Resumir` pure function: at most 2,000 elements, 2,000 chars of text each, `truncado: true`, `resumen:{total,por_tipo}`, `puntos` as count only. (WT: Truncation)
- [ ] 6a.4 RED then create `D/db/pizarra_ops.go` (+ `crearTablaOps` in `db.go`, `terminal_pizarra_ops` with the estado CHECK and the (`sesion_id`,`estado`) index) and tests: insert `pendiente`; ack `aplicada`/`fallida`; lazy sweep turns rows older than 30 s into `vencida`; ops cascade on session delete. Add `AccionPizarraEditada="pizarra_editada"` to `db/eventos.go`. (WT: Draw)
- [ ] 6a.5 RED: create `D/api/pizarra_test.go` for ops lifecycle: ack within 8 s returns `aplicada` with ids; no ack within 8 s returns `pendiente` with the "se aplicará en segundos si la pizarra sigue abierta" text; late row becomes `vencida` and is never delivered; target session not mounted (no latido in 3 s) returns 409 with the design message and enqueues nothing; 201 ops returns 400 and enqueues nothing; redelivery after a lost ack is idempotent (merge by id); no DB connection held while waiting (`SetMaxOpenConns(1)` test with a concurrent read). (WT: Draw, Too many ops, Concurrent edit, Board not open)
- [ ] 6a.6 RED: capture tests: not mounted returns 409 immediately with "La pantalla Terminal no está abierta: ábrela en Bythos para capturar la pizarra."; mounted PNG upload accepted; magic bytes checked (non-PNG rejected); over 4 MiB returns 413; 15 s timeout (fake clock or short seam). (WT: Mounted, Not mounted)
- [ ] 6a.7 RED: session-resolution tests (every response carries `resolucion` in `argumento|entorno|activa|reciente`): explicit unknown id returns 404 "La sesión N no existe" and never falls through; stale `pista` (deleted or non-numeric) is ignored and falls to the last latido session; then most recent `actualizado`; none returns 404 "No hay sesiones de Terminal"; `pizarra_leer` works with no latido (UI closed). (WT: Default, Unknown id, Stale hint, UI closed)
- [ ] 6a.8 Create `D/api/pizarra.go`: `POST latido` (Ventana; records mounted session and last-latido, returns `{ops, capturas}`), `POST pizarra/ops/{id}/ack` and `POST pizarra/capturas/{id}` (Ventana), and Agente routes `GET pizarra?sesion_id=&pista=`, `POST pizarra/ops`, `POST pizarra/captura`; in-memory capture channels; `registrarEvento(pizarra_editada)` on `aplicada`. Agente guard tests: extension Origin on ops or captura returns 403; foreign Origin on ack or upload returns 403; empty Origin accepted on Agente routes only. Wire in `server.go`. (WT: all; Threat Matrix Agent routes)
- [ ] 6a.9 Verify 6a-i: `go vet ./...`, `go test ./...` from `desktop/`. **Split boundary (if accepted): open PR6a-i here.**
- [ ] 6a.10 RED: create `D/agentes/pizarra_test.go`: `pizarra_captura` result has one ImageContent and one TextContent and `StructuredContent == nil`; `pizarra_dibujar` validates early with `pizarra.Validar`; `pizarra_leer` output shape `{sesion, resolucion, version, actualizado, elementos, resumen, limites, ops_pendientes, truncado}`; the PNG is written to `capturas/pizarra-<id>.png` in the agent workspace and the text cites it; ack timeout through httptest uses the 25 s client for capture. (WT: Mounted, Draw)
- [ ] 6a.11 Create `D/agentes/pizarra.go`: `pizarra_leer` (read-only annotation), `pizarra_dibujar` (`DestructiveHint:true`), `pizarra_captura` registered through raw `Server.AddTool` with `InputSchema` from `jsonschema.For` and never setting `StructuredContent`; sends `sesion_id` and the env hint as a separate `pista` field. (WT: pizarra_captura, Active Session)
- [ ] 6a.12 Modify `D/agentes/cliente.go` (`TimeoutLargo` 25 s used for capture only; everything else keeps 15 s) and `D/agentes/servidor.go` (register the three tools). Add or extend tests. (WT: pizarra_captura)
- [ ] 6a.13 Verify: `go vet ./...`, `go test ./...` from `desktop/`; run the real ConPTY test without `-short` once on the stacked tip.
- [ ] 6a.14 Open PR6a (base PR5 branch; or PR6a-ii based on PR6a-i if split).

## Spike: per-CLI check on Windows (between PR6a and PR6b; no code PR)

Run on the real app built from the PR6a tip, in an embedded terminal of session S. Record the results in the table below (edit this file in the PR6b commit); PR6b adjusts configs and text from them. Closes the open questions on OpenCode env/image behavior.

- [ ] S.1 For each CLI (`claude`, `opencode`, `gemini`, `codex`): launch it from the embedded terminal of session S, confirm the image is visible (the model describes the drawing from `pizarra_captura`), confirm `pizarra_leer` -> `resolucion` is `entorno` (BYTHOS_SESION reached the server) or `activa`/`reciente` (the `pista` did not arrive), and record whether an approval or trust prompt appears. If a CLI drops the image, check it can read the cited `capturas/pizarra-<id>.png`.
- [ ] S.2 Fill the table:

| CLI | Approval/trust prompt | `resolucion` seen | Image visible | File fallback works | Config change needed for PR6b |
|---|---|---|---|---|---|
| claude | (pending) | (pending) | (pending) | (pending) | (pending) |
| opencode | (pending) | (pending) | (pending) | (pending) | (pending) |
| gemini | (pending) | (pending) | (pending) | (pending) | (pending) |
| codex | (pending) | (pending) | (pending) | (pending) | (pending) |

## PR6b: UI latido/apply/capture and workspace text (branch `feat/bythos-terminal-fase1-06b-agente-ui`, base `feat/bythos-terminal-fase1-06a-pizarra-agente` or the last 6a branch if split)

Satisfies: WT pizarra_captura, pizarra_dibujar (client side), Delete Rules; TS Revisar (design D12, D13, Workspace).

- [ ] 6b.1 RED: modify `D/workspace/workspace_test.go`: AGENTS.md (and CLAUDE.md) contains the pizarra rule and still keeps the library no-delete rule; MCP `Instructions` contain both rules; `.mcp.json` has `env:{"BYTHOS_SESION":"${BYTHOS_SESION:-}"}`; `.codex/config.toml` has `env_vars = ["BYTHOS_SESION"]`; approval guidance in `LEEME.txt` (Claude: approve the project server; Codex: trust the folder; Gemini: trust the folder; OpenCode: no approval step), adjusted by the spike table. (WT: Instructions, Board delete)
- [ ] 6b.2 Modify `D/workspace/workspace.go`: Rule 1 becomes "the library has no delete tools; on the whiteboard the agent may add, edit, move and delete with `pizarra_dibujar` when the user asks, and looks at the board only when asked (Revisar)"; the same sentence goes into the MCP `Instructions` (in `D/agentes/servidor.go`); env forwarding for `.mcp.json` and Codex; apply any change the spike requires for OpenCode and Gemini. (WT: Delete Rules)
- [ ] 6b.3 Modify `T/Terminal.jsx` (and the adapter if needed): 1 s `latido` POST with `{sesion_id}` only while the view is mounted; apply op batches atomically (reject the whole batch if any target id is missing; merge by id; call `updateScene` with `captureUpdate` immediately so the user can Ctrl+Z); ack `aplicada`/`fallida` with ids; handle `capturas`: `exportToBlob({mimeType:'image/png', maxWidthOrHeight:1280})` then POST the PNG to `pizarra/capturas/{id}`; flush the scene before answering a capture. (WT: Draw, Concurrent edit, Mounted)
- [ ] 6b.4 Add an in-app banner or `LEEME.txt` section showing the per-CLI approval guidance (UI banner text taken from the spike). (WT: Instructions)
- [ ] 6b.5 Resolve the remaining open question (`aiEnabled`, `onPaste`, `regenerateIds:false`) by manual check on the real app; fix inside the adapter only.
- [ ] 6b.6 Verify: `go vet ./...`, `go test ./...` from `desktop/`; `pnpm build` from `desktop/ui`; manual M.11 to M.13.
- [ ] 6b.7 Open PR6b (base the 6a branch) with the spike table and M.11 to M.13 in the description.

## Manual verification checklist (real app, Windows)

Run on an installed or locally built `bythos.exe`. Items are grouped by the PR that makes them testable; tick them in the PR description and again here.

- [ ] M.1 (PR4) Terminal works: open Terminal, create a session, a `cmd` prompt appears in the app; run a CLI (`claude`, or `npm -v`) and confirm it logs in or runs normally.
- [ ] M.2 (PR4) Reload keeps the session: print "hello", press F5/Ctrl+R in the app (the key is blocked inside the terminal) and use the reload of the window; the same process is still running and "hello" is replayed.
- [ ] M.3 (PR4) Clipboard: select text and Ctrl+C copies it (without a selection Ctrl+C interrupts the process); Ctrl+Shift+C copies; Ctrl+V and Ctrl+Shift+V paste once (not twice).
- [ ] M.4 (PR4) Sessions: empty name shows "Escribe un nombre para la sesión."; a folderless session shows "Suelta"; closing a tab keeps the data; deleting asks for confirmation and ends the PTY.
- [ ] M.5 (PR5) Drawing and autosave survive a restart: draw, wait for "Guardado", close Bythos fully, reopen: the same elements load. Edit and switch tabs/quit immediately: the edit is still saved.
- [ ] M.6 (PR5) Image paste and drop onto the board are refused with "Las imágenes llegan en una fase posterior."; the image tool is not in the toolbar.
- [ ] M.7 (PR5) Offline fonts: with the DevTools Network panel open (or network disabled) and text drawn on the board, there are zero non-localhost requests and the font renders.
- [ ] M.8 (PR0) Single instance: with Bythos open, launch `bythos.exe` again: no second listener, the existing window is shown, the new process exits. After killing the process from Task Manager, Bythos starts normally.
- [ ] M.9 (PR0) Installer upgrade with the app open: run the installer; it asks to close Bythos before continuing; same for the uninstaller.
- [ ] M.10 (PR5) Revisar: with pending board edits press Revisar: the chip goes to "Guardado", the terminal has focus, and nothing was typed into the terminal.
- [ ] M.11 (PR6b) An agent draws and captures: from the embedded terminal run an agent with the Bythos MCP, ask it to draw a rectangle (it appears, Ctrl+Z undoes it) and to look at the board (it describes the drawing). With the Terminal view closed, `pizarra_dibujar` and `pizarra_captura` fail with the Spanish 409 messages; `pizarra_leer` still returns the saved scene.
- [ ] M.12 (PR6b) Closing a folder loosens sessions: delete a folder that has a linked session: the session, scene and ops remain and the session shows "Suelta".
- [ ] M.13 (PR6b) Process cleanup: with two sessions running a long command (`ping -t 127.0.0.1`), end `bythos.exe` from Task Manager: no `ping.exe` or `cmd.exe` child remains. Also confirm a PTY detached for more than 10 min is reaped (optional, or shorten `Gracia` via a debug build).
- [ ] M.14 (final) Exe size is about 31 MB (compare with the previous release; the fonts are not embedded in git but are in `dist`, so report the real size).
- [ ] M.15 (final) Windows older than 1809 (or the simulated `ErrNoSoportado` path): the UI shows "La terminal requiere Windows 10 versión 1809 o superior." and offers "Abrir agente".

## Chain order and boundaries

```
master
  └─ PR0  feat/bythos-terminal-fase1-00-instancia
       └─ PR1a feat/bythos-terminal-fase1-01a-conpty
            └─ PR1b feat/bythos-terminal-fase1-01b-gestor
                 └─ PR2  feat/bythos-terminal-fase1-02-sesiones
                      └─ PR3  feat/bythos-terminal-fase1-03-ws
                           └─ PR4  feat/bythos-terminal-fase1-04-ui-terminal
                                └─ PR5  feat/bythos-terminal-fase1-05-pizarra-ui
                                     └─ PR6a feat/bythos-terminal-fase1-06a-pizarra-agente
                                          └─ [spike]
                                               └─ PR6b feat/bythos-terminal-fase1-06b-agente-ui
```

Each child PR description carries this diagram with the current PR marked, plus start, end, prior dependencies, follow-up work and out-of-scope items (chained-pr). Commits follow work-unit-commits: code and its tests in the same commit; conventional commits, no AI attribution.
