# v0.3.16

Release retention deletes whole releases instead of their assets; git tags stay. Phases from `P656`
(highest shipped: `P655`, `docs/history/07-v0.3.15.md`). Branch `chapter/v0.3.16`.

| # | Phase | Status |
|---|-------|--------|
| P656 | Release retention deletes whole releases, never tags | Planned |
| P657 | Memory tooltip Scripts category | Done |
| P658 | SQL data view column search | Done |
| P659 | Task card hover actions UX | Done |
| P660 | Keep module state across tab switches | Planned |
| P661 | Agents module memory efficiency | Planned |
| P662 | Query module UI freeze | Planned |
| P663 | Panel resize bar and update widget placement | Planned |
| P664 | API tab bar plus and request input editing | Planned |
| P665 | Cmd+F search in left panels | Planned |
| P666 | Git module last | Planned |
| P667 | Single-select cards | Planned |
| P668 | Single-select toggle groups | Planned |
| P669 | Drop move up/down from left-panel menus | Planned |
| P670 | Remove connection Secrets section | Planned |
| P671 | Terminal links: copy via right click and reliable file paths | Planned |
| P672 Part 1 | UI inventory and uniformity rules | Planned |
| P672 Part 2 | Apply uniformity (cut into Parts 2..n after Part 1) | Planned |

## P656 Release retention deletes whole releases, never tags

Retention (`scripts/releaseretention`, `internal/releaseline`, `.github/workflows/release-retention.yml`,
`docs/PACKAGING.md` §7, `docs/ARCHITECTURE.md`) today deletes release assets only. New rule: a
release that fails the keep rule is deleted whole (`DELETE repos/{repo}/releases/{id}`). Git tags are
never deleted: no `git/refs` call, no `--cleanup-tag`. Keep rule unchanged: each minor's `.0` (or
first release), last patch of each minor, last 5 patches of the current minor, `/releases/latest`.
Expected on the live listing (latest `v0.3.15`): delete `v0.3.4`, `v0.3.5`, `v0.3.6`, `v0.3.9`,
`v0.3.10`; keep `v0.3.3`, `v0.3.11` to `v0.3.15`. Check updater and `install.sh` behaviour against a
deleted release.

Plan: `plans/P656.md`.

Result: Done. Retention deletes each non-kept release whole through `releases/{id}`; tags stay. `releaseline.Release`
gains `ID`; `Delete` is one entry per release; TSV is `release id, tag`; workflow Apply rewritten; asset-less
releases failing the keep rule are deleted too. Guard grep (`git/refs`, `cleanup-tag`, `releases/assets`,
`release delete`) has no hits. Tests: `go test ./scripts/releaseretention/ ./internal/releaseline/` pass. Live read-only
dry run (latest `v0.3.15`) deletes exactly `v0.3.4`, `v0.3.5`, `v0.3.6`, `v0.3.9`, `v0.3.10` and keeps `v0.3.3`,
`v0.3.11` to `v0.3.15`: `total: 11 releases, 5 to delete (5 assets, 270 MB); tags kept`. `actionlint` not installed;
Apply script passes `bash -n`. Updater and `install.sh` unchanged. User step after merge: dispatch `Release retention`
with `dry_run=true`, confirm the 5 releases, then keep or set `RELEASE_RETENTION=apply`.

## P657 Memory tooltip Scripts category

Status-bar memory tooltip shows Agents category (CPU/RAM) but not script processes. Add a Scripts category next to Agents showing CPU/RAM of script processes, same behavior and layout as Agents. Acceptance: tooltip lists Scripts beside Agents with live values; tests per CLAUDE.md "Tests" (flow test on backend, UI spec on frontend, same scenario).

Plan: `plans/P657.md`.

Result: Done. `internal/procroots` (Agents, Scripts ref-counted root sets) replaces `internal/agentproc`. Scripts roots
tracked at terminal script runs (`OpenParams.Script`), headless script runs and DB pre-connect scripts. Sampler adds a
disjoint Scripts group, precedence Agents > Scripts > App; totals and headline are App + Agents + Scripts. Tooltip shows a
Scripts row between Agents and Total; hint reworded. Ran: `go test ./internal/metrics ./internal/terminal ./internal/scriptruns
./apps/kira-space/internal/preconnect` (incl. 4 new `TestGroupProcesses` cases); flow tests `TestAppMetricsCountsScripts`,
`TestAppMetricsCountsAgents`, `TestSQLitePreconnect*`; Playwright `ui` window-chrome metrics test and `visual` status-bar
test (no baseline change); `bun run typecheck` and `bun run lint` via pre-commit hook. Frontend bindings regenerated locally
(gitignored) for the hook's typecheck. Commits 2 and 3 of the plan's grouping landed in one commit.

