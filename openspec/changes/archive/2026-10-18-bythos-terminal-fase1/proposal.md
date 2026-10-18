# Proposal: Terminal Phase 1 (Whiteboard + Embedded Terminal)

## Intent

Users run their own CLI agent in an external window with no shared visual workspace. Phase 1 of `Bythos — Terminal decisiones.pdf` adds a Terminal screen: an Excalidraw whiteboard plus an embedded terminal, persisted per session, that the agent can read and edit through MCP. Bythos ships no AI.

## Scope

### In Scope
- "Terminal" menu item below Exportar ("NUEVO" badge); session tabs with mode; "+" dialog (nombre, carpeta, modo); status chips
- Whiteboard: free draw, colors, stroke width, eraser, shapes, text; image tool and image paste/drop disabled
- Embedded terminal (ConPTY + xterm.js over WebSocket) as default; external "Abrir agente" kept
- `terminal_sesiones`, `terminal_pizarras`, `terminal_pizarra_ops` in `bythos.db` with autosave; folder delete loosens sessions
- MCP `pizarra_leer`, `pizarra_captura`, `pizarra_dibujar` (whole board; read/write/organize/edit/delete)
- "Revisar" affordance: flushes autosave and focuses the terminal; the agent reads the board only when asked
- AGENTS.md / CLAUDE.md / MCP Instructions: whiteboard deletes allowed, library deletes forbidden; per-CLI approval/trust guidance
- PR0: single-instance mutex + installer `AppMutex`
- Empirical per-CLI spike (claude/opencode/gemini/codex): image visible, `BYTHOS_SESION` visible, approval prompt

### Out of Scope
- Phase 2: paste/drop/attach of images and files, `leer_imagen`, `leer_archivo`
- Phase 3: Revisar typing the request into the terminal, `pizarra_marcar`, tutor rules, agent undo history
- Phase 4: Engram in configs/installer, exports, highlighter, formula, pen pressure, live page nodes
- Non-Windows support, cloud, sharing

## Capabilities

### New Capabilities
- `terminal-sessions`: sessions, modes, folder link, tabs, dialog, chips
- `embedded-terminal`: PTY lifecycle, WebSocket, ticket/Origin/Host guard, reattach
- `whiteboard`: Excalidraw toolset, offline fonts, autosave, scene limits
- `whiteboard-agent-tools`: `pizarra_*`, ops queue, capture, active session, delete-rule text
- `single-instance`: app mutex and installer `AppMutex`

### Modified Capabilities
- None (no archived baseline specs exist)

## Approach

Hand-rolled ConPTY on `x/sys/windows` (suspended child, KILL_ON_JOB_CLOSE job, goroutine per pipe, 1809+ check) behind a `PTY` interface. `coder/websocket` v1.8.15 (only new Go dependency) on the existing loopback listener, gated by exact Origin (`origenesAgentePermitidos`), Host and a one-time 128-bit, ~30 s, session-bound ticket. UI-owned scene; agent ops go through a persisted queue the UI applies by element id and acks. Capture is rendered on demand (`exportToBlob`), downscaled, returned as ImageContent + text, no `structuredContent`. Excalidraw 0.18.1 pinned, lazy, behind an adapter; fonts self-hosted without Xiaolai. xterm 6.0.0 + fit 0.11.0.

Delivery: Feature Branch Chain, 400-line budget (lockfile/fonts excluded).

| PR | Scope | Est. |
|---|---|---|
| PR0 | Mutex + `AppMutex` | ~60 |
| PR1 | ConPTY, Job, manager, ring buffer, tests | ~350 |
| PR2 | WS endpoint, ticket, guard, tests | ~350 |
| PR3 | DB + API: sessions, scenes, limits | ~380 |
| PR4 | Nav, `Terminal.jsx`, tabs, xterm, `ws:true` | ~400 |
| PR5 | Excalidraw, fonts, autosave, layout, chips | ~350 |
| PR6 | `pizarra_*`, ops, capture, active session, spike, instructions | ~400 (6a/6b) |

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `desktop/main.go`, `installer/bythos.iss` | Modified | Single instance |
| `desktop/terminal/` | New/Modified | ConPTY, manager |
| `desktop/api/` | New/Modified | WS, ticket, sessions, scenes, ops |
| `desktop/db/` | New | Three tables |
| `desktop/agentes/`, workspace generator | Modified | Tools, configs, instructions |
| `desktop/ui/` | New/Modified | Terminal screen, deps, fonts, proxy |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Shell WebSocket hijack (top) | Med | Exact Origin + Host + ticket; guard test matrix first |
| ConPTY deadlock/leaks | Med | Drain pipes, Job Object, real-PTY test |
| CLI image/env/approval gaps | High | Text-first `pizarra_leer`; active-session default; spike |
| Exe ~23 to ~31 MB | Certain | Accepted; lazy load |
| Stroke/op races | Med | Merge by element id |
| Ctrl+W closes window | Med | Documented limitation |

## Rollback Plan

Revert PRs in reverse order (later slices build on earlier ones; PR0 stands alone). New tables are additive (`IF NOT EXISTS`) and harmless if left. Reverting PR6 restores the previous no-delete instructions; reverting PR0 removes the mutex with no data impact.

## Dependencies

- `github.com/coder/websocket` v1.8.15; `@excalidraw/excalidraw` 0.18.1; `@xterm/xterm` 6.0.0; `@xterm/addon-fit` 0.11.0
- Windows 10 1809+

## Success Criteria

- [ ] Sessions and scenes survive restart; folder delete loosens sessions
- [ ] Guard tests reject wrong/empty/extension Origin, bad Host, missing/expired/reused ticket
- [ ] No PTY survives app exit; reload reattaches
- [ ] Agent reads, draws, edits and deletes on the board; library stays no-delete
- [ ] No CDN requests; one app instance; each PR under 400 lines
