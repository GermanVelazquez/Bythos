```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:11ebc6843b21a8d11209f702a5ef175a62aafb616b6265c3f5b189ffa3bfe761
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 21/21
scenarios: 71/71
test_command: "cd desktop && go vet ./... && GOOS=linux go vet ./... && go test -count=1 ./..."
test_exit_code: 0
test_output_hash: sha256:bf5224eca0ab4a10f88c45f2520aef3bd13974ad61d8d5c426414a2e1887a951
build_command: "cd desktop/ui && pnpm build"
build_exit_code: 0
build_output_hash: sha256:c0261532ada04a6e776648352564c590569951dce6e6a5c17cb0986f0c433ace
```

# Verification Report: bythos-terminal-fase2

Branch `feat/bythos-terminal-fase2-05b-ui-acciones` (tip of the stacked chain, HEAD `6f2ae75`). Mode: openspec, Strict TDD off, no JS test runner.

## Verdict: PASS WITH WARNINGS

0 CRITICAL, 3 WARNING, 4 SUGGESTION. All automated checks are green. The UI half of many scenarios depends on the manual checklist (M.1 to M.15, none ticked yet). Envelope note: `scenarios: 71/71` counts every scenario with accepted evidence under the task contract (43 fully covered by a passing Go test, 28 resting wholly or partly on manual checks M.x, which remain pending). Strictly by automated evidence the figure is 43/71.

## Completeness

Spec totals: 21 requirements, 71 scenarios (whiteboard-attachments 12/36, whiteboard 2/7, whiteboard-agent-tools 6/23, terminal-sessions 1/5).

Every implementation, RED and PR-opening task is checked. Unchecked tasks are the per-PR "Verify" tasks that bundle commands with a manual checklist:

| Task | Content |
|---|---|
| 3a.6 | pnpm build, go vet/test, manual M.1 to M.3 |
| 3b.4 | build + manual M.4 to M.7 |
| 4a.6 | go vet (both OS), go test |
| 4b.5 | go vet, go test, manual M.8, M.9 |
| 5b.5 | build, go vet/test, manual M.10 to M.15 |

The automated half of all five now passes (see Commands). See W1.

## Commands

| Command (cwd) | Result |
|---|---|
| `go vet ./...` (desktop) | exit 0, no output |
| `GOOS=linux go vet ./...` (desktop) | exit 0, no output |
| `go test -count=1 ./...` (desktop) | all 10 packages ok (root, agentes, api, archivos, db, lan, pizarra, terminal, ventana, workspace). Known flakes did not occur; no rerun needed. |
| `pnpm build` (desktop/ui) | exit 0, built in 14.60s (only the existing chunk-size warning) |
| `git checkout -- desktop/ui/dist/.gitkeep` | done; git status clean |

## Scenario coverage

Legend: T = passing Go test; PART = server side tested, UI side manual; M = manual only; CODE = implemented, no test.

### whiteboard-attachments

| Requirement / Scenario | Evidence |
|---|---|
| Entry Paths / Paste image | M.1 (Pizarra.jsx:126 capture handlers) |
| Drop file | M.3 |
| Adjuntar | M.5 (Pizarra.jsx:127) |
| Pick folder resource | PART: TestAdjuntoDesdeRecurso; picker UI M.12 |
| Session without folder | PART: TestAdjuntoDesdeRecurso (409); UI hidden when no folder (Pizarra.jsx:130), M.12 |
| Allowed Types / Allowed document | T TestAdjuntoTiposPermitidosYRechazados |
| Disallowed type | PART: same test (415); UI message M.3 |
| Spoofed extension | T TestAdjuntoTiposPermitidosYRechazados |
| Size Limit / At the limit | T TestAdjuntoLimiteReal25MiB |
| Oversize | PART: TestAdjuntoLimiteDeTamano, TestAdjuntoLimiteReal25MiB (413); UI message M.3 |
| Upload Endpoint / Upload from the window | T TestAdjuntoTiposPermitidosYRechazados, TestAdjuntoContenidoYTexto |
| Foreign origin | T TestAdjuntoGuardas |
| Unknown session | T TestAdjuntoSesionAjenaODesconocida |
| Duplicate content | T TestAdjuntoDuplicadoYSesiones |
| Session Privacy / Not in library | CODE (separate table; no recursos row is created; TestAdjuntoGuardarEnCarpeta asserts no resource without the explicit action) |
| Other session | T TestAdjuntoSesionAjenaODesconocida, TestAdjuntoDuplicadoYSesiones |
| Same bytes in two sessions | T TestBorrarSesionConservaArchivosYOtrasSesiones, TestAdjuntoDuplicadoYSesiones |
| Image Elements / Downscale | M.2 (client adjuntos.js) |
| Draw on top | M.7 |
| Scene stays small | PART: TestPizarraPutConAdjuntosPropiosAjenosYBytes (files stripped server side); client PUT body M.2 |
| Document Cards / Card appears | M.10 |
| Open in viewer | M.10 (Pizarra.jsx:158 onLinkOpen, toolbar "Abrir") |
| Card in capture | M.10 (spike d mitigation: rectangle + bound text) |
| Rehydration / Restart | M.4 |
| Missing blob | CODE (useAutoguardado.js), M.4 |
| Foreign Attachment Reference / Own | T TestReducirEscenaValidaReferenciasContraAdjuntos, TestPizarraPutConAdjuntosPropiosAjenosYBytes |
| Foreign fileId | T same |
| Unknown fileId | T same |
| Guardar en carpeta / Manual save | T TestAdjuntoGuardarEnCarpeta |
| No automatic save | T TestAdjuntoGuardarEnCarpeta |
| Session without folder | T TestAdjuntoGuardarSinCarpetaOAjeno (409); UI hidden M.11 |
| Duplicate in folder | T TestAdjuntoGuardarEnCarpeta (200 with existing) |
| Remove Attachment / Remove | PART: TestPizarraLeerListaSoloAdjuntosVivos, TestAgenteSoloLeeLoQueEstaEnLaPizarraGuardada; UI M.13. See S1 |
| Library untouched | M.13 / M.14 |
| Failure Handling / Server error | M.5 (Pizarra.jsx:46 catch adds nothing) |
| Pasting plain text | M.3 (Pizarra.jsx:97 passes through) |

