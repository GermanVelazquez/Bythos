Research (2026-09-29) for bythos-movil-qr. Status: done (gaps: PrivilegesRequiredOverridesAllowed primary text unreachable; no QR-truncation standard).

1 Firewall/Inno: Windows default-denies inbound; standard users cannot approve exceptions (MS Learn Q&A). Inno cannot elevate a single step while staying per-user → PrivilegesRequired=admin for whole installer. Rule: `netsh advfirewall firewall add rule name="Bythos" dir=in action=allow program="{app}\bythos.exe" profile=private remoteip=localsubnet enable=yes` in [Run] (delete-then-add on every install/upgrade), delete in [UninstallRun]. Program-path rule keeps working if exe replaced at same path; breaks if path changes. Switching per-user→per-machine changes AppId registry hive (HKCU→HKLM): must detect & uninstall/migrate old per-user install explicitly; user data in %APPDATA% unaffected. Sources: learn.microsoft.com firewall rules, netsh-advfirewall, Q&A 2260674; jrsoftware PrivilegesRequired.

2 Android pinning: OkHttp CertificatePinner can't rescue a self-signed chain → custom X509TrustManager/SSLContext pinning SPKI SHA-256 from QR + custom HostnameVerifier (IP varies); NSC pin-set is build-time only → unusable. Play flags accept-all TrustManagers; OK if checkServerTrusted throws on mismatch. PC cert should include IP SANs anyway. Sources: OWASP MASTG, developer.android.com unsafe-trustmanager.

3 ACCESS_LOCAL_NETWORK: implicit via INTERNET for targetSdk ≤36, opt-in test on Android 16, enforced targetSdk 37 (Android 17); denial = silent timeout → show explicit UI hint. NEARBY_WIFI_DEVICES/location NOT needed. Source: developer.android.com local-network-permission.

4 QR: payload ~100 chars fits easily; keep full 43-char base64url SPKI SHA-256, no truncation; EC level M or Q.

5 Upload: custom tus-inspired offset protocol (HEAD offset / PATCH chunk) to keep deps minimal; chunk few MB (4–8 MB) tunable; per-chunk + whole-file SHA-256; persist upload state for resume after app kill; Android: WorkManager + setForeground with foregroundServiceType=dataSync (Android 15 cap 6h/24h). Sources: tus.io, developer.android.com fgs timeout.

6 Share intents: call takePersistableUriPermission on receipt, persist URI, stream from content:// (no multi-GB copy), handle source deleted mid-transfer.
