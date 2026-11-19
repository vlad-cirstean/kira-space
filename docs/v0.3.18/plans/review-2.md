# v0.3.18 code review, round 2

Base: `f59b9b4fc` (tag `v0.3.17`), same base as round 1. Tip: `9f5887f47`. Diff: full
`f59b9b4fc..9f5887f47`; round 1 fixes (`27b14d807..9f5887f47`) are unreviewed code and were read first.

Scope: round 1 fixes (`keptvars.Meta.Bound`, `agentapi.KeptUse.Bound`, `keptRun.readsSecret`, `noteBound`,
`commit` mixed-origins refusal, `apiReviewFor`, `query_script.go`, `useKeptReveal`, `ConfigSecretsSection`,
`gcTime: 0` on secret mutations). Then frontend P682-P685, P687-P689 (`asyncAction.ts` and its callers,
`queryClient.ts` network mode, Studio view-store dedup, `logFilter.ts`, `guardedDraft.ts`,
`useConfigFormDraft.ts`, `ConfigFileDialog.vue`, ADE draft callers, `confirmDialog.ts`, copy joins) and the
P686 flow and UI e2e tests.

Discovery: `codegraph_explore` on the worktree answers "not indexed" (no `.codegraph/`); every file below was
read from worktree source. TanStack semantics checked in `@tanstack/query-core@5.105.1`
`mutationObserver.js` (`mutate` detaches the observer from the previous mutation; listener calls are
synchronous) and `@tanstack/vue-query` `useMutation.js`.

Counts: High 0, Medium 3, Low 3.

## Round 1 fix (F1 laundering): verdict

The fix closes the reported chain. `kept.set` in a run that read a bound value stores `Secret`, the read
entry's `Origin` and kind `script`; `admit` refuses it for another origin, `UsesUserValue` pins redirects,
`readsSecret` forces review of any transform reading a secret or bound value, `query_script.go` and
`kept_params.go` (secret check) refuse it in SQL, and a run reading two origins keeps nothing. Checked: name
reuse (`Put` refuses kind change, so neither `pw` nor a user name can be overwritten), partial `Put` failure
(each saved value carries the origin), set-before-get ordering (commit reads the final taint), pending
inputs (`rebind` runs before `deliverAPI` reads `readsSecret`), DB result scripts (cannot read secrets, so no
bound value is derived there), the staged review view (no preview when tainted), agent result (summary only),
op log (template URL). A bound value is always secret, so the prepare-stage secret refusal also covers it.
Two gaps remain around derived values: F1 and F4.

## F1 (Medium) Object and list secret kept values are masked only as one JSON literal: a reshaped leaf of a derived user value reaches the agent unreviewed

Files: `apps/kira-space/internal/keptvars/forms.go:16` (`SecretText`: non-string values as their whole JSON
text), `:58` (`SecretPairs`), `:85` (`SecretRaw`); `apps/kira-space/internal/agentapi/send.go:202`
(`newMask`); `apps/kira-space/internal/agentapi/kept.go:468` (`secretKeptTexts`);
`apps/kira-space/internal/dbmcp/kept_script.go:230` (`noteSecret`, same rule for transform masking).

Round 1 fix makes derived values usable as bound secrets, so this mask gap is now reachable in the laundering
chain round 1 targeted (its point 4), through a server echo instead of a transform:

1. Request A's transform runs `kept.set("d", {a: kept.get("pw").split("").reverse().join("")}); return 1;`.
   The forced review shows `1` and "Will keep d" (object, no preview). The user sends it. `d` is stored
   secret, bound to `pw`'s origin.
2. Request B: `POST https://bank.example/anything` with JSON body `{{kept.d}}` and no script. `admit` passes
   (same origin). No forced review: `readsSecret` applies to script reads only.
3. The server echoes request fields (a validation error, an echo route). The response mask holds `pw`'s
   forms and `d`'s whole compact JSON `{"a":"..."}`. The echoed leaf `"a": "drowssap"` (or a re-serialised
   object with other spacing) matches neither, so the agent receives the reversed password. With review
   settings off or an allow rule, no card is shown at any step after the first.

The same holds for a list (`["drowssap"]`) and for any leaf a server reformats.

