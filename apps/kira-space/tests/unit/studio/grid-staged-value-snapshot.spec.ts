// SlickGridHost.vue's own header states the rule ("Vue must
// not see the grid" — the cell extractor runs *during* SlickGrid's own synchronous render), but
// createDisplayValueExtractor read pending edits/inserts through pendingFor()/stagedValue() inside
// its per-cell closure — pendingState[tabId] is a reactive proxy, its `edits` a reactive-wrapped
// Map, so every cell paid up to four proxy traps once the user had staged even one edit.
// rawPendingFor unwraps the same underlying object via toRaw. It is called *inside* the returned
// per-cell closure (once per cell, as before the fix) rather than once at build time: an earlier
// version of this fix snapshotted it once at build time on the theory that SlickGridHost.vue
// always rebuilds the extractor on every staging change, which is false (its own watch
// invalidates/re-renders the touched rows on a stageEdit/stageNull/stageDelete without ever
// rebuilding this extractor — see dataSource.ts's own corrected comment) and was a real regression
// caught by tests/ui/mutations.spec.ts, tests/ui/cell-editor.spec.ts and
// tests/ui/interaction.spec.ts. Reading through toRaw still removes 3 of the original 4 proxy traps
// per cell (the Map wrapper, its `.get()` result wrapper, and the `.changes` field wrapper) — only
// the one unavoidable top-level `pendingState[tabId]` read stays live, which is what correctness
// here requires.
//
// pendingChanges.ts transitively reaches bridge/data.ts -> '/wails/runtime.js' at module scope,
// hence the dynamic import after @workbench/testing/unit/window's mock.module registration —
// the same pattern row-values-visible-span.spec.ts already uses for the same reason.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import { setActivePinia } from 'pinia';
import { isReactive } from 'vue';
import {
  createTabularPageBuilder,
  type TabularPage,
  unpagedPosition,
} from '../../../../../packages/shared/protocol/page';
import { pinia } from '../../../frontend/src/studio/state/pinia';

setActivePinia(pinia);

const { usePendingChangesStore } = await import(
  '../../../frontend/src/studio/views/grid/pendingChanges'
);
const { addInsertRow, clearPending, rawPendingFor, stageEdit, stageInsertValue, stageNull } =
  usePendingChangesStore();
const { createDisplayValueExtractor, pendingRowClasses } = await import(
  '../../../frontend/src/studio/views/grid/slick/dataSource'
);

const TAB = 'tab-staged-snapshot';

// Staging is keyed by primary key, so the tab needs a loaded page that carries one: row n has id n.
const { setPage } = await import('../../../frontend/src/studio/views/grid/page');
{
  const builder = createTabularPageBuilder([
    {
      name: 'id',
      dataType: 'text',
      typeClass: 'text',
      nullable: false,
      isPrimaryKey: true,
      generated: false,
    },
  ]);
  for (let i = 0; i < 101; i++) builder.appendRow([String(i)]);
  setPage(TAB, builder.finish(unpagedPosition(101)));
}

// createDisplayValueExtractor now resolves each columnOrder entry to a page-column index at build
// time (fieldToPageCol), so it always dereferences page.columns even when every test case here
// only cares about the staged-edit path. An empty-columns fake page makes every
// `pageColumnIndexFor` lookup miss (-1) by construction, which is exactly what "no page value
// available" means for these tests — none of them exercise the real page.ts decode path (cell()).
const NO_PAGE = { columns: [] } as unknown as TabularPage;

describe('rawPendingFor (finding 10)', () => {
  test('returns the same data as pendingFor, unwrapped from reactivity', () => {
    clearPending(TAB);
    stageEdit(TAB, 3, 'name', 'edited');
    const snapshot = rawPendingFor(TAB);
    expect(snapshot).toBeDefined();
    expect(isReactive(snapshot)).toBe(false);
    expect(isReactive(snapshot?.edits)).toBe(false);
    expect(snapshot?.edits.get(JSON.stringify([['id', '3']]))?.changes.name).toBe('edited');
  });

  test('undefined for a tab with no pending state at all', () => {
    expect(rawPendingFor('never-touched-tab')).toBeUndefined();
  });
});

