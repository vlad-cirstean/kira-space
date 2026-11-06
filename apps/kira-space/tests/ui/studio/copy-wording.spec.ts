import { installDockerMocks } from '../../../../../packages/docker-ui/src/testing/ui/dockerMock';
import { expect, test } from './fixtures';
import { modeTab, openHttpModeAndNewRequest } from './support/apiMode';
import { installClipboardSpy } from './support/clipboard';
import {
  connectPostgres,
  menuLabels,
  openCompositeTable,
  openContextMenu,
  PG_CONN,
  PG_CONTROL,
  PG_PORT,
} from './support/copyScenarios';
import { gutterCell } from './support/grid';
import { IPC } from './support/ipcChannels';
import { loc, openKubernetes, setupKubernetes } from './support/kubernetesFixture';
import { emitWailsEvent } from './support/mockRuntime';
import { COMPOSITE_PK_PATH } from './support/postgresFixture';
import { openSettings } from './support/settings';
import { readOnlySqsSnapshots, SQS_QUEUE_PATH } from './support/sqsFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';

// P265: pins the final wording of the dialogs and menus whose text changed most. Each assertion
// fails when a rewrite drifts from the voice the catalog fixed.

const BRIDGE_PKG = 'github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge';
const confirm = '[data-testid="confirm-dialog"]';

test('delete connection confirm: title, quoted name, Cancel focused', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.connectionsList, response: [PG_CONN] }],
  });
  await page
    .locator('[data-testid="tree-row"][data-kind="connection"]')
    .filter({ hasText: PG_CONN.name })
    .click({ button: 'right' });
  await page.click('[data-testid="menu-item-delete"]');

  const dialog = page.locator(confirm);
  await expect(dialog.locator('[data-slot="dialog-title"]')).toHaveText('Delete connection?');
  await expect(page.locator('[data-testid="confirm-dialog-message"]')).toContainText(
    `“${PG_CONN.name}” will be removed.`,
  );
  await expect(page.locator('[data-testid="confirm-dialog-cancel"]')).toHaveText('Cancel');
  await expect(page.locator('[data-testid="confirm-dialog-confirm"]')).toHaveText('Delete');
  await expect(page.locator('[data-testid="confirm-dialog-cancel"]')).toBeFocused();
});

test('clear HTTP response history confirm says it cannot be undone; button is Clear', async ({
  relaunch,
}) => {
  const entry = {
    id: 'e1',
    itemId: null,
    tabId: 't1',
    sentAt: '2024-06-01T10:00:00.000Z',
    method: 'GET',
    url: 'https://api.example.com/orders',
    environment: '',
    status: 200,
    statusText: 'OK',
    elapsedMs: 100,
    bodyBytes: 50,
    storedBytes: 140,
  };
  const { window: page } = await relaunch({
    control: [{ channel: IPC.historyList, response: [entry] }],
  });
  await openHttpModeAndNewRequest(page);
  await page.click('[data-testid="http-response-pane-history"]');
  await page.click('[data-testid="http-history-clear"]');

  await expect(page.locator(`${confirm} [data-slot="dialog-title"]`)).toHaveText(
    'Clear response history?',
  );
  await expect(page.locator('[data-testid="confirm-dialog-message"]')).toContainText(
    'This can’t be undone.',
  );
  await expect(page.locator('[data-testid="confirm-dialog-confirm"]')).toHaveText('Clear');
});

test('SQS poll confirm button reads Poll', async ({ relaunch }) => {
  const { control, stream } = readOnlySqsSnapshots([30]);
  const { window: page } = await relaunch({ control, stream });
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  await (await findRow(page, SQS_QUEUE_PATH)).dblclick();
  await page.locator('[data-testid="stream-poll"]').click();

  await expect(page.locator(`${confirm} [data-slot="dialog-title"]`)).toHaveText(
    'Poll this queue?',
  );
  await expect(page.locator('[data-testid="confirm-dialog-confirm"]')).toHaveText('Poll');
});