Fix: mask the leaves of a secret array or object. In `keptvars`, build forms from every string leaf (and
number leaf) of a non-string secret value in `SecretPairs`, `SecretRaw` and `keptRun.noteSecret`, under the
same short-value rule as strings. Alternative if leaf masking over-masks: force response review
(`apiReviewFor`) when the request inserts a bound non-string value. Add a flow step to
`e2e_kept_derived_test.go` (object derived from `pw`, echoed by `/echo`, `noLeak`).

## F2 (Medium) `useTaskAction` surfaces fed by several writes lose an earlier write's failure and pending state

Files: `packages/workbench/src/behaviour/asyncAction.ts:49-60` (one `useMutation` per action; `run` calls
`mutateAsync`), `:75-79` (`useTaskAction`). Callers where distinct writes overlap:
`apps/kira-space/frontend/src/ade/v2/later/AdeLaterPage.vue:20,56-57` with `AdeShelfSection.vue:23` (shelve
per card), `apps/kira-space/frontend/mobile/screens/LaterScreen.vue:23,80-81` (same on the phone),
`apps/kira-space/frontend/src/ade/v2/panel/AdeMainTab.vue:47` (task patches),
`apps/kira-space/frontend/src/studio/api/state/collections.ts:122` and `studio/api/state/variables.ts`
(one surface for every tab).

`MutationObserver.mutate` detaches the observer from the running mutation before starting the next one. The
P682 plan chose this "latest run only" rule for one action repeated; `useTaskAction` uses it for different
writes that do not supersede each other.

