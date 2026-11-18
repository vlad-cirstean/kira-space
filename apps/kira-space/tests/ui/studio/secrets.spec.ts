import type { ConnectionSummary } from '@shared/domain/connection';
import type { ConnectionConfigView, ConnectionFileDetail } from '@shared/domain/connectionConfig';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { connectionRow } from './support/tree';

// P670: the file dialog's secrets section is gone; the General tab's password and URI fields carry
// secret state (set or not) and explain a missing store. Backend half: dbflow TestConnectionSecrets
// and the connfiles tests; this file only covers the rendering.

// biome-ignore lint/suspicious/noTemplateCurlyInString: a secret reference in file text
const PASSWORD_REF = '${secret:pg.password}';

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
  explainThresholdRows: 0,
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

// The View answers dbflow TestConnectionSecrets recorded: the secret set, then deleted.
const VIEW_SET = contract<ConnectionConfigView>(
  'connection-secrets',
  'ConnectionConfigService.View',
);
const VIEW_UNSET = contract<ConnectionConfigView>(
  'connection-secrets',
  'ConnectionConfigService.View#unset',
);

function view(available: boolean, set: boolean): ConnectionConfigView {
  const recorded = set ? VIEW_SET : VIEW_UNSET;
  return {
    ...recorded,
    dir: '/kira/connections',
    files: recorded.files.map((f) => ({ ...f, id: CONN.id })),
    storage: available
      ? recorded.storage
      : {
          available: false,
          backend: 'unavailable',
          insecureFallback: false,
          reason: 'the platform keychain could not be reached',
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
    secrets: (set ? VIEW_SET : VIEW_UNSET).secrets,
  };
}

function control(available: boolean, set: boolean): ControlSnapshot[] {
  return [
    { channel: IPC.connectionsList, response: [CONN] },
    { channel: IPC.connectionConfigView, response: view(available, set) },
    { channel: IPC.connectionConfigFile, response: detail(set) },
    {
      channel: IPC.connectionConfigForm,
      response: {
        text: detail(set).text,
        problem: null,
        value: { name: 'Pg', kind: 'postgres', mode: 'fields', password: PASSWORD_REF },
      },
    },
  ];
}

async function openFile(page: import('@playwright/test').Page): Promise<void> {
  await connectionRow(page, 'Pg').click({ button: 'right' });
  await page.click('[data-testid="menu-item-edit"]');
  await expect(page.locator('[data-testid="connection-file-dialog"]')).toBeVisible();
}

const TABS = ['general', 'advanced', 'preconnect', 'mcp', 'privacy', 'yaml'];

test('with secret storage the General tab shows the password as not set and no section on any tab', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: control(true, false) });

  await openFile(page);
  const input = page.locator('[data-testid="connection-password"]');
  await expect(input).toBeEnabled();
  await expect(input).toHaveAttribute('placeholder', 'Not set');
  await expect(page.locator('[data-testid="connection-secret-storage-off"]')).toHaveCount(0);
  for (const tab of TABS) {
    await page.locator(`[data-testid="connection-tab-${tab}"]`).click();
    await expect(page.locator('[data-testid="connection-secrets"]')).toHaveCount(0);
  }
});

test('without secret storage the General tab explains and the password field is off', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: control(false, false) });

  await openFile(page);
  await expect(page.locator('[data-testid="connection-secret-storage-off"]')).toBeVisible();
  await expect(page.locator('[data-testid="connection-password"]')).toBeDisabled();
});
