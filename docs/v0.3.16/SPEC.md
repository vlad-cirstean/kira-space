# v0.3.16

Release retention deletes whole releases instead of their assets; git tags stay. Phases from `P656`
(highest shipped: `P655`, `docs/history/07-v0.3.15.md`). Branch `chapter/v0.3.16`.

| # | Phase | Status |
|---|-------|--------|
| P656 | Release retention deletes whole releases, never tags | Planned |
| P657 | Memory tooltip Scripts category | Done |
| P658 | SQL data view column search | Done |
| P659 | Task card hover actions UX | Done |
| P660 Part 1 | Keep module state across tab switches | Done |
| P660 Part 2 | Git review panel state across module switches | Planned |
| P661 | Agents module memory efficiency | Done |
| P662 | Query module UI freeze | Done |
| P663 | Panel resize bar and update widget placement | Done |
| P664 | API tab bar plus and request input editing | Done |
| P665 | Cmd+F search in left panels | Done |
| P666 | Git and Memory modules last | Done |
| P667 | Single-select cards | Done |
| P668 | Single-select toggle groups | Done |
| P669 | Drop move up/down from left-panel menus | Done |
| P670 | Remove connection Secrets section | Done |
| P671 | Terminal links: copy via right click and reliable file paths | Done |
| P672 Part 1 | UI inventory and uniformity rules | Done |
| P672 Part 2 | UI uniformity: theme pieces and guards | Planned |
| P672 Part 3 | UI uniformity: forms and labels | Planned |
| P672 Part 4 | UI uniformity: buttons and dialogs | Planned |
| P672 Part 5 | UI uniformity: menus, popovers, tooltips | Planned |
| P672 Part 6 | UI uniformity: headings, detail headers, key-value, cards | Planned |
| P672 Part 7 | UI uniformity: status, tones, colours | Planned |
| P672 Part 8 | UI uniformity: tabs, segmented, choice | Planned |
| P672 Part 9 | UI uniformity: states, icons, type, spacing | Planned |

## P656 Release retention deletes whole releases, never tags

Retention (`scripts/releaseretention`, `internal/releaseline`, `.github/workflows/release-retention.yml`,
`docs/PACKAGING.md` §7, `docs/ARCHITECTURE.md`) today deletes release assets only. New rule: a
release that fails the keep rule is deleted whole (`DELETE repos/{repo}/releases/{id}`). Git tags are
never deleted: no `git/refs` call, no `--cleanup-tag`. Keep rule unchanged: each minor's `.0` (or
first release), last patch of each minor, last 5 patches of the current minor, `/releases/latest`.
Expected on the live listing (latest `v0.3.15`): delete `v0.3.4`, `v0.3.5`, `v0.3.6`, `v0.3.9`,
`v0.3.10`; keep `v0.3.3`, `v0.3.11` to `v0.3.15`. Check updater and `install.sh` behaviour against a
deleted release.

Plan: `plans/P656.md`.

Result: Done. Retention deletes each non-kept release whole through `releases/{id}`; tags stay. `releaseline.Release`
gains `ID`; `Delete` is one entry per release; TSV is `release id, tag`; workflow Apply rewritten; asset-less
releases failing the keep rule are deleted too. Guard grep (`git/refs`, `cleanup-tag`, `releases/assets`,
`release delete`) has no hits. Tests: `go test ./scripts/releaseretention/ ./internal/releaseline/` pass. Live read-only
dry run (latest `v0.3.15`) deletes exactly `v0.3.4`, `v0.3.5`, `v0.3.6`, `v0.3.9`, `v0.3.10` and keeps `v0.3.3`,
`v0.3.11` to `v0.3.15`: `total: 11 releases, 5 to delete (5 assets, 270 MB); tags kept`. `actionlint` not installed;
Apply script passes `bash -n`. Updater and `install.sh` unchanged. User step after merge: dispatch `Release retention`
with `dry_run=true`, confirm the 5 releases, then keep or set `RELEASE_RETENTION=apply`.

## P657 Memory tooltip Scripts category

Status-bar memory tooltip shows Agents category (CPU/RAM) but not script processes. Add a Scripts category next to Agents showing CPU/RAM of script processes, same behavior and layout as Agents. Acceptance: tooltip lists Scripts beside Agents with live values; tests per CLAUDE.md "Tests" (flow test on backend, UI spec on frontend, same scenario).

Plan: `plans/P657.md`.

Result: Done. `internal/procroots` (Agents, Scripts ref-counted root sets) replaces `internal/agentproc`. Scripts roots
tracked at terminal script runs (`OpenParams.Script`), headless script runs and DB pre-connect scripts. Sampler adds a
disjoint Scripts group, precedence Agents > Scripts > App; totals and headline are App + Agents + Scripts. Tooltip shows a
Scripts row between Agents and Total; hint reworded. Ran: `go test ./internal/metrics ./internal/terminal ./internal/scriptruns
./apps/kira-space/internal/preconnect` (incl. 4 new `TestGroupProcesses` cases); flow tests `TestAppMetricsCountsScripts`,
`TestAppMetricsCountsAgents`, `TestSQLitePreconnect*`; Playwright `ui` window-chrome metrics test and `visual` status-bar
test (no baseline change); `bun run typecheck` and `bun run lint` via pre-commit hook. Frontend bindings regenerated locally
(gitignored) for the hook's typecheck. Commits 2 and 3 of the plan's grouping landed in one commit.

## P658 SQL data view column search

Tables have dozens of columns; `apps/kira-space/frontend/src/studio/views/grid/DataView.vue` offers no way to find one. Add search/filter of columns in the SQL data view grid. Acceptance: user narrows visible columns by typing a name; clearing restores all; tests per CLAUDE.md "Tests" (flow test + UI spec halves).

Plan: `plans/P658.md`.

Result: Done. Filter toolbar gets a column-name field (case-insensitive substring, whitespace/comma terms OR-ed, PK columns kept, no match keeps all columns and flags the field). Display-only: no bridge call, no `tab.state`. Text lives in `DataViewRuntime.columnFilter` (P660 seam); `SlickGridHost` applies it via `visibleColumnOrder` after a `TIMING.search` (150 ms) debounce. Palette command `data.filterColumns`; row-search jump to a hidden column clears the filter first (`revealPageCell`, fixes page-vs-display index mix). No Go half: IPC untouched, UI spec asserts every wide-table read keeps `projection: null`. Ran: `bun test visible-column-order.spec.ts` (7 pass), Playwright `ui` `data-view-column-filter.spec.ts` (1 passed) and `data-view.spec.ts` (1 passed), `typecheck` and `lint` via pre-commit. Deviations: debounce 150 ms not 120 (guard U46 requires `TIMING`); status uses `common.ofTotal.label`, field placeholder "Column name", palette label "Filter grid columns" (catalog guards C3/C4 reject duplicate wording); added `columnFilter` to the runtime literal in `grid-pending-by-primary-key.spec.ts` (type).