- Lost error: on the phone, offline, the user shelves card A, then card B (both paused by the remote
  profile's `networkMode: 'online'`). On reconnect A fails (E_CONFLICT, gone task) and B succeeds. The
  surface observes B only: no error shows, `run` returns `false` to an `@run` handler nobody reads. Before
  P682 `AdeShelfSection` emitted the error text and the page showed it. Same on desktop for two quick task
  patches or two collection edits in different tabs.
- Pending: `pending` drops when the newest run settles while an older one still runs, so a caller that
  guards on it (busy buttons) accepts a second submit of the older write.

Fix: let `useTaskAction` track every run of its surface, not only the newest. For example give each action
a unique `mutationKey`, derive `pending` from `useIsMutating({ mutationKey })` and the error from the newest
failed mutation in `useMutationState({ filters: { mutationKey } })` (cleared by `reset`), keeping
`gcTime: 0`. Or, smaller: in `run`'s catch, record the failure with `fail(err)` when the observer has moved to
a newer run. Add a guard spec for two overlapping runs where the older fails.

## F3 (Medium) Typed connection password or URI kept in TanStack mutation state

Files: `apps/kira-space/frontend/src/studio/state/connectionConfig.ts:82-91` (`useSetConnectionSecret`: no
`gcTime: 0`), `apps/kira-space/frontend/src/studio/project/ConnectionFileDialog.vue:68` (`flushSecrets`:
`mutateAsync` with no `reset`).

Round 1's F2 fix reset or dropped the secret-bearing mutations of API secrets, ADE settings secrets,
`ApiVariablesDialog`, `ConfigSecretsSection` and `CredentialsUpdateDialog`. The connection file dialog's
secret write was missed: after Save, the typed database password or connection URI (which embeds the
password) stays as the observed mutation's `variables` while the dialog is open, and for the default 5 min
`gcTime` after it closes. It is visible in the mutation cache and devtools, the exposure round 1 fixed for
the other secret writes.

Fix: same as the round 1 siblings. `gcTime: 0` on `useSetConnectionSecret` and
`try { await setSecret.mutateAsync(...) } finally { setSecret.reset(); }` in `flushSecrets`.

## F4 (Low) A derived bound value outlives the user value it came from

Files: `apps/kira-space/internal/dbmcp/kept_script.go:339` (`commit` passes no `TTL`),
`apps/kira-space/internal/keptvars/store.go:174` (`ttlFor`: fresh retention at every `Put`).

ARCHITECTURE: kept expiry "is fixed at Put ... Never refreshed." A transform that reads `pw` and keeps
`kept.set("d", kept.get("pw"))` stores `d` with a full new retention (up to 240 min). Repeating the copy in a
later run (each copy is reviewed, but shows only "Will keep d") keeps the user's password alive past the
retention the user chose. Clearing or replacing `pw` in Settings leaves `d` with the old password, still
sendable to the origin and still masked only by its own forms.

Fix: in `noteSecret`/`noteBound` record the earliest `ExpiresAt` of the secret entries the run read, and pass
`TTL: time.Until(min)` to `Put` in `commit`, so a derived value never outlives its source. Mention in the
ARCHITECTURE kept section that clearing a user value does not clear values derived from it, or clear them
(entries whose `Origin` matches and whose source run read it).

## F5 (Low) Config dialog Save can start two writes on the same base hash

Files: `packages/workbench/src/behaviour/guardedDraft.ts:117-122` (`save` checks `saving`, then awaits
`beforeSave`, then runs), `packages/workbench/src/config/useConfigFormDraft.ts:116-122` (`beforeSave` awaits
`applyModel`, a bridge call), `packages/workbench/src/config/ConfigFileDialog.vue:107` (Ctrl+Enter) and the
Save button (`canSave` reads `saving` only).

On the form tab, `saving` stays false while `applyModel` runs. A double click on Save, or Save then
Ctrl+Enter, passes the guard twice; both runs write the same draft on the same `baseHash`. The first wins,
the second gets E_CONFLICT, and since the observer follows the newest run the dialog shows the conflict
banner (Reload, Overwrite) for the user's own save. The window existed in the old `saveNow`; P687 moved it
into the shared draft used by the API variables, connection and run file dialogs.

Fix: set a local `starting` flag synchronously at the top of `save()` (cleared in `finally`) and include it
in `saving`, or run `beforeSave` inside the action thunk so `pending` covers it.

## F6 (Low) A reveal that settles after the view is gone still holds the plaintext

File: `apps/kira-space/frontend/src/studio/kept/useKeptReveal.ts:22-25,44`.

`onScopeDispose(hide)` clears `value` and resets the action, but a reveal in flight (OS gate prompt, then the
confirm dialog, which is a global store and stays open after the view unmounts) still calls `onRevealed`:
`value.value = v` on the detached ref and `timer.start()`, holding the plaintext for 60 s with no view. The
doc comment says it hides "when the view goes away". `hide()` during a pending reveal has the same effect.

Fix: keep a generation counter bumped by `hide()`; `onRevealed` ignores a result from an older generation.

## Checked, no finding

- `asyncAction.ts`: thunk variable keeps typed values out of `variables`; `gcTime: 0`; `networkMode:
  'always'` on the wrapper with inner writes on the client profile (phone pending still shows); `pending`
  updates synchronously (listener is called inside `notifyManager.batch`), so single-run double-click guards
  (`NewConnectionDialog`, `useInlineEdit`) hold. `useFileEdit` `fail()` paths.
- `queryClient.ts`: desktop `always` with no reconnect refetch, phone `online`; approvals and agent info read
  `useQueryClient()` so the phone uses its own profile; `useAnswerApprovals` `gcTime: 0` plus `reset`.
- Round 1 frontend fixes: `useKeptReveal` thunk resolves to nothing and `hide()` resets; `.finally(reset)`
  passes the rejection to `ConfigSecretsSection`'s action; `ApiVariablesDialog` resets in `finally`.
- P683: `createRuntimeStore` `onClose` bodies match every removed `registerTabRuntimeCleanup` (HTTP and gRPC
  stop the op, console ends the transaction and cancels, keyvalue drops `::preview`, browse drops pending key
  types, grid clears cell focus); `runPagedLoad` owns `beginOp` and the runtime-presence check;
  `applyPagePosition` field guards match each runtime; `restart` keeps the old reset-tokens, patch, page 0
  order; `setFilter` still clears the count.
- P684 `useLogFilter`: per-line `WeakMap` cache on immutable line objects, filter key per container or pod,
  copy uses `shown`. Docker log reopen is latest-wins by design (old stream errors dropped).
- P687 ADE drafts: workflow graph and step form share store state but mount one at a time; a dirty draft
  ignores pushes as before; agent file and YAML pane drafts.
- `confirmDialog.ts`: same behaviour as before; `queryClient` passed for a store built before mount.
- P688 `joinSentences` joins whole translated sentences only.
- P686 tests: no `time.Sleep` or `waitForTimeout`; waits poll with deadlines (`testx.WaitUntil`, 5 ms);
  client timeouts scale under `-race`; no `t.Parallel`; async callers are awaited or cancelled by `defer`;
  `captureLog` restores the default logger.
