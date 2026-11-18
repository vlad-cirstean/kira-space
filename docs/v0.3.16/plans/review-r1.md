# v0.3.16 code review, round 1

Base commit: `4cf84a7b893659e76ae2b246becedbed6b988e4f` (merge-base with `origin/main`; no prior
review commit in range). Head reviewed: `396bedaf`. Scope: full code diff for P656-P671, P660 Part 1
included (Go, Vue/TS, scripts, workflows).

Dimensions: (1) architecture, structure, maintainability, security; (2) functional correctness and
business logic; (3) performance and resource efficiency.

No Critical or High findings. Correctness: no defect found beyond R1-4's doc status. Checked and
clean: shared embed worker (socket, flock, detach, idle exit, shared retry), release retention plan
and workflow, procroots and metrics sampler, view-state LRU and kept-scroll composables, panel search,
page stats worker and height anchor, SlickGrid column filter, xterm lazy open and link menu,
AutocompleteField underlay, RadioGroupCard and SecondaryTabs choice adoptions, approvals surface.

## Medium

### R1-1 Console document heights decode every row id on each recompute

- Where: `apps/kira-space/frontend/src/studio/views/console/useDocumentResultHeights.ts:27-38`;
  same pattern at `apps/kira-space/frontend/src/studio/views/documents/DocumentView.vue:163-173`.
- What: once any document is expanded (console) or collapsed (documents view), the `heights` /
  `rowHeights` computed calls `documentId(scope, row)` for every row in the result, not only the
  visible window. `setVisibleWindow` (`views/shared/page/store.ts:163-175`) prunes `decodeCache` to
  the window, so ids outside it are decoded again on every recompute. The computed reruns on each
  `pageVersion`, `rowsVersion` and `lineCountsVersion` bump.
- Failure scenario: 10k-row document result, one row expanded. Each scroll that loads a page, or each
  line-count answer from the page-stats worker, re-decodes up to 10k ids on the main thread. This is
  the main-thread cost P662 set out to remove; it returns as jank while scrolling a large result.
- Fix direction: keep a per-scope row-to-id map filled once per page load (not pruned by the visible
  window), or invert the lookup: map each expanded/collapsed id to its row once, then build heights
  from that small set and the default height for every other row.

## Low

### R1-2 TerminalResolve: stale trust comment, redundant `Openable`, per-call repo symlink walks

- Where: `apps/kira-space/internal/bridge/openedfiles.go:94-121`;
  `apps/kira-space/internal/openedfiles/terminal.go:71-80`;
  `apps/kira-space/internal/openedfiles/openedfiles.go:61-90`.
- What: (a) Comment says "the answer is a boolean limited to the supported file types". P671 made
  the call return absolute paths, and any file type inside a repo. The trust reasoning no longer
  matches what it returns. (b) `OpenedFilesTerminalResolveResult.Openable` duplicates `Paths[i] != ""`;
  the frontend reads `paths`, so `Openable` is dead weight on the contract. (c) `resolve` runs
  `filepath.EvalSymlinks` on every repo root for every candidate. With 20 paths, 2 cwds and N repos,
  that is up to 40·N root symlink walks per hovered terminal row.
- Failure scenario: (a) a later change trusts the comment and widens what the call returns. (c) A
  user with many repos on a slow or network filesystem sees laggy link detection on hover.
- Fix direction: rewrite the trust note to cover returned paths. Drop `Openable` from the result and
  its TS type. Resolve repo roots once per `Openable` / `OpenTerminalPath` call and pass them into
  `resolve`.

### R1-3 pprof server has no Host-header check

- Where: `internal/pprofserve/pprofserve.go:28-63`.
- What: `KIRA_PPROF` binds only loopback, but handlers accept any `Host`. A page the user visits can
  use DNS rebinding to reach `127.0.0.1:<port>` and read `/debug/pprof/*` (heap, goroutine stacks,
  cmdline) and the metrics snapshot.
- Failure scenario: developer runs with `KIRA_PPROF=127.0.0.1:6060` and opens a hostile page. The page
  rebinds its domain to 127.0.0.1 and pulls a heap profile containing in-memory query results or
  connection data.
- Fix direction: wrap the mux in a handler that rejects requests whose `Host` is not a loopback
  literal or `localhost` (with the bound port). Opt-in and off by default, hence Low.

### R1-4 SPEC P656 status says Planned while its Result says Done

- Where: `docs/v0.3.16/SPEC.md:8` versus the P656 section Result (`SPEC.md:38`).
- What: the table row reads `Planned`; the Result reads `Done.` and the code landed.
- Failure scenario: the close phase folds the row as not done, or a resumed orchestrator re-plans
  P656.
- Fix direction: set the row status to `Done`.
