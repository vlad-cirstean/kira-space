import type { Page } from '@playwright/test';
import { installClipboardSpy, lastClipboardWrite } from '../ui/studio/support/clipboard';
import { expect, test } from './fixtures';
import {
  GRID,
  gridCell,
  gutterCell,
  headerCell,
  insertRowCount,
  navButton,
  rowState,
  selection,
} from './support/grid';

// One step per prototype parity item (plan §11): the "verified by trying it" evidence behind each
// verdict in the P165 Result. A step that throws is a feature the prototype does not reproduce.

const colWidth = (page: Page, col: number) =>
  page.evaluate(
    ([selector, c]) => {
      const ns = (
        window as unknown as {
          cheetahGrid: {
            ListGrid: {
              getInstanceByElement(el: Element | null): { getColWidth(col: number): number };
            };
          };
        }
      ).cheetahGrid;
      const el = document.querySelector(`${selector} .cheetah-grid`);
      return ns.ListGrid.getInstanceByElement(el).getColWidth(c);
    },
    [GRID, col] as const,
  );

const tooltip = (page: Page) => page.locator('[data-slot="tooltip-content"]');

test('parity', async ({ grid }) => {
  const page = await grid();
  await installClipboardSpy(page);

  await test.step('NULL vs empty, masked, type colour and alignment', async () => {
    expect((await gridCell(page, 12, 'email').state()).isNull).toBe(true);
    expect((await gridCell(page, 16, 'email').state()).isNull).toBe(false);
    expect((await gridCell(page, 0, 'email').state()).masked).toBe(true);
    expect((await gridCell(page, 0, 'id').state()).align).toBe('right');
    expect((await gridCell(page, 0, 'name').state()).align).toBe('left');
    expect((await gridCell(page, 0, 'qty').state()).category).toBe('numeric');
  });

  await test.step('truncated marker and its tooltip', async () => {
    expect((await gridCell(page, 499, 'notes').state()).truncated).toBe(true);
    await gridCell(page, 499, 'notes').hover();
    await expect(tooltip(page).first()).toContainText('truncated');
    await gridCell(page, 0, 'name').hover();
    await expect(tooltip(page)).toHaveCount(0);
  });

  await test.step('cell and range selection, keyboard navigation', async () => {
    await gridCell(page, 1, 'name').click();
    expect((await selection(page)).kind).toBe('cell');
    await page.keyboard.press('ArrowDown');
    expect((await selection(page)).active).toEqual({ row: 2, column: 'name' });
    await page.keyboard.press('ArrowRight');
    expect((await selection(page)).active).toEqual({ row: 2, column: 'email' });
    await page.keyboard.press('Shift+ArrowDown');
    expect((await selection(page)).kind).toBe('range');
    await gridCell(page, 5, 'name').click(['Shift']);
    expect((await selection(page)).rows).toEqual([2, 3, 4, 5]); // extends from the keyboard-moved focus cell
  });

  await test.step('column select, select all, hover row', async () => {
    await headerCell(page, 'qty').click();
    const byColumn = await selection(page);
    expect(byColumn.kind).toBe('column');
    expect(byColumn.columns).toEqual(['qty']);
    await page.keyboard.press('Control+a');
    expect((await selection(page)).kind).toBe('all');
    await gridCell(page, 3, 'name').hover();
    expect((await rowState(page, 3)).hovered).toBe(true);
  });

  await test.step('copy: TSV through the real clipboard path', async () => {
    await gridCell(page, 1, 'name').click();
    await gridCell(page, 2, 'name').click(['Shift']);
    await page.keyboard.press('Control+c');
    await expect
      .poll(() => lastClipboardWrite(page))
      .toBe('Customer number 2 Ltd\nCustomer number 3 Ltd');
  });

  await test.step('paste: a multi-cell block stages through parseDelimited', async () => {
    await gridCell(page, 6, 'name').click();
    await page.evaluate(() => {
      const target = document.querySelector('.grid-focus-control') as HTMLElement;
      const data = new DataTransfer();
      data.setData('text/plain', 'Pasted A\nPasted B');
      target.dispatchEvent(
        new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }),
      );
    });
    await expect.poll(() => gridCell(page, 6, 'name').text()).toBe('Pasted A');
    expect(await gridCell(page, 7, 'name').text()).toBe('Pasted B');
  });

  await test.step('staged value, pending rail, row numbers', async () => {
    await gridCell(page, 8, 'qty').fill('777');
    expect((await gridCell(page, 8, 'qty').state()).staged).toBe(true);
    expect((await rowState(page, 8)).dirty).toBe(true);
    expect((await rowState(page, 8)).gutterLabel).toBe('9');
  });

  await test.step('delete row marks the rail', async () => {
    await gutterCell(page, 9).rightClick();
    await page.click('[data-testid="menu-item-delete-row"]');
    expect((await rowState(page, 9)).deleted).toBe(true);
  });

  await test.step('insert row: one editor, value staged per keystroke', async () => {
    await page.click('[data-testid="add-row"]');
    expect(await insertRowCount(page)).toBe(1);
    await page.keyboard.type('Typed');
    const inserted = await page.evaluate(() => window.__kiraGridProto?.rowState(10000));
    expect(inserted?.inserted).toBe(true);
    expect(inserted?.gutterLabel).toBe('+');
    await expect.poll(async () => (await gridCell(page, 10000, 'id').state()).text).toBe('Typed');
  });

  await test.step('context menus: cell, range, row, header', async () => {
    const ids = async () =>
      page
        .locator('[data-testid^="menu-item-"]')
        .evaluateAll((els) =>
          els.map((el) => el.getAttribute('data-testid')?.replace('menu-item-', '')),
        );
    await gridCell(page, 20, 'name').rightClick();
    expect(await ids()).toEqual(
      expect.arrayContaining([
        'copy',
        'copy-with-header',
        'copy-as-json',
        'paste',
        'edit',
        'set-null',
      ]),
    );
    await page.keyboard.press('Escape');
    await gridCell(page, 20, 'name').click();
    await gridCell(page, 22, 'qty').click(['Shift']);
    await gridCell(page, 21, 'name').rightClick();
    expect(await ids()).toEqual(expect.arrayContaining(['copy', 'copy-as-csv', 'copy-as-json']));
    await page.keyboard.press('Escape');
    await gutterCell(page, 20).click();
    await gutterCell(page, 20).rightClick();
    expect(await ids()).toContain('copy-rows');
    await page.keyboard.press('Escape');
    await headerCell(page, 'name').rightClick();
    expect(await ids()).toEqual(
      expect.arrayContaining([
        'sort-asc',
        'sort-desc',
        'clear-sort',
        'hide-column',
        'show-all-columns',
        'copy-column-name',
        'copy-column-values',
      ]),
    );
    await page.keyboard.press('Escape');
  });

  await test.step('column resize keeps the header-aware floor', async () => {
    const before = await colWidth(page, 3);
    const rect = await headerCell(page, 'name').rect();
    await page.mouse.move(rect.x + rect.width - 1, rect.y + rect.height / 2);
    await page.mouse.down();
    await page.mouse.move(rect.x - 400, rect.y + rect.height / 2, { steps: 8 });
    await page.mouse.up();
    const after = await colWidth(page, 3);
    expect(after).toBeLessThan(before);
    expect(after).toBeGreaterThanOrEqual(40);
  });

  await test.step('column hide and show-all at data level', async () => {
    await headerCell(page, 'ratio').rightClick();
    await page.click('[data-testid="menu-item-hide-column"]');
    expect(await page.evaluate(() => window.__kiraGridProto?.gridCol('ratio'))).toBeNull();
    await headerCell(page, 'name').rightClick();
    await page.click('[data-testid="menu-item-show-all-columns"]');
    expect(await page.evaluate(() => window.__kiraGridProto?.gridCol('ratio'))).not.toBeNull();
  });

  await test.step('PK/FK header badge and header tooltip', async () => {
    expect((await headerCell(page, 'id').state()).key).toBe('PK');
    expect((await headerCell(page, 'country').state()).key).toBe('FK');
    expect((await headerCell(page, 'name').state()).key).toBeNull();
    await headerCell(page, 'qty').hover();
    await expect(tooltip(page).first()).toContainText('qty');
  });

  await test.step('FK preview, PK glyph', async () => {
    expect((await navButton(page, 1, 'country')).kind).toBe('fk');
    await (await navButton(page, 1, 'country')).click();
    await expect(page.locator('[data-testid="fk-preview"]')).toBeVisible();
    await page.keyboard.press('Escape');
    expect((await navButton(page, 1, 'id')).kind).toBe('pk');
  });

  await test.step('find, go to match, hide non-matching', async () => {
    await page.fill('[data-testid="find-input"]', 'number 42');
    await page.press('[data-testid="find-input"]', 'Enter');
    await expect(page.locator('[data-testid="find-count"]')).not.toHaveText('0');
    await page.click('[data-testid="find-next"]');
    const active = (await selection(page)).active;
    expect(active?.column).toBe('name');
    expect((await gridCell(page, active?.row ?? 0, 'name').state()).search).toBe('current');
    await page.click('[data-testid="find-hide"]');
    expect(await page.evaluate(() => window.__kiraGridProto?.recordIndex(0))).toBeNull();
  });

  await test.step('live re-theme repaints without reload', async () => {
    const sample = () =>
      page.evaluate((selector) => {
        const canvas = document.querySelector(`${selector} canvas`) as HTMLCanvasElement;
        const px = canvas.getContext('2d')?.getImageData(canvas.width - 40, 100, 1, 1).data;
        return px ? Array.from(px).join(',') : '';
      }, GRID);
    const before = await sample();
    await page.evaluate(() => document.documentElement.setAttribute('data-proto-theme', 'alt'));
    await expect.poll(sample).not.toBe(before);
  });

  await test.step('Playwright drive through the library', async () => {
    const { gridLocator } = await import('cheetah-grid-playwright');
    const cell = gridLocator(page.locator(GRID)).cell('id', 0);
    expect(await cell.value()).toMatchObject({ text: expect.any(String) });
  });
});
