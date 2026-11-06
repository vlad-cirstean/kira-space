// ensureDdl used to await its own cached pending promise and only clear
// pendingLoads on the success path — `pendingLoads.delete(connectionId)` sat after the `await`,
// so a rejection never reached it. One transient control.schemaGet failure (a backend error, a
// disconnect mid-boot) then poisoned the cache for the rest of the session: every later caller
// (console completion/diagnostics/hover, the Schema dialog) re-awaited and re-threw the same
// stale rejection, with nothing retrying and nothing surfacing to the user. This pins the fix:
// pendingLoads must clear on rejection too, so a later call gets a fresh attempt.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import { deferred } from '@workbench/testing/unit/async';
import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';

const { control } = await import('../../../frontend/src/studio/bridge/control');
restoreAfterEach(control);
const { ensureDdl, saveDdl, schemaQueryKey, initSchemaSync } = await import(
  '../../../frontend/src/studio/state/schemas'
);
const { queryClient } = await import('@workbench/state/queryClient');

describe('state/schemas.ts — ensureDdl pending-load cache (P12 round 1 F4)', () => {
  test('a rejected load does not poison later calls for the same connection', async () => {
    const connectionId = 'conn-ensure-ddl-retry';
    queryClient.removeQueries({ queryKey: schemaQueryKey(connectionId) });

    const first = deferred<{ connectionId: string; ddl: string; updatedAt: string }>();
    // biome-ignore lint/suspicious/noExplicitAny: a minimal stub, not the real schemaGet.
    (control as any).schemaGet = () => first.promise;

    // .catch is attached in the same synchronous tick ensureDdl is called, before the reject
    // below — the safe ordering for a manually-triggered rejection under bun:test.
    const failing = ensureDdl(connectionId).catch((err: unknown) => err);
    first.reject(new Error('backend unavailable'));
    const caught = await failing;
    expect((caught as Error)?.message).toBe('backend unavailable');

    // The retry must actually call schemaGet again, not re-await the same rejected promise —
    // proven by resolving this second call to a real value and getting it back.
    let secondCallCount = 0;
    // biome-ignore lint/suspicious/noExplicitAny: a minimal stub, not the real schemaGet.
    (control as any).schemaGet = () => {
      secondCallCount++;
      return Promise.resolve({ connectionId, ddl: 'create table t (id int);', updatedAt: '' });
    };

    const ddl = await ensureDdl(connectionId);
    expect(ddl).toBe('create table t (id int);');
    expect(secondCallCount).toBe(1);
    expect(queryClient.getQueryData<string>(schemaQueryKey(connectionId))).toBe(
      'create table t (id int);',
    );
  });

  // ensureDdl wrote `schemasState.byConnection[connectionId] = ddl`
  // unconditionally after its own await — a slow initial fetch resolving *after* a faster
  // saveDdl (a real user Save) had already written fresher text overwrote the store back to the
  // stale fetched value, with nothing left to notice until the next relaunch.
  test('a slow initial fetch does not clobber a faster concurrent saveDdl (P12 round 2 F14)', async () => {
    const connectionId = 'conn-ensure-ddl-race';
    queryClient.removeQueries({ queryKey: schemaQueryKey(connectionId) });

    const slowFetch = deferred<{ connectionId: string; ddl: string; updatedAt: string }>();
    // biome-ignore lint/suspicious/noExplicitAny: a minimal stub, not the real schemaGet.
    (control as any).schemaGet = () => slowFetch.promise;
    // biome-ignore lint/suspicious/noExplicitAny: a minimal stub, not the real schemaSet.
    (control as any).schemaSet = (_id: string, ddlText: string) =>
      Promise.resolve({ connectionId, ddl: ddlText, updatedAt: '' });

    // The initial load starts, but its own response is left in flight.
    const loading = ensureDdl(connectionId);

    // A faster concurrent Save (e.g. the user typed and saved before the slow load returned)
    // writes fresh text straight into the cache, bypassing ensureDdl entirely — and cancels the
    // still-in-flight fetch above so its own (now-stale) resolution can't land after it.
    await saveDdl(connectionId, 'create table fresh (id int);');
    expect(queryClient.getQueryData<string>(schemaQueryKey(connectionId))).toBe(
      'create table fresh (id int);',
    );

    // The slow fetch *now* resolves with the stale pre-save text — cancelled, so it must not
    // overwrite the fresher save either in the cache or in what the still-pending ensureDdl call
    // itself resolves to.
    slowFetch.resolve({ connectionId, ddl: 'create table stale (id int);', updatedAt: '' });
    const loaded = await loading;

    expect(queryClient.getQueryData<string>(schemaQueryKey(connectionId))).toBe(
      'create table fresh (id int);',
    );
    expect(loaded).toBe('create table fresh (id int);');
  });
});

