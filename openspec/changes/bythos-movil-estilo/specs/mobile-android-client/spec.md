# Delta Spec: mobile-android-client (bythos-movil-estilo)

Delta against `openspec/changes/bythos-movil-qr/specs/mobile-android-client/spec.md` (not yet archived). ADDED only; no existing requirement is modified. Sequenced after `bythos-movil-feedback`: where the feedback delta quotes a Spanish UI string in voseo (Stale QR Detection), the "Neutral Voice" requirement below governs the wording and the feedback behavior (message, timing) is otherwise unchanged.

## ADDED Requirements

### Requirement: Dark Desktop Theme
All app screens (pairing/home, share receiver, manual paste, tutorial) and the folder dialog MUST use a dark-only theme that mirrors the desktop tokens:

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
| ink-dim | #5A5A5A | hints only |
| ink-accent | #C8C8C8 | links, brand |
| green | #5FA97B | success |
| amber | #C9964A | warning |
| red | #E07A7A | error |

Color beyond monochrome MUST be reserved for semantics (green success, amber warning, red error). `ink-dim` MUST NOT be used for essential text. The app MUST NOT offer a light theme. Colors, dimensions and styles MUST be defined as XML resources, not hard-coded per view. No dependency on AppCompat, Material Components or Compose MAY be added.

#### Scenario: Dark on a light-mode device
- GIVEN the device system theme is light
- WHEN any app screen opens
- THEN it renders with the dark token colors

#### Scenario: Semantic color only
- GIVEN an upload completes successfully and another fails
- WHEN the result is shown
- THEN success uses green and the error uses red, while other text uses ink or ink-muted

#### Scenario: Essential text contrast
- GIVEN any instructional or status text
- WHEN it is rendered
- THEN its color is ink or ink-muted, never ink-dim

### Requirement: Typography and Inter Font
Text MUST use the bundled Inter font in static weights 400, 600 and 700, packaged with its SIL OFL 1.1 license text. If the font cannot be loaded, text MUST fall back to `sans-serif` without crashing. Sizes MUST be: page title 22sp/700, panel title 16sp/600, body 14-15sp/400, micro-label 11-12sp/600 uppercase, button 14sp/600.

#### Scenario: Inter shown
- GIVEN the font resources are present
- WHEN a screen renders
- THEN titles use Inter 700, panel titles and buttons Inter 600, and body Inter 400

#### Scenario: Font fallback
- GIVEN the Inter resource fails to load
- WHEN a screen renders
- THEN text is shown in `sans-serif` and the app does not crash

#### Scenario: License shipped
- GIVEN a built APK or the source tree
- WHEN the repository is inspected
- THEN the Inter OFL license text is included alongside the font files

### Requirement: Button Styles
Actions MUST be real `Button` views (so the instrumented test finds them by class and text) with 8dp corner radius. A primary button MUST have an `ink` fill with dark text (`bg`). A secondary button MUST have a `surface-2` fill with a 1dp `border-strong` outline and `ink` text. Pressed state MUST be visibly distinct. Each screen MUST present at most one primary action.

#### Scenario: Primary button
- GIVEN the paired home screen
- WHEN it renders
- THEN "Enviar un archivo a la PC" is a `Button` with ink fill, dark text and 8dp radius

#### Scenario: Secondary button
- GIVEN the paired home screen
- WHEN it renders
- THEN "Volver a emparejar" and "Cómo enviar archivos" are `Button`s with surface-2 fill, a 1dp outline and 8dp radius

#### Scenario: Instrumented test compatibility
- GIVEN `CompartirYFallbackInstrumentedTest`
- WHEN it looks up controls by `Button` class and visible text
- THEN it still finds them after the restyle

### Requirement: Brand Header
The pairing/home screen MUST show a brand header on a `bg-alt` background with the "BYTHOS" wordmark (14sp, 700, letter spacing about 0.24em, color `ink-accent`).

#### Scenario: Header on home
- GIVEN the home screen opens in either pairing state
- WHEN it renders
- THEN the "BYTHOS" wordmark is visible in the header in ink-accent

