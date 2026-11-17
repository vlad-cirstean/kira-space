# P662 iter2 Documents tab freeze on a large page

Second pass of P662 (working agreement "Multiple passes"). Plans against the current tree
(`c47830bf`, branch `stream/v0.3.16-b2-c`, worktree `/home/user/kira-space-b2-c`), on top of what
pass 1 landed (F1-F3). Scope: the one place pass 1 left frozen, the Documents tab
(`DocumentView`) with a 10,000-document page, plus every other caller of the same height model. The
P662 ask ("fix it for all modules and places") covers it, so it stays P662, not a new phase.

Discovery: `codegraph_explore` (projectPath worktree) over `DocumentView` `rowHeights` `rowAt`,
`useDocumentRowsStore` (`rowHeight`, `parseRow`, `visibleLines`, `pruneRows`, `lineCounts`),
`fieldNamesOnPage` and callers, `useVirtualRows`, parse worker (`client.ts`, `handlers.ts`,
`protocol.ts`, `useParseWorker`), `ConsoleResultGrid` `documentRowHeights`. Targeted reads of
`documents/state.ts` (`isDocumentExpanded`, `setAllExpanded`, `load`), `documents/page.ts`,
`DocumentFilterRow.vue`, `DocumentToolbar.vue`, `ProjectionMenu.vue`, `ejson.ts` `parseDocument`,
`packages/shared/protocol/page.ts` chunk helpers. Live repro below.

## Cause, confirmed

`apps/kira-space/frontend/src/studio/views/documents/DocumentView.vue:161-175` `rowHeights` maps
every row of the page. Every document defaults to expanded (`documents/state.ts:199-206`:
`expanded[id] !== false`), so for each of 10,000 rows:

1. `documentRow(tabId, row)` (`documents/page.ts:30-42`) builds the `'row'` cached view, which
   decodes the id *and the whole body* (20 MiB of `TextDecoder` for the fixture) only to read `.id`.
2. `documentRowsStore.rowHeight(..., expanded=true)` (`shared/document/rows.ts:294-312`) misses
   `lineCounts`, calls `parseRow` (`JSON.parse` plus full `DocNode` tree via `parseDocument`) and
   `visibleLines` (tree walk). All in one synchronous computed, first read by `overscanRows`
   (`:181-187`) inside the virtualizer's `overscan` getter.
3. Then `watch(virtualItems)` (`:237-246`) prunes `parseCache` and the page store's decode caches
   to the window. `lineCounts` survives, but every later `rowHeights` recompute (`pageVersion`,
   `rowsVersion`, expansion map, `editingRow`, search) repeats step 1 for all rows: a full-page body
   re-decode per recompute.

Second, smaller cause: `fieldNamesOnPage` (`documents/page.ts:66-89`) decodes and `JSON.parse`s
every body once per page object, read at mount by `DocumentToolbar.vue:52-58` (`projectionCount`)
and `DocumentFilterRow.vue:57-77` (candidates and the immediate `registerMongoFieldSample` watch).

Console document results (`console/ConsoleResultGrid.vue:108-118`) call the same `rowHeight` for
expanded rows: after "Expand all" on a 10,000-document result the same full-page parse runs. Pass 1
allowed whole-page passes on explicit user action, but a 1 s freeze on a click is still a freeze, so
iter2 covers it with the same mechanism.

### Repro numbers (WebKit, Playwright `ui`, 4-CPU host, load 2-5, 3 runs each)

