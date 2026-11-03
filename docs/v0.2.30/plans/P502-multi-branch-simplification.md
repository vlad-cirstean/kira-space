# P502 plan: ADE multi-branch task panel simplification

Row: `docs/v0.2.30/SPEC.md` P502. Base: `feb70f89` (P501 landed). Frontend only; IPC unchanged, so no flow test.

Paths: `web/` = `apps/kira-space/frontend/src/ade/v2/`, `tests/` = `apps/kira-space/tests/`.

## Today (verified with codegraph_explore and the current tree)

A multi-branch task (fixture `T_bill`: `b_bill` web-app, `b_billdash` web-app stacked on `b_bill`, `b_meter` api) shows its branches in up to four places in the Main tab, each in a different visual language:

- `web/panel/AdeMainTab.vue`: fields, then `AdeWorkflowBlock`, then `AdeMainBranches` at the bottom, below every step block.
- `web/panel/AdeMainBranches.vue`: own row markup (`ade-task-branch` button). Wide text chip for the tag (`AdeChip` + `tagLabel`), `AdeRepoTag`, integration chips, name. No stack indent, no base, no action icons, no progress, no attention. PR line below with a text state chip (`PR_TONE`/`prStateLabel`), title link, `→ base`, checks glyph. Edit menu `AdeTaskBranchMenu` (ellipsis icon).
- `web/panel/AdeStepBlock.vue` run lines (`ade-run-line`): target labelled by `AdeRepoTag` only. `b_bill` and `b_billdash` both read `web-app`, so the impl step shows two identical labels.
- `AdeStepBlock` Release step: `ade-release-row` per mine branch (`main —` / `✓ main` badge, integration chips, repo, name). Duplicates the branch list; merged state is already the branch tag (`merged`) and `chips.merged`.
- `AdeStepBlock` Review step: `ade-review-row` per mine branch with a Review code button. Duplicates the branch row's review action (`ade-branch-review` on `plan/AdeBranchRow.vue`, same `reviewCode.choiceOf(row)` predicate).

The Plan's `plan/AdeBranchRow.vue` (P500) is the look the user likes: tinted repo badge, `BranchChip` base marker, `row.depth` indent with connector, name plus progress/context/chips line, `AdeAttention`, `AdeTagIcon` (`BRANCH_TAG_GLYPH`), at most 2 action icons or 1 plus More (`rowActionSplit`, `BRANCH_ACTION_ICON`), review icon, right-click fix menu. Its data is `BranchRowModel` from `usePlanModel`, which `card.rows` already holds in stack order (`view.branchRows`). `StepBlock.release` has one reader (`AdeStepBlock`); mobile never reads it.

## Design

One branch list, one look, near the top. Steps refer to branches by a name you can match to that list.

### 1. Main branches reuse the Plan row