### whiteboard

| Scenario | Evidence |
|---|---|
| Image accepted | M.1 to M.3 |
| Disallowed file dropped | M.3 |
| Oversized | T TestPizarraCuerpoGrandeDa413 |
| Image of own attachment | T TestReducirEscenaValidaReferenciasContraAdjuntos |
| Foreign reference | T same, TestPizarraPutConAdjuntosPropiosAjenosYBytes |
| Bytes stripped | T TestPizarraPutConAdjuntosPropiosAjenosYBytes |
| Legacy scene | T TestReducirEscenaValidaReferenciasContraAdjuntos, TestPizarraEscenaGetPutYConflicto |

### terminal-sessions

| Scenario | Evidence |
|---|---|
| Delete | T TestSesionesCrearListarPatchBorrar |
| Delete with attachments | T TestBorrarSesionCascadeaVinculosYConservaArchivo (db), TestBorrarSesionConservaArchivosYOtrasSesiones |
| Shared content survives | T TestBorrarSesionConservaArchivosYOtrasSesiones, TestArchivoConVinculoNoSePuedeBorrar, TestContarReferenciasArchivo |
| Library resource survives | PART: no test deletes a session after Guardar en carpeta; resources have no FK to sessions. M.14 (S3) |
| Close tab | CODE (no API call on close) |

### whiteboard-agent-tools

| Requirement / Scenario | Evidence |
|---|---|
| leer_imagen / Read image | T TestLeerImagenDevuelveImagenYFicha, TestAgenteImagenEscalaYLlevaCabeceras |
| UI closed | T TestAgenteImagenEscalaYLlevaCabeceras (no latido) |
| Not an image | T TestAgenteTipoEquivocado, TestLeerAdjuntosErroresClaros |
| Foreign or unknown id | T TestAgenteSoloLeeLoQueEstaEnLaPizarraGuardada, TestLeerAdjuntosErroresClaros |
| Large image | T TestImagenParaAgenteEscalaAl1280 |
| leer_archivo / Text document | T TestLeerArchivoTextoYTruncado, TestAgenteTextoDeTarjetaYTruncado |
| Truncation | T TestLeerArchivoTextoYTruncado, TestPlanoTruncaA40000Runas |
| PDF | T TestLeerArchivoPDFSoloMetadataYRuta, TestAgentePDFSoloDaMetadataYRuta |
| Image attachment | T TestAgenteTipoEquivocado, TestLeerAdjuntosErroresClaros |
| Foreign or unknown id | T TestLeerAdjuntosErroresClaros |
| Extraction failure | PART: TestLeerAdjuntosErroresClaros (error path); no corrupt-docx fixture (S2) |
| Agent Reads On Request / Paste does not notify | CODE (no push path exists); M.9 |
| Attachment Tools Instructions / Instructions | T TestInstruccionesNombranLasHerramientasDeAdjuntos, TestInstruccionesTraenLaReglaDeLaPizarraYLaDeLaBiblioteca |
| pizarra_leer / Truncation | T TestResumirTruncaA2000ElementosYMarcaTruncado |
| UI closed | T TestPizarraLeerSinLatidoDevuelveLaEscenaGuardada |
| Lists attachments | T TestPizarraLeerListaSoloAdjuntosVivos |
| No attachments | T same |
| pizarra_dibujar / Draw, Too many ops, Concurrent edit, Board not open | T (phase 1) TestOpsAckDentroDeLaEsperaDevuelveAplicadaConIds, TestOps201OperacionesDa400YNoEncola, TestOpsSesionNoMontadaDa409YNoEncola |
| Agent inserts image | T TestPizarraDibujarSigueRechazandoImagenSinEncolar |
| Agent deletes an image | CODE (phase-1 delete path; no image-specific test) (S4) |

