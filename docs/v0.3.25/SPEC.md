# v0.3.25

Chapter v0.3.25, branch `chapter/v0.3.25`, tag `v0.3.25`. First phase `P747` (highest phase: `P746`,
in `docs/history/07-v0.3.15.md`, chapter v0.3.24).

Code review at chapter end: 1 round.

| # | Phase | Status |
|---|-------|--------|
| P747 | Run restarted after app restart: fresh script timeout | Done |
| P748 | Proof tool: visible grid borders, title link to window, collapsed by default | Todo |
| P749 | Proof window: images fit the window, larger default size | Todo |
| P750 | Details panel and proofs: auto-update on disk changes | Todo |
| P751 | Chapter close | Todo |

## P747

A run restarted after an app restart fails at once with the script timeout. Restart counts as a new
run: reset its timeout clock (and any other elapsed-time state) so the full timeout applies again.
Plan finds where the original start time survives the restart.

Result: no stored start time feeds any timeout; every restore, resume and restart path arms a fresh
timer at launch. Fixed the two real defects that give a restart-shaped timeout. A: claude, opencode
and gitprepare runners read the parent ctx deadline as the run's own timeout; they now report timed
out only when the parent ctx is live (`&& ctx.Err() == nil`), else cancelled. B: a restore-issued
terminal launch token aged from the restore click and expired after `LaunchTTL` (60 s) before a later
tab opened; `Restore` tokens now live `RestoreLaunchTTL` (30 min), fresh `Start` keeps 60 s. Tests:
termflow `TestRunsRestoreRestartedSmartFreshTimeout`, `TestRunsStopUnderTimeoutIsCancelled`,
`TestRunsRestoreTerminalTokenOutlivesLaunchTTL`; adeflow `TestResumeAfterRestartFreshTimeout`; UI spec
"each restored terminal script opens with its own token" (all pass). ARCHITECTURE.md updated.

## P748

- Proof tool grids and rows get visible borders; today they are hard to tell apart.
- Restore the proofs title link in task details that opens the proof window.
- Proofs default to collapsed: only title and main description show. Reverses P736's auto-expand for
  the details panel; plan states what stays auto-expanded in the proof window.

## P749

- Screenshots in the proof window scale to fit the window so one view shows the whole image.
- Proof window default size grows a bit.

## P750

Details panel, and the proofs inside it, do not refresh when files change on disk. Plan traces the
watch and invalidation path (file watcher, bridge event, TanStack Query) and fixes the broken link.
Flow test asserts the event, UI spec asserts the refresh.

## P751

Chapter close, by hand per `CLAUDE.md` "Closing a chapter".
