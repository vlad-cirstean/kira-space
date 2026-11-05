import type { Page } from '@playwright/test';
import type { ConnectionSummary } from '@shared/domain/connection';
import type {
  ConnectionConfigView,
  ConnectionFileDetail,
  ConnectionSecret,
} from '@shared/domain/connectionConfig';
import type { ControlSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { acceptConfirm, cancelConfirm } from './support/dialogs';
import { IPC } from './support/ipcChannels';

// Confirm-before-reveal for saved credentials, now from a connection file's secrets section
// (P475). Follows secrets.spec.ts's pattern: mocking ConnectionsService.Reveal's response per
// scenario, rather than branching on the host OS or a real LocalAuthentication prompt this tier
// cannot drive. All four RevealResult outcomes run here, on any OS: revealed, cancelled,
// confirmation-required (the non-macOS/OS-auth-unavailable fallback this sandbox always takes),
// and error.

const CONN: ConnectionSummary = {
  file: 'reveal.yaml',
  id: 'conn-reveal',
  name: 'Reveal Test DB',
  kind: 'postgres',
  color: 'blue',
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
  sortOrder: 0,
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
};

const PASSWORD_SECRET: ConnectionSecret = {
  kind: 'password',
  name: 'reveal.password',
  set: true,
  referenced: true,
};
const URI_SECRET: ConnectionSecret = {
  kind: 'uri',
  name: 'reveal.uri',
  set: true,
  referenced: true,
};

function boot(secret: ConnectionSecret, extra: ControlSnapshot[]): ControlSnapshot[] {
  const view: ConnectionConfigView = {
    dir: '/kira/connections',
    files: [{ file: CONN.file, id: CONN.id, name: CONN.name, problem: null, lastGood: false }],
    secrets: [secret],
    storage: { available: true, backend: 'keychain', insecureFallback: false, reason: null },
  };
  const detail: ConnectionFileDetail = {
    file: CONN.file,
    id: CONN.id,
    path: '/kira/connections/reveal.yaml',
    text: `name: ${CONN.name}\n`,
    hash: 'hash-1',
    problem: null,
    lastGood: false,
    editable: true,
    secrets: [secret],
  };
  return [
    { channel: IPC.connectionsList, response: [CONN] },
    { channel: IPC.connectionConfigView, response: view },
    { channel: IPC.connectionConfigFile, response: detail },
    ...extra,
  ];
}

function connectionRow(page: Page) {
  return page
    .locator('[data-testid="tree-row"][data-kind="connection"]')
    .filter({ hasText: CONN.name });
}

async function openFile(page: Page): Promise<void> {
  await connectionRow(page).click({ button: 'right' });
  await page.click('[data-testid="menu-item-edit"]');
  await expect(page.locator('[data-testid="connection-file-dialog"]')).toBeVisible();
}

const field = (page: Page) => page.locator('[data-testid="connection-secret-value"]');
const revealButton = (page: Page) => page.locator('[data-testid="connection-secret-value-reveal"]');

test('opening the file does not fetch or show the secret', async ({ relaunch }) => {
  // No connectionsReveal snapshot at all: an open that fetched the plaintext would fail on the
  // unmocked call, and the call count below catches a regression that swallowed it.
  const { window: page, control } = await relaunch({ control: boot(PASSWORD_SECRET, []) });

  await openFile(page);

  await expect(field(page)).toHaveValue('');
  await expect(revealButton(page)).toBeVisible();
  expect(control.log().filter((e) => e.channel === IPC.connectionsReveal)).toHaveLength(0);
});

test('revealing shows the value once per press', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: boot(PASSWORD_SECRET, [
      {
        channel: IPC.connectionsReveal,
        args: { id: CONN.id, confirmed: false },
        response: { password: 'hunter2', error: null, outcome: 'revealed' },
      },
    ]),
  });

  await openFile(page);
  await revealButton(page).click();
  await expect(field(page)).toHaveValue('hunter2');
  expect(control.log().filter((e) => e.channel === IPC.connectionsReveal)).toHaveLength(1);
});

test('a cancelled reveal shows nothing', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: boot(PASSWORD_SECRET, [
      {
        channel: IPC.connectionsReveal,
        args: { id: CONN.id, confirmed: false },
        response: { password: null, error: null, outcome: 'cancelled' },
      },
    ]),
  });

  await openFile(page);
  await revealButton(page).click();
  await expect(field(page)).toHaveValue('');
  await expect(page.locator('[data-testid="connection-secret-error"]')).toHaveCount(0);
});

test('OS auth unavailable routes through the in-app confirmation', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: boot(PASSWORD_SECRET, [
      {
        channel: IPC.connectionsReveal,
        args: { id: CONN.id, confirmed: false },
        response: { password: null, error: null, outcome: 'confirmation-required' },
      },
      {
        channel: IPC.connectionsReveal,
        args: { id: CONN.id, confirmed: true },
        response: { password: 'hunter2', error: null, outcome: 'revealed' },
      },
    ]),
  });

  await openFile(page);

  // First press: the confirm dialog names the connection; cancelling reveals nothing and makes no
  // second Reveal call.
  await revealButton(page).click();
  await expect(page.locator('[data-testid="confirm-dialog-message"]')).toHaveText(
    `“${CONN.name}” displays the password in plain text.`,
  );
  await cancelConfirm(page);
  await expect(field(page)).toHaveValue('');
  expect(control.log().filter((e) => e.channel === IPC.connectionsReveal)).toHaveLength(1);

  // Second press, confirming: the next Reveal call carries confirmed: true.
  await revealButton(page).click();
  await acceptConfirm(page);
  await expect(field(page)).toHaveValue('hunter2');
  const revealCalls = control.log().filter((e) => e.channel === IPC.connectionsReveal);
  expect(revealCalls).toHaveLength(3);
  expect(revealCalls[2]?.args).toEqual({ id: CONN.id, confirmed: true });
});

test("a reveal error renders in the section's error slot", async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: boot(PASSWORD_SECRET, [
      {
        channel: IPC.connectionsReveal,
        args: { id: CONN.id, confirmed: false },
        response: {
          password: null,
          error: 'The stored credential could not be decrypted.',
          outcome: 'error',
        },
      },
    ]),
  });

  await openFile(page);
  await revealButton(page).click();
  await expect(page.locator('[data-testid="connection-secret-error"]')).toHaveText(
    'The stored credential could not be decrypted.',
  );
  await expect(field(page)).toHaveValue('');
});

test('a URI secret reveals the URI, not a password', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: boot(URI_SECRET, [
      {
        channel: IPC.connectionsReveal,
        args: { id: CONN.id, confirmed: false },
        response: {
          outcome: 'revealed',
          password: null,
          uri: 'postgresql://u:hunter2@h:5432/db',
          error: null,
        },
      },
    ]),
  });

  await openFile(page);
  await revealButton(page).click();
  await expect(field(page)).toHaveValue('postgresql://u:hunter2@h:5432/db');
  expect(control.log().filter((e) => e.channel === IPC.connectionsReveal)).toHaveLength(1);
});
