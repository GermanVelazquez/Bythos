# Spec: activity-history (bythos-movil-qr)

## MODIFIED Requirements

### Requirement: Origin Attribution
The system MUST attach an origin and actor to every mutation event. Origin MUST be one of `app`, `extension`, `agente`, `celular`, else normalized to `desconocido`. Actor is free text ≤80 chars (for `celular`, the paired device name). Validation MUST occur at both the API layer and the DB layer. Event recording MUST NOT fail the underlying request on a write error (log-only).
(Previously: origin enum was `app`, `extension`, `agente`, else `desconocido`, with no `celular` value.)

#### Scenario: Mobile-originated event
- GIVEN an upload or link-save request from a paired device
- WHEN the mutation is recorded
- THEN the historial event has origin=`celular` and actor=the device name

#### Scenario: Unknown origin header
- GIVEN a malformed or missing origin header
- WHEN the mutation is recorded
- THEN origin is normalized to `desconocido` and the request still succeeds
