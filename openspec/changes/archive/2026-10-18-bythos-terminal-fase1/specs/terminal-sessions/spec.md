# terminal-sessions Specification

## Purpose
Persist named work sessions (mode, optional folder) and present them as tabs on a Terminal screen.

## Requirements

### Requirement: Navigation Entry
The sidebar MUST show a "Terminal" item below "Exportar" with a "NUEVO" badge.

#### Scenario: Entry present
- GIVEN the app is open
- WHEN the sidebar renders
- THEN "Terminal" appears directly after "Exportar" with the badge

### Requirement: Create Session
The "+" dialog MUST collect nombre, carpeta (optional) and modo, and MUST reject an empty nombre. The session MUST persist in `terminal_sesiones` and survive restart.

#### Scenario: Create
- GIVEN the dialog has nombre "Mates", a folder, and a mode
- WHEN the user confirms
- THEN a tab appears showing the mode, and the row exists after restart

#### Scenario: Empty name
- GIVEN nombre is blank
- WHEN the user confirms
- THEN creation is refused with "Escribe un nombre para la sesión."

### Requirement: Folder Link
Deleting a folder MUST NOT delete its sessions or scenes; it MUST clear the session's folder link so it becomes loose.

#### Scenario: Folder deleted
- GIVEN a session linked to folder F
- WHEN F is deleted
- THEN the session, its scene and ops remain, with no folder, shown as "Suelta"

### Requirement: Status Chips
Each session MUST show chips for terminal state and board save state.

#### Scenario: Save state
- GIVEN unsaved board edits
- WHEN autosave completes
- THEN the save chip changes from "Guardando…" to "Guardado"

### Requirement: Close and Delete Session
Closing a tab MUST NOT delete data. Deleting a session MUST end its PTY and remove its rows after confirmation.

#### Scenario: Delete
- GIVEN a session with a running PTY
- WHEN the user confirms deletion
- THEN the PTY ends and `terminal_sesiones`, scene, and ops rows are gone

### Requirement: Revisar
"Revisar" MUST flush pending autosave and focus the terminal. It MUST NOT write any bytes to the PTY.

#### Scenario: Revisar
- GIVEN pending board edits
- WHEN the user presses Revisar
- THEN the scene is persisted, the terminal has focus, and the PTY received no input
