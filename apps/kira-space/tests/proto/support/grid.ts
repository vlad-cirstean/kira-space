import type { Locator, Page } from '@playwright/test';
import { type CellRect, gridLocator } from 'cheetah-grid-playwright';
import '../../../frontend/proto/grid/hookTypes';

type Hook = NonNullable<Window['__kiraGridProto']>;
type HookCellState = ReturnType<Hook['cellState']>;
type HookEditor = ReturnType<Hook['editor']>;
type HookHeader = ReturnType<Hook['header']>;
type HookRow = ReturnType<Hook['rowState']>;
type HookSelection = ReturnType<Hook['selection']>;

// The prototype counterpart of tests/ui/support/grid.ts: same addressing (page row, column name),
// but a cell is a canvas region, so it resolves through cheetah-grid-playwright's own locator plus
// the debug hook for everything the library cannot know (markers, staging, selection model).

export const GRID = '[data-testid="data-grid"]';

const grid = (page: Page) => gridLocator(page.locator(GRID));

async function recordOf(page: Page, pageRow: number): Promise<number> {
  const record = await page.evaluate(
    (r) => window.__kiraGridProto?.recordIndex(r) ?? null,
    pageRow,
  );
  if (record === null) throw new Error(`page row ${pageRow} is not on display`);
  return record;
}

function center(rect: CellRect): [number, number] {
  return [rect.x + rect.width / 2, rect.y + rect.height / 2];
}

interface Clickable {
  rect(): Promise<CellRect>;
}

async function press(
  page: Page,
  target: Clickable,
  options: { button?: 'left' | 'right'; dx?: number; modifiers?: ('Shift' | 'Control')[] } = {},
): Promise<void> {
  const rect = await target.rect();
  const [x, y] = center(rect);
  for (const key of options.modifiers ?? []) await page.keyboard.down(key);
  await page.mouse.click(options.dx === undefined ? x : rect.x + options.dx, y, {
    button: options.button ?? 'left',
  });
  for (const key of options.modifiers ?? []) await page.keyboard.up(key);
}

export interface ProtoCell {
  click(modifiers?: ('Shift' | 'Control')[]): Promise<void>;
  dblclick(): Promise<void>;
  rightClick(): Promise<void>;
  hover(): Promise<void>;
  /** Click `dx` CSS px from the cell's left edge. */
  clickAt(dx: number): Promise<void>;
  rect(): Promise<CellRect>;
  fill(value: string): Promise<void>;
  state(): Promise<HookCellState>;
  /** Displayed text, `NULL` for a null cell. */
  text(): Promise<string>;
}

/** A data cell, addressed by page row and column name. */
export function gridCell(page: Page, row: number, column: string): ProtoCell {
  const locate = async () => grid(page).cell(column, await recordOf(page, row));
  const target: Clickable = { rect: async () => (await locate()).rect() };
  const state = (): Promise<HookCellState> =>
    page.evaluate(([r, c]) => window.__kiraGridProto?.cellState(r as number, c as string), [
      row,
      column,
    ] as const) as Promise<HookCellState>;
  return {
    click: (modifiers) => press(page, target, { modifiers }),
    dblclick: async () => (await locate()).dblclick(),
    rightClick: () => press(page, target, { button: 'right' }),
    hover: async () => {
      const [x, y] = center(await target.rect());
      await page.mouse.move(x, y);
    },
    clickAt: (dx) => press(page, target, { dx }),
    rect: () => target.rect(),
    fill: async (value) => (await locate()).fill(value),
    state,
    text: async () => (await state()).text,
  };
}

export function cellText(page: Page, row: number, column: string): Promise<string> {
  return gridCell(page, row, column).text();
}

export interface ProtoGutter {
  click(modifiers?: ('Shift' | 'Control')[]): Promise<void>;
  rightClick(): Promise<void>;
  rect(): Promise<CellRect>;
  /** Press on this gutter cell, drag to another page row's gutter, release. */
  dragTo(other: ProtoGutter): Promise<void>;
}

