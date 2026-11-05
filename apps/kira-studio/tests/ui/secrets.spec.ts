import type { ConnectionSummary } from '@shared/domain/connection';
import type { ConnectionConfigView, ConnectionFileDetail } from '@shared/domain/connectionConfig';
import type { ControlSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { IPC } from './support/ipcChannels';
import { connectionRow } from './support/tree';

// The secrets section of a connection's file dialog (P475). Secret values are write-only: the
// section lists names with set or unset state, saves a typed value once and never echoes it. The
// storage status comes from the View: with no usable store the inputs are off. Backend half:
// dbflow TestConnectionSecrets and the connfiles tests; this file only covers the rendering.

const PASSWORD = 'Zx9!pw;Qq7';

const CONN: ConnectionSummary = {
  file: 'pg.yaml',
  id: 'c-pg',
  name: 'Pg',
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
  mcpReview: { read: 'inherit', write: 'inherit', ddl: 'inherit', masked: 'inherit' },
  mcpMaskedMode: 'inherit',
  mcpStatementTimeoutSec: 10,
  mcpTables: null,
  maskReviewOn: false,
  sortOrder: 0,
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
};

function view(available: boolean, set: boolean): ConnectionConfigView {
  return {
    dir: '/kira/connections',
    files: [{ file: 'pg.yaml', id: CONN.id, name: 'Pg', problem: null, lastGood: false }],
    secrets: [{ kind: 'password', name: 'pg.password', set, referenced: true }],
    storage: {
      available,
      backend: available ? 'keychain' : 'unavailable',
      insecureFallback: false,
      reason: available ? null : 'the platform keychain could not be reached',
    },
  };
}

function detail(set: boolean): ConnectionFileDetail {
  return {
    file: 'pg.yaml',
    id: CONN.id,
    path: '/kira/connections/pg.yaml',
    // biome-ignore lint/suspicious/noTemplateCurlyInString: a secret reference in file text
    text: 'name: Pg\nkind: postgres\npassword: ${secret:pg.password}\n',
    hash: 'hash-1',
    problem: null,
    lastGood: false,
    editable: true,
    secrets: [{ kind: 'password', name: 'pg.password', set, referenced: true }],
  };
}

function control(available: boolean, set: boolean): ControlSnapshot[] {
  return [
    { channel: IPC.connectionsList, response: [CONN] },
    { channel: IPC.connectionConfigView, response: view(available, set) },
    { channel: IPC.connectionConfigFile, response: detail(set) },
  ];
}

async function openFile(page: import('@playwright/test').Page): Promise<void> {
  await connectionRow(page, 'Pg').click({ button: 'right' });
  await page.click('[data-testid="menu-item-edit"]');
  await expect(page.locator('[data-testid="connection-file-dialog"]')).toBeVisible();
}

test('with secret storage the section lists the name and takes a value once', async ({
  relaunch,
}) => {
  const { window: page, control: handle } = await relaunch({
    control: [
      ...control(true, false),
      {
        channel: IPC.connectionConfigSetSecret,
        args: { kind: 'password', name: 'pg.password', value: PASSWORD },
        response: view(true, true),
      },
    ],
  });
  const logged: string[] = [];
  page.on('console', (m) => logged.push(m.text()));

  await openFile(page);
  const row = page.locator('[data-testid="connection-secret-pg.password"]');
  await expect(row).toBeVisible();
  await expect(row.locator('[data-testid="connection-secret-state"]')).toHaveText('Not set');
  await expect(page.locator('[data-testid="connection-secrets-storage-off"]')).toHaveCount(0);

  const input = row.locator('[data-testid="connection-secret-value"]');
  await expect(input).toBeEnabled();
  await expect(input).toHaveAttribute('type', 'password');
  await input.fill(PASSWORD);
  await row.locator('[data-testid="connection-secret-save"]').click();
  await expect(input).toHaveValue('');
  expect(handle.log().filter((e) => e.channel === IPC.connectionConfigSetSecret)).toHaveLength(1);
  expect(logged.join('\n')).not.toContain(PASSWORD);
});

test('without secret storage the section warns and the input is off', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: control(false, false) });

  await openFile(page);
  await expect(page.locator('[data-testid="connection-secrets-storage-off"]')).toBeVisible();
  const row = page.locator('[data-testid="connection-secret-pg.password"]');
  await expect(row.locator('[data-testid="connection-secret-value"]')).toBeDisabled();
  await expect(row.locator('[data-testid="connection-secret-save"]')).toBeDisabled();
});