test('table row menu keeps its final label order', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: PG_CONTROL, stream: PG_PORT });
  await connectPostgres(page);
  await openRowMenu(page, COMPOSITE_PK_PATH);

  expect(await menuLabels(page)).toEqual([
    'Open data',
    'Open in new tab',
    'Open definition',
    'Open query console',
    'Refresh',
    'Copy name',
    'Copy qualified name',
    'Count rows',
    'Saved filters',
  ]);
});

test('Copy name announces Copied name', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: PG_CONTROL, stream: PG_PORT });
  await installClipboardSpy(page);
  await connectPostgres(page);
  await openRowMenu(page, COMPOSITE_PK_PATH);
  await page.click('[data-testid="menu-item-copy-name"]');

  await expect(page.locator('[data-testid="app-live-region"]')).toHaveText('Copied name');
});

test('grid delete label follows the selection size', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: PG_CONTROL, stream: PG_PORT });
  await openCompositeTable(page);

  await openContextMenu(gutterCell(page, 0));
  await expect(page.locator('[data-testid="menu-item-delete-row"]')).toContainText('Delete 1 row');
  await page.keyboard.press('Escape');

  await gutterCell(page, 0).click();
  await gutterCell(page, 1).click({ modifiers: ['Shift'] });
  await openContextMenu(gutterCell(page, 1));
  await expect(page.locator('[data-testid="menu-item-delete-row"]')).toContainText('Delete 2 rows');
});

test('collections empty state title and hint', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.collectionsList, response: { collections: [], items: [] } }],
  });
  await modeTab(page, 'api').click();

  const empty = page.locator('[data-testid="collections-empty"]');
  await expect(empty).toContainText('No collections yet');
  await expect(empty).toContainText('Create one from the');
  await expect(empty).toContainText('above, or import a Postman collection.');
});

test('QueryState failure shows Couldn’t load and Retry', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: [] });
  await installDockerMocks(page, {
    bridgePkg: BRIDGE_PKG,
    handlers: {
      Status: () => ({
        state: 'ok',
        endpoint: {
          context: 'default',
          host: 'unix:///var/run/docker.sock',
          source: 'default',
          secure: true,
          remote: false,
        },
        engine: {
          version: '29.0.0',
          apiVersion: '1.52',
          os: 'linux',
          arch: 'amd64',
          operatingSystem: 'Debian',
          kernelVersion: '6.1',
          cpus: 8,
          memTotal: 16 * 1024 ** 3,
          containers: 0,
          running: 0,
          paused: 0,
          stopped: 0,
          images: 0,
        },
      }),
      Containers: () => ({ error: { code: 'E_FAILED', message: 'bridge rejected' } }),
    },
  });
  await modeTab(page, 'docker').click();

  const panel = page.locator('[data-testid="docker-panel"]');
  await expect(panel.locator('[data-testid="query-error"]')).toContainText('Couldn’t load');
  await expect(panel.locator('[data-testid="query-retry"]')).toHaveText('Retry');
});

test('command palette placeholder and title', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: [] });
  await emitWailsEvent(page, IPC.commandPalette, null);

  await expect(page.locator('[data-testid="command-palette-input"]')).toHaveAttribute(
    'placeholder',
    'Type a command',
  );
  await expect(page.locator('[data-slot="dialog-title"]').first()).toHaveText('Command palette');
});

test('settings reset control reads Reset to default', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: [] });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Appearance"]');

  await expect(
    page.locator('[data-testid="settings-reset-appearance-rowDensity"]'),
  ).toHaveAttribute('aria-label', 'Reset to default');
});

test('Kubernetes unavailable states say what failed and offer Retry', async ({ relaunch }) => {
  const { page } = await setupKubernetes(relaunch);
  await openKubernetes(page);
  await page.locator('[data-testid="kube-cluster-row"][data-context="bad-token"]').click();
  const view = loc(page, 'kube-unavailable');
  await expect(view).toContainText('Sign-in rejected');
  await expect(view).toContainText('The cluster refused the credentials');
  await expect(loc(page, 'kube-retry')).toHaveText('Retry');

  await page.locator('[data-testid="kube-cluster-row"][data-context="plugin-ctx"]').click();
  await expect(view).toContainText('Credential plugin failed');
});