export function gutterCell(page: Page, row: number): ProtoGutter {
  const target: Clickable = {
    rect: async () =>
      grid(page)
        .cellAt(0, (await recordOf(page, row)) + 1)
        .rect(),
  };
  return {
    click: (modifiers) => press(page, target, { modifiers }),
    rightClick: () => press(page, target, { button: 'right' }),
    rect: () => target.rect(),
    dragTo: async (other) => {
      const [x1, y1] = center(await target.rect());
      const [x2, y2] = center(await other.rect());
      await page.mouse.move(x1, y1);
      await page.mouse.down();
      await page.mouse.move(x2, y2, { steps: 6 });
      await page.mouse.up();
    },
  };
}

export interface ProtoHeader {
  click(modifiers?: ('Shift' | 'Control')[]): Promise<void>;
  rightClick(): Promise<void>;
  /** Click the sort arrow strip at the header's right edge. */
  clickSort(modifiers?: ('Shift' | 'Control')[]): Promise<void>;
  hover(): Promise<void>;
  rect(): Promise<CellRect>;
  state(): Promise<HookHeader>;
}

export function headerCell(page: Page, column: string): ProtoHeader {
  const target: Clickable = {
    rect: async () => {
      const col = await page.evaluate((c) => window.__kiraGridProto?.gridCol(c) ?? null, column);
      if (col === null) throw new Error(`column ${column} is hidden`);
      return grid(page).cellAt(col, 0).rect();
    },
  };
  return {
    click: (modifiers) => press(page, target, { modifiers }),
    rightClick: () => press(page, target, { button: 'right' }),
    clickSort: async (modifiers) => {
      const rect = await target.rect();
      await press(page, target, { dx: rect.width - 6, modifiers });
    },
    hover: async () => {
      const [x, y] = center(await target.rect());
      await page.mouse.move(x, y);
    },
    rect: () => target.rect(),
    state: () =>
      page.evaluate((c) => window.__kiraGridProto?.header(c), column) as Promise<HookHeader>,
  };
}

/** Page row shown at a record position (sort and hide reorder these). */
export function pageRowAt(page: Page, record: number): Promise<number> {
  return page.evaluate((r) => window.__kiraGridProto?.pageRow(r) ?? -1, record);
}

export function rowState(page: Page, row: number): Promise<HookRow> {
  return page.evaluate((r) => window.__kiraGridProto?.rowState(r), row) as Promise<HookRow>;
}

export function selection(page: Page): Promise<HookSelection> {
  return page.evaluate(() => window.__kiraGridProto?.selection()) as Promise<HookSelection>;
}

export function editorState(page: Page): Promise<HookEditor> {
  return page.evaluate(() => window.__kiraGridProto?.editor()) as Promise<HookEditor>;
}

export function insertRowCount(page: Page): Promise<number> {
  return page.evaluate(() => window.__kiraGridProto?.insertRowCount() ?? 0);
}

/** Columns currently sorted, in term order. */
export async function sortedColumns(page: Page, columns: readonly string[]): Promise<string[]> {
  const headers = await Promise.all(
    columns.map(async (c) => [c, await headerCell(page, c).state()] as const),
  );
  return headers
    .filter(([, h]) => h.sortOrder !== null)
    .sort((a, b) => (a[1].sortOrder as number) - (b[1].sortOrder as number))
    .map(([c]) => c);
}

export interface NavButton {
  kind: 'fk' | 'pk' | null;
  rect: CellRect | null;
  click(): Promise<void>;
}

/** The FK/PK glyph of one cell: its hit area as the hook reports it. */
export async function navButton(page: Page, row: number, column: string): Promise<NavButton> {
  const cell = gridCell(page, row, column);
  await cell.rect();
  const state = await cell.state();
  const rect = state.navRect && {
    x: state.navRect.x,
    y: state.navRect.y,
    width: state.navRect.width,
    height: state.navRect.height,
  };
  return {
    kind: state.nav,
    rect,
    click: () => cell.clickAt(12),
  };
}

export function fkPreview(page: Page): Locator {
  return page.locator('[data-testid="fk-preview"]');
}

export const GRID_SCROLLER_SELECTOR = `${GRID} .cheetah-grid .grid-scrollable`;
