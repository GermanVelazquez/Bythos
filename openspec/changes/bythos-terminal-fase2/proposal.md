# Proposal: Terminal Phase 2 (Paste and Read)

## Intent

The board blocks images and files, so users cannot share screenshots, notebook photos, or documents with their agent. Phase 2 of `Bythos — Terminal decisiones.pdf` lets users bring them onto the board and lets the agent read them. Library stays Excalidraw 0.18.1.

## Scope

### In Scope
- Entry: Ctrl+V, drag-drop, "Adjuntar" button, pick a resource of the session's folder
- Types: images, PDF, Word, Excel, PowerPoint, text (no video); 25 MiB cap, clear rejection message
- Images: drawable image elements, downscaled client-side to 1440 px
- Documents: board card that opens in the existing viewer
- Attachments private to the session; "Guardar en carpeta" per card/image (only with a folder; never automatic)
- Remove one attachment from the board
- MCP `leer_imagen` (image + text), `leer_archivo` (text with char cap + `truncado`; PDF returns metadata + absolute path); `pizarra_leer` lists attachments
- Agent reads only when asked

### Out of Scope
- Agent-inserted images (`pizarra_dibujar` still rejects `image`)
- Phase 3 (Revisar typing, `pizarra_marcar`, tutor rules, undo history); phase 4 (Engram configs, exports, highlighter, formula, pen pressure, live pages, agent SVG)
- Attachment GC, PDF text extraction, new phone flow, non-Windows

## Capabilities

### New Capabilities
- `whiteboard-attachments`: upload endpoint/guard, per-session link table, limits/types, rehydration, document card, Guardar en carpeta, pick folder resource, removal

### Modified Capabilities
- `whiteboard`: Toolset "Image blocked" scenario replaced; Scene Limits admit image/card elements only when referencing the session's attachments; persisted scene keeps `files: {}`
- `whiteboard-agent-tools`: add `leer_imagen`, `leer_archivo`; `pizarra_leer` lists attachments; untrusted-content notice
- `terminal-sessions`: Delete removes attachment link rows but keeps files on disk

## Approach

Bytes go to the content-addressed `archivos` store; new `terminal_adjuntos` links them to sessions. A dedicated upload endpoint sits behind the window Origin guard (not `POST /api/archivos`). The UI intercepts paste (`onPaste` returns `false`) and drop (capture phase), uploads, then inserts via `addFiles` + `updateScene` with `status: "saved"`; on load it rehydrates files before or via `updateScene`. `leer_imagen` is a raw `AddTool` without `structuredContent`.

Design opens with a spike: drop interception, embeddable card + `renderEmbeddable` (including capture), rehydration order. Fallbacks: no drop interception, ship paste + Adjuntar and defer drop; embeddable missing from captures, use a rectangle+text group with `customData`.

| PR | Scope | Est. |
|---|---|---|
| P1 | DB + upload/list/get/delete API + tests | ~400 |
| P2 | Scene validation + session-delete behavior | ~250 |
| P3 | UI paste/drop/Adjuntar, upload, rehydration | ~400 |
| P4 | `leer_imagen`, `leer_archivo`, `pizarra_leer` listing | ~400 |
| P5 | Card + viewer, pick resource, Guardar, instructions | ~300 |

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `desktop/db/` | Modified | `terminal_adjuntos` |
| `desktop/api/sesiones.go` + new handlers | Modified/New | `reducirEscena`, upload/read routes |
| `desktop/pizarra/`, `desktop/agentes/` | Modified | New tools, instructions |
| `desktop/ui/` (`Pizarra.jsx`, `excalidrawAdapter`) | Modified | Unblock, insert, card, rehydrate |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Drop not interceptable | Med | Spike; defer drop |
| Card missing in captures | Med | Rectangle+text fallback |
| Rehydration race (gray tiles) | Med | Files before `updateScene` |
| Autosave bloat | Low | Strip `files` client and server |
| Excalidraw/server MIME mismatch | Med | UI pre-filter |
| Prompt injection | Med | Untrusted-data notice |
| Orphan blobs | Certain | Accepted; GC deferred |

## Rollback Plan

Revert PRs in reverse order. `terminal_adjuntos` is additive (`IF NOT EXISTS`); leftover blobs are harmless. Reverting P2/P3 restores the image block; existing scenes with image elements must be stripped by `sanearEscena`.

## Dependencies

- None new; phase-1 baseline (Excalidraw 0.18.1, `archivos` store, viewer)

## Success Criteria

- [ ] All entry paths add images/cards that survive restart
- [ ] Persisted scenes contain no file bytes
- [ ] Oversize and disallowed types are rejected clearly
- [ ] Agent reads attachments with the UI closed
- [ ] Session delete keeps files; each PR under 400 lines
