import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import {
  FILES_BRANCH,
  openFilesWindow,
  openTaskFile,
  taskFilesControl,
  taskSearchEvent,
} from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// The task Files window. Same scenario as flows/adeflow TestTaskFiles; both halves read
// the ade-task-files contract: this one asserts the UI over the mocked IPC, the Go test the
// bound calls themselves.

const t = (id: string) => `[data-testid="${id}"]`;
const rowPaths = (page: Page) =>
  page
    .locator(t('ade-files-row'))
    .evaluateAll((els) => els.map((e) => e.getAttribute('data-path')));
const editorText = (page: Page) => page.locator(`${t('ade-files-viewer')} .view-lines`);

interface Listing {
  paths: string[];
}

test('contract: the tree lists the worktree files, root directories open', async ({ relaunch }) => {
  const { window: page, control } = await openFilesWindow(relaunch, taskFilesControl());
  const listing = contract<Listing>('ade-task-files', 'list');
  await expect(page.locator(t('ade-files-row')).first()).toBeVisible();
  const paths = await rowPaths(page);
  // Directories are rows too: every listed file shows, under its (expanded) directory.
  for (const p of listing.paths) expect(paths).toContain(p);
  expect(paths).toContain('src');
  expect(calls(control, IPC.adeTaskFiles)[0]?.args).toEqual({ branchId: FILES_BRANCH });
  await expect(page.locator(t('ade-files-viewer-empty'))).toBeVisible();
});

test('the name filter narrows the tree', async ({ relaunch }) => {
  const { window: page } = await openFilesWindow(relaunch, taskFilesControl());
  await expect(page.locator(t('ade-files-row')).first()).toBeVisible();
  await page.locator(t('ade-files-filter')).fill('REA');
  await expect.poll(() => rowPaths(page)).toEqual(['README.md']);
});

test('contract: a file opens in the read-only viewer', async ({ relaunch }) => {
  const { window: page, control } = await openFilesWindow(relaunch, taskFilesControl());
  await openTaskFile(page, 'README.md');
  const read = contract<{ text: string }>('ade-task-files', 'read');
  await expect(editorText(page)).toContainText('the needle is here');
  expect(read.text).toContain('the needle is here');
  expect(calls(control, IPC.adeTaskFileRead)[0]?.args).toEqual({
    branchId: FILES_BRANCH,
    path: 'README.md',
  });
  // A markdown file offers Reading beside Source.
  await expect(page.locator(t('ade-files-view-toggle'))).toBeVisible();
  await page.locator(t('ade-files-view-reading')).click();
  await expect(page.locator(t('ade-files-markdown'))).toContainText('Readme');
});

test('contract: search lists matches and a match opens its file', async ({ relaunch }) => {
  const { window: page, control } = await openFilesWindow(relaunch, taskFilesControl());
  await page.locator(t('ade-files-mode-search')).click();
  const input = page.locator(t('ade-files-search-input'));
  await input.fill('needle');
  await input.press('Enter');
  await expect.poll(() => calls(control, IPC.adeTaskFilesSearch)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskFilesSearch)[0]?.args).toMatchObject({
    branchId: FILES_BRANCH,
    query: 'needle',
  });
  await emitWailsEvent(page, IPC.codeSearch, taskSearchEvent());
  const hits = contract<Record<string, number[]>>('ade-task-files', 'search');
  const files = page.locator(t('ade-files-search-file-row'));
  await expect(files).toHaveCount(Object.keys(hits).length);
  await expect(page.locator(t('ade-files-search-row'))).toHaveCount(
    Object.values(hits).reduce((n, lines) => n + lines.length, 0),
  );
  await page.locator(`${t('ade-files-search-row')}[data-path="README.md"]`).click();
  await expect(page.locator(`${t('ade-files-viewer')}[data-path="README.md"]`)).toBeVisible();
  await expect(editorText(page)).toContainText('the needle is here');
});

