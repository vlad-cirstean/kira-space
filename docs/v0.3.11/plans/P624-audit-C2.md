# P624 audit C2: sessions, terminal, workflows, dialogs, editors

Base commit: `82cffb85`. Read-only performance audit, Stream C auditor C2. No code changed.

Scope: sessions tab, terminal renderer (xterm output, resize, focus), agents page, workflows graph,
proofs window and inline proof steps, approvals and prompt dialogs (`packages/workbench/src/approvals/`,
`frontend/src/ade/v2` dialogs), leases pane, notes editor, Monaco usage, lazy loading of heavy
dialogs and editors, global listeners and stores. C1 owns plan, board, task cards, branch panel, task
detail; overlaps marked "cross C1".

Method: `codegraph_explore` (projectPath `/home/user/kira-space`) for every discovery question,
then targeted reads of named lines. Numbers given only where cheap (constants in code); no traces run.

Paths: `fe` = `apps/kira-space/frontend/src`, `wb` = `packages/workbench/src`.

## Feedback surface

Standard surface: `fe/ade/v2/state/adeOps.ts` (Pinia store `run(op, fn)`: op map, `toast.loading`
then `toast.success`/`toast.error` by id, `ui.actionError` on fail) plus `AdeOpStatus.vue` chip
(Badge + Spinner). Today `AdeOpKind` is only `removeBranch | removeRepo | archive`, and the chip
renders only in `AdeTaskCard` and `AdeBranchRow`. Extend this store and chip; never add a parallel
mechanism.

Slow or async actions in scope:

| Action | Running state | Finished/failed notice | Gap |
| --- | --- | --- | --- |
| Restart session (`useSessionMenu.ts:37-50`) | menu item disabled via local `busy` Set | error only, to `ui.actionError`; no success | no spinner on strip tab or row, no toast (C2-7) |
| Close session (`useSessionMenu.ts:52-63`) | same local `busy` Set | error only | same (C2-7) |
| Stop session (`AdeSessionActions.vue:50`) | button pending | none on success | success toast (C2-9) |
| Take over (`adeTakeOver.ts:53-77`) | pending Set, setup wait | dialog flow | none |
| Start agent (`useTaskLaunch.ts:43-54`) | launch menu pending | error to `ui.actionError` only | success toast (C2-9) |
| Show/focus session (`AdeTuiPane.vue:54-61`) | none on Show button | "missing" hint only | pending + error toast (C2-9) |
| Run dialog send (`AdeRunDialog.vue`) | Send pending | dialog closes; no toast | success toast; preview loading (C2-8) |
| Agent dialog send (`AdeAgentDialog.vue:115-122`) | `sending` ref | dialog closes | none real |
| Approval answer (`approvalsQueries.ts:63-82`) | per-button pending | push removes item | none |
| Deny all (`ApprovalsDialog.vue:61-68`) | none | none | pending + result toast (C2-9) |
| Workflow save (`AdeWorkflowSaveBar`) | `:saving` | none on success | success toast (C2-9) |
| Lease request / force release (`AdeLeasesPane.vue:64+`, `adeLeases.ts`) | none on menu items | `toast.error` only | pending + success toast (C2-9) |
| Lease add/remove | `save.isPending` | inline | none |
| Agent file save (`useAgentFileEditor.ts`) | `save.isPending` | inline error | none |
| Note save (`AdeNoteEntry.vue`) | `useAsyncAction` saving | inline | none |
| Open proof window (`AdeProofEntry.vue:22-24`) | none | none on failure | pending + error toast (C2-9) |

## Findings (ranked by user-visible impact)

### C2-1 xterm draws with DOM renderer

- Where: `wb/terminal/terminalRenderer.ts:6-9` (imports only `Terminal`, `FitAddon`, `WebLinksAddon`),
  `getOrCreateTerminal` 146-221 (scrollback 5000).
