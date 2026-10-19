# whiteboard-agent-tools Specification (delta)

## ADDED Requirements

### Requirement: pizarra_marcar
MUST let the agent mark the student's steps with `marcas`, each carrying an `objetivo` (element ids, or a rectangle `x`, `y`, `ancho`, `alto`), a `veredicto` of `ok`, `error` or `parcial`, an optional `pista`, and an optional `cita`. The server MUST validate the call and expand each mark into a colored frame plus a standalone text pill, both tagged `customData.bythos="marca"`. Colors MUST be green for `ok`, red for `error`, and amber for `parcial`. A call MUST carry at least 1 and at most 20 marks. `pista` MUST be at most 300 characters. A violation MUST reject the whole call with a validation error and nothing enqueued. The call MUST fail with the same 409 as `pizarra_dibujar` when the board is not open. Marks MUST be applied as one batch with the same persistence and ack as `pizarra_dibujar`.

#### Scenario: Mark a step
- GIVEN the board is open and element E holds a student step
- WHEN the agent marks E with `veredicto` "error" and a `pista`
- THEN a red frame around E and a text pill with the pista appear, and the op row has `estado=aplicada`

#### Scenario: Three verdicts
- GIVEN three targets
- WHEN the agent marks them `ok`, `parcial`, `error`
- THEN the frames are green, amber, and red respectively

#### Scenario: Too many marks
- GIVEN a call with 21 marks
- WHEN `pizarra_marcar` runs
- THEN it fails with a validation error and nothing is enqueued

#### Scenario: Exactly 20 marks
- GIVEN a call with 20 valid marks
- WHEN `pizarra_marcar` runs
- THEN all 20 are applied

#### Scenario: Pista too long
- GIVEN a mark whose `pista` has 301 characters
- WHEN `pizarra_marcar` runs
- THEN the whole call fails with a validation error and nothing is enqueued

#### Scenario: Invalid verdict or target
- GIVEN a mark with an unknown `veredicto`, or an `objetivo` naming an element that does not exist
- WHEN `pizarra_marcar` runs
- THEN the call fails with a validation error and nothing is enqueued

#### Scenario: Board not open
- GIVEN the target session's board is not open in the Terminal view
- WHEN the agent calls `pizarra_marcar`
- THEN the call fails with 409 and the same message as `pizarra_dibujar`, and nothing is enqueued

### Requirement: Mark Citation
A `cita` MUST be `{recurso_id, pagina?, pasaje?}`. The server MUST resolve `recurso_id` as a resource of the session's own folder and MUST reject any other resource, including one from another folder and one when the session has no folder, with a validation error and nothing enqueued. `pasaje` MUST be at most 200 characters. `pagina`, when present, MUST be a positive integer. The visible citation MUST be built by Bythos from the resolved resource title (for example "Unidad 3 -- Sistemas.pdf, pág. 12"); the agent MUST NOT be able to supply the displayed title. Bythos MUST NOT claim to have verified the page against the document.

#### Scenario: Resource in the folder
- GIVEN the session's folder holds resource R titled "Unidad 3 -- Sistemas.pdf"
- WHEN the agent marks with `cita` `{recurso_id: R, pagina: 12}`
- THEN the pill shows "Unidad 3 -- Sistemas.pdf, pág. 12"

#### Scenario: Resource outside the folder
- GIVEN resource X exists in the library but not in the session's folder
- WHEN the agent marks with `cita` `{recurso_id: X}`
- THEN the call fails with a validation error and nothing is enqueued

#### Scenario: Session without folder
- GIVEN the session has no folder
- WHEN the agent marks with any `cita`
- THEN the call fails with a validation error and nothing is enqueued

#### Scenario: Unknown resource
- GIVEN a `recurso_id` that does not exist
- WHEN the agent marks with it
- THEN the call fails with a validation error and nothing is enqueued

#### Scenario: Passage too long
- GIVEN a `pasaje` of 201 characters
- WHEN the agent marks
- THEN the call fails with a validation error and nothing is enqueued

