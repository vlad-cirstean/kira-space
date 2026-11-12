# P624 audit C1: plan, board, cards, panel

Read-only audit, Stream C, auditor C1. Base: `82cffb85`. Scope: `frontend/src/ade/v2` plan view,
day bands, task cards, branch rows, task and branch panels, status/flag chips, board queries and
cache keys, board push handling, Pinia stores for these. Sessions, terminal, agents, workflows,
dialogs, approvals belong to C2; cross-boundary hot spots listed at the end.

Paths below are relative to `apps/kira-space/frontend/src/` unless they start with `packages/`.

## Hot path in one paragraph

Every board input lands in one `computed` (`usePlanModel`, `ade/v2/plan/usePlanModel.ts:384-460`).
It rebuilds the timeline, progress for every live task, needs-you, and a fresh `CardModel` object
for every shown card. New card objects mean every `AdeTaskCard` and `AdeBranchRow` re-renders and
re-runs its computeds. Inputs that trigger it: board refetch, runs push, sessions push, agent phase
change, change-request refetch, task selection, repo filter, and a 30 s clock tick. The board query
holds its data in a deep `reactive`/`readonly` proxy, so each rebuild walks every task, branch and
run through proxies and tracks thousands of deps. A run event triggers it twice (runs merge, then
the debounced board refetch).

## Findings, ranked by user-visible impact

### C1-1 Selecting a task rebuilds the whole plan model

- Where: `ade/v2/plan/usePlanModel.ts:415` (`selectedId = ui.selectedTaskId`), `:430` (`ripple`),
  `:303-304` (`selected`, `ripple` baked into each `CardModel`).
- Slow: click on a card, `ui.select()`, panel open. The model computed reads
  `ui.selectedTaskId`, so every selection rebuilds timeline, all progress, needs-you and every card,
  then every card and branch row re-renders. Same on every keyboard move between cards. This is the
  "open task" latency; it scales with task count and run history, not with what changed (two cards).
- Fix: drop `selected` and `ripple` from `CardModel`. Add a separate `computed` in `usePlanModel`
  for the ripple (`rippleOf(view, selection)` returning `Set<string>` for task and branch ids), read
  only by components. `AdeTaskCard`/`AdeBranchRow` read `ui.selectedTaskId === card.task.id` and
  `ripple.value.has(id)` directly from the `adeBoardUi` Pinia store. Selection then re-renders the old
  card, the new card and ripple members only.
- Effort: S. Risk: low; `card.selected` callers are few (`AdeTaskCard`, `AdeTaskPanel` facts,
  menu model). Grep `\.selected\b` and `\.ripple\b` on `CardModel` users.
- Feedback: none needed; synchronous UI.

### C1-2 Monolithic model: every board, runs, sessions or clock change re-renders every card

- Where: `ade/v2/plan/usePlanModel.ts:384-460`; triggers `:321-326` (30 s tick, `now` read at `:414`),
  `:359-370` (sessions), `:433` (change-request titles), `ade/v2/readSignals.ts:41-42` (runs merge
  into board).
- Slow: one changed run re-creates every `CardModel` and `BranchRowModel`. Every card re-renders
  (header, step segments, ~8 tooltips, chips), every row re-runs `tagText`, `choice`, `split`. Idle
  board still rebuilds all cards every 30 s. With agents running, each sessions push and each phase
  flip (`phaseKey`) rebuilds all. `buildTaskProgress` runs for all `b.tasks` (`:403-413`), not only
  shown ones, over full run history (`Task.runs` keeps history).
- Fix: layer the model into memoized pieces, each its own `computed` in the same shared composable:
  1. `progressByTask`: `Map<taskId, TaskProgress>` reusing the previous entry when the `Task` object
     (structurally shared by TanStack Query, so unchanged tasks keep identity) and its workflow are
     the same reference. Keep a `WeakMap<Task, {wf, progress}>` cache.
  2. `timeline`: depends only on `board.tasks/branches/plan/pairs/history`, settings, `today`,
     UI filters. Not on sessions, change requests, `now` or selection.
  3. `cards`: per id, reuse the previous `CardModel` when its inputs (task ref, progress ref,
     branch refs, session slice, change-request titles of its branches, needs item, status def) are
     shallow-equal. Cache by id across recomputes.
  4. `nowMs`: quantize to the minute and pass only to the parts that print relative time
     (`branchTag`, needs-you age). Better: move the relative-time label into the component
     (`useNow` from VueUse, already used pattern via `liveNow`) so the clock never touches the model.
  Result: a runs push touching one task re-renders one card.
