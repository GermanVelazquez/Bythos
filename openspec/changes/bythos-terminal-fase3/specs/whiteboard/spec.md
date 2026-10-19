# whiteboard Specification (delta)

Note: this delta builds on the phase-2 version of "Scene Limits" (attachment references, empty `files`), which is not yet archived.

## ADDED Requirements

### Requirement: Mark Elements
Marks MUST be ordinary scene elements: a frame and a standalone text pill, each tagged `customData.bythos="marca"` with its verdict. Marks MUST be saved with the scene, MUST reload identically after restart, and MUST be visible in the agent's board capture. The user MUST be able to move, edit, and delete marks like any other element. A mark MUST be identified by its `customData` tag, not by its id, so that copied and pasted marks stay recognizable. A mark MUST NOT carry file or attachment references.

#### Scenario: Marks survive reload
- GIVEN a board with three marks
- WHEN the app restarts and the session opens
- THEN the same frames and pills are displayed with the same colors and text

#### Scenario: Tag survives paste
- GIVEN a mark copied and pasted by the user
- WHEN the scene is inspected
- THEN the pasted frame and pill still carry `customData.bythos="marca"`

#### Scenario: User edits a mark
- GIVEN a mark on the board
- WHEN the user moves or deletes it
- THEN the change is saved like any other element edit

#### Scenario: Mark in capture
- GIVEN a board with marks
- WHEN `pizarra_captura` runs
- THEN the frames and pills are visible in the returned image

### Requirement: Quitar marcas
The board MUST offer a "Quitar marcas" action that removes every mark element in one batch and MUST NOT touch any other element. It is a user action: it MUST be undoable with the editor's Ctrl+Z and MUST NOT be recorded in the agent history. When the board has no marks, the action MUST be unavailable or do nothing.

#### Scenario: Remove all marks
- GIVEN a board with marks and the student's own drawing
- WHEN the user presses "Quitar marcas"
- THEN all marks disappear and the student's drawing is unchanged

#### Scenario: Undo removal with Ctrl+Z
- GIVEN marks were removed with "Quitar marcas"
- WHEN the user presses Ctrl+Z
- THEN the marks return and the agent history has no entry for the removal

#### Scenario: No marks
- GIVEN a board with no marks
- WHEN the user opens the board actions
- THEN "Quitar marcas" is disabled or has no effect

## MODIFIED Requirements

### Requirement: Scene Limits
The API MUST reject scene bodies above 20 MiB, leaving the stored scene unchanged. The API MUST accept an image or document-card element only when it references an attachment of the same session, and MUST reject any other reference with 400, leaving the stored scene unchanged. The persisted scene MUST keep `files` empty: file bytes MUST NOT be stored in the scene. The API MUST accept mark elements (rectangles and text tagged `customData.bythos="marca"`) and MUST reject a mark element that carries a `sha256` key or a case-variant of a reserved key, with 400 and the stored scene unchanged.
(Previously: marks were not mentioned; untagged-`customData` rules applied only implicitly.)

#### Scenario: Oversized
- GIVEN a scene body above 20 MiB
- WHEN it is saved
- THEN 413 is returned and the previous scene is intact

#### Scenario: Image of own attachment
- GIVEN a scene whose image element references an attachment of this session
- WHEN it is saved
- THEN the save succeeds

#### Scenario: Foreign reference
- GIVEN a scene whose image element references an attachment of another session
- WHEN it is saved
- THEN 400 is returned and the previous scene is intact

#### Scenario: Bytes stripped
- GIVEN a save body that carries a non-empty `files` map
- WHEN it is saved
- THEN the stored scene has `files` empty and no file bytes

#### Scenario: Legacy scene
- GIVEN a scene saved in phase 1 with no image or card elements
- WHEN it loads and is saved again
- THEN it works unchanged

#### Scenario: Scene with marks
- GIVEN a scene with tagged frames and text pills
- WHEN it is saved
- THEN the save succeeds and the marks reload

#### Scenario: Forged mark
- GIVEN a mark element whose `customData` carries a `sha256` key, even one that names an attachment of this session
- WHEN the scene is saved
- THEN 400 is returned and the previous scene is intact