## P658 SQL data view column search

Tables have dozens of columns; `apps/kira-space/frontend/src/studio/views/grid/DataView.vue` offers no way to find one. Add search/filter of columns in the SQL data view grid. Acceptance: user narrows visible columns by typing a name; clearing restores all; tests per CLAUDE.md "Tests" (flow test + UI spec halves).

Plan: `plans/P658.md`.

Result: Done. Filter toolbar gets a column-name field (case-insensitive substring, whitespace/comma terms OR-ed, PK columns kept, no match keeps all columns and flags the field). Display-only: no bridge call, no `tab.state`. Text lives in `DataViewRuntime.columnFilter` (P660 seam); `SlickGridHost` applies it via `visibleColumnOrder` after a `TIMING.search` (150 ms) debounce. Palette command `data.filterColumns`; row-search jump to a hidden column clears the filter first (`revealPageCell`, fixes page-vs-display index mix). No Go half: IPC untouched, UI spec asserts every wide-table read keeps `projection: null`. Ran: `bun test visible-column-order.spec.ts` (7 pass), Playwright `ui` `data-view-column-filter.spec.ts` (1 passed) and `data-view.spec.ts` (1 passed), `typecheck` and `lint` via pre-commit. Deviations: debounce 150 ms not 120 (guard U46 requires `TIMING`); status uses `common.ofTotal.label`, field placeholder "Column name", palette label "Filter grid columns" (catalog guards C3/C4 reject duplicate wording); added `columnFilter` to the runtime literal in `grid-pending-by-primary-key.spec.ts` (type).

## P659 Task card hover actions UX

Hover buttons on a task card appear mid-content and confuse. Acceptance: hover buttons align right; buttons that appear over the title move to a clearer place; menu button removed (same as right click); "no branch" shows once, not twice; agent indicator bigger (key info). Tests per CLAUDE.md "Tests" (flow test + UI spec halves where behavior is new).

Plan: `plans/P659.md`.

Result: Done. Card head and board branch row hover clusters moved to the right action zone
(`ade-card-actions`, `ade-branch-actions`); row pencil dropdown is `align="end"`. `ade-card-more` and
board `ade-branch-more` removed; right-click and Shift+F10 (new `menu.open` shortcut on the board row)
open the same menus; panel More kept. Draft row context reads once: `usePlanModel` takes
`line2.context`; `space.ade.baseMarker.draft.label` is now `Forks from {base}` (plan said `From {base}`,
`check-catalog` C3/C4 rejected it as a near-duplicate of other keys). Agent indicator: `AdeTaskSessions`
`size="lg"` (24px button, 16px glyph) in a left lead column spanning both head lines; `AdeActivityIcon`
gains size 16. UI only, no flow-test half. Ran: `typecheck:web` (only preexisting missing-bindings
errors without copied bindings; hook typecheck green), lint hooks, UI specs `ade-v2-card-icons`,
`ade-v2-row-actions`, `ade-v2-task-row`, `ade-v2-plan`, `ade-v2-session-status`, `ade-v2-add`,
`ade-v2-base-rebase`, `ade-v2-force-push`, `ade-v2-review-open`, `ade-v2-review-uncommitted`,
`ade-v2-dialogs`, `copy-wording*`, `ade-v2-panel` all pass. Visual: re-recorded `plan-branches-stacked`
and `plan-day-bars`; the other five baselines unchanged. Spec tweak: row actions right-edge bound is
60px (b_auth keeps tag and danger action after the cluster).

## P660 Keep module state across tab switches

Switching modules/tabs (API, Studio, others) loses scroll position, search bars and similar view state. User reported before; still happens. Find the cause, fix for every module. Acceptance: after switching away and back, each module restores scroll, search text and comparable view state; tests per CLAUDE.md "Tests" (flow test + UI spec halves).