#### Scenario: Agent cannot choose the title
- GIVEN the agent also sends a free-text title for the citation
- WHEN the call is validated
- THEN the title is rejected or ignored and the pill shows only the title of the resolved resource

#### Scenario: Mark without citation
- GIVEN a mark with `veredicto` "ok" and no `cita`
- WHEN `pizarra_marcar` runs
- THEN the mark is applied with no citation line

### Requirement: Mark Replacement
Mark element ids MUST be deterministic per target, so that marking the same target again replaces its previous mark instead of adding another. The replacement MUST belong to the same undoable batch as the new mark.

#### Scenario: Re-mark replaces
- GIVEN element E carries an `error` mark
- WHEN the agent marks E with `ok`
- THEN E has exactly one mark, now green, and the red frame and old pill are gone

#### Scenario: Other marks untouched
- GIVEN elements A and B are marked
- WHEN the agent re-marks only A
- THEN B's mark is unchanged

#### Scenario: Area target
- GIVEN a mark on the rectangle (x, y, ancho, alto)
- WHEN the agent marks the same rectangle again
- THEN the previous mark for that rectangle is replaced

### Requirement: Mark Text Is Data
Mark pills MUST be treated as agent output, never as instructions. AGENTS.md, CLAUDE.md, and MCP Instructions MUST describe `pizarra_marcar`, its caps, and the folder-only citation rule.

#### Scenario: Instructions
- GIVEN generated workspace files
- WHEN inspected
- THEN they name `pizarra_marcar`, the 20-mark cap, and the rule that citations come only from the session's folder

## MODIFIED Requirements

### Requirement: pizarra_dibujar
MUST support create, edit, organize, and delete of any element on the whole board, except that it MUST reject creating `image` elements and document cards. Ops MUST persist in `terminal_pizarra_ops`, be applied by element id, and be acked. A call MUST carry at most 200 ops; a larger batch MUST be rejected with nothing enqueued. The ack of every applied batch, from `pizarra_dibujar` or `pizarra_marcar`, MUST carry the ids of the affected elements, the inverse of the batch computed by the UI from the pre-batch state, and the post-batch version of each affected element. The server MUST store the inverse and the versions with the op row. The agent MUST NOT be able to read an inverse back as an undo command, and no agent tool MUST be able to undo a batch.
(Previously: the ack carried the affected ids only; no inverse or versions were stored.)

#### Scenario: Draw
- GIVEN the UI is open
- WHEN the agent adds a rectangle
- THEN it appears and its op row has `estado=aplicada`

#### Scenario: Too many ops
- GIVEN a call with 201 ops
- WHEN `pizarra_dibujar` runs
- THEN it fails with a validation error and nothing is enqueued

#### Scenario: Concurrent edit
- GIVEN the user moved element E while an op edits E
- WHEN the op applies
- THEN other elements are untouched and E is merged by id

#### Scenario: Board not open
- GIVEN the target session's board is not open in the Terminal view
- WHEN the agent draws
- THEN the call fails with 409 and the message "La pizarra de esta sesión no está abierta: abre la pizarra en la pantalla Terminal de Bythos y vuelve a intentarlo.", and nothing is enqueued

#### Scenario: Agent inserts image
- GIVEN the UI is open
- WHEN the agent sends an op that creates an `image` element
- THEN the call is rejected with a validation error and nothing is enqueued

#### Scenario: Agent deletes an image
- GIVEN an image element exists on the board
- WHEN the agent deletes it by id
- THEN the element is removed from the board

#### Scenario: Ack carries the inverse
- GIVEN the agent edits element E, which had a bound label
- WHEN the UI applies the batch and acks
- THEN the stored op row holds the ids, the inverse (covering E and its label), and the post-batch version of each

#### Scenario: Agent cannot undo
- GIVEN an applied batch with a stored inverse
- WHEN the agent looks for a way to undo it
- THEN no MCP tool offers undo, and the history route rejects requests that do not come from the app window

#### Scenario: Old rows
- GIVEN an op row created before this change
- WHEN it is read
- THEN it loads with no inverse and no versions, and nothing fails
