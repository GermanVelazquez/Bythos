# Spec: device-pairing (bythos-movil-qr)

## Purpose
Establish trust between a phone and the desktop via QR, one-time code, PC-side approval, and a revocable per-device token.

## ADDED Requirements

### Requirement: QR Pairing Initiation
The desktop MUST generate a QR with LAN IP, port, cert fingerprint, and a single-use, time-limited pairing code.

#### Scenario: Valid code
- GIVEN an unexpired, unused pairing code
- WHEN the phone submits it
- THEN a pairing request is created for PC approval

#### Scenario: Expired code
- GIVEN a pairing code past its expiry
- WHEN the phone submits it
- THEN it is rejected and the phone is told to re-scan

#### Scenario: Reused code
- GIVEN a pairing code already consumed once
- WHEN it is submitted again
- THEN it is rejected as single-use

### Requirement: PC-Side Approval
The first connection from a new device MUST require explicit desktop-user approval before any token is issued.

#### Scenario: Approved
- GIVEN a pending pairing request
- WHEN the user approves it
- THEN a hashed per-device token is issued and stored

#### Scenario: Declined
- GIVEN a pending pairing request
- WHEN the user declines or ignores it
- THEN no token is issued and the device cannot proceed

### Requirement: Device Token Enforcement
Every authenticated LAN route MUST validate the presented token against stored hashed tokens and their revoked flag.

#### Scenario: Valid token
- GIVEN a non-revoked, matching token
- WHEN a request is made
- THEN it is processed

#### Scenario: Unpaired device
- GIVEN no token or an unknown token
- WHEN a request is made
- THEN it is rejected (401/403)

#### Scenario: Revoked device
- GIVEN a token marked revoked
- WHEN a request is made
- THEN it is rejected and no data is processed

### Requirement: Revocation
The user MUST be able to revoke a paired device from the desktop UI, effective immediately.

#### Scenario: Revoke
- GIVEN a paired device
- WHEN the user revokes it
- THEN the device's next request is rejected
