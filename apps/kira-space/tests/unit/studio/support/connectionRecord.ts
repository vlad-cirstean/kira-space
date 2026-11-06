import type { ConnectionKind, ConnectionSummary } from '@shared/domain/connection';

/** Minimal record so `openTrackedTab` sees the connection as live. Takes the store: importing it
 *  here would load the bridge before a spec's own runtime mocks. */
export function seedConnectionRecord(
  store: {
    records: ConnectionSummary[];
    connectionRecord(id: string): ConnectionSummary | undefined;
  },
  id: string,
  kind: ConnectionKind = 'postgres',
): void {
  if (store.connectionRecord(id)) return;
  store.records.push({
    id,
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fixture, not a real ConnectionSummary
    ...({ kind, name: id, color: 'blue' } as any),
  } as ConnectionSummary);
}
