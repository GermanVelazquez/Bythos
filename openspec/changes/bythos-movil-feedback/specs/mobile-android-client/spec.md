# Delta Spec: mobile-android-client (bythos-movil-feedback)

Delta against `openspec/changes/bythos-movil-qr/specs/mobile-android-client/spec.md` (not yet archived). Existing requirements are unchanged except where marked MODIFIED.

## MODIFIED Requirements

### Requirement: Background Resumable Upload
The app MUST upload via WorkManager plus a `dataSync` foreground service, using POST-based chunk requests of 4 MiB, resuming interrupted transfers via the offset protocol. The chunk size MUST remain 4 MiB and the protocol MUST NOT change. The upload MUST report byte-level progress as specified in "Upload Progress Reporting". (Previously: no progress reporting was specified.)

#### Scenario: Resume after kill
- GIVEN a large upload interrupted by network loss or app kill
- WHEN WorkManager retries
- THEN it resumes from the last acknowledged offset without re-sending completed bytes

#### Scenario: Source deleted mid-transfer
- GIVEN the shared content URI becomes inaccessible mid-upload
- WHEN the app attempts to read the next chunk
- THEN the upload fails with a visible error and no crash

#### Scenario: Chunk size preserved
- GIVEN a file larger than 4 MiB
- WHEN it is uploaded
- THEN it is sent in 4 MiB parts, and progress inside each part is reported from bytes written, not only at part boundaries

## ADDED Requirements

### Requirement: Upload Progress Reporting
While an upload runs, the app MUST report progress by phase and by bytes. The phases are, in order: hashing (computing the SHA-256 pre-hash), sending (streaming parts to the PC), and verifying (waiting for the PC to confirm). Each phase MUST show a distinct text label. Sending progress MUST be computed from bytes actually written to the connection, so a file smaller than one part shows increasing progress. Hashing progress MUST be computed from bytes read. Verifying MAY be shown as indeterminate. Progress MUST NOT regress within a phase and MUST reach completion only after the PC confirms.

#### Scenario: Small file shows progress
- GIVEN a file smaller than 4 MiB
- WHEN it is sent
- THEN the sending phase shows a percentage that increases from above 0% to 100% before the verifying phase begins

#### Scenario: Large file smooth progress
- GIVEN a file larger than 4 MiB
- WHEN it is sent
- THEN the percentage increases within each part, not only when a part completes

#### Scenario: Phase order
- GIVEN an upload starts
- WHEN it proceeds normally
- THEN the labels shown are hashing, then sending, then verifying, then done

#### Scenario: Resume continues from offset
- GIVEN an upload resumes at a nonzero acknowledged offset
- WHEN the sending phase starts
- THEN displayed progress begins at that offset and does not restart at 0%

### Requirement: Progress Visibility
Upload progress MUST be shown in both the foreground notification and the share screen (`ShareReceiverActivity`). The notification MUST show the phase text and a progress bar (determinate when bytes are known, indeterminate otherwise). The share screen MUST observe the same progress (WorkInfo progress) and show the phase text and percentage.

#### Scenario: Notification progress
- GIVEN an upload is running in the foreground service
- WHEN progress changes
- THEN the notification shows the current phase text and progress bar

#### Scenario: Share screen progress
- GIVEN the share screen is open for a queued upload
- WHEN the upload progresses
- THEN the screen shows the same phase text and percentage as the notification

#### Scenario: Share screen reopened
- GIVEN an upload is already running
- WHEN the user reopens the share screen
- THEN it shows the current phase and progress, not an empty state

### Requirement: Progress Update Throttling
Progress updates to the notification and to WorkManager MUST be throttled to at most about 4 per second. A phase change and the final completion update MUST always be delivered, even if inside the throttle window.

#### Scenario: High-rate writes
- GIVEN bytes are written in many small slices within one second
- WHEN progress is published
- THEN no more than about 4 updates are published in that second

