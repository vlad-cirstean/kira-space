# P480 RAM findings

Report-only. No app code changed. Probe: `packages/workbench/src/testing/ui/memProbe.ts`, `heapSnapshot.ts`; worlds in each app's `tests/perf/heap-ram.spec.ts`.

## 1. Header

- Base commit: `37da4f5f` (P480 plan landed at `40878183`).
- Host: 4 vCPU, 16 GB RAM, Linux 6.18. One run at a time, no overlap (WebKit RSS is machine-wide).
- Browsers: Playwright 1.64.1; Chromium build 1243 (CDP), WebKit build 2370.
- RSS path: Chromium via browser CDP `SystemInfo.getProcessInfo` pids plus `ps -o rss` (renderer and total). WebKit via `ps` match, machine-wide, so renderer RSS and total coincide there. Settle 2 s, minimum of 3 readings 250 ms apart. Heap: `HeapProfiler.collectGarbage` twice, then `Runtime.getHeapUsage`.
- Pass A: minified test build with debug hooks, 3 repeats, `heap` (Chromium) and `heap-webkit` projects, both apps. Studio Chromium 39 tests, Space Chromium 36, Studio WebKit 39, Space WebKit 36; all passed.
- Pass B: unminified build, once per app (`KIRA_RAM_SNAPSHOTS=1`): Studio 14 tests, Space 12 tests, all passed. A targeted drill ran afterwards: closed2 to closed3 snapshot diff for Studio `api`, Space `diff` and `terminal`; left to back diff plus 3 extra mode visits for Space `ade`.
- Load: host otherwise idle; Chromium Pass A used a CPU-busy gate (at most 25 % over 3 s) before each test.
- Measured noise (Chromium, 3 repeats): heap spread 0.0 to 0.2 MB in most rows, 0.4 to 0.7 MB in `discover`, 3.7 MB at Space `diff open1`. Renderer RSS spread 0 to 10 MB, 85 MB at Space `repo-file closed` (allocator timing). DOM node spread 0 to 20. WebKit world rows: spread 1 to 66 MB; WebKit idle rows show spreads over 1 GB (a repeat sampled another WebKit process), so only WebKit medians are usable. WebKit gives RSS only.
- Fake clock: worlds install `page.clock`, so TanStack `gcTime` timers (5 min) never fire unless the world fast-forwards. Studio tab worlds fast-forward 6 min at `closed` (`closed6m`). Space worlds do not.
- Pass B limits: named-root sizes are absent from the first snapshot of every test (V8 object ids resolve only once a snapshot exists) and are dominator-based lower bounds. Pass A heap deltas are the authoritative cost. The unminified build adds 10 to 27 MB `ExternalStringData` (source text) to every Pass B total; ignore it.
- Mock caveat: Space `graph50k boot` heap (19.0 MB) holds the git stream mock's fixture chunks. Use `graph10k` for the baseline.

## 2. Threshold

Proven when all hold (from the plan, with the noise above):

- Median saving at least 2 MB V8 heap, or at least 10 MB renderer RSS, or a leak of at least 0.5 MB per open/close cycle growing across `closed`, `closed2`, `closed3` in all 3 repeats.
- Saving at least 5x the measured spread of that number. Heap spread is 0.0 to 0.2 MB in the rows used below; RSS spread 1 to 5 MB.
- Pass B attributes the bytes to the code the fix changes.
- The fix keeps P433's rule: tab switching never loses state or refetches.

Baseline drift: every tab world gains 0.17 to 0.38 MB heap per open/close cycle with flat DOM node counts. The Pass B diffs (Studio `api`, Space `diff`, `terminal`, closed2 to closed3) show it is `(compiled code)` (V8 JIT, 0.2 to 0.4 MB) plus Blink performance-timing entries, not app objects. Only Studio `api` reaches 0.58 to 0.59 MB per cycle in all 3 repeats, and its diff shows no app constructor growing, so no leak is proven.

## 3. Idle tables

Median [spread]. Heap MB and renderer RSS MB are Chromium; WebKit RSS MB is machine-wide.

### Studio: idle

