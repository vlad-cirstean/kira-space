import { afterAll, beforeAll, beforeEach, describe, expect, test } from 'bun:test';
import {
  createTabularPageBuilder,
  unpagedPosition,
} from '../../../../../packages/shared/protocol/page';
import { initialWidths, resetMeasureCtx } from '../../../frontend/src/studio/views/shared/page/columns';

// DataGrid.vue's `widths` computed depends on the tab's stored columnWidths, so
// every pointermove of a column-resize drag used to re-run initialWidths' canvas-measurement pass
// over every column and up to 50 sample rows each — real work, repeated on every pixel of mouse
// movement, even though a resize never changes the page's own data. initialWidths now memoizes its
// result by page object identity (the page is frozen and only replaced by a new fetch — the
// `page` computed comment) — these tests prove that with a real TabularPage and a measureText call
// counter standing in for "the expensive pass ran again", no DOM/browser required: initialWidths
// and resetMeasureCtx only ever reach the DOM through document.createElement('canvas') and
// getComputedStyle(document.documentElement), both stubbed below.

let measureTextCalls = 0;
const originalDocument = (globalThis as { document?: unknown }).document;
const originalGetComputedStyle = (globalThis as { getComputedStyle?: unknown }).getComputedStyle;

beforeAll(() => {
  const fakeCtx = {
    font: '',
    measureText(text: string) {
      measureTextCalls++;
      return { width: text.length * 7 } as TextMetrics;
    },
  };
  const fakeCanvas = { getContext: (kind: string) => (kind === '2d' ? fakeCtx : null) };
  (globalThis as { document?: unknown }).document = {
    createElement: (tag: string) => (tag === 'canvas' ? fakeCanvas : {}),
    documentElement: {},
  };
  (globalThis as { getComputedStyle?: unknown }).getComputedStyle = () => ({
    getPropertyValue: () => '',
  });
});

afterAll(() => {
  (globalThis as { document?: unknown }).document = originalDocument;
  (globalThis as { getComputedStyle?: unknown }).getComputedStyle = originalGetComputedStyle;
});

beforeEach(() => {
  measureTextCalls = 0;
  resetMeasureCtx();
});

function buildPage(rows: (string | null)[][]) {
  const columns = [
    {
      name: 'id',
      dataType: 'int4',
      typeClass: 'number' as const,
      nullable: false,
      isPrimaryKey: true,
      generated: false,
    },
    {
      name: 'name',
      dataType: 'text',
      typeClass: 'text' as const,
      nullable: true,
      isPrimaryKey: false,
      generated: false,
    },
  ];
  const builder = createTabularPageBuilder(columns);
  for (const row of rows) builder.appendRow(row);
  return builder.finish(unpagedPosition(rows.length));
}

describe('initialWidths caching (P2 R1)', () => {
  test('1. a fresh page measures every column', () => {
    const page = buildPage([['1', 'alice']]);
    initialWidths(page);
    expect(measureTextCalls).toBeGreaterThan(0);
  });

  test('2. calling it again with the same page object is a cache hit: no further measuring', () => {
    const page = buildPage([['1', 'alice']]);
    const first = initialWidths(page);
    measureTextCalls = 0;

    const second = initialWidths(page);

    expect(measureTextCalls).toBe(0);
    expect(second).toBe(first); // same reference, not just equal — proves it's the cached result
  });

  test('3. repeated calls (simulating pointermove during a resize drag) stay cached', () => {
    const page = buildPage([
      ['1', 'alice'],
      ['2', 'bob'],
      ['3', 'carol'],
    ]);
    initialWidths(page);
    measureTextCalls = 0;

    for (let i = 0; i < 50; i++) initialWidths(page);

    expect(measureTextCalls).toBe(0);
  });

  test('4. a different page object is measured fresh, not served from the old cache', () => {
    const pageA = buildPage([['1', 'alice']]);
    const pageB = buildPage([['2', 'a much longer name than the first page had']]);
    initialWidths(pageA);
    measureTextCalls = 0;

    const widthsB = initialWidths(pageB);

    expect(measureTextCalls).toBeGreaterThan(0);
    expect(widthsB.name).toBeGreaterThan(initialWidths(pageA).name);
  });

  test('5. resetMeasureCtx (a font change) invalidates the cache too', () => {
    const page = buildPage([['1', 'alice']]);
    initialWidths(page);

    resetMeasureCtx();
    measureTextCalls = 0;
    initialWidths(page);

    expect(measureTextCalls).toBeGreaterThan(0);
  });
});
