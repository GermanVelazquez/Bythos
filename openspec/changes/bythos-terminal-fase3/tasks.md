# Tasks: Terminal Phase 3 (bythos-terminal-fase3), "Correct by Folder"

Paths: `D` = `desktop`, `UI` = `desktop/ui`, `S` = `desktop/ui/src`, `T` = `desktop/ui/src/terminal`. Specs: `TS` = terminal-sessions, `WT` = whiteboard-agent-tools, `WB` = whiteboard, `WH` = whiteboard-agent-history, `TR` = tutor-rules. Design refs (D1..D21, R1..R4, Threat Matrix) point to `design.md`, which is authoritative. Code, comments and UI strings are Spanish (neutral register for new copy); this document is English.

Verification per PR (all green before opening the PR):

- Go PRs (2, 4, 5, 7, 9) and every PR: from `desktop/`: `go vet ./...`, `GOOS=linux go vet ./...`, `go test ./...` (no `-race` available).
- UI PRs (1, 3, 8): also from `desktop/ui`: `pnpm build`, then `git checkout -- desktop/ui/dist/.gitkeep` (the build deletes the tracked placeholder). Before PR 6 the UI has no automated tests: verify with `pnpm build` and the manual checks.
- PR 6 onward: also `pnpm test` (Vitest) from `desktop/ui`.
- Strict TDD: Go tests are RED first (write, run, see it fail, then implement). The only automated UI tests are the Vitest ones for the inverse logic (PR 6 on).
- Runtime checks R1 to R4 are the FIRST task of their PR, each with its fallback from the design.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | PR1 ~150; PR2 ~340; PR3 ~240; PR4 ~340; PR5 ~370; PR6 ~370; PR7 ~280; PR8 ~330; PR9 ~170. Total ~2,590 text lines (lockfile excluded). |
| 400-line budget risk | PR1 Low; PR2 Medium; PR3 Low; PR4 Medium; PR5 **Medium-High**; PR6 **Medium-High**; PR7 Low; PR8 Medium; PR9 Low |
| Chained PRs recommended | Yes |
| Suggested split | 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9 (design chain) |
| Delivery strategy | auto-chain |
| Chain strategy | stacked-to-main (PR 1 bases on `master`; each later PR bases on the previous PR branch; after a merge, retarget the next PR to `master`; merge commits) |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: Medium (PR5 and PR6 at the edge)

Notes: strategy already chosen, so apply proceeds with PR 1. Apply measures the real diff (`additions + deletions`, lockfile excluded) after each PR. If a PR ends above 400 with no cohesive cut left, make one honest slicing pass, then ask for `size:exception` (chained-pr rule); never trim tests, comments or docs to fit. Candidate cuts: PR5 = (columns, migration, ack storage) / (retention, `ListarCambios`, `GET cambios`); PR6 = (`calcularInversa`, `planDeshacer`, tests) / (ack wiring, Vitest setup, CI step). The marks UI (PR 3) lands before the route (PR 4), so `datos` is never dropped.

### Suggested Work Units

| Unit | Goal | PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|----|----------------------|-----------------|-------------------|
| A | Revisar paste into the PTY | PR1 | `pnpm build` | R1 in real CLIs and pwsh | `PanelTerminal.jsx`, `conexionPty.js`, `PanelSesion.jsx` |
| B | Marks domain (validate, expand, ids, geometry) | PR2 | `go test ./pizarra -run "Marca|Caja|Datos"` | table tests, no I/O | `pizarra/marcas.go`, `Cajas`, `Datos` field (additive) |
| C | Marks render, Quitar marcas, forged-mark guard | PR3 | `go test ./api -run "Escena|Marca"` + `pnpm build` | R2, R4 in real app | `opsPizarra.js`, `Pizarra.jsx` button, `sesiones.go` D11 |
| D | `pizarra_marcar` route and MCP tool | PR4 | `go test ./api ./agentes -run "Marcar|Marca"` | httptest + MCP in-memory client | `marcarPizarra`, `agentes/pizarra.go` tool |
| E | Inverse columns, ack, 2-cap retention, GET cambios | PR5 | `go test ./db ./api -run "Cambios|Inversa|Retencion|Ack"` | httptest + `t.TempDir` | `db/pizarra_ops.go`, ack reader, `GET cambios` (nullable columns) |
| F | Inverse computation, Vitest, CI step | PR6 | `pnpm test` | Vitest node env | `opsPizarra.js`, `usePizarraAgente.js`, `package.json`, `build.yml` |
| G | POST deshacer with claim, liberar, montada | PR7 | `go test ./api ./db -run "Deshacer"` | httptest | `deshacer` handler, `AccionPizarraDeshecha` |
| H | History panel and undo flow | PR8 | `pnpm build` + `pnpm test` | R3 in real app | `HistorialPizarra.jsx`, `useHistorialPizarra.js`, toggle |
| I | Tutor rules, docs, regeneration | PR9 | `go test ./workspace ./agentes` | in-memory MCP client, `t.TempDir` | `workspace.go`, `servidor.go`, `docs/AGENTES.md` |