| checkpoint | heap MB | renderer RSS MB | WebKit RSS MB | DOM nodes |
|---|---|---|---|---|
| boot | 11.0 [0.0] | 148 [1] | 423 [1610] | 459 [0] |
| boot60 | 11.1 [0.0] | 140 [1] | 417 [1871] | 459 [0] |
| mode:api | 11.4 [0.0] | 140 [1] | 430 [2162] | 527 [0] |
| back:api | 11.6 [0.0] | 140 [1] | 431 [1668] | 595 [0] |
| mode:automations | 12.0 [0.0] | 142 [1] | 433 [1849] | 690 [0] |
| back:automations | 11.9 [0.0] | 141 [1] | 435 [1249] | 608 [0] |
| mode:docker | 12.3 [0.0] | 145 [0] | 441 [1652] | 631 [0] |
| back:docker | 12.2 [0.1] | 143 [1] | 439 [1967] | 620 [0] |
| mode:kubernetes | 12.4 [0.0] | 144 [1] | 444 [1467] | 635 [0] |
| back:kubernetes | 12.5 [0.1] | 147 [1] | 445 [1577] | 632 [0] |

### Space: idle

| checkpoint | heap MB | renderer RSS MB | WebKit RSS MB | DOM nodes |
|---|---|---|---|---|
| boot | 10.6 [0.0] | 148 [10] | 420 [1] | 524 [0] |
| boot60 | 10.6 [0.0] | 140 [4] | 414 [1] | 524 [0] |
| mode:ade | 12.1 [0.0] | 143 [4] | 429 [1] | 751 [0] |
| back:ade | 12.5 [0.0] | 143 [4] | 432 [1] | 943 [0] |
| mode:automations | 12.6 [0.0] | 144 [3] | 435 [3] | 948 [0] |
| back:automations | 12.8 [0.0] | 144 [4] | 437 [4] | 956 [0] |
| mode:memory | 13.0 [0.0] | 146 [1] | 441 [7] | 985 [0] |
| back:memory | 13.1 [0.0] | 146 [1] | 447 [8] | 968 [0] |

Idle is flat: `boot60` equals `boot` within 0.1 MB heap (no timer growth over 60 s). Each mode visited adds 0.3 to 0.5 MB heap (lazy chunks); `back:<m>` adds at most 0.2 MB more. Idle Studio: about 11 MB heap, 140 to 148 MB renderer RSS, 459 DOM nodes.

## 4. World tables

Tab worlds: each cell is `heap MB / renderer RSS MB`; slope and leak are `heap / RSS` per tab and per cycle. `closed` is after closing every tab; `closed2` and `closed3` follow two more open/close cycles. Step worlds list each checkpoint.

### Studio: tab worlds (heap MB / renderer RSS MB, median [spread]; slope = (openN-open1)/(N-1); leak = (closed3-closed)/2)

| world | N | mode | open1 | openN | slope | round | closed | closed2 | closed3 | leak/cycle | WebKit RSS mode, openN, closed |
|---|---|---|---|---|---|---|---|---|---|---|---|
| grid | 10 | 14.8 / 155 | 24.9 / 186 | 26.7 / 191 | 0.21 / 0.6 | 27.0 / 192 | 25.2 / 188 | 25.7 / 192 | 25.9 / 190 | 0.31 / 1.3 | 459, 612, 605 |
| console | 10 | 14.8 / 155 | 25.5 / 194 | 29.6 / 202 | 0.47 / 0.8 | 30.0 / 203 | 28.7 / 196 | 28.9 / 199 | 29.3 / 196 | 0.33 / -0.1 | 456, 613, 655 |
| definition | 10 | 14.8 / 154 | 16.0 / 162 | 17.4 / 168 | 0.16 / 0.7 | 17.6 / 169 | 16.5 / 167 | 16.7 / 168 | 17.1 / 169 | 0.29 / 1.1 | 457, 515, 520 |
| document | 10 | 13.6 / 150 | 23.7 / 183 | 25.6 / 190 | 0.21 / 0.8 | 25.9 / 191 | 23.6 / 186 | 23.9 / 188 | 24.1 / 187 | 0.24 / 0.3 | 449, 605, 629 |
| keyvalue | 10 | 13.3 / 149 | 22.2 / 171 | 20.8 / 175 | -0.16 / 0.5 | 20.9 / 176 | 16.4 / 169 | 16.5 / 170 | 16.7 / 167 | 0.17 / -0.8 | 446, 549, 566 |
| stream | 10 | 13.2 / 149 | 14.6 / 157 | 16.9 / 165 | 0.25 / 0.8 | 17.1 / 165 | 16.1 / 163 | 16.3 / 165 | 16.5 / 165 | 0.20 / 0.8 | 446, 509, 523 |
| api | 10 | 23.0 / 180 | 23.0 / 180 | 26.8 / 190 | 0.42 / 1.1 | 27.5 / 192 | 26.1 / 191 | 26.8 / 193 | 27.2 / 194 | 0.59 / 1.9 | 547, 598, 621 |
| terminal | 10 | 11.6 / 150 | 14.8 / 174 | 20.1 / 229 | 0.58 / 6.2 | 20.2 / 229 | 14.8 / 170 | 15.4 / 174 | 15.6 / 177 | 0.38 / 3.3 | 430, 589, 593 |


