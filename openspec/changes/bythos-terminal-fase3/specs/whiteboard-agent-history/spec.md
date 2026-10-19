# whiteboard-agent-history Specification

## Purpose
Record every agent change to a session's board with enough data to undo it later, and let the user undo it safely from a history panel, even after a reload.

## ADDED Requirements

### Requirement: Inverse Capture
When the UI applies an agent batch, it MUST compute the inverse of the batch from the pre-batch scene, including bound labels and elements converted or version-bumped by the batch, and MUST send it with the post-batch version of each affected element in the ack. The server MUST persist the inverse and versions in nullable columns of `terminal_pizarra_ops`. The inverse MUST restore an edited element to its prior state, remove an added element, and re-add a deleted element with its bound label. The ack payload MUST NOT exceed a fixed size limit.

#### Scenario: Undo of an add
- GIVEN the agent added rectangle R
- WHEN the inverse is computed
- THEN it removes R

#### Scenario: Undo of an edit
- GIVEN the agent changed the color of a labeled shape S
- WHEN the inverse is computed
- THEN it restores S's prior properties and its label's prior properties

#### Scenario: Undo of a delete
- GIVEN the agent deleted a shape that had a bound label
- WHEN the inverse is computed
- THEN it re-adds the shape and the label, still bound

#### Scenario: Mark replacement
- GIVEN the agent re-marked a target that had a mark
- WHEN the inverse is computed
- THEN it removes the new mark and restores the old one

### Requirement: Lost or Oversize Ack
If the ack is lost, fails to arrive, or exceeds the size limit, the server MUST keep the op row without an inverse and the batch MUST have no undo. The history panel MUST show that batch as "sin deshacer" with no undo action. A lost ack MUST NOT cause any scene change and MUST NOT block later batches.

#### Scenario: Ack lost
- GIVEN the UI applied a batch but the ack request failed
- WHEN the history is listed
- THEN the batch appears as "sin deshacer" and offers no undo button

#### Scenario: Oversize ack
- GIVEN a batch whose inverse exceeds the size limit
- WHEN the ack is sent
- THEN the server stores the op without an inverse and the panel shows "sin deshacer"

#### Scenario: Later batches unaffected
- GIVEN one batch has no undo
- WHEN the next batch is applied and acked normally
- THEN the next batch is undoable

### Requirement: Retention
The server MUST keep the 50 most recent batches per session that have an inverse (undoable or `deshecho`) and MUST delete older ones. Batches without an inverse (`sin deshacer`, expired, or failed) MUST be capped separately at 50 per session and MUST NOT evict batches that have an inverse. A deleted batch MUST no longer be listed or undoable. Pending batches MUST NOT be pruned. Retention MUST be per session and MUST NOT affect other sessions.

#### Scenario: Fifty-first batch
- GIVEN a session with 50 batches that have an inverse
- WHEN a 51st batch with an inverse is acked
- THEN the oldest batch with an inverse is no longer listed and the other 50 remain

#### Scenario: Batches without undo do not evict
- GIVEN a session with 50 undoable batches
- WHEN 60 batches without an inverse are recorded
- THEN all 50 undoable batches remain undoable and only the 50 newest batches without an inverse are kept

#### Scenario: Other session
- GIVEN session A exceeds 50 batches
- WHEN session B's history is listed
- THEN B's batches are unchanged

### Requirement: List Changes
`GET sesiones/{id}/cambios` MUST return the session's batches newest first with id, time, kind (draw, mark), element count, and state (`undoable`, `deshecho`, `sin deshacer`). It MUST be protected by the window Origin guard and MUST return 404 for an unknown session. It MUST NOT return scene content beyond what the panel needs.

#### Scenario: List
- GIVEN a session with three acked batches
- WHEN the list is requested from the app window
- THEN three entries are returned newest first with their states

#### Scenario: Unknown session
- GIVEN a session id that does not exist
- WHEN the list is requested
- THEN 404 is returned

#### Scenario: Foreign origin
- GIVEN a request without the window Origin
- WHEN it asks for the list
- THEN it is refused

