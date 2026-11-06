// The renderer-side store for a container's cached columns — filled on a view's own
// lifecycle hook (ConsoleView.vue's own onMounted is the only caller today), never from a
// CompletionSource, never on a keystroke (the language layer never fetches). Lives in state/,
// not views/, for the same reason state/schemas.ts does: three different view kinds read it, and
// views/<kind>/* may not import another views/<kind>/* (biome.json).
import {
  type ColumnMeta,
  decodePath,
  encodePath,
  type PathSegment,
  type RelationColumns,
} from '@shared/domain/tree';
import { defineStore } from 'pinia';
import { reactive, toRefs } from 'vue';
import { control } from '../bridge/control';
import { rowKey, useTreeStore } from '../project/state/tree';
import {
  type DdlColumn,
  type DdlSchema,
  type DdlTable,
  EMPTY_DDL_SCHEMA,
} from '../views/console/ddl';
import { useConnectionsStore } from './connections';

const RELATION_CONTAINER_KINDS = new Set(['database', 'schema']);

/** containerPathFor's root branch — a console opened at the connection root (path === '') has
 *  no database:/schema: segment to walk, but it is not scope-less at runtime: every SQL adapter's
 *  Execute resolves an empty path to the connection's own configured database
 *  (internal/adapters/*'s own primary-connection fallback). This resolves the SAME container from
 *  data already in treeStore.children — no new IPC call, no Go change — in order:
 *  1. the root's own `detail === "connected"` database node (postgres/mysqlfamily both stamp this
 *     on the node matching the live connection);
 *  2. else the root database node whose name matches the connection record's own `database` field
 *     (covers ClickHouse, which has no "connected" marker);
 *  3. else, if the root has exactly one database node, that one (covers SQLite, whose `database`
 *     field is a file path and whose tree node is named "main");
 *  4. for postgres only, one level deeper: `schema:public` among that database's already-loaded
 *     children, else its sole non-system schema if there's exactly one — else null even though the
 *     database itself resolved, since a database-only path is rejected outright by every postgres
 *     call this feeds (SchemaColumns requires database:X/schema:Y exactly) and would be a
 *     functionally dead container, not a partial win.
 *  Returns null when nothing resolves (the connection was never expanded, or it's genuinely
 *  ambiguous) — the same honest degradation containerPathFor already had for every other
 *  unresolvable case, not a new failure mode. Reads treeStore.children, so it's reactive: a console
 *  opened before the connection was expanded self-warms the moment the user expands it, since
 *  containerPath is a computed in ConsoleView.vue and ensureSchemaColumns is single-flight. */
function rootContainerPathFor(connectionId: string): string | null {
  const treeStore = useTreeStore();
  const roots = treeStore.children[rowKey(connectionId, '')];
  if (!roots || roots.length === 0) return null;
  const databases = roots.filter((n) => n.kind === 'database');
  if (databases.length === 0) return null;

  const record = useConnectionsStore().connectionRecord(connectionId);
  const dbNode =
    databases.find((n) => n.detail === 'connected') ??
    databases.find((n) => n.name === record?.database) ??
    (databases.length === 1 ? databases[0] : undefined);
  if (!dbNode) return null;

  const dbSegment: PathSegment = { kind: 'database', name: dbNode.name };
  if (record?.kind !== 'postgres') return encodePath([dbSegment]);

  // Postgres requires database:X/schema:Y exactly for SchemaColumns (and every other
  // container-scoped call) — a database-only path is rejected outright, so stopping there would
  // be a functionally dead container, not a partial win. Resolve a path only once the schema is
  // unambiguous too; otherwise null, same honest degradation as every other unresolvable case.
  const schemas = (treeStore.children[rowKey(connectionId, encodePath([dbSegment]))] ?? []).filter(
    (n) => n.kind === 'schema',
  );
  const schemaNode =
    schemas.find((n) => n.name === 'public') ?? (schemas.length === 1 ? schemas[0] : undefined);
  if (!schemaNode) return null;
  return encodePath([dbSegment, { kind: 'schema', name: schemaNode.name }]);
}

