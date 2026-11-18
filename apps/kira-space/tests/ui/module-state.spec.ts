import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { withMemory } from './support/memoryNav';
import { emitWailsEvent } from './support/mockRuntime';
import { roundTrip, switchModule } from './support/moduleRoundTrip';
import { scrollTo, scrollTop } from './support/scroll';
import type { ControlSnapshot } from './support/types';

// P660: view state of module panels and the Agents full view survives leaving the module and
// coming back. One round trip per module: set state, switch away, switch back, assert.

const panelTab = (page: Page, name: 'repos' | 'files' | 'review') =>
  page.locator(`[data-testid="git-panel-tab-${name}"]`);
const REPO = {
  id: 'repo-state-1',
  name: 'state-repo',
  root: '/tmp/state-repo',
  repoId: '/tmp/state-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};

const GIT_CONTROL: ControlSnapshot[] = [
  { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
  {
    channel: IPC.codeWorkspaceListFiles,
    args: { id: REPO.id },
    response: {
      paths: Array.from({ length: 80 }, (_, i) => `file-${String(i).padStart(2, '0')}.ts`),
      status: {},
      truncated: false,
    },
  },
];

test('Git: repo and file search, tree scroll survive a module round trip', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: GIT_CONTROL });
  await page.setViewportSize({ width: 1200, height: 520 });
  const search = page.locator('[data-testid="tree-search"]');

  // Repos tab: the bar and its text.
  await page.locator('[data-testid="toggle-search"]').click();
  await search.fill('state');
  await page.locator(`[data-testid="repo-row"][data-repo-id="${REPO.id}"]`).click();

  // Files tab: its own text, and the tree scroll.
  await panelTab(page, 'files').click();
  await search.fill('file');
  const tree = page.locator('[data-testid="virtual-list"]');
  await expect(tree).toBeVisible();
  await scrollTo(tree, 400);
  const scrolled = await scrollTop(tree);

  await roundTrip(page, 'automations', 'git');

  await expect(search).toHaveValue('file');
  await expect(panelTab(page, 'files')).toHaveAttribute('aria-pressed', 'true');
  await expect.poll(() => scrollTop(page.locator('[data-testid="virtual-list"]'))).toBe(scrolled);
  await panelTab(page, 'repos').click();
  await expect(search).toHaveValue('state');
});

const t = (id: string) => `[data-testid="${id}"]`;

function script(i: number) {
  const id = `script-${i}`;
  return {
    id,
    kind: 'script',
    params: [],
    smart: null,
    name: `Script ${String(i).padStart(2, '0')}`,
    command: 'true',
    workingDir: '/tmp/demo',
    dirMode: 'fixed',
    useAdeDir: false,
    color: 'green',
    collection: '',
    schedule: null,
    savedSecrets: [],
    sortOrder: i,
    createdAt: '2026-01-01T00:00:00.000Z',
    updatedAt: '2026-01-01T00:00:00.000Z',
  };
}

test('Automations: search bar, text and list scroll survive a module round trip', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [
      {
        channel: IPC.customScriptsList,
        response: { collections: [], scripts: Array.from({ length: 40 }, (_, i) => script(i)) },
      },
    ],
  });
  await page.setViewportSize({ width: 1200, height: 520 });
  await switchModule(page, 'automations');
  await page.locator(t('toggle-search')).click();
  await page.locator(t('tree-search')).fill('Script');
  const list = page.locator(`${t('automations-body')} > div`);
  await expect(page.locator(t('script-script-39'))).toHaveCount(1);
  await scrollTo(list, 300);
  const scrolled = await scrollTop(list);

  await roundTrip(page, 'git', 'automations');

  await expect(page.locator(t('tree-search'))).toHaveValue('Script');
  await expect.poll(() => scrollTop(page.locator(`${t('automations-body')} > div`))).toBe(scrolled);
});

function memoryRow(i: number, over: Record<string, unknown> = {}) {
  return {
    id: `m${i}`,
    lineageId: `m${i}`,
    version: 1,
    fact: `Fact number ${i}.`,
    kind: 'fact',
    reason: 'Added for the state spec.',
    keywords: [],
    origin: 'user',
    sourceTag: null,
    scope: 'personal',
    project: null,
    sessionId: null,
    status: 'current',
    historical: false,
    supersedesId: null,
    supersededById: null,
    createdAt: '2026-03-01T10:00:00.000Z',
    supersededAt: null,
    versions: 1,
    ...over,
  };
}

