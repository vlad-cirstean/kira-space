// A second commit while the first awaits data.mutate used to rebuild and resend the same ops
// (duplicate INSERTs, a failing second DELETE), and a discard or edit mid-flight raced
// clearPending. pendingChanges.ts reaches bridge/data.ts -> '/wails/runtime.js' at module scope,
// hence the dynamic imports after the window mock.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';
import { setActivePinia } from 'pinia';
import {
  createTabularPageBuilder,
  unpagedPosition,
} from '../../../../../packages/shared/protocol/page';
import { pinia } from '../../../frontend/src/state/pinia';

setActivePinia(pinia);

const { setPage } = await import('../../../frontend/src/studio/views/grid/page');
const { data } = await import('../../../frontend/src/studio/bridge/data');
restoreAfterEach(data);
const { usePendingChangesStore } = await import(
  '../../../frontend/src/studio/views/grid/pendingChanges'
);
const store = usePendingChangesStore();

describe('commitPending in-flight guard', () => {
  test('a second commit sends nothing, a mid-flight discard is ignored, and the guard clears', async () => {
    const tabId = 'double-submit-tab';
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
    builder.appendRow(['1']);
    setPage(tabId, builder.finish(unpagedPosition(1)));
    store.stageDelete(tabId, [0]);

    let release: () => void = () => {};
    let calls = 0;
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real MutateRequestWire
    (data as any).mutate = () => {
      calls++;
      return new Promise((resolve) => {
        release = () => resolve({ affectedRows: 1 });
      });
    };

    const first = store.commitPending('conn-1', 'db:x/table:t', tabId);
    expect(store.isCommitting(tabId)).toBe(true);
    expect(await store.commitPending('conn-1', 'db:x/table:t', tabId)).toBeNull();
    store.discardPending(tabId);
    store.stageDelete(tabId, [5]);
    store.stageEdit(tabId, 0, 'id', 'x');
    store.discardRowChange(tabId, 0);
    expect(store.isPendingDelete(tabId, 0)).toBe(true);
    expect(store.isPendingDelete(tabId, 5)).toBe(false);
    expect(store.pendingFor(tabId)?.edits.size).toBe(0);
    release();
    await first;

    expect(calls).toBe(1);
    expect(store.hasPending(tabId)).toBe(false);
    expect(store.isCommitting(tabId)).toBe(false);
  });
});
