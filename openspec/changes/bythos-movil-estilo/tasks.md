# Tasks: Android Visual Parity and Sending Tutorial (bythos-movil-estilo)

Paths: `K` = `mobile/android/app/src/main/kotlin/com/bythos/movil`, `T` = `mobile/android/app/src/test/kotlin/com/bythos/movil`, `AT` = `mobile/android/app/src/androidTest/kotlin/com/bythos/movil`, `R` = `mobile/android/app/src/main/res`, `M` = `mobile/android/app/src/main/AndroidManifest.xml`.

All Gradle commands run from `mobile/android`.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | PR1 ~330 (range 300-380); PR2 ~300 text lines (range 270-350) plus binaries (3 TTF) in separate commits |
| 400-line budget risk | PR1 Medium; PR2 Medium (text lines only; binaries excluded) |
| Chained PRs recommended | Yes |
| Suggested split | PR1 style/theme, PR2 tutorial + Inter font + icon |
| Delivery strategy | ask-on-risk |
| Chain strategy | stacked (confirmed by the user 2026-10-12) |

Decision needed before apply: No

size:exception (accepted by the user 2026-10-12): PR1 measured 433 changed lines (120 tracked + 313 new XML/Kotlin), 33 over the 400 budget; moving the card pieces to PR2 would save only ~15 lines.
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: Medium

Resolved 2026-10-12: the user chose stacked PRs for the two-PR chain (PR1 `feat/bythos-movil-estilo-01-tema` based on `feat/bythos-movil-feedback`; PR2 `feat/bythos-movil-estilo-02-tutorial` based on PR1) and the chain strategy. Combined ~630 text lines would exceed the 400-line budget as a single PR.

### Suggested Work Units

| Unit | Goal | PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|----|----------------------|-----------------|-------------------|
| A | Theme tokens, styles, drawables, dialog theme | PR1 | `./gradlew :app:assembleDebug` | A54 visual check | `R/values`, `R/color`, `R/drawable`, manifest theme |
| B | `Estilo.kt` + restyle of the three screens, neutral voice, no link text | PR1 | `./gradlew :app:testDebugUnitTest` | A54 (insets, dialog, keyboard) | `Estilo.kt`, `PairingActivity.kt`, `ShareReceiverActivity.kt`, `ScannerFallback.kt`, strings |
| C | Tutorial screen, decision function, home entry, auto-open | PR2 | `./gradlew :app:testDebugUnitTest` | A54 first launch / reopen / share | `TutorialActivity.kt`, `PairingActivity.kt`, manifest, strings, tests |
| D | Inter font + OFL license | PR2 (binary commit separate) | `./gradlew :app:assembleDebug` | A54 font visible | `R/font`, `assets/licenses`, `themes.xml` font lines |
| E | Adaptive launcher icon | PR2 | `./gradlew :app:assembleDebug` | A54 launcher, circle/squircle masks | `R/drawable/marca*.xml`, `R/mipmap-anydpi-v26`, manifest icon |

## PR1: Style (branch `feat/bythos-movil-estilo-01-tema`, base `feat/bythos-movil-feedback`)

### Unit A: Theme resources

- [x] A.1 Create `R/values/colors.xml` with the 14 spec tokens (bg, bg_alt, surface, surface_2, surface_3, border, border_strong, ink, ink_muted, ink_dim, ink_accent, green, amber, red). (spec: Dark Desktop Theme)
- [x] A.2 Create `R/values/dimens.xml`: 24dp screen padding, 12dp card radius, 16dp dialog radius, 8dp button radius, 1dp border, 48dp touch target, text sizes. (spec: Touch Sizing and Insets)
- [x] A.3 Create `R/values/themes.xml`: `Tema.Bythos` (parent `@android:style/Theme.Material.NoActionBar`, attrs per design, `fontFamily` = `sans-serif`), `Tema.Bythos.Dialogo`, `Boton`, `Boton.Primario`, `Boton.Secundario`, and 5 TextAppearances (Titulo 22sp/700, Panel 16sp/600, Cuerpo 15sp/400, Tenue 13sp, Micro 11-12sp/600 uppercase). (spec: Typography, Button Styles)
- [x] A.4 Create `R/color/texto_boton_primario.xml` and `texto_boton_secundario.xml` (enabled/disabled).
- [x] A.5 Create drawables `boton_primario.xml`, `boton_secundario.xml` (selectors: pressed, disabled, default; 8dp radius; secondary has 1dp border_strong), `tarjeta.xml`, `campo.xml` (focused = border_strong), `fondo_dialogo.xml` (inset 16dp, surface, 1dp border_strong, 16dp radius). (spec: Button Styles, Themed Folder Dialog)
- [x] A.6 In `M` set `android:theme="@style/Tema.Bythos"` on the application.
- [x] A.7 Verify: `./gradlew :app:assembleDebug` succeeds.

