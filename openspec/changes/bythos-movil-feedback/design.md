# Design: Mobile Transfer Feedback (bythos-movil-feedback)

## Technical Approach

Three independent units in one PR (~300 lines), no API or protocol change. **A** threads a progress callback from `Subidor` up to `UploadWorker` (`setProgressAsync` + foreground notification) and is observed by `ShareReceiverActivity`. **B** adds one polling effect to the `App` container in `App.jsx` over the existing `api.listarEventos(1, 'celular')`. **C** gives `PairingActivity` a paired/unpaired mode, a spinner with phase text, a stored `nombrePC`, and a pairing-only short-timeout `ApiClient`.

## Architecture Decisions

| Decision | Choice | Rejected | Rationale |
|---|---|---|---|
| Where bytes are counted | `ServidorHttp.parte` writes `datos` in 64 KiB slices through the existing `postStream` `escribir` lambda (`escribirEnTramos`) and reports after each slice | Counting `OutputStream` wrapper inside `ApiClient.postStream` (proposal wording) | Same effect with zero transport change; `ApiClient` stays protocol-agnostic and `escribirEnTramos` is a pure function testable with `ByteArrayOutputStream` |
| Progress contract | `Servidor.parte(..., progreso: (Long) -> Unit = {})`; `Subidor(avance: (Fase, Long, Long) -> Unit = {...})` | Callback per 4 MiB part only | Files under 4 MiB would still jump 0 -> 100 |
| Throttling | Pure `LimitadorAvance(intervaloMs = 250, reloj)` in the worker: emits on phase change, on completion, or after 250 ms | Throttle inside `Subidor` | Keeps `Subidor` deterministic for tests; clock is injectable |
| App-side observation | `WorkManager.getWorkInfosForUniqueWorkLiveData("envio-$id").observeForever`, removed in `onDestroy` | Lifecycle `observe`, Flow | `ActividadRedLocal` extends plain `Activity` (not `LifecycleOwner`); no new dependency |
| Desktop arrival | `setInterval` 2500 ms, skipped while `document.hidden`, immediate poll on `visibilitychange`; first success seeds `ultimoId` (0 when empty) without toast | SSE/WebSocket, Windows toast | Confirmed by user; endpoint exists; `recargar()` already refreshes the open folder because `FoldersView` filters the `recursos` prop |
| Toast | Inline state `avisoCelular` cleared after 6 s, same pattern as `agenteEstado`; CSS class in `styles.css` | New toast component/library | ~15 lines, follows existing pattern |
| PC label | `nombreVisiblePC(guardado, porDefecto)` -> stored `nombrePC` if non-blank, else neutral `R.string.pc_sin_nombre` ("tu PC") | Fall back to host/IP | User does not want IPs in the UI; also replaces the existing `leido.host` fallbacks in `qr_leido`/`emparejado` |
| Pairing timeout | `ApiClient(..., conexionMs: Int = CONEXION_MS, lecturaMs: Int = LECTURA_MS)`; pairing uses 3000 / 10000 | Lower global timeout | Uploads keep 5 s / 60 s; pairing responses are tiny and immediate |

## Data Flow

```
Subidor.hashArchivo ──avance(PREPARANDO,leidos,total)──┐
Subidor.enviarPartes ─parte(progreso)─> escribirEnTramos│
        avance(ENVIANDO, offset+enviados, total) ───────┤
Subidor.esperarVerificacion ─avance(VERIFICANDO)────────┤
                                                        v
                UploadWorker: LimitadorAvance (<=4/s)
                  ├─ setForegroundAsync(notif: fase + setProgress)
                  └─ setProgressAsync(fase, hechos, total)
                                 v
     ShareReceiverActivity (observeForever per "envio-$id") -> estado text
```

Desktop arrival:

```
App ──every 2.5 s (visible)──> GET /api/eventos?limite=1&origen=celular
   first ok: ultimoId = ev?.id ?: 0          (no toast)
   ev.id > ultimoId: ultimoId = ev.id; setAvisoCelular(actor, detalle, accion); recargar()
```

Pairing sequence:

```
User    PairingActivity          ApiClient(3s/10s)        PC
 |-scan->| spinner ON, "Conectando…"
 |       |-- solicitar ----------->|-- POST /v1/emparejar -->|
 |       |   timeout/refused ------ "La PC no está recibiendo…", spinner OFF
 |       |<- sas ------------------|                         |
 |       | "Confirma el código {sas}" (spinner ON)            |
 |       |-- estado poll 2 s ----->|------------------------->|
 |       |<- aprobado+token -------| save token, host, puerto, huella, nombrePC
 |       | paired mode: "Emparejado con «X»", spinner OFF
```

