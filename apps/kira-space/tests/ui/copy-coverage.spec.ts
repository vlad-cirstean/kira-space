import { readdirSync } from 'node:fs';
import { resolve } from 'node:path';
import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import {
  expectCatalogText,
  setPseudoLocale,
} from '../../../../packages/workbench/src/testing/ui/copyCoverage';
import {
  controlSnapshots as redisControl,
  portSnapshots as redisPorts,
} from '../ipc/redis/redis.fixture';
import { controlSnapshots as sqsControl, portSnapshots as sqsPorts } from '../ipc/sqs/sqs.fixture';
import { runPreviewSnapshots } from './adePromptPreview';
import { apiTest, expect, reviewTest, studioTest, test } from './fixtures';
import { grpcTab, modeTab, openHttpModeAndNewRequest } from './studio/support/apiMode';
import {
  connectPostgres,
  ITEMS_ALLOW,
  ITEMS_CONTROL,
  ITEMS_PORT,
  ITEMS_SQL,
  openCompositeTable,
  openContextMenu,
  openItemsConsole,
  PG_ALLOW,
  PG_CONTROL,
  PG_PORT,
} from './studio/support/copyScenarios';
import { typeInto } from './studio/support/editor';
import { cellText, gridCell, gutterCell, headerCell } from './studio/support/grid';
import { openKind, openKubernetes, setupKubernetes } from './studio/support/kubernetesFixture';
import {
  DB_PATH as MONGO_DB_PATH,
  mongoConnectionSummary,
  WIDGETS_PATH,
  widgetsFixture,
} from './studio/support/mongoFixture';
import { APP_PATH, COMPOSITE_PK_PATH } from './studio/support/postgresFixture';
import { openSettings } from './studio/support/settings';
import {
  connectionRow,
  expandRow,
  findRow,
  openBrowseLeaf,
  openRowMenu,
} from './studio/support/tree';
import { adeFixture, adeV2Control, bandOf, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { withMemory } from './support/memoryNav';
import { emitWailsEvent } from './support/mockRuntime';

// Chrome text on the ADE, repos, settings, memory and title bar surfaces comes from
// the catalog. The pseudo-locale wraps catalog messages, so a literal shows as a letter outside the
// markers. Fixture data (names, titles, ids, paths, times) is allowed by exact match. The git
// surfaces are covered by copy-coverage-git.spec.ts.

const FIXTURES = resolve(__dirname, '../fixtures/ade-v2');
const t = (id: string) => `[data-testid="${id}"]`;

/** Every short string in the ADE fixtures, anchored: a whole text node equal to fixture data. */
function fixtureData(): RegExp[] {
  const strings = new Set<string>();
  const walk = (value: unknown): void => {
    if (typeof value === 'string') {
      // A prompt renders as fragments around its `{variable}` chips, so each piece is data too.
      if (value.trim() && value.length < 200) strings.add(value.trim());
      for (const piece of value.split(/\{[^}]*\}/)) if (piece.trim()) strings.add(piece.trim());
    } else if (Array.isArray(value)) value.forEach(walk);
    else if (value && typeof value === 'object') Object.values(value).forEach(walk);
  };
  for (const file of readdirSync(FIXTURES)) walk(adeFixture(file.replace(/\.json$/, '')));
  const escaped = [...strings]
    .sort((x, y) => y.length - x.length)
    .map((s) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'));
  const token = `${escaped.join('|')}|TUI|dev|stg|rel|Mon|Tue|Wed|Thu|Fri|Sat|Sun|Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec|\\d+[smhdwy]|[0-9a-f]{7,40}`;
  return [
    // A node made only of fixture strings joined by digits and punctuation, e.g. `WEB-311 · web-app`.
    new RegExp(`^(?:${token}|⟦[^⟧]*⟧|[\\s\\d.,:%·–—+\\-/→✓⚠▲›…↩≤])+$`),
    /\b\d{4}\b|\b\d+\s?(second|minute|hour|day|month|year)s?\b|\bago\b/,
    // Sent to Claude, not shown as chrome; notes toolbar glyph letters; palette ids stay raw by decision.
    /^(?:Kira Space tools: |When this step is done, call finish_step)/,
    /^(red|amber|green|cyan|blue|magenta|grey)$/,
    /^(connecting|connected|error)$/,
    /^[BIH]$/,
    /^acme$/,
    // Installed monospace families in the Appearance font picker are proper names.
    /^(Courier New|Menlo|SF Mono|Monaco|Andale Mono|PT Mono|JetBrains Mono|Fira Code|Source Code Pro|IBM Plex Mono|Cascadia Code|Hack|Inconsolata|Roboto Mono)$/,
    /^\/?tmp\/$/,
  ];
}

test('plan board, task card menu and day menu', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await setPseudoLocale(page);
  const allow = fixtureData();
  await expectCatalogText(page.locator(t('ade-plan')), { allow });
  await page.locator('[data-testid="ade-card"][data-task-id="T_alerts"]').click({
    button: 'right',
    position: { x: 60, y: 30 },
  });
  await expect(page.locator(t('context-menu'))).toBeVisible();
  await expectCatalogText(page.locator(t('context-menu')), { allow });
  await page.keyboard.press('Escape');
  await page.setViewportSize({ width: 1440, height: 2400 });
  await bandOf(page, 0).locator(t('ade-band-label')).click({ button: 'right' });
  await expect(page.locator(t('context-menu'))).toBeVisible();
  await expectCatalogText(page.locator(t('context-menu')), { allow });
});