### Unit B: Estilo.kt and screens

- [x] B.1 Create `K/Estilo.kt`: `Context.dp`, `Tono`, and `Estilo` factory (`titulo`, `panel`, `cuerpo`, `tenue`, `botonPrimario`, `botonSecundario`, `campo`, `tarjeta`, `encabezado`, `tono`, `pantalla`). `pantalla` builds header + `ScrollView` (24dp padding), calls `WindowCompat.setDecorFitsSystemWindows(window,false)` and the `ViewCompat.setOnApplyWindowInsetsListener` for systemBars/displayCutout/ime. Use `ScrollView` so `android.R.id.content` traversal still finds the views. (spec: Brand Header, Touch Sizing and Insets)
- [x] B.2 Modify `K/PairingActivity.kt`: use `Estilo.pantalla`; primary "Escanear QR" (unpaired) or "Enviar un archivo a la PC" (paired); everything else secondary; at most one primary per screen; `Tono` in `mostrar`. Keep button class and visible text unchanged for the instrumented test. (spec: Button Styles)
- [x] B.3 Modify `K/ShareReceiverActivity.kt`: use `Estilo.pantalla`, `Tono`, green "Listo" via one `ForegroundColorSpan`; show link items as `enlace_compartido` instead of the URL. The folder `AlertDialog.Builder(this)` stays unchanged (themed via `alertDialogTheme`). (spec: Themed Folder Dialog, Privacy)
- [x] B.4 Modify `K/ScannerFallback.kt`: use `Estilo.campo` and `Estilo.botonPrimario`; ensure the pasted text is never echoed back. (spec: Privacy)
- [x] B.5 Modify `R/values/strings.xml`: `pc_no_recibe` = "La PC no está recibiendo. Abre «Recibir del celular» en la PC y escanea el QR de nuevo."; `pegar_pista` = "Pega el texto que copiaste de la PC"; add `enlace_compartido`. Review all strings for voseo and for IP/port/URL echo. (spec: Neutral Voice, Privacy)
- [x] B.6 Verify: `./gradlew :app:testDebugUnitTest` green; `./gradlew :app:assembleDebug` succeeds.
- [x] B.7 Verify on the A54: `./gradlew :app:connectedDebugAndroidTest` green (existing `CompartirYFallbackInstrumentedTest`; the tutorial does not exist yet in PR1).

### PR1 boundary

Open PR1 with the manual checklist M.1-M.5 below in its description. PR2 branches from PR1.

## PR2: Tutorial, font and icon (branch `feat/bythos-movil-estilo-02-tutorial`, base `feat/bythos-movil-estilo-01-tema`)

### Unit C: Tutorial

- [ ] C.1 RED: create `T/TutorialDecisionTest.kt`: all 16 combinations of `debeAbrirTutorial(desdeLanzador, emparejado, visto, recreada)`; only `(true,false,false,false)` returns true. (spec: First-Launch Auto-Open, Share-Intent Exclusion)
- [ ] C.2 Create `K/TutorialActivity.kt` with pure `debeAbrirTutorial` and `CLAVE_TUTORIAL_VISTO = "tutorialVisto"` (prefs `bythos`); screen via `Estilo.pantalla`: title, five `tarjeta` steps (numbered, in spec order), primary "Entendido" calling `finish()`; Back closes by default. Text only. (spec: Tutorial Content, Visual Consistency)
- [ ] C.3 Add strings in `R/values/strings.xml`: `como_enviar` ("Cómo enviar archivos"), `tutorial_titulo`, `tutorial_paso_1..5`, `entendido`. Step 2 uses `getString(R.string.escanear_qr)`, step 4 `getString(R.string.enviar_archivo)` ("Enviar un archivo a la PC"), step 5 `getString(R.string.fase_listo)`, brand via `app_name`; desktop labels ("Recibir del celular", "Activar recepción", "Aprobar") stay literal. Neutral "tú"; no IP, port, URL or link text. (spec: Labels match the UI, Tutorial Text Privacy)
- [ ] C.4 Modify `K/PairingActivity.kt`: secondary `como_enviar` button in both paired and unpaired modes (48dp, does not replace the primary action; opens the tutorial without touching the flag); auto-open block in `onCreate` computing `desdeLanzador = intent.action == ACTION_MAIN`, `emparejado`, `visto`, `recreada = savedInstanceState != null`; write the flag with `apply()` BEFORE `startActivity`. `ShareReceiverActivity` never calls it. (spec: Permanent Home Entry, Share-Intent Exclusion)
- [ ] C.5 Modify `M`: register `TutorialActivity` with `exported=false`.
- [ ] C.6 Modify `AT/CompartirYFallbackInstrumentedTest.kt`: `@Before` sets `tutorialVisto=true` in prefs `bythos` so the launcher scenario does not auto-open the tutorial.
- [ ] C.7 Verify: `./gradlew :app:testDebugUnitTest` green; `./gradlew :app:connectedDebugAndroidTest` green on the A54.

