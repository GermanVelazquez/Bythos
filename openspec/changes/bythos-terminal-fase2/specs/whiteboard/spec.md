# whiteboard Specification (delta)

## MODIFIED Requirements

### Requirement: Toolset
The board MUST offer free draw, colors, stroke width, eraser, shapes, and text. Images and documents MUST enter only through the attachment flow (paste, drop, Adjuntar, folder resource) defined by `whiteboard-attachments`. Anything the editor would insert on its own, bypassing that flow, MUST be intercepted. Video and SVG MUST NOT be insertable.
(Previously: the image tool MUST be disabled, and image paste and drop MUST be blocked.)

#### Scenario: Image accepted
- GIVEN the board is focused
- WHEN the user pastes or drops a valid image
- THEN the image is uploaded as a session attachment and added as a drawable element, and "Las imágenes llegan en una fase posterior." is never shown

#### Scenario: Disallowed file dropped
- GIVEN the board is focused
- WHEN the user drops a video or SVG
- THEN no element is added and a message states that the type is not allowed

### Requirement: Scene Limits
The API MUST reject scene bodies above 20 MiB, leaving the stored scene unchanged. The API MUST accept an image or document-card element only when it references an attachment of the same session, and MUST reject any other reference with 400, leaving the stored scene unchanged. The persisted scene MUST keep `files` empty: file bytes MUST NOT be stored in the scene.
(Previously: the API rejected scene bodies above 20 MiB only; any image element was rejected.)

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
