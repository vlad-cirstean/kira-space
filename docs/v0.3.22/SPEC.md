# v0.3.22

Chapter v0.3.22, branch `chapter/v0.3.22`, tag `v0.3.22`. First phase `P707` (highest history phase: `P706`).

| # | Phase | Status |
|---|-------|--------|
| P707 | Installer: no GitHub API rate limit, shared by first install and self-update | Done |
| P708 | Real `claude` TUI tests: fix the "no SessionStart hook event" failures | Done |
| P709 | Task list: aligned rows, calmer design | Done |
| P710 | Selection highlight: neutral grey everywhere, not blue | Todo |
| P711 | Command palette: drop commands already one click away, give each a keyboard shortcut | Todo |
| P712 | Show every shortcut: button tooltips, context menus, menu bar | Todo |
| P713 | Real `claude` `TestAdeReviewStep`: resumed implementer lacks `review.reply` | Todo |

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

Result: release index asset `kira-space-releases.txt` (tag, size, sha256 of every eligible release),
built at publish by `scripts/releaseindex` over `internal/releaseline.FormatIndex` and served from
`releases/latest/download/`. `install.sh` and the app checker read it, download from
`releases/download/<tag>/`, and never call `api.github.com` on a normal path. The API stays only as a
transitional fallback while the latest release has no index (HTTP 404), JXA `apiToIndex`, 403/429
dies naming the rate limit; dropped at the v0.4 bump. Stepping is an awk `index_next_step` (S16,
golden `next_step_index.json`); S17 drives `resolve_release` and `download_release` against a local
fake GitHub (11 cases here, the API-listing case runs on macOS only). Release workflow uploads and
verifies both assets; smoke lost its sleep and retries and gained a stepping run and a no-API
assertion. Proven only by a real release run: the redirects answer from a shared macOS runner without
403/429 right after publish, the publish job's `gh api` listing and upload, BWK awk and macOS curl
(`--proto-redir`, `--max-filesize`) behavior, the JXA fallback against real GitHub, the in-app checker
against the real index.

## P708

`TestAdeManualSession`, `TestAdeAdHocSession`, `TestAdeManualSessionStageSettings` and
`TestAdeRestoreSessionContinue` (nested Go modules under `apps/*/tests/claude/`, run with
`bun run test:claude*`, row in `docs/DEV_ENVIRONMENT.md` "Real `claude` tests") fail with "no
SessionStart hook event". Cause unconfirmed. Deliverable: root-cause (verify auth with the probe
in DEV_ENVIRONMENT first; check the installed `claude` version's hook contract against
`internal/harness/claude/hooks` and the settings Kira writes) and fix the code or the tests so
all four pass against the real CLI. If a test cannot run here for a reason outside the repo
(auth, network), say exactly which and why.

Result: root cause was product, not tests. CLI 2.1.330 stops a TUI `--add-dir` path outside trusted
`.claude.json` `projects` at a blocking dialog (default "No, exit") before any hook; Kira passed the
task folder that way on every TUI launch. Fix: `Launch.AddDirs` replaces the flag; the claude hook
listener writes `permissions.additionalDirectories` into the launch's one settings file
(`ComposeInto`, else per-terminal `settings-<id>.json`). Headless keeps `--add-dir`; no listener means
dirs not granted, logged. Plan: `plans/P708.md`. Guard: `TestAdHocSession` asserts no `--add-dir` in
the TUI argv and the task folder in the settings file. Two further blockers showed once SessionStart
fired, fixed in the tests: `CLAUDE_CODE_CHILD_SESSION` inherited from the host session stops a TUI
writing its transcript (Resume found no conversation), now unset with the other parent ids; the
manual step's first message sends haiku exploring and a shell command prompt had no answerer, so
`TestAdeManualSession` and `TestAdeRestoreSessionContinue` seed `Bash` allow in project settings.
`TestHookStopBlock` flaked about half of runs (haiku released its own lease before the Stop); the
prompt now says not to; 6 of 6 pass after. Verified against the real CLI: all five target tests,
the hook, config-isolation rows and `lint:claude` pass, 0.019 USD of `claude -p` spend. Whole module:
`TestAdeReviewStep` fails about half of runs, also at base 605a98ec3 (2 of 4 failed there): a
headless review round resumes the implementer session and haiku sometimes reports no `review.reply`
action; not touched, different subsystem. `TestAdeBranchRequest` failed once in the full run, passed
on rerun and at base.


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


