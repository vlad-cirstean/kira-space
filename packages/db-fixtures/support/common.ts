import type { ResolvedConnectionConfig } from './connectionConfig';

/** The `ResolvedConnectionConfig` fields every db-fixtures `start()` prologue sets identically —
 *  only id/name/kind/host/port/database/username/password vary per engine. */
export function baseConnectionConfig(
  overrides: Pick<
    ResolvedConnectionConfig,
    'id' | 'name' | 'kind' | 'host' | 'port' | 'database' | 'username' | 'password'
  >,
): ResolvedConnectionConfig {
  const now = new Date().toISOString();
  return {
    sortOrder: 0,
    createdAt: now,
    updatedAt: now,
    color: 'blue',
    mode: 'fields',
    readOnly: false,
    uri: null,
    options: {},
    autoExplain: false,
    throttlePerSec: 0,
    mcpEnabled: false,
    mcpDescription: '',
    mcpReadMode: 'allow',
    mcpWriteMode: 'prompt',
    mcpDdlMode: 'deny',
    mcpAutoExplain: true,
    mcpReviewResults: 'inherit',
    mcpStatementTimeoutSec: 10,
    mcpTables: null,
    maskReviewOn: false,
    ...overrides,
  };
}
