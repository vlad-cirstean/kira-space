# Architecture reference

Distilled in P303, one fact per line. Full earlier text: git show 4e1868e6f0c3:docs/ARCHITECTURE.md.

This file is **authoritative for the app as it stands today**: facts about the app —
driver/dependency choices, protocol constraints, capability quirks, structural reasons
behind them — organized by subsystem/engine so a future session can look something up without
reconstructing it from phase-history prose. Environment-specific operational notes (running
Docker in Claude Code's own sandbox, working around a proxy block, which env var a headless
Linux box needs) belong in `docs/DEV_ENVIRONMENT.md`, not here.

The tree outranks this file — if they disagree, the tree is right and this file needs fixing, not
the other way around. Where this file and any chapter's `SPEC.md` disagree (`docs/HISTORY.md (v1)`,
`docs/HISTORY.md (v1.1)`, `docs/HISTORY.md (v1.2)`, `docs/HISTORY.md (v1.3)`, `docs/HISTORY.md (v1.4)`, `docs/HISTORY.md (v1.5)`, `docs/HISTORY.md (v1.6)`, `docs/HISTORY.md (v1.7)`,
`docs/HISTORY.md (v1.8)`),
**this file is authoritative for behavior**: each `SPEC.md` records what that chapter was
*specified* to be, phase by phase, kept as originally written rather than corrected to match later
reality — see each chapter's own `README.md` for what those folders are and aren't.

**Related documents:** [`docs/PERF.md`](PERF.md) (performance budgets and measured results),
[`docs/PACKAGING.md`](PACKAGING.md) (macOS build and packaging verification),
[`docs/design/kira-design-system/`](design/kira-design-system/) (the workbench visual reference),
[`docs/DEV_ENVIRONMENT.md`](DEV_ENVIRONMENT.md) (how to build, run and test this repo in
whatever sandbox a session happens to be in),
[`CLAUDE.md`](../CLAUDE.md) (the working agreement for changes to this repo).

## Stack

- The desktop shell is Wails v3 (v3.1.0) in Go, targeting macOS 26+ on arm64 only.
- The window uses a custom MacTitleBar (AppearsTransparent, HideTitle, FullSizeContent, no toolbar) over a full-size-content window, not the OS-drawn bar, drawn by the shared packages/workbench/src/components/TitleBar.vue.
- On macOS 26 AppKit picks window chrome by the executable's linked-SDK field. The SDK is the building machine's Xcode; CI and release run on `macos-26`, so dev and release both link SDK 26.
- Each app's own frontend/src/workbench/TitleBar.vue supplies its title-bar buttons and mode tabs through the shared TitleBar shell's default slot.
- TypeScript is pinned to 6 (below TS 7) on purpose: TS7 has no stable programmatic compiler API until 7.1, and vue-tsc (run by typecheck:web:studio) consumes that API in-process.
- The tsgo binary from @typescript/native-preview (used by typecheck:tests:studio and typecheck:unit:studio) is a separate tool already at latest upstream; the plan is to converge on one toolchain once TS 7.1 ships and vue-tsc adopts it.
- Bun is used as package manager, script runner and test runner for tooling only; every adapter is native Go, so nothing at runtime depends on Bun.
- Vite builds apps/kira-studio/frontend/src into frontend/dist, which apps/kira-studio/main.go embeds via //go:embed all:frontend/dist and serves through Wails' AssetOptions.Handler.
- The query console's SQL Format button loads sql-formatter lazily through views/console/sqlFormatterEntry.ts's await import() (~37 KB gzip).
- @faker-js/faker/locale/en is reached via two duplicated one-line entry files (views/grid/fakeData/fakerEntry.ts and packages/api-core/src/http/dynamic/fakerEntry.ts), duplicated because api-core is a no-app-import package (P12 D16(e), P1 D7 biome rule).
- Rolldown folds the two identical faker entry files into one shared stub chunk, with the locale data (en-*.js, ~155 KB gzip) in its own shared chunk; generators.ts (58-entry $name to faker-call dispatch table) gets a third lazy chunk (~0.8 KB gzip).
- monaco-editor is loaded lazily via packages/workbench/src/editor/monaco.ts#loadMonaco() and monacoEntry.ts's dynamic import; every CodeMirror mount, including the SQL console and SchemaDialog.vue's DDL editor, sits behind this same memoised import, so it loads once on the first editor surface opened.
- The repo file viewer's monaco.ts (apps/kira-space/frontend/src/views/repo/monaco.ts) re-exports the shared workbench Monaco bootstrap unchanged.
- Measured in v1.9 P109, the Monaco chunk monacoEntry-*.js is 3 819.42 kB raw / 974.99 kB gzip, with CSS 161.81 kB raw / 24.56 kB gzip and one editor.worker-*.js of 300.37 kB raw loaded as a Worker.
- Each registered Monaco language's Monarch grammar sits in its own lazy sub-chunk (0.4-3.7 KB gzip), fetched only when a file of that language is opened; kira-mongo and kira-redis were added in P60a, transliterated from the CodeMirror StreamLanguages.
- P60a moved the response-history Compare dialog onto Monaco's diff editor and deleted the @codemirror/merge dependency and its mergeEntry.ts chunk.
- The lazy chunks (sql-formatter, faker, generators, monaco) are fetched only on first use and do not grow index-*.js or launch cost.
- P60b removed the nine @codemirror/* packages plus @lezer/highlight, which had been eagerly bundled into the main chunk, cutting index-*.js from 1 723.09 kB raw / 520.66 kB gzip to 1 307.65 kB raw / 384.15 kB gzip.
- After the P98-P112 frontend-library migration (Pinia, TanStack Query, shadcn-vue, reka-ui) and P100 git-ui removal, index-*.js measures 1 339.31 kB raw / 355.29 kB gzip (v1.9 P109, bun run build:studio).
- The UI uses Vue 3 with script setup and the Composition API in VDOM mode; Vapor mode was evaluated and declined in P6.
- Styling uses Tailwind v4 with CSS-first config; tokens mirror VS Code Dark Modern.
- Dynamic colours must be Record<Enum,string> class maps of full literals, never template-built class names, because Tailwind emits only scanned literals.
- ADE tones are @theme tokens in ade/v2/tones.css.
- Every palette colour mark must go through colorMarkClass(mark, color) in packages/theme/src/connColor.ts: rail on left-panel rows, bar on tabs, dot inline, band under view toolbars; no colour leaves the slot empty, and a dot shows a ring. The rail is 4px (`w-1`).
- When converting off plain CSS, stop at the first fitting rung: (1) Tailwind default-scale utility; (2) existing @theme token utility backed by a --kira-* property; (3) a new @theme token only when the value recurs 3+ times or is a deliberate design seam; (4) an arbitrary value only for a one-off or genuinely runtime/viewport/calc value, with a stated reason.
- Define each @theme name once, because Tailwind v4 resolves a duplicated name to the last-loaded definition (base.css and shadcn-bridge.css declare @theme on the same root); never re-declare a name shadcn's bridge already owns.
- The duplicate --color-muted, --color-input and --color-border declarations were deleted from base.css and now live only in shadcn-bridge.css; legacy meanings moved to bg-field and text-muted-foreground.
- scripts/check-theme-classes.sh, wired into bun run lint, guards against retired class names returning.
- A DialogContent with no explicit width spans the viewport minus 2rem, since shadcn's default sm:max-w-sm was removed; every dialog sets its own w-N.
- scripts/check-class-conflicts.ts (bun run lint) parses every .vue under app/theme/workbench, resolves static class and conditional :class literals through cn/twMergeKv, and fails on a surviving static duplicate or a conditional literal that drops a static token; it skips where the :class root is itself a cn(...)/twMergeKv(...) call.
- A registration self-check in the class-conflict lint asserts every new @theme token name (colour, spacing, radius, shadow, animation, leading) wins a same-group merge, so a token not wired into that group fails lint.
- There is one canonical name per colour/radius value repo-wide, including components/ui; shadcn-alias/kira pairs collapse onto the kira name (e.g. background/bg, card/elevated, muted/field, input/border-strong, ring/focus, accent/hover, destructive/error, rounded-sm/md/lg/xl to rounded-kira-xs/sm/kira/pill), because no CSS custom property is set at runtime.
- shadcn-bridge.css's @theme inline keeps exactly --color-primary, --color-primary-foreground, --color-muted-foreground and --color-border, and no --radius-* at all.
- check-theme-classes.sh's check_alias guard keeps retired aliases from returning; it never excludes components/ui and is never extended to git-ui.
- alertVariants has no err key; every error alert uses variant="error" (P263).
- Every spinner uses animate-spin (1s); the 0.7s animate-kira-spin timing was retired.
- ResizeHandle is the only splitter mechanism (P264); reka's Splitter* and the Resizable* wrappers are gone.
- focus-ring in packages/theme/src/base.css is the only focus-ring definition; the base :focus-visible rule applies it to every natively focused element, and a component opts out only with outline-none when an ancestor draws the ring (InputGroup).
- Every element holds the focus ring's colour, width and offset at rest via a base-layer * rule, so :focus-visible only flips outline-style and transition-colors/transition-all never animate the ring from currentcolor/medium/0px (P130).
- tabChipVariants (UI/tabs/index.ts) is the only tab-chip class definition.
- ToggleGroup defaults outline groups to connected (spacing 0) and default groups to 2px-spaced chips, every item rounded-kira-sm; selectors key on reka's data-orientation, never Base UI's data-horizontal/data-vertical (P121).
- Chrome surfaces use only the four-value type scale text-kira-sm/md/lg/xl (sm secondary/caption/status-bar, md default/control/body, lg headings, xl hero titles only); Tailwind's text-xs…text-9xl and the retired xs step are reset via --text-*: initial and guarded by check_font_scale (P123).
- Data views (SQL/document/git grids, Monaco, status-bar/tab dirty-dot glyphs) keep their own fixed or Data-font-size-tracking sizes, outside the chrome type scale by design.
- packages/theme/src/tailwind-core.css is the host-neutral Tailwind entry (P131 Part 1); base.css is a thin @import of it plus Kira-app-only @theme tokens.
- Frontend baseline: Vue 3.5 + Vite, Tailwind v4 CSS-first, shadcn-vue on Reka UI, Pinia for client state, TanStack Query for server state, VueUse for DOM composables, @tanstack/vue-virtual for row virtualization.
- The --kira-* tokens are bridged in packages/theme/src/base.css, and the shadcn-vue palette in packages/theme/src/shadcn-bridge.css.
- Each app creates one Pinia instance (frontend/src/state/pinia.ts) and one QueryClient/VueQueryPlugin in main.ts, with the client from packages/workbench/src/state/queryClient.ts.
- P104 deleted all 20 hand-rolled theme/primitives/* components and repointed call sites at components/ui/* (shadcn-vue on Reka UI), or inlined bespoke layout containers (PanelShell, ViewChrome, ViewHeader, ReconnectGate, RunState, ColorPicker) as Tailwind composition over those parts.
- The v-tooltip directive and its singleton AppTooltip.vue were replaced by the Tooltip/TooltipTrigger/TooltipContent trio, with TooltipProvider owning delay/rearm.
- PanelSplitter, VirtualList and TreeHost were replaced by reka's Splitter* (later narrowed to the Resizable wrappers only) and @tanstack/vue-virtual.
- Only the runtime-settable (Appearance) and z-index tokens stay var-backed/bracketed; other spacing/sizing values use Tailwind's default numeric scale.
- Every fetched components/ui/* set (20: alert, badge, button, checkbox, command, dialog, dropdown-menu, empty, field, input, input-group, label, native-select, popover, resizable, separator, textarea, toggle, toggle-group, tooltip) must have a real consumer or be deleted.
- CommandPalette.vue is built on ui/command, where reka's Listbox owns filtering and keyboard navigation.
- ui/scroll-area and ui/context-menu were deleted unused; the app's point-anchored ContextMenu.vue is built on ui/dropdown-menu instead.
- P110 converted every .p-* rule, every remaining hand-rolled Studio/Space/workbench style block, and kira-ui's controls.css onto Tailwind utilities, @theme tokens or shadcn-vue variants.
- packages/theme/src/primitives.css is residue only (75 lines), holding the one rule .p-input.is-grow, which has no Tailwind utility equivalent and is kept unlayered on purpose so it keeps beating a layered utility by construction.
- packages/workbench/src/workbench.css contains only two non-comment lines: @import "@theme/base.css"; @source "./"; which pull in the compiled Tailwind root and this package's own scan.
- packages/git-ui/src compiles in Kira Space's own Tailwind root (P245); the kv: prefix, git-ui's second root and the --vscode-*/--kv-* token layer are gone, and its only CSS is theme/git.css, imported by Space's styles.css.
- scripts/check-theme-classes.sh guards retired class names (the p-* family, legacy text-muted/bg-input names, retired kui-* selectors) from returning, and runs as part of bun run lint.
- Fetch a new shadcn-vue component set by direct-registry pull, not by shadcn-vue add; the procedure is in docs/DEV_ENVIRONMENT.md's shadcn-vue section.
- Empty/EmptyMedia/EmptyTitle/EmptyDescription (components/ui/empty/) is the one empty-state component; Alert with class empty-state (role=alert on a merely empty list) was a misuse and was converted.
- Row components (TreeRow, CollectionRow, RepoTreeRow, RepoSearchRow) take a class prop and own their selection ternary instead of using a shared .tree-row/.selected/.sticky CSS rule.
- Row state class is selected ? 'bg-select' : sticky ? 'bg-bg hover:bg-hover' : 'hover:bg-hover'; sticky is a prop so a row in a sticky layer paints its own background, and selected beats hover by ternary order, not CSS specificity.
- In git-ui dialogs, pair every control that is not natively input-like with an explicit useId()-driven for/id, because biome's noLabelWithoutControl can't see through a wrapped component's inheritAttrs: false to the native element.
- Pass prioritize-position on a point-anchored DropdownMenu so reka's Popper flips before it shifts, since shift alone can clamp overflow to zero against a 0×0 anchor and starve flip of a signal.
- AttributeTooltip.vue (moved from workbench to theme) is one hoisted instance per list/grid container, mounted as a sibling, never a child, of any container SlickGrid's init() can empty, and reads a data-kira-tip attribute on descendants.
- TooltipProvider wraps both packages/git-ui mount roots via MountRoot.vue in main.ts's mount().
- packages/git-ui is fully on shadcn-vue (P131), with no Kui* import left in the package.
- @kira/kira-ui was deleted in P267; before that it was reduced to floatingPosition.ts after P264 deleted KuiColumnResizeHandle.
- The phone terminal transport uses github.com/coder/websocket (ISC): ordered binary frames both ways, context and close codes; already in the module graph; separate from the SSE hub.
- The app uses monaco-editor as the single engine for every editor surface; CodeMirror 6 was removed entirely as of P60b.
- The SQL query console and project/SchemaDialog.vue's DDL editor were the last CodeMirror mount sites; both now mount editor/MonacoHost.vue, and editor/CodeMirrorHost.vue plus the remaining @codemirror/* packages and @lezer/highlight were deleted from package.json.
- The SQL console and DDL editor stayed on CodeMirror through P60a because their language service (completion, diagnostics, hover, DDL extractor) depended on @codemirror/lang-sql's Lezer parse tree; P60b replaced that tree with an in-house one (packages/shared/domain/sqlTokens.ts).
- The SQL Monarch grammar in monacoEntry.ts is colouring-only and needed no change for the migration.
- Every editor surface loads Monaco lazily through the one shared, memoised loadMonaco() in editor/monaco.ts; a session that opens no editor fetches nothing, and the first editor pays the ~972 KB gzip chunk once, cached thereafter.
- packages/shared/domain/ holds three SQL siblings built on one shared quote/comment/dollar-quote lexical scanner (sqlLex.ts): sqlSplit.ts (statement splitting), sqlLint.ts (lexical diagnostics: unterminated quote/comment, unbalanced parens), and sqlTokens.ts (parse layer).
- sqlTokens.ts#tokenizeSql produces the same shallow LNode tree shape (name/from/to/firstChild/nextSibling; Statement, Parens and CompositeIdentifier the only groupings) that @codemirror/lang-sql gave the console language service; consumers in views/console/{ddl,sqlRefs,sqlHover,sqlDiagnostics}.ts only changed to take a plain SqlDialect string.
- packages/shared/domain/sqlKeywords.ts supplies a curated keyword/type vocabulary for each of the five SQL dialects, and a membership test ensures every consumer that branches on name === 'Keyword' stays covered.
- sqlTokens.ts has a module-level memo keyed on (options, source) by reference, size 2, FIFO, so a hover/lint/completion burst over one untouched document costs one real scan.
- No dependency was added for SQL parsing; sql-parser-cst was declined because it is GPL-2.0-or-later while the repo is MIT and ships a packaged desktop binary, which would impose a copyleft obligation across the whole app.
- node-sql-parser was declined because it throws on unparseable input and has no reliable per-token offsets.
- dt-sql-parser and monaco-sql-languages were declined because they cover neither SQLite nor ClickHouse and pull in an ANTLR runtime for a few node kinds.
- The in-house SQL tokenizer covers all five dialects, tolerates half-typed text (never throws), and keeps byte-accurate offsets.
- Zod's remaining TypeScript-side job is connection-dialog input validation; the engine wire protocol it used to guard went with the deletion of src/engine/ (P58f).
- Rows read back out of SQLite are validated in Go with hand-written model decoders in apps/kira-studio/internal/storage/model/.
- Linting uses Biome for TS/Vue and golangci-lint for Go; gocognit and gocyclo run at min-complexity 30 and Biome's noExcessiveCognitiveComplexity at maxAllowedComplexity 30.
- knip checks TS/Vue dead files, dependencies, duplicate exports, and unused exports/types/binaries.
- Biome is the single TS/Vue formatter and linter; ESLint and Prettier are not used.
- bun run lint also runs scripts/check-tokens.sh, scripts/check-theme-classes.sh and scripts/check-class-conflicts.ts.
- golangci-lint and knip run only in the pre-push hook and CI (.githooks/pre-push), not per commit.
- The app's SQLite database lives at ~/.kira-studio/kira.db and is accessed from Go via database/sql with modernc.org/sqlite (pure Go, no cgo), with SetMaxOpenConns(1); there is no ORM and Drizzle is gone.
- Packaging uses wails3 task darwin:package:dmg plus scripts/sign-bundle.sh; the .app and .dmg are ad-hoc signed (identity -), with no notarization.
- App icon is an Icon Composer `appicon.icon` with one full-bleed layer; the system applies the mask. actool 26 compiles it to `Assets.car`, named by `CFBundleIconName` `appicon` (P349).
- The DMG is a styled disk image with an /Applications shortcut, has no silent auto-update (a user-initiated one is scripts/install.sh, P119), and has no runtime/ tree to vendor or sign.
- No DMG carries a .vsix, because Kira Space's VS Code extension was removed in P243 Part 2.
- Real-container DB adapter tests are written in Go with testcontainers-go (apps/kira-studio/internal/adapters/testsupport/, run by bun run test:go), not in Bun; Colima is the container runtime.
- packages/db-fixtures/ no longer holds per-engine specs; it is the shared fixture corpus (fixtures/*.sql, support/*.ts) seeded by Go's testsupport package and apps/kira-studio/tests/e2e-real/, with Bun driving only the e2e-real seeding.
- UI tests use Playwright against the built bundle in real WebKit.
- Logging uses Go log/slog to a single daily-rolling file under ~/.kira-studio/logs/, mirroring the former electron-log configuration.
- The grid engine is slickgrid@5.20.0 core SlickGrid class only, with no SlickDataView, plugin or slickgrid-vue; it is the only grid engine, and the old DataGrid.vue, GridRow.vue and __kiraGridEngine are gone (P22 Pass B).
- views/grid/SlickGridHost.vue hosts the data tab with sort, editor, selection ranges, FK/PK nav and clipboard.
- A left-click on an outbound-FK cell's nav button opens a read-only preview popover (FkPreviewPopover.vue) fetched with one tab-free, attribution-only data.read; its actions are 'Open in new tab' (navigateForeignKey) and 'Edit this record' (P67).
- The FK preview popover is positioned point-anchored through PointPopover rather than as a Vue sibling, because the nav button is imperative DOM with no Vue parent to render a sibling into.
- 'Edit this record' (editReferencedRow in views/grid/menu.ts) opens a new pre-filtered tab on the referenced table and puts the caret on its first non-key column in edit mode via a pending-focus handshake (views/grid/focusRequest.ts); it reuses the ordinary data tab since no second editor exists.
- A multi-candidate FK click still opens the right-click ContextMenu, with each item opening the preview for its own edge; the PK-side 'Referenced by' button/submenu is unchanged and has no preview because it names multiple tables.
- views/console/ConsoleSlickGrid.vue hosts the query console's tabular results (~300 lines) over shared layers: views/shared/slick/kiraSlickGrid.ts, views/shared/slick/dataSource.ts (CustomDataView bridge), slickTheme.css, views/shared/page/columns.ts and theme/cellClass.ts.
- The console grid host has no sort, editor, FK nav or persisted column widths, because a console result has no use for them. It has the same selection model (cell, range, row, column) and Copy menus as the data grid, minus INSERT and every write item.
- Both grids copy through views/shared/slick/selectionCopy.ts (`selectionText`, `rowsText`), so ⌘C and every Copy menu item write the same text for a selection. Right-click acts on the selection when it covers the hit; otherwise it selects the hit first. Both functions stay synchronous because WebKit grants `writeText` only inside the user gesture.
- The data grid prints ⌘C on Copy rows ▸ TSV only for a row selection; on a range or column selection ⌘C copies cells while the item copies whole rows.
- tests/ui/grid-copy-menu.spec.ts reads the real clipboard on Chromium (Playwright project `ui-clipboard`) and runs on WebKit (`ui`) against a pass-through `writeText` recorder, because WebKit refuses an ungestured `readText()`.
- The grid no longer uses @tanstack/vue-virtual because SlickGrid's native virtualization replaced it; the dependency is still used by other list views through packages/workbench/src/behaviour/virtualRows.ts and treeVirtualRows.ts.
- kiraSlickGrid.ts positions columns through --sg-l<i>/--sg-r<i> custom properties on the grid root plus one shared append-only sheet, never SlickGrid's per-grid <style>, because WebKit rebuilds every element's style on a sheet removal or rule mutation (~55 ms per mount on the test page).
- CommitGrid.vue uses plain SlickGrid and keeps the per-grid style sheet.
- slickTheme.css and CommitGrid.vue set contain: layout paint on .grid-canvas, because without one stacking context overlapping runway rows each get a compositing layer rebuilt per row mount (P162).
- The outbound HTTP client is plain net/http in apps/kira-studio/internal/httpclient/ with no client, retry, URL-parsing or multipart-builder dependency.
- The HTTP client uses one package-level *http.Client with a 30s timeout via context.WithTimeout rather than Client.Timeout, so Stop and timeout abort an in-flight body read the same way; it follows redirects (every hop recorded, up to 10), always verifies TLS, and uses http.ProxyFromEnvironment.
- The outbound HTTP client is reachable only from Go; the webview's own fetch is never used (the 'Go owns the network' invariant).
- The request builder supports body modes none/raw/code/urlencoded/formdata/file (internal/httpclient/body.go); a two-pass mime/multipart writer computes an exact Content-Length from a fixed boundary before streaming, so form-data or binary sends are never chunked.
- One stdlib net/http/httptrace.ClientTrace is installed per send, recording each redirect hop's DNS/connect/TLS/wait/download phases, bucketed by checkRedirect (P10).
- The outbound gRPC client lives in apps/kira-studio/internal/grpcclient/ and uses google.golang.org/grpc, google.golang.org/protobuf (dynamicpb/protojson/protodesc/protoregistry, grpc-go's reflection client) and bufbuild/protocompile, with no generated .pb.go code and no protoc/buf build step.
- The gRPC client is dynamic: a method is discovered via server reflection or a supplied .proto compiled by protocompile (no codegen), then called through dynamicpb/protojson.
- The gRPC client supports only unary and server-streaming calls; client- and bidi-streaming are out of scope.
- gRPC deadlines (P232): connect plus HTTP/2 handshake 5 s, whole reflection 15 s; a unary call follows api.requestTimeoutMs (0 = none, default), overridable by GrpcCallArgs.timeoutMs clamped to 0..3_600_000; each timeout ends in E_TIMEOUT naming target and wait.
- A gRPC server stream has no deadline; Stop ends it.
- gRPC Describe takes an optional opId (a cancellable op, not persisted).
- A gRPC Service carries name (short, for display) and fullName (what Call addresses).
- The gRPC stack is the largest single dependency the app has taken, about 14.2 MB of binary (linux/amd64, no flags), the same order as pgx, mongo-driver/v2, both AWS SDK clients and franz-go combined (about 13.5 MB).
- Every gRPC descriptor source gets its own private *protoregistry.Files, never protoregistry.GlobalFiles, because GlobalFiles panics on a duplicate file path, which is realistic when two users' .proto files declare the same package.
- The Git module transport was moved entirely to Kira Space as of v1.9 P100 and is no longer part of Kira Studio's stack.
- The MCP server for internal/dbmcp uses github.com/modelcontextprotocol/go-sdk v1.8.0 over the SDK's Streamable HTTP transport, not stdio.
- The official MCP go-sdk was chosen as the protocol org's reference implementation at stable v1; mark3labs/mcp-go (second implementation) and hand-rolled JSON-RPC framing were declined, since CLAUDE.md's library-first rule finds nothing hand-rolling would earn.
- The MCP server uses Streamable HTTP rather than stdio because it is one long-running process serving many concurrent clients and tool calls, not a process spawned per client; the SDK's auth.RequireBearerToken middleware gates every request.
- mcp.AddTool[In, Out] derives each MCP tool's input schema from a Go struct's tags, so every tool's schema has exactly one source. A tool argument the handler needs carries no `omitempty` (schema-required). A string or array that must be non-empty also goes through `llmprompts.RequireNonEmpty`. go-sdk rejects either before the handler runs.
- The repo-map MCP server was removed in v1.9 P97.
- Local embeddings (P210, Kira Space memory) use ONNX Runtime 1.31.1 via yalue/onnxruntime_go v1.38.0, gomlx/go-huggingface v0.4.20 (hftokenizer + hub downloader), and model Snowflake arctic-embed-s int8 (Apache-2.0).
- Embedding inference cgo-dlopens a bundled libonnxruntime and runs in a memory-embed worker subprocess; vectors are stored as BLOBs in memory.db and search is done in Go.
- Declined for embeddings: hugot pure-Go backend (458 MB loaded, 1.05 GB peak, 40x slower) and its ORT backend (drags GoMLX and go-xla into go.mod for one pipeline).
- Declined for embeddings: sqlite-vec (native extension modernc.org/sqlite cannot load; its KNN is brute force anyway), llama.cpp (second native toolchain), EmbeddingGemma (Gemma terms), jina v3 (non-commercial).
- Memory bulk import (P211, Kira Space) uses goldmark v1.7.16 Markdown AST for chunk boundaries, go-git v5.19.2 plumbing/format/gitignore matcher only, and cenkalti/backoff/v4 for retry.
- Memory bulk import declined a hand-rolled Markdown splitter, a hand-rolled gitignore matcher, and a tokenizer for chunk sizes; the estimate is ceil(runes / 3), within the margin the 3000-token target leaves.
- go-git pulls go-billy and a few indirect dependencies for the single gitignore subpackage used.
- Memory fact ids (km_) and group ids (kg_) are generated by newID in internal/memory/ids.go. Agent branch ids (kira/) come from the same internal/shortid package (github.com/matoous/go-nanoid/v2 v2.1.0, Crockford base32 lowercase); memory ids 12 chars, branch ids 10.
- Migration 4 mints legacy memory ids in SQL (random() & 31) because sqlitex.Migrate runs SQL only.
- monaco-editor (MIT, pinned 0.56.0) is declared in the root package.json dependencies beside slickgrid; neither packages/workbench nor either app's frontend/package.json redeclares it.
- Monaco is read-only only in the native code workspace (repo file viewer outside edit mode, and diff tabs); P60a reuses the same dependency for other mostly editable studio/api editor surfaces, so read-only is not a property of the dependency.
- Load Monaco via monaco-editor/features/register.all.js (every standard contribution, no language service, no worker), never the package root editor.main.js, because it eagerly pulls in all four language services and ~180 grammars.
- Exactly one Monaco worker ships (editor.worker, backing IEditorWorkerService).
- C6's diff editor uses mod.editor.createDiffEditor with hideUnchangedRegions.enabled and renderSideBySide on, and renderMarginRevertIcon and renderGutterMenu off; the diff contribution was already in register.all.js, so the Monaco chunk is unchanged.
- As of P67c, 81 basic languages get a registered Monarch grammar via the register.all.js import in packages/workbench/src/editor/monacoEntry.ts (up from 19).
- .json/.jsonc are colored by Monaco's worker-free JSON tokenizer, not the JavaScript grammar; .vue/.svelte are colored as plain HTML because no grammar exists for either.
- Monaco `editor.addCommand` registers its handler in the global command registry and returns no disposable, so the handler's closure outlives the editor. Use `editor.addAction` and dispose the result on unmount (MonacoHost, `useFileEdit`).
- Call `setModel(null)` before `dispose()` on an editor whose models outlive it (Space `editors.ts` `releaseWidget`); a widget disposed with cached models attached stays reachable from them.
- The git graph in the native workspace moved to Kira Space in v1.9 P100; apps/kira-studio/frontend/package.json no longer depends on @kira/git-ui, @kira/git-ipc, @kira/git-core or seti-icons. @kira/kira-ui was deleted in P267.
- Database drivers are Go-native for all ten kinds as of P58e M9.3: pgx/v5 (postgres), go-sql-driver/mysql (mariadb/mysql via shared mysqlfamily core), modernc.org/sqlite (pure Go), hand-rolled net/http client (clickhouse), mongo-driver/v2, go-redis/v9, aws-sdk-go-v2 sqs/s3 (shared awscfg package), franz-go + kadm (kafka).
- The database/adapter surface is cgo-free, but some darwin-only files use cgo for OS integrations (repo-root internal/secrets and internal/metrics, Kira Studio internal/localauth, Kira Space internal/gitclient FSEvents watcher), each behind a darwin && cgo build tag with a non-cgo companion.
- CGO_ENABLED=1 is needed for real darwin builds (beyond Wails' macOS bindings) but never for a Linux dev/test loop.
- The Node engine child process was removed in P58f M10, along with its build/vendoring machinery and supervision code; it already served no connection traffic after C2 (P58e M9.3).
- Kira Studio app identity: organisation kirathecat, bundle ID com.kirathecat.kira-studio (the -shell suffix is gone), executable Contents/MacOS/Kira Studio (space included).
- There is no silent auto-update; P119 adds a user-initiated update through scripts/install.sh.
- Kira Space (P100) ships as its own DMG under the same organisation, with bundle ID com.kirathecat.kira-space (set in apps/kira-space/build/config.yml, mirrored in build/darwin/Info.plist) and executable Contents/MacOS/Kira Space, using the same install/update path.

## Invariants

- The renderer must never import a DB driver and never parse a wire protocol.
- The grid must not create a DOM node per cell for off-screen rows; it is virtualized in both axes.
- Row data must not use Vue reactivity: rows live in plain frozen typed structures, the grid reads them imperatively and re-renders on an explicit version counter.
- Long lists (tree, log panel, document view) must be virtualized too.
- Every operation exceeding ~150 ms must show progress and a working stop button.
- Every DB read must go through the cache layer; a cache miss is the only thing that produces a query.
- Bulk data is produced and encoded exactly once, in the process that owns the window (P58 D3): the Go adapter builds a result page, it is held in the Go-side L2 cache as native structures, and it is serialized once when a renderer asks for it.
- Go parses every data-plane request envelope, since it answers them. This replaced the Electron-era rule that bulk data skips the main process (MessagePort) and the P58a–P58e interim rule that the Go process forwards without looking.
- The forwarding interim rule held only while a Node-served kind could exist beside a Go-native one; P58f M10 deleted that seam along with the Node engine child.
- The renderer must load no remote content, open no window, and navigate nowhere but its own base URL. Under Wails only the code-level half holds: the renderer contains no such call, but no navigation policy is left to stop one (Electron's shell also enforced it).
- The shell can open a second window via Window → New Window (⇧⌘N, P8), but only from a Go-side menu command, never from a renderer-initiated call; JavaScriptCanOpenWindowsAutomatically is Disabled and the renderer never calls window.open or an equivalent.
- P66/P119 update-availability check and install are Go-side: internal/appupdate fetches GitHub release metadata itself, and InstallUpdate stages the update through its own detached installer. The renderer calls only nullary methods (Status, Check, InstallUpdate, CancelInstall) and sends or receives no URL. Check bypasses the cache and surfaces a failed check; a poll keeps failure silent.

## Adapter contract

- Each database/queue/storage engine is one package under apps/kira-studio/internal/adapters/ (postgres, mysqlfamily shared by mariadb/mysql, sqlite, clickhouse, mongo, redis, sqs, s3, kafka) implementing the Adapter interface in adapter.go.
- The Go Caps struct (adapters/caps.go) declares what each engine can do; its field order is kept identical to the TypeScript Caps type in packages/shared/caps.ts so the two can be diffed against each other.
- KeyBrowser cap is true only for redis and s3: their top-level container's key/object space is unbounded and nested, so the project tree treats it as a leaf and the UI reaches it through a dedicated Browse tab.
- The UI must read only Caps, never a connection.kind check, to decide what to show.
- registry.go's loaders map is a plain constructor table where each adapter package registers its constructor from its own init(); it is not a lazy-import map, because Go links every adapter into the binary so no per-engine baseline-memory concern remains.
- The nativeKinds map is gone; every connection kind routes through the loaders registry. Only a stale comment remains, at apps/kira-studio/internal/storage/repos/connections.go:172.
- Console results are capped: adapters.DefaultConsoleCap is 10,000 rows and 56 MiB of held cell text, shared by every statement of a batch via adapters.RemainingCap; an exhausted budget admits one row, then reports truncated. The cap is sized so a full batch fits the data-frame response limit.
- model.ConsoleRequest.Cap (Go callers only, never on the renderer wire) lowers the console cap, and adapters.ConsoleCapFor clamps anything above the default.
- An engine checks the cap after Next() and before appending, so PagePosition.Truncated means at least one more row existed and an exactly-cap result reports false.
- has_more stays false for console pages because no continuation exists; truncation is signaled by the truncated field on PagePosition (wire and TS).
- Early close at the console cap: SQLite and MongoDB stop for real (rows.Close resets the statement, cursor.Close kills the server cursor); ClickHouse closes the HTTP body and sends cancel_http_readonly_queries_on_client_close.
- PostgreSQL and MySQL/MariaDB drain remaining rows off the wire without allocating them after the cap, so memory is bounded but server and network time are not.
- A CancelRequest after the cap was declined aborts the BEGIN READ ONLY wrap and fails later statements in the batch.
- Redis console rewrites KEYS, HGETALL/HKEYS/HVALS, SMEMBERS, LRANGE, ZRANGE/ZREVRANGE, ZRANGEBYSCORE/ZRANGEBYLEX and XRANGE/XREVRANGE into bounded forms (SCAN/HSCAN/SSCAN loops, clamped ranges, LIMIT/COUNT) in redis/consolebound.go, keeping the reply shape.
- The renderer shows a banner on a truncated console page, and queryplan and console/plan.ts refuse a truncated page as an incomplete plan.
- A Chunk carries an optional sorted binary row list whose cells hold standard base64 of bytes that were not valid UTF-8 (page.BytesCell).
- Kafka keys, values and header values use the binary mechanism instead of a replacement-character string; a non-UTF-8 header value becomes {"base64":"…"} in the headers JSON.
- SQS bodies go through the binary helper but it never fires, because SQS rejects a body outside its Unicode set at SendMessage and binary attributes are already base64 in the headers cell.
- The stream view marks a binary cell with a base64 badge; copy and search use the base64 text.

### Per-database mapping

| DB | Tree levels | Default view | Pagination | Exact count | EXPLAIN form (P18) | Cancel mechanism |
|---|---|---|---|---|---|---|
| PostgreSQL | database → schema → tables (ungrouped), views/matviews/functions/sequences grouped into per-kind folders | tabular | keyset on PK, else `LIMIT/OFFSET` | yes | `EXPLAIN (FORMAT JSON, COSTS TRUE, VERBOSE FALSE, SETTINGS FALSE, BUFFERS FALSE) <sql>` — nested `Plans[]` tree | `pg_cancel_backend(pid)` on a side connection |
| MariaDB | database → tables (ungrouped), views/routines grouped into per-kind folders (routines labelled "Routines") | tabular | keyset on PK, else `LIMIT/OFFSET` | yes | `EXPLAIN FORMAT=JSON <sql>` — `nested_loop[]`/`read_sorted_file`/`filesort` wrappers, scalar `cost` (**a different JSON schema than MySQL's own**, despite the identical statement — F13) | `KILL QUERY <threadId>` on a side connection |
| MySQL | database → tables (ungrouped), views/routines grouped into per-kind folders (routines labelled "Routines"); no sequences (MySQL has no SEQUENCE engine) | tabular | keyset on PK, else `LIMIT/OFFSET` | yes | `EXPLAIN FORMAT=JSON <sql>` — `query_block.table`/`nested_loop`, `cost_info.query_cost` | `KILL QUERY <threadId>` on a side connection |
| SQLite | one `database` node per `PRAGMA database_list` entry (in practice always exactly `main`) → tables (ungrouped), views grouped into a folder; no sequences or routines (SQLite has neither) | tabular | keyset on PK, else a unique index, else the table's own implicit `rowid` (never mutation identity); `LIMIT/OFFSET` only for a view or a text-sorted request | yes (`count(*)` measured at ~9 ms/1M rows) | `EXPLAIN QUERY PLAN <sql>` — N rows × `(id, parent, notused, detail)`, **no cost and no row estimate at all** | `modernc.org/sqlite`'s real `sqlite3_interrupt`, reached by cancelling an adapter-owned per-op `context.Context` on that op's own dedicated `*sql.Conn` (Go-native as of P58b M6.3 — the Node adapter had no such mechanism at all) |
| ClickHouse | one node per `system.databases` row → tables (ungrouped), views/materialized views grouped into per-kind folders; no sequences or routines (ClickHouse has neither); `system` is kept, not hidden | tabular | `LIMIT/OFFSET` only — a MergeTree `PRIMARY KEY` is a sparse index, with no unique row key to build a keyset cursor on | yes (`count()` reads part metadata) | **two statements, one call:** `EXPLAIN PLAN json = 1, indexes = 1, description = 1 <sql>` (index selectivity, no cost) then `EXPLAIN ESTIMATE <sql>` (the only row-count figure) | `KILL QUERY WHERE query_id = '<id>' SYNC` on a second HTTP request (the client's own connection pool already has one free) |
| MongoDB | database → collections (ungrouped, indexes shown in the definition view) | documents | `_id` keyset, `skip/limit` fallback | `countDocuments` (slow) / `estimatedDocumentCount` | — (P18's SPEC row is the five SQL dialects only) | `$currentOp` + `killOp` on the *same* client the adapter already holds (never a side connection) |
| Redis | db index (a leaf — its key namespace is unbounded, browsed in a Browse tab) | key/value | `SCAN` cursor (never `KEYS`) | `DBSIZE` only (approx per-prefix) | — | a permanent, honest `false` — go-redis's blocking commands override the caller's context for the wait itself, so `CheckCancelled` between bounded `SCAN`-family rounds is the entire cancellation surface (`caps.cancel` stays `true`, since that surface is genuinely effective) |
| Kafka | cluster → topics (ungrouped), consumer groups (folder) | stream | offset window per partition | end-offset − begin-offset | — | none server-side — Kafka's protocol has no cancel operation at all, so the op's own `context.Context`, passed directly to every `kadm`/`kgo` call, is the entire mechanism; `caps.cancel` stays `true` since that surface is genuinely effective |
| SQS | region → queues | stream | receive batches | `ApproximateNumberOfMessages` | — | none server-side — the op's own `context.Context`, passed directly to every AWS SDK call (never a detached context), is the entire mechanism; `caps.cancel` stays `true` since that is genuinely effective |
| S3 | account → buckets (a leaf — a bucket's prefix/object space is unbounded, browsed in a Browse tab) | key/value (object browser) | `ListObjectsV2` continuation token | **per-object** exact field-row count via `HeadObject` (not a bucket-wide key count — S3 has no cheap exact answer to "how many keys total") | — | none server-side — same as SQS, the op's own `context.Context` on every SDK call; also load-bearing for `DownloadObject`'s temp-file cleanup ordering |

- SQS reads are never automatic: the stream view has an explicit Poll button with a visible warning, and nothing is fetched on tab open, on refresh, or on a timer.
- SQS has no peek API: every ReceiveMessage raises ApproximateReceiveCount and makes messages invisible to real consumers for the visibility timeout, and a message past the redrive policy's maxReceiveCount moves to its dead-letter queue.
- A read-only SQS connection hides messages for a user-set time (1 s to 12 h, default 1 s) instead of the queue's timeout; 1 s is the smallest because the SDK omits VisibilityTimeout: 0 from the request.
- The SQS hide time is per tab (StreamTabState.sqsVisibilityTimeoutSeconds), set in the toolbar, and sent as {"visibilityTimeoutSeconds":N} in the read request's filter with no wire change.
- The SQS adapter rejects a malformed or out-of-range filter before touching the connection, and the ops log command shows the applied value as ReceiveMessage <url> VisibilityTimeout=N.
- Stop cannot unhide messages early on a read-only SQS poll, because it keeps no receipt handles and ChangeMessageVisibility is a write call a least-privilege principal may lack.
- A read-only SQS poll dedupes by MessageId across the receives of one poll and stores no receipt handles.
- The warning on a read-only SQS connection states the hide trade-off (longer avoids a repeat in one poll but keeps messages from consumers that long; shorter can end a poll early) and that each poll raises the receive count.
- The first SQS Poll per tab and per hide time asks for confirmation naming the hide time and the dead-letter threshold (StreamPage.max_receive_count, else the queue's RedrivePolicy) or its absence; acknowledgement is in-memory (rt.receiveAcknowledged).
- Writable SQS connections ignore the filter value, keep the queue's own visibility timeout, and keep receipt handles for Delete.
- SQS authentication is by named AWS profile; static keys are accepted only in URI mode.
- Cancellation is always forwarded to the server, never just "stop showing the result"; if a driver cannot cancel, the capability is absent and the stop button says so.
- Every adapter maps its driver's errors to the closed ErrorCode set via New/CodeOf in apps/kira-studio/internal/adapters/errors.go, preserving the driver's message verbatim (Adapter rule 4).
- errors.go holds shared adapter guards: Unsupported/NoQueryConsole (E_UNSUPPORTED stubs), AssertWritable (read-only refusal opening Mutate), CheckNotStarted/CheckCancelled (cancellation checks against the op's context), and RequireConnected[T] (guard that Connect ran).
- sqltext.go holds shared driver-agnostic SQL text/planning glue for SQL adapters' read.go: projection resolution, keyset-eligibility rule with per-dialect tiebreaker, page-token encode/decode, WhereClause/ParseCountValue, and the keyset pipeline (AssertKeysetSupported, ResolveFetchColumns, BuildScanOrderBy, BuildKeysetPosition) used by postgres/mysqlfamily/sqlite.
- sqlmutate.go holds shared mutation guards (OrderedOps, AssertColumnsKnown, AssertAffectedExactlyOne, AssertKeyIsPrimaryKey) and a generic row-op-to-SQL renderer (ValueRenderer, LiteralRenderer/NewParamRenderer, RenderRowOp) shared by postgres/mysqlfamily/sqlite apart from dialect placeholder and qualified-name format.
- abort.go's RunWithAbortRace[T] is the single abort/settle race used by every adapter with a callback-style or detached-context driver (postgres, mysqlfamily).

## Per-engine adapter facts

### PostgreSQL / MariaDB / MySQL

- PostgreSQL, MariaDB and MySQL adapters are full read/write SQL adapters using keyset pagination on the primary key (falling back to LIMIT/OFFSET) and cancellation on a side connection (pg_cancel_backend for Postgres, KILL QUERY for MySQL family).
- PostgreSQL is Go-native (apps/kira-studio/internal/adapters/postgres/, pgx/v5) and served in-process by adapterhost.Router through the loaders registry; no Node engine child exists any more (deleted P58f).
- The Go Postgres port keeps keyset-on-PK pagination and pg_cancel_backend on a side connection using the tracked backend pid; only the hosting process and query-context handling changed.
- In the Postgres adapter, never pass the caller-side op cancellation context.Context to pgx's Query/Exec, because pgx honours context cancellation and races its own cancel request against the explicit pg_cancel_backend call; use adapters.RunWithAbortRace (internal/adapters/abort.go).
- Postgres declarative partitions list as children of their partitioned parent (`pg_inherits`, `relispartition`), not at schema level. A partition path uses its own schema.
- MariaDB and MySQL are Go-native (apps/kira-studio/internal/adapters/mysqlfamily/, github.com/go-sql-driver/mysql); mysqlfamily/ holds the single Adapter implementation, while mariadb/ and mysql/ each hold only their own Profile (server label, ApplyEngineOptions).
- In mysqlfamily, never pass the op's own context to the driver, because cancelling the context passed to QueryContext/ExecContext closes the underlying connection outright (driver: bad connection on the next statement); use adapters.RunWithAbortRace with a background context and an explicit KILL QUERY <threadId> on a side connection (query.go).
- The Go MySQL-family adapter lost the query console's "N row(s) affected" status text (a generic "OK" shows instead), because go-sql-driver/mysql's QueryContext path, which the console's multi-statement runner needs, has no RowsAffected().
- allowPublicKeyRetrieval has no equivalent in the Go MySQL-family adapter: go-sql-driver/mysql unconditionally requests the server's RSA public key over plaintext when caching_sha2_password needs one and TLS is off, with no option to refuse.

### SQLite (P35; Go-native as of P58b M6.3)

- The SQLite adapter is Go-native as of P58b M6.3, at apps/kira-studio/internal/adapters/sqlite/, using modernc.org/sqlite (pure Go, cgo-free), the same driver that backs the app's own storage.
- The Go SQLite adapter reports caps.cancel true; the old Node adapter reported false because node:sqlite (Bun 1.4+/Node 22.5+ builtin) had no sqlite3_interrupt and a synchronous API that blocked the event loop so an abort could not be delivered.
- Go SQLite cancels by cancelling an adapter-owned per-op context.Context on that op's own dedicated *sql.Conn (one connection per op, not a pinned shared one), reaching sqlite3_interrupt; Postgres/MariaDB/MySQL instead cancel through a side connection.
- SQLite keyset pagination falls back through primary key, then unique index, then the table's implicit rowid (never surfaced as mutation identity), before degrading to LIMIT/OFFSET; unchanged from the Node design.
- The modernc.org/sqlite driver's rows.Next() unconditionally re-parses TEXT into a Go time.Time when the column's declared type is DATE/DATETIME/TIMESTAMP and the text parses as a recognised time layout; no DSN option disables it.
- The SQLite adapter's readPage (data grid path) avoids the time coercion by wrapping every selected column in CASE WHEN typeof(col) = 'text' THEN col || '' ELSE col END, since SQLite reports no declared type for a CASE expression; other storage classes are untouched.
- The SQLite query console cannot apply the CASE rewrite because it runs the user's raw SQL, so a console SELECT of a DATE/DATETIME/TIMESTAMP column reformats the value to strftime('%Y-%m-%d %H:%M:%f') text instead of passing the original bytes through; this is a documented capability trade.

### ClickHouse (P36; Go-native as of P58b M6.4)

- The ClickHouse adapter is Go-native (since P58b M6.4), at apps/kira-studio/internal/adapters/clickhouse/, using plain net/http; it was the last of the five SQL-family kinds migrated to Go.
- The Go ClickHouse adapter has no driver dependency: a hand-rolled HTTP client POSTs the statement text and reads the JSONCompactStringsEachRowWithNamesAndTypes format line by line (names row, types row, then one JSON array of strings or the ᴺᵁᴸᴸ sentinel per data row).
- No Go-side type system is needed for ClickHouse because its own text rendering already covers exotic types (Decimal128, UUID, Array/Tuple/Map, Enum8, big UInt64/Int64) the way the cell editor needs.
- The ClickHouse adapter must set output_format_json_quote_64bit_integers=1 explicitly, because @clickhouse/client's .json() set it invisibly; without it FORMAT JSON catalog queries render UInt64/Int64 as bare numbers, losing precision past 2^53 and failing decode into Go string fields.
- ClickHouse adapter settings travel as URL query parameters on every request, including database=<db>, since there is no persistent client-level session over stateless HTTP.
- Every ClickHouse statement is fully qualified with `db`.`table` because ClickHouse's HTTP interface has no per-request database override.
- For ClickHouse, canUpdate and canDelete are permanently false (caps.go) because a MergeTree PRIMARY KEY is a sparse index over parts, not a unique row key, so no addressable row exists for UPDATE/DELETE.
- The grid's '− row' button and inline cell editing are disabled for ClickHouse connections, with a tooltip naming the reason.
- ClickHouse query cancellation uses KILL QUERY WHERE query_id = '<id>' SYNC on a second HTTP request (net/http's connection pool, sized to always have one free), because the server keeps executing a query after the original socket closes.

### Kafka (Go-native as of P58e M9.3)

- The Kafka adapter is Go-native (apps/kira-studio/internal/adapters/kafka/), built on github.com/twmb/franz-go and franz-go/pkg/kadm; it was the tenth and last of ten kinds to go Go-native (P58e M9.3).
- Checkpoint C2 (P58 zero-traffic proof) was recorded with the Kafka flip: a full manual pass across all ten kinds plus cancel/settings-save/cache-clear left the Node engine child's connection-count metric at zero.
- franz-go replaces the old TypeScript driver's ABI-specific compiled binding with a pure-Go client, so there is no compiled binding, ABI, rebuild step, or packaged-bundle native-module gap.
- The Kafka adapter never joins a consumer group for a read-only browse; it uses kgo.ConsumePartitions to assign partitions directly at explicit start offsets, never subscribe() or kgo.ConsumerGroup.
- The browse client is fresh and ephemeral per read, never the adapter's long-lived admin-and-produce client, because start offsets are a construction-time option.
- End-of-log detection in Kafka browse is a per-fetch HighWatermark comparison captured from the fetch that delivered the last record (not a follow-up peek poll), with a fixed-count empty-poll counter as a second independent terminator.
- Kafka key, value and header bytes that are not valid UTF-8 are base64-encoded and flagged in the chunk's binary row list, never passed through strings.ToValidUTF8.
- A topic's Configuration section has real rows via kadm.DescribeTopicConfigs, and ConnectInfo.details.cluster reports a real cluster id via kadm.Metadata.
- The group definition's CLASSIC-vs-KIP-848 type row was dropped because kadm has no source for it; partitionAssignor merges into a protocol row that already carries the same value.
- Idempotent producing is explicitly disabled with kgo.DisableIdempotentWrite() to match the old driver's default and avoid an InitProducerId hang on a single-broker cluster whose transaction-log replication factor is Kafka's default of 3.
- The Kafka produce command-preview reads 'ProduceSync <topic> key=<key>' (key=<none> when the key is null), matching the real kgo.ProduceSync call (produce.go's preview).
- Kafka cancellation has no server-side kill because the protocol has none, so the op's own context.Context on every kadm/kgo call is the entire mechanism.
- kgo.Client.PollRecords returns Fetches, not an error; a cancelled context surfaces as an injected fake fetch carrying ctx.Err() (Fetches.Err()), so the browse loop must check that explicitly.
- For Kafka, canDelete is permanently false because a topic's log is immutable, so no per-message delete or update exists at the protocol level, only retention/compaction.

### SQS (Go-native as of P58d M8.2)

- The SQS adapter is Go-native as of P58d M8.2, in apps/kira-studio/internal/adapters/sqs/, using aws-sdk-go-v2/service/sqs; it is the eighth of ten kinds P58 migrates to Go.
- The SQS adapter sets caps.pagination = 'batch': every poll is an independent, non-resumable ReceiveMessage call with no addressable position, so the stream view never auto-loads and only an explicit Poll button polls.
- In the SQS adapter, canDelete is a real per-item removal via the message's receipt handle, kept adapter-local and in-memory and never round-tripped over the wire, because a receipt handle is only valid for the message it names, on the session that received it.
- The Go SQS port needs an explicit mutex plus a FIFO eviction queue for receipt handles, because the JavaScript original relied on single-threadedness and Map insertion order.
- The SQS adapter has no canUpdate, because a delivered message can't be edited in place, only replaced by delete + resend.
- A read-only SQS connection's poll hides messages only for its chosen time and keeps no receipt handles.
- fetchQueueAttributes reads VisibilityTimeout and RedrivePolicy in one GetQueueAttributes call; maxReceiveCount comes from the redrive JSON.
- Neither the SQS nor the S3 adapter has a server-side kill mechanism; both pass the op's own context.Context directly to every AWS SDK call instead of using adapters.RunWithAbortRace, since a cancelled context aborts an in-flight request through the SDK, confirmed against LocalStack.
- Never use the shared adapters.RunWithAbortRace helper in the SQS and S3 adapters, because a cancelled ReceiveMessage would still complete and hide messages via VisibilityTimeout, and a cancelled DownloadObject would keep writing its temp file after the caller's cleanup ran.
- Build the SQS headers cell with a hand-written encoder over MessageAttributeValue, never a plain json.Marshal of the SDK struct, because that emits every absent field (BinaryValue, StringListValues, ...) as explicit null, which the JavaScript original's JSON.stringify(message.MessageAttributes ?? {}) never produced.
- A nonexistent named AWS profile now fails at connect time, because config.LoadDefaultConfig returns config.SharedConfigProfileNotExistError directly, rather than at first use; the Test button thus reports it sooner.

### S3 (Go-native as of P58d M8.3)

- The S3 adapter is Go-native (apps/kira-studio/internal/adapters/s3/, aws-sdk-go-v2/service/s3).
- S3 is the only engine with caps.fileTransfer: items are whole files streamed to/from a local path via a native OS dialog (downloadObject), not a value the mutation-preview model can show inline.
- fileTransfer is orthogonal to the three write flags: Download reads regardless of a connection's read-only flag, while Upload is gated on fileTransfer && canInsert together.
- S3 sets caps.keyBrowser = true (P41) because a bucket's prefix/object space has no fixed size and can nest arbitrarily deep; the bucket node in s3/catalog.go is hasChildren: false, and /-delimited prefix/object navigation happens in a Browse tab (SPEC.md §8.18).
- S3 object node paths carry the full bucket-relative key on their own object segment, nested under every ancestor prefix segment (e.g. bucket:b/prefix:reports/object:reports%2Fnote.txt), not a bare local filename.
- DownloadObject writes to a sibling .kira-partial-<uuid> temp file and renames on success, unlinking it on any failure or cancellation.
- DownloadObject's temp-file cleanup is safe only because the copy runs on the caller's own goroutine, with the op's own context passed directly into io.Copy via the SDK call (no detached context, as SQS does), so cancelled-download cleanup never races a background writer left by RunWithAbortRace.
- PutObject's insert path detects a collision structurally on *types.NotFound rather than a mapped error code, a narrowing from the original TypeScript which treated any query-level error as probably not found and proceeded.
- S3 object metadata keys come back lowercased from HeadObject/GetObject regardless of the case sent (real S3/LocalStack x-amz-meta-* behavior), not a casing bug in the adapter.
- RabbitMQ was dropped from v1's scope (P58 findings) before being ported to Go, because it was Node-served-only via an HTTP-management-API adapter with no AMQP client; connectionKindSchema/nodeKindSchema no longer carry rabbitmq/exchange.

### MongoDB / Redis (P9/P41; Go-native as of P58c M7.3/M7.4)

- The MongoDB and Redis adapters are Go-native as of P58c, located in apps/kira-studio/internal/adapters/mongo/ and .../redis/.
- The MongoDB adapter paginates by _id keyset, falling back to skip/limit.
- MongoDB cancellation uses $currentOp plus killOp on the same client the adapter already holds, never a side connection, because $currentOp's default allUsers: false only matches the polling connection's own authenticated user.
- The Redis adapter paginates with SCAN cursors (never KEYS) and gets exact per-key counts via O(1) type-length commands.
- Redis Cancel is permanently false because go-redis's blocking commands override the caller's context for the wait; CheckCancelled between bounded SCAN-family rounds is the whole cancellation surface, and caps.cancel stays true since that is effective.
- mongo/literal.go, the JSON5-lite tokenizer/BSON-constructor parser used by every Mongo filter/document/console surface, was written and unit-tested alone first (P58 D11), without driver or container.
- IDText (a document's _id as canonical extended JSON text, used for the ids column, Copy _id and page tokens) and literal.go's parser must form a closed loop: whatever IDText emits, the parser must accept back via mutate.go's parseIdKey (bare value) and a filter box (wrapped as {_id: ...}).
- bson.MarshalExtJSON/UnmarshalExtJSON cannot encode or decode a bare top-level scalar, only a document, so both directions need the value wrapped in a one-field document first.
- Redis is a caps.keyBrowser = true engine (P41, alongside S3); redis/catalog.go's database node is hasChildren: false because a db index's key namespace is unbounded, and :-split namespace/key navigation happens in a Browse tab (SPEC.md §8.18).
- Redis's DbConnectionSet is a mysql-family-style LRU with an 8-entry cap, keyed by db index rather than database name.
- go-redis Protocol: 2 is pinned explicitly because the RESP3 default changes HGETALL/CONFIG GET and console generic dispatch replies from a flat array to a map.
- The Redis console runs bounded forms only of commands that can return unbounded replies; other commands (SUNION, SINTER, SDIFF, EVAL/FCALL, module commands, GET of a huge string) are read whole by go-redis before the page cap applies.

### Kibana (P424)

- Talks to the Kibana HTTP API only; read-only kind. A data view is a leaf `collection` tree node; its path segment is the data view id. A connection `database` value names a Kibana space (non-default spaces prefix every route with `/s/<space>`).
- Routes: `GET /api/status` (version; absent for low-privilege users on 9.x, then the 8.19 floor is unchecked), `GET /api/spaces/space/{id}`, `GET /api/data_views`, `GET /api/data_views/data_view/{id}`, `POST /internal/search/es`. Every request sends `kbn-xsrf`; internal routes also send `elastic-api-version: 1` and `x-elastic-internal-origin: Kibana`.
- Auth: username plus password is Basic; blank username with a password is `Authorization: ApiKey` (encoded key); both blank sends no header. `/api/status` answers anyone, so listing data views is what proves credentials (401 and 403 map to `E_AUTH`).
- Execute statement is one JSON `KibanaDiscoverRequest` (`dataView`, `query`, `language`, `timeRange`, `filters`, `sort`, `size`, `timeZone`), decoded with `DisallowUnknownFields`. One statement per run; `_id` and `_index` merge into each hit's document body. Results are `document` pages. Tab and operations-panel re-run send the raw JSON, never split on `;`.
- KQL compiles to Elasticsearch query DSL in Go (`adapters/kibana/kql`). Hand-rolled: no Go KQL library exists and the Kibana parser is TypeScript under a non-OSI license. Lucene syntax goes through `query_string`.
- Histogram interval: `auto` picks the nearest of the standard intervals that yields at most 100 and closest to 50 buckets; `1w`, `1M`, `1y` are calendar intervals. The response reports the interval used and whether it was scaled up. Value suggestions are a `terms` aggregation over the current query and time range; `Complete` is false when more distinct values exist than returned.
- Optional `columns` on the request switches Execute to a `tabular` page: `fields` API with `_source:false`, time field first, nested values read from the nested group. Unknown or duplicate column fails the run. Empty `columns` keeps the `document` page.
- Saved Discover queries reuse `saved_queries` with kind `discover` (path `''`, per connection). Partial unique index `saved_queries_discover_name` on `(connection_id, lower(name))` enforces one name per connection; SQLite `lower` folds ASCII only, so `É` and `é` are distinct names. Body is `model.DiscoverBody` (query, dataViewId, dataViewName, language, time, filters, columns, sort, sampleSize, interval; 256 KiB cap). A preset time stays relative. Update is last-write-wins: no version check, a second window overwrites.
- ES|QL and saved objects are not supported. Fixture: Elasticsearch plus Kibana 9.5.3 (floor 8.19), about 1.5 GB of memory (see DEV_ENVIRONMENT.md).

### `sslmode` semantics per engine (P21 round 1)

- Options["sslmode"] is the app's own vocabulary layered over five different native TLS conventions, so the same string means different things depending on the engine.
- Postgres and MySQL/MariaDB follow their native client convention (libpq / MySQL --ssl-mode): require/prefer encrypt without verifying the server certificate, verify-full verifies; this asymmetry is kept on purpose.
- For Redis, MongoDB and Kafka, sslmode is the app's own invented option with no native precedent, so require/prefer/verify-full all verify the server certificate; verify-none (or insecure) is the escape hatch for a non-verifying connection.
- sslmode disable and an absent sslmode option mean plaintext for every engine.
- A rediss:// URI implies verify-full even with no sslmode option set, matching the standard meaning of that scheme.
- For mysql:// and mariadb:// URIs only the sslmode query parameter is parsed; any other query parameter is dropped with a warn log instead of being forwarded, because go-sql-driver's Params field executes as a literal SET k = v statement at connect time.
- adapters.ParseSSLMode reads Options["sslmode"], applies the empty/disable convention, and fails loudly on a value outside the adapter's accepted set; each adapter keeps its own tls.Config construction.
- Kafka accepts verify-none and insecure, using kgo.DialTLSConfig with InsecureSkipVerify set from those values, matching Redis; previously it rejected them, making brokers with self-signed or internal-CA certificates unreachable.
- ClickHouse's resolveTarget maps sslmode prefer to https, same as require/verify-full, because ClickHouse has no plaintext-with-opportunistic-upgrade transport.
- Postgres verify-ca maps to InsecureSkipVerify true plus a VerifyPeerCertificate that chains against the system roots without checking the hostname; it was previously rejected even in URI mode.
- libpq's sslmode allow is intentionally unimplemented, because the app resolves TLS config once before any connection attempt with no retry-on-refusal path; real support would need a connection-flow change.

## Storage

- ~/.kira-studio/ (mode 0700) contains kira.db (0600), logs/, and the DB MCP server's token mcp-db-token.json (one per KIRA_HOME, no slug).
- As of P100, ~/.kira-studio/ holds no git state; review.db moved to Kira Space's own ~/.kira-space/ under KIRA_SPACE_HOME.
- Credentials in the connections table's password column are encrypted at rest (P25) from Go, using a single symmetric key in the OS keychain for all values, with ciphertext in the app's own database.
- A single keychain key was kept because one keychain item means one authorization decision rather than a prompt per connection, item-count and item-size limits become irrelevant, and SecretStore.copy() stays a raw column copy that never needs the OS key.
- The encryption key is 32 random bytes stored as one generic-password keychain item via github.com/keybase/go-keychain (service 'Kira Studio Secrets', account 'Kira Studio'); values are sealed with AES-256-GCM in a kira:v3:<base64> envelope.
- The keychain item uses kSecAttrAccessibleWhenUnlockedThisDeviceOnly so the key is never restorable from a backup onto another machine, and is non-synchronizable so it never reaches iCloud Keychain.
- secrets/keyring_darwin.go is the only file that touches the keychain library and secrets/cipher.go is the only file that encrypts or decrypts, keeping a future re-key or cross-platform secret store a contained change.
- The keychain service name was 'Kira Studio Safe Storage' through P52-P57 (kept stable in P57 D12 across the Electron-to-Wails rename to avoid orphaning keys) and was renamed to 'Kira Studio Secrets' because 'Safe Storage' is Chromium/Electron naming and misleading.
- The keychain service rename was free only because the app has not shipped; the OS looks the item up by service name, so once real users exist a rename needs a migration (read old service name once, re-write under the new one, then stop looking).
- The envelope bumped from kira:v1: to kira:v2: because the cipher changed (AES-256-GCM under the app's own key versus Chromium's AES-128-CBC under safeStorage); reusing v1 would hand a v1 value to a v2 decrypt and fail confusingly.
- The pre-P25 plaintext passthrough was dropped: Decrypt of a non-enveloped value returns an E_SECRET_STORE error naming the problem instead of returning it verbatim; upgradeLegacySecrets was deleted because it only upgraded pre-P25 rows.
- AES-GCM's authentication tag makes a tampered or truncated ciphertext fail to authenticate rather than decrypt to garbage; the user-facing decrypt-failure message ('may have been written on a different machine or after a keychain reset — re-enter it to fix this connection') stays verbatim.
- Encrypt/Decrypt take a secrets.Scope ("connection", "connection-uri", "variable", "variable-history", "mask-key" or "script-param", one per ciphertext column) used as AES-GCM additional authenticated data, so a ciphertext sealed for one column refuses to authenticate in another (P29).
- The AAD scope is bound to kind, not to a row, because duplication paths (ConnectionsRepo.InsertDuplicateWithSecret, VariablesRepo.DuplicateEnvironment) copy raw ciphertext into a new row of the same kind without using the cipher, which row-scoped AAD would break.
- VariablesRepo.recordHistory re-encrypts a live variable's value under ScopeVariableHistory using the plaintext it already holds, rather than raw-copying ciphertext, because a raw copy would put a live-variable ciphertext in a history row (cross-kind movement the AAD refuses).
- The envelope bumped from kira:v2: to kira:v3: with no migration and no dual-read path (app has not shipped); every kira:v2: and kira:v1: value is refused on first read and the user re-enters it once.
- P447: internal/secrets lives at repo root, shared by Studio and Space. Both apps use one keychain item ('Kira Studio Secrets'), so one authorization prompt and one key. Space needs `KIRA_INSECURE_SECRETS=1` on Linux, like Studio.
- secrets.Store seals values in table secret_values(scope, owner, name, value, updated_ms), PK (scope, owner, name), in both apps' DB. Callers pass their own Execer so a seal joins their tx. Only secrets.Store reads or writes the table.
- secrets.Masker (stdlib strings.Replacer, longest first) replaces a value, each line of a multi-line value and its JSON-escaped form with the mask. It over-masks rather than under-masks. A nil Masker masks nothing.
- The whole connection URI is ciphertext (P181): connections.uri holds a kira:v3: envelope under scope "connection-uri"; query secrets (password, sslpassword, tlsCertificateKeyFilePassword, proxyPassword) stay inside the blob with no separate slot.
- SecretsRepo owns both ciphertext columns (Get, GetURI); ConnectionsRepo never selects uri, so summaries, List results, connections:list-changed events and IPC fixtures carry uri: null, and connectionSummarySchema types it z.null().
- In URI mode the URI is the single source of truth: Create/Update fold the draft password into the URI's userinfo unless the URI has its own, seal the result and clear the password column; Update with uri: null means unchanged.
- Options derive from the URI query in Go (uriQueryOptions) and are stored as {}; Fields mode refuses an Options key that holds a secret (secretOptionKeys); the dialog shows the URI fully masked and nothing derives display from it.
- There is no migration for P181: a pre-P181 plaintext uri fails to decrypt with the envelope error on Connect, Test and Reveal, stays listed, loses no data, and is fixed by Edit then typing a new URI.
- URI mode needs secret storage even for a URI with no secret (Linux dev: KIRA_INSECURE_SECRETS=1); Show URI and Copy URI go through the P14 reveal gate.
- The secret-storage status {available, backend, insecureFallback, reason} is probed once at startup and never changes for the process life; the connection dialog's credential note reflects it.
- On Linux (dev/CI only; v1 targets macOS only) there is no real keychain: with KIRA_INSECURE_SECRETS=1 it falls back to obfuscation under a hardcoded compile-time key (like Chromium's basic_text); without it secret storage is unavailable and a write carrying a password is refused rather than stored in the clear.
- Decrypting a stored credential for display (password field, or URI field in URI mode) is gated (P14): pressing Show password/Show URI triggers the reveal, and internal/localauth confirms the device owner via macOS LAContext.evaluatePolicy(.deviceOwnerAuthentication) before decrypting.
- A successful reveal confirmation grants a 5-minute, process-wide, non-persisted grace window, but each edit dialog still requires its own explicit Show password press.
- Where OS authentication is unavailable (Linux, or a Mac with neither biometry nor a login password), the reveal falls back to the in-app confirmDialog(), which grants that one reveal without recording a grace window.
- Connect, Test, and Duplicate use the stored secret unprompted; the reveal gate only covers turning a secret into visible text.
- The connection dialog (fields mode, non-file kinds) and the tree menu's Update credentials… (saved fields-mode network connection) accept a pasted block of labelled text such as user/pass pairs, PGPASSWORD=…, Key=value; DSN pairs or psql/mysql flags.
- project/credentialPaste/parse.ts parses pasted credentials in the renderer only: the blob never crosses the bridge and never reaches a store, localStorage, the op log or the console; only the resulting fields go through the existing create/update secret path.
- The credential paste panel shows a review (masked password, guessed badge, candidate picker) and applies nothing until Apply; it clears its text and result on apply, cancel and unmount, which drops references but does not zero memory since JS strings are immutable.
- Credential paste does not parse URIs, URLs, JDBC, JSON, YAML or percent-decoding; URIs stay in the Connection URI field.
- Update credentials… writes username and password only, keeps every other field, and reconnects a live connection on a password-only change (Go already reconnects on a username change).
- A collection/environment variable's secret value reveal goes through the same internal/localauth.Authorizer as connection passwords; main.go constructs exactly one and passes it via appwire.Options to both connections.Service and internal/apivars.Service, making the 5-minute grace process-wide.
- Revealing a connection password and then a variable's value within the grace window prompts only once.
- apivars.RevealResult is redeclared rather than imported from connections.RevealResult, because importing Studio's internal/connections from the Api-scoped internal/apivars would violate the SPEC's module boundary; the four-outcome shape (revealed | cancelled | confirmation-required | error) is redeclared on both Go and TypeScript sides.
- Sending a request that substitutes a secret never prompts, because the reveal gate is about display, not use (D9).
- Route any surface that renders a secret variable's substituted value as visible text through the revealVariable/localauth gate, because it counts as a reveal.
- Copy as curl is masked by default (showing {{name}} literally), goes through the revealVariable/localauth gate when un-masked, and never persists what it shows.
- Copy as curl is masked-by-default rather than prompting on open, because prompting would tax requests that reference no secret or users wanting the shareable {{token}} form, the friction (P14) that gets security features switched off.
- Copy as curl is not a new Go entry point: it uses the same bound VariablesService.Reveal call as the variables dialog, and the command string is built renderer-only and never reaches Go, so there is no op_log.command ordering concern.

```
schema_version(version)
settings(key, value)                                   -- fonts, sizes, budgets, toggles
connections(id, name, kind, color, mode, read_only, host, port, database, username, password,
            uri, options_json, preconnect, preconnect_sidecar, preconnect_delay_sec, auto_explain,
            throttle_per_sec,
            created_at, updated_at, sort_order, mcp_enabled, mcp_description, mcp_read_mode,
            mcp_write_mode, mcp_ddl_mode, mcp_auto_explain, mask_correlation_key)
                                                       -- v1.7: mcp_enabled (0/1, default 0, 0020);
                                                       -- mcp_description ('', 0021); mcp_read_mode
                                                       -- ('allow', 0021), mcp_write_mode ('prompt',
                                                       -- 0021), mcp_ddl_mode ('deny', 0021) — the
                                                       -- three independent MCP permission modes;
                                                       -- mcp_auto_explain (1, 0022); mask_correlation_key
                                                       -- ('', 0023) is a kira:v3 envelope under the
                                                       -- mask-key secret scope, read/written only by
                                                       -- MaskKeysRepo, deliberately absent from
                                                       -- ConnectionFields (0023's own header comment);
                                                       -- uri is a kira:v3 envelope, connection-uri
                                                       -- scope (P181), read only by SecretsRepo
connection_tree_filters(connection_id, scope, value)    -- hide/show rules; a set, no id/ordering
                                                       -- (P28 D12) — no per-row size cap: a person
                                                       -- picks these from the tree, doesn't type them
connection_ddl(connection_id, ddl, updated_at)          -- pasted DDL for the SQL language service
connection_mask_rules(id, connection_id, table_name, column_name, mask_kind, keep_hint,
                       correlate, created_at, updated_at)
                                                       -- M5/0023; connection_id ON DELETE CASCADE;
                                                       -- table_name '*' matches any table, matching
                                                       -- itself is by column_name alone; mask_kind is
                                                       -- name|email|text|number|date|redact;
                                                       -- uniqueness is case-insensitive
                                                       -- (lower(table_name), lower(column_name));
                                                       -- correlate is forced 0 for number, enforced in
                                                       -- internal/mask.Apply regardless of the stored
                                                       -- flag. A table, not an options_json key,
                                                       -- because options round-trips through the
                                                       -- connection URI and the Copy URI menu item, so
                                                       -- a privacy rule must not be settable or
                                                       -- clearable by pasting a URI
saved_queries(id, connection_id, path, name, kind, body, pinned, created_at, used_at)
                                                       -- saved filters/queries per table + console
filter_history(id, connection_id, path, where_text, order_by_json, used_at)
                                                       -- history list of past filters/sorts; capped
                                                       -- 20/path, 4,000/connection, 4 KiB/row (P23)
metadata_cache(connection_id, path, kind, payload_json, fetched_at, etag)
op_log(id, connection_id, tab_id, started_at, duration_ms, kind, status, rows,
       command, error, stored_bytes, command_truncated, path, source)
                                                       -- rotated, capped (P23: stored_bytes/
                                                       -- command_truncated added by migration 15);
                                                       -- path (TEXT, migration 0027, P108 Part 11
                                                       -- F5) is the console database/schema an
                                                       -- op actually ran against, NULL for every
                                                       -- non-console-execute op kind and for every
                                                       -- pre-existing row;
                                                       -- source (TEXT, migration 0043, P328) is
                                                       -- 'user' or 'mcp', default 'user'
custom_scripts(id, name, command, working_dir, dir_mode, kind, params_json, smart_json, use_ade_dir, schedule_json,
               color, collection_id, sort_order, created_at, updated_at)
                                                       -- P85, migration 0024; scripts, one row
                                                       -- per script, user-curated (no cap).
                                                       -- dir_mode (P242, Studio 0034 / Space 0026):
                                                       -- 'kira' = <app home>/automations/<id>, 'fixed'
                                                       -- = working_dir, 'home' = legacy $HOME (rows
                                                       -- that predate the choice; never offered anew).
                                                       -- collection_id (P219, Studio 0033 / Space
                                                       -- 0023; replaced 0031's free-text collection)
                                                       -- kind 'script'|'smart' (P242 Part 2, Studio
                                                       -- 0035 / Space 0028), use_ade_dir (Part 3,
                                                       -- 0036 / 0029), schedule_json (Part 4, 0037 /
                                                       -- 0030; '' = not recurring):
                                                       -- NULL = ungrouped, FK to
                                                       -- custom_script_collections ON DELETE CASCADE;
                                                       -- sort_order is dense per collection (NULL =
                                                       -- ungrouped set); Move places the script
                                                       -- before beforeId in the target set, or last
script_runs(id, script_id, script_name, color, kind, trigger_kind, state, terminal_id, cwd, command,
            outcome_json, created_at, started_at, finished_at, model, session_id, prompt, params_json,
            tools_json, task_id, task_title, branch_id, branch_label)
                                                       -- P242 (Studio 0034 / Space 0026, extended by
                                                       -- 0035-0037 / 0028-0030); one row per
                                                       -- Script run, newest 500 kept (finished
                                                       -- rows only are purged). state waiting|running|
                                                       -- done|failed|cancelled|blocked|skipped;
                                                       -- outcome_json is a runoutcome.Outcome.
                                                       -- trigger_kind terminal|manual|ade|scheduled.
                                                       -- model..tools_json describe a smart run (a
                                                       -- secret param is stored masked; saved secrets live
                                                       -- in secret_values, scope script-param); task_*,
                                                       -- branch_* name the ADE context (Space only).
script_run_logs(run_id, seq, stream, text)              -- P242 Part 2; smart-run log lines, stream
                                                       -- stdout|stderr|event
custom_script_collections(id, name, sort_order, created_at, updated_at)
                                                       -- P219; one row per script collection, name
                                                       -- trimmed, 64 runes max, order is sort_order
                                                       -- (user drag order), name. Empty ones may exist.
secret_values(scope, owner, name, value, updated_ms)     -- P447; sealed kira:v3 values, PK (scope, owner, name),
                                                       -- both apps. script-param rows: owner = custom_scripts.id.
ui_layout(key, value)                                   -- panel visibility (app-wide), panel sizes per module (modes.<mode>.*)
windows(key, order, bounds_json)                        -- one row per workbench (P8)
tabs(id, connection_id, path, kind, state_json, order, active, window_key, workspace_id)
                                                       -- session restore, window_key ON DELETE
                                                       -- CASCADE into windows. workspace_id (C5,
                                                       -- migration 0018) used to carry
                                                       -- 'repo:<code_repos.id>' for a tab scoped to
                                                       -- a repository's own workspace; as of P100's
                                                       -- git-module extraction and its
                                                       -- 0026_p100_drop_git_tables.sql, code_repos
                                                       -- is gone from this DB and every remaining
                                                       -- row parses as workspace_id IS NULL -- the
                                                       -- column itself was kept rather than dropped
                                                       -- (no behavioural gain from rewriting the
                                                       -- whole table, and NormalizeMode already
                                                       -- degrades an unrecognised value the same
                                                       -- way), so a pre-P100 DB needs no migration
                                                       -- of its own here
api_collections(id, name, sort_order, origin_json, variables_promoted,
                 created_at, updated_at)                -- P4; variables_promoted added P5;
                                                       -- sort_order is the user's order, set by
                                                       -- ReorderCollections (P381)
api_items(id, collection_id, parent_id, kind, name, sort_order, method, url, protocol,
           request_json, origin_json, created_at, updated_at)
                                                       -- the folder/request tree; parent_id and
                                                       -- collection_id both ON DELETE CASCADE.
                                                       -- protocol ('http'|'grpc', default 'http',
                                                       -- P11) added by ALTER TABLE, not a new
                                                       -- migration's table — see below
api_environments(id, name, sort_order, is_active, created_at, updated_at)  -- P5; top-level,
                                                       -- no collection_id; at most one is_active
api_variables(id, collection_id, environment_id, name, value, is_secret, secret_value,
               sort_order, created_at, updated_at)      -- P5; owned by exactly one of
                                                       -- collection_id/environment_id (CHECK);
                                                       -- value/secret_value are mutually exclusive
api_variable_history(id, variable_id, value, is_secret, secret_value, recorded_at)
                                                       -- P5; per-entry, capped at 20, variable_id
                                                       -- ON DELETE CASCADE
api_response_history(id, item_id, tab_id, scope_key, sent_at, method, url, environment,
                       status, status_text, elapsed_ms, body_bytes, stored_bytes, snapshot_json)
                                                       -- P8; one row per response actually
                                                       -- received. scope_key is GENERATED ALWAYS
                                                       -- AS (COALESCE(item_id, 'tab:'||tab_id))
                                                       -- VIRTUAL. item_id ON DELETE CASCADE
                                                       -- (real FK into api_items); tab_id is
                                                       -- deliberately NOT a foreign key into
                                                       -- tabs (below)
grpc_call_history(id, item_id, tab_id, scope_key, called_at, target, method, streaming,
                   code, code_name, status_message, elapsed_ms, message_count, message_bytes,
                   stored_bytes, snapshot_json)
                                                       -- P11; api_response_history's own shape,
                                                       -- reused verbatim down to the generated
                                                       -- scope_key and the ON DELETE CASCADE /
                                                       -- deliberately-not-a-foreign-key split
                                                       -- between item_id and tab_id (below). One
                                                       -- row per *completed* call — unary or
                                                       -- streaming, cancelled-with-partial-
                                                       -- messages counts as completed (D11)
```

- P100 dropped git_clients, git_repo_settings and code_repos from Studio's kira.db (0026_p100_drop_git_tables.sql) when the git module was extracted into Kira Space, a separate app with its own kira.db and its own copies of these tables.
- Kira Space runs its own migration sequence in apps/kira-space/internal/storage/migrations/, separate from Studio's, with high-water mark 0045 (0045_p332_file_handler_previous.sql adds file_handler_previous).
- Studio's kira.db migration high-water mark is 0037 (0037_p242_recurring_scripts.sql: custom_scripts.schedule_json, script_runs rebuilt with waiting/skipped states).
- Migration 0020 added connections.mcp_enabled (deny-by-default MCP exposure); 0021 added mcp_read_mode/mcp_write_mode/mcp_ddl_mode plus mcp_description; 0022 added mcp_auto_explain (auto-force-EXPLAIN on run_query); 0023 created connection_mask_rules and connections.mask_correlation_key.
- Migration 0027_p108part11_op_log_path.sql added op_log.path TEXT, the console path an op actually ran against.
- Op source rides the RunOp ctx (`adapterhost.WithOpSource`), set by dbmcp's tool wrapper; connection lifecycle ops are always `user`, since MCP's connect is a shared, deduplicated attempt with its own ctx.
- Migrations are forward-only numbered SQL files in apps/kira-studio/internal/storage/migrations/, applied on startup.
- Table access is hand-written database/sql in apps/kira-studio/internal/storage/repos/; there is no ORM (the Drizzle dependency went out with the Electron shell).
- Every row read from settings, ui_layout and connections is decoded through the model types in internal/storage/model/ before use, so a hand-edited or stale-shape row fails loudly instead of propagating zero values into the UI.
- Migration 0010_p12_api_rename.sql renamed the six http_* collection/variable/history tables to api_*, because api_items stores gRPC requests too and api_variables resolves gRPC targets and metadata; grpc_call_history was not renamed since it holds gRPC calls only.
- The 0010 rename migration uses ALTER TABLE RENAME for each table, then DROP INDEX and CREATE INDEX for each of seven indexes, because SQLite has no ALTER INDEX RENAME; foreign key references are rewritten automatically since db.go sets _foreign_keys=1.
- Already-applied migrations 0006-0009 are left untouched, so a fresh install creates http_* tables and renames them in 0010; migrate.go refuses to run against a schema_version newer than the build knows.
- migrate_rename_test.go is the one test that exercises a migration against seeded v9 data; it asserts rows survive under new names, generated scope_key columns still compute, grpc_call_history's foreign key names api_items, and collection delete still cascades.
- Postman collections are stored in kira.db as a normalized api_collections/api_items tree, not as a .json file edited in place; sort_order is data and is rewritten dense within a parent on any insert or delete.
- api_items.parent_id is a self-reference with ON DELETE CASCADE, enforced because the DSN sets _foreign_keys=1 on every pooled connection, so deleting a folder deletes its subtree at any depth in one statement with no recursive Go delete.
- Each api row stores origin_json, the original Postman object verbatim minus its recursive item array, because encoding/json drops undeclared struct members and no typed model could round-trip unmodelled members.
- Postman export starts from origin_json and rewrites only url, header and body, and only when re-importing the origin's member no longer yields what is stored; method is always written from request_json; other members are re-emitted byte-identically.
- CollectionsRepo.SaveRequest sheds each rewritten member from origin_json so an edited request does not carry a stale duplicate of its body.
- Member order within a JSON object is not preserved on Postman export, because map[string]json.RawMessage re-marshals sorted.
- Postman scripts and auth are preserved inert at every level (never executed or applied), so a request relying on collection-level Bearer auth will 401 when sent until an auth surface is built.
- A collection's top-level variable[] is promoted out of origin_json into api_variables rows, because P5 requires that level to be resolvable; folder-, item- and url.variable-level variables stay inert.
- A variable is one api_variables row owned by either a collection or an environment, never both and never neither, enforced by a CHECK constraint rather than a discriminator column; the two nullable foreign keys carry that fact and give ON DELETE CASCADE.
- Environments are top-level, separate from a collection's own variables, because a scratch request tab (no itemId) belongs to no collection and so an environment-scoped variable set must exist independently.
- At send time a variable reference resolves against the active environment first, then the request's own collection, then nothing.
- A secret's plaintext never leaves api_variables except through a gated reveal; this is guaranteed by schema: value and secret_value are separate columns (value = '' whenever is_secret = 1, secret_value set only then), so the editor's list query cannot return plaintext or ciphertext.
- internal/apivars.Service.Reveal and RevealHistory are the only paths to a secret variable's value, gated by the same *internal/localauth.Authorizer instance that connections' reveal uses, so a 5-minute grace granted by either kind of reveal covers the other.
- Variable history is stored in api_variable_history, one row per prior value, capped at 20 per variable with the same dedupe-then-trim discipline as filter_history.
- Variable history is recorded only when a value actually changes; a secret's change is detected by decrypting the stored value and comparing plaintext, because GCM nonces make ciphertext comparison meaningless.
- A history restore writes through the ordinary update path, so the restore is itself recorded in history.
- A pre-P5 collection (variables_promoted = 0) has its variables promoted once, lazily, the first time they are listed.
- Export re-emits variable from api_variables rows rather than origin_json, and a secret's value is always "" in the export, because exported files get shared or committed and writing a decrypted credential would defeat masking.

- Response history is recorded in Go inside the existing send op: bridge/http.go's Send closure calls ResponseHistoryRepo.Record after the response is known and before it returns, writing one api_response_history row per response received.
- Response history recording is best-effort: a failed insert logs and the send still returns its response, because a history feature must never be the reason a user loses the answer.
- Response history stores the stage-1 request args, not ResolveRequest's output, so a secret name stays spelled {{name}} and never its decrypted value; this adds no secret exposure beyond what tabs.state_json already had.
- Response bodies (e.g. a login endpoint's token) are persisted in the clear in response history, a stated decision matching comparable tools.
- Response history storage is bounded by three independent caps: a 256 KiB per-entry cap, a per-scope count cap of 20, and a global byte budget of 128 MiB.
- The 256 KiB per-entry cap applies separately to the request body and the response body, truncating the stored copy independently of internal/httpclient's 10 MiB transfer cap, with two independent truncation booleans so an entry can report both, either, or neither.
- A binary response body (bodyEncoding base64) is not stored in response history; the entry keeps every other field including the true bodyBytes, and the raw inspector likewise elides binary bodies with a '[… N bytes of binary data …]' marker.
- The per-scope count cap of 20 uses the same insert-then-trim SQL shape as filter_history and api_variable_history.
- The 128 MiB global byte budget evicts oldest-first across every scope via one window-function DELETE (SUM(stored_bytes) OVER) inside Record's transaction on every insert; the budget is the same order of magnitude as cache.l2BudgetMb.
- The per-entry cap guarantees no single row exceeds the byte budget, so the row just inserted is never evicted by the global budget.
- Response history has no time-based expiry, because a response row is a result the user asked for and a two-month-old response is not noise, unlike op_log.
- Response.Wire is nilled before Record marshals the snapshot, but Response.Timeline is deliberately kept, since a timeline is small (about half a KB without redirects, around 90 KB for a 10-hop chain) and fits the existing byte budget with no new cap or schema change.
- For a stored response, the Timeline pane shows the real recorded timing while the Raw pane shows the 'no raw view for a stored response' empty state; the two panes intentionally differ for the same entry.
- scope_key is the single axis List/trim/Clear key on (the saved request item_id, else 'tab:'+tab_id), implemented as a GENERATED ALWAYS AS (COALESCE(item_id, 'tab:' || tab_id)) VIRTUAL column, to avoid two writers (Record and Adopt) needing to agree and to keep the index usable without per-call-shape SQL.
- api_response_history.item_id cascades on delete (the DSN sets _foreign_keys=1 on every connection), so deleting a saved request, or its folder or collection, removes its history in one statement.
- api_response_history.tab_id is deliberately not a foreign key into tabs, because TabsRepo.Save deletes and re-inserts a window's whole tab set on a 1-second debounce firing on every URL keystroke, so ON DELETE CASCADE would erase scratch-tab history.
- Scratch-tab response history is swept once per launch by ResponseHistoryRepo.SweepOrphans (called from apps/kira-studio/internal/appwire/wire.go beside oplog's startup prune), deleting rows with item_id NULL whose tab_id is not in tabs.
- Orphan sweeping runs only at launch, not on tab close, deliberately: residue is bounded by the global byte budget, and a call on closeTab could fail silently during a quit.
- Save as… adopts a scratch tab's history onto the new saved request via ResponseHistoryRepo.Adopt (UPDATE api_response_history SET item_id = ? WHERE item_id IS NULL AND tab_id = ?); scope_key follows because it is generated.
- There is no reverse of Adopt: deleting a saved request cascades its history away and does not orphan it back to the tab, since deletion is already an explicit destructive action.
- An existing installation may carry an inert advanced.engineMemoryCapMb settings row left from before P58f M10 removed the setting; nothing reads it and no migration deletes it, to avoid migration-ordering risk.
- ui_layout's window.bounds leaf is left in place as an inert row after P8's 0002_p8_windows.sql seeds the first windows row from it.
- The git module's incremental-review state lives in a separate SQLite file, review.db, under ${KIRA_SPACE_HOME} (Kira Space's file since P100), because of bulk blob content and TTL purges that should reclaim space without locking the main database.
- review.db is neither created nor opened until the first request needing it, so an instance that never serves a review request never creates the file or starts its reaper.
- review.db has six tables: review_session (one per repo_id+branch, last_used_at indexed for the reaper), review_file, review_range (1-based inclusive line ranges in the snapshot's coordinates), review_comment (anchored by commit sha and path blob oid), and review_working_file and review_working_range (the uncommitted review, gitreview 0005).
- review_file has one row per reviewed path with reviewed-at sha, git blob oid, content_kind (text/binary/tooLarge/absent), uncompressed length, and for text the content stored flate-compressed as a BLOB.
- Review sessions are purged after 14 days idle; returning after that starts clean by design, not as an error.
- Incremental review diffs with plain git diff when the last reviewed sha is an ancestor of HEAD (checked via git merge-base --is-ancestor), and then never reads the stored blob.
- After a rebase, squash or amend (last reviewed sha not an ancestor or gone), review writes the stored content to a temp file, resolves current content via cat-file and diffs with git diff --no-index, reusing gitclient's spawn discipline instead of a Go diff algorithm.

- A gRPC request is stored as a protocol value on the existing api_items row, not as a third kind value (P11).
- kind stays structural ('folder' vs. a leaf); protocol says which document shape request_json holds for a leaf: 'http' → model.SavedRequest, 'grpc' → model.SavedGrpcRequest.
- A kind: 'grpc-request' value was rejected because every existing kind === 'request' check across tree/collections/export code would need re-auditing for a third value; with protocol those checks stay as narrow as before.
- grpc_call_history is its own table rather than a widened api_response_history, because the rows differ (gRPC code/code_name/streaming vs HTTP status/redirects) and a nullable-column union would make HTTP-only queries and cap computations reason about rows they never see.
- grpc_call_history shares column names with api_response_history by convention, not via a shared struct or shared repo.
- grpc_call_history uses the same four storage bounds as api_response_history: per-message truncation at 64 KiB (half HTTP's single-payload cap), a 100-stored-message-per-entry elision with a true message_count kept, a 30-per-scope trim, and a global byte budget of 32 MiB (a quarter of HTTP's 128 MiB).
- The 100-message count cap exists because a streaming call's messages have no natural single-payload bound to truncate to, so a count cap does the job a byte cap did for HTTP (D11).
- The gRPC history 32 MiB budget (a quarter of HTTP's) reflects gRPC being a newer, narrower-audience protocol, not that a gRPC call matters less.
- Adopt/SweepOrphans and the generated scope_key for gRPC history are ports of ResponseHistoryRepo's mechanism, unchanged in shape.
- gRPC's response pane has three segments (Messages, Metadata, History) and deliberately has no Raw pane and no Timeline pane; RawExchangePane.vue and TimelinePane.vue are not inherited (P11 decision resolving P9 OQ-9 and P10 OQ-8).
- No Raw pane for gRPC because its raw view would be HTTP/2 DATA frames of length-prefixed protobuf, a transport debugging tool, while the Messages segment (protojson, exactly what was sent and received) is already the faithful view.
- No Timeline pane for gRPC because a server-streaming call is one connection with many messages over time, which the Messages segment's arrival offsets (offsetMs, D8) already render; the HTTP timeline models redirect hops.
- S3 connections reuse the connections columns like SQS fields mode: host/port unused, database holds the AWS region, username holds the AWS named profile.
- S3 static keys are accepted only in URI mode and stored encrypted in the uri column (P181).
- S3 connection options_json holds two independent overrides: endpoint (non-AWS S3-compatible target such as LocalStack or MinIO) and bucket (scopes the tree to one bucket via HeadBucketCommand instead of ListBucketsCommand, for IAM credentials that cannot s3:ListAllMyBuckets).
- SQLite connections reuse the connections columns: database holds the absolute file path, and host/port/username/password are unused since there is no server or credential.
- Every table in kira.db, and the file itself, is growth-bounded (P23): an audit of the nineteen tables then existing found seventeen already bounded, and two unbounded, each with a count cap but no byte cap, where a column holding arbitrary user text had no ceiling.

| Table | Grows with | Bound |
|---|---|---|
| `schema_version` | nothing | one row |
| `settings`, `ui_layout` | nothing | closed key set; every writer is a hand-listed leaf |
| `connections`, `connection_ddl`, `connection_tree_filters`, `saved_queries`, `connection_mask_rules` | user action | user action; all cascade on connection delete |
| `custom_scripts`, `custom_script_collections` | user action | user action; one row per script or collection, created and deleted one at a time; deleting a collection deletes its scripts |
| `secret_values` | save of a secret script param | user clear, param removal, script or collection delete (same tx); one row per saved value |
| `api_collections`, `api_items`, `api_environments`, `api_variables` | user action / import | user action; import capped at 64 MiB upstream |
| `tabs` | open tabs | rewritten per window per save; cascades on window close |
| `windows` | live windows | one row per live window (+1 for session restore) |
| `metadata_cache` | browsing | 4 MiB/row, 200 rows/connection, dropped whole on every reconnect |
| `api_variable_history` | value edits | 20 per variable |
| `filter_history` | filter/sort use | 20 per path, **4,000 per connection**, **4 KiB per row** |
| `api_response_history` | sends | 256 KiB/body, 30/scope, 128 MiB table, orphan sweep at launch |
| `grpc_call_history` | calls | 64 KiB/msg, 100 msgs/entry, 30/scope, 32 MiB table, orphan sweep |
| `op_log` | every DB operation | 30 days, 20,000 rows, **64 KiB command + 8 KiB error**, **32 MiB table** |
| the file itself | — | **`auto_vacuum=INCREMENTAL` on new databases + a startup `incremental_vacuum` above 16 MiB of freelist** |
| `kira.db-wal` | one transaction | **`journal_size_limit` = 4 MiB** |
| `logs/` | one file per day | 30 days by mtime (`logging.Sweep`) |

- A new table whose rows are written by machinery needs both a count bound and a byte bound; a table whose rows are written by a person needs a cascade to whatever they created.
- git_clients, git_repo_settings and code_repos were dropped from kira.db by P100's 0026_p100_drop_git_tables.sql; Kira Space's own kira.db carries equivalent tables under its own growth-bound rules.
- OpsRepo.Finish truncates op_log command to 64 KiB and error to 8 KiB, because they were unbounded TEXT and a pasted batch script was stored whole on every re-run.
- op_log Prune has a third pass: a 32 MiB table-wide byte sweep, oldest-first, gated behind an indexed SUM(stored_bytes), in addition to the 20,000-row cap and 30-day retention.
- op_log command truncation is flagged (command_truncated) and disables the Operations panel's Re-run for that row, because Re-run replays command verbatim; error truncation is silent since nothing acts on error.
- filter_history is capped at 20 rows per (connection_id, path) and at historyPerConnectionLimit of 4,000 rows per connection (200 cached paths times 20 rows, matching metadata_cache), evicting by used_at across the whole connection.
- filter_history where_text/order_by_json are silently truncated to 4 KiB per row, because selecting a history entry only populates the filter box for the user to review and never executes it blindly.
- db.go buildDSN sets _auto_vacuum=INCREMENTAL, which takes effect only on a brand-new database and is inert on an existing one, so it needs no version gate; the previous default NONE meant kira.db never shrank after deletes. INCREMENTAL was chosen over FULL because FULL reorganises pages on every commit, a constant cost for a single-writer store that commits on every debounced save.
- repos.Maintenance.Reclaim runs once at startup and runs PRAGMA incremental_vacuum only when the freelist exceeds 16 MiB (half of op_log's table budget).
- Converting an existing kira.db to incremental auto_vacuum is out of scope because there is no installed base; a developer who wants it runs one VACUUM by hand after setting the pragma.
- buildDSN sets journal_size_limit(4194304) so kira.db-wal truncates back to 4 MiB after a commit; 4 MiB equals the default WAL auto-checkpoint interval (1,000 pages at 4 KiB), so a normal WAL is never truncated.
- DataGrip import (internal/datagrip) reads a DataGrip project folder and creates connections only through internal/connections.Service.Create, never using its own or a parallel secret store.
- DataGrip's dataSources.xml and dataSources.local.xml never contain a password, except that a configured-by-url source's JDBC URL can carry user:password@ in its userinfo; the real password lives in IntelliJ PasswordSafe under the service name 'IntelliJ Platform DB — <uuid>' (U+2014 EM DASH).
- DataGrip import supports only the macOS Keychain backend; file-based KEEPASS support was removed.
- DataGrip import refuses by name rather than guessing: Linux (needs libsecret over cgo, which this repo's Go avoids), Windows (app not shipped there), and projects with security.xml PROVIDER=KEEPASS, reported as credential-store-unsupported.
- internal/datagrip.Scan never touches a credential store: it parses both XML files, maps engine (DataGrip's detected DBMS, then driver-ref family, then JDBC URL scheme, first hit wins), and classifies password outlook from file reads, including security.xml PROVIDER.
- Only Apply, run at the user's explicit confirmation, queries the keychain, because macOS raises an authorization prompt per item; scanning first and fetching once at apply time avoids an eight-connection import costing eight avoidable prompts.
- The decrypted DataGrip password exists only inside Apply's call stack, never in the preview, import report, logs or bridge payload; bridge.DataGripService sends only the project path across the bridge.

## Caching

- L1 metadata cache is persisted in `metadata_cache`, survives restart, and has no TTL; staleness is defined by connection epoch (`ConnectionState.Since`), not a clock (P24).
- An L1 payload is fresh if written after the connection was most recently established, otherwise stale; a stale payload is bypassed and re-fetched on its first read per kind, and not re-fetched again for the rest of that connection session.
- The L1 epoch is compared against a per-kind timestamp stored in `payload_json` under the reserved key `fetchedAt`; the `fetched_at` column only orders the 200-row eviction.
- While a connection is not connected no epoch exists, so the L1 cache serves at any age, making the panel instant on launch and letting a SQL console over a cached container offer completion without a live connection.
- Connecting no longer deletes the connection's L1 rows (it did through P23); it moves the epoch so rows are re-read lazily, once each, only for paths a user opens.
- An explicit Refresh still deletes L1 rows: a node's Refresh drops that node's whole row (all four kinds), and a connection's Refresh or Refresh all drops every row for that connection.
- A fourth L1 kind, `columns` (SchemaColumns: every relation in a container with its columns, one round trip), shares the L1 row, keyed on the container (database/schema node); `metadata_cache`'s unique index is (connection_id, path), not including kind.
- `columns` is merged into one row per container rather than one row per relation, because per-relation rows would compete with the tree's `children` rows for the 200-row-per-connection budget on a large schema.
- `columns` follows the same connection-epoch freshness rule as other kinds, is refreshed on explicit Invalidate, and is readable with no live connection because a cache hit is served before the connection is checked.
- The `columns` cache backs SQL console schema-aware completion/diagnostics/hover (`state/schemaColumns.ts`) automatically when a console or data tab opens over a cached container; the completion layer never opens a connection or fetches on its own.
- L1 freshness is tracked per kind inside the shared row (P24 D3): refreshing a container's `children` does not freshen its `columns`, and vice versa, which is why the per-kind `fetchedAt` map exists.
- L2 result-page cache is an in-memory LRU in the engine, byte-budgeted (default 64 MB, configurable), keyed by hash of {connectionId, path, filter, projection, sort, pageSize, pageToken}, never persisted.
- L2 is invalidated by manual refresh, any local mutation on the same target, and disconnect.
- L3 counts cache maps {connectionId, path, filter} to {count, at} with a 5 min TTL, and is marked stale (shown greyed with a refresh affordance) immediately after any local mutation.
- Counts compute only on explicit user request, never automatically, because they are the most expensive read in the app.
- No speculative fetching: a page loads only in response to a user action (Next/Previous, filter/sort/projection change, Refresh, Count button); background prefetch and automatic count-on-open were removed by user request as unwanted background work, not kept as an opt-out setting.
- The settings Cache pane shows usage, hit rate and a Clear caches action.
- The renderer keeps a fourth, deliberately unbudgeted cache tier: five page stores (one per page kind, each holding one loaded page per open tab) plus the parsed-tree cache in `views/shared/document/rows.ts`, released on tab close and pruned to the rendered window on scroll in all five stores (P5 C3).
- The renderer cache has no byte budget by decision: evicting a cold tab's page was declined twice, trading RAM for the ≤ 50 ms cached-tab-switch interaction budget (docs/PERF.md §2.2 lever L-B); ten tabs of large pages means uncapped renderer memory.
- The follow-up for renderer memory (docs/HISTORY.md v1.1 P5 §8 OQ-2) is surfacing the figure next to the Cache pane's usage, not eviction, and is handed to P7.
- The renderer caches bridge-fetched server state with TanStack Query, one QueryClient per window (`packages/workbench/src/state/queryClient.ts`).
- API client data (collections tree, saved HTTP/gRPC requests, variables, environments, active environment) are query entries with staleTime Infinity: never refetched on a timer, only invalidated.
- Go broadcasts `kira:api:dataChanged` on every API-data mutation (`emitApiData` in bridge/apidata.go) with a batch of {kind, ...scope} changes; the boot-time listener maps each to `queryClient.invalidateQueries` so every window refetches only the changed scopes.
- `draftMerge.ts` keeps an unsaved edit in the active window from being clobbered by the invalidation refetch.
- The default `gcTime` is 5 minutes, so a query nobody observes for 5 minutes is dropped and its next mount reads again. `KEEP_WHILE_OPEN` (`gcTime: Infinity`, in `queryClient.ts`) is only for keys with bounded cardinality: a singleton, an app-wide list, or a per-open-tab key removed when that tab closes (gRPC schema, Discover data view and histogram, API history lists, mask rules). Input-keyed transient keys (completion, filter editors, memory search, previews, docker per id) keep the default, since `Infinity` would grow without bound.
- Kira-owned data is `staleTime: Infinity` and invalidated by the write that changes it. `staleTime: 0` or a short `staleTime` stays only for data Kira does not own and cannot observe: the Kibana data view picker list (60 s), CLI tool and agent usage polls, docker polls, git branch and rebase previews, and the stream partition list.
- P175 moved jar cookies (`['httpJarCookies', url]`, fed by a debounced resolved URL), history lists/snapshots (`[domain, itemId, scratchTabId]`, `[domain+'Snapshot', id]`) and gRPC schema (`['grpcSchema', 'proto'|'reflection', ...]`, mirroring Go's descriptor cacheKey) onto TanStack Query.
- Hidden panes refresh lazily via each query's `enabled` flag (P8 D11), and Pinia stores keep UI state only.
- Mutations must call `cancelQueries` after the bridge call and before `setQueryData`, so an in-flight list fetch cannot resurrect a deleted row.
- `internal/httpclient/cookiejar` is a fork of Go 1.27.3's `net/http/cookiejar` (BSD-3 LICENSE kept) because the stdlib jar lists and deletes nothing; it adds `Entries`/`Delete` in `entries.go`, and `Delete` removes by exact domain, path and name.
- On a Go upgrade, re-copy the cookiejar's `jar.go`, `punycode.go` and their tests, then reapply the `ascii` import path change, the only difference from upstream.
- Dotenv parsing (`BulkVariablesEditor`, `dotenv.ts`): an inline comment starts at whitespace plus `#`; a quoted value counts only when the closing quote is followed by nothing or a comment; an unmatched quote yields an unquoted value, never an error.

## UI architecture

- Dialogs use DialogContent with a required size (sm|md|lg|xl|2xl, plus fixed-height), DialogHeader, DialogBody and DialogFooter; the header close button is on by default and closable=false only while a blocking op runs.
- CommandDialog is the one dialog surface exempt from the DialogContent convention and renders reka content directly.
- Use SecondaryTabs (variant tabs|segmented) for secondary tabs and segmented pickers, never raw ToggleGroup/TabsList; primary tab bars keep tabChipVariants.
- Dialog and form controls are size kira-lg (26px); toolbar controls are kira (22px); all use rounded-kira-sm bg-field. font-data is opt-in per control, never part of an InputGroup variant.
- Use SearchField for search and filter boxes and focus it through its exposed focus(), because biome forbids autofocus.
- Use font weights 400/500 only; font-semibold only in PanelHeader and font-bold only for ANSI log data. Icon sizes are 12, 13, 16, 24.
- Buttons use dialog*/toolbar* variants with kira* sizes; the retired shadcn variants and sizes no longer exist.
- Toolbar dividers use ToolbarSeparator (packages/workbench), never a hand-rolled w-px div or a bare vertical Separator; U104 enforces it.
- scripts/check-ui-primitives.sh, part of bun run lint, enforces the UI language rules and holds the named exemptions (CommandPalette, LogsView ANSI bold, mobile 44px touch inputs, git-ui SearchBox two-stage Escape); an exemption that stops matching fails the run.
- The tone vocabulary is ok, warn, error, info (Badge, Alert, StatusDot); neutral is muted text. Chips tint /16, banners /10 with /20 border; app code never writes a tone alpha.
- Error sentences use FieldError and list/popover lines use InlineState; Alert takes icon and strip and no layout classes.
- A boolean that applies on flip is a Switch; an option in a form committed by a button is a Checkbox (14px box).
- Use Tooltip for hints, never native title; the theme provider owns the 400ms delay. Shortcuts use Kbd. A tooltip with several facts uses TooltipDetails (title, label/value rows, hint); a joined sentence of facts is not allowed.
- Loading uses Spinner; Skeleton only where the layout is known.
- Grey has three roles: text-muted-foreground for secondary content, text-subtle for tertiary chrome, text-disabled for absence markers.
- Card inset holds text only, because its bg-field fill hides an Input, Select or Checkbox; Card section holds controls.
- DropdownMenuContent is content-sized (min-w-45, max-w-80).
- Every pane resize handle binds resizeHandleVariants (2px bar, 4px/8px hit area, bg-focus on hover/focus/drag, 10px arrow step, Home/End); shell, dock and ADE panel handles measure identical at rest, hover and drag.
- Status-bar items are Button variant="toolbar" size="kira".
- text-kira-touch is 16px, the iOS focus-zoom floor; h-board-row and h-board-row-tall size the ADE plan board; RadioGroupItem hit area is -inset-1.
- Bind interaction behaviours from their single implementation in packages/workbench/src/behaviour/ (or the theme primitive it names); never re-implement them in app code.
- List and tree keyboard handling uses useListNavigation; context menus use one store and renderer (state/contextMenu.ts, ContextMenu.vue), with git MenuSection[] passed through sectionsToMenuItems.
- A danger confirm and any DialogContent destructive open focused on the data-dialog-dismiss control; confirms use confirm({ message, title?, confirmLabel, tone, run? }) over ConfirmDialogView.
- Shared behaviour primitives: useInlineEdit/InlineEditInput for inline edit, usePersistedState (key kira.<module>.<name>; its shared refs hold no component instance) for per-device UI state, useCopy/copyText, useAsyncAction, errorText for bridge errors, behaviour/format.ts for formats, TIMING for timers, optimisticUpdate, PointPopover.
- Data states use QueryState over a client from createQueryClient({ remote }) with staleTime Infinity, push-invalidated; a query that must refetch on mount sets staleTime: 0.
- Local chords use useShortcut over SHORTCUTS, which carries scope; a unit test forbids two bindings on one chord in a scope.
- Pane and column resize uses ResizeHandle over usePaneResize (VueUse useDraggable; paneResizeMath holds clamp, direction and percent arithmetic). reka SplitterGroup is retired app-wide.
- SplitterGroup was retired because the P132 hang was a non-deterministic race inside third-party layout code and the sizeUnit="px" panel fought SlickGrid on every ResizeObserver tick.
- Px panes are plain flex (flex: 0 1 <size>px), the pane beside a handle takes flex-1, and the Operations dock stays a full-width sibling below the row.
- A resize drag previews through useLiveSize and commits once on release; pointercancel, window blur and unmount cancel it. panel-resize.spec.ts in each app guards release paths.
- Guards U39-U63 in check-ui-primitives.sh hold named permanent exemptions only (engine hosts for Monaco, SlickGrid and TipTap, combobox focus, protocol timeouts, domain text formats) and never scan behaviour/ or testing/.
- The git module reuses the host Pinia and <ContextMenu />; afterClose on the context-menu store defers an inline rename until the menu releases focus.
- A view is chosen by page kind, never by database type: Page is a discriminated union (TabularPage, DocumentPage, KeyValuePage, StreamPage) and the UI reads the page's kind to pick grid, document, key/value or stream view.
- Because mapping is page-shape to view, a Postgres jsonb column can open in the document view and a Mongo $group result can open in the grid.
- Tab identity is the tab's id, never its path; the same table/collection/key can open in many tabs, each with independent paging, projection, sort, filter and scroll state. This holds for every tab kind, Browse included, whose extra state is state.levelPath.
- A tab's mode is derived, never stored: STUDIO_TAB_KIND_MODE (state/tabDomain.ts) is a total hand-maintained map from TabKind to AppMode (Kira Space has SPACE_TAB_KIND_MODE), so no mode column, migration or Go change exists.
- The second top-level mode Api was named Http through P1-P11 and renamed by P12 D2 once gRPC joined it.
- useModeStore in state/mode.ts is a plain selection (setMode); activateTab, closeTab, closeOthers, closeToTheRight, closeAll and stepTab are scoped to the current workspace's slice of the shared tabs array.
- dropTabs(ids) is the system-initiated close (deleted connection): no discard prompt, one saveNow.
- Switching mode touches no TabRecord and schedules no save; it only reaches Go, debounced, to record which mode a window was last in so it reopens into the same one.
- The mode store and its per-window persistence are shared factories (P128 §2.3/§2.6), not Kira Studio-only: createModeStore<M extends string, E>(control, defaultMode, extend) in packages/workbench/src/state/createModeStore.ts is the whole of useModeStore for both apps.
- createModeStore provides state, hydrateMode (boot-time set, no write-back), setMode (user-facing selection that schedules a write), and debounced-then-flushed persistence.
- Kira Studio's mode store passes activeTab through extend; Kira Space's mode store (new in P128) passes no extension.
- A mode click is never a synchronous write: setMode debounces windowsSetMode by 150 ms (useDebounceFn, MODE_WRITE_DEBOUNCE_MS).
- A window closing inside the mode-write debounce period flushes the pending write immediately instead of losing it, via control.onFlushBeforeClose/onWindowFlushBeforeClose, the native about-to-close/quit signals Go already blocks the close on.
- Window mode persistence is internal/appstorage.WindowModes{Default, Valid} (P128 §2.2), one per app, replacing Kira Studio's own validWindowModes/NormalizeMode.
- Valid window modes are {studio, api, terminal} for Kira Studio and {git, terminal, ade} for Kira Space.
- An unrecognised or missing stored window mode normalizes to Default (git for Kira Space); on Kira Studio a stored git-mode window from before P100 degrades to studio.
- The bound window surface (Ensure/SetMode) is internal/windowsvc.Service, a single package embedded by both apps' WindowsService rather than two duplicated implementations; Kira Space gained Ensure/SetMode in P128 (previously only OpenNew).
- A module's own registry entry, not a per-shell branch, mounts its panel/start/"+" (P128 §2.3); ModeRegistry<M, D=ModeDef> in packages/workbench/src/modes.ts replaces every modeStore.active === '<id>' conditional in both apps' WorkbenchShell.vue.
- ModeDef is a discriminated union: PanelModeDef{label, icon, layout?: 'panel', panel, start, newTab?} or FullModeDef{label, icon, layout: 'full', view} (P129 Part 3 §0.12/§0.13).
- A layout: 'full' module mounts its own view in #main with no left panel, no tab strip and no "+"; the shared WorkbenchShellBase's tabStripVisible prop (default true) gates the tab strip out for it.
- Kira Studio's MODES is pinned to ModeRegistry<StudioMode, PanelModeDef> because its shell reads .panel/.start unconditionally and no full-layout module exists there yet.
- Kira Space's ade mode (P128 placeholder) is the app's first full-layout module, which is what the ModeDef union exists for.
- packages/workbench/src/components/ModeSwitcher.vue, generic over M, is the shared title-bar mode switcher with markup byte-identical to Kira Studio's pre-P128 inline one; Kira Space's TitleBar.vue renders it for the first time.

- The Runs entry in both apps' MODES points at one shared module, packages/workbench/src/automations/module.ts, not two hand-kept-identical copies (renamed from Terminal in P242).
- Each app's App.vue builds one AutomationsModuleContext object (in its own workbench/automationsModule.ts) and provide()s it once at the root, above WorkbenchShell.vue.
- The mode key, windows.mode and the module's tab workspace_id are all automations; real terminals in packages/workbench/src/terminal/ stay terminals, only the launching module was renamed. The module shows as Runs (P438); mode key, i18n keys, code names, test IDs and the `<app home>/automations` folder keep `automations`.
- Script logic lives in repo-root internal/scripts (custom_scripts repo, validation, service); each app's bound bridge.CustomScriptsService keeps its own binding name and delegates to it. List answers and every mutation broadcast one Snapshot{collections, scripts}.
- Script dirMode kira (default) runs in <app home>/automations/<script id>, created 0700 on demand; a symlink or non-directory at any level is refused with "<path> is not a real folder: remove it".
- Script dirMode fixed is the chosen absolute folder and must exist.
- Script dirMode home is the retired $HOME default, kept only for rows that predate the choice, and is refused on create or when switching another mode to it.
- Removing a script removes its kira folder.
- A script's command and folder are Go-authoritative: terminal.Open with a scriptId loads the stored command and folder and ignores the renderer's cwd/command, so a forged argument cannot run another command.
- createCustomScriptsStore in packages/workbench/src/automations is the shared store factory over each app's control; both apps' state/customScripts.ts are one-liners.
- The panel header + ("New script…") is the only add control for scripts; the empty state has none.
- ScriptDialog.vue edits one script in three force-mounted tabs (Script, Parameters, Schedule), hidden when inactive, so ScheduleFields keeps emitting valid and state survives a tab switch. The server error and footer sit outside the tabs.
- The script body field (ScriptBodyField.vue) is a multiline AutocompleteField, not Monaco, because no shared Monaco host exists and .fill() on the real textarea stays valid.
- In multiline AutocompleteField, Enter inserts a newline unless a suggestion was arrowed onto, and Cmd/Ctrl+Enter bubbles to save; autoClose is off and openOnEmptyToken lists every candidate right after { or $.
- A multiline script command runs as-is in the user's login shell with shell -c.
- A prompt offers non-secret params for completion after {name; a command offers KIRA_PARAM_<NAME> for every param (secret included); with ADE (Space) the built-ins task jira repo branch base worktree are offered (KIRA_<NAME> in a command).
- Known vars paint as variables; a secret param in a prompt and an unknown $KIRA_PARAM_X paint as unknown; an unknown {word} stays plain text.
- The "Uses:" chips show the value a run gets without input: the param default, "asked at run" without one, or "from the task" for a built-in.
- Each dialog tab trigger shows a Badge with its error count from scriptErrors.ts, a client mirror of the Go checks; empty-field errors wait until the field was edited, Save stays disabled while errors remain, and Go stays the authority.
- Script collections are real rows managed like API collections; the header new-folder button or list background menu creates one named "New collection" and opens it for inline naming via shared InlineRenameInput.vue. Deleting a collection (confirmed) deletes its scripts too.
- Ungrouped scripts list first, then collections in creation order, empty ones included; collapse state is stored in localStorage key kira.automations.collapsedCollections by collection id (copied once from kira.quickCommands.collapsedCollections).
- Search shows a collection when its name or one of its scripts matches.
- New collections are created from the panel, not typed in the script dialog; Settings has no Scripts section.
- AutomationsPanel.vue, AutomationsStart.vue, AutomationsNewTab.vue, ScriptDialog.vue, TerminalTabView.vue and TerminalHostView.vue are the same six files in both apps, imported lazily via defineAsyncComponent so a git-only session never pays for the terminal launch chunk.

- internal/runoutcome is the single run-result type shared by both stores: Outcome{status done|failed|blocked|cancelled, reason, source agent|exit|timeout|start|user|restart, reported, exitCode, lastError, summary}.
- runoutcome.ForProcess is the only wording table for run outcomes, so ADE step runs (ade_runs.outcome_json, Space only) and script runs use the same wording.
- internal/scriptruns owns the script_runs table and the kira:scriptRuns:changed push (which carries the whole changed run), and it implements terminal.ScriptLauncher.
- ScriptLauncher.Begin records a running run when a script terminal opens; a blocked folder records a failed run with source start and refuses the open.
- ScriptLauncher.Exited finishes a run from the session's exit code and the registry's close cause (CauseUser from Terminal.Close or ScriptRuns.Stop, CauseWindow, CauseQuit).
- Recover runs at boot and fails every run a dead process left running, with the reason "interrupted: Kira Studio closed while it ran".
- Recover also flags those runs `restore_pending` (migrations Space 0061, Studio 0050). `TakeRestore` hands them to an in-memory offer for the session, so a second restart does not repeat it. A user stop never flags. `RestoreState`, `Restore` and `RestoreDismiss` serve the offer; `Restore` re-checks the script hash per item (`E_CONFLICT` on drift) and resumes smart runs.
- The Runs module shows a banner for the offer (`automations/restore/`); clicking it opens the shared restore dialog. No startup auto-open.
- The scriptRuns frontend query is keyed ['scriptRuns','list'] and is patched in place by the push channel, with no polling; memory runs stay out of the Runs list. The Runs panel (RunsSection.vue) lists only running and waiting runs; finished runs live in the Operations panel.
- `runoutcome.Outcome.outputTail` holds the last 40 sanitized lines (8 KiB cap) of a failed terminal or headless script run. A `termtext.Tail` on the script session tap (`ScriptLauncher.Output`) feeds terminal runs; the headless log sink feeds the rest. It lives in `outcome_json`, so it survives tab close and restart. Copy for agent falls back to it once the terminal tab is gone.
- `internal/termtext` is the one ANSI stripper and line sanitizer (gitprepare and scriptruns).
- A script keeps one tab per key {scriptId, taskId, branchId} ('' when none), stored in terminal (launchKind script) and script-run tab state; packages/workbench/src/automations/scriptTabs.ts holds the rule.
- A start for a key whose tab is live (terminal session and run still running, or run running|waiting) focuses that tab and starts nothing; a finished key tab is replaced in place (new tab id, same strip slot, so terminal ids stay per run); no key tab opens a new one.
- Viewing a run (history, chip, resume, agent reveal) uses the same placement: tab already on that run activates, finished key tab is replaced, live key tab with another run opens a new tab.
- createTabsStore OpenTabOpts.replace names the tab a new one takes over. The gate lives in the UI only; Go does not refuse concurrent runs of one script.
- A script row click selects, double-click or Enter opens Edit; the play button, menu Run and Run now are the only starts.
- A smart script (custom_scripts.kind = smart) runs as headless Claude via claude -p through internal/agentmcp (moved from Space's adeagent), sharing the launch, stream-json parser and finish_step loopback server with the ADE run engine.
- Smart script defaults are model sonnet, 1 USD budget (--max-budget-usd) and 15 min timeout; bounds in internal/scripts/smart.go are 20 params, 20000-char prompt, budget 0.05-25 USD, timeout 1 min-2 h.
- Smart runs are Isolated: --setting-sources '' loads no Claude settings file, --strict-mcp-config allows only the report server, and --permission-prompts none denies instead of asking. Exception, only with `agent.hermetic` off (`HeadlessSpec.UserMCP`, P444): a script that ticks a user MCP server runs with --setting-sources user and no --strict-mcp-config; --settings `{"disableAllHooks":true,"allowedMcpServers":[...],"enableAllProjectMcpServers":false,"enabledMcpjsonServers":[]}` limits the loaded servers to the report server plus the ticked ones, never loads project `.mcp.json` servers, and claude loads the ticked ones from its own config. The presence check (`smartBlocker`, ADE `checkMCP`) lists servers from the app home, like the picker, so a name only a folder's `.mcp.json` defines does not count. A cached list answers while it has every ticked name; a miss re-lists unless the list is under 30 s old. A failed listing reuses the last good one; the picker shows its statuses as `unknown`. The shared list flight runs detached from the first caller; each caller waits under its own context. A ticked server grants all its tools (`mcp__<server>`) or named tools. With hermetic on (default) the picker and presence check list the app's `settings.yaml` servers instead; see "Hermetic launches (P455)".
- A smart script's --tools and --allowedTools come from its tool allowlist (built-ins Read, Grep, Glob, Edit, Write, NotebookEdit, Bash; default Read, Grep, Glob).
- A smart run's final report must go through finish_step; a run without it fails with the shared "no report" text.
- Smart script params are text, select or multiselect (a text param may be secret); names are lowercase and must not use the reserved ADE built-ins task, jira, repo, branch, base, worktree; {name} in the prompt is substituted before launch.
- A secret param value reaches a smart run through env only: never in the prompt, stored script_runs.prompt or params_json (stored as the mask), and a smart script whose prompt names a secret param is refused.
- P447: a secret text param can have a saved value, sealed in secret_values (scope script-param, owner custom_scripts.id, name = param name). The script record holds names only (savedSecrets). Create/Update take secretValues: string saves, null clears, absent keeps; the value is write-only, never returned. A secret param has no default. Remove, rename or un-secret a param, or delete the script or collection, prunes the row in the same tx. Both apps; no script duplicate or export exists, and any later path must never copy plaintext (a copy starts with no saved values). Without a keychain, saving returns E_SECRET_STORE.
- A saved value fills a secret param only when none was typed (typed wins), stays env-only, and is used by resume and restore. An unreadable saved value counts as absent and sets Preview.SecretIssue.
- Output masking (Masker over all secret values of the run): log lines before truncation, outcome Reason, LastError, OutputTail and Summary, and the terminal tail. Not masked: the live terminal tab and the Space phone terminal ring (mobileterm), lines cut at linewriter.MaxLine, agent writes through Space task tools, and anything the process writes elsewhere. ADE workflow smart steps keep the old rule (values from step params only; no saved fill, no masking).
- script_run_logs holds the run log (stdout|stderr|event), read by ScriptRuns.ReadLog; script_runs gains model, session_id, prompt, params_json, tools_json.
- A recurring script is a script with a schedule {cron, timezone, enabled, confirm, timeoutSec, taskId, branchId} stored in schedule_json on the script row (empty when not recurring).
- Schedule cron uses 5 fields, no @ tags, an IANA zone, and is parsed by adhocore/gronx v1.20.4 in internal/scriptruns/scheduler.go.
- One scheduler loop per app sleeps on the injected clock to the next fire across all enabled schedules; a fire later than 60 s (app closed, sleep) is dropped silently, never replayed.
- Any trigger (cron, Run now, manual) that finds the script's previous run running or waiting records a skipped run with the reason.
- A confirm script records a waiting run and raises a popup; the run starts only on ConfirmAccept, ConfirmDecline ends it cancelled, and a waiting run never expires. A schedule with confirm off is refused only when a required secret param has no saved value.
- Scheduled normal scripts run headless (terminal session without a tab, output to the run log) with timeoutSec; a Space ADE-bound run holds the board claim and fails with the task reason when the task is missing or archived.
- Scheduling adds run states waiting and skipped and outcome source schedule; the confirm popup (ScheduleConfirmHost) is prompt kind schedule routed by the central prompt router; Space posts a notification for a failed scheduled run.
- A schedule is set only inside the script editor's Schedule tab (create menus have no recurring entry); a scheduled row's menu has Run now and "Edit schedule…", which opens the editor on that tab.
- scriptruns.ADE is the seam to the task board, implemented by ade.TaskBoard; Studio leaves it nil and refuses a taskId with "tasks exist only in Kira Space".
- A script with use_ade_dir runs in the branch worktree; ClaimWorktree creates a missing worktree and holds a claim keyed by branch id under the board's runMu.
- The worktree claim blocks both ways: a step run on a claimed branch is held with note "waiting for automation <name>" until release (and after a restart); a rebase is refused with "automation X is running in <branch>"; StartBranch is refused; a script start is refused while a step run or rebase works on the branch.
- A workflow step may name smart_script plus params instead of a prompt (planSmartStep, smartOutcome, supervised by superviseAgent).
- agentnotify.HandleScriptRun posts for smart runs only: failed under Enabled, blocked under OnNeedsInput, done under OnRunEnded, cancelled never.
- Kira Studio MODE_ORDER is studio, api, automations, docker; Kira Space order is ade, automations, memory, git. Space opens new windows in git (WindowModes.Default), now the last tab. ModeDef.tabStrip: false hides the tab strip for modules without tabs (Docker and Space's Memory).
- ModeSwitcher tabs are icon-only (P438): the module label is the tab's aria-label and its tooltip; no mode shortcut exists.
- WorkbenchShell takes mainFramed (default keeps the main-panel frame); ade passes it off so each pane carries its own single frame.
- Kira Space repo terminals and the Runs module's terminals both render through the shared TerminalTabView.vue/TerminalHostView.vue, not two parallel implementations.
- P128 deleted views/repo/RepoTerminalView.vue, which had been a hand-kept-identical second copy of the terminal tab body for repo-scoped terminals.
- A repo terminal differs from a Runs terminal only in the workspaceId/codeRepoId the opener stamps on the tab record: openRepoTerminalTab (repo-scoped) versus Runs' openTerminalTab (workspace-scoped, visibleWorkspace()'s id).
- TerminalTabView.vue/TerminalHostView.vue read neither workspaceId nor codeRepoId; the tab prop is typed against TerminalHostView's own prop shape, not either app's tab-record type.
- Shift+Enter in every terminal sends `ESC CR` (meta-Enter) via one custom key handler in `terminalRenderer.ts`; a newline in Claude Code and zsh. Cmd+Enter has no binding (xterm sends `CR`).
- Terminal links come from `@xterm/addon-web-links` (plain `http(s)` text) and xterm `linkHandler` (OSC 8, `http(s)` only). Desktop opens on Cmd-click (mac) or Ctrl-click through `LinkService.OpenExternal`, with a hover hint; plain click never opens. `window.open` does nothing in the Wails webview, hence the bridge. Phone (`linkActivation: 'tap'`) opens on tap via `window.open` with `noopener,noreferrer`. No file-path links.
- Terminal scroll options match VS Code (xterm defaults plus `scrollOnEraseInDisplay`). `sessionEnv` sets `CLAUDE_CODE_SCROLL_SPEED=3` unless already set: xterm 6.0.0 has no XTVERSION, so Claude Code otherwise falls back to 1 row per wheel report.
- Terminal background is `--kira-bg-chrome`, VS Code Dark Modern panel/terminal colour.
- The Operations dock is a full-width flex sibling below the shell's project/main row, outside any split, because nesting it as a vertical group hung the render process on a tree-row click (never root-caused).
- WorkbenchShell.vue renders the project panel and the dock through ResizeHandle (px, clamped to [180, 480] and [100, 500]), previewing locally and emitting resize-project/resize-ops on release; project and main share one row so their heights match.
- Project width and dock height are keyed by active module (`modes.<mode>.project.width`, `modes.<mode>.project.widthUserSet`, `modes.<mode>.operations.height`); each shell passes `layoutStore.sizesFor(modeStore.active)`. Visibility and cell editor height stay app-wide.
- The Operations panel is shared: OpLogPanel.vue (generic over R extends OpLogRecord) in packages/workbench is the panel shell, and createOpLogStore<R>({ control, searchText?, facet? }) is the Pinia store factory. It adds `hydrated`, `facetValues`/`facetAllows`, `backgroundCount` and `resetFilters`.
- opLogRecordSchema/OpLogRecord in packages/shared/domain/ops.ts (id, startedAt, durationMs, kind as plain z.string(), status, command, error) is the base contract that consumers' own op records extend.
- Kira Studio's OpRecord extends opLogRecordSchema with connectionId, tabId, kind: opKindSchema, rows, commandTruncated and path; it is the first consumer, and Kira Space's SpaceOpRecord is the second (P132 Part 2).
- Kira Studio's OperationsPanel.vue wraps OpLogPanel with prioritised columns, `fieldsFor`, `codeLanguage` (colorize, no editor instance), a Connection facet, and a `menuFor` with Reveal tab and Re-run first.
- OpLogPanel owns the status column, time format, column fitting (`fitColumns`, `COMMAND_MIN_PX` 240), tree order (`orderOpTree`, `parentOf`), several open details measured by TanStack Virtual through `useVirtualRows`' `virtualizer`, row actions, loading/empty/no-match states and the facet dropdown. `OpLogDetail` renders the detail.
- OpLogPanel.vue shows a live elapsed clock (op-elapsed, m:ss) for every running row, ticked by one shared VueUse interval paused while runningCount is 0; finished rows show the formatted durationMs.
- opLogMenuItems supplies copy-command, copy-error, optional copy-details, a separator, then cancel; consumers find items by `id`.
- toggleOperationsPanel/setOperationsHeight moved from Kira Studio's state/layout.ts into the shared createLayoutStore.ts core, used by both apps.
- Script runs show in both apps' Operations panel as a projection of `script_runs` (`automationOp`, id `run:<id>`), never rows in `op_log`. Panel retention for them is `script_runs`: newest 500 finished kept, newest 200 loaded.
- `useAutomationOps` merges them into each app's panel list. Cancel maps to Stop (running) or Decline (waiting); the menu adds Copy for agent, Open run, Resume and Run again. A waiting run shows its state in the command cell, not the status cell, so Cancel stays reachable.
- `opLogMatches` is the one filter predicate for op rows and script run rows. Clear sets `clearedAt`; script runs started before it stay hidden in that window.
- `AutomationsModuleContext.showOperations` opens the Operations dock; the Runs panel's empty state links to it.
- No Kira-Studio import (MonacoHost, a connections store, kira-studio) may reach the shared package; this was verified by a closing-audit grep in P132 Part 1 §7.

- WorkbenchShell.vue (shared) always mounts #dock; hasDock is removed, opsVisible/opsHeight are required props, and the status bar is always mt-1.
- Kira Space's dock also shows in the ade full layout, gated only by panel.operations.visible.
- The Space op log (apps/kira-space/internal/oplog) is persisted in `op_log` (migration 0060): 30 days or 5000 rows, pruned at start and every 500 finished ops; rows left `running` by a quit are reconciled to `error` at start. It shares nothing with Studio's op log.
- Two in-memory lanes: foreground (500) and background (300). Only foreground rows and failed background rows persist, through one writer goroutine. Foreground changes push on `kira:op:update`; background rows coalesce on `kira:op:batch` (about 1 s).
- A spawn is background when it is `ReadOnly` with no enclosing op, or its `Origin.Background` is set (auto-fetch, repo life reads, the ADE board context). The panel hides the lane until its Background toggle (`kira.ops.showBackground`); background rows never count as running.
- Out-of-process writers (memory MCP server, module `memory`, P415 Part 2): `kira-space memory-mcp` writes `op_log` in `kira.db` through its own `oplog.Log` (same redaction, lanes, persist and prune rules; no migration; a missing `kira.db` or table records nothing). Ids are `<owner token>.<random>`; the owner holds `op-owners/<token>.lock` for its life and releases it after the writer drained. The app polls `PRAGMA data_version` (1 s) and ingests foreign rows by rowid cursor plus the ids still running or in the background lane, never persisting them (`oplog.WatchForeign`). Start reconcile skips foreign rows; a running foreign row whose owner lock is free becomes `error`. Import-mode calls (`--import-ref`) are background: failures only. Memory ops are not cancellable.
- A background failure with the same (module, repo root, kind, first error line) within 10 minutes folds into the earlier row and bumps `repeat`.
- `oplog.Origin` (module, trigger, label, task id, background) travels in ctx. Stamp sites: the gitrpc `ForConn`, `RepoEntry.life`, the auto-fetch ctx, the Files and ADE bridge methods, the ADE board's own ctx. A child op carries `parentId` of the enclosing op in ctx. No row has module `app` unless a spawn has no stamp.
- `oplog.RecordingRunner` decorates the one `gitclient.Runner`, so every spawn is recorded at one seam: a step of the enclosing op, else its own row. `Spec.Stdin` spawns (the persistent `cat-file` pair) are not recorded. `Spec.OKExits` marks expected non-zero exits (`merge-base --is-ancestor`, `check-ref-format`) as ok. The `command` of a git op is its write steps only.
- `OKExits` audit (every caller; each is a documented answer, not a failure): `rev-parse --verify -q` in tip checks and undo capture (exit 1: ref gone), `symbolic-ref -q HEAD` and `rev-parse -q --verify HEAD` in `ResolveHead` (exit 1: detached or unborn), `merge-tree` (exit 1: conflict predicted), `config --get-regexp` (exit 1: no match), `merge-base --is-ancestor` (exit 1 or 128: not an ancestor, unknown commit), `check-ref-format` (exit 1: invalid name). Wired through `runAllowingExit`, `runInDirAllowing` and the ops.go/repo.go sites.
- Non-read spawn census: `ReadOnly: false` spawns are `RunOp`, restack, pull's integrate step, remote fetch/push (`runRemoteSpawn`) and the ADE agent `git` tool (`TaskBoard.RunGit`); each runs under an op, so it is a step. `WorktreeAbort` is the one write with no enclosing op: it becomes its own foreground row through the runner.
- Redaction runs before anything is stored or sent: URL userinfo, secret-keyed `-c` and `--config-env` values, `--password=`-style flags, secret keys and `args` arrays in MCP arguments.
- Session ops (category `session`: `start`, `adHoc`, `takeOver`, `restart`, `manual`, `reviewAgent`, `run`, `resume`, `rebase`, `review`, `setup`, `stop`) and in-app MCP ops (category `mcp`, `trigger mcp`) share the record shape; a running session op registers Cancel that stops the run. MCP calls, in-app and memory, are recorded by one receiving middleware (`mcpcall.Record`, fed by `agentmcp.Options.OnToolCall` and `mcpserver.Options.OnToolCall`), so no tool wraps itself; the status mapping is `oplog.FinishCall`.
- Issue tracker ops (P417, category `tracker`): `oplog.RecordingToolRunner` decorates `jira.Runner`, one background row per acli call. Kind is the subcommand path (`version`, `auth status`, `workitem view`), source is the provider label, command is the redacted argv with `acli` first. A failure persists and folds like any background error. `gh` calls stay unrecorded.
- Space supplies `stepsFor`, `parentOf`, the module facet and badges. "Via <tool>" shows only when the parent is filtered out. Copy details is shared through `opLogMenuItems`.
- The op command is taken from argv only, so the askpass token in the environment is never recorded.
- Cancel is registered only for fetch, pull's fetch stage and restack (CancelRemote/CancelRestack); the push family is never cancellable.
- Each op unregisters its cancel before releasing its opSlot, and Log.Cancel calls the func under the log lock, so a late cancel never hits the next op.
- Registry.OpLog is set in wireGit. OpsService (Recent, Cancel) and the kira:op:update event broadcast to every window; Clear is per window and renderer-only.
- The op source is Conn.ClientLabel: "Kira Space" for the native conn (renamed from "This window", which was false in other windows), "Kira Space ade", or a VS Code client label.
- ChannelOpUpdate and ChannelToggleOperationsPanel live in internal/appevent; onToggleOperationsPanel lives in createCoreControl.
- Cmd+J reaches the renderer only through the Go menu item (Toggle Operations Panel), never a keydown.
- C5 widened tab scoping from per mode to per workspace: TabScope = AppMode | 'repo' in packages/shared/domain/tabs.ts, a repo tab's workspace comes from TabRecord.workspaceId (workspaceKeyOf), and the tabs store uses activeIdByWorkspace/previewIdsByWorkspace.
- For studio/api tabs workspaceId is null, so workspaceKeyOf falls back to TAB_KIND_MODE[kind] and behavior is unchanged.
- packages/shared/domain/tabs.ts keeps only what both apps share: the tab record envelope, TabScope, and the terminal kind; each app declares its own kind union and per-kind schemas in its own state/tabDomain.ts.
- Kira Space's tabDomain.ts declares SpaceTabKind (repo-graph/repo-file/repo-diff/repo-multi-diff/terminal) and is the only real consumer of the 'repo' scope; Kira Studio's StudioTabKind is a 12-kind union with no repo entries and no guard stubs.
- AppMode in packages/shared/domain/mode.ts is 'studio' | 'api' | 'automations' | 'docker' ('git' was dropped); TabScope adds 'repo' as a sentinel only because the shared package still serves the app that uses it.
- The incognito flag on request tabs is per-tab and in-memory only (useTabIncognitoStore in state/tabIncognito.ts); nothing is saved or restored, and it is a separate store from the tabs store to avoid an import cycle with tabKinds.ts and tabs.ts.
- Incognito design is suppress, don't mirror: one flag consulted at each write site.
- Any new bridge write reachable from a request tab must consult isIncognito first, because incognito suppresses writes at each write site.
- Toggling incognito on or off flushes the tab's existing row immediately (registerIncognitoSetListener); closing a tab drops the flag through registerTabRuntimeCleanup.
- On the Go side, incognito is two Incognito bool fields (HttpSendArgs, GrpcCallArgs) threaded into adapterhost.OpSpec, guarding ResponseHistoryRepo.Record and internal/oplog persistence; an incognito op still emits live events to the Operations panel but is never written to op_log.
- An incognito tab's active environment is a per-tab in-memory override (incognitoEnvByTab), never a write to the shared active-environment row.
- usePanelHeaderSearch (packages/workbench/src/behaviour/panelSearch.ts) owns the left panel header's search reveal/toggle and VS-Code-style type-ahead redirect; ProjectPanel.vue and api/CollectionsPanel.vue inline their own header markup over it, and there are still exactly three layout panels.
- useTreeVirtualRows (packages/workbench/src/behaviour/treeVirtualRows.ts) is the shared virtualized-tree hook (@tanstack/vue-virtual, pinned-ancestor sticky band, reveal-scroll with band inset), generic over rows with depth/hasChildren/expanded/key; stickyBand.ts lives in packages/theme/src.
- ProjectTree.vue still owns Studio-specific behaviour: the connection-driven row model, five openable-kind dispatch, context menus, and keyboard shortcuts.
- api/CollectionsTree.vue is the second consumer of useTreeVirtualRows, over its own row model CollectionRowVm (four structural members plus seven of its own versus TreeRowVm's fourteen), since connectionId, color, status, statusDetail, groupKind, badges, loading and error mean nothing there.
- api/CollectionRow.vue is its own row component rather than a widened project/TreeRow.vue, partly because api/** may not import project/** and because the rows differ (no connection rail, status dot or EngineIcon; leading method chip; inline rename input).
- Studio's tree is lazy because its data is remote (expanding a node connects a connection and issues an IPC call), whereas the collections tree is rows in a local SQLite table listed in one call, so its visibleRows is a pure computed with no cache, loading set, or incomplete-search caveat.
- While a collections search is active, every ancestor of a match renders expanded without mutating the expansion set, so clearing the search restores the user's prior shape.
- CollectionsService.MoveItems and DeleteMany are batch calls, each one transaction. MoveItems moves requests and folders, each with its subtree, under a parent of any collection, in argument order before `beforeId` or last. The target's sort_order is rewritten dense, every parent the items left re-indexes dense, and a refused move (into the item itself or a descendant, a non-folder parent, a `beforeId` outside the parent) changes nothing. An item whose ancestor is also moved rides with that ancestor. DeleteMany removes collections and items with their subtrees; a missing id deletes nothing. Neither call has a single-item form.
- The collections tree's selection lives in useCollectionSelectionStore (focus key, range anchor, key set), fed the visible key order by the caller so a range or select-all never covers a hidden row. A row that leaves the visible list leaves the selection. Click picks, Ctrl/Cmd toggles, Shift extends, Shift+arrows extend, Ctrl/Cmd+A selects all visible, Escape collapses to the focused row. Rename, duplicate and Variables… act on the focused row only; a right-click inside a multi-selection opens a menu of the set actions (move, delete).
- Dragging the selection runs on the shared useTreePointerDrag; api/collectionDrop.ts holds the drop rules (zones by row height, topmost rows, placement, descendant and no-op refusal). Requests and folders drag; a collection row and a selection containing one do not. While a search filters the tree only "into a folder or collection" is allowed, since before/after would reorder against a list with hidden siblings.
- Deleting several rows asks one confirmation that names the counts. A deleted request's open tab stays open and reads as unsaved (the orphan rule); a deleted collection closes its variable-set tab.
- An open request tab follows a collection move automatically because its collection is derived from the tree, not stored in tab state.
- The tab strip and content area are registry-driven: state/tabKinds.ts's TAB_KINDS (component-free) supplies each TabKind's title, icon, rail colour, default and duplicate state, page-store cleanup and context-menu extras.
- workbench/tabViews.ts's TAB_VIEWS supplies the view component per tab kind; it is statically imported, so the bundle's two dynamic chunks are unaffected.
- MainView.vue and TabStrip.vue are shared components in packages/workbench/src/components/; MainView resolves host.views[activeTab.kind] through the provided useWorkbenchHost() (packages/workbench/src/host.ts) rather than importing either app's TAB_VIEWS at module scope.
- MainView.vue has an #empty slot fallback for no active tab; each app passes its own fallback: Kira Studio uses the active mode's start component from MODES in workbench/modes.ts, and Kira Space uses GitStart.vue.
- TabStrip.vue reads the TAB_KINDS registry for icon, title, rail and context-menu instead of branching on tab.kind, and no longer imports project/state/tree.
- TabStrip reads the narrow TabStripHost seam (host.ts): an optional host prop, falling back to the injected WorkbenchHost, which extends it. Optional capabilities (close*, duplicate, promote, preview, moveTab) hide their affordance when absent.
- TabStrip has a #tab-leading slot that renders before the title, and the kind flag pinnedTitle makes a pinned chip labelled rather than icon-only.
- Tab reorder uses vue-draggable-plus useDraggable in forceFallback mode on the scrolling row only (pinned chips sit outside it), bound to an id mirror so the library reverts its own DOM move; onUpdate commits once per drop via moveTab.
- The reorder binding is useSortableReorder (packages/workbench/src/behaviour/sortableReorder.ts), shared with Studio's column, environment and variable lists, the Runs panel (SortableList.vue) and Space's Git panel repo list. Its keyboardHandle option limits Alt+Arrow moves to an item's own row.
- Studio's project tree and the collections tree are virtualized, so their pointer drags share workbench/behaviour/treePointerDrag.ts (VueUse pointer hit-testing, press tolerance, edge auto-scroll, Escape and pointercancel, click swallow); the caller supplies what a press may grab, where a drop resolves and what it does. Connection reorder is project/connectionReorder.ts on it (drop line). Alt+Up/Down go through the connection menu's Move up/down items.
- Adding a tab kind takes one registry entry each in state/tabKinds.ts and workbench/tabViews.ts, not editing a dispatch chain in three files.
- 'http-request' is the first Api-mode tab kind: TAB_KIND_MODE['http-request'] is 'api' and every other entry is 'studio'.
- Tab kinds are a vocabulary that Go's model.RenderableTabKinds must also carry; no compiler catches a miss, and tests/unit/go-ts-vocabulary-parity.spec.ts checks it.
- An HTTP request tab's path is the literal constant 'request' because its state is the request and it has no real target; it is non-empty (model.TabRecord.Validate requires one) and never used for identity, which is always the tab's id.
- Duplicating an HTTP request tab copies the source's own request state rather than starting fresh, unlike other kinds, which get the same target with fresh default state.

- A saved collection request opens the same 'http-request' tab kind; P4 added no tab kind or op kind. Its state comes from an api_items row, and the binding is stored in state as itemId, not in path.
- The saved-request binding lives in state.itemId, not in a collection:<id>/request:<id> path, because duplicateTab copies path verbatim while duplicateState clears itemId. A path identity would leave a duplicated tab looking saved, and openTab's reuse lookup (kind + connectionId + path) would activate the duplicate instead of the original.
- Reuse lookup for saved collection request tabs is done explicitly in openCollectionRequestTab, keeping identity in exactly one place.
- The http-request tab state has a second field, name, so httpRequestTitle stays pure; without it a request saved as "Create order" would render as /v2/orders in the tab strip and header.
- Deleting a collection row does not close tabs bound to it, because closing would lose work; the tab reads as unsaved and Save falls back to Save as…
- Since P3, a restored tab's state is normalized through its own kind's Zod schema: TabKindDef (state/tabKinds.ts) has a parseState(raw) member that does safeParse and returns null on failure, and hydrateTabs (state/tabs.ts) keeps the parsed result only on success, otherwise the stored record unchanged.
- Before P3 nothing in the restore path parsed tab state; repos/tabs.go validates only the envelope (ID/path/kind/object-ness), leaving per-kind shape to the renderer, so tabs saved with a missing field restored it as undefined and stale enum values restored as-is.
- Tab-state normalization is merge-only: a successful parse can only add a missing field's default, never drop or reset one, so it is safe under state-widening phases without a migration story.
- tabsSave writes the tabs store's tabs straight back, so a parse also drops any key no schema recognizes; this is deliberate since such keys had no reader.
- P12 renamed the Http module to Api because gRPC (P11) made "Http" too narrow. Rule D1: a name changes to Api only if the thing it names also serves gRPC.
- internal/apivars (renamed from internal/httpvars) resolves gRPC targets and metadata so it carries the module name; internal/httpclient and internal/grpcclient each serve one protocol and were not renamed.
- frontend/src/api/ (renamed from frontend/src/http/) holds the module-level half: the collections tree, six dialogs mounted through one <ApiDialogs />, the module's stores, tabs.ts (twelve tab opener/finder/patcher functions moved out of state/tabs.ts so the shell imports nothing from views/**) and reveal.ts.
- views/httprequest/ and views/grpcrequest/ keep their names as protocol-specific tab views, following the views/<kind>/ convention; renaming one would imply the HTTP tab is the Api tab and gRPC is something else.
- @kira/api-core (packages/api-core/) is a ~2,000-line pure, DOM-free Bun workspace package holding {{name}} substitution, curl parse/generate, raw HTTP parse/generate, the dynamic-value catalog and both protocols' saved-request mappers. It has its own tsgo/bun test runs with no apps/ on its path, and biome.json rules enforce no framework imports.
- P12 D17 is an audit naming every couple that would block splitting Api into a standalone app; it is not a restructuring.
- internal/bridge stays one Go package holding both modules' services (verdict: Accepted). It has one file per service and the six Api files import nothing from the thirteen Studio ones; splitting would rewrite every bound method FQN (mockRuntime.ts BRIDGE_PKG and 136 FQN_SUFFIX_BY_IPC_KEY entries) for no boundary beyond what Go's internal/ rule gives.
- appcore.Deps carries Studio's Connections/Tree/Router into Api services and Api's ApiVars into Studio's ConnectionsService (verdict: Accepted, documented). It is embedded by value in twenty-one bound services as of P127; a standalone Api app would need only Deps{DB, Repos, ApiVars, Events}.
- apps/kira-studio/internal/storage/{model,repos} carries both modules' tables and imports httpclient/postman (model.ResponseHistorySnapshot embeds httpclient.Response by value; repos.CollectionsRepo speaks postman.Tree/Item). This is the real blocker to splitting Api out; the split is a five-constructor, every-repo-test change worth doing only once a second host exists.
- Planned storage split shape (not created): model/{collections,variables,grpc,responsehistory}.go and repos/{collections,variables,response_history,grpc_history}.go move to a new apistore package; repos.Repos embeds *apistore.Repos for its four Api fields; postman imports apistore instead of model; model stops importing httpclient.
- adapterhost.Host.RunOp is the Api module's op scheduler by design: one op log avoids a second dead ring on every request tab or a second useRunState/ops store, and bridge/http.go and bridge/grpc.go pass ConnectionID: nil into the same scheduler every DB adapter uses.
- ConnectionsService.SecretsStatus is the Api module's only call into a Studio service (used by api/VariableSetView.vue's OS-keychain check). It reports a process-wide platform fact and lives on ConnectionsService only historically; the proper fix is a separate SecretsService, a new bound method out of scope for P12.
- No Go domain package may import internal/bridge. Repo-root internal/ipcerr (the error type every bound method returns) was moved out of internal/bridge/ipcerr so connections, secrets and tree no longer depend on the IPC transport layer.
- apps/kira-studio/internal/layering_test.go's TestDomainPackagesDoNotImportBridge runs go list -deps against the named domain packages so imports into internal/bridge/... fail a test. Kira Space has its own apps/kira-space/internal/layering_test.go, and both use the shared helper internal/layeringtest.
- The layering tests do not cover repo-root internal/ packages (e.g. internal/harness, internal/terminal), because Go's internal/ visibility rule already stops them from importing either app's internal/bridge.
- api/reveal.ts's runReveal is the single four-outcome switch (revealed | cancelled | confirmation-required | error) with a recurse-once-on-confirmation shape, shared by revealVariable, revealHistoryEntry and the Copy as curl reveal loop. The Copy as curl loop calls revealVariable rather than hand-rolling its own copy.
- project/ConnectionDialog.vue keeps its own copy of the gated-reveal loop on purpose, because sharing it would need a home both project/** and api/** may import, i.e. a new Studio↔Api shared module, created by the phase whose job is removing couplings between them.
- packages/api-core/src/http/escape.ts is the one exported goQueryEscape, imported by both curl/generate.ts and raw/generate.ts, which now live in one package's src/http/ tree.
- api/state/history.ts's createHistoryQueries factory is shared by the HTTP and gRPC history views; HTTP's compare-selection extra is carried in a generic Extra type parameter and gRPC passes none.
- mergedValuesAndSecrets and collectionIdFor live in api/state/variables.ts and api/state/collections.ts, because views/grpcrequest/** may not import views/httprequest/**, and both view directories already import api/state.
- Decision D18: the four insert-then-trim capped history tables (filter_history, api_variable_history, api_response_history, grpc_call_history) do not share a Go helper, because it would compose SQL from table/column/cap strings, which was already rejected for index-usage reasons. They also differ in what they trim on (count vs. count+bytes) and whether they run inside an outer transaction.
- api/ApiDialogs.vue mounts the api module's six dialogs, each gated by its own store's open flag; it is a template-only wrapper, so App.vue imports one thing instead of eleven.

- No standalone api-ui package exists; shared UI was extracted (P98/P104/P110) into packages/theme (shadcn-vue components/ui/*, Tailwind tokens, CodiconIcon) and packages/workbench (shell components, Pinia store factories, editor bootstrap, test harnesses).
- The Api module's own UI (ApiDialogs.vue, request/response views, api/state/) stays in apps/kira-studio/frontend/src/api/ because only Kira Studio hosts an Api module, so no second app justifies a shared api-ui package.
- Monaco's editor theme lives in packages/workbench/src/editor/monacoTheme.ts; CodeMirrorHost.vue was deleted at P60b and replaced by editor/MonacoHost.vue.
- An HTTP request body is one of six modes in httpBodyModeSchema (packages/shared/domain/http.ts): none, raw, code, urlencoded, formdata, file; there is no GraphQL mode.
- GraphQL support was deleted entirely by a product decision (no query/variables state, no GraphQL StreamLanguage, no server-side envelope builder); raw is plain text only and JavaScript/JSON/HTML/XML became the top-level code mode with its own codeLanguage field.
- On Postman import (internal/postman/body.go), a raw body with options.raw.language absent or "text" becomes raw; one of the four known languages becomes code with that codeLanguage; an unrecognised language degrades to plain raw instead of failing, with the original language kept in origin_json.
- A Postman environment export imports as an app environment (secrets encrypted, disabled values skipped, globals refused). CollectionsService.Import takes several paths and imports each file independently, reporting one entry per file.
- Postman exports only shared variable values; an unshared (local) value arrives as `"value": ""`, which the app cannot recover. Import keeps the key with an empty value and reports the `empty_values` warning (collections and environments).
- Collection `variable[]` and environment `values[]` share one entry decoder (`internal/postman/parse.go` `decodeVariableEntry`): scalar values keep literal text, object and array values import as compact JSON, `enabled: false` or `disabled: true` skips and counts the entry (`disabled_variables`), and a missing or non-scalar key imports as `unnamed_<n>` (`unnamed_variables`). Legacy promotion in `repos/variables.go` calls the same `postman.VariableValue`.
- A Postman item with neither `item` nor `request` imports as an empty GET request (`missing_request`); only `item[]` and variable entries that are not JSON objects are skipped.
- Import also accepts the Postman API envelope, one level only: `{"collection":{info,...}}` and `{"environment":{values,...}}`.
- On Postman export, language "text" is written explicitly for raw bodies, so Postman shows the Text sub-selector instead of relying on a default this app does not control.
- A Postman graphql body imports as code/json carrying the GraphQL-over-HTTP envelope ({query, variables, operationName}), because refusing the file, skipping the item or importing as none would yield a request that silently does nothing instead of staying runnable.
- An untouched imported graphql body round-trips losslessly (export re-emits mode graphql verbatim); once the user edits it, it exports as raw with language json, and the import report says so.
- Collections created by this app, and collections imported from Postman and re-exported untouched, round-trip losslessly (modulo object member order); an imported, edited collection does not, because edited members are rebuilt in canonical form and drop unmodelled data (e.g. query param description/disabled, header description, form-data row description).
- tests/unit/go-ts-vocabulary-parity.spec.ts guards Go/TS drift of body mode and Content-Type vocabularies (reading validBodyModes in internal/httpclient/body.go as text), and a further pair compares postmanCodeLanguages in internal/postman/body.go against CODE_LANGUAGES.
- Without the postmanCodeLanguages parity check, a fifth language added on the TypeScript side would make the importer treat its bodies as plain raw and the exporter stop emitting it, with nothing failing.
- Body builder labels: formdata reads form-data, urlencoded reads x-www-form-urlencoded, file (one local file sent as the entire body) reads binary, and code reads Code.
- Go applies a default Content-Type only when the request carries none (text/plain for raw, the code language's type, application/x-www-form-urlencoded, generated multipart/form-data with boundary, and no header for a binary body); a hand-set Content-Type always wins.
- A user-typed bare multipart/form-data Content-Type with no boundary gets Go's generated boundary appended, because the user cannot know the boundary and the header and body must agree.
- The legacy P2 bodyMode 'json' is preprocessed by httpRequestTabStateSchema into bodyMode 'code' with codeLanguage 'json', moving saved text from the old body field into code's buffer, since raw and code are separate buffers and a rename alone would strand the text.
- A file's bytes reach Go by path only, never through the renderer or control plane: the native file dialog (FilesService.ChooseOpen) returns {path, name, size}, only path rides the send args, and Go re-os.Stats and streams the file from disk.
- {{name}} substitution resolves collection variables and named environments in URL, header names and values, and the active body mode's text fields, but not a form-data file row's path or the file body's path, which are os.Stat-checked at send and came from a native picker.
- The substitution engine (find {{, next }}, trim, look up, else pass through literally, one pass, no re-expansion) is deliberately implemented twice, in packages/api-core/src/http/substitute.ts and internal/apivars/resolve.go, rather than via a template library.
- A template engine like Handlebars or Mustache is not used for substitution because it HTML-escapes by default, corrupting JSON/XML bodies, and its render-or-fail contract cannot produce the per-reference report (resolved | deferred | dynamic | unknown) the unresolved-reference chip needs.
- The Go and TS substitution implementations are pinned to identical behaviour by one shared JSON corpus, internal/apivars/testdata/substitution.json, read by both a Go test and a TS unit test.
- {{$name}} dynamic values are P6's own extension of stage 1 substitution; Go is not involved in generating them.
- substitute.ts's resolve() takes one optional fourth argument, dynamic?: (name: string) => string | null, consulted only inside the existing $-prefixed branch, so no scanning changes.
- The send path supplies loadDynamicGenerator() from packages/api-core/src/http/dynamic/catalog.ts as the dynamic resolver; the live-preview unresolved-reference chip never does.
- Never generate dynamic values in the live-preview chip, because it is a computed re-running resolution on every keystroke and would mint a fresh UUID for {{$guid}} that the send would never use.
- A recognised $name is generated per occurrence, not per send, so two {{$guid}} references in one request yield different values, matching Postman; the app has no pre-request script for reusing one value.
- A recognised dynamic $name is classified resolved, the same as a stored variable; nothing downstream distinguishes them.
- An unrecognised $name (typo, omitted Postman name, or argument form like {{$randomInt:1,100}}, since dynamic variables have no argument syntax) is left verbatim and classified dynamic, as Go's Resolve $ branch does; a send never fails over it.
- The dynamic-variable vocabulary is 58 names using Postman's $name spellings, and is deliberately finite.
- DYNAMIC_NAMES in catalog.ts is an eager const tuple with no faker import; each name maps to one @faker-js/faker@10.6.0 call in generators.ts, the lazy half reached only through a dynamic import().
- A dynamic name is included only when exactly one faker call produces it and it is plausibly useful in a URL, header or body; about 45 of Postman's 100+ names are deliberately excluded.
- Faker 10 lacks internet.color and location.streetName, so they cannot be used for dynamic name mappings.
- $timestamp and $isoTimestamp read the clock (Math.floor(Date.now() / 1000) and new Date().toISOString()) instead of calling faker, because Postman defines both as the current time.
- tsc keeps the dynamic vocabulary and dispatch in step: dispatch is typed Record<DynamicName, (f: Faker) => string> with DynamicName derived from DYNAMIC_NAMES, so a mismatch between tuple and record is a compile error.

- P6 (dynamic values) adds no Go file, migration, bound method, bindings regeneration or packages/shared change; a dynamic value is a renderer-only computation over a root package.json dependency; internal/apivars/resolve.go and testdata/substitution.json are unchanged from P5.
- Resolve's existing $ branch leaves a $name token verbatim and classifies it KindDynamic; this is correct for any $name stage 1 could not generate (a typo or an excluded Postman name), the same leave-literal treatment P5 chose for a secret whose decrypt fails.
- A generated dynamic value reaching op_log.command is not a leak, because SetCommand receives the post-stage-1 URL and the value was never a credential.
- Variable resolution is split in two stages because Go cannot reach the renderer-only @faker-js/faker dependency for {{$dynamic}} values, and resolving everything in the renderer would put secret plaintext in the renderer store (the bug P14 fixed for connection passwords).
- Stage 1 (renderer, send() in views/httprequest/state.ts) resolves every reference except names matching a secret entry, which it leaves verbatim, classifies deferred, and never fetches or holds.
- Stage 2 (internal/apivars.Service.ResolveRequest, called from Send in bridge/http.go) decrypts only the secrets a request's fields actually reference and finishes the remaining resolution.
- In bridge Send, call op.SetCommand with the URL whose secrets are still unresolved (post-stage-1), both before and after the request runs. Run stage 2 strictly after the first call, and never feed its resolved values into anything logged, because op_log.command is persisted.
- curl import and generate (P7) are done entirely in the renderer in packages/api-core/src/http/curl/, with no Go changes; internal/apivars, httpclient, bridge and postman are unchanged from P6.
- curl tokenizing uses shlex@3.0.0 (split/quote) because, of the candidates measured, it alone survives escaped-newline continuation, empty arguments, ANSI-C $'…' quoting, un-expanded $VAR and untouched {{token}} intact, and it also handles generation-side escaping.
- The curl flag table (flags.ts) and walk (parse.ts) are hand-written because published curl parsers mis-parse the commonest paste shapes and cannot know the app's none|raw|code|urlencoded|formdata|file body-mode vocabulary.
- parseCurl turns argv into a Partial<HttpRequestTabState> patch plus a closed CurlWarningKind union (mirroring internal/postman's warning-kind shape), and selects body mode from the request's effective Content-Type as internal/httpclient's buildBody default table does.
- curl import opens a new 'http-request' tab (openHttpRequestTab + patchHttpRequestTabState) and never overwrites the tab the user may be editing.
- toCurl takes an already-resolved request with no {{ }} awareness and emits only client defaults that change server interpretation (-L, the mode's default Content-Type), never ones that only identify the client (User-Agent, request deadline).
- curl parse produces tab state, which model.TabRecord.State already treats as an opaque json.RawMessage, so no Go is needed; generation needs the renderer's dynamic-value generator and the reveal gate.
- curl generation is deliberately not done in Go, because a Go-side generator would hold a fully-resolved credential-bearing request one careless op.SetCommand or slog.Info away from writing a decrypted credential into kira.db; the renderer has no op, persisted column or log sink.
- Response headers display alphabetised rather than in received order because Go's http.Response.Header is a map with canonicalised keys, so internal/httpclient cannot recover original casing or order; this is a known net/http property, not a bug.
- Response.Headers is a []Header{Name, Value} sorted by name with one entry per value, so duplicate headers such as multiple Set-Cookie survive.
- P9 measured and declined lifting the header-order limitation, because recovering received bytes needs byte-level capture, which changes the negotiated protocol or is unavailable for proxied or HTTP/2 connections.
- P9's raw view renders from the real *http.Request/*http.Response: exact for the request half and an explicitly labelled reconstruction for the response half, because genuinely captured wire bytes are not available at an acceptable cost.
- sharedClient's transport already speaks HTTP/2 to most real HTTPS hosts, because onceSetNextProtoDefaults bundles it automatically when TLSClientConfig is nil and no custom dialer is set; many sends therefore have no HTTP/1.1 wire bytes, only HPACK frames.
- Installing a dialer or connection tee capable of capturing real bytes forces ForceAttemptHTTP2: false or silently downgrades negotiation to HTTP/1.1, changing the thing being observed.
- Behind an HTTP proxy (http.ProxyFromEnvironment), connectMethod.scheme() reports the proxy's scheme for a proxied HTTPS target, so a custom TLS dialer is never consulted and a conn-level tee yields ciphertext or nothing.
- Pooled connection reuse interleaves two exchanges on one tee, which would need a third mechanism (httptrace installation, per-connection sink swapping) to de-interleave.
- The raw view's request half uses httputil.DumpRequestOut(httpReq, false) called immediately before sharedClient.Do in client.go's Send; it was measured byte-identical to a real teed wire capture for duplicate headers, a Host: override, and the transport's Accept-Encoding: gzip.
- The raw view's request body is composed separately in internal/httpclient/wire.go, capped at 128 KiB and elided for file/formdata payloads; the header's Content-Length is always the real count, never the elided text's length.
- The response half of the raw view has no exact stdlib equivalent: httputil.DumpResponse reconstructs from the canonicalised *http.Response, alphabetised, and for HTTP/2 writes a status line over HTTP/1.1-style header lines that never existed on the wire.
- Every raw rendering carries a fidelity value: exact (HTTP/1.1, no proxy), http2, or proxied (a proxied plain-HTTP request's absolute-form request line differs from the dump). It is computed from resp.ProtoMajor and the same http.ProxyFromEnvironment call the transport makes.
- P10's timeline measures the send with net/http/httptrace; phase timing is real regardless of framing, so it does not share the raw view's fidelity problem.
- The timeline collector in internal/httpclient/timeline.go installs its httptrace.ClientTrace on the send's context only after httputil.DumpRequestOut has run and right before sharedClient.Do, because the dump issues its own throwaway RoundTrip on a fake in-memory connection that would fire the hooks as if real.
- Response.Timeline (Hops []TimelineHop, TotalMs) buckets trace hooks by checkRedirect, since net/http builds each redirected request with the original context and checkRedirect's call falls between one hop's last trace event and the next hop's first.
- Timeline hops are not bucketed on GetConn, because a transport that finds a pooled connection dead mid-hop retries with a second GetConn/GotConn pair without a redirect, which would invent a phantom hop.
- Response.Redirects (from P2) is now derived from the timeline collector via redirectHops() rather than collected separately, so the two views of one redirect chain cannot disagree.

- Each hop's five timeline phases (dns, connect, tls, wait, download) are nullable *Phase values, never a defaulted zero, because an absent phase and a zero-duration phase are different facts.
- A reused pooled connection fires no DNSStart/ConnectStart/TLSHandshakeStart hooks, so dns, connect and tls are nil rather than instant; sharedClient is package-level so this reuse happens.
- A literal-IP URL has only dns nil (connect is real), and a plain-http:// URL never has a tls phase.
- The wait phase (WroteRequest to GotFirstResponseByte) stays nil when WroteRequest fires after the first response byte or never fires (e.g. a 1xx or early rejection during a large upload), rather than reporting a negative or huge interval.
- The five phases are never summed to claim a hop's total; the leftover (notably a proxied request's CONNECT-tunnel round trip, which has no hook) renders as a labelled, unattributed residue.
- A hop's own response headers are capped at 8 KiB and visibly truncated (headersElided), so an adversarial server's large MaxResponseHeaderBytes allowance is not copied into Response and kira.db history.
- The timeline does not degrade under HTTP/2 or behind a proxy because the httptrace hooks fire regardless of framing, so Timeline carries no fidelity value.
- Behind an HTTP proxy, TLS handshake timing measures the real end-to-end handshake through the tunnel, while ConnectStart/Done measure the dial to the proxy rather than the origin.
- TimelinePane.vue draws each hop as a static five-segment waterfall bar plus residue using the existing --kira-conn-* palette, with no new colour token or charting library, and is never animated or shown while a send is in flight (LAW 12 governs moving indicators).
- httpclient.Response.Wire (*WireExchange, json wire,omitempty) is live-only: ResponseHistoryRepo.Record sets resp.Wire = nil before marshalling, so stored snapshot_json has no "wire" key.
- Selecting a past response and switching to Raw shows an empty state naming that the rendering is live-only, because the wire rendering would double a snapshot's size for a pane that cannot be opened from a stored entry.
- bridge/http.go builds a strings.Replacer from the secret names and values internal/apivars.ResolveRequest returns, and applies it to Wire.Request and Wire.ResponseHead, since a response header such as a redirect's Location can echo a secret.
- The raw pane's masking follows the same posture as the Copy as curl dialog (secrets masked by construction, no separate reveal gate); users needing real values are pointed to Copy as curl.
- Before P10, Response.Redirects[].URL and Response.FinalURL were persisted to kira.db unmasked, so a secret in a query string (?api_key={{token}}) reached the database in plaintext; P10 closed this.
- maskWireSecrets in bridge/http.go was widened to maskSecrets(resp, usedSecrets), applying the replacer to Wire.Request, every Timeline.Hops[].URL and Hops[].Headers[].Value, and Redirects[].URL/FinalURL.
- Secret masking via strings.Replacer can only over-mask (a secret's literal value elsewhere is masked too), never under-mask.
- A failed send's partial timeline is masked by maskSendErrTimeline, called inside the closure that still has usedSecrets in scope, because mapHttpError downstream does not have them.
- On transport failure httpclient.Send still returns (Response{}, err), but err (*httpclient.Error) now carries a Timeline closed at the failure point, with DNS/connect/TLS phases filled and no status, wait or download.
- ipcerr.Error gained an optional Details json.RawMessage field (omitempty), the first structured-detail channel in a bound method's error; other producers' JSON is unchanged. control.ts unwrap() reads details, and TimelinePane.vue renders the partial hop with a failure sentence naming the phase the request never got past.
- The failed-send timeline change touches the error envelope every bound method returns, for one caller, and was landed as its own commit so it is droppable without affecting the successful-send timeline.
- The raw editor is a third representation of tab state, parsed and generated entirely in renderer TypeScript like P7's curl import/generate, and it is pre-substitution by design.
- packages/api-core/src/http/raw/generate.ts builds a raw HTTP/1.1 text buffer from HttpRequestTabState, with {{base_url}}, {{token}} and {{$guid}} left literal, because a post-substitution buffer could not be edited (applying it would overwrite variable references with resolved values).
- packages/api-core/src/http/raw/parse.ts is a hand-written raw-text parser that accepts {{base_url}}/v2/orders as a request-target and preserves header name case and row order verbatim, unlike a conformant HTTP parser; P7 D1 declined published curl parsers for the same reason.
- EditRawRequestDialog.vue parses the buffer once, on Apply, and patches the current tab rather than a fresh one, which is the deliberate difference from Import from curl, since the open request is being re-authored.
- After raw-editor Apply the tab is an ordinary tab, so send() applies its usual two-stage substitution to any {{name}} references in the hand-edited text; there is no second send path.
- A formdata/file body has no text form and is refused by the raw editor (disabled button, named tooltip) rather than generating an elided body the parser would take literally as bytes to send.
- Past responses (P8) are browsed by swapping a source object: ResponsePane.vue's rendering is a pure function of one response object, and response now reads a selected history entry's snapshot before falling back to the live one, so no consumers changed.
- History adds a third Body · Headers · History segment to the response pane.
- Selecting a history entry switches the response pane back to Body and shows a note-tone banner naming the entry's time and method/URL, with Back to latest (or Close for a restored tab with no live response), so an old response is not mistaken for a fresh one.
- Two history storage notices sit beside the transfer-truncation strip, since the booleans mean different things: "only the first 256 KB of this response was kept in history" and "this response's body was binary and was not kept — N bytes".
- A restored tab does not auto-load its latest history entry; it shows the empty state plus a line "N past responses · View history", to never imply an exchange happened when it did not.
- The history runtime (useHttpHistoryStore in views/httprequest/history.ts, a Pinia store over createHistoryQueries in api/state/history.ts) is never persisted (P2 D6); only the pane choice persists, since a pointer to a response is no more persistable than the response.
- History refresh is eager only while the History pane is showing (a send elsewhere just flags the list stale), and one unconditional fetch happens on every mount, so a restored tab can show "N past responses" before the pane is opened.
- collectionsList and variablesListEnvironments are TanStack Query entries in api/state/apiQueries.ts, cached and invalidated rather than fetched once per mount.

- useResponseBody formats a received body once, with format detection and pretty text from one parse, in a Web Worker (prettyBody.worker.ts) for bodies over 64 K chars and inline below that.
- Pretty text is retained only while Pretty view is selected; Raw drops it and switching back re-requests it.
- In Pretty view MonacoHost is not mounted until the formatted result arrives; a muted "Formatting response…" line shows only after 200 ms.
- MonacoHost's pending pre shows at most 16 384 chars, because a full-doc pre forced a layout of the whole wrapped text inside editor.create's size measure, the profiled hot spot (12 MB: nothing painted, RSS ~2.7 GB).
- The Monaco model is created empty then filled: setValue, or for read-only docs over 256 KiB line-aligned 256 KiB applyEdits chunks a frame apart (editor/chunkedText.ts), because one setValue of an 8 MB pretty body blocked 1.1 s; read-only external writes keep no undo copy.
- Default api.maxResponseMb is 5; stored explicit values are kept, with no migration.
- Parse or format of input that can exceed 64 K chars (INLINE_CHARS in workers/parse/client.ts) goes through workers/parse/: one lazy app-wide module worker, one job in flight, FIFO queue, AbortSignal cancellation (a running job with waiters is terminated and the worker respawned).
- A parse worker failure never fails a job: queued and in-flight jobs run inline through the same handlers.ts.
- Callers use useParseWorker().run/runLatest, and parseInline at or under the threshold so small inputs apply in the same tick.
- Parse worker job kinds are body.format, json.beautify, xml.beautify, console.format and ejson.copyAll; add a kind only for a caller that moves.
- A caller migrated to the parse worker drops its result when the buffer changed since the job started.
- Large read-only editor pushes go through editor/chunkedText.ts (textChunks, pumpChunks); an editable Beautify/Format write stays one pushEditOperations (one undo entry), so its Monaco write remains a block (5 MB JSON: ~1 s).
- The parse worker is never terminated while idle, because a parked worker holds no input and a respawn costs ~10 ms, so an idle timer buys nothing measurable.
- Some operations were not moved to the worker because they measured under 50 ms in WebKit (docs/PERF.md): response find, response compare, cell editor detect/validate/beautify at 64 KiB, row menu copies, Mongo lint scan, EXPLAIN parse, grid Copy as JSON, Documents row parse and field names.
- Console "Copy all" writes through a ClipboardItem holding the pending text (copyOrReportErrorLazy), so the write starts inside the user gesture.
- ResponseDiffDialog.vue compares entries with Monaco's diff editor (createDiffEditor via the shared loadMonaco()), both sides read-only, with renderSideBySide true and hideUnchangedRegions enabled; Monaco replaced @codemirror/merge in P60a.
- Diff bodies are pretty-printed with beautifyJson/beautifyXml only when both sides are the same recognised format; pretty-printing is lossless, so it never misrepresents the response.
- The diff headers table is reduced to added/removed/changed/unchanged by header name rather than a text diff, because headers are a keyed structure, so it stays correct when servers emit the same set in a different order.
- The Monaco diff editor shares the single lazy monacoEntry chunk that loadMonaco() fetches and memoizes for all Monaco surfaces, fetched the first time any Monaco surface mounts, not on Compare.
- In the diff, A is always the chronologically older of the two selected entries, never by click order; a binary body on either side withholds only the body level, with summary and headers still shown and the reason stated inline.
- Session restore never auto-reconnects: previous tabs reopen but their connections do not, and a restored tab shows only a centred "Reconnect & load" button until pressed.
- The reconnect gate is built on components/ui/empty's Empty, driven by the shared useConnectionGate composable (needsReconnect/onReconnectAndLoad in views/shared/useConnectionGate.ts), used by every view kind including Browse tabs.
- A view with shown data keeps it on a connection drop or session change, read-only behind views/shared/StaleDataStrip.vue; state/staleData.ts (useStaleDataStore) stamps each read with ConnectionState.since, and KEEPS_DATA_ON_DROP in state/tabs.ts lists kinds whose page survives the drop. All seven data views are covered: SQL data view, Documents, KeyValue (tab and Browse preview), Browse, Stream, Definition and Console results. Definition and Console results have no write path, so the strip is their only change. SQS Poll is a fresh read, not a stale-data action. Immediate-write mutators require `isStale` and refuse a stale write (createImmediateMutator).
- PostgreSQL/MariaDB/MySQL/SQLite table writes (add row, delete row, cell edit) are staged in a per-tab pending-change set; nothing reaches the database until Commit, and Preview command renders the exact statements first.
- A staged edit or delete is keyed by its row's primary key, not page position, so it survives paging, sort, filter, projection and Refresh; staging needs the table's full primary key in the page, otherwise the cell is not editable.
- Each staged entry keeps the key it was staged under, so a primary-key edit still addresses the original row and an off-page entry still commits; the toolbar counts all staged entries and names how many are off the current page.
- Each staged edit keeps the page value from when it was first staged, so the preview's Changes view diffs before and after (shared `MonacoDiff`, `valueDiffSides`) even after the row leaves the page.
- Only closing the tab or a successful commit clears the pending-change set.
- ClickHouse tables get add-row only, staged the same way, because there is no addressable row to update/delete (a MergeTree PRIMARY KEY is a sparse index).
- MongoDB/Redis/Kafka/SQS/S3 write immediately, gated per adapter by canInsert/canUpdate/canDelete capability, with no staging or preview.
- The fake-data generator (P15) calls data.mutate directly with MutationRowOp[] batches built from per-column recipe plans, bypassing the pending-change set, because that store renders one un-virtualized DOM row per staged insert, which is wrong for thousands of rows.
- `MonacoDiff` mounts Monaco on an inner `absolute inset-0` host: Monaco's diff root is `height: 100%` and measures itself, so it collapses to 0 under a parent with only `min-height`. Callers size it with a wrapper block, never a height class on the component.
- The cell editor is a panel mounted by whichever view owns the tab (grid, documents, key/value, stream, console), not a global singleton; it appears only while the tab has a selected cell, and views like definition tabs never mount it. Its Show changes toggle diffs the buffer against the stored value (`SelectedCell.original` under a staged edit), using the shared `MonacoDiff` and `valueDiffSides`.
- Redis and S3 are navigated only in a dedicated browse tab, one lazy level per screen over SCAN/ListObjectsV2, never as a recursive tree; their project-tree node is a leaf, and only the Up button and breadcrumb move between levels.
- The grid, document, key/value and console-result views share the createPageSearch factory and runChunkedScan scanner (views/shared/page/) rather than separate find logic.
- A chunked scan can take an optional priority window (rows currently on screen) scanned first, so find on a large loaded dataset highlights visible matches before continuing the ascending pass in the background.
- Every data view (SQL grid, documents, keyvalue, stream, console, definition) shares one toolbar-mounting shape: badges and PK chip alongside the header, DataToolbar/FilterToolbar in the toolbar rows.
- P104 (§3) deleted ViewChrome and ViewHeader because there is no components/ui/reka counterpart for a bespoke layout container, and inlined their markup at each call site over components/ui parts and Tailwind utilities.
- The shared view chrome look is now a convention each view's own template repeats, not a component it mounts; the logic beneath stays shared.
- The per-tab runtime store (createRuntimeStore) owns setActionError, toggleSearchOpen and setSearchOpen, so views do not re-implement them.
- Saved queries and filter history are cached with `staleTime: Infinity`. Go emits `kira:savedQueries:changed` ({connectionId, kind}) after each Save, SaveConsole, SaveDiscover, Update, UpdateDiscover, Delete and Touch, and `kira:filterHistory:changed` ({connectionId, path}) after each recorded filter, both with Emit; `state/savedQueriesSync.ts` invalidates the matching menu keys. The tree's saved-filters submenu still reads on right-click.
- Studio tab views unmount on every tab switch (no keepMounted). Per-tab view UI state (filter inputs, open flags, find query and matches, unapplied filter-row drafts) lives in the view's runtime store, session-only and cleared on tab close, never in a component ref. An open find bar keeps its scan across a switch and rescans only when its page changed; closing the bar clears query and filter-rows toggle (P375).
- views/shared/viewOp.ts's beginOp/applyLoadFailure are the one load-op preamble and load-failure tail behind every view's load() except Browse, which supersedes by loadSeq, not opId, and has no E_CANCELLED branch.
- views/shared/page/store.ts is the one two-level page-cache implementation behind all five page modules (grid, documents, keyvalue, stream, console).
- Visible-window pruning of the page cache is wired into the view side of all five page modules (P5 C3); documents also prunes the parsed-tree cache in views/shared/document/rows.ts to the same window.
- views/shared/immediateMutation.ts is the one write body behind the ten immediate-mutation functions of documents, keyvalue and stream.
- views/shared/page/PagerControls.vue is the one pager behind both the SQL grid and the document list.
- views/shared/document/DocumentRow.vue is the one Mongo document-row component behind both the document view's editable row and the console's read-only copy.
- views/shared/page/columns.ts's columnHeaderTooltip, GUTTER_WIDTH and DEFAULT_COLUMN_WIDTH are the single column-header tooltip and layout constants shared by the grid and the console's tabular result.

- The SQL console language service reads one layered DdlSchema (effectiveSchema in state/schemaColumns.ts). Precedence: user-pasted DDL document (connection_ddl) wins wholesale once it declares any table; otherwise the P22c schema column metadata cache; otherwise bare relation names from the tree's loaded node cache.
- Only when the DDL document, the schema-column metadata cache and the tree relation names are all empty does a console fall back to bare keyword completion from packages/shared/domain/sqlKeywords.ts (views/console/sqlKeywordCompletion.ts).
- At a connection's root console (path === ''), rootContainerPathFor/rootRelationNames resolve the single container the console runs against, or union the containers already loaded in the tree, from data already in the renderer with no new round trip.
- The language layer never issues schema introspection itself; every fetch is triggered by a view's lifecycle hook (tab activation, tree expansion), never from a CompletionSource.
- P60b replaced @codemirror/lang-sql's per-dialect Lezer parser and vocabulary with a hand-rolled tokenizer (packages/shared/domain/sqlTokens.ts) and sqlKeywords.ts, when the dependency was removed app-wide.
- EXPLAIN is sent as an ordinary query through the existing data:execute op, with the statement text composed in the renderer (views/console/explain.ts); it needed no new Adapter method, Caps field or wire.fbs change.
- Each dialect's EXPLAIN returns an ordinary TabularPage: Postgres/MySQL/MariaDB one row of JSON text, SQLite N rows of (id, parent, notused, detail), ClickHouse a JSON-plan page plus a second EXPLAIN ESTIMATE page; views/console/planParsers/*.ts normalizes them into one QueryPlan tree.
- The expensive-query threshold (advanced.expensiveQueryRows) is an estimated-rows-read number, never a cost unit, because two of five dialects report no cost and MySQL and MariaDB disagree by three orders of magnitude on cost for a comparable scan.
- Never issue EXPLAIN ANALYZE (or an equivalent) for auto-explain, because it executes the statement; plain EXPLAIN is roughly three orders of magnitude cheaper, measured against a real Postgres server.
- Settings dialog edits are staged in a local per-dialog draft and committed as one patch on Save; SettingsDialog.vue is the only caller of patchSettings, and Save still broadcasts app-wide to every window.
- Settings reverting is per-leaf: each of the nine editable leaves has a discard icon button, disabled when the draft equals that leaf's default, driven by generic isAtDefault/resetLeaf helpers; a reset only stages the default into the draft until Save.
- HTTP requests are logged in the existing op log as the connectionless op kind 'http' (HttpService.Send calls Host.RunOp with ConnectionID nil), so the toolbar progress ring, useRunState and the Operations panel Cancel button work without a second log.
- A non-2xx HTTP response is logged with status 'ok'; only a transport failure, a timeout or a cancel is 'error'/'cancelled'. The outcome is stored in op.SetCommand as the method and URL, overwritten with '→ <status> <statusText>' once the response is known.
- connections.throttle_per_sec (0 = unlimited, default) is a first-class column rather than an options_json key, because a rate limit is a safety setting that pasting a connection URI could silently remove from options_json.
- The throttle registry (internal/adapterhost/throttle.go) keeps one golang.org/x/time/rate.Limiter per throttled connection under its own mutex, never Host.mu, with burst max(1, min(10, round(perSec))).
- Host.RunOp is the single throttle gate: read/count/mutate/execute/transfer/children/describe/definition ops wait for a token after registration (stay cancellable) but before op:start (DurationMs excludes queue time); connect/disconnect/test are never gated and cache hits are never throttled.
- A throttle wait is bounded at 30s and expires into E_TIMEOUT; cancelling a queued op yields E_CANCELLED; no new ErrorCode is added and neither op:start nor op:end is emitted for a queued op that is cancelled or times out.
- The throttle limiter is installed just before Connect, replaced live on Update of a connected connection (no reconnect needed), and cleared in Disconnect.
- Row coloring (appearance.rowColoring, default on) is a per-column text colour derived from each column's typeClass via categoryForTypeClass, emitting tc-* classes themed in slickTheme.css; no background, stripe or hash. String-typed cells get no distinct colour, and turning the setting off removes colour entirely.
- The renderer deliberately runs Vue in VDOM mode; Vapor mode was declined in P6 because hot paths already bypass VDOM diffing via the no-reactivity-on-row-data invariant, and partial adoption would ship both runtimes for one component. A Vue 3.6 upgrade keeps VDOM mode.

### Copy and i18n (P265)

- All user-visible text lives in one catalog, packages/theme/src/i18n/en.json; vue-i18n 11 (Composition API, JIT compile, no eval) is the runtime.
- English ships; a new locale is a second file next to en.json, and Go reads the same file for native text.
- t(key, params?) is a global typed function; MessageKey is every leaf path of en.json, so an unknown key fails vue-tsc/tsgo. A numeric params.count formats with locale digit grouping and selects the plural branch.
- Components must import t from '@theme/i18n'; never use $t, and never use useI18n() outside the i18n folder (U68).
- CatalogText puts an element inside a sentence via named slots, so a sentence never splits across literals. Import it from '@theme/i18n/CatalogText.vue', not the index, because the index must stay free of .vue so workers and Node test code can import it. It needs app.use(i18n).
- app.use(i18n) runs at every createApp site: Studio, Space, mobile, git-ui mount(), bootstrapShell, Monaco review zones.
- Never call t() at module scope; it runs at render or call time (U70). In hot paths (SlickGrid formatters, virtual rows, Monaco decorations) resolve once per setup or render pass, never per cell.
- Never translate values that drive logic (enum values, status ids, persisted strings, testids); translate only the label shown for them.
- In test builds (Studio and Space, under __KIRA_DEBUG_HOOKS__), installCopyTestHook() sets window.__kiraSetLocale('en' | 'pseudo'); pseudo wraps each message as ⟦…⟧. Mobile has no test hooks, so the static guard covers it.
- packages/workbench/src/testing/ui/copyCoverage.ts (setPseudoLocale, expectCatalogText) fails on visible letters outside a pseudo-locale marker.
- Catalog keys follow <namespace>.<area>.<surface>.<element>.<role>, camelCase segments, depth 3-6, named for meaning and never for the English text.
- Key namespaces are: automations, backend, common, docker, format, git, memory, mobile, shared, space, studio, theme, workbench.
- Key roles are: label, tooltip, aria, placeholder, title, option, status, announce, confirm, body, hint, description, emptyHint, error, empty, loading, example.
- Confirm dialogs use the key group <surface>.<x>Confirm.{title,body,confirm}. Identical text is one key (C3). Shared vocabulary goes in common.*, or <namespace>.common.* inside a module; format.* holds behaviour/format.ts word templates.
- Use sentence case everywhere (capitals only for first word, proper nouns, acronyms); the native macOS menu (backend.menu.*) is title case.
- Use the single character … on an action only when it opens a dialog or picker needing more input. Loading text always ends with …; placeholders never do.
- No trailing period on labels, buttons, menu items, tooltips, titles, placeholders, options, status words or announcements. Sentences (body, hint, description, emptyHint, error) end with a period. Confirm titles end with ?.
- Use typographic quotes “ ” around a named instance and ’ in contractions; no straight quotes.
- Use contractions can’t, couldn’t, isn’t; the words cannot and could not are banned. Use British spelling (colour, cancelled, behaviour).
- Copy must not use &, (s), e.g., an Error: prefix, or OK/Yes/No buttons (use the verb). Use ' — ' between clauses and ': ' before backend error detail.
- Counts with a noun are plural messages with {count}; sizes, dates and durations come from behaviour/format.ts.
- Name placeholders in camelCase; a user-provided name in prose is “{name}”. Resolved script variables keep VarText inside a CatalogText slot. Never build a string by concatenating translated parts.
- An icon-button tooltip and aria-label share one key (.tooltip), phrased as the action label with no shortcut text.
- Confirm dialogs: title is '<Verb> <object>?'; body names the instance in “ ” and states the consequence; irreversible actions add common.confirm.irreversible; buttons are Cancel plus the title’s verb.
- Empty states read 'No <items> yet' when the user can create one, else 'No <items>'; filtered-empty is 'No matches'.
- Error copy: action failed is 'Couldn’t <verb> <object>.' or with ': {error}'; a blocked precondition is 'Can’t <verb>: <reason>.'; a QueryState load failure is 'Couldn’t load' plus a Retry action.
- Status words are short sentence-case phrases; raw engine states (Docker running, plan node names) are data and shown as is.

| Concept | Use | Not |
|---|---|---|
| Destroy a stored item | `Delete` | |
| Detach, item survives | `Remove` | |
| Empty a list or field | `Clear` | |
| Abandon a dialog | `Cancel` (also confirm cancel) | `Keep`, `Leave it` |
| End a transient surface | `Close` | |
| Toggle a persistent part | `Show <x>` / `Hide <x>` | |
| Remove a notice | `Dismiss` | |
| Drop unsaved changes | `Discard` / `Discard changes` | `Revert` (git word) |
| Git revert, git reset | `Revert`, `Reset` | |
| Settings to default | `Reset to default` | |
| Persist | `Save` | |
| Apply pending, stay open | `Apply` | `Confirm`, `OK` |
| New thing (menu, toolbar) | `New <thing>…` | |
| Creation dialog primary | `Create` | |
| Append in an editor or list | `Add <thing>` (no `+` prefix) | |
| Import | `Import <thing>…` | |
| Re-fetch | `Refresh` | `Reload` |
| Re-attempt | `Retry` | `Try again` |
| Execute once | `Run` | |
| Long-running process | `Start` / `Stop` | |
| Undo an in-progress git operation | `Abort` | |
| Take over an agent run | `Take over` | |
| Choose in a picker | `Choose` | `Pick` |
| Select in a list | `Select` | |
| Edit | `Edit` (`Edit credentials…`) | `Update` (reserved for app update) |
| Settings | `Settings`, `Open settings` | `Preferences` |
| Copy | `Copy <thing>`, `Copy as <format>`, `Copy for agent` | |
| Copy feedback | whole messages (`Copied name`), `Couldn’t copy: {error}` | noun fragments |
| Git checkout | `Check out` (verb), `checkout` (noun) | `Checkout` as a verb |
| Loading | `Loading…`, `Loading <noun>…` | |
| Empty | `No <items> yet`, `No <items>`, `No matches` | `Nothing to show` |
| Load or action failure | `Couldn’t <verb> <object>: {error}` | `Failed to`, `Could not` |
| Precondition | `Can’t <verb>: <reason>.` | |
| Irreversible | `This can’t be undone.` | |
| Show more / load more | `Show more` (reveal capped items), `Load more` (fetch next page) | |
| Terms | `repository` (not `repo`), `folder` (not `directory`, except `working directory`), `ID`, `for example` | |

- The catalog checker scripts/copy/check-catalog.ts (rules in catalogRules.ts) runs under bun run lint and enforces rules C1-C11 on the i18n catalog.
- Catalog rule C10 flags unused keys (--unused, on in lint) and scans internal/ Go callers for backend.* keys.
- Catalog rule C6 requires sentence case (title case under backend.menu.*); C8 covers plural shape and allows only named placeholders in backend.* messages.
- Catalog duplicate-text exemptions live in ALLOWED_DUPLICATES, which is empty; each entry needs a reason.
- The source guard scripts/copy/check-copy.ts (Babel and @vue/compiler-sfc ASTs) runs under bun run lint and enforces U64-U71: no template text, static text attributes, string literals in template expressions, strings in text properties or announce calls, vue-i18n use outside the i18n folder, concatenated or composed translations, t( at module scope, and t params differing from message placeholders.
- Source-guard exemptions are in scripts/copy/exemptions.ts, one rule for one file with a reason; a stale exemption with no hit left fails. They cover only font names, protocol tokens, command syntax, EJSON syntax and CLI flag names.
- bun scripts/copy/check-copy.ts --all prints every source-guard hit.
- The Go reader packages/theme/src/i18n/catalog.go embeds en.json; i18n.T replaces {name} placeholders (given a map such as map[string]string{"appName": name}) and panics on an unknown key.
- backend.* keys hold native menu text, OS notification and prompt titles, native dialog titles, git undo labels, definition section titles and query plan roots; named placeholders only, no plural.
- Go call sites of i18n.T must pass a literal key and nil or a map literal, because catalog_test.go checks key existence and placeholders for every i18n.T( in the repo.
- Menu text duplicates common.* wording by design, because Go reads backend.* only.
- Add new strings to en.json under the owning module's namespace (common.* if shared), keeping it sorted; biome check --write sorts and formats. Use t('…') in script or template, or CatalogText around elements.
- Specs keep literal expected strings, because the wording is the contract.
- To add a locale, copy en.json to <locale>.json keeping keys, placeholders and plural branches, register it in packages/theme/src/i18n/index.ts messages, add it to CopyLocale, and add a switch in settings.
- Monaco, SlickGrid and xterm need their own locale packs when adding a locale.

### Consistency conventions (P266)

- Consistency conventions are enforced by guards: scripts/check-ui-primitives.sh (U72-U82), scripts/check-conventions.ts (N1-N7), internal/conventiontest (G1-G5), Biome noConsole, golangci sloglint.
- Each convention guard keeps a named exemption list with a reason per entry; an entry that exempts nothing fails, so the lists only shrink.
- The UI uses one glyph set: codicons, referenced by bare names (e.g. trash, not codicon-trash).
- Use simple-icons for third-party brand marks and seti for file types.
- @lucide/vue is allowed only inside vendored packages/theme/src/components/ui/** (guard U72).
- The concept-to-icon table is ICON in packages/theme/src/icons.ts; a concept uses its row everywhere, and a kind of thing (file, engine) uses its kind icon.
- Icon names gear, settings and kebab-vertical are banned.
- No 'codicon-…' string literals are allowed outside CodiconIcon.vue, Spinner.vue, and the theme package (guard U73).

| Concept | Icon |
|---|---|
| Delete (destroys stored data) | `trash` |
| Remove from a list, row or selection; close; clear field | `close` |
| Clear all | `clear-all` |
| Add | `add` |
| Copy / duplicate | `copy` / `files` |
| Edit, rename | `edit` |
| Refresh, search, filter, history | `refresh`, `search`, `filter`, `history` |
| Run, stop, restart | `play`, `debug-stop`, `debug-restart` |
| Reset to default, revert, discard | `discard` |
| Settings | `settings-gear` |
| More actions | `ellipsis` |
| Back to previous view | `arrow-left` |
| Step within a sequence (page, month) | `chevron-left`, `chevron-right` |
| New collection or folder | `new-folder` |
| Environments, variables | `server-environment`, `variable-group` |
| Error, warning, info, success | `error`, `warning`, `info`, `check` |

- IPC errors use the wire shape ipcerr.Error{code, message, details?}.
- The error code registry lives in Go at internal/ipcerr/codes.go (typed Code) and in TS at packages/shared/domain/errorCodes.ts (ERROR_CODES, CLIENT_ERROR_CODES raised only in TS, isRetryable); check-conventions N6 fails when the two differ.
- Generic error codes are E_INTERNAL, E_CONTRACT, E_BAD_REQUEST, E_NOT_FOUND, E_CONFLICT, E_UNAVAILABLE, E_TIMEOUT, E_CANCELLED, E_TRANSPORT, E_UNAUTHORIZED, E_FORBIDDEN, E_READ_ONLY, E_TOO_LARGE, E_UNSUPPORTED, E_RATE_LIMITED, E_DISCONNECTED, E_SECRET_STORE.
- Add a domain error code (e.g. E_CONNECT, E_AUTH, E_QUERY, E_ENGINE_DOWN, E_GIT_*, E_PREPARING) only where a UI branches on it.
- OS errno strings such as ESRCH are not error codes.
- E_BAD_REQUEST means user input was refused, including over-long ADE message, yaml and script inputs (backend.space.ade.messageTooLong, scriptTooLarge, yamlTooLarge).
- E_CONFLICT means a state precondition failed, such as a stale baseHash.
- E_CONTRACT means a caller bug (the UI always sets the value).
- User-visible error messages are catalog text via ipcerr.T(code, key, params) (adapters.T for adapters); raw text from outside Kira (driver, server, git, OS) stays verbatim.
- ipcerr.Contract(format, args…) and ipcerr.Required(field) produce E_CONTRACT with English developer text.
- Message catalog keys use the prefixes backend.studio.*, backend.space.*, backend.core.*, backend.error.*; Go sends non-error text (preview blockers, run notes) the same way.
- Convention check G1 flags a literal message, and G2 flags an "E_…" literal outside codes.go.
- errorText(err) in bridge/codedError.ts shows the error message; E_CONTRACT wraps it in common.error.contract.body.
- Retry is shown only when isRetryable(code); never for E_NOT_FOUND, E_CONTRACT, E_BAD_REQUEST, E_FORBIDDEN, E_UNAUTHORIZED, E_UNSUPPORTED, E_READ_ONLY, E_TOO_LARGE, E_CONFLICT.

| Situation | Surface |
|---|---|
| Load | `QueryState` (pane or inline), in place |
| Dialog action, field-specific | `FieldError` under the field |
| Dialog action, other | `Alert variant error` at top of `DialogBody`; dialog stays open |
| Row or tree action | Row error popover, or the module strip `Alert` |
| Copy or open-link failure | Live-region announcement (`common.error.openLink.announce`) |
| Background call nobody waits on | `fireAndForget(promise, scope)`: logs, no UI |

- In the UI, use bridge/log.ts logWarn/logError instead of console.* (enforced by Biome noConsole).
- Never use bare void control.x() calls (U77) or empty .catch(() => {}) handlers (U81).
- Open links through createOpenExternal (bridge/openExternal.ts, U79).
- Go slog calls must carry a "scope" attribute (G3) and log errors under the "err" key.
- Decode errors in packages/shared/protocol/frame.ts stay thrown Errors because they are internal invariants, shown only as E_INTERNAL detail.
- Bridge API method verbs: List, Get, Create, Update (patch), Save (create or replace from a client draft), Upsert, Set (one value or whole document), Delete (destroys a stored record), Remove (detaches, record survives), Cancel<Thing>, Stop.
- Each bridge method takes one <Service><Method>Args struct or none, and list methods return [] never null.
- Bridge events are named kira:<domain>:<event> with both segments camelCase (N5); gitrpc repo.changed dot names and in-process op:start/op:end are separate namespaces.
- Streaming APIs use a start method, events keyed by id, and a stop method.
- Pagination uses PagePosition plus hasMore; bounded lists use limit plus truncated.
- baseHash is the hash of what the client edited and conflicts are refused with E_CONFLICT; hash identifies a confirmed preview. They are two concepts with two names.

| State | Home |
|---|---|
| User preference | Settings (Go DB leaves, `SettingsService`) |
| App-wide panel geometry | Layout store (Go DB) |
| Per-tab view state | Tab `state_json` |
| Per-device convenience | `usePersistedState`, key `kira.<module>.<name>` |
| Git view state | `ViewStateStore` |
| Files under KIRA_HOME | Atomic write (`renameio`) |
| LLM prompt overrides | `<home>/llm-prompts.json` (`renameio`), see "App prompts" |

- Settings sections and leaves are camelCase and new booleans are positive; api.disableCookieJar keeps its name because renaming it rewrites user data.
- Inside the Settings dialog every leaf is draft plus Save; a control outside the dialog applies at once through patchSettings.
- SettingsShell in both apps orders panes with Appearance first and Advanced last.
- Settings field anatomy is a label row with reset button, the control, FieldError, then description; booleans are Switch.
- Shared settings fields live in packages/workbench/src/settings/fields/ (e.g. FontFamilyField used in both Appearance panes); rowColoring is Studio-only.
- Adding a setting requires: Go model, default and validation; leaf upsert; zod schema and default in state/settingsDomain.ts; pane field; catalog keys; reset button; flow test.
- Motion and touch tokens --kira-motion (150ms), --kira-ease and --kira-touch-target (44px; h-touch, size-touch, min-h-touch) are defined in tokens.css.
- App code must not set duration-*, ease-* or animate-in/out classes (U75).
- prefers-reduced-motion: reduce zeroes transitions and animations in base.css, except Spinner.
- Native scrollbars (base.css, pointer: fine only) keep a 12px classic gutter; the thumb is transparent until the pointer is inside that scroller. Thumb colour rides --kira-scrollbar-thumb, set by :where(:hover) on the scroller and reset on every element, because WebKit never repaints a :hover::-webkit-scrollbar-thumb rule and scrollbar-color/scrollbar-width drop the custom styling. Thumb :active keeps a dragged bar shown. Monaco and xterm keep their own hover-reveal scrollbars.
- QueryState shows its default loading state after TIMING.spinnerDelay (300ms), while Button pending shows its spinner at once.
- The drag ghost class is SORTABLE_GHOST_CLASS (U76). No list sets a chosen class: Sortable's unstyled default keeps a press from flashing a ring.
- Mobile code never hard-codes h-11 (U82).
- EmptyState (theme) is the only Empty composition (U74), with props/slots icon?, title, description?, default slot for actions, and media slot for a custom glyph.
- A side panel empty state gets icon, title and a one-line description with no button (U78); its action lives on the module front door.
- ModuleStart (workbench) is the module front door: icon, title, description, one primary action, optional secondary actions and #below; error or unavailable states may carry Retry.
- The command palette (CommandPalette.vue, palette.ts in packages/workbench/src/shortcuts/) is shared; each app supplies commands with providePaletteCommands.
- Both app menus carry View: Command palette and Find.
- WorkbenchTitleBar replaces per-app title bars; LogLines is the one virtualized log viewer; StatusBar stays per app.
- Shared UI test helpers live in packages/workbench/src/testing/ui/; app-only ones in apps/*/tests/ui/support/.
- Persistence ids are TEXT uuid v4.
- New timestamp columns are INTEGER unix ms named *_at; older TEXT ISO columns stay as legacy, and N7 refuses new ones.
- Migrations are named NNNN_<phase>_<slug>.sql.
- Files a person or another program reads must be written with renameio.WriteFile; exempt are test tooling and temp snapshots (G4).
- G5: no CLI credential access under githost/, issuetracker/, internal/docker, internal/harness/claude, internal/harness/opencode and internal/agentport (see CLI credentials: never touched).
- File naming: .vue files PascalCase; .ts files camelCase with role suffixes .worker, .check; colocated tests are <module>.spec.ts, never .test.ts (N1, N2).
- Name helper folders lib/, never util, utils or helpers (N3).
- Stores live in state/ and workbench composables in behaviour/; a Pinia id is the export name without use/create and Store, camelCase (N4).
- Short SHA is 7 characters (shortSha, Go gitclient.ShortSHA, U82); Docker ids stay 12.
- A list that grows with user data and can pass 200 rows must use useVirtualRows, VirtualList or SlickGrid; plain v-for only when bounded to 200 or fewer by construction.
- A picker section showing a large set caps with useCappedList plus ShowMoreButton (U80).
- Performance budgets: keystroke p50 50ms, scroll p95 80ms, tab switch gate 300ms.

### UI leftover conventions (P267)

- Use text-muted-foreground for row meta, KV keys, secondary labels and values, timestamps, counts, icons, DialogDescription and bar status; muted never uses opacity.
- Use text-subtle for section headings and legends, FieldDescription, empty and inline-state lines, placeholders, NULL markers and row numbers; use text-disabled for absence markers.
- Use cursor-default on every control and row; pointer only for Button variant="link", linkify.ts anchors and the PR badge link; grab on drag handles; never help or copy cursors.
- View or detail titles use text-kira-lg font-medium text-fg; sections use SectionHeading (pane, panel, list, picker) or FieldSet + FieldLegend + FieldDescription (form, settings pane).
- A hint paragraph is FieldDescription; an empty line is InlineState kind="empty" or EmptyState.
- Identifiers (ids, shas, branches, refs, paths) use font-data and are shown through Identifier (short, copyable, split="path"); single-line overflow uses TruncatedText.
- Show counts in a tab, section or row as plain rowMeta; use Badge count only for attention; never put "(n)" in label text.
- Button ghost variant is used only for the DialogHeader close; text actions use the link variant.
- Disclosure (built on reka Collapsible) replaces <details>.
- Loading placeholders wait SPINNER_DELAY (packages/theme/src/timing.ts, TIMING.spinnerDelay) before showing.
- CodiconIcon size must be one of 12, 13, 16, 24; RunState is a Spinner at size 12 or a StatusDot error.
- .kira-prose in base.css is the one prose layer.
- Elevation tiers: Popover z-(--kira-z-popover) + shadow-kira; Dialog, overlay, CommandDialog z-(--kira-z-dialog) + shadow-kira-dialog; DropdownMenu z-(--kira-z-menu) + shadow-kira; Tooltip z-(--kira-z-tooltip) with no shadow; in-pane layers z-1..3.
- TooltipContent has two variants sharing one ground (elevated, ring, shadow, no arrow, sideOffset 4, fade only): `default` (padded, max-w-sm, plain text, `\n` breaks lines) and `surface` (layout only, caller pads) for rich content using the ADE tones, which are dark-only. A branch chip inside a default tooltip renders neutral via `data-chip="branch"`.
- Density scale is 0.5, 1, 1.5, 2, 3, 4; 0.25/1.25/1.75/2.5/3.5 are not allowed anywhere. Dialog width uses size variants only.
- With DialogContent dirty, outside click is ignored, and Esc and the header X ask the discard confirm; footer Cancel never asks.
- Every host (Studio, Space, Space mobile, git-ui mount()) calls provideShellServices(app) from bootstrapShell.ts once per createApp, providing CONFIRM_DISCARD_KEY backed by the confirm store, because theme cannot import the store.
- Each host mounts one ConfirmDialog; mobile passes touch for 44px buttons. Tabs ask before closing a dirty tab (TabKindDef.isDirty).
- DialogHeader has no icon, and a dialog footer has one emphasised button.
- Operation failures use Alert variant="error" in the body; FieldError is only under a field or as a one-line footer status, paired with aria-invalid and aria-describedby (useFormErrors).
- Use Button pending for busy state, never :disabled on a busy flag.
- packages/theme/src/format.ts owns formatBytes (1024 base, KB/MB/GB, space before unit), formatPercent, formatDuration (0.42ms, 240ms, 3.4s, no space), formatCompactDuration, formatCurrency, formatElapsed, formatZonedWeekdayClock and displayValue; formatCount lives in i18n/number.ts.
- formatCompactDuration uses ceil for TTLs and steps seconds, minutes, hours, then days at 24h (e.g. 2d 3h).
- packages/workbench/src/behaviour/format.ts re-exports the theme formatters and keeps the reactive useRelativeTime; t() formats numeric params with formatCount while plural still keys on count.
- Right-click selects (or focuses) before opening the context menu, through nav.contextMenu.
- Every useSortableReorder caller passes keyboard: true; explicit-Save surfaces bind form.save.
- Every debounce, poll, tick and UI delay must be a TIMING key, and sort comparators must end on a stable key.
- Bind shortcuts printed on a surface there; trees use useTreeShortcuts; refresh is F5 only; render cap is 500 via useCappedList.
- In-memory trees (Studio collections, Space repo file tree) persist expanded keys with usePersistedState('kira.<module>.expanded', []); lazy IPC trees (Studio project tree) do not restore.
- UI convention guards are wired into bun run lint: scripts/check-ui-primitives.sh (U83-U103) and scripts/check-conventions.ts (N8-N12), plus check-theme-classes.sh which bans stock shadow-(sm|md|lg|xl|2xl).
- check-conventions.ts N8-N12 check duplicate spec helpers, .ts files under components/, .vue files over 600 lines, printed shortcuts without binding, and state/ dir and menu file naming.
- check-ui-primitives.sh keeps 38 named PERMANENT lines, each with a reason; its built-in exempt column fails when an entry exempts nothing (suffix ! opts a blanket directory out).
- vue-tsc checkUnknownComponents, inside typecheck, catches unresolved components.

### Shared pickers (P398)

- One ProjectPicker and one BranchPicker, in packages/workbench/src/components/pickers/, serve every project or branch choice in Studio, Space and git-ui. Callers feed PickerOption[]; no caller builds its own list, combobox or select.
- Both wrap PickerPopover (trigger, Popover, inline mode) over PickerCombobox (shadcn-vue Command with ignoreFilter). foldOptions in lib/pickerOptions.ts filters, groups and orders; useCappedList plus ShowMoreButton cap a section at 50.
- Test ids derive from the caller's testid: the trigger is testid, then testid-search, testid-option-<id>, testid-custom, testid-error, testid-more, testid-content. A PickerOption.testid overrides the option id.
- Reka single-select toggles on reselect, so pickers act on the select event, never on the model change.
- BranchPicker create offers "Create branch <name>" for a validated new name; custom offers "Use <value>" for any ref or sha. ProjectPicker custom offers free entry for keys outside the list.
- A disabled option shows its reason inline, never hover-only.
- ADE base exclusions ship as codes (self, stacked) in adewire.BasePick.Excluded; the frontend owns the text. Candidate branches load per repo (adeTaskCandidateBranches takes codeRepoId).
- git-ui option builders live in lib/refOptions.ts. Suggested base options use the s: value prefix so the same ref in the branch list keeps a unique value.
- The git-ui toolbar popover (tabs for branches, tags, stashes, worktrees, stacks) is RefsPopover.vue, separate from the shared picker.

## Process model

Two processes for the Studio and Api modules: the **webview** running the Vue renderer, and the
**Go shell** owning the window, all app state, and now every database driver too. **As of P100,
this is the whole of Kira Studio's own process model** — the git module moved out entirely into
Kira Space, a separate app with the identical two-process shape (see Git module, above).

```
┌──────────────────────┐
│  webview             │
│  (Vue renderer)      │
└──────────┬───────────┘
           │
           │  control: generated Wails bindings
           │           (HTTP to /wails/runtime)
           │  data:    one "engine" stream — a held poll/send pair on
           │           desktop, a real WebSocket only in a -tags server build
           │
┌──────────┴───────────┐
│  Go shell (Wails v3) │  window, menus, SQLite, settings, op log, keychain,
│  apps/kira-studio    │  pre-connect, every adapter, cache, metrics
│  main.go + appwire   │
└──────────────────────┘
```

**Why there used to be a separate engine process, and what P58f gave up to remove it.** Driver work
(socket reads, protocol parsing, row decoding) is CPU-bursty; a Node child process kept it off the
shell's window/menu handling and off the webview's frame budget, at the cost of the JSON-inflation
hop below and the packaging/vendoring machinery P58f deleted. P58 (`docs/HISTORY.md (v1 P58)`
D16) traded that process isolation for one fewer process, in-process encoding (below), and no
native-module packaging story — the real loss, not papered over: **an adapter panic used to kill
only the engine child** (`E_ENGINE_DOWN`, every connection errors, the window/tabs/settings/op log
all survive); now every adapter call runs behind a `recover()` at the op boundary
(`adapterhost.Host.safeRun`), which converts a panic into a failed op (`E_INTERNAL`) for that one
call instead of taking the whole app down — but a panic *outside* that boundary (a goroutine an
adapter spawns and never joins, for instance) still can. This does not restore the old isolation;
it converts "the app disappears" into "one operation failed" for the panics it catches.

**One process for all connections**, same as before: there was never one V8 isolate per connection,
and no per-connection Go process exists either — the adapter host multiplexes every open
connection through its own registry and cache regardless of how many are open at once.

**Keep-awake is a short-lived child process the Go shell can spawn**, beside
`internal/startupfail`'s `osascript`/`pbcopy` (`gitvsix`'s `code` spawn went with the git module, P100, and is gone entirely as of P243 Part 2). `internal/
keepawake` (repo-root as of v1.9 P116, shared by both apps rather than Kira Studio-only) composes
independent, level-shaped reasons onto one `Controller`, a set of held reasons rather than a
refcount (a refcount double-acquires the moment either source re-asserts a level it already holds).
The titlebar toggle is the manual reason in both apps. The agent reason is Kira Space only (P188):
Settings leaf `agent.keepAwakeWithAgents` (Agent pane), applied by the package-level
`bridge.KeepAwakeRecompute` against the live count of terminal-registry agent sessions plus `ade`
sessions running headless. It emits `ChannelKeepAwake` only when the agent reason flips. Triggers:
terminal `Registry.OnChange`, ade board `OnSessions`, `SettingsService.OnChanged`, boot. Kira Studio
has no agent reason and no such setting (migration `0032` deletes the stored row). While the set is
non-empty, macOS runs
`caffeinate -i -s -w <our pid>` as a child process — argv-only, no shell, the only variable in the
line is this app's own pid; `-w` makes `caffeinate` exit on its own if the app crashes, so a missed
teardown can never orphan an assertion that keeps the machine awake forever. `-s` (system sleep) is
honoured only on AC power, a macOS constraint `-i` (idle sleep) does not share; the titlebar
tooltip and the Settings helper text both state this rather than promising more than `-s` delivers.
A live `caffeinate` process's assertion does not reliably survive a sleep/wake cycle (confirmed
against Orca, `stablyai/orca`), so `internal/shell.AttachSystemWake` — modelled on `AttachReopen`,
registering the listener is what makes Wails post `events.Mac.ApplicationDidWake` at all, called
from both apps' own `main.go` — re-arms the assertion (release, then re-acquire) on every resume, a
no-op while nothing is held. Every non-darwin build gets a documented no-op driver instead: the
titlebar hides its button outright rather than offering a control that does nothing, and Kira
Studio's titlebar toggle is its only reason.

**Hook monitoring is a shared, host-wired package (P127), now the Claude provider's optional hook role (P316).** What was Kira Studio's own
`internal/agenthooks` (P86-P126: the local HTTP listener a Claude Code hook shells out to, plus the
bridge glue that started it, composed a tab's launch and projected running sessions to the
renderer) moved to repo-root `internal/agenthooks`, then (P316) to `internal/harness/claude/hooks` — `Server` (start/stop/status, plus
`Compose(terminalID, cwd, command)` for the `--settings` flag and hook env a launcher adds to a terminal command), the
socket protocol and its documented bounds (one connection at a time, bounded event size). The
renderer-side reducer and store hoisted the same way, to
`packages/workbench/src/state/{agentActivity,createAgentSessionsStore}.ts`; the wire types both
sides share stayed at `packages/shared/domain/agent.ts` (unmoved — already shared, P86); the two
event-channel strings hoisted to `internal/appevent`. What a host app still supplies itself: the
on/off setting (if any), the bound service surface a hook's own launch reaches, and wiring
`harness.Hooks.Compose`'s result into its own `TerminalService.Open` — never `Compose`'s env
map directly, since `KIRA_AGENT_HOOK_TOKEN` must stay inside Go. **Kira Space is the first, and so
far only, consumer (P129 Part 1)** — Kira Studio dropped its own bridge/UI/settings leaves at P127
and wires none of this; the agent keep-awake reason is Kira Space-only (above, P188). Kira Space's `internal/appwire/wire.go` (`wireTracker`) starts the `harness.Hooks` hub, which starts each provider's `harness.HookHost`,
unconditionally, with no on/off setting of its own (P129 Part 1's own posture: a start failure —
`curl` missing, a bind conflict — is logged, never fatal, and sessions still spawn and track with
activity icons simply absent). `hookEvents` (`internal/harness/claude/hooks/config.go`) now also asks Claude
Code to fire `UserPromptSubmit`, the one hook event P127's own set left out — `reduceAgentActivity`
(below) is its first consumer. `AgentEvent` gains `lastAssistantMessage` (P238; Stop's reply text, bounded to 200 bytes on a rune
boundary like `message`; `packages/shared/domain/agent.ts`).

**Hooks reach only Kira Space sessions (P233).** Nothing in either app writes Claude Code hooks into
the user's `~/.claude/settings.json`, project `.claude/settings(.local).json` or any other settings
file (audited: no writer exists, and none ever did). Hooks ride `--settings <hooks.json>` plus the
`KIRA_*` env on launches Kira Space composes, from a private temp dir deleted on `Close`. The shim
exits 0 before any request unless `KIRA_AGENT_HOOK_TOKEN` and `KIRA_TERMINAL_ID` are set, so a
`hooks.json` or copied entry loaded in the user's own sessions is inert. MCP registration
(Settings > Memory, Studio Database MCP) is a separate, explicit user action that edits
`~/.claude.json` through the `claude` CLI and is out of this rule. Real-flow proof:
`flows/claudeflow` (fake HOME, seeded settings, byte snapshots).

**`reduceAgentActivity`'s own phase lattice gained a fourth value, `waiting` (P129 Part 1 §4.8).**
`AgentActivity` gained `wakeArmed` (sticky across a turn, set when a `PreToolUse` names `Monitor` or
`ScheduleWakeup` — the two tools an agent uses to arm a wake-up rather than end its turn for good)
and `at` (every event's own receipt time, including one that leaves `phase` unchanged). `Stop`
resolves to `waiting` instead of `idle` when `wakeArmed` was set, then clears it; `UserPromptSubmit`
(the new hook event above) is the one reliable "working" start for a turn that answers in plain
text with no tool call, and always clears whatever `wakeArmed` the previous turn set. A
`Notification` carrying `idle_prompt`/`auth_success` leaves `phase` (and `message`) unchanged — an
idle session polling for a wake-up is not "needs input." **Known limitation:** a `Bash` call cannot
arm `waiting` even when its own command is really a background watcher script, since `tool_input` is
never decoded (P86's own privacy rule, restated in Known open items below) — only `Monitor`/
`ScheduleWakeup` by name can.

**The renderer talks to Go over two planes.** The **control plane** is the Wails-generated
TypeScript bindings under `apps/kira-studio/frontend/bindings/…/internal/bridge/` (git-ignored, regenerated by
`wails3 task common:generate:bindings`, which `scripts/setup.sh` calls), which `apps/kira-studio/frontend/src/bridge/control.ts` calls as plain
typed async functions — `AppService.Info()`, `ConnectionsService.List()` — resolving under the hood
to HTTP calls against the local `/wails/runtime` endpoint driven by `/wails/runtime.js`
(`@wailsio/runtime`). That specifier is a literal URL the app itself answers — Wails serves its own
runtime bundle there — not a package the build resolves, so `vite.config.ts` marks `/wails/*`
external. Vite's dev server still has to resolve it at transform time, which is what
`apps/kira-studio/frontend/wails/runtime.js` is for: a file at exactly that path under the Vite root, so the
rewritten import URL stays `/wails/runtime.js` and `wails3 dev`'s asset server (which answers
`/wails/*` itself and proxies only the rest to Vite) keeps serving the real bundle. Every call is
wrapped in one `unwrap()` normalizing a Go-side error into
the `{message, code}` shape the renderer already branched on.

**The control plane has its own undocumented size behaviour, measured once (P3) rather than
inherited by accident.** A bound call's serialized argument body over 512 KiB
(`@wailsio/runtime`'s `CHUNK_THRESHOLD`) is sliced into 512 KiB pieces and `await`ed **serially**,
one `fetch` per chunk — and the Go side (`pkg/application/transport_http.go`) refuses an assembled
body over **64 MiB** outright (`"assembled body too large"`), after holding it simultaneously as a
JS string, a `Uint8Array` and a Go `[]byte` before `encoding/json` ever parses it. So a 20 MiB
file base64-encoded (~26.7 MiB) would cross as 54 sequential round trips before a byte reaches Go,
and a 50 MiB file could not cross the control plane at all — a hard ceiling with no `E_TOO_LARGE`
of its own to explain it. This is exactly why the HTTP request body's file modes (P3: form-data
file fields, the binary body) and the pre-existing S3 object upload both cross the wire as a short
local **path** instead of the file's own bytes: Go opens and streams the file itself
(`internal/httpclient/body.go`'s `buildFormData`/`buildFile`, mirroring `internal/adapters/s3/
transfer.go`'s `openUploadBody`), so an upload's size stays bounded by the filesystem, not by this
512 KiB/64 MiB arithmetic, and a multi-GB file becomes one ~200-byte control call rather than hundreds.

The **data plane** is a single named
stream, `"engine"`, opened once per page load by `apps/kira-studio/frontend/src/bridge/port.ts` via
`Stream('engine')`, carrying binary FlatBuffers frames for bulk payloads (grid pages, tree
results) — requests are still plain JSON text (P11 D3); only responses and events are binary.

**The data plane is not a WebSocket, on the build that ships (P4 F2, F3).** `Stream`
wraps Wails' own transport, and which transport that is depends on the build tag. A desktop build
(what this app ships) has no local listener at all: the frontend holds open `GET
/wails/stream/poll` (up to 20 s) until the Go side has a frame to deliver, and posts outbound
frames to `POST /wails/stream/send` — both over the asset server's custom URI scheme, deliberately,
so no local TCP port is open for another process on the machine to reach. `WailsSocket` wraps
that poll/send pair in the same `readyState`/`onmessage`/`send()` shape a `WebSocket` has, which is
why the frontend code reads as if it were talking to one — but a real, on-the-wire `WebSocket` only
exists in a `-tags server` build (`apps/kira-studio/tests/e2e-real/` builds with that tag; the
packaged app does not), where the identical application code runs unchanged over a real
`net.Listener` instead.

- The Go data plane is a server: HandleDataFrame in apps/kira-studio/internal/adapterhost/dataframe.go parses only each inbound frame's op (and connectionId for connection-scoped ops) to route it, then answers every op in-process via adapterhost.Dispatcher.
- Data-plane responses and events are encoded as one FlatBuffers Frame with file identifier "KIF1", whose offsets/truncated vectors are [uint] so decodeFrame in packages/shared/protocol/frame.ts reads each chunk's four buffers as zero-copy typed-array views, not base64 decode-and-copy.
- Requests from renderer to Go still travel as plain JSON text; only responses and events are FlatBuffers.
- The engine is the Go process itself, so ping is answered locally and the status pill's pid is the process's own os.Getpid().
- cache:stats and cache:clear are answered locally, merging both caches' counters while reporting the configured budget once, not doubled.
- adapterhost.Session owns a single bounded queue (64 frames / 32 MiB) drained by one goroutine into the renderer's Send, so exactly one goroutine calls the renderer connection's blocking Send.
- enqueueResponse blocks for room on a full queue instead of dropping, because a dropped response could never settle its pending request, which has no client-side timeout.
- Only enqueueLocal's unsolicited events (cache:stats) drop on a full queue, since the next one supersedes it.
- For a single oversized response, dataframe.go's respond refuses a payload over maxDataFrameBytes - 4096 with a visible error before building the frame, strictly inside Wails' 64 MiB hard cap (streamMaxFrameBytes).
- For sustained throughput the transport-level bound saturates first: Session's 32 MiB/64-frame queue sits in front of Wails' smaller 8 MiB/256-frame per-connection window, acting as a shock absorber; the producer blocks only once the queue backs up.
- Inbound (renderer to Go) has no app-level queue; Wails' limit (256 frames / 8 MiB) answers 429 rather than blocking, because the held poll request is the transport's scarce resource.
- The control plane stays JSON while data-plane responses and events are FlatBuffers; gRPC cannot run over the custom-URI-scheme transport the app ships on.
- FlatBuffers was chosen over Arrow IPC because Arrow pays a 400+ byte schema tax on every independent frame, needs four hand-built accommodations, and its JS package is 51.4 KB gzip; FlatBuffers sends no schema, costs +0.01–1.7% over raw bytes, and its JS runtime is 2.7 KB gzip.
- Cap'n Proto was rejected: comparable wire overhead but no public untyped builder in its JS tooling and a thinner ecosystem.
- internal/page's encode cost came from custom MarshalJSON boundaries plus encoding/json re-scanning; removing both Marshaler boundaries is byte-identical on the wire, 6–15x faster to encode, and cuts allocation from 2.3–3.8x frame size to 1.00x.
- The network-split answer is the -tags server build tag, which runs the same StreamSession application code on a real net.Listener with Stream as a WebSocket, hidden behind the Send([]byte)/Receive() ([]byte, error) seam.
- The server build tag provides no authentication, and three flows still assume Go side and user share a machine: FilesService.ChooseSave's native dialog, the OS keychain in internal/secrets, and internal/preconnect's locally-supervised processes.
- P4's hand-built binary envelope proposal is superseded by FlatBuffers (P11), since a generator-enforced alignment rule beats a comment enforcing one; no hand-built envelope remains to build.
- Each app's internal/appwire owns the object graph; Build(Options) wires every service and returns *Wired, whose Bound() is the registration list and BeforeFlush and Teardown are the ordered shutdown.
- appwire Options carries the OS seams main.go owns (DB, repos, emitter, dialogs, keep-awake driver); Studio adds cipher, authorizer and MCP installer; Space adds browser, git Locator, mobile assets, LAN seams, GhLocator and TrackerGrace.
- main.go keeps the Wails app, windows, menu and startupfail; Space's appwire also has BindShell (window hooks) and StartMobile. Production and both flowharness packages call the same Build.

- Kira Studio's Go side lives in apps/kira-studio/; Wired.Bound() registers 30 bound services under apps/kira-studio/internal/bridge/, with main.go building application.New options from appwire.Build.
- Kira Studio's Bound() binds no git-related service; GitClientsService moved to Kira Space as of P100 and was deleted there in P243 Part 2 with the Connected editors pane.
- Kira Space's Bound() binds a separate set of 35 services, counted by the application.NewService entries in Bound() in apps/kira-space/internal/appwire/appwire.go (P452 AgentContextService, P453 AgentSettingsService, P454 AgentPortService).
- TerminalService and WindowsService are per-app embeddings of one shared Go type each: internal/windowsvc.Service (Ensure/SetMode/OpenNew) and internal/terminal.BoundService (Open/Write/Resize/Close/DefaultCwd), living once at repo root (P128).
- Each app's bridge Windows/Terminal service is a one-line struct embedding the shared type, so Wails' FQN, derived from the embedding type's package, reads WindowsService.Ensure / TerminalService.Open without a binding-name shim.
- Kira Space's WindowsService gained Ensure/SetMode in P128 §2.2; it previously bound only OpenNew.
- Behind Kira Studio's services: internal/storage (repos plus forward-only SQL migrations), internal/tree/service.go (children/describe/definition cache-aside), internal/preconnect (pre-connect script supervisor state machine), repo-root internal/secrets (keychain), internal/adapterhost, internal/adapters, internal/oplog.
- Pre-connect readiness: Supervisor.Start takes a ReadyCheck. When the connection's target is loopback with an explicit port (connections.readyAddr), Start probes a TCP dial every 100 ms while the script lives and returns once it connects (60 s timeout kills the script); every other target keeps the 2 s settle window. A fixed window raced slow tunnels (kubectl port-forward, ssh -L): Connect dialed before the port listened.
- Per-connection `preconnectDelaySec` (0-300, 0 = off) is a floor: connect waits until both the delay and the readiness rule hold. An exit-0 script still waits out the delay; a failing script or Cancel ends it at once; the 60 s timeout starts at delay end. It sizes the wait for non-loopback tunnels and floors a port that already listens before the script runs.
- The script's startup wait runs as the op kind 'preconnect' (Backend.RunPreconnect, command = script text, nil connection id for a dialog Test). It ends before the 'connect' op starts, Cancel in the Operations panel aborts it, and Re-run is disabled for it. `op:start` carries `delayMs` for a delayed preconnect (live only, not in `op_log`); the Operations panel counts it down in the command cell.
- internal/oplog holds the op-log event type and its two topics, relocated from the deleted internal/enginehost (P58f D9); repo-root internal/metrics is shared with Kira Space since v1.9 P116.
- Multi-window (P8): the tab set and active tab (tabs.window_key) and the window's rectangle (windows.bounds_json) are per-window; a second window is reachable via Window → New Window (⇧⌘N).
- App-wide state shared by all windows: connections and live state, settings, op log, all three cache tiers, metrics readout, keychain, pre-connect supervision, panel layout, because they belong to the one process and database.
- The API panel's collections, saved requests, variables and environments are app-wide (P112): one SQLite store, broadcast on every mutation via kira:api:dataChanged (Emit, not EmitTo), cached in the renderer as TanStack Query entries with staleTime Infinity that the broadcast invalidates.
- The local-authentication grace grant (P14) is app-wide, but the action it gates stays per-window: each window's edit dialog still requires its own explicit Show password press, which succeeds without a prompt while a grant is live.
- The window key travels as ?window=<key> on the frontend URL, read synchronously at module load in packages/workbench/src/lib/window.ts, before hydrateTabs() runs; the shell mints a UUID per window and sets URL "/?window="+key and Name key.
- An absent or unrecognised ?window= falls back to the key "main" (as in tests/ui and tests/e2e-real).
- bridge.WindowsService.Ensure is called once at renderer boot to register a window key (needed for -tags server builds with no native shell); on the native shell it is always a no-op because main.go already creates the windows row.
- Closing one of several open windows deletes its windows row, cascading its tabs via tabs.window_key ON DELETE CASCADE; closing the last window leaves the row so the next Dock click or relaunch reopens the same workbench with tabs.
- ApplicationShouldTerminateAfterLastWindowClosed is false (P56), keeping the app running after the last window closes.
- Menu commands are delivered only to the focused window, while state changes (e.g. connection-state change, settings update) are broadcast to every window.
- internal/appevent.Emitter (appcore.Emitter is a plain alias) has three delivery shapes over DispatchWailsEvent: Emit (broadcast), EmitTo(windowKey, …) (exactly one window by key), and EmitFocused (the focused window).
- Emit broadcast is used for the seven state-change channels and for the quit handshake's flush-before-close signal, which every window must answer; EmitTo is used for the per-window close-flush handshake; EmitFocused is used for the menu's twelve signal channels.
- Broadcasting a menu command was a real regression in the Wails port, fixed in P8: Cmd+W closed a tab in every open window and a menu-driven Run could execute a console statement in a window the user wasn't looking at.
- In the -tags server build, EmitTo broadcasts to every browser window (internal/shell/emitto_server.go, P231); the native build is unchanged.
- App-wide CPU/footprint metrics (internal/metrics/, shared by both apps via NewAppTicker(appName)) find the app's processes by executable-path substring match, not a pid-tree walk, because native webview helpers are not ppid children of the shell and a tree walk would under-count.
- The metrics anchor needle is each app's own shipping executable name ("Kira Studio" or "Kira Space"), and HelperNeedles is ["com.apple.WebKit", "webkitgtk", "bwrap"].
- Process matching does one full process-table scan per 60 s (CachedPIDs), revalidated cheaply on every 5 s tick in between rather than a full scan per tick.
- On darwin the memory metric is ri_phys_footprint read via proc_pid_rusage(pid, RUSAGE_INFO_V2) per pid per tick, matching Activity Monitor's "Memory" column, rather than pti_resident_size (RSS), because summing RSS counts shared pages (dyld shared cache, WebKit framework text) once per process.
- On non-darwin platforms the memory metric sums RSS.
- The status bar CPU readout uses the normalized convention (0…100, share of the machine's whole capacity, matching Activity Monitor's CPU-load pane), not the per-process "% CPU" column's unnormalized per-core sum (0…100×N).
- The normalized status bar CPU figure is up to logicalCPUs times smaller than Activity Monitor's per-process % CPU column; this is expected, not a bug. The hover tooltip (AppMetricsItem) is a TooltipDetails listing app name, CPU, cores, memory, processes and the 5s interval.

- A normalized CPU percentage is a share of total core-seconds, not of compute capability, so four E cores at 1 GHz and four P cores at 4.5 GHz can both read 400%.
- The app matches Activity Monitor's CPU accounting, which ignores core frequency and P/E asymmetry; the limitation is in the underlying % CPU/ri_user_time accounting and cannot be fixed by reading something else.
- The old Node-engine-over-stdio hop JSON.stringify'd every frame; a TypedArray serialized as an object keyed "0","1",…, which is why a reviveChunks decode step existed.
- P58a M2 measured the Node engine's index-keyed JSON inflating a chunk 10.872x on the wire and 40.9x in transient heap, versus 1.334x and 6.86x for Go's base64 encoding (P58 D5).
- P11 replaced base64 with a FlatBuffers frame decoded as zero-copy typed-array views, adding +0.01–1.7% over raw buffer bytes with no transient heap copy on decode; no kind uses base64 today.
- The status bar is 22px (`--kira-statusbar-h`); every item is a StatusBarItem (icon, tone, text, `tip` slot) in the interface font. Kira Space's right group is agents, runs, update, metrics. The agents item (`ade/v2/status/`) shows `N agents · M needs input` from `useSessions` and opens a popover list sorted by activity rank; a row opens the session through `useOpenSession`, the footer opens the needs page with `adeBoardUi.showAllSessions`.
- The update check and in-app install (P66, P119) are shared by both apps. The status bar's right-hand group can show an update-available item, placed first so the app-metrics readout keeps its position.
- Clicking the update item, or a new version becoming available while the app is open, opens a modal UpdateDialog.vue with an Update button. P119 deliberately replaced Kira Studio's click-opens-the-release-page behavior with this dialog.
- Repo-root internal/appupdate.Checker, moved out of Kira Studio's internal/ at P119 so Kira Space can share it, makes one plain net/http GET against the kira-studio GitHub releases/latest API and imports nothing under either app's internal/.
- The update checker does not depend on githost/github (it needs an authenticated gh CLI, the wrong precondition for a check run for every user) or internal/httpclient (the user-facing request builder); it only restates their GitHub API version header value.
- When buildinfo.Version is a dev sentinel (0.0.0, 0.0.0-dev, 0.0.0-unknown), the update check is suppressed entirely (no request, cache write or goroutine) and InstallUpdate refuses. Only tagged release builds reach the network or install.
- Each app's renderer polls for updates hourly after the first window mounts, off the boot critical path. Go caches the answer for 6 hours after a successful check and 30 minutes after a failed one, and singleflight collapses concurrent callers, so Go sets the real network cadence.
- A failed update check (network error, 404, or 403 rate-limit) is silent: no item, no error dialog, no console output, only a debug-level log. A 404 occurs because release.yml publishes only after all gates pass and drafts are invisible to /releases/latest.
- On Update click, internal/appupdate.Installer re-checks availability from cache, then Stage fetches scripts/install.sh fresh from main (never bundled, so installer bugfixes reach every installed version) and checks its first two lines for the marker '# kira-install-contract: 1' to guard against captive-portal HTML and incompatible future scripts.
- Stage writes the install script to a 0700 temp file and spawns it detached with Setsid (own session and process group, surviving app exit) using --app=studio|space, --wait-pid=<this process>, and --notify-fd=3.
- The install script prints 'staged <dmg path>' to fd 3 and closes it once the disk image is mounted and copying has started. Stage reads that line and returns, and the app quits only then, because the swap into /Applications needs the app's bundle closed. The script then waits for the pid to exit, replaces the bundle, relaunches, and cleans up.
- Cancel (effective only while idle before hand-off) and the 15-minute backstop timeout signal the whole process group via internal/procgroup.Kill, never just the script's pid, so children like hdiutil and cp are not orphaned.
- Every install run appends to ~/Library/Logs/<app name>/install.log, truncated once past 1 MiB, and UpdateStatus.installLogPath gives the dialog this path to show on a failed install.
- The renderer never sends or receives a URL: UpdateService.Status, Check, InstallUpdate and CancelInstall are all nullary, and Go decides where the script comes from and what it does.
- scripts/verify-packaging.sh check S10 (scoped to apps/, packages/, internal/) enforces that no app code downloads a release asset itself, and only scripts/install.sh may. S11 pins the one raw.githubusercontent.com reference and the two sides' shared contract marker.
- Self-update runs a shell script downloaded at click time with the user's privileges from the main branch of a public repo, so anyone who can push to main can change what every user's Update click runs. Branch protection on main is therefore a security boundary; TLS and the contract marker guard transport and compatibility, not authorship. Since P330 a chapter PR from this repo squash-merges itself once `pr.yml`'s required checks pass, with no review, so push access to a `chapter/v*` branch is the same boundary.

## Git module (v1.3), Kira Space's own app as of P100

- As of P100 the git module (all internal/git* Go packages, internal/ghclient, internal/codeworkspace, and the consumers of packages/git-ui, git-core, git-ipc) moved from apps/kira-studio into apps/kira-space, a separate standalone macOS app.
- Kira Space has its own binary, its own ~/.kira-space/ home (KIRA_SPACE_HOME), and its own kira.db and review.db.
- From P100 through P127 Kira Space's window had no title-bar mode switcher; the repo workspace was the whole app, mounted directly rather than behind a 'git' AppMode.
- P128 gave Kira Space its own module system with modules git, automations (the same shared Runs module Kira Studio uses) and ade (the app's task-planner surface).
- In Kira Space, state/workspace.ts's visibleWorkspace() is the one function reading both the mode store and the git-only workspace store: it returns the active repo (or none) while git is active, otherwise the active module's id. host.ts's activeWorkspace and state/tabs.ts's tab-stepping actions use it.
- Opening a repository (openRepoWorkspace) forces Kira Space's mode back to git; the boot-time fall-forward onto the last active repo (persisted per window as kira.space.lastRepo.<window>) or first restored one does not, so a relaunch into a persisted automations/ade mode stays there.
- P116 gave Kira Space generic window chrome using shared primitives (internal/keepawake, internal/metrics, packages/workbench TitleBarWindowActions and AppMetricsItem): a native menu, keep-awake toggle, New window button, and CPU/memory status-bar item.
- P119 gave Kira Space a status-bar update item and in-app update dialog over its own UpdateService, sharing the repo-root internal/appupdate with Kira Studio.
- In Kira Space's Window menu, Close Tab claims ⌘W and Close Window is ⇧⌘W; before P116 ⌘W closed the window.
- -tags server is a Wails v3 platform facility (pkg/application), not per-app source; Kira Space gets the identical go build -tags server substitute.
- Through P99 (P67b), git was a peer AppMode in Kira Studio with a title bar reading Studio | Api | Git, and repo switching lived in repo/GitPanel.vue; workspaceState.lastRepoKey restored the last repo when returning to Git.
- As of P100 Kira Studio's AppMode dropped 'git', WorkspaceKey was deleted, and validWindowModes dropped "git"; a stored git-mode window degrades to "studio" via the NormalizeMode fallback, so no migration was needed.
- Kira Space never had the peer-AppMode design; it was a single-module app through P127.

### Transport

- ServeGitStream in apps/kira-space/internal/bridge/gitstream.go serves gitrpc.Router over rpcstream via one in-process Wails stream per window, with no socket, handshake, pairing or trust store, because the peer is the process's own webview.
- The Unix socket (git.sock) that served a separately-installed VS Code extension, and the extension itself, are gone since P243 Part 2; removeLegacyGitSocket in main.go deletes the git.sock and git.sock.lock left by older builds in KIRA_SPACE_HOME.
- An flock on ${KIRA_SPACE_HOME}/app.lock is the single-instance guard: config.AcquireLock takes it before storage.Open, and if not acquired the process logs and exits 0.
- A SIGKILLed instance's app.lock is released by the kernel, so no stale-pid file or manual cleanup exists.
- gitrpc.ContractVersion and CONTRACT_VERSION in packages/git-ipc/src/validate.ts must be equal (56 currently), asserted by tests on both sides; a mismatch is a build bug since both sides ship in one binary.
- app.init returns {contractVersion, serverVersion, git, dateFormat}, where dateFormat is Space's app-wide appearance.dateFormat.
- Contract version 47 (P246) dropped credential.request and credential.provide from the stream; credentials go through the relay snapshot and GitCredentialService.Provide.
- Contract version 46 removed the extension-only surface (worktree.prepare, worktree.cancelPrepare, worktree.openWindow, editor.resolveConflict, worktree.progress, settings.changed and connection.changed events, app.init's host/settings/capabilities, and injected params scope, pageSize, baseCandidates, strategySetting); the server reads stored kiraSpace.* repo settings instead.
- Contract version 44 added StatusSummary.autoFetch, the autoFetch.changed event and graph.reportFailure; version 43 refused prepare-script writes from repoSettings.set and removed settings.setGitPath.
- bridge.maxGitStreamFrameBytes caps one frame at 32 MiB, because an 8 MiB cap failed the graph and commit detail on large repositories.
- A session's sendCh holds up to 16 frames, so worst-case memory is 512 MiB, reached only by 16 concurrent near-cap requests; Wails admits one oversize frame into an empty per-window queue under its 256 MiB global budget.
- On the JS side a frame is joined once, decoded, parsed and structured-cloned to the webview, costing about 4-5x frame size transiently.
- On the git stream, control frames (the rpcstream envelope, every request, every small response) are JSON text, while bulk payloads are FlatBuffers, the same split as the studio data plane.
- The git FlatBuffers schema packages/git-ipc/schema/gitwire.fbs uses file identifier "KIG1", distinct from the studio plane's "KIF1" so the two can never be cross-decoded; a frame without "KIG1" is a hard error with no dual-format decoder or compatibility shim.
- gitwire.fbs has one Frame wrapping a Payload union whose only member is PackedCommitChunk, the graph's column-wise commit block (sha bytes, CSR parent offsets, identity/time table, subject bytes plus offsets, delta-numbered string dictionary, per-row ref decorations).
- Keep gitwire.fbs append-only: never renumber, reorder or delete a field; retire fields with (deprecated).
- The schema file is named gitwire.fbs, not gitWire.fbs, because flatc's TypeScript generator names its entry point after the filename and its barrel after the namespace, and names differing only in case collide on case-insensitive filesystems.
- rpcstream is module-agnostic infrastructure and the deliberate exception to the module-boundary rule: a correlated-RPC-with-credits state machine (req/res/evt/open/chunk/end/credit/cancel) transcribed field-for-field from the TypeScript rpc.ts, and it never learns what a method means (Handlers do).
- rpcstream lives at repo-root internal/rpcstream since P100, hoisted from apps/kira-studio/internal/bridge/rpcstream so both apps/kira-studio and apps/kira-space can import it, since Go's internal/ visibility blocks cross-app imports.

### Session model

**Several windows can hold one repository at once**: native Kira Space windows, plus ADE's board
connection, each with its own `gitsession.Conn`. The structure follows one rule — *a fact about the
repository is shared; a fact about one viewer's session is private* — the one genuine structural
departure from a single-session design that conflates the two.

```
Registry (internal/gitsession): map[RepoID]*RepoEntry — mutex + refcount
  RepoEntry — SHARED by every connection open on that repository
    reader/writer gate · git driver · cat-file --batch session · repo watcher
    detail / diff / refs / stack caches · live HEAD · undo slot · active remote op (<=1)
    subscribers: map[ConnID]chan Event
Conn (internal/gitsession): one per native stream (ServeGitStream) or ADE connection
  client identity · its own ctx · its own rpcstream session and Handlers
  walks: map[RepoID]*Walk — PRIVATE per (connection, repository)
    log session, commit store, dictionary marks, paging state, the active review walk
```

- RepoID is the absolute git dir, NFC-normalized, so two clients reaching the same repository by different path spellings share one entry rather than racing two.
- Registry.Acquire(ctx, path) refcounts repository entries; real teardown (kill the cat-file pair, stop the watcher, drop caches) happens only at zero refs and only after a five-minute linger, so closing and reopening a repository is free.
- The reader/writer gate is unchanged from the single-client design but now lives on the shared RepoEntry, so it serializes across connections instead of within one.
- There is one git watcher per repository, fanned out to subscribers. It covers HEAD, refs/**, packed-refs, index, FETCH_HEAD, MERGE_HEAD, rebase-*, CHERRY_PICK_HEAD, REVERT_HEAD and sequencer in commonDir/gitDir only, never the worktree.
- A source file saved by an editor produces no event on the git watcher, and its output (chan Signal) carries no path.
- Nothing else in the app watches the worktree; the native code workspace's project tree and search are refresh-on-demand by design, not backed by a second watcher.
- Watcher events are debounced 200 ms and delivered to each subscriber over its own coalescing buffered channel, so one slow client cannot stall the watcher for others.
- The darwin watcher backend is FSEvents (gitclient/watcher_fsevents_darwin.go, darwin && cgo); the fsnotify implementation remains the !darwin || !cgo companion used by Linux dev/test.
- FSEvents was chosen over fsnotify on macOS because kqueue needs one open file descriptor per watched directory under commonDir/refs, a cost against kern.maxfilesperproc on repositories with many loose refs; FSEvents watches recursively through one stream.
- A client disconnect never kills a write: on peer close rpcstream.Serve cancels in-flight requests and streams, the connection's log-session processes are killed and RepoEntry refcounts released, but an in-flight write is detached and finishes with its result delivered nowhere.
- Writes are detached on disconnect because a half-applied checkout from a closed window is far worse than a result nobody reads.
- The undo slot is one per repository, not per connection, and its label names the originating client, e.g. "Undo reset of main (window: repo-review)".
- Credential prompts open in Kira Space's own window (P178): internal/gitcred.Relay holds each prompt labelled "<source> · <repo folder>" and pushes the full pending list on kira:git:credential.
- The gitCredential Pinia store queues relay prompts behind the native stream's own prompts in one dialog; when the relay withdraws a prompt (answered elsewhere, op cancelled, 120 s timeout) the next snapshot closes the stale dialog.
- With no Space window open, an arriving credential prompt opens one as a Dock click would (U1); otherwise it focuses the first window. ADE's board connection uses the same relay.
- internal/gitaskpass brokers prompts over its own private socket behind a GIT_ASKPASS shim, relaying to the gitsession.Conn owning the op: to the relay for ADE connections, to the native stream's dialog for the native window's op (P67e).
- If the owning connection dies mid-prompt, the askpass broker fails the credential request non-zero rather than hanging, and the wait is bounded at 120 s (gitaskpass.DefaultTimeout), so git never blocks forever on an unanswered prompt.
- The native window's credential answer path (gitCredential.ts and GitCredentialDialog.vue) is a FIFO queue feeding one always-mounted dialog, so a prompt from one repo tab stays answerable after switching away; two open workspaces can each prompt at once.
- The credential dialog has no client-side timeout because the broker's 120 s bound covers it, and answering a request the broker already gave up on is a documented no-op.
- Never log, persist, or store credential prompt data beyond the one in-flight prompt; the typed secret lives only in the dialog field, cleared on submit, cancel, Escape or frame close, and the prompt text is treated the same since it may contain a typed username.
- protectedBranches, fetch.autoInterval and git.path are server-owned settings, edited in Kira Space's Settings dialog (Git section) and read fresh on every push pre-flight and auto-fetch tick, never cached, because two windows disagreeing about a protected-branch list is a safety bug.
- Per-viewer display settings (graph page size and scope, stash visibility, and similar) are per repository, stored server-side in git_repo_settings and edited from a dialog opened in the graph view.

### Go packages

- Every git package in Kira Space's git module is its own internal/git* package (plus internal/githost); no phase merged git code into a shared file where a per-module one would do.
- startupfail is not a git package; it is repo-root shared infrastructure, hoisted by P100 Part 1 and reused by both apps' boot sequences. It is listed in the git module's package table only because Kira Space's boot depends on it.
- TestDomainPackagesDoNotImportBridge in apps/kira-space/internal/layering_test.go covers the git packages, as Kira Studio's own copy covers the Studio and Api domain packages.
- No internal/git* package imports internal/bridge, and none imports or is imported by an adapter package.

| Package | Owns |
|---|---|
| `gitclient` | Spawn discipline (argv-only, no shell, env hygiene, `-c core.quotepath=false`, `--no-optional-locks`, `Setpgid` plus group-kill on cancellation, a graceful `WaitDelay`), a streaming runner returning a live pipe rather than a buffered `[]byte`, discovery (macOS-only, a **git 2.38 floor**, an explicit Xcode Command-Line-Tools-shim gate so the shim is never spawned blind, a short TTL cache), the capability probe, the typed error vocabulary, and the per-repository reader/writer gate |
| `gitclient/porcelain` | Framing and parsing for `log`, `for-each-ref`, `status --porcelain=v2`, `diff-tree`, `diff`, `stash list`, `merge-tree` and `cat-file --batch` — NUL and `%x1f` record splitting, against a committed golden-byte corpus rather than hand-written expectations |
| `gitclient/catfile`, `gitclient/logsession` | The two persistent child processes: one `cat-file --batch` pair per repository, and the pausable/resumable paged log walk |
| `gitpath` | The module's single Unicode-canonicalisation point (below) |
| `gitstore` | The column-wise commit store, sha table, string interner and `PackedCommitChunk` builder. It only ever appends and packs — a parent's sha is stored directly and never resolved to a row, because resolving is a renderer concern |
| `gitpreflight` | Hazard classification for checkout, stash pop, reset, revert, cherry-pick, push, pull, stash-branch, restack and worktree add/remove, plus the protected-branch glob matcher and the undo slot. Computed server-side and crossed as data — never duplicated client-side, so the two can't disagree |
| `gitops` | The write side: branch, checkout, tag, revert, reset, cherry-pick, stash, fetch, push, pull, worktree, stack/restack, conflict handling, and stderr progress parsing |
| `gitsearch` | The cancellable, time-boxed tail scan and the Go matcher, plus the RE2/`RegExp` dialect reconciliation (below) |
| `gitreview` | `review.db`'s whole surface: compressed content snapshots, fast/slow-path diff selection, partial-review ranges, the flat AI-comment list, and the TTL reaper (Storage, above) |
| `gitsession` | `Registry`, `RepoEntry`, `Conn`, `Walk` — the session model above. Imports `gitclient`, `gitpreflight`, `gitreview`, `githost` and stdlib only |
| `gitrpc` | The method table (`app.init` through `stack.cancelRestack`, plus the one `graph.stream` stream method), `ContractVersion` (**56**, P413), and the wire types |
| `gitwire` | Generated FlatBuffers code for the git data plane |
| `gitaskpass` | The credential broker and its `GIT_ASKPASS` shim, over its own private socket, with a bounded wait |
| `gitprepare` | The worktree prepare script's execution seam, called by ADE only — the one shell exception, below |
| `githost` | Provider-neutral code host contract and registry (see Git host abstraction, below). `githost/github` is the `gh` provider: CLI discovery and spawn discipline mirroring `gitclient`'s own `Locator`/probe/TTL-cache shape, a status classification, and change request lookup through `gh api`. `githost/providers` builds the registry |
| `startupfail` (repo-root, shared — not a git package, see above) | Native, pre-window failure alerts for every boot step (below) |
| `rpcstream` (repo-root, shared) | The correlated-RPC-with-credits protocol (above) — module-agnostic by design, hoisted out of `apps/kira-studio/internal/bridge` at P100 so both apps import it |
| `config` (`AcquireLock`) | The `app.lock` single-instance flock (Transport, above) |

- Every spawn in the codebase passes a fixed argv to os/exec with no shell, except gitprepare, which runs the user's worktree prepare script through a shell.
- Never interpolate an app-supplied value into the gitprepare command string; the string is the user's own typed command, and app data (worktree path, branch, repo root) reaches the script only as environment variable values.
- repoSettings.set refuses to set the prepare command on every gitrpc connection (P172), so no renderer can choose or change what runs; ADE runs the prepare script and the Git window never does (P202).
- The gitprepare package imports only the standard library and knows nothing about repositories, sessions or approval; gitsession owns every policy decision, so the feature can be tested without spawning a real shell.
- A provider owns its credentials; the app holds none. The github provider delegates to gh: no OAuth flow, no token prompt, no reading of gh's keychain entry; lookups go through gh api.
- githost.Status kind is ok, notFound (install the CLI), unauthenticated (log in) or forbidden (403 from insufficient scope or unauthorized org SSO, shown as the fix rather than a raw HTTP status). It mirrors GitStatus.
- A Status failure never blocks git itself; it only blanks the change request indicator (fail-open).
- gh stdout is capped at 64 MiB while reading, derived from the worst case of a 100-item open-PR page; overflow reports forbidden with a too-large reason, never "unreadable response".
- autoFetchOutcome classifies ticks: busy (OperationInProgress, Cancelled) reschedules at the normal interval and counts nothing; transient (NetworkFailed, LockHeld) backs off; permanent (every other kind, including AuthFailed, RemoteNotFound, Unknown) stops auto-fetch.
- Transient auto-fetch failures reschedule at min(interval << failures, max(interval, 60 min)) with the shift clamped at 16 and no attempt limit, so an offline night never stops it; a success resets the count.
- NetworkFailed is a catch-all that may hide a permanent cause (403, TLS); this costs one fetch per hour, which was judged cheaper than a stop requiring user action after every laptop sleep.
- Unknown auto-fetch failures stop auto-fetch because silent endless retry would hide a real problem.
- An auto-fetch stop sets a stopped marker, writes one op-log record (source Auto-fetch, kind autoFetch, status error) and emits autoFetch.changed; retries are never logged; StatusSummary.autoFetch carries state for cold start and reconnect.
- A successful explicit fetch or pull with a conn re-arms auto-fetch; a settings interval change does not, since the stop is about the remote.
- The toolbar shows an Auto-fetch stopped marker (autofetch-stopped) even with write controls hidden; clicking runs Fetch when Fetch is enabled.
- Failures are shown in an inline banner (FailureBanner.vue, shadcn Alert, role=region) under ConnectionBanner in App.vue, not a toast, because a toast needs a host, covers grid rows and vanishes before being read; the banner stays until dismissed or superseded.
- Graph failures add Retry (graph.refresh) and "Show in Operations" to the banner; a corrupted graph stream is reported with graph.reportFailure, where {reason: 'corrupted'} is the only reason and Go composes the message and records it in the op log.
- The op log records every git spawn from every module (background reads hidden by default); failures before an op starts, and anything while the transport is down, are banner-only.
- Search must make Go's RE2 and JavaScript RegExp agree exactly, because the server-side tail scan and client-side scan of loaded rows must match; the module never runs a second, different engine on the same query text.
- Literal-mode search runs no regex engine: it enumerates occurrences of an escaped fixed needle and does a byte-level word-boundary post-check, byte-exact to JavaScript by construction.
- Regex-mode search translates rewritable JS constructs into RE2 and applies whole-word as a consuming rewrite, not a post-check, because a post-check disagrees with JavaScript backtracking on patterns like foo|foobar.
- Lookahead, lookbehind and numbered or named backreferences, which RE2 cannot express, are refused as data ({kind: "unsupportedPattern"}), never dropped or approximated.
- Never edit only one side of the search matcher: the Go matcher and packages/git-core/src/search/ are twins pinned by packages/git-core/testdata/searchConformance.json, so add a row there first for any semantic change.
- Two search divergences are accepted: regex-mode case folding uses RE2's (?i) rather than ECMA-262 Canonicalize (Kelvin-sign class), and \S inside an open character class falls back to RE2's ASCII-only \S.
- The remaining RE2-versus-RegExp gap is accepted as is, with no in-UI note and no Go-side shim (user decision, P173).
- internal/gitpath is the single canonicalisation point for Unicode path normalization, because five unrelated packages need it and share no common import that wouldn't invert the layering.
- Always normalize absolute and directory paths (RepoID, root, git dir, common dir, worktree dirs, filesystem-event paths, client-supplied directory parameters) to NFC at ingestion; they are only keys, comparands, chdir targets or stat operands.
- Never normalize repository-relative file paths from porcelain output (status, diff-tree, diff, stash show), because they are passed back to git as pathspecs or <rev>:<path> where git compares bytes; NFC of an NFD entry gives fatal 'path does not exist' or a silently empty diff.
- gitpath.NFC is deliberately unguarded because norm.NFC.String has its own fast path and measured faster than an IsNormalString guard; it is byte-transparent for invalid UTF-8 and returns no error.
- APFS returns filenames in NFD while git stores paths as the committer's platform produced them, usually NFC.
- The app's reserved ref namespace is refs/kira/*; the reusable global stash is a real ref at refs/kira/globalstash/<sha>, because git's stash is a single pop-once stack that can't keep and reuse an entry.
- Every refs/kira/ string is built from one exported constant, and every revision set the graph walks passes --exclude=refs/kira/* unconditionally so the app's bookkeeping never appears in the user's graph.
- Pre-window startup failures in main.go (config.EnsureLayout, logging.Init, storage.Open including newer schema_version refusal, repos.New, settings read, window list/create) are shown by internal/startupfail as a native OS alert through an argv-only osascript spawn.
- Wails' dialog API cannot be used at startup-failure sites because it depends on globalApplication and a.impl, which are nil before New() and Run().
- A cgo NSAlert shim was rejected for startup alerts because a pure-Go package cross-compiles and unit-tests for darwin/arm64 from the Linux dev container, while a cgo one cannot be compiled there.
- internal/gitreview's equivalent refusal is not routed through startupfail because it fires mid-session in an open window as an RPC-level error.

### Frontend packages

- The git UI is the package packages/git-ui, mounted by Kira Space's RepoGraphView.vue.
- The git UI reaches Go through packages/git-ipc's streamChannel.ts, a Wails stream behind the MessageChannelLike seam.
- Kira Space's repo/git/hostHandlers.ts answers the few host calls (dialogs, clipboard, open externally, reveal in graph) locally.
- Repository settings are edited in Kira Space, and the settings dialog is always shown.
- app.init.dateFormat carries Space's app-wide appearance.dateFormat, read at each app.init.

| Package | Holds |
|---|---|
| `packages/git-ipc` | The shared vocabulary: `contract.ts`, `rpc.ts` (client only), `transport.ts`, `codec.ts`, `validate.ts` (`CONTRACT_VERSION`), `streamChannel.ts`, `blobFrame.ts`, `schema/gitwire.fbs` with its generated code, and `graphChunkCodec.ts` |
| `packages/git-core` | Client-side domain logic: the commit store, the lane-layout graph worker, the client half of search, the wire model types and the settings schema |
| `packages/git-ui` | The UI itself — the graph panel, the review panel, the dialogs, the file tree — Vue, mounted by Kira Space |
| `packages/kira-ui` | Deleted (P267). Was `floatingPosition.ts` only; `AutocompleteField` and `PointPopover` position through reka `Popover` |

- RefsPopover.vue's tab fold (branches, tags, stashes, worktrees, stacks in one tabbed popover) is a pure-fold module, pickerModel.ts in packages/git-ui/src/lib/, with no Vue and no bridge, so pickerModel.spec.ts exercises it directly with plain fixtures.
- The convention of keeping a pure fold out of the template is also followed by refListModel.ts, stashListModel.ts and stackListModel.ts.
- filterPickerInput scopes the single filter box to the active tab while still computing every tab's own live match count, so a query typed on one tab hints a match on another.
- orderAndCapTab ranks local branches HEAD-first, then worktree-checked-out rows, then committerDate descending, while tags, the stash stack and stacks keep their own existing order (version sequence, stack-position addressing, and a forest whose order is its structure).
- capWithPins and capStackGroups never cap a pinned row out and never split a stack group mid-order; capSteps lets a caller raise one list's own cap for the current panel-open without touching any other list's.
- refListModel.ts keeps no branch-list fold; the review and dialog branch pickers use the shared BranchPicker.
- Every viewport-anchored floating surface in both frontends goes through Floating UI's collision-aware middleware (flip/shift/size), rendered teleported so no overflow: hidden ancestor can clip it.
- The floating-surface audit is stated per mechanism, not per module, because a first pass judged one frontend clear based on its most common mechanism and missed a second; a native title attribute, a Monaco hover/suggest widget, and any bespoke click-point popup are each their own path, and a module is not clear because one mechanism is.
- A Monaco hover/suggest widget's container defaults to the editor's own DOM node unless explicitly reparented to document.body (P60a fixed this, 789fc4c7).
- Monaco widgets by default carry Monaco's own theme variables uncoordinated with the app's tokens; P67c themed Monaco's menu/suggest/list widgets from the Kira palette (e7e2c546).
- No .vsix is built or bundled: P243 Part 2 removed the VS Code extension, build:vscode, package:vscode and the Connected editors pane.
- A user who installed the removed VS Code extension should run: code --uninstall-extension vladcirstean.kira-space-vscode

### The native code workspace (C5-C7, P67c)

- P100 extracted the native code workspace (Go internal/codeworkspace and frontend views/repo/* and repo/* trees) from Kira Studio into Kira Space, a separate standalone app; Kira Studio no longer has repo import, project tree, diff tabs or code search.
- The path-containment check stayed in repo-root internal/pathsafe and was never moved into codeworkspace; codeworkspace/paths.go keeps ValidateRelPath as a two-line delegate.
- The repo list lives in repo/GitPanel.vue, the Git module's left panel, because a repository is an instance inside the Git module, not a sibling of it.
- Tab isolation is an orthogonal workspace dimension, not a new AppMode: TabRecord has a nullable workspaceId and workspaceKeyOf(tab) = tab.workspaceId ?? TAB_KIND_MODE[tab.kind], because two open repos share the same tab kinds (repo-graph, repo-file) which a mode-per-kind mapping cannot express.
- Tabs that predate the workspace dimension have workspaceId null, so studio/api behave identically.
- The preview slot is a per-workspace cohort (tabs store previewIdsByWorkspace, in-memory only): single click opens a replaceable italic preview tab; double-click or a second single-click promotes it via promoteTab; a permanent open never evicts the cohort.
- A non-cohort preview open evicts every id in the cohort and splices the new tab into the first evicted tab's array position instead of mutating a closed tab in place, so closeTab (which frees page stores/runtime) still runs.
- openTab's previewCohort flag (P74 §5.2) lets a bulk opener such as "Open all changes" join several tabs into one replaceable batch: the first file evicts, later files join with the flag set.
- Pinning is a property of tab kind (TabKindDef.pinned); repo-graph is the only pinned kind: permanently first, unclosable, un-reorderable, un-duplicable.
- C10 replaced the TAB_VIEWS['repo-graph'] placeholder view (RepoGraphView.vue) in workbench/tabViews.ts with the real packages/git-ui mount (RepoGraphTabView).
- The file list comes from codeworkspace.EnumerateAll (internal/codeworkspace/enumerate.go), using git ls-files -z --cached --others --exclude-standard (same argv as the search scanner), unfiltered by extension, capped at 200,000 paths as an IPC-payload limit.
- The tree view virtualizes rendered rows with @tanstack/vue-virtual (P104's replacement for the deleted hand-rolled tree-host component).
- The file list refreshes only on workspace open and explicit Refresh; there is no worktree watcher, so outside changes appear only then.
- Worktree files are editable only through CodeWorkspaceService.WriteFile (P419); every git invocation stays ReadOnly: true. Monaco opens readOnly and domReadOnly and switches to EDIT_OPTIONS (useReadonlyFileViewer.ts) only in edit mode; only repo-file tabs on the worktree (no revision pin) define a dirty flag and badge. Diff, local-file and revision-pinned tabs never edit.
- WriteFile is guarded by a base hash: it takes the SHA-256 of the bytes ReadFileForEdit returned, and a file whose current hash differs (or is gone) answers E_CONFLICT with disk untouched. A hash of "" expects the file absent, which recreates a deleted file. A mutex serializes saves inside the process; an external writer racing the microseconds between hash compare and replace is not guarded.
- WriteFile replaces the file atomically with renameio, keeping its mode; this breaks hard links, and a symlinked leaf keeps its link while the target is replaced. Text over MaxReadBytes is refused.
- ReadFileForEdit answers kind notUtf8 for found text that is not valid UTF-8, since JSON would replace the bad bytes and a save would corrupt the file. A BOM is preserved by saving with getValue(undefined, true), and the model's detected EOL keeps CRLF.
- Edit mode finds external changes by re-reading on window focus, not by a worktree watcher: a clean buffer follows the disk silently, a dirty one raises the Refresh/Overwrite alert. A save writes no op-log row, since it spawns nothing.
- Edit sessions (base hash, dirty flag, conflict) live in the frontend state/fileEdit.ts store, keyed by tab id and never persisted, so a restored window opens read only. The draft lives in the cached Monaco model, which survives a tab switch.
- Paths crossing the bound service must be repository-relative, rejecting absolute paths and '..' segments, then resolved with filepath.EvalSymlinks and required to stay under the session root; resolving before the containment check prevents a repo symlink from being used to read files like ~/.ssh/id_rsa.
- internal/pathsafe is a leaf package so consumers needing only the containment check (e.g. codeworkspace/search.go) don't pull in catfile/porcelain or the Session type.
- P67c widened Monaco from 19 to all 81 languages by importing upstream's languages/definitions/register.all.js in packages/workbench/src/editor/monacoEntry.ts; grammar bodies stay lazily loaded, adding ~12 KB raw / ~3 KB gzip.
- .json/.jsonc use Monaco's worker-free tokenizer (languages/features/json/tokenization.js) instead of the JavaScript grammar, which distinguishes string.key.json from string.value.json; no jsonMode, workerManager or json.worker is used.
- Monaco 0.56.0's shared JS/TS Monarch tokenizer has no rule for '@', so decorators painted as invalid; packages/workbench/src/editor/monarch/decorators.ts withDecorators() shallow-clones the tokenizer (not mutating the shared export) and prepends a rule emitting an annotation token.
- .vue/.svelte color as plain HTML (no Monaco grammar, so script blocks are not TypeScript-colored) and toml colors via the ini grammar.
- Exactly one Monaco worker ships (editor.worker, backing IEditorWorkerService), never a language-service worker, which is what actually disables IntelliSense/diagnostics.
- The shared overflow widgets container in monaco.ts (overflowWidgetsContainer()) carries the monaco-editor class because Monaco scopes its --vscode-* custom properties to .monaco-editor, .monaco-diff-editor, .monaco-component, so a plain document.body child had none.
- defineKiraTheme in monacoTheme.ts grew from ~16 to ~46 colors keys (menu, suggest, list, dropdown, input, widget, scrollbarSlider, peekView, findMatch, textLink) so widgets stop falling back to vs-dark hardcoded literals.
- normalizeColor in monaco.ts converts rgba() to #RRGGBBAA because editor.defineTheme colors go through Color.fromHex, which accepts only hex and returns red, not an error, for other forms; translucent tokens like --kira-scrollbar and --kira-search-match normalize to rgba.

- Build Monaco model URIs (kira-repo://<repoId>/<path>) with Uri.from, not string interpolation, because paths containing a space, #, ? or % do not survive a template-literal round trip.
- The repo-diff tab reads a path's HEAD-vs-worktree content via ReadDiff in internal/codeworkspace/diff.go: the worktree side through ReadFile classification, the HEAD side through the session's lazily-built catfile.Session, both at the same 8 MiB gate.
- The diff wire format has no status field: HEAD missing means the file was added, worktree missing means deleted.
- The diff editor is mounted readOnly/domReadOnly with renderMarginRevertIcon false and renderGutterMenu false, which is not cosmetic: both widgets surface revert/apply affordances that would let a user trigger a write.
- Keep the diff editor's diffAlgorithm at its pinned default; never use 'advanced-wasm' or 'advanced-external', because they resolve an external computer this bundle doesn't ship.
- The repo-diff tab has no session state (repoDiffTabStateSchema is {}); a restored diff tab re-reads both sides and opens at Monaco's first change.
- "Open changes" in the tree's context menu is gated on the tree's status glyph and opens the diff as a permanent tab, never the preview slot.
- The diff editor is the first real consumer of editor.worker; the Monaco chunk size was unchanged by the diff phase because the diff contribution was already inside register.all.js.
- A reveal (e.g. a search result) applies to a tab whose editor is already mounted and active: openRepoFileTab's reveal option calls requestReveal, which moves the cursor immediately if the editor is live and otherwise stores the request for the next mount.
- Code search uses no search library: RE2 (stdlib regexp), git ls-files and a bounded worker pool, with a hand-rolled scanner in internal/codeworkspace/search.go applying binary/size/long-line gates, UTF-16 columns and preview windowing. RE2 is linear time so user patterns can't hang the app.
- Indexed search engines (sourcegraph/zoekt, google/codesearch) were declined because they build and serve a second index that isn't needed when grepping the current worktree.
- Search enumerates files with the same codeworkspace.EnumerateAll as the project tree (git ls-files -z --cached --others --exclude-standard); .git internals are never reported and committed vendor trees are searched, with no hardcoded skip list.
- Search skips files that fail pathsafe.ValidateRelPath, because a search touches every worktree file and a committed symlink to something like ~/.ssh/id_rsa is a real risk.
- Search skips files with a NUL byte in the first 8 KiB or larger than MaxReadBytes (same rules as ReadFile), because a match in a file the viewer can't open can't be clicked.
- A NUL found later in a file is checked once per matching line and drops the whole file, including matches already collected; the skip count (FilesSkipped) is always reported, never silent.
- Match columns are computed by utf16Units, extracted from LineIndex.Position (internal/codeworkspace/textpos.go), so they follow the same UTF-16 rules and a click doesn't land a column off on non-ASCII lines; it is LineIndex's sole surviving consumer since P97 removed native code navigation.
- Search results stream over the coalescing push channel kira:code:search, flushing on 60 ms, an accumulated match count, or the terminal event (which always fires, even on cancel), emitted only to the window that asked.
- Only one search runs per workspace (Session.BeginSearch/CancelSearch): a new search cancels the previous one and Session.Close cancels it too; it is not a queue because the UI has one query box per workspace.
- Search results live in the repo panel's left-panel Files/Search switch rather than a new tab kind, so the result list stays visible while opened files take over the main area.
- In-file search uses Monaco's built-in find widget via the view.find command registered in RepoFileView.vue and RepoDiffView.vue; in the diff editor it runs against getModifiedEditor() because IStandaloneDiffEditor has no getAction and the worktree pane matches the file on disk.
- Only a true markdown file (monacoLanguageFor in views/repo/language.ts returning 'markdown') gets the Source/Reading toggle; .mdx is excluded because it colors as its own Monaco language, and every other file type is unchanged with no wrapper element or toolbar.
- Markdown files open on Source like every other file type; the reading view is opt-in. The choice persists per tab via repoFileTabStateSchema's markdownReading field (.default(false)) so tabs saved before the field existed still restore.
- Use v-show, never v-if, on the Monaco container for the Source/Reading toggle, because v-if would dispose and recreate the editor and lose scroll position, selection and find state.
- With automaticLayout: true, Monaco re-measures via a ResizeObserver that fires when v-show restores the container size, so the toggle needs no editor.layout() call; one is still made on the transition back to Source as belt-and-braces.
- The markdown reading view uses markdown-it (MIT, pinned 15.0.2), dynamically imported in views/repo/markdownReading.ts, so it is never in the boot bundle and never fetched unless a markdown file is opened.
- markdown-it was chosen over marked because its default html: false escapes raw HTML, which matters since the view renders a file from disk inside the app's privileged WKWebView; this avoids needing a DOMPurify-style sanitizer.
- Never set markdown-it's html option to true in the reading view, because raw HTML in files rendered in the privileged webview would then pass through unescaped. Other options are linkify: true, typographer: false, breaks: false.
- Every anchor click in the markdown reading pane calls preventDefault() unconditionally, because the webview has no back button and accidental navigation would strand the user until a restart.
- In the reading pane, same-page #anchor links still scroll to their heading (a heading_open renderer override assigns GitHub-style slug ids, numbered on collision); other links stay inert, with the target shown in the link's title attribute via a link_open override.
- Opening external links from the markdown reading pane is out of scope: OpenURL (pkg/application) validates nothing and a markdown file's links are untrusted. LinkService.OpenExternal (bridge/link.go) now exists, but the reading pane deliberately does not use it.
- Fenced code blocks in the markdown reading view have no syntax highlighting and render as themed monospace, because Monaco's editor.colorize() is async per block and this is a reading view, not a second editor.

### External file opens and default app (P332)

- `~/.kira-space/agent-secrets.db` (P453, 0600, both apps open it) holds the sealed agent credentials: table `secret_values`, scope `agent-credential`, owner `agent`, its own migration. Values never enter `kira.db`, logs or the op log.
- Kira Space is a Viewer for code and data files. `apps/kira-space/internal/filetypes` is the one extension vocabulary (41 code, 14 data); the generated `CFBundleDocumentTypes` (role Viewer, `LSHandlerRank` Alternate), `LSItemContentTypes` and `UTImportedTypeDeclarations` blocks in both plists come from it (`tools/gendoctypes`, markers `kira:doctypes:begin/end`, `--check` is verify-packaging S13). Excluded: html, txt, rtf, extensionless names, sqlite/db, ipynb, parquet, xlsx. `ts` resolves to MPEG-2 video on macOS; the registry reports a non-text type unavailable.
- Wails emits `ApplicationOpenedWithFile` once per file, cold start included. `openedfiles.Coalescer` batches adds (300 ms after the last) into one new window. `Resolve` follows symlinks, then maps a path inside an imported repo (longest root wins) to `{repoId, relPath}`, else keeps the absolute path. The window pulls its targets once through `OpenedFilesService.Pending(windowKey)`; `Forget` runs on window close.
- A path outside every repo opens a `local-file` tab in the general workspace, read through `OpenedFilesService.Read`, which accepts only an absolute, clean path with a supported extension. `RepoFileView` and `LocalFileView` share `useReadonlyFileViewer`.
- The File types settings pane writes LaunchServices through `filehandlers.Registry` (`NSWorkspace setDefaultApplicationAtURL:toOpenContentType:`; macOS cgo build only, `Unsupported` elsewhere and in `-tags server`). Status re-reads live state on every call; nothing is applied on launch. The previous handler is stored per extension in `file_handler_previous` before the first switch; switching off restores it, and a gone app leaves the extension on Kira Space and reports it as `unrestored`.
- Kira Space has 35 bound services (P452 adds `AgentContextService`, P453 `AgentSettingsService`, P454 `AgentPortService`).
- Not supported: Linux and Windows (no package; the single-instance lock drops a second process's argv), folders dropped on the Dock icon, editing or live reload of an opened file, a table view for data files, auto-importing an un-imported repo that contains the opened file.

### Git graph in the native workspace (C10)

- The pinned first tab of every repo workspace (views/repo/RepoGraphView.vue) mounts packages/git-ui's own graph by calling mount() directly in the native window, with no reimplementation and no fork of git-ui.
- ServeGitStream in apps/kira-space/internal/bridge/gitstream.go mirrors ServeEngineStream over gitrpc.Router as an in-process Wails stream, with no handshake and no pairing token because the peer is this process's own webview.
- packages/git-ipc/src/streamChannel.ts is a Wails-stream channel using the blob-frame body shape with no length prefix, no recvBuffer and no drain loop, because a Wails stream is already message-framed.
- Kira Space graph view stays mounted, hidden via v-show in MainView.vue, from first activation until its tab closes or 4 more recently used kept views displace it (evicted views restore cold from view state). Opt-in by TabKindDef.keepMounted; only repo-graph sets it. A full-layout module (ADE) keeps MainView mounted and passes `suspended`, so kept views go inactive.
- A Space file or diff editor's view state (scroll, cursor, folds) is saved on unmount and restored on the next mount, held with the tab's models in `views/repo/editors.ts` until the tab closes. A restored view state replaces the persisted `revealLine` reveal; a pending reveal request still wins.
- `ReadFile` and `ReadDiff` take optional `ifStat` {size, hash} (SHA-256 of the bytes) and `ifHeadOid`, and answer kind `unchanged` with no content for a side that still matches. The frontend sends them only while the tab's model is unedited (Monaco version id), and skips the read for a fixed commit whose models are cached. An unchanged answer still reads and hashes the file; it skips the payload, not the disk read.
- RepoGraphView.vue constructs a TabViewStateStore per mount; git-ui's unmount()/mount() serialize the lane layout, scroll position and loaded rows into the pinned graph tab's persisted state.viewState. Restore applies to a cold mount only (app restart, tab reopen).
- RepoGraphView.vue drives MountHandle.setVisible from its active prop (graphVisibility.ts's GRAPH_VISIBLE_KEY). A hidden graph defers grid rebuilds and GraphViewState auto-refresh (setSuspended) to one catch-up on show. RepoGraphView pushes appearance.dateFormat and the repo head through MountHandle.update.
- Change request ancestry is rebuilt with a base cutoff (state/changeRequests.ts rebuildAncestry): each baseRef is resolved locally via rowOfBranchTip (a decoration scan, no new RPC) and its ancestors excluded, so badges no longer tag nearly all loaded history behind a branch.
- Bursts of individually-resolving change request branches coalesce into one ancestry rebuild per tick via scheduleAncestryRebuild's nextTick drain.
- There is no gitrpc method allowlist (removed in P243 Part 2 with the extension): every gitrpc method is served, and a method the contract does not name does not exist.
- guardRepoSettingsSet in gitstream.go refuses repoSettings.set with E_READ_ONLY when its patch touches the prepare script or worktreeBasePath, which only Kira Space writes in-process (ADE repo config).
- The gitrpc Router refuses the prepare-script leaf on every connection (P172), and there is no gitrpc method for shell execution since 46 removed worktree.prepare.
- repo.open must never transitively arm a write, because via Registry.Acquire/EnsureAutoFetch it could arm a timer running git fetch --prune when git.fetchAutoIntervalMinutes is above its default of 0.
- gitsession.Conn has a noAutoFetch flag (DisableAutoFetch), set by ServeGitStream before serving requests, which routes Open through Registry.AcquireQuiet and skips EnsureAutoFetch on existing entries, so the native mount's repo.open never arms auto-fetch.
- The auto-fetch opt-out is scoped to one Conn only: RepoEntry and its timer are shared per repository, so another Conn on the same repository that does not opt out can still arm it.
- P67e kept the auto-fetch opt-out even though the native mount is no longer read-only, because every fetch the native mount runs should be one the user pressed a button for, not an unrequested periodic background fetch.
- CodeRepo.ID (code_repos.id) is used by native tab/workspace calls, while gitclient.RepoSummary.RepoID is used by git-ui and every gitrpc method; the two are never equal or interchangeable.
- repo/git/hostHandlers.ts's gitRepoIdFor/codeRepoIdFor are the only conversion between the two repo ID kinds, returning undefined on a miss rather than guessing.
- A commit diff extends the existing repo-diff tab kind: repoDiffTabStateSchema gained a nullable left/right/leftLabel/rightLabel revision pair, null for the HEAD-vs-worktree comparison and non-null for a commit diff, keyed by revision pair so different commits' diffs of one file get separate tabs.
- For a commit diff, RepoDiffView.vue reads both sides via two file.read calls over the git transport instead of control.codeWorkspaceReadDiff, reshaped into the found/missing/binary/tooLarge classification.
- A root commit's no-parent side reads git's well-known empty-tree object id rather than null, so file.read's existing missing classification handles it.
- TabViewStateStore (repo/git/viewStateStore.ts) backs git-ui's ViewStateStore with the pinned tab's persisted state (repoGraphTabStateSchema's viewState, a z.unknown() passthrough); git-ui's parsePersistedViewState is the sole validator of the version-6 shape.
- The git transport is cached one per repo workspace (gitTransportFor in repo/git/transport.ts), independent of tab mount lifecycle, and disposed only when the workspace closes.
- The native transport sends repo.open once per repo before the first request or stream naming it, so a reloaded Review pane or diff tab works without the graph tab (P231).
- git-ui has no token layer of its own: its markup uses the app's Tailwind utilities and --kira-* tokens, compiled by Kira Space's root styles.css which @sources packages/git-ui/src; the only git CSS is packages/git-ui/src/theme/git.css, imported by styles.css because @theme needs the host root.
- git.css defines the graph scale --kira-graph-t-* (xs/sm/md = graph font size -2/-1/0px), --kira-graph-h-xs and --kira-graph-row-h, exposed as text-graph-*, h-graph-row-compact and h-graph-h-xs utilities registered in packages/theme/src/lib/utils.ts so cn merges them.
- The lane palette graph-lane-0..7 aliases the connection palette --kira-conn-*, and SlickGrid structural rules (.kira-commit-grid .slick-*, in @layer components) live in git.css because SlickGrid builds those nodes itself.
- Other git colours are app utilities: status ok/warn/error, remote and stash origin info, stash and draft muted-foreground, rows bg-hover/bg-select, borders border-border; there is no kv- marker layer (lint bans it).
- TokenReader measures the graph tokens for SlickGrid's rowHeight and rowSvg.ts geometry.
- Test hooks in git-ui are data-testid; the grid's runtime classes are kira-* (kira-cell-message, kira-row-selected).
- Size a codicon span with a utility using the ! suffix (text-graph-xs!), because codicon.css's unlayered font shorthand beats layered utilities.
- check-theme-classes.sh fails any kv:, --kv- or --vscode- under packages/git-ui/src and packages/theme/src; Monaco's own --vscode-* stay scoped to its editor nodes.
- Chrome (dialogs, toolbar, review chrome, and picker popovers for branch, base, search results) reads text-kira-* and tracks Appearance font size (P256).
- Data surfaces (grid cells, detail meta, file tree, review rows) read text-graph-*, which follows git.graphFontSize (--kira-graph-font-size, set by applyAppearance only when above 0) and falls back to Appearance font size; the split is deliberate, as the grid is a data view outside P123's four-value chrome scale.

- The Git module follows the app look, with Kira Studio as the reference and never Agents/ADE.
- Git graph, detail pane, strip and toolbar sit on bg-bg; GitViewHead, the toolbar, the detail head and review headers are ViewToolbars.
- Commit rows are one line at --kira-graph-row-h-compact, the larger of --kira-row-height and --kira-graph-h-xs + 2px (28px in Space).
- A decorated row (ref or PR badge) adds a badge line above the subject, making --kira-graph-row-h = compact + --kira-graph-h-xs; the graph node stays on the subject line.
- Ref badges shrink and ellipsize their label and never clip; the +N chip never shrinks.
- The row plan publishes with its layout (#rebuildLayout), and loading goes idle only after the last streamed relayout, so a row never renders laneless.
- Never use semibold anywhere in git-ui and Space repo/; lint enforces it.
- Ref badges use Studio Badge tones: branch ok, remote info, tag/stash default, detached warn; lanes use the connection palette.
- Dialog bodies use gap-3 p-3 with Field/FieldDescription/FieldError, controls size="kira-lg", warnings Alert variant="warn" and failures Alert variant="error".
- Never use a raw label element or outline-focus in git-ui; lint enforces it.
- Empty panels use Empty/EmptyMedia/EmptyTitle; list rows use rowVariants menu/tree at 22px; section heads use text-kira-sm uppercase; FileTree indents 8px + depth * 14px with TreeTwisty and a 13px folder icon.
- Visual baselines for the Git module live in apps/kira-space/tests/visual/git-module.spec.ts.
- Raw checkboxes in git-ui were retired: all dialogs and FileTree.vue use components/ui/checkbox, leaving zero raw checkbox inputs in packages/git-ui/src; app-shell.css with its main.ts import and the kv-mount-root class was deleted.
- A graph restart at row 0 (after refsChanged or Refresh) never empties the live list: PackedStreamState with stageRestarts streams into a staging CommitStore, and once staging holds as many rows as the live store or the stream ends, commitStaged moves it in via CommitStore.replaceWith. Corruption and reset() discard the staging store.
- One synchronous #publish sets layout, plan and laneCount and runs onPlanPublished listeners; CommitGrid repaints inside it so a click or scroll in the same task never meets a stale grid.
- Never let the plan reference store rows the store lacks, because CommitGrid's generation watcher throws 'ShaTable: row N out of range' and SlickGrid half-updates (blank rows, missing commits).
- GraphOrderState.rebuild reconciles tips against the store: because refs reload before the graph re-walks, a tip sha the store lacks keeps the sha its group last used (HEAD by index, others by key), else HEAD's group would empty and the history collapse.
- The auto-refresh viewport restore anchors on the top row's sha, and CommitGrid resizes the SlickGrid canvas on a height-only change.
- A walk reset remembers its loaded row count (gitsession.Walk.keepRows); the next graph.stream re-reads whole pages until the store holds that many rows, so Refresh after Show more keeps the loaded depth.
- Change request facts (gitsession/changerequests.go) survive a refs change: markStale() drops only remote detection, stale entries (up to 1 h) serve at once with one single-flight background refresh per key, and the post-fetch purge reads fresh so closed change requests still purge.
- changeRequests.ts keeps last-known facts per repo (cap 8), restores them on setRepoId and revalidates.
- CommitGrid badges a change request only on the commit its head points at; the details pane keeps the ancestry-derived changeRequestForCommit.
- The graph column never renders narrower than graphColumnWidth(laneCount) (at most 12 lanes, 173px) in any mode.
- CommitGrid keeps a grow-only lane floor per mount: a refresh restarts laneCount at 0 and never shrinks it, and a repo switch remounts and resets it.
- On a first-ever mount (no persisted view state) the graph column tracks the lane floor, even below the stored width, until the user drags it; stored widths are preferences and only a drag writes one.
- Effective column widths come from columnFit.ts and always fit the viewport: author, then date, shrink to their minimums so the message column keeps 120px; the graph never shrinks; drags stop where the message column reaches 120px; stored widths return when the window grows.
- The graph viewport's overflow-x is forced hidden as a last guard against a sideways wheel scrolling the graph off screen.
- CommitGrid emits the drawn graph width and UncommittedChangesStrip sizes its cell from it.
- Edge records (EDGE_STRIDE 6) carry three lanes: fromLane (source node), EDGE_RUN_LANE (lane on pass-through rows, set at append, never patched) and toLane (target node); edges carry no kind.
- Convergence at the target row patches toLane only, so a branch-out edge that later converges keeps running in its own lane and bends into the claiming lane in its last row; rowSvg.edgeCommand draws from the three lanes.
- The graph walks git log --date-order (children before parents, which lanes need); the walk, the tail scan and the ranged review walk share it.
- Display order is date order. Only sibling chains forking from one commit that interleave are made contiguous, client-side in git-core graph/displayOrder.ts (localTopoOrder, flatqueue min-heap); with no such siblings display order equals store order.
- Group assignment (rowPlan assignRowRanks) is final on arrival; display order is not, since a later page can join two chains and rows next to the loaded boundary can move.
- Collapse contracts each maximal run of one non-zero group's consecutive rows (more than 3) to tip, placeholder and oldest; a group can form several runs, and expanding its key expands all of them.
- The graph has no fork stubs: the display order is topological, so every fork is a real lane, and an unloaded fork parent keeps a pending lane running off the bottom.

### Code review, ported natively (C11)

- In Kira Space, the review panel is a third left-panel segment (Repos/Files/Review) in GitPanel.vue, not a tab. A repo-review tab kind mirroring repo-graph was rejected because the review workflow (pick a file, read its diff, mark it, pick the next) would cost two tab switches per file and hide the diff being reviewed.
- ReviewView.vue is built for sidebar width and persists no per-mount viewState (it takes NullViewStateStore).
- GitPanel.vue is one persistent instance across repo workspaces, so its review mount is tracked per repoId (reviewActivatedRepoIds); switching Files/Review/Search within the same repo keeps it mounted (v-show), while switching repo workspace remounts it, relying on session resume.
- The chosen Repos/Files/Review tab persists in localStorage key kira.git.panelTab (shared by all windows, last write wins) and defaults to Repos; opening a repo never switches it, and the panel shows Repos without writing it while no repo workspace is active.
- The thirteen review.* methods never touch the user's repository: their only persistence is review.db (INSERT/UPDATE/DELETE on review_session/review_file/review_range/review_comment), and internal/gitreview has no exec.Command, os.WriteFile or git invocation.
- The git work review.* handlers perform is read-only porcelain (cat-file, merge-base --is-ancestor, diff), with no update-index, write-tree, commit-tree or ref update.
- "Open in graph" from a review row is graph.revealCommit ({repoId, sha} -> {revealed}), answered locally by repo/git/hostHandlers.ts and never reaching the Go server; the status-bar blame item click reuses it.
- review.session.save/.load are not Go methods; review session resume uses repo/git/reviewSession.ts via the pinned graph tab's state.
- Review session resume stores state in a second opaque field, reviewSession, on repoGraphTabStateSchema beside viewState; reviewSession.ts reads/writes it like viewStateStore.ts, and the host treats the value as opaque (git-ui's state/review.ts defines and validates the shape).
- Review comment anchoring needs no client-side line mapping: the diff's right side is <branchTip>:<path>, ranges are in those coordinates, and cross-revision drift is resolved server-side in gitsession/comments.go anchorOne (blob-oid equality, then git diff projection, then stale/removed label).
- reviewDecorations.ts places a decoration at comment.range.start and renders anchor as a hover label, with no mapping of its own, so panel and editor never disagree about position.
- The review diff has no glyph margin (`glyphMargin: false` on both panes; Monaco forces one onto the original pane side by side, so reviewDecorations.ts switches it back off on change). The hovered line's number shows the add mark (`lineNumberClassName`), a commented line's number is tinted and opens its thread, and `selectOnLineNumbers` is off on the right pane. Each hunk with new-side lines gets a one-line view zone above its first changed line; its margin node holds the mark button (`ReviewHunkMark.vue`). View zones are not model lines, so anchors, ranges and navigation keep their coordinates, and the diff editor aligns the left pane to the zone (P413).
- In the review diff, glyphMargin is still set explicitly (now false) because monaco-editor@0.56.0's .d.ts prose and registered option default disagree.
- The review diff collapses unchanged regions like the plain diff (3 context lines). monaco-editor 0.56.0 has no public API to expand one line, so reviewDecorations.ts's revealCommentLines selects every comment line; HideUnchangedRegionsFeature's onDidChangeCursorPosition listener (skips only ContentFlush) then unhides each, contiguously from the nearer region edge, so a comment mid-region exposes more than 3 context lines. It reruns on onDidUpdateDiff and alternates the cursor column, since the event fires only when the primary cursor moves.
- A folder checkbox in the review file tree sends one `review.markMany` (whole files, one branch-tip resolution, one repaint) for the files under it in the filtered tree; state is all-full, all-none or mixed.
- Next/previous change uses diffNavigation.ts's own change index over `getLineChanges()`, not `goToDiff`, which wraps around and cannot report the last change. Past the end it opens the review list's adjacent file through the `reviewNavigation` store, landing on that file's first or last change.
- All repo diff editors follow one per-device layout choice, `kira.repo.diffLayout` (side by side or inline); `useInlineViewWhenSpaceIsLimited` stays false so width never overrides it.
- A comment thread is a spacer view zone plus an overlay widget hosting ReviewThread.vue, mounted with createApp and unmounted with the zone. This is VS Code's ZoneWidget mechanics over public API, since `ZoneWidget` is not public. The widget is as wide as the visible editor, its height is capped at `max(12 lines, 80% of the editor height)`, and a ResizeObserver re-lays it out; a wheel over it scrolls it, not the editor. The spacer zone uses `showInHiddenAreas` so a thread beside a collapsed region stays visible. Zone ordinals: thread 0, hunk row 1.
- repo/git/transport.ts fans a mutation's success out to every open review surface (onReviewRepaint) because the panel and editor share one Transport per repo workspace.
- The review banner zone carries z-index 10 (VIEW_ZONE_Z_INDEX), because Monaco 0.56.0 appends .view-zones before .view-lines (position absolute, z-index auto), so a later z-index auto sibling loses hit tests; 10 is VS Code's own value.
- review.comment.edit takes id, body and kind; it keeps range, state and reply and clears sentAt. Thread actions (send, edit, state, delete) show their error inline in the thread.
- A fully-reviewed file's whole-line tint is skipped: paint() guards on coverage over the full line range being 'full'; per-hunk marks and the partial-review overview ruler are unchanged.
- The file tree's mark-reviewed control is a real checkbox input with a genuine indeterminate state for partial review, and is click.prevent so the server's applied result.review is its only state, avoiding an optimistic toggle contradicting it.
- review.open's native answer switches the panel segment, ensures visibility, and pushes a review.target event over transport.ts's local event bus; a pending-target map covers the race where first activation mounts RepoReviewView.vue after review.open fired.
- "AI-review feedback" has no AI integration: users write plain-text line comments shown as gutter icons and threads, and review.comment.export formats them for manual pasting into an AI conversation.

### Git blame, inline (P62)

- P62 (inline git blame) reused the v1.4 P5 backend with no Go change to blame and no ContractVersion or method-set bump.
- blame.line runs one git blame spawn per line, blames the working tree only, and takes no atSha.
- The only Go touched by P62 is internal/storage/{model,repos}/settings.go, to persist the inlineBlame setting.
- The inline blame annotation renders as injected text at the end of the cursor's line in views/repo/RepoFileView.vue, because that surface's displayed bytes and blame.line's answer are the same document by construction; diff tabs compare historical revisions blame.line cannot answer.
- P62 declined a status-bar blame item, but P76 shipped it as a left-side status-bar item.
- Whole-file gutter blame is deliberately not built, because blame.line is one spawn per line; it would need a new multi-hunk porcelain parser, a new blame.file method, and a ContractVersion bump.
- views/repo/blameLine.ts owns the blame.line request, debounce, dedupe, per-mount cache and cancellation, so the status-bar renderer shares one request; blameAnnotation.ts keeps only the Monaco decoration/hover/reveal-action rendering.
- blame.line requires the calling connection to already hold the repository, so the blame controller ensures this with a memoised repo.open, which is idempotent per (connection, repoId).
- The blame request uses a cursor-line trigger, 150 ms debounce, dedupe on the line, a per-mount cache, and repo.changed invalidation; the cache is sound because the model is immutable for the mount's life.
- There is no dirty-buffer state for blame because the repo file view is readOnly/domReadOnly and nothing writes the model, so the buffer is always the saved file.
- Blame failures (untracked path, line past EOF, aborted request) render nothing, silently.
- RepoFileView.vue creates the blame controller whenever the file is blameable, regardless of appearance.inlineBlame; the setting only attaches or detaches the inline renderer, so the status bar still has data.
- The status-bar blame item publishes through state/blameStatus.ts, an owner-token store mirroring cacheStats.ts/appMetrics.ts, so workbench/ never imports views/repo/.
- blameable requires gitRepoId !== undefined && rev === null, because revision-pinned repo-file tabs read through file.read and blame.line only blames the working tree; a pinned tab shows no blame rather than wrong blame.
- The context-menu action "Open Blame Commit in Graph" and the status-bar item click both reveal the blamed commit in the pinned graph tab via graph.revealCommit.
- hostHandlers.ts has a pending-target map pendingBlameRevealByCodeRepoId, consumed once by RepoGraphView.vue's mount as MountOptions.pendingUiAction, to solve the cold-mount race when the graph tab has not yet mounted.
- Date formatting for git UI lives in @workbench/behaviour/format (formatAbsoluteUtc, formatAgeCompact, formatDateTime); packages/git-ui/src/lib/dateFormat.ts keeps only the DOM-bound measureAbsoluteDateWidth, with no second copy.
- appearance.inlineBlame is a settings toggle, defaulted on, that gates only the inline renderer and is watched live by RepoFileView.vue so toggling affects open tabs immediately.
- Settings are Go-persisted, one row per leaf, typed via AppearanceSettings/AppearancePatch in internal/storage/model/settings.go; a leaf without a Go counterpart is silently discarded on Set and reverts to its zod default on read.

### Git host abstraction (P318)

- internal/githost is the neutral contract. A Provider composes four roles: Remotes (ParseRemote, Claims), Auth (Status, Account; both take the Repo, since Azure signs in per organization), ChangeRequests (ForCommit, ForBranch, Open) and URLs (ChangeRequestURL, RepoURL, BranchURL, IsChangeRequestURL, ParseLink). Callers depend on the narrowest role.
- "Change request" is the neutral name for a pull or merge request. A provider supplies Terms (label, change request, noun, short, CLI); the UI reads them from githost.Info. No neutral caller holds a provider word.
- providers.New builds the Registry: github first, then extras. appwire default is github then azure; flowharness order is github, azure, extras (`WithAzure` swaps the az runner). `githost.Status.Site` carries the account scope (Azure organization URL). appwire.Options.GitHosts (a Registry; nil is github alone) and flowharness.WithGitHost add providers. flowharness/fakehost is the test-only second host (forge.test, merge requests).
- Selector.Resolve(setting, remoteURL) picks the provider. Setting kiraSpace.gitHost.provider (per repo) is auto, off or a provider id. auto picks the first provider that parses the origin remote and Claims its host. An unknown id logs a warning and falls back to auto. A pinned id that does not parse the remote resolves to none.
- The server composes every link (changeRequest.browserUrl, ChangeRequestsResult.BranchURLs, HostLink on tasks); the renderer never builds a host URL. Only URLs a provider approves via IsChangeRequestURL open. Contract 53 drops GitHostInfo.fileReviewSync (P369). Contract 54 adds changeRequest.reviewState, changeRequest.submitReview and changeRequest.comment (P370). Contract 55 adds optional number on changeRequest.reviewState (P392).
- git-ipc adds gitHost.info (the repo's resolved host, null for none) and gitHost.providers. Space adds GitHostService.Providers and AdeTaskService.HostProviders (phone). Contract version 50 renames the PR-named requests and fields to change request names. Contract 51 adds branch.changeRequestIndex, branch.syncChangeRequests and the changeRequestsChanged event kind.
- ChangeRequestSearch (P417) is an optional role: SearchChangeRequests(key, limit) finds change requests by text in the title. GitHub runs `gh search prs --match title`, open and merged. CLISetup (P417) is an optional role: Setup (docs URL and install, login, check steps) and CLIStatus, for the Settings CLI tools pane.
- Breaker, snapshot and per-branch caches live in the repo entry's host state, not in a provider. The snapshot and per-branch caches are the fallback for a provider without ChangeRequestSync and while the persisted index is incomplete.
- ChangeRequestSync (P352) is an optional role: Page (newest-updated first, cursor), Probe (conditional listing check) and Checks (batch refresh by node id). GitHub implements it with GraphQL pages, `gh api -i` plus If-None-Match for Probe, and nodes(ids) for Checks. A 304 probe costs no rate limit. The 304 path is covered by golden header blocks only; the sandbox proxy blocks the GitHub API, so no live 304 was observed.
- The index persists in git_host_change_requests and git_host_sync_state (migration 0047). Key `repo_key` is `lower(provider|host|owner/name)`, so clones and worktrees of one repo share rows. Times are INTEGER ms. head_owner is NOCASE. Sync state holds etag, watermark_updated_at, backfill_complete and cursor, last-sync stamps, account and schema_version.
- Sync order per run (gitsession/crsync.go, one single-flight crSyncer per RepoEntry): backfill while incomplete (resumable pages, newest first, PageCap 10 per run, cursor saved); else Probe, and on change a delta walk from the top until updatedAt falls below watermark minus 2 min; then a checks refresh. Hitting the page cap in a delta resets backfill_complete and saves the old watermark as delta_floor (migration 0049); the resumed walk completes on reaching it.
- Checks refresh: pending or expected non-draft rows every 2 min while focused, every open row on a 15 min sweep. A node missing from the result deletes its row. A merged or closed row deleted on the host stays until invalidation, since only open rows are checked.
- Triggers: start (2 s delay), timer (5 min focused, 15 min blurred), refs change, and branch.syncChangeRequests (focus regain; `wait` blocks until the run ends). All but the explicit call are off when CRSyncConfig.Disabled. MinGap 30 s and the rate-limit gate bind every trigger, a backfill continuation included (armed past MinGap); an explicit one skips only the failure backoff.
- Failure: cenkalti/backoff/v4 ExponentialBackOff (30 s to 15 min), a hard gate until the host's reset time, the existing per-repo breaker, and a stop under 200 remaining GraphQL budget. A transient remote-read failure never drops rows.
- Invalidation: a different signed-in account or a bumped GitHostSchemaVersion drops the repo's rows. Turning the host setting off drops them. Rows of a repo with no sync-state write (updated_at, migration 0048) in 30 days are pruned once per process, at the first Registry.Acquire before any syncer reads state, so no run can resurrect a pruned repo as complete.
- Delivery: repo.changed kind changeRequestsChanged goes through the coalescing subscriber after a run that changed rows. branch.changeRequestIndex serves one record per head branch from SQLite (newest row per branch picked in SQL, so cost tracks branch count), reports `complete` false when refs cannot be read, only branches present in refs plus open requests; git-ui fills byBranch in one assignment, skips per-branch lookups when `complete`, and reloads on the event (one in flight, one queued). Repo-scoped git reloads ignore the kind.
- ADE storage: ade_tasks.host_url. Migration 0044 renames github_url, ade_gh_synced and pr_number. Migration 0050 (P369) drops ade_viewed_sync. Migration 0051 (P379) adds ade_task_statuses with 4 seeds and ade_tasks.status_id, status_by, status_step, backfilled from the entry step. Migration 0055 (P405) adds ade_task_plan.backlog. Migration 0056 (P405) folds ade_backlog rows into repo-less tasks on the Backlog shelf and drops the table. Migration 0059 (P411) drops ade_task_statuses.category, renames tone to color (16 colours) and adds ade_task_status_defaults, seeded from the first status of each old category.
- ChangeRequestReviews (P370) is an optional role: ReviewTarget (fresh state, head and author), SubmitReview (event approve, comment or requestChanges, body, commit) and Comment (conversation). GitHub uses REST: GET pulls/{n}, POST pulls/{n}/reviews, POST issues/{n}/comments. Every field goes through `gh api -f`, never `-F` (an @-prefixed value would be read from a file). A 422 reason is GitHub's first errors[] entry.
- BranchLinks (P383) is an optional role: ParseBranchLink (pull, tree, issue links on a claimed host; no host call), ChangeRequestHead (fresh head, author) and ChangeRequestRef (refs/pull/<n>/head on GitHub). ADE ResolveHostLink is read only; AddHostLink re-resolves, fetches one refspec (fork or closed PR: the change-request ref into <owner>/<head>; same-repo open PR: the head into its tracking ref) and attaches through the AddExistingBranch path, so kind follows the tip-author rule. Repo match walks every remote, origin first. Issue links are refused.
- ChangeRequestDetails (P427) is an optional role: ParseChangeRequestLink (a change request link on a claimed host; no host call) and ChangeRequestDetail (one request's description, conversation, reviewers and checks). It has its own link parser because `fakehost.Forge` lacks BranchLinks and adding it there would change `BranchLinkLabels()`.
- ChangeRequestComments (P429) is an optional write role, apart from the read and review roles: ReplyKinds (thread kinds a reply may target) and PostChangeRequestComment (body, replyTo). GitHub posts through `gh api -f` and replies to code threads with GraphQL `addPullRequestReviewThreadReply`. Azure posts through `az devops invoke`. See Comments (P429).
- No CI status role exists. Add one as a new optional role, like ChangeRequestSync.

#### Azure Repos (P423)

- `githost/azure` is the `az` provider with `az devops` (extension azure-devops). Same roles as GitHub: Remotes, Auth, ChangeRequests, URLs, ChangeRequestSync, ChangeRequestReviews, BranchLinks, ChangeRequestSearch, CLISetup.
- Repo mapping: `Repo{Host: "dev.azure.com", Owner: "<org>/<project>", Name: "<repo>"}`. Remotes parsed: `dev.azure.com`, `{org}.visualstudio.com` (optional collection segment), ssh `v3/` forms. Links parsed: PR, work item, `?version=GB<branch>`.
- Every org-scoped call goes through one `argv()` helper: `--org <url> --detect false --only-show-errors -o json`. Env sets `AZURE_EXTENSION_USE_DYNAMIC_INSTALL=no` and `AZURE_CORE_ONLY_SHOW_ERRORS=true`. `--detect false` stops az reading the cwd git remote.
- Discovery: tool probe (az version and extension) cached apart from the per-org probe (`az devops project list --top 1`) and the cached `az devops configure --list` defaults. TTL 5 min when ok, 30 s when not. A refresh forgets the defaults.
- Sync: Azure gives no PR update time. `UpdatedAt` is the creation time; Page orders by id descending; Probe uses the newest PR id as etag. Page marks open rows `ChecksState=EXPECTED`, so the pending checks pass reads their policies in the same run. Checks lists active PRs and runs `az repos pr policy list` per PR (limit 4); a PR that left active is read with `pr show`; a gone PR is omitted.
- Reviews: the body posts as a new thread through `az devops invoke` (git, pullRequestThreads, API 7.1) from a 0600 temp file, then `set-vote` (approve to approve, requestChanges to wait-for-author, comment to no vote). `ForCommit` uses the pullRequestQuery resource.
- Azure serves no PR head ref. `ChangeRequestRef` is `refs/pull/<n>/merge`, so ADE fetches the merge ref.
- Differs from the GitHub op log: `az` calls are not in the op log. Flows use the in-process `flowharness/fakeaz`, not the binary.

#### Pull request view (P427)

- Window: `HostViewService` (bridge) opens one window per change request. Key `hostview-<uuid>` (`HostWindowKeyPrefix`). Ephemeral: never restored, row dropped on close, never the last window. `main.go` purges `hostview-` rows at boot, before `Windows.List()`. `OnWindowClosing` forgets the key in `HostViewTargets`.
- Calls: `Open(url)` resolves the link against the registry, stores the canonical URL as the target and focuses an existing window for the same request (`opened` false). `WindowTarget(windowKey)` answers the window's target, `null` for any other window. `ChangeRequest(url)` reads the detail and answers kind `ok`, `unavailable` (reason in `status`) or `gone`. A host failure is data, not an IPC error. A non-change-request link is refused as bad request.
- Breaker: per repo (`provider|host|owner|name`). A rate limited read arms it until `ResetAt`, else 15 minutes; reads in that window answer `unavailable` without a host call.
- GitHub: one GraphQL query through `gh api graphql`. Azure: three concurrent `az` reads: `repos pr show`, `repos pr policy list` and `az devops invoke` GET `pullRequestThreads` (newest 200 threads). Every read carries a `--query` projection, since `toolexec` caps stdout at 1 MiB.
- Caps (`githost.DetailMax*`): 100 conversation comments, 50 reviews, 50 code threads, 30 comments per thread, 100 checks, 50 labels, 50 reviewers and 50 assignees. The newest stay; truncation flags show a note.
- Markdown: `frontend/src/hostview/markdown.ts` (markdown-it). Raw HTML never renders: comments and tags drop and a block's text shows escaped. Images render as links. Only http, https, mailto and `#` links pass; a click opens http(s) in the OS browser through `openExternal`.
- Frontend: `main.ts` asks for the host view target before the review target; a window with one skips workspace boot, like a review window. `App.vue` renders `HostViewWindow`. Refresh and Retry refetch; nothing refetches on focus.
- `HostViewTarget.Kind` is `changeRequest` or `issue` (P428). `Host`, `Repo` and `Number` are set for change requests only.
- Entry points: the ADE branch panel's PR row and the task tab's host link row offer "View". `OpenHostViewChangeRequest` is a package function for a later opener.

#### Issue view (P428)

- Same window, service and frontend as the PR view. Target kind `issue`: `{url, provider, tracker (terms), key}`. Identity is `issue|<provider>|<site>|<KEY>`; a bare key resolves its site through the cached tracker status, so a key and its link share one window. Title "KEY · Label".
- `Open(urlOrKey)` takes a change request link, else an issue key or link of a tracker that implements `IssueDetails`; anything else is a bad request naming both. `OpenHostViewIssue` is the package function for a later opener (P430).
- `Issue(ref)` answers kind `ok` (detail), `gone` (reason `notFound`) or `unavailable` (reason: unavailable, unauthenticated, otherSite, timeout, failed). A tracker failure is data, not an IPC error. No breaker: acli has no rate-limit signal and every call is bounded (10 s per acli call, 25 s per detail).
- Every load is a fresh read. The detail drops the 60 s `Issue` cache entry and runs two acli calls at once: the `Issue` read (Development summary, PR rows) and `workitem view` with `jira.DetailFields`. A failed `Issue` read leaves the detail with `changeRequestsReason`.
- Caps (`issuetracker.DetailMax*`): 100 comments (newest stay), 50 labels, 50 components. Comments show oldest first.
- Frontend: `HostViewFrame.vue` holds the chrome shared by `ChangeRequestView.vue` and `IssueView.vue` (title bar, loading, failure with Retry). `IssueView` is two columns at 900 px and up, stacked below. The sidebar opens a pull request row in the PR view and the parent in its own issue view. Entry point: the task tab issue line "View issue" button.

## Agent harness (P316)

The agent CLI sits behind one interface, so a second provider needs no caller change.

- `internal/harness` holds the neutral contract. `Provider` is one value with these required roles:
  `Locator` (find the CLI), `Interactive` (the PTY launch, resume, first prompt), `Sandboxed` (what the OS
  sandbox opens and denies) and `MCPHost` (MCP config, tool names, user servers, installing Kira's
  servers). Names (`Label`, `ProductLabel`, `HeadlessLabel`, `Command`), `Models` (aliases and a default)
  and `ModelLabel` are plain methods.
- Optional roles are found by type assertion, so a provider compiles without stubs: `Structured` (one
  isolated JSON call, memory; `harness.StructuredOf`), `Headless` (a streamed run with an MCP report, ADE
  steps and smart scripts; `harness.HeadlessOf`), `HookHost` (hook events for interactive sessions;
  `harness.Hooks` starts every host and composes launches by provider id), `UsageReporter` (feeds
  `agentusage`), `SessionLocator` (finds the id of a session the agent assigned itself) and `CostCapper`
  (`CapsCost()`; `harness.CapsCost(p)` is false for a provider without it). A provider without hooks has no
  activity icons and no finished/needs-input notes; without usage, no usage item; a smart script or a
  step with a budget on a provider without a cost cap is refused (`agentBlocker`, `backend.core.agent.noCostCap.body`).
- `SettingsHost` (P453, optional) takes the app's agent settings: `SettingsSchema` (the provider's section of
  `settings.yaml`, self-contained JSON Schema), `RenderSettings(AgentSettings, LaunchKind) (NativeConfig, error)`
  and `KiraAdds`. `AgentSettings` is resolved env, enabled MCP servers, `Allow`/`Deny`, user hooks minus the
  provider's `skip_hooks`, and the provider's raw section. `NativeConfig` is files (role, name, bytes, `Inline`
  for a provider that takes bytes through env), env and not-applied notes; it writes nothing. `AppConfig` is the
  same after Kira wrote the files (0600): `Files` by role, `Inline`, `Env`. It reaches `InteractiveSpec.App` and
  `HeadlessSpec.App`; `Launch.ConfigFile` is the settings file the launch uses.
- `ConfigMerger` (P453, optional on a hook host) folds Kira's hooks into that file: `harness.Hooks.Compose(provider,
  terminalID, cwd, command, configFile)` returns the command, the hook env and an error. Claude's `ComposeInto`
  prepends Kira's entry to every event and wraps `statusLine`; a read or write error fails the launch.
- Claude maps `RenderSettings` to `--mcp-config` (app file joins the grant's), `--settings` (hooks, permissions,
  `model`, `statusLine`) and env. A headless render drops `defaultMode`, `allow` and `statusLine`: an unattended run
  must not enter plan mode or widen its tools, and `claude -p` never runs a status line; `deny` still applies.
  `disableAllHooks` stays only on isolated runs that tick a user CLI server with hermetic off (P444); hermetic runs tick app servers and take app settings.
  Details in "Agent settings (P453)".
- A missing optional role is handled per caller. Memory (`memory.AgentRunner`) falls back to the default
  provider: background work with no per-run provider choice. Smart scripts refuse: `plan` sets
  `Preview.Blocker` to the `harness.MissingRole` text (`backend.core.agent.missingRole.body`), which covers
  Start, schedule and resume. ADE steps, smart steps, rebase and review rounds refuse in `superviseAgent`:
  the run fails with the same text before the sandbox starts. `MissingRole` matches
  `harness.ErrUnsupported`.
- `Interactive.InteractiveCommand` returns a `harness.Launch{Command, Env, SessionID}`. `Env` is for a CLI
  that takes config through the environment; it never crosses the bridge. `Tracker.Prepare` keeps it on the
  pending intent and `Compose` returns it ahead of the hooks env. Continue in terminal has no `Compose`:
  `scriptruns.ResumeCommand` returns `{command, token}`, the token is `""` when `Env` is empty, else a
  single-use `LaunchTTL` token the terminal open sends back as `agentLaunchToken`;
  `ScriptLauncher.BeginAgent` checks it against the command and returns the env, a stale or mismatched
  token is the `backend.core.scriptRun.expired.body` conflict. `SessionID` is `""` when the agent assigns it
  after launch.
- Hook events carry a neutral `harness.EventKind` (`SessionStart`, `SessionEnd`, `UserPromptSubmit`,
  `PreToolUse`, `PostToolUse`, `Notification`, `Stop`); the values equal the wire strings, so the bridge
  and frontend see no change. The provider maps its own names (Claude: table in `claude/hooks/http.go`,
  unknown names dropped) and sets the Go-only flags `Wakes` (tool start that arms a self-wake) and
  `NeedsInput` (notification asks the user). `agentnotify` and `Tracker.HandleEvent` read only kinds and
  flags. The frontend still reads Claude's `WAKE_TOOLS` and `notification_type` rules itself.
- Two providers ship: `internal/harness/claude` and `internal/harness/opencode`. `providers.New` is the
  single registration point (claude, opencode, then any extra). Everything provider-specific lives under
  its package: argv, stream parsing, MCP config shape, sandbox profile needs, hooks, installer and error
  texts. `harness.ResolveCLI` (`internal/harness/locate.go`) is the shared locator both use: PATH, the
  login shell, then the provider's candidate paths; each provider adds its own names and candidates.
- OpenCode (CLI 1.19.5, P449) implements Locator, Interactive, Sandboxed, MCPHost, Headless, HookHost,
  SessionLocator and CostCapper (no cost cap). It has no Structured role (memory falls back to the default
  provider) and no UsageReporter. Facts about the CLI live in `opencode/facts.go`, each marked verified or
  UNVERIFIED with its probe id; Known open items lists the unverified ones.
  - Interactive: `opencode [-s <id>]`. One owner per config layer: the MCP grant rides `OPENCODE_CONFIG`,
    extra directories ride `OPENCODE_PERMISSION`, the agent context rides `OPENCODE_CONFIG_CONTENT`
    (`{"instructions":["<ContextFile>"]}`), and the hook plugin rides `OPENCODE_CONFIG_DIR` (hooks dir
    with an `opencode.json` loading `plugin.js`). The model is never passed.
    OpenCode assigns the session id itself, so `FindSession` runs `opencode session list --format json`
    in the worktree and takes the newest session created after the launch. Kira never opens OpenCode's
    database or config.
  - Headless: `opencode run --format json`, prompt on stdin, `--dir`, `-s` to resume, `--pure` unless the
    run loads project config. A run that loads no user config redirects `XDG_CONFIG_HOME` to a directory of
    symlinks that omits `opencode`, and sets `OPENCODE_DISABLE_CLAUDE_CODE=1` and
    `OPENCODE_DISABLE_PROJECT_CONFIG=1`. `OPENCODE_CONFIG` carries the MCP grant (several configs merge into
    one temp file); `OPENCODE_CONFIG_CONTENT` carries the permission document plus `instructions` when the run has a `ContextFile`.
  - Tool names stay in Kira's vocabulary (Claude names, `mcp__server__tool`) all the way to the provider.
    `permissionDoc` translates them to OpenCode permission keys (`srv_tool`, `srv_*`) and fails the run
    naming any entry it cannot translate, so a step's `allowed_tools` and `rebaseBuiltins` need no mapping
    at the caller. Pattern maps keep insertion order: OpenCode takes the last match.
  - Hooks: a Kira plugin (`opencode/hooks/plugin.js`, loaded by a `file://` plugin spec) posts to a
    unix-socket HTTP server guarded by a per-process token; plugin event names map to neutral
    `EventKind`s in `hookEvents`. No wake events.
  - `docs/DEV_ENVIRONMENT.md` lists the dev install and the probes that need no model key.
- `harness.Selector` resolves `Active()` from the `agent.provider` setting per use; an unset, unreadable or
  unregistered id falls back to `claude` with a warning, never an error. `For(id)` serves a stored
  conversation and returns `ErrProviderGone` for an unregistered id, mapped to the
  `backend.core.agent.providerGone.body` conflict. Settings validation accepts any well-formed id, so a
  provider can be removed without bricking the setting.
- Run binding: `ade_sessions` and `script_runs` store `provider`. New work uses the provider its caller
  names (table below) or `Active()`; resume, take over and Continue in terminal use the stored provider,
  even after the setting changes.
- Which agent starts a new conversation, in order: the launch's own pick (the plus menu's
  `StartAdHoc{provider}`), then the workflow step's `agent` key, then the Agent setting. Resumes never
  choose: they keep the provider that started the conversation.

  | Entry | Provider |
  |---|---|
  | "+" menu (`StartAdHoc`) | `provider` argument, else the Agent setting |
  | `LaunchStep`, `StartBranch` (ADE Start) | the step's `agent`, else the Agent setting |
  | Headless step run, review round | the step's `agent`, else the Agent setting (`agentForStep`) |
  | Review agent's new conversation | the review step's `agent`, else the Agent setting |
  | Take over, resume, restart, Continue in terminal | the stored provider |
- Workflow step key `agent` (agent, review and manual steps with `session: true`; `""` removes it): a
  registered provider id, validated by the parser from `providers.New().IDs()`. The editor shows it in the
  step's Advanced section.
- `agentsvc.Service.List()` returns `Entry{AgentInfo, available}` for every registered provider, in
  registration order; `available` comes from `Locator.Available()` with no login shell. The frontend reads
  it through `useAgentList()` (key `['agent','list']`, under the `['agent']` prefix a settings save
  invalidates). Space shows a provider `Select` in Settings, Agent (a missing provider is disabled), one
  plus-menu entry per installed provider when there are two or more, and an Agent select in the step
  editor. Studio has no picker.
- A provider that assigns its own session id headless too reports it through `Handler.OnSessionID`: the id
  comes from the first stream event, else `FindSession` after the run. A headless `ade_sessions` row may
  hold `''` until then; resume paths refuse an empty id (`backend.space.ade.sessionIdUnknown.body`,
  `backend.core.scriptRun.sessionIdUnknown.body`). A smart run stores the id from the same callback.
- Models: `memory.models` maps provider id to alias; smart scripts store an alias in their own `model`
  field, validated against the union of registered catalogs.
- UI derivation: the bound `AgentService` (`internal/harness/agentsvc`, both apps) answers `Info()` for the
  active provider and `InfoFor(id)` for a stored one: `AgentInfo{id, label, productLabel, command,
  headlessLabel, models, defaultModel, hooks, usage}`. The frontend reads it through
  `packages/workbench/src/agent/agentInfo.ts` (TanStack Query, keys `['agent','info']`,
  `['agent','infoFor',id]`; a settings save that changes `agent.provider` invalidates `['agent']`) and
  feeds `{agent}`, `{product}`, `{headless}` and `{command}` to every catalog message as ambient defaults
  (`setMessageDefaults`), so templates need no per-call param. The phone reads `GET /api/agent/info`.
  Settings hide the hook-only notification switches and the usage switch when `hooks`/`usage` are false.
  Provider labels are plain Go strings, not catalog keys.
- Outcome and sandbox texts name the provider (`runoutcome.Agent`, `agentsandbox.Namer`).
- Still Claude-specific by nature: `internal/flowtest/fakeagent` keeps its `claude` persona (the Claude
  CLI double); real-claude tests (`apps/*/tests/claude`) test the Claude provider; the built-in tool
  vocabulary of scripts and the `--allowedTools` argv use Claude's tool names, which a new provider maps in
  `MCPHost.ToolArgs`; the `--color-agent` CSS token keeps Claude's colour value.
- Stage settings (P431): `Provider.StageSettings()` declares per-workflow-stage settings (`harness.StageSetting`: key, kind `int`, min, max, i18n label and hint keys). The step's validated values reach the provider as `HeadlessSpec.Stage` and `InteractiveSpec.Stage` (`harness.StageValues`); only the provider maps them onto its CLI. `providers.StageCatalog()` is static, like `ConfigDirs`, and feeds YAML validation and the editor.
- Claude declares `auto_compact_window` (100000 to 1000000 tokens, CLI 2.1.316) and maps it to `--autocompact <n>` in `Script` and `InteractiveCommand`, ahead of `--setting-sources`/`--add-dir`. Per process, so tools the agent runs do not inherit it. An old CLI without the flag fails the launch loudly. Measured with `claude -p --debug-file` (effective window = setting minus 20000): the flag alone gives 280000 for 300000. An exported `CLAUDE_CODE_AUTO_COMPACT_WINDOW=150000` wins over `--autocompact 300000` (130000). Kira does not strip the variable; see Known open items.
- `ContextFile` (P452): neutral field on `InteractiveSpec` and `HeadlessSpec`, path of the agent context snapshot. Only the provider maps it. Claude: `--append-system-prompt-file '<path>'` in both builders, flag name in one constant (`appendSystemPromptFileFlag`). The flag is hidden from `claude --help` but present in CLI 2.1.316 (named in the `--bare` help, strings in the binary). Rejected: `--append-system-prompt` (text in argv, visible in `ps`, size-limited), a generated `CLAUDE.md` plus `--add-dir` (also loads the dir's other files).
- OpenCode `ContextFile` (P452, verified on 1.19.5, Q-OC1 to Q-OC4): `instructions` in `OPENCODE_CONFIG_CONTENT`; the file's text reaches the model. OpenCode re-reads it on `run -s`, so it does not implement `ContextKeeper`: a resumed OpenCode session sees an edit. Observed on a stub provider with a throwaway HOME: `~/.config/opencode/AGENTS.md` and `~/.claude/CLAUDE.md` did not reach the request; the config dir's `AGENTS.md` did (P455 data).
- `ContextKeeper` (P452): optional role, `KeepsContextOnResume() bool`. Only Claude returns true: `--system-prompt-snapshot` is on by default, so a resumed conversation keeps its recorded prompt until it compacts. Feeds the editor's resume note.
- Add a provider: implement `harness.Provider` and any optional roles in `internal/harness/<id>`, add it to
  `providers.New`, and keep its CLI strings and error texts inside its package. No caller changes.
- Test: `flows/harnessflow` `TestOpenCodeProvider` and `TestOpenCodeHeadless` drive OpenCode against the
  `opencode` persona of `internal/flowtest/fakeagent` (`WithOpenCode()` puts it on PATH): list, per-launch
  pick, plugin events, take over, the cost-cap and tool refusals, the late id, `-s` resume and a lost
  conversation. `internal/harness/opencode/parse_test.go` covers the parsers and `permissionDoc`.
  `flows/harnessflow` `TestLeanProviderSeams` registers `fakeprovider.Lean` (id `fake-lean`: no
  structured, headless, hooks or usage; launches carry env; the `kira-fake-agent` persona assigns its own
  session id into `sessions.jsonl`, which `FindSession` reads) and drives env delivery, Continue in terminal
  token reuse, the late id and each missing-role path.
- Test: `flows/harnessflow` `TestSecondProviderRegistered` registers `internal/flowtest/fakeprovider`
  (id `fake`, models `fast`/`deep`, no hooks, no usage; interactive sessions run the `kira-fake-agent`
  persona) and drives distill, a smart script, an ADE headless step, an interactive session, resume after
  a switch, unknown-id fallback and provider-gone. UI half: `tests/ui/agent-provider.spec.ts`.

## ADE: Kira Space task planner (v2.0)

Kira Space's `ade` module plans tasks across repositories and runs Claude Code on them. It is a
rewrite of the P129 agent merge queue. v1 data is not
migrated (D5); history of the removal is migration `0011`. Phase detail: `docs/HISTORY.md (v2.0)` P143-P149
and `docs/HISTORY.md (v2.0)`.

#### Comments (P429)

- Bridge: `HostViewService.PostComment`, keyed by window key. The target comes from `HostViewTargets`, never from the frontend. `ChangeRequest` and `Issue` results carry `compose` (`maxBodyBytes`, `replyKinds`), absent when the provider has no write role.
- Text only on every host: no attachments, no drop zone, no picker, no paste handling. Kira never touches a CLI's credentials, and no driven CLI uploads a comment attachment (see CLI credentials: never touched).
- One post at a time per window (`HostViewService.beginPost`). A timeout answers `unavailable` with a "may have been recorded" hint, so the UI warns before a retry.
- Azure replies use `parentCommentId` 1.
- Frontend: `hostview/CommentComposer.vue` (Markdown source in a textarea, Edit and Preview tabs). TanStack mutation with optimistic pending comment, rollback on error, invalidate on settle. Draft in session storage per window, kind and thread. HostViewWindow cancels window-level dragover and drop so a stray drop never navigates the webview.
- Tests: `flows/githostflow/comments_test.go` (contract `host-view-comments`), `tests/ui/host-view-comments.spec.ts`.

### Clipboard capture (P430)

- Global hotkey (default `CmdOrCtrl+Shift+K`, user-set in Settings > Capture, on by default) opens a small popup showing the clipboard text with actions that fit it. Wails `App.GlobalShortcut` registers the chord; `App.Clipboard.Text` reads it. No new library. No Accessibility permission, no selection reading.
- `shell.ParseChord` validates a chord (needs CmdOrCtrl, Ctrl or Alt; key A-Z, 0-9, F1-F12, Space or punctuation) and `ConflictsWithMenu` rejects menu accelerators. `shell.HotkeyBinding` owns registration: `Apply(enabled, accel, cb)` swaps it live, `ReportError` turns a later OS refusal into `failed`. States: `off`, `active`, `requested` (Wayland: the portal decides the trigger), `failed` (with reason).
- Wails defers a pre-`Run` register; an OS rejection arrives at `Options.ErrorHandler`. main.go routes the `failed to register global shortcut` and `global shortcuts:` prefixes to `bridge.CaptureReportShortcutError`.
- Popup: `shell.OpenPopup` opens an ephemeral window, key prefix `capture-`, frameless, centered, always on top. macOS uses a non-activating floating panel. Rows delete on close and purge at startup (`IsCaptureKey`, `CaptureForget`, `purgeCaptureWindows`). A second press while open pushes `kira:capture:item` to the same window and focuses it.
- `bridge.CaptureService`: `WindowItem`, `Status`, `Dismiss`, `Confirm`. Classification order: change request link (`githost.Registry.ParseChangeRequestLink`), issue (key or link), text, empty. Caps: 64 KiB text, 2048 bytes for link detection, 500 bytes title. Actions: change request: open, Later; issue: open, Later; text: Later. A stale item id is refused.
- Later is `CaptureTask` then one `UpdateTask` (notes with the full text, `hostUrl` for a recognised link, `jira` for an issue). The two calls are not atomic: a failed patch leaves the task and shows an error in the popup.
- Settings: `capture.enabled` and `capture.hotkey` leaves (storage `settings` table, validated in `model.validateCaptureSection`). `Settings.OnChanged` calls `CaptureApply`; each state change pushes `kira:capture:status`.
- Frontend: `capture/CaptureWindow.vue` (radio actions, title input, Enter confirms, Esc or blur dismisses), `workbench/settings/CapturePane.vue` (chord recorder from `capture/chord.ts`, status line).
- Tests: `flows/captureflow` (fake clipboard and shortcut manager in `flowharness`; contract `capture`), `tests/ui/capture.spec.ts`, `tests/ui/settings-capture.spec.ts`.

### Issue tracker abstraction (P417)

- internal/issuetracker is the neutral contract, shaped like githost. A Provider offers ID, Terms (label, CLI name, issue example), Setup, ParseRef (bare key or link), Status and Issue. Registry holds providers; providers.New registers Jira first. internal/clisetup holds the Setup and Step shape shared with githost.
- The Jira provider (issuetracker/jira) shells out to `acli` through toolexec: `--version`, `jira auth status` (cached 5 min ok, 30 s not ok) and `jira workitem view KEY --json --fields ...`. Credentials stay in acli; Kira stores none. Candidate paths: PATH, /opt/homebrew/bin, /usr/local/bin.
- acli returns title, status and the Development field summary (pull request count and state), not the pull request rows. Rows come from the code host: `ade.KeyChangeRequests` searches each host (`ChangeRequestSearch`, title match) and the persisted change request index (`head_ref` contains the key), then keeps whole-token matches. A gap shows Jira's count against rows found.
- The first read per site fetches all fields except comment, attachment, worklog and description to find the Development field id; later reads ask for `summary,status,<id>`. An oversize first read falls back to `*navigable` and marks the site without a Development field for 10 min.
- Results cache 60 s (15 s when partial or failed), coalesced per key by singleflight on a detached 20 s context, so a caller that gives up leaves the result for the next call. A ref on another site than the signed-in one returns ErrOtherSite.
- Wire: `IssueTrackerService.TaskIssue` (title, status, status category, summary, rows, reason), `Trackers`, `Tools` (status and setup of every host and tracker CLI, `refresh` skips the status caches). A tracker failure comes back as a `reason` (unavailable, unauthenticated, notFound, otherSite, timeout, failed, noTracker; `changeRequests<Reason>` for a failed row lookup), never as an error.
- `IssueDetails` (P428) is an optional role beside `Provider`: `IssueDetail(ctx, ref)` returns title, fields, people, labels, components, dates, parent, description, comments and the PR rows. Jira reads it with `workitem view --fields jira.DetailFields`. When acli output passes the 1 MiB cap, it re-reads without `comment` and takes the newest 50 from `workitem comment list --key K --json --limit 50 --order -created`; if that fails too, `commentsOmitted` is set. ADF becomes Markdown in Go (`jira/adf.go`), hand-rolled: no maintained Go ADF converter (checked ajbeck/adf-to-markdown, rgonek/jira-adf-converter, seflue/adf-converter) escapes text and filters link schemes, and the output must be safe for the sanitising renderer.
- `IssueComments` (P429) is an optional write role: PostIssueComment (ref, body). Text only, no reply kinds, no attachments. Jira turns Markdown into ADF in Go (`jira/mdadf.go`, goldmark with GFM) and posts `acli jira workitem comment create --key K --body-file F` from a 0600 temp file, so the body never reaches argv or the op log. Body limit 32 KiB, write timeout 30 s.
- Frontend: Settings section CLI tools (`CliToolsPane.vue`, install and sign-in commands with copy), `AdeTaskIssueLine.vue` in the Task tab, tracker label and gap text in `AdeArtifactList.vue` (`artifacts/gapText.ts`).
- Tests: issuetracker/jira unit tests over hand-written acli fixtures, flows/adeflow `TestTaskIssueTracker` (contract `ade-issue-tracker`), flows/appflow `TestCliTools` (contract `space-cli-tools`), tests/ui/ade-v2-issue-tracker.spec.ts, tests/ui/settings-cli-tools.spec.ts. flowharness/fakeacli stands in for acli; the default locator finds none.

### Model and scope

- In Kira Space, a task owns branches across repos.
- A branch in Kira Space is one of three kinds: mine (created here, has a name), review (another author's), or parked (kept out of the merge order; shown as Not merging).
- A review task converts one way to a work item (task menu, UpdateTask kind task): task owner cleared, its review branches become mine and get a worktree plus prepare script. Clicking a review card opens the task panel; its branch panel opens from the task's branch list (P377).
- A task follows a workflow, a flat list of steps of kind agent (claude -p), script (a shell command), review or manual (your own work, optionally an interactive session).
- A workflow step runs once (on the first mine branch in task order), each repo, or only <repo>, with one run per step and branch.
- Task status is stored (P379, P411): table ade_task_statuses (id slug, name, color, position), 1-20 rows, names unique ignoring case. A status is a label: Kira reads no meaning into name, colour or position. Tasks hold status_id, status_by (auto|user), status_step; no SQL FK, so a removed id is reassigned in the same transaction as the set write. The editor lives on the Workflows page (Task statuses row), not Settings.
- Four default slots in ade_task_status_defaults (slot, status_id) name the status of each automatic change; an absent row keeps the current status. `new`: task created, review task turned into a task, task back at its first step with nothing run (required). `step`: step starts with no status of its own. `review`: review step starts with none; a new review task starts in `review` when set, else `new`. `finished`: workflow finishes. A step `status:` empty uses the slot of its kind; an id missing from the set does the same.
- Editing a status or a slot never touches an existing task: tasks keep their status_id, and a slot applies at the next automatic move only. Delete requires a reassign target for every removed id (live and archived tasks); a slot naming a removed status moves to the same target. Workflow files are not rewritten.
- Status sync is last event wins. The engine re-applies a step's status only when the workflow position key (current step id, `done`, or empty when not started) differs from status_step. A manual move (UpdateTask statusId) sets status_by user and leaves status_step, so it holds until the next step change.
- Blocked is not a status: a derived overlay (stuck or failed run, failed setup) shown as a red chip beside the status.
- Workflow step `status:` holds a status id or is absent (the slot of its kind). The parser checks id syntax only and maps legacy names (To do, In progress, In review, Done) to the seed ids. The writer writes `status:` only when set.
- Tasks sit on one of three shelves (P405). Plan: dated, or review items. Later: live non-review task with no plan day and backlog 0. Backlog: plan row with backlog 1, day always NULL. ShelveTask moves a task onto Later or Backlog and drops its day; SetPlan with a day returns it to the Plan. Review items never shelve. CaptureTask adds a title-only task at the top of Later (quick add, phone add).
- The Later tab shows Later and Backlog as two drag lists sharing one Sortable group. Plan band LATER (key 9999) is labelled Further out: dated work past the 7-day window only, shown when it has entries, refuses drops. Undated tasks never enter the Plan sequence.
- The Plan uses flat token surfaces only: no textures, shadows, dashed borders or tint washes. State shows as rail, border, outline or chip colour. Each branch row has one `surface` tooltip (`AdeBranchTip`) in the plan colour coding.
- CreateTask takes an optional plan day (nil lands in Later). The UI sends today, or the next work day, and no repos; repos arrive via AddTaskRepo, AddExistingBranch or agent declare_repos.
- A task holds one row per repo; `name = ""` means branch not created. SetTaskBranch, UpdateTaskRepo, RemoveTaskBranch, RemoveTaskRepo edit rows independently, refused while a session, launch or background run uses the row, and on review tasks. Detach hard-deletes a never-created, run-less row, else archives it and hides its runs from the task wire. Dependents fall back to the old branch name as base. Git refs never change; detach removes the task link and ADE linked worktree only.
- A review item with no plan day sits above the first task that builds on or conflicts with it, else in Further out (the tail band). Moving it to a day pins that day; clearing its day returns it to that placement (P368).
- A repoless task refuses Start (no branch to run on) until a repo is added. This is designed behavior.
- An estimate is free text h or d, settable, changeable and clearable at any time; only the cap (480h, 60d) applies.
- The Branch wire carries no files or commits list or commit count; ahead, behind and dirty remain. Board facts still use the range file set for shared-file pairs.

### Process wiring

- In internal/appwire/wire.go, wireAdeTask builds ade.TaskBoard from TaskBoardDeps: repos, discovery GitStatus (no GitPath), its own gitsession.Conn named ade-board, adeflow.Reader over <home>/workflows, the agentmcp finish_step server, the Tracker, log sink and session store.
- wireAdeTask calls Recover() then Start() on the task board, and installs board.OnTUIStopped as the tracker's stopped handler.
- bridge.AdeTaskService binds 69 methods, each with one adeTask* entry in frontend/src/bridge/index.ts.
- AdeTaskService push channels are defined in bridge/adewire/channels.go with prefix kira:adetask: and suffixes board, workflows, repos, runs, log, sessions and open-session; open-session is emitted to one window via EmitTo.
- bridge/adewire is the frozen wire contract; tests/fixtures/ade-v2/ holds 27 fixtures decoded by adewire/wire_test.go and the mock runtime.
- layeringtest.RunAllowing admits the adewire package.

### Storage

- Migration 0011 drops the six v1 ADE tables and rebuilds ade_sessions v2-only.
- Migration 0038 (P293) resets ADE task, run and session data for the flat model with no backward compatibility; tasks carry entry_step_id and since_json.
- Migration 0014 adds ade_runs.launch_note, launch_resume_id, launch_prompt and launch_extra, which hold a held run's send-back spec and are cleared at launch.
- Migration 0036 rebuilds ade_runs (states add review, purposes add review) and adds ade_reviews (P286).
- Space migrations 0020 (scripts) and 0024 (code_repos.color, P222) are not ADE migrations.
- code_repos.color is a PaletteColor name and is never empty; migration 0024 backfills it by sort_order % 6 over blue, amber, magenta, green, red, cyan, and Create assigns the next hue via internal/palette.AutoRepoColor.
- Repo colour is written by CodeWorkspaceService.SetRepoColor and read through state/coderepos.ts (colorOf, repoTint).
- Repo colour is painted via --kira-conn-* as colorMarkClass row rails on the Repositories dialog and Git panel, repo tab bars, and ADE repo tags; the 20-slot ADE work palette now colours tasks only.
- internal/palette is the one Go palette set, though Studio's model.ValidPaletteColor still has its own.
- Worktrees take their anchor repo's colour (repo/state/repoLinks.ts repoColorOf, used by tabKinds.ts and ADE; P227).
- A deployment environment (ade_repo_envs) is a name plus a shell command that prints the deployed commit SHA; it runs in the repo root on board refresh and marks branches ▲name ✓/⚠/?.
- In the deployment environments UI, a new row stays a local draft and is written only once both name and command are filled.
- Run and setup logs keep a tail of 2 MiB per log in chunks of at most 8 KiB, set truncated once head chunks drop, and batch writes every 250 ms.
- Archive keeps all task data (P399): logs, review sessions and comments are never purged; per-log size cap still bounds each log. Archive closes review windows only; review sessions stay pinned.
- Migrations 0053 (Studio) and 0063 (Space), P452: generic `config_last_valid(folder, file, value_json, recorded_at)`, the `configfs.TableProjection` table.
- Migration 0052 (P399) adds cr_* columns to ade_task_branches. Archive snapshots each branch's change request there (best effort, 5 s) so the archived view shows it when the host is unreachable.
- AdeTaskService.ArchivedTask returns an archived task read-only: task, statuses, branches (live PR when the host answers, else the snapshot; prSource says which), stored review comments, no worktree paths. Phone reads it at /api/ade/archived-task.
- AdeTaskService.RestoreTask revives the task and the branch rows archived with it (same archived_at), adds a Later plan row, and returns the task. It refuses when a branch name is live on another task; nothing changes then. Worktrees come back on next launch via ensureWorktree.
- No Archived tab. Archived tasks list in the Plan: today's archive rows always show, past days load with the History toggle (`AdeHistoryControl`), and the archived finder (`AdeArchivedFinder`) searches all of board.history. Each selects the task; the panel is read-only, with Restore to edit.
- The per-repo prepare timeout is the GitRepoSettings leaf worktreePrepareTimeout (default 15m, max 2h), shared with git-ui worktree add; Spec.Timeout must be > 0.

### Git facts

- FileChange carries only path, status kind, rename original path and similarity, all from one --name-status spawn (porcelain.ParseNameStatusRecords); file lists have no line counts and no binary flag (P257).
- --numstat survives only in stash list and the global stash log, as record framing for FileCount; the FileTree directory row keeps N files.
- Board facts per repo (branch inventory, ahead/behind against base, files, dirty state, caches keyed by tip pairs) run on the ade-board.
- Board refresh fetches the default remote and then derives facts again through the same path; without a remote it rescans locally and the chip reads no remote. Autofetch interval is a setting.
- The board opens repos with GitStatus().Path (discovery), never the raw git.gitPath setting (empty by default); non-ok git fails the open with: ade: git is unavailable
- lastFetchAt is the mtime of the worktree git dir's FETCH_HEAD.
- Conflict pairs and the rebase-conflict check use git merge-tree --write-tree, which needs git >= 2.38 (enforced by gitclient.Discovery; older git reports failed). It touches no worktree, index, HEAD or ref, so it is safe beside running agents.
- Conflict pairs are computed only for a mine branch against a mine or review one, neither parked nor merged, neither an ancestor of the other, with intersecting changed-file sets.
- go-git was declined for conflict detection (D1) because v5 Merge is fast-forward only and cannot answer whether A conflicts with B; go-git v5 is in go.mod since P211 only for the memory importer's plumbing/format/gitignore matcher.
- Open repos follow a changed git.gitPath (P231) via gitclient.Repo.SetGitPath; the gitsession registry and TaskBoard.openRepo re-apply the live setting.
- Integration (merged/stale/not merged) and deploy (deployed/stale/not deployed) facts use git merge-base --is-ancestor, git cherry and patch-id --stable, only through gitclient.
- Marks in ade_branch_marks and ade_env_state remember was merged, rebased since and environment moved back; RecordMerge is the only writer of recorded = 1, and a mark keeps the branch tip at that moment.
- Deploy scripts run through gitprepare plumbing in the repo root with a fixed 60s timeout, at startup, on env change and on each repo refresh; failure reports unknown.
- The stale note counts commits, reading 1 commit ... is missing or N commits ... are missing.
- ForcePush runs --force-with-lease --force-if-includes.
- Git credential prompts route through the git module's askpass broker to the Space credential relay (kira:git:credential), answered in the shared dialog.
- Archive is ArchiveRisk (dirty and unmerged work per worktree) then ArchiveTask, which stops the task's runs and prepare scripts, closes its terminals, removes worktrees and moves the task to history.
- The engine's Archive has no discard flag; "Delete anyway" is the dialog's own choice.

### Workflows

- Workflows are YAML files in <home>/workflows, parsed by adeflow.Reader; no workflow is seeded (D2).
- The workflows folder is a `configfs.Folder` (P450, see Config files): watched, debounced 200 ms; a broken edit keeps the last valid version and reports the line plus a message. Saves are atomic with a baseHash check. `ade_workflow_last_valid` rows follow file deletion. The writer keeps key order and comments.
- `apps/kira-space/internal/adeflow/options.go` is the single list of workflow keys (P451): `Parse` and the writer derive their key sets from it, and the folder's `AGENTS.md` is generated from it. A new key needs a row there or the parser refuses it.
- A workflow has a flat list `steps` of kind agent|script|review|manual (P293); `kind` is required and each kind allows only its own keys.
- Allowed keys: agent: runs_on before results max_returns timeout prompt allowed_tools smart_script params sandbox settings; script: command runs_on before results max_returns timeout; review: review runs_on before results max_returns timeout sandbox settings; manual: session and, with it, prompt and settings.
- A top-level `stages` key fails with the error "stages" is no longer supported: list every step under "steps".
- Agent `allowed_tools` feeds the headless `--allowedTools` list.
- Sample workflows in apps/kira-space/internal/adeflow/testdata/ import and validate in the app.
- Step results (P247): an agent step may declare 1 to 12 `results`; a script step has fixed `done` and `failed` results with only `next` editable; a manual step has none.
- Each result has `id`, required `ok`, `description` (one line, at most 200) and `next` (next|end|stop|<step id>, default next for ok and stop for not ok); `needs_input` is reserved; at least one result must route forward.
- Wire `StepResult.loop` (route targets the step itself or an earlier one) replaces the old per-result `max`; the limit is `PipelineStep.MaxReturns` (P288). `on_failure` and `retry` are gone.
- The parser always materializes `Results`; the writer writes `results` plus `max_returns` only when they differ from the defaults (adewire/results.go: ImplicitResults, DefaultLimit).
- Return limit (P288) is `max_returns` per step: agent default 3, range 1..10; review default 0 (no limit), range 0..10. A return is a result routed to the step itself or an earlier one.
- The return counter is ade_runs rows in state `back` with purpose '' per (task, step, branch); no migration, never reset, and it survives restart and Resume (countReturns, Run.Returns).
- `decideLoop` stops at the return limit with catalog note backend.space.ade.returnLimit.body.
- The Workflows page graph editor (P247) edits a draft as a Vue Flow canvas (@vue-flow/core, MIT) with @dagrejs/dagre auto-layout on every edit; no stored positions and nodes are not draggable.
- board/workflowGraph.ts is the pure graph model: edges grouped by target, green for all ok, red for all not ok, neutral for mixed; loop edges dashed.
- `setRoute` turns an edge into a loop when the target already leads to the source, else moves the target after it; `normalizeOrder` keeps the start first and forward routes pointing later.
- The step menu offers Move up/down, which is off when the swap would turn an explicit forward id route into a return (`canMove`); Remove is off for the last step.
- A manual step draws one implicit `done` edge to the next step.
- A kind change in the inspector that would drop a prompt, command or session prompt asks for confirmation first (`hasContent`, `withKind`).
- The editor mode is graph | yaml; the graph editor loads asynchronously so the board bundle stays free of Vue Flow.
- Graph layout is locked (P255): top to bottom only, every node 256x104, steps and end node on one spine column, results branches fan to the side, loops dashed in a left gutter entering the target from above, native vertical scroll, no zoom or pan.
- Workflow edits save explicitly (P196): Save and Discard, Cmd/Ctrl+S, no autosave; leaving with unsaved edits asks through ConfirmDialog. Validation stays debounced.
- Stage settings (P431): a step's `settings:` block is keyed by agent provider id, then setting key (`settings: {claude: {auto_compact_window: 200000}}`). Allowed on agent, review and manual-with-session steps. The parser checks provider ids and keys against `providers.StageCatalog()`, so an unknown id or key fails with its line. Absent means the provider's default. The writer adds the block only when a value is set and drops it when cleared, so a file without it stays byte-identical. Wire `WorkflowStep.settings` is always a map; `WorkflowsResult.stageSettings` lists the catalog for the editor, whose step inspector (Advanced) renders one field per entry.
- A review step (P286) has `review: {send_to: implementer|new, implementer}`, takes no prompt, tools or smart script, and `timeout` stays required.
- On reaching a review step, startRun calls enterReview (ade/reviewstep.go): the run goes to state `review` and the step waits. Wire Run.review carries phase waiting|sent|working|stuck|done, round and note, derived from the latest round.
- A review send is a round: its own ade_runs row (purpose='review', spec_json = model.AdeReviewSpec) run through superviseAgent with the reply_review_comment tool and implicit results.
- Review rounds never count as step runs: LatestRuns partitions by purpose, plan() skips Purpose != '', and toWireTask hides rounds behind the review run.
- Review send state is held in table ade_reviews(run_id, route, target_run_id, dedicated_claude_id, rounds).
- Review route `implementer` resumes the Claude session of the chosen candidate (default the nearest earlier agent run); route `new` launches one dedicated session reused by later rounds.
- If the review target is lost (EndResumeLost), the round fails with outcome.lost and queueFallback starts a round on `new` with the note.
- RecoverRunning keeps launch_resume_id for review rounds, so a quit leaves the round `stuck` and resumable; ResumeRun on a round resumes it.
- FinishReview accepts a result of the step only while no round is live, ends the review run and routes like any step result; the note travels as the next step's send-back note.
- Task snapshot (P196, ade/taskwf.go): a task follows the live workflow file until its first run, session or move; then snapshotWorkflow (idempotent, first wins) stores the workflow in ade_tasks.workflow_json with a sha256 workflow_hash.
- StepDone, SetTaskStep and refreshSnapshot read the task snapshot. Wire Task.workflow is null until started; Task.workflowOutdated is a drift flag shown as an "updated, applies to new work only" badge.
- SetTaskWorkflow on a started task with no live run still resets the task and now clears the snapshot; Recover backfills a started task lacking a snapshot from the current live file.
- Task position (P293): Task.entryStepId is the step last entered (`done` when finished, empty without a workflow); Task.since maps "<stepId>/<branchId>" to an attempt, and runs at or below it are history, so moving back reruns a step without deleting runs.
- The current step is derived by walkPath from the entry: skip steps are passed over, a done step moves to its forwardTarget, and the first step not done (a manual step never is) ends the path. Running past the last step, or an `end` route, finishes the task.
- `skip: true` steps (adewire.WorkflowStep.Skip) are passed over by moves and routes; a workflow needs at least one unskipped step.
- SetTaskStep(taskId, stepId) moves to any step or `done` and is refused while a run of the task is live; StepDone finishes a manual step. The stale guard FromStepID compares with the derived current step.
- Agent tools are always on (P408). A legacy `kira_space_mcp` key is accepted and ignored, and dropped on save.

### Run engine

- Headless runs execute via a login shell running claude -p with --output-format stream-json, --verbose, --session-id, --mcp-config and --setting-sources, cwd the worktree, prompt on stdin, using Setsid and procgroup.GracefulCancel; no --permission-mode and no --strict-mcp-config.
- App setting ade.headlessSettingSources is either all (the default, meaning user,project,local) or user.
- The finish_step MCP server (internal/agentmcp) uses one loopback listener with a bearer token per run (tokenauth); the last call wins and applies at process exit. Its summary is the run's handover (P407). Its reply lists the task's artifacts (P409).
- The per-run MCP config is <home>/ade/runs/<runId>.mcp.json with mode 0600, deleted at process exit; the token is never put on argv.
- A grant with declared results (P247) gets its own per-run mcp.Server whose status enum is the result ids plus needs_input; implicit grants keep the shared servers and done|failed|needs_input.
- suffix.go holds the text appended to the message; FinishStepSuffixFor(results) names the result ids, and the TS mirror is finishStepSuffix.
- A run that ends without calling finish_step fails with 'ended without finish_step'; a timeout is failed; script steps fail by exit code.
- A step starts auto or waits for approval; every automatic step has results, and the implicit pair is done (ok, next) and failed (not ok, stop).
- decideRoute maps the reported result to a route: next, a later step (steps between are skipped, a view only), end, stop, or a loop to the step itself or an earlier one.
- A forward or end route marks the run done (a not-ok result keeps outcome status failed); stop or a spent loop marks it failed; a loop marks it back.
- Loop counts are cumulative per task, step and branch, measured against the step's max_returns.
- A self loop starts a fresh attempt; an earlier-step loop queues a fix run of the target with claude -p --resume <session id> (same Claude session); loops is the round epoch.
- A fix run's resume spec is kept on its run row until launch, so a run held behind a setup resumes the same Claude session after a restart.
- If no report arrives (crash, timeout), the step takes its failed result, else the run stops.
- walkPath follows the routes; nextAction and chainRerun use that path; the run outcome stores result and route.
- A once step runs in the first mine branch's worktree (D12), and there is no concurrency cap (D8).
- Restart recovery (D7): Recover() runs before Start(); running runs become stuck with note 'interrupted by restart', running setups fail, task sessions stop, and stale *.mcp.json files are removed.
- After restart, pending runs keep their launch spec and launch when their gate opens; nothing auto-resumes, and a restart-stuck agent run offers Resume.
- Agent tools (P199) are implemented in ade/agenttools.go as agentmcp.SpaceTools on *ade.TaskBoard; the loopback MCP server issues scoped grants (run, task, Space) with one bearer token per launch.
- Every task grant except rebase runs and review rounds carries the Space tools: task_info, declare_repos, request_branches, branch_status. Releasing a grant (run end, session stop, app close) cancels its in-flight calls.
- declare_repos accepts registered repos only and is additive.
- request_branches takes `repos`, 1 to 20, and waits for the user (see Branch requests). Kira Space names a new branch kira/<10-char nanoid> (claimBranchName generated mode, up to 5 tries on a taken name), never an agent-chosen name; a named row with a worktree returns `existing`. Worktree and prepare script go through ensureWorktree/startSetup. Outcomes per repo: created, existing, denied, timeout, failed.
- Run on an agent step leaves an unnamed branch to the agent: the run starts at the repo root, and its sandbox may write the per-repo worktree folders. A script or review step derives `feat/<slug>` from the task title.
- Agent tools act only on the token's task; repo refs resolve by nickname, then name, then id; errors return as actionable ToolError text.
- Branches and repos added by an agent carry origin = 'agent' and show an agent chip.
- Git refresh (P191): the UI sends the explicit repo ids it shows; refreshRepo runs env scripts beside the fetch, not before it.
- A repo with no remote gets a local rescan (P230) instead of a fetch error row.

### Resume after restart and quit confirmation (P279)

- After a SIGKILL mid-turn, claude --resume <id> works: the transcript ends on an unanswered tool_use, and the CLI appends a synthetic error tool_result ("outcome is unknown, check whether it took effect") and continues in the same session id.
- When a session is resumed after a kill, the turn in flight is lost and its tool effects may be half-applied.
- Resuming an unknown session id makes the Claude CLI exit 1 with No conversation found with session ID: <id>.
- The Claude CLI treats --resume together with --session-id as an error, so a resumed launch drops --session-id.
- AdeTaskRepo.RecoverRunning writes launch_resume_id from ade_sessions.agent_session_id in the same UPDATE that marks the run stuck, only for agent and smart-step runs (purpose = '' with a session).
- Rebase and script-step runs, user stops and needs_input runs get no launch_resume_id; adewire.Run.resumable is stuck plus that id.
- TaskBoard.ResumeRun clears the old spec first, so a second click refuses, then queues attempt+1 with Launch{ResumeID, Prompt: RunResumeLine, Note: "resumed after restart"}.
- On resume, startRun reuses the agent session id and composeResumePrompt appends the finish instruction; RunResumeLine (run.resume.line) is one shared prompt for ADE and smart scripts.
- When the run's provider reports ResumeLost for the last stderr line (Claude: No conversation found), the outcome becomes runoutcome.EndResumeLost (failed, source start) with no automatic fresh run; Retry or Run again starts anew.
- Smart scripts in both apps store script_runs.resume_id and resumed_from (Studio migration 0038, Space 0035).
- Repo.FailRunning and finish set resume_id = session_id for a smart run ended with source restart (crash and graceful quit).
- Smart script resume: Service.Resume re-plans from the current script, TakeResume compare-and-swaps the id, and the new run row carries resumedFrom and the same session id.
- Smart script resume refuses (resumeMissing) when a param is secret or was added since, because secret values are never stored.
- The quit gate is set via shell.Quitter.SetGate(QuitGate): with live work ShouldQuit returns false and asks once; ConfirmQuit runs the normal flush and teardown, and CancelQuit withdraws.
- The shared prompts.QuitGate raises prompt kind quit through the router, reopening a window when none is open.
- Live work for the quit gate is running smart and headless script runs plus agent PTYs, plus running ADE runs in Space; plain shell tabs do not count.
- LifecycleService gains ConfirmQuit, CancelQuit and QuitStatus; QuitPrompt.vue renders ConfirmDialogView titled "Quit {app}?".
- Closing the last window and OnShutdown never ask for quit confirmation.
- Not solved: there is no detached runner and no PTY survival across restart.

### Task base and rebase runs (P241)

- base_pending_from on ade_task_branches is set when a base change is stored and cleared only after a rebase is verified in git.
- A task branch base ref that no longer resolves is marked baseMissing, and Review code refuses it.
- Base ref resolution tries the remote first (origin/<ref>) and is validated by git check-ref-format; base cycles are refused.
- Rebase, Queue after and Change base are each one ade_runs row with purpose='rebase', run headless through the run engine.
- RebasePreview composes the prompt, blockers and stack server-side; Rebase sends it (empty message means default) and returns {runId, noOp}.
- One mutex per task covers rebase start, abort and recovery.
- Change base stores the base first and never reverts it on failure; retrying is done with Rebase.
- Stacked branches restack in order; a child rebases with --onto <parent> <parent's old tip>.
- decideRebaseOutcome checks git after the run (each branch on its base, no rebase in progress); an agent 'done' that git disagrees with becomes 'failed' with source 'verify'.
- A rebase run that ends without a report gets the reason 'no report: <why>' and facts taken from git.
- model.AdeRunOutcome is runoutcome.Outcome plus an optional Report (conflicted files, last git error, what was tried) and Rebase facts; finish_step takes reason, conflictedFiles, lastGitError, tried.
- The rebase run timeout is Deps.RebaseTimeout.
- The rebase agent has no Bash and loads no settings file (Isolated); git runs only through the git tool on the kira-ade MCP server.
- ADE git runs through gitsession and the shared runner, including `check-ref-format` (`CheckBranchName`) and the agent `git` tool (`agentmcp.Options.Git`, `TaskBoard.RunGit`); there is no second `exec git` path. Output no longer interleaves stdout and stderr.
- The git tool requires worktree to be a stack worktree and args to match an exact shape in agentmcp.CheckGitArgs; -c, -C, --output, --exec and --upload-pack are rejected, and unlisted options fail, abbreviations included.
- Stack worktrees are passed as --add-dir so child branches can restack and edit.
- AbortRebase runs git rebase --abort in the worktree; BranchInventory keeps the worktree path of a mid-rebase branch (detached HEAD, read from rebase-merge/head-name) so abort and rebaseInProgress work.
- MCP run_outcome (Space grant, same task only) returns the latest ended runs with outcome so another agent can read why a rebase failed; kind automation returns the task's ended script runs.
- The P238 notification titles a rebase run 'Rebase <verdict> · <task>'.
- board/rebaseActions.ts decides every rebase action; tag action, panel header, fix menu, task menu and card button share it, and adeDialogs.act runs one.
- There is no automatic rebase and no popup when a base moves; merge, archive, session and start dialogs still deliver to a TUI session.

### Interactive sessions

- A single ade.Tracker instance is shared by terminal.BoundService.ComposeAgent (Compose), terminal.Registry.OnChange (Reconcile), harness.Hooks OnEvent (HandleEvent) and the board (Send, Get).
- The agent session id is learned, not always chosen: a provider that is a `harness.SessionLocator` may launch with `Launch.SessionID == ""`. The row then stores `''` (only headless rows require an id). A `SessionStart` event with a valid id (`harness.ValidSessionID`) fills it. At stop (`Stop`, `Reconcile`) and at resume (`resumeTarget`, `takeOverLocked`) the tracker asks `FindSession{Cwd, Since: StartedAt}`; still unknown at resume, Take over and Resume refuse with `backend.space.ade.sessionIdUnknown.body`.
- Tracker.Prepare records a pending launch intent with a 2 min TTL; the renderer opens the terminal through the ordinary TerminalService.Open; ComposeAgent then consumes the intent, inserts the ade_sessions row as running and appends hooks env, --settings and the initial prompt.
- Reconcile stops a session record only when its terminal is gone and a 30s grace window has passed.
- The user message follows ' -- ' in argv so it cannot parse as a flag.
- A resume runs in the recorded cwd; a missing cwd yields ErrInvalidInput and the directory is not recreated.
- StartBranch refuses a second session on a branch with the error 'a session is already open on ...'.
- StartAdHoc (P410) opens an agent TUI on a branch worktree with no first message. It shares StartBranch's rules and grant (handover, Space tools); the row is a plain branch session.
- A taken-over run can still call finish_step from the TUI (R14). Take over confirms when the run is live and passes stopIfRunning.
- Sessions are window-scoped like every PTY: closing the window stops the session record, which stays resumable.
- FocusSession raises the window that holds a session, via kira:adetask:openSession.
- CloseSession (P436) ends a running session and keeps its history. Interactive: Tracker.Stop then CloseTerminal, so the row is stopped at once and resumable by Take over. Headless: StopRun.
- RestartSession of a headless step run stops it and queues the next attempt resuming the same conversation (note "restarted by you"). Rebase and review runs refuse; their panels own retry.
- The session tab menu offers Restart and Close without a confirm.
- The Sessions dropdown (P437) lists every session in scope, Running then Finished or stopped. Enter or click selects a running tab or takes over a stopped session. The filter shows from 6 rows. The row menu comes from useSessionMenu: Take over or Resume, log toggle, Close and Restart for a running session, Copy session ID. The strip scrolls the selected tab into view.
- The agent-hooks Stop event ends a dialog turn; the renderer's turnWatch then calls RecordMerge (merge dialog) or ArchiveTask (send-then-archive), and that pending state lives in the renderer.

### Agent sandbox (P287)

- The agent sandbox lives in package internal/agentsandbox, shared by apps/kira-space/internal/ade and internal/scriptruns.
- ResolvePolicy expands ~/, canonicalizes paths with EvalSymlinks because Seatbelt matches real paths, and probes each worktree's git layout (ProbeGit: linked or main).
- Profile renders SBPL text with golden files in testdata/*.sb (regenerate with -update); paths enter the profile as escaped string literals, never regex.
- `Input.ContextDir` (P452) adds a `(subpath ...)` read allow for the agent context snapshot dir. Never an `--add-dir`.
- The config-file read allow (P453) is `<ConfigDir>/<run>-<n>.mcp.json` or `<run>-app.<role>.json`: the grant's MCP file and the app settings files of a headless run.
- The engine side of the sandbox is in ade.sandbox.go (prepareSandbox, watchSandbox, sandboxOutcome).
- The sandbox profile starts with (allow default) then denies and re-allows; the last matching rule wins.
- Profile reads are restricted through file-read-data, never file-read-metadata, because realpath of ancestors must work.
- The profile denies file writes outside the policy, keychain writes, AppleEvents, LaunchServices, the SSH agent socket, signals to other processes, and ~/.claude config and extension writes.
- Git rules: the worktree is read-write except its .git file; the linked gitdir is read-write except gitdir, commondir, locked, config.worktree; common objects are append-only; the run's own branch ref, lock and reflog are writable.
- The rest of the git common dir is read-only, so branch, worktree, tag, stash, remote-ref, config and hook writes fail with EPERM; allow entries never open these because the protections come last.
- Sandboxed agents always read the macOS keychain; the CLI signs in from it. Keychain writes stay denied (P341).
- A step without a sandbox block defaults to access write with network on.
- The sandbox covers headless agents from superviseAgent (steps, review rounds, rebase runs with every stack worktree writable) and scriptruns smart runs in dir mode worktree.
- The sandbox does not cover TUI sessions and Take over, script steps, setup scripts, folder-mode scripts, or memory claude calls.
- The Enforcer interface has Support, Prepare and Watch; System() is real only on darwin, elsewhere Support is Enforced:false with Reason NotEnforcedKey and Prepare still builds profile, flags and env but no wrapper.
- sandboxtest.Fake stands in for flow tests; the flow harness defaults to a not-enforced fake so contracts do not depend on the host OS.
- With agentmcp.Spec.Sandbox, Script emits exec /usr/bin/env -u … K=V /usr/bin/sandbox-exec -f <profile> claude … after the login shell sources the user profile.
- The profile is checked with sandbox-exec -f <profile> /usr/bin/true first; a failure fails the run before claude starts.
- The sandbox env redirects build caches to <AgentDir>/sandbox-cache/ and sets TMPDIR to the run scratch.
- The second sandbox layer uses --add-dir and --disallowedTools (Bash(git branch:*), Bash(git worktree:*), edit tools for read); it is the only layer on Linux, where the notice says it does not stop shell commands.
- With network off, Seatbelt allows loopback only; HTTPS_PROXY points at an in-app goproxy CONNECT proxy that passes the Claude API hosts, chaining an upstream proxy from the environment.
- One shared /usr/bin/log stream child runs while a sandboxed run is live; events map to a run by pid ancestry (Handler.OnStart gives the root pid).
- Tracker keeps the first 50 distinct denials and counts all of them.
- A failed run with denials gets reason 'Sandbox blocked <op> <path>.'; denial and notice lines go to the run log on stdout.
- Sandbox data is stored in table ade_run_sandbox (migration 0037): access, enforced, resolved policy JSON, profile, denials; AdeTaskService.Run.sandbox carries the badge and RunSandbox returns the detail.
- fetch_remote and push_branch run in the app, unsandboxed (TaskBoard implements agentmcp.SandboxGit via Grant.Sandbox).
- push_branch pushes the run's branch to the same-named remote branch (RemoteOpParams.RemoteBranch), refuses protected branches, and is refused for read steps.
- request_branches remains the way to create a branch and its worktree.

### Agent launch check and folder-scan hiding (P277)

- The provider's Locate finds its CLI before any launch (Claude: PATH, then well-known install paths, then the user's login shell, because a GUI app's PATH can differ; internal/harness/claude/locate.go).
- Tests replace the system well-known claude install paths with claude.SetSystemPaths.
- When Locate finds no CLI it returns a harness.ErrNotFound error that lists every probed place.
- ade.Tracker.Prepare, the terminal agent-launch path and the headless runners call Locate, so a missing CLI shows a Kira error instead of the shell's text.
- code_repos.hidden_rev (migration 0034) counts writes of the hidden flag.
- A folder scan applies its stored hidden flag (ApplyFolderHidden) only to repos whose hidden_rev still equals the one stored at import, in one transaction, so a repo unhidden or toggled meanwhile is never re-hidden.

### Agent notifications (P238)

Desktop notification when a Kira Space agent finishes or needs input. Package `internal/agentnotify`,
built in `appwire.Build` and fed by two existing callbacks: the hooks `OnEvent` (every Kira-launched
agent PTY) and the board's `OnRuns` (headless runs, which have no hooks).

| Source | Note |
|---|---|
| `Stop`, no wake tool armed | `finished`: "Claude finished · name", body is `last_assistant_message` |
| `Stop` after `PreToolUse` of `Monitor` or `ScheduleWakeup` | none (session waits); `UserPromptSubmit`, `SessionStart`, `Stop` clear the arm |
| `Notification` `permission_prompt`, `elicitation_dialog`, or untyped with a message | `needs-input` |
| `Notification` `idle_prompt`, `auth_success` | none |
| run state `pending` or `running` to `done`, `failed`, `stuck` | `run-ended`; a run first seen already ended records state only |

- An agent notification is named by the ADE task title, else the code repository whose root holds the cwd, else the cwd basename.
- Terminal tabs are not persisted, so no tab title exists to use as the notification name.
- Finished notes skip while any app window (review windows included) is the key window (WindowRegistry.AnyFocused, native, checked at decision time; P359).
- Other kinds are suppressed while the owning window is focused and shows that terminal (ActiveTerminalID) or that ADE task.
- Each window reports focus through AgentNotifyService.ReportFocus, debounced 150 ms (state/agentNotify.ts).
- Agent notifications have a cooldown of 10 s per (terminal or run, kind).
- A note id is <kind>:<id> so a newer note replaces the older one.
- Settings agent.notifyEnabled (master), notifyOnFinished, notifyOnNeedsInput, notifyOnRunEnded and notifyIncludeMessage all default to on.
- SendTest ignores focus suppression and cooldown.
- Notifier.Click focuses the window and pushes kira:agent:revealTerminal or kira:agent:revealTask (EmitTo, Space only); an ADE record goes through FocusSession.
- notify_darwin.go (darwin && !server) registers the Wails v3 notifications service; its sink WailsSink lives in internal/desknotify, shared with prompt notifications (P246).
- notify_other.go returns no service, so Linux and -tags server builds post no notifications.
- Notification message text only goes to the local notification centre.
- AgentNotifyService is a Kira Space bound service.
- Agent notification tests: flows/notifyflow (hook events through the real shim) and tests/ui/settings-agent-notify.spec.ts.

### Prompt routing (P246)

One router decides which window shows every app-originated popup. Package `internal/prompts` holds
metadata only (`Prompt{ID, Kind, Ref, Origin, Title, CreatedAt}`); the owner keeps its queue and
answers. Bound as `PromptsService` (`List`, `MainWindow`, `Claim`, `Raise`, `Dismiss`, `SendTest`).

| Kind | Owner | Origin |
|---|---|---|
| `schedule` | `scriptruns` waiting insert (cron confirm) | none |
| `dbmcp` | `dbmcp.ApprovalBroker` (Studio) | none |
| `git-credential` | `gitcred.Relay` (Space) | stream window key, else none |
| `mobile-pairing` | `pairing.Broker` (Space) | none |
| `update` | renderer, through `Raise` and `Dismiss` | the raising window |
| `handover` | `ade.TaskBoard` (ref = task id) | none |
| `branch-request` | `ade.TaskBoard` (ref = request id) | none |

- A prompt's routing target is the claimed window if still registered, else the origin window if registered, else the main window, else "" (queued).
- The main window is shell.Registry.MainKey: the lowest open order, ties broken by key, never an ephemeral window.
- A registry change calls Reroute, so a queued prompt lands on the next window.
- The push kira:prompts:changed carries the full routed list; kira:prompts:reveal carries one id for a window.
- Claim(id, windowKey) pins a prompt to a window (used by 'Review and run').
- PromptHost (packages/workbench/src/prompts/) is mounted once in each App.vue and shows the oldest prompt whose target is its window.
- A kind dialog takes entry, renders its owner's state and emits hide on Escape; hidden prompts stay in the list until the owner closes them.
- Prompt notifications are per kind, with id prompt:<kind> and thread kira-prompts; sent on open when the target window is not focused, with a count title when several wait, and removed when the last of the kind closes.
- Prompt notifications are gated by advanced.notifyPrompts, which defaults to on.
- Clicking a prompt notification calls RevealKind: reopen a window when none is open, focus the target, push reveal. Answering in the app removes the note. Notifications have no action buttons.
- Deps.RaiseOnOpen (per kind; nil means never) brings a prompt's window forward on open: Open starts RevealKind in a goroutine after releasing its locks, because Focus blocks on the main thread. Studio raises only `dbmcp`, gated by dbMcp.focusOnApproval. Space passes none.
- A raise is skipped when the target window already has focus, fires at most once per kind per 2 s (the macOS Space-switch animation, when IsFocused may still read false), and never fires on Close, Claim or Reroute. A request resolved before the goroutine runs makes RevealKind a no-op.
- Activation trace (P396): internal/shell.AttachActivationTrace and Attach log app and window activation events at Debug (scope=window); WindowRegistry.Focus logs "window raise" and Router.Open logs "prompt raise" at Info.
- shell.WindowRegistry.Focus calls App.Show first on darwin only (unhide after Cmd+H); on Windows and Linux App.Show re-shows every window, so it is skipped there. A window Studio hid on last close comes back through Show.
- Platform behaviour of a raise. macOS: unhide, un-minimise, show, activate; macOS switches to the window's Space under the default Mission Control setting "When switching to an application, switch to a Space with open windows". Windows: best effort; foreground lock usually refuses SetForegroundWindow from a background app and the taskbar button flashes; a window on another virtual desktop is not switched to. Linux: best effort; gtk_window_present, Wayland compositors usually show "is ready" instead of focusing, workspace switch depends on the window manager.
- The notification sink is internal/desknotify (Sink, WailsSink), shared with internal/agentnotify; it is Darwin only, like P238.
- The stream no longer carries credential.request or credential.provide (contract 47); the relay snapshot kira:git:credential plus GitCredentialService.Provide is the only path for Git credentials.
- Prompt routing is tested by flows/promptflow in both apps, dbmcpflow TestDbMcpApprovalRoutes and TestDbMcpApprovalFocus, Space gitflow TestCredentialRoutes, mobileflow TestMobilePairingRoutes, tests/ui/prompts.spec.ts and tests/e2e-real/prompts-two-windows-real.spec.ts; the recording sink is internal/flowtest/notifysink.

### Agent usage limits (P239, P316)

ADE status-bar item `5h 23% · wk 41%` with reset times in a tooltip. No credential read, no network
call: Kira only listens to what the agent already computes. Package `apps/kira-space/internal/agentusage`
(`Service.Ingest`, `Get`), bound as `AgentUsageService` (Kira Space only), push
`kira:agent:usage` (Space only, at most one per 10 s on a changed value). Usage is an optional provider
role (`harness.UsageReporter`): only a provider that reports it feeds the service (Claude does), and
`AgentService.Info.usage` hides the item for a provider that does not. Windows are `harness.RateWindow`
(`UsedPercent` 0..100).

| Feed | Source | Units |
|---|---|---|
| `session` | interactive tab: statusline `rate_limits.five_hour` and `seven_day` | `used_percentage` 0-100, `resets_at` epoch s |
| `run` | headless run: stream-json `rate_limit_event.rate_limit_info.unifiedWindows` | `utilization` 0..1, `resetsAt` epoch s |

- claude -p never invokes a statusLine (checked on claude 2.1.295), so headless runs feed usage limits through their output instead, via agentmcp.Handler.OnRateLimits chained from ade/runs.go.
- For usage readings, the newest reading wins per window; the tooltip shows the source and its age.
- hooks.Server.Compose(provider, terminalID, cwd, command, configFile) writes <server dir>/settings-<12 hex>.json (0600; hooks plus a statusLine pointing at the shim in statusline mode) and passes it to --settings. With an app settings file (P453) `ComposeInto` merges into that file instead.
- Claude Code replaces the user's own statusline for the session, so the shim runs the user's statusline command and prints its output. The command comes from `claude.status_line` in the agent settings only (P453), passed as KIRA_USER_STATUSLINE. No Claude settings file is read or written.
- The shim forwards a payload containing rate_limits at most once per 30 s per terminal, in the background, to POST /statusline (same token and terminal checks as /hook, 64 KiB cap).
- The /statusline listener decodes rate_limits only; nothing else of the payload is kept.
- Usage states are ok, waiting (nothing yet: Usage: –, hint "Start a Claude Code session to see usage") and off.
- A usage window whose reset time has passed is dropped on read.
- The last usage numbers persist atomically to <KIRA_SPACE_HOME>/agent-usage.json (0600) and reload at start.
- Setting agent.usageEnabled (default on): when off, no statusLine is injected into new launches, the usage item is hidden, and Get returns off; it applies only to launches after the change.
- Frontend usage display: state/agentUsage.ts (TanStack Query, 60 s poll, push replaces the cache) and workbench/AgentUsageItem.vue (StatusBarItem; tooltip rows for session window and weekly, tones at 80 and 95 percent), shown in ADE mode only.
- Usage limit tests: flows/usageflow (statusline through the real shim and socket, run stream through the board) and tests/ui/agent-usage.spec.ts.

### Worktree setup

- A worktree is created by a run creating branches, Start, Take over in a new worktree, or Add existing branch; the repo's prepare script then runs with its timeout.
- Worktree setup has states preparing, ready and failed; the full output of the prepare script is stored.
- Until the worktree is ready, pipeline steps for that branch wait (waiting for worktree setup) and Start/Take over are not offered.
- Retry setup reruns the worktree setup and recreates the worktree.
- Attaching a remote-only branch creates a local branch from <remote>/<name> (origin preferred) that tracks it, implemented in ade/setup.go ensureWorktree (P231).
- Every start path (StartRun, LaunchStep, StartBranch, TakeOver, review agent, AddExistingBranch, RetrySetup, request_branches) records a setup failure reason in ade_worktree_setup.note (wire WorktreeSetup.note, recordSetupFailure) (P202).
- A launch blocked by a pending setup returns E_PREPARING (ErrSetupPending), and the UI waits (useSetupWait) and starts when the worktree is ready.
- One ScriptProgress component in packages/theme (running, ready, failed, elapsed, actions slot) serves both ADE (AdeSetupProgress, AdeWorktreeSetup) and git-ui (WorktreeDialog, toolbar strip with an Output popover via PrepareOutput.vue, Cancel while running).
- OpsState keeps worktreePrepareStatus, StartedAt, FinishedAt and LastPath; a prepare failure stays until dismissWorktreePrepareResult() is called, while success fades after 5 s.
- The Space Git window refuses worktree.prepare by design.

### Frontend

- Pure logic of the ADE frontend lives in frontend/src/ade/v2/board/ (status, actions, needs-you rank, timeline, drop plan, fix menu); components use <script setup>.
- ADE Pinia stores each have one concern: adeBoardUi (selection, tabs), adeDialogs (dialog flow and pending keys such as merge:<branch>:<target>), adeTakeOver, adeTerminals.
- ADE server state goes through TanStack Query (queries.ts), invalidated by the push channels.
- Task plus menu (P410, `sessions/AdeTaskLaunchMenu.vue`) on the card head and the Sessions tab bar: per worktree a Terminal (Git module tab via `openRepoTerminalTab`) or an ad hoc agent session (`StartAdHoc`). Disabled items show their reason inline.
- The repository list is one Pinia store (state/coderepos.ts) fed by the boot-time kira:adetask:repos subscription (initCodeRepos, called from main.ts), so every module in every window sees every change.
- All repository configuration lives in one Git-module dialog, repo/ReposDialog.vue (P205), with a Repositories tab (import, list, per-repo form, remove) and a Scan folders tab (add, watch, hide all, remove). Agents has no Repos tab.
- The per-repo form in ReposDialog sets nickname, prepare script and timeout, integration branches, environments and worktreeBasePath (via UpdateRepo).
- ReposDialog opens from the Git panel header, GitStart.vue, a repo-row "Configure repository…" item and the Agents empty state.
- Repo hiding (P259): code_repos.hidden decides visibility for one-by-one and folder imports alike; ade_folders.hidden is the folder "hide all", which bulk-writes its repos, makes later discoveries import hidden, and still lets a single repo be shown inside a hidden folder.
- Hidden only filters the Git panel Repos list (repo/state/repoVisibility.ts); ADE, agent task_info, mobile and open workspaces ignore it, and the dialog lists every repo.
- The header eye toggle "Show hidden" is session-only; hiding a repo from the panel closes that repo's workspace in this window.
- Re-importing a hidden repo unhides it (ExistingRecord, no E_ALREADY_IMPORTED). RepoHeads runs for listed repos only.
- Hiding is bound to CodeWorkspaceService.SetRepoHidden and AdeTaskService.SetFolderHidden.
- "Remove repository…" lives in the dialog's repo head (ViewToolbar) and always goes through confirmRemoveCodeRepo, which names the scan folder that would re-import the repo.
- Session tabs (P193) are named "<step> · <repo>" (headless) or "<step> · <repo|spec>" (TUI), from the session's own stepId; review keeps "Review agent".
- `sessions/useOpenSession.ts` is the one path that opens a session (terminal id from `useSessions`); needs actions and the status bar agents list call it.
- Session tabs show state and age in a tooltip. Right-click menu (`sessions/useSessionMenu.ts`) has Copy session ID and, for a running TUI, Restart: `AdeTaskService.RestartSession` stops the record (`Tracker.Stop`, no grace wait), closes the terminal, then runs Take over on it.
- The task context menu (P198, board/taskMenu.ts, plan/useTaskMenu.ts) opens by right-click, Shift+F10 or the panel tab row's More actions button, and is built on existing calls.
- The menu primitive is the workbench singleton ContextMenu.vue (shadcn-vue DropdownMenu), used by every right-click menu in both apps.
- Pane convention (P194, P207): one frame per pane (shell passes mainFramed off), PanelHeader on every pane except the ADE Plan and its panel (P389: no PanelHeader; the shell tab names them, Refresh all sits in the Plan repo row, panel actions at the tab row's right end), nav and panel tabs on tabChipVariants, shadcn-vue controls at kira/kira-lg/kira-icon sizes, and tokens instead of arbitrary pixel classes.
- Arbitrary pixel classes are allowed only for timeline geometry, glyph ring weight and a few font-relative values; font-data is used only for SHAs, branches, paths, commands, YAML and code.
- Raw <button> remains in only three places: the task card title, the AdeAttention glyph badge, and AdeSessionStrip chips.
- ADE dialogs share dialog/flow.ts (target pick: new session via StartBranch, or Send to a running session), compose.ts (message templates), deliver.ts and turnWatch.ts.
- Plan drag and drop (plan/usePlanDrag.ts, board/dropPlan.ts) writes the day and position directly with no dialog.
- ADE notes use TipTap (notes/), and the fix menu uses the shared context-menu primitive (R20).
- ade/v2/tones.ts and palette.ts are dark-only; scripts/check-ade-colours.sh (part of bun run lint) fails any other colour literal under ade/.
- Plan board colour: status colour (16-colour palette, `--kira-status-*`) on the task status tag, red failures and amber needs-you/stale; branch tags other than red/amber render grey; task colour is the head dot and a 3px left card edge; the branch row repo badge wears the repo colour (repoTint). Plan pane ground is bg-chrome (terminal ground); cards of one day have no line between, 16px apart; weekend and day-off bands carry no tint. Task cards have a 1px border-badge edge. The task open in the panel wears a focus frame (border-focus ring-1) and aria-current; its head, branch row, span and history rows use bg-select.
- Plan window: shows today plus 6 days (`WINDOW_DAYS` in `timeline.ts`). Work scheduled later sits in Later, in date order before unscheduled and parked items; its card meta starts with the date. No horizon setting, no add-day control. Past bands come only from history or overdue work.

### Kibana Discover (P424)

- A Discover tab is a console tab carrying `kibana` state (data view, language, time, filters, sort, sample size, interval); the query text is the tab's own `text`. `ConsoleTabView.vue` dispatches on connection kind: kibana opens `views/kibana/KibanaDiscoverView.vue`, everything else `ConsoleView.vue`.
- `MonacoHost` props added for it: `monacoLanguage` (`{id, register(m)}`, registered once per id), `triggerCharacters`, `asyncCompletionSources` (abortable, may return `incomplete`), plus a `submit` emit on Enter in single-line hosts.
- `kqlContext.ts` is a tolerant lexer that classifies the cursor as field, operator, value or conjunction. `kqlCompletion.ts` serves fields, operators and conjunctions synchronously; values come from the `kibanaSuggest` bridge call through `queryClient.fetchQuery` (30 s stale, 150 ms debounce, aborted via `opsCancel`).
- Charts use Apache ECharts 6.1.0 with vue-echarts 8.3.2 (exact pins, Apache-2.0 and MIT, tree-shaken in `charts/echarts.ts`, SVG renderer so P426 can capture exact vector output). Declined: the shadcn-vue chart (Unovis; registry files carry a second plain `<script>` block and `@unovis/ts` pulls `three`, `maplibre-gl`, `leaflet`, `elkjs`), Chart.js (no brush range select), a hand-rolled SVG histogram (brush, ticks, tooltips, time axis).
- Saved-query link: tab state `kibana.saved` holds `{id, name, body}`, the body as last saved. Dirty is pure over tab state (`discoverDirty`, ignores `dataViewName`), so unmounted tabs show it in the tab strip. Save with no link prompts a name; with a link it updates, and on `E_NOT_FOUND` it unlinks and saves as the old name. Duplicating a tab drops the link. Bridge: `QueriesService.ListDiscover|SaveDiscover|UpdateDiscover`; rename, pin and delete reuse `Update` and `Delete`. `form.save` binds on the view root; the Monaco query bar emits `save` itself because Monaco consumes the chord.


### Review code (P150)

- The per-branch Review code button opens an ephemeral Wails window via OpenReviewWindow; a second call returns false and only focuses the window. The focus path (WindowRegistry.Focus) dispatches the App-level unhide to the main thread via InvokeSync, because AppKit aborts the process on it off-thread (P354).
- Review window rows live in ade_review_windows, are never restored on relaunch and are purged at boot (purgeReviewWindows).
- closeDecision keeps the main window from closing while a review window is open.
- The review window has three panes: git-ui review files (every changed file, target locked), diff tabs, and the review agent panel. One title bar (AdeReviewHeader inside the shared TitleBar strip, so the drag region and traffic-light inset stay) holds the task colour, title, repo, branch, base and an uncommitted badge.
- The files pane calls ensureRepoOpen first, because the git socket needs repo.open on its connection.
- The committed list reloads on repo.changed refsChanged (ReviewFilesState, coalesced), on window focus (AdeReviewFiles) and, with lockTarget, through ReviewSessionState auto-apply (new tip adopted without a manual apply). A selected file re-runs its diff after a reload, or clears if it left the list.
- Uncommitted changes show in a separate section under the committed list (review-working), reviewed apart from the committed review (see the uncommitted review bullets below). Source is AdeTaskService.ReviewWorking (porcelain status of the worktree against HEAD; branchId resolves the worktree server-side, never a client path). ReviewWorkingDiff returns the HEAD blob and the worktree file. The diff tab is repo-diff with a `working` branchId; it follows live data and has no go-to-file.
- The files pane splits into Committed (review-committed-header) and Uncommitted (review-working-header, warn border), each with its own reviewed count. A working diff tab carries a warn Uncommitted badge (repo-diff-uncommitted) and, inside a review window, the same hunk marks, gutter comments and thread as a committed tab. reviewDecorations runs over a ReviewSource (views/repo/reviewSource.ts): committedReviewSource talks to the git transport, workingReviewSource to the four AdeTaskService methods ReviewWorkingFile, ReviewWorkingMark, ReviewWorkingComments and ReviewWorkingCommentAdd. Edit, setState and remove of a working comment reuse the id-scoped review.comment.* git calls. Repaint events carry a scope (commit or working) so a mark of one review never reloads the other.
- Uncommitted review state lives in review_working_file and review_working_range, keyed by path, not by commit. A record stores the reviewed content (text up to 1 MiB, compressed) with a sha256 content_hash and ranges in that content's coordinates; binary, tooLarge and absent files take whole-file marks only. Reads project the ranges onto the current worktree file through a diff --no-index of snapshot against worktree (gitsession.contentDelta, shared with the committed slow path): a reviewed line survives an edit above it and is lost by an edit inside it.
- ReviewWorkingDiff returns the sha256 `contentHash` of the worktree text it read, and the diff tab keeps it in step with the Monaco models. The tab echoes it on ReviewWorkingMark (`contentHashes` by path) and ReviewWorkingCommentAdd; the file list sends the per-path `ReviewWorking.contentHashes` it shows. A mismatch answers E_CONFLICT before anything is written, and the diff query reloads, so text, hunks and hash move together and an agent edit between view and click never counts as reviewed. A path with no hash is unchecked; absent content hashes to the token `absent`, never "".
- Review comments carry an origin (commit or working) and, for working ones, anchor_content (gitreview 0005). A working comment anchors against the worktree as exact, projected or removed. Listed against a commit it reports anchor `uncommitted` with its stored range and is painted only in working tabs. The Comments pane and the workflow panel badge it Uncommitted; agent prompts mark it ` (uncommitted)`.
- Promotion (gitsession.PromoteWorkingReview, per path under Store.Lock, idempotent): when a worktree commit lands, checks and comments on lines the commit contains move onto the committed review at the new tip; the rest stay uncommitted. A path is touched only when the tip blob differs from the blob at the record's head sha, and only lines the work added carry over, so context lines never read as reviewed. It runs on ReviewWorking, the working mark, comments and comment-add calls, OpenReviewWindow and forwardMessage, from the worktree HEAD (not the cached refs); ReviewWorking reports the moved paths in `promoted` and the files pane then refreshes.
- The uncommitted queries (TanStack, useReviewWorking) refresh on the ade board push, window focus and agent PostToolUse or Stop events whose cwd is inside the worktree (debounced TIMING.fileSettle). No polling: an edit from another tool shows on refocus. FSEvents delivery and WKWebView focus are unverified off macOS.
- Since-review diff left side: the stored snapshot when the file changed since review (changedSinceReview), otherwise the merge base, so a reviewed file still shows its diff like a PR. Left equal to right is never used.
- The title bar toggle (ade-review-toggle-agent, last item at the far right, P392; useAdeReviewWindowStore.agentPanelVisible, off by default and persisted per device in kira.ade.review.agentPanel) shows the right pane. In a review window Cmd+J toggles it and Cmd+B is a no-op; the main window keeps the layout panels.
- The title bar's right group (AdeReviewActions) has a Review dropdown (Comment, Approve, Request changes) and Comment on {short}. Both open AdeReviewSubmitDialog, which posts a review or a conversation comment through changeRequest.submitReview or changeRequest.comment. Local review comments stay agent-only and are never part of a submitted review. Own change requests disable approve and request changes; merged or closed ones disable every review event. The review applies to the host's current head (commit_id); `headStale` (local tip set and not equal to the host head) shows a warning in the dialog and the title bar badge below (P412). Works the same for others' change requests (P392): lookup uses the host head name (an origin remote-tracking spelling loses its `origin/` prefix), and a task linked to a change request passes its number (ReviewWindowTarget.changeRequest), which covers forks. No change request found shows a disabled Review button with the reason. Errors render inline; there is no toast.
- Head indicator (P412): `ade-review-head` in AdeReviewHeader, from `useReviewHead`, which shares the `changeRequest.reviewState` query. Shows only for an open change request with a local tip: `current` ("At {short} head") or `stale` (warn badge; tooltip lists local and host shas). Refreshes on mount, window focus, 60 s staleness and git-stream `repo.changed` refsChanged of the window's repo (debounced TIMING.fileSettle). No polling.
- The since-review review_session is pinned (SetPinned, gitreview 0003) so rebases cannot drop the reviewed blobs.
- The since-review diff's left side is the stored snapshot (review.snapshot), not a git ref; a renamed unchanged file keeps its review.
- The review agent is one interactive TUI session per task, stored as a row with purpose='review' (unique per task), step id review, cwd the first created branch's worktree, other branches via --add-dir.
- The review agent's first message (`ade.review.message`) and the dedicated round message (`ade.review.round.message`) carry the branch's PR link (`changeRequestURL`): the task link's change request when it names this repo (GitHub links), else the host lookup by branch name; empty when the host is off, unavailable or has none. A missing link never blocks a launch. The followup and forward messages carry none.
- A stopped review agent row resumes with --resume and the same claudeSessionId; if the recorded cwd is gone, the old row loses purpose and a new one launches.
- TakeOver of the review agent row goes through launchReviewAgent.
- The review agent is a plain terminal with no question box. ForwardReviewComments pastes ade.review.forward.message with the branch's open comments through Tracker.Send and stamps sentAt; the button is disabled while the agent waits on input or no comment is open.
- Review comments carry kind (change|question) and state (open|addressed) plus the agent's reply, repliedAt and sentAt (gitreview 0004); older rows read as change/open.
- review.comment.add takes kind; review.comment.setState reopens or addresses a comment; the export marks the kind and state.
- The review window's right pane has SecondaryTabs (ade-review-right-tabs): AdeWorkflowReviewPanel (comments, route, Send to agent, round block, finish buttons; shown while the branch has a review step run) and the review agent. The tab pick lives in useAdeReviewWindowStore.
- Keep Monaco objects in shallowRef (useDiffEditor), because a deep ref hangs the page.
- Review entry rule (P240): board/reviewCode.ts reviewChoice hides (parked), disables with a reason (uncreated branch, ahead === 0) or enables each branch; useReviewCode is the one caller of OpenReviewWindow.
- The ade.reviewCode command uses Cmd/Ctrl+Shift+R as a local binding with no accel.go row; a task with several branches shows a picker.
- Interpolated UI values render through VarText (packages/theme, TextPart) as the resolved value in a marked chip; MenuItem.hint takes TextPart[]. A part with `kind: 'branch'` renders as `BranchChip` (git-branch Badge, the git graph ref-badge recipe); ADE builds branch-name parts with `branchParts`. `kind: 'path'` renders `PathChip` (folder icon, same Badge). Both share `ValueChip` (Badge + icon + truncating value).

### Stage handover (P407)

Stage = workflow step. A handover is text one stage leaves for the next. Table `ade_task_handovers` (migration 0057): one `handed` row per stage move, plus at most one open `draft` per (task, stage), enforced by a partial unique index.

- Sources: `agent` (the `handover` tool), `run` (a finished run's summaries), `user` (notes typed in the stage dialog).
- A draft is written by `TaskBoard.RecordHandover` (`ade/handover.go`). A stage move (`StepDone`, `SetTaskStep`) turns the draft into a handed row in the same transaction as `SetEntryStep`; `notes` replace the draft, empty notes drop it, nil keeps it. A move with no text records nothing.
- Run handover: a step run that ends `done` while the route path now ends past the step inserts a handed `run` row (summaries joined, an open draft of the step folded in).
- MCP tool `handover` (`notes`, `finished`, notes at most `agentmcp.MaxHandoverBytes` = 16 KiB) is granted to task sessions without a run (`TaskID != "" && RunID == ""`): manual step sessions, branch Start sessions and the review agent. Run grants report through `finish_step` instead.
- `Grant.StepID` is the stage the session was opened for. The recorder refuses a call after the task moved on: "The task moved on to <name>; nothing was recorded. Tell the user." An empty `StepID` skips the check.
- A `finished` draft on a live stage opens the routed prompt `handover` (ref = task id). It reopens at `Start()` and closes on a stage move, `DismissHandover`, draft delete and archive. `DismissHandover` keeps the draft and marks it dismissed.
- Next-stage input: `handoverFor(tc, stepId)` returns the newest handed row only when its target is that stage. The step prompt gets Notes, Handover and HandoverFrom outside the `Message` block, so an edited Run dialog message keeps them; manual, review and start messages gain the same block (fragment `ade.handoverBlock`) and the tool line (`ade.handoverLine`).
- Wire: `Task.handoverIn` (newest handed row), `Task.handoverDrafts`; `AdeTaskService.Handovers` (history), `UpdateHandover` (empty body deletes), `DismissHandover`; `StepDoneArgs.notes` and `SetTaskStepArgs.notes` (the mobile `POST /api/ade/tasks/step` passes `notes` through). Notes over the limit fail with `backend.space.ade.handoverTooLong.body`.
- Frontend: `state/adeStageDialog.ts` (one store) drives `stage/AdeStageDialog.vue`. Every step move opens it: step mover, step block menu, task menu, the Done action and the routed `stage/AdeHandoverPrompt.vue`. Moving a manual step to its natural next stage calls `StepDone`; any other move calls `SetTaskStep` with `fromStepId`. `notes/AdeHandoverList.vue` lists drafts and history in the Notes tab and the archived panel. The session dialog's `compose.ts` mirrors the server's handover block and tool line.
- A `handover` call with `finished: true` replies with the task's artifacts (see Task artifacts).
- Tests: flows/adeflow `TestStageHandover` (contract `ade-handover`) and tests/ui/ade-v2-handover.spec.ts.

### Branch requests (P408)

An agent asks the user to approve branches and worktrees. `request_branches` blocks until the user answers; `ade/branchrequest.go` holds the engine.

- Phase 1 validates under the task lock: repos resolve, the task is writable, each repo is failed, `existing` (branch and worktree exist) or an item for the user. Items are kind `branch` (unnamed, base shown) or `worktree` (named, no checkout). No item means the call answers at once and raises no prompt.
- Phase 2 waits with the lock released, so board writes continue. One request per task: a second call gets "already waits". One resolver wins; end conditions:

| End | Result |
|---|---|
| Approve | checked repos created, unchecked `denied` |
| Deny | every repo `denied` |
| Timeout (30 min, `TaskBoardDeps.BranchRequestWait`) | `timeout`, nothing created |
| Caller gone (grant released, session stop) | request abandoned, nothing created |
| Archive, app close | abandoned |

- Phase 3 re-locks, reloads the task and creates the approved branches (`createBranch`, origin `agent`, `startSetup`), since the task may have changed during the wait.
- Prompt: routed kind `branch-request` (ref = request id), opened and closed under the broker mutex so a fast answer cannot close it before it opens. While a run waits its note reads "waiting for branch approval". `Task.branchRequestId` shows the open request on the board.
- The CLI cuts an MCP call at 60 s. Every `claude` launch sets `MCP_TOOL_TIMEOUT` to 35 minutes (headless env in `harness/claude/headless.go`, TUI env in `hooks.Server.Env`). `TestAdeBranchRequest` holds a request 65 s to guard it.
- Desktop only: the answer comes from the dialog (`AdeBranchRequestDialog`, store `adeBranchRequest`), the routed prompt or the Task tab row. Escape hides the dialog and never answers. Mobile has no approval surface.
- Bridge: `AdeTaskService.BranchRequest`, `ApproveBranchRequest` (non-empty subset, rest denied), `DenyBranchRequest`; `resolved` is false when the request is already gone.
- Tests: flows/adeflow `TestBranchRequest` (contract `ade-branch-request`) and tests/ui/ade-v2-branch-request.spec.ts.

### Task artifacts (P409)

An artifact is a change request, issue, branch, commit, deployment, doc or link a task produced. Table `ade_task_artifacts` (migration 0058) holds only reported ones: `agent` (MCP) or `user` (Artifacts tab). Detected ones derive on read, never stored.

- Aggregator `ade/artifacts.go`: `TaskArtifacts` runs every `ArtifactSource` under a 5 s timeout via errgroup. A failed source becomes an `ArtifactGap` (source, repo nickname, and for trackers a reason and a count), never an error. Built-ins, in order: `taskLinkSource` (code host link only), `repoSource` (branch, commits, change request, deployments through the code host), one `trackerSource` per issue tracker (P417), `storedSource`. Extra sources plug in through `TaskBoardDeps.ArtifactSources`.
- Dedup by URL, else (kind, repo, ref). A detected row wins over a stored one. Sorted by kind.
- Stored rows: unique on (task, url) when url is set; a re-report updates kind, title, ref. Limits in `agentmcp`: 20 per call, 200 per task, title 200, url 2048, ref 512; url must be absolute http or https.
- MCP tool `report_artifacts` (`artifacts`: kind, title, url, ref) is in `SpaceToolIDs`. `finish_step` and `handover` with `finished: true` append the full list as text: header line, then compact JSON `{"artifacts","gaps"}`, at most 100 rows plus a "more" line. A list failure appends an unavailable line and never fails the call.
- Wire: `AdeTaskService.Artifacts`, `AddArtifact`, `RemoveArtifact` (stored rows only). Board pushes invalidate the artifacts query.
- Frontend: Artifacts tab in `AdeTaskPanel.vue` and `AdeArchivedPanel.vue` (read-only); `AdeArtifactList.vue` also shows in the stage dialog when moving to Done.
- Tests: flows/adeflow `TestTaskArtifacts` (contract `ade-artifacts`), tests/ui/ade-v2-artifacts.spec.ts, real `claude` `TestAdeReportArtifacts`.

### Tests

- ADE Go tests cover internal/ade (board, integration, deploy, run engine, steps, tracker), adeflow, agentmcp and bridge/adewire; run them with go test -race, together with gitsession.
- The P199 protocol is covered by agentmcp TestGrantScopes (go-sdk client) and agenttools_test.go.
- ADE frontend tests: tests/unit/ade-v2-* cover board parity with the mockup's logic, dialog, drop plan, progress and timeline; ade-notes-markdown is also covered; 33 tests/ui/ade-v2-*.spec.ts run on the mock runtime (ade-v2-task-launch covers the task plus menu; contract `ade-adhoc`, flow `TestAdHocSession`, real `claude` `TestAdeAdHocSession`).
- There is one ADE visual spec, tests/visual/ade-workflow-graph.spec.ts (P247).
- The live smoke test (server build, real claude -p) covers runs, send-back, restart recovery, Spec/Start/Take over sessions, and merge, rebase and archive dialogs.

## Mobile agents web (P212, Kira Space)

- The mobile web server (apps/kira-space/internal/mobileweb) is off by default (setting mobile.enabled, mobile.port default 7790) and is changed only through bridge.MobileAccessService actions, which restart the server when it is running.
- Mobile web transport is plain HTTP with no TLS, PWA or setup page, by user decision: the phone view works on the trusted home LAN only, with no cert install and no Tailscale.
- The mobile server listens on exactly one address, the trusted interface's IPv4; never 0.0.0.0 and never loopback, and Start refuses a non-LAN address.
- The guard middleware rejects peers that are not private or link-local IPv4 inside the bound interface's subnet, and rejects any Host that is not the bound IP literal (DNS-rebinding guard; no localhost, no .local).
- SetNetwork rebinds on a DHCP address change: the new listener starts first, then the old one closes; on failure the old listener is kept.
- Trusted network identity (internal/lannet) is subnet + router IP + router MAC; the interface name is display only, so Wi-Fi and Ethernet on one LAN match. A single row mobile_trusted_network is set by TrustCurrentNetwork and cleared by ForgetNetwork.
- lannet.Detect uses the default IPv4 route, the interface address containing the gateway, and the router MAC from the ARP table. Linux reads /proc/net/route and /proc/net/arp; macOS reads the route RIB via golang.org/x/net/route.
- Network trust uses no SSID (macOS needs a location permission) and sends no probe traffic (it could raise the macOS 15 Local Network prompt).
- While mobile access is enabled, a bridge supervisor (bridge/mobilenet.go, every 10 s) runs lannet.Find: it stops the server with a stop reason (notTrusted, away, otherRouter, unavailable) when the trusted network is absent, restarts it when it returns, and rebinds on a new address.
- lannet.Find ignores the default route, so a VPN owning it does not stop the server, but Trust requires the default route to be on the LAN.
- SetPort reconciles whenever the supervisor is active, so a server stopped by a busy port retries on the new port.
- A router missing from the ARP table reads as otherRouter and is retried at the next poll. MAC spoofing is not prevented: the trusted-network check guards against use away from home and is not authentication.
- Because traffic is plaintext, anyone on the same LAN can read the board and terminal output and can copy a phone's cookie to act as it. Mitigations: trusted network only, device tokens expire after 30 days, revoke, and an always-visible warning.
- Pairing uses the generic internal/pairing.Broker[M] with a 120 s timeout, 60 s cooldown on deny keyed by remote IP, and queue 8; POST /api/pair long-polls, the phone shows a 4-digit code, and the desktop dialog shows it with IP and label.
- On pairing approval a tokenauth token is minted and the mobile_devices row (migration 0021) is written before the cookie is sent; only a salted hash is stored, with constant-time verify and a dummy verify on a missing row.
- The device cookie kira-space-device is HttpOnly and SameSite=Strict but not Secure, because a __Host- name needs Secure, which plain HTTP never sends; cookies are not port-scoped, so another HTTP service on the same IP would receive it.
- Device rows expire 30 days after pairing; an expired device gets 401 E_EXPIRED and re-pairs. Expiry also ends live use: handleEvents ends the SSE at ExpiresAt, the terminal authorized callback checks it, and a one-minute maintain loop using Server.expiries disconnects streams and releases terminals of expired devices.
- Migration 0025 revoked the HTTPS-era mobile device rows, and boot deletes the pre-P223 CA files in KiraSpaceHome()/mobile.
- The Origin header on mobile writes must be http:// and match Host; Sec-Fetch-Site is optional because it is absent on plain HTTP.
- Mobile rate limits (x/time/rate, per IP, bounded map): pairing 1 per 10 s burst 3; failed auth 1 per minute burst 10 (then 429 on every /api); reads 20/s burst 40 per device. singleflight coalesces Board/Prs.
- The routes() table is the whole mobile API and a test walks it; each row has a method, permission (permNone, permWrite, permAgentInput), kind (plain, SSE, WebSocket upgrade) and audit action. A test checks every non-GET row has a permission and an action, and no other POST exists.
- The mobile API's only public route is POST /api/pair. Writes are POST: /api/ade/later/items, /api/ade/shelf/move, /api/ade/tasks/{step,run,launch}, /api/agent/sessions/{id}/{send,take-over}; the terminal is a GET upgrade at /api/agent/sessions/{id}/terminal.
- /api/ade/sessions, /api/agent/sessions and their SSE channels drop cwd (withoutCwd).
- Write chain per route: csrfGuard (Origin check, also on the upgrade), deadline, withDevice, requirePerm, writeLimit, idempotency (POST), audited, handler. Handlers call the same AdeTaskService methods as desktop via bridge.MobileWriter, so validation and error text match; no ADE rule lives in mobileweb.
- Per-device permissions are can_write ("Changes") and can_agent_input ("Agent input") (migration 0022), plus global mobile.agentInput (default off); agent input needs both. New pairings get write on and agent input off; pre-0022 rows get both off. Only the desktop pane sets them.
- Permission refusals return 403 E_WRITE_OFF or E_AGENT_INPUT_OFF. Turning agent input off (global or per device) or revoking a device ends its terminal holds.
- Every mobile write except /api/pair needs an Idempotency-Key header (UUID per user intent, reused on retry). The store is an expirable LRU keyed deviceID|key, 1024 entries, 10 min. Same key and path replays the stored response; another path gives 422 E_IDEMPOTENCY_MISMATCH; still running gives 409 E_IN_FLIGHT; 5xx results are not stored.
- Write rate per device is 1/s burst 10; terminal attach is 1 per 2 s burst 3.
- Stale guard: FromStepID on SetTaskStepArgs, StartRunArgs and LaunchStepArgs is checked inside taskMu; a mismatch is ade.ErrStale (bridge E_STALE, HTTP 409). Desktop leaves it empty.
- PTYs stay window-owned: a phone launch or take-over makes the server EmitTo kira:mobile:openLaunch to one real (non-ephemeral) window, which runs the desktop openLaunch and acks via MobileAccessService.LaunchOpened. The request waits 20 s (504 E_LAUNCH_TIMEOUT); no window gives 409 E_NO_WINDOW; the phone gets only the ADE session id, never a terminal id.
- internal/terminal.Arbiter (nil in Kira Studio) hooks BoundService for agent terminals, sees output from spawn and gates window Write and Resize; mobileterm.Broker implements it. Each terminal has a 1 MiB ring with absolute byte offsets, so attach replays history; a slow phone gets {type:"reset"} plus the oldest byte held and never blocks the PTY reader.
- Takeover has one controller per terminal: desktop (default), or phone (connected: phone input and size apply, window Write dropped, window Resize only recorded; offline: 60 s grace). It returns to desktop on desktop Reconnect, phone Release or leaving the screen, grace expiry, 15 min without phone input, device revoked or agent input off, server stop, or PTY exit.
- On return to desktop the PTY is resized to the recorded desktop size. A second device gets 409 E_TERMINAL_BUSY; the same device replaces its old socket. Attach resolves the session via Tracker.TerminalOf and accepts only a running TUI task session.
- The terminal uses WebSocket via github.com/coder/websocket (ISC) because SSE plus POST cannot keep keystrokes ordered on HTTP/1.1. Binary frames carry terminal bytes; text frames are JSON (hello, reset, throttled out; resize, release in). ?cols&rows are required and ?from=<offset> resumes.
- Terminal socket limits: read limit 64 KiB, input frame cap 16 KiB, input 100/s burst 200, resize 10/s burst 20. Close codes: 4000 released, 4001 reclaimed by the computer, 4002 replaced, 4003 session exited, 4004 timeout, 4005 permission off; the phone stops reconnecting on any of them.
- CSP connect-src is 'self' ws://<Host>, because older WebKit does not match ws: against 'self'.
- Audit logs one "mobileweb: write" line per write, attach, release and reclaim (device, action, ids), never message text or keystrokes; there is no persisted audit table.
- appevent.Tap forwards only window-wide Emit (not EmitTo/EmitFocused) to a hub that applies an SSE channel allowlist (kira:adetask:* read channels, kira:agent:sessions, kira:agent:event). Every kira:mobile:* channel is desktop only. Terminal bytes travel only over the attach socket; the client invalidates all queries on SSE reconnect.
- ade/v2/reader.ts (AdeReader) is the transport seam: desktop binds it to control, mobile to HTTP. The mobile app in frontend/mobile/ is a second Vite build (vite.mobile.config.ts, tsconfig.mobile.json, dist-mobile, embedded by main.go) with no @bindings alias, so an accidental desktop control import fails the build.
- The phone app has no service worker, manifest or install metas (the shell is one index.html) and avoids secure-context-only APIs: idempotency keys are a v4 UUID from crypto.getRandomValues because crypto.randomUUID is undefined on an insecure origin.

## Memory MCP server and module (P201, Kira Space)

- appwire.Options.EmbedSpec (nil means embed.Default) reaches MemoryService; embed.Spec.WithBaseURL swaps the scheme and host of the pinned model file URLs while keeping path and checksums.
- Memory storage is $KIRA_MEMORY_HOME/memory.db (default ~/.kira-memory/memory.db), its own SQLite file with its own migration sequence (internal/memory/migrations), shared by Kira Space and the stdio server, opened lazily with _txlock=immediate, WAL and busy_timeout 5000.
- The distilled/ directory beside memory.db holds documents written by the Memory module's 'Import as document', kept as import sources.
- The memories table holds immutable versions (current/superseded) with one current row per lineage enforced by a partial unique index; a trigger blocks update and delete except current to superseded.
- memories_fts is an FTS5 external-content index using tokenizer 'porter unicode61 remove_diacritics 2' with prefix 2 3; memory_events records every add/update/noop with origin, source tag, session, source, request id and rationale.
- Search is recall-first: every term becomes a quoted prefix term OR-ed plus the whole phrase, stopwords are dropped, and gate-written keywords are indexed. One builder, memory.BuildMatch, serves MCP, module and reconcile candidates.
- User input never reaches FTS5 as syntax.
- memory_embeddings (migration 2) holds one L2-normalised little-endian float32 vector per memory version, superseded ones too; one model at a time, rows carry the model id and search filters on it. Writing a vector never bumps memory_revision.
- The embedding model is Snowflake arctic-embed-s int8 ONNX (Apache-2.0, 33M params, 384 dims, CLS pooling, query prefix, pinned HF revision e596f50, id arctic-embed-s-int8-e596f50), embedding only the fact text.
- Model files (model.onnx 34 MB, tokenizer.json) download only on an explicit click in Settings > Memory to $KIRA_MEMORY_HOME/models/<id>/ with pinned SHA-256 (embed.Install, manifest written last), shared by the app and every memory-mcp process. MCP never downloads.
- The embedding runtime (internal/memory/embed, a leaf package) uses ONNX Runtime 1.31.1 via yalue/onnxruntime_go (cgo, dlopen) and WordPiece via gomlx/go-huggingface hftokenizer.
- Embedding runs in a worker subprocess (<Kira Space executable> memory-embed --model-dir …, via appwire.RunArgvShim), spawned lazily by whichever process needs vectors, speaking NDJSON on stdin/stdout in batches of at most 32 (embed.Client).
- The embed worker exits when its stdin closes: after 5 min idle (so the OS reclaims the model, which in-process Session.Destroy does not) and whenever the parent dies. A crash or cancelled request kills only the worker. A failed spawn is refused for 60 s (Reset clears it).
- Worker lifecycle lives in internal/memory/workerproc; model download, SHA-256 check and manifest live in internal/memory/modelstore (embed uses both).
- Builds with CGO_ENABLED=0 compile an embedding stub whose status is unavailable.
- The ONNX runtime library is taken from $KIRA_ORT_LIB, else Kira Space.app/Contents/Frameworks/libonnxruntime.1.31.1.dylib, bundled arm64-only by darwin:package (scripts/fetch-onnxruntime.sh, checksum-pinned); native code is never downloaded at runtime.
- ORT 1.31.1 has no x86_64 macOS build, so the x86_64 slice of a universal bundle reports embeddings unavailable; keyword search is unaffected.
- Search candidate pools are clamp(4 x limit, 100, 400) from each source, unioned by seq. Limit defaults to 40, max 200 (Space panel search uses 200).
- Keyword score is bm25 (columns fact 10, keywords 6, reason 3) divided by the pool best: 0.7 x bnorm + 0.3 x term coverage, coverage restoring order under the OR query. Semantic score is cosine mapped linearly between embed.Spec.QueryFloor and QueryCeil.
- Fusion with both sources is rel = 0.45 kw + 0.45 sem + 0.10 min(kw, sem); with no semantic, rel = kw. RRF was dropped because it kept rank only.
- Ranking boosts are multiplicative: grade user 1.10, external 1.05, agent 1.00; scope vs target project (same 1.15, company 1.05, personal 1.00, other project 0.85; none without target); recency 1 + 0.05 x 2^(-ageDays/90); superseded 0.60; group 1 + 0.05 x min(n,3) for other result lineages sharing a document, thread, task or request group (session groups never count).
- Only a semantic-only row at or below QueryFloor is dropped; nothing else is cut by score.
- QueryFloor 0.55 and QueryCeil 0.70 are calibrated for arctic-embed-s int8 (unrelated cosine p90 0.550, paraphrase median 0.602, max 0.698); the plan's 0.20/0.60 defaults sat inside the unrelated band.
- Search filters scopes and origins are hard filters (closed sets, else ErrInvalid). project is a boost target, not a filter; MCP defaults it to the server's project, and an empty string opts out. The gate passes the first item's project.
- Memory.ContextCount counts other current facts across the result's non-session groups (MCP ctx=N), a hint that memory_context is worth a call; there is no inline sibling expansion.
- Search results are ordered by score, relevance, keyword, then newer seq; Memory.match is keyword, semantic or both.
- Vector search is an exact scan over a per-process decoded cache (Store.vectors, keyed on model, dim, row count and max seq; migration 6 indexes memory_embeddings(model)), about 1.5 KB per fact resident; budgets are 150 ms at 20k facts and 600 ms at 100k.
- The search query embeds with a 5 s timeout, and any embedder failure leaves keyword results unchanged. The gate's related-memory lookup uses the same search (limit 10).
- Writes embed in the same transaction as the insert (best effort, 10 s). Reconcile candidates are FTS top 10 plus up to 5 vector neighbours with doc-to-doc cosine at least 0.85; without that floor every store would reach the Claude call.
- Backfill (Service.StartBackfill, kicked by Search, Store, service open and install) embeds memories lacking a vector in batches of 32, newest current first; each batch commits alone, and two racing processes write identical rows.
- Service.SemanticStatus is off (no embedder), notInstalled, unavailable (with worker message), indexing (done/total) or ready; the bridge overlays downloading (bytes). kira:memory:semantic, coalesced to one per 250 ms, tells the UI to refetch. MCP search_memories output carries semantic and a note when results are keyword-only.
- Service.Store pipeline: validation, gate (one Claude call that splits atomic facts, challenges ambiguity, weak reasons and secrets, adds keywords), reconcile (one call for facts with candidates; hash short-circuit and no-candidate add skip it), then per-fact commit in one BEGIN IMMEDIATE transaction with optimistic revision check (2 retries).
- A gate challenge stores nothing and is stateless: the caller resubmits with clarifications. At most 2 store pipelines run at once.
- memory.AgentRunner runs the active provider's Structured role with the model from Kira Space setting memory.models[provider id] (default: the provider's catalog default), resolved per call. The Claude provider (internal/harness/claude/structured.go) runs claude -p with --model, --safe-mode, empty --setting-sources, --strict-mcp-config, no tools, --disable-slash-commands, --no-session-persistence, --permission-prompts none, JSON output with schema, --max-budget-usd 0.50; input on stdin, empty temp cwd, session env variables stripped, auth variables kept, 120 s timeout.
- The Claude provider does not use --bare because it forces API-key auth.
- memory.models (P314, P316) is a Kira Space settings leaf in kira.db: a map from provider id to a model alias of that provider's catalog (Claude: haiku, sonnet or opus; aliases, so the CLI picks the current model of that family). AgentRunner takes a ModelSource: the app reads it through SettingsRepo, and memory-mcp and memory-ingest open kira.db per call (appwire.memoryModelAt; a missing file means the provider default and is never created). An invalid or unreadable value falls back to the provider default with a warning. A change applies from the next agent call in every process, with no restart. scripts/docshistory always runs the default.
- toolexec.RunIO returns stdout alongside an *ExecError because claude -p prints its structured error to stdout; unknown-flag stderr maps to harness.ErrOutdated ('update Claude Code').
- The MCP server is '<Kira Space executable> memory-mcp' over stdio, spawned per session by Claude Code, working with the app closed or open; main.go calls appwire.RunArgvShim before startupfail and acquireSingleInstance.
- MCP tools are store_memory (progress checking, reconciling, saving), search_memories (includeHistory, scopes, origins, project, limit), memory_history, memory_context, plus ingest_files and ingest_status when Options.Ingest is set (not in import mode); ingest_files with distill true needs Options.Distill, else refused.
- The memory MCP server registers as kira-memory with claude mcp add-json --scope user from Kira Space Settings > Memory, through the active provider's MCPInstaller (Claude: internal/harness/claude/mcpinstall).
- Memory MCP agent doc texts live in internal/llmprompts/registry.go (memory.mcp.*) and are read once per session.
- Memory store contract: the store splits and rewords facts; the agent sends each with reason, origin and scope and never invents a reason; files go by path.
- Take numbers in memory MCP tool descriptions from Go constants (memory.MaxItems, MaxGroups, search and context limits, maxIngestPaths), never literals.
- A code comment cites a memory by fact id (one token km_ plus 12 base32 chars, memfmt.FactID; P296), never a version id, and carries no header, fact copy, reason, origin or scope; the tool never writes or returns comment text.
- bridge.MemoryService emits kira:memory:changed after its own writes and, via a 2 s PRAGMA data_version watcher, after the MCP subprocess writes.
- memory is the last mode in MODE_ORDER (windows.mode vocabulary, no migration); the memory UI lives in shared packages/workbench/src/memory/ and Kira Studio hosts none of it.
- Memory setup lives only in Kira Space Settings > Memory (MemoryPane.vue), which also holds the memory model select, saved with the dialog; the module hosts no setup action, and MemorySetupHint only links there through MemoryModuleContext.openSettings.
- useModelDownloadsStore holds the download AbortController, so Cancel survives closing Settings.
- settings/SemanticModelSection.vue adds a semantic badge on rows found by meaning only.
- There is no detection of whether the memory MCP is registered with Claude Code (open item).
- Add memory is one free-text box capped at 1000 (MaxFactLen), sent as one item with a fixed manual reason; the gate splits it into atomic facts. Origin is user, source ui, with a scope picker (default Personal) and a project combobox for Project.
- Fact id is the lineage id: km_ plus 12 Crockford base32 lowercase chars, matched by memory.FactIDPattern; version id is <fact id>.<version>, so grepping a fact id finds every version.
- Lookups (memory_history, memory_context, Lineage) accept a fact id (current version), a version id or a legacy uuid; agents cite the fact id.
- Migration 4 (0004_p272_provenance.sql) rebuilt memories and memory_events with new ids, kept seq (FTS and embeddings key on it) and recorded old ids in memory_legacy_ids(legacy_id, memory_id).
- The same fact and reason in the same scope and project is a noop (hash short-circuit).
- Each stored item must name origin (no default): user = human-challenged, correct; external (requires sourceTag, [a-z0-9-] 1 to 32 chars) = correct, probably incomplete; agent = reference only. Order user > external > agent.
- Reconcile may not update a candidate of higher grade; Go turns it into add ("Kept beside a higher-graded fact."). CHECKs tie source_tag to external.
- Provenance migration backfill: origin derived from old author, and external/doc for import-created versions.
- Each item names scope (required): company, project or personal. Hash noop and reconcile candidates stay within the same scope and project, so an update never crosses scopes. Backfill made every old row personal.
- Project key is the normalised origin remote (host/path lowercase, no scheme, credentials, port or .git, via memory.NormalizeRemote); with no remote it is local:<main worktree path> with whitespace percent-escaped (memory.DetectProject).
- Outside a git repo memory.DetectProject returns ErrNotGitRepo, and the MCP ingest_files falls back to the server's project only for that error. memory-mcp detects the project once from its cwd; a project item without a key uses it.
- memories.session_id is the session tool argument, else CLAUDE_CODE_SESSION_ID read by memorycli.Run at start, else NULL; UI, import and pre-existing rows are NULL. Shape: 1 to 128 chars of [A-Za-z0-9._:-].
- Memory groups are stored in memory_groups(id kg_…, kind, key, title, UNIQUE(kind, key)) and memory_group_members(group_id, lineage_id), append-only by trigger; membership is per fact so it survives versions.
- Group kinds: session (auto when session known), request (auto per store call), document (auto in import mode); callers may also pass task, document, thread (at most 5 per call, key 1 to 300). Every add, update and noop of a call joins every group.
- memory_context(id, limit) (Service.Context) returns, per group of the fact, newest first, total and up to limit other current facts (default 20, max 100).
- The memory text formats live in pure Go package internal/memory/memfmt (no I/O); internal/memory imports it and ids.go builds id patterns from it.
- Doc format is grep-first: each line is <id> <fact> with an optional reason line indented by two spaces; no field names, per-fact keywords, version or inline Markdown pairs; km_/kg_ tokens in text must be full ids; other memories are cited by id only.
- memory-check never resolves an id because the store is per user and teammates and CI could not; it flags only retired block headers (<marker> kira-memory <key>=).
- Migrate retired comment blocks manually: store each fact, cite the returned id, and never reuse an id from the block because its add ids were never stored.
- Retired v1 lines (<id> fact:, <id> reason:, <id> keywords:) yield one problem naming the P273 format; the formatter refuses fact text starting 'fact: ', 'reason: ' or 'keywords: '.
- Doc format kira-memory: 3 (P297) over .md/.txt has front matter, body sections ## Facts and ## Rules (each at most once, any order), optional ### <source section> under a category, and - <id> <text> items with an optional two-space-indented reason line.
- In the doc format, a fact line before a category, a ### before one, and any other heading are problems.
- A doc is in scope only when line 1 is --- and line 2 is kira-memory: 1, 2 or 3; versions 1 and 2 yield one problem each and no reader (retired, P297).
- The 'Stated in <from>, <heading>' locator uses the ### heading, never the category.
- FormatDoc writes ## Facts then ## Rules and drops an empty category; writeDocFile migrates a v2 doc of its own group in place; ApplyDoc takes the kind from the category; sameDocFact compares kind, so a kind change is a new version.
- memcheck reports one id with two kinds as drift. A doc has one provenance.
- memories.kind is fact or rule (migration 8, default fact, in the immutable trigger); the gate sets it and it is never caller-set; empty means fact, any other value is harness.ErrOutput.
- Reconcile keeps kinds apart: checkDecision turns a noop or update across kinds into an add. Existing rows are facts and there is no reclassification.

- Bulk import (P211) lives in internal/memory/importer and is bound as MemoryImportService, which shares the engine MemoryService opens lazily; the scheduler runs in a detached runner.
- Import scan accepts .md .markdown .mdx .txt .text .rst .adoc .json .jsonl .ndjson, at most 2000 files and 1 MiB each; binary, non-UTF-8, empty or too large files are skipped with a reason.
- Scan honours per-directory .gitignore, skips dot directories and build directories, never follows symlinks, and skips files whose content hash an earlier import already finished.
- The doneHash skip matches content hash and provenance (origin, tag, scope, project).
- Import chunking uses the goldmark AST for Markdown and blank-line blocks for text, packed to a 3000-token target (4000 hard cap, ceil(runes/3) estimate), keeping heading path and a 200-token tail of the previous chunk as context; oversize blocks split on lines, then sentences.
- ChunkJSON walks encoding/json tokens and keeps raw elements, so key order and number spelling survive; units are top-level array elements or object members, an over-size array/object element recurses, and an over-size string splits as text.
- For JSON chunks, HeadingPath is the JSON path (e.g. $[12]); .jsonl/.ndjson files use one unit per line; invalid JSON or a bad line fails the file with the byte or line.
- Import step 1 (extract) runs one claude -p per chunk, three at once, with the same isolation as the gate (--safe-mode, no tools, no MCP), returning atomic facts with an evidence quote.
- loadExtracted sets each fact's Section from the chunk heading_path and Reason to memfmt.Locator(rel, headingPath), both by code; the evidence quote is never stored (extract keeps it for the cross-check and finalize sees it).
- In the cross-check, a fact is questionable when extract marked it unresolved or its evidence is not in the chunk (whitespace collapsed, case folded); such files go from extracted to verifying.
- The cross-check makes one call per up to 40 facts, run with the source copied into an empty temp dir and flags --restricted --tools Read,Grep --allowedTools Read,Grep --max-turns N --permission-prompts none, without --safe-mode.
- The cross-check call must use --restricted, because with --safe-mode and --tools Read,Grep alone a Read of an absolute path outside the cwd succeeds (probed P274).
- Cross-check verdicts: confirmed and rewritten clear unresolved, unsupported drops the fact with a reason, ambiguous stays unresolved.
- Cross-check limits: timeout 120 s + 10 s per fact (max 10 min), budget 0.30 + 0.03 USD per fact (max 2.00), turns 3 + facts/4 (max 20).
- Import step 2 (finalize) runs one serial call per file with all the file's facts, using an inline --mcp-config for memory-mcp --import-ref, --strict-mcp-config, --tools "", --allowedTools limited to the three mcp__kira-memory__* tools, --permission-prompts none and --setting-sources "".
- Import finalize does not use --safe-mode, because it disables MCP servers; user-level CLAUDE.md stays out through --setting-sources "" and a tool outside the allowlist is denied.
- In import mode, memory-mcp --import-ref sets provenance, groups and session from the import job, source import, and source_ref = file id regardless of tool input, and joins the file's document group.
- If the import job is no longer found, import mode falls back to external/doc/personal.
- memory_events (migration 3) holds source_ref, constrained as (source = 'import') = (source_ref IS NOT NULL); history shows "via import (<path>)" through a join on import_files. The gate and reconcile run on every imported fact.
- Import mode sets StoreRequest.NoChallenge (Go-only) for origin external; decide stores accepted items and returns challenged ones with status stored, the gate input carries challenge: false, and finalize lists what the store still challenged under dropped, never unresolved.
- Live store_memory keeps challenges for every origin.
- StoreRequest.ImportPath (Go-only, set by requestFor from ImportProvenance.RelPath) pins the committed reason of every import add or update to the item's reason when ParseLocator accepts it, else to the bare locator.
- Finalize merges facts of one subject and aspect (at most 300 characters); the gate keeps a list about one subject as one fact and keeps a locator reason verbatim.
- Import state is in import_jobs, import_files and import_chunks in memory.db; extracted facts persist per chunk so a file resumes at the step it stopped.
- A runner taking the lock pauses running import jobs as "Interrupted"; nothing resumes without a click.
- P274 adds import file state verifying and file kinds json, jsonl, memdoc.
- Import retries (backoff/v4) are 3 per call; rate-limit errors share one cooldown across workers; usage-limit and auth errors pause the job; a budget error fails only that item.
- Added, updated and noop counts come from memory_events grouped by source_ref, never from the agent, so a re-run finalize cannot double count.
- Import security: containment of the Claude call that can store memories is the three-tool allowlist, forced attribution, the gate on every fact and no shell, file or network tools; a hostile document can still make the agent store a false fact, visible in history with its source path.
- MCP tools ingest_files and ingest_status on kira-memory ingest by path: content never crosses MCP; the tool creates a job and returns its id and the runner reads the files.
- Ingest origin is user, agent or external (UI jobs speak as user or external); MCP job facts carry the requesting session, the file's document group and the job's task/thread groups (job groups cap at 4).
- Ingest scope project without a project argument uses DetectProject of the first path, then the server's project.
- MCP import limits: a job over 300 chunks stops at awaiting for UI confirmation; at most 5 active MCP jobs; same roots and provenance as an active job returns that job's id; a relative or missing path is refused.
- ingest_files with distill true runs the import pipeline synchronously in process and stores nothing; there is no distill tool and no job rows.
- Distill pipeline: chunk, ModelAgent.Extract per chunk (three at once), build facts, cross-check questionable ones with ModelAgent.Verify in batches, then one ModelAgent.Merge call (no tools, no MCP) returning lines, unresolved and dropped.
- The distill merge covers the input only and never reads the store.
- Distill input is text (at most 120 000 characters, source name text.md) or one absolute file; store-only arguments are refused with distill, and text without distill is refused.
- Distill limits: 120 000 characters, 16 chunks, a tooManyFacts guard, and a 2.00 USD spend cap per call across all stages; no stage starts once reported spend reaches it. No retry.
- Distill is not registered in import mode.
- Distill mode `facts` (default) runs the pipeline above. Mode `rewrite` cleans one short text in its own form: text only (path refused), at most 2000 characters (`tooLong`), optional read-only `context` (6000 characters), refused without rewrite.
- Every text gets the rewrite call; there is no pre-check and no check call.
- Rewrite is one structured `ModelAgent.Rewrite` call (system prompt `memory.rewrite.system`, no tools, 90 s, 0.25 USD budget). Unusable output (`harness.ErrOutput`, empty text) retries once, then keeps the input with gate `output`. Other errors return as errors.
- Rewrite returns the full text: the input verbatim when already clean. Outcomes: rewritten (`changed` true), unchanged (`changed` false, gate empty, an accepted no-op), kept (a gate rejected the draft; input returned, `draft` and `gate` set). No shrink rule; `grew` (draft longer than input, plus a reflow allowance when the draft fixes an over-width input) is the only length guard.
- Text that does not open with a comment marker is prose: only `grew`, `form` and `tokens` apply, with no width rule and no reflow. Rewrite gates accept or reject, never edit text. `hedge` runs first, then the rest in order, stopping at the first failure: `grew`, `form` (comment markers, trailing newline; for prose, the draft must be the input minus listed hedge words, same line structure), `indent`, `width` (100 columns, tab 4; a changed draft keeps no line over 100, inherited lines included), `directive` (lines stay byte for byte), `tokens` (backtick spans, quoted spans outside backticks (backticks inside a span are stripped before comparing), numbers, identifiers must survive unless listed in `dropped`; path tokens checked per component; line breaks joined). `hedge` runs right after the model call, before every other gate, on every draft except one identical to the input: a draft with really, basically, actually or simply outside backtick code and quoted spans is retried once; a second such draft keeps the original, as does a retry call that errors (outcome `hedge`). A quoted span may wrap across one line break.
- Rewrite result: `Chunks` 0, `Lines` and `Unresolved` empty, `CostUSD` sums every call. MCP renders the final text, then `kept: <gate>` or `unchanged`, then `dropped: <text> | <why>` lines for a changed text. The result carries `draft` and `dropped` for audit; the UI schema omits `draft`.
- Distill UI: Facts and Rewrite tabs under source Text. Rewrite shows the text, why a gate kept the original, the dropped list and Copy; no Store or Import.
- bridge.MemoryDistillService runs the same importer.Distiller per call, sharing prompts and engine with MemoryService; only one distill call runs at a time app-wide (TryLock), a second gets E_CONFLICT.
- MemoryDistillService.ToImport writes <memory home>/distilled/distilled-<UTC>-<4 hex>.md (dir 0700, file 0600, heading plus one bullet per line, 1 to 640 lines) and creates an import job on channel ui.
- Distill progress is emitted on kira:memory:distill with payload {requestId, stage, done, total, costUsd}; stage is extract, verify, merge or rewrite.
- Cancelling a bound distill call cancels ctx (E_CANCELLED); stages already finished stay charged.
- importer.DistillError reasons (tooLong, tooManyParts, tooManyFacts, unreadable, spendCap) map to catalog keys backend.space.memory.distill.<reason>.body with E_BAD_REQUEST; other errors go through memoryErr.
- Distill is read-only: no memory.db write and no kira:memory:changed event.
- Distill UI state (draft, run, result, AbortController) lives in useDistillUiStore (Pinia), which survives a module switch; bridge calls go through TanStack Query (distillQueries.ts).
- Store… from distill results opens AddMemoryDialog in lines mode (one item per line, gate decides the kind, challenge unchanged, 20 lines at most).
- The import scheduler is one detached kira-space memory-ingest process per memory.db, guarded by an exclusive flock on <memory home>/import-runner.lock; a loser exits 0. Stderr goes to import-runner.log, restarted over 1 MiB.
- Engine is control only (DB writes plus ensureRunner); the runner communicates through the DB, ticking every 500 ms, arming running jobs and cancelling in-flight calls of jobs that left running.
- The runner exits after 30 s idle with no scanning or running job, or when the home is gone; a new runner pauses running jobs as interrupted and scanning jobs rescan; app quit no longer pauses imports.
- The app watcher polls data_version every 500 ms, emitting kira:memory:import on every foreign change and kira:memory:changed only when the revision moved.
- A .md/.txt whose first two lines are --- and kira-memory: 3 is kind memdoc (versions 2 or 1 fail); it parses with memfmt.ParseDoc (first 5 problems fail the file), with no chunk and no claude call, and provenance and group come from the doc header.
- Service.ApplyDoc per fact id: no lineage adds with the doc's id, equal fact and reason is a noop, an edit updates when the doc grade is at least the stored grade, else it is skipped.
- In ApplyDoc, a fact without a reason line takes Locator(from or the job path, section); an update keeps the stored keywords, an add takes the doc-level keywords; facts the doc no longer lists stay stored and count as orphaned; GroupRef.ID joins or creates the group by the doc's kg_ id.
- Memdoc header provenance wins on apply, so a memdoc whose header origin grades above the job's origin, or names another scope or project than the job's (narrowing to project of the job's own project passes), holds an MCP job at awaiting until the user confirms in the import view.
- Write-back is opt-in per job (writeDoc, confirm-dialog checkbox) and writes <dir>/<stem>.memory.md beside the source with renameio, never over the source; it holds only facts with the job's provenance and is skipped for memdoc input.
- Write-back overwrites an existing target only when it is a kira-memory doc with the same group; otherwise the file stays done with doc_error.
- importer.RenderDoc is the pure renderer shared by write-back and eval: a fact whose reason is the locator of its section takes that section as heading and no reason line, any other reason is written under the running section, and there are no per-fact keywords.
- Migration 5 (0005_p274_ingest.sql) adds job and file columns for ingest and rebuilds the three import tables through TEMP copies, because DROP TABLE with foreign keys on cascades into fresh rows.
- Migration 7 (0007_p281_verify_pending.sql) adds import_files.verify_pending, the count of questionable facts without a verdict (-1 = not computed), settled once per extracted file so verify and finalize claims query it and an idle scheduler tick reloads no file.
- Speech to text was removed in P224; Kira Space deletes the leftover speech model directory models/whisper-small.en-q5_1-5359861 at startup via modelstore.RemoveRetired.

### Memory ingest eval (P285)

- Never run the memory ingest eval in hooks or CI, because it runs real claude and costs money; it is an opt-in harness that scores the ingest pipeline on real repo text.
- The memory ingest eval runs with bun run eval:memory (TestMemoryIngestEval in apps/kira-space/tests/claude/memory_eval*_test.go); the script sets KIRA_MEMORY_EVAL=1.
- KIRA_EVAL_MODE is prepare, golden or run; run needs KIRA_EVAL_ROUND.
- KIRA_EVAL_PROMPTS is a JSON map {prompt id: text} that replaces registry prompts for one run, and KIRA_EVAL_BUDGET overrides the chunk budget as Target,Max,MinFill,MinTail,Context, both without code edits.
- KIRA_EVAL_LEDGER sets the spend ledger file name under the runs dir, one per part; the default is spend.jsonl.
- The eval corpus is 15 files pinned to commit 9ed83ae85 (manifest in apps/kira-space/tests/claude/testdata/memory-eval/P285-corpus/), read by git show: 10 tuning and 5 hold-out, one hold-out per category.
- The golden list has 585 facts (372 essential), written by Opus from the source text before any run; the judge is Opus, one call per source.
- Eval metrics are R_ess and R_all (golden recall, partial counts 0.5), P (precision), atomic, standalone, unsupported, duplicate, C_fact (stored fact chars over source chars), C_rec (fact, reason, keywords) and C_doc (the .memory.md text RenderDoc renders); noise bands come from two baseline runs.
- A tuning run costs about 4 to 5 USD and 8 to 10 min; a hold-out run about 1.3 USD.
- Eval caveats: the embedder is off (reconcile sees keyword candidates only); token numbers use ceil(runes/3); judge and extractor noise is as large as most single changes; the golden list rewards qualifier-heavy facts.
- In Part 2 (P285) the terse gate was reverted because R_all fell under its floor (final tuning numbers R_ess 0.859, R_all 0.658, P 0.904, C_fact 0.451); unified import was tried and reverted because C_fact reached 0.55.
- Part 4 (P289) used terse registry defaults on a 5-source tuning subset with ledger spend-p289.jsonl; round-p289-01 passed every Part 1 band (R_ess 0.877, R_all 0.756, P 0.614, C_fact 0.452) against a baseline mean of R_ess 0.871, R_all 0.712, P 0.635, C_fact 0.443.
- A bolder extract edit (01c/01d) was reverted in Part 4 because mean P 0.568 was under the floor 0.570.
- Part 3 (P285) added compact output format v2, locator by code and a merge rule; KIRA_EVAL_MODE=footprint with KIRA_EVAL_ROUNDS=<r1,r2> is free and re-renders stored facts of the listed rounds.
- C_doc is the RenderDoc render over source runes for every source, with renderProblems, docMismatch and locatorShare per source.
- Part 3 round-p3-01 gave tuning C_fact 0.434, C_doc 0.532, R_ess 0.885, unsupported 0.005, and hold-out C_fact 0.457, R_ess 0.876.
- P297: the extract prompt keeps rules and drops examples, tables and run results. Subset round-p297-01 gave R_ess 0.899, R_all 0.721, P 0.521, C_fact 0.444, C_store 0.654; P is inside the baseline spread.
- The P297 subset round needs KIRA_EVAL_PHASE_CAP_USD set above the run cap plus 2.
- Part 5 (P291) retrieval eval seeds a real memory.db through Service.ApplyDoc, one kg_ group per source with other rounds' sources as distractors, and uses real Service.Search; the embedding worker runs via KIRA_EVAL_EMBED_WORKER=1 re-exec and needs KIRA_ORT_LIB.
- Part 5 added modes: questions (Opus, a question and short query per golden fact, P291-corpus/questions.jsonl, 372 items), keys (Sonnet subject and aspect per stored fact), telegraph (Sonnet post-pass, token check internal/textloss), score with KIRA_EVAL_STYLE=tele, free retrieval and free compare (paired bootstrap, 2000 resamples).
- Part 5 base round-p3-01 retrieval: hybrid MRR 0.886, keyword 0.863, semantic 0.823, hit@10 0.957 (0.998 when the answer is stored); noise bands are hybrid MRR 0.026, keyword 0.020, token recall 0.024.
- Part 5 not adopted: embedding recipe fact-kw (+0.005 MRR), subject and aspect keys (+0.004 to +0.014), section grouping (returned chars 4.1x), telegraph style (fact chars -7%, C_store -4.5%, MRR -0.004), unified import (C_doc 1.017, MRR +0.019), telegraph on extract (C_store 0.753 vs 0.661).
- The .memory.md writer adjusts a title containing ': ' or ' #' and strips inline Markdown from facts before memfmt.FormatDoc; a fact that stays invalid is left out and counted in doc_error. Before this fix most imports failed write-back with a format error.

## Docker module (P200, Kira Studio)

- The Docker module is registered in Kira Studio only.
- The Docker backend is Go package internal/docker, using the Engine API via github.com/moby/moby/client v0.5.1 with typed Options/Result structs; ssh:// endpoints use docker/cli connhelper.
- DockerService embeds *docker.BoundService, and the module uses channels kira:docker:changed, status, stats, logs and exec.
- The Docker frontend is package packages/docker-ui (@kira/docker-ui), with server state on TanStack Query.
- Live logs virtualize unless Wrap is on; with Wrap on, the last 2000 filtered lines render. The log buffer cap is 20 000.
- Docker exec uses TerminalHostView with storeId 'docker-exec'; the Terminal tab opens no session until New session is clicked.
- Per-container CPU and RAM show in the engine overview table, the container header and a Stats view; the side list shows none.
- Docker endpoint resolution order (endpoint.go): selected UI context, DOCKER_HOST, DOCKER_CONTEXT, then `docker context show` (cached until the client is rebuilt; TLS contexts included), default socket, then socket probe (Desktop, Colima, OrbStack).
- Docker streams are per window (windowKey); they end on WindowOpenerDeps.OnWindowClosing, service shutdown and quit, and run only while the module is visible. One exception: the restore tracker (P334).
- Stats share one engine stream per container across windows.
- ModeDef.tabStrip: false hides the tab strip.
- Storage metrics (P252) are on demand only: DiskUsage (/system/df, verbose) and ContainerSize (inspect Size) are bound methods, and singleflight dedupes concurrent calls.
- Storage metrics use Docker API data only: no host free/total disk, no statfs, no helper container.
- Disk-usage query keys sit under ['docker-disk'], outside ['docker'], so Refresh, container actions or live-sync invalidation never trigger a walk; queries use enabled: false plus manual refetch().
- The engine-wide last disk-usage result persists per engine scope in localStorage (kira.docker.diskUsage, newest 5 scopes kept) with its timestamp; container size is session memory only, never persisted.
- Container.origin/originName are inferred from labels, first match wins, in order: devcontainer, testcontainers, kind, kubernetes, swarm, buildx (name prefix buildx_buildkit_), compose.
- A compose-based devcontainer stays grouped under its Compose project and its row shows the devcontainer icon. Only Compose projects group; other origins get a row icon (OriginIcon.vue).
- The Compose icon is lucide Boxes in Compose blue (#00b4ff), because no allowed icon library ships a Compose mark.
- Container edit (P253) uses bound ContainerEditSpec, UpdateContainer and RecreateContainer; the spec splits into in-place changes (limits, restart policy, name, networks) and recreate (everything else).
- baseHash is the sha256 of the canonical spec JSON; applying an edit with a stale hash returns E_CONFLICT. One lock per container (Manager.editing).
- ContainerUpdate treats zero as unchanged, so clearing a limit needs recreate; alias changes need network disconnect then connect.
- Recreate sequence: create -kira-new-<hex>, rename old to -kira-old-<hex>, stop old, rename new, start new, remove old. Any failure rolls back and returns E_RECREATE_FAILED with details {step, restored, containers}.
- On recreate, anonymous volumes are detached, not deleted, and no image is pulled. Swarm and Kubernetes containers are read-only.
- mapErr maps engine conflict to E_CONFLICT and permission denied to E_INVALID.
- The container Edit view uses two shadcn Tabs (In place, Recreate) with a neutral dirty dot and no per-field mode badge; an In place field whose mode resolves to recreate shows a plain hint.
- The registry link is lazy: DockerService.ImageRegistry resolves one image's page on demand, so the image list makes no Hub call. registryLink picks the repo digest naming the first tag's repository.
- With a containerd image store, locally built images also get a digest, so an implied Docker Hub name is Unconfirmed (button shown, no Hub call). Clicking calls ConfirmImageRegistry, which asks hub.docker.com, opens the page on 200 and shows "No registry page found" on 404 or error.
- Opening a detail view sends no local image name out; a named registry host links without a check.
- Flow tests set WithHubAPI to a fake.
- The registry button opens through LinkService.OpenExternal (Studio binds it again, 30 services); docker-ui takes openExternal in its context options.
- Container size lives in the Stats tab, which also opens for stopped containers.
- Restore after engine restart (P334): the tracker is a backend loop owned by Manager (restore*.go), independent of windows. Ticks 15 s while the engine answers, 30 s while it does not. Remote endpoints (tcp, ssh) tick only while a window watches.
- Boot identity: Info.ID keys the snapshot. The default bridge network ID marks the boot: new on every daemon start, kept under live-restore (measured on dockerd 29.9.2).
- Snapshot rules: never written while the engine is unreachable. A stopped container leaves after a 30 s grace. Pending is frozen until Restore clears it or Dismiss. Testcontainers, buildx, kind, kubernetes, swarm and `--rm` containers are never snapshotted.
- Storage: tables docker_restore_engines and docker_restore_containers (migration 0044). Snapshots not updated for 30 days are pruned at start. internal/docker sees only the RestoreStore interface.
- Restore is click-only. A Compose project starts as one unit in depends_on order (service_healthy waits up to 60 s). Deleted containers are listed, never recreated. Boot-time restore is out of scope.
- Channel kira:docker:restore; query key ['docker-restore'] sits outside ['docker']. The banner shows only inside the Docker module.
- The restore dialog is the shared presentational `workbench/restore/RestoreDialog.vue` (P441); docker-ui and the Runs module each wrap it with their own data and calls.
- Flow seams: docker.WithRestoreTiming, docker.WithBootProbe, flowharness.BootGen (simulates a daemon restart). A real dockerd restart runs only with KIRA_FLOW_DOCKER_RESTART=1.
- UI styling: start uses text-ok and stop text-error module-wide; selected rows use neutral bg-select; left-panel section tabs are the git module's ToggleGroup.

## Kubernetes module (P439, Kira Studio)

- The Kubernetes module is registered in Kira Studio only, next to Docker. It is read-only: the backend issues GET requests and nothing else, with no create, update, delete, exec, attach or port-forward path.
- The backend is Go package internal/kubernetes. `Service` is the domain surface the P440 MCP tools call; `BoundService` is the Wails surface, exposed as KubernetesService. Channel: kira:kubernetes:logs.
- Client: k8s.io/client-go, api and apimachinery v0.37.2 (Apache-2.0) with sigs.k8s.io/yaml. It adds about 25.9 MB to the stripped Studio binary (74.5 MB to 100.4 MB, same embedded frontend, `go build -ldflags="-s -w"`). `kubectl` shell-out was declined: it needs a kubectl install and parses text. The kubectl describe package was declined: it adds about 36 MB (cli-runtime, kustomize). Describe is built from unstructured objects plus an events list.
- Environment: PATH and KUBECONFIG come from a login-shell probe (cached, `WithEnv` seam), so a Dock-launched app sees what the terminal sees. Exec credential plugins (aws, gcloud, kubelogin) run with the resolved PATH; their command is pinned to the resolved path. Interactive exec plugins are not supported.
- Kubeconfig is loaded per call (cheap, so edits apply without restart). KUBECONFIG may list several files. A missing or invalid file is reported in `Clusters().config`, not as an error.
- Cluster rows: one per kubeconfig context, stored in `kube_clusters` (migration 0049, unique per context and server, INTEGER ms timestamps) through `ClusterStore.Ensure`. The row id is stable and is the key P440 hangs per-cluster permissions on.
- Connect is lazy: no cluster is probed until it is expanded or selected, so SSO plugins never fire for clusters the user does not open. Clients are built once per cluster (singleflight) and dropped by `Status{refresh: true}`.
- Errors map to reasons (unreachable, unauthorized, forbidden, plugin, config, timeout) with details `{"reason": ...}`; the UI picks the copy by reason.
- Lists use the server-side Table format (`Accept: application/json;as=Table;v=v1;g=meta.k8s.io`), so columns and cells match kubectl. Rows page by limit 500 and continue, capped at 5000 rows; past the cap `truncated` is true. Label selectors are validated before the call (E_BAD_REQUEST).
- Describe returns overview fields, labels, annotations, owners, conditions, up to 200 events and a YAML manifest. Secret data and stringData become `<N bytes>`; the last-applied annotation and managedFields are dropped.
- Logs stream per window and per stream id through a coalescer (16 ms, 500 lines). A line is capped at 64 KiB; the UI keeps the newest 20 000 lines. Follow with Tail all asks the server for 20 000 lines, because a byte limit would end the follow stream. Streams end on LogsClose, window close, service shutdown and quit.
- Frontend: apps/kira-studio/frontend/src/kubernetes/ (app-local, no second consumer). Mode `kubernetes` has no tab strip. Server state is on TanStack Query under ['kubernetes', clusterId, ...]; lists refetch every 10 s while the document is visible. UI state is one Pinia store; the last cluster, kind and namespace per cluster persist in localStorage (kira.kubernetes.selection).
- Flow tests run against `flowharness.KubeFake`, an in-process HTTPS API server (client-go sends no credentials over plain HTTP). It serves discovery, Table lists, objects, events, logs and an exec-plugin cluster. No real cluster is used anywhere.
- P440: `Service.SetMcp` stores per-cluster MCP modes and description (migration 0051, above Studio's reserved 0050). `ListCapped` and `ReadLogs` (one non-follow request, 256 KiB cap) feed the MCP tools. See "Kubernetes tools (P440)" under Database MCP server.

## Database MCP server (v1.7)

### The server (M1)

- internal/dbmcp is an MCP protocol server using go-sdk/mcp over loopback-only Streamable HTTP with mcpauth bearer token, fronting the app's own database connections.
- mcpauth writes token files with renameio.NewPendingFile and WithStaticPermissions(0600), so an existing looser file is re-tightened.
- The database MCP server is this app's only embedded MCP server since v1.9 P97 removed the repo-map one it originally sat beside.
- The database MCP server uses DefaultPort 8766 only; on a port conflict it refuses to start rather than falling back to a kernel-assigned port, because every existing registration names 8766 and a fallback would leave it pointing at whatever else listens there, which would receive the live bearer token.
- There is one token file, mcp-db-token.json, with no slug, because one instance exists per app process per KIRA_HOME (dbMcpTokenName in internal/bridge/dbmcp.go).
- The database MCP server's lifecycle is managed by bridge.DbMcpService: it is constructed and started when the Settings toggle turns on (or already is at boot) and stopped when the toggle turns off or the app quits.
- The ApprovalBroker is constructed once in apps/kira-studio/internal/appwire/wire.go and outlives the server's start/stop, so the boot-time event subscription stays valid across a restart.

### The six tools

- The database MCP server exposes six tools: list_connections, list_children, describe_table, describe_schema, run_query and explain_query, all registered via mcp.AddTool in buildMCPServer in server.go.
- The MCP server's metadata tools route through the existing internal/adapters layer, and run_query uses the same adapter query path as the console (adapterhost.Router.Execute); no second metadata or query path exists.
- adapterhost.Router.Execute is exported so that run_query can use the console's adapter query path.
- run_query passes its maxRows as the console cap, so the adapter stops reading at that row count; rowCount is the number of rows fetched and is only a lower bound on the total when truncated is true.
- list_children replaces the fixed-name list_databases/list_schemas pair named in the M1 row of docs/HISTORY.md (v1.7), because the adapter metadata primitive is one lazy level (Adapter.Children) and levels differ per kind, so one tool returning the node's own kind carries more information.
- list_children, describe_table and describe_schema are gated on read-mode deny, not only run_query.
- list_connections is deliberately not gated by read-mode deny, because its job is to show a caller a connection's configured permissions before it tries any of them.

### Kubernetes tools (P440)

- Six more tools register when `Config.Kube` and `Config.Ops` are set: kube_list_clusters, kube_list_namespaces, kube_list_kinds, kube_list_resources, kube_describe, kube_pod_logs. All read-only.
- Each cluster has four modes, `deny|prompt|allow`, default prompt: discover (namespaces, kinds), list, describe, logs. Stored on `kube_clusters` (migration 0051).
- No exposure switch. A cluster with all four modes deny is hidden and answers `no cluster %q exposed to MCP`.
- Prompt mode uses the shared ApprovalBroker (`KindDbMcp`) with `ApprovalRequest.Cluster`. The dialog shows a kubectl-equivalent statement. It is display and op command only, never run. The cluster is re-resolved after approval.
- Calls log as op kind `kubernetes` (connectionless), source MCP.
- Settings: cluster tree menu "MCP access" and Settings, Database MCP pane list.

### Permissions and statement classification (M2)

- Each connection has three independent permission modes (read, write, DDL), each allow, deny or prompt, defaulting to allow/prompt/deny (migration 0021).
- OpClass in internal/adapters/classify.go is one of read, write, ddl or unknown.
- Statement classification is exposed through an optional StatementClassifier interface rather than a required Adapter method, because only kinds with a console can answer it.
- Discover-style engines implement the optional `adapters.Discoverer` interface (`Histogram`, `Suggest`; adapters/discover.go), type-asserted by the Router like `StatementClassifier`. Only kibana implements it.
- ClassifySQL is shared by the five SQL kinds: postgres, mysql, mariadb, sqlite and clickhouse. Mongo and Redis reuse their own existing statement/command inspection.
- Kafka, S3 and SQS implement no classifier, so calls against them resolve to unknown with no error.
- run_query gate order (tools.go): resolve, refuse before connecting if all three modes deny, clamp maxRows, connect, classify, verdict (a deny refuses before any EXPLAIN work), forced EXPLAIN when mcp_auto_explain is on, heavy check, at most one approval, execute, render.
- A classifier error degrades to unknown and never bypasses the gate.
- An unknown classification resolves to the strictest of the three modes, rather than passing through them.
- Comment stripping in statement classification must be quote- and dollar-quote-aware, because otherwise a permission bypass reopens (fixed in commit accaadf8).
- Never strip MySQL/MariaDB /*! … */ executable-comment syntax as a real comment, because its content executes and stripping it bypasses classification (fixed in commit 16ea4f89).

### The approval flow

- The Database MCP server's ApprovalBroker in internal/dbmcp/approval.go is modeled on the FIFO queue of internal/pairing.Broker; its ApprovalTimeout and maxPendingApprovals reuse that broker's bounds.
- A client disconnect stops the approval wait via ctx, with no external expiry ticker.
- The approval UI is workbench/DbMcpApprovalDialog.vue, an always-mounted modal at App.vue's root, beside Studio's other always-mounted dialogs such as ConnectionDialog, DataGripImportDialog, ApiDialogs and ConfirmDialog.
- The shared packages/workbench/src/components/ConfirmDialog.vue cites DbMcpApprovalDialog's teleport-ordering precedent in its own comment.
- The dialog shows the statement in a read-only MonacoHost (ApprovalStatement.vue), formatted by formatConsoleText, with an "As sent" toggle for the verbatim text. Word wrap is always on, so no clause sits off screen.
- An approval prompt's Reason is either "permission" or "heavy". At most one prompt is raised per call when both apply; the stricter reason (permission) wins the label, and the plan evidence rides along.
- Every approval request carries a Target (ApprovalTarget): connection colour, read-only flag, host, configured database (SQLite: the file), and the decoded run_query/explain_query path. The dialog opens with ApprovalTarget.vue, a banner above the lede and statement, so the human sees which database the call acts on.
- A request has a Stage: "query" (may the statement run) or "response" (it ran; may its result reach the AI client). Both share one FIFO queue, one timeout and one dialog, and the title names the stage.
- Setting dbMcp.previewResponses (default off, Settings > Database MCP) raises a response-stage request after run_query executes. The handler goroutine holds the final result, so the AI client stays blocked, and only Send returns that same result value untouched.
- The preview runs after execution, never before approval, for three reasons: a pre-run would defeat the query gate, a heavy read would pay its cost twice, and a rollback transaction held across a human wait would keep database locks for up to 120 s (Redis, Kafka, SQS and S3 have no rollback at all). A write has therefore already run when its result shows; Withhold hides the result, it does not undo the change, and the dialog says so.
- Every exit after Execute is reviewed: success, empty, adapter error, masked error, masking refusal, internal Go error. Exits before Execute carry only app-authored text and are not.
- The held bytes are the TextContent of the result the client would get. Masking happens once, in render.go. The UI parses those bytes into a table (ApprovalResponse.vue) and shows them verbatim on the As sent tab. It never re-renders or re-masks.
- The text stays out of queue snapshots, which fan out to every window and can be MB-sized. Snapshots carry only isError and bytes; DbMcpService.PendingResponse(requestId) serves the text while the request is queued.
- Withhold, timeout, abandon and a full queue return an app-authored error with no row data. Approve first re-checks MCP exposure and compares the current mask set with the one used to render (mask.Set.Equal); a change withholds the result so a rule added during review cannot be bypassed.
- Each connection has mcp_review_results (migration 0045, P382): inherit (default), review or send. Inherit reads dbMcp.previewResponses fresh per call (bridge previewResponses); review and send skip that read. A read failure fails closed (review on), since sending unreviewed data is the unsafe direction. A stored value outside the three coerces to review on scan.
- go-sdk v1.8.0 (mcp/server.go toolForErr) wraps a regular handler error as an IsError text result carrying err.Error(); only a *jsonrpc.Error becomes a protocol error. A bare Go error from run_query therefore reaches the client as that text, and the preview shows the same text.
- Only run_query is reviewed, as the only tool that returns row data. explain_query, describe_* and list_* are unchanged. There is no undo for a withheld write and no editing of the result before sending.
- Both stages and heavy-read prompts raise the Studio window when it is not focused (P338), gated by dbMcp.focusOnApproval (default on, read fresh per open; a read error raises). A stray key landing after the raise can only deny: the dialog focuses Deny on mount and Escape denies.
- Two stages can hold one call for up to 4 minutes. A client with a shorter MCP tool timeout ends the call as abandoned and the result is discarded.

### EXPLAIN (M3)

- internal/queryplan is a Go port of the frontend EXPLAIN parser (plan.ts, planModel.ts, planIssues.ts, planParsers/*.ts), not a language bridge; every function is pure data transformation over the page each dialect's EXPLAIN returns, importing only internal/page and stdlib.
- The Go and frontend EXPLAIN parsers are pinned against the same fixture set, tests/fixtures/explain-plans/*.{input,expected}.json, read by both internal/queryplan/parse_test.go and tests/unit/explain-plan.spec.ts, so drift in either port fails on the same bytes.
- explain_query refuses anything but a leading SELECT/WITH (queryplan.Explainable) and refuses unsupported adapter kinds before connecting.
- queryplan.Supported accepts only postgres, mysql, mariadb, sqlite and clickhouse adapter kinds.
- The SELECT/WITH-only restriction on explain_query exists because on ClickHouse EXPLAIN can execute its own target on some forms, so letting DDL/DML through would make the path a write oracle.
- mcp_review_results is a first-class column, not an options_json key, for the 0022 reason: options round-trips through the connection URI, and a safety gate must not be settable by pasting one.
- Auto-force-explain uses its own mcp_auto_explain column (migration 0022), deliberately not the human-facing AutoExplain console flag, because one switch cannot mean both showing a warning strip after the fact and blocking an AI client on a modal a human must answer.
- The heaviness threshold for auto-force-explain is the existing advanced.expensiveQueryRows setting (default 100,000), read fresh per call via explainThreshold in bridge/dbmcp.go.

### Masking (M5)

- internal/mask defines six masking kinds: name, email, text, number, date, redact.
- The mask package has no id kind: the plan's draft defined one but it was dropped before implementation, because masking is opt-in per column and an internal id nobody marks sensitive is already untouched.
- Masking in internal/mask is grapheme-aware via github.com/rivo/uniseg.
- Stricter() resolves two masking rules conflicting on one column by folding them: the kind ranking and the KeepHint and Correlate flags each resolve independently toward their own stricter value.
- The correlation tag is a keyed HMAC-SHA256 over the real value's raw UTF-8 bytes alone, with no column or table identity mixed in, so different columns holding the same real value (e.g. customers.id and orders.customer_id) tag identically and joins survive. It suffices for cross-row and cross-join correlation but never to recover the value.
- The number mask kind never correlates, because a bucket is many-to-one.
- The MCP render path (internal/dbmcp/render.go) is the masking security boundary; the grid preview (views/grid/maskPreview.ts) is only a preview, not a control, since the user there already holds the real values.
- The Go mask implementation and the TS grid preview are kept in parity by 66 shared Go-generated fixture pairs under tests/fixtures/mask/, read by internal/mask/parity_test.go and tests/unit/mask-parity.spec.ts.
- A document/stream page is refused rather than silently skipped when masking rules exist.
- A masked column that is aliased or transformed is refused rather than returned unmasked (0176243e, re-opened and properly closed by 5ff00d57 for the duplicate-name case).
- A Redis key-value result is masked by real field name and refused otherwise, since some reply shapes carry no per-entry field name a rule could match (640e5580).
- Raw adapter error text is withheld on a masked connection, because a type-cast error routinely embeds the literal it failed on (1ad96285).

### The UI surfaces

- The sections array in state/settings.ts has eight entries: Appearance, Data, Cache, Api, Scripts, Claude Code, Database MCP, Advanced.
- 'Database MCP' is its own settings section and is the app's only embedded-MCP-server settings pane, since P97 removed the repo-map one.
- The Database MCP settings section holds the enable toggle, the "Review results before the AI gets them" toggle (dbMcp.previewResponses, an instant action that bypasses draft/Save), the "Bring Kira Studio forward for approvals" toggle (dbMcp.focusOnApproval, instant, default on), the registration command and Install button, the token-expiry line, and a read-only Exposed connections glance.
- The Exposed connections glance shows per-row read/write/DDL modes, auto-explain, and M5's masked-column count, and has no second editor.
- In project/ConnectionDialog.vue, DetailTab has five values: 'General', 'Advanced', 'Pre-connect', 'MCP', 'Privacy'.
- The MCP tab of the connection dialog also holds the result review override (inherit, always, never); the glance shows a line only for a connection that overrides it. The connection dialog is size 2xl so the tab fits its fixed height.
- All permission and description editing happens in the MCP tab of the connection dialog, and all mask-rule editing happens in the Privacy tab.
- The grid's two masking surfaces are the header menu's 'Mark column as PII' submenu (headerMenu in views/grid/menu.ts) and the toolbar preview toggle (toolbar-mask-preview in views/grid/DataToolbar.vue).

## App prompts (P271)

- internal/llmprompts holds every LLM prompt and LLM-facing instruction both apps send; internal/prompts is the P246 popup router, not this package.
- registry.go is the only file with prompt text; it has 139 entries (Space 115, Studio 34), accessed via typed handle Prompt[V] with Text(src, vars), and NoVars for entries without variables.
- User-authored text (smart-script bodies, workflow step prompts), tool and server names, tool error strings and result data formatting are kept out of the prompt registry.
- Prompt kinds are system, message, suffix, instructions, tool, argument (one MCP tool property description, with Parent tool and Field), toolResult, and fragment (included via {{template}}).
- InputSchema[T] sets each tool property's description from its argument entry, a mismatch panics at server build, and jsonschema tags are not used.
- Prompts use text/template, one set per resolve, with only the funcs inc and join; an entry with NoVars and no fragments is literal and never parsed.
- After an intended default prompt change, regenerate testdata/golden with go test ./internal/llmprompts -run TestGolden -update and review the diff.
- RenderParts wraps each string variable in marker runes and splits, so Settings Preview marks resolved values with VarText; a failed self-check returns one literal part.
- Prompt overrides are stored per app in <home>/llm-prompts.json (~/.kira-studio/, ~/.kira-space/), written with renameio at mode 0600, shaped {version, revision, overrides: {id: {text, defaultHash, updatedAt}}}.
- Saving the default text removes the override; override text must be 1 byte to 32 KiB.
- A prompt change carries baseHash, and a stale one is refused with E_CONFLICT.
- If the store file is corrupt or an override no longer validates, the default is used, a warning is logged, and it is flagged in Settings (storeError, invalid); defaultChanged flags an override made against an older default.
- Server-side validation requires that the template parses, every prompt:"required" variable is referenced, every required fragment is included, and it executes against example vars.
- The memory system prompts must include the memory.dataHygiene fragment, which is the injection defence.
- Schema-bound prompts (gate, reconcile, merge, rewrite, import extract, verify and finalize) keep their JSON schema fixed, shown read-only in Settings.
- There is no global prompt source; consumers get a llmprompts.Source (Resolve, Revision) through their options (memory.ServiceOptions, mcpserver.Options, agentmcp, scriptruns, ADE, dbmcp).
- Defaults() is the registry-only source, and Store.Resolve stats the file per call and re-reads on mtime or size change.
- Gate, reconcile, import, headless suffixes and ADE messages resolve per call, so an edit applies on the next call.
- memory-mcp (stdio subprocess) builds server instructions, tool and argument descriptions once per process, so edits apply only to new Claude sessions (newSession, shown in Settings).
- dbmcp and agentmcp rebuild their MCP server when Revision() changes.
- LlmPromptsService (List, Save, Preview) exists in both apps and emits event kira:llmPrompts:changed with {revision}; Save is all-or-nothing with per-id errors returned as an E_BAD_REQUEST payload.
- Preview with real vars also renders the ADE run dialog and step inspector, so no TypeScript copy of a prompt exists.
- Settings has a Prompts section before Advanced in both apps (packages/workbench/src/llmPrompts/) with list, Monaco editor, diff against default, Preview and per-entry reset; drafts live in Pinia store llmPromptDrafts, and dialog Save saves prompts first, then the settings patch.
- The guard go run ./internal/llmprompts/cmd/checkprompts (part of bun run lint) fails on a jsonschema tag, on an mcp.Tool/Prompt/PromptArgument Description, mcp.ServerOptions Instructions or memory.Call System not from llmprompts.
- The guard also fails a copy of a registry default (first 48 characters; whole text when 24 to 47) in a non-test .go, .ts or .vue file outside internal/llmprompts and internal/flowtest.
- The guard fails a default containing a filler word (e.g. please, just, simply, basically, actually, really, very, of course, certainly, kindly, in order to, make sure, note that, it is important, you should, feel free, →) outside template actions, backticks and double-quoted spans.
- The guard fails an entry whose rendered example exceeds its line in internal/llmprompts/testdata/budget.tsv; it is a ratchet, so a new prompt needs a line and a longer one a visible diff.
- Prompt defaults (P289) follow CLAUDE.md Communication style: fragments, imperative, no filler.
- When tersening prompt defaults, keep verbatim: memory.dataHygiene, the secrets clauses, the dbmcp.instructions permissions and masked-columns sentences, error strings, negations, numbers, template actions, and the fakeagent match keys.
- promptsizes (under go run ./internal/llmprompts/cmd/...) prints sizes; -snapshot f.json writes defaults and sizes; -compare f.json prints before/after per id and fails on a lost protected token, with allowed losses in internal/llmprompts/testdata/P289-runs/allowed-losses.tsv.
- Protected tokens checked by -compare are actions, numbers, quoted spans, identifiers, tool argument names and negations.
- The P289 terse rewrite reduced the registry from 126 entries' 30567 to 29507 characters (-3.5 %), -6.0 % over the 52 rewritten entries; memory texts were already at the P285 Part 2 floor.

## Validation proof (P420)

Agent-written evidence the user reviews in a dedicated window. Part 1 is storage, tools, bridge and window plumbing. Part 2 is the window UI.

- **Tools on kira-db.** `proof_start_session`, `proof_edit_session`, `proof_add_step`, `proof_close_session`, `proof_list_sessions` (`internal/dbmcp/proof_tools.go`). Logic in `internal/proof`, storage in `storage/repos/proof.go`, migration 0046.
- **Draft/revision model.** A session has `current_rev` and at most one `pending_rev`. Steps are added to a draft; close publishes it. An edit draft closes into the pending revision. User accepts (copies view state of unchanged steps, drops old revision) or rejects.
- **Provenance diff.** `keepOrdinal` marks `unchanged`, `replacesOrdinal` marks `changed`, neither marks `added`. Old steps no draft step references are `removed`. Computed at read time in `Service.Session`, never stored.
- **Retention.** Newest 30 closed sessions kept, pruned in the close transaction. `closed_at` is strictly monotonic (+1 ms bump on tie) so order is total. Delete frees the name. Names compare trimmed and lower-cased.
- **Limits.** 100 steps, 5 open drafts, 32 MiB per session, per-field caps in `proof/validate.go`. Open drafts older than 24 h purge at startup and on start. HTTP body capped at 8 MiB.
- **Payload precision.** The MCP SDK decodes arguments into a map, losing number precision and key order. `data` is read from the raw request JSON instead, so integers beyond 2^53 and key order survive.
- **Timestamps.** `*_at` columns are INTEGER unix ms (convention N7), not TEXT.
- **No op_log rows.** Proof tools read nothing from a connection and run no statement; the agent supplies the evidence. Nothing to audit as a data access.
- **Prompt.** Close raises a `proof` prompt (`prompts.KindProof`, ref = session id). Startup re-raises it for closed sessions never seen or with a pending edit.
- **Window.** Ephemeral windows keyed `proof-*` (`proof.IsWindowKey`). Their `windows` rows are purged at startup, so a restart never restores one. `ProofService.OpenWindow` raises the live one or opens a new one; `WindowTarget` tells it which session. `kira:proof:changed` (app-wide) and `kira:proof:select` (to one window) are the events.
- **Studio DB.** `PRAGMA foreign_keys` is ON per connection (DSN `_foreign_keys=1`, `internal/sqlitex`) and `SetMaxOpenConns(1)`. Proof cascades rely on both. Multi-row changes run in one transaction with no query issued meanwhile, which the single connection would deadlock on.
- **Boot audit for the proof window.** `frontend/src/main.ts` boots layout, connections, tabs, terminals, DB MCP, ops and schema sync stores. The proof window needs only i18n, Pinia, Vue Query, settings hydration (theme) and `windowsEnsure`. The window boots through `src/proof/boot.ts` (`windowKey.startsWith('proof-')` in `main.ts`).
- **Window UI.** `frontend/src/proof/`. Three panes like the Kira Space review window: history list, tab strip plus step body, and a change panel while an edit is pending. Pane widths persist (`kira.proof.leftWidth`, `kira.proof.rightWidth`). Server state is TanStack Query (`['proof', ...]` keys); `kira:proof:changed` invalidates all but view state. One Pinia store (`useProofWindowStore`) holds the open session, side (current/proposed/compare) and active tab.
- **Result views reuse the console.** Each step builds a page with the console page builders (`pages.ts`) and renders `ConsoleResultGrid`; the query shows in a read-only `MonacoHost`. No separate renderer.
- **View state.** Per step and revision: query scroll, result scroll, highlighted cell or row, expanded document ids, column widths. `ConsoleResultGrid` takes a `viewState` prop and emits `update:viewState`; `useStepViewState` seeds from the backend once, mirrors into the query cache on each change, and writes the backend after `TIMING.scrollPersist` or on unmount.
- **Chart view (P426).** View `chart`, `data.type` bar, line, area, scatter or pie; a histogram is a bar chart and a time series a line or area chart by axis type. `proof/chart.go` validates and normalizes: time x becomes epoch ms, line and area points are sorted, every field is present, number text is kept. Caps: 20 series, 1000 categories, 50 slices, 20000 points for line and area, 1000 for bar and scatter. Each bar and scatter point is one SVG node and the screenshot capture cost grows with node count. Migration 0048 rebuilds `proof_steps` and `proof_view_state` to widen the view CHECK, children first, with foreign keys on.
- **Chart rendering.** `ProofChart.vue` and `chartOption.ts` over `charts/echarts.ts`: SVG renderer, animation off. Selection, hidden series and zoom are built into the option, so the first `finished` event is the ready signal. The tooltip is the one HTML sink; every agent string passes `format.encodeHTML`. Line and area charts over 1000 points draw no symbols and have no point selection. Legend entries are mouse-only: ECharts has no keyboard legend.
- **Chart view state.** `chart {hidden, selected, zoom}` is optional on `ViewState` and omitted for other views. Bounds are checked in `proof/viewstate.go`.
- **Chart screenshot.** Inline SVG clones with the DOM; the capture filter drops `.kira-proof-chart-tooltip`. Measured in headless WebKit: 1000 scatter points 5.7-6.3 s, one line series of 1000 points with symbols 6.8 s, 20 series of 1000 points 3.5-4.1 s. 2000 scatter points took 10.6-12.6 s, so the bar and scatter cap is 1000.
- **Screenshot.** `modern-screenshot` (`screenshot.ts`) clones the step element to a canvas; `restoreScrollPosition` makes the clone match the live scroll. `includeStyleProperties` must list painted properties only; without it a clone of a 160-row grid takes minutes in WebKit. Capture waits for every `[data-step-key]` to report `data-ready`. Default action copies a PNG to the clipboard; the dropdown saves via `ProofService.SavePng`. All tabs stack in order.

## Config files (P450)

Config lives as plain files in app-home folders; `internal/configfs` is the one foundation every module uses. Terse facts, decisions in `docs/v0.2.26/plans/P450.md`.

- Layout: a config folder is a direct child of an app home, `<home>/<module>/`. Workflows stay `<home>/workflows/`. A module's folder lives in the home of the app whose DB holds its state; a process projects files only into its own DB.
- A definition shared by both apps has one folder, `~/.kira-space/agent/`. Both apps read, write and watch it. `agentctx` is the first user (P452); `kirapaths.SharedAgentDir()` is the resolver.
- Reserved names: `AGENTS.md` (generated) and hidden files. The store never reads either as config. Folder mode 0755, file mode 0644; files never hold a secret value.
- Per-machine values: `<name>.local.yaml` beside `<name>.yaml`, merged over it key by key. Code lands with the first module that has one (P461).
- Formats: structured config is YAML (`.yaml`), prose is Markdown (`.md`). No JSON config files. Anchors, aliases and merge keys are refused with a line. Collections are one item per file, `<id>.yaml`, stem is the id. Singletons are one fixed file name. Cap 1 MiB, refused on read and write.
- Validation: JSON Schema draft 2020-12 per kind, `github.com/santhosh-tekuri/jsonschema/v6`. `google/jsonschema-go` declined: its error names no instance location, so no file line. Pipeline: `yaml.Node` parse, node-to-JSON walk, schema validate, error mapped back to line and column, decode into the Go type (unknown fields refused), module `Rule`s. `Rule.Doc` is mandatory and renders into `AGENTS.md`.
- A `Problem` message never echoes the offending value. Messages are English catalog keys `backend.core.config.*`.
- Workflows keep their hand-written `adeflow.Parse` and run on `configfs.Folder` without a schema.
- DB sync, one mechanism: files to DB only. Each module keeps a projection (last valid parsed JSON per file) in its own SQLite table through `configfs.Projection`. The projection exists so last good survives restart, state rows keep a local key, and hot paths read rows. Workflows' projection is `ade_workflow_last_valid`; rows follow file deletion.
- App writes never touch the projection. A UI edit writes the file (atomic, `baseHash` check), then the same load path projects it. Load: `Folder.List` at start, then on every debounced (200 ms) watch event. Unchanged JSON is skipped. Invalid file: no DB change, a `Problem` with the last good value, one Warn per new hash (file name and line, never content). File gone: `Forget`.
- State references config only by id (file stem). A rename is a delete plus a create. Each module states whether a delete cascades or orphans its state.
- Migration of legacy rows (P459 to P462): one-shot, export each row to `<id>.yaml`, then set a marker setting row; never export again once set. Legacy secret values are never exported: they move into the secret store and the file gets a reference.
- Config versus state: workflow files are config; `ade_workflow_last_valid` is a derived projection; task workflow snapshots are state; `AGENTS.md` is derived. The `settings` table stays in SQLite this chapter.
- References: `${NAME}` reads an env var, `${secret:name}` an app secret, `$${` is a literal `${`. Anything else inside `${...}` is a load-time `Problem`. Only schema fields marked `x-kira-ref` are interpolated; `x-kira-secret` requires the whole value to be one secret reference. Shell command fields are never marked.
- Load keeps references verbatim in the decoded value and the projection. `Resolver.Resolve` runs at use: it returns the text plus `[]Used{Name, Value, Placeholder}` for secrets, for the caller's masker. A missing env var or secret fails naming the reference, never an empty string. Env source is injected.
- Secret seam: `configfs.SecretSource` (`Secret(ctx, namespace, name)`, errors wrap `ErrSecretMissing`). The namespace is the owning module's, never written in a file. P450 wires no production source; P453 adapts the secret store.
- `AGENTS.md` is generated per folder: `DocFromSchema` builds the options list from the schema, rule docs are listed, the intro is a per-module template with prose only. `Folder.WriteDocs` writes it at app start when bytes differ.
- Workflows (P451) build their model from the `adeflow` option table, not a schema: `adeflow.DocModel` adds per-agent settings from `providers.StageCatalog` and one doc line per cross-field rule (`TestRuleDocsEnforced` binds each rule to the validator). `DocModel.LocalOverrides` is opt-in, so the `.local.yaml` line prints only for a module that reads it. `ade.TaskBoard.Start` writes the file at Space start, non-fatal; a user edit is overwritten.
- UI: plain file edit in the user's editor is the default; a module may add small edit actions through `Folder.Save`. `packages/workbench/src/config/ConfigFileBar.vue` shows path, status (valid or "Line N: message"), Open, Reveal and Copy path; the app passes its bound calls in.
- `configsvc.Service` (`internal/configfs/configsvc`) is the bound Open/Reveal surface. It takes a registered folder name plus a visible file in it; no renderer path reaches the OS. Kira Space registers `workflows`. The OS seam is `shell.NewDeferredFileOpener` (Wails `Browser.OpenFile`, `Env.OpenFileManager`).

### Agent context (P452)

- File `~/.kira-space/agent/context.md`, plain Markdown, cap 32 KiB (`agentctx.MaxBytes`). A blank or missing file means no context.
- `agentctx.Store` reads it per launch. Invalid file (over cap, bad UTF-8) keeps the last good text, projected in `config_last_valid` (shared `configfs.TableProjection`, both apps). No last good text means no context.
- `ContextFile()` writes an immutable snapshot `<snapshots>/<hash16>/AGENTS.md` (0600), touches its mtime on reuse, prunes dirs older than 30 days. A running agent never sees an edit; its path never changes under it.
- Launch sites that set `ContextFile`: `ade.Tracker.Prepare`, `TaskBoard.superviseAgent`, scriptruns smart run, `ResumeCommand`. `superviseAgent` resolves it before `prepareSandbox` so the sandbox knows `ContextDir` (plan said after `Register`). A resolve error fails the launch.
- Which launches see an edit: new conversations and resumes both pass the current file. Claude's snapshot keeps the recorded prompt on resume until compaction, so a resumed Claude conversation shows the old context. The editor says so per provider (`keepsOnResume`).
- Space hosts and edits the file (`AgentContextService` Get and Save, Context tab in Agents, `ConfigFilesService` folder `agent`, generated `AGENTS.md`). Studio only reads it from the same path. Spec says Studio owns it; Space hosts the folder because the editor lives in Space. Studio has no editor.
- Save checks `baseHash` against the file in `Store.Save` itself: `Folder.Save` skips the check for an empty hash, so a create over a concurrent create needs it.
- Structured calls (memory distill, import) get no context. Their prompts are app prompts; user text stays out of `internal/llmprompts`.

### Agent settings (P453)

- File `~/.kira-space/agent/settings.yaml`, cap 256 KiB (`agentcfg.MaxBytes`). One schema assembled at start: common keys `env`, `permissions` (`allow`, `deny`), `mcp_servers` (stdio `command`, `args`, `env`; http `url`, `headers`; `enabled`), `hooks` (name to `event`, `matcher`, `command`, `timeout`) plus one section per `SettingsHost` provider under its id (`claude`: `model`, `permission_mode`, `status_line`, `hooks`, `skip_hooks`; `opencode`: `model` as `provider/model`, `plugins` as `file://` URLs, `skip_hooks`). Extra keys are refused with a line.
- Rules (`agentcfg/rules.go`): a server has a command or a url, never both; transport fields match the transport; names starting `kira` and env starting `KIRA_` are reserved; `matcher` only on `tool_start` and `tool_end`; `skip_hooks` names a common hook.
- Hooks: Kira's own hooks always run and never appear in the file. User hooks run in addition; a provider's `skip_hooks` drops a common hook for that provider.
- References: only `x-kira-ref` fields (`env` values, server `url`, `command`, `args`, `headers`, `env`) take `${NAME}` and `${secret:name}`. The Claude section has no ref field, so no secret reaches `settings.json`. Env comes from one login-shell probe (`loginshell.ProbeEnv`, `-l -i -c`, 5 s, only the names asked for), then the process env. A missing env var or secret fails the launch naming the reference.
- Secrets: `~/.kira-space/agent-secrets.db`, one file both apps open (`kirapaths.SharedAgentSecrets`, `agentcfg.OpenSecrets`, `sqlitex.OpenImmediate`, own migration). Table `secret_values`, scope `agent-credential`, owner `agent`, sealed by the shared cipher. Set from Agents > Settings; write-only, no call returns a value. Storage off: `E_SECRET_STORE` on save.
- Which launches get settings: every Space TUI (`prepareWithGrant`), Continue in terminal in both apps (`ResumeCommand`), and non-isolated ADE agent steps and review rounds (headless). Not isolated runs (smart scripts in both apps, ADE smart steps, rebase runs): their tool and MCP policy is explicit. Not structured calls (memory).
- Preparation (`Store.Prepare`): last good settings, resolve, `RenderSettings`, write each file as `<dir>/<stem>-app.<role>.json` (0600, dir 0700) in the ADE agent dir (`ade/runs`) or `automations-mcp`. `Prepared.Release` removes them: TUI with the grant release at record stop or intent expiry; headless after the run; Continue in terminal with the token (`BeginAgent` binds it to the terminal, `Exited` releases, token expiry and `Service.Close` sweep the rest); `Store.Close` at quit; `Store.Sweep` at start for a crash.
- OpenCode (`opencode/settings.go`): `RenderSettings` returns one inline `NativeFile` (`opencode config`), the app part of `OPENCODE_CONFIG_CONTENT`: `model` (only when the section has one), `mcp` (remote `type`/`url`/`headers`, local `command` array/`environment`), `permission` (allow then deny through the tool table, last match wins), `plugin`. Interactive merges it with Kira's `instructions`; headless renders deny only and merges into the permission document (app deny joins Kira's, deny wins; allow never widens `Tools`), and an isolated run refuses app settings. Untranslatable deny fails the launch naming the entry; untranslatable allow and each common hook are view notes. The content text is substituted by OpenCode (Q-OC7), so a value with `{env:` or `{file:` fails rendering. Cap 64 KiB. Layers merge, not replace: `plugin` with the hooks `OPENCODE_CONFIG_DIR` (Q-OC5), `mcp` with the grant `OPENCODE_CONFIG` file (Q-OC6), `permission` with `OPENCODE_PERMISSION` (Q-OC8); local `environment` reaches the server (Q-OC9). No file holds a secret. The view shows the document with references verbatim and a note that no file is written. Tests: flow `TestAgentSettingsOpenCode`.
- Masking: the ADE log sink masks resolved secret values in headless step output (`••••`). A live terminal stays unmasked.
- Sandbox: the profile allows reading `<ConfigDir>/<run>-app.<role>.json` (regex `-([0-9]+\.mcp|app\.[a-z]+)\.json$`); the dead `user` alternative is gone.
- Statusline: a user status line comes only from `claude.status_line`. Kira wraps it when usage limits are on (`KIRA_USER_STATUSLINE`); with no app settings the wrapper stands alone. `userStatusLine` no longer reads any Claude settings file.
- Service (`agentcfg.Service`, bound in Space as `AgentSettingsService`): `Get`, `Save(text, baseHash)`, `SetSecret`, `DeleteSecret`. `View` carries the file state, `storage`, `secrets` (name, set, referenced) and per provider the resulting config rendered with references verbatim, notes (unset secrets, the headless line) and what Kira adds. Event `kira:agentSettings:changed`. Studio has no binding, watch or docs: it opens the same store and reads the file at launch.
- Config versus state: `settings.yaml` is config; `config_last_valid` (`agent`, `settings.yaml`) is a per-app projection with references verbatim; secret values are state, sealed; run files are derived and removed at exit; `AGENTS.md` is derived and written by Space (`Store.WriteDocs` with the context model as a second section).
- Space hosts the editor and Studio reads, as for the context (the spec names Studio as owner). Tests: flow `TestAgentSettingsReachEveryLaunch` (Space) and `TestAgentSettingsStudio`, UI `ade-agent-settings.spec.ts`, real claude `TestAgentSettingsClaude`.

### Agent config import and export (P454)

- Package `internal/agentport`, bound in Space as `AgentPortService` (`List`, `ImportPreview`, `ImportApply`, `ExportPreview`, `ExportApply`, `Cancel`; `Close` stays unbound). Agents > Settings shows "Import from" and "Export to" links under each provider tab that `List` names (Claude, OpenCode). Dialogs are local component state: `AdePortImportDialog`, `AdePortExportDialog`, TanStack Query mutations.
- Files, all user-level, never project-level. Claude: `~/.claude/settings.json`, `~/.claude/CLAUDE.md`, `mcpServers` of `~/.claude.json`. OpenCode: `opencode.json`, `.jsonc` or `config.json` and `AGENTS.md` under `$XDG_CONFIG_HOME/opencode` (else `~/.config/opencode`). A file past 4 MB is not read. A symlink is resolved and kept.
- Only a click opens them: a preview reads, nothing else does. Decode is by mapped key (`topLevel` streams the top-level keys and decodes only those); `~/.claude.json` yields `mcpServers` only. Login keys (`apiKeyHelper`, `awsAuthRefresh`, `awsCredentialExport`, `otelHeadersHelper`, `forceLoginMethod`) are not listed. Other unmapped keys (`provider` in OpenCode) show by name as skipped and are never decoded.
- Item model: id (`mcp/<name>`, `env/<NAME>`, `allow/<rule>`, `deny/<rule>`, `claude/model`, `hook/...`, `context`, `skip/<key>`), kind, state (`new`, `same`, `conflict`, `skipped`), snippet, current text, secret slots. A conflict is unchecked by default. Context has modes `replace` and `append`.
- Credentials (`classify.go`): a literal is a credential by location (`env`, `headers`, server env), name (`token`, `key`, `secret`, `password`, `authorization`), value shape (`sk-`, `ghp_`, JWT, long hex) or flag adjacency (`--api-key x`). A `Bearer` or `Basic` scheme splits off and only the value is secret. A secret becomes `${secret:<server>-<field>}` in `settings.yaml` and a slot with a name, field and `reuse` (an equal saved value is reused). Names are sanitized and de-duplicated with `-2`, `-3`. Slot values go straight to the sealed store at apply; no preview, result or error carries one. `classify_test.go` is the only unit test.
- Writes to Kira's files: `settings.yaml` through yaml.v3 node edits (comments and order kept; `agentcfg.Store.Check` validates before saving) with the settings `baseHash`; `context.md` through the context store.
- Export: settings merge into the user's file by key and leave other keys alone. JSON files go through `tailscale/hujson` and RFC 6902 patch, so JSONC comments and layout survive; a file that was not canonical JSON is not reformatted. Claude refs: `${NAME}` stays, `${secret:x}` becomes `${KIRA_SECRET_<X>}`; OpenCode uses `{env:NAME}`. `envNames` lists the names the user must set (copy lines carry no values). A credential literal in Kira's file blocks that item. An existing OpenCode deny rule is kept when Kira would only widen it. Hooks: Claude only (OpenCode runs no hook commands).
- Claude MCP export goes through the CLI, never a file write: `claude mcp remove` then `add-json -s user` (`harness.JSONInstaller.InstallJSON`, shared with `mcpinstall`), with `CLAUDE_CONFIG_DIR` when set. `~/.claude.json` is backed up before the first call. OpenCode servers are part of its config file.
- Safety: a preview is an in-memory session (128-bit token, 10 min, single use). Apply compares a content hash per file; a file changed since the preview is `E_CONFLICT` and nothing is written. Each overwritten file is first copied to `<file>.kira-backup-<UTC 20060102T150405Z>` beside it. Writes are atomic (renameio), parent dirs 0700. The diff masks credential-looking strings (`••••`). OpenCode hashes ignore a top-level `$schema`.
- Config versus state: the user's files are theirs, read and written only on a click; backups are state beside them; sessions are memory.
- Tests: flows `TestAgentPortClaude` and `TestAgentPortOpenCode` (contracts `agent-port-import.json`, `agent-port-export.json`), UI `ade-agent-port.spec.ts`, real claude `TestAgentPortClaude` (no model call). The fake claude plays `mcp add-json` and `mcp remove` on `$HOME/.claude.json`.
- OpenCode 1.19.5 facts (probed, OC1 to OC8) live as comments in `harness/opencode/facts.go`.

## CLI credentials: never touched

Kira drives gh, az and acli but never touches their credentials. It runs the CLI and reads its output (JSON or text). No token commands, no reads of CLI config or credential files, no auth headers on Kira's own HTTP.

- gh: runner passes os.Environ through plus hygiene vars; never reads GH_TOKEN or GITHUB_TOKEN. `gh auth status --hostname` is parsed for the account name only.
- az: env passthrough. `az account show --query user.name` only.
- acli: env passthrough. `jira auth status` parsed for site and account only. Comment body goes through a 0600 temp file.
- claude: the MCP picker runs `claude mcp list` and keeps name, transport, host or command name and health status only. With hermetic off, a run that ticks a user server lets claude load it (`--setting-sources user`, `--settings` with `allowedMcpServers` and `disableAllHooks`). Kira never runs `claude mcp get`, which prints stored headers and env. Outside the user-initiated exception below Kira never opens `~/.claude.json`.
- opencode: the MCP picker runs `opencode mcp list` and keeps name, transport, host or command name and status only. Kira never opens OpenCode's `auth.json`, never runs `opencode auth` or `providers`, and never sets `OPENCODE_AUTH_CONTENT`. Session ids come from `opencode session list --format json`, not from its database. Outside the exception below Kira never opens `opencode.json`.
- Exception, user-initiated (P454): only the package `internal/agentport` opens `~/.claude/settings.json`, `~/.claude.json`, and OpenCode's `opencode.json`, and only on an Import or Export click. It decodes mapped keys only (`~/.claude.json` only `mcpServers`; never `oauthAccount`, `apiKeyHelper`, OpenCode `provider` or any other key's value), a literal credential goes straight into the sealed store and never into a log, preview field, error or file of Kira's, and it never runs `claude mcp get`, `opencode auth` or `providers`. Claude MCP export goes through `claude mcp remove` and `add-json -s user`, not a file write. Launch paths still never open those files (`TestRealClaudeSettingsUntouched`).
- Setup steps are copy-only commands the user runs in their own terminal.
- Consequence: comments carry no attachments (no driven CLI uploads one; Azure would need a bearer token).
- Guard G5 (internal/conventiontest) fails on token commands, credential env names, CLI config paths, auth headers, net/http and os.Getenv under githost/ and issuetracker/, `LoadX509KeyPair`, `X509KeyPair`, `AppendCertsFromPEM` and `config.json` literals under internal/docker, and under internal/harness/claude a `.claude.json` literal, a `json:"headers"` struct tag, a `claude mcp get` argv, `Header.Set`/`Header.Add` calls and `RoundTrip` methods. `sandbox.go` is exempt: it names `~/.claude.json` as a sandbox write path and never opens it. Under internal/harness/opencode the guard fails on the literals `auth.json`, `opencode.json`, `.local/share/opencode`, `OPENCODE_AUTH_CONTENT`, an `auth` or `providers` argv, `Header.Set`/`Header.Add` and `RoundTrip` methods, and on `os.ReadFile`. `sandbox.go` only names directories; `headless.go` is exempt from the `os.ReadFile` check because it reads only the MCP configs Kira wrote itself; `hooks/server.go` is exempt because it writes Kira's own `opencode.json` into a private temp dir. Under internal/agentport the guard fails on the literals `.credentials.json`, `auth.json`, `.local/share/opencode` and `OPENCODE_AUTH_CONTENT`, a `claude mcp get` or `opencode auth`/`providers` argv, header writes and an `apiKeyHelper` run.
- Non-secret reads are allowed: docker context `meta.json` (name, host, SkipTLSVerify) and stat of PEM paths. Never a CLI config file that can hold credentials (`~/.docker/config.json`): ask the CLI (`docker context show`). Key material is opened only by the owning library (`go-connections/tlsconfig`).

## Renderer security surface

**This section is much shorter than it was, and that is the finding, not an omission.** Most of what
P46 hardened were default-on Electron/Chromium capabilities. Under Wails the webview is a native
WKWebView, not a bundled Chromium, so most of those switches have **no subject at all** —
nothing to turn off, since the thing was never on. A smaller number have **no analogue**:
Wails exposes no equivalent, and the guarantee is genuinely weaker than it was. Those are listed as
losses below rather than papered over.

Repo-root `internal/shell/security.go` is the one module owning what remains. `Harden()` returns the
posture, and `window.go`'s `Options` is its single caller. It does four things: deny every
permission except clipboard reads, set `JavaScriptCanOpenWindowsAutomatically` false, leave
`EnableFileDrop` false, and leave `OpenInspectorOnStartup` false.

| P46 control | Status under Wails |
|---|---|
| `contextIsolation` / `sandbox` / `nodeIntegration: false` | **No subject, strictly better.** There is no Node in the webview to isolate it from, and no `contextBridge`/`window.kira` surface at all — the renderer reaches Go only through generated bindings and the `engine` stream. |
| DevTools in a packaged build | **Ports, by a different mechanism.** It is a Go build tag rather than a runtime option: `-tags production`, already set by `apps/kira-studio/build/darwin/Taskfile.yml`. |
| Every Chromium permission except the clipboard | **Set; on macOS and Linux only camera and microphone are enforced.** `WebviewWindowOptions.Permissions` exists and is populated (microphone/camera/geolocation/notifications denied, `PermissionClipboardRead` allowed). Wails v3.1.0's `permissions_darwin.go` consults it for `getUserMedia` only (`resolveMediaCapturePermission`: a denied microphone or camera is refused instead of prompting); geolocation, notifications and clipboard are not consulted there (`permissions_linux.go` likewise covers media capture only). On macOS the real clipboard answer is WebKit's own user-gesture heuristics. |
| `window.open` deny | **Partial.** `MacWebviewPreferences.JavaScriptCanOpenWindowsAutomatically` is false, which denies JS-initiated windows; there is no per-request handler (no `WKUIDelegate createWebViewWithConfiguration:`). Still zero `window.open`, zero `target="_blank"` and zero `<a href>` in `apps/kira-studio/frontend/src`/`packages/shared`, and file pickers remain native dialogs via `FilesService`, not popups. |
| Navigation lock to the base URL | **No analogue — a real loss, already known.** There is no navigation-policy delegate on darwin at all (`webview_window_darwin.m` has no `decidePolicy`). This is weaker than the Electron `will-frame-navigate` guard plus fuses it replaces, and is recorded as a loss rather than mitigated. |
| `webviewTag: false` | **No subject.** |
| The spellchecker | **No analogue in the shell** — Wails exposes no spellcheck control. The mitigation moved into the renderer instead: `spellcheck="false"` on the field itself. The reason is unchanged: the connection dialog's password field becomes plain `type="text"` once the eye toggle reveals it. |
| WebGL off | **No subject.** |
| The seven `disable-*` Chromium switches | **No subject.** |
| `grantFileProtocolExtraPrivileges` | **No subject, and the whole class with it.** Assets are no longer served over `file://` — `apps/kira-studio/main.go` embeds `frontend/dist` and serves it through a plain Go `http.Handler` (`AssetOptions.Handler`), so the `file://`-module-CORS trap that made this fuse mandatory does not exist. |
| The three Electron fuses (`runAsNode` and friends) | **No subject.** There is no Electron binary to re-run as Node. |

**As of P100, the next paragraph describes Kira Space's renderer, not Kira Studio's** —
`packages/git-ui` moved there in full, and `apps/kira-studio/frontend` no longer depends on it at
all. As of P120, `LinkService`/`link.go` was Kira Space's own; Kira Studio's copy had no live
caller and was removed. P260 rebinds a Studio `LinkService` (`apps/kira-studio/internal/bridge/link.go`)
for the Docker registry button, called from `apps/kira-studio/frontend/src/docker/context.ts`. The
`linkOpenExternal` wrapper lives in the shared `packages/workbench/src/bridge/createCoreControl.ts`;
the git UI is its only caller (`repo/git/hostHandlers.ts:433`), binding Space's `bridge/link.go`.

**The `<a href>`/`window.open`/`target="_blank"` posture now extends to `packages/git-ui` too
(P74 §3, P79 batch B).** That package used to be outside the `window.open` deny row's own scope.
Every externally-openable link in the git UI is a `<button>` calling a host capability, never an
anchor: the PR surfaces (`CommitMeta.vue`, `RefsPopover.vue`/`StackList.vue` stack badges,
`refBadges.ts`'s SlickGrid formatter) go through `changeRequest.browserUrl`/`GitHostService.OpenChangeRequestURL`,
and a URL found inside a commit message body (`linkify.ts`) goes through a new generic
`link.openExternal`. Both are gated on `capabilities.openExternal`, and a surface renders a
plain `<span>` when the host does not report it (`CommitMeta.vue`'s own button/span split — P79
batch F closed the one PR icon that was missing the gate). `link.openExternal` is generic where
`changeRequest.openExternal` is not, per `internal/bridge/link.go`'s own reasoning: a change request URL is composed
server-side from data the app controls, while a commit-body URL **is** the untrusted content,
already visible to the renderer as linkified text — so the only check left is the URL's own shape.
`LinkService.OpenExternal` refuses anything that is not a well-formed `http`/`https` URL with a
non-empty host, before `Browser.OpenURL` ever sees it, and the git-ui host handlers do the
equivalent shape validation. The check for a change request URL is
`Registry.IsChangeRequestURL`: some registered provider accepts it (github: `github.com` plus any host `gh`'s own `Discovery` has authenticated
against) — shared by the composer and the validator, not two hand-rolled allowlists (P79 batch B; the
previous hardcoded literal broke GitHub Enterprise Server entirely, since the validator's allowlist
was not really an allowlist for a GHES host).

**Whether the `WKWebView` itself is configurable beyond this table — checked, and closed, at P22
(D5).** `MacWebviewPreferences` (`webview_window_options.go:773-796`) is Wails v3.1.0's
entire macOS webview surface — ten fields, unchanged since beta.15 — and none is a compositing or
tiling knob; the `WKWebViewConfiguration` is built and `autorelease`d inside one cgo block
(`webview_window_darwin.go:139-197`) with no hook of any kind between "config allocated" and
"webview created" (the whole options file has exactly one func-typed field, `KeyBindings`).
`WindowsOptions` carries `EnabledFeatures`/`DisabledFeatures`/`AdditionalBrowserArgs` for WebView2;
`MacOptions` has only `ActivationPolicy` and terminate-on-last-window-closed — macOS is the one
platform with no engine-flags escape hatch. `NativeWindow()` reaches Wails' private ObjC
`WebviewWindow` subclass, whose `webView` ivar is castable, but there is nothing to set once you
have it: the coverage-rect/tile-pooling behaviour `git show 7d8a72ab4a5e:docs/WEBVIEW-SCROLL-MEMORY.md` §7 describes lives
in WebCore, below anything `WKWebView`/`WKWebViewConfiguration`/`WKPreferences` expose publicly.
Full citations: `docs/HISTORY.md (v1.1 P22)` §3 F8–F11. **A bounded SPI-header
grep on a real Mac (that plan's §6.2 C6) is still pending** — no macOS SDK is reachable from this
sandbox — and its result, whichever way it goes, does not change this disposition (F8–F10 close the
Wails half on their own). Re-check after any Wails version bump; P19's posture means this repo keeps
moving through betas.

**Autofill** is unchanged and still a renderer-side control: zero `<form>` elements means the engine
has no form owner to attach autofill heuristics to regardless, and the one free-text input that
actually sets the attribute — `packages/workbench/src/components/AutocompleteField.vue` (`TextField.vue`'s own
replacement, P104) — still opts out explicitly (`autocomplete: 'off'`) rather than relying on that.

**The clipboard allowlist is an allowlist, not a deny-all, because a deny-all breaks the app.**
Denying clipboard reads throws at `packages/workbench/src/behaviour/clipboard.ts`'s `copyText` (23 files
import it, plus the grid's own paste path). This reasoning still governs the `Permissions` map even
though that map is inert on macOS — it is the correct value, and the value that actually applies on
Linux.

**Deliberately left alone, each a decision rather than an oversight:**
- **Hardware acceleration** stays on. The grid's scroll budget (see Invariants, above, and
  `docs/PERF.md` §1) depends on GPU compositing.
- **`window.confirm()` is gone.** `packages/workbench/src/state/confirmDialog.ts` ("Replaces
  `window.confirm()` for every destructive action") plus `ConfirmDialog.vue` now gate every
  destructive action (deleting a key, an S3 object, a message, a document, a connection) through
  the app's own in-page confirm store instead.

**What actually holds this**, so a revert is never silent: repo-root `internal/shell/security_test.go`
(the posture value) and `menu_test.go` (`TestBuildMenuAcceleratorsAllParse`, against the shared
`menutemplate.go` vocabulary — each app's own real menu content is `internal/appshell/menu.go` now,
P103 Part 3) — the successors to the deleted `tests/unit/security.spec.ts`/`menu.spec.ts`, whose
subjects moved to Go. No
Wails analogue of the old `tests/e2e/hardening.spec.ts` exists, and none was written: with the table
above reduced to "no subject" for most rows, nothing is left for such a spec to assert that
the Go test does not already cover. The macOS-only behaviours — WebKit's clipboard gesture
heuristics, and the absent navigation delegate — have no automated coverage on any platform this
repo's CI runs.

## Testing

- Studio test suites under apps/kira-studio/tests/ are unit/, ipc/, ui/, e2e-real/, visual/, plus opt-in perf/ and proto/ (playwright.proto.config.ts); fixtures/ and support/ are not suites.
- The ipc/ directory holds two suites, a Go backend half and a Playwright frontend half per adapter, sharing one fixture module by design (P50).
- Kira Space is a separate app with its own test tree (unit/, ui/, mobile/, e2e-real/, visual/, perf/, plus the Go flow tier); it has no ipc/ tier because it has no database adapters to split against.
- Kira Space's tests/unit/ and tests/ui/ (bun run test:ui:space) run against Space's own built bundle; its internal/ Go suite is covered by bun run test:go, which runs both apps' packages together.
- Studio flow tier (P232): apps/kira-studio/internal/appwire is the composition root, and internal/flowharness boots it in-process on a temp home with real HTTP and gRPC servers, a real Docker daemon and real PTYs.
- Studio flow Docker tests skip with a message when no Docker daemon is available; KIRA_FLOW_DOCKER=require makes them fail instead.
- Studio flow tests live in internal/flows/{apiflow,httpflow,grpcflow,dockerflow,termflow}.
- The flow gRPC server runs unary methods through its interceptor, so Calls() and required metadata cover unary and streams.
- createTerminalsStore (shared by both apps and the Docker exec store) keeps one terminalWrite in flight per tab and coalesces chunks typed meanwhile (state/orderedWrites.ts), because concurrent bound calls reach the backend in any order.
- Space flow tier: apps/kira-space/internal/appwire is the composition root; Build(Options) wires repos, git, terminal, ADE, memory, mobile and bound services; main.go supplies native pieces and tests supply fakes.
- Space flow harness uses real SQLite, git (fixed dates, git fast-import history), git stream, PTY, watchers and mobile HTTP; it fakes the claude and gh CLIs (internal/flowtest/fakeagent, scenario JSON), a second code host (flowharness/fakehost, registered with WithGitHost), dialogs, browser, keep-awake, LAN detection and window manager.
- appwire.Options exposes test seams GhLocator, TrackerGrace and MobilePoll, set via flowharness.WithoutGh, WithTrackerGrace and WithMobilePoll.
- The fake claude prints no stream init line under --output-format json, accepts an inline --mcp-config, and answers headless calls by --system-prompt through scenario prompts rules.
- Space flow tests live in internal/flows/{gitflow,repoflow,editorflow,adeflow,memoryflow,termflow,appflow,mobileflow,githostflow}; bun run test:flows:space is the general suite, and test:flows:space:complete sets KIRA_FLOW_COMPLETE=1 and adds slow permutation coverage.
- Space e2e-real tier (project e2e-real, bun run test:e2e-real:space) uses a go build -tags server binary on a temp home, fake claude/gh first on PATH, and Chromium on /?window=<uuid>.
- The Space server build locates git through gitclient.NewHostLocator (locator_server.go), not the darwin-only locator.
- Space e2e-real has no native dialogs (stubbed with page.route) and no mobile access.
- The server build's EmitTo broadcasts to every browser window (internal/shell/emitto_server.go); listeners filter by their own ids.
- Build lock, port and health helpers for e2e-real are shared between Space and Studio via packages/workbench/src/testing/e2eReal.ts.
- Contract fixtures (P249): scenarios not needing the built bundle against the real backend run twice, as a Go flow test via app.Contract and a Playwright UI spec via contract(scenario, key), both reading apps/<app>/tests/contract/<scenario>.json.
- A backend change that moves a contract value fails the Go flow test; a hand edit of the contract JSON fails both halves.
- Contract keys have forms Service.Method[#variant], args:Service.Method, git:<rpc method>[#variant], event:<channel>, and phone keys http:<METHOD> <path>[#variant].
- Contract volatile values are auto-replaced with placeholders (<home>, <tmp>, <work>, <bin>, UUIDs as <id:n>, RFC3339 as <time>), plus flowharness.Mask/Omit/Replace options.
- KIRA_CONTRACT=write rewrites contract files by merging; stale keys must be deleted by hand.
- For args: contract keys, the UI half compares key sets when the UI mints its own ids.
- e2e-real keeps only what cannot split at the IPC boundary: Studio Docker, MariaDB, SQLite wiring, Postgres connection tests, multi-window routing, terminal keystroke order; Space boot, multi-window routing, terminal keystroke order.
- The graph chunk golden (gitflow.TestFixtures_CaptureGraphChunkFrame) compares by default and writes only under KIRA_CONTRACT=write.
- Biome ignores the contract directories because they hold generated JSON.
- Studio flowharness.New boots appwire.Build in-process with temp HOME/KIRA_HOME, TZ=UTC and KIRA_INSECURE_SECRETS=1; Restart rebuilds the app and Quit runs ServiceShutdown like Wails.
- Studio flow harness uses real SQLite, HTTP/HTTPS servers with a per-binary test CA (CAFile), gRPC with reflection and testdata/flow.proto, a silent TCP server and the Docker engine; it fakes dialogs (queued answers), keep-awake driver, MCP installer (never touches Claude config) and OS auth.
- Studio Docker flow tests use the single image mirror.gcr.io/library/alpine:3.20; labelled resources are swept in flowharness.Main; DOCKER_HOST resolves before HOME is swapped.
- Shared between Studio and Space: internal/flowtest, with an Events recorder implementing appevent.Emitter and a Complete gate on KIRA_FLOW_COMPLETE.
- bun run test:flows:studio is the general Studio flow suite; test:flows:studio:complete sets KIRA_FLOW_COMPLETE=1 KIRA_FLOW_DOCKER=require.
- The real claude tier (P237, P249) lives in apps/kira-space/tests/claude/ and apps/kira-studio/tests/claude/, nested Go modules with own go.mod and replace to the repo module, so go test ./..., lint:go, CI and hooks never see them.
- Real claude tests boot the real appwire.Build with the real claude CLI (haiku, temp HOME, 0.05 USD cap per run) and assert what fakes cannot, such as hook injection, untouched user Claude settings, ADE runs, memory gate and Database MCP install.
- Real claude tests are opt-in and token-costing, run via bun run test:claude, test:claude:space or test:claude:studio; per-area commands are in docs/DEV_ENVIRONMENT.md, section "Real claude tests".
- Run container-backed and UI test suites against their own KIRA_HOME and Testcontainers-provisioned databases, never the developer's ~/.kira-studio or a database used by a running bun run dev:studio, and never disturb that instance.
- Deliberate exception (P25 F10): on macOS the app's Keychain item is shared with the developer's login keychain, so tests saving a connection password touch the same OS encryption key; this is safe because secrets stay in each test's temp KIRA_HOME kira.db and no test rotates or clears the key.
- tests/unit/ needs nothing external and finishes in about a second (bun test tests/unit); it uses plain TypeScript modules exercised with fakes rather than a real container or real app process.
- A unit test exists only for genuinely complex or deeply-nested logic (parsers, cursor/pagination boundary arithmetic, cache eviction, crypto, concurrency state machines); CRUD-, wrapper- and constructor-shaped tests were deleted as low-value.
- The Go suite was pruned against the same strict bar as the unit tests in the same pass.
- security.spec.ts and menu.spec.ts were deleted because their subject moved; they are now repo-root internal/shell/security_test.go and menu_test.go.
- A shared runtime stub (packages/workbench/src/testing/unit/wailsRuntime.ts) registers a fake /wails/runtime.js and is imported for its side effect by every spec needing one, because Bun's module registry is shared across all spec files in a run, so the first-loaded stub wins for the whole run.
- packages/db-fixtures/ is a shared fixture corpus, not a spec suite; all per-engine *.spec.ts files are gone, the last four (clickhouse, mariadb, mysql, sqlite) retired in P58f M10 alongside src/engine/.
- The per-engine specs were kept only while a passing TypeScript spec served as a live oracle to diff the Go port against; that argument expired once src/engine/adapters/ was removed.
- packages/db-fixtures/fixtures/*.{sql,ts} survive because they are read by apps/kira-studio/internal/adapters/testsupport/{postgres,mariadb,mysql,sqlite,clickhouse}.go by absolute path and by apps/kira-studio/tests/e2e-real/.
- Seven support/*.ts modules in packages/db-fixtures (common, connectionConfig, docker, postgres, mariadb, sqlite, kafka) survive; apps/kira-studio/tests/e2e-real/support/*.ts re-exports them for container seeding.
- The kafka spec could never run under Bun because the old TypeScript driver's compiled binding loaded under no Bun ABI; it moved to apps/kira-studio/internal/adapters/kafka/kafka_test.go in P58e M9, ahead of the other specs.

- Per-engine adapter scenario coverage (connect, tree, describe, paging, count, filter, cancel-mid-query asserted server-side, cache, add/delete row, command preview) lives in apps/kira-studio/internal/adapters/*/*_test.go, one Go test file per engine, run by bun run test:go.
- scripts/db-compat.sh (bun run test:compat) runs the conformance packages against every supported kind's oldest and newest server image (sixteen kind/min|max pairs) via testsupport.ImageFor's env-var override, running every pair even after an earlier one fails.
- db-compat's only CI entry point is the db-compat job in .github/workflows/release.yml, a needs:-gated job every release tag must pass; there is no standalone workflow_dispatch workflow. One-off runs use bun run test:compat -- --only <kind> --min|--max locally.
- The supported server version floor/ceiling is surfaced to users via the MIN_SERVER_VERSION map in packages/shared/domain/connection.ts, rendered per kind by apps/kira-studio/frontend/src/project/ConnectionDialog.vue.
- Each adapter's real-container coverage has two suites: a general suite (the *_test.go file) that runs unconditionally under bun run test:go with one assertion per capability a caps.go declares, and a complete suite (authmatrix_test.go per adapter) gated behind KIRA_TEST_MATRIX=1 and run by scripts/test-matrix.sh (bun run test:matrix).
- The complete suite is the full auth/config permutation matrix per adapter (root vs. least-privilege principal, with/without password, with/without database field), with each connecting case's functional consequences attached via testsupport.Scenario/RunMatrix's Then.
- testsupport.RunScenarios runs the same Scenario body outside a matrix table so one scenario backs both test tiers instead of being written twice.
- test-matrix's only CI entry point is the test-matrix job in release.yml, with no standalone workflow_dispatch workflow; one-off runs use bun run test:matrix -- --only <kind> locally. test-matrix and db-compat run concurrently and release needs both.
- Test images pin exact patch tags. PR CI caches images keyed on `testsupport/*.go`; a floating tag would run different builds per job. Mongo stays on 7.0 because 8.x/9.x exit at startup on runner kernels 6.19-7.0.13 (SERVER-125742).
- The complete suite is built to catch a permission refusal getting the wrong ErrorCode (authorization failure read as E_AUTH wrong password); clickhouse, mongo, redis and kafka are known pinned instances, each with a comment at the assertion.
- tests/ipc/ per adapter holds <adapter>.frontend.spec.ts, which drives the real Vue UI with both planes mocked via bun run test:ipc:fe (no Docker, container or native driver), and <adapter>.fixture.ts, which is generated and never hand-written.
- Fixtures are generated by KIRA_IPC_FIXTURES=write go test ./apps/kira-studio/internal/ipcfixture/..., which captures real responses from a real container; plain go test runs assert real responses against the committed fixture. Postgres and sqlite generate no fixture of their own.
- Anti-drift guarantee: a frontend spec cannot mock a shape the backend has stopped producing without that same fixture module's own backend assertion failing first; it holds only while something regenerates the fixtures.
- The fixture generator uses SetEscapeHTML(false) and typed structs, never maps, because Go's encoding/json escapes HTML and sorts map keys by default.
- Non-reproducible fields (timestamps, ephemeral ports, random ids, approximate row-count estimates) freeze to fixed placeholders (frozen.go) after structural validation against the real value, never invented.
- tests/ipc/support/types.ts is the one TypeScript file kept in tests/ipc; it holds the frontend specs' shared types.
- tests/e2e/ (the _electron.launch() tier) was retired with the Electron shell; mongo.spec.ts's full-stack anchor value was not recovered, an accepted documented loss.
- Other retired e2e specs were recovered elsewhere: sqlite and postgres wiring by tests/e2e-real/, and s3.spec.ts's file-write contract by a packages/db-fixtures/ case. hardening.spec.ts and startup.spec.ts were deleted with no analogue; cold start is now a manual procedure (docs/PERF.md §3).
- tests/ui/ (bun run test:ui:studio) drives the real built frontend/dist bundle over a static HTTP file server in real WebKit, with no native app process or container; both wire planes are mocked.
- In tests/ui/, the control plane is mocked by page.route('**/wails/runtime') answering from fixtures keyed by a hand-maintained CHANNEL_TO_FQN table, and the data plane by a fake window._wails.streamFactory socket.
- tests/e2e-real/ is a deliberately small full-stack wiring tier that builds the Go shell with -tags server (Wails v3 server mode), serving both planes over HTTP and WebSocket to a plain Chromium tab with no native window, against a real backend, adapter and container.
- multiwindow-real.spec.ts is the only full-stack proof of P8's tabs.window_key isolation (two windows, one backend, each keeps only its own tabs).
- e2e-real uses Chromium rather than WebKit because it is a wiring tier, not a UI-fidelity tier; UI fidelity is tests/ui/'s job.
- Server mode has no file dialogs (FilesService.ChooseSave/ChooseOpen answer HTTP 422), so a spec needing one stubs that method through a passthrough route reusing CHANNEL_TO_FQN.
- mariadb-real.spec.ts's second test proves two native kinds (MariaDB and Kafka) coexist in one session across a page reload, replacing the retired test that killed the Node engine child.
- A worker-scoped flowServers fixture spawns apps/kira-studio/bin/flowservers (built once under the shared build lock), so e2e-real and the Go flow tier share one server implementation; the serverEnv option adds environment to it.

- tests/visual/ (bun run test:visual:studio) is a bounded pixel-diff tier of eight specs and 13 snapshots, reusing tests/ui/'s fixtures and mocked wire planes.
- Seven visual specs (workbench shell, data grid, SQL console, connection dialog, Schema (DDL) editor, HTTP request view, Runs script dialog) have one at-rest screenshot each; settings.spec.ts contributes 6, one per Settings pane.
- The visual tier is its own Playwright project, not folded into ui, mirroring ui-timing, because a different measurement contract earns a different project.
- Capture or update visual baselines only from the ui CI job's ubuntu-latest environment, never a local macOS run, because WKWebView and WebKitGTK glyph rendering differ regardless of font choice.
- Playwright's animations: 'disabled' default freezes the app's state-gated infinite CSS animations before visual capture.
- tests/visual/support/pin-fonts.css, loaded via stylePath and affecting only visual specs, collapses --kira-font-ui and --kira-font-data to their trailing generic keyword to remove run-to-run ambiguity from multi-hop fontconfig fallback on the CI image.
- Kira Space has its own visual tests in apps/kira-space/tests/visual/ (bun run test:visual:space): settings (4 panes: Advanced, Appearance, Git, Memory) and git-module (3: graph, graph detail, stash dialog).
- The git module's Go tests (internal/git* packages) build real repositories under t.TempDir() against the git on PATH, need no container, and self-skip without git; the 2.38 git floor applies to tests too, so older git skips more.
- gitclient/porcelain is guarded by a committed golden-byte corpus of recorded real git output, because a porcelain parser has several interacting rules.
- gitsearch is guarded by packages/git-core/testdata/searchConformance.json, read by both its Go suite and the TypeScript twin's suite.
- The git perf probes TestGraphStreamPerf and TestG8PerfBaseline (internal/flows/gitflow/) run only with KIRA_GIT_PERF=1 and assert nothing, printing one key=value line each, because a hard threshold would be flaky on real macOS hardware; numbers are in docs/PERF.md §2.13.
- internal/queryplan/parse_test.go and tests/unit/explain-plan.spec.ts read the same fixtures in tests/fixtures/explain-plans/, pinning the Go EXPLAIN-parser port against the frontend parser so drift in either fails on the same bytes.
- internal/mask/parity_test.go and tests/unit/mask-parity.spec.ts both read 66 shared masking fixture pairs under tests/fixtures/mask/.
- apps/kira-studio/playwright.config.ts runs six projects: ui, ui-timing, ipc-frontend, e2e-real, visual and perf; perf is opt-in, serial, in no suite script, and lives in tests/perf/. All but ui-timing are fullyParallel.
- Kira Space's apps/kira-space/playwright.config.ts has projects ui, mobile-ios, mobile-android, e2e-real (workers: 2) and visual.
- The ui project can be fullyParallel (unlike the old e2e project's workers: 1) because the serialisation existed for Electron apps contending over wall-clock/RSS budgets and Docker containers, which this tier lacks.
- ui-timing runs with workers: 1, serially after ui finishes (dependencies: ['ui']), because its real-millisecond wall-clock assertions need the rest of the tier's CPU contention out of the way.
- e2e-real runs workers: 2 safely because a per-test KIRA_HOME plus WAILS_SERVER_PORT gives each instance its own SQLite app storage and secrets.
- Terminal Close hangs up only the shell's process group; jobs the shell does not hang up (dash background jobs, bash/zsh disown, nohup, trap '' HUP) outlive the tab, as in VS Code, tmux and kitty.
- No sweep of the session's process groups exists on Terminal Close, because such a sweep cannot tell a disowned job from a dash job.
- On Linux the terminal session ends exitDrain (200 ms) after the shell exits, not when the last slave holder closes, so a surviving job neither stalls Close nor keeps a shell-exited tab open.

## Presentation site (P280)

- The public presentation page for both apps is https://vlad-cirstean.github.io/kira-studio/; it is static with zero client JavaScript.
- The site has two static routes: / is Studio and /space/ is Space. AppTabs renders a nav with aria-label "Apps" holding two links, the current one with aria-current="page"; no tablist, no script, so deep links work.
- layouts/Landing.astro renders both tabs; each tab sets its own title, description and og:image.
- main data-app sets the --tint variable (--kira-info for Studio, --kira-ok for Space), which utilities such as text-tint and bg-tint/10 read. Only existing tokens are used; no new hues.
- Icons are Codicons (@vscode/codicons, CC BY 4.0) inlined as SVG through Icon.astro and the typed src/data/icons.ts map. The footer carries the attribution; decorative icons are aria-hidden.
- site/ is a Bun workspace member (@kira/site) using Astro 7 and Tailwind 4 through @tailwindcss/vite, because @astrojs/tailwind supports only Tailwind 3.
- Route every link and asset through import.meta.env.BASE_URL or astro:assets, never a hard-coded '/' path, because astro.config.mjs sets base to '/kira-studio'.
- The site palette comes from packages/theme/src/tokens.css, which global.css maps into @theme. Fonts are Inter and JetBrains Mono (Fontsource), the same pair the screenshots use.
- Page copy lives in site/src/data/ and follows the CLAUDE.md "Communication style".
- The pages.yml deploy workflow (configure-pages, upload-pages-artifact, deploy-pages) only builds site/dist, with no screenshot capture and no Playwright in the Action. Go-live steps are in docs/DEV_ENVIRONMENT.md.
- bun run site:check builds, then runs site/tests/site.spec.ts in WebKit at 375 px and 1280 px per route. It asserts no horizontal scroll, images load, links answer 200, no console errors, phone crop and desktop framed window, 16 px text, 44 px tap targets, plus tab bar, icons, head tags and footer attribution.
- visual.spec.ts compares full-page screenshots per tab against Linux-recorded baselines and is skipped on other platforms.
- bun run screenshots drives the production frontend bundles of both apps against the mocked Wails runtime (same fixtures as the ui tier; no backend, no credentials) at 1024 x 640 window, scale 2, WebKit, pinned clock and en-US/UTC.
- Screenshots use Inter and JetBrains Mono in place of -apple-system and Menlo, which Linux lacks.
- Screenshot output is raw screenshots/<id>.png, phone crop <id>-detail.png (440 x 330 CSS px), and framed/<id>.png. Framing composites screenshots/assets/traffic-lights.svg over a rounded transparent frame so README and page match; raw captures never contain window buttons.
- Keep screenshot generation byte-identical: a rerun must leave git status screenshots/ clean. Monaco tokenizes JSON asynchronously (the shot waits for coloured tokens) and fades scrollbars on a JS timer (capture hides them).
- Docs use Starlight (MIT), serving 18 pages from site/src/content/docs/docs/ at /docs/. The docs collection extends docsSchema with a required sources: list of repo-relative paths.
- Starlight's Tailwind layer is wired in global.css, Docs is in src/data/nav.ts, and src/lib/shots.ts serves screenshot imports to any page.
- site/scripts/check-docs.ts (rules in docsRules.ts) parses each docs page with unified and remark and enforces the CLAUDE.md style rules plus frontmatter and link checks. Each sources: path must exist. It runs inside root lint and site:build.
- Developer docs are not on the site: P290 removed the Develop section; they live in README.md and docs/.
- A phase that changes a file listed in a docs page's sources: must update that page in the same phase; grep sources: for the changed path to find it.
- Take docs claims from code and packages/theme/src/i18n/en.json, never from README wording alone.
- Jekyll was declined for the docs site because its Ruby toolchain sits beside Bun and Go and it has no component model.
- VitePress was declined because its docs-centric default theme fights a landing page and the Vue SFC docs model is not needed for one page.

## Hermetic launches (P455)

`agent.hermetic` (default true, per app kira.db; Space switch in Settings, Agent; Studio always on) makes a Kira
agent launch skip the user's CLI config. Config the run needs comes from Kira: app `settings.yaml`, the grant's MCP
config, project files.

- Seam: `InteractiveSpec.Hermetic` and `IsolationDir`, `HeadlessSpec.Hermetic`. A provider implements `harness.Isolator`
  to take a per-launch dir; `harness.PrepareIsolation` makes `<stem>-iso` (`IsolationGlob`) under the agent dir and
  returns its release. The release rides the caller's release chain (ADE `bindTUI`, scriptruns, headless end), not
  `Store.Track`; `Sweep` removes leftover `*-iso` dirs at start. Refused with `UserMCP`.
- Claude: `--setting-sources project,local`, env `ENABLE_CLAUDEAI_MCP_SERVERS=false` and
  `CLAUDE_CODE_DISABLE_AUTO_MEMORY=1`, and an inline `--settings` with `claudeMdExcludes` for `~/.claude/CLAUDE.md`:
  a repo under `$HOME` loads that file as its ancestor's project memory even without the user source (C9, found by
  `TestHermeticSession`). Local-scope MCP stays (Q4). Isolated (headless) runs keep `''`;
  `ade.headlessSettingSources: user` gives `''` under hermetic. Facts C1 to C9 in `claude/hermetic.go`.
- OpenCode: `XDG_CONFIG_HOME=<iso>/xdg`, a dir linking every `~/.config` entry except `opencode`, plus
  `OPENCODE_DISABLE_CLAUDE_CODE_PROMPT=1`. Words go on `/usr/bin/env` after the login profile, with `-u` for layer vars
  the launch does not set. Probes H-OC1 to H-OC5 on 1.19.5 in `opencode/facts.go`; `TestHermeticDebugConfig`.
- Ticked MCP servers: `agentcfg.ServerSource{Store, Hermetic}`. Hermetic: servers come from `settings.yaml`
  (`<Stem>-app.mcp.json`, secrets resolved and masked), `McpServers()` returns `source: "app"` with no status, a name
  missing from the app settings blocks the run. Off: the CLI list (`source: "agent"`, P444).
- Tests: `claudeflow` `TestHermeticClaude`, `harnessflow` `TestHermeticOpenCode`, Studio `TestHermeticStudio`;
  real `TestHermeticSession`.

## Known open items

Kept only while genuinely open — delete an item the moment it's resolved, never mark it done in
place. `CLAUDE.md` states the process rule; this is the list itself.

- **Agent context rests on a hidden Claude flag (P452).** `--append-system-prompt-file` is absent from `claude --help`. A CLI that drops it fails the launch loudly. `TestAgentContextHeadless` catches it. Studio has no context editor; edit the file in an editor.

- **Agent settings secrets show in a live terminal (P453).** Only headless step output is masked. A TUI that echoes its env or a hook that prints a secret shows it in the tab and the Space phone terminal ring, as for script secrets below.
- **Project statuslines are not chained (P453).** A project `.claude/settings*.json` statusline never ran under Kira (flag settings outrank it); the user statusline comes only from `claude.status_line`. Import brings the user's.

- **Exported hooks run twice with hermetic off (P454, P455).** Hermetic launches skip the user's config, so an exported hook runs once. With `agent.hermetic` off the user's config still loads and the hook also runs from `settings.yaml`.
- **Managed config cannot be isolated (P455).** An admin-managed policy config still loads under hermetic launches, for both providers; the CLIs read it by design.
- **Sign-in kept in user config breaks under hermetic (P455).** Claude or OpenCode credentials held in a user config file, not the CLI's own store, are not seen by a hermetic launch. Workaround: sign in through the environment (a key the shell exports) or turn `agent.hermetic` off. Kira adds no passthrough keys.
- **Shell profile exports override hermetic env (P455).** A login profile that exports `CLAUDE_CONFIG_DIR`, `ENABLE_CLAUDEAI_MCP_SERVERS`, or an OpenCode layer variable Kira sets (`OPENCODE_CONFIG_DIR` included) wins over Kira's value on Claude. OpenCode words ride `/usr/bin/env` after the profile, so they win except `OPENCODE_CONFIG_DIR`, which the hook plugin sets later.
- **Studio has no hermetic switch (P455).** Studio is always hermetic. Space edits `agent.hermetic` in Settings, Agent.
- **OpenCode TUI preferences do not apply under hermetic (P455).** `~/.config/opencode` is not linked, so theme and keybinds there are lost. Turn hermetic off to keep them.
- **OpenCode still loads `~/.opencode` under hermetic (P455, H-OC2).** The XDG redirect does not move it; probe on 1.19.5 showed `$HOME/.opencode/opencode.json` loads. Kira does not read or move it.
- **OpenCode user skills leak under hermetic (P455, H-OC3).** No env drops `~/.claude/skills` and `~/.agents/skills` while keeping project skills; Kira keeps project skills (Q6).
- **Hermetic OpenCode does not load project `CLAUDE.md` (P455, H-OC4).** `OPENCODE_DISABLE_CLAUDE_CODE_PROMPT=1` is required with the redirect and also drops it. Project `AGENTS.md` loads.
- **OpenCode applies no common hook (P453).** Plugin hooks have no user-command form; the settings view shows each common hook as not applied.
- **Saved script secrets show in a live terminal (P447).** A terminal-run script prints into its live tab and the Space phone terminal ring unmasked; only the run tail and log are masked. A process that echoes its secret shows it there.
- **OpenCode facts without a model key are unverified (P449).** `facts.go` marks each with its probe id: the lost-session error text (U5), the MCP tool permission key `<server>_<tool>` (U7), plugin event names and `fetch` over a unix socket (U12), install paths (U15), the model API host list for network-off runs (U16), and the tool and cost event shapes of `run --format json` (`step_start`, `text` with `part.text` and `step_finish` with `part.reason` are verified). A wrong guess degrades quietly: a lost session reads as a plain failure, a MCP grant may not match, activity icons may stay idle. Run the probes in `docs/DEV_ENVIRONMENT.md` with a key, then fix `facts.go` and delete this item.
- **OpenCode has no cost cap (P449).** A smart script or step with a budget is refused on OpenCode. The run reports cost when its events carry one; the shape is unverified.
- **OpenCode network-off runs reach only the listed model hosts (P449).** The list is a guess; a model on another host fails under the sandbox.
- **OpenCode extra directories ride permission only (P449).** `AddDirs` become `external_directory` allow patterns, not a CLI flag; the OS sandbox profile is the real boundary.
- **Studio has no agent picker (P449).** Only Space chooses the provider per launch and per step; Studio runs the provider its Agent setting names.
- **The OpenCode MCP install command is a snippet (P449).** `opencode mcp add` has no non-interactive local form, so Kira shows a config snippet with a placeholder token the user pastes into `opencode.json` themselves.
- **Interactive exec plugins are unsupported (P439).** A kubeconfig credential plugin that prompts on a terminal fails with the `plugin` reason. Non-interactive plugins work.
- **Studio Discover remounts still grow the renderer heap (P433).** Switching away from and back to a Kibana Discover tab adds about 0.6 MB per round after round 2 (5 rounds: 31.4, 32.3, 32.8, 33.9, 34.1 MB), and a second open/close cycle adds 1.9 MB. Close-all ends 18.5 MB over baseline, mostly first-load module code plus Monaco and KQL source strings. The Monaco `addCommand`, grammar re-registration and `usePersistedState` leaks are fixed; the rest is unattributed (V8 JIT code is the main suspect). Probe: `heap` project in Studio, `docs/DEV_ENVIRONMENT.md` "Perf probes". Find the retainer with a heap snapshot diff across rounds 3 and 5, then delete this item.
- **Kibana search uses an internal route (P424).** `POST /internal/search/es` is not a public API; a Kibana upgrade can change its body or headers. The conformance suite runs against 9.5.3 and the 8.19 floor via `KIRA_COMPAT_IMAGE_KIBANA`.
- **Azure Repos search covers the default project only (P423).** `SearchChangeRequests` reads 2 pages of 100 in the configured default organization and project. A PR elsewhere is not found by issue key.
- **A reactivated Azure PR stays closed in the index (P423).** Azure has no update time, so the delta walk never revisits it. A re-backfill (account change or schema bump) fixes it.
- **Azure serves no PR head ref (P423).** ADE fetches `refs/pull/<n>/merge` for fork and closed PRs. It is a merge preview and disappears once the PR closes.
- **`az` calls are not in the op log (P423).** Like `gh`. Needs a host category in `oplog`.
- **`ade.changeRequestRepo` has no Azure URL form (P423).** Title-search hits on Azure show no repo label.
- **PR view caps its content (P427).** Comments, threads and checks past the caps show the newest, with a note. Open the request on the host for the rest.
- **The PR view has no live update (P427).** It refreshes on demand only.
- **Azure PR detail fixtures are hand-written (P427).** No org was reachable. Confirm `pr_detail.json`, `policy_detail.json` and `threads_get.json` against a real org.
- **Images in PR descriptions render as links (P427).** Never inline.
- **Azure `@<GUID>` mentions show raw (P427).** No identity lookup.
- **Azure threads beyond the newest 200 are not read (P427).**
- **Comments carry no attachments (P429).** Kira never touches a CLI's credentials, and none of gh, az, acli uploads a comment attachment. Add files on the host.
- **Comment posting is checked against documentation only (P429).** The acli `--body-file` ADF input is not seen live; the Azure reply assumes the first comment id is 1.
- **With hermetic off, a smart run that ticks a user MCP server loads the user's Claude settings (P444).** Claude loads user MCP servers only with the `user` setting source, so the user's permission allow rules, env and plugins apply; hooks are off and only ticked servers plus `kira-ade` load. `allowManagedPermissionRulesOnly` in `--settings` does not drop the user rules (CLI 2.1.316). The picker cannot show tool names or descriptions, because Kira no longer connects to user servers; it lists what `claude mcp list` reports and health-checks every server on each call (cached 30 s). Hermetic runs avoid it (P455). Delete if the hermetic switch goes away.
- **An agent's own shell `git` in a non-isolated run is not an app op (P415).** It is the agent's process; only the app's own spawns reach the op log.
- **Jira pull requests missing from commit-only links (P417).** acli cannot list the Development field's pull requests. Rows come from code host search by issue key in the title or head branch. A pull request Jira links only through a commit message is not found; the task shows Jira's count against the rows found as a gap. Delete once acli exposes the rows.
- **Mission Control Space switch (P396) unconfirmed on macOS.** Three-finger swipe up moves focus to another Space; code finds no app caller, trace added. Check: set log level `debug`, two Spaces each with a Kira window, swipe up/down 5 times; send log lines within 2 s of each jump (`window raise`, `prompt raise`, `app event`, `window event`) plus macOS version. Delete once the fix lands.
- **Archived ADE data grows without bound (P399).** Archive keeps run and step logs (2 MiB tail each), pinned review sessions and history rows forever; nothing purges them. Every board read and push carries the whole history. No user action deletes an archived task yet and the archived finder lists all history at once. Delete this item once a delete action (cascading its logs) and a paged history read land.
- **The macOS default-app switch is unverified on a real Mac (P332).** No Mac was available: the cgo registry, the document types in LaunchServices, whether a system prompt appears when setting a content-type default, and the UTI of each imported row are untested. Check: install the DMG, run it once from `/Applications`, then `lsregister -dump` lists both document types, Open With lists Kira Space, and the pane's switch changes `mdls -name kMDItemContentType` output.
- **MCP `run_query` read verdicts run under `READ ONLY` wrap, which does not stop every side effect (P329).** A statement classified read runs in `BEGIN READ ONLY` (Postgres) or `START TRANSACTION READ ONLY` (MySQL/MariaDB) unless writes are allowed. Postgres still permits `pg_terminate_backend`, `pg_cancel_backend` and `dblink_exec`; MySQL/MariaDB still permits `GET_LOCK` and writes to temporary tables. SQLite and ClickHouse get no per-request wrap: no write-capable function is reachable from a read there.
- **MongoDB 8.x/9.x servers are unverified in CI.** Every 8.x/9.x image exits at startup on runner kernels
  6.19-7.0.13 (SERVER-125742; SERVER-136848 for Ubuntu). Test and db-compat max pin `mongo:7.0.45`. Restore an
  8.x/9.x max once runner kernels leave that range. 7.0 support ends 2027-08-31.
- **Sandboxed OAuth token refresh is unverified on a real Mac (P341).** The profile still denies `file-write*` under `~/Library/Keychains`. The legacy file keychain writes `login.keychain-db` in-process, so a `claude` refresh of an expired token from a sandboxed run may fail. `setup-token` is gone, so no supported workaround exists. Check: `DEV_ENVIRONMENT.md` Mac checklist step 9. If refresh fails, allow writes on the login keychain file literals (db, journal, lock) only.
- **The sandbox profile leaves some macOS services open (P287).** The profile allows by default and lists
  denies, so a launchd or XPC service that acts for the agent (for example `launchctl submit`) is not closed
  beyond the listed denies.
- **`~/.claude.json` is writable to a sandboxed agent (P287).** It holds the user's MCP servers; the CLI writes
  it on every run.
- **`~/.claude` root scripts are writable to a sandboxed agent (P292).** Writes to all of `~/.claude` are allowed;
  denies cover only named config files and the extension dirs. A script a user's `settings.json` points at outside
  them (for example `statusline-command.sh`) runs unsandboxed in the next interactive session. An allowlist of the
  subdirs the CLI needs cannot be checked without a real Mac.
- **Toolchain presets grant writes to toolchain binaries (P287, P292).** The Node, Go, Rust and Python presets add
  write paths such as `~/.nvm`, `~/go` and `~/.cargo`, so an agent can replace a binary the user runs later.
  Narrowing the presets to cache dirs is a user decision (P287 plan).
- **A network-off profile allows every loopback port (P287).** Local dev servers and test DBs need it, so a local forward proxy or tunnel on `localhost` lets a `network: false` run bypass the allowlist. Restricting remote loopback to `Policy.Ports` needs an opt-in for runs that bind local servers. `Policy.Ports` only feeds the profile header comment.
- **A short-lived process of a sandboxed run can lose its denial (P292).** The system-wide denial stream names a pid, and an exited pid is attributed only when its process name matches an earlier denial of the run.
- **Hooks and build scripts an agent edits run unsandboxed later (P287).** Kira's own git operations and the
  user run them outside the sandbox.
- **The sandbox is unverified on a real Mac (P287).** Linux checks the generator and flows only. Delete once
  the Mac checklist in `docs/DEV_ENVIRONMENT.md` passes.
- **A written memory doc holds no per-fact keywords (P285 Part 3).** Importing it into another store indexes its facts with the doc-level keywords only; the store regenerates per-fact keywords only for facts the gate sees.
- **Memory ingest compression target not met (P285, P291).** `round-p3-01`: `C_fact` 0.434, `C_store` 0.661, `C_doc` 0.734, `R_ess` 0.885. Golden essential facts alone are 0.47 of source chars. The telegraph style cuts fact chars 7% and `C_store` 4.5% on identical facts; run on the pipeline it raised recall and stored 18% more facts (XT, commit `a487cbc18`, `C_store` 0.753), so it is not adopted. Residual token loss of the style: 14 of 209 facts fail the preservation check.

- **No detached runner for agent runs and PTYs (P279).** Quit or crash stops every agent and PTY. The memory ingest runner is separate and does detach (P274). Resume restores the Claude conversation; the turn in flight is lost and its tool effects may be half-applied. The `kira-db` MCP stops with Studio. Resume needs the saved transcript under `~/.claude`; without it the run fails with "could not resume".

- **Studio Settings footer shows `~/.kira-studio/kira.sqlite`; the real file is `kira.db`.** Label in `apps/kira-studio/frontend/src/workbench/SettingsDialog.vue`. The docs say `kira.db`.
- **Quit discards staged grid edits and dirty edit buffers (P267).** Tab close and dialogs guard unsaved edits; app quit does not. Accepted by the user (P266 decision).
- **Backend, git-core and flow engine text is English-only (P265).** Accepted by the user. Go errors outside the catalog, `packages/git-core` strings and `@vue-flow`'s hardcoded `vue-flow__*` ARIA hints stay English; `copyCoverage` skips the vue-flow subtree.
- **Claude Code usage limits show only what Kira-started sessions report (P239).** Claude Code sends
  `rate_limits` only for claude.ai Pro and Max plans and only after the first API response of a
  session, so the item reads `Usage: –` until then. A session in another terminal is invisible.
  Unverified on a real Mac build and on Team or Enterprise seats.

- **Desktop notifications are unverified on macOS (P238, P246).** Covers agent notes in Space and
  prompt notifications in both apps. Needs a signed bundle and the user's
  one-time OS permission; a dev build run outside a bundle cannot post (Wails checks the bundle id).
  The darwin sink compiles only with cgo on a Mac. Linux and `-tags server` builds post nothing.
  Delete once checked on a signed Mac build.
- **Approval focus is unverified on macOS (P338).** Compiled and flow-tested with a fake window
  seam only; `internal/shell/unhide_darwin.go` needs cgo on a Mac to compile. Wails calls
  `activateIgnoringOtherApps:`, deprecated since macOS 14. Checks on a signed build, with a write
  connection set to prompt: window on another Space; full-screen; another app full-screen; minimised;
  hidden with Cmd+H; last window closed; preview on raises both stages; five requests at once switch
  once; deny and timeout leave focus alone; setting off raises nothing. Record the macOS version and
  the result with Mission Control "switch to a Space" off. Delete once checked.
- **Mobile agents web is unverified on a real phone (P212).** Covered by Go tests and Playwright
  WebKit/Chromium emulation only. Unchecked: the iOS soft keyboard with xterm (the compose row exists
  for it), the attach WebSocket surviving a backgrounded tab, and touch drag on a real device (tests
  drag with a mouse). IPv6 is not served. Delete once checked on a real iPhone and Android phone.
- **`lannet` is unverified on macOS (P223).** The route RIB default-route selection and the ARP
  table read (`NET_RT_FLAGS` with `RTF_LLINFO`, parsed with `route.ParseRIB`) are compile-checked
  only (`GOOS=darwin go vet`); Linux is covered by parser tests and a live check. Also unchecked:
  whether accepting LAN connections raises the macOS Local Network prompt. Delete once checked on a
  Mac.
- **Mobile traffic is plaintext on the LAN (P223).** Anyone on the same network can read what the phone
  shows and can copy the phone's cookie to act as it until it expires (30 days) or is revoked. Mitigated
  by the trusted-network gate, the warning and expiry; not removable without TLS, which the user
  declined. Delete only if TLS is adopted.
- **Claude Code redraw after a phone resize or reclaim is unobserved (P212 Part 2).** The PTY resizes
  to the phone's size on attach and back to the window's on return; the real `claude` suite does not attach a phone,
  so only the Go and e2e sides are checked. Delete once checked with an authenticated `claude` and a
  real phone.
- **Console reads on PostgreSQL and MySQL/MariaDB drain past the cap (P174).** At the cap `rows.Close()` discards the remaining rows off the wire: memory is bounded, server and network time are not. The Stop button ends a long drain. Delete once a driver offers a cheap server-side stop that keeps later statements in the batch intact.
- **Some Redis console commands are read whole before the cap (P174).** `SUNION`, `SINTER`, `SDIFF`, `EVAL`/`FCALL`, module commands and `GET` of a huge string have no bounded form; go-redis reads the reply whole, then the page is capped. Delete once those commands get bounded rewrites or a streaming reader.
- **Parts of an interactive `claude` turn are unobserved (P147, P149, P150).** The real suite (`bun run test:claude:space`) launches the TUI, sees hooks including `Stop`, and drives `Send` into a session. Never seen against a real `claude`: the folder-trust prompt (the suite pre-trusts the folder), `claude --resume` of a stopped session, the initial ` -- ` message, send-then-archive completing after the turn, and for the P150 review agent `--add-dir` resume, a paste into a `working` turn and a forwarded comment paste (P354). Delete once checked on an authenticated desktop build.
- **A taken-over review round has no reply or finish tools (P286).** `TakeOver` of a round opens the plain interactive session: `reply_review_comment` and `finish_step` are not on it, so its comments stay open until answered by hand. Delete once a take-over loads them.
- **Native close/hide wiring of review windows is unobserved (P150).** A server build has no native window; `closeDecision` has a test, the Wails hooks do not. The P354 main-thread unhide fix on a second Review code press is covered by an ordering test only; the AppKit abort itself is unobserved. Delete once checked on a desktop build.
- **Tab switch remounts above the 50 ms product budget in the WebKit sandbox (P139 Part 2)**. Every
  switch remounts `DataView` and SlickGrid, no tab caching. The sandbox measures p95 159-363 ms and
  gates at 300 ms (`budgets.spec.ts`); ~47 ms of it is Vue component work, and Tailwind v4's
  `@property` fallback, live only on WebKit builds without `margin-trim`, doubles every restyle.
  Real WKWebView is unmeasured. Delete once the Mac check in `docs/PERF.md` §3 settles it.

- **Documents list: the fastest wheel flick still misses 50 ms frames (P161)**. A flick jumps the
  window past every rendered row each frame, so each frame mounts a whole new window of 160 px
  expanded rows (head, badges, tree lines). After P161's shared row tooltip and px overscan, the WebKit
  sandbox measures flick frame p95 62-72 ms, 26-56 of 80 frames over 50 ms (before: p95 157-177, 78-82
  over). Further cut needs lighter rows while scrolling (`isScrolling` placeholders), which changes
  what is drawn: user call. Delete once decided and measured.

- **A backgrounded `Bash` tool call cannot arm the `waiting` activity phase (agent-hooks rule, §4.8)**.
  `reduceAgentActivity`'s `wakeArmed` flag — the signal that turns a session's `Stop` into `waiting
  on monitor` instead of plain `idle` — is set only when a `PreToolUse`'s own `toolName` is `Monitor`
  or `ScheduleWakeup`; a `Bash` call backgrounding an equivalent watcher script is indistinguishable
  from any other `Bash` call here, since `tool_input` is never decoded (P86's own privacy rule, kept
  deliberately: decoding it would mean parsing arbitrary tool arguments for a UI nicety). Closing
  this for real would mean either decoding `tool_input` for `Bash` specifically (breaking the
  privacy rule for one tool) or Claude Code itself distinguishing a "waiting" background run from an
  ordinary one at the hook level.

- **The ADE board's conflict pairs drop a rename/delete conflict whose two paths differ and
  neither lies in the pair's own shared-file set (`apps/kira-space/internal/ade/facts.go`)**. `pairFacts` intersects
  `git merge-tree --write-tree`'s conflicted paths against `shared` (each branch's own changed-path
  set) before reporting `conflicts`; a rename or delete on one side names a path the other side
  never touched, so the intersection is empty and the conflict is silently dropped from the pair's
  `conflicts` list even though `merge-tree` itself saw it. Closing this needs surfacing merge-tree's
  own conflicted paths outside the `shared` filter as a different signal (a follow-up phase's own
  call, since it changes the wire shape `pairs` promises).

- **A linked worktree's own dirty state is read only on board load, an explicit `Refresh` and
  `ArchiveRisk` — never watched (P148).** A worktree a *person* or
  an agent dirties between two board loads (or between `ArchiveRisk`
  and the confirmed `ArchiveTask` that follows it) can still go stale for that window — no file
  watcher covers a linked worktree's own working tree the way `gitsession`'s existing repo watcher
  covers `.git` itself.

- **A pending "Send to Claude, then archive" lives in the renderer, not persisted (P148)**. The v2 `adeDialogs` store's in-flight bookkeeping (`archive:<task>`) is a plain reactive `Map`, gone on
  reload or window close; a reload or close between the Send and the agent's own `Stop` drops the
  pending archive with no record it was ever requested, and the branch stays unarchived with no
  further prompt. Closing this needs the pending archive itself surviving a reload (a persisted
  Go-side flag on the task), a later phase's call.

- **`apps/kira-studio/internal/ipcfixture`'s fixture connection rows carry no MCP mode columns** (v1.7 M3). The committed `testdata/*.fixture.json` hold no `mcp_read_mode`/`mcp_write_mode`/`mcp_ddl_mode`, so each fixture connection logs "coercing connection row: unrecognised MCP ... mode". The `keyTypes` gap this item once named is fixed. Not rerun here for lack of Docker: whether the on-demand suite still diffs against the committed JSON is unconfirmed. Closing it needs the suite's own regeneration path (`KIRA_IPC_FIXTURES=write`) against the current schema.

- **`maskedColumnRenamedOrHidden`'s outer-statement scan can't see into a pre-existing view
  definition** (M7 round 2, finding #1). A view that itself renames a masked column
  (`CREATE VIEW v AS SELECT email AS e FROM customers; SELECT e FROM v`) mentions the masked
  column's real name nowhere in the statement `run_query`/`explain_query` actually see, so the
  refusal that catches an inline alias (`SELECT email AS e FROM customers`) can't fire here — `e`
  renders unmasked. Closing this needs the view's own definition visible to the scanner (a
  per-adapter catalog lookup resolving a view name back to its defining query), out of proportion
  to a text-scanning heuristic that deliberately does no real SQL parsing elsewhere either.

- **M5's correlation tag doesn't verify a masked value's provenance** (M6 round 1, architecture/
  functional-correctness findings). The tag lets an AI client tell two masked cells apart or match
  them (same real value → same tag) without seeing the real value — but it's computed from whatever
  text lands in a result column matching a masked column's name, with no check that the value
  actually came from a real table row. A query like `SELECT 'Maria Gonzalez' AS name` gets tagged
  the same as a real row holding that name, so a client that deliberately probes with guessed values
  can build a tag-to-value lookup table. **Accepted, not a defect to fix**: this app's threat model
  for masking is accidental exposure to an AI client, not an adversarial client trying to exfiltrate
  data on purpose — by explicit decision. Fixing it for real would need per-adapter SQL parsing to
  confirm a column is a genuine table reference rather than a literal/expression, which is out of
  proportion to a threat this app doesn't defend against. Noted here so a future review doesn't
  re-flag it as new.

- **A legitimate JSON/array column is over-refused on a masked connection** (F1, P108 Part 7).
  `renderPage`'s fail-closed rule refuses any `TabularPage` column whose `TypeClass` is JSON or
  Other on a masked connection outright, with no attempt to tell a genuinely unrelated JSON/array
  column apart from one carrying a masked value through `row_to_json`/`json_agg`/a whole-row
  composite. The stronger fix — resolving an output column back to its real source column via
  Postgres's own `FieldDescription.TableOID`/`TableAttributeNumber` provenance — was declined as out
  of proportion to this finding: it would span the adapter's `OpCtx`/statement-execution path and
  `render.go`'s matching logic, a materially larger structural change than the finding itself.
  Closing this needs that provenance wiring.

- **Response bodies, gRPC response messages and jar cookies reach `kira.db` history unmasked**
  (F16, P108 Part 8). `apps/kira-studio/internal/bridge/http.go`'s `maskSecrets` masks headers, redirect/final URLs and the
  rendered `Wire` exchange, but never `resp.Body` — `apps/kira-studio/internal/httpclient/options.go`'s own
  `WireMask`/`wire.go`'s `capWireText` (P108 F12) mask the *request* body's wire text earlier, before
  truncation, but that is a different pass over a different field, and the response body stays
  untouched by either; `bridge/grpc.go`'s `maskGrpcResult` never masks `Messages[].JSON`; the
  jar-cookie bridge (`http.go`'s `Cookies`) returns `JarCookies` unmasked.
  Accepted by design (P8 OQ-6, P11) — a request secret is masked because it's substituted from a
  known name/value pair; a response body is opaque server output with no such pairing to mask
  against. A server that echoes a secret back in its response body, a gRPC reply message, or a
  `Set-Cookie` therefore stores that secret in plaintext in history. Closing this needs the same
  substituted-secret value set applied as a body/message/cookie scan, not just a header/URL one.

- **`apps/kira-studio/internal/dbmcp/http.go`'s HTTP server leaves `WriteTimeout`/`ReadTimeout` unset** (M7 finding
  #7 gave it `ReadHeaderTimeout`/`IdleTimeout` plus the cross-origin wrapper). Deliberate: the
  Streamable HTTP transport holds long-lived server-to-client streams, and a write/read deadline
  would cut one mid-response; a slow client body on a long POST is not a threat on loopback the way
  a slow header (`ReadHeaderTimeout`, now bounded) is. Loopback-only binding plus the cross-origin
  wrapper narrow who can reach the server further still, but a client that opens a connection and
  simply never finishes its response body can still tie one up indefinitely.

- **The SQL splitter has no notion of a compound statement body** (F4, P108 Part 11). A SQLite
  `CREATE TRIGGER … BEGIN …; …; END`, a MySQL stored procedure body, or a Postgres `BEGIN ATOMIC …
  END` all use `;` to separate the *inner* statements of one outer statement — `packages/shared/domain/sqlSplit.ts`'s
  `scanSqlSpan`-driven scan (`sqlLex.ts`) tracks only quote/comment/dollar-quote lexical state, not `BEGIN`/`END`
  nesting, so it splits at every inner `;` too. A verified SQLite trigger body splits into four
  fragments instead of staying one statement. Closing this needs real `BEGIN`/`END` depth tracking
  per dialect (a fifth `SqlLexOptions`-style flag isn't enough — it's a grammar-level construct, not
  a lexical span), out of proportion to this finding's own five lexical-form fixes (`#`/`//`
  comments, `E'…'` strings, nested block comments, `[ident]`, the `$`-in-identifier guard), which
  this phase did close.

- **First-launch window-size clamp (P22 D6(a)) still can't apply to the very first window a fresh
  install opens** (round-2 review finding 4). Repo-root `internal/shell/openwindow.go`'s
  `OpenWindow` now resolves `app.Screen.GetPrimary()` fresh per call rather than once before
  `app.Run()`, which lets `DefaultBounds` (`internal/shell/window.go`) see a real work area for a
  window opened well after `Run()` (Dock-reopen minting a fresh window, "New Window" with nothing to
  cascade from). But every window opened at startup is still built before `app.Run()` even runs
  (`main.go`'s own top comment), and on macOS the
  screen cache isn't populated until `ApplicationDidFinishLaunching` fires, strictly after `Run()`
  is called. So a genuinely first-ever launch still gets the unclamped 1280×800 default until the
  window is resized once. Fixing this needs deferring startup window creation until after that
  event fires — a materially larger structural change than this fix.

**As of P100, every item from here through the comment-reload fan-out item below (including the
Performance: subsection) describes Kira Space, not Kira Studio** — the native code
workspace and git module both moved there in full, unchanged, so each limitation moved with the code
it describes. The items after it are Kira Studio's own (or, for the workflow-coverage item,
repo-wide).

- **C5's native project tree does not follow the filesystem** (§7.1). It refreshes on workspace
  open and on an explicit Refresh action only — a file created, deleted or modified outside the app
  is not reflected until one of those happens. There is no worktree watcher of any kind (P97 removed
  the only one this app ever had), so a live-refreshing tree needs one built for this purpose alone —
  a watcher's worth of complexity on a button.
- **`.vue`/`.svelte` files color as plain HTML in C5's Monaco viewer** (§9.4). No Monaco grammar
  exists for either, so a `<script>` block's contents color as HTML text, not as TypeScript/
  JavaScript — the alternative is a hand-written SFC Monarch grammar, which `CLAUDE.md`'s
  library-reuse-first rule declines.
- **C7's repository-wide search has no include/exclude filter.** A repository that commits its
  dependencies (a vendored `vendor/` tree) searches them, because enumeration is unchanged
  `codeworkspace.EnumerateAll` — the right answer is a glob library (`bmatcuk/doublestar` or
  `gobwas/glob`, both MIT, since `path.Match` has no `**`) plus a two-field UI, deferred whole
  rather than half-built.
- **C7's search results are a point-in-time snapshot with no live update.** A file changed,
  created or deleted after a search ran is not reflected until the search is re-run — the same
  posture C5's project tree already takes for its own listing.
- **C7's search cannot match a pattern spanning multiple lines.** The scanner hands the matcher one
  line at a time (`^`/`$` anchor per line, D5); a `--multiline`-equivalent search is out of scope.
- **Native `review.session.save`/`.load` (`repo/git/reviewSession.ts`, backed by the pinned
  repo-graph tab's own persisted state) has no expiry**, unlike `gitreview`'s own `IdleTTL` reaper window. A native resume point can
  be arbitrarily old and still gets silently reapplied on the next cold mount — no fix
  attempted here, since the pinned tab's state is small (a branch/mode pointer, not the review data
  itself) and already goes away when the tab or workspace closes.
- **No standalone merge or rebase operation exists anywhere in this stack** (P67e §7). `op.run`'s
  `OpRequest` kinds have no such kind, `packages/git-ipc`'s contract has none, and `packages/git-ui`
  has no `MergeDialog`/`RebaseDialog`. Merge and rebase are reachable only as `remote.run`'s own pull
  strategies (`ff-only`/`merge`/`rebase`) and inside `stack.restack`'s own branch-by-branch rebase.
  P111 (`ContractVersion` 41) only added a `rebaseMerges` flag onto that existing pull path — still
  no standalone operation. Building one needs a new `OpRequest` kind, a new `opSpec` with an undo
  policy, a new preflight, a new dialog and a further `ContractVersion` bump — out of scope until a
  phase actually asks for it.

Performance:
- **`repo/state/fileTree.ts`'s `useFileTreeStore` (Pinia now, same file) tree-filter computed has
  per-row reactive dependency tracking** costing 3-3.5x at the 200,000-file cap (C13-6's `markRaw`
  covered the tree structure but not the `status`/`expanded` reactive lookups inside the filter,
  still true post-Pinia: `state.status`/`state.expanded` stay reactive fields the filter reads per
  row).
- **One comment mutation with N open review diff tabs on the same branch triggers N+1 redundant
  `review.comment.list` re-anchoring passes** (a server-side cost) — C13-12 fixed the diff-reload
  fan-out (C14-2 fixed a regression in that same split) but not the comment-reload fan-out.

- **`.github/workflows/pr.yml` has no `test:ui:space` step.**
  `typecheck`/`lint` cover Kira Space (whole-repo scripts), `build:space` and the Space `gitclient`
  tests run. `verify:packaging` in `pr.yml` finds no packaged bundle, so its artifact checks skip;
  `release.yml` runs them. Closing this needs its own follow-up. This session cannot push a
  `.github/workflows/*.yml` change (`docs/DEV_ENVIRONMENT.md`); stage it under
  `docs/pending-changes/`.

- **No light theme; `ade`'s kept tone and palette values are dark-only (P138)**. Kira Space has
  one dark `:root`; design §7 has no light palette. Kept tone text measures 1.6-2.4:1 on white
  (amber 1.79, red 2.40, green 1.78, blue 2.02, purple 2.11, grey 2.03) and work palette slots
  1.86-3.76:1 (`ade/v2/tones.ts`, `palette.ts`). All `ade` chrome resolves through `--kira-*`, so a light `:root` flips it with no
  `ade` edit. Closing this needs light tone variants plus a light `:root` in `packages/theme`,
  which changes kept values.

- **ClickHouse console: a `Nullable` string equal to `ᴺᵁᴸᴸ` renders as NULL (P168 Part 3).** The `*Strings` JSON format prints NULL as that text. The table read path settles it exactly with an `isNull(col)` flag; arbitrary console SQL has no safe place for one. Non-Nullable columns are exact. Delete if the console moves to a typed format.

- **Go and TS masks count Indic conjuncts differently (P168 Part 6).** `uniseg` v0.4.7 (latest) counts `क्ष` as 2 graphemes, ICU (Unicode 15.1, rule GB9c) as 1, so a Hindi name masks to a different bullet count in the grid preview than over MCP. The MCP render path (Go) is the boundary; the preview is advisory. No library fix exists.

- **A pending insert row's cell is a single-line `<input>` (P176).** A staged value with a line break (pasted TSV/CSV) shows flattened, and typing in that input restages it without the break. Delete once insert cells move to the inline cell editor's textarea.

- **`memory-mcp` as a macOS app-bundle executable is unverified (P201).** Verified with a `-tags server` Linux binary only. Spawning the bundle's executable as a CLI child of Claude Code on a Mac is unobserved. Delete once checked on a Mac.
- **Memory has no delete (P201).** Stored memories are never deletable by design (updates keep a `historical` version).
- **Re-importing a changed document never retracts facts (P211).** New facts add or update; facts the edit removed stay as current memories. Delete if memories gain a delete or retract path.
- **Semantic memory search is unverified on a Mac (P210).** Loading the bundled dylib from `Contents/Frameworks`, int8 kernels on arm64 and the worker footprint were checked on Linux only (95 MB loaded, 137 MB peak). Delete once checked on a Mac.
- **Semantic search is English-oriented (P210).** arctic-embed-s is English-only; non-English memories rank by keywords mostly. The smallest multilingual alternative under 500 MB (multilingual-e5-small int8, 362 MB peak) retrieves worse on English.
- **`declare_repos` is untested against a real board and real `claude` (P199).** Covered by the `agentmcp` go-sdk client test and `agenttools_test.go` only. `request_branches` and `branch_status` run in `TestAdeBranchRequest`. Delete once one live run calls `declare_repos`.
- **Unsaved ADE workflow edits are lost when the whole Agents module is switched away (P196).** The leave guard covers list switch, import/new, mode toggle and shell tab change, not a module switch.
- **Change request facts correct on the next request, not by push (P189).** No server-to-client change notification exists; a client revalidation after `refsChanged` can hit a stale server entry, bounded by the TTL and the next refs change.
- **The graph keeps its pre-refresh scroll offset after an auto refresh (P203).** A new tip is off screen until scrolled or remounted.
- **`ui-timing` budgets fail in the WebKit sandbox even on an idle host (P250, P264, P266, P267, P277, P443).** Two Studio tests fail (`budgets.spec.ts` interactions, `perf.spec.ts` scroll); the rest of `ui-timing` passes. Measured idle (load below 2, no leftover process): scroll work p50 17-18 ms vs 16, `perf.spec.ts` scroll frame p95 89-96 ms vs 80, tab switch p95 up to 363 ms vs 300, cached tree expand p95 58 ms vs 50, console keystroke p50 55 ms vs 50. Cause: this host's WebKit runs JS work 1.8x and DOM work 2.9x slower than Chromium on the same host. The same code passes in Chromium with margin. Profiling found no hot-path work left to cut (SlickGrid render, keystroke and tree paths are already batched), so no `perf` change landed. Budgets are never loosened. P443 (2026-11-01): the same failure showed on base `6ba267359` (Playwright 1.63) at keystroke p50 53-56 ms; on Playwright 1.64.1 with an idle start (load 0.68 and 0.97) all four tests passed twice: keystroke p50 39.0 ms in the Before run, 36.0 ms in `bun run test:ui:timing:studio` (budget 50 ms, unchanged). Two passes do not clear the item; the drift from the P139 32-36 ms and the pass/fail swing between runs stay unexplained. `ui-timing` left `bun run test:ui:studio`; CI still runs it. Delete once the project passes on the CI runner, a Mac or a faster host.
- **Docker: remote `tcp://` without TLS is allowed (P200),** flagged `secure: false` in the UI. Exec sessions stay in the Terminal tab's chip list after the container stops; close the chip by hand.
- **Docker restore: the bridge-ID boot marker is unverified on Docker Desktop, Colima and OrbStack (P334).** Measured on Linux dockerd only. Check on a Mac: restart the VM with containers running, open Docker, expect the banner. Delete once confirmed.
- **Docker restore: engines without a default bridge network (P334)** detect a restart only when this session saw the outage. No launch-time detection.
- **Docker restore: a restart while Kira Studio was closed, then a deliberate CLI stop, is offered for restore (P334).** Indistinguishable from a casualty. Click-only, so harmless.
- **Visual baseline of the ADE workflow graph shows the Preview placeholder (P271).** The finish note is rendered by `LlmPromptsService.Preview`, unmocked in that spec. The PNG does not show the real note text.
- **Visual baselines are recorded in the dev sandbox (P227, P229, P242, P245, P247, P264, P266).** Accepted by the user. 13 Studio, 4 Space Settings, 3 git-module and 1 ADE workflow graph PNGs; the suites pass there. CI render may drift (0.6-1% on 13 specs before the P264 re-record); if the CI job disagrees, regenerate there and check each diff is the expected change only.
- **The phone sees desktop worktree paths (P231 B-4).** Board `branches[].worktree` is a local path served to paired phones. Open user decision; delete once decided.
- **Space repo-graph specs time out on first paint under heavy load (P230, P231, P243, P277).** Under 8 busy loops with `--repeat-each=40 --workers=8` (recipe in `docs/DEV_ENVIRONMENT.md`) app boot to graph row 0 takes 5-11 s against a 5 s default wait, 1.3 s idle. Failures: `repo-graph-refresh` scroll test 8+ of 40, `repo-graph-paging` 9 of 48 (`graph-svg` wait in `openRepo`), `repo-graph-columns` 3 of 400, `repo-workspace` 4 of 252. All pass idle (`test:ui:space`: 576 passed). Cause: CPU starvation of the whole app boot, not a test step. Delete once the boot is cheaper or the machine faster.
- **Jira wiki-markup descriptions and comments show as literal text (P428).** A non-ADF string body is escaped plain text, not converted.
- **Issue comments past 100, or past the newest 50 in the fallback, are not shown (P428).** The view says so and offers the tracker link.
- **Comments over the 1 MiB acli cap even through `comment list` are omitted (P428).** The view shows a note.
- **acli detail and comment-list fixtures are hand-written (P428).** acli does not run in the sandbox; the JSON shape follows the REST v3 issue shape.
- **Quitting Kira Space with an unsaved file edit discards it without asking (P419).** The quit confirm is computed in Go from agents, runs and scripts and has no renderer state. Closing the last window only hides it, so the draft survives that path.
- **A user-exported `CLAUDE_CODE_AUTO_COMPACT_WINDOW` overrides a step's auto compact limit (P431).** Claude Code reads the env var before `--autocompact`, and Kira passes its environment through, so the step setting has no effect while the variable is set.
- **The capture hotkey is unverified on a real macOS, Windows or Wayland desktop (P430).** Tests drive a fake shortcut manager. Chord registration, the macOS panel not stealing focus and the Wayland portal prompt need a manual run (plan checklist).
- **The Wayland state stays `requested` (P430).** The portal does not report the trigger the user bound, so Kira cannot confirm it works.
- **Capture reads clipboard text only (P430).** Images and files on the clipboard show the empty state.
