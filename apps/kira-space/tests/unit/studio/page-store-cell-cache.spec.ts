// Every page/*.ts row/cell accessor built on top of views/shared/page/store.ts's
// `cached()` (memoized decoded text) still allocated a brand-new return object on *every* call,
// even for the same already-decoded row — and grid/page.ts's `cell()` in particular is called
// several times per cell per render straight from a template (ConsoleResultGrid.vue, DataGrid.vue).
// `cachedView` fixes that by memoizing the accessor's own built object the same way `cached`
// memoizes decoded text. This exercises `cachedView` directly (the shared primitive every
// page/*.ts accessor now goes through) plus grid/page.ts's `cell()` as one real caller.
import { describe, expect, test } from 'bun:test';
import {
  type ColumnDescriptor,
  createTabularPageBuilder,
  unpagedPosition,
} from '@shared/protocol/page';

const { createPageStore } = await import('../../../frontend/src/studio/views/shared/page/store');
const gridPage = await import('../../../frontend/src/studio/views/grid/page');

describe('page store cachedView (P2 R2 #99)', () => {
  test('1. a repeat call for the same row/subKey returns the identical object, not a new one', () => {
    const store = createPageStore<{ rowCount: number; byteSize: number }>();
    store.setPage('scope', { rowCount: 1, byteSize: 0 });

    let builds = 0;
    const build = () => {
      builds++;
      return { text: 'hello' };
    };

    const first = store.cachedView('scope', 0, 'k', build);
    const second = store.cachedView('scope', 0, 'k', build);

    expect(second).toBe(first);
    expect(builds).toBe(1);
  });

  test('2. a different subKey or row gets its own independent cache slot', () => {
    const store = createPageStore<{ rowCount: number; byteSize: number }>();
    store.setPage('scope', { rowCount: 2, byteSize: 0 });

    const a = store.cachedView('scope', 0, 'a', () => ({ v: 'a' }));
    const b = store.cachedView('scope', 0, 'b', () => ({ v: 'b' }));
    const row1 = store.cachedView('scope', 1, 'a', () => ({ v: 'row1' }));

    expect(a).not.toBe(b);
    expect(a).not.toBe(row1);
  });

  test('3. setPage (a new page landing) invalidates every previously cached view', () => {
    const store = createPageStore<{ rowCount: number; byteSize: number }>();
    store.setPage('scope', { rowCount: 1, byteSize: 0 });
    const before = store.cachedView('scope', 0, 'k', () => ({ n: 1 }));

    store.setPage('scope', { rowCount: 1, byteSize: 0 });
    const after = store.cachedView('scope', 0, 'k', () => ({ n: 2 }));

    expect(after).not.toBe(before);
  });

  test('4. setVisibleWindow prunes cached views outside the window, same as the decode cache', () => {
    const store = createPageStore<{ rowCount: number; byteSize: number }>();
    store.setPage('scope', { rowCount: 3, byteSize: 0 });
    const row0 = store.cachedView('scope', 0, 'k', () => ({ n: 0 }));

    store.setVisibleWindow('scope', 1, 3);
    const row0Again = store.cachedView('scope', 0, 'k', () => ({ n: 0 }));

    // Pruned out of the window: a fresh object, not the one built before the window moved past it.
    expect(row0Again).not.toBe(row0);
  });

  test("5. grid/page.ts's cell() returns the same CellView reference across repeat calls (the exact template-facing bug: ConsoleResultGrid/DataGrid call cellAt() several times per cell per render)", () => {
    const columns: ColumnDescriptor[] = [
      {
        name: 'name',
        dataType: 'text',
        typeClass: 'text',
        nullable: false,
        isPrimaryKey: false,
        generated: false,
      },
    ];
    const builder = createTabularPageBuilder(columns);
    builder.appendRow(['widget']);
    const page = builder.finish(unpagedPosition(1));

    gridPage.setPage('tab-1', page);

    const first = gridPage.cell('tab-1', 0, 0);
    const second = gridPage.cell('tab-1', 0, 0);

    expect(second).toBe(first);
    expect(first.text).toBe('widget');
  });
});
