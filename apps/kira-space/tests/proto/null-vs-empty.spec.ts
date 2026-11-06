import { expect, test } from './fixtures';
import { gridCell } from './support/grid';

// Excerpt of tests/ui/data-view.spec.ts "NULL vs ''": a NULL cell and an empty-string cell look
// different. Row 12 `email` is NULL, row 16 is ''.
test("NULL vs ''", async ({ grid }) => {
  const page = await grid();
  const nullCell = gridCell(page, 12, 'email');
  const emptyCell = gridCell(page, 16, 'email');
  expect((await nullCell.state()).isNull).toBe(true);
  expect(await nullCell.text()).toBe('NULL');
  expect((await emptyCell.state()).isNull).toBe(false);
  expect(await emptyCell.text()).toBe('');
});