test('later and needs pages', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await setPseudoLocale(page);
  const allow = fixtureData();
  await page.locator(t('ade-tab-later')).click();
  await expectCatalogText(page.locator(t('ade-later')), { allow });
  await page.locator(t('ade-tab-needs')).click();
  await expectCatalogText(page.locator(t('ade-needs-page')), { allow });
});

test('task panel, every tab', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await setPseudoLocale(page);
  const allow = fixtureData();
  await page
    .locator('[data-testid="ade-card"][data-task-id="T_bill"] [data-testid="ade-card-head"]')
    .click();
  const panel = page.locator(t('ade-panel'));
  await expect(panel).toBeVisible();
  for (const tab of ['main', 'details', 'sessions']) {
    await page.locator(t(`ade-panel-tab-${tab}`)).click();
    await expectCatalogText(panel, { allow });
  }
});

test('workflows page, editor and inspectors', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await setPseudoLocale(page);
  const allow = fixtureData();
  await page.locator(t('ade-tab-workflows')).click();
  await expectCatalogText(page.locator(t('ade-workflows')), { allow });
  await page.locator(t('ade-wf-row')).first().click();
  await expectCatalogText(page.locator(t('ade-workflows')), { allow });
  await page.locator(t('ade-wf-node')).first().click();
  await expectCatalogText(page.locator(t('ade-workflows')), { allow });
});

test('run dialog', async ({ relaunch }) => {
  const board = adeFixture<{
    tasks: {
      id: string;
      workflowId: string;
      entryStepId: string;
      runs: unknown[];
    }[];
  }>('board');
  const bill = board.tasks.find((x) => x.id === 'T_bill');
  const alerts = board.tasks.find((x) => x.id === 'T_alerts');
  if (!bill || !alerts) throw new Error('fixture tasks missing');
  alerts.workflowId = 'standard';
  alerts.entryStepId = 'impl';
  alerts.runs = [];
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
    ...runPreviewSnapshots(),
  ]);
  await setPseudoLocale(page);
  const allow = fixtureData();
  await page
    .locator(
      `[data-testid="ade-task"][data-task-id="T_alerts"] ${t('ade-card-head')} ${t('ade-task-action-run')}`,
    )
    .click();
  const dialog = page.locator(t('ade-run-dialog'));
  await expect(dialog).toBeVisible();
  await expectCatalogText(dialog, { allow });
});

