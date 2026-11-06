// Staged edits are keyed by primary key: they survive a new page, address the original row on
// commit even when it is off-page, and are refused when the page cannot supply the full key.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import type { ObjectMeta } from '@shared/domain/tree';
import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';
import { setActivePinia } from 'pinia';
import {
  type ColumnDescriptor,
  createTabularPageBuilder,
  unpagedPosition,
} from '../../../../../packages/shared/protocol/page';
import { pinia } from '../../../frontend/src/studio/state/pinia';

setActivePinia(pinia);

const { setPage } = await import('../../../frontend/src/studio/views/grid/page');
const { useGridViewStore } = await import('../../../frontend/src/studio/views/grid/state');
const gridViewStore = useGridViewStore();
const { data } = await import('../../../frontend/src/studio/bridge/data');
restoreAfterEach(data);
const { usePendingChangesStore } = await import(
  '../../../frontend/src/studio/views/grid/pendingChanges'
);
const store = usePendingChangesStore();

function col(name: string, isPrimaryKey: boolean): ColumnDescriptor {
  return {
    name,
    dataType: 'text',
    typeClass: 'text',
    nullable: !isPrimaryKey,
    isPrimaryKey,
    generated: false,
  };
}

function useMeta(tabId: string, primaryKey: string[]): void {
  gridViewStore.runtime[tabId] = {
    status: 'idle',
    error: null,
    actionError: null,
    opId: null,
    count: null,
    countOpId: null,
    countError: null,
    meta: { primaryKey } as unknown as ObjectMeta,
    lastStrategy: 'offset',
    nextToken: null,
    prevToken: null,
    hasMore: false,
    selection: null,
    searchOpen: false,
    maskPreview: false,
    settledWidths: null,
    whereDraft: null,
    orderByDraft: null,
  };
}

function page(columns: ColumnDescriptor[], rows: string[][]) {
  const builder = createTabularPageBuilder(columns);
  for (const r of rows) builder.appendRow(r);
  return builder.finish(unpagedPosition(rows.length));
}

const columns = [col('id', true), col('name', false)];

function captureMutate(): { ops: () => unknown[] } {
  let sent: unknown[] = [];
  // biome-ignore lint/suspicious/noExplicitAny: minimal fake of the bridge
  (data as any).mutate = async (req: any) => {
    sent = req.ops;
    return { affectedRows: 1 };
  };
  return { ops: () => sent };
}

describe('staged changes keyed by primary key', () => {
  test('an edit follows its row to a new row position and survives an off-page load', async () => {
    const tabId = 'pk-tab-1';
    useMeta(tabId, ['id']);
    setPage(
      tabId,
      page(columns, [
        ['1', 'a'],
        ['2', 'b'],
      ]),
    );
    store.stageEdit(tabId, 1, 'name', 'b2');

    setPage(
      tabId,
      page(columns, [
        ['2', 'b'],
        ['1', 'a'],
      ]),
    );
    expect(store.stagedValue(tabId, 0, 'name')).toBe('b2');
    expect(store.stagedValue(tabId, 1, 'name')).toBeUndefined();

    setPage(tabId, page(columns, [['9', 'z']]));
    expect(store.hasPending(tabId)).toBe(true);
    expect(store.offPageCount(tabId)).toBe(1);

    const sent = captureMutate();
    await store.commitPending('c', 'p', tabId);
    expect(sent.ops()).toEqual([{ kind: 'update', key: { id: '2' }, changes: { name: 'b2' } }]);
  });

  test('a primary-key edit still addresses the original row', async () => {
    const tabId = 'pk-tab-2';
    useMeta(tabId, ['id']);
    setPage(tabId, page(columns, [['1', 'a']]));
    store.stageEdit(tabId, 0, 'id', '7');
    store.stageEdit(tabId, 0, 'name', 'x');
    const sent = captureMutate();
    await store.commitPending('c', 'p', tabId);
    expect(sent.ops()).toEqual([
      { kind: 'update', key: { id: '1' }, changes: { id: '7', name: 'x' } },
    ]);
  });

  test('a page without the full primary key stages nothing', () => {
    const noPk = 'pk-tab-3';
    useMeta(noPk, []);
    setPage(noPk, page([col('name', false)], [['a']]));
    store.stageEdit(noPk, 0, 'name', 'x');
    store.stageDelete(noPk, [0]);
    expect(store.hasPending(noPk)).toBe(false);
    expect(store.pageHasFullPrimaryKey(noPk)).toBe(false);

    const partial = 'pk-tab-4';
    useMeta(partial, ['tenant', 'id']);
    setPage(partial, page([col('tenant', true), col('name', false)], [['t', 'a']]));
    store.stageEdit(partial, 0, 'name', 'x');
    expect(store.hasPending(partial)).toBe(false);
    expect(store.pageHasFullPrimaryKey(partial)).toBe(false);
  });

  test('a composite key commits with both columns', async () => {
    const tabId = 'pk-tab-5';
    useMeta(tabId, ['tenant', 'id']);
    setPage(tabId, page([col('tenant', true), col('id', true)], [['t', 'e']]));
    store.stageDelete(tabId, [0]);
    const sent = captureMutate();
    await store.commitPending('c', 'p', tabId);
    expect(sent.ops()).toEqual([{ kind: 'delete', key: { tenant: 't', id: 'e' } }]);
  });
});
