# Design: Android Visual Parity and Sending Tutorial

## Technical Approach

Framework-only styling (no AppCompat/Material/Compose): XML tokens (`colors`, `dimens`, `themes` with styles and TextAppearances), shape/state-list drawables, and a small `Estilo.kt` factory so the screens stay code-built and keep real `Button`s. Typography lives only in TextAppearances, so PR2 swaps in Inter without touching Kotlin. Insets are applied once in `Estilo.pantalla`. The tutorial is a plain `Activity`, gated by a pure decision function. Specs: `mobile-android-client` (delta), `mobile-onboarding-tutorial`.

## Architecture Decisions

| Topic | Choice | Rejected | Rationale |
|---|---|---|---|
| Theme parent | `Tema.Bythos` extends `@android:style/Theme.Material.NoActionBar` (dark) | `Theme.DeviceDefault.*` | DeviceDefault becomes One UI on the A54 and differs from the desktop look; Material is identical on API 26–36 |
| Theme attrs | `windowBackground`/`colorBackground`=bg; `statusBarColor`/`navigationBarColor`=bg_alt (pre-35 only); `windowLightStatusBar`=false; `textColorPrimary`=ink, `textColorSecondary`=ink_muted, `textColorHint`=ink_dim; `colorAccent`/`colorControlActivated`=ink_accent; `fontFamily` (PR1 `sans-serif`, PR2 `@font/inter`); `buttonStyle`=`@style/Boton.Primario`; `alertDialogTheme`=`@style/Tema.Bythos.Dialogo` | Per-view colors | Spec bans hard-coded per-view styling; theme defaults cover unstyled views (ProgressBar, EditText cursor) |
| Weights | One TextAppearance per role, each pointing to a static font file (`@font/inter_bold`, ...) | `fontWeight` / `Typeface.create(tf, w, it)` | Weight selection needs API 28; minSdk is 26 |
| Buttons | `Boton` base style (`textAllCaps=false`, `stateListAnimator=@null`, `minHeight`/`minWidth`=48dp, 14sp/600) → `Boton.Primario` / `Boton.Secundario` with `<selector>` drawables (pressed, disabled, default) + `res/color` text state lists | `<ripple>` | Explicit pressed/disabled states match the flat desktop look; real `Button`s keep the test valid |
| Insets | `WindowCompat.setDecorFitsSystemWindows(window,false)` on every API, then `ViewCompat.setOnApplyWindowInsetsListener` (core-ktx) using `systemBars() or displayCutout() or ime()` | `enableEdgeToEdge()` | That API needs `ComponentActivity`; the screens extend `android.app.Activity`. One code path for 26–36 |
| Status bar bg on 35+ | Header (`bg_alt`) takes the top inset as padding, so it extends under the status bar | `statusBarColor` | Ignored on API 35+ with enforced edge-to-edge |
| Dialog | `Tema.Bythos.Dialogo` extends `Theme.Material.Dialog.Alert`: `windowBackground=@drawable/fondo_dialogo` (inset 16dp, surface, 1dp border_strong, 16dp radius), `textColorAlertDialogListItem`=ink, `listPreferredItemHeightSmall`=48dp, `buttonBarButtonStyle`=`Boton.Secundario`. The `AlertDialog.Builder(this)` call stays unchanged | Custom Dialog view | Zero code change, so the existing folder list stays as it is |
| Scroll | Content sits in a `ScrollView` inside `pantalla` | Fixed layout | Spec requires scrolling on small screens or at large font scale. The test walks `android.R.id.content` recursively, so it still finds the views |
| Font | Inter static 400/600/700 TTF in `res/font/inter_regular.ttf`, `inter_semibold.ttf`, `inter_bold.ttf`, plus the `font/inter.xml` family; license in `app/src/main/assets/licenses/Inter-OFL.txt` | Downloadable fonts, variable TTF | The license file ships inside the APK and in the tree; downloadable fonts need GMS and network |
| Launcher icon | Adaptive icon in `mipmap-anydpi-v26/ic_launcher.xml`: background `@color/icono_fondo` (#0B0B0C); foreground and monochrome `@drawable/ic_launcher_marca` (an `<inset>` of 27% over the vector `marca.xml`) | 5 density PNGs; converting `icon.svg` | minSdk 26 means anydpi-v26 covers every device. The SVG draws its B with `<text>` (font-dependent) and cannot become a VectorDrawable. `marca.xml` copies the rectangle/sine geometry of `gen-icons.mjs` that produces `icon-512.png` |
| Semantic tone | `Tono { NORMAL, EXITO, ERROR }` sets the text color: ink, green or red | Spans everywhere | Error and done messages are single strings. ShareReceiver colors only the "Listo" lines, using one `ForegroundColorSpan` |

## Interfaces / Contracts

```kotlin
// Estilo.kt — extensions on Context, all values read from resources
fun Context.dp(v: Int): Int
object Estilo {
    fun titulo(c: Context, t: CharSequence): TextView          // TextAppearance.Bythos.Titulo 22sp/700
    fun panel(c: Context, t: CharSequence): TextView           // 16sp/600
    fun cuerpo(c: Context, t: CharSequence = ""): TextView     // 15sp/400 ink
    fun tenue(c: Context, t: CharSequence): TextView           // 13sp ink_muted
    fun botonPrimario(c: Context, t: CharSequence, alPulsar: () -> Unit): Button
    fun botonSecundario(c: Context, t: CharSequence, alPulsar: () -> Unit): Button
    fun campo(c: Context): EditText                            // @drawable/campo, focused = border_strong
    fun tarjeta(c: Context, vararg hijos: View): LinearLayout  // surface, 1dp border, 12dp, 16dp padding
    fun encabezado(c: Context): View                           // bg_alt strip, "BYTHOS" 14sp/700/0.24 ink_accent
    fun tono(v: TextView, t: Tono)
    /** Root: [header] + ScrollView(24dp padding); applies insets; returns the content column. */
    fun pantalla(a: Activity, conEncabezado: Boolean = true): LinearLayout
}
// TutorialActivity.kt — pure, JVM-testable
fun debeAbrirTutorial(desdeLanzador: Boolean, emparejado: Boolean, visto: Boolean, recreada: Boolean): Boolean =
    desdeLanzador && !emparejado && !visto && !recreada
const val CLAVE_TUTORIAL_VISTO = "tutorialVisto"   // in prefs "bythos"
```

Insets pattern (the only non-obvious code):
```kotlin
ViewCompat.setOnApplyWindowInsetsListener(raiz) { _, wi ->
    val b = wi.getInsets(WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.displayCutout() or WindowInsetsCompat.Type.ime())
    cabecera.setPadding(pad + b.left, pad + b.top, pad + b.right, pad / 2); scroll.setPadding(b.left, 0, b.right, b.bottom); WindowInsetsCompat.CONSUMED
}
```

## Data Flow: first launch

    Launcher ──MAIN──> PairingActivity.onCreate
       │  emparejado = clienteEmparejado(this) != null
       │  visto = prefs("bythos").getBoolean(CLAVE_TUTORIAL_VISTO)
       │  desdeLanzador = intent.action == ACTION_MAIN
       │  recreada = savedInstanceState != null
       ├─ debeAbrirTutorial(...) ──true──> prefs.putBoolean(visto,true).apply()
       │                                   startActivity(TutorialActivity)
       └─ "Cómo enviar archivos" button ──> startActivity(TutorialActivity)  (flag untouched)
    Share app ──SEND──> ShareReceiverActivity  (never calls debeAbrirTutorial; flag untouched)

The flag is written before `startActivity`, so the tutorial opens only once even if the user kills the app on it. Tutorial step strings are format strings filled with `getString(R.string.escanear_qr | enviar_archivo | fase_listo | app_name)`. The desktop labels ("Recibir del celular", "Activar recepción", "Aprobar") stay literal because no Android resource holds them.

## File Changes (base `mobile/android/app/`)

**PR1: style (about 330 lines)**

| File | Action | ~Lines |
|---|---|---|
| `src/main/res/values/colors.xml` | Create: 14 tokens | 18 |
| `src/main/res/values/dimens.xml` | Create: padding, radii, 48dp target, text sizes | 16 |
| `src/main/res/values/themes.xml` | Create: app theme, dialog theme, `Boton*`, 5 TextAppearances | 60 |
| `src/main/res/color/texto_boton_primario.xml`, `texto_boton_secundario.xml` | Create: enabled/disabled text | 14 |
| `src/main/res/drawable/boton_primario.xml`, `boton_secundario.xml`, `tarjeta.xml`, `campo.xml`, `fondo_dialogo.xml` | Create: shape selectors | 70 |
| `src/main/AndroidManifest.xml` | Modify: `android:theme="@style/Tema.Bythos"` | 2 |
| `src/main/kotlin/com/bythos/movil/Estilo.kt` | Create | 110 |
| `.../PairingActivity.kt` | Modify: `pantalla`, primary "Escanear QR" or "Enviar", secondary for the rest, `Tono` in `mostrar` | 35 |
| `.../ShareReceiverActivity.kt` | Modify: `pantalla`, `Tono`, green "Listo" span, link items shown as `enlace_compartido` instead of the URL | 20 |
| `.../ScannerFallback.kt` | Modify: `Estilo.campo` and `botonPrimario` | 8 |
| `src/main/res/values/strings.xml` | Modify: `pc_no_recibe` in "tú"; `pegar_pista` becomes "Pega el texto que copiaste de la PC"; add `enlace_compartido` | 5 |

**PR2: tutorial, font and icon (about 300 text lines plus binaries in separate commits)**

| File | Action | ~Lines |
|---|---|---|
| `.../TutorialActivity.kt` | Create: header, title, 5 `tarjeta` steps, primary "Entendido" that calls `finish()`; Back works by default | 65 |
| `src/main/res/values/strings.xml` | Add `como_enviar`, `tutorial_titulo`, `tutorial_paso_1..5`, `entendido` | 10 |
| `.../PairingActivity.kt` | Modify: secondary `como_enviar` button in both modes, auto-open block | 15 |
| `src/main/AndroidManifest.xml` | Modify: `TutorialActivity` (exported=false), `android:icon`/`roundIcon` | 4 |
| `src/test/kotlin/.../TutorialDecisionTest.kt` | Create | 35 |
| `src/androidTest/.../CompartirYFallbackInstrumentedTest.kt` | Modify: `@Before` sets `tutorialVisto=true` | 8 |
| `src/main/res/font/inter.xml` + 3 TTF (binary commit) | Create | 12 |
| `themes.xml` | Modify: `fontFamily` set to Inter in the theme and TextAppearances | 8 |
| `src/main/assets/licenses/Inter-OFL.txt` | Create: verbatim OFL 1.1 | 93 |
| `src/main/res/drawable/marca.xml`, `ic_launcher_marca.xml`, `mipmap-anydpi-v26/ic_launcher.xml`, `ic_launcher_round.xml`, color `icono_fondo` | Create | 45 |

## Testing Strategy

| Layer | What | Approach |
|---|---|---|
| Unit (JVM) | `debeAbrirTutorial`: all 16 boolean combinations; only `(true,false,false,false)` returns true | `TutorialDecisionTest`, `gradlew testDebugUnitTest` |
| Instrumented | Existing paste test, with the tutorial pre-marked as seen (`ActivityScenario` launches with MAIN+LAUNCHER and would otherwise auto-open the tutorial) | `connectedDebugAndroidTest` |
| Manual (A54, API 36, gesture navigation and 3-button bar) | Dark theme with the phone in light mode; header under the status bar; bottom button above the nav bar; keyboard does not cover the paste field; folder dialog dark with 48dp rows; pressed and disabled states; first launch opens the tutorial once; a share launch does not open it; the entry works when paired; no voseo; no IP, port or URL on screen; adaptive icon in circle and squircle masks; Inter visible; 200% font scale scrolls | Checklist in the PR description |

## Threat Matrix

N/A: no routing, shell, subprocess, VCS/PR automation, executable-file classification or process-integration boundary.

## Migration / Rollout

No migration is required. PR1, then PR2, on top of `feat/bythos-movil-feedback`. A leftover `tutorialVisto` flag is harmless.

## Open Questions

- [ ] The spec says the icon comes "in the standard mipmap densities". The design uses only `mipmap-anydpi-v26`, which is enough with minSdk 26. The spec should accept that wording, or PNG foregrounds must be added through `gen-icons.mjs`.
- [ ] The desktop header uses an arcs logo, but the phone header uses only the wordmark, as the spec requires.
