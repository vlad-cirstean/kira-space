# Review round 1: frontend (P624)

Base commit: `82cffb85`. Scope: `git diff 82cffb85..HEAD -- apps/kira-space/frontend packages apps/kira-space/tests`
(reviewed at `35a6bbf5`). Discovery via `codegraph_explore` plus Read/Grep; bundle checked with a scratch
`vite build`.

Checked and clean: `adeOps` kinds, i18n keys and toast policy; `slowNotice`; mirror kinds and fields
against P623 contract (`agent`/`agentApproval|agentBranches|agentRename|agentChange`, `mcp` +
module `ade` `fetch_remote`/`push_branch`, `subject`, `taskId`, `durationMs`, `error`); hydration never
replays toasts; `Pair.checking`, `RepoState.updating` JSON names; board payload names `refs`/`taskIds`;
optimistic patchers stay immutable under `shallow: true`; `cachedProgress` facts cover every branch
field `buildTaskProgress` reads; `reuseModels` never stale; selection and ripple out of model;
`topologyKey` covers every field `layoutWorkflow` reads; terminal resize guard keeps the settled size
(drag-back cancels a pending flush correctly); run log cap drops whole chunks (ANSI parsed per line,
no carried state); `useSharedNow`/presence composables dispose with last subscriber; AdeNoteBody
`v-html` is schema-serialized (TipTap 3 Link sanitizes href).

## F1 (Medium) Refetch gate joins a fetch it did not start, losing a push

`apps/kira-space/frontend/src/ade/v2/refetchGate.ts:18`. `invalidateQueries(..., { cancelRefetch: false })`
returns the in-flight promise when any fetch of the key already runs (query-core `Query.fetch`
dedupes). Other paths start board fetches: mutation `onSettled` invalidations
(`queries.ts:443`, `queries.ts:518`), optimistic `onError`, mount. Scenario: a mutation settles and
starts fetch F1; backend reads its snapshot; a background tail (conflict check, run state, refresh tail)
changes the board and pushes while F1 is in flight; gate is idle, so `pump` joins F1 and sets no dirty
flag; F1 lands pre-change data; board stays stale until some unrelated next push. Before P624 every
push used default `cancelRefetch: true`, which restarted F1. Unit and UI specs only drive gate-owned
fetches, so they miss it.
Fix: before each invalidate, if `queryClient.getQueryState(queryKey)?.fetchStatus === 'fetching'` and
the fetch is not the gate's own, await it (query `promise`, errors swallowed) and keep `dirty = true`
so one gate-owned fetch follows. Add a unit case: external fetch in flight, signal, expect a second fetch.

## F2 (Medium) Needs-page Approve lost its stale-board repo retry

`apps/kira-space/frontend/src/ade/v2/needs/useNeedsAction.ts:60`. `gated(taskId, needsRepo, () => op(...))`
puts `ops.run` inside `retryAfterRepo`. `ops.run` catches every error and resolves false, so
`retryAfterRepo` never sees `E_TASK_NO_REPO`: the repo picker no longer opens when the bridge refuses an
approve the board thought fine; user gets an error toast with the raw refusal instead. `useTaskAction`
nests it correctly (`run/useTaskAction.ts:37`).
Fix: `if (await ensureRepo(n.taskId, needsRepo)) await op('approve', n, () => retryAfterRepo(n.taskId, () => approve.mutateAsync(...)))`.

## F3 (Medium) Force push from the panel's branch list has no running feedback