- `web/plan/AdeBranchRow.vue`:
  - New prop `testid?: string`, default `'ade-branch-row'`, bound to the root `data-testid`. Panel passes `'ade-task-branch'`, so plan and panel rows never collide.
  - Root gets `:data-depth="row.depth"` (spec reads stacking from it).
  - Default `<slot />` rendered after the review icon, for panel-only trailing controls. The plan passes nothing.
  - No other behaviour change. Click selects the branch (same as today's `ade-task-branch`), right-click opens the fix menu, actions act. In the task panel `ui.selectedBranchId` is null, so no row shows selected.
- `web/panel/AdeMainBranches.vue` rewrite:
  - Header unchanged (`Branches` label, `ade-add-repo` ProjectPicker, `ade-add-branch`).
  - Per row: wrapper `div` (`ade-task-branch-item`, `:data-branch-id`) holding `<AdeBranchRow :row :color="card.color" testid="ade-task-branch" @force-push=...>`, with `AdeTaskBranchMenu` in its slot when `!review`.
  - Drop local tag chip, `AdeRepoTag`, `integrationChips`, `branchLabel`, `chipClass`, `liveNow`/`tagLabel` imports.
  - Force push: local `forcePushTarget` ref plus `AdeForcePushDialog` (same shape as `plan/AdePlanView.vue:302-305,418`).
- `web/panel/AdeTaskBranchMenu.vue`: trigger icon `ICON.edit` (`@theme/icons`) instead of `ellipsis`, so it never reads as a second More next to `ade-branch-more`. Size `size-5`, `:size="13"`. Testid and items unchanged. Aria key `space.ade.taskTab.branchMenu.trigger.aria` text becomes "Change branch or repo".
- `web/shell/AdeShell.vue` `reviewTarget`: row selector `:is([data-testid="ade-branch-row"],[data-testid="ade-task-branch"])[data-branch-id]`, so the review shortcut works on a focused panel row too.

### 2. PR line: glyphs, indented under its row

- In the wrapper, under the row, only when `prOf(row.id)`: `ade-task-branch-pr` with `:data-branch-id`, left padding `row.depth * 16 + 8` px (inline style, matches the row indent) so it sits under its branch.
- State: icon, not text chip. New `PR_GLYPH` in `web/plan/glyphs.ts`: `open` `git-pull-request`, `draft` `git-pull-request-draft`, `merged` `git-merge`, `closed` `git-pull-request-closed`, fallback `git-pull-request`. Colour `TONE_TEXT_CLASS[PR_TONE[state] ?? 'grey']`. `AdeTip` with `prStateLabel(state)`. Testid `ade-task-branch-pr-state`, `:data-state`.
- Then the title link (unchanged, `useOpenHostView`), `ade-task-branch-pr-base`, checks glyph `ade-task-branch-pr-checks` (unchanged).
- Compute `pr` once per row (`computed` map `branchId → ChangeRequest`) instead of six `prOf(...)!` calls.

### 3. Branches above the workflow

- `web/panel/AdeMainTab.vue`: render `AdeMainBranches` before `AdeWorkflowBlock`. Branches are short and are what run lines refer to; the workflow is long. Update the file comment.

### 4. Step blocks stop repeating branches

- `web/panel/AdeStepBlock.vue`:
  - Delete the Release rows (`released`, `mainBadge`, `mainNameOf`, `ade-release-row`) and the Review rows (`reviewRows`, `reviewOff`, `ade-review-row`, `ade-review-code`). `onReview` stays for run-line `ade-run-review`.
  - Drop imports left unused (`integrationChips`, `TONE_TAG_CLASS`, `TooltipDisabledTrigger`, `CodiconIcon`, `usePlanModel` if unused, `chipClass`).
  - Header comment: drop "or the branches of the Release step".
- `web/board/stepBlocks.ts`: remove `StepBlock.release` and its assignment.
- Review from the panel goes through the row's `ade-branch-review` icon; its open failure lands in `ui.actionError[taskId]` (`ade-panel-action-error`), per `useReviewCode.open` default.

### 5. Run lines name their branch

- In `AdeStepBlock`, a `target(branchId)` helper returns `{ repo, codeRepoId, name }` from `card.rows`; `''` is the task folder (`space.ade.stepBlock.taskFolder.label`, no name).
- Show the branch name (`font-data text-kira-sm text-subtle truncate max-w-40`, testid `ade-run-branch`) after `AdeRepoTag` only when another row of the card shares that repo. Single-branch and one-branch-per-repo tasks look as today.
- Order run lines by the row's index in `card.rows` (task folder first), so they read in the same order as the branch list. Sort in a `computed` over `block.runs`; `buildStepBlocks` stays as is (mobile shares it).

### Not changed

Task list, branch mode panel (`AdeBranchPanel`), archived panel, mobile, IPC, `board/actions.ts` (rows already carry `tag` and `actions`).

## i18n (`packages/theme/src/i18n/en.json`)

- Remove: `space.ade.stepBlock.mainOpen.status` (only reader was `mainBadge`). Keep `space.ade.progress.stepDone.label` (`progress.ts` reads it).
- Change text: `space.ade.taskTab.branchMenu.trigger.aria` "Change branch or repo".
- Add: none (`prStateLabel` keys exist). Catalog `--unused` lint confirms.

## Tests

### New UI spec `tests/ui/ade-v2-main-branches.spec.ts`

Header comment: Main tab of a multi-branch task; frontend only, no backend half. Scenario `T_bill` (`openPlan`, `openTask` from `support/adeV2`, change requests fixture as `ade-v2-task-panel.spec.ts` uses):

1. Branches come first: `ade-main-tab` holds `ade-task-branch` rows before `ade-workflow-block` (compare `boundingBox().y`).
2. Rows in stack order `b_bill`, `b_billdash`, `b_meter`; `b_billdash` `data-depth` greater than `b_bill`'s.
3. Same language as the Plan: for each branch, panel row `ade-tag` `data-kind` and `data-icon` equal the plan row's (`[data-testid="ade-branch-row"][data-branch-id=...] [data-testid="ade-tag"]`). Panel row has `ade-branch-repo` and, for `b_billdash`, `ade-base-marker`.
4. PR line: `ade-task-branch-pr[data-branch-id="b_bill"]` shows `ade-task-branch-pr-state` with `data-state`, base `→ main`, checks glyph; `b_billdash` has none.
5. Run lines: impl step `ade-run-line` for `b_bill` and `b_billdash` show `ade-run-branch` with their names; `b_meter` shows none. Lines appear in row order.
6. Release and Review step blocks hold no branch rows: `ade-step-block` contains no `ade-task-branch`, and `ade-release-row`/`ade-review-row` count 0.
7. Edit menu: `ade-task-branch-menu[data-branch-id="b_bill"]` opens `ade-task-branch-menu-content`.
8. Click `ade-task-branch[data-branch-id="b_meter"]` opens branch mode for `b_meter`.

### Existing specs to update

- `tests/ui/ade-v2-run.spec.ts:398-405`: title becomes "a script run offers Output"; drop the `ade-release-row` assertion.
- `tests/ui/ade-v2-review-open.spec.ts:182-193`: click `ade-task-branch[data-branch-id="b_auth"] ade-branch-review`; expect `ade-panel-action-error` to contain `no window` (not `ade-step-block-error`). Rename test to "...shows in the panel".
- `tests/ui/ade-v2-review-open.spec.ts:208-213`: same locator swap.
- `tests/ui/ade-v2-task-panel.spec.ts:102-113`: replace the `xpath=../..` check with `ade-task-branch-pr[data-branch-id="b_billdash"]` count 0.
- Inner AdeBranchRow testids now also render in the panel. Where a spec opens a task panel and then locates one unscoped, scope it to the plan row or the panel row. Check: `ade-v2-add`, `ade-v2-base-rebase`, `ade-v2-card-icons`, `ade-v2-dialogs`, `ade-v2-force-push`, `ade-v2-launch`, `ade-v2-panel`, `ade-v2-plan`, `ade-v2-review-open`, `ade-v2-task-menu`, `copy-wording` (testids `ade-tag`, `ade-rebase`, `ade-force-push`, `ade-abort-rebase`, `ade-see-log`, `ade-take-over`, `ade-see-error`, `ade-branch-more`, `ade-branch-review`, `ade-base-marker`, `ade-branch-repo`, `ade-branch-agent`, `ade-branch-prog`, `ade-branch-merged`, `ade-branch-deployed`, `ade-attention`). Run each; fix strict-mode failures by scoping, never by weakening an assertion.
- `ade-task-branch` callers keep working (testid kept): `ade-v2-panel`, `ade-v2-task-branches`, `ade-v2-task-flow`, `ade-v2-branch-request`, `git-host-provider`. Run them.
- Run lines still carry `ade-run-line`/`data-branch-id`: `ade-v2-run`, `ade-v2-return-limit`, `ade-v2-review-step`, `ade-v2-sandbox`, `consistency`, visual `ade-sandbox` (popover baseline only; no re-record expected).
- Grep before finishing, must return nothing outside this plan: `grep -rn "ade-release-row\|ade-review-row\|mainOpen" apps/kira-space packages/theme/src`.

Run own and changed specs only: `bun run build:test:space && npx playwright test --config=apps/kira-space/playwright.config.ts --project=ui <names>`. Full suite runs once at chapter end, before P503.

### Fast checks

`bun run typecheck:space-web`, `bun run typecheck:space-tests`, `bun run lint` (catalog `--unused`, knip, check-ui-primitives). Pre-commit runs them.

No unit test: run-line ordering and the shared-repo check are one sort and one count.

## Docs

- `docs/ARCHITECTURE.md` task panel paragraph (~line 2650, P501 entry): Main lists branches first with the Plan's `AdeBranchRow` (testid `ade-task-branch`) plus edit menu and PR line (state, base, checks glyphs); workflow follows; step blocks no longer list branches (no Release or Review rows); run lines name the branch when its repo is shared.
- `SPEC.md` P502: Status Done and Result (design, specs, deviations), in the last commit.

## Commits

1. `refactor: P502 step blocks stop listing branches` — `AdeStepBlock` Release/Review rows out, `StepBlock.release` out, `mainOpen` key out; `ade-v2-run`, `ade-v2-review-open` updated.
2. `feat: P502 Main branches reuse the plan branch row` — `AdeBranchRow` testid prop, `data-depth`, slot; `AdeMainBranches` rewrite with force push dialog and PR glyph line; `PR_GLYPH`; `AdeTaskBranchMenu` edit icon and aria text; `AdeShell` selector; `AdeMainTab` order; `ade-v2-task-panel` and scoping fixes in existing specs.
3. `feat: P502 run lines name their branch` — `AdeStepBlock` target name and order.
4. `test: P502 Main tab multi-branch UI spec` — `ade-v2-main-branches.spec.ts`.
5. `docs: P502 architecture and result` — `ARCHITECTURE.md`, `SPEC.md`.

Each commit passes the hook, never `--no-verify`.

## Split

One sequential Sonnet implementer. Commits 2 and 3 share `AdeStepBlock.vue` with 1, and 4 asserts all three. No streams.

## Rationale

The user likes the task list and finds the panel confusing. The panel invented a second branch row style, put branches under a long workflow, and repeated them inside two step blocks with a third and fourth style. Reusing `AdeBranchRow` gives the panel the exact glyphs, stacking and actions the list already teaches, with no new model or backend (`card.rows` and `board/actions.ts` already supply everything). Removing the Release and Review rows leaves one place per branch; their information survives on the row (merged tag and chips, review icon). Naming the branch on a run line only where repos repeat fixes the one real ambiguity without lengthening simple tasks.
