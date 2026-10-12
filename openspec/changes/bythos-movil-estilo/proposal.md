# Proposal: Android Visual Parity and Sending Tutorial

## Intent

The Android app uses a stock light Material theme with hard-coded px views, while the desktop app is dark, monochrome and Inter-based. Users also get no in-phone guidance on how to send files. Align the phone with the desktop look and add a short tutorial.

## Scope

### In Scope
- Dark-only theme mirroring the desktop token table (colors, touch-scaled type, 8dp buttons, 12dp cards, 16dp dialogs, 24dp padding, 48dp targets)
- Apply the theme to Pairing, ShareReceiver, folder-picker dialog and manual paste screens, plus a brand header and system-bar insets
- `TutorialActivity` with 5 text steps; auto-open once on first unpaired launch; permanent "Cómo enviar archivos" home entry, also after pairing; never on share-intent launches
- Neutral "tú" voice (fix `pc_no_recibe`); no IP, port or link text anywhere
- Bundled Inter 400/600/700 with OFL license; launcher icon from `assets/icon-512.png`

### Out of Scope
- Light theme, Material Components, Compose
- Desktop tutorial panel, docs, tutorial images

## Capabilities

### New Capabilities
- `mobile-onboarding-tutorial`: tutorial content, first-launch trigger, reopen entry, share-intent exclusion, privacy of shown text

### Modified Capabilities
- `mobile-android-client`: ADDED requirements for visual theme/brand consistency, voice and on-screen privacy. Delta targets `openspec/changes/bythos-movil-qr/specs/mobile-android-client/spec.md` (the full baseline, same convention as `bythos-movil-feedback`); it is ADDED-only, so it does not overlap the feedback delta's MODIFIED requirement.

## Approach

Plain XML `themes.xml`/`colors.xml`/`dimens.xml`/styles plus shape drawables, consumed by the existing code-built views through a small `Estilo.kt` helper. Real `Button`s stay so `CompartirYFallbackInstrumentedTest` keeps working. Tutorial strings reuse existing UI labels; a SharedPreferences flag gates first launch.

Delivery: two PRs, 400-line budget, based on `feat/bythos-movil-feedback` (1423a0e).
- PR1: U1 foundation + U2 screen application (~280 lines)
- PR2: U3 tutorial + U4 font and icon in separate binary commits (~140–200 lines)

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `mobile/android/app/src/main/res/values/` | New/Modified | themes, colors, dimens, strings |
| `mobile/android/app/src/main/res/drawable/`, `font/`, `mipmap-*/` | New | shapes, Inter, launcher icon |
| `mobile/android/app/src/main/AndroidManifest.xml` | Modified | theme, icon, TutorialActivity |
| `.../PairingActivity.kt`, `ShareReceiverActivity.kt`, `ScannerFallback.kt` | Modified | styled views, home entry |
| `.../Estilo.kt`, `TutorialActivity.kt` | New | style helper, tutorial |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Edge-to-edge (target 36) overlaps content | Med | Explicit inset handling |
| AlertDialog theming varies by API | Med | Dedicated dialog theme; manual check |
| Conflicts with unmerged feedback branch | Med | Base off it; rebase if it changes |
| Instrumented test breaks | Low | Keep real `Button`s and texts |
| Tutorial text drifts from UI | Low | Reuse string resources |
| Missing OFL attribution | Low | Ship license file |

## Rollback Plan

Revert PR2 then PR1 (or either alone); no data, protocol or storage migrations. The first-launch flag is harmless if left behind.

## Dependencies

- `feat/bythos-movil-feedback` merged or used as base

## Success Criteria

- [ ] All screens and dialogs render dark with desktop tokens and Inter
- [ ] Tutorial auto-opens once when unpaired, never on share launches, reopenable from home
- [ ] No IP, port or link text on any screen; no voseo
- [ ] Instrumented test passes; each PR stays under 400 lines