`apps/kira-space/frontend/src/ade/v2/panel/AdeMainBranches.vue:116` passes `taskId: ''`. These rows are
`props.card.rows`, the task's own branches, not main branches. With no `taskId`, `AdeOpStatus`
(`forBranch(row.branch.taskId, row.id)`) and the card chip never match the op: after confirm the dialog
closes and nothing shows until the 2 s toast. Breaks the cross-cutting running-feedback requirement and
plan commit 8 ("add `taskId` at the three render sites"). The `ForcePushTarget` comment ("empty for a
main branch") is wrong for this site.
Fix: `taskId: row.branch.taskId` (or `card.task.id`); correct the comment.

## F4 (Medium) Dialogs close before a synchronous refusal, dropping typed input

- `run/AdeRunDialog.vue:141`: `close()` runs before `start`; a refusal (branch name taken, invalid
  ref, setup refusal) loses per-branch names and an edited message. `ade-run-error` inline alert removed.
- `state/adeStageDialog.ts:135`: move or pass refusal (stale `fromStepId`, step guard) loses typed
  handover notes; `open()` resets `typed` on reopen.
- `dialog/flow.ts:132-141`: rebase refusal (dirty worktree without autostash, bad `onto`) used to stay
  in the dialog; now dialog closes, options and message are gone. `Rebase` returns once accepted (runs
  in background), so wrapping it gains no latency.
Plan accepted "closes at once", but the refusal path was not designed: user must reopen and retype.
Fix: keep each dialog's inputs in its store and, on `ops.run` false, reopen with them (Run, stage);
for rebase, await acceptance in the dialog (fast) and start the op chip only for the background run,
or reopen with state on refusal.

## F5 (Low) Op chip and slow toast run while the repo picker waits for the user

`run/useTaskAction.ts:37`, `run/AdeRunDialog.vue:142-145`: `ops.run` wraps `retryAfterRepo`, whose
`gate.ask` opens `AdeRepoGateDialog` on `E_TASK_NO_REPO`. While the user picks, the card shows
"Starting run" and after 2 s a loading toast appears; Cancel yields an error toast with the raw
`E_TASK_NO_REPO` text. Fix: let `retryAfterRepo` run outside the op (catch `E_TASK_NO_REPO`, ask, then
start a fresh `ops.run`), or treat a cancelled gate as a silent settle.

## F6 (Low) Second action on another step or run of the same task is silently dropped

`panel/AdeStepBlock.vue:173` (and `useNeedsAction.ts` `op`): op ids are `<kind>:<taskId>`, while
Approve, Retry and Resume target a step or run. Clicking Retry on run B while run A's retry is in flight
returns false from `ops.run`; buttons of other runs are not disabled and nothing is shown. Fix: key the
id by run or step (`retry:<runId>`, `approve:<taskId>:<stepId>`) or disable sibling buttons via
`ops.forTask`.

## F7 (Low) Empty `taskIds` read as "no task" while backend doc says "any task"

`ade/v2/boardEvent.ts:14`. Go `adewire.BoardChangedEvent` doc: "TaskIDs empty means any task may have
changed". Coalescer never emits `[]` today, but a future `taskChanged()` with no ids would skip every
panel refetch. Fix: `Array.isArray(raw?.taskIds) && raw.taskIds.length > 0 ? raw.taskIds : null`.

## F8 (Low) History bar misses a one-frame jump across the whole viewport

`plan/AdePlanView.vue:315`. IntersectionObserver notifies only when intersection state changes. With
today's band above the line (scrolled into the future), `revealDay(k<0)` (Go to date, find archived)
jumps it straight below the viewport: not intersecting before and after, no callback, `inHistory` stays
false, the back-to-today bar is missing. Old computed re-read on every `scrollY`. Fix: after
programmatic scrolls (`scrollToDay`, `openHistory`, `hideHistory`) recompute `todayBelowLine` once from
`getBoundingClientRect`, or use thresholds `[0, 1]` plus that check.

## F9 (Low) Lazy-loading claims overstate the bundle effect

- `notes/asyncEditor.ts:5` says "the entry chunk carries no ProseMirror". Scratch build: entry
  `index-*.js` (3.40 MB) still holds `prosemirror-view`, `taskList`, StarterKit and the Markdown
  tokenizer, pulled by `AdeNoteBody.vue` (`generateHTML` + `notesExtensions()` + `notesDoc`). Only
  `@tiptap/vue-3` and the editor component split. SPEC Result names `@tiptap/core` only.
- `shell/AdeShell.vue:29` lazy `AdeStageDialog` is ineffective: Vite warns
  `INEFFECTIVE_DYNAMIC_IMPORT`, `stage/AdeHandoverPrompt.vue:7` imports it statically.
Fix: correct the comment and SPEC Result; import `AdeStageDialog` async in `AdeHandoverPrompt` too, or
drop the dead dynamic import.
