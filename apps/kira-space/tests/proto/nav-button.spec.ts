import { expect, test } from './fixtures';
import { fkPreview, gridCell, navButton } from './support/grid';

// Excerpt of tests/ui/slick-grid.spec.ts "P22 Pass B C11 T9": a nav button on every nav cell,
// none off a nav column, kind recorded, left offset within 24 px; plus the FK preview opening
// anchored to its cell.
test('nav glyph per nav cell, absent off one, FK preview anchors to the cell', async ({ grid }) => {
  const page = await grid();

  for (const row of [0, 1, 2]) {
    const fk = await navButton(page, row, 'country');
    expect(fk.kind).toBe('fk');
    expect(fk.rect?.width).toBeLessThanOrEqual(24);
    const pk = await navButton(page, row, 'id');
    expect(pk.kind).toBe('pk');
  }
  expect((await navButton(page, 0, 'name')).kind).toBeNull();

  const fk = await navButton(page, 1, 'country');
  const cellRect = await gridCell(page, 1, 'country').rect();
  await fk.click();
  await expect(fkPreview(page)).toBeVisible();
  const preview = await fkPreview(page).boundingBox();
  expect(preview).not.toBeNull();
  const left = preview?.x ?? 0;
  const right = left + (preview?.width ?? 0);
  expect(left).toBeLessThanOrEqual(cellRect.x + cellRect.width); // overlaps the cell's columns
  expect(right).toBeGreaterThanOrEqual(cellRect.x);
  expect(Math.abs((preview?.y ?? 0) - (cellRect.y + cellRect.height))).toBeLessThanOrEqual(24);
});