- Slow: no `@xterm/addon-webgl`, so xterm 6 falls back to DOM renderer: one span per styled cell run,
  full row rebuild per write. Agent TUIs (claude) redraw whole screen at high rate; Go side already
  coalesces at 16ms / 16KiB (`internal/terminal/service.go:154-201`), so client paint is bottleneck.
  Shows as laggy scroll and typing in busy session tabs.
- Fix: load `@xterm/addon-webgl` (MIT, version matching xterm 6) after `term.open`; on
  `onContextLoss` dispose addon and stay on DOM renderer. Keep it in lazy terminal chunk.
- Effort M. Risk: WebGL context limit (browsers cap ~16 live contexts) with many headless/attached
  terminals; attach addon only to mounted visible terminal and dispose on detach.
- Feedback: none (no user action).

### C2-2 Resize storm on every frame of drag

- Where: `wb/terminal/useTerminalMount.ts:115-121` (ResizeObserver: `fitTerminal` then
  `resizeTerminal(tabId, cols, rows)` each callback); `wb/state/createTerminalsStore.ts:246-248`
  (`resizeTerminal` fire-and-forget bridge call); `attachHeadlessTerminal` 161-176 (headless xterm
  in detached div at default 80x24).
- Slow: no unchanged-dims guard, no debounce. Panel drag fires bridge call per frame; each one sends
  pty SIGWINCH and agent TUI repaints full screen, which then floods output back (feeds C2-1). First
  attach of headless terminal always resizes from 80x24, so every session switch costs one full TUI
  redraw.
- Fix: keep local `fitTerminal` per frame; skip bridge call when cols/rows equal last sent; send
  trailing resize via VueUse `useDebounceFn` (~100ms). Seed headless terminal with last known
  cols/rows of its pane so first attach is no-op. Optionally move observer to VueUse
  `useResizeObserver` (existing comment declines it; reason no longer applies once debounce exists).
- Effort S. Risk low: last resize must still reach pty (trailing edge).
- Feedback: none.

### C2-3 Full TipTap editor per note in task chain (cross C1)

- Where: `fe/ade/v2/details/AdeNoteEntry.vue:187-194` renders `AdeNotesEditor` readonly per note
  and handover; `fe/ade/v2/notes/AdeNotesEditor.vue:67-102` builds TipTap `Editor` (StarterKit,
  TaskList, Placeholder, Markdown) in setup.
- Slow: N ProseMirror instances plus N markdown parses when task Details opens; long chains stall
  first paint of task detail.
- Fix: render readonly body without editor: parse markdown once to HTML (TipTap `MarkdownManager` +
  `@tiptap/static-renderer`, or cache HTML per note rev) and `v-html` it with Tailwind `prose`
  classes. Mount editor only on Edit. Mount off-screen entries via VueUse `useIntersectionObserver`.
- Effort M. Risk: visual parity of task lists/checkboxes between static and editor render; cover in
  existing note UI spec.
- Feedback: none.

### C2-4 TipTap and all ADE dialogs in boot chunk

- Where: `wb/modes.ts:5` statically imports `AdeView`; `fe/ade/v2/shell/AdeShell.vue:1-25,163-178`
  statically imports every page and always mounts `AdeRunDialog`, `AdeRepoGateDialog`,
  `AdeAgentDialog`, `AdeAbortRebaseDialog`, `AdeTakeOverDialog`, `AdeStageDialog`.
  `AdeNotesEditor` imported statically by `AdeAddPopover` (top bar, always present), `AdeStageDialog`,
  `AdeTaskChain`, `AdeNoteEntry`, `request/TaskCreateBody`.
- Slow: tiptap + ProseMirror + markdown and every dialog body parse at startup; only
  `defineAsyncComponent` in `ade/v2` is `AdeWorkflowGraph`. Startup time and memory, not steady
  state.