Plan: `plans/P660.md`.

## P661 Agents module memory efficiency

App uses ~1.1 GB RAM with no other module opened and few tasks and sessions. Make the agents module very memory efficient. Acceptance: measure before and after, name the biggest retainers found, fix them, report the reduction in Result.

Plan: `plans/P661.md`.

## P662 Query module UI freeze

Some queries freeze the whole UI before data shows (unknown whether data arrives). Opening a new request from the menu bar unfreezes it, until returning to Studio. Find the root cause and fix it for all modules and places in the app. Acceptance: no query freezes the UI; root cause named in Result; tests per CLAUDE.md "Tests" (flow test + UI spec halves).

Plan: `plans/P662.md`.

## P663 Panel resize bar and update widget placement

Resize bar between panels is strong blue; app-update version widget sits on the status bar's right. Acceptance: (1) resize bar uses a neutral grey theme token (muted, no blue) in idle, hover and drag states; hover and drag stay distinguishable from idle by a step in grey; (2) update version widget moves to the status bar's left side, right side loses it; UI specs cover bar color per state and widget position; visual baselines re-recorded where changed.

Plan: `plans/P663.md`.

## P664 API tab bar plus and request input editing

(1) API module tab bar gets a "+" button for a new request tab, like the Runs module tab bar. (2) Request param/header (key-value etc.) inputs misbehave, at least for multiline values, possibly single-line too (untested): left/right arrow navigation broken, caret stops on spaces not in the actual string or moves more than one character. Find cause (likely rendering overlay/mirror or custom editor), fix for every such input in the API module. Acceptance: arrow keys move one character, caret positions match the string, single and multiline; "+" opens a new request tab like Runs. UI-only, no backend: UI specs for both (plus, caret navigation per input kind).

Plan: `plans/P664.md`.

## P665 Cmd+F search in left panels

Focus in a left panel (Connections, Collections, etc.) plus Cmd/Ctrl+F opens that panel's search bar and focuses it, typing starts at once. Acceptance: every left panel with a search bar; palette/shortcut registry entry; UI spec per panel. UI-only, no backend.

Plan: `plans/P665.md`.

## P666 Git module last

Move the Git module to the last position in the module switcher/navigation: sidebar order, shortcut numbering if tied to order, persisted order defaults. Acceptance: resulting order stated in Result; shortcuts and docs updated; UI spec. UI-only, no backend.

Plan: `plans/P666.md`.

## P667 Single-select cards

Round RadioGroup becomes shadcn card-style radio: whole card is the click target, title plus description, selected state clearly visible (border and tint from theme tokens). Keyboard and aria radio semantics stay via RadioGroup. Surfaces: Reset mode (`packages/git-ui/src/components/dialogs/ResetDialog.vue`, soft/mixed/hard with descriptions), Review route (`apps/kira-space/frontend/src/ade/v2/review/AdeWorkflowReviewPanel.vue`), mainline parent pickers (`CherryPickDialog.vue`, `RevertDialog.vue`). One shared theme component for card radios (`packages/theme/src/components/ui/radio-group` or a sibling), Tailwind only. Existing data-testids kept. Acceptance: states stated (idle, hover, selected, focus-visible, disabled); existing UI specs that click these radios updated; visual baselines re-recorded where changed. UI-only, no flow half.

Plan: `plans/P667.md`.

## P668 Single-select toggle groups

Round RadioGroup becomes ToggleGroup (type single, segmented, solid selected fill). A value must stay selected: a click on the selected item must not deselect. Surfaces: Runs params `select` type (`packages/workbench/src/automations/run/ParamsForm.vue`; wrap for many options, Select past ~5 options), `RunMcpTab.vue` (both groups), `ClusterMcpDialog.vue` (`apps/kira-space/frontend/src/studio/kubernetes/components`), `RunFolderTab.vue`, `AdeAddPopover.vue` (kind), `AdeStageDialog.vue` (close/keep), `WorktreeDialog.vue`, `StashDialog.vue` (save source). Stay radios: `ConnectionEnginePicker`, `AdeTaskStatuses` colours, `EnvironmentsView` active env. Existing data-testids kept. Acceptance: states stated (idle, hover, selected, focus-visible, wrapped, Select fallback); existing UI specs that click these radios updated; visual baselines re-recorded where changed. UI-only, no flow half.

