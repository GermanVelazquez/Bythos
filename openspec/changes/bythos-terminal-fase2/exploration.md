# Exploration: bythos-terminal-fase2

Scope: paste/drop/attach of images and files on the whiteboard, `leer_imagen`, `leer_archivo`.
Canonical Engram copy: `sdd/bythos-terminal-fase2/explore`.

## Source of truth: "Bythos — Terminal decisiones.pdf"

Phase 2 ("Pegar y leer"), verbatim:

> Pegar con Ctrl+V, arrastrar y adjuntar imágenes y archivos; traer recursos de la carpeta a la pizarra.
> MCP: `leer_imagen` y `leer_archivo`, reusando la extracción de texto que ya existe.

Whiteboard section:

> Qué se puede pegar: imágenes (capturas, fotos del cuaderno, también desde el celular), PDF, Word, Excel, PowerPoint y texto. Entran con Ctrl+V, arrastrando, con Adjuntar o eligiendo un recurso de la carpeta. Las imágenes se pueden dibujar o resaltar encima; los documentos se ven como tarjeta y se abren en el visor.
>
> El agente lee imágenes y documentos pegados y los recursos de la carpeta.

Open decision listed by the PDF itself:

> Los archivos pegados en una sesión, ¿pasan solos a la carpeta o solo al tocar «Guardar en carpeta»?

Derived facts:
- Target is the board (not the terminal panel). Entry points: Ctrl+V, drag and drop, an Adjuntar button, and picking a resource of the session's folder.
- Images become image elements users can draw on. Documents become a card on the board that opens in the existing viewer.
- Allowed types: images, PDF, Word, Excel, PowerPoint, text. Video is not listed.
- The agent only reads in phase 2. Agent-generated SVG/graphics, the highlighter, and exports are phase 4. Revisar typing and `pizarra_marcar` are phase 3.

## Current state (verified in code)

- Images are blocked in four layers:
  - `Pizarra.jsx`: `tools.image=false`, capture-phase paste/drop block, `onPaste` guard.
  - `excalidrawAdapter.sanearEscena`: strips images and sends `files: {}`.
  - `api/sesiones.go` `reducirEscena`: returns 400 on any image element.
  - `pizarra/ops.go:133`: rejects `image` in `pizarra_dibujar`.
- The whiteboard spec's "Image blocked" scenario and the string "Las imágenes llegan en una fase posterior." must change.
- Scene persistence:
  - The scene is JSON in `terminal_pizarras.escena`, re-sent whole on every autosave (debounce 1.5 s, 20 MiB cap, optimistic version).
  - Inline dataURLs would bloat every autosave.
- Capture: `usePizarraAgente` already passes `getFiles()` to the export, so images appear in `pizarra_captura` for free.
- The `archivos` store already provides:
  - SHA256 content addressing with dedupe.
  - A magic-byte whitelist (pdf, png/jpg/gif/webp, video, docx/xlsx/pptx without macros, txt/md; SVG and HTML rejected).
  - `ExtraerTexto` for docx/pptx/xlsx/txt/md. There is no PDF extractor.
  - Safe serving headers.
- `POST /api/archivos` is not reusable for session attachments:
  - It sits behind `conGuardia`, which admits the extension Origin and no Origin.
  - It requires `carpeta_id` and creates a library resource.
- MCP: `pizarra_captura` is a raw `Server.AddTool` with no StructuredContent, because Claude Code drops images otherwise. `leer_imagen` must follow the same pattern.

## Approaches (where bytes live)

| # | Approach | Verdict |
|---|---|---|
| A | dataURL inline in the scene JSON | Rejected: autosave bloat, 20 MiB cap |
| B | Blob table in SQLite | Blocks the single DB connection; reinvents the store |
| C | `archivos` store + `terminal_adjuntos(sesion_id, archivo_id, file_id, nombre, creado)` with FK cascade; scene keeps refs, `files` stays `{}`, UI rehydrates via `addFiles()` | **Recommended** |
| D | Per-session folder in the agent workspace | Loses whitelist/dedupe unless reusing `Guardar` |

## MCP tools

- **`leer_imagen`:**
  - Raw AddTool returning ImageContent (downscaled to ≤1280 px) plus text (id, name, mime, size, path).
  - Agent route behind `conAgente`.
- **`leer_archivo`:**
  - Typed text tool reusing `ExtraerTexto`, with a character cap and a `truncado` flag.
  - PDF has no extractor (decision needed).
- **`pizarra_leer`:** lists image and document elements so the agent can discover them.
- Both new tools work with the UI closed.

## Security

- Upload goes behind `conVentana` and reads behind `conVentanaLectura`, with a dedicated `MaxBytesReader` cap and 413 via `esBodyDemasiadoGrande`.
- `reducirEscena` allows image elements only when their `fileId` is an attachment of that session.
- The UI pre-filters types that Excalidraw accepts but the server rejects (SVG, BMP, AVIF).
- Attached content is untrusted data (prompt injection); the tool text must say so.
- Blobs are shared by sha256 with library resources, so a refcount/GC policy is needed before deleting.

## Size and slices (~1,300–1,700 lines, 400 per PR)

| PR | Scope | Est. |
|---|---|---|
| P1 | db `terminal_adjuntos` + upload/list/get/delete API + tests | ~400 |
| P2 | Scene validation for image/card elements + spec delta | ~250 |
| P3 | UI unblock: paste/drop/Adjuntar → upload → `addFiles`, rehydrate on load | ~400 |
| P4 | `leer_imagen`, `leer_archivo`, agent routes, `pizarra_leer` listing | ~400 |
| P5 | Document card + viewer, "pick folder resource", workspace rule text, manual checklist | ~300 |

## Open product questions

1. Pasted files: do they go to the session's folder automatically, or only via "Guardar en carpeta"? (open in the PDF)
2. `leer_archivo` for PDF: metadata + path only (the agent opens it itself), or add a PDF text extractor (new dependency)?
3. Limits: per-file cap and behavior on exceed.
4. Lifecycle: delete attachments with the session? Can the user remove one? Orphan blob GC.
5. "También desde el celular": is phone → board in phase 2, or does the existing phone → library flow plus "pick folder resource" cover it?
6. Does the agent see attachments only on request (consistent with "the agent looks only when Revisar is tapped")?

## Spike before design

On Excalidraw 0.18.1, verify:
- paste/drop handling;
- `addFiles` / `initialData.files` rehydration;
- the `MAX_ALLOWED_FILE_BYTES` value;
- the accepted MIME list;
- how to render a document card (custom element vs image of a thumbnail).

## Risks

- Autosave bloat if files leak into the scene.
- Shared-blob deletion hazards.
- MIME mismatch between Excalidraw and the server.
- Rehydration race on load (broken-image placeholders).
- Per-CLI image-return behavior.
- Prompt injection.
