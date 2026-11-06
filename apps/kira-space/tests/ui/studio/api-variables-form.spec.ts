import {
  codeEditorText,
  expectHighlighted,
  replaceCodeEditorText,
} from '@workbench/testing/ui/codeEditor';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openHttpMode } from './support/apiMode';
import { collectionRow } from './support/apiRows';

// The variables dialog edits an environment or collection file as a form and as YAML over one draft.
// Backend half: apiflow.TestApiVariablesForm writes the api-variables-form contract.

const KEY = 'api-variables-form';
const ENV_FILE = 'staging.environment.yaml';

interface FileFx {
  file: string;
  hash: string;
  text: string;
  kind: string;
}
interface FormFx {
  text: string;
  problem: { line: number; message: string } | null;
}

const file = (key: string) => contract<FileFx>(KEY, `file#${key}`);
const form = (name: string) => contract<FormFx>(KEY, `form#${name}`);
const formCall = (name: string): ControlSnapshot => ({
  channel: IPC.apiConfigForm,
  args: contract(KEY, `args:ApiConfigService.Form#${name}`),
  response: form(name),
});

const NOW = '2026-01-01T00:00:00Z';
const COLLECTIONS = {
  collections: [
    {
      id: 'col-1',
      name: 'Orders API',
      file: file('collection').file,
      sortOrder: 0,
      createdAt: NOW,
      updatedAt: NOW,
    },
  ],
  items: [],
};
const ENVIRONMENTS = [
  {
    id: 'env-staging',
    name: 'Staging',
    sortOrder: 0,
    isActive: false,
    color: 'green',
    description: '',
    file: ENV_FILE,
  },
];

const dlg = '[data-testid="api-variables-dialog"]';

async function openEnvironment(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  extra: ControlSnapshot[] = [],
) {
  const app = await relaunch({
    control: [
      { channel: IPC.collectionsList, response: COLLECTIONS },
      { channel: IPC.variablesListEnvironments, response: ENVIRONMENTS },
      { channel: IPC.apiConfigFile, args: { file: ENV_FILE }, response: file('read') },
      formCall('read'),
      ...extra,
    ],
  });
  await openHttpMode(app.window);
  await app.window.click('[data-testid="api-environments"]');
  await app.window.click('[data-testid="environment-edit"]');
  await expect(app.window.locator(dlg)).toBeVisible();
  await expect(app.window.locator('[data-testid="api-environment-name"]')).toHaveValue('Staging');
  return app;
}

test('contract: a form edit lands in the YAML tab with its comments', async ({ relaunch }) => {
  const { window: page } = await openEnvironment(relaunch, [formCall('rename')]);
  await page.locator('[data-testid="api-environment-name"]').fill('Staging EU');
  await page.locator('[data-testid="api-tab-yaml"]').click();
  const editor = page.locator('[data-testid="api-variables-text"]');
  await expect.poll(() => codeEditorText(editor)).toBe(form('rename').text);
  expect(form('rename').text).toContain('# Staging servers for the web team.');
  expect(form('rename').text).toContain('# shown in the picker');
  await expectHighlighted(editor, 'yaml');
});

test('contract: a YAML edit shows in the form', async ({ relaunch }) => {
  const { window: page } = await openEnvironment(relaunch, [formCall('yaml')]);
  await page.locator('[data-testid="api-tab-yaml"]').click();
  await replaceCodeEditorText(
    page,
    page.locator('[data-testid="api-variables-text"]'),
    form('yaml').text,
  );
  await page.locator('[data-testid="api-tab-environment"]').click();
  await expect(page.locator('[data-testid="api-environment-name"]')).toHaveValue('Staging yaml');
});

test('contract: bad YAML disables the form and names the line', async ({ relaunch }) => {
  const { window: page } = await openEnvironment(relaunch, [formCall('bad')]);
  await page.locator('[data-testid="api-tab-yaml"]').click();
  await replaceCodeEditorText(
    page,
    page.locator('[data-testid="api-variables-text"]'),
    form('bad').text,
  );
  await page.locator('[data-testid="api-tab-variables"]').click();
  await expect(page.locator('[data-testid="api-variables-error"]')).toContainText(
    `Line ${form('bad').problem?.line}`,
  );
  await expect(page.locator('[data-testid="api-variable-name-0"]')).toBeDisabled();
  await expect(page.locator('[data-testid="api-variables-save"]')).toBeDisabled();
});

