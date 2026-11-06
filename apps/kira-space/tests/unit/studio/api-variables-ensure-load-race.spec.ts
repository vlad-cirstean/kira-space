// ensureVariablesLoaded had no dedupe or sequencing. `if (!ownerId || listCache[key])
// return; listCache[key] = await control.variablesList(...)` let two concurrent callers each fire
// their own request, and a slow first reply landing after a later loadVariableSetRows (a post-edit
// refresh) overwrote the just-written fresh rows with the pre-edit list — every subsequent send for
// that scope then substituted stale plain values until the next edit.
//
// `listCache` and its hand-rolled dedupe are gone. loadVariableRows (apiQueries.ts) is
// `queryClient.query()`, which dedupes concurrent callers for the same key on its own — the first
// half needs no dedicated fix any more, just a pin that the dedupe still holds. The second
// half — a stale in-flight reply landing after a fresher write must not clobber it — is
// refreshApiQuery's own documented race (apiQueries.ts, above its definition): an observer-less
// in-flight `queryClient.query` caller (e.g. send()) must not have its invalidation cleared by that
// in-flight fetch resolving afterward with pre-change data.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import type { ApiVariable } from '@shared/domain/variables';
import { deferred } from '@workbench/testing/unit/async';
import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';

const { control } = await import('../../../frontend/src/studio/bridge/control');
restoreAfterEach(control);
const { apiVariablesKey, loadVariableRows, refreshApiQuery } = await import(
  '../../../frontend/src/studio/api/state/apiQueries'
);

function variable(overrides: Partial<ApiVariable>): ApiVariable {
  return {
    id: 'id',
    scope: 'collection',
    ownerId: 'col-1',
    name: 'token',
    value: 'value',
    isSecret: false,
    sortOrder: 0,
    description: '',
    ...overrides,
  };
}

describe('loadVariableRows / refreshApiQuery race handling (P108 F2 successor)', () => {
  test('two concurrent loads for the same owner share one control.variablesList call', async () => {
    const collectionId = 'col-race-1';
    let calls = 0;
    const result = deferred<ApiVariable[]>();
    (control as unknown as { variablesList: typeof control.variablesList }).variablesList =
      async () => {
        calls++;
        return result.promise;
      };

    const first = loadVariableRows('collection', collectionId);
    const second = loadVariableRows('collection', collectionId);
    // Give both callers' own synchronous dedupe check a tick to run before the fetch settles.
    await Promise.resolve();
    expect(calls).toBe(1);

    result.resolve([variable({ id: 'v1', ownerId: collectionId, value: 'loaded' })]);
    expect(await first).toHaveLength(1);
    expect(await second).toHaveLength(1);
    expect(calls).toBe(1);
  });

  test('a stale in-flight fetch resolving after refreshApiQuery does not clobber the fresh reply', async () => {
    const collectionId = 'col-race-2';
    const slow = deferred<ApiVariable[]>();
    let call = 0;
    (control as unknown as { variablesList: typeof control.variablesList }).variablesList =
      async () => {
        call++;
        // The first call (already in flight when refreshApiQuery runs, e.g. a concurrent send())
        // is slow and stale; refreshApiQuery's own re-check once it settles gets the fresh rows.
        return call === 1 ? slow.promise : [variable({ id: 'fresh', value: 'fresh-value' })];
      };

    const inFlight = loadVariableRows('collection', collectionId);
    await Promise.resolve(); // let the in-flight fetch actually start (and be captured as in-flight)

    const refreshing = refreshApiQuery(apiVariablesKey('collection', collectionId));

    // The stale in-flight fetch's own reply lands after the change that triggered refreshApiQuery.
    slow.resolve([variable({ id: 'stale', value: 'stale-value' })]);
    await inFlight;
    await refreshing;

    // No active observer exists for this key here (no mounted useVariableRows/store), so
    // refreshApiQuery's own refetch is a no-op the moment it runs — same as any other unobserved
    // key ("invalidating keys nothing observes yet is a no-op"). What it still
    // guarantees is that the query is left marked invalidated, so the *next* read — a real send(),
    // never this test's own probe — does not keep trusting the stale reply and fetches again.
    const after = await loadVariableRows('collection', collectionId);
    expect(after).toEqual([variable({ id: 'fresh', value: 'fresh-value' })]);
    expect(call).toBe(2);
  });
});
