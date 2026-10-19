# whiteboard-attachments Specification

## Purpose
Let the user bring images and documents onto a session's board (paste, drop, Adjuntar, folder resource), keep them private to the session, and restore them after restart.

## ADDED Requirements

### Requirement: Entry Paths
The board MUST accept attachments through four paths: Ctrl+V paste, drag-and-drop, an "Adjuntar" button, and picking a resource of the session's folder. Every path MUST apply the same type and size rules and the same upload flow. If drag-and-drop cannot be intercepted reliably, the other three paths MUST still ship.

#### Scenario: Paste image
- GIVEN the board is focused and the clipboard holds a PNG
- WHEN the user presses Ctrl+V
- THEN the image is uploaded and appears on the board as a drawable image

#### Scenario: Drop file
- GIVEN the board is open
- WHEN the user drops a valid file on it
- THEN the file is uploaded and appears on the board without any "invalid file" dialog from the editor

#### Scenario: Adjuntar
- GIVEN the board is open
- WHEN the user presses "Adjuntar" and picks a valid file
- THEN the file is uploaded and appears on the board

#### Scenario: Pick folder resource
- GIVEN the session has a folder with resources
- WHEN the user picks one resource from that folder
- THEN it is attached to the session and appears on the board

#### Scenario: Session without folder
- GIVEN the session has no folder
- WHEN the user opens the attach options
- THEN picking a folder resource is unavailable or empty, and the other three paths still work

### Requirement: Allowed Types
Attachments MUST be limited to images (PNG, JPEG, GIF, WebP), PDF, Word, Excel, PowerPoint, and plain text or Markdown. Video and SVG MUST be rejected. The type MUST be decided by the file content, not by name or declared MIME. The UI SHOULD pre-filter types the server would reject, and MUST show the same clear message.

#### Scenario: Allowed document
- GIVEN a valid .docx file
- WHEN it is attached
- THEN the upload succeeds

#### Scenario: Disallowed type
- GIVEN an MP4 video, an SVG, or a BMP image
- WHEN the user attaches it
- THEN nothing is added to the board and a message states that the file type is not allowed

#### Scenario: Spoofed extension
- GIVEN an executable renamed to "foto.png"
- WHEN the user uploads it
- THEN the server rejects it as a disallowed type and nothing is stored or linked

### Requirement: Size Limit
Each attachment MUST NOT exceed 25 MiB. A larger file MUST be rejected with a clear message stating the limit, with nothing stored, linked, or added to the board.

#### Scenario: At the limit
- GIVEN a valid file of exactly 25 MiB
- WHEN it is attached
- THEN the upload succeeds

#### Scenario: Oversize
- GIVEN a valid file above 25 MiB
- WHEN it is attached by any path
- THEN the upload is rejected with 413, the user sees a message naming the 25 MiB limit, and the board and session links are unchanged

### Requirement: Upload Endpoint
Uploads MUST go through a dedicated session-attachment endpoint, protected by the window Origin guard. The existing library upload endpoint MUST NOT be used for board attachments. A successful upload MUST store the bytes once per content hash and MUST link them to the session with their original name.

#### Scenario: Upload from the window
- GIVEN a request from the app window Origin for an existing session
- WHEN a valid file is uploaded
- THEN the response returns the attachment id, name, type, and size, and the attachment is listed for that session

#### Scenario: Foreign origin
- GIVEN a request without the window Origin or from another origin
- WHEN it uploads a file
- THEN it is refused and nothing is stored

#### Scenario: Unknown session
- GIVEN a session id that does not exist
- WHEN a file is uploaded to it
- THEN 404 is returned and nothing is stored

#### Scenario: Duplicate content
- GIVEN the same file is attached twice to one session
- WHEN the second upload completes
- THEN bytes are stored once and the session has one attachment per distinct content

### Requirement: Session Privacy
An attachment MUST belong only to the session it was added to. It MUST NOT appear in the library and MUST NOT be readable through another session's attachment list or tools.

#### Scenario: Not in library
- GIVEN a file pasted in session A
- WHEN the user browses the library
- THEN the file is not listed there

#### Scenario: Other session
- GIVEN an attachment of session A
- WHEN session B lists or reads attachments
- THEN the attachment is not visible and a direct read by id returns 404

#### Scenario: Same bytes in two sessions
- GIVEN the same file attached to sessions A and B
- WHEN one session's link is removed
- THEN the other session still reads it

### Requirement: Image Elements
An attached image MUST appear as a drawable image element that the user can move, resize, and draw over. Images larger than 1440 px on the longest side MUST be downscaled before upload. Image bytes MUST NOT be stored inside the persisted scene.

