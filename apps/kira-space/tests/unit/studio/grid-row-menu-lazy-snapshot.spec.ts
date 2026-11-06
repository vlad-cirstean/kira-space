// rowMenu() used to eagerly run ctx.rows.map(ctx.snapshot)
// while *building* the menu — decoding every selected row across every column whether or not any
// of the four Copy row(s) items ever runs, including on every Delete keypress (which routes
// through this same builder purely to dispatch, per the "printed shortcut and executed
// action can't drift" rule, and never reads a snapshot at all). This pins that building the menu
// no longer touches ctx.snapshot at all unless a Copy row(s) item's own run() executes,
// and that running two of them only decodes once (the shared thunk is memoized).
//
// menu.ts transitively reaches bridge/control/bridge/data, so this has to be a dynamic import()
// after @workbench/testing/unit/window's mock.module registration has run (the same pattern
// row-values-visible-span.spec.ts already uses).
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import type { RowSnapshot } from '../../../frontend/src/studio/views/shared/clipboardFormats';

const { rowMenu } = await import('../../../frontend/src/studio/views/grid/menu');
type MenuItemWithId = Extract<ReturnType<typeof rowMenu>[number], { id: string }>;

// The Copy row(s) items' own run() calls copyText() -> navigator.clipboard.writeText, which Bun's
// runtime does not implement — stubbed so run() can actually execute in this test.
(globalThis.navigator as unknown as { clipboard: { writeText: (t: string) => Promise<void> } }) = {
  ...globalThis.navigator,
  clipboard: { writeText: async () => {} },
};

function findRun(items: ReturnType<typeof rowMenu>, ...path: string[]): () => void {
  let level = items;
  let run: (() => void | Promise<void>) | undefined;
  for (const id of path) {
    const item = level.filter((i): i is MenuItemWithId => 'id' in i).find((i) => i.id === id);
    if (!item) throw new Error(`menu item ${id} not found`);
    if (item.type === 'submenu') {
      level = item.items;
    } else if (item.type === 'item') {
      run = item.run;
    }
  }
  if (!run) throw new Error(`no run() found at path ${path.join(' > ')}`);
  return run;
}

describe('rowMenu — lazy, memoized snapshotting (finding A1)', () => {
  test('building the menu never calls snapshot()', () => {
    let calls = 0;
    const snapshot = (_row: number): RowSnapshot => {
      calls++;
      return { columns: [], values: {} };
    };
    rowMenu({
      tabId: 'tab-1',
      rows: [0, 1, 2],
      qualifiedName: 'public.t',
      snapshot,
      canEdit: true,
      canDelete: true,
      dialect: 'postgres',
    });
    expect(calls).toBe(0);
  });

  test('Delete row(s) never calls snapshot() either', () => {
    let calls = 0;
    const snapshot = (_row: number): RowSnapshot => {
      calls++;
      return { columns: [], values: {} };
    };
    const items = rowMenu({
      tabId: 'tab-1',
      rows: [0, 1, 2],
      qualifiedName: 'public.t',
      snapshot,
      canEdit: true,
      canDelete: true,
      dialect: 'postgres',
    });
    findRun(items, 'delete-row')();
    expect(calls).toBe(0);
  });

  test('running one Copy row(s) item calls snapshot() exactly once per row', () => {
    let calls = 0;
    const snapshot = (row: number): RowSnapshot => {
      calls++;
      return { columns: ['a'], values: { a: String(row) } };
    };
    const items = rowMenu({
      tabId: 'tab-1',
      rows: [0, 1, 2],
      qualifiedName: 'public.t',
      snapshot,
      canEdit: true,
      canDelete: true,
      dialect: 'postgres',
    });
    findRun(items, 'copy-rows', 'copy-rows-tsv')();
    expect(calls).toBe(3);
  });

  test('running two Copy row(s) items in the same menu instance still decodes only once (shared, memoized thunk)', () => {
    let calls = 0;
    const snapshot = (row: number): RowSnapshot => {
      calls++;
      return { columns: ['a'], values: { a: String(row) } };
    };
    const items = rowMenu({
      tabId: 'tab-1',
      rows: [0, 1, 2],
      qualifiedName: 'public.t',
      snapshot,
      canEdit: true,
      canDelete: true,
      dialect: 'postgres',
    });
    findRun(items, 'copy-rows', 'copy-rows-tsv')();
    findRun(items, 'copy-rows', 'copy-rows-csv')();
    expect(calls).toBe(3); // not 6 — the second run reuses the first pass's memoized result
  });
});