### Studio: step worlds (heap MB / renderer RSS MB / DOM nodes / WebKit RSS MB)

| world | checkpoint | heap | renderer RSS | DOM nodes | WebKit RSS |
|---|---|---|---|---|---|
| discover | boot | 13.4 [0.0] | 151 [1] | 1106 [0] | 446 [1] |
| discover | open1 | 31.2 [0.7] | 222 [3] | 3322 [20] | 652 [29] |
| discover | round1 | 32.5 [0.2] | 228 [7] | 3422 [0] | 685 [30] |
| discover | round2 | 33.6 [0.2] | 233 [4] | 3422 [0] | 710 [22] |
| discover | round3 | 34.4 [0.4] | 235 [5] | 3422 [20] | 731 [16] |
| discover | round4 | 35.2 [0.4] | 237 [4] | 3422 [20] | 742 [12] |
| discover | round5 | 35.2 [0.2] | 237 [4] | 3422 [20] | 742 [33] |
| discover | round6 | 35.4 [0.7] | 237 [2] | 3422 [20] | 711 [18] |
| discover | round7 | 35.7 [0.7] | 238 [4] | 3422 [0] | 725 [56] |
| discover | round8 | 37.6 [0.6] | 237 [6] | 3422 [20] | 720 [63] |
| discover | closed | 34.1 [0.2] | 206 [3] | 2888 [0] | 726 [66] |
| docker | boot | 11.0 [0.0] | 147 [2] | 459 [0] | 422 [1] |
| docker | list | 64.5 [0.2] | 237 [2] | 26835 [0] | 619 [4] |
| docker | logs0 | 16.3 [0.2] | 169 [1] | 1559 [0] | 613 [2] |
| docker | logs1k | 16.7 [0.2] | 171 [1] | 2325 [0] | 616 [2] |
| docker | logs10k | 17.9 [0.2] | 172 [1] | 2325 [0] | 644 [1] |
| docker | logs50k | 19.0 [0.2] | 176 [1] | 2325 [0] | 731 [7] |
| docker | left | 14.6 [0.2] | 168 [6] | 481 [0] | 737 [5] |
| kubernetes | boot | 11.0 [0.0] | 147 [2] | 459 [0] | 422 [2] |
| kubernetes | panel | 11.6 [0.2] | 150 [2] | 487 [0] | 431 [3] |
| kubernetes | logs0 | 13.1 [0.2] | 156 [1] | 1089 [0] | 450 [5] |
| kubernetes | logs1k | 13.4 [0.2] | 158 [2] | 1390 [0] | 452 [4] |
| kubernetes | logs10k | 14.8 [0.2] | 161 [2] | 1390 [0] | 459 [8] |
| kubernetes | logs50k | 16.0 [0.2] | 166 [2] | 1390 [0] | 503 [4] |
| kubernetes | left | 15.8 [0.2] | 169 [10] | 1263 [0] | 508 [6] |
| documents-list | boot | 14.8 [0.0] | 162 [2] | 459 [0] | 435 [2] |
| documents-list | open100 | 28.3 [0.0] | 208 [10] | 2592 [4] | 582 [2] |
| documents-list | open5000 | 30.6 [0.0] | 202 [1] | 2604 [4] | 654 [8] |
| documents-list | closed | 26.6 [0.0] | 199 [1] | 1092 [4] | 655 [9] |

