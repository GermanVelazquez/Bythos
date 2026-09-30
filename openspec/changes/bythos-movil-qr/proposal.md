# Proposal: Bythos Mobile QR Reception (bythos-movil-qr)

## Intent
Users cannot send links/files from their phone to Bythos. Add a native Android app that pairs by QR and sends links and large files to the PC over the local Wi-Fi via pinned TLS, with no cloud. Paso 0 file support is the prepared base.

## Scope
### In Scope
- Separate LAN HTTPS listener in `bythos.exe`, open only while reception is enabled; private-IP enforcement.
- Self-signed cert (IP SANs) in `%APPDATA%\Bythos`; QR = LAN IP, port, full base64url SPKI SHA-256, one-time code.
- PC-side approval of the first connection; per-device hashed, revocable token.
- Routes: pair, list folders, save link, chunked resumable upload (HEAD offset / PATCH chunk) reassembled into `archivos.Guardar`; cap from cumulative bytes.
- Historial origin `celular` with device name as actor.
- "Recibir del celular" screen: QR, approval, devices, revoke, transfers, reachability hint.
- Installer: admin, private+localsubnet program rule, per-user migration, updated README/release copy.
- Android app in `mobile/android` (Kotlin, minimal deps): scan, pinning TrustManager + HostnameVerifier, share intents, WorkManager + dataSync FGS, "re-scan QR" on cert change, ACCESS_LOCAL_NETWORK.

### Out of Scope
- iPhone; mDNS discovery; PC-to-phone sync; streaming chunk API in `archivos`; Play Store publishing.

## Capabilities
### New Capabilities
- `lan-reception`: listener lifecycle, TLS cert, private-IP guard, routes, chunked upload.
- `device-pairing`: QR, one-time code, approval, tokens, revocation.
- `mobile-android-client`: Android pairing, pinning, sharing, background upload.
- `installer-firewall`: admin install, firewall rule, install-scope migration.
### Modified Capabilities
- `activity-history`: new `celular` origin.

## Approach
Same-binary second listener (exploration approach 1) in new package `desktop/lan`, own mux + device-token auth (not `conGuardia`), reusing `db`, `archivos`, `metadata` SSRF client. Seven chained units (each about 400 lines or fewer):
1. DB `dispositivos` table + `OrigenCelular`.
2. `desktop/lan`: cert, interface selection, start/stop, private-IP guard.
3. Pairing + QR + approval + token routes.
4. Link + resumable upload + Historial events.
5. UI screen `ReceptorMovil.jsx` + local control API.
6. Installer firewall/admin/migration + docs.
7. Android app (tasks may split it into more units).

## Affected Areas
| Area | Impact |
|---|---|
| `desktop/lan/` | New |
| `desktop/db/db.go`, `eventos.go`, `api/origen.go` | Modified |
| `desktop/api/server.go`, `main.go` | Modified (thin wiring) |
| `desktop/ui/src/App.jsx`, `ReceptorMovil.jsx` | Modified/New |
| `installer/bythos.iss`, `README.md` | Modified |
| `mobile/android/` | New |

## Risks
| Risk | Likelihood | Mitigation |
|---|---|---|
| Firewall rule fails on real machines | Med | Test on admin and standard accounts; in-app reachability hint |
| Per-user to admin migration leaves duplicate installs | Med | Detect the HKCU AppId; uninstall silently; data in `%APPDATA%` stays |
| Smart App Control blocks unsigned dev builds | Med | Dev machine with SAC off; signing tracked separately |
| Android 17 local-network enforcement | Low now | Request the permission from v1; show timeout hint |
| Silent LAN timeouts (AP isolation, VPN) | Med | Short timeouts; clear error; manual IP override |

## Rollback Plan
Each unit is a separate PR, reverted with `git revert`. The listener stays off by default, so disabling the screen hides the whole feature. The `dispositivos` table is additive. If needed, the installer reverts to `PrivilegesRequired=lowest`, and uninstall removes the firewall rule.

## Dependencies
- Paso 0 (`desktop/archivos`, `db/archivos.go`, `api/archivos.go`) committed first.
- Android SDK/Gradle toolchain available locally.

## Success Criteria
- [ ] A phone pairs by QR after PC approval; a revoked device is refused.
- [ ] A 2 GB video resumes after interruption and lands deduplicated in the folder chosen on the phone.
- [ ] A cert change shows "re-scan QR" and blocks the connection.
- [ ] The listener is unreachable when reception is off or from non-private IPs.
- [ ] The installer creates/removes the firewall rule and migrates per-user installs.
- [ ] `go vet` and `go test ./...` pass.
