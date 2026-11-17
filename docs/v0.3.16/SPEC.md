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
