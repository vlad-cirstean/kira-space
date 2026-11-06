import type { Locator, Page } from '@playwright/test';
import { defaultLayout } from '@shared/domain/layout';
import { installDockerMocks } from '../../../../packages/docker-ui/src/testing/ui/dockerMock';
import { expect, studioTest, test } from './fixtures';
import { modeTab, openHttpModeAndNewRequest } from './studio/support/apiMode';
import { PG_CONN } from './studio/support/copyScenarios';
import {
  kubeError,
  loc,
  openKind,
  openKubernetes,
  setupKubernetes,
} from './studio/support/kubernetesFixture';
import { openSettings } from './studio/support/settings';
import { connectionRow } from './studio/support/tree';
import { adeFixture, adeV2Control, emitLog, FIXED_NOW, openPlan } from './support/adeV2';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// P266 Stream B: the Space consistency pass. Backend halves: adeflow TestStaleWorkflowSaveReportsCatalogText,
// migrate_ade_panel_width_test.go, appflow layout_test.go.

interface BoardFx {
  tasks: { id: string; est: string }[];
}

const t = (id: string) => `[data-testid="${id}"]`;

const REPO = {
  id: 'repo-1',
  name: 'demo-repo',
  root: '/tmp/demo-repo',
  repoId: '/tmp/demo-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
  hidden: false,
};

const HIDDEN_REPO = { ...REPO, id: 'repo-hidden', name: 'hidden-repo', hidden: true };

const DIFF_SNAP: ControlSnapshot = {
  channel: IPC.codeWorkspaceReadDiff,
  args: { id: REPO.id, path: 'c.ts' },
  response: {
    path: 'c.ts',
    language: 'typescript',
    head: { kind: 'found', text: 'export const c = 1;\n', bytes: 20, limitBytes: 8 * 1024 * 1024 },
    worktree: {
      kind: 'found',
      text: 'export const c = 2;\n',
      bytes: 20,
      limitBytes: 8 * 1024 * 1024,
    },
  },
};

async function openDiff(page: Page): Promise<void> {
  await page.locator(`${t('repo-row')}[data-repo-id="${REPO.id}"]`).click();
  await page.locator(t('git-panel-tab-files')).click();
  await page.locator('[data-testid="repo-tree-row"][data-path="c.ts"]').click({ button: 'right' });
  await page.locator(t('menu-item-open-changes')).click();
  await expect(page.locator(t('repo-diff-editor'))).toBeVisible();
}

test('the menu command palette event opens the palette with the Space commands', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch();
  await emitWailsEvent(page, IPC.commandPalette, undefined);
  await expect(page.locator(t('command-palette-input'))).toBeVisible();
  const items = page.locator(t('command-palette-item'));
  await expect(items.filter({ hasText: 'Open settings' })).toHaveCount(1);
  await expect(items.filter({ hasText: 'Show or hide repositories' })).toHaveCount(1);
  await expect(items.filter({ hasText: 'Go to next tab' })).toHaveCount(1);
});

test('the menu find event focuses the open diff find widget', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
      {
        channel: IPC.codeWorkspaceListFiles,
        args: { id: REPO.id },
        response: { paths: ['c.ts'], status: { 'c.ts': 'M' }, truncated: false },
      },
      DIFF_SNAP,
    ],
  });
  await openDiff(page);
  await expect(page.locator('.monaco-diff-editor .monaco-editor').first()).toBeVisible();
  await expect(async () => {
    await emitWailsEvent(page, IPC.viewFind, undefined);
    await expect(page.locator('.monaco-editor .find-widget.visible').first()).toBeVisible({
      timeout: 500,
    });
  }).toPass();
});

test('Memory mode has no tab strip', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.windowsEnsure, response: { mode: 'memory' } }],
  });
  await expect(page.locator(t('memory-panel'))).toBeVisible();
  await expect(page.locator(t('tab-strip'))).toHaveCount(0);
});

test('the ADE panel width comes back from the stored layout', async ({ relaunch }) => {
  const layout = { ...defaultLayout, panel: { ...defaultLayout.panel, ade: { width: 520 } } };
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.layoutGetAll, response: layout },
  ]);
  await page.locator(`${t('ade-card')}[data-task-id="T_bill"] ${t('ade-card-head')}`).click();
  await expect(page.locator(t('ade-panel-resize-handle'))).toHaveAttribute('aria-valuenow', '520');
});

test('Appearance shows the font family field', async ({ relaunch }) => {
  const { window: page } = await relaunch();
  await emitWailsEvent(page, IPC.openSettings, undefined);
  await page.locator(t('settings-section-Appearance')).click();
  await expect(page.locator(t('settings-font-family'))).toBeVisible();
  await expect(page.locator(t('font-preview'))).toBeVisible();
});

