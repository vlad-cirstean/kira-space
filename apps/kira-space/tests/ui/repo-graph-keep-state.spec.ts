import type { Page } from '@playwright/test';
import { defaultSettings } from '../../frontend/src/state/settingsDomain';
import { expect, test } from './fixtures';
import { freezeClock, runCoalesceWindow } from './support/clock';
import { gitStreamEmit, gitStreamRequests } from './support/gitStreamMock';
import {
  manyRowShas,
  manyRows,
  openPortGraphWithFile,
  PORT_REPO,
  type Relaunch,
  refsAtTip,
  singleRowChunks,
} from './support/gitUiPortFixtures';
import { IPC } from './support/ipcChannels';
import { trackErrors } from './support/pageErrors';
import { repoTab } from './support/repoRows';
import type { ControlSnapshot } from './support/types';

// The pinned graph tab stays mounted (hidden) while another tab is active: opening a file and
// returning must not re-walk the graph or lose scroll/selection.

const grid = '[data-testid="commit-grid"]';
const row = (p: Page, n: number) => p.locator(`${grid} .slick-row[data-row="${n}"]`);
const viewport = (p: Page) => p.locator(`${grid} .slick-viewport-top.slick-viewport-left`);
const count = async (p: Page, method: string) =>
  (await gitStreamRequests(p)).filter((m) => m === method).length;

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.codeWorkspaceListRepos, response: [PORT_REPO] },
  {
    channel: IPC.codeWorkspaceReadFile,
    args: { id: PORT_REPO.id, path: 'a.ts' },
    response: {
      kind: 'found',
      text: 'export const a = 1;\nexport const b = 2;\n',
      bytes: 40,
      limitBytes: 8 * 1024 * 1024,
      language: 'typescript',
    },
  },
];

async function boot(
  relaunch: Relaunch,
  extra: Record<string, unknown> = {},
  control: ControlSnapshot[] = CONTROL,
): Promise<Page> {
  const page = await openPortGraphWithFile(relaunch, {
    chunks: singleRowChunks(manyRows(300)),
    results: { 'refs.list': refsAtTip(manyRowShas(0)), ...extra },
    control,
    file: 'a.ts',
    fileActive: false,
  });
  await expect(row(page, 0)).toBeVisible({ timeout: 15_000 });
  await expect
    .poll(() => viewport(page).evaluate((el) => el.scrollHeight > el.clientHeight * 4), {
      timeout: 15_000,
    })
    .toBe(true);
  return page;
}

async function openFileTab(page: Page): Promise<void> {
  await repoTab(page, 'repo-file').click();
  await expect(page.locator('[data-testid="repo-file-editor"]')).toBeVisible();
  await expect(page.locator('[data-testid="repo-graph-host"]')).toBeHidden();
}

const tag = (p: Page) =>
  p.locator(grid).evaluate((el) => {
    (el as HTMLElement).dataset.p342 = 'kept';
  });

test('opening a file and returning keeps the graph mounted with scroll and selection', async ({
  relaunch,
}) => {
  const page = await boot(relaunch);
  const errors = trackErrors(page);
  await viewport(page).evaluate((el) => {
    el.scrollTop = 2000;
  });
  await expect.poll(() => viewport(page).evaluate((el) => el.scrollTop)).toBeGreaterThan(1500);
  const saved = await viewport(page).evaluate((el) => el.scrollTop);
  const selected = page.locator(`${grid} .slick-row[aria-selected="true"]`);
  const visibleRow = page.locator(`${grid} .slick-row`).nth(3);
  await visibleRow.click();
  await expect(selected.first()).toBeVisible();
  await tag(page);
  const before = {
    init: await count(page, 'app.init'),
    stream: await count(page, 'graph.stream'),
    refresh: await count(page, 'graph.refresh'),
  };

  await openFileTab(page);
  await repoTab(page, 'repo-graph').click();

  await expect(page.locator(grid)).toHaveAttribute('data-p342', 'kept');
  expect(await count(page, 'app.init')).toBe(before.init);
  expect(await count(page, 'graph.stream')).toBe(before.stream);
  expect(await count(page, 'graph.refresh')).toBe(before.refresh);
  await expect
    .poll(async () => Math.abs((await viewport(page).evaluate((el) => el.scrollTop)) - saved), {
      timeout: 5000,
    })
    .toBeLessThanOrEqual(28);
  await expect(selected.first()).toBeVisible();
  await expect(page.locator('[data-testid="boot-error"]')).toHaveCount(0);
  expect(errors).toEqual([]);
});

test('"Open in graph" from a file scrolls the hidden graph to the commit once shown', async ({
  relaunch,
}) => {
  const page = await boot(relaunch, {
    'blame.line': {
      sha: manyRowShas(250),
      author: 'Fake Author',
      summary: 'Row 250',
      authorTimeSeconds: 1_700_000_000,
    },
  });
  await repoTab(page, 'repo-file').click();
  const editor = page.locator('[data-testid="repo-file-editor"]');
  await expect(editor).toBeVisible();
  await editor.locator('.view-lines').click();
  await page.keyboard.press('ArrowDown');
  await page.locator('[data-testid="blame-status"]').click();

  await expect(page.locator('[data-testid="repo-graph-host"]')).toBeVisible();
  await expect(page.locator(`${grid} .slick-row[aria-selected="true"]`).first()).toBeInViewport();
  await expect.poll(() => viewport(page).evaluate((el) => el.scrollTop)).toBeGreaterThan(5000);
});

