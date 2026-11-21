# v0.3.22

Chapter v0.3.22, branch `chapter/v0.3.22`, tag `v0.3.22`. First phase `P707` (highest history phase: `P706`).

| # | Phase | Status |
|---|-------|--------|
| P707 | Installer: no GitHub API rate limit, shared by first install and self-update | Todo |
| P708 | Real `claude` TUI tests: fix the "no SessionStart hook event" failures | Todo |
| P709 | Task list: aligned rows, calmer design | Todo |
| P710 | Selection highlight: neutral grey everywhere, not blue | Todo |

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
