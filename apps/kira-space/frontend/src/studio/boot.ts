import { pinia } from '../state/pinia';
import { initApiDataSync } from './api/state/apiQueries';
import { useTreeStore } from './project/state/tree';
import { useCacheStatsStore } from './state/cacheStats';
import { initConnectionConfigSync } from './state/connectionConfig';
import { useConnectionsStore } from './state/connections';
import { useDbMcpStore } from './state/dbmcp';
import { initMaskRulesSync, loadMaskRuleCounts } from './state/maskRules';
import { initSavedQueriesSync } from './state/savedQueriesSync';
import { useSchemaColumnsStore } from './state/schemaColumns';
import { initSchemaSync } from './state/schemas';

/** Subscriptions the database and API modules need live before their panels ever mount, whether or
 *  not one mounts this session. Each is idempotent; needs no data, so it runs ahead of hydration. */
export function startStudioSync(): void {
  initApiDataSync();
  initConnectionConfigSync();
  useTreeStore(pinia).initTreeSync();
  initSchemaSync();
  useSchemaColumnsStore(pinia).initSchemaColumnsSync();
  initMaskRulesSync();
  initSavedQueriesSync();
  useCacheStatsStore(pinia).initCacheStats();
}

/** The database and API modules' own boot state. Joins the shell's critical hydrate group. */
export function hydrateStudio(): Promise<unknown[]> {
  return Promise.all([
    useConnectionsStore(pinia).hydrateConnections(),
    loadMaskRuleCounts(),
    useDbMcpStore(pinia).hydrateDbMcp(),
  ]);
}