### Space: tab worlds (heap MB / renderer RSS MB, median [spread]; slope = (openN-open1)/(N-1); leak = (closed3-closed)/2)

| world | N | mode | open1 | openN | slope | round | closed | closed2 | closed3 | leak/cycle | WebKit RSS mode, openN, closed |
|---|---|---|---|---|---|---|---|---|---|---|---|
| repo-file | 10 | 15.8 / 168 | 30.3 / 234 | 54.9 / 355 | 2.73 / 13.4 | 59.2 / 387 | 32.5 / 327 | 32.8 / 322 | 33.0 / 344 | 0.26 / 8.4 | 474, 830, 830 |
| diff | 10 | 15.8 / 168 | 27.2 / 209 | 33.8 / 223 | 0.73 / 1.5 | 34.7 / 224 | 33.4 / 220 | 33.7 / 223 | 33.9 / 223 | 0.25 / 1.6 | 482, 678, 735 |
| terminal | 5 | 11.5 / 147 | 14.4 / 175 | 19.3 / 230 | 1.22 / 13.7 | 19.5 / 232 | 14.4 / 174 | 15.0 / 178 | 15.2 / 181 | 0.37 / 3.6 | 432, 602, 616 |


### Space: step worlds (heap MB / renderer RSS MB / DOM nodes / WebKit RSS MB)

| world | checkpoint | heap | renderer RSS | DOM nodes | WebKit RSS |
|---|---|---|---|---|---|
| graph10k | boot | 12.3 [0.0] | 149 [3] | 524 [0] | 425 [1] |
| graph10k | open | 14.8 [0.0] | 172 [2] | 1660 [8] | 503 [6] |
| graph10k | left | 15.2 [0.0] | 169 [2] | 1805 [8] | 502 [4] |
| graph10k | back | 15.5 [0.0] | 174 [2] | 1854 [8] | 504 [4] |
| graph50k | boot | 19.0 [0.0] | 151 [2] | 524 [0] | 445 [1] |
| graph50k | open | 16.1 [0.1] | 188 [2] | 1660 [8] | 576 [872] |
| graph50k | left | 16.4 [0.1] | 182 [1] | 1805 [8] | 569 [34] |
| graph50k | back | 16.7 [0.1] | 187 [2] | 1854 [8] | 571 [33] |
| kept-graphs | boot | 11.1 [0.0] | 149 [4] | 660 [0] | 424 [421] |
| kept-graphs | graph1 | 14.9 [0.0] | 168 [1] | 1745 [0] | 478 [9] |
| kept-graphs | graph2 | 16.7 [0.0] | 176 [1] | 2796 [8] | 493 [5] |
| kept-graphs | graph3 | 18.5 [0.0] | 183 [1] | 3847 [16] | 511 [1975] |
| kept-graphs | graph4 | 20.0 [0.0] | 188 [0] | 4906 [8] | 516 [1498] |
| kept-graphs | graph5 | 21.4 [0.0] | 191 [1] | 5338 [16] | 516 [876] |
| local-file | boot | 20.2 [0.1] | 184 [1] | 635 [0] | 541 [8] |
| local-file | mode:automations | 20.5 [0.1] | 179 [1] | 698 [0] | 556 [5] |
| local-file | back:git | 21.3 [0.0] | 180 [0] | 804 [0] | 558 [4] |
| local-file | closed | 21.7 [0.0] | 182 [1] | 885 [0] | 564 [3] |
| ade20 | boot | 23.4 [0.0] | 183 [7] | 4662 [0] | 504 [2] |
| ade20 | board | 23.4 [0.0] | 175 [1] | 4662 [0] | 503 [5] |
| ade20 | task | 26.3 [0.0] | 190 [1] | 5790 [0] | 532 [12] |
| ade20 | left | 24.9 [0.0] | 171 [1] | 5984 [0] | 535 [9] |
| ade20 | back | 37.8 [0.0] | 191 [0] | 11382 [0] | 563 [6] |
| ade200 | boot | 23.1 [0.0] | 174 [4] | 3642 [0] | 513 [3] |
| ade200 | board | 23.2 [0.0] | 173 [1] | 3642 [0] | 513 [3] |
| ade200 | task | 26.2 [0.0] | 188 [3] | 4770 [0] | 541 [0] |
| ade200 | left | 25.1 [0.0] | 171 [0] | 4979 [0] | 546 [569] |
| ade200 | back | 35.7 [0.0] | 187 [1] | 9357 [0] | 562 [420] |
| memory50 | boot | 10.5 [0.0] | 148 [1] | 497 [0] | 419 [516] |
| memory50 | list | 12.3 [0.2] | 157 [1] | 1580 [0] | 434 [556] |
| memory50 | detail | 12.6 [0.2] | 158 [2] | 1667 [0] | 437 [592] |
| memory50 | left | 11.9 [0.2] | 149 [1] | 758 [0] | 437 [617] |
| memory500 | boot | 10.5 [0.0] | 147 [3] | 497 [0] | 421 [602] |
| memory500 | list | 19.8 [0.0] | 173 [1] | 9680 [0] | 470 [697] |
| memory500 | detail | 20.5 [0.0] | 174 [2] | 9767 [0] | 486 [759] |
| memory500 | left | 12.6 [0.0] | 152 [1] | 758 [0] | 479 [800] |