#### Scenario: Phase change not dropped
- GIVEN an update was published less than 250 ms ago
- WHEN the phase changes from sending to verifying
- THEN the new phase is published immediately

### Requirement: Paired Home Screen
On launch, if a valid pairing exists, the app MUST show a paired home state: the text "Emparejado con «<PC name>»", "Enviar archivo" as the primary action, and a secondary action "Volver a emparejar". The QR scan control MUST NOT be visible on the paired home state and MUST be reachable only through "Volver a emparejar". If no valid pairing exists, the app MUST show the unpaired state with the QR scan control.

#### Scenario: Paired launch
- GIVEN the phone is paired with a PC named "ESCRITORIO"
- WHEN the app opens
- THEN it shows "Emparejado con «ESCRITORIO»", a primary "Enviar archivo" action, and "Volver a emparejar", and no QR scan control

#### Scenario: Re-pair
- GIVEN the paired home state is shown
- WHEN the user taps "Volver a emparejar"
- THEN the QR scan flow starts

#### Scenario: Unpaired launch
- GIVEN no pairing exists
- WHEN the app opens
- THEN the unpaired state with the QR scan control is shown

### Requirement: PC Name Storage and Fallback
On successful pairing the app MUST store the PC name from the QR in preferences. For a device paired before this change (no stored name), the app MUST show a generic label ("Emparejado con tu PC") and MUST NOT display the raw host or IP address in the UI. The app SHOULD fill in the name when it becomes available on a later successful pairing.

#### Scenario: Name stored
- GIVEN the user pairs by scanning a QR containing a PC name
- WHEN pairing succeeds
- THEN the name is persisted and shown on the paired home state after restart

#### Scenario: Legacy pairing without name
- GIVEN the device was paired before this change and has no stored name
- WHEN the app opens
- THEN it shows "Emparejado con tu PC" and no IP address or host

### Requirement: Pairing Progress Feedback
While pairing is in progress the app MUST show an indeterminate spinner and a phase text for the current step (connecting, then waiting for approval, with the confirmation code when applicable). The spinner MUST be hidden when pairing ends in success or failure.

#### Scenario: Pairing in progress
- GIVEN the user scanned a valid QR
- WHEN the app connects and waits for approval on the PC
- THEN a spinner and the current phase text are visible

#### Scenario: Pairing ends
- GIVEN pairing is in progress
- WHEN it succeeds or fails
- THEN the spinner is hidden and the result is shown

### Requirement: Stale QR Detection
Pairing connection attempts MUST use a short connect timeout of about 3 seconds, applied to pairing only; the regular connect timeout for uploads MUST NOT change. When the timeout elapses, or the connection is refused, during pairing, the app MUST show a clear message that the PC is not receiving: "La PC no está recibiendo. Abrí «Recibir del celular» en la PC y escaneá el QR de nuevo." The QR MUST NOT carry an expiry field and the QR format MUST NOT change.

#### Scenario: Stale QR
- GIVEN the PC listener has closed or the code expired
- WHEN the user scans the old QR
- THEN within about 3 seconds the app shows the "PC is not receiving" message and the spinner stops

#### Scenario: Uploads unaffected
- GIVEN a paired phone uploads a large file over a slow link
- WHEN the connection is slow
- THEN the 3-second pairing timeout is not applied

#### Scenario: Certificate mismatch still distinct
- GIVEN the certificate fingerprint does not match
- WHEN pairing fails
- THEN the existing "certificate changed, re-scan QR" message is shown, not the "not receiving" message

## Open points
- Host display: the spec forbids showing the raw IP/host in the UI (user preference). If the design finds no way to show a PC name for legacy pairings, the generic "tu PC" label stands; confirm with the user if they prefer showing the host.
- Reproduction needed: the proposal notes the silent wait on a stale QR may not come from the connect timeout (`CONEXION_MS` is 5 s). The stale-QR requirement states the observable outcome (message within about 3 s); design must confirm the cause on a device.
