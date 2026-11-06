import {
  codeEditorText,
  expectHighlighted,
  replaceCodeEditorText,
} from '@workbench/testing/ui/codeEditor';
import { expect, test } from './fixtures';
import { adeV2Control, type openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// A repo's settings file in the Repositories dialog. Backend half: repoflow TestRepoConfigFile.

interface FileFx {
  file: string;
  filePath: string;
  path: string;
  text: string;
  hash: string;
  exists: boolean;
  problem: { line: number; message: string } | null;
  lastGood: boolean;
}

interface ItemFx {
  file: string;
  path: string;
  problem: { line: number; message: string } | null;
  repo: string;
}

const t = (id: string) => `[data-testid="${id}"]`;

const saved = () => contract<FileFx>('repo-config-file', 'RepoConfigService.SaveFile');
const none = () => contract<FileFx>('repo-config-file', 'RepoConfigService.File#none');
const broken = () => contract<FileFx>('repo-config-file', 'RepoConfigService.File#broken');

async function open(relaunch: Parameters<typeof openPlan>[0], extra: ControlSnapshot[] = []) {
  const app = await relaunch({
    control: adeV2Control([{ channel: IPC.windowsEnsure, response: { mode: 'git' } }, ...extra]),
  });
  await app.window.locator(t('manage-repos')).click();
  await app.window.locator(t('repos-dialog')).waitFor();
  await app.window.locator(t('repo-file')).waitFor();
  await app.window.locator(t('repo-file-tab-yaml')).click();
  return app;
}

test('contract: the pane shows the file and Save sends the hash it loaded', async ({
  relaunch,
}) => {
  const fx = saved();
  const { window: page, control } = await open(relaunch, [
    { channel: IPC.repoConfigFile, response: fx },
  ]);
  await expect.poll(() => codeEditorText(page.locator(t('repo-file-text')))).toBe(fx.text);
  await expect(page.locator(t('config-file-bar'))).toContainText('proj.yaml');
  await expectHighlighted(page.locator(t('repo-file-text')), 'yaml');
  await expect(page.locator(t('repo-file-save'))).toBeDisabled();
  expect(calls(control, IPC.repoConfigFile)[0]?.args).toEqual({
    path: '/tmp/acme-customer-dashboard-web-frontend',
    file: '',
  });

  await replaceCodeEditorText(
    page,
    page.locator(t('repo-file-text')),
    'path: x\nnickname: Edited\n',
  );
  await page.locator(t('repo-file-save')).click();
  await expect.poll(() => calls(control, IPC.repoConfigSaveFile)).toHaveLength(1);
  expect(calls(control, IPC.repoConfigSaveFile)[0]?.args).toEqual({
    path: fx.path,
    file: fx.file,
    text: 'path: x\nnickname: Edited\n',
    baseHash: fx.hash,
  });
});

test('contract: a repo without a file shows the template and Save creates it', async ({
  relaunch,
}) => {
  const fx = none();
  const { window: page, control } = await open(relaunch, [
    { channel: IPC.repoConfigFile, response: fx },
  ]);
  await expect(page.locator(t('repo-file-none'))).toBeVisible();
  await expect.poll(() => codeEditorText(page.locator(t('repo-file-text')))).toBe(fx.text);
  await expect(page.locator(t('config-file-bar'))).toHaveCount(0);
  await expect(page.locator(t('repo-file-save'))).toHaveText('Create settings file');
  await page.locator(t('repo-file-save')).click();
  await expect.poll(() => calls(control, IPC.repoConfigSaveFile)).toHaveLength(1);
  expect(calls(control, IPC.repoConfigSaveFile)[0]?.args).toEqual({
    path: fx.path,
    file: '',
    text: fx.text,
    baseHash: '',
  });
});

test('contract: a broken file shows its line and the last-good note', async ({ relaunch }) => {
  const fx = broken();
  const { window: page } = await open(relaunch, [{ channel: IPC.repoConfigFile, response: fx }]);
  await expect(page.locator(t('config-file-status'))).toContainText(String(fx.problem?.line));
  await expect(page.locator(t('repo-file-last-good'))).toBeVisible();
  await expect.poll(() => codeEditorText(page.locator(t('repo-file-text')))).toBe(fx.text);
});

test('a save refused for a changed file offers Reload and Overwrite', async ({ relaunch }) => {
  const { window: page, control } = await open(relaunch, [
    {
      channel: IPC.repoConfigSaveFile,
      error: {
        code: 'E_CONFLICT',
        message: 'This file changed on disk since you started editing.',
      },
    },
  ]);
  await replaceCodeEditorText(page, page.locator(t('repo-file-text')), 'path: x\n');
  await page.locator(t('repo-file-save')).click();
  await expect(page.locator(t('repo-file-save-error'))).toContainText('changed');
  await page.locator(t('repo-file-overwrite')).click();
  await expect.poll(() => calls(control, IPC.repoConfigSaveFile)).toHaveLength(2);
  await page.locator(t('repo-file-reload')).click();
  await expect(page.locator(t('repo-file-save-error'))).toHaveCount(0);
});

test('a repoConfig push re-reads the open file', async ({ relaunch }) => {
  const { window: page, control } = await open(relaunch);
  const before = calls(control, IPC.repoConfigFile).length;
  await emitWailsEvent(page, IPC.repoConfigChanged, null);
  await expect.poll(() => calls(control, IPC.repoConfigFile).length).toBeGreaterThan(before);
});

test('contract: a repo with a file error shows a dot, and unbound files list under Other files', async ({
  relaunch,
}) => {
  const dup = contract<ItemFx>('repo-config-file', 'RepoConfigService.List#duplicate');
  const { window: page, control } = await open(relaunch, [
    { channel: IPC.repoConfigList, response: { dir: '~/.kira-space/config/repos', files: [dup] } },
  ]);
  await expect(
    page.locator(
      `${t('repos-dialog-repo')}[data-repo-id="repo-api"] ${t('repos-dialog-repo-problem')}`,
    ),
  ).toBeVisible();
  await expect(
    page.locator(
      `${t('repos-dialog-repo')}[data-repo-id="repo-web-app"] ${t('repos-dialog-repo-problem')}`,
    ),
  ).toHaveCount(0);

  await expect(page.locator(t('repos-dialog-files-count'))).toHaveText('1');
  await page.locator(t('repos-dialog-tab-files')).click();
  await expect(page.locator(t('repos-dialog-file'))).toHaveCount(1);
  await expect
    .poll(() => calls(control, IPC.repoConfigFile).map((e) => e.args))
    .toContainEqual({ path: dup.path, file: dup.file });
});
