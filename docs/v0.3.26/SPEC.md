# v0.3.26

Chapter v0.3.26, branch `chapter/v0.3.26`, tag `v0.3.26`. First phase `P756` (highest phase: `P755`,
in `docs/history/07-v0.3.15.md`, chapter v0.3.25).

Code review at chapter end: 1 round.

| # | Phase | Status |
|---|-------|--------|
| P756 | Revert v0.3.25 P754: checkboxes and toggles back to the neutral on state | Done |
| P757 | Task shows merged on main wrongly: own-commit count, recorded merge tip, PR head match | Done |
| P758 | Task Details opens slowly: non-blocking Artifacts, cached facts, no skeleton on refetch | Done |
| P759 | Remove the proof ready dialog | Done |
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

Result: migration `0109_p757_merged_tip.sql` adds `merged_tip`, drops live recorded merges, recounts
`had_commits` of stacked and non-main-based live rows. A branch reads merged iff it has own commits
(latched from `Ahead` against its resolved base, never main) and its tip is in main, or a recorded
merge holds at exactly the current tip. Merged PR records only when its head is the local tip or
descends from it. A build clears a record the tip moved past (compare-and-clear). Rebase stack,
rebase refusal, archive risk and quick artifacts read the tip too. Flow test `TestMergedIntoMain`
(6 cases, contract `ade-merged`) and UI spec `ade-v2-merged.spec.ts` (6 `contract:` tests) pass. Checks:
storage and `ade` with `-race`, all `internal/flows` (one `TestSmartRunSandboxed` timeout under
load, passes alone), go vet, golangci-lint (extracted `mergedFacts` from `computeBranch` for
gocyclo), typecheck, lint, knip, `TestSchemaSnapshot` with regenerated golden and `kiraschema`, `ui`
on ade-v2-merged, ade-v2-base-rebase, ade-v2-main-branches. No visual baseline changed.

## P758

Opening task Details is slow although Jira fetching is gone. Findings and fix list:
`plans/audit-details-open.md`. Plan picks the fixes in priority order a to f; confirm by timing the
`AdeTask.Artifacts` bridge call. Real latency on the owner's machine is unmeasured.

Result: plan `plans/P758.md`. (a) UI `Artifacts` never waits on the host: change requests come from
`CachedBranchChangeRequest` (any age), else the stored row; one tracked job per task (`ade/artifacthost.go`)
resolves the host, and pushes `{refs: true, taskIds: [id]}` only when its share differs from the served
one. The host gap shows from the last lookup kind (memory). (b) Branch facts come from `boardLatest()`;
`repoShare` is the pure builder; facts decide status when present. (c) No host or git wait is left in
the call; the links skeleton waits `SPINNER_DELAY`. (d) Details stays mounted after the first visit,
disk query paused while hidden. (e) Link, note, handover writes push `taskChanged`; Rescan names the idle
live tasks. (f) `DiskChanges` `TryLock` with a last-result cache dropped by every folder write.
`ade artifacts: slow` Info log over 250 ms. Measured with the fake host at 500 ms per lookup: cold
call 0.5 s, 1.5 s, 3.5 s at 1, 8, 24 live branches before; 4 to 5 ms after; warm 10 to 34 ms before, 0.25
to 1.1 ms after (table in the plan). Flow `TestArtifactsOpen`, `TestDetailsScopedPushes`, in-package
`TestDiskChangesDuringTaskOp`; UI `ade-v2-artifacts-open.spec.ts` and an AddNote case in
`ade-v2-details-autoupdate.spec.ts`; contracts `ade-artifacts-open`, `ade-details-autoupdate`. Steps
changed in `TestTaskArtifacts` (wait for the PR and the gap after the confirm push). Fixed
`TestTaskStartNeedsRepo` (-race timing). No visual baseline changed. Rescan push not covered by a flow
test (the harness cannot raise one). Real gh latency on the owner's machine stays unmeasured.

## P759

The proof ready dialog (`ProofReadyPrompt`) pops on screen and annoys the owner. Remove the dialog
and everything only it uses. Plan checks what else hangs off it (`proofReady` backend event,
`NotifyProofReady`, settings, i18n, specs, docs) and whether a quiet signal (task row badge, existing
notification) already tells the user a proof is ready; keep that, drop the popup.

Result: plan `plans/P759.md`. Removed the ready popup end to end: `ProofReadyPrompt.vue`, prompt kind
`proof`, `Events.Accepted`, `Notify`/`NotifyProofReady`, `RaiseReady`, `MarkSeen`, `seen_at` (migration
`0110_p759_drop_proof_seen.sql`, regenerated `kiraschema_gen.go` and schema golden), `Detail.TaskTitle`
and `proofMarkSeen`, i18n keys, specs. `RaiseUnreviewed` became `RaisePendingReviews` (pending edit
reviews only). New proof now emits `kira:proof:changed` from `unitApply`. Quiet signals stay: Details
chain and `proof.review`. Flow half in `TestProofFiles` (no prompt, proof changed), UI half in
`ade-v2-proof-chain.spec.ts`. Contracts lose `taskTitle`. Tests: touched Go packages with `-race`,
proofflow, promptflow, adeflow, `golangci-lint`, typecheck, lint, knip, own UI specs pass. No visual
baseline changed. Owner's desktop notification for a new proof is gone with the prompt.

## P760

Drop Kibana as a connection kind: adapter (`internal/adapters/kibana`), test fixtures
(`testsupport/kibana.go`), connection form, kind registry, docs, specs, guards. Keep the data
visualisation module Kibana support provided (time series and charts); it stays and must still work for
the remaining sources. Plan names the exact keep/drop line per file and symbol, and any stored
connection rows of kind kibana need a migration (patch release: allowed) that removes them.
