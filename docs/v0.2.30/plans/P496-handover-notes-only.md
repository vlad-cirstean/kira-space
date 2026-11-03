# P496 plan: ADE handover only in Notes

Row: `docs/v0.2.30/SPEC.md` P496. Base: `2b86256c`. Frontend only; IPC unchanged, so no backend flow test.

## Findings (codegraph)

- `apps/kira-space/frontend/src/ade/v2/panel/AdeTaskTab.vue` is the only place showing a handover in the Task tab:
  - script lines 63-73: `// ---- handover` block, `currentStepId` and `handoverSnippet` computeds.
  - template lines 177-190: `<template v-if="!review && handoverSnippet">` with `<dt>` `space.ade.taskTab.handover.label` and the `ade-task-handover-row` button calling `tabs.setTaskTab(task.id, 'notes')`.
  - line 26 imports `useAdePanelTabsStore`; line 41 `const tabs = useAdePanelTabsStore();`. `tabs` has no other use in the file (only line 184). `currentStepId` has no other use (lines 69, 72 only).
- Notes tab already renders `AdeHandoverList` (`AdeTaskPanel.vue` line 211, testid `ade-handovers`). Archived panel renders it too (`AdeArchivedPanel.vue`). Neither changes.
- `task.handoverIn` / `task.handoverDrafts` stay in use elsewhere (`dialog/useDialogCtx.ts`, `state/adeStageDialog.ts`, `run/AdeRunDialog.vue`). Wire types untouched.
- i18n key `space.ade.taskTab.handover.label` ("Handover") in `packages/theme/src/i18n/en.json` is used only by `AdeTaskTab.vue` line 178. It becomes unused; `bun scripts/copy/check-catalog.ts --unused` (in `bun run lint`) would fail if left. Other `handover` keys (`space.ade.handover.*`, `space.ade.runDialog.handover.*`) stay.
- Phone (`frontend/mobile`) has no handover code. Unaffected.
- Only spec referencing `ade-task-handover-row`: `apps/kira-space/tests/ui/ade-v2-handover.spec.ts` line 153, test `the Notes tab lists the draft and the stage moves; an edit sends UpdateHandover`.
- No doc under `site/` or `docs/` describes the Task-tab handover row. `docs/ARCHITECTURE.md` needs no change.

## Library rules

Pure removal. No new UI surface, primitive, composable, store or query. Removing the last `useAdePanelTabsStore` use in `AdeTaskTab.vue` drops a store dependency; the store itself stays (used by `AdeTaskPanel.vue`, `adeBoardUi.ts`). Component stays `<script setup lang="ts">`.

## Changes

1. `apps/kira-space/frontend/src/ade/v2/panel/AdeTaskTab.vue`
   - Delete script block lines 63-73 (`// ---- handover`, `currentStepId`, `handoverSnippet`).
   - Delete template lines 177-190 (handover `<template>`).
   - Delete `import { useAdePanelTabsStore } ...` (line 26) and `const tabs = ...` (line 41).
   - Header comment (line 36) unchanged: already says "editable fields, then the branches".
2. `packages/theme/src/i18n/en.json`: delete `space.ade.taskTab.handover` (`{"label": "Handover"}`). Touch nothing else in the file. Key types derive from `en.json` (`types.ts` `MessageKey`); no generated file to update, and typecheck flags any leftover caller.
3. `apps/kira-space/tests/ui/ade-v2-handover.spec.ts`, test at line 147:
   - Replace `await expect(page.locator(t('ade-task-handover-row'))).toContainText(BODY);` with a Task-tab absence check, while the Task tab is open (default tab after `openTask`):
     `await expect(page.locator(t('ade-task-tab'))).toBeVisible();`
     `await expect(page.locator(t('ade-task-tab'))).not.toContainText(BODY);`
     `await expect(page.locator(t('ade-task-handover-row'))).toHaveCount(0);`
   - Rest of the test (Notes tab lists draft and handed rows, edit sends `UpdateHandover`) stays; it proves "still in Notes".
   - Rename test to `the Task tab shows no handover; the Notes tab lists the draft and the stage moves; an edit sends UpdateHandover` (or similar, under ~100 chars).
   - `toBeVisible` before `not.toContainText` keeps the negative assertion from passing on an unrendered tab.

One commit: `feat: P496 handover only in Notes` (Conventional Commits, `feat!` not needed). Optional second commit only for SPEC result.

## Checks

- Fast: `bun run typecheck:space-web`, `bun run typecheck:space-tests`, `bun run lint` (covers `check-catalog --unused`). Pre-commit hook runs these anyway.
- Own spec only: `bun run build:test:space && npx playwright test --config=apps/kira-space/playwright.config.ts --project=ui ade-v2-handover`.
- Grep after: `grep -rn "handoverSnippet\|ade-task-handover-row\|taskTab.handover" apps packages` returns nothing.
- Full UI suite is not run here (once per chapter, after P502).

## SPEC result

Mark P496 Done in `docs/v0.2.30/SPEC.md` with a one-line Result: Task-tab handover row, its computeds and `space.ade.taskTab.handover` key removed; Notes keeps `AdeHandoverList`; `ade-v2-handover` asserts absence on Task tab and presence in Notes.

## File ownership and overlap

P496 owns:
- `apps/kira-space/frontend/src/ade/v2/panel/AdeTaskTab.vue`
- `apps/kira-space/tests/ui/ade-v2-handover.spec.ts`
- `packages/theme/src/i18n/en.json`: key `space.ade.taskTab.handover` only (deletion)
- `docs/v0.2.30/SPEC.md`: row P496 Result only

Overlap:
- P497 (`sessions/AdeTaskLaunchMenu.vue`, `packages/workbench/src/automations/*`, new shared component under `packages/workbench`, specs `ade-v2-task-launch`, `ade-v2-launch`, Runs spec): no common code file. Shared file `en.json` only, disjoint keys (P496 deletes `space.ade.taskTab.handover`; P497 adds/removes keys only in its launch/automations namespaces). `SPEC.md` row-local edits. No ordering dependency either way. Safe as a parallel stream if the orchestrator chooses; on rebase, `en.json` and `SPEC.md` hunks are at different lines.
- P500 (`plan/AdeTaskCard.vue`, `AdeActionCell.vue`, `AdeBranchRow.vue`, `board/actions.ts`, card specs): no common file except `SPEC.md` row-local and possibly `en.json` (disjoint keys). No ordering dependency. SPEC says P500 runs in sequence (shares `board/actions.ts` with P495, not with P496).
- P501 (task panel restructure: `panel/AdeTaskPanel.vue`, `panel/AdeTaskTab.vue`, sibling tabs) and P502 (`AdeTaskPanel.vue`, Main tab) touch `AdeTaskTab.vue` after P496. They must build on the P496 tree: no handover row in Task/Main tab; handovers live in Notes, which P501 merges into Details. P501's plan must keep "handover only in Notes/Details, never in Main". P496 leaves `AdeTaskPanel.vue` untouched, so no conflict there.