// The kira:code:search subscription and the merge-by-path insert: Go tests cannot reach either.
test('search results stream out of order and merge by path', async ({ relaunch }) => {
  const { window: page, control } = await openFilesWindow(
    relaunch,
    taskFilesControl([
      { channel: IPC.adeTaskFilesSearch, response: { searchId: 'search-1' }, hold: true },
    ]),
  );
  await page.locator(t('ade-files-mode-search')).click();
  const input = page.locator(t('ade-files-search-input'));
  await input.fill('const');
  await input.press('Enter');

  const group = (path: string) => ({
    path,
    truncated: false,
    matches: [
      {
        line: 1,
        column: 8,
        endColumn: 13,
        preview: `export const ${path[0]} = 1;`,
        previewMatchStart: 7,
        previewMatchEnd: 12,
        truncatedStart: false,
        truncatedEnd: false,
      },
    ],
  });
  // b.ts first, before the start's reply: enumeration order, not path order, and the event
  // overtakes the reply that used to drop it.
  await emitWailsEvent(page, IPC.codeSearch, {
    searchId: 'search-1',
    seq: 0,
    files: [group('b.ts')],
    done: false,
  });
  control.release(IPC.adeTaskFilesSearch);
  const fileRows = page.locator(t('ade-files-search-file-row'));
  await expect(fileRows).toHaveCount(1);

  await emitWailsEvent(page, IPC.codeSearch, {
    searchId: 'search-1',
    seq: 1,
    files: [group('a.ts')],
    done: true,
    stats: { filesScanned: 2, filesMatched: 2, filesSkipped: 0, matches: 2, truncated: false },
  });
  await expect(fileRows).toHaveCount(2);
  await expect(fileRows.nth(0)).toHaveAttribute('data-path', 'a.ts');
  await expect(fileRows.nth(1)).toHaveAttribute('data-path', 'b.ts');
  await expect(page.locator(t('ade-files-search-status'))).toContainText('2 results in 2 files');
});

test('file rows carry per-language icons, directories keep their codicon', async ({ relaunch }) => {
  const { window: page } = await openFilesWindow(
    relaunch,
    taskFilesControl([
      { channel: IPC.adeTaskFiles, response: { paths: ['main.go', 'app.ts', 'src/util.ts'] } },
    ]),
  );
  const row = (path: string) => page.locator(`${t('ade-files-row')}[data-path="${path}"]`);
  const goIcon = row('main.go').locator('.node-icon');
  const tsIcon = row('app.ts').locator('.node-icon');
  await expect(goIcon).toBeVisible();
  await expect(tsIcon).toBeVisible();
  const [goStyle, tsStyle] = await Promise.all([
    goIcon.getAttribute('style'),
    tsIcon.getAttribute('style'),
  ]);
  expect(goStyle).toContain('mask-image');
  expect(tsStyle).toContain('mask-image');
  expect(goStyle).not.toBe(tsStyle);

  // The directory synthesized from 'src/util.ts' keeps a codicon glyph, not a mask icon.
  const dir = row('src');
  await expect(dir.locator('.codicon-folder, .codicon-folder-opened')).toHaveCount(1);
  await expect(dir.locator('.node-icon[style*="mask-image"]')).toHaveCount(0);
});

test('contract: a refused read shows the message in the viewer', async ({ relaunch }) => {
  const refusal = contract<{ code: string; message: string }>('ade-task-files', 'refusal');
  const { window: page } = await openFilesWindow(
    relaunch,
    taskFilesControl([
      {
        channel: IPC.adeTaskFileRead,
        args: { branchId: FILES_BRANCH, path: 'escape' },
        error: refusal,
      },
      {
        channel: IPC.adeTaskFileRead,
        args: { branchId: FILES_BRANCH, path: 'README.md' },
        response: contract('ade-task-files', 'read'),
      },
    ]),
  );
  await page.locator(`${t('ade-files-row')}[data-path="escape"]`).click();
  await expect(page.locator(t('ade-files-viewer-error'))).toContainText(refusal.message);
});

