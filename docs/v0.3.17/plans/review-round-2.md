# v0.3.17 code review round 2

Base: `d2df252d6` (v0.3.16 squash merge). Tip: `380fa1edd`. Same base as round 1; full diff
reviewed, round-1 fixer commits (`3874869d5`, `e4602f59c`) included. PNG content skipped.

Areas read closely: menu radio/checkbox kinds (`contextMenu.ts`, `MenuItems.vue`, `MenuHint.vue`,
`MenuRowLabel.vue`) and every producer converted to them (`collectionMenu.ts`, studio API, project
and grid menus, ADE task and step menus, memory filters); P660 Part 2 keep seam (`reviewKeep.ts`,
`review.ts` `streaming`, `behaviour/viewState.ts`, `state/viewState.ts`, git-ui `main.ts`,
`RepoReviewView.vue`, `workspace.ts` drop, `ReviewView.vue`, scroll keys in three panes); theme
primitives (TextTooltip, SecondaryTabs, KeyValueList, SwatchPicker, DetailHeader, SectionHeading,
Disclosure, PopoverContent `flush`, Button `kbd`, Checkbox indeterminate, FieldLabel `required`,
SaveBar, statusTone and its consumers); `scripts/check-ui-primitives.sh` (U13-U161 regexes, heredoc
expansion, `\2` back-reference against the `(?<t>)` DEFINE group, exempt bookkeeping; U161 and U91
exercised on scratch fixtures); P673 `scripts/fetch-onnxruntime.sh`.

Count: 4 findings (1 Medium, 3 Low).

## F1: radio option equal to `value` can never run; API "Move to collection" to own root is dead

- File: `packages/workbench/src/components/MenuItems.vue:60-62` (`onOptionSelect`), producer
  `apps/kira-space/frontend/src/studio/api/menu.ts:53-57` via
  `packages/workbench/src/lib/collectionMenu.ts:41`.
- Severity: Medium. Dimension: functional correctness (regression from P672 Part 5 iter2,
  `5c5f803fb`/`a7991b10f`).
- Scenario: in the API collections tree, right-click a request inside a folder of collection
  "Billing", open "Move to collection". `moveMenu` passes `current: home` ("Billing") and
  `canMoveTo` that returns true for "Billing" because the row is not at its root (`parentId !==
  null`); the comment there says a collection stays a valid target unless every row already sits at
  its root. The option is enabled and checked. Clicking it hits `opt.value === current` and returns
  without calling `run`, so `actions.moveTo(rows, 'Billing')` never fires. At base the same item
  was a plain `item` with `checked` and ran. Moving a nested request or folder to its own
  collection's root is now impossible from the menu, with no feedback. `moveToCollectionMenu`'s own
  doc ("Default: any id other than the current one") advertises a `canMoveTo` that may allow the
  current id, which the renderer now silently overrides.
- Fix: let the producer decide. Drop the `opt.value === current` short-circuit in `onOptionSelect`
  (keep the `disabled` guard) and have producers disable the current option where reselecting is a
  no-op (`collectionMenu.ts` default `canMoveTo` already does). Alternatively keep the no-op but
  give `MenuRadioOption` an explicit `rerun?: boolean`, set it in `collectionMenu.ts` when
  `canMoveTo(opts.current)` is true. Update the `MenuRadioOption` doc comment
  (`contextMenu.ts:7`) to match. Extend `apps/kira-space/tests/ui/studio/api-lifecycle.spec.ts`
  with a nested-row move to its own collection.

## F2: Button `kbd` writes display glyphs into `aria-keyshortcuts`

- File: `packages/theme/src/components/ui/button/Button.vue:35`.
- Severity: Low. Dimension: architecture/maintainability (a11y contract).
- Scenario: the 26 callers pass `:kbd="formatShortcut(id)"`. `formatShortcut`
  (`packages/workbench/src/shortcuts/keys.ts:41`) returns `⌘⏎`, `⇧⎋`, `⌘⌫` on macOS and
  `Ctrl+Enter`, `Shift+Esc` elsewhere. `aria-keyshortcuts` takes ARIA key names (`Meta+Enter`,
  `Shift+Escape`). On macOS, the shipped platform, every value is invalid, so VoiceOver reads glyph
  names or nothing; `Esc` is invalid on other platforms too.
- Fix: drop `aria-keyshortcuts` from Button (the visible `Kbd` stays), or add a
  `formatAriaShortcut(id)` beside `formatShortcut` that emits ARIA names and pass it as a separate
  prop.

## F3: review row restore rescans from row 0 on every chunk

- File: `packages/git-ui/src/lib/reviewKeep.ts:86-90`.
- Severity: Low. Dimension: performance.
- Scenario: while `pending` for a branch is non-empty, each `loadedRows` change (one per 500-row
  chunk, `ChunkRows` in `internal/gitrpc/graph.go:17`) loops `shaAt(i)` from 0 to `rows`. A kept
  sha that is no longer in the range (branch rebased, base changed) keeps `pending` non-empty for
  the whole stream, so a long review range costs O(rows² / 500) `shaAt` calls on the main thread
  during the stream (a 50k-row range: about 2.5M calls).
- Fix: keep a per-key scanned index next to `pending` and start each pass at it
  (`for (let i = scanned; i < rows; i++)`, then `scanned = rows`); reset it when `pending` is
  created.

## F4: SPEC table still marks P672 Part 10 Planned

- File: `docs/v0.3.17/SPEC.md:19`.
- Severity: Low. Dimension: maintainability (process record).
- Scenario: P672 Part 10 is implemented and committed (`f931bbddd`, `ed4f2cc35`, iter2 and final
  results written under its section, lines 120-129), the chapter full run ran after it
  (`cb67c41b6`), yet its table row reads `Planned` while the later row P673 reads `Done`. The close
  phase folds history from table status and Results; a resumed session reading the table would see
  an open phase and re-enter it. The section also has no single `Result:` line like P673's.
- Fix: mark the row `Done` and add a `Result:` line summarising the per-Part iter2 lines, before
  the close phase folds the chapter.

## Dimensions with nothing further

- Security: nothing found. Fetch script pins the NuGet package SHA-256 before `unzip`, extracts
  named members only, and keeps https; no credential handling changed.
- Guard script: no defect found. Heredoc is unquoted on purpose (`${TOK}` expansion); no guard
  carries `$` or a backtick that the shell would eat; `\2` in U161 correctly names the tag group
  after the DEFINE group; U161 and U91 matched and rejected the intended fixtures.
- P660 Part 2 beyond F3: write/restore ordering, `streaming` reset on abort and supersede,
  `forgetBranch` and workspace-close prefixes (trailing `:`), and the host-provided store
  injection checked; no defect found.
