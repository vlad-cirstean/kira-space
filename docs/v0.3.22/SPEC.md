# v0.3.22

Chapter v0.3.22, branch `chapter/v0.3.22`, tag `v0.3.22`. First phase `P707` (highest history phase: `P706`).

Code review at chapter end: 2 rounds (user request).

| # | Phase | Status |
|---|-------|--------|
| P707 | Installer: no GitHub API rate limit, shared by first install and self-update | Done |
| P708 | Real `claude` TUI tests: fix the "no SessionStart hook event" failures | Done |
| P709 | Task list: aligned rows, calmer design | Done |
| P710 | Selection highlight: neutral grey everywhere, not blue | Done |
| P711 | Command palette: drop commands already one click away, give each a keyboard shortcut | Done |
| P712 | Show every shortcut: button tooltips, context menus, menu bar | Done |
| P713 | Real `claude` `TestAdeReviewStep`: resumed implementer lacks `review.reply` | Done |
| P714 | Real `claude` details bench gate: F5 and F6 fail at base | Todo |
| P715 | Approval prompt dialogs (database, API) redesign; data review rows resizable | Todo |

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

**Result.** Tokens (`packages/theme/src/tokens.css`): `--kira-select` #36363c (fill), new
`--kira-select-edge` #71717a (frame, ring, underline, drop target), new `--kira-select-text` #45454d
(`::selection`, Monaco, xterm), `--kira-focus` #a1a1aa (keyboard focus only, 7.24:1 on chrome).
`--kira-accent` stays blue for primary action and on-state controls. Hover stays `bg-field`.
Monaco: 13 selection, highlight, list, drop and find-toggle keys now grey (vs-dark blues no longer
leak). xterm selection, chart brush edge, slick editing outline, selected plan card, radio card,
document row, workflow node, link pill, tag, SecondaryTabs underline, mobile latched Ctrl
(`dialog` variant `aria-pressed`) grey. Non-focus uses of the focus token retargeted: drop lines and
drop zones to `select-edge`, drag badge `bg-fg`, git HEAD ring, current-branch check and links to
`info`. Site pins its blue `--sl-color-accent-low`. Guard N18 (`scripts/check-conventions.ts`, 4 spec
tests) keeps tokens, engine keys, selection classes and CSS neutral. Blue kept (primary, info,
links, data palettes, checkbox, switch, radio dot, progress): plan `P710.md` section 3.3.
No flow test: pure styling, no IPC.

