# Performance: budgets, measurement, and results

See the [README](../README.md) for what the app is and how to run it.

This doc is the P12 deliverable named by SPEC.md §2's two hard requirements (§2.1 "Silky UI",
§2.2 "Small RAM footprint"): where each budget is measured, whether measurement is automated or
manual, and the numbers from a real run. Re-measure it; don't rewrite it from scratch —
`docs/HISTORY.md (v1 P12)` is the frozen design record, this file is the living one.

§2.1's numbers were first captured on this repo's Linux/Xvfb dev container (headless,
software-rendered Chromium), 2026-08-23, and re-measured 2026-08-26 on the macOS/Colima dev machine
described in §2.2 — see §2.1's scroll-response methodology note for why the two environments'
numbers aren't directly comparable on the scroll-response row specifically. macOS **packaged**
numbers (§3's manual procedures, run against the built app rather than this dev build) are still
not recorded — no opportunity to run them yet; §3 documents the procedure and what to fill in.
**Every Go-side wall-clock figure in §2.5-§2.8 predates Go 1.26's Green Tea GC** — see §2.11's own
caveat below for exactly which of those numbers that affects and which it doesn't.

## 1. Budget table

| §2 budget | Metric actually measured | Where | Automated? |
|---|---|---|---|
| Grid scroll frame ≤ 8 ms (p50 ≤ 16 ms and max ≤ 80 ms on the `tests/ui/` tier, see §2.1) | app-work delta (`KiraSlickGrid.render()` entry's scroll-work mark → DOM committed), **p50** over 20 steps on a 10 000-row page (see methodology note below) | `tests/ui/budgets.spec.ts` | **asserted — passing; see §2.1** |
| Grid scroll frame, horizontal axis (P29) ≤ 8 ms | same work-delta measurement, **p50** over 20 steps, on `app.scroll_grid` (60 cols x 5000 rows) | `tests/ui/budgets.spec.ts` | **asserted — passing; see §2.1** |
| Grid scroll frame, vertical axis, wide table (P29) ≤ 8 ms | same work-delta measurement, **p50** over 20 steps, on `app.scroll_grid` | `tests/ui/budgets.spec.ts` | **asserted — passing; see §2.1** |
| Row window stays coalesced to <= 1 re-render/frame during sustained fast scroll (P22 D1) | `notifiesPerFrame` <= 1 at 40/100/200/456 px/frame; `uncoveredPx` === 0 at 40-100 px/frame | `tests/ui/budgets.spec.ts` | **asserted — passing; see §2.1a** |
| — (secondary) | rAF interval p95 < 24 ms (80 ms on the `tests/ui/` tier, see §2.1), DOM cells < 1500 | `tests/ui/perf.spec.ts` | asserted |
| Cell selection → editor populated ≤ 50 ms | click cell → `.view-lines` contains the cell's text (`.cm-content`, CodeMirror, before P60a moved the cell editor to Monaco) — the DOM element the click-to-DOM measurement waits on, not a full-document read, so it stays a plain DOM text-wait rather than the `data-kira-editor-text` debug hook `tests/ui/support/editorText.ts` uses elsewhere, p95 over 20 cells | `tests/ui/budgets.spec.ts` | **asserted** |
| Tab switch ≤ 50 ms (product budget) | click tab → the other table's header cell present, p95 over 20 alternations | `tests/ui/budgets.spec.ts` | **asserted at p95 ≤ 300 ms** (WebKit sandbox bound) plus a machine-independent guard (no `<style>`/`<link>` churn, no `.slickgrid_` rule); 50 ms unverified on WKWebView after P139 Part 2 — see §2.1 |
| Tree node expand (cached) ≤ 50 ms | click twisty → child rows present, p95 over 20 collapse/expand cycles of an already-cached schema node | `tests/ui/budgets.spec.ts` | **asserted** |
| Console keystroke → completion popup visible ≤ 50 ms (p50) | last keypress → suggest widget on screen (computed `visibility: visible` and a list row; Monaco's own suggest widget, P60b — was `.cm-tooltip-autocomplete`), p50 over 20 keystrokes. Not `.visible`: Monaco adds that class 100 ms after the widget is on screen | `tests/ui/budgets.spec.ts` | **asserted** |
| Any DB round-trip async/cancellable | — | covered by every adapter's cancel scenario (P1–P10) | n/a |
| < 350 MB total RSS, 5 connections / 10 tabs | min of 10 `app.getAppMetrics()` sums over an idle window | removed — see §2.2 below | **not automated; documented structural finding** |
| Same, packaged | `ps -o rss` sum for the same scenario | §3 procedure below | manual (macOS) |
| Cold start (no SPEC number) | no automated dev-build check exists any more — `tests/e2e/startup.spec.ts` (`process.uptime()`) was deleted with the rest of `tests/e2e/` at P57 and has no `tests/ui/` successor | — | **not automated — manual only, see next row** |
| Cold start, packaged | ≤ 1500 ms median of 3 warm launches, read from the `did-finish-load` log line | §3 procedure below | manual (macOS) |

## 2. Automated results (this environment)

### 2.1 Interaction budgets — `tests/ui/budgets.spec.ts`, `tests/ui/perf.spec.ts` (originally `tests/e2e/budgets.spec.ts`/`tests/e2e/perf.spec.ts`, ported at P57 M5 — see that sub-section below)

| Metric | p50 (work) | p95 (work) | p50 (e2e, logged) | p95 (e2e, logged) | Budget | Result |
|---|---|---|---|---|---|---|
| Scroll response | 2.2 ms | 4.1 ms | 4.8 ms | 14.5 ms | ≤ 8 ms (**p50 gated on work** — see note) | pass |
| Scroll response, horizontal (P29) | 6.2 ms | 12.7 ms | 9.0 ms | 19.0 ms | ≤ 8 ms (p50 gated on work) | pass |
| Scroll response, vertical, wide table (P29, `scroll_grid`) | 5.1 ms | 7.6 ms | 8.3 ms | 13.9 ms | ≤ 8 ms (p50 gated on work) | pass |
| Cell → editor | 17-22 ms | 23-32 ms | — | — | ≤ 50 ms (p95) | pass |
| Tab switch (P139 Part 2) | 113-125 ms | 159-194 ms | — | — | ≤ 300 ms (p95, WebKit sandbox bound; product budget 50 ms, see below) | pass |
| Cached tree expand | 17-20 ms | 24-53 ms | — | — | ≤ 50 ms (p95) | pass |
| Console keystroke → completion popup (P18 addendum D26) | 32-36 ms (P139); 36-39 ms, p95 48-55 ms (P443, idle; 53-56 ms on some earlier runs, see ARCHITECTURE Known open items) | 38-49 ms | — | — | ≤ 50 ms (p50), max ≤ 200 ms | pass idle; open item |
| Cell-editor populate latency (informational) | — | 41 ms | — | — | — | logged |
| `perf.spec.ts` rAF scroll frame time | 16.7 ms | 17.6 ms | — | — | < 24 ms (secondary tripwire) | pass |
| Cold start, fresh | wall 589 ms / in-app uptime 537 ms | — | — | — | ≤ 2500 ms | pass |
| Cold start, restored (5 conns, 10 tabs) | wall 640 ms / in-app uptime 522 ms | — | — | — | ≤ 3000 ms | pass |

Scroll and cold-start rows: macOS/Colima dev machine, 2026-08-26 (P47 checkpoint). Cell to editor,
tab switch, tree expand and keystroke rows: Linux sandbox WebKit (headless, 4 vCPU, `ui-timing`
project, ranges over 5 runs), replaced in P139 Part 2.

**Console keystroke → completion popup.** The `<= 1000` gate from the P57 M5 port (`9ee240f0`)
measured ~113 ms, which was Monaco's fixed 100 ms `.visible` class timer, not the popup (on screen at
27-32 ms). P139 Part 2 measures the on-screen state and restores p50 <= 50 / max <= 200.

**Tab switch (P139 Part 2).** Every switch remounts `DataView` and SlickGrid (`MainView.vue` keys it
by tab id); no tab caching. Before the fix, WebKit here measured p50 311-318 / p95 436-447 ms. Two
causes: SlickGrid built its columns twice per mount (`ba41ec3f`), and its per-grid `<style>` (append,
mutate rules, remove on unmount) forced a full-document style rebuild of ~55 ms each on WebKit, where
a sheet appended with its text set costs 2-3 ms. `kiraSlickGrid.ts` now positions columns through
`--sg-l<i>`/`--sg-r<i>` on the grid root plus one shared append-only sheet, so no per-grid sheet
exists. Result: p50 113-125 / p95 159-194 ms.

Why the gate is 300 ms, not 50 ms: a real remount costs ~47 ms of Vue work alone on this sandbox's
WebKit (~29 ms on Chromium; Chromium's whole remount measures p95 58-75 ms), so 50 ms p95 is not
reachable here on either engine. This WebKit build also lacks `margin-trim`, which turns on
Tailwind v4's `@property` fallback (~68 `--tw-*` custom properties on every element) and roughly
doubles every restyle; dropping it measured p95 136-162 ms, but it is dead CSS on macOS 14+ and the
build was left unchanged. Measured without it, the stylesheet fix alone gave p95 170-212 ms; 300 ms is
~1.5x that and below every pre-fix run. The bound is a sandbox tripwire. The guard is the real
regression check: zero `<style>`/`<link>` churn in `<head>` (Monaco's per-editor `media="screen"`
sheets excluded) and zero `.slickgrid_` rules across the 20 switches. Not measured: real WKWebView.
Run on a Mac (§3): `CSS.supports('margin-trim: inline')` should be `true`, and the 20-switch p50/p95
gives the true figure against the 50 ms product budget.

**Scroll-response methodology note.** A script-driven `el.scrollTop = x` dispatches its native
`scroll` event on the next "update the rendering" step, the same cadence as `requestAnimationFrame`.
On a headless, software-rendered display roughly half of every 20 steps pay an extra frame regardless
of app work; a double-rAF wait made every sample jump to ~one frame period, confirming a
frame-scheduling artifact. So p95 over a mix of "no wait" / "one frame wait" samples does not measure
app work (nor can `perf.spec.ts`'s rAF tripwire demonstrate 8 ms). `budgets.spec.ts` asserts **p50**
with a looser `max` sanity bound and logs p95. On macOS the compositor hit a full frame on all 20
steps; `tests/e2e/support/measure.ts`'s work/e2e split (`61ba523`) gates on the work delta
(`KiraSlickGrid.render()` entry to DOM committed, P139), which removes both hops.

**P29 and P47.** The horizontal and wide-table-vertical measurements, overscan-coverage invariants,
DOM-cell bound and sub-row zero-mutation check all pass (`.grid-row { contain: layout }` stays, D8).
The `@tanstack/vue-virtual` migration (P47) was flat-to-improved on work p50 (D13): `big_rows` vertical
2.4 → 2.2 ms, `scroll_grid` horizontal 6.3 → 6.2 ms, wide-table vertical 6.2 → 5.1 ms (~18%). No D14
no-go criterion tripped. Verdict: go.

**P161 `ui-timing` re-measurement.** The scroll-response case failed 4 of 5 runs against p50 <= 12 /
max <= 50 (p50 9-14 ms, max 51-68 ms), identical at P139's own commit `058623df`; no grid render code
changed between. This host runs the render ~1.4x slower than P139's (p50 6-8 ms). Bounds became **p50
<= 16 ms and max <= 80 ms**. The 8 ms product budget is unchanged and unverified here.

**P161 Documents list (WebKit sandbox, `perf:documents`).** 5 000 documents, 160 px rows, fast
flick: before frame p95 171 ms (80 of 82 over 50 ms), after 66 ms (41 of 80). Fixes: one shared
tooltip instead of two reka `Tooltip`s per row (~51 ms of a 142 ms frame), overscan in px (320). The
target (flick p95 <= 50 ms) was not met: mounting a full new row window per frame floors at 21 ms.

**P311 Docker container list (WebKit sandbox, `docker-scroll` probe).** 400 containers, 28 rows
mounted, 5-step flick ladder, 3 runs each at `load1 <= 1.0`. Frame p95 medians before 44 / 40 / 46 /
42 / 43 ms, after 20 / 20 / 20 / 20 / 20 ms (-50 to -57%). Frames over 50 ms before 13 / 5 / 17 / 10 /
13, after 0-1. One baseline run (p95 58-68 ms, 92-157 frames over 50 ms) was an outlier and is in the
medians. Fixes: row-key `v-for` keys plus per-key cached nav handlers in `VirtualList` so mounted rows
skip re-render on range change, row components with primitive props, one `AttributeTooltip` instead
of a reka `Tooltip` per row button and origin icon.

**P163 large-input parse callers (WebKit sandbox, `perf:parse`).** Moving parse and format to
a worker cut the longest main-thread block: Console Format 64 KB 631 → 84 ms, 236 KB 1 848 → 187 ms,
1 MB 6 913 → 683 ms; Console Copy all 198-411 → 83-85 ms (10 000 x 0.4 KB). Beautify keeps a ~1 s
block at 5 MB because Monaco applies the result as one undoable edit. Documents field-name
memoization saves ~20 ms, inside noise. Worker cold start 7-14 ms, warm round trip 0-1 ms, 12 MB
`postMessage` 15-18 ms. Every other caller timed in-page (find, compare, `detectFormat`,
`parseDocument`, row menu, Mongo lint, EXPLAIN, Copy as JSON) stayed <= 51 ms (slowest: response find `findRanges` on 12 MB, 35-51 ms) and was left alone. The
HTTP response viewer showed no regression within run noise.

**P57 M5 (Wails/Go migration) — `tests/ui/budgets.spec.ts` and `tests/ui/perf.spec.ts`.** Both specs
ported against real-captured Postgres fixtures, no Electron, no live backend: every timing is a
click/scroll/keystroke against data already in memory, so a mocked backend changes nothing about what
the numbers mean. Measured on the sandboxed Linux container (headless, software-rendered real WebKit,
a different engine and a more virtualized host than the figures above):

| Metric | p50 (work) | p95 (work) | Budget | Result |
|---|---|---|---|---|
| Scroll response (`big_rows`) | 7-10 ms | 12-17 ms | ≤ 12 ms (p50; see note) | pass |
| Scroll response, horizontal (`scroll_grid`, mocked to one pageSize=100 page) | 19-21 ms | 27-43 ms | ≤ 1000 ms (sanity bound only) | pass |
| Scroll response, vertical, wide table (`scroll_grid`) | 14-16 ms | 29-48 ms | ≤ 1000 ms (sanity bound only) | pass |
| Cell → editor | ~6 ms | ~15 ms | ≤ 50 ms (p95) | pass |
| Cached tree expand | ~4 ms | ~10 ms | ≤ 50 ms (p95) | pass |
| Console keystroke → completion popup | ~13 ms | ~20 ms | ≤ 50 ms (p50) | pass |
| `perf.spec.ts` rAF scroll frame time | 34-47 ms | 37-54 ms | < 80 ms (secondary tripwire) | pass |

P139: the scroll-work mark moved from the host scroll listener to `KiraSlickGrid.render()` entry,
because SlickGrid's `scrollRenderThrottling` defers a far-jump render past that listener. `big_rows`
work p50 is now 6-8 ms, p95 9-22 ms.

Two budgets needed a documented adjustment for this tier, neither for app-work reasons:

- **`perf.spec.ts` rAF tripwire, 24 ms → 80 ms.** Raw rAF deltas carry this environment's frame-pump
  cadence (~35 ms/frame idle, load 0.1-0.3), the same artifact as the methodology note. The
  work-isolated scroll measurement stayed at 7-10 ms. 80 ms is far below what an unvirtualized
  render would produce.
- **`budgets.spec.ts` primary scroll response, 8 ms → 12 ms** (later 16 ms, P161 above). The `ui`
  project runs `fullyParallel: true`, so it shares CPU with other spec files: 7-8 ms alone, 9-10 ms
  under full-suite contention, reproducible. The horizontal and wide-table-vertical rows are gated at
  a 1000 ms sanity bound here; the P29 coverage invariants they exist to prove are unaffected.

Not ported: `perf.spec.ts`'s "L2 budget never exceeded after twenty pages", because
`window.__kiraCacheStats` wraps a real engine request and a mock could only make "usage ≤ budget"
true by construction. The budget-respecting behaviour is asserted in Go against the real cache,
`apps/kira-space/internal/enginecache/lru_test.go`'s `TestByteLru_*`. The `leaks.spec.ts`
sub-scenarios "L3 is bounded" and "clearing the cache resets the hit rate" drop for the same reason;
"deleting a connection closes its tabs" drops because `tests/ui/support/mockRuntime.ts` has no
`Events.On` analogue for `connectionsChanged`. `tests/e2e-real/` is the only tier that could recover
them.

### 2.1a P22 — the fast-scroll rendering lag: pass 1's fix was a no-op, corrected in a second pass

**This section previously recorded pass 1 (P22 D1, `observeScrollElementOffset`'s rAF-deferred
notify) as a solved problem with a measured root cause. It wasn't — the premise was refuted, the
instrument that certified it was unfalsifiable by construction, and the "fix" changed nothing a user
could see.** Kept below in full, not deleted, because the corrected understanding is only legible
against what it corrects. See
`docs/HISTORY.md (v1.1 P22)` for the complete evidence (§2)
and decisions (§5); this is the short version.

**What pass 1 assumed, and why it's false.** Pass 1's F1 asserted *"a fling can fire many native
`scroll` events within a single frame"*, and that virtual-core's stock offset observer therefore
drove N full re-renders per frame. That's false in every modern engine: HTML's *run the scroll
steps* dispatches a scrolled element's `scroll` event at most once per rendering update, before *run
the animation frame callbacks* runs in that same update — §2.1 above already documents this for
Chromium (the paragraph this section used to cite), and pass 1's own commit message independently
confirmed it for WebKit ("WebKit coalesces any number of synchronous writes into exactly one native
`scroll` event"). The observer was already ≤ 1 notify per frame before pass 1 touched it, so
coalescing it into one `requestAnimationFrame` per "burst" coalesced a burst that never occurs.

**Why the old table above (now removed) looked like proof anyway.** `measureScrollCoverage`
(deleted) manufactured the burst itself: `SUB_STEPS_PER_FRAME = 8` synthetic
`dispatchEvent(new Event('scroll'))` calls per simulated frame, straight into the observer's own
listener. The "before" column was bounded above by 8 **by construction** (the loop's own constant),
and the "after" column of 1 was what `requestAnimationFrame` does to eight synchronous dispatches
**by definition**. Neither number ever described the application. The accompanying `uncoveredPx ===
0` reading was separately unfalsifiable: that harness drives `scrollTop` from the main thread, so
the DOM's scroll offset and the main thread's own knowledge of it are the same value by
construction — the condition that produces the real symptom (WebKit's scrolling thread moving the
composited layer ahead of what the main thread has rendered, during a real momentum scroll) can't
occur when a script is doing the scrolling.

**The corrected picture.** The rAF deferral is reverted — `observeScrollElementOffset` is back to
virtual-core's own stock synchronous timing, with only its `isScrollingResetDelay` debounce kept.
Two mechanisms replace it, aimed at what actually governs the symptom on real hardware:

- **Runway** — the row axis's overscan (`OVERSCAN_PX`, 560 px) was symmetric and direction-blind.
  `rowRangeExtractor` (`views/shared/page/columns.ts`) extends it asymmetrically toward the scroll
  direction, scaled by a measured velocity, capped in *cells* so a wide table can't blow the DOM-size
  budget. At zero velocity it's byte-identical to the old symmetric window — every at-rest budget in
  this file is unchanged by it.
- **Throughput** — `renderRows` (`DataGrid.vue`) now returns a reference-stable `RowVM` object for a
  row nothing relevant changed about, and rows render through `GridRow.vue`, a child component whose
  one prop lets Vue's own `shouldUpdateComponent` skip re-rendering a retained row entirely (no
  vnode diff, no DOM patch) instead of rebuilding all ~43 mounted rows on every scroll frame.

Both are sandbox-provable as *mechanisms* — `tests/ui/budgets.spec.ts` gates D3's at-rest window
staying exactly what it was, D3's cell cap holding under a synthetic high velocity, and D4's own
render-count property (a row updates only if its signature actually changed). **Neither is provable
as *the fix* from this tier.** Whether either one moves the user's actual perceived lag is a
real-hardware question this environment can't answer — see the protocol below.

**The lesson.** A passing scroll-response budget (§2.1) and a visible fling lag are measurements of
different things, and pass 1 additionally paired a refuted mechanism with an instrument that could
not have disproven it either way. The general shape — an instrument that can't observe the
phenomenon produces a confident, wrong conclusion — is the same failure
`git show 7d8a72ab4a5e:docs/WEBVIEW-SCROLL-MEMORY.md` §2.1 hit and fixed first, for the memory half.

**What a real measurement needs, and how to take it.** `window.__kiraScrollTrace`
(`views/grid/scrollTrace.ts`) is an in-page probe, not a `tests/ui/` instrument: it records, per
animation frame, the real native `scroll` events observed, a measured px/frame velocity, live
`uncoveredPx`, and Vue's own update duration — all read from a *real* fling on *real* hardware, which
this environment has never been able to produce (`git show 7d8a72ab4a5e:docs/WEBVIEW-SCROLL-MEMORY.md` §9's own
`CGEventPost` injector never got a TCC grant, but that gap was for driving momentum from *outside*
the process; this probe lives inside the page and lets the human supply the momentum instead). It's
reachable from a dev build's View → Open DevTools (`internal/shell/menutemplate.go`). Protocol:

1. `bun run dev`, open a 50 000+-row table at page size 10 000, comfortable density.
2. Web Inspector → Console → `__kiraScrollTrace.start()`.
3. One hard two-finger flick, let momentum die out.
4. `copy(JSON.stringify(__kiraScrollTrace.stop()))` — report `summary.pxPerFrame` (the first real
   measurement of a macOS momentum scroll's velocity this repo has ever had) and the `scrollEvents`
   histogram (the direct, real-hardware test of the "at most one `scroll` per frame" premise above —
   if it's ever > 1, that premise is wrong on real WebKit and this section needs revisiting).
5. `window.__kiraGridTuning.maxLeadPxOverride = 4000` (then flick, `stop()`), then `= 560` (today's
   runway, flick again), then `.incrementalRows = false` (D4 off, flick again) — three traces, one
   build, no rebuild between them. If the lag visibly changes with the runway setting and
   `uncoveredPx` moves accordingly, that's the constraint; if not, compare `renderMs` instead.

Full detail, including what to do if neither mechanism moves the needle, is
`docs/HISTORY.md (v1.1 P22)` §7.3.

### 2.1b P22 D6 — the first-launch window default, and what it is and is not worth

A UX fix, not the fix for §2.1a's memory doc. `internal/shell/window.go`'s unconditional 1280×800
(edge-to-edge on a same-size laptop panel) is now `DefaultBounds`, which clamps to the primary
screen's work area minus a margin, never grows it, and falls back to 1280×800 with no work area
(`TestDefaultBounds`). `MinWidth`/`MinHeight` stay 1024×640 (D6(b) declined Electron's 900×600). It
applies to first launch only: stored window bounds override it.

Measured: Electron's 900×600 first-launch area against Wails' 1280×800 is **1.90×** (540 000 px² vs
1 024 000 px²), and the floor ratio is **1.21×**. "Cost scales with viewport area" is a projection
from one 1440×960 harness (`git show 7d8a72ab4a5e:docs/WEBVIEW-SCROLL-MEMORY.md`), not a measurement;
no megabyte figure exists for the new default. The clamp's effect on a real first launch is unverified
on any platform here (Wails v3.0.0-beta.16 resolves no primary screen before `Run()`).

### 2.1c P22 spike — the SlickGrid A/B, and how to run it on real hardware

**Superseded 2026-09 (P29 §3.1): the two-engine comparison below can no longer be run as written.**
`window.__kiraGridEngine` and the incumbent `DataGrid.vue`/`@tanstack/vue-virtual` grid it switched
away from are both deleted — the SlickGrid cutover this section's own A/B fed into is complete
(`bd6c369`, `402f42e`), and there's exactly one grid engine in the tree now. The recorded numbers,
the finding (SlickGrid's own wheel handler ate trackpad momentum), and the reasoning below stay as
the evidence for that cutover; steps 1-5's two-engine comparison itself is history, not a protocol to
re-run. Step 6's `window.__kiraGridTuning` A/B, and every A/B further down this section, still work
standalone and are rewritten below to drop the now-dead engine-switch step. **Run any of them from a
build where the hooks are actually compiled in** (P29 F1 gates `window.__kira*` behind
`__KIRA_DEBUG_HOOKS__`): `bun run dev` (`wails3 task dev`)'s Vite dev server, or a native build via
`build:dev`/`build:test` — **never** a packaged `.dmg`, where they're gone by design.

**What this section isn't: it doesn't say the fast-scroll lag is fixed.** Pass A
(`docs/HISTORY.md (v1.1 P22)`) built a second, additive grid —
`views/grid/SlickGridHost.vue`, wrapping `6pac/SlickGrid`'s core engine — behind
`window.__kiraGridEngine === 'slick'`, mounted by `views/grid/DataView.vue` instead of the
incumbent `DataGrid.vue`/`@tanstack/vue-virtual` grid. The incumbent stays the default and every
pre-existing `tests/ui/`/`tests/unit/` spec keeps passing against it, unmodified. What Pass A proves
is mechanism, not the user's actual symptom: `tests/ui/slick-grid.spec.ts` gates, in this sandbox,
that the new bridge decodes real data correctly, the decode-cache stays pinned to the mounted window
(P5 C1), a sub-row scroll mutates nothing, the mounted-cell count stays bounded, the at-rest runway
covers at least what the incumbent's does for the same fixture, and — the item this phase's own
brief singled out — closing the tab actually tears the grid down (`grid.destroy(true)`, no leaked
`.slick-viewport`, no leaked `<style>` element, `__kiraRetention()` back at its pre-open reading).
**None of that's the same claim as "the reported lag is gone."** §7.4(a)/(b) of the plan split those
two claims into separate columns on purpose; this section is the protocol for the second one, which
only real hardware can answer.

**The gate.** Per the plan's own §7.4(b): PASS authorises Pass B (feature parity + cutover);
INCONCLUSIVE or FAIL don't, and hand the question to the plan's §11 (a runway-only fix, or a
conclusion that no DOM grid can help — see that section for the reasoning). **The result of this A/B
isn't yet known — it has never been run.** Whoever runs it should update this section, and the
plan's own §7.4(b)/§8.6, with the real numbers and the real verdict once it has been.

**Protocol — one build, both engines, the same real fling twice (superseded, kept verbatim as the
historical record of the A/B this ran; step 3's engine switch no longer exists — see the note at
this section's head).** This is §2.1a's own protocol (above), doubled: run it once against the
incumbent grid, once against the spike, same window size, same table, same page size, same hard
two-finger flick.

1. `bun run dev`, open a 50 000+-row table at page size 10 000, comfortable density — exactly §2.1a
   step 1.
2. Web Inspector → Console → `__kiraScrollTrace.start()`, one hard two-finger flick, let momentum
   die, `copy(JSON.stringify(__kiraScrollTrace.stop()))`. This is the **incumbent** run — do it
   first, before touching the engine switch, so both runs come from the same build.
3. `window.__kiraGridEngine = 'slick'`, then reload the tab (a fresh mount reads the switch once —
   `DataView.vue`'s own comment on why it's not reactive). Re-open the same table at the same page
   size and density.
4. Repeat step 2 against the spike grid — `__kiraScrollTrace.registerGrid` works against either
   engine on this same build (D9's own `rowSelector` parameter, `.slick-row` for the spike,
   `[data-testid="grid-row"]` for the incumbent), so no rebuild is needed between the two runs.
5. Report, for **each** run: `summary.pxPerFrame` (p50/p95/max), `summary.uncoveredPx`
   (p50/p95/max), `summary.renderMs` (p50/p95/max), the `scrollEvents` histogram, and one sentence on
   whether the lag was *perceptibly* different between the two — the perceptual read is what actually
   answers the original report; the numbers are what keep that read honest (§9's own discipline,
   restated in the plan's own §7.4(b) header).
6. **Still directly runnable today** (a live console line, no rebuild — the two-engine comparison
   above isn't a prerequisite): `window.__kiraGridTuning.maxLeadPxOverride = 4000` (§7.3 step 6's
   own cheap next experiment) is a five-minute test that costs nothing and separates "the renderer
   is too slow" from "the runway is too short" — see the plan's §7.4(b) INCONCLUSIVE branch for the
   full reasoning.

**Found and fixed on a real-hardware run of this protocol: SlickGrid's own wheel handler ate
trackpad momentum.** `enableMouseWheelScrollHandler` (default `true`) quantizes every wheel/trackpad
tick to `deltaY * rowHeight` in JS instead of letting the native `scroll` listener (already bound,
already sufficient — see the plan's F3 addendum) and WebKit's own momentum physics drive `scrollTop`
directly. That's what read on macOS as "that fluid velocity sensitive mac scroll is gone." Fixed by
`enableMouseWheelScrollHandler: false` in `SlickGridHost.vue`'s grid options; frozen-pane sync and
this app's own velocity/runway logic are unaffected, since both already run off native `scroll`, not
off SlickGrid's wheel-specific internals.

**Bundle size, measured in this session** (`bun run build`, before/after this phase's own C2/C5,
gzip): the launch chunk grew from **353.31 KB to 397.64 KB** (**+44.33 KB**), and the CSS asset from
21.75 KB to 22.90 KB (**+1.15 KB**) — **+45.48 KB total**, against the plan's own **≤45 000 B**
ceiling (§7.4(a) item 7) and its **~42 KB JS / ~1.3 KB CSS** projection (F9). This is **~480 B (about
1%) over the plan's stated ceiling** — flagged here plainly rather than rounded away. It isn't
F9's own named failure mode: an isolated re-measurement of `{ SlickGrid, SlickEventHandler }` alone
(the only two named imports from `slickgrid` anywhere in the tree) reproduces F9's own ~42 KB figure
almost exactly (41 960 B gzip, esbuild@0.28.2, this session) — so nothing pulled in `SlickDataView`
or a plugin. The extra ~2.4 KB is this phase's own new application code (`SlickGridHost.vue`,
`views/grid/slick/dataSource.ts`, `views/grid/slick/kiraSlickGrid.ts`), which the plan's own ≤45 000 B
figure budgeted headroom for but not quite enough of. Worth a look if Pass B adds materially more
code to this path; not, on its own, a reason to hold Pass A back — the acceptance criterion's real
concern (an accidentally-imported `SlickDataView`/plugin) is affirmatively ruled out here, not just
asserted.

**P22 iter2-scroll-gaps: fast-fling rendering gaps and dropped frames, found while attempting to run
the protocol above.** Once the wheel-handler fix (above) restored genuinely native trackpad momentum,
a real-Mac run of this same protocol surfaced two further symptoms on very fast flicks: brief
rendering gaps at the leading edge (content not yet painted where the compositor already scrolled to)
and occasional dropped frames. Doubling `leadFramesOverride`/`maxLeadPxOverride` live from the
console — the first, most obvious lever — made no difference, which is itself a real finding: those
two overrides govern how *wide* the target window is, not how much of that window's *newly-entering*
portion a single synchronous SlickGrid `render()` call is allowed to build, and a fling landing in
never-before-mounted territory has no `rowsCache` overlap either way — doubling the runway only
doubles the size of the batch that was already too large.

Traced against SlickGrid 5.20.0's own source
(`docs/HISTORY.md (v1.1 P22)` §1-§3, cited by line number) to
`render()`'s own unconditional cost: every row entering `rowsCache` for the first time pays a
synchronous decode (`page.ts`/`store.ts`, real and un-batched, but memoised after the first access)
**and** unconditional per-cell DOM construction (`Utils32.createDomElement`, class-list joins,
attribute writes — paid identically whether or not the decode cache happens to be warm, because the
build/skip decision is keyed on `rowsCache` membership, never on decode freshness). A secondary,
independent contributor: SlickGrid's own `_handleScroll` defers to a 10 ms-windowed
`scrollThrottle.enqueue()` whenever a single frame's delta exceeds one viewport height, which a hard
fling's high end reaches routinely — deferring the render rather than speeding it up, on a macrotask
boundary unrelated to the display's own frame cadence.

**The fix** (landed as four commits, `docs/HISTORY.md (v1.1 P22)`
§5-§6): `KiraSlickGrid.getRenderedRange` now always returns the strictly-visible range in full — nothing
on screen is ever deferred — and caps the *extra* runway per call at `MAX_NEW_CELLS_PER_RENDER = 600`
cells (`columns.ts`, provisional like `LEAD_FRAMES`/`MAX_LEAD_PX`, overridable live via
`window.__kiraGridTuning.maxNewCellsPerRenderOverride`), independent of `CELL_BUDGET`'s total-window
cap. When the velocity-scaled target exceeds what the per-call budget allows, the shortfall is made up
by a self-scheduled `requestAnimationFrame`-chained `render()` that converges on the full target over a
handful of frames — turning "synchronous cost proportional to how far the fling jumped" into
"synchronous cost bounded by viewport size, converging over N frames." `forceSyncScrolling: true`
(only safe once the batch is capped — a separate, later commit, deliberately bisectable from the batch
cap) removes the 10 ms throttle-deferral branch entirely. `scrollTrace.ts` also gained `noteRenderMs`
— before this pass, `__kiraScrollTrace.renderMs`/`.notified` were **always 0/false** for the SlickGrid
engine (a stale comment claimed otherwise; nothing actually called `noteNotify()` on this path), so the
protocol below was unusable for this engine until that was fixed first, alone, ahead of the batch cap.

**What this phase can and can't claim from the sandbox alone.** The mechanism is confirmed and
refined from SlickGrid's own source, and the batch a single render pass can be forced to build is now
provably bounded independent of fling distance (`tests/ui/slick-grid.spec.ts`'s own convergence
assertion: the mounted `.slick-cell` count after a jump well past the whole runway grows over several
animation frames rather than appearing all at once). **Nothing here claims the gap/drop symptom itself
is fixed** — like every other perceptual/latency question in this document's history, only a real
compositor can answer that, and this sandbox has none.

**The extended real-Mac protocol — same setup, same gesture, same `__kiraScrollTrace`, now with real
`renderMs` for this engine.** Extends the steps above rather than replacing them (the two symptoms
were found *during* an attempt to run that exact protocol):

1. **Baseline**, on a build with only the `noteRenderMs` instrumentation fix landed (no batch cap, no
   `forceSyncScrolling`): `__kiraScrollTrace.start()`, one hard two-finger flick, let momentum die,
   `copy(JSON.stringify(__kiraScrollTrace.stop()))`. Report `summary.renderMs` (p50/p95/max) — the
   first *real* number this engine has ever produced — and `summary.uncoveredPx` (p50/p95/max). Note,
   qualitatively, whether the gap/drop happens on this build.
2. **With the batch cap and `forceSyncScrolling` both landed**, same build, same gesture: repeat step
   1's trace, report the same numbers, plus one sentence on whether the gap/drop was *perceptibly*
   different.
3. **The A/B on `MAX_NEW_CELLS_PER_RENDER` itself**, batch cap live:
   `window.__kiraGridTuning.maxNewCellsPerRenderOverride = 150` (small — more, cheaper chase frames),
   then `= 2200` (equal to `CELL_BUDGET` — no capping at all, today's pre-fix behaviour in effect), same
   flick each time. If the gap/drop tracks this dial — worse at 2200, better at 150 — that's the
   direct, decisive real-hardware confirmation of the mechanism.
4. **Isolate `forceSyncScrolling` from the batch cap**: `window.__kiraGridTuning.
   forceSyncScrollingOverride` now toggles it live (`true`/`false`), so this no longer needs a
   build-level A/B — compare both values against the same batch cap, same gesture.

Report the three `summary` blocks (baseline, both-fixes, the `maxNewCellsPerRenderOverride` sweep),
this section's own `pxPerFrame`/`scrollEvents` context so the fling being compared is the same shape
each time, and one sentence per step on whether the gap/drop was perceptible.

**Real-hardware update (first run): the D2+D3 build stutters, and is worse than the incumbent's own
motion smoothness.** `forceSyncScrolling: true` (unconditional, `0865ef6`) couples main-thread render
work to every native scroll-event tick during a fling — on real macOS hardware this produced visible
stutter (the motion itself hitching), a less forgivable failure than the incumbent tanstack grid's own
"content lags, motion stays smooth" gap symptom. `forceSyncScrolling` has been defaulted back to
`false` (D2's batch cap alone) and made live-toggleable
(`window.__kiraGridTuning.forceSyncScrollingOverride`), so step 4 above can be re-run from the console
without a rebuild per variant. **Still open**: whether D2's batch cap alone (without D3) actually
restores smooth motion, or the gap/drop symptom persists at `forceSyncScrolling: false` too — whoever
runs the full A/B above should update this section with the real numbers and verdict.

**Found chasing the above with a real Safari Web Inspector Timeline recording: `slickTheme.css`'s port
of `GridRow.vue` had dropped `contain: layout` from `.slick-row`, fragmenting every paint into
one-per-cell** (`docs/HISTORY.md (v1.1 P22)` §10 — mechanism,
proof, and fix). SlickGrid's own `slick.grid.css` gives `.slick-cell` a non-`auto` `z-index`, making
every cell its own stacking context; without `.slick-row` itself being one, each cell's stacking
context escaped to the far-away scroller ancestor instead of nesting in its row. Proven reversibly, on
both engines, with real WebKit computed-style checks and Chromium CDP paint-count tracing: 20,054
paints (~89% cell-sized) without the declaration, 3,399 (0% cell-sized) with it restored, matching the
incumbent grid's own profile; geometry confirmed byte-identical either way. Fixed by restoring
`contain: layout;` to `.slick-grid-host .slick-row`. **Still not confirmed on real macOS hardware**:
paint count isn't the same claim as reduced frame stutter — the original real-Mac recording left
~73% of frame wall-clock time unattributed to anything a WebContent-process trace can see (likely
GPU-process rasterization/compositing downstream of paint count), so a fresh real-Mac Timeline
recording of the same hard-fling scenario, before/after this fix, is still needed and is the next step
for whoever picks this back up.

**P22 iter2-pacing: the chase-frame doubling, its fix, and the real-Mac protocol that judges it.**
The `contain: layout` recording above (fce3e54) was also the first real second-derivative reading
this investigation has: p50 41.2 → 32.1 ms, p95 99.7 → 50.5 ms, composite cost per frame 10.19 →
4.34 ms — genuinely better — but **pacing stayed uneven**, durations swinging ~17-58 ms within the
same fling, and the share of frame time unattributable to a WebContent-process trace *rose* to
**88%**. The one actionable finding in that recording: frames in which the self-scheduled catch-up
("chase") render fires average **49.8 ms against 33.3 ms** for frames without one — a mechanism this
app owns outright, root-caused and fixed in
`docs/HISTORY.md (v1.1 P22)`. Read that document for the full
evidence (a real WebKit reproduction — two render() passes in 131 of 140 wheel-driven frames — and
the source citations behind it); this entry is the real-Mac protocol it specifies, folded into this
section per that plan's own instruction, not a new one.

*What changed, in one paragraph.* The chase never spread work across frames while a fling was in
flight — it fired on the very next `requestAnimationFrame` unconditionally, which (per HTML's *run
the scroll steps*) is with overwhelming probability a frame that also carries that frame's own
scroll-driven render, doubling every fixed per-render cost. The fix (`CHASE_QUIET_MS`,
`kiraSlickGrid.ts`'s `scheduleChase`) gates the chase on scroll **quiescence** instead — deliberately
*not* "did a render already run this frame?", because two sandbox harnesses gave opposite intra-frame
orderings for the two renders (a `scrollTop +=` write from inside a rAF vs. a `page.mouse.wheel`
drive), so a same-frame-token fix would be right in one engine/input-path and wrong in another.
Live-tunable (`window.__kiraGridTuning.chaseQuietMsOverride`, `0` = the exact pre-fix policy) so the
A/B below needs one build, not a rebuild per variant. `__kiraScrollTrace` also gained the signal this
protocol is judged by: `frameMs` (the frame-duration series itself), `renderCount` per frame, a
`renderMs` that's now a **sum reset every frame** (before this pass it was a single sticky value
that never reset — a no-render frame silently reported the *previous* frame's cost, and a doubled
frame reported only its second render's own cost), and — the pacing signal specifically —
**`mean`/`stddev` on every `ScrollTraceStats` block**, not just p50/p95/max. **"Proper pacing" is a
statement about spread; a percentile alone can't answer it, which is why `frameMs.stddev` is the
number every step below is actually judged by, not `frameMs.mean`.**

**Don't compare a `renderMs` figure recorded before this pass to one recorded after it — they're
not the same quantity.** Every `renderMs`/`renderCount` number in this document's history before P22
iter2-pacing landed came from the sticky, never-reset field; treat every one of them as an
order-of-magnitude sanity check only, never as a baseline this protocol's own numbers should match.

*What this pass is allowed to claim from the sandbox alone, and what it isn't.* Provable here: that
the chase collided with the scroll render under the old policy (measured, two engines); that after
the fix it provably never does (`tests/ui/slick-grid.spec.ts`'s T1, gated against a real WebKit
reproduction, with T3 proving the harness would have caught the old behaviour); that the trace now
reports per-frame counts and sums instead of a stale value (T5); that a chase left armed at teardown
no longer re-enters `render()` against a torn-down grid (T4, verified to throw
`TypeError: undefined is not an object (evaluating 'r.isNull')` with the teardown fix reverted, and
to pass stably three runs in a row with it in place). **Nothing here claims the fix reduces perceived
stutter, or that `frameMs.stddev` actually falls on real hardware — this investigation's own history
(`forceSyncScrolling`, `0865ef6` → `18a2cc4`, directly above) is the standing reason a sandbox-plausible
fix doesn't get to certify itself. The fix isn't confirmed working until a real Mac trace, run
through the protocol below, says so.**

*The three readings that would refute this pass — stated before running, not after* (the plan's own
§7.2): **(a)** `renderCountHistogram` on the baseline shows essentially no 2-render frames — the
doubling doesn't occur on real hardware, and the fix is aimed at nothing; revert
`chaseQuietMsOverride`'s default (already an identity operation at `0`) and re-open the investigation.
**(b)** The histogram goes to all-1 after the fix but `frameMs.stddev` doesn't move — the chase was
never the pacing term, the remaining variance is in the invisible (GPU-process) bucket, and the
honest next step is Xcode Instruments, not another app-level pass; this is the branch the plan
considers most likely if (a) passes but the numbers stay flat. **(c)** `uncoveredPx` p95 regresses
materially — the runway is now too thin during a fling; lower `chaseQuietMsOverride` or raise the
per-frame runway cap, both console lines.

**Protocol — same build, same 50 000+-row table at page size 10 000, comfortable density, one hard
two-finger flick per run, momentum allowed to die,
`copy(JSON.stringify(__kiraScrollTrace.stop()))`.** Extends §2.1c's own protocol above (same setup,
minus its now-dead engine-switch step — there's one engine now, so no reload-to-switch is needed)
rather than replacing it.

**Report for every run:** `summary.frameMs` `{mean, stddev, p50, p95, max}` — the pacing number and
the point of the whole exercise; `summary.renderCountHistogram`; `summary.renderMs` (now a per-frame
sum); `summary.uncoveredPx`; `summary.pxPerFrame`; `summary.scrollEventsHistogram`; the share of
`scrollTopAtEvent[].afterRaf` that's `true` (§3 F2's own correction: this session's sandbox found
`true` on essentially every event under a wheel-driven scroll in WebKit, the opposite of this
module's original premise — a real-Mac `true` share is expected, not a bug signal, until this
protocol's own first reading says otherwise); and one sentence on whether the motion *felt*
different. The perceptual sentence is what answers the report; the numbers keep it honest.

1. **Baseline, on a build with only the scrollTrace fix landed** (`fix(grid): per-frame render
   accounting in __kiraScrollTrace`) **— no pacing fix yet.** This is the first trace in this
   engine's history whose `renderMs` is a real per-frame sum rather than a sticky last-render value.
   The decisive reading is `renderCountHistogram`: a substantial share of frames at 2 confirms the
   doubling on real hardware. Refuting reading (a) above if it doesn't.
2. **With the pacing fix landed**, same gesture. `renderCountHistogram` should be ~all-1 during the
   fling; `frameMs.stddev` and `renderMs` p95 should both fall; `uncoveredPx` p95 must not rise
   materially. Refuting reading (b) if the histogram clears but `frameMs.stddev` stays flat.
3. **The chase-quiescence A/B, live, no rebuild:** `window.__kiraGridTuning.chaseQuietMsOverride = 0`
   (pre-fix behaviour), then unset (24, the default), then `= 48`. Same flick each time. If
   `frameMs.stddev` tracks this dial, that's the direct, decisive confirmation of the mechanism —
   and whatever this step says is what re-sets `CHASE_QUIET_MS` (provisional, like every other
   runway constant before it).
4. **The runway-growth-cap A/B:** `window.__kiraGridTuning.maxNewLeadCellsPerRenderOverride = 600`
   (neutral/today's default), then `= 200`, then `= 100`. Read `frameMs.stddev` **and**
   `uncoveredPx` p95 *together* — this dial trades one against the other by design (smaller, more
   even runway steps vs. slower convergence), and a setting that lowers stddev while pushing
   `uncoveredPx` past the baseline isn't an improvement. Set `MAX_NEW_LEAD_CELLS_PER_RENDER` from
   whatever this step says, or leave it neutral (its shipped default) if it says nothing. Refuting
   reading (c) if `uncoveredPx` regresses materially anywhere in this pass.
5. **Re-run this section's own step 4** (`forceSyncScrollingOverride` true/false) — the chase fix
   changes what synchronous scrolling costs per frame, so the earlier verdict recorded above doesn't
   carry over.
6. **A Safari Web Inspector Timeline recording alongside step 2**, same capture shape as the one that
   found the `contain: layout` bug. Look specifically for whether the rAF-attributed script records
   that used to sit adjacent to the scroll-attributed ones are gone — a confirmation of de-stacking
   independent of the app's own instrumentation, worth having because refuting reading (a) is a real
   possibility and the app's own trace isn't a neutral witness to it.
7. Update this section with the real numbers and verdict, and update
   `docs/HISTORY.md (v1.1 P22)` §7/§10 to match. **This entry is
   not that update** — it's the protocol, written before anyone has run it on real hardware, exactly
   like `forceSyncScrolling`'s own history two sections above should be a reminder to keep it that
   way until a real trace actually reports back.

**P22 iter2-onset: the gesture-onset artifact, a correction to the pacing gate, and how to A/B
both.** After the pacing fix above landed, real-hardware feedback was that the grid "feels really
good" at a consistent ~30ms/frame, with one small artifact left: at the very *start* of a fast
fling — peak velocity, before deceleration begins — there's a brief, visible moment, a few frames
long, where content isn't fully rendered. Once deceleration starts it's gone. This entry extends
the protocol above (same setup, same build, same gesture); it isn't a new section, and it doesn't
supersede anything above.

*What was wrong, and it wasn't the chase.* SlickGrid binds its own viewport `scroll` listener
inside `finishInitialization()` (`slickgrid` `dist/esm/index.js:7572`, reached from the constructor
because `explicitInitialization: false`). `SlickGridHost.vue` binds its own `onViewportScroll` on
that same element only *after* `new KiraSlickGrid(...)` returns. Two non-capturing listeners on one
target fire in registration order, so SlickGrid's `handleScroll` — and the synchronous `render()` →
`getRenderedRange()` → `velocity()` it drives — **always ran one sample ahead of the host's own
sampling of the very event that triggered it**. Mid-fling that's invisible (velocity barely moves
frame to frame). At the first render of a fresh gesture it isn't: the only sample on hand was taken
*before* the gesture, so the sampler's own 150ms at-rest test fires and `velocity()` returns
`{0, 0}`. The grid sizes its runway as if standing still — `target` collapses to the base runway,
the per-call new-cell budget (~50 rows at 12 mounted columns) is left entirely unspent, and
`getRenderedRange` doesn't even flag a deficit, because the range it returned *does* reach that
collapsed target. One whole frame of runway-building is lost at the exact moment a fling needs it
most, **on every gesture** — which is why the artifact appeared at the start of every fling and not
only the first one in a tab. The fix samples the viewport at the point of consumption, so no render
can run on a sample older than the event that triggered it.

*The alternative that was considered and rejected, stated so nobody re-proposes it.* Letting the
first catch-up render after a rest-to-motion transition bypass the quiescence gate. It reintroduces
exactly one doubled frame per gesture, at the moment the user is most sensitive, on a "was it idle?"
heuristic that a main-thread stall can false-positive mid-fling — and it can't help on the frame
that actually matters, because on the onset frame the pre-fix code doesn't *want* a chase at all
(`chaseWanted` is false, since `target` collapsed), so there's nothing for a bypass to release. The
shipped fix never touches the chase gate.

*The correction to the pacing pass, found while measuring this one, and the more important half of
this entry.* **`CHASE_QUIET_MS` is 24ms, and the real-Mac recording the pacing pass was written from
reports a p50 frame duration of 32.1ms — the threshold is already shorter than one frame on the
hardware it was designed for.** When a frame outlasts it, "24ms since the last scroll event" stops
meaning "no scroll event is driving this frame": the event was simply in the previous, long, frame,
which is precisely a frame the main thread is already behind on. The gate then opens on a frame that
*does* carry a scroll-driven render. Measured here, once a chase is actually wanted on every frame:
**7 to 9 doubled frames out of ~80** — the exact regime the pacing fix exists to prevent. It went
unnoticed because the gate was never really being asked: the stale sampler above reported *zero*
velocity on much of a fast fling, `target` collapsed, no chase was wanted, and the gate was passing
its own test by not being exercised. So the wall-clock gate is now joined by a **per-frame** one a
slow frame can't outrun — a catch-up render additionally requires that no native `scroll` event
arrived between the previous animation frame and this one, read as a sequence number rather than a
duration (the refinement the plan's own §10 anticipated). It stays ordering-agnostic: it never asks
whether the scroll render or the chase ran first inside a frame, only whether a scroll event
happened across the last one. It's strictly conservative — it can only ever *delay* a chase, never
release one — and its cost is that after a fling stops the chase converges from the second quiet
frame rather than the first.

*What the sandbox is allowed to claim here, and what it isn't.* Provable here, and proved
(`tests/ui/slick-grid.spec.ts`, the `P22 iter2-onset` test): that under the old policy a frame's
runway velocity is *exactly* the previous frame's measured `pxPerFrame`, for the whole length of a
fling (the one-sample lag is measured, not inferred); that after the fix no render sizes its runway
at rest while the viewport is moving, across three rest-to-motion transitions; that the pacing
invariant still holds on that same recording (no frame renders twice); and that removing the new
per-frame gate re-introduces doubled frames on the same fling, so the invariant holds *because* of
it. **Nothing here claims the onset artifact is gone. It isn't confirmed fixed until a real Mac
says so — this investigation's own history (`forceSyncScrolling`, `0865ef6` → `18a2cc4`) is the
standing reason a sandbox-plausible fix doesn't get to certify itself.**

*The readings that would refute this pass, stated before running.* **(a)** `staleVelocityFrames` on
the baseline (with `freshVelocitySampleOverride = false`) is ~0 during a real fling — then the
sampler wasn't stale on real hardware, the mechanism doesn't occur there, and the fix is aimed at
nothing. **(b)** `staleVelocityFrames` goes to 0 and the onset artifact is still visible — then the
runway's cold start wasn't what produced it; the remaining suspects are the per-call new-cell cap
(the runway still needs ~2 budget-capped frames to converge, which this fix shortens but doesn't
eliminate) and the GPU-process bucket §7.4 hands forward. **(c)** `renderCountHistogram` gains
2-render frames after this pass — then the new per-frame gate isn't doing what the sandbox says it
does, and `chaseFrameGateOverride` is the first thing to A/B.

**Protocol — same build, same table, same gesture as the pacing protocol above.** Report
`summary.staleVelocityFrames`, `summary.runwayVelocity` `{mean, p50, p95, max}` **next to**
`summary.pxPerFrame`'s own (the gap between the two is the runway's input lag, and it's what this
pass closes), plus everything the pacing protocol above already asks for, and one sentence on
whether the onset artifact is still there.

1. **Baseline, live, no rebuild:** `window.__kiraGridTuning.freshVelocitySampleOverride = false`,
   then one hard flick from a dead stop. Expect `staleVelocityFrames` >= 1 per gesture and
   `runwayVelocity` visibly lagging `pxPerFrame`. Refuting reading (a) if it doesn't.
2. **Unset it** (the shipped default) and repeat the same flick. `staleVelocityFrames` should be 0
   and `runwayVelocity` should track `pxPerFrame`. **The reading that matters is the perceptual
   one: is the few-frames-of-unrendered-content at the very start of the fling still there?**
   Refuting reading (b) if the numbers move and the artifact doesn't.
3. **The per-frame chase gate A/B:** `window.__kiraGridTuning.chaseFrameGateOverride = false`, same
   flick, then unset. Read `renderCountHistogram` and `frameMs.stddev`. If the histogram gains
   2-render frames with it off, the sandbox measurement reproduces on real hardware and the gate is
   load-bearing; if it doesn't, the wall-clock gate was sufficient there and this half can be
   reconsidered (do **not** simply delete it — a threshold shorter than a frame is wrong in
   principle whether or not this particular gesture exposes it).
4. **Re-run steps 3 and 4 of the pacing protocol above** (`chaseQuietMsOverride`,
   `maxNewLeadCellsPerRenderOverride`) *after* this pass. Both dials now sit behind a different
   chase gate and a different runway input, so their earlier verdicts — which were never actually
   run — must not be inherited from the text above.
5. **If the artifact survives step 2**, the next candidate is named and bounded: the per-call
   new-cell cap means the runway still takes roughly two budget-capped frames to converge from a
   standing start, and this fix removes the *wasted* first frame but not the cap itself. Raising
   `maxNewCellsPerRenderOverride` for one run is the console-line experiment that says whether the
   cap is the remaining term; **it trades directly against the single-frame batch cost the
   iter2-scroll-gaps pass existed to bound**, so a win there's a re-opened trade-off, not a free
   fix.
6. Fold the result into this section and update
   `docs/HISTORY.md (v1.1 P22)` §11. **This entry isn't that
   update** — it's the protocol, written before anyone has run it on real hardware.

**The ceiling, restated so nobody re-derives it** (the plan's own §7.4): even the calmest frames in
the post-`fce3e54` recording — before the heavy scroll-dispatch bursts start, with no chase firing —
ran **23.5-24.5 ms**, past the 16.7 ms a 60 Hz budget allows, with composite + script together
accounting for only a few ms of that. **~18-20 ms of even the best frame is in the GPU-process
bucket**, invisible to `__kiraScrollTrace`, to a WebContent-process Timeline, and to this sandbox.
Nothing in this pass — or the two before it — can reach it. This is a measured floor from the tools
available, not a proven hard ceiling; Xcode Instruments, which this project doesn't have access to,
is what would settle whether it's addressable at all. This is exactly why the pass optimises
**variance**, per the user's own stated goal ("smooth, ideally 60fps, but it can be lower with proper
pacing"), rather than average frame rate.

### 2.2 Memory budget — `tests/e2e/memory.spec.ts` (removed)

**Status: the budget fails here no matter what, on non-app-controllable process
overhead — `tests/e2e/memory.spec.ts` was removed rather than kept red forever.** This section is the
documented structural finding that justifies the removal. Per plan decision D21, the assertion stayed
unmodified through every pre-approved lever and failed every time; the spec was deleted once the
levers were exhausted.

Scenario (D4): 5 connections (2× Postgres, MariaDB, MongoDB, Redis), 10 tabs. Min of 10
`app.getAppMetrics()` samples: **739.0 MB** at baseline (0 connections), **764.8 MB** loaded (max
836.2 MB). The assertion `< 350 MB` failed identically across 3 runs (`Received: 801984512`).

**Structural finding.** Baseline overhead alone — Browser + GPU + NetworkService + Tab, everything
except the engine — is ≈ **620–626 MB**, ~270 MB over budget with zero connections and zero data. That
is Chromium/Electron process overhead no P12 lever acts on. The baseline-to-loaded gap (≈ 25–97 MB
across the five processes) is where the app's own footprint shows.

**Lever L-A (eager driver loading) — fired.** `engine/adapters/registry.ts` imported all six
adapters statically, so every driver sat in the engine process from boot (engine RSS ~151 MB with
zero connections, trigger > 100 MB). Per-kind lazy `import()` loaders dropped it to ~119 MB. ~32 MB
recovered, small next to the ~450 MB gap. Caveat (P32): the Kafka driver became the native
`@confluentinc/kafka-javascript`, whose compiled code and librdkafka buffers stay resident once loaded
and are invisible to V8's GC, so lazy loading only defers that cost. Not re-measured.

**Other levers, none fired:**

| Lever | Trigger | Measured | Fired? |
|---|---|---|---|
| L-B renderer page retention | Tab RSS delta (10 vs 0 tabs) > 120 MB and retained bytes explain < half | delta ≈ 18–43 MB | no |
| L-C L2 budget default | steady state at the 64 MB budget and total RSS > 350 MB | L2 at 0.3 MB / 64 MB | no |
| L-D bundle weight | packaged `.app` > 300 MB on disk | not re-measured for the Wails bundle; the old 252 MB figure was an electron-builder build that no longer exists. `scripts/vendor-node.sh` and the vendored Node are gone (P58f M10). Record `du -sh "apps/kira-space/bin/Kira Space.app"` (§3) on the next macOS run | unknown |
| L-E engine old-space cap | peak engine RSS > 400 MB in any §3 scenario | peak 155.6 MB | no |
| L-F cached tree expand | p95 > 50 ms | 2.8 ms | no |
| L-G cell editor populate | p95 > 50 ms | 4.9 ms | no |
| L-H scroll response | p95 > 8 ms | p95 14.2 ms, a frame-scheduling artifact (§2.1 methodology note); p50 5.6 ms is under budget | no (see caveat) |

Per D21: only L-A fired; the 350 MB budget still fails, dominated by Chromium/Electron overhead.
Candidates left for a human decision, not in P12's scope: a different process model, a higher budget
for this class of app, or measuring only the app-attributable delta.

### 2.3 P52 gate G1 — Wails/Go walking-skeleton RSS (Linux sandbox; not the gate's own verdict)

**Status: measured, and explicitly NOT a G1 verdict.** G1 is defined against a real macOS arm64
build; this sandbox is Linux x86_64 with WebKitGTK 2.52.3, structurally unlike WKWebView. The verdict
is §2.4.

Instrument: `apps/kira-space/cmd/g1measure` over `internal/metrics`' `gopsutil` RSS summation (the
same package behind `kira:app:metrics`), cross-checked against `ps --forest`. It is gate-G1
measurement scaffolding. On Linux, WebKitGTK's `WebKitNetworkProcess` and `WebKitWebProcess` are true
children of the Go process, with `WebKitWebProcess` re-executed through two `bwrap` wrappers (1.6-2.2
MB each). Method matches §2.2: 10 samples 1 s apart, idle, minimum.

Min of 10: blank page **616.3 MB**; real renderer with the nine boot-path reads **689.5 MB**
(`WebKitWebProcess` 317.8 MB, Go shell 274.7 MB). The engine child alone was 45-46 MB (≤ 150 MB
check passes, platform-independent). Read against §2.2's 620-626 MB Electron baseline, this build is
not smaller on Linux, because WebKitGTK is a full separate library instance. That motivated the real
macOS re-run. The blank-page scaffold (`KIRA_G1_BLANK=1`, `apps/kira-space/blank/`) was removed in
P29.

### 2.4 P52 gate G1 — real macOS arm64 result and verdict: **Go**

Re-run on real Apple Silicon (macOS 26.5.2), app built, signed and launched, same instrument and
method as §2.3, `g1measure` cross-checked against `ps -o rss=` (agreed to rounding). Min of 10:
blank **216.3 MB**, real renderer **261.7 MB** (Go shell 103.1, `node` 40.6, WebContent 71.9, GPU
30.4, Networking 15.2).

**Verdict: 261.7 MB ≤ 300 MB = Go**, a **≈58% reduction** against §2.2's 620–626 MB Electron
baseline before any `src/main` port. The engine child alone was 40.6–41.6 MB, under the 150 MB
ceiling. P53 was authorized.

Getting this number surfaced three bugs, all fixed: `CFBundleExecutable` underscores versus the
hyphenated binary (broke `codesign --verify`); dangling `bin/npm`/`npx`/`corepack` symlinks left by
`scripts/vendor-node.sh` (failed `codesign --deep --strict`); and `internal/metrics.MatchingPIDs`'
substring match counting other apps' WebKit helpers (+~87 MB, ≈300 MB reported for a real 261.7 MB).
The last is fixed by `internal/metrics.AppProcessSet`, which on darwin counts a helper only when
`responsibility_get_pid_responsible_for_pid` resolves it to one of this app's anchor pids.

**Methodology note.** Launch via Finder/`open`: a directly `exec`'d binary does not establish the
responsibility chain and undercounts (2 of 5 processes found). `KIRA_G1_BLANK=1` reaches an
`open`-launched process via `launchctl setenv`/`unsetenv`.

### 2.5 P58a M2 — chunk wire-encoding: Go base64 vs the Node engine's index-keyed JSON

Controlled re-take of the P52-P57 ~11x/48x figures (P58 §1.4, P58a §4.6), while both producers still
existed. Fixture, identical on both sides: a 2000-row `TextColumnChunk`, 49-byte ASCII value per
non-NULL row, 1 row in 97 NULL, 1 in 53 truncated; raw size **103,394 bytes**. Throwaway programs,
not committed.

| Encoding | Wire bytes | Ratio to raw | Transient heap (one encode call) |
|---|---|---|---|
| Go, base64 of exact LE bytes (D5) | 137,914 | **1.334x** | 709,312 B (6.86x) |
| Node engine, index-keyed JSON object | 1,124,081 | **10.872x** | 4,232,600 B (40.9x) |

The wire comparison is the one D5 was decided on and reproduces the predicted ~1.33x/~11x. The heap
row confirms direction and order of magnitude of the old ~48x, but the two runtimes expose different
instruments (`runtime.MemStats.TotalAlloc` delta versus `heapUsed` after a forced GC), so 40.9x is
this fixture's own number, not a reproduction.

### L2 cache note (D19)

L2's 64 MB default budget and its `> budget / 2` no-cache refusal rule (one page larger than half the
budget is never cached, so it cannot evict everything) are unchanged and user-adjustable in Settings →
Cache. Consequence: at page size 10 000 rows on a wide table, one page can exceed 32 MB and is then
never cached, whatever the headroom.

### 2.6 P4 — page-encode cost, and what the successor binary envelope would additionally buy

Cost of the JSON+base64 encoder itself, before and after `perf(page): encode a page without the
nested json.Marshal round trips`, wire bytes unchanged. Timed unit: `json.Marshal(wireResponse{...})`,
`adapterhost/dataframe.go`'s exact `respond` expression, via a verbatim copy of `internal/page`
(median of 5-300 in-process repetitions; allocation is the `TotalAlloc` delta). Full findings:
`docs/HISTORY.md (v1.1 P4)`.

| Fixture | Wire bytes | Before | After | Speedup | Alloc/wire ratio, before → after |
|---|---|---|---|---|---|
| 100 × 12 | 46,558 | 332 µs / 114.0 KB | 38 µs / 50.0 KB | 8.7x | 2.45x → 1.07x |
| 1,000 × 12 | 448,081 | 3.46 ms / 1.13 MB | 0.37 ms / 0.45 MB | 9.5x | 2.53x → 1.01x |
| 10,000 × 12 | 4,462,372 | 32.0 ms / 11.9 MB | 4.38 ms / 4.83 MB | 7.3x | 2.67x → 1.08x |
| 10,000 × 40 | 35,985,413 | 448 ms / 113.1 MB | 30.7 ms / 36.0 MB | 14.6x | 3.14x → 1.00x |

Wire bytes are identical before and after on all four fixtures (the program aborts on a mismatch).
Lean on the allocation ratio, not the wall-clock speedup (GC noise at large sizes): transient
allocation fell from 2.4-3.1x the frame size to almost exactly 1.00x.

Still unaffected by this change: base64 inflates the wire **1.33-1.36x**, surviving gzip at the same
ratio, and the frontend decode pass (`TextDecoder` → `JSON.parse` → `reviveChunks`) costs
**0.2-38 ms per page view** under a JavaScriptCore proxy. A binary envelope removes both. The P4 plan
(§4, R5) specified it but deferred the build until a trigger fires (budget regression with decode
implicated, a page-kind or default-size change, or a frontend/backend network split). This commit
closes roughly 85-90% of the Go-side gap to that envelope.

### 2.7 P11 — the FlatBuffers data-plane measurements

Wire-format change: the same four fixtures through the real FlatBuffers pipeline
(`internal/page.EncodePage` plus `adapterhost/frame.go`) and `packages/shared/protocol/frame.ts`'s
`decodeFrame`, against JSON+base64 at `f1efaab`. Full findings: `docs/HISTORY.md (v1.1 P11)`. Fixtures
are built with the real `internal/page` builder (12 text columns of 24-byte cells; 40 of 64-byte for
the wide case; 1 row in 97 NULL); median of repetitions, allocation in a separate pass. Decode runs
under Bun/JavaScriptCore, a proxy for WKWebView, not an equivalent. Nothing committed.

**Wire bytes: -24.3% to -24.8%** at every size (e.g. 10,000 × 12: 4,498,980 → 3,373,784 against
3,373,516 raw). **Overhead versus raw buffer bytes:**

| Fixture | Before (JSON+base64) | After (FlatBuffers) |
|---|---|---|
| 100 × 12 | 1.361x | **1.008x** |
| 1,000 × 12 | 1.336x | **1.001x** |
| 10,000 × 12 | 1.334x | **1.00008x** |
| 10,000 × 40 | 1.333x | **1.00001x** |

The fixed per-frame overhead (`"KIF1"` identifier, table wrappers) is a few hundred bytes and
vanishes at real page sizes.

**Frontend decode:** 6x (100 × 12: 0.0650 → 0.0106 ms), 140x, 556x, **1,141x** (10,000 × 40: 41.4034
→ 0.0363 ms). A FlatBuffers frame decodes to typed-array views over the received bytes, with no
`JSON.parse` and no base64 materialisation.

**Go encode was slower after the switch, 1.3x to 2.5x**, with allocation ratio rising from ~1.0x to
**3.95-6.49x** the wire size. Two causes, both fixed in §2.8:

- `flatbuffers.NewBuilder(0)` starts empty and grows by doubling, copying its whole backing array each
  time. This is the dominant allocation cost, and the reason the ratio rose rather than converged.
- The `offsets`/`truncated` `[uint]` vectors (P11 D4) were a `PrependUint32` call per element, about
  400,000 calls at the widest fixture, which is a meaningful share of the encode-time regression.

`"KIF1"` as the file identifier lets `FinishedBytes()` go straight to `Send`. A length-prefixed header
cannot be prepended in place to a back-to-front FlatBuffers build, so it would copy the entire frame
(~27 MB at the widest fixture) on every response. `ChunkByteSize` is unchanged (P11 D9), so L2
accounting is unchanged.

### 2.8 — the Go-side encode regression, fixed

§2.7's regression fixed without touching the wire format (same bytes, §8.3 correctness proof re-run
on 12 fixtures including a new 500-row wide one):

1. **`adapterhost/frame.go` sizes the builder from `page.Page.Size()`** (the measured `ByteSize`)
   instead of `NewBuilder(0)`. Alone, this recovers the entire allocation regression.
2. **`internal/page/encode.go`'s `createUint32Vector` bulk-copies** `offsets`/`truncated`. On a
   little-endian host a `[]uint32` already matches the wire layout, so it reinterprets the slice as
   `[]byte` via `unsafe.Slice`, reuses `CreateByteVector`'s single copy, and patches the length prefix
   to the element count. A runtime little-endian check falls back to the per-element loop. Alone, it
   cuts wall-clock time but not allocation.

| Fixture | §2.7 time / alloc-ratio | §2.8 time / alloc-ratio | Time speedup |
|---|---|---|---|
| 100 × 12 | 52.9 µs / 6.49x | 6.5 µs / **1.18x** | 8.1x |
| 1,000 × 12 | 953.1 µs / 5.39x | 49.9 µs / **1.07x** | 19.1x |
| 10,000 × 12 | 5.89 ms / 4.18x | 685 µs / **1.06x** | 8.6x |
| 10,000 × 40 | 63.35 ms / 3.95x | 7.09 ms / **1.06x** | 8.9x |

This clears the original §2.6 JSON+base64 baseline: **5.2-7.7x faster** wall-clock at every fixture,
with an allocation ratio (1.06-1.18x) near JSON's own (1.00-1.05x). No axis remains where the pre-P11
codec beats the current one.

### 2.9 P5: the renderer's own memory

Method: production bundle booted in **Chromium** (V8, a proxy for the shipped JavaScriptCore)
against `tests/ui` mock fixtures; heap via `HeapProfiler.collectGarbage` x2 then
`Runtime.getHeapUsage`; `window.__kiraRetention` counts structures directly. Full findings are
`docs/HISTORY.md (v1.1 P5)`. Conclusions:

- Grid/console control (10 000 x 12 result): **+2.49 MB**, flat when scrolled; C1-C7 do not touch it.
- Mongo document tab after C3 pruning: **-21% at 100 docs, -25% at 1 000, -32% at 5 000** versus
  baseline. Retained parse/decode rows drop from the whole page to the rendered window plus
  overscan (100 → 18).
- C4 find cap: ~101 B/match, so 50 000 matches ≈ 5 MB versus 38.8 MB at 400 000 uncapped.
- C5 schema tree-shaking: −1,910 B (−0.18%) of the JS chunk.
- C7 multi-page frame sharing: closing one of two `Run all` results now frees its own share
  (frame buffers `{1, 31,256 B}` → per-page `{2, 6,000 B}` → `{1, 3,000 B}`).
- Declined, no change: D8 (CodeMirror async chunk; premise gone since P60a moved to Monaco) and D9
  (`sessionQueueBytes`: peak 35.4 MiB against the 32 MiB budget, so the budget works as designed).

### 2.10 P12 round 1 — bundle size re-measured, P18's own "unchanged in size" claim now stale

P18's EXPLAIN code (`ExplainResultView.vue` plus five plan parsers) grew `index-*.js` to
**1,117,138 B raw / 356,081 B gzip**, +5.6% from P15's 1,057,495 B / 334,906 B. Correctly eager:
well under the size P13/P15 split dependencies out at. `sqlFormatterEntry-*.js` 141,507 B and
`fakerEntry-*.js` 416,026 B stayed separate lazy chunks. No dependency leaked into the main chunk.

### 2.11 P19 — bundle re-measured under Vite 8/Rolldown; a Green Tea GC caveat on every Go-side RSS/CPU figure above

Vite 8.2.2/Rolldown: `index-*.js` **1,115,370 B raw / 351.14 KB gzip**, `sqlFormatterEntry-*.js`
130,746 B / 37.41 KB, `fakerEntry-*.js` 415,801 B / 155.46 KB. Both lazy splits survive Rolldown.

**Green Tea GC caveat.** Go 1.26+ enables Green Tea (10-40% less GC overhead). Every Go-side
wall-clock figure in §2.3-§2.8 predates it and may move; allocation figures (`TotalAlloc`,
"Alloc/wire ratio", "Transient heap") count bytes handed out and stand. Frontend decode timings run
under Bun/JavaScriptCore and are unaffected. Re-run before reading wall-clock numbers as current.

### 2.12 P21 — bundle re-measured at the close of the v1.1 chapter; §2.11 confirmed still current

`62c7e84`: `index-*.js` **1,115,990 B raw / 351.31 KB gzip** (+0.06% against §2.11, noise),
`index-*.css` 120,067 B / 21.77 KB (first recorded here); both lazy chunks byte-identical to §2.11.
The Green Tea caveat still applies; no Go-side wall-clock figure was re-measured.

### 2.13 G3/G8 — the git module's transport, re-measured rather than re-derived

Upstream's ≤300 ms first-paint budget was measured over in-process `postMessage`. v1.3 replaced
that with a Unix socket plus FlatBuffers framing, so `docs/HISTORY.md (v1.3)` carried a standing note
asking for re-measurement, not re-derivation. G3 built the probe (`TestGraphStreamPerf`) and G8
extended it to nine (`TestG8PerfBaseline`), both in `internal/gitsock/` at the time (the
suites now live in `apps/kira-space/internal/flows/gitflow/`, P243; `gitsock` is gone). Both are
**opt-in** — `KIRA_GIT_PERF=1`, skipped in `-short`, skipped without `git` on `PATH` — and **assert
nothing**: each prints one `key=value` line. That's a decision, not an omission: a hard threshold
in a suite that also runs on real macOS hardware would be flaky in exactly the way
"re-measurement, not re-derivation" warns against.

**This container (Linux, Go 1.27, git 2.43):**

| Probe | Result |
|---|---|
| `graph.stream`, 20 000 commits, one connection | `chunks=10 firstChunk=218ms total=220ms meanBytes/chunk=42167` |
| Same, two connections on **one** repository | A `firstChunk=230ms total=232ms`; B `233ms`/`235ms`; wall `235ms` |
| Same, two connections on **two** repositories (8 000 commits) | A `106ms`/`110ms`; B `110ms`/`110ms`; wall `110ms` |
| `commit.detail` x50 | solo mean `6.05ms`, p95 `7.01ms`; two connections mean `0.097ms`, p95 `0.178ms` (cache-warm) |
| Large patch / blob | `commit.fileDiff` 282 B encoded in `97ms`; `file.read` 1.48 MB encoded (1.44 MB raw) in `49ms` |
| Ranged walk, 200 commits — upstream's own ≤300 ms case | `total=9.5ms` |
| Chunk-size distribution, 20 000 commits | min `42156` / mean `42167` / max `42272` / p99 `42156` bytes |
| Watcher fan-out latency, 1 / 2 / 8 connections | all `~203-204ms` — the watcher's own 200 ms debounce, not fan-out cost |
| `inotify` watches, 2 000 loose refs | 4 watches — a Linux proxy only; it does not answer the macOS kqueue fd-cost question, which G9 removed structurally by switching to FSEvents |

**Three things this says beyond the numbers.** First, **the budget holds with margin**: a
20 000-commit first page lands at 218-235 ms against ≤300 ms, one connection or two, and the ranged
walk the budget was originally written for is 9.5 ms. Second, **`firstChunk ≈ total`** — all ten
chunks leave within about 2 ms of each other, because a whole page (`logsession.DefaultPageSize =
5000`) is read and parsed into the commit store before anything is emitted. First paint is bounded
by `git log` plus parsing, **not** by the socket and not by FlatBuffers, so further transport
optimisation would buy nothing and a page-size change is the lever that would. Third, a page is
~422 KB in 10 chunks of ~42 KB — three orders of magnitude under the 8 MiB frame cap, so frame
sizing isn't a live constraint.

**Not measured on real hardware.** These are container numbers, like every other figure in §2. The
macOS equivalents belong in §3's manual procedures, which have still not been run.

### 2.14 G34 — the visual-parity phase's own CSS delta, measured after the fact

G34's exit criterion #9 asked for the built CSS asset size before and after (±3 KB raw expected);
none of its commits recorded it. Measured later with the Vite production build at `521fcba` versus
`bf0a1d8`: raw 65,409 B → 61,466 B (**−3,943 B**), gzip 9,867 B → 9,700 B (−167 B). A shrink: shared
component classes replaced duplicated `.kv-*` styling. Container build, not a packaged one.

### 2.15 P481 — frontend RAM wins (P480 probe)

Base `c9ec9d14` (chapter `v0.2.28` after P479). Host: 4-core container, Linux 6.18, Chromium
headless shell 1243, WebKit 2370. Probe: `heap-ram.spec.ts` of each app, Pass A (minified build,
3 repeats, median [spread]); the probe's load gate waits for `load1 <= 1.0`. Commands:
`KIRA_RAM_WORLD=docker bunx playwright test --config=apps/kira-space/playwright.perf.config.ts --project=heap heap-ram --repeat-each=3`
and `KIRA_RAM_WORLD=memory,diff,ade` for Space (`apps/kira-space/playwright.perf.config.ts`).
"Before" was measured on the base commit, "after" on the fixed tree, same host.

Docker overview table (`ContainerTable.vue`, 400 containers, checkpoint `list`):

| number | before | after |
|---|---|---|
| V8 heap MB | 64.3 [0.0] | 18.1 [0.0] |
| renderer RSS MB | 237 [1] | 168 [2] |
| DOM nodes | 26,832 | 3,270 |
| WebKit renderer RSS MB | 619 (P480) | 478 [2] |

Other docker checkpoints moved by at most -1.1 MB heap (`logs0` 16.2 to 15.3, `left` 14.5 to 13.7).

Memory list (`MemoryPanel.vue`, checkpoint `list`; memory500 before is 2 repeats, the first run
was cut off by a tool timeout):

| number | before | after |
|---|---|---|
| memory500 heap MB | 19.9 [0.0] | 12.4 [0.0] |
| memory500 renderer RSS MB | 173 [0] | 154 [3] |
| memory500 DOM nodes | 9,682 | 1,044 |
| memory50 heap MB | 12.5 [0.0] | 12.2 [0.0] |

Closed diff tabs (`RepoDiffView.vue`, 10 tabs opened then closed, checkpoint `closed`):

| number | before | after |
|---|---|---|
| V8 heap MB | 33.5 [0.0] | 31.3 [0.0] |
| DOM nodes (counter) | 3,196 | 2,568 |
| Pass B: `StandaloneDiffEditor2` | 10 | 1 |
| Pass B: `Detached <div>` retained | 3.5 MB | 0.78 MB |

The one editor left is held by Monaco's global hover delegate (`vscodeWindowId` getter, then
`InstantiationService`, then the editor's context-key service), not by app code. Heap saving
2.2 MB meets the 2 MB threshold; the planned 28.5 to 30 MB range was not reached.

ADE first mount retained after leaving (ade20, `left` 24.9 to `back` 37.9 MB): not fixed. Three
trials, none moved `back`: (1) `useCandidates` queryFn without caller getters (Pass B, 3,618
components still retained); (2) probe without `page.clock.install()` (left 24.8, back 37.7, so the
fake clock is not the cause); (3) `autocomplete="off"` on the capture box, title and estimate
inputs (left 24.9, back 37.9). Inputs present at the `task` checkpoint: `ade-capture`,
`ade-panel-title`, `ade-task-host-input`, `ade-estimate-num`. Open item in `docs/ARCHITECTURE.md`.

## 3. Manual procedures (macOS, packaged build)

Not yet run — no macOS hardware here. Run these once on macOS 26+ arm64 and record the results
here. **Rewritten for the Wails/Go bundle at P57 M8**: the steps below are a
re-pointed procedure, not a re-measurement — nothing in this section has been executed on real
hardware since the migration, and no number in this file changed as a result of the rewrite.

**The bundle these procedures run against.** `bun run package` (`cd apps/kira-space && wails3 task
darwin:package:dmg`, then `scripts/sign-bundle.sh`; see `docs/PACKAGING.md`) produces both
**`apps/kira-space/bin/Kira Space.app`** and, around it, **`Kira Space.dmg`** — the shipped
artifact as of P10. electron-builder's `dist/mac-arm64/` output, its `app.asar` and its
`out/main/` entry points no longer exist. As of P58f M10 there's also no `Contents/MacOS/runtime/`
tree of any kind — no vendored Node runtime, no engine-child bundle — since every adapter now runs
natively inside the one Go binary. The only measurement-relevant path inside the bundle is
`Contents/MacOS/Kira Space` itself.

**Tab switch (P139 Part 2).** Open `big_rows` and `wide_table` data tabs. In the packaged app's Web
Inspector, confirm `CSS.supports('margin-trim: inline')` is `true`. Record a Timeline over 20
alternating tab switches: median and p95 of click to header cell present against the 50 ms budget,
and the count of full style recalculations per switch (expect none from SlickGrid). Record the
numbers here.

**Packaged cold start** (target: ≤ 1500 ms median of 3 warm launches):
1. `bun run package`.
2. Launch `apps/kira-space/bin/Kira Space.app` 3 times via Finder or `open`, discarding the first
   (Gatekeeper's quarantine scan of an ad-hoc-signed bundle on first launch isn't the app's cost).
   Launch it the way a user would rather than `exec`ing the binary — §2.4's own methodology note
   records that a directly-`exec`'d run is a structurally different process set on macOS.
3. Read the cold-start line from `~/.kira-studio/logs/kira-<YYYY-MM-DD>.log` for each of the 3
   launches and take the median. That line is emitted by `apps/kira-space/internal/shell/window.go`'s
   `events.Common.WindowRuntimeReady` handler (P56 — Wails' analogue of `did-finish-load`, and the
   measurement point that replaces Electron's `process.uptime()`, which no longer exists): it reads
   `msg="did-finish-load at uptime <N>ms" scope=startup`, where `<N>` is milliseconds from
   `apps/kira-space/main.go`'s `startedAt` (captured at process entry) to the frontend runtime being ready.
   A packaged run writes to the file only — the stderr copy is dev-only (`config.IsDev()`).
4. Record: `<median> ms — <date>, <machine>`.

**Packaged RSS** (target: < 350 MB total, 5 connections / 10 tabs — §2.2's own automated version of
this scenario was removed as a permanently-failing, non-app-controllable finding; this manual
packaged check is what's left):
1. With the packaged app running, build the scenario by hand: 2× Postgres + MariaDB + MongoDB +
   Redis connections; 4 Postgres tabs + 2 MariaDB tabs + 2 MongoDB tabs + 2 Redis tabs, all loaded.
2. Read the memory figure from the app's own status bar (bottom right, `data-testid=app-metrics-mem`).
   That readout *is* the replacement for `app.getAppMetrics()`: `internal/metrics`' `Sampler` +
   `Ticker` sum CPU and, on darwin, **`ri_phys_footprint`** (not RSS — since P7,
   `internal/metrics/probe_darwin.go` reads `proc_pid_rusage(RUSAGE_INFO_V2)` per pid per tick)
   across the app's own process set every 5 s (`metrics.Interval`) and emit it as `kira:app:metrics`.
   There's no separate manual command to run for the headline number, and no per-process breakdown
   — it's one app-wide figure by construction (§2.2's per-process table has no equivalent here).
3. **Optional second opinion — not the same instrument as step 2, and the two numbers aren't
   directly comparable.** `cd apps/kira-space && go run ./cmd/g1measure` (its `-anchor`/`-helper`
   defaults are `metrics.AnchorNeedles`/`HelperNeedles`; min of 10 samples 1 s apart) and a plain
   `ps -o rss=` sum both report **RSS**, never phys_footprint — `cmd/g1measure/main.go` sums
   gopsutil's `mi.RSS` and was never updated when P7 switched the status-bar readout to
   phys_footprint. RSS counts shared dyld/WebKit pages once per process; phys_footprint counts them
   once total — so an RSS sum and a phys_footprint reading over the same process set are two
   different numbers, not two ways of measuring the same one (`docs/ARCHITECTURE.md`'s explanation
   of the difference). The **350 MB budget below was set against an RSS sum** (§2.2), so a step-2
   phys_footprint reading is *not* a like-for-like comparison against it — record the RSS figure
   (step 3) against the budget, and record the status-bar figure too if in doubt which one a future
   reader will want. As of P58f M10 the process set for either instrument is just the `Kira Space`
   binary and the `com.apple.WebKit.*` helpers (there's no vendored Node process left to include).
   Don't grep `com.apple.WebKit` by hand unfiltered: it also matches every *other* running app's
   idle WebKit helpers, the over-count §2.4's third bug records (≈ 87 MB of other apps on that
   machine) and the reason `metrics.AppProcessSet` exists.
4. Record both figures if in doubt, and label each with which instrument it came from: `<RSS total>
   MB (g1measure/ps) / <phys_footprint total> MB (status bar) — <date>, <machine>`.

**Window-bounds debounce timer on close (F8, D8)** — now `apps/kira-space/internal/shell/window.go`: the
resize/move debouncer (300 ms, `boundsDebounce`) is cancelled on `events.Common.WindowClosing`, and
again by `Attach`'s detach at quit. **The Electron-era symptom this check watched for doesn't carry
over.** It looked for the process lingering on an unref'd-but-still-pending `setTimeout`; a pending
Go `time.AfterFunc` never holds process exit, so "exits promptly" can't fail here and isn't worth
a manual run. What is still worth verifying by hand is D8's deliberate *non-flush*: move the
packaged app's window and let it settle (> 300 ms, so that rectangle is persisted), move it again
and quit within 300 ms of that second move, then relaunch and confirm the window comes back at the
**first** rectangle — the in-flight bounds write is dropped on purpose, not flushed. Re-aimed from
the old check, not re-measured.

**Op-log reconciliation on an engine crash (F10, D10)** — **no manual procedure any more, as of
P58f M10.** There's no separate engine child process left to kill: every adapter runs in-process
inside the one Go binary, so "kill the child" is no longer a distinct failure mode — it would mean
killing the whole app. The reconciliation this check protected now happens at two levels instead,
both covered by automated tests rather than a manual run: (1) `adapterhost.Host.safeRun` (P58 D16)
recovers a panic inside any single adapter call into a normal failed op (`E_INTERNAL`) on that op's
own `op:end`, so one adapter's crash can't leave that op stuck `running`; (2) `internal/oplog`'s
`Wiring.finishInFlight` — generalised from "the engine child died" to "the event source is done"
(P58f D9) — finishes any row still `running` when the op-event channel itself closes, e.g. at
shutdown. A hard kill of the whole process (SIGKILL, OOM, a panic outside `safeRun`) still leaves
`running` rows on next launch, same as before P58f; there's nothing left this manual procedure
would exercise that `apps/kira-space/internal/adapterhost` and `apps/kira-space/internal/studiooplog`'s own unit tests don't
already cover.

**Log file retention (F12, D12)** — now `internal/logging`'s `Sweep`, which deletes `kira-*.log`
files older than `LogRetentionDays` (30) by mtime at startup. Verify by backdating a log file's mtime
past 30 days in `~/.kira-studio/logs/`, relaunching the packaged app, and confirming that file is
gone while newer ones remain.

**App size (lever L-D)** — `du -sh "apps/kira-space/bin/Kira Space.app"`, against the > 300 MB trigger. See
§2.2's lever table for what that row currently does and doesn't claim.

## 4. P13's nonfunctional sweep — the three items P12 handed forward

All three resolved in P13. Their cache coverage moved to Go with L2/L3 (P58a A21) and is asserted in
`apps/kira-space/internal/enginecache/lru_test.go`'s `TestByteLru_*` cases.

1. **L3 count store had no byte budget — fixed (F19, D19).** Now a `ByteLru` like L2:
   `L3_BUDGET_BYTES = 256 * 1024`, `COUNT_ENTRY_BYTES = 128`, so 2 048 entries before eviction
   (ported into `internal/enginecache`). Driving 2 500 distinct `{path, filter}` pairs plateaued
   `l3Entries` at exactly 2 048.
2. **Page retention for inactive tabs (lever L-B) — answered, not fixed (D21).** Tab close releases
   pages (P13 F4/F5, `tests/ui/leaks.spec.ts`). Inactive-but-open tabs keep theirs: the 10-tab delta
   is ≈ 18–43 MB (2–4 MB per tab, §2.2), and evicting would risk the ≤ 50 ms cached-tab-switch
   budget (§2.1) for single-digit megabytes. Declined twice.
3. **L2 `hits`/`misses` were lifetime-cumulative — fixed (F20, D20).** `clearPages()` resets both, so
   the hit rate after Settings → Cache → Clear reads `—`.

## 5. Test-suite duration (P294, replaces the v1.4 P1 baseline)

Measured on this box (4 cores, 15 GB, Docker 29, real WebKit). Plan and method:
`git show ca2fa17c172b:docs/v2.6/plans/P294-test-ci-speed.md`. CI numbers were not measured.

| Item | Before | After |
|---|---|---|
| Pull 11 test images | 22 manual pull/tag lines | `bun run test:images`, 60s cold |
| Go containers packages, `-count=1` | in `go test ./...`, 218s total | `bun run test:go:containers` 81s |
| Go other packages, `-count=1` | same | `bun run test:go:rest` 134s |
| Go rerun, no changes | 218s | containers 1.5s (cached); rest 114s (`ade` 44s and `i18n` never cache) |
| Playwright `ui`, 4 workers | 598s, 546 tests | 2 shards 304s + 321s, 273 tests each (CI later moved to 4 shards, not re-measured) |
| Playwright `ipc-frontend` | 7 of 7 fail, missing headless shell | 7 passed, 11s |
| Pre-commit, docs-only commit | 43s | 0.5s |
| Pre-commit, Go-only commit | 43s | 0.1s |
| Pre-commit, studio frontend commit | 43s warm | 56s (lint rules 17s plus typecheck of the changed projects) |
| Pre-push, Go-only range | 12s warm, plus tests none | 10s (build, lint and test the changed package) |

The `ui` project is CPU bound: 2340s of WebKit work, about 10 min on 4 cores. See the plan for the CI
job times.
