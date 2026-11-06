// P21 round 2 performance finding 2: the exact eager-row-menu-snapshot pattern round 1 fixed in
// views/grid/menu.ts (grid-row-menu-lazy-snapshot.spec.ts) existed unfixed in two sibling menus —
// documents/menu.ts's rowMenu (a whole-page `_id` decode built at right-click time, even though
// setAllExpanded's own `true`/Expand-all branch never reads it at all) and
// console/menu.ts's rowAsJsonMenu (a whole-page decode/JSON.stringify built at right-click
// time for ConsoleResultGrid.vue's document/key-value branches). Both `allIds`/`allJson` are
// thunks now, invoked only inside the "all" item's own run().
//
// menu.ts transitively reaches bridge/data.ts -> '/wails/runtime.js' at module scope, so this has
// to be a dynamic import() after @workbench/testing/unit/window's mock.module registration has run (the same
// pattern grid-row-menu-lazy-snapshot.spec.ts already uses).
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import type { MenuItem } from '@workbench/state/contextMenu';
import { createPinia, setActivePinia } from 'pinia';

setActivePinia(createPinia());

const { rowMenu } = await import('../../../frontend/src/studio/views/documents/menu');
const { rowAsJsonMenu } = await import('../../../frontend/src/studio/views/console/menu');

// The Copy items' own run() calls copyText() -> navigator.clipboard.writeText, which Bun's
// runtime does not implement — stubbed so run() can actually execute in these tests.
(globalThis.navigator as unknown as { clipboard: { writeText: (t: string) => Promise<void> } }) = {
  ...globalThis.navigator,
  clipboard: { writeText: async () => {} },
};

type MenuItemWithId = Extract<MenuItem, { id: string }>;

function findRun(items: MenuItem[], ...path: string[]): () => void {
  let level = items;
  let run: (() => void | Promise<void>) | undefined;
  for (const id of path) {
    const item = level.filter((i): i is MenuItemWithId => 'id' in i).find((i) => i.id === id);
    if (!item) throw new Error(`menu item ${id} not found`);
    if (item.type === 'submenu') level = item.items;
    else if (item.type === 'item') run = item.run;
  }
  if (!run) throw new Error(`no run() found at path ${path.join(' > ')}`);
  return run;
}

describe('documents/menu.ts rowMenu — lazy allIds (P21 round 2 performance finding 2)', () => {
  function menuWithCountedAllIds() {
    let calls = 0;
    const allIds = () => {
      calls++;
      return ['a', 'b', 'c'];
    };
    const items = rowMenu(
      'tab-1',
      'a',
      '{}',
      allIds,
      () => {},
      { editable: true, label: 'Edit' },
      {
        deletable: true,
        label: 'Delete',
      },
    );
    return { items, getCalls: () => calls };
  }

  test('building the menu never calls allIds()', () => {
    const { getCalls } = menuWithCountedAllIds();
    expect(getCalls()).toBe(0);
  });

  test("Expand all never calls allIds() — setAllExpanded's own true branch does not read it", () => {
    const { items, getCalls } = menuWithCountedAllIds();
    findRun(items, 'expand-all')();
    expect(getCalls()).toBe(0);
  });

  test('Collapse all calls allIds() exactly once', () => {
    const { items, getCalls } = menuWithCountedAllIds();
    findRun(items, 'collapse-all')();
    expect(getCalls()).toBe(1);
  });

  test('a non-expand/collapse action (Copy _id) never calls allIds()', () => {
    const { items, getCalls } = menuWithCountedAllIds();
    findRun(items, 'copy-id')();
    expect(getCalls()).toBe(0);
  });

  // P108 Part 11 F17: the Delete item now carries the same editable/label gate editGate already
  // gives Edit — a row whose Delete button DocumentView.vue disables (read-only connection, no
  // canDelete cap) must offer no working Delete in the context menu either.
  test('Delete is enabled and labelled "Delete" when deleteGate.deletable is true', () => {
    const items = rowMenu(
      'tab-1',
      'a',
      '{}',
      () => [],
      () => {},
      { editable: true, label: 'Edit' },
      { deletable: true, label: 'Delete' },
    );
    const item = items
      .filter((i): i is MenuItemWithId => 'id' in i)
      .find((i) => i.id === 'delete-document');
    expect(item?.type === 'item' && item.disabled).toBeFalsy();
    expect(item?.type === 'item' && item.label).toBe('Delete');
  });

  test('Delete is disabled and relabelled when deleteGate.deletable is false', () => {
    const items = rowMenu(
      'tab-1',
      'a',
      '{}',
      () => [],
      () => {},
      { editable: true, label: 'Edit' },
      { deletable: false, label: 'Connection is read-only' },
    );
    const item = items
      .filter((i): i is MenuItemWithId => 'id' in i)
      .find((i) => i.id === 'delete-document');
    expect(item?.type === 'item' && item.disabled).toBe(true);
    expect(item?.type === 'item' && item.label).toBe('Connection is read-only');
  });
});

describe('console/menu.ts rowAsJsonMenu — lazy allJson (P21 round 2 performance finding 2)', () => {
  function menuWithCountedAllJson() {
    let calls = 0;
    const allJson = () => {
      calls++;
      return ['{"a":1}', '{"a":2}'];
    };
    const items = rowAsJsonMenu({ json: '{"a":1}', allJson, onError: () => {} });
    return { items, getCalls: () => calls };
  }

  test('building the menu never calls allJson()', () => {
    const { getCalls } = menuWithCountedAllJson();
    expect(getCalls()).toBe(0);
  });

  test('Copy as JSON (this row only) never calls allJson()', async () => {
    const { items, getCalls } = menuWithCountedAllJson();
    await findRun(items, 'copy-as-json')();
    expect(getCalls()).toBe(0);
  });

  test('Copy all as JSON calls allJson() exactly once', async () => {
    const { items, getCalls } = menuWithCountedAllJson();
    await findRun(items, 'copy-all-as-json')();
    expect(getCalls()).toBe(1);
  });
});
