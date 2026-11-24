# v0.3.25

Chapter v0.3.25, branch `chapter/v0.3.25`, tag `v0.3.25`. First phase `P747` (highest phase: `P746`,
in `docs/history/07-v0.3.15.md`, chapter v0.3.24).

Code review at chapter end: 1 round.

| # | Phase | Status |
|---|-------|--------|
| P747 | Run restarted after app restart: fresh script timeout | Done |
| P748 | Proof tool: visible grid borders, title link to window, collapsed by default | Done |
| P749 | Proof window: images fit the window, larger default size | Done |
| P750 | Details panel and proofs: auto-update on disk changes | Done |
| P751 | Drop issue and change request viewer windows and acli support | Todo |
| P752 | Task detail: title no longer overlaps the colored bar | Done |
| P753 | Chapter close | Todo |

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

Result: proof window grids get a stronger line colour (`--kira-grid-line`, `lined` prop on the console
grids); the proof name in task details is the link that opens the window again; each proof entry is a
shadcn-vue `Collapsible`, collapsed by default (title and summary show; steps and diagrams unmount),
with the state kept for the session. The proof window keeps P736's auto-expand. UI specs only; baseline
updates left to PR CI. Plan: `plans/P748.md`.

## P749

- Screenshots in the proof window scale to fit the window so one view shows the whole image.
- Proof window default size grows a bit.

Result: proof images scale down in CSS to fit the step pane whole (aspect kept, never upscaled, no
scroll), so a window screenshot shows the full image. Default proof window grows from 1024x640 to
1152x720; `shell.FitSize` shrinks it on small screens (floor 1024x640) and `shell.CascadeFromSized`
checks the cascade wrap at the proof size. Plan: `plans/P749.md`.

## P750

Details panel, and the proofs inside it, do not refresh when files change on disk. Plan traces the
watch and invalidation path (file watcher, bridge event, TanStack Query) and fixes the broken link.
Flow test asserts the event, UI spec asserts the refresh.

Result: two broken links fixed. Backend: nothing watched a task folder unless an agent ran on it; an idle
watcher on the task files root now pushes the board for any save, skipping tasks with a live agent
watcher. Frontend: a board push never refreshed proofs; it now invalidates the task proofs and open
proof steps (an accepted edit keeps `rev`, so steps are invalidated by prefix). Flow tests plus a UI
spec under contract `ade-details-autoupdate`. Open: an open proof window can still go stale after an
accepted agent edit (no `kira:proof:changed`). Plan: `plans/P750.md`.

## P751

Drop the in-app issue viewer and the in-app change request viewer, and drop acli support. Their
implementation is not what the owner wants.

- Viewers: the windows that show a Jira issue or a GitHub pull request. A link to either opens in the
  browser directly, like any other link. The same applies to Azure DevOps if it has such a viewer.
- Stays: the change request tracker inside the task (`gh`-driven state, review state, updates).
- acli: remove the Jira provider that drives `acli`, its settings, UI, docs and guard G5's `issuetracker/`
  coverage that only it needs. Plan lists every dependent first (task issue line, updates, add-by-key
  popover, `parseJira`) and states what each becomes: a plain stored link, never half-removed.
- Stored data and settings for removed features: plan states the cleanup; no legacy code across a
  minor bump, but this is a patch, so a migration step must keep existing installs clean.
- Run after P750 lands: it touches task link and detail code that P748 and P750 also touch.

## P752

The task detail title overlaps the colored bar right after it. Fix the layout so they never overlap,
at every title length and window width. Plan finds the cause (fixed height, missing wrap or clamp,
negative margin) and the fix in Tailwind utilities. UI spec asserts no overlap with a long title.

Result: the colored line is absolutely positioned and took no space; the header's 3px bottom padding
equalled its height, so the title sat flush on it, and the fixed-height title input overflowed at
larger app fonts. The header wrapper now reserves the line's space, task and branch headers pad 4px,
and the task title input uses a new `fluid` size (min height, ellipsis). UI spec measures the gap
at two widths and two font sizes. Plan: `plans/P752.md`.

## P753

Chapter close, by hand per `CLAUDE.md` "Closing a chapter".