test('the Git front door and the ADE no-repos view render module-start', async ({ relaunch }) => {
  const git = await relaunch({
    control: [{ channel: IPC.windowsEnsure, response: { mode: 'git' } }],
  });
  await expect(git.window.locator(`${t('git-start')}[data-slot="module-start"]`)).toBeVisible();
  const ade = await relaunch({
    control: adeV2Control([
      { channel: IPC.windowsEnsure, response: { mode: 'ade' } },
      { channel: IPC.codeWorkspaceListRepos, response: [] },
    ]),
  });
  await expect(ade.window.locator(`${t('ade-view')} [data-slot="module-start"]`)).toBeVisible();
});

test('the Git panel empty state has no button', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.codeWorkspaceListRepos, response: [HIDDEN_REPO] }],
  });
  const empty = page.locator(t('repos-all-hidden'));
  await expect(empty).toHaveAttribute('data-slot', 'empty-state');
  await expect(empty.locator('button')).toHaveCount(0);
});

test('the estimate unit is a secondary tabs control', async ({ relaunch }) => {
  const board = adeFixture<BoardFx>('board');
  const target = board.tasks.find((x) => x.id === 'T_notif');
  if (target) target.est = '';
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await page.locator(`${t('ade-card')}[data-task-id="T_notif"] ${t('ade-card-head')}`).click();
  await expect(
    page.locator(t('ade-panel')).locator('[data-slot="secondary-tabs"]', {
      has: page.locator(t('ade-estimate-unit-d')),
    }),
  ).toBeVisible();
});

test('a 5000-line run log renders a window of rows and follows the tail', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator(`${t('ade-card')}[data-task-id="T_bill"] ${t('ade-card-head')}`).click();
  const line = page
    .locator(`${t('ade-step-block')}[data-step-id="impl"]`)
    .locator(`${t('ade-run-line')}[data-branch-id="b_bill"]`);
  await line.locator(t('ade-run-log-toggle')).click();
  const log = page.locator(t('ade-run-log'));
  await expect(log).toBeVisible();
  await emitLog(
    page,
    'run',
    'r_T_bill_impl_b_bill',
    Array.from({ length: 5000 }, (_, i) => ({
      seq: 1000 + i,
      at: 1790067600000 + i,
      stream: 'event',
      text: `line ${i}`,
    })),
  );
  await expect(log).toContainText('line 4999');
  expect(await log.locator(t('ade-run-log-line')).count()).toBeLessThanOrEqual(80);
});

test('a stale workflow save shows the same text the backend answers with', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskSaveWorkflow,
      error: { code: 'E_CONFLICT', message: 'The workflow changed. Refresh and retry.' },
    },
  ]);
  await page.locator(t('ade-tab-workflows')).click();
  await page.locator(t('ade-workflows')).waitFor();
  await page.locator(t('ade-wf-form-name')).fill('Renamed');
  await page.locator(t('ade-wf-save')).click();
  await expect(page.locator(t('ade-wf-save-error'))).toContainText(
    'The workflow changed. Refresh and retry.',
  );
});

test('a repo colour swatch tooltip names the colour', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: adeV2Control([{ channel: IPC.windowsEnsure, response: { mode: 'git' } }]),
  });
  await page.locator(t('manage-repos')).click();
  await page.locator(t('repos-dialog')).waitFor();
  await page.locator(`${t('repo-color')} ${t('color-blue')}`).hover({ force: true });
  await expect(page.locator('[data-slot="tooltip-content"]')).toContainText('Blue');
});

test('day band labels use the locale weekday names', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  const tomorrow = new Date(FIXED_NOW + 24 * 60 * 60 * 1000);
  const weekday = new Intl.DateTimeFormat('en', { weekday: 'short', timeZone: 'UTC' }).format(
    tomorrow,
  );
  await expect(
    page.locator(`${t('ade-day-band')}[data-ade-day="1"] ${t('ade-band-label')}`),
  ).toContainText(`${weekday} ${tomorrow.getUTCDate()}`);
});