## Design coherence

| Decision | Status |
|---|---|
| D1 link table, no CASCADE to archivos | Matches (TestArchivoConVinculoNoSePuedeBorrar) |
| D2 orphan count includes links | Matches (TestContarReferenciasArchivo) |
| D4 Quitar keeps the link row | Matches code; see S1 on the spec wording |
| D5 agent reads only what the saved scene references | Matches (TestAgenteSoloLeeLoQueEstaEnLaPizarraGuardada) |
| D9 fail-closed reducirEscena | Matches (table test) |
| D11/D12 upload limits, servirArchivo reuse, guards | Matches |
| D13/D14/D15 agent tools, 40,000 runes, untrusted-data prefix, no StructuredContent | Matches |
| D10 onPaste guard, D8 insert order, D7 rehydration | Matches code; behavior M.4, M.6 |

No contradiction between code and spec or design found.

## Findings

### CRITICAL
None.

### WARNING

- W1 `openspec/changes/bythos-terminal-fase2/tasks.md` (tasks 3a.6, 3b.4, 4a.6, 4b.5, 5b.5 unchecked): the automated halves now pass; the manual halves (M.1 to M.13) are still unrun. Tick 4a.6 and the automated parts now; the rest after the manual pass.
- W2 `desktop/ui/src/terminal/Pizarra.jsx:44-160` and `desktop/ui/src/terminal/useAutoguardado.js`: the UI half of roughly 23 scenarios (paste, drop, Adjuntar, downscale, draw-over, cards, viewer, rehydration, missing blob, Remove, Failure Handling, whiteboard Image accepted / Disallowed dropped) has no automated check and rests entirely on M.1 to M.13 being executed. These are the user-facing core of the change.
- W3 `desktop/api/adjuntos.go:471` (known follow-up): `desde-recurso` rejects only video through a blacklist, so any other library resource type (even one the upload route would reject with 415) is attachable. Documented and accepted; listed for completeness.

### SUGGESTION

- S1 `desktop/api/adjuntos.go:202-213` vs the "Remove" scenario: the window-facing `GET sesiones/{id}/adjuntos` lists every link, including removed ones; only the agent-facing listing (pizarra_leer) filters by the saved scene. D4 and the requirement text ("MAY remain") make this acceptable; consider clarifying the scenario wording.
- S2 `desktop/agentes/adjuntos_test.go:147`: add a corrupt-docx fixture to cover "Extraction failure" fully.
- S3 `desktop/api/sesiones_test.go:300`: extend the session-delete test with a resource created by Guardar en carpeta to automate "Library resource survives" (currently only M.14).
- S4 `desktop/agentes/adjuntos_test.go:170`: add a test for deleting an image element by id via pizarra_dibujar to pin "Agent deletes an image".

### Known follow-ups (not failures)

- desde-recurso uses only a video blacklist (W3).
- Deleting a session does not garbage-collect blobs (accepted in the design, D1/D2).
- The pre-existing FK pragma is not set in the DSN; the session-delete cascade is verified by tests (TestBorrarSesionCascadeaVinculosYConservaArchivo).

## Manual checks still pending

M.1 paste screenshot + link icon; M.2 PNG and >1440 px photo, PUT body without bytes; M.3 drop, mp4/svg, oversize, plain-text paste; M.4 restart survival; M.5 Adjuntar + failed upload; M.6 cross-session paste; M.7 stroke over image after restart, shared file across sessions; M.8 agent reads image/docx/PDF with the Terminal view closed; M.9 agent reads only on request, Quitar then "El adjunto N no está en la pizarra"; M.10 card open + capture; M.11 Guardar en carpeta; M.12 folder picker; M.13 Quitar and Ctrl+Z; M.14 delete session with attachments and library resource interplay. M.15 is satisfied by this run (vet x2 + tests green on the stack tip).