reviewTest('review window', async ({ relaunch }) => {
  const target = adeFixture<{ gitRepoId: string }>('review-target');
  const change = (path: string) => ({ kind: 'modified', path, similarity: 0 });
  const { window: page } = await relaunch({
    control: adeV2Control([
      { channel: IPC.adeTaskReviewWindowTarget, response: target },
      { channel: IPC.adeTaskReviewAgent, response: { session: null, hostWindowKey: '' } },
    ]),
    gitStream: {
      repoId: target.gitRepoId,
      extraResults: {
        'review.resolveBase': {
          branch: 'feat/billing-dashboard',
          base: 'main',
          reason: 'override',
          range: { kind: 'ready', commitCount: 3 },
          candidates: [],
        },
        'review.files': {
          branchTip: 'a'.repeat(40),
          mergeBase: 'b'.repeat(40),
          files: [
            {
              change: change('src/pending.ts'),
              review: { kind: 'none', changedSinceReview: false },
            },
          ],
        },
        'review.session.load': null,
        'repo.open': null,
      },
    },
  });
  await page.locator(t('ade-review-window')).waitFor();
  // The file list and range announcement load async; settle them so the run is deterministic.
  await expect(page.getByText('pending.ts').first()).toBeVisible();
  await expect(page.locator('[data-testid=live-announcements]')).toContainText('3 commits');
  await setPseudoLocale(page);
  await expectCatalogText(page.locator(t('ade-review-window')), {
    allow: [
      ...fixtureData(),
      /^src\/pending\.ts$|^pending\.ts$/,
      // Folder name is data; the live-region announcement is one-shot, made before the locale switch.
      /^src$/,
      /^Comparing .+, 3 commits$/,
    ],
  });
});

test('repositories dialog', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: adeV2Control([{ channel: IPC.windowsEnsure, response: { mode: 'git' } }]),
  });
  await page.locator(t('manage-repos')).click();
  await page.locator(t('repos-dialog')).waitFor();
  await setPseudoLocale(page);
  // The settings file's path is data from the contract, not chrome.
  const allow = [...fixtureData(), /^(?:\/[\w.-]+)*\/repos\/$|^proj\.yaml$/];
  await expectCatalogText(page.locator(t('repos-dialog')), { allow });
  await page.locator(t('repos-dialog-repo')).nth(1).click();
  await expectCatalogText(page.locator(t('repos-dialog')), { allow });
  await page.locator(t('repos-dialog-tab-folders')).click();
  await expectCatalogText(page.locator(t('repos-dialog')), { allow });
});

test('settings dialog, every pane', async ({ relaunch }) => {
  const status = contract<{ command: string }>('memory-settings', 'MemoryService.McpStatus');
  const { window: page } = await relaunch({
    control: withMemory([
      { channel: IPC.memoryMcpStatus, response: status },
      {
        channel: IPC.fileHandlersStatus,
        response: contract('file-handlers', 'FileHandlersService.Status#off'),
      },
      {
        channel: IPC.memoryCheckHook,
        response: contract('memory-settings', 'MemoryService.CheckHook'),
      },
    ]),
  });
  await emitWailsEvent(page, IPC.openSettings, undefined);
  await page.locator(t('settings-dialog')).waitFor();
  await setPseudoLocale(page);
  // Provider names (the section label, model options) come from the provider, not the catalog.
  const agent = contract<{ productLabel: string; models: { label: string }[] }>(
    'agent',
    'AgentService.Info',
  );
  const allow = [
    ...fixtureData(),
    ...[agent.productLabel, ...agent.models.map((m) => m.label)].map((v) => new RegExp(`^${v}$`)),
    /^(id|email)$/,
  ];
  for (const section of [
    'Appearance',
    'Git',
    'File types',
    'Mobile access',
    'Agent',
    'Memory',
    'Advanced',
    'Experimental',
  ]) {
    await page.locator(t(`settings-section-${section}`)).click();
    await expectCatalogText(page.locator(t('settings-dialog')), { allow });
  }
});

test('memory panel, add memory dialog and import view', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: withMemory() });
  await page.locator('[data-testid="mode-tab"][data-mode="memory"]').click();
  await expect(page.locator(t('memory-panel'))).toBeVisible();
  await setPseudoLocale(page);
  await expectCatalogText(page.locator(t('memory-panel')));
  await page.locator(t('memory-add')).click();
  const dialog = page.locator(t('add-memory-dialog'));
  await expect(dialog).toBeVisible();
  await expectCatalogText(dialog);
  await page.keyboard.press('Escape');
  await page.locator(t('memory-import')).click();
  await expectCatalogText(page.locator(t('memory-panel')));
});

