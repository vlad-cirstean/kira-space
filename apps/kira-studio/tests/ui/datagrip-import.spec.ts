import type { ConnectionSummary } from '@shared/domain/connection';
import type { DataGripPreview, DataGripReport } from '@shared/domain/datagrip';
import type { SecretStorageStatus } from '@shared/domain/secrets';
import { CHANNEL } from '@shared/protocol/events';
import type { ControlSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// P28 D18: the panel button is gone — the DataGrip import is a menu-bar command now
// (App → Import DataGrip Connections…), so these specs drive the channel the menu item emits.
async function importFromMenu(page: Parameters<typeof emitWailsEvent>[0]): Promise<void> {
  await emitWailsEvent(page, CHANNEL.importDataGrip, undefined);
}

// §4.4.1: one importable Postgres source, one unsupported-engine Oracle source, one
// password-not-saved source, and one row that matches an already-existing connection
// (EXISTING below, by name+host+port+database).
const EXISTING: ConnectionSummary = {
  file: 'fixture.yaml',
  id: 'conn-existing',
  name: 'orders-db',
  kind: 'postgres',
  color: 'cyan',
  mode: 'fields',
  readOnly: false,
  host: 'db.example.internal',
  port: 5432,
  database: 'orders',
  username: 'app',
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

const PROJECT_DIR = '/Users/dev/orders-service';

const PREVIEW: DataGripPreview = {
  projectDir: PROJECT_DIR,
  rows: [
    {
      uuid: 'uuid-pg',
      name: 'pg-main',
      importable: true,
      kind: 'postgres',
      host: 'pg.example.internal',
      port: 5432,
      database: 'appdb',
      username: 'app',
      passwordOutlook: 'will-attempt',
      warnings: [],
    },
    {
      uuid: 'uuid-oracle',
      name: 'legacy-oracle',
      importable: false,
      skipReason: 'unsupported-engine',
      skipDetail: 'oracle',
      warnings: [],
    },
    {
      uuid: 'uuid-not-saved',
      name: 'ci-db',
      importable: true,
      kind: 'mysql',
      host: 'ci.example.internal',
      port: 3306,
      database: 'ci',
      username: 'ci',
      passwordOutlook: 'not-saved',
      warnings: [],
    },
    {
      uuid: 'uuid-dup',
      name: 'orders-db',
      importable: true,
      kind: 'postgres',
      host: 'db.example.internal',
      port: 5432,
      database: 'orders',
      username: 'app',
      passwordOutlook: 'will-attempt',
      warnings: [],
    },
  ],
};

const REPORT: DataGripReport = {
  rows: [
    { uuid: 'uuid-pg', name: 'pg-main', created: true, passwordImported: true },
    { uuid: 'uuid-not-saved', name: 'ci-db', created: true, passwordImported: false },
  ],
};

function baseControl(): ControlSnapshot[] {
  return [
    { channel: IPC.connectionsList, response: [EXISTING] },
    { channel: IPC.filesChooseFolder, response: { canceled: false, path: PROJECT_DIR } },
    { channel: IPC.datagripScan, response: PREVIEW },
  ];
}

test('datagrip import — the preview lists every row, greys the unsupported one, starts a probable duplicate unchecked, and confirm sends only the checked uuids', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    ...baseControl(),
    { channel: IPC.datagripImport, response: REPORT },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await importFromMenu(page);

  const dialog = page.locator('[data-testid="datagrip-import-dialog"]');
  await expect(dialog).toBeVisible();
  await expect(
    page.locator('[data-testid="datagrip-preview-rows"] > div[data-testid^="datagrip-row-"]'),
  ).toHaveCount(4);

  // The unsupported-engine row is greyed, has no checkbox, and names the driver.
  const oracleRow = page.locator('[data-testid="datagrip-row-uuid-oracle"]');
  await expect(oracleRow).toHaveAttribute('data-importable', 'false');
  // P104: reka's CheckboxRoot renders `<button role="checkbox">`, not a native
  // `<input type="checkbox">` -- matched by its own data-slot instead (tree.spec.ts precedent).
  await expect(oracleRow.locator('[data-slot="checkbox"]')).toHaveCount(0);
  await expect(oracleRow.locator('[data-testid="datagrip-row-skip-reason"]')).toContainText(
    'oracle',
  );

  // The password-not-saved row is still importable and checked by default, with its outlook shown.
  const notSavedRow = page.locator('[data-testid="datagrip-row-uuid-not-saved"]');
  await expect(notSavedRow.locator('[data-slot="checkbox"]')).toBeChecked();
  await expect(notSavedRow.locator('[data-testid="datagrip-row-outlook"]')).toContainText(
    'didn’t save',
  );

  // The row matching an existing connection is badged and starts unchecked (D10).
  const dupRow = page.locator('[data-testid="datagrip-row-uuid-dup"]');
  await expect(dupRow.locator('[data-testid="datagrip-row-duplicate"]')).toBeVisible();
  await expect(dupRow.locator('[data-slot="checkbox"]')).not.toBeChecked();

  // Confirm's label counts only checked rows: pg-main + ci-db, not the dup or the oracle row.
  await expect(page.locator('[data-testid="datagrip-import-confirm"]')).toHaveText(
    'Import 2 connections',
  );

  await page.click('[data-testid="datagrip-import-confirm"]');

  const importCalls = control.log().filter((e) => e.channel === IPC.datagripImport);
  expect(importCalls).toHaveLength(1);
  const args = importCalls[0].args as { path: string; selectedUuids: string[] };
  expect(args.path).toBe(PROJECT_DIR);
  expect([...args.selectedUuids].sort()).toEqual(['uuid-not-saved', 'uuid-pg']);

  // Review finding: the dialog must show the report, not auto-close — every row's outcome from
  // the Import call is rendered right here rather than discarded.
  await expect(dialog).toBeVisible();
  await expect(page.locator('[data-testid="datagrip-import-confirm"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="datagrip-report-row-uuid-pg"]')).toContainText(
    'Created with password',
  );
  await expect(page.locator('[data-testid="datagrip-report-row-uuid-not-saved"]')).toContainText(
    'Created',
  );

  await page.click('[data-testid="datagrip-import-report-close"]');
  await expect(dialog).toHaveCount(0);
});

test('datagrip import — the report renders every row outcome, including a failure, and closes only on an explicit Close', async ({
  relaunch,
}) => {
  const REPORT_WITH_FAILURE: DataGripReport = {
    rows: [
      { uuid: 'uuid-pg', name: 'pg-main', created: true, passwordImported: true },
      {
        uuid: 'uuid-not-saved',
        name: 'ci-db',
        created: false,
        passwordImported: false,
        error: 'name must be 1-120 characters',
      },
    ],
  };
  const CONTROL: ControlSnapshot[] = [
    ...baseControl(),
    { channel: IPC.datagripImport, response: REPORT_WITH_FAILURE },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await importFromMenu(page);
  await page.click('[data-testid="datagrip-import-confirm"]');

  const dialog = page.locator('[data-testid="datagrip-import-dialog"]');
  await expect(dialog).toBeVisible();

  // The summary counts both the success and the failure.
  await expect(page.locator('[data-testid="datagrip-report-summary"]')).toContainText('1 of 2');
  await expect(page.locator('[data-testid="datagrip-report-summary"]')).toContainText('1 failed');

  const okRow = page.locator('[data-testid="datagrip-report-row-uuid-pg"]');
  await expect(okRow.locator('[data-testid="datagrip-report-row-outcome"]')).toContainText(
    'Created with password',
  );

  const failedRow = page.locator('[data-testid="datagrip-report-row-uuid-not-saved"]');
  await expect(failedRow.locator('[data-testid="datagrip-report-row-outcome"]')).toContainText(
    'Not created',
  );
  await expect(failedRow.locator('[data-testid="datagrip-report-row-outcome"]')).toContainText(
    'name must be 1-120 characters',
  );

  // Escape/click-outside is still a valid way to dismiss it — only the auto-close on confirm is
  // gone — but the dialog must never disappear on its own.
  await expect(dialog).toBeVisible();
  await page.click('[data-testid="datagrip-import-report-close"]');
  await expect(dialog).toHaveCount(0);
});

test('datagrip import — cancelling the dialog never calls Import', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: baseControl() });

  await importFromMenu(page);
  await expect(page.locator('[data-testid="datagrip-import-dialog"]')).toBeVisible();

  await page.click('[data-testid="datagrip-import-cancel"]');
  await expect(page.locator('[data-testid="datagrip-import-dialog"]')).toHaveCount(0);
  expect(control.log().filter((e) => e.channel === IPC.datagripImport)).toHaveLength(0);
});

