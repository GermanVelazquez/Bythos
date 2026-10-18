# Design: Bythos Mobile QR Reception (bythos-movil-qr) — rev 2 (corrective)

## Technical Approach
Same-binary second listener (proposal approach). New hexagonal package `desktop/lan` owns TLS, pairing, auth, uploads, network-category detection. It depends only on `db`, `archivos` and two ports; `api` implements the ports (reusing the SSRF metadata client and the Paso 0 save sequence) and exposes a strict-origin control API. `main.go` wires 3 lines. Android is a separate Gradle project in `mobile/android` using platform `HttpsURLConnection` (no OkHttp).

    Phone --TLS1.3 pinned (HttpsURLConnection)--> lan.Receptor (IP:48080, /v1/*)
                               | auth Bearer -> db.dispositivos
                               | Biblioteca port -> api.puenteLAN -> metadata / archivos.Guardar / db / eventos(celular)
    UI (WebView2) --POST /api/receptor/* (exact Origin)--> api.receptor -> ControlReceptor port -> lan.Receptor
    lan.Receptor --in-process COM (NLM)--> network category of bound adapter

## Rev 2 changes (confirmed decisions)
1. Android HTTP = `HttpsURLConnection`; all upload mutations are POST (no PATCH/HEAD/DELETE on /v1).
2. QR scanner = Play Services code scanner + manual paste fallback.
3. PC QR = `rsc.io/qr`.
4. Reception only on Private network category; firewall rule Private only; in-app guide to mark network Private; never a Public rule.
5. Remote guard = same subnet as the bound interface (explicit chosen rule; stricter than RFC1918). Spec wording "private-range" must be updated to this.
6. Listener stops on screen close, explicit disable, short reception window (10 min without a successful request after start; 2 min after the last successful request once used), app exit.
7. Explicit folder-not-found contract (404) on /v1/links and /v1/subidas.
8. Android local-network-permission-denied UX.

## Architecture Decisions
| Topic | Choice | Rejected | Rationale |
|---|---|---|---|
| Package boundaries | `lan` defines ports `Biblioteca` (ListarCarpetas, ExisteCarpeta, GuardarLink, GuardarArchivo) and is consumed by `api` via `ControlReceptor`; `api/puente_lan.go` adapts | `lan` importing `api` (cycle), duplicate metadata client | No cycle, one metadata fetcher, fakes in tests |
| Listener lifecycle | Off by default. Starts on UI activate. Stops on: (a) screen close — `ReceptorMovil` unmount effect and `pagehide` send `POST /api/receptor/desactivar` (fetch `keepalive:true`); (b) explicit disable toggle; (c) short reception window: 10 min after start with no successful request, or 2 min after the last successful request (only responses with status < 400 count; 403/401/error responses never extend it), and no active upload; (d) app exit (`main` shutdown hook). Stop = `http.Server.Shutdown` with 10s grace (in-flight chunk ≤8 MiB completes) then `Close` | Always bound + gated flag; UI heartbeat lease | If the UI dies without sending stop (WebView crash, killed renderer), the idle timer is the backstop; uploads interrupted by a stop resume later via offset protocol. Heartbeat rejected: WebView2 throttles timers when minimized |
| Bind | Chosen interface IP only, fixed port 48080, no fallback (busy -> UI error `puerto_ocupado`) | 0.0.0.0, ephemeral port | Firewall rule pins localport; stable address for paired phones |
| Remote guard (CHOSEN RULE) | Remote IPv4 MUST be inside the bound interface's subnet (IP & mask of the bound address); else 403 `origen_no_permitido` + close, no processing, no event. Seam `var remotoPermitido` for loopback tests | RFC1918-only check | Since the bound interface is itself RFC1918, same-subnet ⊂ private-range and is strictly narrower; mirrors firewall `remoteip=localsubnet` |
| Network category gate | Before `Listen`, detect category of the bound adapter in-process: `lan/red_windows.go` uses COM `INetworkListManager` (CLSID DCB00C01-570F-4A9B-8D69-199FDBA5723B) via `golang.org/x/sys/windows` (promoted to direct dep) + `ole32.CoCreateInstance` lazy proc; `GetNetworkConnections` -> per `INetworkConnection`: `GetAdapterId` matched against `GetAdaptersAddresses().AdapterName` of the bound IP -> `GetNetwork().GetCategory()`. `runtime.LockOSThread` + `CoInitializeEx(MTA)` per call. Result `privada|publica|dominio|desconocida`. `publica`/`dominio` -> refuse start, error `red_no_privada` + guide. `desconocida` (COM failure) -> start, show guide banner; firewall remains the enforcement. `red_other.go` stub returns `desconocida`. Port `var categoriaRed func(ip) (Categoria, error)` for tests | Spawning PowerShell `Get-NetConnectionProfile` at runtime; registry `NetworkList\Profiles` scan | No runtime subprocess (no shell boundary, no console flash, no AV heuristics). Registry cannot map a profile to the active adapter reliably. COM NLM is the documented per-connection API, readable by standard users |
| Firewall | Installer adds rule `profile=private remoteip=localsubnet` only; NEVER a Public/Domain rule; app never modifies firewall | Public rule, runtime netsh | Public networks stay closed by OS even if in-app detection fails |
| Interface selection | Up, non-loopback IPv4 RFC1918, not 169.254; exclude vEthernet/Hyper-V/WSL/VirtualBox/VMware/TAP/WireGuard/Tailscale/ZeroTier; rank Wi-Fi > Ethernet > other; UI override; each candidate annotated with its category | First non-loopback IP | VPN/Hyper-V machines guess wrong otherwise |
| Cert | ECDSA P-256 key persisted `%APPDATA%\Bythos\lan\clave.pem`; self-signed cert re-issued at each start with current IP SANs (10y, same key) | Persist cert; RSA | SPKI SHA-256 base64url (43 chars) stable across IP changes |
| TLS config | MinVersion TLS1.3; ReadHeaderTimeout 10s, Read/Write 60s, Idle 60s, MaxHeaderBytes 16KiB | defaults | Slowloris / limits |
| Pairing secrets | In memory: one active code (16B base64url, TTL 10 min, single use, rotates); max 3 pending; 5 bad codes/min -> rotate + 30s lock | Codes in DB | Valid only while listener up |
| Anti-race | 6-digit SAS = first 20 bits of SHA-256(code‖requestID) mod 10^6 on both screens; PC approves | Name-only approval | Leaked QR cannot be silently raced |
| Tokens | 32B crypto/rand base64url; DB stores SHA-256 hex UNIQUE | bcrypt/argon2 | High entropy, fast lookup |
| Upload state | Sidecar JSON + `.parte` in `%APPDATA%\Bythos\lan\subidas`; running SHA-256 state via `MarshalBinary` | DB table; re-hash at end | No migration, O(1) resume, `archivos` untouched |
| Finalization | Async: last chunk -> 202, worker verifies whole hash, re-checks folder, then `archivos.Guardar` -> CrearArchivo -> GuardarArchivo -> evento; phone polls estado | Synchronous finalize | 2 GiB save may exceed phone timeouts |
| History origin | `db.OrigenCelular="celular"` accepted ONLY by `db.normalizarOrigen`; `api.origenDeCabeceras` unchanged; actor = device name from DB; `AccionDispositivoEmparejado/Revocado` (origen app) | Accept header | Local clients cannot forge `celular` |
| Control API | POST/DELETE with EXACT Origin (`http://localhost:8080`; Vite only with BYTHOS_DEV=1), like `agente.go`; state read `POST /api/receptor/estado` | GET + Sec-Fetch-Site | Extension must never read the pairing code |
| QR rendering | Go-side PNG data URI via `rsc.io/qr` | JS lib | Presentational UI, one tiny dep |

## Data Model
```sql
CREATE TABLE IF NOT EXISTS dispositivos (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  nombre TEXT NOT NULL,              -- <=80 chars, sanitized
  token_hash TEXT NOT NULL UNIQUE,
  creado_en TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  ultimo_uso TEXT, revocado_en TEXT);
```
`crearTablaDispositivos` from `crearTablas` before `crearTablaEventos`. Funcs: `CrearDispositivo`, `DispositivoPorTokenHash` (non-revoked), `ListarDispositivos`, `RevocarDispositivo`, `TocarDispositivo` (1/min).

## Wire Protocol (LAN)
Errors: `{"error":"<codigo>","mensaje":"<es text>"}`. Codes: `token_invalido`(401), `origen_no_permitido`(403), `codigo_invalido`/`codigo_expirado`(410)/`bloqueado`(429), `carpeta_no_encontrada`(404), `subida_no_encontrada`(404), `offset_invalido`(409, body includes `offset`), `subida_ocupada`(409), `tamano_excedido`(413), `hash_parte_invalido`(422), `tipo_no_permitido`(415), `demasiadas_subidas`(429), `cuerpo_invalido`(400).
QR: `bythos://pair?v=1&h=<ip>&p=48080&k=<spki43>&c=<code22>&n=<pcName>` (EC level M).

| Route | Auth | Contract |
|---|---|---|
| GET /v1/salud | none | `{app:"bythos",api:1,version}` |
| POST /v1/emparejar | code | `{codigo,nombre}` -> 202 `{solicitud,sas}` |
| GET /v1/emparejar/{solicitud} | request id secret | `{estado:pendiente\|aprobado\|rechazado,token?,dispositivo_id?}`; token once |
| GET /v1/carpetas | Bearer | `[{id,nombre}]` |
| POST /v1/links | Bearer | `{carpeta_id,url,titulo?}`; folder checked FIRST (before metadata fetch) -> 404 `carpeta_no_encontrada`, nothing written, no event; else 201 `{recurso_id,titulo}` |
| POST /v1/subidas | Bearer | create `{cliente_id(uuid),carpeta_id,nombre,tamano,sha256}`; folder missing -> 404 `carpeta_no_encontrada` with NO sidecar/.parte created; tamano>TamanoMaximo -> 413; max 3 active/device; idempotent on (device,cliente_id); 201 `{id,offset,chunk_max:8MiB}` |
| POST /v1/subidas/{id}/estado | Bearer+owner | query, empty body -> 200 `{estado:recibiendo\|verificando\|listo\|error,offset,tamano,recurso_id?,error?}` (replaces HEAD + GET) |
| POST /v1/subidas/{id}/partes | Bearer+owner | headers `X-Offset`, `X-Chunk-SHA256` (hex), `Content-Length` required; body octet-stream ≤chunk_max (MaxBytesReader). Offset mismatch -> 409 with current offset; offset+len>tamano -> 413 and upload discarded; bad chunk hash -> truncate to prior offset, restore hash state, 422; concurrent (TryLock) -> 409 `subida_ocupada`; 200 `{offset}`; 202 `{offset,estado:"verificando"}` when complete |
| POST /v1/subidas/{id}/cancelar | Bearer+owner | delete sidecar + temp -> 204 |

Cumulative cap = declared tamano ≤ TamanoMaximo, enforced per chunk. Finalize: whole-hash mismatch -> `error`; folder deleted meanwhile -> `error:carpeta_no_encontrada`; magic-byte reject -> `error:tipo_no_permitido`; all error paths delete temp, record no event. Crash recovery: on load truncate `.parte` to sidecar offset (sidecar temp+rename after each verified chunk). Cleanup: on start + hourly (idle >24h), on revoke, on completion.

Control API: `POST /api/receptor/estado|activar{ip?}|desactivar|codigo/renovar|solicitudes/{id}/aprobar|solicitudes/{id}/rechazar`, `DELETE /api/receptor/dispositivos/{id}`. Estado: `{activo,ip,puerto,categoria_red,interfaces[{ip,nombre,categoria}],qr_png,qr_texto,codigo_expira,solicitudes[{id,nombre,sas}],dispositivos[],transferencias[],ultima_conexion,error?}`.

## UI (`ReceptorMovil.jsx`, sidebar `recibir`)
States: Desactivado -> Activando -> Activo (QR + copy `qr_texto` + interface picker + renew) -> Solicitud (name + SAS) -> Dispositivos/Transferencias. Errors: `puerto_ocupado`, `red_no_privada` -> guide panel "Marcar la red como Privada" (Settings > Network & Internet > Wi-Fi/Ethernet > properties > Private network; button opens `ms-settings:network-status` via existing open-URL path if present, else text only) with "Reintentar". `desconocida` -> same guide as banner. No LAN request 60s after QR: checklist hint (same Wi-Fi, network Private, AP isolation, VPN). Polls estado every 1.5s while visible; unmount/pagehide sends desactivar. App.jsx owns calls via `api.js`; component presentational.

## Installer
`PrivilegesRequired=admin`, `DefaultDirName={autopf}\Bythos`, same AppId. [Run]: `{sys}\netsh.exe advfirewall firewall delete rule name="Bythos (celular)"` then `add rule ... dir=in action=allow program="{app}\bythos.exe" protocol=TCP localport=48080 profile=private remoteip=localsubnet` (runhidden). No public/domain rule ever. Launch with `runasoriginaluser`. [UninstallRun] delete rule. Migration in `InitializeSetup`: HKCU `{AEEA0123-...}_is1` UninstallString `/VERYSILENT /SUPPRESSMSGBOXES /NORESTART`; %APPDATA% untouched.

## Android (`mobile/android`, `:app`, Kotlin)
| Topic | Choice | Rationale |
|---|---|---|
| SDK | minSdk 26, compile/target 36; declare `android.permission.ACCESS_LOCAL_NETWORK`; runtime request when SDK_INT≥37 (string literal) | Additive bump later |
| Deps | androidx core/activity, work-runtime-ktx, play-services-code-scanner; platform Views; NO OkHttp, no Compose/Room | Minimal |
| HTTP | `HttpsURLConnection` per request: `sslSocketFactory` from pinned `SSLContext("TLS")`, `hostnameVerifier` accepting only the stored host; connect timeout 5s, read 60s; chunk POST uses `setFixedLengthStreamingMode(len)` (no 8 MiB buffering); only GET/POST used | Platform API, no dep; POST-only routes avoid PATCH gap |
| Pinning | X509TrustManager: SHA-256(chain[0].publicKey.encoded) base64url, constant-time compare, throws `PinMismatchException`; checkClientTrusted throws; NSC cleartext disabled | Mismatch -> "El certificado de la PC cambió: volvé a escanear el QR", all work stopped |
| Scanner | `GmsBarcodeScanning` (no CAMERA permission). If Play Services missing/module install fails -> manual paste screen for `bythos://pair?...` (PC shows copyable `qr_texto`). Devices without Play Services always use the fallback | Explicit degradation |
| Failure UX | Pure-Kotlin `clasificarFallo(ex, sdkInt, permisoRedLocal)`: `SSLHandshakeException` caused by pin -> PIN_CAMBIO; timeout/`ConnectException`/`NoRouteToHost`/`SocketException` EPERM with SDK≥37 and permission not granted -> PERMISO_RED_LOCAL: hint "Bythos no tiene acceso a la red local" + button `ACTION_APPLICATION_DETAILS_SETTINGS`; same with permission granted or SDK<37 -> INALCANZABLE: checklist "misma Wi-Fi, recepción activa en la PC, red marcada como Privada"; 401 -> NO_EMPAREJADO | Short timeout gives fast feedback; permission check disambiguates |
| Secrets | Keystore AES-GCM key encrypts token in SharedPreferences | security-crypto deprecated |
| Re-scan | Same pin + token -> update host/port; different pin -> re-pair | IP changes |
| Share | ACTION_SEND/SEND_MULTIPLE; takePersistableUriPermission or copy to cache (free-space check); in-app ACTION_OPEN_DOCUMENT | Temporary grants |
| Outbox | SQLiteOpenHelper `envios` (tipo, uri/url, nombre, tamano, sha256, carpeta_id, cliente_id, subida_id, offset, estado, error) | Survives kill |
| Worker | Unique work per envio, UNMETERED, setForeground dataSync; sha256 before create; `/estado` on resume; 401 -> unpaired; onStopped persists offset | Resume source of truth |

## File Changes
| File | Action |
|---|---|
| `desktop/db/dispositivos.go`, `db.go`, `eventos.go` | Create/Modify |
| `desktop/lan/{cert,interfaces,red,red_windows,red_other,receptor,guardia,emparejar,auth,subidas_store,subidas,links,qr,respuestas}.go` + tests | Create |
| `desktop/api/puente_lan.go`, `api/receptor.go`, `api/server.go` | Create/Modify |
| `desktop/main.go`, `go.mod` | Modify (wiring, shutdown stop, rsc.io/qr, x/sys direct) |
| `desktop/ui/src/ReceptorMovil.jsx`, `App.jsx`, `api.js`, `styles.css` | Create/Modify |
| `installer/bythos.iss`, `README.md`, `CHANGELOG.md`, `docs/*` | Modify |
| `mobile/android/**` | Create |

## Testing Strategy
| Unit | Tests |
|---|---|
| db | idempotent table, hash lookup, revoked excluded, `celular` normalized, header cannot forge `celular` |
| lan cert/iface | key reuse, SPKI stable, IP SANs; ranking with fake list; golden SPKI fixture shared with Android |
| lan red | fake `categoriaRed`: publica/dominio refuse start with `red_no_privada`; desconocida starts with warning; windows-tagged smoke test that NLM call returns without error on CI host (skip if COM unavailable) |
| lan listener | httptest TLS + pinned client; non-subnet remote 403; off = refused; idle auto-off with fake clock; Stop during in-flight chunk completes then refuses; stop idempotent |
| pairing | TTL, single use, lock, SAS, token once, revoked 401 |
| uploads | create/estado/partes/cancelar; resume after restart; 409/413/422; TryLock; crash truncation; cap via lowered TamanoMaximo; dedupe; whole-hash mismatch; folder missing at create -> 404 and temp dir empty; folder deleted before finalize -> error + temp removed; links 404 before metadata fetch (fake fetcher not called) |
| api | exact-Origin 403; desactivar via keepalive path; puente origin/actor |
| Android JVM (pure, no Robolectric) | QR parse; pin TM (fixture cert); chunk planner; outbox state machine; `clasificarFallo` table; client vs JDK `com.sun.net.httpserver.HttpsServer` on 127.0.0.1 with the Go fixture key/cert (same `javax.net.ssl` API on JVM): pin match/mismatch, hostname reject, 409 resume, 422 retry, fixed-length streaming |
| Android instrumented | Keystore round-trip, share routing, scanner-unavailable fallback (smoke) |
| Manual | admin + standard firewall; Public network refused + guide; screen close stops listener; 2 GB resume; permission denied hint on Android 17 device/emulator |

## Threat Matrix
Documentation-like paths: N/A (no file-type-driven execution; archivos allowlist unchanged). Git repository selection / Commit state / Push state / PR commands: N/A (no VCS automation). Runtime subprocess: none (network category via in-process COM; PowerShell spawn explicitly rejected). Only subprocess is installer netsh with static arguments — manual installer tests.

## Migration / Rollout
Additive table; listener off by default; installer migrates per-user install. Units (≤~400 lines each): 1 db; 2a lan cert/iface/listener/guard/lifecycle; 2b lan network category (NLM COM + stub + gate); 3 pairing+auth; 4a upload store; 4b upload/link handlers (POST routes, 404 contracts) + puente; 5a control API + wiring + stop triggers; 5b UI + Private guide; 6 installer+docs; 7a Android pairing/pinning/keystore/scanner+fallback; 7b HttpsURLConnection client/links/folders/failure classification; 7c outbox/worker/share.

## Open Questions
- [ ] Fixed port 48080 with no fallback acceptable?
- [ ] Admin install under over-the-shoulder elevation puts HKCU/{userdesktop} on the admin account; acceptable?
- [ ] Should `dominio` category be allowed? (Design: refused, since firewall rule is Private-only.)
