# Research: bythos-terminal-fase1

- schema: gentle-ai.sdd-research/v1
- revision: 1
- outcome: partial
- requests: `rq-terminal-fase1-a` (lanes 1-2), `rq-terminal-fase1-b` (lanes 3-4)
- source classes granted and used: documentation, open-web
- admission: exact grants declared by the orchestrator (WebSearch, WebFetch)
- accessed_at: 2026-10-17
- local verification: the orchestrator ran it on the dev machine on 2026-10-17 (marked
  [LOCAL])

## Questions

1. Does each CLI (Claude Code, OpenCode, Gemini CLI, Codex CLI) pass MCP `ImageContent`
   to the model?
2. Do stdio MCP servers inherit `BYTHOS_SESION` from the launching shell?
3. Do project-scoped MCP configs need approval?
4. Excalidraw packaging: version, bundle size, offline fonts, APIs, image tool.
5. ConPTY via `x/sys/windows`: pitfalls and Job Object.
6. `coder/websocket`: status and origin semantics.
7. xterm.js: versions and clipboard.

## Sources

| ID | Class | Title / publisher | URL |
|---|---|---|---|
| A1 | documentation | Claude Code MCP docs, Anthropic | https://code.claude.com/docs/en/mcp |
| A2 | documentation | Gemini CLI MCP servers, Google | https://geminicli.com/docs/tools/mcp-server/ |
| A3 | documentation | Codex MCP docs, OpenAI | https://learn.chatgpt.com/docs/extend/mcp?surface=cli |
| A4 | documentation | OpenCode MCP servers | https://opencode.ai/docs/mcp-servers/ |
| A5 | open-web | codex-rs `rmcp-client/src/utils.rs`, OpenAI GitHub | https://raw.githubusercontent.com/openai/codex/main/codex-rs/rmcp-client/src/utils.rs |
| A6 | open-web | MCP TypeScript SDK `client/stdio.ts` | https://raw.githubusercontent.com/modelcontextprotocol/typescript-sdk/main/packages/client/src/client/stdio.ts |
| A7 | open-web (search snippet) | Claude Code issue mirror: image dropped with structuredContent | https://claudeissues.com/issue/54737-bug-mcp-tool-results-image-content-blocks-dropped-when-structuredcontent-is-also |
| A8 | open-web (search snippet) | Codex PRs #5600 / #11346 mirror | https://forge.lthn.ai/core/core-agent-ide/commit/9c4656000f8d1289cb4d28ead4784f8500dd1969 |
| B1 | documentation | npm registry `@excalidraw/excalidraw` latest | https://registry.npmjs.org/@excalidraw/excalidraw/latest |
| B2 | documentation | Excalidraw docs (installation, UIOptions, export, skeleton, API) | https://docs.excalidraw.com/docs/@excalidraw/excalidraw/installation |
| B3 | open-web | excalidraw master `packages/excalidraw/index.tsx`, root `package.json` | https://github.com/excalidraw/excalidraw |
| B4 | documentation | Creating a Pseudoconsole Session; CreatePseudoConsole, Microsoft Learn | https://learn.microsoft.com/en-us/windows/console/creating-a-pseudoconsole-session |
| B5 | documentation | Job Objects; JOBOBJECT_BASIC_LIMIT_INFORMATION, Microsoft Learn | https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects |
| B6 | open-web | coder/websocket on pkg.go.dev and GitHub releases | https://pkg.go.dev/github.com/coder/websocket |
| B7 | open-web | npm `@xterm/xterm`, `@xterm/addon-fit`; `xterm.d.ts` | https://registry.npmjs.org/@xterm/xterm/latest |
| B8 | documentation | `golang.org/x/sys/windows`; UserExistsError/conpty | https://pkg.go.dev/golang.org/x/sys/windows |

## Validated claims

### Q1 MCP images

- Claude Code shows PNG, JPEG, GIF and WebP MCP images inline, possibly downscaled, and
  saves the original under the project's `tool-results`. Images count toward
  `MAX_MCP_OUTPUT_TOKENS` (default 25,000, warning at 10,000). [A1]
- Claude Code once dropped image blocks when `structuredContent` was also set. The issue
  is reported closed, but the fixed version is unconfirmed. [A7, snippet only]
- Gemini CLI sends the image as a separate `inlineData` part. No size limit is
  documented. [A2]