test('contract: Save sends the edited text and the loaded hash', async ({ relaunch }) => {
  const { window: page, control } = await openEnvironment(relaunch, [
    formCall('rename'),
    { channel: IPC.apiConfigSaveFile, response: file('saved') },
  ]);
  await page.locator('[data-testid="api-environment-name"]').fill('Staging EU');
  await page.locator('[data-testid="api-variables-save"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.apiConfigSaveFile)?.args)
    .toEqual({ file: ENV_FILE, text: form('rename').text, baseHash: file('read').hash });
});

test('a save over a file changed elsewhere offers Reload and Overwrite', async ({ relaunch }) => {
  const { window: page } = await openEnvironment(relaunch, [
    formCall('rename'),
    {
      channel: IPC.apiConfigSaveFile,
      error: {
        code: 'E_CONFLICT',
        message: 'This file changed on disk since you started editing.',
      },
    },
  ]);
  await page.locator('[data-testid="api-environment-name"]').fill('Staging EU');
  await page.locator('[data-testid="api-variables-save"]').click();
  await expect(page.locator(`${dlg} [data-testid="api-variables-reload"]`)).toBeVisible();
  await expect(page.locator(`${dlg} [data-testid="api-variables-overwrite"]`)).toBeVisible();
});

test('a typed secret goes through the write-only call, never the file', async ({ relaunch }) => {
  const view = contract<{ storage: object }>('api-files', 'ApiConfigService.View');
  const stored = { ...view, storage: { ...view.storage, available: true, reason: null } };
  const { window: page, control } = await openEnvironment(relaunch, [
    { channel: IPC.apiConfigView, response: stored },
    { channel: IPC.apiConfigSetSecret, response: stored },
  ]);
  await page.locator('[data-testid="api-tab-variables"]').click();
  await page.locator('[data-testid="api-variable-secret-1"]').fill('hunter2');
  await page.locator('[data-testid="api-variables-save"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.apiConfigSetSecret)?.args)
    .toEqual({ name: 'staging.token', value: 'hunter2' });
  expect(control.log().some((e) => e.channel === IPC.apiConfigSaveFile)).toBe(false);
});

test('dotenv text refuses a value typed on a secret line', async ({ relaunch }) => {
  const { window: page, control } = await openEnvironment(relaunch);
  await page.locator('[data-testid="api-tab-variables"]').click();
  await page.locator('[data-testid="api-variables-bulk-toggle"]').click();
  const editor = page.locator('[data-testid="variables-bulk-text"]');
  await replaceCodeEditorText(
    page,
    editor,
    'base_url=https://staging.example.com\ntoken=hunter2\n',
  );
  await expect(page.locator('[data-testid="variables-bulk-error"]')).toContainText('Line 2');
  await expect(page.locator('[data-testid="variables-bulk-error"]')).toContainText('token');
  expect(control.log().some((e) => e.channel === IPC.apiConfigSetSecret)).toBe(false);
});

test('a collection file edits its variables from the tree menu', async ({ relaunch }) => {
  const { window: page } = await (async () => {
    const app = await relaunch({
      control: [
        { channel: IPC.collectionsList, response: COLLECTIONS },
        { channel: IPC.variablesListEnvironments, response: [] },
        {
          channel: IPC.apiConfigFile,
          args: { file: file('collection').file },
          response: file('collection'),
        },
        {
          channel: IPC.apiConfigForm,
          args: { file: file('collection').file, text: file('collection').text, edits: [] },
          response: { text: file('collection').text, value: { name: 'Orders API' }, problem: null },
        },
      ],
    });
    await openHttpMode(app.window);
    await collectionRow(app.window, 'col-1').click({ button: 'right' });
    await app.window.click('[data-testid="menu-item-edit-variables"]');
    return app;
  })();
  await expect(page.locator(dlg)).toBeVisible();
  await expect(page.locator('[data-testid="api-tab-environment"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="api-tab-variables"]')).toBeVisible();
});
