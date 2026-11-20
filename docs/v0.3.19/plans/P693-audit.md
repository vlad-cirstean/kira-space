# P693 audit: agents module RAM

Phase P693 Part 1. Measure and audit only. No product source changed. Fixes are Part 2.

## 1. Header

- Date: 2026-11-20.
- Probe commit (base of Pass A, Go and WebKit runs): `1276c7691` `test(perf): agents RAM probe checkpoints and Go probe`.
- Product tip under it: `12f90fcec` (docs plan commit), product source = `bf5b79054`. Probe commit changes no product behavior (hooks gated by `__KIRA_DEBUG_HOOKS__`).
- Pass B base: `0107b563d` `test(perf): ade-idle heap snapshots for pass B` (adds snapshot `keep` on `ade-idle` hop steps; test-only). Pass A ran on `1276c7691`; this second commit changes no measured code path.
- Host: Linux 6.18.44-fc-v726, 4 vCPU (`nproc` 4), 16087 MB RAM, no swap. `go version go1.27.3 linux/amd64`. Playwright 1.64.1, `chromium-1243` (headless shell 1243/1248), `webkit-2370` (WPE). Another agent shared the host.
- Gate: every run waited for `load1 <= 1.0`; probe gate (`waitForQuietHost`) on; `KIRA_RAM_NOGATE` never set.

## 2. Figure of record

Definition (plan D1): Kira-owned PSS in the agents fixture = (a) WebKit WebContent PSS, `ade-agents`, checkpoint `final`; (b) Go app memory at `idle60`; (c) Kira helper processes (shells, git, `js-worker`) at `idle60`. `claude` excluded; own line.

Deviation on (b): app PSS is distorted. The 4 fake `claude` children are the re-exec'd test binary and share its text pages with the app, so app PSS reads 95.8 at `idle60` and 193 to 204 once they exit. Used app anonymous memory (`anonMb`, smaps_rollup) instead, minus 11.5 MB of harness-retained base64 event strings (see section 8). Raw PSS stays in the tables.

Limits (verbatim):
- PSS splits shared pages across processes; macOS `phys_footprint` counts private dirty plus compressed and excludes clean shared.
- V8 heap sizes are not JSC heap sizes.
- WPE has no GPU process and no IOSurface memory; macOS WebContent does.
- (a) and (b)+(c) come from two harnesses with the same fixture shape, not one live process set.
- The Go probe app is the test binary; harness code is in its baseline, cancelled by checkpoint deltas.

User figure (Mac "Agents module footprint run", `docs/DEV_ENVIRONMENT.md`): not supplied. Mapping when supplied: App to (b)+(c), WebContent to (a), Agents to `claude` plus shells; state proxy-to-Mac ratio per component.

## 3. Commands

All from the worktree root `/home/user/kira-space-v0.3.19`. Probe extensions: commit `1276c7691`. Wait for `cat /proc/loadavg` load1 <= 1.0 and no other `playwright|go test|vite|vitest` process before each.

Pass A Chromium (minified):
```
bun run build:test
KIRA_RAM_WORLD=ade-agents,ade-idle,ade bunx playwright test \
  --config=apps/kira-space/playwright.perf.config.ts --project=heap heap-ram --repeat-each=3 | tee passA-chromium.log
```
Pass A WebKit:
```
KIRA_RAM_WORLD=ade-agents,ade-idle,ade bunx playwright test \
  --config=apps/kira-space/playwright.perf.config.ts --project=heap-webkit heap-ram --repeat-each=3 | tee passA-webkit.log
```
Pass B (unminified, one repeat, snapshots kept):
```
cd apps/kira-space/frontend && KIRA_DEBUG_HOOKS=1 bunx vite build --minify false && cd -
KIRA_RAM_SNAPSHOTS=1 KIRA_RAM_KEEP_SNAPSHOTS=1 KIRA_RAM_WORLD=ade-agents,ade-idle,ade \
  bunx playwright test --config=apps/kira-space/playwright.perf.config.ts --project=heap heap-ram | tee passB-chromium.log
bun run build:test
```
Go (three separate processes):
```
for i in 1 2 3; do
  KIRA_RAM_PROBE=1 KIRA_RAM_OUT=apps/kira-space/perf-ram/go/r$i CGO_ENABLED=1 GOTMPDIR=/tmp/gotmp \
    go test ./apps/kira-space/internal/flows/adeflow -run '^TestRAMProbeAgents$' -count=1 -v -timeout 15m
done
go tool pprof -sample_index=inuse_space -top -nodecount=40 -base <out>/heap-boot.pb.gz <out>/heap-idle60.pb.gz
```
Retainer walk (section 8): `heapSnapshot` toolkit over `perf-ram/heap/space/ade20/{left,back}.heapsnapshot`, shortest path to the `AdeShell` component instance with DevTools console handles excluded.

## 4. Runs

Median [spread], spread = max - min over kept repeats. Noise floors: JS heap 2 MB, renderer or process PSS 10 MB, Go inuse 2 MB, goroutines 10; noise = max(spread, floor).

| world / engine | kept | discarded (RAMLOAD load1) |
|---|---|---|
| ade20, ade200 / chromium | 3 each | none |
| ade-agents, ade-idle / chromium | 3 each | none |
| ade20, ade200 / webkit | 3 each | none (load1 0.44 0.90 0.38 and 0.60 0.53 0.51) |
| ade-agents / webkit | 3 (reps 2, 3, 4) | rep 1 (load1 1.21); rerun gated, load1 0.13 |
| ade-idle / webkit | 3 (reps 2, 4, 5) | rep 1 (1.14), rep 3 (1.30); reruns gated, load1 0.51 and 0.21 |
| Go probe | 3 processes | none |
| Pass B chromium | 1 (attribution) | none |

Kept WebKit `ade-agents` load1: 0.83, 0.79, 0.13. WebKit `ade-agents` spreads (18 to 60 MB at later checkpoints) exceed the 10 MB floor: WebKit deltas under about 25 MB there are noise; Chromium carries the attribution.