### Requirement: Touch Sizing and Insets
Layouts MUST use touch-scaled dimensions: 24dp screen padding, 12dp cards, 16dp dialogs, 1dp borders. Every interactive control MUST have a touch target of at least 48dp in height and width. Because the app targets API 36 with enforced edge-to-edge, each screen MUST apply system-bar insets (status bar, navigation bar, display cutout) so that no content or control is hidden under or unreachable behind a system bar, and system bar backgrounds SHOULD match `bg-alt`.

#### Scenario: Minimum touch target
- GIVEN any button, link or list row
- WHEN its bounds are measured
- THEN height and width are at least 48dp

#### Scenario: Gesture navigation
- GIVEN a device with gesture navigation or a three-button bar
- WHEN the bottom-most control of a screen is shown
- THEN it is fully visible and tappable above the navigation bar

#### Scenario: Status bar and cutout
- GIVEN a device with a status bar and a display cutout
- WHEN a screen opens
- THEN the header and first line of text are not overlapped by them

#### Scenario: Small screen
- GIVEN a small display or a large font scale
- WHEN content exceeds the visible height
- THEN the screen scrolls so every control stays reachable

### Requirement: Themed Folder Dialog
The folder picker dialog in the share receiver MUST use the dark theme: `surface` background, `border-strong` outline, 16dp corner radius, Inter font, `ink` text, and dialog buttons styled as secondary (and primary where it confirms). Its list rows MUST meet the 48dp target.

#### Scenario: Dialog look
- GIVEN the user must choose a destination folder
- WHEN the dialog opens
- THEN it shows the dark surface, rounded corners, Inter text and styled buttons, not the default light system dialog

#### Scenario: Dialog on different API levels
- GIVEN devices on the minimum supported API and on API 36
- WHEN the dialog opens
- THEN it is dark and readable on both

### Requirement: Neutral Voice
All user-visible strings MUST use neutral "tú" voice and MUST NOT use voseo (for example "Abrí", "escaneá", "tocá"). This includes the "PC is not receiving" message (`pc_no_recibe`), whose wording is: "La PC no está recibiendo. Abre «Recibir del celular» en la PC y escanea el QR de nuevo." This wording supersedes the voseo wording in the feedback delta; the behavior (shown on pairing timeout or refusal, distinct from the certificate-mismatch message) is unchanged.

#### Scenario: Stale QR message
- GIVEN pairing fails because the PC is not receiving
- WHEN the message is shown
- THEN it reads "La PC no está recibiendo. Abre «Recibir del celular» en la PC y escanea el QR de nuevo."

#### Scenario: No voseo anywhere
- GIVEN the full set of string resources
- WHEN they are reviewed
- THEN no string uses voseo imperatives or "vos" conjugations

### Requirement: On-Screen Privacy of Network Identifiers
No screen, dialog, notification or error message MAY display an IP address, port number, URL or link text, including the QR payload and the manual-paste screen contents once entered. Error messages MUST describe the problem in words without echoing host or port values.

#### Scenario: Pairing error
- GIVEN pairing fails by timeout, refusal or certificate mismatch
- WHEN the message is shown
- THEN it contains no IP, port or URL

#### Scenario: Manual paste
- GIVEN the user pastes a pairing link in the manual screen
- WHEN the text is accepted or rejected
- THEN no host, port or link text is echoed back in the UI

#### Scenario: Legacy pairing label
- GIVEN a device paired without a stored name
- WHEN the paired home screen shows
- THEN it shows "Emparejado con tu PC" and no address

### Requirement: Launcher Icon
The app MUST declare a launcher icon matching `assets/icon-512.png` (as drawn by `gen-icons.mjs`), provided as an adaptive icon in `mipmap-anydpi-v26` with a vector foreground and a dark background, which covers every supported device at minSdk 26, referenced from the manifest.

#### Scenario: Icon shown
- GIVEN the app is installed
- WHEN the launcher lists apps
- THEN the Bythos icon is shown instead of the default Android icon

#### Scenario: Adaptive masking
- GIVEN a launcher that applies circular or squircle masks
- WHEN the icon renders
- THEN the arcs logo is not clipped
