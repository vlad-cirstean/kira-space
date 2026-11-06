import type { OpRecord } from '@shared/domain/ops';
import { createOpLogStore } from '@workbench/state/createOpLogStore';
import { control } from '../bridge/control';
import { useConnectionsStore } from './connections';

// P132 Part 1 (§2.2): the shared skeleton now lives in
// packages/workbench/src/state/createOpLogStore.ts — this app's own extra is the connection-name
// filter match (searchText below), resolved per call since the connections store may not exist at
// module load.
export const useOpsStore = createOpLogStore<OpRecord>({
  control: {
    recent: (limit) => control.opsRecent(limit),
    onUpdate: (cb) => control.onOpUpdate(cb),
    cancel: (id) => control.opsCancel(id),
  },
  facet: (r) => r.connectionId ?? '',
  searchText: (r) => useConnectionsStore().connectionRecord(r.connectionId)?.name ?? '',
});
