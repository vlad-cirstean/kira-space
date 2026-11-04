import type { Page } from '@playwright/test';
import { codeEditorText, replaceCodeEditorText } from '@workbench/testing/ui/codeEditor';
import { expect, test } from './fixtures';
import { adeV2Control, type openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// The repo form picks a Run script for the prepare hook and a deployed-SHA row. Backend half:
// repoflow TestRepoScriptPicker writes the repo-script-picker contract.

interface FileFx {
  file: string;
  hash: string;
  text: string;
}
interface FormFx {
  text: string;
}
interface ScriptsFx {
  scripts: { name: string; kind: string; collection: string }[];
}

const t = (id: string) => `[data-testid="${id}"]`;
const KEY = 'repo-script-picker';

const file = () => contract<FileFx>(KEY, 'file#read');
const saved = () => contract<FileFx>(KEY, 'file#saved');
const list = () => contract<ScriptsFx>(KEY, 'scripts#list');
const form = (name: string) => contract<FormFx>(KEY, `form#${name}`);

const formCall = (name: string): ControlSnapshot => ({
  channel: IPC.repoConfigForm,
  args: contract(KEY, `args:RepoConfigService.Form#${name}`),
  response: form(name),
});

async function open(
  relaunch: Parameters<typeof openPlan>[0],
  forms: string[],
  scripts: unknown = list(),
  extra: ControlSnapshot[] = [],
) {
  const app = await relaunch({
    control: adeV2Control([
      { channel: IPC.windowsEnsure, response: { mode: 'git' } },
      { channel: IPC.repoConfigFile, response: file() },
      { channel: IPC.customScriptsList, response: scripts },
      formCall('read'),
      ...forms.map(formCall),
      ...extra,
    ]),
  });
  await app.window.locator(t('manage-repos')).click();
  await app.window.locator(t('repos-dialog')).waitFor();
  await app.window.locator(t('repo-file')).waitFor();
  await expect(app.window.locator(t('repo-nick'))).toHaveValue('Web');
  return app;
}

// Waits until the draft holds `text`: the form call has answered and the next edit diffs against it.
async function expectDraft(page: Page, text: string): Promise<void> {
  await page.locator(t('repo-file-tab-yaml')).click();
  await expect.poll(() => codeEditorText(page.locator(t('repo-file-text')))).toBe(text);
}

const toForm = (page: Page) => page.locator(t('repo-file-tab-form')).click();

async function pickPrepare(page: Page): Promise<void> {
  await page.locator(t('repo-prepare-mode-run')).click();
  await page.locator(t('repo-prepare-run-script')).selectOption('Install deps');
  await expectDraft(page, form('pick-prepare').text);
  await toForm(page);
}

test('contract: the picker lists normal scripts only, grouped by collection', async ({
  relaunch,
}) => {
  const { window: page } = await open(relaunch, []);
  await page.locator(t('repo-prepare-mode-run')).click();
  const select = page.locator(t('repo-prepare-run-script'));
  const normal = list().scripts.filter((s) => s.kind === 'script');
  expect(normal.length).toBeGreaterThan(0);
  await expect(select.locator('option')).toHaveText(['None', ...normal.map((s) => s.name)]);
  await expect(select.locator('optgroup')).toHaveAttribute(
    'label',
    normal.find((s) => s.collection)?.collection ?? '',
  );
  await expect(select.locator('option', { hasText: 'Review' })).toHaveCount(0);
});

test('contract: picking a script writes prepare_run', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, ['pick-prepare']);
  await page.locator(t('repo-prepare-mode-run')).click();
  await page.locator(t('repo-prepare-run-script')).selectOption('Install deps');
  await expectDraft(page, form('pick-prepare').text);
  expect(form('pick-prepare').text).toContain('prepare_run');
  expect(form('pick-prepare').text).not.toContain('prepare_script');
  await toForm(page);
});

test('contract: params, then back to a command', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, [
    'pick-prepare',
    'pick-prepare-view',
    'params',
    'params-view',
    'back-to-command',
  ]);
  await pickPrepare(page);

  const params = page.locator(t('repo-prepare-run-params'));
  await params.locator(t('run-param-option-mode-ci')).click();
  await params.locator(t('run-param-option-flags-a')).click();
  await params.locator(t('run-param-option-flags-b')).click();
  await expect(params.locator(t('run-param-input-token'))).toHaveCount(0);
  await expect(page.locator(t('repo-prepare-run-secret'))).toContainText('token');
  await expect(page.locator(t('repo-prepare-run-dir'))).toContainText('worktree');
  await expectDraft(page, form('params').text);
  await toForm(page);

  await page.locator(t('repo-prepare-mode-command')).click();
  await page.locator(t('repo-prepare')).fill('pnpm i');
  await expectDraft(page, form('back-to-command').text);
  expect(form('back-to-command').text).not.toContain('prepare_run');
});

test('contract: an environment row picks a script', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, ['pick-env']);
  const rows = page.locator(t('repo-env'));
  await expect(rows).toHaveCount(2);
  await rows.nth(0).locator(t('repo-env-script-mode-run')).click();
  await rows.nth(0).locator(t('repo-env-script-run-script')).selectOption('Staging sha');
  await expectDraft(page, form('pick-env').text);
  await toForm(page);
  await expect(rows.nth(1).locator(t('repo-env-script-run-script'))).toHaveValue('Prod sha');
  await expect(rows.nth(1).locator(t('repo-env-script-run-error'))).toBeVisible();
});

test('contract: refusals show before a run fails and Remove drops unknown params', async ({
  relaunch,
}) => {
  const { window: page } = await open(relaunch, [
    'smart-read',
    'missing-read',
    'unknown-read',
    'drop-unknown',
  ]);
  const error = page.locator(t('repo-prepare-run-error'));
  const load = async (name: string) => {
    await page.locator(t('repo-file-tab-yaml')).click();
    await replaceCodeEditorText(page, page.locator(t('repo-file-text')), form(name).text);
    await toForm(page);
  };

  await load('smart-read');
  await expect(error).toContainText('smart script');
  await load('missing-read');
  await expect(error).toContainText('No run script named');
  await load('unknown-read');
  await expect(error).toContainText('zzz');
  await expect(page.locator(t('repo-prepare-run-script'))).toHaveValue('Install deps');
  await page.locator(t('repo-prepare-run-unknown-remove')).click();
  await expectDraft(page, form('drop-unknown').text);
});

test('contract: Save sends the edited text and the loaded hash', async ({ relaunch }) => {
  const { window: page, control } = await open(relaunch, ['pick-env'], list(), [
    { channel: IPC.repoConfigSaveFile, response: saved() },
  ]);
  const row = page.locator(t('repo-env')).nth(0);
  await row.locator(t('repo-env-script-mode-run')).click();
  await row.locator(t('repo-env-script-run-script')).selectOption('Staging sha');
  await expectDraft(page, form('pick-env').text);
  await page.locator(t('repo-file-save')).click();
  await expect.poll(() => calls(control, IPC.repoConfigSaveFile)).toHaveLength(1);
  expect(calls(control, IPC.repoConfigSaveFile)[0]?.args).toMatchObject({
    file: file().file,
    text: form('pick-env').text,
    baseHash: file().hash,
  });
});

test('with no normal scripts the picker says so', async ({ relaunch }) => {
  const only = { ...list(), scripts: list().scripts.filter((s) => s.kind === 'smart') };
  const { window: page } = await open(relaunch, [], only);
  await page.locator(t('repo-prepare-mode-run')).click();
  await expect(page.locator(t('repo-prepare-run-empty'))).toBeVisible();
});
