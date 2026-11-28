# v0.3.27

Chapter v0.3.27, branch `chapter/v0.3.27`, tag `v0.3.27`. First phase `P762` (highest phase: `P761`,
in `docs/history/07-v0.3.15.md`, chapter v0.3.26).

Code review at chapter end: 1 round.

Batch (owner request): P762, P763, P764 run as three concurrent streams, see `plans/batch.md`.
P762 ships fully. P763 and P764 are redesigns: each stops at a plan plus a mockup for the owner to
review and iterate on; implementation follows the owner's sign-off.

| # | Phase | Status |
|---|-------|--------|
| P762 | Branches MCP cannot target a worktree when a repo has several branches | Done |
| P763 | Redesign links to show the host too (design for review, owner decision deferred) | Pending |
| P764 | Redesign task colours with a faint background, shared by task details and review window (design for review) | Done |
| P765 | Repo files window opens without a worktree and starts fully collapsed | Done |
| P766 | Unify colour components: one module owns palette, band, mark and selection ring so a later colour change is one edit | Pending |
| P767 | Apply the faint colour band to Studio, API, repos and every other coloured surface, with screenshots | Pending |
| P768 | Chapter close | Pending |

## P762

The branches MCP tool fails when one repo has more than one branch: the caller cannot name the
worktree. Add worktree selection to the tool, resolve it in the backend, cover with a flow test and a
UI spec split at the IPC boundary.

Result: forward commits `518dde91d` (`taskCandidates` lists each repo once), `bb16aba2a` (optional
`worktree` arg on `branch.rename`, `branch.change`, `worktree.remove`; `taskRow` resolver; tool wording
and `budget.tsv`), `fa0ebf454` (`TestAgentMultiWorktree`, contract `ade-agent-multi-worktree.json`, UI
spec `ade-v2-agent-multi-worktree`), `109bef5c8` (ARCHITECTURE). Checks: go tests for agentmcp,
llmprompts, ade, adeflow agent/rename/change tests (`-race -count=20` on the new one), checkprompts,
typecheck:tests, biome, UI specs 3/3, real `claude` `TestAdeBranchRequest` green.

## P763

Links list shows only a title or URL. Show the host too. Design first: mockup and plan, no
implementation until the owner approves.

## P764

Owner sign-off: Pure-B, 1.5px neutral selection ring, ripple dropped. Implement.

Task colours become a faint background tint. Task details and the review window use the same tint and
design. Design first: mockup and plan, no implementation until the owner approves.

Result: task colours are 10 slots (`maxColorSlots = 10`, migration `0112_p764_color_slots_10.sql` folds
`color % 10`). `palette.ts` holds 10 OKLCH inks plus precomputed rest and hover band hex; recipe tokens
`--kira-task-band-*` live in `tokens.css`. The band paints card heads, day-band span rows and, through
the new shared `AdeTaskBand`, the task/branch panel head and the review and Files window title bars
(`TitleBar` gained a `class` prop). Cards keep a neutral 1px edge; the selected card has a 1.5px
`--kira-fg` inset outline, the selected branch row `bg-field` plus a 1px inset ring. Ripple edge removed
(`TASK_EDGE_RIPPLE`, `rippleIds`, `rippleOf`, i18n `nothingToRebase`/`rebaseList`). `ColorLine` marks
stay in list rows, base marker and phone dot, in the new inks. `check-ade-colours.sh` allowlist now the
30 new hex values. Tests: flow `TestTaskColorSlotsTen`, migration test `TestMigration0112ColorSlotsTen`,
unit `ade-v2-palette-contrast.spec.ts`, UI specs `ade-v2-plan`, `selection-colour`, `ade-v2-task-row`,
`ade-v2-task-panel`, `ade-v2-task-title-line`, `ade-v2-review`, `ade-v2-task-files-window` (124 pass).
Fixture colours 10-13 moved into 0-9. Chromium snaps the 1.5px ring to 1px at 1x, so UI specs accept
`1px` or `1.5px`. Visual baselines and the new `ade-task-tint` visual spec left to PR CI.

## P765

Repo files window: make it available when the repo has no worktree (it opens the main one), and
stop auto-expanding the first folder row; every folder starts collapsed.

Result: the task Files window opens for any task with a repo row. A row without a worktree shows
the repo's main worktree (`mainWorktree`, `ErrNoMainWorktree`); `AdeTaskFileListing` carries `root` and
`main`, and the window shows a Main worktree badge. `initExpanded` is gone: every folder starts
collapsed, user toggles live for the window's life. Flow test `TestTaskFilesMainWorktree` and contract
keys `list#main`, `noMainWorktree`; UI specs `ade-v2-task-files`, `ade-v2-task-files-window` and
`list-keyboard` extended and passing. Visual baselines `files-open`, `files-reading`,
`files-launch-error` need re-record and `files-main` is new; left to PR CI.

## P766

Owner request: unify the colour components so a future colour change is modular and simple. One
module owns palette, band, mark and selection ring; every coloured surface consumes it. Runs after
P764 ships, builds on its shared band component. Plan audits every current colour consumer.

Owner decisions (audit `plans/colour-audit.md` section 5, mockup `plans/colour-rollout-mockup.html`):
one palette app-wide, the task Pure-B inks and bands; band only, no colour dots in heads; tabs and
list rows take the band too (every coloured module, terminal scripts included); engine and connection
icons are already coloured and stay untouched; task status first took the same palette, then the owner reversed it: status gets its own stronger, purer colours, distinct from the task colours (P766 plan to be revised before implementing). Legacy
names and repo chips unanswered: take the audit's recommendations (`lime`/`sky` stored names, band
ground with neutral text).

## P767

Owner request: apply the P764 faint-band design to the other parts of the app: Studio, API, repos,
and any other coloured surface the audit finds. Deliver screenshots of every changed surface.
Runs after P766.

## P768

Chapter close, by hand per `CLAUDE.md`, after P763, P764, P766 and P767 ship.
