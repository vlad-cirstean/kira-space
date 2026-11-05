import type { ConnectionSummary } from '@shared/domain/connection';
import type { ControlSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { installClipboardSpy, lastClipboardWrite } from './support/clipboard';
import { createConnection, newConnectionArgs } from './support/connect';
import { acceptConfirm } from './support/dialogs';
import { IPC } from './support/ipcChannels';
import { connectionRow } from './support/tree';

// Connection menu and lifecycle over the file-backed config (P475). Fields live in the file, so
// these scenarios cover what stays in the app: create, duplicate, delete, copy URI, and the menu
// no longer offering a colour or read-only edit. The file dialog is covered by connection-files.

function summary(over: Partial<ConnectionSummary> & Pick<ConnectionSummary, 'id' | 'name'>) {
  const base: ConnectionSummary = {
    file: 'fixture.yaml',
    id: over.id,
    name: over.name,
    kind: 'postgres',
    color: 'none',
    mode: 'fields',
    readOnly: false,
    host: '127.0.0.1',
    port: 5432,
    database: 'testdb',
    username: 'testuser',
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
    mcpReviewResults: 'inherit',
    mcpStatementTimeoutSec: 10,
    mcpTables: null,
    maskReviewOn: false,
    sortOrder: 0,
    createdAt: '2026-01-01T00:00:00.000Z',
    updatedAt: '2026-01-01T00:00:00.000Z',
  };
  return { ...base, ...over };
}

// A colour retired from the picker still paints its own rail.
const RETIRED = summary({ id: 'conn-retired', name: 'Retired Colour Conn', color: 'orange' });
const TEST_PG = summary({ id: 'conn-test-pg', name: 'Test PG', color: 'green', sortOrder: 1 });
const URI_CONNECTION = summary({
  id: 'conn-uri',
  name: 'URI Connection',
  mode: 'uri',
  host: null,
  port: null,
  database: null,
  username: null,
  sortOrder: 2,
});
const DUPLICATE_OF_TEST_PG = summary({
  id: 'conn-test-pg-copy',
  name: 'Test PG copy',
  color: 'green',
  sortOrder: 3,
});

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [RETIRED, URI_CONNECTION] },
  {
    channel: IPC.connectionsCreate,
    args: newConnectionArgs('Test PG', 'postgres'),
    response: TEST_PG,
  },
  {
    channel: IPC.connectionsReveal,
    args: { id: URI_CONNECTION.id, confirmed: false },
    response: {
      outcome: 'revealed',
      password: null,
      uri: 'postgresql://uriuser:secretpw@10.0.0.9:5555/uridb',
      error: null,
    },
  },
  { channel: IPC.connectionsDuplicate, args: { id: TEST_PG.id }, response: DUPLICATE_OF_TEST_PG },
  { channel: IPC.connectionsDelete, args: { id: DUPLICATE_OF_TEST_PG.id }, response: null },
];

test('connection menu and lifecycle over files', async ({ relaunch }) => {
  test.setTimeout(120_000);
  const { window: page, control } = await relaunch({ control: CONTROL });
  await installClipboardSpy(page);

  const retiredRow = await connectionRow(page, 'Retired Colour Conn');
  await expect(retiredRow).toBeVisible();
  await expect(retiredRow.locator('[data-testid="tree-rail"]')).toHaveClass(/\bbg-conn-orange\b/);

  // New connection: engine and name only; the file dialog opens next.
  await page.click('[data-testid="add-connection"]');
  await expect(page.locator('[data-testid="new-connection-dialog"]')).toBeVisible();
  await page.click('[data-testid="connection-kind-postgres"]');
  await page.fill('[data-testid="new-connection-name"]', 'Test PG');
  await page.click('[data-testid="new-connection-create"]');
  await expect(page.locator('[data-testid="new-connection-dialog"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="connection-file-dialog"]')).toBeVisible();
  await page.click('[data-testid="connection-file-close"]');
  await expect(page.locator('[data-testid="connection-file-dialog"]')).toHaveCount(0);

  const row = await connectionRow(page, 'Test PG');
  await expect(row).toBeVisible();
  await expect(row.locator('[data-testid="tree-rail"]')).toHaveClass(/\bbg-conn-green\b/);
  await expect(row.locator('.status-dot')).toHaveAttribute('data-status', 'disconnected');

  // The menu edits the file; colour and read-only are no longer menu items.
  await row.click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-edit"]')).toBeVisible();
  await expect(page.locator('[data-testid="menu-item-open-file"]')).toBeVisible();
  await expect(page.locator('[data-testid="menu-item-reveal-file"]')).toBeVisible();
  await expect(page.locator('[data-testid="menu-item-color"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="menu-item-readonly"]')).toHaveCount(0);
  await page.keyboard.press('Escape');

  // Copy URI on a URI-mode row is a gated reveal.
  const revealCalls = () => control.log().filter((e) => e.channel === IPC.connectionsReveal);
  const uriConnRow = await connectionRow(page, 'URI Connection');
  await uriConnRow.click({ button: 'right' });
  await page.click('[data-testid="menu-item-copy-uri"]');
  await expect
    .poll(() => lastClipboardWrite(page))
    .toBe('postgresql://uriuser:secretpw@10.0.0.9:5555/uridb');
  expect(revealCalls()).toHaveLength(1);

  // Duplicate, then delete with confirm.
  const rowCount = () => page.locator('[data-testid="tree-row"][data-kind="connection"]').count();
  const beforeDuplicate = await rowCount();
  await (await connectionRow(page, 'Test PG')).click({ button: 'right' });
  await page.click('[data-testid="menu-item-duplicate"]');
  await expect.poll(rowCount).toBe(beforeDuplicate + 1);
  await expect(await connectionRow(page, 'Test PG copy')).toBeVisible();

  await (await connectionRow(page, 'Test PG copy')).click({ button: 'right' });
  await page.click('[data-testid="menu-item-delete"]');
  await acceptConfirm(page);
  await expect.poll(rowCount).toBe(beforeDuplicate);
});

test('a file kind asks for its database file', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [] },
      {
        channel: IPC.connectionsCreate,
        args: newConnectionArgs('Local DB', 'sqlite', '/tmp/local.db'),
        response: summary({
          id: 'conn-sqlite',
          name: 'Local DB',
          kind: 'sqlite',
          database: '/tmp/local.db',
        }),
      },
    ],
  });
  await createConnection(page, 'sqlite', 'Local DB', '/tmp/local.db');
  await expect(await connectionRow(page, 'Local DB')).toBeVisible();
});
