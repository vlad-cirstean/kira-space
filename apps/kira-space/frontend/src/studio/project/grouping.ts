// P19 D2/D3: renderer-only tree grouping — no adapter changes, no new NodeKind, no path segment.
// A pure transform over an already-fetched child list (project/state/tree.ts's buildRows() is the
// one caller), so it costs no round trip and survives a cached listing unchanged.
import type { ConnectionKind } from '@shared/domain/connection';
import type { NodeKind, TreeNode } from '@shared/domain/tree';
import { type MessageKey, t } from '@theme/i18n';
import { nodeKindLabel } from '../state/nodeKindLabel';

// P28 D14: the one place a kind gets a plural human name (singular names live in nodeKindLabel).
// Every NodeKind has an entry, not just the five that get their own P19 folder — the checkbox
// filter's Object types section (P28) needs a label for every kind present under a connection.
const PLURAL_KEYS: Record<NodeKind, MessageKey> = {
  connection: 'common.module.connections.label',
  database: 'studio.project.group.database.label',
  schema: 'studio.project.group.schema.label',
  table: 'studio.project.group.table.label',
  view: 'studio.project.group.view.label',
  matview: 'studio.project.group.matview.label',
  function: 'studio.project.group.function.label',
  sequence: 'studio.project.group.sequence.label',
  column: 'studio.common.columns.label',
  collection: 'studio.api.collections.title',
  namespace: 'studio.project.group.namespace.label',
  key: 'studio.browse.list.aria',
  topic: 'studio.project.group.topic.label',
  partition: 'studio.project.group.partition.label',
  consumerGroup: 'studio.project.group.consumerGroup.label',
  queue: 'studio.project.group.queue.label',
  bucket: 'studio.project.group.bucket.label',
  prefix: 'studio.project.group.prefix.label',
  object: 'studio.project.filtersDialog.objects.heading.label',
};

// MariaDB's and MySQL's `function` nodes include stored procedures (P34 F21b), and §5.1 calls that
// level "routines".
function isRoutineKind(kind: NodeKind, connectionKind: ConnectionKind): boolean {
  return kind === 'function' && (connectionKind === 'mariadb' || connectionKind === 'mysql');
}

/** The display label for any node kind, per connection kind. Plural for a folder or a checkbox
 *  row ("Views"), singular where a single object is named. The one place a kind gets a human
 *  name — GROUPED_KINDS' own labels now derive from it rather than duplicating it. */
export function labelForKind(
  kind: NodeKind,
  connectionKind: ConnectionKind,
  form: 'singular' | 'plural' = 'plural',
): string {
  const routine = isRoutineKind(kind, connectionKind);
  if (form === 'singular')
    return routine ? t('studio.project.nodeKind.routine.label') : nodeKindLabel(kind);
  return t(routine ? 'studio.project.group.routine.label' : PLURAL_KEYS[kind]);
}

// The complete, curated list of kinds that get their own folder, in render order. Any kind not
// named here is never grouped and keeps its position — which is what leaves redis namespaces/keys
// and s3 prefixes/objects exactly as they are today.
const GROUPED_KINDS: readonly { kind: NodeKind }[] = [
  { kind: 'view' },
  { kind: 'matview' },
  { kind: 'sequence' },
  { kind: 'function' },
  // P23 D1 (revised): kafka follows the same rule as SQL — the primary kind (topics) shows first,
  // ungrouped, and only the auxiliary kind (consumer groups) folders. Topics are what a user
  // browses; a folder around them would bury the thing this tree exists to show.
  { kind: 'consumerGroup' },
];

const GROUPED_KIND_SET = new Set(GROUPED_KINDS.map((g) => g.kind));

export function labelForGroup(kind: NodeKind, connectionKind: ConnectionKind): string {
  return labelForKind(kind, connectionKind, 'plural');
}

// Splits an already-filtered child list into [ungrouped, folders] — ungrouped in adapter order
// first, then one folder per non-empty GROUPED_KINDS entry, in table order.
export function partitionChildren(nodes: TreeNode[]): {
  ungrouped: TreeNode[];
  groups: { kind: NodeKind; nodes: TreeNode[] }[];
} {
  const ungrouped: TreeNode[] = [];
  const byKind = new Map<NodeKind, TreeNode[]>();
  for (const node of nodes) {
    if (!GROUPED_KIND_SET.has(node.kind)) {
      ungrouped.push(node);
      continue;
    }
    const bucket = byKind.get(node.kind);
    if (bucket) bucket.push(node);
    else byKind.set(node.kind, [node]);
  }
  const groups = GROUPED_KINDS.map((g) => ({
    kind: g.kind,
    nodes: byKind.get(g.kind) ?? [],
  })).filter((g) => g.nodes.length > 0);
  return { ungrouped, groups };
}

// Kinds whose rows never expand in the tree, whatever a cached TreeNode's `hasChildren` says
// — a view's/matview's columns moved into the definition view, and the adapters already stopped
// emitting column children, but a cached L1 payload can still carry `hasChildren: true` until the
// connection's next reconnect. `topic` is included for the same reason: its partitions moved
// into the definition view too. `table` is not here: see isLeafNode.
//
// `keyBrowser`, true only for a connection whose Caps say so (redis/s3), adds `database`/
// `bucket` to the same stale-cache guard — the adapters already report `hasChildren: false` for
// them (redis/catalog.ts, s3/catalog.ts), and their own key space is now browsed in a Browse tab
// instead. Not a bare `NodeKind` entry the way `table`/`topic` are: `database` is shared
// by five engines that must stay expand-only (postgres/mysql-family/sqlite/clickhouse/mongo), so
// the cut needs the connection's own capability, not the kind alone.
function isLeafKind(kind: NodeKind, keyBrowser = false): boolean {
  if (keyBrowser && (kind === 'database' || kind === 'bucket')) return true;
  return kind === 'view' || kind === 'matview' || kind === 'topic';
}

/** A table expands only when the adapter says so: a Postgres partitioned table lists its
 *  partitions. view/matview/topic stay leaves whatever a cached node says (P19 D5). */
export function isLeafNode(
  node: Pick<TreeNode, 'kind' | 'hasChildren'>,
  keyBrowser = false,
): boolean {
  return node.kind === 'table' ? !node.hasChildren : isLeafKind(node.kind, keyBrowser);
}
