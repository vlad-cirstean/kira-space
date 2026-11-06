import { installDockerMocks } from '../../../../packages/docker-ui/src/testing/ui/dockerMock';
import { expect, studioTest, test } from './fixtures';
import { modeTab, openHttpModeAndNewRequest } from './studio/support/apiMode';
import { installClipboardSpy } from './studio/support/clipboard';
import {
  connectPostgres,
  menuLabels,
  openCompositeTable,
  openContextMenu,
  PG_CONN,
  PG_CONTROL,
  PG_PORT,
} from './studio/support/copyScenarios';
import { gutterCell } from './studio/support/grid';
import { loc, openKubernetes, setupKubernetes } from './studio/support/kubernetesFixture';
import { COMPOSITE_PK_PATH } from './studio/support/postgresFixture';
import { readOnlySqsSnapshots, SQS_QUEUE_PATH } from './studio/support/sqsFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from './studio/support/tree';
import { adeBoard, adeFixture, adeV2Control, openPlan } from './support/adeV2';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// P265 Stream B: the wording of the confirm dialogs, empty states, plurals and the reset action.
// Each assertion pins the catalog text a user reads (title, body, verb buttons).

const t = (id: string) => `[data-testid="${id}"]`;

type Relaunch = Parameters<typeof openPlan>[0];

async function openReposDialog(relaunch: Relaunch, extra: ControlSnapshot[] = []) {
  const app = await relaunch({
    control: adeV2Control([{ channel: IPC.windowsEnsure, response: { mode: 'git' } }, ...extra]),
  });
  await app.window.locator(t('manage-repos')).click();
  await app.window.locator(t('repos-dialog')).waitFor();
  return app;
}

test('removing a repository says what stays and offers Remove / Cancel', async ({ relaunch }) => {
  const { window: page } = await openReposDialog(relaunch, [
    { channel: IPC.codeWorkspaceRemoveRepo },
  ]);
  await page.locator(t('repos-dialog-repo-remove')).click();
  const dialog = page.locator(t('confirm-dialog'));
  await expect(dialog).toContainText('Remove repository?');
  await expect(page.locator(t('confirm-dialog-message'))).toContainText('Files on disk stay.');
  await expect(page.locator(t('confirm-dialog-confirm'))).toHaveText('Remove');
  await expect(page.locator(t('confirm-dialog-cancel'))).toHaveText('Cancel');
});

test('force push asks with Cancel / Force push', async ({ relaunch }) => {
  const board = adeFixture<{ plan: { unpushed: Record<string, boolean> } }>('board');
  board.plan.unpushed = { b_deps: true };
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await page.locator(t('ade-force-push')).click();
  const dialog = page.locator(t('ade-confirm-dialog'));
  await expect(dialog).toContainText('Force push?');
  await expect(dialog.locator(t('ade-confirm-no'))).toHaveText('Cancel');
  await expect(dialog.locator(t('ade-confirm-yes'))).toHaveText('Force push');
});

test('abort rebase asks with Cancel / Abort', async ({ relaunch }) => {
  const board = adeBoard((b) => {
    const auth = b.branches.find((br) => br.id === 'b_auth');
    Object.assign(auth ?? {}, { base: 'develop', basePendingFrom: 'main', rebaseInProgress: true });
  });
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await page.locator('[data-testid="ade-branch-row"][data-branch-id="b_auth"]').click();
  await page.locator(t('ade-panel-action-abort-rebase')).click();
  await expect(page.getByText('Abort rebase?')).toBeVisible();
  await expect(page.locator(t('ade-confirm-no'))).toHaveText('Cancel');
  await expect(page.locator(t('ade-confirm-yes'))).toHaveText('Abort');
});

test('a stuck run offers Take over on the Needs you page', async ({ relaunch }) => {
  const board = adeBoard((b) => {
    for (const task of b.tasks) {
      for (const run of task.runs)
        if (String(run.id).startsWith('r_T_bill_impl_')) run.state = 'stuck';
    }
  });
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await page.locator(t('ade-tab-needs')).click();
  await expect(page.locator(t('ade-needs-item')).first().locator(t('ade-needs-action'))).toHaveText(
    'Take over',
  );
});

test('the plan board with no repositories says where to add them', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: adeV2Control([{ channel: IPC.codeWorkspaceListRepos, response: [] }]),
  });
  const view = page.locator(t('ade-view'));
  await expect(view).toContainText('No repositories yet');
  await expect(view).toContainText(
    'Add repositories from the Git module to start planning agent work.',
  );
});

test('the needs footer pluralises interactive sessions', async ({ relaunch }) => {
  const all = adeFixture<{ sessions: { mode: string }[] }>('sessions').sessions;
  const tui = all.filter((s) => s.mode === 'tui');
  const withCount = (n: number): ControlSnapshot => ({
    channel: IPC.adeTaskSessions,
    response: { sessions: tui.slice(0, n) },
  });

  const one = await openPlan(relaunch, [withCount(1)]);
  await one.window.locator(t('ade-tab-needs')).click();
  await expect(one.window.locator(t('ade-needs-footer'))).toHaveText(/ 1 interactive session$/);
});