studioTest.describe('databases module', () => {
  // P266 Stream A: UI half of the consistency audit. The Go half is
  // internal/flows/appflow/errors_test.go; texts below are the catalog's literals.

  const BRIDGE_PKG = 'github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge';

  const DOCKER_OK = {
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
      images: 1,
    },
  };

  type Relaunch = Parameters<Parameters<typeof test>[2]>[0]['relaunch'];

  async function openDockerWith(
    relaunch: Relaunch,
    handlers: Record<string, (args: unknown) => unknown>,
    control: readonly ControlSnapshot[] = [],
  ): Promise<Page> {
    const { window: page } = await relaunch({ control });
    await installDockerMocks(page, {
      bridgePkg: BRIDGE_PKG,
      handlers: { Status: () => DOCKER_OK, ...handlers },
    });
    await modeTab(page, 'docker').click();
    await expect(page.locator('[data-testid="docker-panel"]')).toBeVisible();
    return page;
  }

  const dockerError = (code: string, message: string) => () => ({ error: { code, message } });

  studioTest('QueryState: E_NOT_FOUND offers no Retry, E_INTERNAL does', async ({ relaunch }) => {
    const notFound = await openDockerWith(relaunch, {
      Containers: dockerError('E_NOT_FOUND', 'gone'),
    });
    const panel = notFound.locator('[data-testid="docker-panel"]');
    await expect(panel.locator('[data-testid="query-error"]')).toBeVisible();
    await expect(panel.locator('[data-testid="query-retry"]')).toHaveCount(0);
  });

  studioTest('QueryState: E_INTERNAL offers Retry', async ({ relaunch }) => {
    const page = await openDockerWith(relaunch, {
      Containers: dockerError('E_INTERNAL', 'database is locked'),
    });
    const panel = page.locator('[data-testid="docker-panel"]');
    await expect(panel.locator('[data-testid="query-error"]')).toContainText('database is locked');
    await expect(panel.locator('[data-testid="query-retry"]')).toHaveText('Retry');
  });

  studioTest(
    'QueryState: E_CONTRACT renders the fixed Kira sentence around the detail',
    async ({ relaunch }) => {
      const page = await openDockerWith(relaunch, {
        Containers: dockerError('E_CONTRACT', 'id is required'),
      });
      await expect(
        page.locator('[data-testid="docker-panel"] [data-testid="query-error"]'),
      ).toContainText('Something went wrong inside Kira (id is required).');
    },
  );

  studioTest(
    'QueryState: the loading spinner stays hidden at 100 ms and shows within 1 s',
    async ({ relaunch }) => {
      const { window: page } = await relaunch({ control: [] });
      const docker = await installDockerMocks(page, {
        bridgePkg: BRIDGE_PKG,
        handlers: {
          Status: () => DOCKER_OK,
          Containers: () => new Promise<never>(() => undefined),
        },
      });
      // A paused fake clock: only `runFor` moves page time, so a loaded host cannot skew the delay.
      await page.clock.install();
      await page.clock.pauseAt(new Date(Date.now() + 1000));
      await modeTab(page, 'docker').click();

      const loading = page.locator('[data-testid="docker-panel"] [data-testid="query-loading"]');
      let elapsed = 0;
      await expect
        .poll(async () => {
          await page.clock.runFor(10);
          elapsed += 10;
          return docker.calls('Containers').length;
        })
        .toBeGreaterThan(0);
      expect(elapsed).toBeLessThan(100);
      // The placeholder mounts invisible and shows after SPINNER_DELAY (300 ms).
      await expect(loading).toBeHidden();
      await page.clock.runFor(1000);
      await expect(loading).toBeVisible();
    },
  );

  studioTest(
    'reduced motion: dialogs skip animation, the loading Spinner keeps animate-spin',
    async ({ relaunch }) => {
      const { window: page } = await relaunch({ control: [] });
      await page.emulateMedia({ reducedMotion: 'reduce' });
      await openSettings(page);

      const ms = await page.locator('[data-testid="settings-dialog"]').evaluate((el) => {
        const raw = getComputedStyle(el).animationDuration.split(',')[0].trim();
        return raw.endsWith('ms') ? Number.parseFloat(raw) : Number.parseFloat(raw) * 1000;
      });
      expect(ms).toBeLessThanOrEqual(0.01);
      await page.keyboard.press('Escape');

      await installDockerMocks(page, {
        bridgePkg: BRIDGE_PKG,
        handlers: {
          Status: () => DOCKER_OK,
          Containers: () => new Promise<never>(() => undefined),
        },
      });
      await modeTab(page, 'docker').click();
      await expect(
        page.locator(
          '[data-testid="docker-panel"] [data-testid="query-loading"] .codicon.animate-spin',
        ),
      ).toBeVisible();
    },
  );

  studioTest(
    'LinkService.OpenExternal rejection is announced in the live region',
    async ({ relaunch }) => {
      const image = {
        id: 'sha256:img1',
        tags: ['alpine:3.20'],
        size: 8_000_000,
        created: 1_700_000_000,
        containers: 0,
        dangling: false,
      };
      const page = await openDockerWith(
        relaunch,
        {
          Containers: () => [],
          Images: () => [image],
          ImageRegistry: () => ({ url: 'https://hub.docker.com/_/alpine', unconfirmed: false }),
          Volumes: () => [],
          Networks: () => [],
        },
        [{ channel: IPC.linkOpenExternal, error: { code: 'E_INTERNAL', message: 'no handler' } }],
      );
      await page.locator('[data-testid="docker-section-images"]').click();
      const row = page.locator('[data-testid="docker-row"]', { hasText: 'alpine:3.20' });
      await row.click();
      await page.locator('[data-testid="docker-resource-registry"]').click();

      await expect(page.locator('[data-testid="app-live-region"]')).toHaveText(
        'Couldn’t open the link',
      );
    },
  );

  studioTest('first-run and api-start render the module front door', async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: [{ channel: IPC.connectionsList, response: [] }],
    });
    await expect(page.locator('[data-testid="first-run"][data-slot="module-start"]')).toBeVisible();

    await modeTab(page, 'api').click();
    await expect(page.locator('[data-testid="api-start"][data-slot="module-start"]')).toBeVisible();
  });

  studioTest('side-panel empties are EmptyState with no button', async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: [{ channel: IPC.connectionsList, response: [] }],
    });
    const connections = page.locator('[data-testid="project-panel"] [data-slot="empty-state"]');
    await expect(connections).toBeVisible();
    await expect(connections.locator('button')).toHaveCount(0);

    await modeTab(page, 'api').click();
    const collections = page.locator('[data-testid="collections-empty"][data-slot="empty-state"]');
    await expect(collections).toBeVisible();
    await expect(collections.locator('button')).toHaveCount(0);
  });

  const icon = (el: Locator, name: string) => expect(el.locator(`.codicon-${name}`)).toBeVisible();

  studioTest('delete is trash in the connection menu', async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: [{ channel: IPC.connectionsList, response: [PG_CONN] }],
    });

    await (await connectionRow(page, PG_CONN.name)).click({ button: 'right' });
    await icon(page.locator('[data-testid="menu-item-delete"]'), 'trash');
    await page.keyboard.press('Escape');
  });

  studioTest('Duplicate tab shows the files icon', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: [] });
    await openHttpModeAndNewRequest(page);
    await page.locator('[data-testid="tab"]').first().click({ button: 'right' });
    await icon(page.locator('[data-testid="menu-item-duplicate-tab"]'), 'files');
  });

  studioTest(
    'a refused SQLite test shows the backend catalog text in the file dialog',
    async ({ relaunch }) => {
      const sqlite = {
        ...PG_CONN,
        id: 'conn-sqlite',
        name: 'relative',
        kind: 'sqlite' as const,
        file: 'relative.yaml',
      };
      const { window: page } = await relaunch({
        control: [
          { channel: IPC.connectionsList, response: [sqlite] },
          {
            channel: IPC.connectionConfigFile,
            response: {
              file: 'relative.yaml',
              id: sqlite.id,
              path: '/kira/connections/relative.yaml',
              text: 'name: relative\nkind: sqlite\ndatabase: data/app.db\n',
              hash: 'hash-1',
              problem: null,
              lastGood: false,
              editable: true,
              secrets: [],
            },
          },
          {
            channel: IPC.connectionsTest,
            response: { ok: false, error: 'The database file must be an absolute path.' },
          },
        ],
      });
      await (await connectionRow(page, 'relative')).click({ button: 'right' });
      await page.click('[data-testid="menu-item-edit"]');
      await expect(page.locator('[data-testid="connection-file-dialog"]')).toBeVisible();
      await page.click('[data-testid="connection-test"]');

      await expect(page.locator('[data-testid="connection-test-result"]')).toHaveText(
        'The database file must be an absolute path.',
      );
    },
  );

  studioTest('renamed menu events still open their UI', async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: [{ channel: IPC.connectionsList, response: [] }],
    });
    await emitWailsEvent(page, IPC.newConnection, null);
    await expect(page.locator('[data-testid="new-connection-dialog"]')).toBeVisible();
    await page.click('[data-testid="new-connection-cancel"]');
    await expect(page.locator('[data-testid="new-connection-dialog"]')).toHaveCount(0);

    await emitWailsEvent(page, IPC.openSettings, null);
    await expect(page.locator('[data-testid="settings-dialog"]')).toBeVisible();
  });

  studioTest(
    'Kubernetes: a list failure offers Retry; an empty kubeconfig is an EmptyState without a button',
    async ({ relaunch }) => {
      const failing = await setupKubernetes(relaunch, {
        List: () => kubeError('E_INTERNAL', 'boom'),
      });
      await openKubernetes(failing.page);
      await openKind(failing.page, 'kind-dev', 'v1/pods');
      await expect(loc(failing.page, 'query-error')).toBeVisible();
      await expect(loc(failing.page, 'query-retry')).toBeVisible();

      const empty = await setupKubernetes(relaunch, {
        Clusters: () => ({ config: { state: 'ok', paths: [], message: '' }, clusters: [] }),
      });
      await openKubernetes(empty.page);
      const none = loc(empty.page, 'kube-clusters-empty');
      await expect(none).toBeVisible();
      await expect(none.locator('button')).toHaveCount(0);
    },
  );
});
