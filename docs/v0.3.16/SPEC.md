# v0.3.16

Release retention deletes whole releases instead of their assets; git tags stay. Phases from `P656`
(highest shipped: `P655`, `docs/history/07-v0.3.15.md`). Branch `chapter/v0.3.16`.

| # | Phase | Status |
|---|-------|--------|
| P656 | Release retention deletes whole releases, never tags | Planned |
| P657 | Memory tooltip Scripts category | Planned |
| P658 | SQL data view column search | Planned |
| P659 | Task card hover actions UX | Planned |
| P660 | Keep module state across tab switches | Planned |
| P661 | Agents module memory efficiency | Planned |
| P662 | Query module UI freeze | Planned |
| P663 | Panel resize bar and update widget placement | Planned |
| P664 | API tab bar plus and request input editing | Planned |
| P665 | Cmd+F search in left panels | Planned |
| P666 | Git module last | Planned |
| P667 | Single-select cards | Planned |
| P668 | Single-select toggle groups | Planned |

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

## P658 SQL data view column search

Tables have dozens of columns; `apps/kira-space/frontend/src/studio/views/grid/DataView.vue` offers no way to find one. Add search/filter of columns in the SQL data view grid. Acceptance: user narrows visible columns by typing a name; clearing restores all; tests per CLAUDE.md "Tests" (flow test + UI spec halves).

Plan: `plans/P658.md`.

## P659 Task card hover actions UX

Hover buttons on a task card appear mid-content and confuse. Acceptance: hover buttons align right; buttons that appear over the title move to a clearer place; menu button removed (same as right click); "no branch" shows once, not twice; agent indicator bigger (key info). Tests per CLAUDE.md "Tests" (flow test + UI spec halves where behavior is new).

Plan: `plans/P659.md`.

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
