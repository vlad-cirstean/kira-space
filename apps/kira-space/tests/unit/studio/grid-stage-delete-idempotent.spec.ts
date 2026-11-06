// Real-interaction fix (reported bug — invoking the delete shortcut a second time on an
// already-deleted row UNDID the deletion): pendingChanges.ts's own delete-marking function used
// to be named `toggleDelete` and, true to its name, un-marked an already-pending row on a second
// call. Every one of its three callers (the right-click menu's "Delete row(s)"/"Delete row", the
// `grid.deleteRows` keyboard shortcut — both via grid/menu.ts's rowMenu()/cellMenu() — and
// DataToolbar.vue's own toolbar button) presents itself as a plain, non-toggling delete action, so
// a repeat invocation silently reversed the user's own most recent action. Renamed to `stageDelete`
// and made idempotent: marking an already-deleted row for delete again is a no-op, matching
// stageEdit/stageNull (neither toggles either) — "Revert row(s)" is the one way to undo a delete.
//
// pendingChanges.ts transitively reaches bridge/data.ts -> '/wails/runtime.js' at module scope,
// hence the dynamic import after @workbench/testing/unit/window's mock.module registration — the
// same pattern grid-staged-value-snapshot.spec.ts already use.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import { setActivePinia } from 'pinia';
import {
  createTabularPageBuilder,
  unpagedPosition,
} from '../../../../../packages/shared/protocol/page';
import { pinia } from '../../../frontend/src/state/pinia';

setActivePinia(pinia);

const { usePendingChangesStore } = await import(
  '../../../frontend/src/studio/views/grid/pendingChanges'
);
const { isPendingDelete, stageDelete, stageEdit } = usePendingChangesStore();
const { setPage } = await import('../../../frontend/src/studio/views/grid/page');

// Staging is keyed by primary key: each tab needs a loaded page where row n has id n.
function loadPage(tabId: string): void {
  const builder = createTabularPageBuilder([
    {
      name: 'id',
      dataType: 'text',
      typeClass: 'text',
      nullable: false,
      isPrimaryKey: true,
      generated: false,
    },
    {
      name: 'name',
      dataType: 'text',
      typeClass: 'text',
      nullable: true,
      isPrimaryKey: false,
      generated: false,
    },
  ]);
  for (let i = 0; i < 5; i++) builder.appendRow([String(i), `n${i}`]);
  setPage(tabId, builder.finish(unpagedPosition(5)));
}

describe('stageDelete (grid/pendingChanges.ts)', () => {
  test('1. marks a row for delete', () => {
    const tabId = 'stage-delete-tab-1';
    loadPage(tabId);
    stageDelete(tabId, [3]);
    expect(isPendingDelete(tabId, 3)).toBe(true);
  });

  test('2. invoking it again on the same, already-deleted row leaves it deleted — never un-deletes (the reported toggle bug)', () => {
    const tabId = 'stage-delete-tab-2';
    loadPage(tabId);
    stageDelete(tabId, [3]);
    expect(isPendingDelete(tabId, 3)).toBe(true);

    stageDelete(tabId, [3]); // the second Cmd+Backspace / second "Delete row(s)" invocation
    expect(isPendingDelete(tabId, 3)).toBe(true);

    stageDelete(tabId, [3]); // a third, for good measure — still no toggle back
    expect(isPendingDelete(tabId, 3)).toBe(true);
  });

  test('3. marking a row with a pending edit for delete drops the edit (edits and deletes stay mutually exclusive)', () => {
    const tabId = 'stage-delete-tab-3';
    loadPage(tabId);
    stageEdit(tabId, 1, 'name', 'new value');
    stageDelete(tabId, [1]);
    expect(isPendingDelete(tabId, 1)).toBe(true);

    // Re-invoking delete on the same row must not resurrect the dropped edit or un-delete it.
    stageDelete(tabId, [1]);
    expect(isPendingDelete(tabId, 1)).toBe(true);
  });

  test('4. deleting a whole multi-row selection twice keeps every row deleted', () => {
    const tabId = 'stage-delete-tab-4';
    loadPage(tabId);
    stageDelete(tabId, [0, 1, 2]);
    stageDelete(tabId, [0, 1, 2]);
    expect(isPendingDelete(tabId, 0)).toBe(true);
    expect(isPendingDelete(tabId, 1)).toBe(true);
    expect(isPendingDelete(tabId, 2)).toBe(true);
  });
});
