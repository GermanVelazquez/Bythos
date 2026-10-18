# Spec: lan-reception (bythos-movil-qr)

## Purpose
LAN-only HTTPS listener in `bythos.exe` for receiving links/files from paired mobile devices, active only while the user enables reception.

## ADDED Requirements

### Requirement: Listener Lifecycle
The system MUST start the LAN listener only while reception is enabled on the "Recibir del celular" screen, and MUST stop it when the screen is closed, when the user explicitly disables reception, after 30 minutes of inactivity (no requests received while enabled), or when the app exits.

#### Scenario: Reception enabled
- GIVEN the screen is opened and reception enabled
- WHEN the listener starts
- THEN the QR and LAN address are shown

#### Scenario: Reception off
- GIVEN reception is disabled or the screen is closed
- WHEN a client attempts to connect
- THEN the connection is refused; no route is reachable

#### Scenario: Idle auto-off
- GIVEN reception has been enabled with no requests received for 30 minutes
- WHEN the idle timeout elapses
- THEN the listener stops automatically and the QR/LAN address are hidden

### Requirement: Private Network Enforcement
The system MUST reject any LAN request whose source IPv4 address is not inside the subnet of the network interface the listener is bound to. This is stricter than accepting any RFC1918 private address: a private-range address outside the bound interface's subnet MUST be rejected.

#### Scenario: Same-subnet private source
- GIVEN a request from a private IPv4 address inside the subnet of the interface the listener is bound to
- WHEN it reaches the listener
- THEN it is processed normally

#### Scenario: Private address, different subnet
- GIVEN a request from a private-range IPv4 address outside the bound interface's subnet
- WHEN it reaches the listener
- THEN it is rejected (403), no data processed, no event recorded

#### Scenario: Public address
- GIVEN a request from a public (non-private) IPv4 address
- WHEN it reaches the listener
- THEN it is rejected (403), no data processed, no event recorded

### Requirement: Windows Network Profile Requirement
The system MUST allow reception only while the current Windows network profile is Private. When the active profile is Public, the desktop MUST show a guide directing the user to switch the network to Private, and the listener MUST NOT accept any connections; no firewall rule permitting the Public profile is ever created.

#### Scenario: Private profile
- GIVEN the active Windows network profile is Private
- WHEN reception is enabled
- THEN the listener starts and accepts connections normally

#### Scenario: Public profile
- GIVEN the active Windows network profile is Public
- WHEN the user opens the "Recibir del celular" screen
- THEN the listener does not start, no connections are accepted, and the desktop shows a guide to switch the network to Private

### Requirement: TLS Certificate Integrity
The system MUST serve a self-signed cert (IP SANs) whose SPKI SHA-256 fingerprint is embedded in the QR; a regenerated/rotated cert MUST invalidate previously pinned clients.

#### Scenario: Cert unchanged
- GIVEN a client pinned to the current fingerprint
- WHEN it connects
- THEN TLS handshake succeeds

#### Scenario: Cert rotated
- GIVEN the desktop cert was regenerated (e.g. data reset)
- WHEN a previously paired client connects with the old pin
- THEN the handshake fails until the device re-pairs via a new QR

### Requirement: Folder Target Validation
Save/upload routes MUST validate the destination folder exists before accepting data.

#### Scenario: Valid folder
- GIVEN a folder id that exists
- WHEN a save/upload request targets it
- THEN the request is accepted

#### Scenario: Folder not found
- GIVEN a folder id that does not exist or was deleted
- WHEN a save/upload request targets it
- THEN the request is rejected with an error and no partial write occurs

### Requirement: File Type Validation
Upload routes MUST apply the same magic-byte allowlist used by `archivos.Guardar`.

#### Scenario: Allowed type
- GIVEN file bytes matching an allowed type
- WHEN uploaded
- THEN the file is saved and an event is recorded

#### Scenario: Disallowed type
- GIVEN file bytes that fail the allowlist (e.g. HTML/SVG disguised as another extension)
- WHEN uploaded
- THEN the upload is rejected and no event is recorded

### Requirement: Chunked Resumable Upload and Size Cap
The system MUST support offset-based resumable chunk upload (HEAD offset / POST chunk), enforce the cumulative size cap across chunks, and reassemble into one stream for `archivos.Guardar`.

#### Scenario: Resume after interruption
- GIVEN a transfer interrupted mid-upload
- WHEN the client queries the offset and resumes remaining chunks
- THEN the file completes intact and is deduplicated by hash

#### Scenario: Oversize transfer
- GIVEN cumulative received bytes would exceed the size cap
- WHEN the next chunk is submitted
- THEN the upload is rejected before completion and partial data is discarded
