# v0.3.22

Chapter v0.3.22, branch `chapter/v0.3.22`, tag `v0.3.22`. First phase `P707` (highest history phase: `P706`).

| # | Phase | Status |
|---|-------|--------|
| P707 | Installer: no GitHub API rate limit, shared by first install and self-update | Todo |
| P708 | Real `claude` TUI tests: fix the "no SessionStart hook event" failures | Done |
| P709 | Task list: aligned rows, calmer design | Todo |
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
symbols, other platforms text), reflecting user rebinds. Reuse the existing `DropdownMenuShortcut`
and the menu item `shortcut` field; add a shared tooltip wrapper for buttons instead of per-button
strings. The plan lists every button, menu and context menu audited and any shortcut still
missing. Tests: UI spec that a rebind updates the displayed hint; visual baselines re-recorded and
inspected. Runs after P711.

## P713

Found in P708. `TestAdeReviewStep` fails about half of runs, also at base `605a98ec3`. A resumed
implementer session makes the model report the state tool has no `review.reply` action, though the
review grant adds it. Headless runs only. Deliverable: root-cause (is the grant missing on resume,
or does the resumed session cache the tool list) and fix code or test so it passes repeatedly
against the real CLI. Also watch `TestAdeBranchRequest` (one 3 min headless timeout in a full run).