## P659 Task card hover actions UX

Hover buttons on a task card appear mid-content and confuse. Acceptance: hover buttons align right; buttons that appear over the title move to a clearer place; menu button removed (same as right click); "no branch" shows once, not twice; agent indicator bigger (key info). Tests per CLAUDE.md "Tests" (flow test + UI spec halves where behavior is new).

Plan: `plans/P659.md`.

Result: Done. Card head and board branch row hover clusters moved to the right action zone
(`ade-card-actions`, `ade-branch-actions`); row pencil dropdown is `align="end"`. `ade-card-more` and
board `ade-branch-more` removed; right-click and Shift+F10 (new `menu.open` shortcut on the board row)
open the same menus; panel More kept. Draft row context reads once: `usePlanModel` takes
`line2.context`; `space.ade.baseMarker.draft.label` is now `Forks from {base}` (plan said `From {base}`,
`check-catalog` C3/C4 rejected it as a near-duplicate of other keys). Agent indicator: `AdeTaskSessions`
`size="lg"` (24px button, 16px glyph) in a left lead column spanning both head lines; `AdeActivityIcon`
gains size 16. UI only, no flow-test half. Ran: `typecheck:web` (only preexisting missing-bindings
errors without copied bindings; hook typecheck green), lint hooks, UI specs `ade-v2-card-icons`,
`ade-v2-row-actions`, `ade-v2-task-row`, `ade-v2-plan`, `ade-v2-session-status`, `ade-v2-add`,
`ade-v2-base-rebase`, `ade-v2-force-push`, `ade-v2-review-open`, `ade-v2-review-uncommitted`,
`ade-v2-dialogs`, `copy-wording*`, `ade-v2-panel` all pass. Visual: re-recorded `plan-branches-stacked`
and `plan-day-bars`; the other five baselines unchanged. Spec tweak: row actions right-edge bound is
60px (b_auth keeps tag and danger action after the cluster).

## P660 Part 1: Keep module state across tab switches

Switching modules/tabs (API, Studio, others) loses scroll position, search bars and similar view state. User reported before; still happens. Find the cause, fix for every module. Acceptance: after switching away and back, each module restores scroll, search text and comparable view state; tests per CLAUDE.md "Tests" (flow test + UI spec halves).

Plan: `plans/P660.md`.

