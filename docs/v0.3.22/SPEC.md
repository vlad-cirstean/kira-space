# v0.3.22

Chapter v0.3.22, branch `chapter/v0.3.22`, tag `v0.3.22`. First phase `P707` (highest history phase: `P706`).

| # | Phase | Status |
|---|-------|--------|
| P707 | Installer: no GitHub API rate limit, shared by first install and self-update | Todo |
| P708 | Real `claude` TUI tests: fix the "no SessionStart hook event" failures | Todo |
| P709 | Task list: aligned rows, calmer design | Done |

## P707

Release v0.3.21 install smoke failed: `scripts/install.sh` got HTTP 403 from `api.github.com`
(unauthenticated, shared runner IP rate limit) on `releases/latest`, three attempts, all 403. A
delay or retry cannot fix a rate limit and the script is also the self-updater, so real users on
shared IPs hit the same wall. Deliverable: the installer stops depending on the unauthenticated REST
API for the normal path. Resolve the release without `api.github.com` (for example the
`releases/latest` redirect and direct `releases/download/<tag>/<asset>` URLs, which are not
API-rate-limited), keep every existing guarantee (hash check, prerelease/draft exclusion,
`--tag`, minor stepping from `internal/releaseline`, `verify-packaging.sh` S16), and keep the API
only where nothing else carries the data, with a clear error naming the rate limit when it
answers 403. Remove the release workflow's smoke sleep and retry loop added in v0.3.20 once the
installer itself is robust. Tests: `scripts/verify-packaging.sh` cases against a local fake
GitHub; plan states what only a real release run proves.

## P708

`TestAdeManualSession`, `TestAdeAdHocSession`, `TestAdeManualSessionStageSettings` and
`TestAdeRestoreSessionContinue` (nested Go modules under `apps/*/tests/claude/`, run with
`bun run test:claude*`, row in `docs/DEV_ENVIRONMENT.md` "Real `claude` tests") fail with "no
SessionStart hook event". Cause unconfirmed. Deliverable: root-cause (verify auth with the probe
in DEV_ENVIRONMENT first; check the installed `claude` version's hook contract against
`internal/harness/claude/hooks` and the settings Kira writes) and fix the code or the tests so
all four pass against the real CLI. If a test cannot run here for a reason outside the repo
(auth, network), say exactly which and why.

## P709

Task list design pass on the real frontend, no mockups. The elements and structure are good; the
rows are not aligned: each task row looks different because names and content differ in length
and shape, and the overall look is tiring. Deliverable: a consistent row grid (fixed columns for
status, name, repos, branch/PR, activity, time; truncation instead of reflow), steadier spacing
and type scale, calmer colour and contrast, hover and selection states that do not shift layout.
Tailwind utilities and shadcn-vue only; no new styling mechanism. Same behaviour, same test ids.
Result shows before and after screenshots of the running app (Playwright against the real
frontend) with long, short and empty-field tasks.

Result: fixed column grid (`plan/taskGrid.ts`) shared by card heads, board branch rows, span and
history rows. Status, estimate, action, PR and tag keep one x whatever the content; title, meta
and branch name truncate. Hover actions overlay the text cell end on the row ground, so hover
never moves a box. Branch names drop to regular weight, muted until hover or selection. Plan
content caps at `max-w-4xl`. Step cell widened (`w-32`, `@lg:w-48`); step percent hides in the
narrow pane. New layout spec `plan rows keep fixed columns and hover never reflows`; the "hover
actions sit right" assertions now pin the overlay edge and unmoved boxes. Dropped stale U37
exemption for `AdeTaskCard.vue`. Visual baselines re-recorded (`plan-branches-stacked`,
`-review`, `-pending`, `plan-day-bars`); full `visual` run 53 passed; 280 targeted `ui` tests
passed. Dark only: no light theme exists (ARCHITECTURE Known open item). Before and after
screenshots (uncommitted, gitignored): `screenshots/p709/{before,after}/`.

Iteration 2 (user feedback on the iteration 1 shots): (1) Cards drop their elevated fill and sit
on the Plan ground (`bg-chrome`, already black: pixel sample rgb(19,19,21)) with a 1px
`border-border` edge, dashed when parked, `border-subtle/50` in the selection ripple. Reading:
"black like the board" = the Plan ground; the right panel (`bg-bg`) is unchanged. If another
surface was meant, the change is local to `boxClass` / `GROUND_CARD`. (2) Reading: "step button" =
the step action on the card head (the only bordered control there; the step menu trigger has no
border). It is now a `toolbar` icon with the tone on the glyph (new `toolbar` outline
compounds, U210 allows both); panel header and workflow block keep the tone outline. (3) Task
colour is a 10px rounded swatch (`AdeTaskSwatch`) before the title on cards, span, review and shelf
rows (outline when parked); history rows keep an empty slot; the 3px left bar and `max-w-4xl`
are gone. (4) The task panel is always open beside the Plan (`AdeShell`); no selection, or one
gone from board and History, falls back to the top Plan task (`AdePlanView`); an empty plan shows
`ade-panel-empty` ("No tasks yet", catalog keys `space.ade.panel.empty` and `.emptyHint`). Narrow
list default stays 400px. History meta capped (`max-w-28`) so titles keep room. Specs: new
`ade-v2-panel-open.spec.ts`, `selectTask` helper, card edge / fill / swatch assertions in
`ade-v2-plan` and `ade-v2-task-row`, card step action border in `ade-v2-action-tone`, 560px card
layout test now runs with a 340px panel at 900px, estimate field scope in `ade-v2-row-actions`.
Baselines re-recorded (`plan-branches-stacked`, `-review`, `-pending`, `plan-day-bars`); other
visual baselines unchanged. Full `ade-v2` + `consistency` + `panel-resize` ui run and full
`visual` run green after fixes. Shots (uncommitted, gitignored): `screenshots/p709-iter2/{before,after}/`,
before = `beeeebe48`. Cards on black: the edge (`#323237` on `#131315`) plus 16px gaps separate
them; read the after shots for contrast.