#### Scenario: Downscale
- GIVEN a 4000x3000 photo
- WHEN it is attached
- THEN the board shows it and the stored image has its longest side at most 1440 px

#### Scenario: Draw on top
- GIVEN an image on the board
- WHEN the user draws a stroke over it
- THEN the stroke is saved with the scene and reloads over the image

#### Scenario: Scene stays small
- GIVEN a board with three large images
- WHEN autosave runs
- THEN the saved scene contains image references only, with no file bytes

### Requirement: Document Cards
An attached document (PDF, Word, Excel, PowerPoint, text, Markdown) MUST appear on the board as a card showing its name and kind. Activating the card MUST open the document in the existing viewer. The card MUST remain visible in the agent's board capture.

#### Scenario: Card appears
- GIVEN the user attaches "notas.docx"
- WHEN the upload completes
- THEN a card showing "notas.docx" is on the board

#### Scenario: Open in viewer
- GIVEN a card for an attached PDF
- WHEN the user activates it
- THEN the document opens in the existing viewer

#### Scenario: Card in capture
- GIVEN a board with a document card
- WHEN `pizarra_captura` runs
- THEN the card is visible in the returned image

### Requirement: Rehydration
When a session's board loads, every image and card referenced by the saved scene MUST be restored from the session's attachments before it is shown, so no placeholder tile remains. Attachments restored this way MUST NOT change the saved scene.

#### Scenario: Restart
- GIVEN a board with two images and one card
- WHEN the app restarts and the session opens
- THEN the same images and card are displayed with their content

#### Scenario: Missing blob
- GIVEN a scene references an attachment whose bytes cannot be read
- WHEN the board loads
- THEN the board still loads, that element shows as unavailable, and other elements are unaffected

### Requirement: Foreign Attachment Reference
The scene save endpoint MUST accept an image or card element only when its file id is an attachment of that same session. Any other reference MUST be rejected with 400 and the stored scene MUST stay unchanged.

#### Scenario: Own attachment
- GIVEN session A has attachment F
- WHEN a scene with an image element for F is saved to A
- THEN the save succeeds

#### Scenario: Foreign fileId
- GIVEN F is an attachment of session B only
- WHEN a scene referencing F is saved to session A
- THEN 400 is returned and A's previous scene is intact

#### Scenario: Unknown fileId
- GIVEN a scene references a file id that exists nowhere
- WHEN it is saved
- THEN 400 is returned and the previous scene is intact

### Requirement: Guardar en carpeta
Each attached image or card MUST offer "Guardar en carpeta" only when the session has a folder. Saving MUST be a manual per-item action that copies the file into that folder as a library resource. Attachments MUST NEVER be saved to the folder automatically.

#### Scenario: Manual save
- GIVEN a session with folder F and an attached image
- WHEN the user chooses "Guardar en carpeta"
- THEN the file becomes a resource of F and the attachment stays on the board

#### Scenario: No automatic save
- GIVEN a session with folder F
- WHEN the user pastes a file
- THEN F gains no new resource

#### Scenario: Session without folder
- GIVEN the session has no folder
- WHEN the user opens the actions of an attachment
- THEN "Guardar en carpeta" is not offered

#### Scenario: Duplicate in folder
- GIVEN F already holds identical content
- WHEN the user saves the attachment to F
- THEN no duplicate is created and the user sees the existing resource reported

### Requirement: Remove Attachment
The user MUST be able to remove one attachment from the board. Removal MUST delete the element from the board; the session link row MAY remain (so undo can restore it and the save still validates), but a removed attachment MUST NOT appear in the session's attachment list or be readable by the agent once the board is saved without it. Removal MUST keep the file on disk, and MUST NOT affect other attachments or any library resource.

#### Scenario: Remove
- GIVEN an image on the board
- WHEN the user removes it
- THEN it disappears from the board and from the session's attachment list, and after restart it does not return

#### Scenario: Library untouched
- GIVEN an attachment previously saved to folder F
- WHEN the user removes it from the board
- THEN the resource in F still exists

### Requirement: Failure Handling
If an upload fails, the board MUST add nothing and MUST show a clear error. The scene MUST NOT contain an element whose attachment was never stored.

#### Scenario: Server error
- GIVEN the upload request fails
- WHEN the user pastes an image
- THEN no element is added and an error message is shown

#### Scenario: Pasting plain text
- GIVEN the clipboard holds only text
- WHEN the user pastes on the board
- THEN the editor's normal text paste behaves as before