/** Walks a console/table/etc.'s own path back to the last database:/schema: segment —
 *  the exact container completion.ts's own consoleRelationNames already resolves relation names
 *  against, factored out here so the two can never disagree about what "this container" means.
 *  A path with no such segment falls to rootContainerPathFor only when it's the connection
 *  root itself (path === '') — every other unresolvable path still yields null unchanged. */
export function containerPathFor(connectionId: string, path: string): string | null {
  let segments: PathSegment[];
  try {
    segments = decodePath(connectionId, path).segments;
  } catch {
    return null;
  }
  for (let i = segments.length - 1; i >= 0; i--) {
    if (RELATION_CONTAINER_KINDS.has(segments[i]?.kind ?? '')) {
      return encodePath(segments.slice(0, i + 1));
    }
  }
  return path === '' ? rootContainerPathFor(connectionId) : null;
}

function toDdlColumn(col: ColumnMeta): DdlColumn {
  return {
    name: col.name,
    type: col.dataType,
    primaryKey: col.isPrimaryKey || undefined,
    notNull: col.nullable ? undefined : true,
    description: col.comment ?? undefined,
  };
}

function ddlSchemaFromCached(relations: readonly RelationColumns[]): DdlSchema {
  const tables: DdlTable[] = relations.map((rc) => ({
    name: rc.name,
    isView: rc.kind === 'table' ? undefined : true,
    columns: rc.columns.map(toDdlColumn),
  }));
  return { tables };
}

