import { expect, filesTest, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// Same scenario as flows/windowflow TestOpenedFilesWindow: the files window shows only the opened
// files as tabs, a later open adds to it, and a workbench window never drains them.

type Target = { path: string; repoId: string; relPath: string; line?: number; col?: number };

const REPO = {
  id: 'repo-1',
  name: 'demo-repo',
  root: '/tmp/demo-repo',
  repoId: '/tmp/demo-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};

const withRepo = (t: Target): Target => (t.repoId ? { ...t, repoId: REPO.id } : t);
const pending = contract<Target[]>('opened-files', 'OpenedFilesService.Pending').map(withRepo);
const reuse = contract<Target[]>('opened-files', 'OpenedFilesService.Pending:reuse').map(withRepo);
const terminal = contract<Target[]>(
  'terminal-file-links',
  'OpenedFilesService.Pending:terminal',
).map(withRepo);
const isFilesWindow = contract('opened-files', 'OpenedFilesService.IsFilesWindow');
const read = contract('opened-files', 'OpenedFilesService.Read');

const repoTarget = pending.find((t) => t.repoId !== '');
const outside = pending.find((t) => t.repoId === '');
const repoText = 'package a\nfunc a() {}\nvar x = 1\n';

function boot(targets: Target[]): ControlSnapshot[] {
  if (!repoTarget || !outside) throw new Error('contract lacks a repo and an outside target');
  return [
    { channel: IPC.openedFilesIsFilesWindow, response: isFilesWindow },
    { channel: IPC.openedFilesPending, response: targets },
    { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
    { channel: IPC.openedFilesRead, args: { path: outside.path }, response: read },
    {
      channel: IPC.codeWorkspaceReadFile,
      args: { id: REPO.id, path: repoTarget.relPath },
      response: {
        kind: 'found',
        text: repoText,
        bytes: repoText.length,
        limitBytes: 8 * 1024 * 1024,
        language: 'go',
      },
    },
  ];
}

const tabs = (page: import('@playwright/test').Page) => page.locator('[data-testid="tab"]');

filesTest('contract: a files window shows only its file tabs', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: boot(pending) });
  await expect(tabs(page)).toHaveCount(2);
  await expect(page.locator('[data-testid="tab"][data-tab-kind="repo-file"]')).toContainText(
    'a.go',
  );
  const last = page.locator('[data-testid="tab"][data-tab-kind="local-file"]');
  await expect(last).toContainText('b.json');
  await expect(last).toHaveAttribute('data-active', 'true');
  await expect(page.locator('[data-testid="local-file-editor"]')).toBeVisible();
  await expect(page.locator('[data-testid="opened-files-title"]')).toHaveText('Opened files');
  for (const absent of [
    '[data-tab-kind="repo-graph"]',
    '[data-testid="mode-tab"]',
    '[data-testid="project-panel"]',
    '[data-testid="status-bar"]',
    '[data-testid="tab-strip-new"]',
  ]) {
    await expect(page.locator(absent)).toHaveCount(0);
  }
});

filesTest('contract: a later open adds a tab to the same window', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: boot(pending) });
  await expect(tabs(page)).toHaveCount(2);

  control.respond(IPC.openedFilesPending, reuse);
  await emitWailsEvent(page, IPC.openedFilesArrived, null);
  const repoTab = page.locator('[data-testid="tab"][data-tab-kind="repo-file"]');
  await expect(repoTab).toHaveAttribute('data-active', 'true');
  await expect(tabs(page)).toHaveCount(2);

  control.respond(IPC.openedFilesPending, terminal);
  await emitWailsEvent(page, IPC.openedFilesArrived, null);
  const line = terminal[0]?.line;
  await expect(repoTab).toHaveAttribute('data-active', 'true');
  await expect(tabs(page)).toHaveCount(2);
  await expect
    .poll(() =>
      control
        .log()
        .some(
          (e) =>
            e.channel === IPC.tabsSave && JSON.stringify(e.args).includes(`"revealLine":${line}`),
        ),
    )
    .toBe(true);
});

filesTest('contract: a repo file and a loose file close as one strip', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: boot(pending) });
  await expect(tabs(page)).toHaveCount(2);

  const loose = page.locator('[data-testid="tab"][data-tab-kind="local-file"]');
  await loose.click({ button: 'right' });
  await page.click('[data-testid="menu-item-close-others"]');
  await expect(tabs(page)).toHaveCount(1);
  await expect(loose).toBeVisible();

  await loose.click({ button: 'right' });
  await page.click('[data-testid="menu-item-close"]');
  await expect(page.locator('[data-testid="opened-files-empty"]')).toBeVisible();
  await expect(page.locator('[data-testid="opened-files-window"]')).toBeVisible();
});

test('contract: a workbench window never drains opened files', async ({ relaunch }) => {
  const { window: page, control } = await relaunch();
  await expect(page.locator('[data-testid="status-bar"]')).toBeVisible();
  await expect(page.locator('[data-testid="mode-tab"]').first()).toBeVisible();
  expect(control.log().filter((e) => e.channel === IPC.openedFilesPending)).toHaveLength(0);
});
