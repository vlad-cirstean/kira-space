import { expect, test } from './fixtures';
import { headerCell, pageRowAt, sortedColumns } from './support/grid';

// Excerpt of tests/ui/interaction.spec.ts header menu (sort-asc, sort-desc, clear-sort) and
// data-view.spec.ts's sort-arrow cycle, plus a shift-click second term with badge order 1, 2.
const COLUMNS = ['id', 'name', 'qty'];

test('header sort: menu, arrow cycle, second term', async ({ grid }) => {
  const page = await grid();

  await headerCell(page, 'name').rightClick();
  await page.click('[data-testid="menu-item-sort-desc"]');
  expect((await headerCell(page, 'name').state()).sort).toBe('desc');
  expect(await pageRowAt(page, 0)).toBe(9998); // 'Customer number 9999 Ltd' sorts last as text
  await headerCell(page, 'name').rightClick();
  await page.click('[data-testid="menu-item-clear-sort"]');
  expect((await headerCell(page, 'name').state()).sort).toBeNull();

  await headerCell(page, 'qty').clickSort();
  expect((await headerCell(page, 'qty').state()).sort).toBe('asc');
  await headerCell(page, 'qty').clickSort();
  expect((await headerCell(page, 'qty').state()).sort).toBe('desc');
  await headerCell(page, 'qty').clickSort();
  expect((await headerCell(page, 'qty').state()).sort).toBeNull();

  await headerCell(page, 'name').clickSort();
  await headerCell(page, 'qty').clickSort(['Shift']);
  expect(await sortedColumns(page, COLUMNS)).toEqual(['name', 'qty']);
  expect((await headerCell(page, 'name').state()).sortOrder).toBe(1);
  expect((await headerCell(page, 'qty').state()).sortOrder).toBe(2);
});
