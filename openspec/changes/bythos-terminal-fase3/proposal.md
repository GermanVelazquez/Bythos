# Proposal: Terminal Phase 3 (Correct by Folder)

## Intent

Phase 3 of `Bythos — Terminal decisiones.pdf`: the student solves on the board, presses Revisar, and the agent marks each step (ok / error / parcial) with a pista and a citation from the session's folder. Every agent change can be undone. Today Revisar is a stub, marks do not exist, and agent batches mix into Excalidraw's Ctrl+Z and are lost on reload.

## Scope

### In Scope
- Revisar pastes a fixed single-line request (numeric session id only) via `term.paste`; no newline, no Enter
- MCP `pizarra_marcar`: server validates and expands marks into a frame + standalone text pill (`customData.bythos="marca"`); folder-scoped `cita`, Bythos-built title; caps 20 marks / 300 / 200 chars; re-mark replaces; "Quitar marcas"
- Agent history: UI-computed inverse + versions in the ack, nullable columns on `terminal_pizarra_ops`, last 50 batches, conflict-safe partial undo from a UI panel only (`conVentana`)
- `ReglaTutor` in AGENTS.md, CLAUDE.md, MCP `Instructions`, `docs/AGENTES.md`; full solution only on explicit request; always propose % and ask before `actualizar_progreso`
- Runtime checks as early tasks, each with a fallback

### Out of Scope
- Phase 4: Engram in MCP config, exports, highlighter, formulas, pen pressure, live pages
- Agent-triggered undo, automatic progress, exercise model, PDF page verification, server-side PTY write

## Capabilities

### New Capabilities
- `whiteboard-agent-history`: inverse capture, storage, retention, list, conflict-safe undo, history panel
- `tutor-rules`: `ReglaTutor` content, placement, progress-consent rule

### Modified Capabilities
- `terminal-sessions`: Revisar replaces "MUST NOT write any bytes to the PTY" with the fixed unsubmitted paste
- `whiteboard-agent-tools`: add `pizarra_marcar`; `pizarra_dibujar` ack carries inverse + versions
- `whiteboard`: mark elements in Scene Limits; "Quitar marcas" action

## Approach

Client WS paste via a new `escribirRef` from `PanelTerminal` (same Origin/ticket path as typing). Marks are ordinary tagged scene elements with deterministic per-target ids. Undo replays stored inverses, skipping elements whose version changed, and reports the skipped count. Agent ops stay on `CaptureUpdateAction.IMMEDIATELY`.

| Runtime check (early task) | Fallback |
|---|---|
| Revisar line lands unsubmitted (Claude Code, Codex, OpenCode, Gemini, pwsh) | Shorten template; document the CLI caveat |
| `customData` survives `updateScene`, copy/paste, import | Identify marks by id prefix |
| NEVER-vs-user-edit undo interplay | Keep IMMEDIATELY |

**Vitest: recommend for**, dev-only, scoped to pure inverse logic in `opsPizarra.js`. Pro: covers bound labels and version bumps that Go tests cannot. Con: new devDeps; outside CI (desktop-only) unless a step is added later.

## PR Plan (auto-chain, stacked-to-main, ≤400 lines)

| PR | Scope | Est. |
|---|---|---|
| 1 | Revisar paste + spec delta + check | ~150 |
| 2 | `pizarra_marcar` server + citation + tests | ~380 |
| 3 | Marks UI, replace, Quitar marcas, `customData` check | ~250 |
| 4 | Ops migration, extended ack, `GET cambios`, retention | ~350 |
| 5 | Inverse computation + Vitest | ~350 |
| 6 | `POST deshacer`, conflict check, event | ~300 |
| 7 | History panel + NEVER check | ~300 |
| 8 | `ReglaTutor` + docs + tests | ~120 |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Prompt injection via board/pills | Med | Fixed template; content-as-data rule |
| CLI submits pasted line | Low | No newline; runtime check |
| Wrong inverse | Med | Vitest; partial undo |
| Lost ack = no undo | Low | Show "sin deshacer" in panel |
| Mark buildup | Med | Replace + Quitar marcas |

## Rollback Plan

Revert PRs in reverse order. New columns are nullable and additive; old code ignores them. Mark elements remain valid scene elements. Reverting PR 1 restores focus-only Revisar and the original spec.

## Dependencies

- Phase 1/2 baseline; Vitest (dev-only)

## Success Criteria

- [ ] Revisar places one unsubmitted line, never scene text
- [ ] Out-of-folder citations rejected; caps enforced
- [ ] Undo restores untouched elements and reports skipped ones, after reload
- [ ] Rules present in all four places; each PR ≤400 lines