## 5. Raw numbers

### 5.1 Chromium (Pass A). Heap after two GCs, renderer RSS and PSS, DOM, suspects, idle queries / queries with no observers, terminal buffer lines, drain bytes. Query bytes = JSON length per family, all-zero families omitted. No Pinia store exceeded 20 KB at the last checkpoint in any world.

#### ade20 / chromium (3 repeats)

| cp | heapMb | rendRSS | rendPSS | nodes | listeners | xterm | monaco | queries (idle) | termBufLines | drainB |
|---|---|---|---|---|---|---|---|---|---|---|
| boot | 34.0 [0.0] | 193.2 [1.3] | - | 7323.0 [0.0] | 1377.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 34.0 [0.0]/6.0 [0.0] | - | - |
| board | 34.1 [0.0] | 193.2 [1.1] | 147.3 [0.9] | 7323.0 [0.0] | 1377.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 34.0 [0.0]/6.0 [0.0] | - | - |
| task | 37.2 [0.0] | 206.5 [0.4] | 156.8 [0.3] | 8661.0 [0.0] | 1593.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 36.0 [0.0]/6.0 [0.0] | - | - |
| left | 34.3 [0.0] | 188.2 [0.5] | 144.3 [0.5] | 8782.0 [0.0] | 1537.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 36.0 [0.0]/22.0 [0.0] | - | - |
| back | 54.1 [0.0] | 215.9 [1.3] | 170.2 [0.6] | 16752.0 [0.0] | 2964.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 36.0 [0.0]/6.0 [0.0] | - | - |
| left2 | 51.1 [0.0] | 207.5 [0.2] | 163.5 [0.5] | 16863.0 [0.0] | 2907.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 36.0 [0.0]/22.0 [0.0] | - | - |
| back2 | 54.5 [0.0] | 218.1 [0.5] | 172.3 [0.8] | 16752.0 [0.0] | 2964.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 36.0 [0.0]/6.0 [0.0] | - | - |
| left3 | 51.5 [0.0] | 209.4 [0.6] | 165.2 [0.8] | 16863.0 [0.0] | 2907.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 36.0 [0.0]/22.0 [0.0] | - | - |
| back3 | 54.7 [0.0] | 218.0 [0.7] | 172.6 [0.5] | 16752.0 [0.0] | 2964.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 36.0 [0.0]/6.0 [0.0] | - | - |

Query bytes (KB, JSON length) by family, median over repeats:

| family | boot | board | task | left | back | left2 | back2 | left3 | back3 |
|---|---|---|---|---|---|---|---|---|---|
| adetask/board | 56.6 [0.0] | 56.6 [0.0] | 56.6 [0.0] | 56.6 [0.0] | 56.6 [0.0] | 56.6 [0.0] | 56.6 [0.0] | 56.6 [0.0] | 56.6 [0.0] |
| adetask/board#history | 0.7 [0.0] | 0.7 [0.0] | 0.7 [0.0] | 0.7 [0.0] | 0.7 [0.0] | 0.7 [0.0] | 0.7 [0.0] | 0.7 [0.0] | 0.7 [0.0] |
| adetask/changeRequests | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] |
| adetask/sessions | 8.0 [0.0] | 8.0 [0.0] | 8.0 [0.0] | 8.0 [0.0] | 8.0 [0.0] | 8.0 [0.0] | 8.0 [0.0] | 8.0 [0.0] | 8.0 [0.0] |
| adetask/workflows | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] |
| adetask/repos | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] |
| adetask/hostProviders | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] |
| adetask/panelForward | 0.0 [0.0] | 0.0 [0.0] | 0.1 [0.0] | 0.1 [0.0] | 0.1 [0.0] | 0.1 [0.0] | 0.1 [0.0] | 0.1 [0.0] | 0.1 [0.0] |


#### ade200 / chromium (3 repeats)

| cp | heapMb | rendRSS | rendPSS | nodes | listeners | xterm | monaco | queries (idle) | termBufLines | drainB |
|---|---|---|---|---|---|---|---|---|---|---|
| boot | 30.6 [0.0] | 190.6 [1.9] | - | 5744.0 [0.0] | 1130.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 34.0 [0.0]/6.0 [0.0] | - | - |
| board | 30.6 [0.0] | 190.5 [1.9] | 144.1 [2.0] | 5744.0 [0.0] | 1130.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 34.0 [0.0]/6.0 [0.0] | - | - |
| task | 33.8 [0.0] | 203.4 [0.4] | 153.1 [0.8] | 7082.0 [0.0] | 1358.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 36.0 [0.0]/6.0 [0.0] | - | - |
| left | 31.5 [0.0] | 187.1 [0.7] | 142.7 [1.0] | 7213.0 [0.0] | 1316.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 36.0 [0.0]/22.0 [0.0] | - | - |
| back | 47.2 [0.0] | 208.8 [0.9] | 163.0 [1.1] | 13604.0 [0.0] | 2508.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 36.0 [0.0]/6.0 [0.0] | - | - |
| left2 | 45.0 [0.0] | 201.9 [0.8] | 157.6 [0.9] | 13725.0 [0.0] | 2465.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 36.0 [0.0]/22.0 [0.0] | - | - |
| back2 | 47.8 [0.0] | 210.9 [1.1] | 164.7 [1.7] | 13604.0 [0.0] | 2508.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 36.0 [0.0]/6.0 [0.0] | - | - |
| left3 | 45.5 [0.0] | 203.4 [1.4] | 158.6 [1.9] | 13725.0 [0.0] | 2465.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 36.0 [0.0]/22.0 [0.0] | - | - |
| back3 | 48.1 [0.0] | 211.0 [1.1] | 165.1 [1.2] | 13604.0 [0.0] | 2508.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 36.0 [0.0]/6.0 [0.0] | - | - |

Query bytes (KB, JSON length) by family, median over repeats:

| family | boot | board | task | left | back | left2 | back2 | left3 | back3 |
|---|---|---|---|---|---|---|---|---|---|
| adetask/board | 347.0 [0.0] | 347.0 [0.0] | 347.0 [0.0] | 347.0 [0.0] | 347.0 [0.0] | 347.0 [0.0] | 347.0 [0.0] | 347.0 [0.0] | 347.0 [0.0] |
| adetask/board#history | 0.7 [0.0] | 0.7 [0.0] | 0.7 [0.0] | 0.7 [0.0] | 0.7 [0.0] | 0.7 [0.0] | 0.7 [0.0] | 0.7 [0.0] | 0.7 [0.0] |
| adetask/changeRequests | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] |
| adetask/sessions | 8.0 [0.0] | 8.0 [0.0] | 8.0 [0.0] | 8.0 [0.0] | 8.0 [0.0] | 8.0 [0.0] | 8.0 [0.0] | 8.0 [0.0] | 8.0 [0.0] |
| adetask/workflows | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] |
| adetask/repos | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] |
| adetask/hostProviders | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] |
| adetask/panelForward | 0.0 [0.0] | 0.0 [0.0] | 0.1 [0.0] | 0.1 [0.0] | 0.1 [0.0] | 0.1 [0.0] | 0.1 [0.0] | 0.1 [0.0] | 0.1 [0.0] |


#### ade-agents / chromium (3 repeats)

| cp | heapMb | rendRSS | rendPSS | nodes | listeners | xterm | monaco | queries (idle) | termBufLines | drainB |
|---|---|---|---|---|---|---|---|---|---|---|
| boot | 36.2 [0.0] | 195.7 [1.8] | - | 8201.0 [0.0] | 1537.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 34.0 [0.0]/6.0 [0.0] | - | - |
| board | 36.2 [0.0] | 195.7 [1.6] | 149.8 [1.3] | 8201.0 [0.0] | 1537.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 34.0 [0.0]/6.0 [0.0] | - | - |
| launch4 | 40.2 [0.0] | 216.8 [0.8] | 170.2 [1.0] | 9288.0 [0.0] | 1776.0 [0.0] | 1.0 [0.0] | 0.0 [0.0] | 44.0 [0.0]/12.0 [0.0] | 4212.0 [0.0] | 0.0 [0.0] |
| panes2 | 40.4 [0.0] | 223.0 [0.8] | 173.1 [0.9] | 9292.0 [0.0] | 1776.0 [0.0] | 1.0 [0.0] | 0.0 [0.0] | 44.0 [0.0]/12.0 [0.0] | 4212.0 [0.0] | 0.0 [0.0] |
| switch10 | 46.0 [0.0] | 238.3 [1.9] | 186.1 [1.4] | 12224.0 [0.0] | 2089.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 56.0 [0.0]/26.0 [0.0] | 4212.0 [0.0] | 0.0 [0.0] |
| push10 | 46.1 [0.0] | 232.7 [1.8] | 184.9 [1.2] | 12224.0 [0.0] | 2089.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 56.0 [0.0]/26.0 [0.0] | 4212.0 [0.0] | 0.0 [0.0] |
| push20 | 46.1 [0.0] | 235.5 [1.8] | 187.7 [1.3] | 12224.0 [0.0] | 2089.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 56.0 [0.0]/26.0 [0.0] | 4212.0 [0.0] | 0.0 [0.0] |
| push30 | 46.1 [0.0] | 237.9 [1.7] | 190.2 [1.3] | 12224.0 [0.0] | 2089.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 56.0 [0.0]/26.0 [0.0] | 4212.0 [0.0] | 0.0 [0.0] |
| push40 | 46.1 [0.0] | 240.6 [1.6] | 192.8 [1.1] | 12224.0 [0.0] | 2089.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 56.0 [0.0]/26.0 [0.0] | 4212.0 [0.0] | 0.0 [0.0] |
| push50 | 46.1 [0.0] | 243.5 [2.6] | 195.7 [2.3] | 12224.0 [0.0] | 2089.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 56.0 [0.0]/26.0 [0.0] | 4212.0 [0.0] | 0.0 [0.0] |
| final | 46.2 [0.0] | 236.1 [1.4] | 188.1 [0.9] | 12224.0 [0.0] | 2089.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 56.0 [0.0]/26.0 [0.0] | 4212.0 [0.0] | 0.0 [0.0] |
| approval | 47.0 [0.0] | 241.6 [1.3] | 191.3 [1.0] | 12408.0 [0.0] | 2113.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 56.0 [0.0]/26.0 [0.0] | 4212.0 [0.0] | 0.0 [0.0] |
| log3m | 52.4 [0.0] | 249.1 [1.4] | 200.0 [1.0] | 13149.0 [0.0] | 2177.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 57.0 [0.0]/26.0 [0.0] | 4212.0 [0.0] | 0.0 [0.0] |
| files1 | 63.0 [0.0] | 274.5 [1.4] | 227.5 [1.6] | 13734.0 [0.0] | 2490.0 [0.0] | 0.0 [0.0] | 2.0 [0.0] | 60.0 [0.0]/27.0 [0.0] | 4212.0 [0.0] | 0.0 [0.0] |
| stopped4 | 63.0 [0.0] | 274.6 [1.2] | 227.3 [0.7] | 13694.0 [0.0] | 2484.0 [0.0] | 0.0 [0.0] | 2.0 [0.0] | 60.0 [0.0]/27.0 [0.0] | 4212.0 [0.0] | 0.0 [0.0] |
| left | 59.6 [0.0] | 260.7 [1.0] | 215.6 [1.2] | 13696.0 [0.0] | 2225.0 [0.0] | 0.0 [0.0] | 1.0 [0.0] | 59.0 [0.0]/45.0 [0.0] | 4212.0 [0.0] | 0.0 [0.0] |
| back | 76.0 [0.0] | 291.5 [1.0] | 244.5 [0.9] | 18290.0 [0.0] | 3618.0 [0.0] | 0.0 [0.0] | 2.0 [0.0] | 60.0 [0.0]/27.0 [0.0] | 4212.0 [0.0] | 0.0 [0.0] |