Reading the tables:

- The first Monaco-using tab (grid, console, document, repo-file, diff) costs a one-time chunk load of about 10 MB heap and 30 MB RSS that stays after `closed` (Studio `grid`: `mode` 14.8 to `closed` 25.2 MB). Later tabs cost 0.2 to 0.5 MB heap and under 1 MB RSS each. Not a leak.
- Live Monaco editors, SlickGrids and xterms never exceed 1 at `openN` (Space `diff`: 3 editors plus 1 diff editor); views remount on tab switch and models survive per P433.
- `closed` DOM nodes fall to within 0 to 420 of `mode` in every Studio tab world and within 312 in Space `repo-file`; Space `diff` stays 1007 nodes above `mode` (rank 5).

## 5. Attribution (Pass B)

Top constructors by retained size, unminified. `(compiled code)`, `(string)` and `ExternalStringData` appear in every row (the loaded bundle) and are omitted unless they moved.

- Studio `docker list` (400 containers; 92.8 MB snapshot vs 36.4 MB after `logs50k`): Vue component instances `{uid, vnode, type, parent, ...}` 16,266 objects, 26.6 MB retained; VNodes `{__v_isVNode, ...}` 31,756 objects, 9.7 MB; `(array)` 10.6 MB; `Array` 91,071 objects, 10.6 MB. Freed on leaving the list.
- Studio `terminal openN` (10 terminals; 83.1 MB vs 31.1 MB at `closed`): xterm buffers (`$i`, `zi`, 10 instances, 51.6 MB retained) over 25,305 BufferLine objects `s`; `Uint32Array` 49.6 MB (`ArrayBuffer` 48.2 MB). All freed at `closed`. Scrollback is 5000 lines (`packages/workbench/src/terminal/terminalRenderer.ts:151`).
- Space `terminal openN`: same structure, `Uint32Array` 49.6 MB. Freed at `closed`.
- Studio `documents-list open5000` vs `open100`: heap +2.3 MB, DOM flat at 2.6k nodes; the list is already windowed.
- Studio `kubernetes logs50k`: 20,000 `{text, id, stream, plain}` objects, 2.2 MB retained; the log buffer caps at 20,000 lines. Docker is the same. Both free on leaving the mode.
- Studio `console`, `grid`, `document`, `keyvalue`: `openN` constructor mix is Vue component instances plus lazily loaded Monaco and grid code; no data constructor over 3 MB.
- Space `repo-file openN` (10 tabs): `TextModel` 10 instances, 21.3 MB retained (2.1 MB each); none left at `closed`. Heap at `closed` stays 16.7 MB over `mode`, all lazily loaded Monaco code.
- Space `kept-graphs graph5`: Vue component instances 4.6 MB (2,030 objects) for 5 graph views; 4 SlickGrids live (the kept cap).
- Space `graph50k` vs `graph10k` open: snapshot 51.2 vs 34.6 MB; `ArrayBuffer` 11.6 vs 2.9 MB and `Array` 13.5 vs 3.3 MB, so 40k more rows cost 16.6 MB (4.2 MB per 10k rows, packed chunks).
- Space `memory500 list` vs `memory50`: Vue component instances 2,432 objects (3.9 MB), `Array` 21,342 (2.4 MB), `(array)` 28.0k (3.7 MB). Leaving memory frees them (`left` snapshot 24.3 MB vs `list` 36.5 MB).
- Space `ade20 task`: query `state.data` retained: `adetask changeRequests` 4.6 MB, `board` 3.8 MB, `repos` 3.3 MB, `workflows` 2.5 MB, `sessions` 0.6 MB (shared fixture strings may overlap). At 200 tasks `board` is 4.5 MB, so board data is mostly fixed cost.
- Named roots elsewhere: every Pinia store root retains under 0.1 KB; the largest other query `state.data` is 392.8 KB (docker containers, kept after leaving docker), then 258.5 KB (`memory search` at 500 after leaving memory); `i18n:messages` 0.5 MB.
- Leak retainer paths (Pass B drill):
  - Space `diff`, `StandaloneDiffEditor2` x10 retained at `closed`: editor, held by the scope of `attachDiffNavigation`, held by a `ComputedRefImpl` closure, held through a `Dep`/`Link` chain to the `canNext` computed of the view's setup scope, held by the `queryFn` closure of a TanStack `Query` (gc timer in the fake clock). The `useReviewWorkingDiff` `queryFn` in `RepoDiffView.vue` keeps the view's setup scope alive, and that scope's `canNext` computed still links the disposed navigation.
  - Space `ade20`, `left` to `back`: 3,626 Vue component instances, 23.6k `Link`, 21.0k `Dep` added, DOM counters 5,984 to 11,382 nodes while live DOM is 119 (left) and 1,037 (back). Nearest retainer of the old tree: `<input data-slot="input">` `onKeydown` invoker, held by Blink `DocumentState`, `HistoryItem`, `WebLocalFrameImpl`.
  - Studio `api`; Studio and Space `terminal`, closed2 to closed3: only `(compiled code)` and Blink timing entries grow. No app retainer exists.

