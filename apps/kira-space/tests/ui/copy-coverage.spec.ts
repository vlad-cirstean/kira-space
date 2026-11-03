import { readdirSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  expectCatalogText,
  setPseudoLocale,
} from '../../../../packages/workbench/src/testing/ui/copyCoverage';
import { runPreviewSnapshots } from './adePromptPreview';
import { expect, reviewTest, test } from './fixtures';
import { adeFixture, adeV2Control, bandOf, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
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
  for (const tab of ['task', 'notes', 'sessions']) {
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
    control: [
      { channel: IPC.memoryMcpStatus, response: status },
      {
        channel: IPC.fileHandlersStatus,
        response: contract('file-handlers', 'FileHandlersService.Status#off'),
      },
      {
        channel: IPC.memoryCheckHook,
        response: contract('memory-settings', 'MemoryService.CheckHook'),
      },
    ],
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
  ];
  for (const section of [
    'Appearance',
    'Git',
    'File types',
    'Mobile access',
    'Agent',
    'Memory',
    'Advanced',
  ]) {
    await page.locator(t(`settings-section-${section}`)).click();
    await expectCatalogText(page.locator(t('settings-dialog')), { allow });
  }
});

test('memory panel, add memory dialog and import view', async ({ relaunch }) => {
  const { window: page } = await relaunch();
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

test('title bar', async ({ relaunch }) => {
  const { window: page } = await relaunch();
  await setPseudoLocale(page);
  await expectCatalogText(page.locator(t('toggle-project-panel')).locator('xpath=..'));
});
