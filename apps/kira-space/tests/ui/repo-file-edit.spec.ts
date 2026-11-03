import type { Page } from '@playwright/test';
import { expect, filesTest, type KiraApp, type RelaunchOptions, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { repoRowById, repoTab, treeRow } from './support/repoRows';
import type { ControlSnapshot } from './support/types';

// Same scenario as flows/repoflow TestEditFile: fixtures from contract repo-file-edit.

interface EditContent {
  kind: string;
  text: string;
  hash: string;
  language: string;
}
const base = contract<EditContent>('repo-file-edit', 'CodeWorkspaceService.ReadFileForEdit');
const saved = contract<{ hash: string }>('repo-file-edit', 'CodeWorkspaceService.WriteFile');
const conflict = contract<{ code: string; message: string }>(
  'repo-file-edit',
  'CodeWorkspaceService.WriteFile#conflict',
);

const REPO = {
  id: 'repo-1',
  name: 'demo-repo',
  root: '/tmp/demo-repo',
  repoId: '/tmp/demo-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};
const FILE = 'a.ts';
const CHANGED = { ...base, text: 'package other\n', hash: 'c'.repeat(64) };

const readSnap = (text: string): ControlSnapshot => ({
  channel: IPC.codeWorkspaceReadFile,
  args: { id: REPO.id, path: FILE },
  response: {
    kind: 'found',
    text,
    bytes: text.length,
    limitBytes: 8 * 1024 * 1024,
    language: 'go',
  },
});
const editSnap = (...responses: EditContent[]): ControlSnapshot[] =>
  responses.map((response) => ({
    channel: IPC.codeWorkspaceReadFileForEdit,
    args: { id: REPO.id, path: FILE },
    response,
  }));
const writeSnap = (text: string, baseHash: string, result: { hash: string }): ControlSnapshot => ({
  channel: IPC.codeWorkspaceWriteFile,
  args: { id: REPO.id, path: FILE, text, baseHash },
  response: result,
});

const SETUP: ControlSnapshot[] = [
  { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
  {
    channel: IPC.codeWorkspaceListFiles,
    args: { id: REPO.id },
    response: { paths: [FILE], status: {}, truncated: false },
  },
  readSnap(base.text),
];

async function openFile(
  relaunch: (o?: RelaunchOptions) => Promise<KiraApp>,
  control: ControlSnapshot[],
) {
  const app = await relaunch({ control: [...SETUP, ...control] });
  await repoRowById(app.window, REPO.id).click();
  await app.window.locator('[data-testid="git-panel-tab-files"]').click();
  await treeRow(app.window, FILE).click();
  await expect(lines(app.window)).toContainText('package main');
  return app;
}

type ControlLog = { log(): { channel: string; args: unknown }[] };
const lines = (page: Page) => page.locator('[data-testid="repo-file-editor"] .view-lines');
const writes = (control: ControlLog) =>
  control.log().filter((e) => e.channel === IPC.codeWorkspaceWriteFile);

async function startEditing(page: Page): Promise<void> {
  await page.locator('[data-testid="repo-file-edit"]').click();
  await expect(page.locator('[data-testid="repo-file-done"]')).toBeVisible();
  await page.locator('[data-testid="repo-file-editor"] .view-lines').click();
  await page.keyboard.press('ControlOrMeta+End');
}

test('contract: edit and save with the shortcut and the button', async ({ relaunch }) => {
  const { window: page, control } = await openFile(relaunch, [
    ...editSnap(base),
    writeSnap(`${base.text}x`, base.hash, saved),
    writeSnap(`${base.text}xy`, saved.hash, { hash: 'd'.repeat(64) }),
  ]);
  // Read only by default: typing changes nothing.
  await page.locator('[data-testid="repo-file-editor"] .view-lines').click();
  await page.keyboard.type('z');
  await expect(lines(page)).not.toContainText('z');

  await startEditing(page);
  await page.keyboard.type('x');
  await expect(page.locator('[data-testid="repo-file-modified"]')).toBeVisible();
  await expect(repoTab(page, 'repo-file').locator('[data-testid="tab-badge"]')).toBeVisible();

  await page.keyboard.press('Meta+s');
  await expect(page.locator('[data-testid="repo-file-modified"]')).toHaveCount(0);
  expect(writes(control)).toHaveLength(1);
  expect(writes(control)[0]?.args).toEqual({
    id: REPO.id,
    path: FILE,
    text: `${base.text}x`,
    baseHash: base.hash,
  });
  await expect(repoTab(page, 'repo-file').locator('[data-testid="tab-badge"]')).toHaveCount(0);

  await page.keyboard.type('y');
  await page.locator('[data-testid="repo-file-save"]').click();
  await expect(page.locator('[data-testid="repo-file-modified"]')).toHaveCount(0);
  expect(writes(control)).toHaveLength(2);

  await page.locator('[data-testid="repo-file-done"]').click();
  await expect(page.locator('[data-testid="repo-file-edit"]')).toBeVisible();
});

test('contract: a file changed on disk offers Refresh and Overwrite', async ({ relaunch }) => {
  const { window: page, control } = await openFile(relaunch, [
    ...editSnap(base, CHANGED),
    {
      channel: IPC.codeWorkspaceWriteFile,
      args: { id: REPO.id, path: FILE, text: `${base.text}x`, baseHash: base.hash },
      error: conflict,
    },
    writeSnap(`${base.text}x`, CHANGED.hash, saved),
  ]);
  await startEditing(page);
  await page.keyboard.type('x');
  await page.locator('[data-testid="repo-file-save"]').click();

  await expect(page.locator('[data-testid="repo-file-conflict"]')).toBeVisible();
  await page.locator('[data-testid="repo-file-overwrite"]').click();
  await expect(page.locator('[data-testid="repo-file-conflict"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="repo-file-modified"]')).toHaveCount(0);
  expect(writes(control).at(-1)?.args).toMatchObject({ baseHash: CHANGED.hash });
});

test('closing a tab with unsaved edits asks to discard', async ({ relaunch }) => {
  const { window: page } = await openFile(relaunch, editSnap(base));
  await startEditing(page);
  await page.keyboard.type('x');
  const tab = repoTab(page, 'repo-file');
  await tab.hover();
  await tab.locator('[data-testid="tab-close"]').click();
  await page.locator('[data-testid="confirm-dialog-cancel"]').click();
  await expect(tab).toHaveCount(1);

  await tab.hover();
  await tab.locator('[data-testid="tab-close"]').click();
  await page.locator('[data-testid="confirm-dialog-confirm"]').click();
  await expect(repoTab(page, 'repo-file')).toHaveCount(0);
});

test('Done with unsaved edits asks to discard and returns to the disk text', async ({
  relaunch,
}) => {
  const { window: page } = await openFile(relaunch, editSnap(base));
  await startEditing(page);
  await page.keyboard.type('x');
  await page.locator('[data-testid="repo-file-done"]').click();
  await page.locator('[data-testid="confirm-dialog-confirm"]').click();
  await expect(page.locator('[data-testid="repo-file-edit"]')).toBeVisible();
  await expect(lines(page)).not.toContainText('x');
  await expect(lines(page)).toContainText('package main');
});

test('a tab switch keeps the draft and the dirty state', async ({ relaunch }) => {
  const { window: page } = await openFile(relaunch, editSnap(base));
  await startEditing(page);
  await page.keyboard.type('x');
  await repoTab(page, 'repo-graph').click();
  await repoTab(page, 'repo-file').click();
  await expect(lines(page)).toContainText('x');
  await expect(page.locator('[data-testid="repo-file-modified"]')).toBeVisible();
  await expect(page.locator('[data-testid="repo-file-save"]')).toBeEnabled();
});

test('no suggestions appear while editing', async ({ relaunch }) => {
  const { window: page } = await openFile(relaunch, editSnap(base));
  await startEditing(page);
  await page.keyboard.type('pack');
  await page.keyboard.press('Control+Space');
  // Absence of a suggest popup: no event to await.
  await page.waitForTimeout(300);
  await expect(page.locator('.suggest-widget.visible')).toHaveCount(0);
});

test('a file that is not UTF-8 text stays read only', async ({ relaunch }) => {
  const { window: page } = await openFile(
    relaunch,
    editSnap({ kind: 'notUtf8', text: '', hash: '', language: 'plaintext' }),
  );
  await page.locator('[data-testid="repo-file-edit"]').click();
  await expect(page.locator('[data-testid="repo-file-edit-error"]')).toBeVisible();
  await expect(page.locator('[data-testid="repo-file-edit"]')).toBeVisible();
  await page.locator('[data-testid="repo-file-editor"] .view-lines').click();
  await page.keyboard.type('z');
  await expect(lines(page)).not.toContainText('z');
});

test('a revision-pinned tab has no Edit button', async ({ relaunch }) => {
  const REV = 'b'.repeat(40);
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
      {
        channel: IPC.codeWorkspaceListFiles,
        args: { id: REPO.id },
        response: { paths: [FILE], status: {}, truncated: false },
      },
      {
        channel: IPC.tabsList,
        response: [
          {
            id: 'restored-repo-graph',
            kind: 'repo-graph',
            connectionId: null,
            path: '',
            order: 0,
            active: false,
            workspaceId: REPO.id,
            state: { viewState: null, reviewSession: null },
          },
          {
            id: 'restored-repo-file-rev',
            kind: 'repo-file',
            connectionId: null,
            path: FILE,
            order: 1,
            active: true,
            workspaceId: REPO.id,
            state: { revealLine: null, markdownReading: false, rev: REV },
          },
        ],
      },
    ],
    gitStream: {
      repoId: REPO.repoId,
      extraResults: {
        'repo.open': null,
        'file.read': { kind: 'found', content: 'export const a = 1;\n' },
      },
    },
  });
  await expect(lines(page)).toContainText('export const a = 1;');
  await expect(page.locator('[data-testid="repo-file-edit"]')).toHaveCount(0);
});

filesTest('a local file has no Edit button', async ({ relaunch }) => {
  const pending = contract<Array<{ path: string; repoId: string }>>(
    'opened-files',
    'OpenedFilesService.Pending',
  );
  const outside = pending.find((t) => t.repoId === '');
  if (!outside) throw new Error('contract has no outside-repo target');
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.openedFilesIsFilesWindow, response: true },
      { channel: IPC.openedFilesPending, response: [outside] },
      {
        channel: IPC.openedFilesRead,
        args: { path: outside.path },
        response: contract('opened-files', 'OpenedFilesService.Read'),
      },
    ],
  });
  await expect(page.locator('[data-testid="local-file-editor"]')).toBeVisible();
  await expect(page.locator('[data-testid="repo-file-edit"]')).toHaveCount(0);
});