## 6. Candidates, ranked by expected saving

Proven means the section 2 threshold holds. RSS is Chromium renderer RSS.

| Rank | Suspect | Scenario | Measured cost | Evidence | Fix | Expected saving | Proven | Behaviour risk |
|---|---|---|---|---|---|---|---|---|
| 1 | Large lists: Studio docker container list | 400 containers, list shown | Heap 11.0 to 64.5 MB (+53.5 [0.2]); renderer RSS 147 to 237 MB (+90 [2]); DOM 26,835 nodes (about 67 per row); 0.13 MB heap and 0.22 MB RSS per container. WebKit RSS 422 to 619 MB. | Pass B: 16,266 component instances (26.6 MB), 31.8k VNodes (9.7 MB). Freed on leaving the list (`logs0` heap 16.3 MB). | Virtualize the container list in `apps/kira-studio/frontend/src/views/docker` (VueUse `useVirtualList` or TanStack Virtual): visible rows plus overscan. | About 50 MB heap, 85 MB RSS at 400 containers; scales per row. | Yes | Row focus, keyboard navigation and in-page find of off-screen rows. Tab state unaffected. |
| 2 | xterm scrollback | 10 Studio terminals (5 in Space), 10,000 lines fed each | Studio: 0.58 MB heap and 6.2 MB RSS per terminal; RSS 174 to 229 MB (+55 [1]); heap +5.3 MB. Space: 1.22 MB heap and 13.7 MB RSS per terminal (5 terminals, RSS 175 to 230). WebKit RSS 430 to 589 MB, not returned at `closed` (593). | Pass B: `Uint32Array` 49.6 MB over 25.3k BufferLines for 10 terminals (about 5 MB each at 5000 lines). Freed at `closed` in Chromium (RSS 170). | Lower `scrollback` in `terminalRenderer.ts:151` from 5000 (1000 is the VS Code default). | About 4 MB of array bytes per full terminal; about 40 MB RSS at 10 full terminals. | Yes | Product call: shorter scrollback depth. |
| 3 | Large lists: Space memory list | 500 memories, list shown | Heap 10.5 to 19.8 MB (+9.3 [0.0]); RSS 147 to 173 MB (+26 [3]); DOM 9,680 nodes (about 19 per row). At 50 rows: heap +1.8, RSS +9, 1,580 nodes. | Pass B: 2,432 component instances (3.9 MB), `Array`/`(array)` 6.1 MB. Freed on leaving (`left` heap 12.6 MB). | Virtualize the memory list in `apps/kira-space/frontend/src/memory`. | About 8.5 MB heap, 22 MB RSS at 500 rows. | Yes | Same list-focus and find caveats. |
| 4 | Kept-alive or hidden panes: Space ADE mount retained after leaving | `ade` mode, 20 or 200 tasks, leave to git and return | `left` to `back`: heap +12.9 MB (ade20 24.9 to 37.8; ade200 25.1 to 35.7, +10.6); RSS +20 and +16 MB; DOM counters 5,984 to 11,382. Bounded: 3 more visits stay flat (heap 36.9 to 39.6 MB, 11,566 nodes). | 3,626 component instances plus 23.6k `Link` retained while live DOM is 119 nodes. Retainer reaches Blink `DocumentState` through an `<input data-slot="input">` `onKeydown` closure. | Find the ADE input whose handler closure outlives unmount; release the reference on unmount. | About 13 MB heap, 16 to 20 MB RSS, once per session. | No: attribution stops at native `DocumentState`; the owning ADE component is not identified. | Mode switch must keep state P433 requires. |
| 5 | Kept-alive or hidden panes: Space diff editors held until query gc | 10 diff tabs opened then closed | Retained at `closed`: 96 detached `div` (3.5 MB) plus 556 detached `Text` (1.6 MB); DOM nodes 2,188 to 3,195. No growth across cycles (0.23 to 0.26 MB drift, JIT). | Retainer chain in section 5 through `RepoDiffView.vue`. Frees when the 5 min `gcTime` timer fires; not measured (no fast-forward in Space worlds). | In `RepoDiffView.vue`, stop the `canNext` and `canPrevious` computeds on unmount, or hoist the `useReviewWorkingDiff` `queryFn` out of setup scope. | About 5 MB heap per 10 closed diff tabs, held up to 5 min. | Yes (3.5 MB of detached `div` alone clears 2 MB at 35x the 0.1 MB spread); lowest of the proven because gc bounds it. | None for tab switching; only closed tabs. |
| 6 | Kept-alive or hidden panes: Space kept graphs | 5 repos opened, cap of 4 kept | Heap 14.9 (1 graph) to 21.4 MB (5): +1.63 MB per graph; RSS 168 to 191 MB (+5.8 each). Cap holds at 4 SlickGrids. | Pass B: component instances 4.6 MB. | Lower the kept-graph cap. | At most 1.6 MB heap, 6 MB RSS per graph dropped. | No: below 2 MB heap per graph, and eviction loses state. | Violates P433. |
| 7 | Git graph store | `graph10k` vs `graph50k` | RSS 172 vs 188 MB at open; Pass B snapshot +16.6 MB for 40k rows (4.2 MB per 10k rows). Leaving the tab changes heap by +0.3 to 0.4 MB (pinned tab, no release). | `ArrayBuffer` 11.6 vs 2.9 MB, `Array` 13.5 vs 3.3 MB. | None: chunks already packed; P479 changes the data plane. | Not applicable. | No | None. |
| 8 | Discover open item (Studio) | 8 mode round trips over a Discover tab | Heap 31.2 (open1) to 37.6 MB (round8); RSS 222 to 237 MB; DOM flat at 3,422. | Section 8. | None. | None. | No | None. |
| 9 | Monaco instances and models | Space repo-file 10 tabs; Studio console, document | Studio: 0.2 to 0.5 MB heap and under 1 MB RSS per extra tab. Space `repo-file`: 2.73 MB heap and 13.4 MB RSS per tab; 10 `TextModel` retained 21.3 MB, freed at `closed`. After close RSS stays 327 [85] vs 168 MB at `mode` while heap is 32.5 MB (allocator, not heap). | `TextModel` absent in the `closed` snapshot. | None provable: the models are the open files, kept per P433. | Not applicable. | No | Dropping models loses editor state. |
| 10 | Docker and Kubernetes logs | 50,000 lines fed | Docker heap +2.7 [0.2] MB (16.3 to 19.0) vs 0 lines; Kubernetes +2.9 MB. Capped at 20,000 lines (2.2 MB retained). Freed on leaving. | 20,000 `{text, id, stream, plain}` objects. | Lower the 20,000-line cap. | About 1.5 MB at a 10,000-line cap. | No: below 2 MB and a behaviour change. | Shorter log history. |
| 11 | i18n catalog | `en.json`, 460 KB | Blank page, `JSON.parse` held: heap 0.79 to 1.62 MB (+0.83, single run). Named root `i18n:messages` retains 0.5 MB. | Pass B. | Per-module lazy catalog chunks. | At most 0.8 MB. | No: below 2 MB. | None. |