export const useSchemaColumnsStore = defineStore('schemaColumns', () => {
  // Keyed by rowKey(connectionId, containerPath) — the SAME key project/state/tree.ts uses for its
  // own children cache, so the two never disagree about what "this container" means.
  const state = reactive({
    byContainer: {} as Record<string, RelationColumns[]>,
  });

  const pendingLoads = new Set<string>();

  // Bumped by dropSchemaColumns, per connection — the ConsoleView.vue watch below reads it as a
  // third source so an invalidation re-triggers a fetch. `byContainer[key]`
  // alone can't do that: the watch's own source is (connectionId, containerPath), neither of which
  // changes on invalidation, so a plain-object drop with no reactive signal left nothing to re-run
  // ensureSchemaColumns until the tab remounted.
  const generation = reactive<Record<string, number>>({});

  /** Read by ConsoleView.vue's own warm-on-container watch (F5) — see `generation` above. */
  function generationFor(connectionId: string): number {
    return generation[connectionId] ?? 0;
  }

  /** Idempotent, single-flight, fire-and-forget. Called on tab activation; never from a
   *  CompletionSource, never on a keystroke. Resolves from the Go-side cache with no
   *  connection when one is cached, and is a no-op when the container is already loaded
   *  or a fetch for it is already in flight. */
  async function ensureSchemaColumns(connectionId: string, containerPath: string): Promise<void> {
    const key = rowKey(connectionId, containerPath);
    if (state.byContainer[key] || pendingLoads.has(key)) return;
    pendingLoads.add(key);
    // Captured before the await, checked after — dropSchemaColumns bumps this on
    // every drop (reconnect, explicit Refresh). Without the check, a fetch already in flight when a
    // drop lands writes its now-stale result straight back into byContainer[key] once it resolves,
    // undoing the drop with no further fetch to correct it (the guard above then treats that stale
    // write as "already loaded").
    const startGeneration = generationFor(connectionId);
    try {
      const result = await control.treeSchemaColumns(connectionId, containerPath);
      if (generationFor(connectionId) !== startGeneration) return;
      state.byContainer[key] = result.relations;
    } catch {
      // Same discipline as views/grid/state.ts's own loadMeta: a nicety that warms completion, not
      // something a failure here should block reading rows or opening a console over.
      //
      // Deliberately NOT memoized as `[]` (a prior comment here argued a failure
      // should be cached the same as a genuinely-empty container, to stop it re-firing on every
      // activation/reconnect) — but `byContainer[key] || pendingLoads.has(key)` above treats `[]`
      // as "loaded", so a TRANSIENT failure (SchemaColumns's own Disconnected-on-cache-miss while a
      // console restores before its connection finishes connecting) got permanently stuck
      // empty: every later retry (a reconnect, the generation bump) short-circuited on the
      // cached `[]` and never called treeSchemaColumns again. Leaving `byContainer[key]` unset
      // costs nothing extra: the generation bump already re-triggers this on every drop/reconnect,
      // and a container the adapter genuinely, permanently rejects keeps retrying on the same
      // schedule a real empty container already gets polled on (once per mount/reconnect)
      // — never on a keystroke, so there is no tight retry loop to worry about.
    } finally {
      pendingLoads.delete(key);
    }
  }

  /** The raw cached relations for a container — read by ConsoleView.vue to feed
   *  sqlCompletionSources' own `cached` parameter directly, in the shape the completion
   *  source wants (via namespaceFromCached), separately from
   *  effectiveSchema's DdlSchema-shaped projection below (the lint/hover supply). A plain property
   *  lookup that returns [] on a miss — never a fetch (the guard: nothing exported here can
   *  trigger one). */
  function cachedRelationsFor(
    connectionId: string,
    containerPath: string,
  ): readonly RelationColumns[] {
    return state.byContainer[rowKey(connectionId, containerPath)] ?? [];
  }

  /** The cached columns projected into the DdlSchema shape the three console language providers
   *  (completion, diagnostics, hover) already share — so they can never disagree about what
   *  the console knows. The precedence: a hand-authored document still wins WHOLESALE the moment
   *  it declares any table — never merged table-by-table with the cache — since a user who pasted
   *  one deliberately (a read replica they can't introspect, a schema they're designing before it
   *  exists) keeps getting exactly what they get today. The cache only fills the case that was
   *  previously empty. */
  function effectiveSchema(
    connectionId: string,
    containerPath: string,
    document: DdlSchema,
  ): DdlSchema {
    if (document.tables.length > 0) return document;
    const cached = cachedRelationsFor(connectionId, containerPath);
    if (cached.length === 0) return EMPTY_DDL_SCHEMA;
    return ddlSchemaFromCached(cached);
  }

  /** Drops this connection's cached columns — every container (containerPath omitted) or just one
   *  (P24 D10). Shared by the reconnect-invalidation sync below and project/state/tree.ts's three
   *  explicit-refresh actions (refresh/refreshConnection/refreshObject), so an explicit Refresh
   *  clears the console's own copy of 'columns' the same way a reconnect already does (F11: before
   *  this, only a reconnect ever reached this store — a tree Refresh dropped the Go-side cache but
   *  left completion/diagnostics/hover offering the previous column set until the next reconnect). */
  function dropSchemaColumns(connectionId: string, containerPath?: string): void {
    generation[connectionId] = (generation[connectionId] ?? 0) + 1;
    if (containerPath !== undefined) {
      delete state.byContainer[rowKey(connectionId, containerPath)];
      return;
    }
    const prefix = rowKey(connectionId, '');
    for (const key of Object.keys(state.byContainer)) {
      if (key.startsWith(prefix)) delete state.byContainer[key];
    }
  }

  let unsubscribeInvalidated: (() => void) | null = null;

  /** Rides the same broadcast project/state/tree.ts already listens to for metadata invalidation
   *  (D3) — one subscription, mirroring an existing one, not a second invalidation mechanism. */
  function initSchemaColumnsSync(): void {
    unsubscribeInvalidated?.();
    unsubscribeInvalidated = control.onConnectionMetadataInvalidated((connectionId) => {
      dropSchemaColumns(connectionId);
    });
  }

  return {
    ...toRefs(state),
    ensureSchemaColumns,
    cachedRelationsFor,
    effectiveSchema,
    dropSchemaColumns,
    initSchemaColumnsSync,
    generationFor,
  };
});