### Unit D: Inter font and license

- [ ] D.1 Add `assets/licenses/Inter-OFL.txt` (verbatim SIL OFL 1.1 text, in `app/src/main/assets/licenses/`) and keep it in the source tree. (spec: License shipped)
- [ ] D.2 Binary commit (separate): add `R/font/inter_regular.ttf`, `inter_semibold.ttf`, `inter_bold.ttf` (static 400/600/700).
- [ ] D.3 Create `R/font/inter.xml` family; in `themes.xml` set `fontFamily` to `@font/inter` in the theme and each TextAppearance (per-role static file, no `fontWeight`; minSdk 26). No Kotlin change. (spec: Typography, Font fallback via resource fallback to `sans-serif`)
- [ ] D.4 Verify: `./gradlew :app:assembleDebug` succeeds; the license file is present in the built APK assets.

### Unit E: Launcher icon

- [ ] E.1 Add color `icono_fondo` (#0B0B0C) to `colors.xml`; create `R/drawable/marca.xml` (vector copying the rectangle/sine geometry of `gen-icons.mjs`), `R/drawable/ic_launcher_marca.xml` (`<inset>` of 27% over `marca.xml`). (spec: Launcher Icon)
- [ ] E.2 Create `R/mipmap-anydpi-v26/ic_launcher.xml` and `ic_launcher_round.xml` (adaptive: background `@color/icono_fondo`, foreground and monochrome `@drawable/ic_launcher_marca`).
- [ ] E.3 Modify `M`: `android:icon="@mipmap/ic_launcher"` and `android:roundIcon="@mipmap/ic_launcher_round"`.
- [ ] E.4 Verify: `./gradlew :app:assembleDebug` succeeds (resource linking passes).

### PR2 final verification

- [ ] F.1 From `mobile/android`: `./gradlew :app:testDebugUnitTest`, `./gradlew :app:assembleDebug`, and `./gradlew :app:connectedDebugAndroidTest` (A54 connected) are all green.
- [ ] F.2 Open PR2 with the manual checklist below in its description.

## Manual verification checklist (Samsung A54, API 36)

- [ ] M.1 Phone in light mode: all screens (home, share, paste, tutorial) and the folder dialog render dark.
- [ ] M.2 Gesture navigation: header sits below the status bar/cutout; bottom-most button is fully visible and tappable above the gesture bar. Partial 2026-10-12: the user confirmed the dark look on the A54 (home, folder dialog); the per-item details are not checked yet.
- [ ] M.3 3-button navigation: same as M.2 with the three-button bar.
- [ ] M.4 Folder dialog: dark surface, rounded corners, `ink` text, secondary-styled buttons, rows at least 48dp; link shares show "enlace" text, never a URL.
- [ ] M.5 Manual paste: keyboard open does not cover the paste field; hint reads "Pega el texto que copiaste de la PC"; nothing pasted is echoed back.
- [ ] M.6 Buttons: pressed and disabled states are visibly distinct; one primary per screen; at 200% font scale the screens scroll and every control stays reachable.
- [ ] M.7 Voice and privacy: stale-QR message reads "La PC no está recibiendo. Abre «Recibir del celular» en la PC y escanea el QR de nuevo."; no voseo and no IP/port/URL anywhere (pairing errors included); legacy pairing shows "Emparejado con tu PC".
- [ ] M.8 (PR2) Icon: Bythos adaptive icon on the launcher, not clipped under circle and squircle masks; themed (monochrome) icon works if the launcher supports it.
- [ ] M.9 (PR2) Inter is visible (titles 700, buttons 600, body 400).
- [ ] M.10 (PR2) Tutorial first launch: fresh install, unpaired, launcher open -> tutorial opens once; kill and relaunch -> does not open again.
- [ ] M.11 (PR2) Tutorial reopen: "Cómo enviar archivos" opens it again in both unpaired and paired home; "Enviar un archivo a la PC" stays the primary action when paired; "Entendido" and Back both close it.
- [ ] M.12 (PR2) Tutorial not on share: with fresh install (flag unset), share a file from another app -> share flow runs without the tutorial and the flag stays unset; later launcher open still auto-opens it once.
- [ ] M.13 (PR2) Upgrade case: paired device with flag unset opens from launcher without tutorial.
