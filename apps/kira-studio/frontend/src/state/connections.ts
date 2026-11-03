import type { ConnectionState, ConnectionSummary } from '@shared/domain/connection';
import type { SecretStorageStatus } from '@shared/domain/secrets';
import { defineStore } from 'pinia';
import { reactive, toRefs } from 'vue';
import { control } from '../bridge/control';

interface ConnectionsState {
  records: ConnectionSummary[];
  states: Record<string, ConnectionState>;
  // P25: reported once at startup and never changes for the life of the process — the
  // connection file dialog's secrets section reads this directly.
  secretStorage: SecretStorageStatus | null;
}

export const useConnectionsStore = defineStore('connections', () => {
  const state = reactive<ConnectionsState>({
    records: [],
    states: {},
    secretStorage: null,
  });

  // P39 iter2 F8: connectionsState.records is a plain array — every one of its twenty-six
  // find-by-id call sites re-derived this same predicate. Accepts null/undefined so a call site's
  // own `connectionId ? … : undefined` ternary collapses into the call.
  function connectionRecord(id: string | null | undefined): ConnectionSummary | undefined {
    if (!id) return undefined;
    return state.records.find((r) => r.id === id);
  }

  let unsubscribeState: (() => void) | null = null;
  let unsubscribeListChanged: (() => void) | null = null;

  async function hydrateConnections(): Promise<void> {
    const [records, states, secretStorage] = await Promise.all([
      control.connectionsList(),
      control.connectionsStates(),
      control.connectionsSecretsStatus(),
    ]);
    state.records = records;
    for (const s of states) state.states[s.connectionId] = s;
    state.secretStorage = secretStorage;

    unsubscribeState?.();
    unsubscribeState = control.onConnectionState((s) => {
      state.states[s.connectionId] = s;
    });

    // Covers any mutation that didn't go through this store's own wrappers below (duplicateConnection(),
    // ...) — a file edit, a connection created via a direct IPC call — so the tree never silently
    // diverges from what main actually persisted.
    unsubscribeListChanged?.();
    unsubscribeListChanged = control.onConnectionsChanged((records) => {
      state.records = records;
    });
  }

  // P57 finding: `onConnectionsChanged`'s own full-replace (above) and this function's optimistic
  // append are independently scheduled — the Wails server-mode transport (tests/e2e-real/) delivers
  // the event on an independent WebSocket ahead of the bound call's own HTTP response often enough
  // to expose it, and nothing in the desktop transport actually guarantees the opposite order
  // either. Pushing a record the event's replace already added duplicates the row; upserting instead
  // of pushing makes either arrival order produce the same one-record result.
  function upsertRecord(record: ConnectionSummary): void {
    const idx = state.records.findIndex((r) => r.id === record.id);
    if (idx >= 0) state.records[idx] = record;
    else state.records.push(record);
  }

  async function duplicateConnection(id: string): Promise<ConnectionSummary> {
    const created = await control.connectionsDuplicate(id);
    upsertRecord(created);
    return created;
  }

  // Optimistic like the tab strip; a rejection (an id deleted in another window) restores the
  // persisted order before rethrowing.
  async function reorderConnections(ids: string[]): Promise<void> {
    const byId = new Map(state.records.map((r) => [r.id, r]));
    const listed = ids.flatMap((id) => byId.get(id) ?? []);
    state.records = [...listed, ...state.records.filter((r) => !ids.includes(r.id))];
    try {
      state.records = await control.connectionsReorder(ids);
    } catch (err) {
      state.records = await control.connectionsList();
      throw err;
    }
  }

  async function moveConnection(id: string, step: -1 | 1): Promise<void> {
    const ids = state.records.map((r) => r.id);
    const from = ids.indexOf(id);
    const to = from + step;
    if (from < 0 || to < 0 || to >= ids.length) return;
    [ids[from], ids[to]] = [ids[to], ids[from]];
    await reorderConnections(ids);
  }

  async function deleteConnection(id: string): Promise<void> {
    await control.connectionsDelete(id);
    state.records = state.records.filter((r) => r.id !== id);
    delete state.states[id];
  }

  async function connectConnection(id: string): Promise<void> {
    const s = await control.connectionsConnect(id);
    state.states[id] = s;
  }

  async function disconnectConnection(id: string): Promise<void> {
    const s = await control.connectionsDisconnect(id);
    state.states[id] = s;
  }

  // Writes the username into the connection's file and seals the password. Go reconnects a live
  // connection, and the file load pushes the new record.
  async function updateConnectionCredentials(
    id: string,
    credentials: { username?: string; password?: string },
  ): Promise<void> {
    await control.connectionsUpdateCredentials(id, credentials);
  }

  return {
    ...toRefs(state),
    connectionRecord,
    hydrateConnections,
    upsertRecord,
    duplicateConnection,
    reorderConnections,
    moveConnection,
    deleteConnection,
    connectConnection,
    disconnectConnection,
    updateConnectionCredentials,
  };
});