- Effort: L. Risk: medium; correctness of memo keys. Guard with existing
  `tests/unit/ade-v2-board-parity.spec.ts` and `ade-v2-timeline.spec.ts`; add one unit test for the
  card-reuse rule (cache invalidation with interacting inputs earns a test per working agreement).
- Feedback: none.

### C1-3 Board-family queries hold data in deep reactive proxies

- Where: `ade/v2/readQueries.ts:22-52` (`useBoard`, `useChangeRequests`, `useWorkflows`,
  `useSessions`). `@tanstack/vue-query@5.105.1` `useBaseQuery`: `shallow` defaults to false, so
  state is `reactive(...)` and returned through `readonly(...)` (checked in
  `node_modules/.../vue-query/build/modern/useBaseQuery.cjs:30,78`).
- Slow: every property read inside the model rebuild (`buildTimeline`, `buildCard`, `needsYou`,
  progress) goes through a readonly-reactive proxy and registers a dependency. For a board of N tasks
  with R runs and B branches the model computed tracks O(N+R+B) fine-grained deps and pays proxy
  get-trap cost on each read. Data is replaced wholesale on every fetch anyway, so deep tracking buys
  nothing.
- Fix: pass `shallow: true` to these four `useQuery` calls (official vue-query option). Data
  becomes a plain object behind `shallowReadonly`; the model still re-runs on a new `data` reference.
  Consider making it the default in `packages/workbench/src/state/queryClient.ts` for ADE keys via
  `queryClient.setQueryDefaults(['adetask'], { shallow: true })` if the option is accepted there;
  otherwise per call.
- Effort: S. Risk: low; nothing mutates query data (writes go through `setQueryData`). Any code
  relying on deep reactivity of `data` (e.g. `watch(() => data.value.x.y)` without a new reference)
  would stop firing; grep for watchers on `board.data.value.` paths first.
- Feedback: none.

### C1-4 Board push: refetch cancel-and-restart, double rebuild per run event, wide invalidation

- Where: `ade/v2/readSignals.ts:28-32,41-48`; `ade/queries.ts:76-86,99`; backend
  `internal/ade/runs.go:305-308` (`emitRuns` sends the runs push and schedules the board push),
  `internal/ade/gitfacts.go:20` (250 ms trailing debounce).
- Slow:
  - `invalidateQueries` uses `cancelRefetch: true` by default. A push that arrives while the board
    fetch is in flight cancels it client-side and starts a new one. The Wails call is not aborted, so
    the backend still computes the discarded `Board()` (git facts per repo, fan-out 4). Under steady
    agent activity (worktree edits every few hundred ms, debounce 250 ms) and a `Board()` slower than
    the push interval, the client can go long stretches with no fresh board while the backend does
    duplicate work.
  - Each run state change: runs push merges into board (full model rebuild, C1-2), then ~250 ms later
    the board push refetches (second full rebuild).
  - Each board push also refetches `changeRequests` (host lookups per repo, backend-cached but stale
    marked), `panelForward`, `reviewStep`, `handovers`, `artifacts`, `issue` for every mounted panel.
    With the task panel open that is about 7 IPC calls per push.
- Fix:
  - Single-flight board refetch, shared by desktop and phone in `installAdeReadSignals`: on push, if
    `queryClient.isFetching({ queryKey: boardKey, exact: true })` set a `dirty` flag and
    `invalidateQueries(..., { cancelRefetch: false })`; subscribe to `queryClient.getQueryCache()`
    `updated` events for `boardKey` and refetch once when the fetch settles with `dirty` set. Apply
    the same helper to `changeRequestsKey`.
  - Invalidate `changeRequestsKey` only when branch names or remote refs can have changed. Needs a
    reason on the board event (cross-stream A1: carry `{ refs: boolean, taskIds: string[] }` in
    `AdeTaskBoardChanged`). Until then keep current behaviour.
  - With task ids in the payload, invalidate per-task keys (`artifacts`, `issue`, `handovers`,
    `panelForward`) for those tasks only.
  - Optional (A1): skip the board refetch after a runs push when the board push reason is only
    "runs changed", since `mergeRuns` already patched the cache.