Plan: `plans/P668.md`.

## P669 Drop move up/down from left-panel menus

Right-click menus in the left panels (Studio Connections, API Collections, Kubernetes clusters, any other left-panel tree) have Move up / Move down items. Drag and drop already covers reordering. Remove the items and their keyboard shortcuts, commands, palette entries and i18n keys, plus backend or state code only they used. Keep what drag and drop still uses (e.g. `orderAfterDrop`; `canMoveStep` only if still used). Acceptance: no Move up/down in any left-panel menu or the shortcut registry; drag reorder still works; UI specs of removed items removed, drag specs kept; `docs/ARCHITECTURE.md` and shortcut docs updated. UI specs; flow tests only where backend code is removed (planner states if none needed).

Plan: `plans/P669.md`.

## P670 Remove connection Secrets section

Connection edit dialog shows a Secrets block on every tab; user finds it confusing and does not know its purpose. Planner first establishes what it does (`apps/kira-space/frontend/src/studio/project/connection/ConnectionSecrets.vue`, `ConnectionSecret` in `packages/shared/domain/connectionConfig.ts`, `revealConnectionSecret`, `deleteSecret`, their Go side) and whether any user flow depends on it (how passwords/URIs are stored, where else they are edited). Then remove the UI block and every code path only it used, keeping whatever stores/edits the connection password elsewhere. If removal loses a capability users need (no other way to set/clear a password or URI secret), the plan says so prominently and proposes the minimal alternative; the orchestrator confirms with the user before implementation. Acceptance: Secrets block gone from every tab; connections still save and connect with and without password; specs updated (credential-reveal, confirm-focus etc.); docs updated. UI specs plus flow tests where backend code is removed (planner states if none needed).

Plan: `plans/P670.md`.

## P671 Terminal links: copy via right click and reliable file paths

Two fixes. (1) In terminal output a URL link opens only on Cmd-click. Right click on a link must show a menu with Copy link and Open, as in other terminals. Same for file path links: Copy path, Open. (2) File paths in terminal output do not always become links. Planner finds why (`packages/workbench/src/terminal/fileLinkProvider.ts` `findFilePaths`/`resolve`, `apps/kira-space/frontend/src/state/terminalFileLinks.ts`, Go `openedFilesTerminalResolve` against the terminal's cwd) and fixes the misses. Shapes to check: relative paths, `./` `../` `~`, `path:line:col`, paths wrapped across rows, paths inside quotes and parens, cwd of agent and script terminals, exited terminals. Acceptance: right-click menu on web and file links; a table of path shapes that must link in the plan, each covered by a test (unit test for the path finder if it has interacting rules, flow test for the resolve backend, UI spec for the right-click menu and a clickable path); no regression of Cmd-click.

Plan: `plans/P671.md`.

## P672 Part 1: UI inventory and uniformity rules

User feedback: nothing in the UI is uniform. Not only per module: each dialog, menu, card in the same module differs in components and design. "Elements" means more than dialog and card: how information shows inside them. Covers alignment, colors, fonts, sizes, spacing, labels, icons, empty/loading/error states, headers/footers, button placement and order, form layouts, lists/tables/rows, tooltips, badges. Part 1 is analysis only, no UI change. Deliver: (1) exhaustive inventory of element types and each variant actually used, with counts and file references (via codegraph), grouped per element type; (2) a small set of canonical patterns; (3) a mapping of every variant to its canonical pattern, or a justified exception. Output committed under `plans/`: inventory, canonical rules, migration list. From the inventory the planner decides how to cut Part 2 into Parts 2, 3, ... by element type or module, each sized for one planner and one implementer; the orchestrator then adds them as rows. Acceptance: exhaustive (every dialog, menu, card, list, form in every module appears) and canonical rules concrete enough to review (token values, component to use, markup shape). Tests: none (docs).

Plan: `plans/P672.md`.

## P672 Part 2: Apply uniformity

Placeholder, cut into Parts 2..n after Part 1. Migrate variants to the canonical patterns per Part 1's migration list. Each part keeps behavior unchanged except look and layout. Tests: UI specs and visual baselines updated; no new unit tests unless complex logic.

Plan: `plans/P672-part2.md`.
