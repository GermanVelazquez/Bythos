# Tasks: Bythos Mobile QR Reception (bythos-movil-qr)

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | ~3,400–3,800 (12 units, each ≤~400) |
| 400-line budget risk | High (aggregate; per-unit risk is Low if delivered as 12 PRs) |
| Chained PRs recommended | Yes |
| Suggested split | PR0 (prereq) → PR1..PR12, one per design unit below |
| Delivery strategy | auto-chain equivalent — 12 chained PRs, one per design unit (decided by user 2026-09-30) |
| Chain strategy | feature-branch-chain (tracker branch `feat/bythos-movil-qr`; PR#1 targets tracker, each child targets previous PR branch; only tracker merges to master) |

Decision needed before apply: No (resolved 2026-09-30)
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 0 | Paso 0 committed (prerequisite) | PR 0 | `cd desktop && go build ./...` | N/A — prerequisite check only | Blocks all units; not independently revertible |
| 1 | DB `dispositivos` table + `celular` origin | PR 1 | `go test ./db/...` | N/A — no listener yet | `db/dispositivos.go` + origin const, additive table |
| 2a | lan cert/interfaces/listener/guard/lifecycle | PR 2 | `go test ./lan/... -run TestReceptor` | Local: start listener, curl from non-subnet IP → 403 | `lan/{cert,interfaces,receptor,guardia}.go`, off by default |
| 2b | Network-category gate (NLM COM) | PR 3 | `go test ./lan/... -run TestRed` | Real Windows box: toggle profile, observe gate | `lan/{red,red_windows,red_other}.go`, additive gate |
| 3 | Pairing + QR + tokens | PR 4 | `go test ./lan/... -run TestEmparejar` | Manual: scan QR / curl pairing script | `lan/{emparejar,auth,qr,respuestas}.go` |
| 4a | Upload store (sidecar/.parte) | PR 5 | `go test ./lan/... -run TestSubidasStore` | Local: interrupt + resume via curl script | `lan/subidas_store.go` |
| 4b | Upload/link handlers + puente | PR 6 | `go test ./lan/... ./api/... -run "TestSubidas|TestLinks|TestPuenteLAN"` | Manual: curl chunked upload against running listener | `lan/{subidas,links}.go`, `api/puente_lan.go` |
| 5a | Control API + wiring + stop triggers | PR 7 | `go build ./... && go test ./api/...` | Manual: curl `/api/receptor/activar\|desactivar` | `api/receptor.go`, `main.go` wiring |
| 5b | UI screen + Private guide | PR 8 | `cd desktop/ui && pnpm build` (no JS test harness in repo) | Manual: open "Recibir del celular", force Public profile | `ui/src/ReceptorMovil.jsx`, sidebar entry |
| 6 | Installer firewall/admin/migration + docs | PR 9 | N/A — no installer CI | Manual: run installer on clean VM (see 13.1–13.3) | `installer/bythos.iss` Run/UninstallRun blocks |
| 7a | Android pairing/pinning/keystore/scanner | PR 10 | `./gradlew :app:testDebugUnitTest` | Manual: pair a real/emulated device | `mobile/android/**` (new module, isolated) |
| 7b | Android HTTPS client + failure classification | PR 11 | `./gradlew :app:testDebugUnitTest` | Manual: airplane-mode mid-transfer | Same module, additive files |
| 7c | Android outbox/worker/share | PR 12 | `./gradlew :app:testDebugUnitTest` | Manual: share from Photos, kill app, confirm resume | Same module, additive files |

**Windows environment quirk**: Smart App Control sometimes blocks a freshly compiled `go test` binary before it runs. Workaround: `go test -c -o <scratch-dir>\pkg.test ./lan/...` then execute the produced `.test` binary directly from the scratch dir, or use the SAC-off dev machine noted in the proposal risks — do not blindly retry `go test`.

## Task 0 — Prerequisite (blocks everything)

- [x] 0.1 Confirm `desktop/archivos/`, `desktop/db/archivos.go`, `desktop/api/archivos.go` (Paso 0) are committed to `master` (currently uncommitted per state, awaiting user test build). Verify: `git status` clean, `cd desktop && go build ./...`. **Blocks all units below.** — Done: committed to `master` as `aa80bd0` ("feat(archivos): guarda y muestra PDFs, videos, imágenes y documentos"), confirmed by user test build.

## Phase 1 — Unit 1: DB `dispositivos` + `celular` origin

- [x] 1.1 Create `desktop/db/dispositivos.go`: table DDL, `CrearDispositivo`, `DispositivoPorTokenHash` (excludes revoked), `ListarDispositivos`, `RevocarDispositivo`, `TocarDispositivo` (1/min throttle).
- [x] 1.2 Modify `desktop/db/db.go`: wire `crearTablaDispositivos` into `crearTablas` before `crearTablaEventos`.
- [x] 1.3 Modify `desktop/db/eventos.go`: add `OrigenCelular="celular"`; update `normalizarOrigen` to accept it, else `desconocido`.
- [x] 1.4 Test `desktop/db/dispositivos_test.go`: idempotent creation, hash lookup excludes revoked, throttled touch.
- [x] 1.5 Test `desktop/db/eventos_test.go`: `normalizarOrigen` accepts `celular`; malformed header → `desconocido` (spec: Origin Attribution scenarios). Also added `desktop/api/eventos_test.go` case confirming `origenDeCabeceras` never accepts `celular` from `X-Bythos-Origen`.
- Verification: `cd desktop && go test ./db/...`
- Est. changed lines: ~180

## Phase 2 — Unit 2a: lan cert/interfaces/listener/guard/lifecycle

- [x] 2a.1 `desktop/lan/cert.go`: ECDSA P-256 key at `%APPDATA%\Bythos\lan\clave.pem`; self-signed cert reissued per start with current IP SANs; SPKI SHA-256 base64url fingerprint func.
- [x] 2a.2 `desktop/lan/interfaces.go`: candidate selection (up, non-loopback RFC1918, exclude VPN/Hyper-V/WSL/etc., rank Wi-Fi>Ethernet>other).
- [x] 2a.3 `desktop/lan/receptor.go`: `Receptor` struct, `Start`/`Stop` (`Shutdown` 10s grace then `Close`), fixed port 48080 no fallback → `puerto_ocupado`.
- [x] 2a.4 `desktop/lan/guardia.go`: `var remotoPermitido` seam; same-subnet IPv4 check → 403 `origen_no_permitido`, no processing, no event.
- [x] 2a.5 Idle-timeout stop trigger (fake clock); stub hooks for the remaining 3 stop triggers (wired in 5a). — Done: `Stop` is the single idempotent hook; unit 5a wires it from 3 call sites (screen close, explicit disable, app exit), no separate stub methods needed.
- [x] 2a.6 Tests: httptest TLS + pinned client; non-subnet remote 403; off = refused; idle auto-off; stop mid-chunk completes then refuses; stop idempotent; key reuse/SPKI-stable golden fixture (shared with Android 7a). — Done: `cert_test.go`, `interfaces_test.go`, `guardia_test.go`, `receptor_test.go` (9 tests, all pass).
- Acceptance: spec scenarios "Reception off", "Idle auto-off", "Same-subnet private source", "Private address, different subnet", "Public address", "Cert unchanged/rotated".
- Verification: `cd desktop && go test ./lan/... -run TestReceptor`
- Est. changed lines: ~380

## Phase 3 — Unit 2b: network-category gate (NLM COM)

- [ ] 2b.1 `desktop/lan/red.go`: `Categoria` type (`privada|publica|dominio|desconocida`), `var categoriaRed func(ip) (Categoria, error)` seam.
- [ ] 2b.2 `desktop/lan/red_windows.go` (build tag `windows`): COM `INetworkListManager` via `golang.org/x/sys/windows` + `ole32.CoCreateInstance`; `LockOSThread`+`CoInitializeEx(MTA)` per call; match bound IP via `GetAdaptersAddresses`.
- [ ] 2b.3 `desktop/lan/red_other.go` (build tag `!windows`): stub returns `desconocida`.
- [ ] 2b.4 Wire gate into `receptor.Start`: `publica`/`dominio` → refuse, `red_no_privada`; `desconocida` → start + warning banner.
- [ ] 2b.5 `go.mod`: promote `golang.org/x/sys` to direct dependency.
- [ ] 2b.6 Tests: fake `categoriaRed` table (publica/dominio refuse; desconocida starts+warns); windows-tagged smoke test, skip if COM unavailable.
- Acceptance: spec "Private profile", "Public profile" (Windows Network Profile Requirement).
- Verification: `cd desktop && go test ./lan/... -run TestRed`
- Manual: real-hardware category detection (task 13.5).
- Est. changed lines: ~230

## Phase 4 — Unit 3: pairing + QR + tokens

- [ ] 3.1 `desktop/lan/qr.go`: `rsc.io/qr` PNG data URI + `bythos://pair?...` text builder.
- [ ] 3.2 `desktop/lan/emparejar.go`: one-time code (16B base64url, TTL 10min, single-use, max 3 pending, 5 bad/min → rotate+30s lock); SAS = first 20 bits SHA-256(code‖requestID) mod 1e6; `POST /v1/emparejar`, `GET /v1/emparejar/{solicitud}`.
- [ ] 3.3 `desktop/lan/auth.go`: 32B `crypto/rand` token, SHA-256 hex stored; Bearer middleware via `db.DispositivoPorTokenHash`.
- [ ] 3.4 `desktop/lan/respuestas.go`: `{error,mensaje}` envelope + full wire-code table.
- [ ] 3.5 Tests: TTL, single-use, lock, SAS derivation, token issued once, revoked → 401.
- Acceptance: spec "Valid/Expired/Reused code", "Approved/Declined", "Valid/Unpaired/Revoked token".
- Verification: `cd desktop && go test ./lan/... -run TestEmparejar`
- Est. changed lines: ~330

## Phase 5 — Unit 4a: upload store

- [ ] 4a.1 `desktop/lan/subidas_store.go`: sidecar JSON + `.parte` in `%APPDATA%\Bythos\lan\subidas`; running SHA-256 via `MarshalBinary`; crash recovery (truncate `.parte` to sidecar offset on load); cleanup on start/hourly/revoke/completion.
- [ ] 4a.2 Tests: resume after restart, crash truncation, idle>24h cleanup.
- Acceptance: spec "Resume after interruption".
- Verification: `cd desktop && go test ./lan/... -run TestSubidasStore`
- Est. changed lines: ~270

## Phase 6 — Unit 4b: upload/link handlers + puente

- [ ] 4b.1 `desktop/lan/links.go`: `POST /v1/links` — folder check FIRST → 404 `carpeta_no_encontrada` before metadata fetch (nothing written, no event); else 201.
- [ ] 4b.2 `desktop/lan/subidas.go`: `POST /v1/subidas` (create, 404 if folder missing before sidecar, 413 oversize, max 3 active/device, idempotent on `cliente_id`), `POST .../estado`, `POST .../partes` (`X-Offset`/`X-Chunk-SHA256`, `MaxBytesReader`, `TryLock`→409, offset mismatch→409, bad hash→422 truncate+restore, complete→202), `POST .../cancelar`.
- [ ] 4b.3 Async finalize worker: whole-hash check, folder re-check, `archivos.Guardar`, evento; error paths delete temp, no event.
- [ ] 4b.4 `desktop/api/puente_lan.go`: `Biblioteca` port adapter (`ListarCarpetas`, `ExisteCarpeta`, `GuardarLink`, `GuardarArchivo`) wrapping `db` + `archivos` + SSRF metadata client.
- [ ] 4b.5 Tests: create/estado/partes/cancelar; 409/413/422; folder missing at create / deleted before finalize; links 404 before metadata fetch (fake fetcher not called); dedupe; whole-hash mismatch; magic-byte reject → `tipo_no_permitido`.
- Acceptance: spec "Valid/Folder not found" folder, "Allowed/Disallowed type", "Resume after interruption", "Oversize transfer".
- Verification: `cd desktop && go test ./lan/... ./api/... -run "TestSubidas|TestLinks|TestPuenteLAN"`
- Est. changed lines: ~390

## Phase 7 — Unit 5a: control API + wiring + stop triggers

- [ ] 5a.1 `desktop/api/receptor.go`: `ControlReceptor` port impl; routes `POST /api/receptor/{estado,activar,desactivar,codigo/renovar,solicitudes/{id}/aprobar|rechazar}`, `DELETE /api/receptor/dispositivos/{id}`; exact-Origin check.
- [ ] 5a.2 `desktop/api/server.go`: mount receptor routes.
- [ ] 5a.3 `desktop/main.go`: wiring (`lan.NuevoReceptor` + adapters) + shutdown hook calling `Receptor.Stop`; completes the remaining stop triggers from 2a.5 (screen close, explicit disable, app exit).
- [ ] 5a.4 Tests: exact-Origin 403; `desactivar` via keepalive path; puente records origin=`celular`/actor=device name.
- Acceptance: spec "Reception enabled/off" end-to-end; Origin Attribution "Mobile-originated event".
- Verification: `cd desktop && go build ./... && go test ./api/...`
- Est. changed lines: ~270

## Phase 8 — Unit 5b: UI + Private guide

- [ ] 5b.1 `desktop/ui/src/ReceptorMovil.jsx`: states Desactivado→Activando→Activo (QR+`qr_texto` copy+interface picker+renew)→Solicitud(SAS)→Dispositivos/Transferencias; poll `estado` every 1.5s; unmount/`pagehide` → `POST desactivar` (`fetch keepalive:true`).
- [ ] 5b.2 `desktop/ui/src/App.jsx`: add `recibir` sidebar entry, own API calls, presentational component.
- [ ] 5b.3 `desktop/ui/src/api.js`: add `receptor*` client functions.
- [ ] 5b.4 `desktop/ui/src/styles.css`: guide panel for `puerto_ocupado`/`red_no_privada`/`desconocida` ("Marcar la red como Privada" + Reintentar); 60s no-request checklist hint.
- Acceptance: spec "Reception enabled", "Public profile" guide, Windows Network Profile Requirement scenarios.
- Verification: `cd desktop/ui && pnpm build` (no JS test harness in repo — manual visual check of screen states).
- Est. changed lines: ~320

## Phase 9 — Unit 6: installer + docs

- [ ] 6.1 `installer/bythos.iss`: `PrivilegesRequired=admin`; `[Run]` delete+add firewall rule (`profile=private remoteip=localsubnet`, `runhidden`); `[UninstallRun]` delete rule; `InitializeSetup` HKCU migration (silent uninstall, `%APPDATA%` untouched); `runasoriginaluser` launch.
- [ ] 6.2 `README.md`, `CHANGELOG.md` (`[Unreleased]`), `docs/AGENTES.md`/`docs/DESARROLLO.md`: document mobile-reception feature (Spanish, per project convention).
- Acceptance: spec "Fresh install", "Uninstall", "Migration".
- Verification: no installer CI — manual only (tasks 13.1–13.3).
- Est. changed lines: ~170

## Phase 10 — Unit 7a: Android pairing/pinning/keystore/scanner

- [ ] 7a.1 Gradle module `mobile/android` (`:app`), minSdk 26 / target 36, deps androidx core/activity/work-runtime-ktx/play-services-code-scanner (no OkHttp/Compose/Room); `AndroidManifest.xml` declares `ACCESS_LOCAL_NETWORK`.
- [ ] 7a.2 `PinnedTrustManager.kt`: `X509TrustManager` comparing SHA-256(chain[0].publicKey.encoded) base64url constant-time; throws `PinMismatchException` on mismatch/`checkClientTrusted`.
- [ ] 7a.3 `KeystoreTokenStore.kt`: Keystore AES-GCM key encrypts token in SharedPreferences.
- [ ] 7a.4 `PairingActivity.kt` + `ScannerFallback.kt`: `GmsBarcodeScanning`; manual-paste screen for `bythos://pair?...` when Play Services unavailable/install fails.
- [ ] 7a.5 JVM tests (pure, no Robolectric): QR parse; pin TrustManager against fixture cert (shared SPKI golden from 2a.6); keystore round-trip logic.
- Acceptance: spec "Fingerprint match/mismatch"; client-side "Valid/Expired/Reused code".
- Verification: `cd mobile/android && ./gradlew :app:testDebugUnitTest`
- Est. changed lines: ~380

## Phase 11 — Unit 7b: HttpsURLConnection client + failure classification

- [ ] 7b.1 `ApiClient.kt`: per-request `HttpsURLConnection` with pinned `SSLContext`+hostnameVerifier; connect 5s/read 60s; `setFixedLengthStreamingMode` for chunk POST; GET/POST only.
- [ ] 7b.2 `LinksApi.kt`/`FoldersApi.kt`: typed `POST /v1/links`, `GET /v1/carpetas`.
- [ ] 7b.3 `FailureClassifier.kt`: `clasificarFallo(ex, sdkInt, permisoRedLocal)` → `PIN_CAMBIO|PERMISO_RED_LOCAL|INALCANZABLE|NO_EMPAREJADO`.
- [ ] 7b.4 Tests vs `com.sun.net.httpserver.HttpsServer` fixture on 127.0.0.1 (Go fixture key/cert): pin match/mismatch, hostname reject, 409 resume, 422 retry, fixed-length streaming; `clasificarFallo` table test.
- Acceptance: spec "Permission granted/denied" (classification only), Folder Target Validation (client contract), Certificate Pinning.
- Verification: `cd mobile/android && ./gradlew :app:testDebugUnitTest`
- Est. changed lines: ~330

## Phase 12 — Unit 7c: outbox/worker/share

- [ ] 7c.1 `EnviosDbHelper.kt` (`SQLiteOpenHelper`): `envios` table (tipo, uri/url, nombre, tamano, sha256, carpeta_id, cliente_id, subida_id, offset, estado, error).
- [ ] 7c.2 `ShareReceiverActivity.kt`: `ACTION_SEND`/`SEND_MULTIPLE`, `takePersistableUriPermission` or cache-copy with free-space check, `ACTION_OPEN_DOCUMENT`.
- [ ] 7c.3 `UploadWorker.kt`: unique work per envio, `UNMETERED` constraint, `setForeground` `dataSync` FGS; sha256 before create; `/estado` on resume; 401 → mark unpaired; `onStopped` persists offset.
- [ ] 7c.4 Tests: outbox state-machine transitions; worker resume-from-offset (fake `ApiClient`).
- [ ] 7c.5 Instrumented smoke (manual/CI-optional): keystore round-trip, share routing, scanner-unavailable fallback.
- Acceptance: spec "Share to app", "Resume after kill", "Source deleted mid-transfer".
- Verification: `cd mobile/android && ./gradlew :app:testDebugUnitTest`
- Est. changed lines: ~370

## Phase 13 — Manual Verification (cannot be automated)

- [ ] 13.1 Firewall rule, admin account: fresh install; confirm inbound rule `profile=private remoteip=localsubnet`; connect from a paired phone; confirm no Public-profile rule is ever created.
- [ ] 13.2 Firewall rule, standard account + UAC elevation: repeat 13.1; document the over-the-shoulder elevation limitation (accepted per design open questions, not solved).
- [ ] 13.3 Per-user → per-machine install migration: on a machine with a prior HKCU install, confirm silent HKCU uninstall, `%APPDATA%` preserved, no duplicate Start Menu entries.
- [ ] 13.4 Android local-network permission on a real/emulated device enforcing it: deny → confirm "local network access denied" hint + settings button; grant → confirm normal connection.
- [ ] 13.5 Windows network-category detection on real hardware: toggle a real adapter Private/Public/Domain; confirm NLM COM detection matches Windows Settings and gates listener start/UI guide correctly.

## Key Learnings

1. All 12 design units are individually sized at or under the 400-line budget, but their sum (~3,500 lines) forces chained delivery.
2. Four spec-critical checks (firewall, install migration, Android permission, network-category on real hardware) cannot be automated and must stay manual gates before release.
3. Smart App Control on the dev machine can block freshly compiled Go test binaries; `go test -c` into a scratch dir is the documented workaround, not a blind retry.
4. Chain strategy is feature-branch-chain on tracker branch `feat/bythos-movil-qr`, decided by the user 2026-09-30 — `sdd-apply` starts unit 1 next.