- Codex does not document image handling. PR evidence says MCP images reach the model,
  and text-only models get a text placeholder instead. [A3, A8]
- OpenCode: UNVERIFIED, since its docs are silent. [A4]

### Q2 Environment

- Codex stdio servers get a fixed default allowlist, plus names listed in `env_vars`,
  plus the `env` map. `BYTHOS_SESION` passes only through `env_vars` or `env`. [A5, A3]
- Gemini CLI inherits the host environment but redacts sensitive-looking names (TOKEN,
  SECRET, KEY, AUTH, ...). `BYTHOS_SESION` probably passes; that is inference. Gemini
  supports `$VAR`, `${VAR}` and `%VAR%`. [A2]
- Claude Code `.mcp.json` supports `${VAR}` and `${VAR:-default}`. Without a default, an
  unset variable stays as the literal text. Whether the full environment is inherited is
  UNVERIFIED. [A1]
- The MCP TypeScript SDK passes a small safe allowlist by default, plus the configured
  `env`. [A6]
- OpenCode: `environment` map; inheritance UNVERIFIED. [A4]

### Q3 Approval

- Claude Code prompts in interactive sessions before using project `.mcp.json` servers
  (`claude mcp reset-project-choices` resets the choice). Non-interactive runs skip the
  prompt. [A1]
- Codex honors project `.codex/config.toml` only for trusted projects. [A3]
- Gemini CLI connects stdio servers only in trusted folders (`gemini trust`). [A2]
- OpenCode documents no approval prompt. [A4]
- Freshness: the Gemini docs say Antigravity CLI replaced Gemini CLI for unpaid and
  Google One users on 2026-06-18. [A2]

### Q4 Excalidraw

- Latest stable is 0.18.1, with peers React `^17.0.2 || ^18.2.0 || ^19.0.0`. The
  package declares no `engines`, and the monorepo root requires Node >=18. [B1, B3]
- Offline fonts: copy `dist/prod/fonts` to a served path and set
  `window.EXCALIDRAW_ASSET_PATH` before load. By default the fonts come from a CDN. [B2]
- `convertToExcalidrawElements` exists but its Skeleton API is beta. `exportToBlob`
  exists. `updateScene` takes `elements`, `appState` and `captureUpdate`.
  `getSceneVersion` is exported on master but undocumented. Refs, `ready` and
  `readyPromise` were removed in 0.17; use the `excalidrawAPI` prop. [B2, B3]
- The image tool is hidden with `UIOptions={{tools:{image:false}}}`. Pasted or dropped
  images may still be accepted: UNVERIFIED. [B2]