Query bytes (KB, JSON length) by family, median over repeats:

| family | boot | board | launch4 | panes2 | switch10 | push10 | push20 | push30 | push40 | push50 | final | approval | log3m | files1 | stopped4 | left | back |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| adetask/board | 73.3 [0.0] | 73.3 [0.0] | 73.3 [0.0] | 73.3 [0.0] | 73.3 [0.0] | 73.3 [0.0] | 73.3 [0.0] | 73.3 [0.0] | 73.3 [0.0] | 73.3 [0.0] | 73.3 [0.0] | 73.3 [0.0] | 73.3 [0.0] | 73.3 [0.0] | 73.3 [0.0] | 73.3 [0.0] | 73.3 [0.0] |
| adetask/board#history | 35.0 [0.0] | 35.0 [0.0] | 35.0 [0.0] | 35.0 [0.0] | 35.0 [0.0] | 35.0 [0.0] | 35.0 [0.0] | 35.0 [0.0] | 35.0 [0.0] | 35.0 [0.0] | 35.0 [0.0] | 35.0 [0.0] | 35.0 [0.0] | 35.0 [0.0] | 35.0 [0.0] | 35.0 [0.0] | 35.0 [0.0] |
| adetask/changeRequests | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] |
| adetask/sessions | 115.6 [0.0] | 115.6 [0.0] | 115.6 [0.0] | 115.6 [0.0] | 115.6 [0.0] | 115.6 [0.0] | 115.6 [0.0] | 115.6 [0.0] | 115.6 [0.0] | 115.6 [0.0] | 115.6 [0.0] | 115.6 [0.0] | 115.6 [0.0] | 115.6 [0.0] | 115.6 [0.0] | 115.6 [0.0] | 115.6 [0.0] |
| adetask/workflows | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] |
| adetask/repos | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] |
| adetask/hostProviders | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] |
| adetask/panelForward | 0.0 [0.0] | 0.0 [0.0] | 0.4 [0.0] | 0.4 [0.0] | 0.9 [0.0] | 0.9 [0.0] | 0.9 [0.0] | 0.9 [0.0] | 0.9 [0.0] | 0.9 [0.0] | 0.9 [0.0] | 0.9 [0.0] | 0.9 [0.0] | 0.9 [0.0] | 0.9 [0.0] | 0.9 [0.0] | 0.9 [0.0] |
| adetask/log | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 2070.4 [0.0] | 2070.4 [0.0] | 2070.4 [0.0] | 2070.4 [0.0] | 2070.4 [0.0] |
| adetask/files | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 0.4 [0.0] | 0.4 [0.0] | 0.2 [0.0] | 0.4 [0.0] |


#### ade-idle / chromium (3 repeats)

| cp | heapMb | rendRSS | rendPSS | nodes | listeners | xterm | monaco | queries (idle) | termBufLines | drainB |
|---|---|---|---|---|---|---|---|---|---|---|
| boot | 35.4 [0.0] | 194.8 [0.9] | - | 7951.0 [0.0] | 1471.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 34.0 [0.0]/6.0 [0.0] | - | - |
| board | 35.4 [0.0] | 194.8 [1.0] | 148.9 [1.0] | 7951.0 [0.0] | 1471.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 34.0 [0.0]/6.0 [0.0] | - | - |
| idle60 | 35.5 [0.0] | 189.1 [0.8] | 142.8 [1.1] | 7951.0 [0.0] | 1471.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 34.0 [0.0]/6.0 [0.0] | - | - |
| left | 32.9 [0.0] | 179.1 [0.9] | 134.7 [1.1] | 8096.0 [0.0] | 1419.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 34.0 [0.0]/20.0 [0.0] | - | - |
| back | 51.6 [0.0] | 207.6 [0.5] | 160.9 [0.8] | 15403.0 [0.0] | 2730.0 [0.0] | 0.0 [0.0] | 0.0 [0.0] | 34.0 [0.0]/6.0 [0.0] | - | - |

Query bytes (KB, JSON length) by family, median over repeats:

| family | boot | board | idle60 | left | back |
|---|---|---|---|---|---|
| adetask/board | 73.3 [0.0] | 73.3 [0.0] | 73.3 [0.0] | 73.3 [0.0] | 73.3 [0.0] |
| adetask/board#history | 35.0 [0.0] | 35.0 [0.0] | 35.0 [0.0] | 35.0 [0.0] | 35.0 [0.0] |
| adetask/changeRequests | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] | 3.4 [0.0] |
| adetask/workflows | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] | 10.6 [0.0] |
| adetask/repos | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] | 2.6 [0.0] |
| adetask/hostProviders | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] | 0.2 [0.0] |

Store bytes (KB) over 20 KB at last checkpoint:

| store | KB @back |
|---|---|
### 5.2 WebKit (Pass A). WebContent and Network PSS, MB; process RSS from `ps`. WebKit `ade-idle` and `ade-agents` rows come from the kept repeats listed in section 4.


#### ade20 / webkit (3 repeats; RAMLOAD load1 [0.44, 0.9, 0.38])

| cp | WebContent PSS MB | Network PSS MB | process RSS MB (ps) |
|---|---|---|---|
| boot | - | - | 554.2 [8.3] |
| board | 425.7 [8.5] | 35.7 [0.0] | 546.3 [8.3] |
| task | 450.2 [9.1] | 35.7 [0.0] | 571.0 [9.0] |
| left | 454.1 [5.7] | 35.7 [0.0] | 574.9 [5.6] |
| back | 503.7 [4.3] | 35.2 [0.5] | 624.0 [4.7] |
| left2 | 496.7 [3.4] | 35.2 [0.1] | 617.0 [3.6] |
| back2 | 546.1 [3.7] | 35.2 [0.1] | 666.2 [3.6] |
| left3 | 548.1 [3.0] | 35.2 [0.1] | 668.2 [3.0] |
| back3 | 547.0 [41.4] | 35.2 [0.1] | 667.3 [41.6] |