## PR1: Revisar (branch `feat/bythos-terminal-fase3-01-revisar`, base `master`)

Satisfies: TS Revisar (5 scenarios) (D1, D2).

- [ ] 1.1 (manual, see M.1) Runtime check R1 (first task): in the real app, paste the template through the WebSocket path into Claude Code, Codex, OpenCode and pwsh; confirm the text lands unsubmitted (no Enter). Fallback if any CLI submits or mangles it: shorter template plus a documented caveat in the PR. Record results in the PR description and adjust 1.3. (R1)
- [x] 1.2 Modify `T/conexionPty.js` (near lines 87-89): expose a `pegar(texto)` path that returns `false` when `conexion.abierta()` is false, otherwise calls `term.paste`. Strip C0 control characters and DEL. (D1, D2)
- [x] 1.3 Modify `T/PanelTerminal.jsx` (line ~49): `escribirRef` exposing the paste; sanitize id with `/^\d{1,12}$/`; template "Revisa la pizarra de la sesión ${id}: léela con pizarra_leer y marca cada paso con pizarra_marcar según las reglas de tutor."; order vaciar → paste → focus; no Enter. (D1, D2; TS Revisar sends, Not connected)
- [x] 1.4 Modify `T/PanelSesion.jsx`: "Revisar" button wired to `escribirRef`; when not connected show "La terminal no está conectada: no se escribió el pedido." and write nothing. (D2; TS Not connected)
- [x] 1.5 Verify: `pnpm build`, `git checkout -- desktop/ui/dist/.gitkeep`; `go vet ./...`, `GOOS=linux go vet ./...`, `go test ./...`. Manual: M.1, M.2.
- [x] 1.6 Open PR1 (base `master`) with the chain diagram, R1 results, M.1, M.2. (#41)

## PR2: Marks domain (branch `feat/bythos-terminal-fase3-02-marcas-dominio`, base `feat/bythos-terminal-fase3-01-revisar`)

Satisfies: WT pizarra_marcar (validation, expansion, caps), WB Scene Limits delta (reserved prefix) (D3 to D9). Threat: prompt injection (`limpiarTexto`), scene injection (reserved `marca-` prefix).

Split in two: 02 (validation) and 02b (expansion).

- [x] 2.1 RED: create `D/pizarra/marcas_test.go`: `ValidarMarcas` rejects >20 marks, `pista` >300 runes, `pasaje` >200, `pagina` outside 1..100000, unknown veredicto, empty targets; `limpiarTexto` strips controls. (D9)
- [ ] 2.2 RED: ids and expansion: `ExpandirMarcas` yields exactly 2 `agregar` ops per mark (frame and pill); id is `marca-` + base64url(sha256(sesion|clave))[:10] + `-m`/`-t`; replace bumps version to prev+1; ops carry `Datos {bythos:"marca", veredicto, clave}`. (D3, D4, D5)
- [ ] 2.3 RED: geometry and text: frame = bounding box + 6 px pad, stroke 2; pill alto 20 wrapped at 48 runes; verdict and citation lines never truncated (citation ≤73 runes, ≤2 lines); pista ≤10 lines and pasaje ≤6 lines cut with "…"; max 19 lines; colors ok #2f9e44, error #e03131, parcial #f08c00; citation "<título>, pág. N" with title cut to 60 runes with "…". (D6, D8)
- [x] 2.4 RED: `Cajas` in `D/pizarra/resumen_test.go`: ids resolve to boxes from the saved scene, unknown id errors, an area target is used as given. `Validar` rejects `datos` from the agent and agregar ids with the `marca-` prefix in `D/pizarra/ops_test.go`. (D4, D7) The area-target case of `Cajas` is tested in 02b with `ExpandirMarcas`.
- [x] 2.5a Create `D/pizarra/marcas.go` (types, `ValidarMarcas`, `limpiarTexto`). (D9) [PR 02]
- [ ] 2.5b Create `D/pizarra/marcas_expandir.go` (wrapping, ids, geometry, `ExpandirMarcas`, `Describir`). (D3 to D8) [PR 02b]
- [x] 2.6 Modify `D/pizarra/resumen.go` (`Cajas`) and `D/pizarra/ops.go` (`Elemento.Datos`, reserved prefix check in `Validar`). (D4, D7)
- [ ] 2.7 Verify: `go vet ./...`, `GOOS=linux go vet ./...`, `go test ./...`.
- [ ] 2.8 Open PR2 (base PR1 branch).

## PR3: Marks UI, Quitar marcas, forged-mark guard (branch `feat/bythos-terminal-fase3-03-marcas-ui`, base `feat/bythos-terminal-fase3-02-marcas-dominio`)

Satisfies: WB Quitar marcas, Forged mark; WH tagging round trip (D4, D10, D11). Threat: scene injection.

- [ ] 3.1 Runtime checks R2 and R4 (first task): (R2) an element created with `customData` through `convertToExcalidrawElements` / `updateScene` survives save and reload; fallback: identify marks by the `marca-` id prefix only. (R4) the glyphs used in the pill render; fallback: ASCII. Record results in the PR. (R2, R4)
- [ ] 3.2 RED: in `D/api/sesiones_test.go` add the D11 cases: scene PUT with `customData.bythos=="marca"` and a sha256 key -> 400, including when the sha is a permitted attachment; exact-key check for `sha256` and `bythos`; legacy attachment cards still accepted. (D11; WB Forged mark)
- [ ] 3.3 Modify `validarElemento` in `D/api/sesiones.go` (line ~369) to implement D11. (D11)
- [ ] 3.4 Modify `T/opsPizarra.js`: map `datos` to `customData`; on replace of an existing `marca-` id bump the element version to prev+1. (D3, D5)
- [ ] 3.5 Modify `T/Pizarra.jsx`: "Quitar marcas" button, disabled when no marks; removes tagged elements via `updateScene` with `captureUpdate: IMMEDIATELY`; NOT recorded in the agent history; Ctrl+Z restores. (D10; WB Quitar marcas, Undo removal)
- [ ] 3.6 Verify: `pnpm build`, `git checkout -- desktop/ui/dist/.gitkeep`; Go commands. Manual: M.3 to M.5.
- [ ] 3.7 Open PR3 (base PR2 branch) with R2, R4 results and M.3 to M.5.

## PR4: Marcar route and MCP tool (branch `feat/bythos-terminal-fase3-04-marcar-mcp`, base `feat/bythos-terminal-fase3-03-marcas-ui`)

Satisfies: WT pizarra_marcar, MODIFIED pizarra_dibujar, citation rules (D3, D7, D8, D19). Threat: HTTP routing, prompt injection, PTY write N/A server side.

- [ ] 4.1 RED: `D/api/pizarra_test.go`: valid mark call enqueues the 2-op batch and returns 200; foreign Origin -> 403 and empty Origin accepted on Agente routes only; citation resource outside the session folder -> 400; session without folder with citation -> 400; >20 marks -> 400; unknown target id -> 400; batch uses IMMEDIATELY capture. (D7, D8, D19)
- [ ] 4.2 Modify `D/api/pizarra.go`: `marcarPizarra` handler; refactor shared `encolarYEsperar`; citation resolution against the session folder (title "<título>, pág. N"). Wire in `D/api/server.go`. (D3, D8)
- [ ] 4.3 RED: `D/agentes/pizarra_test.go` (in-memory MCP client): `pizarra_marcar` happy path, schema caps, error messages, untrusted-data wording; `pizarra_dibujar` still rejects `datos` and `marca-` ids. (WT)
- [ ] 4.4 Modify `D/agentes/pizarra.go` (tool `pizarra_marcar`, calls the route) and `D/agentes/cliente.go` if the client call lives there. (WT)
- [ ] 4.5 Verify: Go commands. Manual: M.6.
- [ ] 4.6 Open PR4 (base PR3 branch) with M.6.

## PR5: Schema, ack, retention, GET cambios (branch `feat/bythos-terminal-fase3-05-historial-db`, base `feat/bythos-terminal-fase3-04-marcar-mcp`)

Satisfies: WH Storage, Ack payload, List Changes, Retention (D12, D13, D17, D18). Threat: HTTP routing.

- [ ] 5.1 RED: `D/db/pizarra_ops_test.go`: `migrarOpsHistorial` adds nullable `inversa`, `versiones`, `deshecho` columns (PRAGMA first), idempotent on an old DB; ack stores inversa/versiones; over-cap payload stores NULL. (D12, D13)
- [ ] 5.2 RED: retention: keep 50 newest rows with an inverse; separately keep 50 newest vencida/fallida/aplicada-without-inverse; pending rows never pruned; batches without undo do not evict undoable ones. (D18; WH Retention)
- [ ] 5.3 RED: `ListarCambios` returns `[{id, creado, tipo: dibujo|marcas, elementos, estado}]` with estado `deshacible|deshecho|sin_deshacer`; fallida and pendiente hidden; vencida -> `sin_deshacer`. (D17)
- [ ] 5.4 Modify `D/db/pizarra_ops.go`: columns, migration, ack storage, 2-cap retention, `ListarCambios`. (D12, D17, D18)
- [ ] 5.5 RED: `D/api/pizarra_test.go`: ack with `inversa {restaurar, quitar}`, `versiones {id:[version, nonce]}` and `sin_deshacer` persists; ack above 2 MiB read via its own reader stores NULL and still completes the op; `GET cambios` guarded by `conVentanaLectura` (cross-site -> 403). (D13, D17)
- [ ] 5.6 Modify `D/api/pizarra.go` (ack with the 2 MiB reader, `GET sesiones/{id}/pizarra/cambios`) and wire in `D/api/server.go`. (D13, D17)
- [ ] 5.7 Verify: Go commands.
- [ ] 5.8 Open PR5 (base PR4 branch).

## PR6: Inverse, Vitest, CI (branch `feat/bythos-terminal-fase3-06-inversa-vitest`, base `feat/bythos-terminal-fase3-05-historial-db`)

Satisfies: WH Inverse computation, Conflict check, Partial undo count (D13 to D15, D21).

- [ ] 6.1 Add Vitest: devDependency and `"test": "vitest run"` (node environment) in `UI/package.json` and the lockfile; confirm `pnpm test` runs with zero tests. (D21)
- [ ] 6.2 RED: create `T/opsPizarra.test.js` (`vi.mock` the adapter): `calcularInversa` cases (added -> quitar; changed -> restaurar previous; removed -> restaurar), identity diff, no-op batch, marks batch; `planDeshacer` conflict check by version+nonce, container grouped with its bound texts as one unit, `omitidos` counts elements, messages "1 elemento no se restauró porque lo cambiaste" / "N elementos no se restauraron porque los cambiaste"; 512 KiB cap sets `sin_deshacer`. (D13 to D15)
- [ ] 6.3 Modify `T/opsPizarra.js`: `calcularInversa`, `planDeshacer`. (D14, D15)
- [ ] 6.4 Modify `T/usePizarraAgente.js`: compute the inverse before applying, send `inversa`, `versiones`, `sin_deshacer` in the ack; enforce the UI cap of 512 KiB. Add the ack fields in `S/api.js` if the call is defined there. (D13)
- [ ] 6.5 Modify `.github/workflows/build.yml`: add a `pnpm test` step after `pnpm build`. (D21)
- [ ] 6.6 Verify: `pnpm test`, `pnpm build`, `git checkout -- desktop/ui/dist/.gitkeep`; Go commands. Manual: M.7.
- [ ] 6.7 Open PR6 (base PR5 branch); note the CI step is part of the Vitest approval.

## PR7: POST deshacer (branch `feat/bythos-terminal-fase3-07-deshacer-api`, base `feat/bythos-terminal-fase3-06-inversa-vitest`)

Satisfies: WH Undo, Claim, Release, Board not open (D16). Threat: HTTP routing.

- [ ] 7.1 RED: `D/api/pizarra_test.go`: `POST .../cambios/{id}/deshacer` with no Origin -> 403; returns `{inversa, versiones}` and sets `deshecho`; second call -> 409; op without inverse -> 409; board without heartbeat (not `montada`) -> 409 and the batch stays undoable; `{"liberar":true}` clears `deshecho` and logs "deshacer no aplicado"; unknown id/other session -> 404. (D16)
- [ ] 7.2 RED: `D/db` claim is atomic (two concurrent claims, one wins) and release is idempotent. (D16)
- [ ] 7.3 Implement claim/release in `D/db/pizarra_ops.go` and the handler in `D/api/pizarra.go`; add `AccionPizarraDeshecha` in `D/db/eventos.go`; wire in `D/api/server.go`. (D16)
- [ ] 7.4 Modify `S/api.js`: `listarCambios`, `deshacerCambio`, `liberarCambio`. (D16, D17)
- [ ] 7.5 Verify: Go commands.
- [ ] 7.6 Open PR7 (base PR6 branch).

## PR8: History panel (branch `feat/bythos-terminal-fase3-08-historial-ui`, base `feat/bythos-terminal-fase3-07-deshacer-api`)

Satisfies: WH History panel, Undo flow, Partial undo, Board not open, Apply fails after claim (D16, D17, D19).

- [ ] 8.1 Runtime check R3 (first task): applying an agent batch and an undo with the capture mode interplay; confirm Ctrl+Z and the undo do not corrupt each other. Fallback: keep IMMEDIATELY for agent batches (D19). Record in the PR. (R3)
- [ ] 8.2 Create `T/HistorialPizarra.jsx` (presentational): list with kind, element count, estado, "Deshacer" only for `deshacible`. (D17)
- [ ] 8.3 Create `T/useHistorialPizarra.js` (container): offered only while the board is mounted; if `excalidrawApi` is missing show "Abre la pizarra para deshacer" and send nothing; claim via `deshacerCambio`; `planDeshacer`; apply with IMMEDIATELY; on throw post `liberar`; show partial-undo message. (D16)
- [ ] 8.4 Modify `T/Pizarra.jsx`: history toggle. (D16)
- [ ] 8.5 Verify: `pnpm test`, `pnpm build`, `git checkout -- desktop/ui/dist/.gitkeep`; Go commands. Manual: M.8 to M.11.
- [ ] 8.6 Open PR8 (base PR7 branch) with R3 and M.8 to M.11.

## PR9: Tutor rules, docs, regeneration (branch `feat/bythos-terminal-fase3-09-reglas-tutor`, base `feat/bythos-terminal-fase3-08-historial-ui`)

Satisfies: TR (4 requirements, 12 scenarios), WT Instructions (D20). Threat: prompt injection (rule wording).

- [ ] 9.1 RED: `D/workspace/workspace_test.go` asserts `ReglaTutor` and the generated AGENTS.md/CLAUDE.md contain "pizarra_marcar", "20", "carpeta de la sesión", "No puedes deshacer", "confirma"; the Pizarra tool list names `pizarra_marcar`; phase-2 rules remain. (D20; TR)
- [ ] 9.2 RED: regeneration test: a stale AGENTS.md is rewritten and the user file `notas.md` in the workspace is preserved. (Open question: files are fully managed, so test file preservation.) (TR Regeneration)
- [ ] 9.3 RED: `D/workspace/instrucciones_mcp_test.go` asserts `agentes.Instrucciones` appends `ReglaTutor`. (D20)
- [ ] 9.4 Modify `D/workspace/workspace.go`: rewrite `ReglaPizarra` in the neutral register (marks ≤20 per call, citations only from the session folder, agent cannot undo), add `ReglaTutor` as rule 5 following the outline, add the Pizarra tool list. (D20)
- [ ] 9.5 Modify `D/agentes/servidor.go`: append `ReglaTutor` to `Instrucciones`. (D20)
- [ ] 9.6 Modify `docs/AGENTES.md` with the matching copy. (D20)
- [ ] 9.7 Verify: Go commands; `pnpm test`, `pnpm build`, `git checkout -- desktop/ui/dist/.gitkeep`. Manual: M.12, M.13.
- [ ] 9.8 Open PR9 (base PR8 branch) with M.12, M.13.

## Manual verification checklist (real app, Windows)

Tick in each PR description and again here.

- [ ] M.1 (PR1) R1: Revisar pastes the template unsubmitted into Claude Code, Codex, OpenCode and pwsh; fallback recorded if not.
- [ ] M.2 (PR1) With the terminal disconnected, Revisar writes nothing and shows the aviso; the ID is never outside `\d{1,12}`.
- [ ] M.3 (PR3) R2: a mark's `customData` survives autosave and restart; forged mark PUT is rejected (400).
- [ ] M.4 (PR3) R4: pill glyphs render correctly; fallback recorded if ASCII.
- [ ] M.5 (PR3) "Quitar marcas" removes all marks, is disabled with none, and Ctrl+Z restores them.
- [ ] M.6 (PR4) From the embedded terminal, the agent marks 3 steps with a citation from the folder; frames and pills appear; a citation outside the folder is refused.
- [ ] M.7 (PR6) An agent drawing batch applies and the ack carries an inverse (inspect `GET cambios`).
- [ ] M.8 (PR8) R3: agent batch then undo from the panel; no interference with Ctrl+Z.
- [ ] M.9 (PR8) Undo a marks batch and a drawing batch; states move to `deshecho`; a second click is not offered.
- [ ] M.10 (PR8) Edit an agent element, then undo: partial message with the right singular/plural and count.
- [ ] M.11 (PR8) Board closed: the panel is not offered; with the board unmounted the API returns 409 and the batch stays undoable.
- [ ] M.12 (PR9) After restart, AGENTS.md and CLAUDE.md contain the tutor rules; `notas.md` is preserved.
- [ ] M.13 (final) `go vet ./...`, `GOOS=linux go vet ./...`, `go test ./...`, `pnpm test`, `pnpm build` pass on the stacked tip; end-to-end Revisar → marks → undo works.

## Chain order and boundaries

```
master
  └─ PR1 feat/bythos-terminal-fase3-01-revisar
       └─ PR2 feat/bythos-terminal-fase3-02-marcas-dominio
            └─ PR3 feat/bythos-terminal-fase3-03-marcas-ui
                 └─ PR4 feat/bythos-terminal-fase3-04-marcar-mcp
                      └─ PR5 feat/bythos-terminal-fase3-05-historial-db
                           └─ PR6 feat/bythos-terminal-fase3-06-inversa-vitest
                                └─ PR7 feat/bythos-terminal-fase3-07-deshacer-api
                                     └─ PR8 feat/bythos-terminal-fase3-08-historial-ui
                                          └─ PR9 feat/bythos-terminal-fase3-09-reglas-tutor
```

Each child PR description carries this diagram with the current PR marked, plus start, end, prior dependencies, follow-up work and out-of-scope items (chained-pr). Stacked-to-main: after a parent merges (merge commit), retarget the next PR to `master`. Commits follow work-unit-commits: code and its tests in the same commit; conventional commits, no AI attribution. Rollback is reverse order. Never ship PR3 reverted while PR4 stays (`datos` would be dropped).