- [LOCAL] A Vite 5.4 build with React 18.3.1, Excalidraw 0.18.1 (lazy), xterm 6.0.0 and
  fit 0.11.0, on Node 24.19, succeeded with the four APIs imported. Output:
  - JS: 8.1 MB raw across 133 files. The largest chunks are a 1.82 MB subset chunk,
    `percentages` (1.15 MB), `index` (0.75 MB), `cynefin` (0.69 MB) and cytoscape
    (0.44 MB).
  - Most chunks are mermaid diagram modules (Excalidraw's mermaid-to-excalidraw) and
    katex. They load lazily, but `go:embed` still embeds them.
  - CSS: 152 KB.
  - Today's `desktop/ui/dist` is 253 KB.
- [LOCAL] Font sizes: `dist/prod/fonts` is 14 MB in 234 files. 13 MB of that is
  **Xiaolai** (CJK). The other eight families (Excalifont, Virgil, Nunito, Assistant,
  Cascadia, ComicShanns, Liberation, Lilita) total about 0.5 MB.

### Q5 ConPTY

- ConPTY requires Windows 10 1809 or later. Service each pipe on its own thread and keep
  draining it, or it can deadlock. After `CreateProcess`, close the handles you passed
  in. `ClosePseudoConsole` may emit a final frame that must be drained, and it ends the
  attached clients. Closing during child startup can show a 0xc0000142 dialog. The
  streams are UTF-8 VT. `PSEUDOCONSOLE_INHERIT_CURSOR` needs a reply to the cursor query
  or it can hang. [B4]
- `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` (through the extended limit information) ends
  every process in the job when the last handle closes. Children inherit the job, and
  jobs nest on Windows 8 and later. [B5]
- [LOCAL] `x/sys` v0.47.0 (already in go.mod) defines
  `PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE = 0x20016` and
  `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE = 0x2000`, plus the ConPTY and Job APIs. No cgo
  and no new dependency are needed. [B8]

### Q6 coder/websocket

- [LOCAL] `go list -m -versions` shows v1.8.15 as the latest, which resolves the
  pkg.go.dev banner conflict. License is ISC. [B6]
- `Accept` rejects cross-origin requests by default, and the request's own Host is
  always allowed. `OriginPatterns` uses `path.Match`; a pattern containing "://" matches
  the scheme too (enforced since v1.8.14). `InsecureSkipVerify` disables the check. The
  default read limit is 32,768 bytes, adjustable with `SetReadLimit`. [B6]

### Q7 xterm

- `@xterm/xterm` 6.0.0 (MIT) and `@xterm/addon-fit` 0.11.0.
  `attachCustomKeyEventHandler` returns false to stop xterm handling the event. Use
  `getSelection()` / `hasSelection()` for copy and `paste()` for paste (bracketed). [B7]
- [LOCAL] Both build together in Vite; the chunk is 334 KB raw, 85 KB gzip. Runtime
  compatibility is UNVERIFIED.

## Contradictions, uncertainty, freshness

- pkg.go.dev's "not latest" banner for coder/websocket conflicted with GitHub; [LOCAL]
  `go list` resolved it as v1.8.15.
- The Excalidraw export list was read from master, not the 0.18.1 tag. The [LOCAL] build
  against 0.18.1 imported all four APIs successfully.
- A7 and A8 are search snippets; the pages themselves were not read.
- CLI docs moved and products were renamed during 2026 (Codex docs domain, Gemini to
  Antigravity).

## Gaps (why the outcome is partial)

- OpenCode: image handling, env inheritance and approval are undocumented.
- Claude Code: stdio env inheritance and the approval storage key are undocumented.
- Codex: image size limits and the Windows default env list are unknown.
- Runtime behavior on Windows is untested for all four CLIs (image visible, env visible,
  prompt shown).
- Excalidraw paste and drop of images with the image tool hidden: untested.

## Implications for the design (non-authoritative; the orchestrator decides)

1. **Text-first holds.** `pizarra_leer` (structured JSON) is the guaranteed channel and
   the `pizarra_captura` image is a bonus.
   - Always include a text part.
   - Do not set `structuredContent` on the image tool.
   - Keep the PNG under the Claude Code token cap: downscale it and consider including a
     file path.
2. **Active session holds.** Default to the session the UI reports as active.
   `BYTHOS_SESION` is only a hint (Codex drops it unless it is in `env_vars`). The
   generated configs may add `env_vars = ["BYTHOS_SESION"]` (Codex) and
   `${BYTHOS_SESION:-}` (Claude Code).
3. **Approval flows** exist for Claude Code (prompt), Codex (trusted project) and Gemini
   (trusted folder). Bythos should tell the user to approve or trust and surface a
   pending state.
4. **Excalidraw is heavy.** About 8 MB of JS (mostly mermaid and katex chunks) plus
   14 MB of fonts, against today's 253 KB UI. The exe would grow from about 23 MB to
   about 45 MB if everything ships.
   - Shipping the fonts without Xiaolai (CJK, 13 MB) cuts the fonts to about 0.5 MB.
     The cost: Chinese text renders in a fallback font.
   - Lazy loading keeps startup fast, but it does not shrink the exe.
   - Pin 0.18.1 and wrap the beta APIs behind an adapter.
5. **Hand-rolled ConPTY holds:** one goroutine per pipe, close the child-side handles,
   drain on close, create the child suspended and assign it to a KILL_ON_JOB_CLOSE job,
   and check for Windows 10 1809 or later.
6. **coder/websocket v1.8.15 holds**, with an explicit `scheme://host:port`
   `OriginPatterns`, never `InsecureSkipVerify` or `*`, plus a one-time ticket and a
   loopback bind. Raise `SetReadLimit` for large pastes, or chunk them.
7. **Recommended empirical spike** (part of the implementation, before relying on it): a
   dummy MCP tool returning ImageContent plus Text and echoing `BYTHOS_SESION`, run on
   Windows with each of the four CLIs.