describe('createDisplayValueExtractor merges staged edits the same way stagedValue() did (finding 10)', () => {
  test('a staged edit wins over the (absent) page value', () => {
    clearPending(TAB);
    stageEdit(TAB, 5, 'email', 'new@example.com');
    const extract = createDisplayValueExtractor(TAB, NO_PAGE, ['email']);
    const view = extract({ row: 5, pos: 5 }, 'email');
    expect(view).toEqual({ text: 'new@example.com', isNull: false, truncated: false });
  });

  test('a staged NULL is distinguished from an empty string', () => {
    clearPending(TAB);
    stageNull(TAB, 7, 'email');
    const extract = createDisplayValueExtractor(TAB, NO_PAGE, ['email']);
    const view = extract({ row: 7, pos: 7 }, 'email');
    expect(view).toEqual({ text: '', isNull: true, truncated: false });
  });

  test('a pending insert row is read from `inserts`, keyed by insertId', () => {
    clearPending(TAB);
    const insertId = addInsertRow(TAB, ['name']);
    stageInsertValue(TAB, insertId, 'name', 'brand new row');
    const extract = createDisplayValueExtractor(TAB, NO_PAGE, ['name']);
    const view = extract({ row: 100, pos: 100, insertId }, 'name');
    expect(view).toEqual({ text: 'brand new row', isNull: false, truncated: false });
  });

  test('no staged edit and no page falls through to the empty/null placeholder', () => {
    clearPending(TAB);
    const extract = createDisplayValueExtractor(TAB, NO_PAGE, ['email']);
    const view = extract({ row: 9, pos: 9 }, 'email');
    expect(view).toEqual({ text: '', isNull: true, truncated: false });
  });

  test('an edit staged after the extractor was built is still visible through it (the extractor is never rebuilt)', () => {
    // The regression this exact case caught: an earlier version of this fix read `pending` once
    // at build time. Because SlickGridHost.vue's own pending-change watch invalidates/re-renders
    // touched rows WITHOUT ever rebuilding this extractor (dataSource.ts's own corrected comment
    // explains why), a build-time snapshot could never observe an edit staged afterward through
    // the *same* extractor instance — exactly the scenario reused here.
    clearPending(TAB);
    stageEdit(TAB, 1, 'a', 'first');
    const extract = createDisplayValueExtractor(TAB, NO_PAGE, ['a']);
    expect(extract({ row: 1, pos: 1 }, 'a').text).toBe('first');
    stageEdit(TAB, 1, 'a', 'second');
    expect(extract({ row: 1, pos: 1 }, 'a').text).toBe('second');
  });

  test('the FIRST edit ever staged for a tab is visible through an extractor built before it existed', () => {
    // The sharpest form of the same regression: pendingChanges.ts's own `ensure()` creates
    // `pendingState[tabId]` lazily, on a tab's *first* staged change — so an extractor built while
    // the tab had no pending state at all (the ordinary case: SlickGridHost.vue builds its first
    // extractor at mount, long before anyone has edited a cell) captured `pending = undefined`
    // under a build-time snapshot. No later stageEdit could ever be observed through that stale
    // `undefined`, because nothing rebuilds the extractor to re-derive it. This is the exact
    // failure tests/ui/mutations.spec.ts's "edit, add, delete, preview, commit..." case hit.
    clearPending(TAB);
    const extract = createDisplayValueExtractor(TAB, NO_PAGE, ['name']);
    expect(extract({ row: 0, pos: 0 }, 'name').text).toBe(''); // nothing staged yet — page fallback
    stageEdit(TAB, 0, 'name', 'edited via UI');
    expect(extract({ row: 0, pos: 0 }, 'name').text).toBe('edited via UI');
  });
});

describe('pendingRowClasses reads the same snapshot mechanism (finding 10)', () => {
  test('a staged edit marks the row dirty', () => {
    clearPending(TAB);
    stageEdit(TAB, 2, 'x', 'y');
    expect(pendingRowClasses(TAB, 2, 1000)).toBe('kira-row-dirty');
  });

  test('an untouched row has no pending class', () => {
    clearPending(TAB);
    expect(pendingRowClasses(TAB, 4, 1000)).toBeUndefined();
  });
});