## File Changes

| File | Action | Description |
|---|---|---|
| `mobile/android/app/src/main/kotlin/com/bythos/movil/UploadWorker.kt` | Modify | `enum Fase`; `progreso` param on `Servidor.parte`; `escribirEnTramos`; `avance` in `Subidor` (hash loop, part slices, verify); `LimitadorAvance`; worker builds notification with `setProgress`, calls `setProgressAsync` |
| `.../ApiClient.kt` | Modify | Optional `conexionMs`/`lecturaMs` constructor params |
| `.../ShareReceiverActivity.kt` | Modify | After `encolar`, observe each unique work; render "Enviando a.bin: 42 %" / phase; remove observers in `onDestroy` |
| `.../PairingActivity.kt` | Modify | Paired/unpaired modes, `ProgressBar`, phase text, `nombrePC` persisted, short-timeout client, stale-QR message, `Log.i` phase timestamps |
| `.../ActividadRedLocal.kt` | Modify | `nombreVisiblePC` helper |
| `.../res/values/strings.xml` | Modify | `pc_sin_nombre`, `emparejado_con`, `volver_a_emparejar`, `conectando`, `pc_no_recibe`, `fase_preparando`, `fase_enviando`, `fase_verificando` |
| `.../src/test/.../SubidorTest.kt` | Modify | Fake `parte` accepts `progreso` and reports per slice; new tests |
| `.../src/test/.../ApiClientTest.kt` | Modify | `escribirEnTramos` slices test |
| `desktop/ui/src/App.jsx` | Modify | Polling effect, `avisoCelular` state and render |
| `desktop/ui/src/styles.css` | Modify | `.aviso-celular` fixed-position toast |

No Go changes.

## Interfaces / Contracts

```kotlin
enum class Fase { PREPARANDO, ENVIANDO, VERIFICANDO }
interface Servidor { fun parte(id: String, offset: Long, sha256: String, datos: ByteArray, progreso: (Long) -> Unit = {}): Long /* others unchanged */ }
fun escribirEnTramos(out: OutputStream, datos: ByteArray, progreso: (Long) -> Unit, tramo: Int = 64 * 1024)
// WorkInfo progress keys: "fase" (String name), "hechos" (Long), "total" (Long)
```

Prefs (`bythos`): new key `nombrePC` (String, may be absent for old pairings).

## Testing Strategy

| Layer | What | Approach |
|---|---|---|
| Unit | `Subidor` reports monotonic `ENVIANDO` bytes ending at `tamano`, `PREPARANDO` during hash, `VERIFICANDO` last | `ServidorFalso.parte` calls `progreso(datos.size)`; recording lambda |
| Unit | Existing `SubidorTest` cases | Only the fake signature changes; assertions unchanged |
| Unit | `escribirEnTramos` writes identical bytes and reports cumulative counts per 64 KiB | `ByteArrayOutputStream` |
| Unit | `LimitadorAvance` drops updates within 250 ms, passes phase change and completion | Fake clock |
| Unit | `nombreVisiblePC` blank/null -> default | Plain JUnit |
| Manual (A54) | Progress under/over 4 MiB, paired home, stale QR, desktop toast within ~3 s | Field checklist |

Desktop UI has no test runner; verify with `npm run build` and manual check. `go test ./...` must stay green (untouched).

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary.

## Migration / Rollout

No migration. Old pairings lack `nombrePC` and show "Emparejado con «tu PC»" until re-paired; the raw host is never shown.

## Open Questions

- [ ] **Stale-QR delay source.** `CONEXION_MS` is 5 s, so the long wait may come from: Android's OkHttp-backed `HttpURLConnection` silently retrying the route/TLS fallback (multiplying the connect timeout), the 60 s read timeout covering a stalled TLS handshake, or Windows Firewall stealth-dropping SYNs. Confirm on the A54 before tuning: `Log.i("Bythos", …)` with elapsed ms at connect start, `solicitar` return, each poll, and failure (exception class); reproduce (1) QR >2 min after leaving "Recibir del celular", (2) desktop closed, (3) code >10 min with view open; read `adb logcat -s Bythos`. The 3 s / 10 s pairing client ships regardless; if retries dominate, bound total pairing time with a wall-clock deadline in the `emparejar` thread.
- [ ] Toast text for `recurso_guardado` (link) vs `archivo_subido` — spec should fix the wording.