### Requirement: Conflict-Safe Partial Undo
`POST sesiones/{id}/cambios/{op}/deshacer` MUST undo a batch by replaying its stored inverse, restoring each element only when its current version equals the version stored at ack. An element changed since the batch MUST be skipped, and all other elements MUST still be restored. The result MUST report the number of skipped elements, counting every element of a skipped shape together with its bound label, and the UI MUST show "1 elemento no se restauró porque lo cambiaste" when N is 1 and "N elementos no se restauraron porque los cambiaste" when N is above 1. Undo MUST NOT touch elements outside the batch. An undone batch MUST be marked `deshecho` and MUST NOT be undoable again. Undo MUST be recorded and persisted so that the result survives a reload.

#### Scenario: Clean undo
- GIVEN a batch whose elements are all unchanged since it was applied
- WHEN the user undoes it
- THEN every element is restored, the skipped count is 0, and the batch is `deshecho`

#### Scenario: Partial undo
- GIVEN a batch that edited unlabeled elements A and B and the user later moved A
- WHEN the user undoes the batch
- THEN B is restored, A keeps the user's move, and the user sees "1 elemento no se restauró porque lo cambiaste"

#### Scenario: All elements changed
- GIVEN every element of a batch was edited by the user afterwards
- WHEN the user undoes the batch
- THEN nothing is restored, the skipped count equals the batch size, and the batch is `deshecho`

#### Scenario: Undo twice
- GIVEN a batch already `deshecho`
- WHEN undo is requested again
- THEN it is refused and the scene is unchanged

#### Scenario: After reload
- GIVEN an agent batch applied before the app restarted
- WHEN the user reopens the session and undoes the batch
- THEN the batch is undone as in a session without restart

#### Scenario: Batch without inverse
- GIVEN a batch marked "sin deshacer"
- WHEN undo is requested
- THEN it is refused and the scene is unchanged

#### Scenario: Board not open
- GIVEN the session's board is not mounted
- WHEN undo is requested
- THEN it fails with a clear message to open the board, the batch stays undoable, and nothing changes

#### Scenario: Apply fails after claim
- GIVEN the server claimed a batch for undo
- WHEN the UI fails to apply the inverse
- THEN the UI releases the claim and the batch is undoable again

### Requirement: Undo Is User-Only
Undo MUST be reachable only from the history panel in the app window, protected by the window Origin guard. No MCP tool MUST be able to undo, redo, or delete a batch, and AGENTS.md, CLAUDE.md, and MCP Instructions MUST state that the agent cannot undo.

#### Scenario: No MCP undo
- GIVEN the MCP tool list
- WHEN inspected
- THEN no tool undoes a batch

#### Scenario: Foreign origin
- GIVEN a request to the undo route without the window Origin
- WHEN it is sent
- THEN it is refused and the scene is unchanged

### Requirement: History Panel
The Terminal view MUST offer a history panel listing the session's agent batches with an undo action per undoable batch, the state of each batch, and the skipped-count result after an undo. The panel MUST be usable after a restart. It MUST show "sin deshacer" for batches without an inverse.

#### Scenario: Panel lists batches
- GIVEN a session with agent batches
- WHEN the user opens the panel
- THEN each batch shows time, kind, element count, and state

#### Scenario: Undo from panel
- GIVEN an undoable batch in the panel
- WHEN the user presses its undo action
- THEN the batch is undone and the panel shows the result and the skipped count

#### Scenario: Empty history
- GIVEN a session with no agent batches
- WHEN the user opens the panel
- THEN an empty-state message is shown

### Requirement: Agent Batches and Editor Undo
Agent batches MUST keep the immediate capture mode of phase 1 unless a runtime check shows that never-capturing agent updates keeps the editor's own undo correct after the user edits the same element. The persisted history, not the editor's undo stack, MUST be the authoritative way to undo an agent batch. The history MUST NOT clear the editor's undo stack.

#### Scenario: Editor undo still works
- GIVEN the user drew a stroke after an agent batch
- WHEN the user presses Ctrl+Z
- THEN the user's stroke is undone and the scene stays valid

#### Scenario: History independent of editor stack
- GIVEN the editor's undo stack was reset by a reload
- WHEN the user opens the history panel
- THEN previous batches remain undoable
