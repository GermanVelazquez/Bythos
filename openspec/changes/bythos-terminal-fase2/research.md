# Research: Excalidraw 0.18 image and file handling

Lane: Excalidraw image/file flows for `bythos-terminal-fase2`.
Accessed 2026-10-18. Sources were read at tag v0.18.0; the app ships 0.18.1.
Excerpts come from fetcher summaries, so line numbers are approximate.

## Sources

- **S1:** `packages/excalidraw/constants.ts` @ v0.18.0 (raw.githubusercontent.com/excalidraw/excalidraw/v0.18.0/…)
- **S2:** `packages/excalidraw/components/App.tsx` @ v0.18.0
- **S3:** `packages/excalidraw/types.ts` @ v0.18.0
- **S4:** the following files @ v0.18.0:
  - `element/types.ts`
  - `element/image.ts`
  - `renderer/renderElement.ts`
  - `renderer/staticScene.ts`
  - `scene/export.ts`
  - `data/blob.ts`
- **S5:** docs.excalidraw.com pages:
  - api/props
  - api/props/excalidraw-api
  - api/utils/export
  - excalidraw-element-skeleton
- **S6:** GitHub issues:
  - #7240
  - #6146 (fix PR #8471)
  - #10861 (fix PR #10870)
  - #7886 (snippet only)
  - discussion #6255 (snippet only)

## Findings

### Paste and drop

- **Limits:**
  - `DEFAULT_MAX_IMAGE_WIDTH_OR_HEIGHT = 1440`.
  - `MAX_ALLOWED_FILE_BYTES = 4 MiB`, applied after the resize on the editor's own insert path.
  - Above the limit the editor shows an error dialog and does not insert. [S1, S2]
- **Accepted image MIME types:** svg+xml, png, jpeg, gif, webp, bmp, x-icon, avif, jfif. [S1]
- **Paste:**
  - `onPaste(data, event)` returning strict `false` blocks the native paste. This is documented. [S2, S3, S5]
  - Whether `onPaste` fires for image-file pastes is likely but UNVERIFIED.
- **Drop:**
  - There is no `onDrop` prop.
  - Image drops go to `createImageElement` and `insertImageElement`.
  - Non-image drops (PDF, docx) go to the scene/library loader and show an "invalid file" error dialog. [S2, S3]
  - A capture-phase DOM interception is UNVERIFIED.
- **`UIOptions.tools.image = false`:**
  - It does not stop the paste/drop handlers. The insert ends in the `imageToolNotSupported` error dialog. [S2, S3]
  - Clean suppression requires intercepting first.

### Programmatic insertion

- **`BinaryFileData` fields:**
  - `{ id, mimeType, dataURL, created }`;
  - optional `lastRetrieved` and `version`.
  - [S3]
- **`addFiles(BinaryFileData[])`:**
  - It skips ids already present, so replacing bytes requires a new id. [S2, S3]
  - Files the host inserts this way bypass the editor's 1440 px and 4 MiB limits.
- **Image element:**
  - Fields: `type: "image"`, `fileId`, `status: "pending" | "saved" | "error"`, `scale`, `crop`. [S4]
  - The host must create it with `status: "saved"`.
  - `convertToExcalidrawElements` support for images in 0.18.1 is UNVERIFIED, so elements are built manually. [S6 #7240]
- **Missing file:**
  - A gray `#E7E7E7` placeholder tile is drawn; there is no broken-image icon.
  - A fileId with no entry in `files` is skipped silently.
  - An undecodable file or an `application/octet-stream` file gets `status: "error"`. [S4]
- **Rehydration race:**
  - A late `initialData` left images as placeholders (#6146, fixed by #8471).
  - The mitigation is to have the files ready before mounting, or to call `addFiles` and then `updateScene`. [S6]

### onChange and garbage collection

- `onChange(elements, appState, files)` reports the full files map. [S2]
- The editor does not garbage-collect files when elements are deleted, and undo relies on that.
- The host must filter files by live `fileId`s. [S2, S5]
- Issue #10861: serializing before rehydration drops image elements whose file is missing.
  - Persist elements only, and keep `files: {}` server-side.

### Document cards

- There is no document primitive, and `application/octet-stream` cannot enter the image cache. [S2, S4]
- **Best native option:**
  - Use an `embeddable` element whose `link` is a same-origin viewer URL.
  - `validateEmbeddable` returns true for it.
  - `renderEmbeddable(element, appState)` renders a custom card. [S3, S5]
- UNVERIFIED for this option:
  - custom URL schemes;
  - export rendering (staticScene draws a placeholder label for iframe-like elements).
- **Alternatives:**
  - a rectangle+text group with `customData`;
  - a frame;
  - a thumbnail image.

### Export

- `exportToBlob` / `exportToCanvas` take `files`.
- Images whose ids are absent are skipped silently, so `getFiles()` must be passed. This is already done in `usePizarraAgente`. [S4, S5]

## Implications for design

1. Intercept paste with `onPaste` (return `false`). Intercept drop with a capture-phase listener, to be confirmed by the spike. Upload to the server, then insert programmatically with `addFiles`, `updateScene` and `status: "saved"`.
2. The host enforces its own limits and resize, because `addFiles` bypasses the editor's.
3. On load, have the files ready before mounting, or `addFiles` and then `updateScene`. Persist only elements, never `files`.
4. Use the server-side content hash as the `fileId`.
5. Document card: embeddable + `renderEmbeddable` is preferred, pending a spike on link validation and export. The fallback is a rectangle+text group with `customData`.

## Spike checklist (before or during design, against 0.18.1)

- `onPaste` fires for image pastes.
- Capture-phase drop interception works.
- `addFiles` + `updateScene` rehydration order.
- Embeddable with a same-origin link + `renderEmbeddable`, and how it exports.
- `exportToBlob` with and without files.
