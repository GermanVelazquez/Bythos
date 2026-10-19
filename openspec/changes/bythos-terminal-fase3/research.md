# Research: terminal paste and Excalidraw history

- Change: `bythos-terminal-fase3`
- Lane: terminal paste + Excalidraw history
- Request: `fase3-research-1`, revision `sha256:explore-247`, open-web
- Accessed: 2026-10-19
- Status: **partial**. Q1, Q3 and Q5 are confirmed from source; Q2 and Q4 are largely UNVERIFIED.

## Sources

| Id | Source | Kind |
|---|---|---|
| S1 | xterm.js @5.5.0 `src/browser/Clipboard.ts` | primary |
| S2 | docs.excalidraw.com, excalidraw-api (`CaptureUpdateAction`) | primary |
| S3 | excalidraw v0.18.0 `history.ts`, `components/App.tsx` | primary |
| S4 | excalidraw v0.18.0 `data/transform.ts`, `data/restore.ts`, `clipboard.ts`, `element/newElement.ts` | primary |
| S5 | microsoft/terminal #395 (bracketed paste via ConPTY) | primary |
| S6 | PowerShell/PSReadLine #1471 (bracketed paste, still open) | primary |
| S7 | openai/codex PasteBurst PRs #8774, #9348, #9020 and issues #13729, #5843 | secondhand (search summaries) |
| S8 | Claude Code issue mirrors #73157, #47658, #13183, #24418; Ink `usePaste` | secondhand |
| S9 | opencode #13800 and the OpenTUI docs | secondhand |

## Findings

### 1. xterm.js `paste()` (S1, confirmed)

- **Newlines.** `paste()` replaces `\r?\n` with `\r`.
- **Bracketing.** It wraps the text in `ESC[200~ … ESC[201~` only when the running program has enabled DECSET 2004 and `ignoreBracketedPasteMode` is not set.
- **Delivery.** It emits through `triggerDataEvent`, so `onData` fires and the WebSocket bridge forwards it.
- **When the mode is off:** the text goes out raw, and any `\r` in it acts as Enter.

### 2. Agent CLIs (S7–S9, mostly UNVERIFIED)

Behavior varies by version and terminal.

- **Claude Code:** Ink enables bracketed paste. On Windows, a pasted newline has been read as Enter.
- **Codex on Windows:** it uses a timing heuristic ("PasteBurst") instead of bracketed paste, and fast multiline pastes may submit.
- **OpenCode:** it relies on bracketed paste.
- **Gemini CLI:** unclear. Large pastes reportedly auto-submit.
- **ConPTY:** it passes DECSET 2004 through, per maintainer comments in S5. Not checked against the specific build the app embeds.

### 3. PSReadLine (S6, confirmed)

PSReadLine has no shipped bracketed-paste support. Pasted text with `\r` executes line by line. Text without a newline stays in the buffer.

### 4. Excalidraw `CaptureUpdateAction` (S2 and S3 confirmed; interplay UNVERIFIED)

| Action | Behavior |
|---|---|
| IMMEDIATELY | Goes onto the undo stack at once |
| EVENTUALLY | Folded into the next IMMEDIATELY update |
| NEVER | Remote or initialization updates; never recorded. Updates the store snapshot (`shouldUpdateSnapshot`) without recording |

- **Rebasing.** History entries are rebased with `applyLatestChanges`, which was designed for remote changes.
- **`history.clear()`.** Empties both stacks and fires no change event.
- **UNVERIFIED.** Whether Ctrl+Z after a user edits an element that was applied with NEVER yields a correct state.

### 5. `customData` (S4, inferred from code)

- **Kept** by `newElement`, `newTextElement`, `restore`, clipboard JSON, and `convertToExcalidrawElements` for shapes.
- **Bound labels** are separate elements and do not inherit the container's `customData`.
- **UNVERIFIED:**
  - whether a skeleton `label.customData` is read;
  - `.excalidraw` export (`clearElementsForExport`);
  - `exportToBlob`.

## Implications for design

1. Revisar sends a **single-line** fixed template through `term.paste`, with no `\n`, no `\r` and no Enter. That keeps it from submitting even when bracketed paste is off (PowerShell, Codex) or ignored.
2. Strip C0 control characters and newlines from any dynamic part. Never echo scene text.
3. Agent ops keep `CaptureUpdateAction.IMMEDIATELY` (the phase-1 behavior) until the NEVER interplay is checked at runtime. The authoritative undo is the persisted inverse-op history, not Excalidraw's stack. Do not use `history.clear()`.
4. Tag marks as `customData.bythos = "marca"`, plus the mark fields, on **both** the frame and the pill text. The pill is a standalone text element, not a bound label, so there is no inheritance to rely on.
5. Identify marks by `customData`, not by id across copy and paste, because pasted elements get new ids.

## Runtime checks still needed

1. In the embedded terminal, run Revisar against Claude Code, Codex, OpenCode, Gemini CLI and pwsh. Confirm the line lands unsubmitted.
2. Confirm that `ESC[?2004h` reaches xterm.js through the app's ConPTY.
3. In Excalidraw 0.18.1: apply an update with NEVER, edit the same element as the user, then undo and redo. Compare the states. This decides whether to switch agent ops to NEVER.
4. Round-trip `customData` through `updateScene`, copy/paste, export and import, and `convertToExcalidrawElements`.