studioTest.describe('databases module', () => {
  // P265: with the pseudo-locale on, every catalog message shows wrapped in ⟦…⟧, so any letter
  // outside a marker is a literal that skipped t(). Fixture names go through `allow`. Surfaces reuse the fixtures of the existing UI specs.

  // Product and typeface names are proper nouns; `1k`/`10k` are page-size numerals.
  const NAMES =
    /^(PostgreSQL|MariaDB|MySQL|SQLite|ClickHouse|MongoDB|Redis|Kafka|SQS|S3|Kibana|Courier New|Menlo|SF Mono|Monaco|Andale Mono|PT Mono|JetBrains Mono|Fira Code|Source Code Pro|IBM Plex Mono|Cascadia Code|Hack|Inconsolata|Roboto Mono|\d+k)$/;
  // Column, table and key names from the fixtures: data, not chrome.
  const SCHEMA = /^(id|email|name|text|integer|order_id|product_id|quantity|orders|products)$/;
  const REDIS = /^(Test Redis \/|events|queue|session|tags|counter|leaderboard)$/;

  // Palette ids are raw by decision (Stream A); byte sizes are formatted data; the definition scope
  // note is Go-originated (backend text, P266).
  const PALETTE = /^(red|amber|green|cyan|blue|magenta|grey)$/;
  const BYTES = /^[\d.,]+ (B|KB|MB|GB)$/;
  const BACKEND_NOTE = /^Composed from catalog metadata:/;

  // The DB MCP review fixtures (dbmcp.json, dbmcp-preview.json): connection name, file, columns.
  const DBMCP_FIXTURE = /^(notes db|\/data\/mcp\.db|id|body)$/;

  const ALLOW: RegExp[] = [...PG_ALLOW, NAMES, BYTES, DBMCP_FIXTURE];
  const loc = (page: Page, testid: string) => page.locator(`[data-testid="${testid}"]`);

  studioTest('title bar, mode switcher and status bar', async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: [{ channel: IPC.connectionsList, response: [] }],
    });
    await setPseudoLocale(page);
    await expectCatalogText(page.locator('.h-titlebar'), { allow: ALLOW });
    await expectCatalogText(page.locator('[data-testid="mode-tab"]'), { allow: ALLOW });
    await expectCatalogText(loc(page, 'status-bar'), { allow: ALLOW });
    await expectCatalogText(page.locator(t('toggle-project-panel')).locator('xpath=..'));
  });

  studioTest('project panel header and empty state', async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: [{ channel: IPC.connectionsList, response: [] }],
    });
    await setPseudoLocale(page);
    await expectCatalogText(loc(page, 'project-panel'), { allow: ALLOW });
  });

  studioTest('connection row menu', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: PG_CONTROL, stream: PG_PORT });
    await setPseudoLocale(page);
    await openRowMenu(page, '');
    await expectCatalogText(loc(page, 'context-menu'), { allow: ALLOW });
  });

  studioTest('new connection dialog, both steps', async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: [{ channel: IPC.connectionsList, response: [] }],
    });
    await setPseudoLocale(page);
    await page.click('[data-testid="add-connection"]');
    const dialog = loc(page, 'new-connection-dialog');
    await expect(dialog).toBeVisible();
    await expectCatalogText(dialog, { allow: [...ALLOW, PALETTE] });
    await page.click('[data-testid="connection-kind-postgres"]');
    await expectCatalogText(dialog, { allow: [...ALLOW, PALETTE] });
  });

  studioTest('settings dialog, every pane', async ({ relaunch }) => {
    const agent = contract<{ productLabel: string; models: { label: string }[] }>(
      'agent',
      'AgentService.Info',
    );
    const providerNames = [agent.productLabel, ...agent.models.map((m) => m.label)].map(
      (v) => new RegExp(`^${v}$`),
    );
    const { window: page } = await relaunch({
      control: [
        {
          channel: IPC.memoryMcpStatus,
          response: contract('memory-settings', 'MemoryService.McpStatus'),
        },
      ],
    });
    await setPseudoLocale(page);
    await openSettings(page);
    for (const section of ['Appearance', 'Data', 'Cache', 'Api', 'Database MCP', 'Advanced']) {
      await loc(page, `settings-section-${section}`).click();
      await expectCatalogText(loc(page, 'settings-dialog'), {
        allow: [...ALLOW, SCHEMA, ...providerNames],
      });
    }
  });

  studioTest(
    'data view toolbar, column menu, cell menu and cell editor dock',
    async ({ relaunch }) => {
      const { window: page } = await relaunch({ control: PG_CONTROL, stream: PG_PORT });
      await setPseudoLocale(page);
      await openCompositeTable(page);
      await expectCatalogText(loc(page, 'data-toolbar'), { allow: ALLOW });

      await openContextMenu(headerCell(page, 'name'));
      await expectCatalogText(loc(page, 'context-menu'), { allow: ALLOW });
      await page.keyboard.press('Escape');

      await openContextMenu(gridCell(page, 0, 'name'));
      await expectCatalogText(loc(page, 'context-menu'), { allow: ALLOW });
      await page.keyboard.press('Escape');

      await openContextMenu(gutterCell(page, 0));
      await expectCatalogText(loc(page, 'context-menu'), { allow: ALLOW });
      await page.keyboard.press('Escape');

      await gridCell(page, 0, 'name').click();
      expect(await cellText(page, 0, 'name')).not.toBe('');
      await expectCatalogText(loc(page, 'cell-editor'), { allow: ALLOW });
    },
  );

  studioTest('generate data dialog', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: PG_CONTROL, stream: PG_PORT });
    await setPseudoLocale(page);
    await openCompositeTable(page);
    await page.click('[data-testid="toolbar-generate-data"]');
    const dialog = loc(page, 'generate-data-dialog');
    await expect(dialog).toBeVisible();
    await expectCatalogText(dialog, { allow: [...ALLOW, SCHEMA] });
  });

  studioTest('console toolbar', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: PG_CONTROL, stream: PG_PORT });
    await setPseudoLocale(page);
    await connectPostgres(page);
    await openRowMenu(page, COMPOSITE_PK_PATH);
    await page.click('[data-testid="menu-item-open-console"]');
    await expect(loc(page, 'console-view')).toBeVisible();
    await expectCatalogText(loc(page, 'console-view'), { allow: ALLOW });
  });

  studioTest('command palette', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: [] });
    await setPseudoLocale(page);
    await emitWailsEvent(page, IPC.commandPalette, null);
    await expect(loc(page, 'command-palette-input')).toBeVisible();
    await expectCatalogText(page.locator('[data-slot="dialog-content"]'), { allow: ALLOW });
  });

  studioTest('operations panel', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: [] });
    await setPseudoLocale(page);
    await page.click('[data-testid="toggle-operations-panel"]');
    await expect(loc(page, 'operations-panel')).toBeVisible();
    await expectCatalogText(loc(page, 'operations-panel'), { allow: ALLOW });
  });

  studioTest('DB MCP approval dialog', async ({ relaunch }) => {
    const [item] = contract<ApprovalItem[]>('dbmcp', 'ApprovalsService.List');
    if (!item) throw new Error('contract has no item');
    const { window: page } = await relaunch({ control: [] });
    await setPseudoLocale(page);
    await pushApprovals(page, [liveItem(item)]);
    const dialog = loc(page, 'approvals-dialog');
    await expect(dialog).toBeVisible();
    await expectCatalogText(dialog, { allow: ALLOW });
  });

  studioTest('DB MCP response review dialog', async ({ relaunch }) => {
    const [item] = contract<ApprovalItem[]>('dbmcp-preview', 'ApprovalsService.List');
    if (!item) throw new Error('contract has no item');
    const { window: page } = await relaunch({
      control: [
        {
          channel: IPC.dbMcpPendingResponse,
          response: contract('dbmcp-preview', 'DbMcpService.PendingResponse'),
        },
      ],
    });
    await setPseudoLocale(page);
    await pushApprovals(page, [liveItem(item)]);
    const dialog = loc(page, 'approvals-dialog');
    await expect(loc(page, 'db-mcp-preview-table')).toBeVisible();
    await expectCatalogText(dialog, { allow: ALLOW });
  });

  studioTest('HTTP request view, every tab, empty response', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: [] });
    await setPseudoLocale(page);
    await openHttpModeAndNewRequest(page);
    await expectCatalogText(loc(page, 'http-request-view'), { allow: ALLOW });
    for (const pane of ['params', 'headers', 'body', 'settings', 'cookies']) {
      await loc(page, `http-request-pane-${pane}`).click();
      await expectCatalogText(loc(page, 'http-request-view'), { allow: ALLOW });
    }
  });

  studioTest('collections panel empty state', async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: [{ channel: IPC.collectionsList, response: { collections: [], items: [] } }],
    });
    await setPseudoLocale(page);
    await modeTab(page, 'api').click();
    await expect(loc(page, 'collections-empty')).toBeVisible();
    await expectCatalogText(loc(page, 'project-panel'), { allow: ALLOW });
  });

  studioTest('environments view', async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: [{ channel: IPC.collectionsList, response: { collections: [], items: [] } }],
    });
    await setPseudoLocale(page);
    await modeTab(page, 'api').click();
    await page.click('[data-testid="api-environments"]');
    await expect(loc(page, 'environments-dialog')).toBeVisible();
    await expectCatalogText(loc(page, 'environments-dialog'), { allow: ALLOW });
  });

  studioTest('saved queries menu', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: ITEMS_CONTROL, stream: ITEMS_PORT });
    await setPseudoLocale(page);
    await openItemsConsole(page);
    await page.click('[data-testid="console-saved-toggle"]');
    await expect(loc(page, 'console-saved-entry').first()).toBeVisible();
    await expectCatalogText(loc(page, 'console-saved-menu'), {
      allow: [...ITEMS_ALLOW, NAMES, SCHEMA],
    });
  });

  studioTest('explain result', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: ITEMS_CONTROL, stream: ITEMS_PORT });
    await setPseudoLocale(page);
    await openItemsConsole(page);
    const view = loc(page, 'console-view');
    await typeInto(view, page, ITEMS_SQL);
    await page.click('[data-testid="console-explain"]');
    const result = view.locator('[data-testid="explain-result-view"]');
    await expect(result).toBeVisible();
    await expectCatalogText(result, { allow: [...ITEMS_ALLOW, NAMES, SCHEMA] });
  });

  studioTest('definition view, structure and source', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: ITEMS_CONTROL, stream: ITEMS_PORT });
    await setPseudoLocale(page);
    await connectPostgres(page);
    await openRowMenu(page, `${APP_PATH}/table:order_items`);
    await page.click('[data-testid="menu-item-open-definition"]');
    const view = loc(page, 'definition-view');
    await expect(view).toHaveAttribute('data-origin', 'composed', { timeout: 15_000 });
    await expectCatalogText(view, { allow: [...ITEMS_ALLOW, NAMES, SCHEMA, BACKEND_NOTE] });
    await view.locator('[data-testid="definition-pane-source"]').click();
    await expect(view.locator('.monaco-host')).toBeVisible();
    await expectCatalogText(view, { allow: [...ITEMS_ALLOW, NAMES, SCHEMA, BACKEND_NOTE] });
  });

  studioTest('documents toolbar', async ({ relaunch }) => {
    const conn = mongoConnectionSummary('conn-copy-doc', 'Mongo DB', 'green');
    const fixture = widgetsFixture(conn.id);
    const { window: page } = await relaunch({
      control: [{ channel: IPC.connectionsList, response: [conn] }, ...fixture.control],
      stream: fixture.port,
    });
    await setPseudoLocale(page);
    await openRowMenu(page, '');
    await page.click('[data-testid="menu-item-connect"]');
    await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
      'data-status',
      'connected',
      { timeout: 10_000 },
    );
    await expandRow(page, '');
    await expandRow(page, MONGO_DB_PATH);
    await (await findRow(page, WIDGETS_PATH)).dblclick();
    await expect(loc(page, 'document-row').first()).toBeVisible({ timeout: 15_000 });
    await expectCatalogText(loc(page, 'document-view'), {
      allow: [...ALLOW, /Mongo DB|widget|kira_test|\d/i],
    });
  });

  studioTest('key-value view', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: redisControl, stream: redisPorts });
    await setPseudoLocale(page);
    await openRowMenu(page, '');
    await page.click('[data-testid="menu-item-connect"]');
    await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
      'data-status',
      'connected',
      { timeout: 10_000 },
    );
    await expandRow(page, '');
    const db0 = 'database:db0';
    const ns = `${db0}/namespace:user`;
    const ns1 = `${ns}/namespace:1`;
    const key = `${ns1}/key:user%3A1%3Abighash`;
    await (await findRow(page, db0)).dblclick();
    const browse = loc(page, 'browse-view');
    await expectCatalogText(browse, { allow: [...ALLOW, REDIS, /user|db0|key|\d/i] });
    await browse.locator(`[data-testid="browse-row"][data-path="${ns}"]`).dblclick();
    await browse.locator(`[data-testid="browse-row"][data-path="${ns1}"]`).dblclick();
    await openBrowseLeaf(page, key);
    const view = page.locator(`[data-testid="keyvalue-view"][data-path="${key}"]`);
    await expect(view.locator('[data-testid="keyvalue-row"]').first()).toBeVisible({
      timeout: 15_000,
    });
    await expectCatalogText(view, { allow: [...ALLOW, /user|bighash|field|value|\d/i] });
  });

  studioTest('stream view', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: sqsControl, stream: sqsPorts });
    await setPseudoLocale(page);
    await openRowMenu(page, '');
    await page.click('[data-testid="menu-item-connect"]');
    await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
      'data-status',
      'connected',
      { timeout: 10_000 },
    );
    await expandRow(page, '');
    await (await findRow(page, 'queue:orders-queue')).dblclick();
    const view = page.locator('[data-testid="stream-view"][data-path="queue:orders-queue"]');
    await view.locator('[data-testid="stream-poll"]').click();
    await expect(view.locator('[data-testid="stream-row"]').first()).toBeVisible({
      timeout: 15_000,
    });
    await expectCatalogText(view, { allow: [...ALLOW, /orders|queue|sqs|message|\d/i] });
  });

  apiTest('gRPC request view, every tab', async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: [
        { channel: IPC.tabsList, response: [grpcTab({})] },
        {
          channel: IPC.grpcHistoryList,
          args: { itemId: '', tabId: 'tab-grpc-1' },
          response: [],
        },
      ],
    });
    await setPseudoLocale(page);
    const view = loc(page, 'grpc-request-view');
    await expect(view).toBeVisible();
    await expectCatalogText(view, { allow: ALLOW });
    for (const pane of ['message', 'metadata', 'schema']) {
      await loc(page, `grpc-request-pane-${pane}`).click();
      await expectCatalogText(view, { allow: ALLOW });
    }
    for (const pane of ['messages', 'metadata', 'history']) {
      await loc(page, `grpc-response-pane-${pane}`).click();
      await expectCatalogText(view, { allow: ALLOW });
    }
  });

  studioTest('collections panel menu', async ({ relaunch }) => {
    const now = '2026-01-01T00:00:00.000Z';
    const { window: page } = await relaunch({
      control: [
        {
          channel: IPC.collectionsList,
          response: {
            collections: [
              { id: 'col-1', name: 'Orders API', sortOrder: 0, createdAt: now, updatedAt: now },
            ],
            items: [
              {
                id: 'item-health',
                collectionId: 'col-1',
                parentId: null,
                kind: 'request',
                name: 'Health check',
                sortOrder: 0,
                method: 'GET',
                url: 'https://api.example.com/healthz',
                createdAt: now,
                updatedAt: now,
              },
            ],
          },
        },
      ],
    });
    await setPseudoLocale(page);
    await modeTab(page, 'api').click();
    const fixtureNames = [...ALLOW, /Orders API|Health check|api\.example\.com|GET/];
    await expectCatalogText(loc(page, 'project-panel'), { allow: fixtureNames });

    await page
      .locator('[data-testid="collection-row"][data-id="col-1"]')
      .click({ button: 'right' });
    await expect(loc(page, 'context-menu')).toBeVisible();
    await expectCatalogText(loc(page, 'context-menu'), { allow: fixtureNames });
    await page.keyboard.press('Escape');

    await page.locator('[data-testid="collection-row"]').first().focus();
    await page.keyboard.press('ArrowRight');
    await page
      .locator('[data-testid="collection-row"][data-id="item-health"]')
      .click({ button: 'right' });
    await expect(loc(page, 'context-menu')).toBeVisible();
    await expectCatalogText(loc(page, 'context-menu'), { allow: fixtureNames });
  });

  // Cluster, kind and pod names, versions and table cells are data, not chrome.
  const KUBE_DATA = [
    /kind-dev|bad-token|plugin-ctx|down|dev-user|web-\d|db-\d|db-creds|default|kube-system|apps|ReplicaSet|kubelet|v1\.\d+/,
    /^(Deployment|Pod|Service|Secret|Event|Namespace|Node|Widget|v1|apps\/v1|example\.com\/v1|Ready|Status|Restarts|Age|Name|Namespace|Running|Opaque|True|False|Normal|PodScheduled|Scheduled|Started|Pulled|app|web)$/,
    /^\s*[\d/.,]+\s*(m|s|h|d)?\s*$/,
    /container started|image pulled|assigned to node|\bUTC\b|\d{4}-\d{2}-\d{2}|apiVersion|metadata|creationTimestamp|uid/,
  ];

  studioTest('Kubernetes panel, list, describe and logs use catalog text', async ({ relaunch }) => {
    const { page } = await setupKubernetes(relaunch);
    await setPseudoLocale(page);
    await openKubernetes(page);
    await expectCatalogText(loc(page, 'kubernetes-panel'), { allow: KUBE_DATA });

    await openKind(page, 'kind-dev', 'v1/pods');
    const view = loc(page, 'kubernetes-view');
    await expect(loc(page, 'kube-row').first()).toBeVisible();
    await expectCatalogText(view, { allow: KUBE_DATA });

    await loc(page, 'kube-row').first().click();
    await expect(loc(page, 'kube-describe-section-events')).toBeVisible();
    const panel = page.locator('[data-testid="kube-detail"][data-active="true"]');
    await expectCatalogText(panel, { allow: KUBE_DATA });
    await expectCatalogText(loc(page, 'kube-tabs'), { allow: KUBE_DATA });

    await loc(page, 'kube-kind-more').click();
    await expect(loc(page, 'kube-kind-more-content')).toBeVisible();
    await expectCatalogText(loc(page, 'kube-kind-more-content'), { allow: KUBE_DATA });
    await page.keyboard.press('Escape');

    await panel.locator('[data-testid="kube-detail-tab-logs"]').click();
    await expect(loc(page, 'kube-logs')).toBeVisible();
    await expectCatalogText(loc(page, 'kube-logs'), {
      allow: [...KUBE_DATA, /^app$|^init$|^sidecar$/],
    });

    await page.locator('[data-testid="kube-cluster-row"][data-context="bad-token"]').click();
    await expect(loc(page, 'kube-unavailable')).toBeVisible();
    await expectCatalogText(loc(page, 'kube-unavailable'), { allow: KUBE_DATA });
  });

  studioTest('Kubernetes MCP dialog and approval use catalog text', async ({ relaunch }) => {
    const { page } = await setupKubernetes(relaunch);
    await setPseudoLocale(page);
    await openKubernetes(page);
    await page
      .locator('[data-testid="kube-cluster-row"][data-context="kind-dev"]')
      .click({ button: 'right' });
    await page.locator('[data-testid="menu-item-kube-cluster-mcp"]').click();
    await expect(loc(page, 'kube-mcp-dialog')).toBeVisible();
    await expectCatalogText(loc(page, 'kube-mcp-dialog'), { allow: KUBE_DATA });
  });

  studioTest('Kubernetes MCP approval dialog', async ({ relaunch }) => {
    const [item] = contract<ApprovalItem[]>('dbmcp-kube', 'ApprovalsService.List');
    if (!item) throw new Error('contract has no item');
    const { window: page } = await relaunch({ control: [] });
    await setPseudoLocale(page);
    await pushApprovals(page, [liveItem(item)]);
    const dialog = loc(page, 'approvals-dialog');
    await expect(dialog).toBeVisible();
    await expectCatalogText(dialog, { allow: [...ALLOW, ...KUBE_DATA, /<kube>|kubectl/] });
  });
});