- Fix: `defineAsyncComponent` for `AdeNotesEditor` with plain textarea-sized placeholder, prefetch on
  idle (`requestIdleCallback`) or on popover hover so first open has no blank frame. Mount dialogs
  with `v-if` on their open state and load them async; keep shadcn-vue `Dialog` shell. Lazy page
  components in `AdeShell`.
- Effort M. Risk: first-open flash; prefetch removes it. UI specs that assert dialog DOM while closed
  need checking.
- Feedback: none.

### C2-5 Run log: O(n) work per push and per render

- Where: `fe/ade/v2/panel/AdeRunLog.vue:16-22,38` (`toLines(data.chunks)` inline in template: maps
  all chunks + regex `/exit code [1-9]/i` every render); `pushMerge.ts` `appendChunks` 19-24 (copies
  whole chunks array per push); `LogLines.vue:115,119` (`segmentsOf(index)` twice per row, so
  `parseAnsi` twice).
- Slow: each output push copies full array and re-derives every line, in each mount
  (`AdeHeadlessPane`, `AdeSessionsRow`, `AdeRunOutcome`, `AdeNeedsRow` mount it independently).
  Client cache grows unbounded while server keeps 2MB tail (`repos/adelogs.go` `AdeLogTailBytes`).
  Long headless runs slow down over time.
- Fix: derive line/stream fields once inside `appendChunks` (or WeakMap memo per chunk); `computed`
  in `AdeRunLog`; hold chunks in `shallowRef` and push in place; cap client chunks to same 2MB tail;
  call `segmentsOf` once per row (local const in template via `v-for` scoped variable or memo).
- Effort S-M. Risk low; ordering logic in `pushMerge` stays.
- Feedback: none.

### C2-6 Workflow graph re-layouts on every keystroke

- Where: `fe/ade/v2/workflows/AdeWorkflowGraph.vue:97-99` (`layout` computed runs `layoutWorkflow`,
  full dagre layout, `workflowGraph.ts:219-300`); `flowNodes` 120-130 and `flowEdges` 131-157 rebuild
  all; `adeWorkflowDraft.ts` `patchStep` 97-117 via `edit` 72-78 replaces draft per keystroke.
- Slow: typing a step name or prompt in inspector reruns dagre and rebuilds every Vue Flow node.
  Noticeable on larger workflows.
- Fix: memo layout on topology key (step ids, kinds, results/next, loops); text-only patches leave
  positions untouched. Keep node `data` separate from positions so Vue Flow updates one node.
  Debounce draft writes from text inputs with `useDebounceFn`.
- Effort M. Risk: stale layout if key misses a topology field; derive key from same fields
  `layoutWorkflow` reads.
- Feedback: none.

### C2-7 Session restart/close: no visible running state or result

- Where: `fe/ade/v2/sessions/useSessionMenu.ts:37-63`. `busy` is a reactive Set local to each
  `useSessionMenu` instance, so strip tab and row do not share it; it only disables menu item.
- Slow: restart waits on pty teardown and agent relaunch (seconds); user sees nothing until session
  list push arrives, and no success notice. Failure goes to `ui.actionError` only.
- Fix: add `restartSession` and `closeSession` to `AdeOpKind`; route both through `adeOps.run` (gives
  loading/success/error toast); render `AdeOpStatus` (new `forSession` getter) on strip tab and
  sessions row; drop local `busy` Set in favor of store state.
- Effort S-M. Risk low.
- Feedback: this finding is the feedback requirement.

### C2-8 Run dialog: blank message and previews on open

- Where: `fe/ade/v2/run/AdeRunDialog.vue:70-89` (three `useAdePromptPreview` queries,
  `queries.ts:793-808`, `staleTime: 0`), 152-157 (`watch(initial)` fills textarea after load).
- Slow: dialog opens with empty textarea and no loading state; content pops in after Go previews
  return; reopen refetches.