#### ade200 / webkit (3 repeats; RAMLOAD load1 [0.6, 0.53, 0.51])

| cp | WebContent PSS MB | Network PSS MB | process RSS MB (ps) |
|---|---|---|---|
| boot | - | - | 550.6 [5.6] |
| board | 421.7 [4.8] | 35.8 [0.2] | 542.4 [4.7] |
| task | 443.8 [2.1] | 35.6 [0.6] | 564.5 [2.7] |
| left | 447.1 [1.0] | 35.1 [0.1] | 567.4 [1.0] |
| back | 480.1 [1.2] | 35.1 [0.1] | 600.4 [1.2] |
| left2 | 471.8 [2.6] | 35.1 [0.0] | 592.1 [2.6] |
| back2 | 511.5 [4.7] | 35.1 [0.1] | 631.9 [4.7] |
| left3 | 513.6 [1.6] | 35.1 [0.1] | 633.8 [1.6] |
| back3 | 513.1 [2.0] | 35.1 [0.1] | 633.4 [1.6] |

#### ade-agents / webkit (3 repeats; RAMLOAD load1 [1.21, 0.83, 0.79, 0.13])

| cp | WebContent PSS MB | Network PSS MB | process RSS MB (ps) |
|---|---|---|---|
| boot | - | - | 564.1 [12.9] |
| board | 435.7 [12.1] | 35.8 [0.1] | 556.3 [12.1] |
| launch4 | 570.2 [14.3] | 35.2 [0.1] | 690.5 [14.5] |
| panes2 | 564.3 [14.1] | 35.2 [0.1] | 684.6 [14.3] |
| switch10 | 590.5 [5.0] | 35.2 [0.1] | 710.7 [5.0] |
| push10 | 564.9 [38.2] | 35.2 [0.1] | 685.1 [38.2] |
| push20 | 552.0 [18.9] | 35.2 [0.1] | 672.3 [18.9] |
| push30 | 559.7 [23.4] | 35.2 [0.1] | 680.0 [23.3] |
| push40 | 568.3 [29.8] | 35.2 [0.1] | 688.6 [29.7] |
| push50 | 555.4 [26.3] | 35.2 [0.1] | 675.8 [26.3] |
| final | 556.0 [24.6] | 35.2 [0.1] | 676.3 [27.3] |
| approval | 574.1 [33.9] | 35.2 [0.1] | 694.4 [33.8] |
| log3m | 647.8 [18.8] | 35.2 [0.1] | 768.0 [18.8] |
| files1 | 727.5 [60.5] | 35.2 [0.4] | 847.9 [60.7] |
| stopped4 | 714.3 [33.4] | 35.2 [0.4] | 834.7 [33.6] |
| left | 731.2 [34.0] | 35.2 [0.4] | 851.6 [34.2] |
| back | 771.3 [2.5] | 35.4 [0.2] | 891.8 [2.4] |

#### ade-idle / webkit (3 repeats; RAMLOAD load1 [1.14, 0.61, 1.3, 0.51, 0.21])

| cp | WebContent PSS MB | Network PSS MB | process RSS MB (ps) |
|---|---|---|---|
| boot | - | - | 556.7 [6.2] |
| board | 427.0 [6.4] | 35.8 [0.2] | 547.7 [6.7] |
| idle60 | 418.5 [0.6] | 35.2 [0.8] | 538.5 [1.2] |
| left | 432.5 [1.1] | 35.2 [0.8] | 552.7 [1.9] |
| back | 478.3 [3.5] | 35.2 [0.8] | 598.5 [4.2] |

### 5.3 Go probe (3 processes, median [spread])

