# Tasks: Mobile Transfer Feedback (bythos-movil-feedback)

Paths: `K` = `mobile/android/app/src/main/kotlin/com/bythos/movil`, `T` = `mobile/android/app/src/test/kotlin/com/bythos/movil`, `R` = `mobile/android/app/src/main/res`.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~300 (range 250-380) |
| 400-line budget risk | Medium |
| Chained PRs recommended | No |
| Suggested split | Single PR (units A, B, C are independent; split only if it grows past 400) |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Medium

Base branch: chain tip `feat/bythos-movil-qr-07c-envios`. Risk is Medium because tests plus the A54 diagnostic could push past 400. If so, split B (desktop) out as its own PR.

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| A | Android upload progress | PR 1 | `./gradlew :app:testDebugUnitTest` (from `mobile/android`) | Samsung A54: share a <4 MiB and a >4 MiB file | `UploadWorker.kt`, `ShareReceiverActivity.kt`, A strings and tests |
| B | Desktop arrival notice | PR 1 (or PR 2 if split) | `cd desktop/ui && pnpm build` | Desktop app + phone upload | `App.jsx`, `styles.css` |
| C | Pairing UX | PR 1 | `./gradlew :app:testDebugUnitTest` | Samsung A54: paired home, stale QR | `PairingActivity.kt`, `ApiClient.kt`, `ActividadRedLocal.kt`, C strings |

## Unit A: Android progress

- [ ] A.1 RED: in `T/ApiClientTest.kt` add `escribirEnTramos` test: bytes identical, cumulative counts per 64 KiB slice.
- [ ] A.2 RED: in `T/SubidorTest.kt` update the fake `parte` to accept `progreso`; add tests: PREPARANDO during hash, monotonic ENVIANDO ending at `tamano`, VERIFICANDO last, resume starts at offset, small file (<4 MiB) progresses.
- [ ] A.3 RED: add `LimitadorAvance` tests in `T/SubidorTest.kt` with fake clock: drops within 250 ms, passes phase change and completion.
- [ ] A.4 In `K/UploadWorker.kt` add `enum Fase`, `progreso` param on `Servidor.parte`, and `escribirEnTramos` (64 KiB), wired in `ServidorHttp.parte`.
- [ ] A.5 In `K/UploadWorker.kt` add `avance` callback to `Subidor` (hash loop, part slices, verify); add `LimitadorAvance(250 ms)`.
- [ ] A.6 In `K/UploadWorker.kt` worker: notification with phase text and `setProgress` (indeterminate for VERIFICANDO), plus `setProgressAsync` with keys `fase`, `hechos`, `total`.
- [ ] A.7 In `K/ShareReceiverActivity.kt` observe `getWorkInfosForUniqueWorkLiveData("envio-$id")` via `observeForever`; show phase and percent; remove observers in `onDestroy`.
- [ ] A.8 Add `fase_preparando`, `fase_enviando`, `fase_verificando` to `R/values/strings.xml`.
- [ ] A.9 Verify: `./gradlew :app:testDebugUnitTest` (from `mobile/android`) is green.

## Unit B: Desktop arrival notice

- [ ] B.1 In `desktop/ui/src/App.jsx` add a 2500 ms polling effect over `api.listarEventos(1, 'celular')`: skip when `document.hidden`, poll on `visibilitychange`, first success seeds `ultimoId` (0 if empty) with no toast, errors keep `ultimoId`.
- [ ] B.2 In `App.jsx` on `ev.id > ultimoId`: set `avisoCelular` (actor, detalle), call `recargar()`, clear after 6 s. Fix the wording for link vs file events.
- [ ] B.3 Add `.aviso-celular` fixed-position toast to `desktop/ui/src/styles.css`.
- [ ] B.4 Verify: `cd desktop/ui && pnpm build`, then `git restore desktop/ui/dist/.gitkeep` (build deletes it).
- [ ] B.5 Verify: `cd desktop && go test ./...` stays green (no Go changes).

## Unit C: Pairing UX

- [ ] C.1 RED: add `nombreVisiblePC` tests (null/blank -> default, stored name wins) in `T/QrEmparejamientoTest.kt`.
- [ ] C.2 Add `nombreVisiblePC(guardado, porDefecto)` to `K/ActividadRedLocal.kt`; replace `leido.host` fallbacks.
- [ ] C.3 In `K/ApiClient.kt` add optional `conexionMs`/`lecturaMs` params (defaults unchanged); test defaults in `T/ApiClientTest.kt`.
- [ ] C.4 In `K/PairingActivity.kt` add paired mode ("Emparejado con «X»", "Enviar archivo", "Volver a emparejar", no QR control) and unpaired mode; persist `nombrePC` on success.
- [ ] C.5 In `PairingActivity.kt` add `ProgressBar` with phase text ("Conectando…", "Confirma el código {sas}"); use a 3000/10000 pairing `ApiClient`; show the "La PC no está recibiendo…" message on timeout/refused; keep the certificate-mismatch message distinct.
- [ ] C.6 Add `pc_sin_nombre`, `emparejado_con`, `volver_a_emparejar`, `conectando`, `pc_no_recibe` to `R/values/strings.xml`.
- [ ] C.7 Diagnostic: add `Log.i("Bythos", ...)` with elapsed ms at connect start, `solicitar` return, each poll, and failure (exception class) in `PairingActivity.kt`.
- [ ] C.8 Verify: `./gradlew :app:testDebugUnitTest` is green.

## Manual verification

- [ ] M.1 A54, `adb logcat -s Bythos`: repro (1) QR >2 min after leaving "Recibir del celular", (2) desktop closed, (3) code >10 min with view open. Record elapsed ms per phase.
- [ ] M.2 If retries dominate, add a wall-clock deadline in the `emparejar` thread (`PairingActivity.kt`); otherwise document the cause in `design.md` Open Questions.
- [ ] M.3 A54: <4 MiB file shows rising percent; >4 MiB is smooth; notification and share screen match; reopening the share screen shows current state.
- [ ] M.4 A54: paired launch shows the PC name (legacy pairing shows "tu PC", no IP); "Volver a emparejar" starts the scan; stale QR shows the message within ~3 s.
- [ ] M.5 Desktop: phone upload shows the toast (device and file name) and the file lists within ~3 s without F5; no OS notification; no toast on startup.
