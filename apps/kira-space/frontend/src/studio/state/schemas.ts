import type { ConnectionKind } from '@shared/domain/connection';
import type { ConnectionDdl } from '@shared/domain/schema';
import { useMutation } from '@tanstack/vue-query';
import { t } from '@theme/i18n';
import { queryClient } from '@workbench/state/queryClient';
import { defineStore } from 'pinia';
import { reactive, toRefs } from 'vue';
import { control } from '../bridge/control';
import type { EditorCompletionSource } from '../editor/completion';
import { type DdlSchema, EMPTY_DDL_SCHEMA, parseDdl } from '../views/console/ddl';
import { sqlKeywordCompletionSource } from '../views/console/sqlKeywordCompletion';
import { type SqlDialect, sqlDialectFor } from '../views/shared/sqlIdent';

// Each connection's own pasted DDL document — app-wide like
// connections/settings/layout (docs/ARCHITECTURE.md's UI architecture section), keyed by
// connectionId. A connection with no saved document has no entry, matching the "absent
// until the user writes one" (never an error, never a placeholder row).
//
// The text itself lives in TanStack Query's cache under this key, not in a reactive —
// ensureQueryData's own in-flight dedupe replaces the old hand-written pendingLoads map, and its
// cache replaces the old byConnection. A rejection is not cached, matching the old `finally` evict.
export function schemaQueryKey(connectionId: string): readonly ['schema', string] {
  return ['schema', connectionId] as const;
}

// Per-connection counter bumped by every authoritative write straight into the schema query
// cache (saveDdl and applyRemote below, both through commitDdl) — lets schemaQueryOptions's
// queryFn tell a stale in-flight fetch apart from a fresh one, without falling back to "prefer
// whatever's cached", which made a genuine remote push permanently invisible: applyRemote's
// invalidateQueries always triggered a refetch, and that refetch's queryFn always re-returned the
// old cached value, so the cache could never move on a remote write.
const writeGeneration = new Map<string, number>();

/** Writes `ddl` into the query cache for `connectionId` and bumps its write generation, so a
 *  queryFn fetch already in flight for this connection (captured generation, see below) knows its
 *  own result is now stale and must not overwrite this write. */
function commitDdl(connectionId: string, ddl: string): void {
  queryClient.setQueryData(schemaQueryKey(connectionId), ddl);
  writeGeneration.set(connectionId, (writeGeneration.get(connectionId) ?? 0) + 1);
}

export function schemaQueryOptions(connectionId: string): {
  queryKey: readonly ['schema', string];
  queryFn: () => Promise<string>;
} {
  return {
    queryKey: schemaQueryKey(connectionId),
    queryFn: async () => {
      const generation = writeGeneration.get(connectionId) ?? 0;
      const ddl = await control.schemaGet(connectionId).then((r) => r.ddl);
      // F1: a local saveDdl or a remote applyRemote committed straight into the cache (bumping
      // the generation) while this fetch was in flight — that write is newer than this response,
      // so keep it instead of overwriting it with this fetch's now-stale result.
      if ((writeGeneration.get(connectionId) ?? 0) !== generation) {
        return queryClient.getQueryData<string>(schemaQueryKey(connectionId)) ?? ddl;
      }
      return ddl;
    },
  };
}

/** Memoised per connectionId (D3's dialog and C5's completion source both call this) — Query's own
 *  in-flight dedupe means a fetch only ever happens once per connection per session, shared with
 *  any component's own `useQuery` for the same key (schemaQueryOptions above). */
export function ensureDdl(connectionId: string): Promise<string> {
  return queryClient.ensureQueryData(schemaQueryOptions(connectionId));
}

/** SchemaDialog.vue's own Save — writes the saved text straight into the same query cache entry
 *  ensureDdl reads, via commitDdl, which is what schemaQueryOptions's own queryFn comment above
 *  protects: a save landing while a fetch is still in flight is picked up by that fetch's own
 *  resolution instead of being clobbered by it. A plain function (not just a `useMutation`
 *  `mutationFn`) so it stays directly callable/testable outside a component, the same way
 *  `ensureDdl` already is. */
export async function saveDdl(connectionId: string, ddl: string): Promise<string> {
  const result = await control.schemaSet(connectionId, ddl);
  commitDdl(connectionId, result.ddl);
  return result.ddl;
}

export function useSaveDdlMutation() {
  return useMutation({
    mutationFn: ({ connectionId, ddl }: { connectionId: string; ddl: string }) =>
      saveDdl(connectionId, ddl),
  });
}

export const useSchemaDialogStore = defineStore('schemaDialog', () => {
  const dialog = reactive({
    open: false,
    connectionId: null as string | null,
  });

  function openSchemaDialog(connectionId: string): void {
    dialog.open = true;
    dialog.connectionId = connectionId;
  }

  function closeSchemaDialog(): void {
    dialog.open = false;
    dialog.connectionId = null;
  }

  return { ...toRefs(dialog), openSchemaDialog, closeSchemaDialog };
});

