import { createOpLogStore } from '@workbench/state/createOpLogStore';
import { control } from '../bridge/control';
import { useConnectionsStore } from '../studio/state/connections';
import type { SpaceOpRecord } from './opsDomain';

// P415: the shared op-log store over Kira Space's persisted op log. Background reads arrive batched
// and stay hidden until the toggle. The filter also matches repo, origin, module and subject.
// The facet dropdown filters by module; each database connection is its own entry in place of the
// shared "database" module.
export const DB_FACET_PREFIX = 'database:';

export function opFacet(r: SpaceOpRecord): string {
  return r.module === 'database' ? `${DB_FACET_PREFIX}${r.connectionId ?? ''}` : r.module;
}

export const useOpsStore = createOpLogStore<SpaceOpRecord>({
  control: {
    recent: (limit) => control.opsRecent(limit),
    onUpdate: (cb) => control.onOpUpdate(cb),
    onBatch: (cb) => control.onOpBatch(cb),
    cancel: (id) => control.opsCancel(id),
  },
  background: (r) => r.background,
  facet: opFacet,
  searchText: (r) =>
    `${r.repoName} ${r.source} ${r.module} ${r.subject} ${
      useConnectionsStore().connectionRecord(r.connectionId)?.name ?? ''
    }`,
});