- Fix: `queryClient.prefetchQuery` on Run trigger hover/menu open; shadcn-vue `Skeleton` while
  pending; short `staleTime` (invalidated by existing read signals) instead of 0.
- Effort S. Risk low.
- Feedback: loading skeleton; success toast on send via `adeOps` (new `runTask` kind) or
  `toast.success`.

### C2-9 Missing pending/notice on small async actions

- Where and gap:
  - Show/focus session, `AdeTuiPane.vue:54-61,106`: no pending on Show; failure only as "missing"
    hint.
  - Deny all, `wb/approvals/ApprovalsDialog.vue:61-68`: no pending, no result.
  - Workflow save, `AdeWorkflowSaveBar`: pending exists, no success notice.
  - Lease request / force release, `fe/ade/v2/agents/AdeLeasesPane.vue:64+`, `state/adeLeases.ts`:
    no pending on menu items, `toast.error` only.
  - Stop session, start agent: pending exists, no success notice.
  - Open proof window, `details/AdeProofEntry.vue:22-24,48,118`: mutation has no pending or error
    surface.
- Fix: use `isPending` of existing mutations for Button disabled + Spinner (shadcn-vue); route
  longer ones (deny all, lease force release, stop) through `adeOps.run` with new kinds; plain
  `toast.error` for instant ones (show, open proof).
- Effort S each. Risk low.
- Feedback: this finding is the feedback requirement.

### C2-10 Per-push and per-row churn in sessions list (low)

- Where: `sessions/AdeSessionsTab.vue:33-44` (running/stopped computed recreate every `SessionView`
  per sessions push; panes keyed by session id at 118-119); `sessions/AdeSessionsRow.vue:37` (own
  `useTimeAgo` interval per row) and `useMarkSessionSeen.ts:16-17` (`useDocumentVisibility` +
  `useWindowFocus` listeners per row); `state/adeTerminals.ts:21-40` (watch getter builds new
  Set/array each run, fires on every push); `sessions/useSessionViews.ts:13-27` (lookup Map rebuilt
  per caller).
- Slow: small; scales with session count. Not felt below ~20 sessions.
- Fix: wrap visibility/focus in `createSharedComposable`; one shared `useNow({ interval: 30_000 })`
  feeding `formatRelative` instead of per-row timers; reuse unchanged `SessionView` objects by id;
  compare ids in `adeTerminals` watch before acting.
- Effort S. Risk low.
- Feedback: none.

## Minor, not ranked

- `agents/useAgentFileEditor.ts:32`: `bytes` computed runs `TextEncoder` per keystroke. Cheap for
  agent files; leave unless file sizes grow.
- `readSignals.ts:27-54`: `onSessions` invalidates sessions + `panelForward` + `runSandbox`; every
  session push refetches task panel forward data (cross C1 / Stream B: narrow by changed task).
- Backend `internal/approvals/queue.go:176-199`: `Queue.Answer` calls `owner.Answer` synchronously
  and serially; slow owners (`task.create`, `branch.request`) hold approval spinner and block next
  answer. Cross Stream B.

## Nothing real

- Approvals surface: Monaco preloaded when items arrive (`ApprovalsSurface.vue:42-47`), answer
  begin/done/fail wired, 1s countdown per card fine. Only deny-all feedback (C2-9).
- Agents page and leases pane: no rendering cost worth fixing; feedback only (C2-9).
- Monaco: lazy via `loadMonaco`; `AdePortExportDialog` `MonacoDiff` lazy inside. Fine.
- Proofs: inline diagram steps lazy-import mermaid (`diagramRender.ts`), one step query each; proof
  window loads per tab. Nothing beyond C2-9 open feedback.
- Terminal output ordering (`orderedWrites.ts`), drain limit 256KB: fine.
- `usePlanModel` already `createSharedComposable` with phase-key guard: no per-hook rebuild.
- Workflow graph already lazy (`AdeWorkflowEditor.vue:17`).
