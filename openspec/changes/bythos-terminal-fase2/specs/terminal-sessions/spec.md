# terminal-sessions Specification (delta)

## MODIFIED Requirements

### Requirement: Close and Delete Session
Closing a tab MUST NOT delete data. Deleting a session MUST end its PTY and remove its rows after confirmation, including its attachment link rows. Deleting a session MUST NOT delete attachment files from disk and MUST NOT delete library resources.
(Previously: removed session, scene, and ops rows only.)

#### Scenario: Delete
- GIVEN a session with a running PTY
- WHEN the user confirms deletion
- THEN the PTY ends and `terminal_sesiones`, scene, and ops rows are gone

#### Scenario: Delete with attachments
- GIVEN a session with two attachments
- WHEN the user confirms deletion
- THEN the attachment link rows are gone and the files still exist on disk

#### Scenario: Shared content survives
- GIVEN the same file is attached to sessions A and B
- WHEN A is deleted
- THEN B still lists and reads the attachment

#### Scenario: Library resource survives
- GIVEN an attachment was saved to folder F with "Guardar en carpeta"
- WHEN the session is deleted
- THEN the resource in F remains

#### Scenario: Close tab
- GIVEN a session with attachments
- WHEN the user closes its tab
- THEN all attachments remain linked