Specs: `selection-colour.spec.ts` new (computed fill, edge, hover, `::selection`; plan and grid);
updated `automationsSmart.ts`, `control-sizing.spec.ts`, `resize-handle-color.spec.ts`,
`ade-v2-plan.spec.ts`. 197 targeted UI specs pass. Visual baselines re-recorded and inspected (6;
every other baseline unchanged because no selection shows in it): `workflow-graph`,
`workflow-graph-manual`, `workflow-graph-script`, `workflow-graph-review` (selected step node and
choice chips blue to grey, checkbox stays blue), `git-graph`, `git-graph-detail` (HEAD ring and halo
`info`, near identical; selected row grey). Follow-up: on-state controls grey too. New `--kira-on` (alias `--kira-fg`, #d4d4d8) and `--kira-on-fg`
(alias `--kira-bg-chrome`): checkbox, switch track and thumb, radio and progress fill use them;
checked vs off `bg-field` is 10:1, mark on fill 11:1. Toggles and segmented groups were already grey.
Blue stays: `dialog-primary` buttons, link text, info, data. N18 now fails any primary, accent, info or
focus utility in the four on-state primitives. Visual run with `--update-snapshots=all`: Playwright's
default tolerance had kept 20+ stale blue-selection baselines, so every baseline was re-recorded and
the 37 changed were inspected (blue selection and checkbox to grey; the rest sub-threshold fills). Two
carry unrelated drift now recorded: `branch-panel-details` (Open PR badge info tone) and
`approvals-toast` (primary Show button tint); git HEAD ring and check now render in `info`. Fix: the
environment radio sat under a tooltip trigger whose `data-state` (closed) replaced the radio's own,
so `data-[state=checked]:bg-on` never applied and the checked radio rendered dark. Checkbox, radio and
switch fills now key on `aria-checked`. Unchecked edge is new `--kira-control-edge` (#71717a, 3.1:1 on
field), progress track tints it, toggle on is `bg-select` with `select-edge` border. Checked every state
(off, on, indeterminate, hover, focus, disabled) on chrome, field, dialog and menu grounds as 2x crops;
sheets in `screenshots/p710/onstate2/`. 25 more baselines re-recorded and inspected. After
shots `screenshots/p710/onstate/`. Before and after shots (13 pairs, gitignored):
`screenshots/p710/{before,after}/`.

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

Result: palette 27 to 3 commands (`api.variables`, `api.dynamicValues`, `api.importCurl`), listed
in main-window API mode only; every other mode shows the empty copy (`CommandPalette.vue` now
renders it with no search, `CommandEmpty` needs one). 24 removed: 14 already had a chord, 10 gain
one. Registry: 9 new ids (`app.newAlternate` ⌥⌘N, `app.import` ⌘O, `app.checkForUpdates` ⇧⌘U,
`api.environments` ⌥⌘E global; `api.copyAsCurl` ⇧⌘C, `api.editRaw` ⇧⌘E, `view.explain` ⇧⌘X,
`data.generate` ⇧⌘I, `data.filterColumns` ⇧⌘F view), `app.newConnection` renamed `app.new`.
New, New Alternate, Import and Environments are Go menu accelerators plus 4 channels; module
dispatch in `shortcuts/moduleActions.ts` (`MODULE_ACTIONS`). Automations goes through
`useNewScriptRequestStore` (field `requested`, not `pending`: lint U51). Guard in
`shortcutsConflicts.spec.ts`: adds `accel.go`/TS parity and reserved-chord checks. Go
`TestBuildTemplateGlobalChordItems` replaces the Review Code test. `module-actions.spec.ts`: 11
cases. Chord specs appended to the click specs; palette specs rewritten. Dead i18n keys deleted.
Backend flow coverage unchanged: `apiflow` `TestPostmanImportSendExport`, `appflow`
`TestDataGripScan`, `TestUpdateCheckFailsClosed`, `httpflow` `TestCurlReplayMatchesSend`.
Deviations: UI specs press `Meta+Shift+<key>` (test UA is Mac; `ControlOrMeta` resolves by host
OS). View chords act only with focus inside the view, so the specs click a grid cell or the URL
field first. Copy-as-curl chord is not run with focus in the body editor. Tests run: Go
`internal/shell`, `appshell`, theme i18n; `bun test` shortcuts and automations; typecheck and lint
via the hook on every commit; UI: module-actions, consistency, panel-find, http-curl, http-raw,
console-explain, fake-data, data-view-column-filter, kubernetes-tabs, copy-coverage, copy-wording,
dialog-surface, connections, automations-scripts, all green.
Only a real macOS build proves: ⌘N, ⌥⌘N, ⌘O, ⇧⌘U, ⌥⌘E fire from the menu with focus in Monaco, a
grid, a terminal and a dialog input; ⌥⌘N works despite Option composing; the menu shows the new
items and accelerators; the view chords work in WKWebView with Monaco focused. Manual checklist
for the user. Open questions answered: palette kept as is, no rebind UI (no user rebinding exists).

## P712

Shortcuts exist but are not shown. Deliverable: every action that has a keyboard shortcut shows
it. Buttons show it in the tooltip, context-menu items show it right-aligned, menu-bar items show
it right-aligned. One source of truth: the shortcut registry, formatted per platform (macOS
symbols, other platforms text). No user rebinding exists; none is added here. Reuse the existing `DropdownMenuShortcut`
and the menu item `shortcut` field; add a shared tooltip wrapper for buttons instead of per-button
strings. The plan lists every button, menu and context menu audited and any shortcut still
missing. Tests: UI spec that each shown hint matches the registry; visual baselines re-recorded and
inspected. Runs after P711.

Result: chord text has one source, `packages/theme/src/shortcutText.ts` (`formatShortcut`,
`ariaKeyShortcuts`, `IS_MAC`), rendered only by `ShortcutKbd`. TooltipIconButton, TextTooltip,
Button and SearchField take `shortcut: ShortcutId` (chip after the label, `aria-keyshortcuts`;
SearchField chip shows while empty). 28 `kbd` callers converted. Context menus use the menu item
`shortcut` field via `MenuItems`; module controls use `moduleShortcut(mode, action)` over
`MODULE_SHORTCUTS` (`MODULE_ACTIONS` typed from it). Done: title bar, new-window, tab close (now a
TooltipIconButton), SaveBar, view toolbars (data, console, filter, key-value, definition, stream,
document, kibana, HTTP, gRPC, git toolbar), module New/Import/Environments in every module and
menu, repo row Open/Rename/Remove, workflow step Remove, tree delete items, review comment
controls. Dialog header X shows Esc, or Shift+Esc for a prompt; commit buttons keep the inline chip
and gain `form.submit` where missing (config file Save, run script, review submit, connection
Save). Native menu bar already carried accelerators: `TestBuildTemplateShowsEveryGlobalChord`.
Guard N19 (`check-conventions.ts`, 5 spec cases) fails on a bound registry chord with no display
site. Deliberately no chip: ADE task, branch and step review buttons (chord resolves from the
focused element or selection, no-op in the later view, step block targets a run branch); automations
group-menu New script and git worktree rows (no matching chord action). Deviation found by the
suite: the auto-focused dialog X opened its tooltip and took the first Esc (copy-coverage-git,
module-actions); DialogHeader now ignores focus events on the X (tooltip on hover only).
Tests: `ui/shortcut-hints.spec.ts` 20 cases (Mac UA and Linux UA via userAgent override; table of
expected chips written out by hand); Go appshell/shell, bun check-conventions, typecheck and lint
via the hook on each commit; UI: module-actions, copy-coverage(-git), consistency (dialogs),
tooltips, tree, data-view, window-chrome, repo-workspace, panel-find, automations-scripts. Visual:
3 new baselines (title bar tooltip, repo row menu, console Run tooltip). Re-recorded after
inspection: `data-grid` (column filter chip), `connection-privacy-tab`, `connection-privacy-empty`
(Save chip), `git-graph`, `git-graph-detail` (HEAD node halo, stale since the P710 focus-token
retarget, not this phase). `proof-window` and `ade-task-files` failed once under load, passed
alone.
Follow-up, not fixed: in the git module the native View > Refresh and Find menu items may swallow
F5 and Cmd+F on macOS before the module sees them. Only a real macOS build shows it.
Manual macOS checklist: menu bar items show accelerators right-aligned; tooltips show the chip
after the label on hover and on Tab focus; context-menu chips right-aligned in a native WKWebView;
each shown chord fires its action with focus in Monaco, a grid and a dialog input; dialog X
tooltip does not eat Esc.

## P713

Found in P708. `TestAdeReviewStep` fails about half of runs, also at base `605a98ec3`. A resumed
implementer session makes the model report the state tool has no `review.reply` action, though the
review grant adds it. Headless runs only. Deliverable: root-cause (is the grant missing on resume,
or does the resumed session cache the tool list) and fix code or test so it passes repeatedly
against the real CLI. Also watch `TestAdeBranchRequest` (one 3 min headless timeout in a full run).

Result: root cause was product, not test. CLI 2.1.330 saves a deferred MCP tool's schema in the
transcript (`deferred_tools_record`) at the first `ToolSearch` load and replays it on `--resume`; a
later `ToolSearch` does not refresh it (planner probe: stale description and enum quoted, new action
still accepted by the server). The implement run is a Space grant, whose `state` enum lacked
`review.reply`; the review round resumes that conversation, so haiku saw a stale enum and failed about
half of runs. Fix: `internal/agentmcp/state.go` lists `review.reply` for review or Space tool sets
(`reviewOrSpace`); `reviewState` refusal says it works only in a review round. Rule in ARCHITECTURE:
a tool's schema never gains an action across resumes of one conversation. Plan: `plans/P713.md`.
Guards: `TestReviewRoundStateIsSubsetOfSpaceState` (hermetic); `TestAdeReviewStep` reads the
implementer transcript and asserts the recorded `state` enum has `review.reply` (red before the fix
every run, green after). Schema budgets unchanged (run 474 of 500, push 512 of 540). Real runs:
`TestAdeReviewStep` 6 of 6 after the fix (293 s, spend under 0.15 USD); whole ADE row passes
(0.0986 USD). `TestAdeBranchRequest` flaked, two causes, both test-side haiku choices: a shell
redirect into the worktree was refused outside the working folders (run stalled before the rename), and
a finish with `failed` after the expected refusal. Prompt now names the Write tool, `git -C` and a
`done` finish; failing runs dump the run log tail. 5 of 5 with the final prompt (before: 4 of
20 failed: two 3 min stalls, two failed finishes at about 99 s; two had their log). No `testTimeout` change. Flow test item skipped: the fake agent records no `tools/list`.
Details bench (free check passes; paid round capped at its 1.50 USD guard, 18 of 24 runs, 1.49 USD):
all runs pass; context R/T F4 0.90, F5 0.98, F6 1.06, F1 0.99; cost R/T F4 0.97, F5 1.28, F6 1.43,
F1 1.12. The gate marks F5 and F6 fail (2 reps each); the Space schema grew by one enum value and one
line, about 20 tokens. Baseline round at the parent of the fix (same guard, 18 of 24 runs, 1.44 USD):
all pass; context R/T F4 0.90, F5 0.97, F6 1.06, F1 1.01; cost R/T F4 0.87, F5 1.39, F6 1.48, F1 1.28.
The gate fails F5 and F6 there too (F1 on cost, inconclusive): pre-existing, not caused by P713;
post-fix ratios are the same or lower. Budgets and thresholds unchanged.

## P714

Found in P713. The paid details-bench round (DEV_ENVIRONMENT "Real `claude` tests") fails its
gate on F5 and F6 (context R/T 0.97-1.06, cost R/T 1.28-1.48 on 2 reps each), also at the commit
before the P713 fix, so it predates it. F1 cost is inconclusive. One round costs about 1.5 USD
and hits the guard at 18 of 24 runs. Deliverable: root-cause with the P713 Result numbers as the
starting point (real regression in the details tool/prompt path, stale thresholds or baseline, or
sample noise at 2 reps), then fix the code or recalibrate the gate with evidence; the plan states
the budget and rep count needed for a trustworthy verdict and what only a paid run proves.

Result (not closed; gate still fails). Diagnosis from transcripts (`KIRA_SPIKE_DUMP`): harness
artifact first, then product prompt waste, then inherent ceremony. (a) Arm R's prompt never named
the draft while the stub refused a draft-less ok finish: one refused `step.finish` in F5 and F6.
Fixed (`benchDraftPath`, `TestDetailsBenchArmRServes`). (b) The fragment's "before an ok finish"
primed status `ok`, refused by `state` in 12 of 12 R runs; fixed ("before you finish", goldens, ADE
real `claude` row passes). (c) Residual: README read, draft write, `step.finish`, `ToolSearch`.
Verdict round after (a) only, 24 of 24 runs, 3 reps, 1.89 USD, all pass: cost R/T F4 0.93, F5 1.34,
F6 1.52, F1 1.31; context R/T F4 0.90, F5 0.98, F6 1.06, F1 0.99. After (b), R only, 1 rep (T from
the round): cost F5 1.18, F6 1.37, F1 1.29, context F5 0.96, F6 1.05, F1 1.00. Gate not met on F6
(cost, context), F1 and, at 3 reps, F5 (cost). Total spend about 2.8 USD. Open: T on F5 and F6
never finishes while every Kira step does; plan Open question 1 (A: give T the v0.3.13 finish
instruction, B: record as Known open item, C: recalibrate). Plan: `plans/P714.md`.

## P715

Two user complaints, one phase. (1) The approval prompt dialogs for database and API agent
operations are cluttered: roughly five things stacked one after another, and the facts that
matter most (read vs write operation, read-only or not) do not stand out. Deliverable: audit
every field these dialogs show (`apps/kira-space/frontend/src/studio/approvals/`, the toast and
the dialog), rank by what a user needs to decide, and redesign: one clear verdict row for
operation kind and read-only state, the statement or request as the focus, secondary detail
collapsed, actions unambiguous. The plan proposes the design with screenshots (before from the
real frontend, proposal as a prototype) and the user approves it before implementation starts.
(2) The data review view (result and proof review, `ResultReview.vue`, `ProofReviewBody.vue`)
crops cell values and rows cannot be resized, so full values are unreadable. Deliverable: resize
a row (drag handle) and view the full value of a cell (for example expand row or cell popover),
reusing the grid's existing row-resize behaviour if one exists. Tests: UI specs for both; visual
baselines re-recorded and inspected.
