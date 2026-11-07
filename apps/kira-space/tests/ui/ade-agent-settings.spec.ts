import {
  codeEditorText,
  expectHighlighted,
  replaceCodeEditorText,
} from '@workbench/testing/ui/codeEditor';
import { expect, test } from './fixtures';
import { openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// The Settings tab: one shared agent settings file, its secrets and what each provider receives.
// The fixtures are the Go flow test's own answers (tests/contract/agent-settings.json).

const ref = ['$', '{secret:gh-token}'].join('');
const t = (id: string) => `[data-testid="${id}"]`;

const base = contract<Record<string, unknown>>('agent-settings', 'view');
const view = (over: Record<string, unknown> = {}) => ({ ...base, ...over });
const unsetSecret = [{ name: 'gh-token', set: false, referenced: true }];

async function openSettings(
  relaunch: Parameters<typeof openPlan>[0],
  over: Record<string, unknown> = {},
  extra: Parameters<typeof openPlan>[1] = [],
) {
  const app = await openPlan(relaunch, [
    { channel: IPC.agentSettingsGet, response: view(over) },
    ...extra,
  ]);
  await app.window.locator(t('ade-tab-agents')).click();
  await app.window.locator(t('ade-agents-tab-settings')).click();
  await app.window.locator(t('ade-settings')).waitFor();
  await app.window.locator(t('ade-settings-note')).waitFor();
  return app;
}

test('the YAML and the resulting config show, with references as written', async ({ relaunch }) => {
  const { window: page } = await openSettings(relaunch);
  await expect
    .poll(() => codeEditorText(page.locator(t('ade-settings-text'))))
    .toBe(String(base.text ?? ''));
  await expectHighlighted(page.locator(t('ade-settings-text')), 'yaml');
  const files = page.locator(t('ade-settings-file'));
  await expect(files.first()).toContainText(`Bearer ${ref}`);
  await expect(page.locator(t('ade-settings-result'))).not.toContainText('p453-');
  await expect(page.locator(t('ade-settings-kira'))).toContainText('kira-space');
  await expect(page.locator(t('ade-settings-notes'))).toContainText('Headless');
});

test('Set on a secret that is not set sends the value once and clears the field', async ({
  relaunch,
}) => {
  const { window: page, control } = await openSettings(relaunch, { secrets: unsetSecret }, [
    {
      channel: IPC.agentSettingsSetSecret,
      response: view({ secrets: [{ name: 'gh-token', set: true, referenced: true }] }),
    },
  ]);
  const row = page.locator(t('ade-settings-secret-gh-token'));
  await expect(row.locator(t('ade-settings-secret-state'))).toContainText('Not set');
  await row.locator(t('ade-settings-secret-set')).click();
  const input = page.locator(t('ade-settings-secret-value'));
  await expect(input).toHaveAttribute('type', 'password');
  await input.fill('s3cret-value');
  await page.locator(t('ade-settings-secret-value-reveal')).click();
  await expect(input).toHaveAttribute('type', 'text');
  await page.locator(t('ade-settings-secret-value-reveal')).click();
  await expect(input).toHaveAttribute('type', 'password');
  await page.locator(t('ade-settings-secret-save')).click();
  await expect.poll(() => calls(control, IPC.agentSettingsSetSecret)).toHaveLength(1);
  expect(calls(control, IPC.agentSettingsSetSecret)[0]?.args).toEqual({
    name: 'gh-token',
    value: 's3cret-value',
  });
  await expect(row.locator(t('ade-settings-secret-state'))).toContainText('Set');
  await expect(page.locator(t('ade-settings-secret-dialog'))).toHaveCount(0);
  await row.locator(t('ade-settings-secret-set')).click();
  await expect(page.locator(t('ade-settings-secret-value'))).toHaveValue('');
  await expect(page.locator(t('ade-settings'))).not.toContainText('s3cret-value');
});

test('Save sends the text with the loaded hash and a reload shows the saved text', async ({
  relaunch,
}) => {
  const saved = view({ text: 'env:\n  A: b\n', hash: 'h2' });
  const { window: page, control } = await openSettings(relaunch, {}, [
    { channel: IPC.agentSettingsSave, response: saved },
    // The second answer to Get is what the file holds after the save.
    { channel: IPC.agentSettingsGet, response: saved },
  ]);
  await expect(page.locator(t('ade-settings-save'))).toBeDisabled();
  await replaceCodeEditorText(page, page.locator(t('ade-settings-text')), 'env:\n  A: b\n');
  await page.locator(t('ade-settings-save')).click();
  await expect.poll(() => calls(control, IPC.agentSettingsSave)).toHaveLength(1);
  expect(calls(control, IPC.agentSettingsSave)[0]?.args).toEqual({
    text: 'env:\n  A: b\n',
    baseHash: base.hash,
  });
  await expect(page.locator(t('ade-settings-save'))).toBeDisabled();
  await page.reload();
  await page.locator(t('ade-tab-agents')).click();
  await page.locator(t('ade-agents-tab-settings')).click();
  await expect
    .poll(() => codeEditorText(page.locator(t('ade-settings-text'))))
    .toBe('env:\n  A: b\n');
});

test('a file with an error shows its line and the last-good note', async ({ relaunch }) => {
  const { window: page } = await openSettings(relaunch, {
    problem: { line: 4, message: 'event: not one of the events' },
    lastGood: true,
  });
  await expect(page.locator(t('config-file-status'))).toContainText('4');
  await expect(page.locator(t('ade-settings-last-good'))).toBeVisible();
});

test('a missing file shows the none hint', async ({ relaunch }) => {
  const { window: page } = await openSettings(relaunch, {
    exists: false,
    text: '',
    hash: '',
    secrets: [],
  });
  await expect(page.locator(t('ade-settings-none'))).toContainText('settings.yaml');
  await expect(page.locator(t('ade-settings-secrets-none'))).toBeVisible();
});

test('storage that is unavailable disables Set and shows its reason', async ({ relaunch }) => {
  const { window: page } = await openSettings(relaunch, {
    secrets: unsetSecret,
    storage: {
      available: false,
      backend: 'none',
      insecureFallback: false,
      reason: 'No keychain on this computer.',
    },
  });
  await expect(page.locator(t('ade-settings-storage-off'))).toContainText('No keychain');
  await expect(page.locator(t('ade-settings-secret-set'))).toBeDisabled();
});

test('Open names the agent folder and file', async ({ relaunch }) => {
  const { window: page, control } = await openSettings(relaunch);
  await page.locator(t('config-file-open')).click();
  await expect.poll(() => calls(control, IPC.configFilesOpen)).toHaveLength(1);
  expect(calls(control, IPC.configFilesOpen)[0]?.args).toEqual({
    folder: 'agent',
    file: 'settings.yaml',
  });
});

test('a save refused for a changed file offers Reload and Overwrite', async ({ relaunch }) => {
  const { window: page, control } = await openSettings(relaunch, {}, [
    {
      channel: IPC.agentSettingsSave,
      error: { code: 'E_CONFLICT', message: 'The settings file changed since you loaded it.' },
    },
  ]);
  await replaceCodeEditorText(page, page.locator(t('ade-settings-text')), 'env:\n  A: b\n');
  await page.locator(t('ade-settings-save')).click();
  await expect(page.locator(t('ade-settings-save-error'))).toContainText('changed');
  await page.locator(t('ade-settings-overwrite')).click();
  await expect.poll(() => calls(control, IPC.agentSettingsSave)).toHaveLength(2);
  await page.locator(t('ade-settings-reload')).click();
  await expect(page.locator(t('ade-settings-save-error'))).toHaveCount(0);
});
