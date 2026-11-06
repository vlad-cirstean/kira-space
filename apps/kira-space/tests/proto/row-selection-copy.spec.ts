import type { Page } from '@playwright/test';
import { installClipboardSpy, lastClipboardWrite } from '../ui/studio/support/clipboard';
import { expect, test } from './fixtures';
import { gridCell, gutterCell, selection } from './support/grid';

// Excerpt of tests/ui/interaction.spec.ts row selection: Shift range, Control toggle, gutter drag,
// row context menu on the whole selection, and the three row copy formats.
async function copyRows(page: Page, format: 'tsv' | 'csv' | 'json'): Promise<string> {
  await page.hover('[data-testid="menu-item-copy-rows"]');
  await page.click(`[data-testid="menu-item-copy-rows-${format}"]`);
  return lastClipboardWrite(page);
}

test('row selection and copy', async ({ grid }) => {
  const page = await grid();
  await installClipboardSpy(page);

  await gutterCell(page, 0).click();
  await gutterCell(page, 2).click(['Shift']); // rows [0,1,2]
  await gutterCell(page, 1).rightClick(); // inside the selection -> acts on all 3
  const allRowsTsv = await copyRows(page, 'tsv');
  expect(allRowsTsv.split('\n')).toHaveLength(3);
  expect(allRowsTsv).toContain('Customer number 1 Ltd');
  expect(allRowsTsv).toContain('Customer number 2 Ltd');
  expect(allRowsTsv).toContain('Customer number 3 Ltd');

  await gutterCell(page, 0).click(); // plain click -> replaces with [0]
  await gutterCell(page, 2).rightClick(); // outside [0] -> replaces with [2] alone
  const row2Csv = await copyRows(page, 'csv');
  expect(row2Csv.split('\n')[0]).toContain('3,');
  expect(row2Csv).toContain('Customer number 3 Ltd');

  await gutterCell(page, 0).click(); // [0]
  await gutterCell(page, 2).click(['Control']); // toggles row 2 in -> [0,2] disjoint
  expect((await selection(page)).rows).toEqual([0, 2]);
  await gutterCell(page, 0).rightClick(); // inside [0,2] -> acts on both
  const disjointJson = JSON.parse(await copyRows(page, 'json')) as Array<Record<string, unknown>>;
  expect(disjointJson).toHaveLength(2);
  expect(disjointJson.map((r) => r.name).sort()).toEqual(
    ['Customer number 1 Ltd', 'Customer number 3 Ltd'].sort(),
  );

  await gutterCell(page, 0).dragTo(gutterCell(page, 2));
  expect((await selection(page)).rows).toEqual([0, 1, 2]);
  await gutterCell(page, 1).rightClick(); // inside the dragged-out range -> acts on all 3
  const draggedRowsTsv = await copyRows(page, 'tsv');
  expect(draggedRowsTsv.split('\n')).toHaveLength(3);

  await gridCell(page, 0, 'name').click();
  expect((await selection(page)).kind).toBe('cell');
});
