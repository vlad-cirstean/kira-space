# v0.3.17 code review round 1

Base: `d2df252d6` (v0.3.16 squash merge to `main`, holds that chapter's last review fixes). Head:
`9eace21d4`. 224 commits, 628 non-PNG files. Covers P672 Parts 1-10, P660 Part 2, P673.

Scope checked: `packages/git-ui/src/lib/reviewKeep.ts`, git-ui `main.ts` mount `keep`,
`ReviewView.vue`, `ReviewCommitList.vue`, `FileTree.vue`, `ReviewCommentsPane.vue`, workbench
`viewState` seam (`KeptViewState`, `keptViewState`), host `RepoReviewView.vue`, `workspace.ts`
close drop, `reviewSession.ts`; `scripts/fetch-onnxruntime.sh` and its callers (darwin Taskfile,
S12); `scripts/check-ui-primitives.sh` new and extended guards (runner, `\2` backreference in U161
smoke-tested); new theme pieces (`TextTooltip`, `DetailHeader`, `DetailSection`, `SwatchPicker`,
`statusTone.ts`, `SaveBar`); `MenuItems.vue` and `contextMenu.ts` radio/checkbox kinds plus their
model hosts (`board/menu.ts`, `useTaskMenu.ts`, studio `project/menu.ts`, grid `menu.ts`,
`collectionMenu.ts`, `EnvironmentSelect`, `MemoryFilters`, `AdeStepNode`, `CellEditorView`);
non-Vue logic changes (`fileTreeModel`, `badgeClass`, `opLog`, docker `format.ts`). Vue files that
lost event handlers were checked one by one (menu conversions, `@mousedown.prevent` fallthrough on
`TooltipIconButton`, Toggle in `ApiVariablesDialog`). Visual baseline PNGs: only under
`tests/visual/**-snapshots`, nothing else.

## Findings

### R1-1 Review keep restores expanded rows and scroll from the first 500-row chunk only

- File: `packages/git-ui/src/lib/reviewKeep.ts:68-80` (expanded restore), `:60-67` (expanded
  write); `packages/git-ui/src/components/review/ReviewCommitList.vue:60` (scroll `ready`).
- Severity: Medium. Dimension: functional correctness (P660 Part 2 acceptance).
- Scenario: review stream sends 500-row chunks (`gitrpc/graph.go` `ChunkRows`), page size
  1000-5000. Reviewer expands a commit at row 700, scrolls there, switches module and back. The
  restore watcher fires on the first `loadedRows > 0` (row count 500), marks the key restored and
  never runs again: row 700 stays collapsed. Next expand or collapse anywhere writes the visible
  `expandedShas` over the kept list, so row 700's expansion is lost for good. Scroll: `ready` is
  `totalSize > 0`, true after chunk 1 (about 22,000px); a saved `scrollTop` past that is clamped by
  the browser, `settledKey` is set, and the restore never retries. Spec `review-module-state.spec.ts`
  uses a short range, so it does not catch this.
- Fix: restore incrementally. Keep `pending = wanted` per key; on each `loadedRows` growth expand
  the newly loaded shas found in `pending` and remove them; stop when `pending` is empty or the
  stream settles (`exhausted`, or the page's last chunk applied). Until then, the expanded write
  stores `[...expandedShas, ...pending]` so unseen rows survive. For scroll, pass a `ready` that
  waits for the saved position to fit (`totalSize >= saved.top + clientHeight`) or the stream to
  settle; expose the saved position via `KeptViewState.scroll(key)` in `keptScrollIf` callers, or
  gate on stream settle only.

### R1-2 Menu radio and checkbox rows drop the screen-reader hint that item rows carry

- File: `packages/workbench/src/components/MenuItems.vue:114-160` (radio option and checkbox
  branches) versus `:168` and `:182-190` (item branch: `aria-describedby` plus sr-only
  `-reason` span).
- Severity: Medium. Dimension: architecture/maintainability (a11y regression from P672 Part 5
  iter2 menu kinds).
- Scenario: hosts moved from `item` (with `hint`) to `radio` options: task menu Move to step
  (`board/menu.ts` stepEntries, hint "live" or "skipped" on disabled options), `AdeStepMover`
  (skipped hint), cell editor format picker (`CellEditorView.vue:330-351`, per-format help). Before,
  a screen reader announced the disabled reason or help through `aria-describedby`; now the text
  exists only in a hover tooltip, which keyboard and screen-reader users never get on a disabled
  menu row. Checkbox rows with `hint` have the same gap.
- Fix: give the radio option and checkbox branches the item branch's pattern:
  `:aria-describedby="opt.hint ? \`${uid}-${opt.id}-hint\` : undefined"` and the sr-only span with
  `:id` and `data-testid="${tidOf(opt)}-reason"`. Extract one small `MenuHint` piece (or a shared
  render helper) so all three branches use one copy.

## Dimensions with no finding

- Security: none. P673 fetch keeps HTTPS plus a pinned SHA-256 of the whole `.nupkg` before
  extraction; `unzip` names fixed members (no path traversal); no credential touched.
  `KeptViewState` stays session memory, keys never leave the window.
- Performance: none. Review keep watchers are O(loaded rows) once per key; `useViewStateStore` LRU
  cap holds; converted menus build their `MenuItem[]` in lazy computeds rendered only while open;
  `TextTooltip` swaps one-for-one with the Tooltip triples it replaced. The 160 MB package download
  is the plan's recorded trade (`P673.md`), release-only.
