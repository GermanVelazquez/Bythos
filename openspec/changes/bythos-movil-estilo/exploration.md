# Exploration: bythos-movil-estilo

Request (2026-10-12): make the Android app look like the desktop app (colors, typography, button style) and add a tutorial on how to transfer files. Engram: `sdd/bythos-movil-estilo/explore` (id 185).

## Current state

- **Android.**
  - Views are built in code (PairingActivity, ShareReceiverActivity with a framework AlertDialog folder picker, PegadoManual in ScannerFallback.kt) with hard-coded px padding.
  - The manifest theme is `@android:style/Theme.Material.Light.NoActionBar`.
  - `res/` holds only `values/strings.xml`: no themes, colors, fonts, drawables or launcher icon.
  - Dependencies: core-ktx, activity-ktx, work-runtime-ktx, play-services-code-scanner. No AppCompat, Material or Compose; the Activities extend `android.app.Activity`.
  - `CompartirYFallbackInstrumentedTest` finds views by `Button` class and text.
  - Target 36 enforces edge-to-edge.
  - Voice is mixed: `pc_no_recibe` uses voseo, the rest uses tú.
- **Desktop (`desktop/ui/src/styles.css`).**
  - Dark only (`color-scheme: dark`).
  - Font: Inter 300–900 from Google Fonts, falling back to system-ui.
  - Monochrome look, with color reserved for semantics.
  - Primary button: ink fill with dark text. Secondary: surface-2 with a 1px border. Ghost: transparent.
  - Brand: arcs logo (#C8C8C8) plus the "BYTHOS" wordmark (12px, 700, .24em tracking). Icon at `assets/icon.svg` (with PNG and .ico variants).
- **Desktop guidance (`ReceptorMovil.jsx`).**
  - Explains QR scanning, the 6-digit code, a 60 s "nobody connected" hint and the network guides.
  - Does not explain how to send a file from the phone.
  - The QR text (which contains an IP) is never shown.

## Token table

| Token | Hex | Use |
|---|---|---|
| bg | #0A0A0A | window background |
| bg-alt | #0F0F0F | system bars, header |
| surface | #141414 | cards, fields |
| surface-2 | #1A1A1A | secondary button, pressed |
| surface-3 | #202020 | progress track, chips |
| border | #1F1F1F | 1dp outlines |
| border-strong | #2A2A2A | focus, dialogs |
| ink | #EDEDED | text, primary button fill |
| ink-muted | #8A8A8A | secondary text |
| ink-dim | #5A5A5A | hints only (~2.7:1) |
| ink-accent | #C8C8C8 | links, brand |
| green | #5FA97B | success |
| amber | #C9964A | warning |
| red | #E07A7A | error |

Error banner (optional): background #1A0A0A, border #4A1A1A.

**Typography (desktop → Android).**

| Role | Desktop | Android |
|---|---|---|
| Page title | 18px / 700 | 22sp / 700 |
| Panel title | 13px / 600 | 16sp / 600 |
| Body | 12px | 14–15sp |
| Micro-label | 10px / 600, uppercase, .14em | 11–12sp |
| Button | 11–12px / 600 | 14sp / 600 |
| Wordmark | 12px / 700 / .24em | 14sp / 700 / .24em |

**Shape and spacing.**
- Desktop radii: 6 controls, 8 cards, 12 modals, 99 pills.
- Android: 8dp buttons, 12dp cards, 16dp dialogs.
- 1dp borders, 24dp screen padding, touch targets of at least 48dp.

**Font.** Inter is under SIL OFL 1.1, so it can be bundled with the license text. `res/font` works on API 26+. Static 400/600/700 weights add about 0.3–0.8 MB. Fallback: `sans-serif`.

## Approaches: style

| Option | Tradeoff | Est. lines |
|---|---|---|
| **1. XML theme + styles + colors on the existing code-built views** | No dependencies, keeps real `Button`s (test stays valid), matches the flat desktop look. Needs hand-made drawables, dialog theming and manual insets. | ~250 |
| 2. Material Components | Ready-made components, but +1.5–2.5 MB APK, defaults differ from the flat look, and setup is awkward with plain `Activity`. | 250–300 + build risk |
| 3. Jetpack Compose | Rewrites every screen, adds the compiler plugin and BOM, breaks the Button-based test. | 500+ |

## Approaches: tutorial

| Option | Tradeoff | Est. lines |
|---|---|---|
| **T1. Phone `TutorialActivity` with numbered text steps** | Opens once automatically on the first unpaired launch; a "Cómo enviar archivos" button on the home screen reopens it. | 130–160 |
| T2. Inline steps on the home card | Crowds the screen; no first-launch concept. | 40–60 |
| T3. Guide panel in the desktop "Recibir del celular" view | Useful later, but no help once the user is on the phone. | 30–50 |
| T4. Doc only | Cheap; users won't read it. | — |

Suggested steps (no IP, port or link text on screen):

1. On the PC, open Bythos > "Recibir del celular" and press "Activar recepción" (same Wi-Fi, PC network set to Private).
2. On the phone, tap "Escanear QR".
3. Check that the 6-digit code matches, then press "Aprobar" on the PC.
4. Tap "Enviar un archivo a la PC", or in any app use Compartir > Bythos.
5. Choose the folder and wait for "Listo".

## Work units

| Unit | Content | Est. lines |
|---|---|---|
| U1 | Foundation: XML resources, drawables, manifest theme, `Estilo.kt` | ~170 |
| U2 | Apply to screens, dialog theme, brand header | ~110 |
| U3 | Tutorial: activity, strings, manifest, first-launch flag, home button | ~140 |
| U4 (optional) | Font, launcher icon, desktop panel | ~30–60 + binaries |

The total of about 420–480 lines is over the 400 budget, so this needs two PRs:

- **PR1, style:** U1 + U2, ~280 lines.
- **PR2, tutorial:** U3, plus U4 as separate commits.

Binary files (fonts, icons) go in their own commits.

## Product decisions (recommended default)

1. Theme: dark only.
2. Sizes: scaled for touch, not the literal desktop px.
3. Button radius: 8dp.
4. Font: bundle Inter static 400/600/700 with the OFL license.
5. Launcher icon: yes, in a separate commit.
6. Tutorial location: phone (T1); desktop panel later; no doc.
7. Tutorial format: text steps, no images.
8. When to show the tutorial: once automatically on the first unpaired launch, plus a permanent home entry; never on share-intent launches.
9. Voice: neutral "tú"; fix the voseo in `pc_no_recibe`.
10. Tutorial reopenable after pairing: yes.
11. Privacy: no IP, port or link text anywhere.

## Risks

- Edge-to-edge at target 36 needs explicit system-bar handling.
- `ink-dim` has low contrast and must not carry essential text.
- Controls that are not real `Button`s would break the instrumented test.
- AlertDialog theming varies by API level.
- The unmerged `feat/bythos-movil-feedback` branch also edits `PairingActivity.kt` and `strings.xml`, so this change must be sequenced after it.
- Inter requires the OFL attribution.
- Tutorial text can drift from the real UI labels; reuse string resources.
- Verification is manual only.

### Confirmed by the user (2026-10-12)

All 11 recommended defaults above are accepted as listed. Research: not selected.
