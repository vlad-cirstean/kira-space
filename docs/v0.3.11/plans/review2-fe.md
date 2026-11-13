# Review round 2: frontend (P624)

Base commit: `82cffb85`. Reviewed head: `535862a6`. Scope: `git diff 82cffb85..HEAD -- apps/kira-space/frontend
packages apps/kira-space/tests`; focus on round-1 fixes (`git log 1a31d9dd..HEAD`). Discovery via
`codegraph_explore` (index stale for `wire.ts`, `actions.ts`, `rebasecheck.go`; those read directly) plus Grep/Read.

Checked and clean: refetch gate (waits a foreign fetch via `Query.promise`, query-core 5.105.1; a superseded
foreign fetch rejects into the catch and loops; no board polling, so no starvation; installed once per app, no
unmount); `OpAborted` settles the notice (dismisses a shown loading toast), deletes the op, leaves `actionError`
clear; F2 nesting fixed; F3 `taskId: row.branch.taskId`; F7 empty `taskIds` means all; F8 sync runs once per
programmatic scroll only (instant `scrollTo`, no `scroll-behavior: smooth`), no per-scroll layout read; F9
stage dialog lazy in both mounts, `open` partitions by `routed` so it never shows twice; rebase dialog awaits
acceptance, `pending` removed in `finally`, refusal inline; `rebase` kind gone from type, policy, texts and
catalog; op kinds, `Pair.checkFailed`, `Pair.checking`, `RepoState.updating` match Go JSON names and the
P622/P623 tables; mirror kinds match `agentops.go`.

## N1 (Medium) A late refusal reopens a modal unprompted, over another dialog, or with no mount

- `run/AdeRunDialog.vue:153-157`: `send` reopens the Run dialog whenever `ops.run` resolves false. The op runs
  in the background (worktree prep; the slow toast exists because it passes 2 s), so a failure 10 s later pops
  the modal over whatever the user does now. Only `ui.runTaskId` is checked: a stage, rebase, take-over or repo
  gate dialog opened meanwhile gets the Run dialog stacked on it. If the user left ADE mode, `AdeRunDialog`
  unmounted (`WorkbenchShell` `v-if`); the continuation still sets `ui.runTaskId`, so returning to ADE opens a
  Run dialog out of nowhere, and the dead instance's `kept` is lost, so it opens blank anyway.
- `state/adeStageDialog.ts:164-176`: same for a move. A routed `handover` accepted from another mode (the
  prompt host is app-global, `App.vue` `promptKinds`) reopens with `routed = false`, whose only mount is
  `AdeShell`: nothing shows, `isOpen` stays true, and the dialog appears on the next visit to ADE.
- Also both reopen on a dedupe `false` (same op id live), with no toast: the second submit is dropped silently
  and the dialog pops back.
Fix: reopen only when the refusal lands promptly (e.g. within `TIMING.slowOp`) and no other ADE dialog is open
and the mount is alive; otherwise keep the typed input in the store and offer it on the next open of that task
(or show it from the error toast's action). Distinguish dedupe from failure (`ops.run` returning a reason) and
skip the reopen on dedupe.

## N2 (Low) Routed handover keeps the prompt host busy, invisibly, for the whole move

`stage/AdeStageDialog.vue:52`: `move()` emits `close` (hiding the routed prompt) only after `store.submit()`
awaits the whole op. The dialog closed at once, so the host shows nothing while it still holds the `handover`
entry: a queued routed prompt (another handover, proof, git credential) waits unseen until `SetTaskStep` /
`StepDone` / `NextPass` returns. Fix: emit `close` once `submit` has captured its inputs (have `submit` return
after `close()` and run the op detached), keeping the reopen path from N1.

## N3 (Low) Retry and resume op ids differ per surface, so cross-surface dedupe is lost; empty `runIds` collide

Round-1 F6 fix keys ids per surface: card `retry:<taskId>:<stepId>` (`run/useTaskAction.ts:70`), panel
`retry:<runId>` (`panel/AdeStepBlock.vue:188-190`), Needs `retry:<runIds joined>` (`needs/useNeedsAction.ts:53-57`).
The same failed run retried from the panel while the Needs item's op runs starts a second op (two chips; backend
singleflight joins the call, so no double run). A stuck run with no `runId` (`board/needsYou.ts` `runIds: []`)
gives id `retry:` / `resume:`, shared by every task. Fix: one id helper per target (`retry:<runId>` per run;
for a step, the sorted run ids, falling back to `<taskId>:<stepId>`), used by all three surfaces.

## N4 (Low) A failed pair check is hidden behind `sharesFiles`, `behindMain` and `notPushed`

`board/actions.ts:783-789`: `checkFailed` ranks below `notPushed`, `behindMain` and `sharesFiles`. Go fills
`Shared` independently of `Failed` (`board_facts.go:315`), so a pair with shared files and a failed check shows
"After X" ("merges after it"), which reads as resolved, never "unknown". Commit `8db80ddc` says a failed pair
reads as unknown, not clean or conflict. Fix: rank the pair-failed branch of `checkFailed` above `sharesFiles`
(below `conflict`/`rebaseConflicts`), or append the unknown note to the `sharesFiles` tooltip; add a spec case
with shared files.

## N5 (Low) New specs overclaim what they assert

- `tests/ui/ade-v2-pair-check.spec.ts:14`: titled "on both branches" but checks only `.first()`. Assert the
  tag on both branch ids of the pair.
- `tests/ui/ade-v2-repo-required.spec.ts:122-124`, `tests/ui/ade-v2-needs.spec.ts:159`: "no loading toast while
  the prompt waits" is checked instantly; the slow toast appears only after 2 s, so these pass even if the op
  still ran. Hold the repo gate past `TIMING.slowOp` (Playwright `page.clock`, already used in
  `consistency.spec.ts`) before asserting no toast.

## N6 (Low) SPEC P624 Result misses round-1 frontend behavior

`docs/v0.3.11/SPEC.md:40` does not mention the `checkFailed` unknown tag (new contract consumption), per-target
op ids, the repo prompt outside the op (`OpAborted`), refusal reopen of Run/stage dialogs, or rebase leaving
`adeOps` (it now awaits acceptance in the dialog). Fix: add one sentence on the round-1 changes.