test('Memory: results scroll and detail scroll survive a module round trip', async ({
  relaunch,
}) => {
  const rows = Array.from({ length: 40 }, (_, i) => memoryRow(i));
  const long = memoryRow(0, {
    reason: Array.from({ length: 120 }, (_, i) => `Line ${i}`).join('\n\n'),
  });
  const { window: page } = await relaunch({
    control: withMemory([
      { channel: IPC.memoryRecent, response: [long, ...rows.slice(1)] },
      { channel: IPC.memoryHistory, response: { memories: [long], events: [] } },
      { channel: IPC.memoryContext, response: { memory: long, groups: [] } },
    ]),
  });
  await page.setViewportSize({ width: 1200, height: 520 });
  await switchModule(page, 'memory');
  await page.locator(t('memory-row-m0')).click();
  const detail = page.locator(t('memory-start'));
  await expect(page.locator(t('memory-detail'))).toBeVisible();
  await scrollTo(detail, 200);
  const detailTop = await scrollTop(detail);
  const results = page.locator(t('memory-results'));
  await scrollTo(results, 300);
  const resultsTop = await scrollTop(results);

  await roundTrip(page, 'git', 'memory');

  await expect.poll(() => scrollTop(page.locator(t('memory-start')))).toBe(detailTop);
  await expect.poll(() => scrollTop(results)).toBe(resultsTop);
});

test('Agents: plan scroll, All sessions filter and picked file survive page and module switches', async ({
  relaunch,
}) => {
  const dup = contract<{ file: string; path: string; problem: unknown; repo: string }>(
    'repo-config-file',
    'RepoConfigService.List#duplicate',
  );
  const second = { ...dup, file: 'second.yaml', path: dup.path.replace(dup.file, 'second.yaml') };
  const { window: page } = await openPlan(relaunch, [
    {
      channel: IPC.repoConfigList,
      response: { dir: '~/.kira-space/config/repos', files: [dup, second] },
    },
  ]);
  await page.setViewportSize({ width: 1200, height: 420 });
  const plan = page.locator(t('ade-plan'));
  await scrollTo(plan, 200);
  const scrolled = await scrollTop(plan);

  // Page switch inside Agents: Needs -> All sessions stopped, then back to Plan.
  await page.locator(t('ade-tab-needs')).click();
  await page.locator(t('ade-needs-all')).click();
  await page.locator(t('ade-all-stopped')).click();
  await expect(page.locator(t('ade-all-session'))).toHaveCount(7);
  await page.locator(t('ade-tab-plan')).click();
  await expect.poll(() => scrollTop(page.locator(t('ade-plan')))).toBe(scrolled);

  // Repositories: pick the second unclaimed file.
  await page.locator(t('ade-tab-repos')).click();
  await page.locator(t('ade-repos-tab-files')).click();
  await page.locator(`${t('ade-repos-file')}[data-file="second.yaml"]`).click();
  await expect(page.locator(`${t('ade-repos-file')}[data-file="second.yaml"]`)).toHaveAttribute(
    'aria-pressed',
    'true',
  );

  await roundTrip(page, 'git', 'ade');

  await expect(page.locator(`${t('ade-repos-file')}[data-file="second.yaml"]`)).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await page.locator(t('ade-tab-needs')).click();
  await expect(page.locator(t('ade-all-session'))).toHaveCount(7);
  await page.locator(t('ade-tab-plan')).click();
  await expect.poll(() => scrollTop(page.locator(t('ade-plan')))).toBe(scrolled);
});

test('Terminal tab: output scrolled up survives a module round trip', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [...GIT_CONTROL, { channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } }],
  });
  await page.locator(`[data-testid="repo-row"][data-repo-id="${REPO.id}"]`).click();
  await page.locator(t('tab-strip-new')).click();
  await expect(page.locator('.xterm-rows')).toBeVisible();
  const terminalId = await page
    .locator('[data-testid="tab-strip-wrapper"] [data-testid="tab"][data-tab-kind="terminal"]')
    .getAttribute('data-tab-id');
  const lines = Array.from({ length: 200 }, (_, i) => `output line ${i}\r\n`).join('');
  await emitWailsEvent(page, IPC.terminal, {
    terminalId,
    data: Buffer.from(lines).toString('base64'),
    exited: false,
  });
  // xterm 6 scrolls its own scrollable element, driven by the wheel, not a DOM scrollTop.
  const topLine = () => page.locator('.xterm-rows > div').first().innerText();
  await expect.poll(topLine).toContain('output line');
  const atBottom = await topLine();
  await page.locator('.xterm-screen').hover();
  await page.mouse.wheel(0, -1500);
  await expect.poll(topLine).not.toBe(atBottom);
  const scrolled = await topLine();

  await roundTrip(page, 'automations', 'git');

  await expect(page.locator('.xterm-rows')).toBeVisible();
  await expect.poll(topLine).toBe(scrolled);
});
