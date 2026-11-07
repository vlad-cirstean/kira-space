import {
  codeEditorText,
  expectHighlighted,
  replaceCodeEditorText,
} from '@workbench/testing/ui/codeEditor';
import { expect, test } from './fixtures';
import { openAdeRepos, type openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// A repo's settings file as a form and as YAML over one draft. Backend half: repoflow
// TestRepoConfigForm writes the repo-config-form contract.

interface FileFx {
  file: string;
  hash: string;
  text: string;
}
interface FormFx {
  text: string;
  problem: { line: number; message: string } | null;
}

const t = (id: string) => `[data-testid="${id}"]`;
const KEY = 'repo-config-form';

const file = () => contract<FileFx>(KEY, 'file#read');
const saved = () => contract<FileFx>(KEY, 'file#saved');
const form = (name: string) => contract<FormFx>(KEY, `form#${name}`);
const args = (name: string) => contract(KEY, `args:RepoConfigService.Form#${name}`);

const formCall = (name: string): ControlSnapshot => ({
  channel: IPC.repoConfigForm,
  args: args(name),
  response: form(name),
});

async function open(relaunch: Parameters<typeof openPlan>[0], extra: ControlSnapshot[] = []) {
  const app = await openAdeRepos(relaunch, [
    { channel: IPC.repoConfigFile, response: file() },
    formCall('read'),
    ...extra,
  ]);
  await app.window.locator(t('repo-file')).waitFor();
  await expect(app.window.locator(t('repo-nick'))).toHaveValue('Web');
  return app;
}

test('contract: a form edit lands in the YAML tab with its comments', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, [formCall('rename')]);
  await page.locator(t('repo-nick')).fill('Web app');
  await page.locator(t('repo-file-tab-yaml')).click();
  await expect
    .poll(() => codeEditorText(page.locator(t('repo-file-text'))))
    .toBe(form('rename').text);
  expect(form('rename').text).toContain('# Settings of the web repo.');
  expect(form('rename').text).toContain('# shown in the board');
  await expectHighlighted(page.locator(t('repo-file-text')), 'yaml');
});

test('contract: typing a prepare script replaces the run reference', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, [formCall('seam')]);
  await page.locator(t('repo-prepare-mode-command')).click();
  await page.locator(t('repo-prepare')).fill('bun install');
  await page.locator(t('repo-file-tab-yaml')).click();
  await expect
    .poll(() => codeEditorText(page.locator(t('repo-file-text'))))
    .toBe(form('seam').text);
  expect(form('seam').text).not.toContain('prepare_run');
  expect(form('seam').text).toContain('prepare_script: bun install');
});

test('contract: typing a row script keeps the other row run reference', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, [formCall('env')]);
  const rows = page.locator(t('repo-env'));
  await expect(rows).toHaveCount(2);
  await rows.nth(0).locator(t('repo-env-script-mode-command')).click();
  await rows
    .nth(0)
    .locator(t('repo-env-script'))
    .fill('curl -fsS https://staging.example.com/version');
  await page.locator(t('repo-file-tab-yaml')).click();
  await expect.poll(() => codeEditorText(page.locator(t('repo-file-text')))).toBe(form('env').text);
  expect(form('env').text).toContain('Prod sha');
  expect(form('env').text).not.toContain('Staging sha');
});

test('contract: a YAML edit shows in the form', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, [formCall('yaml')]);
  await page.locator(t('repo-file-tab-yaml')).click();
  await replaceCodeEditorText(page, page.locator(t('repo-file-text')), form('yaml').text);
  await page.locator(t('repo-file-tab-form')).click();
  await expect(page.locator(t('repo-nick'))).toHaveValue('Shop');
});

test('contract: bad YAML disables the form and names the line', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, [formCall('bad')]);
  await page.locator(t('repo-file-tab-yaml')).click();
  await replaceCodeEditorText(page, page.locator(t('repo-file-text')), form('bad').text);
  await page.locator(t('repo-file-tab-form')).click();
  await expect(page.locator(t('repo-form-problem'))).toContainText(
    `Line ${form('bad').problem?.line}`,
  );
  await expect(page.locator(t('repo-nick'))).toBeDisabled();
  await expect(page.locator(t('repo-file-save'))).toBeDisabled();
});

test('contract: Save sends the edited text and the loaded hash', async ({ relaunch }) => {
  const { window: page, control } = await open(relaunch, [
    formCall('rename'),
    { channel: IPC.repoConfigSaveFile, response: saved() },
  ]);
  await page.locator(t('repo-nick')).fill('Web app');
  await page.locator(t('repo-file-save')).click();
  await expect.poll(() => calls(control, IPC.repoConfigSaveFile)).toHaveLength(1);
  expect(calls(control, IPC.repoConfigSaveFile)[0]?.args).toMatchObject({
    file: file().file,
    text: form('rename').text,
    baseHash: file().hash,
  });
});

test('a save over a file changed elsewhere offers Reload and Overwrite', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, [
    formCall('rename'),
    {
      channel: IPC.repoConfigSaveFile,
      error: {
        code: 'E_CONFLICT',
        message: 'This file changed on disk since you started editing.',
      },
    },
  ]);
  await page.locator(t('repo-nick')).fill('Web app');
  await page.locator(t('repo-file-save')).click();
  await expect(page.locator(t('repo-file-reload'))).toBeVisible();
  await expect(page.locator(t('repo-file-overwrite'))).toBeVisible();
});