test('Reveal acts on the open file, else the worktree root; the label follows the launcher', async ({
  relaunch,
}) => {
  const { window: page, control } = await openFilesWindow(relaunch, taskFilesControl());
  const reveal = page.locator(t('ade-files-reveal'));
  await expect(reveal).toHaveAttribute('aria-label', /Reveal in file manager/);
  await reveal.click();
  await expect.poll(() => calls(control, IPC.adeTaskFileReveal)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskFileReveal)[0]?.args).toEqual({
    branchId: FILES_BRANCH,
    path: '',
  });
  await openTaskFile(page, 'README.md');
  await reveal.click();
  await expect.poll(() => calls(control, IPC.adeTaskFileReveal)).toHaveLength(2);
  expect(calls(control, IPC.adeTaskFileReveal)[1]?.args).toEqual({
    branchId: FILES_BRANCH,
    path: 'README.md',
  });
});

test('Open in VS Code sends the open file', async ({ relaunch }) => {
  const { window: page, control } = await openFilesWindow(relaunch, taskFilesControl());
  await openTaskFile(page, 'README.md');
  await page.locator(t('ade-files-open-editor')).click();
  await expect.poll(() => calls(control, IPC.adeTaskFileOpenInEditor)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskFileOpenInEditor)[0]?.args).toEqual({
    branchId: FILES_BRANCH,
    path: 'README.md',
  });
  await expect(page.locator(t('ade-files-launch-error'))).toHaveCount(0);
});

test('contract: a missing editor shows its message', async ({ relaunch }) => {
  const missing = contract<{ code: string; message: string }>('ade-task-files', 'editorMissing');
  const { window: page } = await openFilesWindow(
    relaunch,
    taskFilesControl([{ channel: IPC.adeTaskFileOpenInEditor, error: missing }]),
  );
  await page.locator(t('ade-files-open-editor')).click();
  await expect(page.locator(t('ade-files-launch-error'))).toContainText(missing.message);
});

test('a build with no launchers shows neither action', async ({ relaunch }) => {
  const { window: page } = await openFilesWindow(
    relaunch,
    taskFilesControl([
      { channel: IPC.adeTaskFilesLaunchers, response: { reveal: 'none', editor: false } },
    ]),
  );
  await expect(page.locator(t('ade-files-refresh'))).toBeVisible();
  await expect(page.locator(t('ade-files-reveal'))).toHaveCount(0);
  await expect(page.locator(t('ade-files-open-editor'))).toHaveCount(0);
});

test('a task without a worktree has an empty Files window', async ({ relaunch }) => {
  const { window: page } = await openFilesWindow(relaunch, taskFilesControl(), 'T_alerts');
  await expect(page.locator(t('ade-files-empty'))).toBeVisible();
});

test('a task with several worktrees picks the branch, and leaving one closes its session', async ({
  relaunch,
}) => {
  const { window: page, control } = await openFilesWindow(relaunch, taskFilesControl(), 'T_bill');
  const branch = page.locator(t('ade-files-branch'));
  await expect(branch.locator('option')).toHaveCount(3);
  await expect.poll(() => calls(control, IPC.adeTaskFiles)).toHaveLength(1);
  const first = calls(control, IPC.adeTaskFiles)[0]?.args as { branchId: string };
  const other = (
    await branch
      .locator('option')
      .evaluateAll((os) => os.map((o) => (o as HTMLOptionElement).value))
  ).find((v) => v !== first.branchId) as string;
  await branch.selectOption(other);
  await expect.poll(() => calls(control, IPC.adeTaskFiles)).toHaveLength(2);
  expect(calls(control, IPC.adeTaskFiles)[1]?.args).toEqual({ branchId: other });
  await expect.poll(() => calls(control, IPC.adeTaskFilesClose)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskFilesClose)[0]?.args).toEqual(first);
});
