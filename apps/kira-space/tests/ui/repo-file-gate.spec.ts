// P433 F6: a repo file tab remounts without re-sending content. The backend half is repoflow's
// "ifStat skips an unchanged file"; the mock stands in for it here. The files window opens the tabs.
import { expect, filesTest } from './fixtures';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { filesWindowControl, openMoreFiles } from './support/repoFiles';
import { repoTab } from './support/repoRows';
import type { ControlSnapshot } from './support/types';

const REPO = {
  id: 'repo-1',
  name: 'demo-repo',
  root: '/tmp/demo-repo',
  repoId: '/tmp/demo-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};
const LIMIT = 8 * 1024 * 1024;
const A_STAT = { size: 22, hash: 'hash-a' };
const D_STAT = { size: 20, hash: 'hash-d1' };
const D_STAT_NEW = { size: 20, hash: 'hash-d2' };

const LONG = Array.from({ length: 400 }, (_, i) => `export const row${i} = ${i};\n`).join('');

const CONTROL: ControlSnapshot[] = [
  {
    channel: IPC.codeWorkspaceReadFile,
    args: { id: REPO.id, path: 'long.ts' },
    response: {
      kind: 'found',
      text: LONG,
      bytes: LONG.length,
      limitBytes: LIMIT,
      language: 'typescript',
    },
  },
  {
    channel: IPC.codeWorkspaceReadFile,
    args: { id: REPO.id, path: 'a.ts' },
    response: {
      kind: 'found',
      text: 'export const a = 1;\n',
      bytes: 20,
      limitBytes: LIMIT,
      language: 'typescript',
      stat: A_STAT,
    },
  },
  {
    channel: IPC.codeWorkspaceReadFile,
    args: { id: REPO.id, path: 'a.ts', ifStat: A_STAT },
    response: { kind: 'unchanged', stat: A_STAT, text: '', bytes: 0, limitBytes: LIMIT },
  },
  {
    channel: IPC.codeWorkspaceReadFile,
    args: { id: REPO.id, path: 'b.ts' },
    response: {
      kind: 'found',
      text: 'export const b = 2;\n',
      bytes: 20,
      limitBytes: LIMIT,
      language: 'typescript',
    },
  },
  {
    channel: IPC.codeWorkspaceReadFile,
    args: { id: REPO.id, path: 'd.ts' },
    response: {
      kind: 'found',
      text: 'export const d = 1;\n',
      bytes: 20,
      limitBytes: LIMIT,
      language: 'typescript',
      stat: D_STAT,
    },
  },
  {
    // Same size, new content: the hash differs although a size or mtime gate would match.
    channel: IPC.codeWorkspaceReadFile,
    args: { id: REPO.id, path: 'd.ts', ifStat: D_STAT },
    response: {
      kind: 'found',
      text: 'export const d = 2;\n',
      bytes: 20,
      limitBytes: LIMIT,
      language: 'typescript',
      stat: D_STAT_NEW,
    },
  },
];

const boot = (relPath: string) => filesWindowControl(REPO, [relPath], CONTROL);
const editorLines = (page: import('@playwright/test').Page) =>
  page.locator('[data-testid="repo-file-editor"] .view-lines');

filesTest(
  'switching away from a file and back sends the previous stat and shows the same text',
  async ({ relaunch }) => {
    const { window: page, control } = await relaunch({ control: boot('a.ts') });
    const editor = editorLines(page);
    await expect(editor).toContainText('export const a = 1;');
    await openMoreFiles(page, control, REPO, ['b.ts']);
    await expect(editor).toContainText('export const b = 2;');

    const readA = () =>
      calls(control, IPC.codeWorkspaceReadFile).filter(
        (c) => (c.args as { path: string }).path === 'a.ts',
      );
    expect(readA()).toHaveLength(1);

    // Back to a.ts: the second read carries the stat the first returned, and the text is unchanged.
    await repoTab(page, 'repo-file').first().click();
    await expect(editor).toContainText('export const a = 1;');
    await expect.poll(() => readA().length).toBe(2);
    expect((readA()[1].args as { ifStat?: unknown }).ifStat).toEqual(A_STAT);
  },
);

filesTest('a file tab reopens at the scroll position it was left at', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: boot('long.ts') });
  const lines = editorLines(page);
  await expect(lines).toContainText('row0 ');
  await lines.hover();
  await page.mouse.wheel(0, 3000);
  await expect(lines).not.toContainText('row0 ');
  const visible = (await lines.locator('.view-line').first().textContent()) ?? '';

  await openMoreFiles(page, control, REPO, ['b.ts']);
  await expect(lines).toContainText('export const b = 2;');
  await repoTab(page, 'repo-file').first().click();

  await expect(lines).not.toContainText('row0 ');
  await expect(lines.locator('.view-line').first()).toHaveText(visible);
});

filesTest(
  'a same-size edit answered found replaces the text on tab return',
  async ({ relaunch }) => {
    const { window: page, control } = await relaunch({ control: boot('d.ts') });
    const editor = editorLines(page);
    await expect(editor).toContainText('export const d = 1;');
    await openMoreFiles(page, control, REPO, ['b.ts']);
    await expect(editor).toContainText('export const b = 2;');

    await repoTab(page, 'repo-file').first().click();
    await expect(editor).toContainText('export const d = 2;');
    const readD = calls(control, IPC.codeWorkspaceReadFile).filter(
      (c) => (c.args as { path: string }).path === 'd.ts',
    );
    expect(readD).toHaveLength(2);
    expect((readD[1].args as { ifStat?: unknown }).ifStat).toEqual(D_STAT);
  },
);