Ranks 1 to 5 are ordered by expected saving; 6 to 11 are the not-proven items ordered by measured cost.

Proven: 4 candidates (ranks 1, 2, 3, 5). Rank 4 is the largest unproven cost and the first item for a P481 investigation, not an implementation.

## 7. No real cost

- Images: 0 `img` elements and 0 decoded bytes in every world at `openN` (both apps, both passes).
- TanStack Query `gcTime`: a 6 min fast-forward at `closed` frees at most 0.1 MB heap in every Studio tab world (`grid` 25.2 to 25.3, `console` 28.7 to 28.6, `document` 23.6 to 23.6, `api` 26.1 to 26.1, `stream` 16.1 to 16.1). Inactive query `state.data` at `closed` is at most 0.4 MB each (largest: docker containers 392.8 KB).
- Pinia stores with large data: no store root retains over 0.1 KB in any snapshot. `documents-list` at 5,000 documents adds 2.3 MB heap in total, held by the page query. Named roots are lower bounds, but no Pass A delta is left unexplained by code, DOM or lists.
- SlickGrid grids and row caches: `slickGrids` is 1 after `round` in the grid world and 0 at `closed`; per-tab slope 0.21 MB heap and 0.6 MB RSS.
- Idle timers: `boot60` equals `boot` within 0.1 MB heap in both apps; no growth over 60 s.
- Kept-alive modes in Studio: `back:<mode>` adds at most 0.2 MB heap and at most 70 DOM nodes over `mode:<mode>`.
- Space `local-file` tab: heap 20.2 to 21.7 MB over a mode round trip and close.
- Space memory module at 50 rows: +1.8 MB heap, below threshold; memory queries retain at most 258.5 KB after leaving.
- Space ADE board data: `board` query is 3.8 MB at 20 tasks and 4.5 MB at 200, so row count is not the cost; the cost is the first mount (rank 4).

## 8. Discover open item

Snapshot diff `round3` to `round5` (Studio `discover`, unminified, one run): the top growing constructor is `(compiled code)`, +6,888 objects and +1,098 KB. Everything else is under 35 KB: `system / WeakArrayList` +34 KB, `PerformanceEventTiming` +8.5 KB, `(object shape)` +7 KB, `(array)` +4.8 KB, `Object` +2.5 KB. No Vue, store or query constructor grows. Retainer path of the top grower: compiled code held by `WeakFixedArray` and `FeedbackVector`, i.e. V8 JIT feedback and compiled code. Pass A heap rises 31.2 to 37.6 MB over 8 rounds (spread up to 0.7 MB) with DOM flat at 3,422 nodes. The growth is JIT warm-up, not retained app data. Not proven; no fix.
