import type { Page } from '@playwright/test';
import type { ConnectionKind } from '@shared/domain/connection';
import { DEFAULT_PORT } from '@shared/domain/connection';
import { expect } from '../fixtures';
import { APP_PATH, DB_PATH } from './postgresFixture';
import { connectionRow, expandRow, openRowMenu } from './tree';

// connectionCreateArgs/connectAndExpand/connectRedis/connectMongo/openConsoleFromMenu
// were copied, byte-identically or nearly so, across 8/5/2/2/6 spec files (a genuinely divergent
// connectAndExpand stays local to cell-editor.spec.ts and leaks.spec.ts — see their own doc
// comments). One copy here, imported everywhere.

/** The `connections.create` args the new-connection dialog sends: the engine's defaults only, the
 *  rest is edited in the file. `database` is set for file kinds only. */
export function newConnectionArgs(
  name: string,
  kind: string,
  database: string | null = null,
): Record<string, unknown> {
  return {
    name,
    kind,
    color: 'none',
    mode: 'fields',
    readOnly: false,
    host: database === null ? 'localhost' : null,
    port: database === null ? (DEFAULT_PORT[kind as ConnectionKind] ?? null) : null,
    database,
    username: null,
    password: null,
    uri: null,
    options: {},
    preconnect: null,
    preconnectSidecar: false,
    preconnectDelaySec: 0,
    autoExplain: false,
    throttlePerSec: 0,
    mcpEnabled: false,
    mcpDescription: '',
    mcpReadMode: 'allow',
    mcpWriteMode: 'prompt',
    mcpDdlMode: 'deny',
    mcpAutoExplain: true,
    mcpReview: { read: 'inherit', write: 'inherit', ddl: 'inherit', masked: 'inherit' },
    mcpMaskedMode: 'inherit',
    mcpStatementTimeoutSec: 10,
    mcpTables: null,
    maskReviewOn: false,
  };
}

/** A `connections.create` args fixture for a Postgres connection. The colour lives in the file now,
 *  so the call carries none. */
export function connectionCreateArgs(name: string, _color?: string): Record<string, unknown> {
  return newConnectionArgs(name, 'postgres');
}

/** Creates a connection through the new-connection dialog and closes the file dialog that opens
 *  next. The canned `Create` response decides the row; the file holds everything else. */
export async function createConnection(
  page: Page,
  kind: string,
  name: string,
  database?: string,
): Promise<void> {
  await page.click('[data-testid="add-connection"]');
  await page.click(`[data-testid="connection-kind-${kind}"]`);
  await page.fill('[data-testid="new-connection-name"]', name);
  if (database !== undefined) await page.fill('[data-testid="new-connection-database"]', database);
  await page.click('[data-testid="new-connection-create"]');
  await expect(page.locator('[data-testid="new-connection-dialog"]')).toHaveCount(0);
  await page.click('[data-testid="connection-file-close"]');
  await expect(page.locator('[data-testid="connection-file-dialog"]')).toHaveCount(0);
}

export interface ConnectAndExpandOptions {
  readonly autoExplain?: boolean;
}

/** Fills the "add connection" dialog for a Postgres connection through the UI, connects it, and
 *  expands the root row plus `DB_PATH`/`APP_PATH` — the shape `console.spec.ts`, `console-
 *  explain.spec.ts`, `interaction.spec.ts`, `data-view.spec.ts` and `definition.spec.ts` all
 *  shared already. */
export async function connectAndExpand(
  page: Page,
  name: string,
  _color: string,
  _options: ConnectAndExpandOptions = {},
): Promise<void> {
  await createConnection(page, 'postgres', name);

  const connRow = connectionRow(page);
  await expect(connRow).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);
}

/** Fills the "add connection" dialog for a Redis connection and connects it — no tree expand (no
 *  schema/database hierarchy to descend into). */
export async function connectRedis(page: Page, name: string, _color: string): Promise<void> {
  await createConnection(page, 'redis', name);

  const connRow = connectionRow(page);
  await expect(connRow).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
}

export interface ConnectMongoOptions {
  /** Expands the root row plus `dbPath` after connecting — off by default (console-format.spec.ts's
   *  own shape never expands); pass `dbPath` whenever `expand` is true. */
  readonly expand?: boolean;
  readonly dbPath?: string;
}

/** Fills the "add connection" dialog for a MongoDB connection and connects it. */
export async function connectMongo(
  page: Page,
  name: string,
  _color: string,
  options: ConnectMongoOptions = {},
): Promise<void> {
  await createConnection(page, 'mongodb', name);

  const connRow = connectionRow(page);
  await expect(connRow).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  if (options.expand) {
    if (!options.dbPath) throw new Error('connectMongo: dbPath is required when expand is true');
    await expandRow(page, '');
    await expandRow(page, options.dbPath);
  }
}

export async function openConsoleFromMenu(page: Page, path: string): Promise<void> {
  await openRowMenu(page, path);
  await page.click('[data-testid="menu-item-open-console"]');
}

/** Disconnects the connection whose root row is shown, through the tree menu. */
export async function disconnectConnection(page: Page): Promise<void> {
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-disconnect"]');
}
