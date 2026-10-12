# Spec: mobile-onboarding-tutorial (bythos-movil-estilo)

## Purpose
A short in-app tutorial on the Android client that explains how to send files to the PC. It opens once automatically for a new, unpaired user and is always reachable again from the home screen.

## ADDED Requirements

### Requirement: Tutorial Content
The app MUST provide a tutorial screen (`TutorialActivity`) with exactly five numbered text steps, in this order:
1. On the PC, open Bythos > "Recibir del celular" and press "Activar recepción" (same Wi-Fi network, PC network set to Private).
2. On the phone, tap "Escanear QR".
3. Check that the 6-digit code matches the one on the PC, then press "Aprobar" on the PC.
4. Tap "Enviar un archivo a la PC", or in any app use Compartir > Bythos.
5. Choose the folder and wait for "Listo".

Quoted labels MUST be taken from the same string resources as the real UI controls they name, so the tutorial cannot drift from the UI. The tutorial MUST be text only (no images) and MUST use the neutral "tú" voice. The screen MUST provide a clear way to close it ("Entendido" or equivalent) and MUST also close with the system Back action.

#### Scenario: Five steps shown
- GIVEN the tutorial screen is open
- WHEN the user reads it
- THEN exactly five numbered steps are visible in the order above (scrolling allowed on small screens)

#### Scenario: Labels match the UI
- GIVEN a label used in the tutorial, such as "Escanear QR" or "Enviar un archivo a la PC"
- WHEN the corresponding UI string is changed
- THEN the tutorial shows the changed text, because both read the same string resource

#### Scenario: Closing
- GIVEN the tutorial is open
- WHEN the user taps the close action or presses Back
- THEN the tutorial closes and the previous screen is shown

### Requirement: First-Launch Auto-Open
On a launch from the launcher with no valid pairing, if the tutorial has never been shown, the app MUST open the tutorial automatically exactly once. The app MUST record that the tutorial was shown in a persistent flag (SharedPreferences) at the time it is opened, so that it never auto-opens again, including after the app is killed and restarted, and including if the user leaves it without finishing.

#### Scenario: First unpaired launch
- GIVEN a fresh install with no pairing and the tutorial flag unset
- WHEN the user opens the app from the launcher
- THEN the tutorial opens automatically and the flag is set

#### Scenario: Second launch
- GIVEN the tutorial was auto-opened before and the phone is still unpaired
- WHEN the user opens the app again
- THEN the tutorial does NOT open automatically

#### Scenario: Paired device never auto-opens
- GIVEN a valid pairing exists and the flag is unset (for example an upgrade from an earlier version)
- WHEN the user opens the app from the launcher
- THEN the tutorial does NOT open automatically

### Requirement: Permanent Home Entry
The home screen MUST show a control labelled "Cómo enviar archivos" that opens the tutorial, both in the unpaired state and in the paired state. The control MUST be a secondary-style action that does not replace or demote the primary action of the state, and MUST meet the 48dp minimum touch target. Opening the tutorial from this control MUST NOT depend on, nor change, the first-launch flag.

#### Scenario: Entry when unpaired
- GIVEN no pairing exists
- WHEN the home screen is shown
- THEN "Cómo enviar archivos" is visible and opens the tutorial when tapped

#### Scenario: Entry when paired
- GIVEN a valid pairing exists
- WHEN the paired home screen is shown
- THEN "Cómo enviar archivos" is visible, "Enviar un archivo a la PC" remains the primary action, and tapping the entry opens the tutorial

#### Scenario: Reopen after auto-open
- GIVEN the tutorial already auto-opened once
- WHEN the user taps "Cómo enviar archivos"
- THEN the tutorial opens again

### Requirement: Share-Intent Exclusion
The tutorial MUST NOT open automatically when the app is launched through a share intent (`ACTION_SEND` / `ACTION_SEND_MULTIPLE`), regardless of pairing state or flag value. A share-intent launch MUST NOT set the first-launch flag.

#### Scenario: Share launch while unpaired and flag unset
- GIVEN no pairing exists and the tutorial flag is unset
- WHEN the user shares a file to Bythos from another app
- THEN the share flow proceeds without opening the tutorial and the flag stays unset

#### Scenario: Later launcher launch
- GIVEN a share-intent launch happened first and the flag is still unset and the phone is unpaired
- WHEN the user later opens the app from the launcher
- THEN the tutorial opens automatically once

### Requirement: Tutorial Text Privacy
The tutorial MUST NOT display any IP address, port number, URL or link text. Steps MUST refer to the PC only by user-facing names and UI labels.

#### Scenario: No network identifiers
- GIVEN the tutorial is open
- WHEN every visible string is inspected
- THEN none contains an IP address, a port, a URL or a link

### Requirement: Tutorial Visual Consistency
The tutorial screen MUST follow the "Dark Desktop Theme" and "Touch Sizing and Insets" requirements of `mobile-android-client` (dark background, Inter font, system-bar insets handled, 48dp targets).

#### Scenario: Tutorial look
- GIVEN the tutorial is open on a device with gesture navigation
- WHEN it renders
- THEN it uses the dark token colors and the Inter font, and no text sits under a system bar
