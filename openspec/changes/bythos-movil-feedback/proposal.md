# Proposal: Mobile Transfer Feedback (bythos-movil-feedback)

## Intent
The `bythos-movil-qr` field test (2026-10-11, SM-A546E) showed four feedback gaps: phone progress stuck at 0%, no desktop notice on arrival, folder view needs F5, and weak pairing feedback (the app never shows it is already paired). The function works; the user cannot tell what is happening.

## Scope
### In Scope
- **A. Android progress**: byte-level progress via a counting `OutputStream` in `ApiClient.postStream` plus a hash-loop callback; phases preparing/hashing, sending, verifying; throttled (~4/s); shown in the foreground notification and in `ShareReceiverActivity` (WorkInfo progress). 4 MiB chunks kept.
- **B. Desktop arrival**: `App.jsx` polls `GET /api/eventos?limite=1&origen=celular` every 2–3 s, paused while `document.hidden`; a new event id triggers `recargar()` and an in-app toast with device name and file.
- **C. Android pairing**: when paired, home shows "Emparejado con «PC»" and "Enviar archivo" as primary; QR scan behind "Volver a emparejar". `nombrePC` stored in prefs (host fallback). Indeterminate spinner plus phase text. Short pairing-only connect timeout (~3 s) with a "the PC is not receiving; open Recibir del celular and scan again" message.

### Out of Scope
- Windows OS toast; SSE/WebSocket; QR expiry field; code signing; pending manual tasks of `bythos-movil-qr`; any API or protocol change.

## Capabilities
### New Capabilities
- `mobile-arrival-notice`: desktop polling, in-app toast, and folder auto-refresh on phone arrivals.
### Modified Capabilities
- `mobile-android-client`: adds upload progress reporting, paired home state, and pairing feedback/stale-QR message (delta against the `bythos-movil-qr` change spec, not yet archived).

## Approach
Three independent units in one PR (~300 lines, within the 400-line budget): A (~120), B (~70), C (~110). `Servidor.parte` gains an optional `progreso` parameter; the `Subidor` test fake is updated. Spanish identifiers and UI strings, matching `strings.xml` and `App.jsx`.

## Affected Areas
| Area | Impact |
|---|---|
| `mobile/android/.../ApiClient`, `Subidor`, `UploadWorker`, `ShareReceiverActivity` | Modified |
| `mobile/android/.../PairingActivity`, prefs, `res/values/strings.xml`, layouts | Modified |
| `mobile/android` `Subidor` unit tests/fakes | Modified |
| `desktop/ui/src/App.jsx` (+ toast component) | Modified/New |

## Risks
| Risk | Likelihood | Mitigation |
|---|---|---|
| Stale-QR silent wait is not the connect timeout (`CONEXION_MS` is 5 s) | Med | Reproduce on device before settling the fix |
| Constant poll load | Low | 2–3 s interval, paused when hidden |
| Android rate-limits notification updates | Med | Throttle ~4/s |
| `Servidor.parte` change breaks tests/fakes | Low | Optional parameter; update fake |
| Old pairings lack PC name | High | Fall back to host |
| Base change not archived | Low | Branch from `feat/bythos-movil-qr-07c-envios` |

## Rollback Plan
Single PR reverted with `git revert`. No API, schema, or protocol changes; units are independent and can be reverted file-by-file.

## Dependencies
- `bythos-movil-qr` (base branch `feat/bythos-movil-qr-07c-envios`).

## Success Criteria
- [ ] A file under 4 MiB shows non-zero, increasing progress with phase text in notification and app.
- [ ] Desktop shows a toast and refreshes the folder view within ~3 s of arrival, without F5.
- [ ] A paired phone opens on "Emparejado con «PC»" with "Enviar archivo" primary.
- [ ] A stale QR shows the "not receiving" message within ~3 s.
- [ ] `go test ./...` and `gradlew testDebugUnitTest` pass.