Scratch spec built from the case pass 1 dropped (`git show
ca8d5339:apps/kira-space/tests/ui/studio/console-large-result.spec.ts`, test "documents tab stays
responsive on a 10,000-document page"), variants toggled by a global flag:

| Measure window | Variant | max rAF gap |
|---|---|---|
| dblclick to first row (as pass 1 measured) | unfixed | 917-1305 ms |
| dblclick to first row | 20-row page, unfixed | 291-314 ms |
| tab mount only (reply held 2.5 s) | any page size | 230-315 ms |
| reply arrival to first row + 1.5 s | unfixed | 857-957 ms |
| reply arrival | `rowHeights` returns uniform 26 px | 221-364 ms |
| reply arrival | uniform heights and `fieldNamesOnPage` returns `[]` | 159-215 ms |
| reply arrival | 20-row page, unfixed | 66-96 ms |

Chromium CPU profile (CDP `Profiler`, same scratch spec) of the unfixed load: 715 ms inclusive under
the virtualizer's `overscan` getter, i.e. the first `rowHeights` evaluation. With both stubs, JS
work under 125 ms.

Conclusions:

- `rowHeights` is about 600-700 ms of the freeze, `fieldNamesOnPage` about 60-150 ms; the rest is
  transport and render (pass 1 measured transport at 61-92 ms).
- Opening a Documents tab costs 230-315 ms regardless of page size (20 rows: same). That is tab
  mount (filter-row editors, toolbar), not page-size work, and not this freeze. Pass 1's documents
  case reset the probe before the dblclick, so it could never pass at 250 ms even with a perfect
  fix. The re-added case holds the reply so mount lands before the probe resets. The mount floor is
  reported to the orchestrator (see "Out of scope"); iter2 does not change it.

## Design

Rule kept from pass 1: on mount and every re-render, main-thread work is bounded by the visible
window, never by page row count or body size.

Options weighed:

- Measured heights (`measureElement`, estimate first): drops the exact-height model (D20) that
  `scrollToIndex`, `overscanRows`, go-to-match and keyboard nav read; every default-expanded row
  differs in height, so upward scroll jumps on each newly measured row. Rejected.
- Collapsed by default for large pages: changes UX and the persisted `expanded` semantics
  (`state.ts:199-241`, absent key = expanded). Rejected.
- Time-sliced main-thread parse: same code, no copy, but still 0.7 s+ of main-thread work per page
  (about 4 s at the 56 MiB console cap) competing with input, and WebKit has no
  `requestIdleCallback`. Rejected.
- Chosen: exact heights kept, filled from three sources by priority. (1) Window rows: exact,
  main thread, bounded by the window; these rows are parsed for rendering anyway. (2) The whole page:
  exact top-level line counts plus field names, computed in the parse worker in one job per page.
  (3) Rows neither has reached yet: an estimate (running mean of known counts). The window is always
  exact, so nothing on screen overlaps. Off-screen estimates turn exact when the worker answers. A
  scroll anchor keeps the first visible row fixed while heights above it change.

Line count equivalence: with an empty nested-expansion set (always true right after `setPage`:
`resetRows` clears `expandedPaths`), `visibleLines(...).length === parseDocument(body).children.length`,
and `null` (raw fallback) when `parseDocument` returns `null`. The worker runs the same
`parseDocument` code path, so its counts equal the main-thread ones exactly.

## Changes

### C1. Worker job `doc.pageStats`

- `apps/kira-space/frontend/src/studio/views/shared/document/ejson.ts`: split `parseDocument` into
  `documentFromValue(parsed: unknown): DocNode | null` (the `isPlainObject` check plus
  `buildNode(parsed, '', '')`) and `parseDocument(body)` = `JSON.parse` in try plus
  `documentFromValue`. No behavior change. Confirm `ejson.ts`'s `t` import (`@theme/i18n`) already
  loads in the worker bundle (`copyAll.ts` pulls document text helpers into it); if `t` touches
  `window`/DOM at import, move the label call out of the worker path rather than stub it.
- New `apps/kira-space/frontend/src/studio/views/shared/document/pageStats.ts` (pure, no Vue):
  `documentPageStats(input: { data: Uint8Array; offsets: Uint32Array }): { lineCounts: Int32Array;
  fieldNames: string[] }`. Per row: decode `data.subarray(offsets[r], offsets[r+1])` with one fatal
  `false` decoder, one `JSON.parse` in try. `lineCounts[r] = documentFromValue(parsed)?.children.length
  ?? -1` (`-1` = raw fallback, the `null` in `lineCounts`). Field names exactly as today's
  `fieldNamesOnPage`: plain non-array object, `Object.keys`, skip `_id`, union, sort with
  `compareNatural`. A parse failure gives `-1` and no names.
- `workers/parse/protocol.ts`: add `'doc.pageStats': { input: { data: Uint8Array; offsets:
  Uint32Array }; output: { lineCounts: Int32Array; fieldNames: string[] } }`.
- `workers/parse/handlers.ts`: `'doc.pageStats': (input) => documentPageStats(input)`.
- Input built by the caller from the page's `bodies` chunk: `{ data: chunk.data, offsets:
  chunk.offsets }`. Structured clone copies the chunk's exactly-sized buffer (a memcpy, about 5 ms
  for 20 MiB); no transfer, the page keeps its bytes. No inline path by size: the client already
  falls back inline only on worker failure.

### C2. Height model in the rows store

`apps/kira-space/frontend/src/studio/views/shared/document/rows.ts` (`useDocumentRowsStore`):

- `TabRows` gains `generation: number` (bumped by `resetRows`; `dropRows` deletes the entry) and
  `knownSum`/`knownCount` for the estimate. `lineCounts` keeps its meaning and its pruning rules.
- New reactive `lineCountsVersion = reactive({ n: 0 })`, separate from `rowsVersion` so a count fill
  does not re-run every mounted `DocumentTree.lines`.
- `knownRowHeight(scope, row, editingRow, isExpanded, hasSearchPreview)`: same branches as
  `rowHeight`, but an expanded row with no `lineCounts` entry returns the estimate
  `HEAD_H + round(knownSum / knownCount || EST_LINES) * LINE_H + BODY_PADDING_V`, `EST_LINES = 8`.
  Never calls `documentRow`, `parseRow` or `visibleLines`.
- `fillLineCounts(scope, rows: Iterable<number>): void`: for each row with no entry, compute it as
  `rowHeight` does today (`parseRow` plus `visibleLines`), record it, update the estimate sums; bump
  `lineCountsVersion` once when anything was added. Callers pass only window rows.
- `generationOf(scope): number` and `primeLineCounts(scope, generation, counts: Int32Array)`: no-op
  when `generation` differs (page replaced or scope dropped since the request). Otherwise set each
  row with no entry *and* an empty (or absent) `expandedPaths` set to `counts[r] < 0 ? null :
  counts[r]`, update sums, bump `lineCountsVersion` once. Never overwrites an entry
  `fillLineCounts` or `togglePath` produced.
- `rowHeight` stays as is (sync exact) for callers that ask about one row; `togglePath` keeps
  deleting the row's entry, and the window fill recomputes it.

### C3. Off-thread page stats store

New `apps/kira-space/frontend/src/studio/views/shared/document/pageStatsStore.ts`, Pinia store
`useDocumentPageStatsStore`. One concern: per-scope lifecycle of the `doc.pageStats` job.

- State: `shallowReactive` record `fieldNames: Record<string, readonly string[]>` and a plain
  `Map<string, { page: DocumentPage; ctrl: AbortController }>` of requests.
- `request(scope, page)`: no-op when the same `page` object is already requested. Otherwise abort
  the previous controller, delete `fieldNames[scope]`, capture `rowsStore.generationOf(scope)`, run
  `parseClient.run('doc.pageStats', { data: page.bodies.data, offsets: page.bodies.offsets },
  { signal })` (`workers/parse/instance.ts`; a store has no component scope, so not
  `useParseWorker`). On resolve, if the request is still current: `rowsStore.primeLineCounts(scope,
  generation, out.lineCounts)` and `fieldNames[scope] = out.fieldNames`. `AbortError` is ignored;
  any other error goes to `console.error` with the scope and leaves estimates and the empty field
  list (heights still exact for the window, field list honest-empty).
- `fieldNamesFor(scope): readonly string[] | null` (`null` = not computed yet).
- `drop(scope)`: abort and forget. Register with `registerTabRuntimeCleanup` (document tab close).
  Console calls it from the place that already calls `documentRowsStore.dropRows` for a result key.

### C4. Documents tab

`documents/page.ts`:

- `onSet: (tabId) => { useDocumentRowsStore().resetRows(tabId); const page = getPage(tabId); if
  (page) useDocumentPageStatsStore().request(tabId, page); }` (`resetRows` first, so the captured
  generation is the new page's). Check the `createPageStore` `onSet` call site (`store.ts:104`)
  runs after the page is stored; if not, pass the page through `onSet`.
- New `documentId(tabId, row): string | null`: id-only decode through `store.cached(tabId, row,
  'id', ...)`, same shape as pass 1's `console/resultPages.ts:58` `documentId`. `documentRow` keeps
  reading the same `'id'` entry.
- `fieldNamesOnPage(tabId)`: becomes the explicit-action fallback only: return
  `useDocumentPageStatsStore().fieldNamesFor(tabId)` when ready, else today's synchronous pass
  (keep the `WeakMap` cache). Update its comment: reactive readers use `pageFieldNames`.
- New `pageFieldNames(tabId): readonly string[] | null` = `fieldNamesFor(tabId)`.

`documents/DocumentView.vue`:

- `rowHeights` (`:161-175`): read `props.tab.state.expanded` once. Empty map (the default): every
  row is expanded; no id decode. Otherwise build the set of collapsed ids and read each row's id
  through `documentId`. Height through `documentRowsStore.knownRowHeight(...)`. Dependencies:
  `pageVersion.n`, `documentRowsStore.lineCountsVersion.n`, `documentRowsStore.rowsVersion.n`.
- `idsOf` (`:152-159`): `documentId` instead of `documentRow` (Collapse all and the row menu's
  `allIds` no longer decode bodies).
- `watch(virtualItems)` (`:237-246`): after the existing prune calls, `documentRowsStore.
  fillLineCounts(props.tab.id, windowRows)` where `windowRows` maps `items` through `rows.value`.
  The fill bumps `lineCountsVersion` only when it added counts, so the watch, `rowHeights` and
  `measure()` converge once the window is exact (each pass adds rows, the set is finite). Order:
  prune first, then fill, so the fill's parses stay in `parseCache` for the render.
- Scroll anchor: new composable (C6) called with `scrollEl` and `rowHeights`.
- `onGoToMatch`, keyboard `scrollToIndex`, `overscanRows`: unchanged; they read `rowHeights`, which
  the virtualizer reads too, so positions agree while estimates are in.

`documents/DocumentToolbar.vue` `projectionCount` (`:52-58`): `pageFieldNames(props.tab.id)`;
`null` or empty gives `null` (badge hidden until ready, as for an empty page today).

`documents/DocumentFilterRow.vue`:

- `filterCandidates`/`sortCandidates` (`:57-64`): `['_id', ...(pageFieldNames(id) ?? [])]`.
- Registration watch (`:66-77`): source `[pageFieldNames(id), connectionId, path]`; skip while
  `null`. `immediate: true` stays.

`documents/ProjectionMenu.vue:29` keeps `fieldNamesOnPage(props.tabId)` (explicit user action; the
fallback pass runs only if the menu opens before the worker answers).

`shared/mongoFieldSample.ts`: comment references only (`fieldNamesOnPage` reader names); no code.

### C5. Console document results

`console/ConsoleResultGrid.vue`:

- `documentRowHeights` (`:108-118`): `knownRowHeight` instead of `rowHeight`; depend on
  `lineCountsVersion.n` too. The empty-expanded fast path (pass 1's F1) stays.
- When the expanded set becomes non-empty for a page (`watch` on `expanded.length > 0` plus
  `pageVersion.n`, immediate): `pageStatsStore.request(props.pageKey, getPage(pageKey))` for a
  document page. Field names are unused here.
- `watch(docVirtual.virtualItems)` (`:186-197` area): after its prune calls, `fillLineCounts` over
  the window rows when the expanded set is non-empty.
- Scroll anchor composable with `docScrollEl` and `documentRowHeights`.
- `console/state.ts:141` calls `documentRowsStore.dropRows(result.key)`: add
  `pageStatsStore.drop(result.key)` next to it.

### C6. Scroll anchor

New `apps/kira-space/frontend/src/studio/views/shared/document/useHeightAnchor.ts`:
`useHeightAnchor(scrollEl: Ref<HTMLElement | null>, heights: () => readonly number[] | undefined)`.
`watch(heights, (next, prev) => ..., { flush: 'post' })`: skip when either is undefined, lengths
differ (new page), or `scrollTop === 0`. From `prev`, find the first row whose bottom passes
`scrollTop` and its offset into that row; sum `next` up to the same index; if the start moved, set
`el.scrollTop = newStart + offset`. O(rows) additions per height change, no decode. `flush: 'post'`
runs after `useVirtualRows`' `measure()` watch and before paint, so the first visible row never
moves when the worker's counts land above it or when a row above is toggled.

Local to the two document views, not inside `packages/workbench/src/behaviour/virtualRows.ts`:
P660 adds `keepScroll` there, and this keeps iter2 off that file. With P660's restore, remount reads
`lineCounts` and field names that survived in their stores, so heights are exact at restore time.

## Tests

### UI spec (re-added case, must fail first)

`apps/kira-space/tests/ui/studio/console-large-result.spec.ts`, new test "documents tab stays
responsive on a 10,000-document page", built from pass 1's dropped body (`widgetsFixture`,
`connectMongoAndExpand`, `docPage()`, `findRow(page, WIDGETS_PATH).dblclick()`), changed so it
measures page work, not tab mount:

1. Read snapshot gets `delayMs: DELAY_MS` (1,500 ms). After the dblclick, wait for
   `document-list` to be visible plus 500 ms (mount settled, reply still held), then
   `resetFreezeProbe`. If 1,500 ms leaves too little room, raise `delayMs` for this case only.
2. Wait for the first `document-row`, then `page.waitForTimeout(1500)` so the worker result and its
   height recompute fall in the window. `expectResponsive(page, 'documents tab arrival',
   BUDGET_MS)`.
3. Exactness after stats: the first row's rendered height equals `26 + fieldCount * 18 + 8` (read
   `document-field-count`; assert from the badge, not a literal).
4. Scroll: 8 wheel steps like the other cases, probe under budget, a row stays visible.
5. Collapse all (`document-collapse-all`) then Expand all (`document-expand-all`), probe under
   budget after each, rows visible.
6. Toggle the first visible row (`document-toggle-expand`), probe under budget, the next row's top
   equals the toggled row's top plus its new height (no overlap).

Run it on the current tree first: step 2 must fail (expected 850-950 ms). If it passes, raise the
fixture, never the budget. No module round trip for this case: tab mount alone is 230-315 ms
(see Cause), so a remount gap measures mount, not page work.

Also add one console step to the existing document console case: after arrival, click the result's
expand-all control (`console-expand-all`, `ConsoleResultStrip.vue:128`), probe under budget,
first row expanded. Must fail on the current tree too (full-page `rowHeight` parse); if it passes,
drop the step and say so in Result.

### Unit test

The height model has interacting rules (window fill vs worker prime vs toggle vs reset vs estimate,
plus the generation guard), so extend the existing
`apps/kira-space/tests/unit/studio/document-row-height-cache.spec.ts` (it already counts
`JSON.parse` calls through `registerDocumentRows`):

- `knownRowHeight` on an unfilled expanded row parses nothing and returns the estimate; the
  estimate follows the mean of filled counts.
- `fillLineCounts` parses only rows with no entry; a second call parses nothing.
- `primeLineCounts` fills rows with no entry, leaves filled and path-toggled rows alone, maps `-1`
  to the raw-fallback height, and is a no-op after `resetRows` (stale generation).
- Equivalence: for a set of bodies (plain doc, nested doc, wrapper-shaped root such as
  `{"$oid":"..."}`, truncated body, array body, non-JSON), `documentPageStats(...).lineCounts`
  equals the main-thread `fillLineCounts` result, and `fieldNames` equals the old
  `fieldNamesOnPage` rule (keep a copy of the old loop in the test as the oracle).

No test for `useHeightAnchor` beyond UI step 6 (one sum and one assignment).

### Runs

New and changed UI cases (`console-large-result.spec.ts` whole file, WebKit `ui`, 3 runs in a row),
`document-edit`, `document-view-readonly`, `document-new-save-double-submit`, `autocomplete`,
`list-keyboard`, `copy-coverage`, `immediate-write-connection-drop`, `console`, `console-copy-all`;
`bun test` for the unit spec plus `document-collapse-all-preserves-other-pages`,
`document-projection-menu-close`, `parse-worker-client`; typecheck and lint via pre-commit. Browsers:
WebKit through Playwright `ui`, Chromium at `/opt/pw-browsers` (`PLAYWRIGHT_BROWSERS_PATH`); never
`playwright install`.

## Implementation order

1. Re-add the UI case (and the console expand-all step); run on the current tree; record the
   failing numbers. Commit `test:`.
2. C1 (ejson split, `pageStats.ts`, protocol, handler) and the unit equivalence test. Commit `feat:`.
3. C2 and C3 plus the rest of the unit spec. Commit `fix:`.
4. C4, C6. Commit `fix:`.
5. C5. Commit `fix:`.
6. Runs above; fix; commit.
7. `SPEC.md` P662 row back to Done; rewrite its Result: drop the "Not fixed" sentence and the
   pointer line, add the Documents tab cause (full-page `rowHeights` parse for default-expanded rows
   plus per-recompute body decode; `fieldNamesOnPage` share), fix (window-exact, worker-filled,
   estimate, anchor), before and after numbers, the tab-mount floor as a measured fact.

## File ownership

| File | Owner | iter2 edit | Shared with |
|---|---|---|---|
| `apps/kira-space/frontend/src/studio/views/documents/DocumentView.vue` | C | `rowHeights` `:161-175`, `idsOf` `:152-159`, `watch(virtualItems)` `:237-246`, one `useHeightAnchor` call, imports | A (P660 adds `keepScroll` to the `useVirtualRows` call near `:189-196`). Disjoint hunks; do not touch that call. |
| `apps/kira-space/frontend/src/studio/views/documents/page.ts` | C | `onSet`, new `documentId`, `pageFieldNames`, `fieldNamesOnPage` body | none |
| `apps/kira-space/frontend/src/studio/views/documents/DocumentToolbar.vue` | C | `projectionCount` | none |
| `apps/kira-space/frontend/src/studio/views/documents/DocumentFilterRow.vue` | C | candidates, registration watch | none known |
| `apps/kira-space/frontend/src/studio/views/shared/document/rows.ts` | C | C2 | none |
| `apps/kira-space/frontend/src/studio/views/shared/document/ejson.ts` | C | `documentFromValue` split | none |
| `apps/kira-space/frontend/src/studio/views/shared/document/pageStats.ts` (new) | C | whole file | none |
| `apps/kira-space/frontend/src/studio/views/shared/document/pageStatsStore.ts` (new) | C | whole file | none |
| `apps/kira-space/frontend/src/studio/views/shared/document/useHeightAnchor.ts` (new) | C | whole file | none |
| `apps/kira-space/frontend/src/studio/workers/parse/protocol.ts` | C | one job entry | none |
| `apps/kira-space/frontend/src/studio/workers/parse/handlers.ts` | C | one handler, one import | none |
| `apps/kira-space/frontend/src/studio/views/console/ConsoleResultGrid.vue` | C | C5 hunks in `documentRowHeights`, virtual-items watch, one request watch, one anchor call | possibly A (P660 scroll and view-state restore around `useResultScrollState`). Disjoint hunks. |
| `apps/kira-space/frontend/src/studio/views/console/state.ts` | C | one `drop` call next to `dropRows` (`:141`) | possibly A (view-state fields). Additive. |
| `apps/kira-space/frontend/src/studio/views/shared/mongoFieldSample.ts` | C | comment only | none |
| `apps/kira-space/tests/ui/studio/console-large-result.spec.ts` | C | new test, one console step | none |
| `apps/kira-space/tests/unit/studio/document-row-height-cache.spec.ts` | C | new cases | none |
| `docs/v0.3.16/SPEC.md` | C | P662 row status and Result | A and B edit their own rows. |

Not touched: `packages/workbench/src/behaviour/virtualRows.ts` (P660 owns its `keepScroll` hunk),
`MainView.vue`, `tabKinds.ts`, `documents/state.ts` (expansion semantics unchanged), `ProjectionMenu.vue`,
`DocumentTree.vue`, `DocumentRow.vue`, i18n catalogs. Overlap with P660: small, disjoint hunks in
`DocumentView.vue`, `ConsoleResultGrid.vue`, `console/state.ts`; no ordering dependency.

## Out of scope, reported

Documents tab mount costs 230-315 ms max rAF gap in WebKit for any page size (20 rows included).
It is not page-size work and not the reported freeze. The orchestrator decides whether it becomes
its own follow-up phase row in `SPEC.md` (working agreement: out-of-scope work is a phase, never a
Result line). iter2 does not profile or change it.