`app anon` = smaps_rollup anonymous of the Go app. `app PSS` is distorted by shared test-binary pages (4 fake `claude` children are the same binary; PSS jumps to 193 to 204 after they exit). `shell` and `git` classes: none seen (fake `claude` is exec'd by `sh`; no git child in the process table, peak included). `js-worker`: not started.

| cp | heapInuse MB | heapAlloc MB | Sys MB | goroutines | app anon MB | app PSS MB (distorted) | 4 fake claude, anon MB total |
|---|---|---|---|---|---|---|---|
| boot | 12.1 [0.3] | 8.3 [0.0] | 38.4 [0.5] | 31 [0] | 34.9 [4.2] | 173.1 [4.2] | - |
| board200 | 13.0 [0.3] | 8.6 [0.0] | 38.9 [0.3] | 36 [0] | 38.6 [1.8] | 177.6 [0.8] | - |
| sessions4 | 21.3 [4.0] | 16.6 [3.9] | 46.9 [4.0] | 53 [0] | 48.7 [6.1] | 85.8 [5.7] | 74.5 [6.3] |
| idle60 | 29.0 [0.5] | 24.4 [0.3] | 67.5 [4.0] | 53 [0] | 66.0 [2.8] | 95.8 [2.7] | 96.8 [8.1] |
| pushes50 | 31.4 [0.5] | 24.8 [0.3] | 67.9 [4.2] | 53 [0] | 69.7 [0.9] | 99.8 [0.5] | 96.8 [8.2] |
| stopped4 | 27.1 [0.3] | 20.4 [0.0] | 67.9 [4.2] | 41 [0] | 63.8 [4.0] | 204.0 [4.0] | - |
| final | 27.1 [0.2] | 20.4 [0.0] | 67.9 [4.2] | 41 [0] | 54.6 [0.3] | 193.1 [1.9] | - |

Go attribution (`inuse_space`, `-base heap-boot` vs `heap-idle60`, all three runs agree): `encoding/base64.EncodeToString` 11.2 / 11.6 / 11.5 MB (harness, section 8 note); `mobileterm.(*Ring).Append` 4.3 / 4.6 / 4.3 MB; everything else under 0.2 MB cum each (`oplog` flush 0.09, `ade.TaskBoard` 0.13, `gitclient` 0.13). No package other than the two above exceeds 1 MB.

Per session (`sessions4` - `board200`) / 4: heapInuse 2.1 MB (ring 1.06 of it), goroutines 4.25, anon 2.5 MB. Retained after stop (`stopped4` - `board200`): heapInuse +14.1 MB of which 11.5 is harness base64 (residual 2.6), goroutines +5 (4 `runStateWriter` plus 1), heapAlloc 24.4 to 20.4 on stop (ring freed, entry removed). Runtime overhead (`Sys` - `heapInuse`) at `idle60`: 38.5 MB; boot 26.3: baseline of the test binary, not product.

Harness artifact: `internal/flowtest/events.go` `Events` records every emit forever, so the base64 strings from `terminal.outputCoalescer.flush` never free. Not product memory; subtracted from (b).

## 6. Record baseline

| part | MB median [spread] |
|---|---|
| (a) WebKit WebContent PSS, `ade-agents` `final` | 556.0 [24.6] |
| (b) Go app anon at `idle60`, raw | 66.0 [2.8] |
| (b) minus harness base64 (11.5) | 54.5 [2.8] |
| (c) shells, git, `js-worker` | 0 (none observed) |
| Total (a)+(b)+(c) | 610.5 (spreads sum 27.4) |

`claude`: not Kira-owned, not measured here. Fake `claude` (4 x test binary) anon 96.8 [8.1] at `idle60` is harness, excluded. User Mac figure: not supplied.

Context for (a): WebKit `ade-idle` `board` 427.0 [6.4], `idle60` 418.5 [0.6]; `ade-agents` `board` 435.7 [12.1]. Chromium for the same checkpoint (not in the record): `ade-agents` `final` PSS 188.1 [0.9], heap 46.2 [0.0]; `ade-idle` `board` PSS 148.9 [1.0], heap 35.4 [0.0]. WebKit WebContent reads about 3x Chromium renderer PSS at baseline; engines do not compare (JSC vs V8, WPE software paint, section 2 limits). Noise on (a): 24.6 MB.

## 7. Ranked attribution

Share = MB / 610.5. MB basis: WebKit (record) where a WebKit delta exists, with Chromium Pass A/B corroboration; Go rows from the Go probe. Rows after `final` are outside the record checkpoint (share n/a). Sorted by MB.

| id | owner | MB | share | fixable | why | source |
|---|---|---|---|---|---|---|
| R1 | App frontend baseline (bundle, Vue, i18n, DOM of ADE board, engine) | 435.7 [12.1] | 71.4% | no | Chromium same point: heap 36.2, PSS 149.8, native 113.6. Pass B `ade-idle` `idle60` (unminified): ExternalStringData 19.3 MB (bundle source), compiled code 8.4, 5461 Vue instances 8.8 retained, queries 2.3 total, SourceTextModule 2.2, i18n under 0.1 | `ade-agents` `board`, webkit A; Chromium A/B |
| R2 | 4 live sessions, renderer (final - board) | 120.3 | 19.7% | unknown, diagnose first | WebKit steps: launch4 +134.5, panes2 -5.9, switch10 +26.2, final -34.5 (spread up to 38, noisy). Chromium +38.3 PSS: launch4 +20.4, panes2 +2.9, switch10 +13.0, rest +2.0; heap only +10.0 (launch4 +4.0, switch10 +5.6); native +28.3. Native, not JS: owner not named by the probe (1 xterm in DOM at launch4, +1087 nodes; switch10 +2932 nodes) | `ade-agents` `board` to `final`, webkit A; Chromium A |
| R3 | Go runtime + test binary baseline | 34.9 [4.2] | 5.7% | no | `boot` anon, harness and runtime | Go `boot` |
| R4 | Go product increment (4 sessions, idle60 anon - boot - harness) | 19.6 | 3.2% | partly | ring 4.3 MB (R8), rest runtime-retained pages, goroutine stacks 0.1 | Go `idle60` |
| R5 | Monaco first file view (files1 - log3m) | 79.7 [60.5] WebKit; Chromium +27.5 | n/a | no | Chromium heap +10.6, native +14.9 (ExternalStringData 20.6 to 34.7 = lazy chunk source, compiled code 11.9 to 17.7). Chunk stays loaded after leave (34.7 at `left`, `back`). Shared with repo files view | `ade-agents` files1, A/B |
| R6 | Retained ADE tree (back - board) | 51.3 [6.4] WebKit `ade-idle`; ade20 +78.0 [8.5], ade200 +58.4 [4.8]; Chromium PSS +12.0, heap +16.2 | n/a | yes | Section 8. Chromium Vue instances 5461 to 10750 (+5.3k), DOM nodes 7951 to 15403 | `ade-idle`, `ade20`, A/B |
| R7 | Run log, 3 min (log3m - approval) | 73.7 WebKit [18.8, 33.9]; Chromium heap +5.4, PSS +8.7 | n/a | no (under noise as a cap) | 2070 KB log JSON; strings +1.6, Array +0.7, Detached div retained +1.5, Vue instances +0.5, nodes +741. `LogLines.vue` already virtualizes. Query stays after panel close (gcTime 30 s) | `ade-agents` log3m, A/B |
| R8 | Go mobile terminal ring | 4.3 [0.3] Go inuse | 0.7% | yes | `mobileterm.Ring.Append`, 1.06 MB per agent terminal, filled with phone access off (`Broker.Output` has no access gate); freed on exit | Go `idle60` |
| R9 | Headless xterm buffers, 4 terminals | 8.2 Chromium (ArrayBuffer retained at `final`, count 4263 vs 4212 buffer lines) | in R2 | yes | 2.05 MB per terminal; `__kiraTerminalStats` 4212 lines constant from `launch4` to `back`, not reaped at `stopped4` (ArrayBuffer 8422 KB at `stopped4`) | `ade-agents` final, stopped4, B |

Separate line: `claude` processes (user CLI): not Kira-owned, not measured here; user Mac figure not supplied.

## 8. ade20 retained tree (D2)

Step sizes, WebKit WebContent PSS (3 repeats): `ade20` `board` 425.7, `left` 454.1, `back` 503.7 [4.3], `back2` 546.1, `left3` 548.1, `back3` 547.0 [41.4]. `back` - `board` = +78.0 (floor 10). `ade200`: 421.7 to 480.1 (+58.4), `back3` 513.1 (+91.4). `ade-idle`: 427.0 to 478.3 (+51.3). Gate D2 (1) met in all three worlds. WebKit grows again on the second return (`back` to `back2` +42.4 on ade20) while Chromium heap is flat (54.1 to 54.5): JSC GC timing or a second retained tree; not separated, F1 re-measure covers it.

Chromium: `ade20` heap `board` 34.1, `back` 54.1 (+20.0), `back2` 54.5; PSS 147.3 to 170.2 (+22.9). `ade-idle` heap 35.4 to 51.6 (+16.2), PSS 148.9 to 160.9 (+12.0). DOM nodes double (ade20 8782 at `left`, 16752 at `back`; `ADEDOM` `back` shows 1 view, 10 cards, 1429 nodes in the visible tree). Pass B `ade-idle`: Vue instances 5461 at `board`, 5556 at `left` (ADE tree still alive after leave, retained 8.8 to 12.0 MB, Array retained 3.2 to 11.7 MB), 10750 at `back`; v8heap 34.7 to 49.7.

Owner (D2 (2) met): the `AdeShell` component instance. `WorkbenchShell.vue` mounts the full view with `<component :is="fullView" v-if="isFull" />` (no KeepAlive); the unmounted `AdeShell` instance stays reachable, and its `subTree` keeps the whole ADE tree. Shortest retainer path from GC roots, DevTools console handles excluded (`ade20` `left`): `Window` to module Context to a Pinia store `EffectScope.effects` array to a `ReactiveEffect.onStop` closure to the pre-flush `job` (`job.i` is the component instance) to the `AdeShell` instance. Reading: a watcher created while `AdeShell` was the current component instance, owned by a store scope (store setup runs inside a component's setup), pins that instance for the store's lifetime. A second route found earlier: Pinia `repoVisibility` `showHidden` `Dep.subs` link chain to stale component effects, which pin the detached inputs (`AdeCaptureBox`, panel title input, `AdeEstimateField`). Both root at Pinia store scopes; neither is the cause of the leak by itself. The walk's root is the dev-only `__kiraTreeConnectionIds` hook closure (Pass B build); the store scope is independently rooted by the active Pinia instance, so the path exists in the minified build. That inference and the exact store are the first steps of F1.

Trials: 0 of 3 used. The retainer path names the pinned instance and mechanism; naming the store and testing the change is Part 2's F1.

Verdict: retention is real in both engines (not Chromium-only); D2 conditions (1) and (2) hold; F1 added.

## 9. Suspects

- S1 cleared. Board query 73.3 KB JSON, history 35.0 KB (48%); Go `board200` - `boot` heapInuse +0.9 MB; est. heap under 0.3 MB; both under floor.
- S2 cleared as retention. `push10` to `push50` heap 46.1 to 46.1; renderer PSS 184.9 to 195.7 (+10.8) then 188.1 at `final`: churn, not retained.
- S3 confirmed (R7), not fixable above noise. 2070 KB log JSON; heap +5.4, PSS +8.7; query still present at `back`. Capping `LOG_RETAIN_CHARS` yields at most 1.9 MB (section 10).
- S4 confirmed (R9). Headless terminals persist: 4212 lines from `launch4` to `back`, none reaped at `stopped4`; ArrayBuffer retained 8192 KB at `final`, 8423 KB at `stopped4`.
- S5 confirmed (R8). `Ring.Append` 4.25 to 4.56 MB at 4 sessions, freed on exit (heapAlloc 24.4 to 20.4).
- S6 confirmed (R6), section 8.
- S7 confirmed, not fixable (R5). `approval`: heap +0.8, PSS +3.3. `files1`: 2 editors, heap +10.6, native +14.9, PSS +27.5. 1 editor still live at `left`, 2 at `back` (part of R6); chunk (ExternalStringData 34.7 MB) never unloads; shared with repo view.
- S8 cleared. `storeBytes`: no store over 20 KB at any checkpoint or world; no `pinia:*` named root over 100 KB in Pass B.
- S9 not exercised. No git child process in the Go process table (peak included); `gitclient` 0.13 MB cum; `gitsession` caches unused by the fixture.
- S10 cleared under noise. Per session 2.1 MB heap (1.06 ring), 4.25 goroutines. After stop: goroutines +5 (floor 10), heapInuse residual 2.6 MB after harness base64 (floor 2, same run-to-run spread 0.3).
- S11 cleared. `mobileweb` absent from pprof top rows; nodes under 0.08 MB cum dropped.
- S12 cleared. `noObservers` 26 of 56 queries at `switch10`, 27 of 60 later; `panelForward` 0.9 KB total, `archived` 0 KB.
- S13 not exercised. The fake `claude` is exec'd by `sh`; class `shell` absent. Needs a Mac or real-shell run. Condition for any fix: `exec` is safe only if the login-shell PATH survives it.
- S14 measured (R1). `ade-idle` `board`: heap 35.4 [0.0], Chromium PSS 148.9 [1.0], WebKit 427.0 [6.4].
- S15 not exercised. No `js-worker` in the process table.

## 10. Targets

Row-level, each re-measured by Part 2 on the same checkpoints.

- R6 (F1): row = 51.3 MB WebKit `ade-idle` `back` - `board`; saving = the duplicate tree = 51.3; target <= 10 MB (floor, noise 6.4). Chromium: heap 16.2 to <= 2 (floor); PSS 12.0 to <= 10.
- R9 (F2): row 8.2 MB at 4 terminals; saving by reap at stop = 8.2 (`stopped4` ArrayBuffer retained 8.4 minus remaining 0.2 base); target <= 0.5 retained after stop. At 4 sessions the saving is under the 10 MB PSS floor, over the 2 MB heap floor; grows 2.05 MB per stopped session.
- R8 (F3): row 4.3 MB Go inuse; lazy ring when phone access off saves 4.3 (1.06 x 4); target <= 0.3 MB. Over the Go 2 MB floor.
- R2: no target until a mechanism is named (F4 gate).
- R7 cap (dropped): 2070 KB to 500 KB caps strings, Array, Map: (1.6 + 0.7 + 0.23) MB x (1 - 0.5/2.07) = 1.9 MB, under the 2 MB floor.

Record target: record 610.5. F1 and F2 act after `final` (F2 only after stop), so the record at `final` moves only by F3: 610.5 - 4.3 = 606.2 MB. Best case floor (all of R2 and R4 fixed, R1 untouched): 610.5 - 120.3 - 19.6 = 470.6 MB. Part 2 should add secondary budget rows at `back` and `stopped4`; record definition left to the user (section 13).

Proposed `docs/PERF.md` §1 budget rows (Part 2 adds):
- `ADE leave and return | ade20 WebKit WebContent PSS, back - board | <= 10 MB (was +78.0)`
- `ADE leave and return | ade-idle Chromium heap, back - board | <= 2 MB (was +16.2)`
- `Stopped agent sessions | ade-agents Chromium ArrayBuffer retained, stopped4 | <= 0.5 MB (was 8.4)`
- `Agent terminal ring | Go probe mobileterm inuse, idle60 | <= 0.3 MB (was 4.3)`
- `Kira-owned PSS | ade-agents webkit WebContent final + Go anon idle60 (harness base64 removed) | <= 606 MB (was 610.5)`

## 11. Part 2 fix list

| id | rows | mechanism | expected saving | user-visible | test half | files |
|---|---|---|---|---|---|---|
| F1 | R6 | Name the Pinia store whose scope holds the `ReactiveEffect` pinning the `AdeShell` instance (print `scope.effects` at the retainer path's Context). Instantiate that store before first mount (app bootstrap) so its watcher is created with no current component instance; trial: one `back - board` run. No library; Vue/Pinia lifecycle fix | 51.3 WebKit (ade20 up to 78.0); Chromium heap 16.2, PSS 12.0 | no | UI spec: ade20 leave and return, assert `__kiraCount` Vue/DOM nodes at `back` equal `board` (heap-ram probe numbers in PERF) | store file named by step 1; `frontend/src/main.ts` or the store's first consumer |
| F2 | R9 | Reap the headless terminal of a stopped session (`adeTerminals.ts` reap path; `attachHeadlessTerminal` in `createTerminalsStore.ts`). First check what the reap contract says and whether a stopped session's viewer needs the buffer | 8.2 MB at 4 sessions (2.05 each); grows per stopped session | maybe: stopped session's terminal scrollback gone (run log remains); confirm | UI spec: stop 4 sessions, assert no terminal stats; flow half: none (frontend state) | `packages/workbench/src/state/createTerminalsStore.ts`, `adeTerminals.ts` |
| F3 | R8 | Create the mobile replay ring only while phone access is on; no ring while off. No library; named requirement: replay needs history only for a phone attached after enabling | 4.3 MB Go inuse (1.06 per session); 0.7% of record | yes, edge: a phone enabled mid-session replays only output after enabling | flow test: 4 agent sessions with access off, `Broker` holds no ring bytes; UI half: not applicable (no UI) | `internal/mobileterm/broker.go`, `ring.go`, `internal/terminal/bound.go` |
| F4 (gated) | R2 | Diagnose first: native renderer growth +28.3 Chromium / +120.3 WebKit after sessions launch, heap only +10.0. Owner not named (xterm layers, session panel DOM, fonts). Add a Pass B ADE DOM breakdown for `ade-agents`; fix only if a mechanism over noise is named | unknown | unknown | decided after diagnosis | none yet |

Commit groups: (1) F1; (2) F2; (3) F3; (4) F4 diagnosis if kept.

## 12. Dropped (under noise or cleared)

- S1 board history: ~0.3 MB heap vs 2 MB floor (JS heap).
- S2 push churn: heap +0.0, PSS +10.8 and back to 188.1 at `final` vs 10 MB floor (renderer PSS); no retention.
- S3 log cap: 1.9 MB saving vs 2 MB floor (JS heap).
- S8 stores: all under 20 KB, floor 2 MB.
- S10 Go per-session residual after stop: 2.6 MB vs 2 MB floor, goroutines +5 vs 10.
- S11 `mobileweb`: under 0.08 MB cum vs 2 MB floor (Go inuse).
- S12 TanStack lingering families: 0.9 KB vs 2 MB floor.
- Approval dialog: +0.8 MB heap, +3.3 MB PSS vs floors 2 and 10.
- Board scale (20 vs 200 tasks): `ade200` heap 30.6 below `ade20` 34.1, no scale cost vs floor 2.
- `i18n:messages`: under 0.1 MB in Pass B named roots vs floor 2.
- Go runtime overhead (`Sys` - `heapInuse` 38.5 vs boot 26.3, +12.2): runtime page retention, no product owner; vs Go floor 2 MB but no mechanism, left.

## 13. Out of scope

- `claude` reduction (D3): not Kira-fixable. Measured Kira cost around each `claude`: Go 2.1 MB heap and 4.25 goroutines, renderer xterm buffer 2.05 MB, no resident shell on the Linux probe (S13). A session cap or idle-stop policy is a user decision, not in Part 2.
- Record definition: `final` precedes log, files and leave; F1 and most savings never show in the record. User decides whether to add `back` and `stopped4` as figures of record.
- User Mac "Agents module footprint run": not supplied; needed to confirm WebKit-vs-WebContent mapping and S13.
- Monaco chunk (R5): shared, lazy, cannot unload.
- R1 baseline (71% of record): bundle, Vue, engine; no single owner; separate phase if wanted.
- Chromium-only items: none; retained tree is in both engines.