- Effort: M (coalescer S; payload work is A1's). Risk: low for the coalescer; missed-update risk if
  the dirty flag is cleared before the refetch starts, so set it per push and clear on fetch start.
- Feedback: none directly; makes running-state chips on cards update promptly under load.

### C1-5 Per-card hook fan-out: ~15 mutation observers and ~20 tooltip roots per card

- Where: `ade/v2/plan/AdeTaskCard.vue:56-64` (`useTaskMenu`, `useReviewCode`, `usePlanModel`,
  `useTaskUpdates`); `ade/v2/plan/useTaskMenu.ts:36-52` (`useSetPlan`, `useShelveTask`, `useStopRun`,
  `useUpdateTask`, `useTaskAction`, `useReviewCode`, `useRunAutomation`, `useHostLabels`);
  `ade/v2/run/useTaskAction.ts:14-22` (`useStartRun`, `useApprove`, `useRetryRun`);
  `ade/v2/run/AdeTaskActionButton.vue:14` (second `useTaskAction` per card);
  `ade/v2/plan/AdeTaskStatusMenu.vue:28` (`useUpdateTask`); `ade/v2/plan/AdeBranchRow.vue:62-66`
  (`useRepos` query observer, `useReviewCode` mutation per row);
  `ade/v2/automation/AdeAutomationChip.vue:18-20` (`useScriptRuns` observer plus a scan of all runs per
  card); `ade/v2/AdeTip.vue` (one reka `TooltipRoot` per tip, ~8 per card header plus per row).
- Slow: mount cost for first paint of the Plan, for "Load all", and for every card that enters the
  list. Counted: AdeTaskCard 1 + useTaskMenu 9 + AdeTaskActionButton 3 + status menu 1 = 14
  `useMutation` per card, each a `MutationObserver` + reactive state + options watcher, nearly all
  unused until a menu item is clicked. Rows add a query observer and a mutation each. Not measured;
  cost is linear in shown cards and dominated by C1-2 today, so it surfaces once C1-2 lands.
- Fix:
  - Hoist the menu and action mutations into one shared composable (`createSharedComposable` from
    VueUse, as `usePlanModel` already does): `useTaskWrites()` owning `setPlan`, `shelve`, `stopRun`,
    `update`, `start`, `approve`, `retry`, `openReviewWindow`, each with a `mutationKey`
    (`['ade', 'startRun']` etc.).
  - Per-card pending from `useMutationState({ filters: { mutationKey, status: 'pending' }, select: m
    => m.state.variables })` filtered by `taskId`, or `useIsMutating` with a predicate. This also
    fixes C1-F3 (pending visible on every surface).
  - `useTaskMenu`: build menu items lazily on open (already does), take writes from the shared
    composable.
  - `AdeBranchRow`: read repo integration targets from one shared `computed` keyed by repo id instead
    of `useRepos()` per row.
  - `AdeAutomationChip`: one shared `computed` `Map<taskId, chipRun>` instead of a scan per card.
  - Tooltips: measure first (Vue devtools component count / mount timing with 50 cards). If roots
    show up, render static text tips through the native `title` only where a rich tip is not needed,
    never a hand-rolled tooltip.
- Effort: M. Risk: low-medium; pending/error semantics per surface must stay (errors still land in
  `ui.actionError[taskId]`).
- Feedback: improves C1-F3.

### C1-6 Plan model rebuilds while the Plan is hidden

- Where: `ade/v2/shell/AdeShell.vue:33-39` (`needsCount` reads `model.value?.needs.badge` in the
  always-rendered tab bar); `:35-38` (`shelfSelected` reads `cardFor`).
- Slow: on the Agents, Workflows, Repos or Needs tab the full plan model (C1-2) still rebuilds on
  every board, runs and sessions push, only to feed the badge. Agents tab is where pushes are most
  frequent (C2's surface pays this cost).
- Fix: after C1-2 layering, expose `needs` as its own computed (progress + sessions + board, no
  timeline, no cards) and read only that in `AdeShell`. `shelfSelected` reads
  `board.plan`/shelf facts via `shelfOf(board, id)` directly, not `cardFor`.
- Effort: S once C1-2 lands. Risk: low.
- Feedback: none.

### C1-7 Day band props built by template function calls

- Where: `ade/v2/plan/AdePlanView.vue:88-132` (`bandCards`, `bandSpans`, `bandHistory`), used at
  `:360-378`; `:182-184` (`dragOverDay`).
- Slow: each AdePlanView render creates new arrays for every band, so every `AdeDayBand` re-renders
  whenever AdePlanView does: on refresh-chip state, plan error, history toggle, and on every drop
  target change during a drag (pointer crossing a band). Cards themselves skip when their object is
  unchanged, so the cost is band chrome, spans, history rows.
- Fix: one `computed` `bandProps: Map<bandKey, { cards, spans, history, notes }>` derived from the
  model; template reads `bandProps.get(band.key)`. `dragOver` stays a per-band boolean prop.
- Effort: S. Risk: low.
- Feedback: none.

### C1-8 ADE shell loads every tab page and dagre eagerly

- Where: `ade/v2/shell/AdeShell.vue:7-25` (static imports of `AdeAgentsPage`, `AdeWorkflowsPage`,
  `AdeReposPage`, `AdeLaterPage`, `AdeNeedsPage`, plus `AdeRunDialog`, `AdeAgentDialog`,
  `AdeStageDialog`, `AdeTakeOverDialog`, `AdeAbortRebaseDialog`, `AdeRepoGateDialog`);
  `workbench/modes.ts:5` (static `AdeView`); `ade/v2/state/adeWorkflowDraft.ts:18` imports
  `board/workflowGraph.ts:1` (`@dagrejs/dagre`). Built bundle (`dist/assets/index-*.js`, 3.98 MB)
  contains dagre `graphlib`, despite `AdeWorkflowGraph` being async
  (`ade/v2/workflows/AdeWorkflowEditor.vue:17`).
- Slow: app start parses and evaluates the whole agents module plus dagre before the Plan paints.
- Fix: `defineAsyncComponent` for the five non-Plan pages in `AdeShell` (they render under
  `v-else-if`, so lazy load is natural) and for dialogs that open on demand. Split
  `workflowGraph.ts`: keep pure step/edge helpers the draft store needs in a dagre-free module; only
  `layoutWorkflow` imports dagre, reached from the async `AdeWorkflowGraph` chunk. `AdeView` stays
  eager since `ade` is first in `MODE_ORDER`.
- Effort: S-M. Risk: low; first open of a tab shows a brief load, cover with the existing
  `QueryState` loading pattern or a `Spinner` fallback in `defineAsyncComponent({ loadingComponent })`.
  C2 owns the page internals; this finding touches only the shell imports and the draft store's
  import line (coordinate with C2).
- Feedback: tab switch fallback spinner on first load.

### C1-9 "Load all" renders every card with no windowing

- Where: `ade/v2/board/timeline.ts:26,334-336` (`ITEM_LIMIT = 10`, lifted by `showAllItems`);
  `ade/v2/plan/AdeDayBand.vue:184-191`.
- Slow: default cap keeps the list short; after "Load all" every task mounts a full card (C1-5 cost)
  and all are laid out and painted.
- Fix: measure first (50+ tasks, "Load all", performance panel). If paint/layout dominates, add
  `[content-visibility:auto] [contain-intrinsic-size:auto_96px]` Tailwind arbitrary classes on the
  card wrapper. If mount dominates after C1-5, window per band with `@tanstack/vue-virtual`; SortableJS
  drop resolution already uses pointer hit-testing (`usePlanDrag.ts`), so unmounted offscreen cards
  only need their band to stay mounted.
- Effort: M. Risk: medium (drag, scroll-to-day, keyboard focus). Only after measurement.
- Feedback: none.

### C1-10 `inHistory` forces layout reads on every scroll event

- Where: `ade/v2/plan/AdePlanView.vue:273-281` (reads `scrollY`, calls `getBoundingClientRect` twice
  and `querySelector`).
- Slow: while history is open, each scroll event re-evaluates the computed and reads layout. No write
  in between, so no thrash, but it runs at scroll frequency.
- Fix: `useIntersectionObserver` (VueUse) on the today band with `rootMargin` equal to the sticky
  header height; `inHistory` = today band below the viewport top.
- Effort: S. Risk: low.
- Feedback: none.

Nothing real found: deep watchers (`deep: true` absent in `ade/v2`), `adeOps.forTask` cost (op map
is near empty), `AdeBranchRow.tagText` per-second tick (string computed, re-renders only on change),
optimistic updates for plan, status, shelve, step (present via `optimisticUpdate`).

## Feedback inventory (slow or async actions in scope)

Surface rule: extend `useAdeOpsStore` (`ade/v2/state/adeOps.ts:8,45-78`), `AdeOpStatus` chip
(`ade/v2/plan/AdeOpStatus.vue`, already supports `branchId`) and the vue-sonner toast it raises.
Never a parallel mechanism.

| Action | Running state | Finished / failed notice | Gap |
|---|---|---|---|
| Archive task, remove branch, remove repo | adeOps chip + loading toast | success/error toast, `actionError` | none |
| Refresh repos (fetch) | per-repo chip spinner (`AdePlanView.vue:50-72`) | summary in chip, inline error | none |
| Plan move, rollover, overflow move | optimistic | `planError` Alert | none |
| Day off move | confirm dialog `pending` | dialog error | none |
| Status change, park, orchestrator, shelve | optimistic | `actionError` in panel | none |
| Run / Approve / Retry (card, panel) | button `pending`, per instance | failure `actionError`; success via push | C1-F3 |
| Open review window | button disabled only | failure `actionError` | C1-F3 |
| Stop runs (task menu) | none | failure `actionError` only | C1-F2 |
| Force push (row, branch panel) | modal blocks with `pending` | dialog error | C1-F1 |
| Retry setup (branch panel, setup progress) | none until push shows `preparing` | inline error | C1-F4 |

### C1-F1 Force push holds a modal open for the whole push

- Where: `ade/v2/AdeForcePushDialog.vue:15-22` (`useConfirmRun` awaits `forcePush.mutateAsync`).
- Gap: network push can take seconds; user cannot act on the board meanwhile. No chip on the row.
- Fix: add `AdeOpKind` `'forcePush'` with loading/done i18n keys; on confirm close the dialog and
  `ops.run({ id: 'forcePush:<branchId>', kind: 'forcePush', taskId, branchId, name: branchName },
  () => forcePush.mutateAsync(...).then(throwOnRemoteError))`. A result with `res.error` must reject so
  the op raises the error toast. `AdeOpStatus` on the branch row shows the chip already.
- Effort: S. Risk: low; keep the risk text in the confirm step.

### C1-F2 Stop runs from the task menu shows nothing

- Where: `ade/v2/plan/useTaskMenu.ts:75-81`.
- Gap: stopping agent processes can take a while; no running state, no success notice.
- Fix: `AdeOpKind` `'stopRuns'`, `ops.run({ id: 'stopRuns:<taskId>', ... }, () => Promise.all(...))`.
  Same for `AdeWorkflowReviewPanel` and `AdeSessionActions` stop buttons (C2 surfaces; flag to C2).
- Effort: S. Risk: low.

### C1-F3 Action pending is per component instance

- Where: `ade/v2/run/useTaskAction.ts:76-80` (`useAsyncAction` per instance), used by
  `AdeTaskActionButton` on the card and in the panel, and by `useTaskMenu`;
  `ade/v2/review/useReviewCode.ts:93` (`pending` per instance).
- Gap: clicking Run on the card shows busy only there; the panel's Run button for the same task stays
  live and can launch twice. Review button shows no spinner.
- Fix: pending from `useMutationState`/`useIsMutating` keyed by `mutationKey` and `taskId` (see
  C1-5). Pass `:pending` to the review `TooltipIconButton` (falls through to `Button`). Keep these
  fast actions off adeOps (no toast for a start; the card state changes at once).
- Effort: S with C1-5. Risk: low.

### C1-F4 Retry setup has no running state

- Where: `ade/v2/panel/AdeSetupProgress.vue:19-36`, `ade/v2/panel/AdeBranchPanel.vue:116`.
- Gap: between click and the board push showing `preparing`, nothing changes.
- Fix: bind `retry.isPending` to the button `pending` prop. No toast: the setup progress row is the
  finished/failed surface.
- Effort: S. Risk: low.

## Cross-boundary hot spots

- A1 (backend board): `Board()` cost per push and absence of a reason/task-id payload on
  `AdeTaskBoardChanged` limit C1-4. Wails call for a discarded board fetch is not cancelled.
- A1: every board push makes the frontend refetch `ChangeRequests` (host lookups) and `adeTaskIssue`
  (tracker CLI behind a kept copy) for open panels.
- C2: rebase, change base, merge, start branch run through `adeDialogs` flows; their running and
  finished feedback is C2's call. Stop buttons in `AdeWorkflowReviewPanel.vue:67` and
  `AdeSessionActions.vue:17` need the same `'stopRuns'` op as C1-F2.
- C2: Agents tab pays the hidden plan-model rebuild (C1-6). Async page split in C1-8 touches C2's
  page entry points and `adeWorkflowDraft.ts` imports.
