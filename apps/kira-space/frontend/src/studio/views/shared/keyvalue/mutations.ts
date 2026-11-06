import { decodePath, encodePath, pathParent } from '@shared/domain/tree';
import { t } from '@theme/i18n';
import { pinia } from '../../../../state/pinia';
import { useTabsStore } from '../../../../state/tabs';
import { data } from '../../../bridge/data';
import { browseInvalidate } from '../../../state/viewCommands';
import { createImmediateMutator } from '../immediateMutation';
import { keyValueHost } from './host';
import { useKeyValueViewStore } from './state';

// Keyvalue mutates immediately (mirrors views/documents/mutations.ts's discipline exactly) — no
// pendingChanges.ts-style staged plan, no preview step.
//
// The same two reserved sentinels as engine/adapters/redis/mutate.ts (`_key`/`$value`) — see
// that file's own comment for why `plan.path` alone can't name the target key.
const KEY_SENTINEL = '_key';
const VALUE_SENTINEL = '$value';

// P63: findTab reads through the host seam (host.ts) rather than state/tabs.ts's
// findKeyValueTab directly — createImmediateMutator only needs {connectionId, path}, which
// KeyValueHost already carries for either a real tab or the browse split's preview pane.
const mutate = createImmediateMutator({
  findTab: keyValueHost,
  reload: (viewKey: string) => useKeyValueViewStore(pinia).reload(viewKey),
  isStale: (viewKey: string) => useKeyValueViewStore(pinia).isStale(viewKey),
});

export async function saveValueEdit(
  viewKey: string,
  keyName: string,
  newValue: string,
): Promise<void> {
  await mutate(viewKey, [
    { kind: 'update', key: { [KEY_SENTINEL]: keyName }, changes: { [VALUE_SENTINEL]: newValue } },
  ]);
}

export async function deleteKey(viewKey: string, keyName: string): Promise<void> {
  // The key this tab was showing is now gone — reload still runs (mirrors deleteDocument's own
  // unconditional reload) so a stale row set never lingers; the read that follows surfaces the
  // ordinary "key no longer exists" query-time condition (read.ts's own precedent) rather than
  // this module inventing a second way to report the same fact.
  await mutate(viewKey, [{ kind: 'delete', key: { [KEY_SENTINEL]: keyName } }], (host) => {
    // P43 F11/D15: a deleted key's own container level (the level a Browse tab shows) just lost
    // a member — a no-op when no Browse tab is open (browseInvalidate's own contract).
    browseInvalidate(host.connectionId, pathParent(host.path) ?? '');
  });
}

// Scoped to string-type keys only (same as edit — see redis/mutate.ts's assertEditableType).
// Unlike edit/delete, there is no existing key for `plan.path` to already point at, so the new
// key's name travels entirely through the `insert` op's own `_key`/`$value` sentinels; the plan's
// path only needs to resolve to the right database (the current host's own path already does).
// On success this opens the new key in its own tab rather than reloading the current one — the
// current view is still showing an unrelated, still-existing key.
export async function addKey(
  viewKey: string,
  newKeyName: string,
  initialValue: string,
): Promise<void> {
  const host = keyValueHost(viewKey);
  if (!host?.connectionId) return;
  if (useKeyValueViewStore(pinia).isStale(viewKey)) {
    throw new Error(t('studio.common.connectionLost.error'));
  }
  await data.mutate({
    tabId: viewKey,
    connectionId: host.connectionId,
    path: host.path,
    ops: [
      {
        kind: 'insert',
        values: { [KEY_SENTINEL]: newKeyName, [VALUE_SENTINEL]: initialValue },
      },
    ],
  });
  // P43 F11/D15: the new key's own container level just gained a member.
  browseInvalidate(host.connectionId, pathParent(host.path) ?? '');
  const databaseSegment = decodePath(host.connectionId, host.path).segments.find(
    (s) => s.kind === 'database',
  );
  if (!databaseSegment) return;
  const newPath = encodePath([
    { kind: 'database', name: databaseSegment.name },
    { kind: 'key', name: newKeyName },
  ]);
  useTabsStore().openKeyValueTab(host.connectionId, newPath, { newTab: true });
}