Iteration 3 (user feedback on the iteration 2 shots): (1) Card meta is schedule only (`3d → Mon 28`):
repo names left it (branch rows show them); shelf rows keep repo names in their facts, since they
have no branch rows (one line in `AdeShelfRow.vue` if "nowhere" was meant). (2) Jira key
(`AdeTaskKey`, muted mono) follows the title, never truncated, skipped when it is the title; the
title truncates ahead of it. (3) Status leads line 2 in the 80px column at the title x; line 1 is
title, key, hover overlay and step action; the estimate cell (50px, right-aligned) spans the branch
rows' PR and tag slots; step cell `w-28` (`@lg:w-48`). (4) Task colour is a 3px line in the full
colour (`AdeTaskLine`), replacing the swatch: a left bar was rejected because its mass grows with
branch count and it crowds the structure column, a top edge has one mass on every card. Cards and
span rows colour their top border (dashed when parked, selected keeps the colour on top and the
focus colour on the other sides); task and branch panel heads and the review and Files title bars
overlay a 3px edge (review window without a task: no line); shelf rows, All sessions, needs rows
and the branch tooltip get a 16x3 mark. `AdeTaskSwatch` deleted. U105 allows the parked dash
gradient in `AdeTaskLine`; stale U132 exemptions dropped. Shelf facts cap `max-w-1/4` so the title
keeps room. Specs: `ade-v2-task-row` (grid, key, time cell, top edge), `ade-v2-plan` (card box,
focus on the left edge, meta, drag grab on line 1 since line 2 now starts with the status
button), `ade-v2-card-icons`, `ade-v2-task-panel`, `ade-v2-review`, `ade-v2-task-files-window`.
Baselines re-recorded: `plan-branches-stacked`, `-review`, `plan-day-bars`, `plan-branch-tip`,
`plan-branch-plain-tip`, `files-open`, `-reading`, `-search`, `-launch-error`. Full `ade-v2` +
`consistency` + `panel-resize` ui run (845) green after fixes; full `visual` run green after
re-record. Shots (uncommitted, gitignored): `screenshots/p709-iter3/{before,after}/`, before =
`22a7d1cab`.

Iteration 3 follow-up: (1) No dashed rendering for parked tasks anywhere: `AdeTaskLine` has no
parked variant, the card no longer uses `border-dashed` (iteration 2's dashed card edge is gone
too), U105 no longer allows the gradient in `AdeTaskLine`, the parked-dashed specs are replaced by
"every card edge is solid" plus a `notMerging` tag check. A parked card says so with the existing
`notMerging` tag glyph (circle-slash, tooltip "Not merging") on line 2, and its panel shows the
"Not merging" badge; no new mark was needed. (2) The review and Files title-bar line moved from
above to directly under the bar (`AdeTaskLine variant="under"`, overlay on the bar's bottom 3px, bar
height unchanged). The task panel and branch panel heads keep their top edge: it is the pane's top
edge under the app bar (a card-style top edge), with the title row below it. Specs and the four
`files-*` baselines updated; after shots `screenshots/p709-iter3/after2/`.

## P710

Every selected thing is blue today: a selected task, a selected grid cell or row in Studio, a
selected list item, menu item or tab. Looks dated. Deliverable: one neutral grey highlight
across the whole app. Find the global token(s) behind selection and hover (theme tokens in
`packages/theme`, Tailwind `select`/`field`/accent utilities, AG/slick grid and Monaco/xterm
selection colours, shadcn-vue `accent`), change them to a grey that reads on the black ground
in every surface (list rows, cards, menus, trees, grids, cells, tabs, dialogs, command palette,
mobile app where it shares tokens), and fix every hardcoded blue that stands in for selection.
Keep real blue where it means something other than selection (links, info, open PR, primary
actions): the plan lists each remaining blue use and why it stays. Focus rings keep a visible,
accessible contrast. Tests: visual baselines re-recorded with each changed baseline inspected;
a guard or lint so a hardcoded selection blue cannot return. Runs after P709 (shares the
selected-card styles).

## P711

The command palette lists actions that are already one click away (toggle project or operations
panel, open settings, check for updates, new request, new connection, and similar). They add
noise. Deliverable: audit every palette command in `packages/workbench/src/shortcuts/`,
`apps/kira-space/frontend/src/shortcuts/appCommands.ts` and
`apps/kira-space/frontend/src/studio/shortcuts/studioCommands.ts`. Any command with a visible
button, menu or panel control gets removed from the palette. Each removed action keeps a keyboard
shortcut (add one where missing, through the shortcut registry in `@shared/domain/shortcuts`,
rebindable like the others). Module-contextual actions (new request, new connection, new file,
and similar) share one shortcut whose effect follows the focused or selected module; the plan
lists each action, its module mapping and conflicts with existing bindings. The palette itself
stays, with only what has no other entry point. Tests: UI spec per new shortcut (same scenario as
the click path), a guard for shortcut conflicts, flow test where the action has a backend half.

## P712

Shortcuts exist but are not shown. Deliverable: every action that has a keyboard shortcut shows
it. Buttons show it in the tooltip, context-menu items show it right-aligned, menu-bar items show
it right-aligned. One source of truth: the shortcut registry, formatted per platform (macOS
symbols, other platforms text). No user rebinding exists; none is added here. Reuse the existing `DropdownMenuShortcut`
and the menu item `shortcut` field; add a shared tooltip wrapper for buttons instead of per-button
strings. The plan lists every button, menu and context menu audited and any shortcut still
missing. Tests: UI spec that each shown hint matches the registry; visual baselines re-recorded and
inspected. Runs after P711.

## P713

Found in P708. `TestAdeReviewStep` fails about half of runs, also at base `605a98ec3`. A resumed
implementer session makes the model report the state tool has no `review.reply` action, though the
review grant adds it. Headless runs only. Deliverable: root-cause (is the grant missing on resume,
or does the resumed session cache the tool list) and fix code or test so it passes repeatedly
against the real CLI. Also watch `TestAdeBranchRequest` (one 3 min headless timeout in a full run).