test('switching to the ADE module and back keeps the graph mounted', async ({ relaunch }) => {
  const page = await boot(relaunch);
  await tag(page);
  const before = { init: await count(page, 'app.init'), stream: await count(page, 'graph.stream') };

  await page.locator('[data-testid="mode-tab"][data-mode="ade"]').click();
  await expect(page.locator('[data-testid="repo-graph-host"]')).toBeHidden();
  await page.locator('[data-testid="mode-tab"][data-mode="git"]').click();

  await expect(row(page, 0)).toBeVisible();
  await expect(page.locator(grid)).toHaveAttribute('data-p342', 'kept');
  expect(await count(page, 'app.init')).toBe(before.init);
  expect(await count(page, 'graph.stream')).toBe(before.stream);
});

test('a date-format change applies to the already-mounted graph', async ({ relaunch }) => {
  const page = await boot(relaunch, {}, [
    ...CONTROL,
    {
      channel: IPC.settingsSet,
      response: {
        ...defaultSettings,
        appearance: { ...defaultSettings.appearance, dateFormat: 'absolute' },
      },
    },
  ]);
  await tag(page);
  const detail = page.locator('[data-testid="detail-region"]');
  await expect(async () => {
    await page.keyboard.press('Escape');
    await expect(detail).toHaveCount(0, { timeout: 1000 });
  }).toPass();
  const date = row(page, 0).locator('.kira-cell-date').first();
  const relative = await date.innerText();

  await page.locator('[data-testid="open-settings"]').click();
  await page.locator('[data-testid="settings-date-format-absolute"]').click();
  await page.locator('[data-testid="settings-save"]').click();

  await expect(date).not.toHaveText(relative);
  await expect(page.locator(grid)).toHaveAttribute('data-p342', 'kept');
});

test('a refresh while the graph is hidden defers to one in-place refresh on show', async ({
  relaunch,
}) => {
  const page = await boot(relaunch);
  await tag(page);
  await openFileTab(page);
  const init = await count(page, 'app.init');
  const stream = await count(page, 'graph.stream');
  const refresh = await count(page, 'graph.refresh');

  await freezeClock(page);
  await gitStreamEmit(page, 'repo.changed', { repoId: PORT_REPO.repoId, kind: 'refsChanged' });
  await gitStreamEmit(page, 'repo.changed', { repoId: PORT_REPO.repoId, kind: 'refsChanged' });
  await runCoalesceWindow(page);
  expect(await count(page, 'graph.refresh')).toBe(refresh);

  await repoTab(page, 'repo-graph').click();
  await runCoalesceWindow(page);
  await expect.poll(() => count(page, 'graph.refresh'), { timeout: 2000 }).toBe(refresh + 1);
  await expect(page.locator(grid)).toHaveAttribute('data-p342', 'kept');
  await expect(row(page, 0)).toBeVisible();
  expect(await count(page, 'app.init')).toBe(init);
  expect(await count(page, 'graph.stream')).toBe(stream);
});

test('closing the workspace unmounts the kept graph; reopening mounts cold', async ({
  relaunch,
}) => {
  const page = await boot(relaunch);
  const errors = trackErrors(page);
  await tag(page);
  await openFileTab(page);

  await page.locator('[data-testid="git-panel-tab-repos"]').click();
  await page
    .locator(`[data-testid="repo-row"][data-repo-id="${PORT_REPO.id}"]`)
    .click({ button: 'right' });
  await page.locator('[data-testid="menu-item-close"]').click();
  await expect(page.locator('[data-testid="git-start"]')).toBeVisible();
  await expect(page.locator('[data-testid="repo-graph-host"]')).toHaveCount(0);

  await page.locator(`[data-testid="repo-row"][data-repo-id="${PORT_REPO.id}"]`).click();
  await expect(
    page.locator('[data-testid="repo-graph-host"] [data-testid="connection-state"]'),
  ).toHaveText('connected');
  await expect(page.locator(grid)).not.toHaveAttribute('data-p342', 'kept');
  expect(errors).toEqual([]);
});

test('opening a file and returning keeps the commit detail pane open', async ({ relaunch }) => {
  const page = await boot(relaunch);
  const errors = trackErrors(page);
  const detail = page.locator('[data-testid="detail-region"]');
  await page.locator(`${grid} .slick-row`).nth(3).click();
  await expect(detail).toBeVisible();
  await detail.evaluate((el) => {
    (el as HTMLElement).dataset.p343 = 'kept';
  });

  await openFileTab(page);
  await page.locator('[data-testid="repo-file-editor"]').click();
  await repoTab(page, 'repo-graph').click();

  await expect(detail).toBeVisible();
  await expect(detail).toHaveAttribute('data-p343', 'kept');
  expect(errors).toEqual([]);
});
