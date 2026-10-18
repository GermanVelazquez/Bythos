# whiteboard-agent-tools Specification

## Purpose
MCP tools that let the user's agent read, capture, and edit the whole whiteboard of the active session.

## Requirements

### Requirement: Active Session
Each tool MUST accept an optional session id and default to the active session (`BYTHOS_SESION` or the UI-selected one).

#### Scenario: Default
- GIVEN session 3 is active and no id is passed
- WHEN a tool runs
- THEN it acts on session 3

#### Scenario: Unknown id
- GIVEN an explicit `sesion_id` N that does not exist
- WHEN a tool runs
- THEN it fails with 404 "La sesión N no existe"

#### Scenario: Stale hint
- GIVEN `BYTHOS_SESION` points at a deleted session and no explicit id is passed
- WHEN a tool runs
- THEN the hint is ignored and resolution falls through to the active session

### Requirement: pizarra_leer
MUST return the full scene as text (all elements, not a selection), read from the saved scene, and MUST work when the UI is closed. Output MUST be capped at 2,000 elements, with `truncado: true` when the cap is hit.

#### Scenario: Truncation
- GIVEN a saved scene with 2,500 elements
- WHEN `pizarra_leer` runs
- THEN 2,000 elements are returned and `truncado` is true

#### Scenario: UI closed
- GIVEN a saved scene and no open window
- WHEN `pizarra_leer` runs
- THEN the saved elements are returned

### Requirement: pizarra_captura
MUST return an ImageContent plus a text block, MUST NOT return `structuredContent`, and MUST fail with a clear error when the Terminal view is not mounted.

#### Scenario: Mounted
- GIVEN the Terminal view is open
- WHEN `pizarra_captura` runs
- THEN the result has one image and one text item and no `structuredContent`

#### Scenario: Not mounted
- GIVEN the Terminal view is closed
- WHEN it runs
- THEN an error says to open the Terminal view, with no image

### Requirement: pizarra_dibujar
MUST support create, edit, organize, and delete of any element on the whole board. Ops MUST persist in `terminal_pizarra_ops`, be applied by element id, and be acked. A call MUST carry at most 200 ops; a larger batch MUST be rejected with nothing enqueued.

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

### Requirement: Delete Rules
Board element deletes MUST be allowed to the agent. Library (file) deletes MUST stay forbidden. AGENTS.md, CLAUDE.md, and MCP Instructions MUST state both rules.

#### Scenario: Board delete
- GIVEN element E exists
- WHEN the agent deletes E
- THEN E is removed

#### Scenario: Instructions
- GIVEN generated workspace files
- WHEN inspected
- THEN they allow whiteboard deletes and forbid library deletes
