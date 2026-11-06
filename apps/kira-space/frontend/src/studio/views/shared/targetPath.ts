import { decodePath } from '@shared/domain/tree';
import { useConnectionsStore } from '../../state/connections';

// P48 F20: DataView.vue and DocumentView.vue each built this same view-head breadcrumb prefix —
// decode the path, drop the last segment (the target itself), prepend the connection name, join
// with ' / ' — byte-identical apart from how each reached the connection record.
export function ancestorPathPrefix(connectionId: string | null, path: string): string {
  if (!connectionId) return '';
  const connectionName = useConnectionsStore().connectionRecord(connectionId)?.name;
  const segments = decodePath(connectionId, path).segments;
  const parts = [connectionName, ...segments.slice(0, -1).map((s) => s.name)].filter(
    (p): p is string => !!p,
  );
  return parts.length ? `${parts.join(' / ')} / ` : '';
}
