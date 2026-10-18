# Exploration: bythos-movil-feedback

Source: field test of `bythos-movil-qr` on 2026-10-11 (Samsung SM-A546E, Android 16, desktop built from the repo). Engram: `sdd/bythos-movil-feedback/explore`, `bythos/movil-field-test-feedback`.

## Problems

1. Android upload progress stays at 0% and then jumps to done.
2. The desktop shows no notice when a file arrives from the phone.
3. The desktop folder view needs F5 to show the new file.
4. Weak feedback after scanning the QR on Android; the app never shows that it is already paired.

None of these is covered by a requirement in the `bythos-movil-qr` specs: they are new requirements, not spec violations.

## Verified current behavior

- **Progress.** `UploadWorker.anunciar()` posts a static foreground notification without progress. `Subidor.enviarPartes` saves the offset once per 4 MiB part, so a file under 4 MiB is a single step. `hashArchivo` (SHA-256 pre-hash) and `esperarVerificacion` (up to 60 one-second polls) report nothing. `ApiClient.postStream` already takes an `escribir: (OutputStream) -> Unit` lambda in fixed-length streaming mode, so byte-level progress is cheap.
- **Arrival notice.** `puenteLAN.GuardarArchivo` and `GuardarLink` (`desktop/api/puente_lan.go`) only call `registrar()`, which writes an activity event with origin `celular` and the device name as actor. There is no SSE, WebSocket or OS toast in the codebase.
- **Folder refresh.** `GET /api/eventos?limite=1&origen=celular` already exists (`desktop/api/server.go`) and returns `id`, `actor`, `accion`, `detalle`. `recargar()` in `desktop/ui/src/App.jsx` runs only on mount and after local mutations. The only polling is `receptorEstado` every 1.5 s on the "recibir" view.
- **Pairing feedback.** `PairingActivity` has one `estado` TextView and no spinner. It always opens on "Emparejar" and never calls `clienteEmparejado()` on launch. The PC name is not stored in prefs. `ApiClient.CONEXION_MS` is 5 s, so the long silent wait on a stale QR may come from another path; reproduce on the device before settling the fix. The QR (`bythos://pair?v=1&h&p&k&c&n`) has no expiry field; the code TTL is 10 min and the listener closes 2 min after the last use.

## Options

### 1. Progress (Android: worker and notification)

| Option | Tradeoff | Est. lines |
|---|---|---|
| A. `onProgress` callback in `Subidor`, per part | Simple; useless for files under 4 MiB | ~40 |
| **B. Counting OutputStream in `postStream` (64 KiB slices) plus a hash-loop callback** | Smooth progress for any size; no protocol change | ~90–120 |
| C. Smaller chunks | More round trips and offset writes; does not cover hash or verify | not recommended |

Recommended: B, with phase labels (hashing, sending, verifying) and updates throttled to about 4 per second. It adds an optional `progreso` parameter to the `Servidor.parte` interface and its test fake.

### 2 + 3. Arrival notice and folder refresh (desktop UI)

| Option | Tradeoff | Est. lines |
|---|---|---|
| **A. Poll `/api/eventos?limite=1&origen=celular` every 2–3 s; on a new id, call `recargar()` and show an in-app toast** | Reuses an existing endpoint; pauses on `document.hidden` | ~50–70 |
| B. SSE or WebSocket | Instant; new endpoint, hub and lifecycle | ~200 |
| C. Windows toast | Visible when minimized; needs a shell or library; packaging not verified | ~80–150 |

Recommended: A. The toast and the refresh share the same poll, so they belong in one unit.

### 4. Pairing feedback (Android UI)

- Paired header on launch: "Emparejado con <PC>", "Enviar archivo" as the primary action, "Re-emparejar" as secondary. Store `nombrePC` in prefs on pairing; fall back to the host for devices paired earlier. ~40 lines.
- Indeterminate spinner and phase text while pairing (connecting, waiting for approval with the code). ~30 lines.
- Stale QR, option A (recommended): short connect timeout for pairing only and a clear "the PC is not receiving" message. ~20 lines, no PC change.
- Stale QR, option B (deferred): optional `&e=<unix>` expiry in the QR. Touches `desktop/lan/qr.go`, the Kotlin parser and the device-pairing spec; covers only the 10-minute code TTL, not the 2-minute listener close.

## Work units

| Unit | Scope | Est. lines | Depends on |
|---|---|---|---|
| A | Android progress (item 1) | ~120 | — |
| B | Desktop poll, toast and refresh (items 2 + 3) | ~70 | — |
| C | Android pairing feedback (item 4) | ~110 | — |

Total about 300 lines: one PR within the 400-line budget, no chaining needed. No new API.

## Product decisions for the user

1. Arrival notice: in-app toast only, or also a Windows toast?
2. Keep 4 MiB chunks and report byte-level progress?
3. Phone progress: notification only, or also inside the app?
4. Paired home screen: hide "Escanear QR" behind "Re-emparejar", or keep both visible?
5. Stale QR: short-timeout message only, or also a QR expiry field?

### Confirmed by the user (2026-10-11)

1. Arrival notice: in-app toast only (no Windows toast).
2. Keep 4 MiB chunks; report byte-level progress.
3. Phone progress: notification and in the app (share screen).
4. Paired home screen: hide "Escanear QR" behind "Volver a emparejar".
5. Stale QR: short-timeout message only (no QR expiry field).

Research: not selected.

## Risks

- The stale-QR wait may not be the 5 s connect timeout; reproduce on the device first.
- Polling every 2–3 s adds constant light load; pause while the window is hidden.
- Changing `Servidor.parte` touches the `Subidor` tests and fakes.
- Android can rate-limit foreground notification updates; throttle them.
- Already-paired devices have no stored PC name; fall back to the host.