Result: Done. Cause: module panels, start views and the ADE full view unmount on every module switch, tab views on every tab switch; only stores and `tab.state` kept state, and component refs (search bar visibility, per-surface text, list and editor scroll, roving row) were lost. Fix: session-only `useViewStateStore` (`packages/workbench/src/state/viewState.ts`, LRU 400 keys, tab keys drop on tab close) with `useKeptState`, `useKeptScroll`, `useKeptEditorScroll`; `useVirtualRows`/`useTreeVirtualRows` take `keepScroll`; `VirtualList` takes `keepScrollKey`; `usePanelHeaderSearch` takes required `stateKey` (a non-empty text reveals the bar). Applied to Studio, API, Git, Automations, Memory, Docker, Kubernetes and Agents surfaces per `plans/P660.md`; no KeepAlive. Guard U109 plus three PERMANENT lines (RepoFileTree derived debounce copy; git-ui `FileTree.vue`, `ReviewView.vue` stay on `stateKey: null`, Part 2). ARCHITECTURE bullet added. No Go half: state is frontend-only, never over IPC; no flow test. Ran: `bun run lint` (clean), `bun run lint:dead` (clean), typecheck via pre-commit on every commit; Playwright `ui`: new `module-state.spec.ts` (5 tests) and `studio/module-state.spec.ts` (4 tests) all pass, with restore disabled 4 of 5 space tests fail (specs are not vacuous); existing `tab-filter-state`, `modules`, `repo-graph-keep-state`, `data-view-column-filter`, `docker-module`, `kubernetes-module`, `collections`, `memory-module`, `automations-module`, `git-panel-tab`, `ade-v2-needs`, `ade-v2-plan`, `repo-config-file`, `http-request`, `definition`, `cell-editor`: 142 passed. Terminal viewport scroll survives a round trip already (module-level `terminalRenderer` map), so `terminalRenderer.ts` is untouched. Deviations: Repos list scroll kept on `GitPanel`'s `<section>` (the scroller; `RepoList`'s `listEl` is not); `ResponseFindBar` callers also include `HttpRequestView`, `DefinitionView`, `CellEditorView`; gRPC keeps find, find query and message-list scroll, not per-message editor scroll or the expanded set; also kept: docker logs `findOpen`, ADE All sessions filter, Memory/Automations roving key (Memory's selected-id watch no longer clears it), docker exec session, kube log container (default only when the kept one is gone); `useKeptEditorScroll.initial` is a per-key snapshot (Monaco emits scroll 0 before restore); `BrowseView.vue` loses a stale P104 comment and `HttpRequestView.vue` uses a literal `tab:` key to stay under the N10 600-line cap; `headerNames` moved to `support/grid.ts` (guard N8). Not covered by a spec: Studio documents scroll, kube resource-table and cluster-list scroll, docker container-table scroll, ADE panel body scrolls (same seams as the covered lists). Git and Terminal specs use Automations as the away module.

## P660 Part 2: Git review panel state across module switches

Git review segment (`RepoReviewView.vue` hosting git-ui `ReviewView`) loses its in-flight review, file filter and scroll on module switch. git-ui is its own Vue app with no Pinia (`packages/git-ui/src/main.ts` `mount`, `NullViewStateStore`). Fix changes git-ui's mount contract so it keeps view state across module switches. Part 1 passes `stateKey: null` for git-ui's `usePanelHeaderSearch` callers (`ReviewView.vue`, `FileTree.vue`). Acceptance: after switching away and back, review segment restores in-flight review, file filter and scroll; own plan `plans/P660-part2.md`, UI spec, same rules as Part 1.

Plan: `plans/P660-part2.md`.

## P661 Agents module memory efficiency

App uses ~1.1 GB RAM with no other module opened and few tasks and sessions. Make the agents module very memory efficient. Acceptance: measure before and after, name the biggest retainers found, fix them, report the reduction in Result.

Plan: `plans/P661.md`.

Result: Done. Fixture: `ade-agents` world (`heap-ram.spec.ts`: 10 tasks, 200 archived, 300 stopped rows, 4 running agents at 2 MB each, 2 panes, 10 switches, 50 pushes, one approval), Chromium, PSS from `/proc/<pid>/smaps_rollup` (descendants of the Playwright pid only). `memory-mcp` fixture: real binary, ORT 1.31.1, arctic-embed-s int8, 4 `memory-mcp` plus one search each (`/tmp` script, not committed). Same fixtures before and after.
Named retainers, ranked by measurement:
1. H1 one `memory-embed` per session: 4 workers, 431 MB PSS (936 MB RSS). F1 shared worker over `embed.sock` (flock spawn, idle exit): 1 worker, 134 MB PSS (241 MB RSS). `memory-mcp` plus `memory-embed` total PSS 598.8 to 335.3 MB (-263 MB, -44%), RSS 1562 to 875 MB.
2. H4 Monaco after an agent approval: renderer PSS 328.2 MB at the approval step, a +107 MB jump (JS heap 48.7 to 56.8 MB). F5 loads Monaco only when a pending approval's kind renders in it (`monaco` flag on the kind def). After: 189.3 MB (-139 MB, -42%); JS heap 46.4 MB (-10 MB).
3. H3 xterm buffers: typed arrays 33.2 MB (4 sessions x 5000 lines at 200 cols), HEAPSNAP total 97.8 to 80.4 MB, typed arrays 8.2 MB (-25 MB). F4: agent scrollback 1000 and `term.open` only on first view (a never-opened session renders no DOM). Renderer PSS after `launch4` 205.5 to 166.3 MB; at `final` 221-223 to 185.2 MB (-37 MB, -17%), before the approval step.
Total, renderer at the approval step plus `memory-mcp`/`memory-embed` (two separate fixtures, summed): 927 to 525 MB PSS (-402 MB, -43%).
Dropped as measured and below the gate (5% or 20 MB): F2 sessions bound (300 rows = 122 KB JSON per push); F3 history out of board (200 entries = 33 KB JSON, 50 pushes grew Go total 34.4 to 38.4 MB); F6 app `SetMemoryLimit` (live heap 8.4 MB, total 34-38 MB); `memory-mcp` `SetMemoryLimit`/`GOGC` (heap objects 16 MB, total 32 MB). F4 focus-only `cursorBlink` dropped: xterm's DOM renderer blinks by CSS animation on the focused row only, no timer per terminal; toggling the option on focus broke Cmd-click link activation (`terminal-file-links.spec.ts`). No top-3 retainer outside the plan list, no P661-iter2.
Added: `KIRA_PPROF` loopback pprof (app and `memory-mcp`), `ade-agents` RAM world, "Agents module footprint run" in `docs/DEV_ENVIRONMENT.md`.
Ran: `go test ./internal/memory/embed/...` (shared worker race, one spawn, idle exit, respawn), Playwright `ui` `approvals-payload.spec.ts` (ADE approval leaves `MonacoEnvironment` unset, db.query defines it), `ade-terminal-lazy.spec.ts` (never-opened agent terminal has no `.xterm`, pane shows earlier output), terminal link, takeover, sessions, automations and repo-workspace specs (62 passed), typecheck, lint, knip via pre-commit.
Deviations: no `memoryflow` two-process flow test (needs the real model, ORT and binary, absent in the flow tier); covered by the `embed` race test and the real-binary run above. No `sessions_bound` or `history` flow tests and no `ade-v2-history` spec (F2, F3 dropped). Not measured here: Mac `footprint` per process, WebKit WebContent PSS, tooltip App/Agents totals, Go app `pprof` on a real run. User run: `docs/DEV_ENVIRONMENT.md` "Agents module footprint run (user-run)"; WebKit sandbox proxy `KIRA_RAM_WORLD=ade-agents bunx playwright test --config=apps/kira-space/playwright.perf.config.ts --project=heap-webkit heap-ram`.

## P662 Query module UI freeze

Some queries freeze the whole UI before data shows (unknown whether data arrives). Opening a new request from the menu bar unfreezes it, until returning to Studio. Find the root cause and fix it for all modules and places in the app. Acceptance: no query freezes the UI; root cause named in Result; tests per CLAUDE.md "Tests" (flow test + UI spec halves).

Plan: `plans/P662.md`, `plans/P662-iter2.md`.

Result: Done. Root cause: result views did work proportional to page size or cell size on mount and on every re-render. H1: `ConsoleResultGrid` parsed every document body of the page (`JSON.parse` plus DocNode tree) in one synchronous flush on mount and on every `pageVersion` bump, because the virtualizer prune dropped exactly those caches outside the window. H1b: tabular, key/value and stream cells put up to 64 KiB of text each into the DOM and WebKit shaped every clipped run. H1c: column width sampling measured whole cells. Leaving Studio unmounts the views (freeze lifts); returning remounts onto the cached page (freeze returns). Fixes: F1 window-only document row views and heights (`documentId`, `resultDocExpandedIds`); F2 `displayText` cap of 2 000 chars in console grid, data grid, key/value (console and tab) and stream cells (editor, copy, search keep full text); F3 `cellPrefixText`, widths measured on 512 bytes. Rejected: H2 transport (unmounted page landing: documents 61-92 ms, tabular 69-103 ms max gap), H3 (`body` pointer-events never `none`), H4, H5 (flow test pins caps and cancel). Numbers are WebKit (Playwright `ui`), max rAF gap, budget 250 ms, on a shared 4-CPU host (load swung 2-19; noise floor with 16-byte cells: data grid 95-106 ms). Before then after: console documents arrival (10 000 x 2 KiB) 1273-1309 ms then 92-157 ms; documents remount 1203 ms then 110-166 ms; console tabular arrival (250x4x16 KiB plus 50x64 KiB truncated) 595-620 ms then 203-218 ms; data grid load (150x4x16 KiB) 559-607 ms then 161-237 ms; key/value scroll 343-365 ms then 101-141 ms; stream case passes before and after (regression guard only). Flow test `TestConsoleLargeResultCapped` (12 000 rows of 80 000 chars: 896 rows, cells at 64 KiB, truncated) and `TestConsoleSlowQueryResponsiveAndCancellable` (new `DataStream.ExecuteFire`) pass. UI spec `console-large-result.spec.ts` 8 of 8 pass 3 runs in a row; `console`, `console-copy-all`, `console-format`, `console-script`, `data-view`, `cell-editor`, `document-edit`, `document-view-readonly`, `redis-edit`, `stream-copy`, `stream-produce`, `sqs-poll-visibility` pass. Deviations: fixtures smaller than the plan (tabular 250 rows, 150 rows for grid/key-value/stream) because the mock harness transport of 38-64 MiB alone cost 175-270 ms of the 250 ms budget; scroll uses 8 wheel steps; cancel case asserts Stop going idle (no result exists to carry a "cancelled" label). Pass 2 (Documents tab, 10 000 x 2 KiB page): root cause `DocumentView` `rowHeights` mapped every row of the page; every document defaults to expanded, so each recompute decoded every id and body (`documentRow`) and, on a miss, parsed every body (`JSON.parse` plus DocNode tree) for an exact line count, all inside one synchronous computed read by the virtualizer's `overscan`; a smaller share was `fieldNamesOnPage` (decode and `JSON.parse` of every body, read at mount by toolbar and filter row). Console "Expand all" ran the same whole-page `rowHeight` pass. Fix: heights stay exact (no measurement) but come from three sources: window rows exact on the main thread (`fillLineCounts`), whole page counted in the parse worker (`doc.pageStats`: top-level line counts plus field names, `pageStatsStore`), rows neither has reached estimated by the running mean (`knownRowHeight`). Ids decode alone (`documentId`), and not at all while nothing is collapsed. `useHeightAnchor` keeps the first visible row fixed while counts above it change. Same mechanism in `ConsoleResultGrid` (`useDocumentResultHeights`). Deviation from `P662-iter2.md`: the job is sliced into 1 MiB batches (one `doc.pageStats` job per slice, counts primed per batch at their row offset, field names unioned at the end) because one `postMessage` of the whole 20 MiB page blocked the main thread 87-253 ms (structured clone copies the buffer behind the typed array); with slices the largest synchronous step is a 1 MiB copy. Before then after, WebKit max rAF gap, budget 250 ms, 4-CPU shared host: documents tab arrival (reply held 1.5 s, probe reset after mount) 1326 ms (3 plan repro runs 857-957 ms from reply to first row plus 1.5 s) then 129-192 ms (4 runs); console documents expand all 748 ms then 77-104 ms. Documents tab scroll 97-118 ms, collapse all 109-194 ms, expand all 68-124 ms, row toggle 33-46 ms, all under budget; first row height equals 26 + fields x 18 + 8 and the next row starts at the toggled row's end. Both new cases (`documents tab stays responsive on a 10,000-document page`, and the `console-expand-all` step) failed on the pass-1 tree first. `console-large-result.spec.ts` 9 of 9 pass; `document-row-height-cache` unit spec (estimate, fill, prime, stale generation, `documentPageStats` equals the main-thread count and the old `fieldNamesOnPage` rule) passes. Measured, out of scope: opening a Documents tab costs 230-315 ms max gap for any page size (20 rows included), so it is tab mount (filter-row editors, toolbar), not page-size work; pass 1's case reset the probe before the dblclick and could never pass at 250 ms, so the re-added case lets mount settle while the reply is held. Needs its own phase if wanted. Not verified: Chromium or real WKWebView/WebKitGTK app, Kibana/proof-window path (shares `ConsoleResultGrid`, not driven), HTTP and logs modules (audited unchanged), key/value console branch in a spec.

## P663 Panel resize bar and update widget placement

Resize bar between panels is strong blue; app-update version widget sits on the status bar's right. Acceptance: (1) resize bar uses a neutral grey theme token (muted, no blue) in idle, hover and drag states; hover and drag stay distinguishable from idle by a step in grey; (2) update version widget moves to the status bar's left side, right side loses it; UI specs cover bar color per state and widget position; visual baselines re-recorded where changed.

Plan: `plans/P663.md`.

Result: Done. Tokens `--kira-resize` (= border), `--kira-resize-hover` (#52525b), `--kira-resize-active` (= fg-subtle) plus `bg-resize*` utilities; `resizeHandleVariants` uses grey hover and drag, keeps `bg-focus` on keyboard focus only. SlickGrid column handle and Monaco `sash.hoverBorder` use the same tokens. Update item moved first in the status bar left group; groups carry `status-bar-left`/`status-bar-right` test ids. ARCHITECTURE lines updated. No Go half: UI-only, no IPC. Ran (Chromium, Playwright `--trace off`): `ui` `resize-handle-color.spec.ts` (5 new: project panel, ops dock, line handle, Monaco sash, SlickGrid column) and `update-dialog.spec.ts` (position assertions added): 14 passed; `visual` `status-bar`: `status-bar-visual-linux.png` re-recorded (update item left), tooltip and popover shots unchanged, 3 passed after; `typecheck`, `bun run lint:dead` clean, pre-commit hook clean. Deviations: the status bar move landed in the first commit, not its own.

## P664 API tab bar plus and request input editing

(1) API module tab bar gets a "+" button for a new request tab, like the Runs module tab bar. (2) Request param/header (key-value etc.) inputs misbehave, at least for multiline values, possibly single-line too (untested): left/right arrow navigation broken, caret stops on spaces not in the actual string or moves more than one character. Find cause (likely rendering overlay/mirror or custom editor), fix for every such input in the API module. Acceptance: arrow keys move one character, caret positions match the string, single and multiline; "+" opens a new request tab like Runs. UI-only, no backend: UI specs for both (plus, caret navigation per input kind).

Plan: `plans/P664.md`.

Result: Done. Part 1: new `studio/api/ApiNewTab.vue` (Runs pattern: `TabStripNewButton` plus context menu, New request and New gRPC request, wording shared with the collection menu), wired as `newTab` on the `api` mode; Databases strip unchanged. Part 2 root cause class: two independent text layouts. `AutocompleteField` ranges-only mode painted glyphs in an overlay div and left the caret to a transparent native control, so any per-glyph advance, wrap or scroll difference between the two moves the caret off the visible text. The drift did not reproduce on WebKit WPE or Chromium here (arrows moved `selectionStart` by exactly 1 everywhere measured); it is unverified on the shipped engines (WKWebView, WebKitGTK), so the fix is by construction, not confirmed against the user's macOS report. Fix: ranges-only mode now uses an underlay (`is-underlay`): the native control paints glyphs (`text-fg`) and caret, the overlay only tints `{{var}}` behind it; ligatures off on control and overlay; Monaco loads only for language-highlighted fields. Measured or code defects also fixed: CR/CRLF in values (grow replica read CR as a space: box too short, hover offsets off by one per CRLF) now normalised for display only (`displayText`); `FieldRowsTable` sent ArrowUp/ArrowDown to the next row inside multi-line or wrapped cells, now row stepping applies to one-line cells only. Visible change: `{{var}}` glyphs render in the foreground colour on a tint (secret dotted, unknown wavy underline kept) instead of coloured text. Also applies to the Runs script body field (same mode). Ran: Playwright `ui` (WebKit) new `api-field-caret.spec.ts` (8 kinds: computed-style ownership guard plus real ArrowRight/ArrowLeft walks, multi-line Up/Down, CR box height) and `api-tab-strip-new.spec.ts`: 10 passed; existing `http-request`, `api-ui-consistency`, `http-variables`, `automations-module`, `tabs-scope-db`, `module-state`, `autocomplete`: 86 passed; visual `http-request-view` and `workbench` pass unchanged (the screenshot covers neither the strip nor a variable), no baseline re-recorded; `lint:dead` clean. Deviations: no unit test for `paintRangesHtml` (thin wrapper); specs prove the layout ownership, not a WKWebView drift fix.

## P665 Cmd+F search in left panels

Focus in a left panel (Connections, Collections, etc.) plus Cmd/Ctrl+F opens that panel's search bar and focuses it, typing starts at once. Acceptance: every left panel with a search bar; palette/shortcut registry entry; UI spec per panel. UI-only, no backend.

Plan: `plans/P665.md`.

Result: Done. New shortcut `panel.find` (scope `panel`, same chord as `view.find`), routed by focus: `usePanelHeaderSearch` binds it on the panel root inside `data-slot="side-panel"` (innermost searchable panel wins), plus a module registry behind `openFocusedPanelSearch` (menu Find event in `App.vue`, falls back to `view.find`) and `openActivePanelSearch` (palette entry `panel.find`, "Search panel"). Palette commands with `movesFocus` run on the dialog's close-auto-focus (`CommandDialog` forwards it) so focus restore cannot undo them. Six toggle buttons print the chord. Ran (Playwright `ui`, WebKit): new `panel-find.spec.ts` 7 of 8 pass (Automations, Memory, Git Repos, Files, Files>Search, menu, palette), `studio/panel-find.spec.ts` 5 of 5 pass; `consistency`, `module-state`, `studio/module-state`, `studio/collections` pass. Unverified here: Git Review case and `repo-review-interaction.spec.ts` (11 tests) fail in this sandbox at `openReviewListing`, also at base commit `10066c82` (`consistency-git` review row), so the review-file-filter path is covered by CI only. macOS WKWebView menu-accelerator versus keydown ordering is unverified here (Linux WebKit, no native menu); the menu path and local chord both call the idempotent `openSearch`, so either order converges. Deviations: Studio and API panels hide the bar when empty, so `searchable` is now `!empty` (Cmd+F then falls through to `view.find`); `consistency.spec.ts` find test clicks the diff editor first, since Find with focus in the left tree now opens the panel search; UI helper `support/panelFind.ts` presses Meta or Control by UA (Linux WebKit reports Mac).

## P666 Git and Memory modules last

Move the Git module to the second-to-last position and Memory (when on) to the last in the module switcher/navigation: sidebar order, shortcut numbering if tied to order, persisted order defaults. Acceptance: resulting order stated in Result; shortcuts and docs updated; UI spec. UI-only, no backend.

Plan: `plans/P666.md`.

Result: Done. `MODE_ORDER` (`workbench/modes.ts`) is now ade, automations, studio, api, docker, kubernetes, git, memory. Memory shows only under `experimental.memory`, so the default order ends with Git. Scope grew mid-phase (user): Memory also last, noted in the plan. No shortcut renumbering existed: no per-module chord, so nothing to renumber. Default module stays Git (by id: `WindowModes.Default`, `createModeStore`, memory-gate fallback); no Go change, no persisted order, no migration. `modules.spec.ts` order tests assert the new order (Git `nth(-2)`, Memory last, Git still `is-active`): 5 of 5 pass. Docs updated: ARCHITECTURE, README, app README, site how-it-works. Visual baselines re-recorded with Memory off (Git icon now last): `new-connection-dialog`, `console`, `data-grid`, `schema-dialog`, `workbench-shell`; `ade-repos-page` unchanged. All six pass on rerun. Besides icon order, a faint sub-pixel diff (max 41 of 255) sits on the focused primary button in three baselines, and a thin one on the data-grid row; neither is visible on inspection.

## P667 Single-select cards

Round RadioGroup becomes shadcn card-style radio: whole card is the click target, title plus description, selected state clearly visible (border and tint from theme tokens). Keyboard and aria radio semantics stay via RadioGroup. Surfaces: Reset mode (`packages/git-ui/src/components/dialogs/ResetDialog.vue`, soft/mixed/hard with descriptions), Review route (`apps/kira-space/frontend/src/ade/v2/review/AdeWorkflowReviewPanel.vue`), mainline parent pickers (`CherryPickDialog.vue`, `RevertDialog.vue`). One shared theme component for card radios (`packages/theme/src/components/ui/radio-group` or a sibling), Tailwind only. Existing data-testids kept. Acceptance: states stated (idle, hover, selected, focus-visible, disabled); existing UI specs that click these radios updated; visual baselines re-recorded where changed. UI-only, no flow half.

Plan: `plans/P667.md`.

Result: Done. New theme `RadioGroupCard` (reka `RadioGroupItem` as the card: title and description slots, idle/hover/`border-focus bg-select`/focus-visible/disabled from tokens, Tailwind only). Used by Reset mode (`reset-mode-soft|mixed|hard`), Review route (new descriptions, verified against `sendReview`: implementer resumes the picked run's agent session, new starts or resumes the dedicated review session), Cherry-pick and Revert mainline (`cherry-pick-mainline-N`, `revert-mainline-N`, new `revert-confirm`; catalog `git.common.mergeParent.option` is now "Parent {number}", sha plus subject move to the description). Review `NativeSelect` moved out of the radio group. UI-only, no flow half. Ran: Playwright `ui` `git-choice-cards.spec.ts` (3 passed), `ade-v2-review-step.spec.ts` (new route-card test, all pass), `copy-coverage-git.spec.ts` (all pass except `review view uses catalog text`, which times out in `openReviewListing` with an empty Review sidebar; fails identically on an untouched checkout, no radio involved, not fixed); visual: re-recorded `review-step-panel`, added `git-reset-dialog`; `lint:dead`, typecheck and lint via pre-commit. Deviations: Revert sha heading uses `FieldDescription` (guard U95 rejects a hand `<p>` hint); revert spec tabs until the first card is focused (dialog focuses its own first control); full `visual` project and `copy-wording-git.spec.ts` not run.

## P668 Single-select toggle groups

Round RadioGroup becomes ToggleGroup (type single, segmented, solid selected fill). A value must stay selected: a click on the selected item must not deselect. Surfaces: Runs params `select` type (`packages/workbench/src/automations/run/ParamsForm.vue`; wrap for many options, Select past ~5 options), `RunMcpTab.vue` (both groups), `ClusterMcpDialog.vue` (`apps/kira-space/frontend/src/studio/kubernetes/components`), `RunFolderTab.vue`, `AdeAddPopover.vue` (kind), `AdeStageDialog.vue` (close/keep), `WorktreeDialog.vue`, `StashDialog.vue` (save source). Stay radios: `ConnectionEnginePicker`, `AdeTaskStatuses` colours, `EnvironmentsView` active env. Existing data-testids kept. Acceptance: states stated (idle, hover, selected, focus-visible, wrapped, Select fallback); existing UI specs that click these radios updated; visual baselines re-recorded where changed. UI-only, no flow half.

Plan: `plans/P668.md`.

Result: Done. `SecondaryTabs` gains `variant="choice"` (outline strip, selected `bg-select`, idle muted) and `wrap` (spaced, wrapping chips); it stays the single ToggleGroup wrapper (U4) and still drops the empty update, so a click on the selected item changes nothing. Nine groups moved from RadioGroup in 8 files: `ParamsForm` (select param; `NONE` sentinel for None; `NativeSelect` past 5 choices, None counted; required select's None option disabled), `RunMcpTab` (run, mode), `RunFolderTab`, `ClusterMcpDialog`, `AdeAddPopover` (kind), `AdeStageDialog` (close/keep, new testid `ade-stage-close-choice`), `WorktreeDialog` (`worktree-mode*`), `StashDialog` (`stash-save-source*`, `wrap`, entry item has a tooltip, untracked Alert moved below the group). Existing testids kept; ConnectionEnginePicker, AdeTaskStatuses colours, EnvironmentsView stay radios; ResetDialog, CherryPickDialog, RevertDialog, AdeWorkflowReviewPanel untouched (P667). Keyboard behaviour change: ToggleGroup follows reka, so arrows move focus only (looping), Space/Enter select; radios moved and selected on arrows. Screen readers announce pressed toggle buttons, not radios; `data-state` is `on|off` (specs use `data-state`/`aria-pressed`, not `toBeChecked`). No flow half, UI-only. ARCHITECTURE: `choice`/`wrap` rule, Runs params rule, stage dialog and `AdeAddPopover` wording. Ran: Playwright `ui` (webkit, `--trace off`): new `single-select-toggles.spec.ts` (3 passed: RunFolderTab, WorktreeDialog, StashDialog), `automations-mcp.spec.ts` (12 passed, new keyboard test), `studio/kubernetes-mcp.spec.ts`, `ade-v2-orchestrator.spec.ts`, `ade-v2-stage-move.spec.ts`, `automations-smart.spec.ts` (scenario, new 5-vs-7 choice test, selected fill `rgb(31, 51, 84)`), `repo-script-picker.spec.ts`: 47 of 47 passed in that run; `bun run lint:dead` clean; typecheck and lint via pre-commit. Deviations: `scripts/check-ui-primitives.sh` gets one U37 exemption for `SecondaryTabs.vue` (U37 bans `bg-select` outside `rowVariants.ts`; the choice fill is a toggle, not a row), not in the plan's ownership table; commits 1 and 2 landed as one commit (a failed hook left the files staged); automations-mcp keyboard test starts on `short` (the `mcp` form answer has no mode) so it presses ArrowRight then Space as planned; kube-mcp re-click asserts `on` rather than a second Save arg. Not run locally (PR CI): `copy-coverage*.spec.ts`, `ui-leftovers.spec.ts`, `consistency.spec.ts`, visual baselines (none expected to change).

## P669 Drop move up/down from left-panel menus

Right-click menus in the left panels (Studio Connections, API Collections, Kubernetes clusters, any other left-panel tree) have Move up / Move down items. Drag and drop already covers reordering. Remove the items and their keyboard shortcuts, commands, palette entries and i18n keys, plus backend or state code only they used. Keep what drag and drop still uses (e.g. `orderAfterDrop`; `canMoveStep` only if still used). Acceptance: no Move up/down in any left-panel menu or the shortcut registry; drag reorder still works; UI specs of removed items removed, drag specs kept; `docs/ARCHITECTURE.md` and shortcut docs updated. UI specs; flow tests only where backend code is removed (planner states if none needed).

Plan: `plans/P669.md`.

Result: Option A. Removed Move up/down menu items and Alt+Up/Down bindings from every left panel: Studio Connections, API Collections, Kubernetes clusters, Automations (scripts and collections, both apps) and the Git Repos list. Deleted the code only they used (`moveConnection`, store `moveStep`/`canMoveStep`, `collectionOrder.ts`, `runMenuShortcutAsync`, menu action members, `list.moveUp/Down` from `TREE_IDS`). `SortableList` takes a `keyboard` prop (default true); Automations passes false. `list.moveUp/Down` registry rows stay, since Alt+Up/Down still reorders the four non-left-panel lists (grid Columns menu, API Environments, ADE later shelf, ADE task statuses); the acceptance clause "not in the shortcut registry" is therefore only partly met. No i18n key removed (`common.moveUp/moveDown.tooltip` still used by ParamsEditor and AdeStepNode). UI specs rewritten to assert no Move items and no reorder call on Alt+ArrowDown, drag specs kept; `collectionStepOrder` unit test removed. No Go change, no flow test. Ran: `api-collection-drop` unit, UI (WebKit) automations-order, script-order, connection-order, api-order, kubernetes-order, repo-order, tree, ade-v2-task-status: all pass; `lint:dead` clean. Keyboard-only reorder is gone in left panels.

## P670 Remove connection Secrets section

Connection edit dialog shows a Secrets block on every tab; user finds it confusing and does not know its purpose. Planner first establishes what it does (`apps/kira-space/frontend/src/studio/project/connection/ConnectionSecrets.vue`, `ConnectionSecret` in `packages/shared/domain/connectionConfig.ts`, `revealConnectionSecret`, `deleteSecret`, their Go side) and whether any user flow depends on it (how passwords/URIs are stored, where else they are edited). Then remove the UI block and every code path only it used, keeping whatever stores/edits the connection password elsewhere. If removal loses a capability users need (no other way to set/clear a password or URI secret), the plan says so prominently and proposes the minimal alternative; the orchestrator confirms with the user before implementation. Acceptance: Secrets block gone from every tab; connections still save and connect with and without password; specs updated (credential-reveal, confirm-focus etc.); docs updated. UI specs plus flow tests where backend code is removed (planner states if none needed).

Plan: `plans/P670.md`.

Result: Done. Option A, user confirmed: `ConnectionSecrets.vue` removed from every tab, with `useDeleteConnectionSecret`, `connectionConfigDeleteSecret`, Go `ConnectionConfigService.DeleteSecret` and `ConnectionSecretNameArgs`, five i18n keys. Clear is lost; no UI deletes a connection secret. General tab gains a storage-off hint (`connection-secret-storage-off`). Password and URI entry, reveal and set state stay on the General tab. Known open item added: orphaned sealed values. Flow `TestConnectionSecrets` calls `Loader.DeleteSecret` directly; contract keys unchanged. Specs: `secrets.spec.ts` rewritten for General tab, `credential-reveal`, `secret-reveal`, `confirm-focus`, `connection-form` retargeted. Ran: `go test ./internal/flows/dbflow -run 'TestConnectionSecrets|TestSecretsStatusAndReveal'` ok, `go vet` bridge and dbflow clean, `lint:dead` clean, Playwright `ui` five specs 20 passed, pre-commit full lint and typecheck green. Deviations: flow test edit shipped in commit 1 (keeps Go tests compiling); `golangci-lint` not run (binary built with go1.25, repo targets newer Go); `wails3 task common:generate:bindings` regenerated bindings (gitignored).

## P671 Terminal links: copy via right click and reliable file paths

Two fixes. (1) In terminal output a URL link opens only on Cmd-click. Right click on a link must show a menu with Copy link and Open, as in other terminals. Same for file path links: Copy path, Open. (2) File paths in terminal output do not always become links. Planner finds why (`packages/workbench/src/terminal/fileLinkProvider.ts` `findFilePaths`/`resolve`, `apps/kira-space/frontend/src/state/terminalFileLinks.ts`, Go `openedFilesTerminalResolve` against the terminal's cwd) and fixes the misses. Shapes to check: relative paths, `./` `../` `~`, `path:line:col`, paths wrapped across rows, paths inside quotes and parens, cwd of agent and script terminals, exited terminals. Acceptance: right-click menu on web and file links; a table of path shapes that must link in the plan, each covered by a test (unit test for the path finder if it has interacting rules, flow test for the resolve backend, UI spec for the right-click menu and a clickable path); no regression of Cmd-click.

Plan: `plans/P671.md`.

Result: Done. Causes of missed paths: `Resolve` required the OS extension vocabulary even inside imported repos (`Makefile`, `notes.txt`, `.env.example` never linked); one cwd only (live cwd won, so a path relative to the start folder failed after `cd` or while a subprocess held the foreground); grammar gaps (`:L-L2`, `#L12`, `(12,5)`, Python `", line N`, quoted or escaped spaces, `file://`, git diff `a/`/`b/`, Unicode framing). Fixes: `filePaths.ts` grammar rewrite; `openedfiles` `resolve(..., anyTypeInRepo)` (vocabulary applies only outside repos), `terminalCandidates`/`firstOpenable`, `Openable(cwds, ...)` returns absolute paths, `OpenTerminalPath` replaces `OpenAt`; bridge `termCwds` (live cwd, then start cwd), `TerminalResolve` result gains `paths`; frontend `resolve` returns absolute path or null. Right-click menu (`linkMenu.ts`, capture-phase `contextmenu` via VueUse, existing `ContextMenu.vue` store): Open and Copy link for web and OSC 8 links, Open and Copy path (absolute, no `:line`) for file links; link activation now primary button only. No Reveal (no bridge for arbitrary paths). Ran: `bun test terminal-file-paths.spec.ts` (48 pass); `go test ./apps/kira-space/internal/flows/windowflow -run TestTerminalFileLinks` (11 subtests pass), `go test ./apps/kira-space/internal/openedfiles/... ./apps/kira-space/internal/bridge/...` pass; Playwright `ui` `terminal-file-links`, `terminal-links`, `ade-terminal-lazy` (19 passed); `bun run lint:rules`, `lint:dead`, typecheck clean. Contract `terminal-file-links.json` regenerated (adds `paths`). Deviations: after the menu closes xterm does not re-hover the same cell, so specs move to another cell of the link before the second right-click; hint stays hidden until the pointer enters another cell. Not verified here: macOS behaviour (real Cmd+right-click and trackpad two-finger click, native contextmenu timing); the UI harness fakes a macOS user agent on Linux Chromium.

## P672 Part 1: UI inventory and uniformity rules

User feedback: nothing in the UI is uniform. Not only per module: each dialog, menu, card in the same module differs in components and design. "Elements" means more than dialog and card: how information shows inside them. Covers alignment, colors, fonts, sizes, spacing, labels, icons, empty/loading/error states, headers/footers, button placement and order, form layouts, lists/tables/rows, tooltips, badges. Part 1 is analysis only, no UI change. Deliver: (1) exhaustive inventory of element types and each variant actually used, with counts and file references (via codegraph), grouped per element type; (2) a small set of canonical patterns; (3) a mapping of every variant to its canonical pattern, or a justified exception. Output committed under `plans/`: inventory, canonical rules, migration list. From the inventory the planner decides how to cut Part 2 into Parts 2, 3, ... by element type or module, each sized for one planner and one implementer; the orchestrator then adds them as rows. Acceptance: exhaustive (every dialog, menu, card, list, form in every module appears) and canonical rules concrete enough to review (token values, component to use, markup shape). Tests: none (docs).

Plan: `plans/P672.md`.

Result: Done. Inventory `plans/P672-inventory.md` (716 `.vue` files, 14 modules; codegraph walks plus template and class-token scans at `6527aec4`). Finding: primitives are uniform and lint-held; divergence sits in composition (primitive picked per role, module-local wrappers, free spacing/type, two colour vocabularies). 16 families with canonical rules (RD dialogs to RTa tabs), ranked top 12 inconsistencies (labels in two colours and five layouts; running = info in Runs vs warn in ADE and ADE pastels leaking into Runs; 11 hand detail headers; three button looks for in-view actions; 24 popover class variants), migration items M1-M36, appendix A.1-A.13 listing every dialog, popover, card, heading, menu, list, form and tabs host. Apply work cut into Parts 2..9 (plan `plans/P672.md`): Part 2 theme pieces and guards first, then sequential; optional batch pairs {5, 6} and {7, 8}. Decisions for the user: D1 ADE palette scope (blocks Part 7), D2 new shared pieces (`DetailHeader`, `Field orientation="responsive"`, `PopoverContent flush`, `KeyValueList density`, `DetailSection`/`TextTooltip` in theme), D3 toast scope (blocks Part 9 toast item). No app code changed; no tests (docs).

## P672 Part 2: UI uniformity: theme pieces and guards

New shared theme pieces and lint guards every later Part builds on (`plans/P672-inventory.md` §20 M1-M3). Theme: `Field orientation="responsive"` (label column 7.5rem at `@24rem`, stacked below), `PopoverContent flush`, `KeyValueList density="compact"`, `DetailHeader` composite (icon 16, title lg 500, meta and actions slots), Docker `DetailSection` and ADE `AdeTip` moved into theme (`TextTooltip`), imports switched. Guards in `scripts/check-ui-primitives.sh` for the regex-checkable rules (checkbox restyle, popover padding, heading classes, KV grid overrides, `font-mono`, toned menu icons, segmented in forms, ADE tones off-board after D1), each with a dated temporary baseline of current offenders, never PERMANENT. ARCHITECTURE "UI architecture" gains the canonical rules RD..RTa in short form. No call-site restyle beyond the two moves. Tests: affected UI specs run; no new unit tests.

Plan: `plans/P672-part2.md`.

## P672 Part 3: UI uniformity: forms and labels

Apply RF.1-RF.8 (`P672-inventory.md` §4, M4-M9) in every module: one field label (`FieldLabel`), stacked or `Field orientation="responsive"` columns (retire `AdeInspectorRow`, `EditField` label, `AdeRow` editable label column), checkbox rows as `Field horizontal` (drop `CheckboxRow`, checkbox `size-*` and custom indicators, 34 uses), `FieldSet` without gap overrides, width and textarea steps, hand error text to `FieldError`/`InlineState`/`Alert`, `kira` inputs in forms to default. Files: appendix A.4, A.6, A.9, A.13. Empties this Part's guard baselines. Tests: UI specs updated; visual baselines in PR CI; no new unit tests.

Plan: `plans/P672-part3.md`.

## P672 Part 4: UI uniformity: buttons and dialogs

Apply RB.1-RB.3 and RD.1-RD.6 (§2, §3, M10-M15): button size by host, outline only to commit or decide, in-view actions `toolbar@kira`, ADE `ACTION_CLASS` board-only; dialog sizes per table, one emphasised footer button rightmost (5 dialogs), header title only, intro as `DialogDescription`, no `DialogBody` overrides. Files: §3.1 hosts, A.1. Tests: UI specs updated; visual baselines in PR CI; no new unit tests.

Plan: `plans/P672-part4.md`.

## P672 Part 5: UI uniformity: menus, popovers, tooltips

Apply RM.1-RM.4 and RT.1-RT.2 (§5, §11, M16-M19): hand `DropdownMenuItem` lists (21 files) to `MenuItems`, menu icons muted 13 untoned, every `PopoverContent` (41) to default form shape or `flush` list shape with section rows, widths w-56/80/96/120, plain text tooltips via `TextTooltip`. Files: A.2, A.11. Tests: UI specs updated; visual baselines in PR CI; no new unit tests.

Plan: `plans/P672-part5.md`.

## P672 Part 6: UI uniformity: headings, detail headers, key-value, cards

Apply RH.1-RH.4 and RC.1-RC.3 (§7, §8, M20-M23): the 11 hand detail headers onto `DetailHeader`, the 13 `h3 text-kira-lg` section titles to `SectionHeading`/`FieldLegend`, `KeyValueList` only (two densities, no grid overrides, hand KV grids replaced), cards per job (inset, section, list), code blobs as `CodeBlock`, card-as-row to rows. Files: §7.1, A.3, A.5. Tests: UI specs updated; visual baselines in PR CI; no new unit tests.

Plan: `plans/P672-part6.md`.

## P672 Part 7: UI uniformity: status, tones, colours

Needs user decision D1 first (`P672-inventory.md` §19). Apply RS.1-RS.3, RL.2, RL.4, RI.2, RCo.1-RCo.2 (§6, §9, §12, M24-M27): one status-to-tone map (running = info, so `AdeOpStatus` leaves warn), ADE pastel tones per D1 (recommended: board and task cards only; Runs script rows and every other surface on theme tones and `Badge`), warning/error icon colours, row meta subtle, redundant `text-fg`. Files: A.7, `AdeChip`/`AdeOpStatus` hosts, Git change-request badges. Tests: UI specs updated; visual baselines in PR CI; no new unit tests.

Plan: `plans/P672-part7.md`.

## P672 Part 8: UI uniformity: tabs, segmented, choice

Apply RTa.1-RTa.2 (§17, M28-M29): `SecondaryTabs` variant by role (form value `choice`: 9 form `segmented` uses move; content `tabs` at default size; bar view switch `segmented@kira`), `ConnectionEnginePicker` onto `RadioGroupCard` (engine icon 16) and its U36 PERMANENT line deleted. Files: A.8. Tests: UI specs updated; visual baselines in PR CI; no new unit tests.

Plan: `plans/P672-part8.md`.

## P672 Part 9: UI uniformity: states, icons, type, spacing

Apply RE.1-RE.4 (toast item needs decision D3), RI.1, RTy.1-RTy.3, RSp.1-RSp.2, RBo.1-RBo.3 (§10, §13-§16, M30-M36): `EmptyState` shape and no toned media, three skeleton shapes, toasts only for off-screen outcomes, icon 13 in labelled buttons, size roles, `font-mono` to `font-data`, pane roots `p-3`, radii, borders, shadows. Deletes every remaining temporary guard baseline from Part 2. Files: A.10, class-token offenders by fresh scan. Tests: UI specs updated; visual baselines in PR CI; no new unit tests.

Plan: `plans/P672-part9.md`.
