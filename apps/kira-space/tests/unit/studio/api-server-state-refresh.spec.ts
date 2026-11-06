// The history list's lazy-when-hidden refresh and the jar cookie list's
// delete-vs-in-flight ordering are invalidation rules over TanStack Query observers, plus two
// races. Observers are mounted with effectScope().run(useQuery) — every composable here passes the
// shared queryClient explicitly — and results are read through queryClient.getQueryData.
import '@workbench/testing/unit/window';

import { afterAll, afterEach, beforeEach, describe, expect, test } from 'bun:test';
import type { HttpCookieWire } from '@shared/domain/http';
import type { ResponseHistoryEntry } from '@shared/domain/responseHistory';
import { queryClient } from '@workbench/state/queryClient';
import { deferred, sleep } from '@workbench/testing/unit/async';
import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';
import { setActivePinia } from 'pinia';
import { type EffectScope, effectScope } from 'vue';
import { pinia } from '../../../frontend/src/state/pinia';

setActivePinia(pinia);

const { control } = await import('../../../frontend/src/studio/bridge/control');
const originalTabsSave = control.tabsSave;
afterAll(() => {
  control.tabsSave = originalTabsSave;
});
(control as unknown as { tabsSave: typeof control.tabsSave }).tabsSave = () => Promise.resolve();
restoreAfterEach(control);
const { findHttpRequestTab, openApiRequestTab, patchHttpRequestTabState } = await import(
  '../../../frontend/src/studio/api/tabs'
);
const { useHttpHistoryList, useHttpHistoryStore } = await import(
  '../../../frontend/src/studio/views/httprequest/history'
);
const { deleteJarCookie, useJarCookies } = await import(
  '../../../frontend/src/studio/views/httprequest/cookies'
);

// Observers outlive their test otherwise and refetch through the next test's stubs.
const scopes: EffectScope[] = [];
function observe(fn: () => unknown): void {
  const scope = effectScope();
  scopes.push(scope);
  scope.run(fn);
}
// Not queryClient.clear(): other specs' stores keep permanent observers on unrelated queries.
beforeEach(() => {
  queryClient.removeQueries({ queryKey: ['httpHistory'] });
  queryClient.removeQueries({ queryKey: ['httpJarCookies'] });
});
afterEach(() => {
  for (const scope of scopes.splice(0)) scope.stop();
});

function entry(id: string): ResponseHistoryEntry {
  return { id } as ResponseHistoryEntry;
}

/** Lets invalidate-then-refetch chains (several macrotasks deep) run to completion. */
async function settle(): Promise<void> {
  for (let i = 0; i < 10; i++) await sleep(5);
}

function mountHistoryList(tabId: string): void {
  observe(() => useHttpHistoryList(() => findHttpRequestTab(tabId) as never));
}

describe('history list refresh (P8 D11)', () => {
  test('a send with the History pane hidden issues no list call; showing the pane issues exactly one', async () => {
    const tabId = openApiRequestTab();
    let calls = 0;
    let latest: ResponseHistoryEntry[] = [];
    (control as unknown as { historyList: typeof control.historyList }).historyList = async () => {
      calls++;
      return latest;
    };
    mountHistoryList(tabId);
    await settle();
    expect(calls).toBe(1); // the one initial fetch

    latest = [entry('new')];
    useHttpHistoryStore().noteSendRecorded(tabId);
    await settle();
    expect(calls).toBe(1);

    patchHttpRequestTabState(tabId, { responsePane: 'history' });
    await settle();
    expect(calls).toBe(2);
    const key = ['httpHistory', '', tabId];
    expect(queryClient.getQueryData<ResponseHistoryEntry[]>(key)?.map((e) => e.id)).toEqual([
      'new',
    ]);
  });

  test('a list fetch in flight when a send lands with the pane showing still ends with the new entry', async () => {
    const tabId = openApiRequestTab();
    patchHttpRequestTabState(tabId, { responsePane: 'history' });
    const gate = deferred<ResponseHistoryEntry[]>();
    let calls = 0;
    (control as unknown as { historyList: typeof control.historyList }).historyList = () => {
      calls++;
      return calls === 1 ? gate.promise : Promise.resolve([entry('new')]);
    };
    mountHistoryList(tabId);
    await settle();
    expect(calls).toBe(1);

    useHttpHistoryStore().noteSendRecorded(tabId);
    gate.resolve([]); // the pre-send answer lands after the send
    await settle();

    const key = ['httpHistory', '', tabId];
    expect(queryClient.getQueryData<ResponseHistoryEntry[]>(key)?.map((e) => e.id)).toEqual([
      'new',
    ]);
  });
});

describe('jar cookie list', () => {
  test('a list fetch in flight when Remove succeeds does not resurrect the cookie', async () => {
    const url = 'https://api.example.com/x';
    const cookie = {
      name: 'sid',
      value: '1',
      domain: 'api.example.com',
      path: '/',
    } as HttpCookieWire;
    const gate = deferred<HttpCookieWire[]>();
    (control as unknown as { httpCookies: typeof control.httpCookies }).httpCookies = () =>
      gate.promise;
    (control as unknown as { httpDeleteCookie: typeof control.httpDeleteCookie }).httpDeleteCookie =
      async () => [];
    observe(() =>
      useJarCookies(
        () => url,
        () => true,
      ),
    );
    await settle();

    await deleteJarCookie(url, cookie);
    gate.resolve([cookie]); // the pre-delete answer lands after the delete
    await settle();

    expect(queryClient.getQueryData<HttpCookieWire[]>(['httpJarCookies', url])).toEqual([]);
  });
});
