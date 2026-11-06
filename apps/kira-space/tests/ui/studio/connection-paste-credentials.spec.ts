import type { Page } from '@playwright/test';
import {
  type ConnectionKind,
  type ConnectionSummary,
  connectionStateSchema,
} from '@shared/domain/connection';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { connectionRow } from './support/tree';

// P248: "Update credentials…" on a saved connection takes a pasted block. The pasted password
// must never reach the console. The connection file dialog holds no password field (P475).

const PASSWORD = 'Zx9!pw;Qq7'; // distinctive; contains `;` to exercise label-based value bounds

const BASE: Omit<ConnectionSummary, 'id' | 'name' | 'kind' | 'mode' | 'sortOrder'> = {
  file: 'fixture.yaml',
  color: 'blue',
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
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
};

function conn(
  id: string,
  name: string,
  kind: ConnectionKind,
  mode: 'fields' | 'uri',
  sortOrder: number,
): ConnectionSummary {
  return { ...BASE, id, name, kind, mode, sortOrder };
}

const PG = conn('c-pg', 'Fields PG', 'postgres', 'fields', 0);
const PG_URI = conn('c-uri', 'Uri PG', 'postgres', 'uri', 1);
const LITE = conn('c-lite', 'File DB', 'sqlite', 'fields', 2);
const SQS = conn('c-sqs', 'Queue', 'sqs', 'fields', 3);

const CREDENTIAL_CALLS: readonly string[] = [
  IPC.connectionsUpdateCredentials,
  IPC.connectionsDisconnect,
  IPC.connectionsConnect,
];

function consoleLog(page: Page): string[] {
  const seen: string[] = [];
  page.on('console', (m) => seen.push(m.text()));
  page.on('pageerror', (e) => seen.push(String(e)));
  return seen;
}

async function paste(page: Page, text: string): Promise<void> {
  await page.fill('[data-testid="paste-credentials-input"]', text);
}

test('tree menu offers Update credentials only for fields-mode network connections', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.connectionsList, response: [PG, PG_URI, LITE, SQS] }],
  });

  const item = page.locator('[data-testid="menu-item-update-credentials"]');
  const expected: [ConnectionSummary, number][] = [
    [PG, 1],
    [PG_URI, 0],
    [LITE, 0],
    [SQS, 0],
  ];
  for (const [c, count] of expected) {
    await connectionRow(page, c.name).click({ button: 'right' });
    await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
    await expect(item).toHaveCount(count);
    await page.keyboard.press('Escape');
    await expect(page.locator('[data-testid="context-menu"]')).toHaveCount(0);
  }
});

test('Update credentials sends only user and password in one call; the backend owns the reconnect', async ({
  relaunch,
}) => {
  const state = {
    connectionId: PG.id,
    status: 'connected',
    serverVersion: '16.0',
    error: null,
    since: 1735689600000,
    caps: null,
  };
  const control: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [PG] },
    { channel: IPC.connectionsStates, response: [state] },
    { channel: IPC.connectionsUpdateCredentials, response: null },
  ];
  const { window: page, control: handle } = await relaunch({ control });
  const logged = consoleLog(page);

  await connectionRow(page, PG.name).click({ button: 'right' });
  await page.click('[data-testid="menu-item-update-credentials"]');
  await expect(page.locator('[data-testid="credentials-update-dialog"]')).toBeVisible();
  await expect(page.locator('[data-testid="credentials-update-live-note"]')).toBeVisible();

  // Host, port and database are recognised but outside this dialog's scope.
  await paste(page, `PGHOST=elsewhere\nPGUSER=rotated\nPGPASSWORD=${PASSWORD}`);
  await expect(page.locator('[data-testid="paste-credentials-also-found"]')).toContainText('host');
  await page.click('[data-testid="paste-credentials-apply"]');
  await expect(page.locator('[data-testid="credentials-update-dialog"]')).toHaveCount(0);

  const calls = handle.log().filter((e) => CREDENTIAL_CALLS.includes(e.channel));
  expect(calls.map((e) => e.channel)).toEqual([IPC.connectionsUpdateCredentials]);
  expect(calls[0]?.args).toEqual({ id: PG.id, username: 'rotated', password: PASSWORD });
  expect(logged.join('\n')).not.toContain(PASSWORD);
});

test('Update credentials with a password only sends no username', async ({ relaunch }) => {
  // Backend half: dbflow TestPostgresUpdateCredentials (complete suite).
  const state = {
    ...contract('paste-credentials', 'ConnectionsService.Connect#connected', {
      schema: connectionStateSchema.omit({ caps: true }),
    }),
    connectionId: PG.id,
    caps: null,
  };
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [PG] },
      { channel: IPC.connectionsStates, response: [state] },
      { channel: IPC.connectionsUpdateCredentials, response: null },
    ],
  });

  await connectionRow(page, PG.name).click({ button: 'right' });
  await page.click('[data-testid="menu-item-update-credentials"]');
  await paste(page, `password: ${PASSWORD}`);
  await page.click('[data-testid="paste-credentials-apply"]');
  await expect(page.locator('[data-testid="credentials-update-dialog"]')).toHaveCount(0);

  const update = control.log().find((e) => e.channel === IPC.connectionsUpdateCredentials);
  expect(update?.args).toEqual({ id: PG.id, username: null, password: PASSWORD });
});
