# v0.3.26

Chapter v0.3.26, branch `chapter/v0.3.26`, tag `v0.3.26`. First phase `P756` (highest phase: `P755`,
in `docs/history/07-v0.3.15.md`, chapter v0.3.25).

Code review at chapter end: 1 round.

| # | Phase | Status |
|---|-------|--------|
| P756 | Revert v0.3.25 P754: checkboxes and toggles back to the neutral on state | Done |
| P757 | Task shows merged on main wrongly: own-commit count, recorded merge tip, PR head match | Todo |
| P758 | Task Details opens slowly: non-blocking Artifacts, cached facts, no skeleton on refetch | Todo |
| P759 | Remove the proof ready dialog | Todo |
| P760 | Remove Kibana from connections, keep its time series and chart visualisation | Todo |
| P761 | Chapter close | Todo |

## P756

Owner dislikes the blue on state of checkboxes, toggles and the keep-awake button. Revert all of
v0.3.25 P754: `--kira-toggle-on` and `--kira-toggle-on-edge` tokens, primitives (Checkbox, Switch,
Toggle, ToggleGroup, SecondaryTabs and others P754 touched), guard N18, keep-awake blue icon and
status dot, UI specs, visual baselines, `ARCHITECTURE.md` text. Restore the pre-P754 neutral look
(`data-[state=on]:bg-select`). Land as a forward change, not a git revert of the squash. Keep every
other v0.3.25 change. Plan lists P754's file set from the `chapter/v0.3.25` history (commits
`28780367d`, `1aa4a0da2`, `f0112b77f`, `787bbbc98`, `6b2a08eeb`).

Result: forward commits `deb3c0f9f` (theme, guard N18, specs), `15fa0a691` (keep-awake), `a4ba64403`
(ARCHITECTURE), `5a8931951` (33 PNGs). Code files match pre-P754 tree; no `P754` string and no
`keep-awake-dot` left. Checks: typecheck, lint with guards, knip, conventions spec (44 pass), `visual`
on the 18 owning specs (51 pass after re-record), `ui` on selection-colour, window-chrome,
ade-v2-dialogs, control-sizing, automations-smart green. Re-recorded `ade-links-visual` and
`branch-panel-details-visual`: restored pre-P754 blobs carried stale P751/P748 drift (removed
`PAY-10` chip, etc.); new images are byte-equal to the P754 ones (no on-state control in shot, no blue
tint), so they show as unchanged against HEAD.

## P757

Tasks often show merged on main when they are not. Findings and fix list: `plans/audit-merged.md`.
Fix the three causes: `merged_at` never clears after new commits, branches with no own commits counted
against main (stacked or `develop`-based), PR merge matched by branch name only. Reset existing wrong
`had_commits` rows (migration). Each new case gets a backend flow test and a matching UI spec.

## P758

Opening task Details is slow although Jira fetching is gone. Findings and fix list:
`plans/audit-details-open.md`. Plan picks the fixes in priority order a to f; confirm by timing the
`AdeTask.Artifacts` bridge call. Real latency on the owner's machine is unmeasured.

## P759

The proof ready dialog (`ProofReadyPrompt`) pops on screen and annoys the owner. Remove the dialog
and everything only it uses. Plan checks what else hangs off it (`proofReady` backend event,
`NotifyProofReady`, settings, i18n, specs, docs) and whether a quiet signal (task row badge, existing
notification) already tells the user a proof is ready; keep that, drop the popup.

## P760

Drop Kibana as a connection kind: adapter (`internal/adapters/kibana`), test fixtures
(`testsupport/kibana.go`), connection form, kind registry, docs, specs, guards. Keep the data
visualisation module Kibana support provided (time series and charts); it stays and must still work for
the remaining sources. Plan names the exact keep/drop line per file and symbol, and any stored
connection rows of kind kibana need a migration (patch release: allowed) that removes them.
