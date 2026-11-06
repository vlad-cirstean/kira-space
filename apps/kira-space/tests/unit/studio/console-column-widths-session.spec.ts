// Real-interaction fix (reported bug — column widths reset on every subsequent query in the same
// session): ConsoleSlickGrid.vue's own header used to say "no persisted column widths — always the
// measured/default width, reset on every remount" as a deliberate design decision, because every
// query run swaps `pageKey` (ConsoleResultGrid.vue's own `:key`), remounting the grid component
// from scratch. consoleColumnWidths/setConsoleColumnWidths (state.ts) give that remount something
// to read that survives it — a plain field on the tab's own ConsoleViewRuntime record, which lives
// for the tab's whole open lifetime (the "session" the bug report names) and is discarded only
// when that record itself is (registerTabRuntimeCleanup, on tab close — a genuinely new session).
//
// console/state.ts transitively reaches bridge/control.ts -> '/wails/runtime.js' at module scope
// (control.ts's own top-level service-binding imports), hence the dynamic import after
// @workbench/testing/unit/window's mock.module registration — the same pattern
// console-run-after-tab-close.spec.ts already uses.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import { setActivePinia } from 'pinia';
import { pinia } from '../../../frontend/src/studio/state/pinia';

setActivePinia(pinia);

const { useConsoleViewStore } = await import('../../../frontend/src/studio/views/console/state');
const consoleViewStore = useConsoleViewStore();

describe('consoleColumnWidths / setConsoleColumnWidths (console/state.ts)', () => {
  test('1. a tab with no runtime yet reads as empty — never throws', () => {
    expect(consoleViewStore.consoleColumnWidths('never-touched-console-tab')).toEqual({});
  });

  test('2. a width set for one tab is read back exactly, unaffected by other tabs', () => {
    const tabId = 'console-widths-tab-1';
    consoleViewStore.setConsoleColumnWidths(tabId, { id: 80, name: 220 });
    expect(consoleViewStore.consoleColumnWidths(tabId)).toEqual({ id: 80, name: 220 });
    expect(consoleViewStore.consoleColumnWidths('a-different-tab')).toEqual({});
  });

  test('3. survives across separate calls — the same tab reused for a later "query run" sees the width a prior one committed', () => {
    const tabId = 'console-widths-tab-2';
    // First query's grid mount measures `id` and commits it as the tab's sticky width.
    consoleViewStore.setConsoleColumnWidths(tabId, { id: 64 });
    // A second query in the same tab (a fresh ConsoleSlickGrid.vue mount, new pageKey) reads it
    // back before deciding whether to measure `id` fresh — this is the read that must see 64,
    // not fall through to a new measurement.
    expect(consoleViewStore.consoleColumnWidths(tabId).id).toBe(64);

    // The second query's result also introduces a column `id` never had a width for — the sticky
    // store gains it without disturbing `id`'s own already-committed width (buildColumns' own
    // merge logic, `{ ...stored, [newName]: measuredWidth }`).
    consoleViewStore.setConsoleColumnWidths(tabId, {
      ...consoleViewStore.consoleColumnWidths(tabId),
      status: 96,
    });
    expect(consoleViewStore.consoleColumnWidths(tabId)).toEqual({ id: 64, status: 96 });
  });

  test('4. only reset when the tab runtime itself is gone (a genuinely new session), not by anything else touching that runtime record', () => {
    const tabId = 'console-widths-tab-3';
    consoleViewStore.setConsoleColumnWidths(tabId, { id: 64 });
    // Some unrelated runtime field changing (e.g. a new result set becoming active) must not
    // reset the width store — it's a distinct field on the same record.
    consoleViewStore.runtime[tabId].activeKey = 'some-other-result-key';
    expect(consoleViewStore.consoleColumnWidths(tabId)).toEqual({ id: 64 });

    // Only deleting the whole runtime record (what tab close does) clears it.
    delete consoleViewStore.runtime[tabId];
    expect(consoleViewStore.consoleColumnWidths(tabId)).toEqual({});
  });
});