test('discarding a memory import asks with Discard / Cancel', async ({ relaunch }) => {
  const job = {
    id: 'J2',
    roots: ['/docs'],
    base: '/docs',
    state: 'paused',
    reason: '',
    truncated: false,
    ignoredCount: 0,
    estimate: { files: 1, chunks: 1, tokens: 100, calls: 1, seconds: 10 },
    progress: { filesDone: 0, filesTotal: 1, chunksDone: 0, chunksTotal: 1 },
    totals: {
      added: 0,
      updated: 0,
      noop: 0,
      dropped: 0,
      unresolved: 0,
      failedFiles: 0,
      skippedFiles: 0,
    },
    channel: 'ui',
    origin: 'external',
    sourceTag: 'doc',
    scope: 'personal',
    project: null,
    sessionId: null,
    groups: [],
    autoStart: false,
    writeDoc: false,
    calls: 0,
    costUsd: 0,
    createdAt: '2026-03-01T10:00:00.000Z',
    startedAt: null,
    finishedAt: null,
  };
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.memoryImportJobs, response: [job] },
      { channel: IPC.memoryImportJob, args: { id: 'J2' }, response: { job, files: [] } },
      { channel: IPC.memoryImportDiscard },
    ],
  });
  await page.locator('[data-testid="mode-tab"][data-mode="memory"]').click();
  await page.locator(t('memory-import-open')).click();
  await page.locator(t('import-discard')).click();
  const dialog = page.locator(t('confirm-dialog'));
  await expect(dialog).toContainText('Discard import?');
  await expect(dialog).toContainText('Memories already stored stay.');
  await expect(page.locator(t('confirm-dialog-confirm'))).toHaveText('Discard');
  await expect(page.locator(t('confirm-dialog-cancel'))).toHaveText('Cancel');
});

test('a settings field resets with Reset to default', async ({ relaunch }) => {
  const { window: page } = await relaunch();
  await emitWailsEvent(page, IPC.openSettings, undefined);
  await page.locator(t('settings-section-Agent')).click();
  await expect(page.locator(t('settings-reset-agent-notifyIncludeMessage'))).toHaveAccessibleName(
    'Reset to default',
  );
});

studioTest.describe('databases module', () => {
  // P265: pins the final wording of the dialogs and menus whose text changed most. Each assertion
  // fails when a rewrite drifts from the voice the catalog fixed.

  const BRIDGE_PKG = 'github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge';
  const confirm = '[data-testid="confirm-dialog"]';

  studioTest(
    'delete connection confirm: title, quoted name, Cancel focused',
    async ({ relaunch }) => {
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
    },
  );

  studioTest(
    'clear HTTP response history confirm says it cannot be undone; button is Clear',
    async ({ relaunch }) => {
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
    },
  );

  studioTest('SQS poll confirm button reads Poll', async ({ relaunch }) => {
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

  studioTest('table row menu keeps its final label order', async ({ relaunch }) => {
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

  studioTest('Copy name announces Copied name', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: PG_CONTROL, stream: PG_PORT });
    await installClipboardSpy(page);
    await connectPostgres(page);
    await openRowMenu(page, COMPOSITE_PK_PATH);
    await page.click('[data-testid="menu-item-copy-name"]');

    await expect(page.locator('[data-testid="app-live-region"]')).toHaveText('Copied name');
  });

  studioTest('grid delete label follows the selection size', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: PG_CONTROL, stream: PG_PORT });
    await openCompositeTable(page);

    await openContextMenu(gutterCell(page, 0));
    await expect(page.locator('[data-testid="menu-item-delete-row"]')).toContainText(
      'Delete 1 row',
    );
    await page.keyboard.press('Escape');

    await gutterCell(page, 0).click();
    await gutterCell(page, 1).click({ modifiers: ['Shift'] });
    await openContextMenu(gutterCell(page, 1));
    await expect(page.locator('[data-testid="menu-item-delete-row"]')).toContainText(
      'Delete 2 rows',
    );
  });

  studioTest('collections empty state title and hint', async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: [{ channel: IPC.collectionsList, response: { collections: [], items: [] } }],
    });
    await modeTab(page, 'api').click();

    const empty = page.locator('[data-testid="collections-empty"]');
    await expect(empty).toContainText('No collections yet');
    await expect(empty).toContainText('Create one from the');
    await expect(empty).toContainText('above, or import a Postman collection.');
  });

  studioTest('QueryState failure shows Couldn’t load and Retry', async ({ relaunch }) => {
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

  studioTest('command palette placeholder and title', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: [] });
    await emitWailsEvent(page, IPC.commandPalette, null);

    await expect(page.locator('[data-testid="command-palette-input"]')).toHaveAttribute(
      'placeholder',
      'Type a command',
    );
    await expect(page.locator('[data-slot="dialog-title"]').first()).toHaveText('Command palette');
  });

  studioTest(
    'Kubernetes unavailable states say what failed and offer Retry',
    async ({ relaunch }) => {
      const { page } = await setupKubernetes(relaunch);
      await openKubernetes(page);
      await page.locator('[data-testid="kube-cluster-row"][data-context="bad-token"]').click();
      const view = loc(page, 'kube-unavailable');
      await expect(view).toContainText('Sign-in rejected');
      await expect(view).toContainText('The cluster refused the credentials');
      await expect(loc(page, 'kube-retry')).toHaveText('Retry');

      await page.locator('[data-testid="kube-cluster-row"][data-context="plugin-ctx"]').click();
      await expect(view).toContainText('Credential plugin failed');
    },
  );
});