test('datagrip import — cancelling the folder picker never opens the dialog or calls Scan', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    { channel: IPC.filesChooseFolder, response: { canceled: true, path: null } },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await importFromMenu(page);
  await expect(page.locator('[data-testid="datagrip-import-dialog"]')).toHaveCount(0);
  expect(control.log().filter((e) => e.channel === IPC.datagripScan)).toHaveLength(0);
});

test('datagrip import — shows the secret-storage-unavailable banner and still lets rows import password-free', async ({
  relaunch,
}) => {
  const UNAVAILABLE: SecretStorageStatus = {
    available: false,
    backend: 'unavailable',
    insecureFallback: false,
    reason: 'No system keychain is available on Linux in this build.',
  };
  const CONTROL: ControlSnapshot[] = [
    ...baseControl(),
    { channel: IPC.connectionsSecretsStatus, response: UNAVAILABLE },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await importFromMenu(page);
  const banner = page.locator('[data-testid="datagrip-secrets-unavailable"]');
  await expect(banner).toBeVisible();
  await expect(banner).toContainText('No system keychain');

  // The rows underneath are unaffected — this dialog only ever shows what Scan itself found.
  await expect(
    page.locator('[data-testid="datagrip-preview-rows"] > div[data-testid^="datagrip-row-"]'),
  ).toHaveCount(4);
});

test('datagrip import — a failed scan opens the dialog on the error and offers another folder', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    { channel: IPC.filesChooseFolder, response: { canceled: false, path: PROJECT_DIR } },
    {
      channel: IPC.datagripScan,
      response: null,
      error: { code: 'E_DATAGRIP', message: 'no .idea/dataSources.xml in this folder' },
    },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await importFromMenu(page);
  await expect(page.locator('[data-testid="datagrip-import-error"]')).toContainText(
    'dataSources.xml',
  );
  await expect(page.locator('[data-testid="datagrip-preview-rows"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="datagrip-import-choose-another"]')).toBeEnabled();
  await expect(page.locator('[data-testid="datagrip-import-confirm"]')).toBeDisabled();
});