// Memoised per (connectionId, textHash) so a keystroke in the console never
// re-parses the DDL — keyed here by the raw text itself rather than a hash, since the whole point
// is a cheap `===` check against the one string per connection the caller already holds (its own
// `useQuery`/`ensureDdl` result).
const parsedCache = new Map<string, { text: string; schema: DdlSchema }>();

/** The parsed DdlSchema for `connectionId`'s current DDL `text`, empty when there is none or
 *  when `dialect` is undefined (a non-SQL console never calls this). Takes `text` directly (the
 *  caller's own query cache read) rather than looking it up itself, so this stays a plain
 *  memoised parse with no TanStack Query dependency of its own. */
export function ddlSchemaFor(
  connectionId: string,
  text: string | undefined,
  dialect: SqlDialect | undefined,
): DdlSchema {
  if (!text || !dialect) return EMPTY_DDL_SCHEMA;
  const cached = parsedCache.get(connectionId);
  if (cached?.text === text) return cached.schema;
  const schema = parseDdl(dialect, text);
  parsedCache.set(connectionId, { text, schema });
  return schema;
}

// project/ (SchemaDialog.vue, menus.ts) must not import views/ directly — these four
// wrappers are its one dispatch point into the SQL surface (views/shared/sqlIdent.ts,
// views/console/ddl.ts, views/console/sqlKeywordCompletion.ts), mirroring state/viewCommands.ts's
// own role for other project/ callers.

/** undefined for a kind with no SQL surface — SchemaDialog.vue's own guard for whether a
 *  connection even has a DDL document to edit, and MonacoHost.vue's `sql-dialect` prop. */
export function schemaDialectFor(kind: ConnectionKind | undefined): SqlDialect | undefined {
  return sqlDialectFor(kind);
}

/** SchemaDialog.vue passes `:autocomplete="true"` with no `completionSources` and used
 *  to rely on lang-sql's own implicit language-data keyword source (its own now-stale comment at
 *  the mount site said so). `MonacoHost.vue`'s `override`-shaped `completionSources` prop has no
 *  such implicit fallback, so this is that keyword/type source, made explicit — undefined for a
 *  kind with no SQL surface, matching `schemaDialectFor`. */
export function sqlKeywordCompletionSourceFor(
  kind: ConnectionKind | undefined,
): EditorCompletionSource | undefined {
  const dialect = sqlDialectFor(kind);
  return dialect && sqlKeywordCompletionSource(dialect);
}

/** D3's live parse summary — "N tables, M columns", or an explanatory line when nothing was
 *  recognised; null while there's nothing to summarise yet (empty text, or no SQL dialect). */
export function ddlParseSummary(kind: ConnectionKind | undefined, text: string): string | null {
  if (!text.trim()) return null;
  const dialect = sqlDialectFor(kind);
  if (!dialect) return null;
  const schema = parseDdl(dialect, text);
  if (schema.tables.length === 0) return t('studio.project.schemaDialog.noTables.status');
  const columns = schema.tables.reduce((n, table) => n + table.columns.length, 0);
  return t('common.commaPair.label', {
    first: t('studio.project.schemaDialog.tables.status', { count: schema.tables.length }),
    second: t('studio.grid.columnCount.status', { count: columns }),
  });
}

function applyRemote(ddl: ConnectionDdl): void {
  // The push already carries the fresh DDL, so write it straight into the cache (via commitDdl)
  // rather than invalidating and refetching — an invalidate-driven refetch always ran into
  // schemaQueryOptions's own "prefer cache" guard and could never move the cache forward. An active
  // observer (an open SchemaDialog or console on this connection) picks this up the same way any
  // other queryClient.setQueryData does: reactively, with no refetch.
  // commitDdl's generation bump also protects this write from a fetch that was already in flight
  // when this push landed.
  commitDdl(ddl.connectionId, ddl.ddl);
}

let unsubscribeChanged: (() => void) | null = null;
let unsubscribeConnectionsChanged: (() => void) | null = null;

// D2/D4: a document lives as long as its connection does (not tree-cache-like — nothing here
// evicts on disconnect, unlike project/state/tree.ts's own dropConnectionState), so the only
// cleanup this module owns is dropping a deleted connection's entry.
export function initSchemaSync(): void {
  unsubscribeChanged?.();
  unsubscribeChanged = control.onSchemaChanged(applyRemote);

  unsubscribeConnectionsChanged?.();
  unsubscribeConnectionsChanged = control.onConnectionsChanged((records) => {
    const liveIds = new Set(records.map((r) => r.id));
    for (const query of queryClient.getQueryCache().findAll({ queryKey: ['schema'] })) {
      const id = query.queryKey[1] as string;
      if (!liveIds.has(id)) {
        // P12 round 1 finding #11: parsedCache is keyed on connectionId too, and this module's
        // own comment above claims dropping a deleted connection's entry is the only cleanup it
        // owns — measured ~521 KiB per stale entry left behind without this.
        queryClient.removeQueries({ queryKey: query.queryKey });
        parsedCache.delete(id);
      }
    }
  });
}
