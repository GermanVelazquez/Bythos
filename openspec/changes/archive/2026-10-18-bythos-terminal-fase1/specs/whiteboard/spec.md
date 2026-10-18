# whiteboard Specification

## Purpose
Per-session Excalidraw board with autosave, usable offline.

## Requirements

### Requirement: Toolset
The board MUST offer free draw, colors, stroke width, eraser, shapes, and text. The image tool MUST be disabled, and image paste and drop MUST be blocked.

#### Scenario: Image blocked
- GIVEN the board is focused
- WHEN the user pastes or drops an image
- THEN no image element is added and "Las imágenes llegan en una fase posterior." is shown

### Requirement: Autosave
Scene changes MUST be saved to `terminal_pizarras` after a debounce and on Revisar, tab switch, and unmount. A saved scene MUST reload identically after restart.

#### Scenario: Restart
- GIVEN a drawn scene
- WHEN the app restarts
- THEN the same elements load for that session

#### Scenario: Save failure
- GIVEN the save request fails
- WHEN autosave runs
- THEN the chip shows "Error al guardar" and the scene stays in memory for retry

### Requirement: Scene Limits
The API MUST reject scene bodies above 20 MiB, leaving the stored scene unchanged.

#### Scenario: Oversized
- GIVEN a scene body above 20 MiB
- WHEN it is saved
- THEN 413 is returned and the previous scene is intact

### Requirement: Offline Assets
Fonts MUST be self-hosted, MUST exclude Xiaolai, and the board MUST make no CDN or external network request.

#### Scenario: No external requests
- GIVEN the board loads with network blocked
- WHEN text is drawn
- THEN every request targets the local origin and the font renders

### Requirement: Lazy Load
Excalidraw MUST load only when the Terminal screen opens, behind an adapter, version pinned to 0.18.1.

#### Scenario: Other screens
- GIVEN the user opens another screen
- WHEN it loads
- THEN no Excalidraw chunk is requested
