# Spec: mobile-android-client (bythos-movil-qr)

## Purpose
Android app that pairs via QR, pins the desktop's certificate, and sends links/files including large video.

## ADDED Requirements

### Requirement: Certificate Pinning
The app MUST pin the SPKI SHA-256 fingerprint from the QR via a custom TrustManager/HostnameVerifier and MUST NOT accept a mismatched certificate.

#### Scenario: Fingerprint match
- GIVEN the served cert fingerprint matches the pinned value
- WHEN the app connects
- THEN the TLS handshake succeeds

#### Scenario: Fingerprint mismatch
- GIVEN the served cert fingerprint differs (rotation or rogue host)
- WHEN the app connects
- THEN the connection is refused and the app prompts "certificate changed, re-scan QR"

### Requirement: Local Network Permission
The app MUST request `ACCESS_LOCAL_NETWORK` on OS versions that enforce it before connecting.

#### Scenario: Permission granted
- GIVEN the permission is granted
- WHEN the app connects to the paired PC
- THEN the connection proceeds normally

#### Scenario: Permission denied
- GIVEN the permission is denied
- WHEN the app attempts to connect
- THEN the OS silently blocks the connection and the app detects the timeout and shows an explicit "local network access denied" hint

### Requirement: Share-Intent Ingestion
The app MUST accept `ACTION_SEND`/`ACTION_SEND_MULTIPLE` for links and files and persist URI permission for background access.

#### Scenario: Share to app
- GIVEN the user shares a file or link from another app
- WHEN Bythos is chosen as target
- THEN the item is queued with a persisted URI for the selected folder

### Requirement: Background Resumable Upload
The app MUST upload via WorkManager plus a `dataSync` foreground service, using POST-based chunk requests, resuming interrupted transfers via the offset protocol.

#### Scenario: Resume after kill
- GIVEN a large upload interrupted by network loss or app kill
- WHEN WorkManager retries
- THEN it resumes from the last acknowledged offset without re-sending completed bytes

#### Scenario: Source deleted mid-transfer
- GIVEN the shared content URI becomes inaccessible mid-upload
- WHEN the app attempts to read the next chunk
- THEN the upload fails with a visible error and no crash