// P108 Part 12 F1: applyRemote used to only invalidateQueries, which triggered a refetch that ran
// straight into the "prefer whatever's cached" guard above and could never actually move the
// cache — so a genuine remote push (another window's Schema dialog Save) never reached this one.
describe('state/schemas.ts — remote schema push reaches the cache (P108 Part 12 F1)', () => {
  test('a onSchemaChanged push writes the new DDL into the cache with no refetch', async () => {
    const connectionId = 'conn-remote-push';
    queryClient.removeQueries({ queryKey: schemaQueryKey(connectionId) });

    let deliver: (ddl: { connectionId: string; ddl: string; updatedAt: string }) => void = () => {};
    // biome-ignore lint/suspicious/noExplicitAny: a minimal stub, not the real control surface.
    (control as any).onSchemaChanged = (cb: typeof deliver) => {
      deliver = cb;
      return () => {};
    };
    // biome-ignore lint/suspicious/noExplicitAny: a minimal stub, not the real control surface.
    (control as any).onConnectionsChanged = () => () => {};

    let fetchCount = 0;
    // biome-ignore lint/suspicious/noExplicitAny: a minimal stub, not the real control surface.
    (control as any).schemaGet = () => {
      fetchCount++;
      return Promise.resolve({ connectionId, ddl: 'create table old (id int);', updatedAt: '' });
    };

    initSchemaSync();
    expect(await ensureDdl(connectionId)).toBe('create table old (id int);');
    expect(fetchCount).toBe(1);

    // Another window saved a new document; the Go side pushes it here.
    deliver({ connectionId, ddl: 'create table new (id int);', updatedAt: '' });

    expect(queryClient.getQueryData<string>(schemaQueryKey(connectionId))).toBe(
      'create table new (id int);',
    );
    // The push must not have triggered a refetch — it writes the cache directly.
    expect(fetchCount).toBe(1);
  });

  test('a fetch already in flight when a remote push lands does not overwrite the push', async () => {
    const connectionId = 'conn-remote-push-race';
    queryClient.removeQueries({ queryKey: schemaQueryKey(connectionId) });

    let deliver: (ddl: { connectionId: string; ddl: string; updatedAt: string }) => void = () => {};
    // biome-ignore lint/suspicious/noExplicitAny: a minimal stub, not the real control surface.
    (control as any).onSchemaChanged = (cb: typeof deliver) => {
      deliver = cb;
      return () => {};
    };
    // biome-ignore lint/suspicious/noExplicitAny: a minimal stub, not the real control surface.
    (control as any).onConnectionsChanged = () => () => {};

    const slowFetch = deferred<{ connectionId: string; ddl: string; updatedAt: string }>();
    // biome-ignore lint/suspicious/noExplicitAny: a minimal stub, not the real control surface.
    (control as any).schemaGet = () => slowFetch.promise;

    initSchemaSync();
    const loading = ensureDdl(connectionId);

    // The remote push lands while the initial fetch is still in flight.
    deliver({ connectionId, ddl: 'create table pushed (id int);', updatedAt: '' });
    expect(queryClient.getQueryData<string>(schemaQueryKey(connectionId))).toBe(
      'create table pushed (id int);',
    );

    // The slow initial fetch now resolves with pre-push (stale) text.
    slowFetch.resolve({ connectionId, ddl: 'create table stale (id int);', updatedAt: '' });
    const loaded = await loading;

    expect(queryClient.getQueryData<string>(schemaQueryKey(connectionId))).toBe(
      'create table pushed (id int);',
    );
    expect(loaded).toBe('create table pushed (id int);');
  });
});
