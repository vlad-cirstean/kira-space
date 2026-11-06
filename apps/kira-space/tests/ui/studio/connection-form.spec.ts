import type { ConnectionSummary } from '@shared/domain/connection';
import type { ConnectionConfigView, ConnectionFileDetail } from '@shared/domain/connectionConfig';
import {
  codeEditorText,
  expectHighlighted,
  replaceCodeEditorText,
} from '@workbench/testing/ui/codeEditor';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { connectionRow } from './support/tree';

// A connection file as a form and as YAML over one draft. Backend half: dbflow
// TestConnectionForm writes the connection-form contract.

const KEY = 'connection-form';
const FILE = 'local.yaml';

interface FormFx {
  text: string;
  problem: { line: number; message: string } | null;
}

const list = () => contract<ConnectionSummary[]>('connection-files', 'ConnectionsService.List');
const view = () =>
  contract<ConnectionConfigView>('connection-files', 'ConnectionConfigService.View');
const detail = (key: string) => ({
  ...contract<ConnectionFileDetail>(KEY, `file#${key}`),
  file: FILE,
  id: list()[0]?.id ?? '',
});
const form = (name: string) => contract<FormFx>(KEY, `form#${name}`);
// The contract was recorded on pg.yaml; the dialog opens local.yaml.
const formCall = (name: string): ControlSnapshot => ({
  channel: IPC.connectionConfigForm,
  args: { ...(contract(KEY, `args:ConnectionConfigService.Form#${name}`) as object), file: FILE },
  response: contract(KEY, `form#${name}`),
});

const dlg = '[data-testid="connection-file-dialog"]';

async function open(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  extra: ControlSnapshot[] = [],
) {
  const app = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: list() },
      { channel: IPC.connectionConfigView, response: view() },
      { channel: IPC.connectionConfigFile, response: detail('read') },
      formCall('read'),
      ...extra,
    ],
  });
  await (await connectionRow(app.window, 'Local')).click({ button: 'right' });
  await app.window.click('[data-testid="menu-item-edit"]');
  await expect(app.window.locator(dlg)).toBeVisible();
  await expect(app.window.locator(`${dlg} [data-testid="connection-name"]`)).toHaveValue('Pg');
  return app;
}

test('contract: a form edit lands in the YAML tab with its comments', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, [formCall('rename')]);
  await page.locator('[data-testid="connection-name"]').fill('Pg staging');
  await page.locator('[data-testid="connection-tab-yaml"]').click();
  const editor = page.locator('[data-testid="connection-file-text"]');
  await expect.poll(() => codeEditorText(editor)).toBe(form('rename').text);
  expect(form('rename').text).toContain('# Staging database for the web team.');
  expect(form('rename').text).toContain('# shown in the sidebar');
  await expectHighlighted(editor, 'yaml');
});

test('contract: a YAML edit shows in the form', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, [formCall('yaml')]);
  await page.locator('[data-testid="connection-tab-yaml"]').click();
  await replaceCodeEditorText(
    page,
    page.locator('[data-testid="connection-file-text"]'),
    form('yaml').text,
  );
  await page.locator('[data-testid="connection-tab-general"]').click();
  await expect(page.locator('[data-testid="connection-name"]')).toHaveValue('Pg yaml');
});

test('contract: bad YAML disables the form and names the line', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, [formCall('bad')]);
  await page.locator('[data-testid="connection-tab-yaml"]').click();
  await replaceCodeEditorText(
    page,
    page.locator('[data-testid="connection-file-text"]'),
    form('bad').text,
  );
  await page.locator('[data-testid="connection-tab-general"]').click();
  // The problem's pointer (/kind) belongs to a form field, so it shows there, not as a banner.
  await expect(page.locator('[data-testid="connection-kind-error"]')).toContainText(
    form('bad').problem?.message ?? '',
  );
  await expect(page.locator('[data-testid="connection-name"]')).toBeDisabled();
  await expect(page.locator('[data-testid="connection-file-save"]')).toBeDisabled();
});

test('contract: Save sends the edited text and the loaded hash', async ({ relaunch }) => {
  const { window: page, control } = await open(relaunch, [
    formCall('rename'),
    { channel: IPC.connectionConfigSaveFile, response: detail('saved') },
  ]);
  await page.locator('[data-testid="connection-name"]').fill('Pg staging');
  await page.locator('[data-testid="connection-file-save"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.connectionConfigSaveFile)?.args)
    .toEqual({ file: FILE, text: form('rename').text, baseHash: detail('read').hash });
});

test('a save over a file changed elsewhere offers Reload and Overwrite', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, [
    formCall('rename'),
    {
      channel: IPC.connectionConfigSaveFile,
      error: {
        code: 'E_CONFLICT',
        message: 'This file changed on disk since you started editing.',
      },
    },
  ]);
  await page.locator('[data-testid="connection-name"]').fill('Pg staging');
  await page.locator('[data-testid="connection-file-save"]').click();
  await expect(page.locator(`${dlg} [data-testid="connection-file-reload"]`)).toBeVisible();
  await expect(page.locator(`${dlg} [data-testid="connection-file-overwrite"]`)).toBeVisible();
});

test('a typed password goes through the write-only call, never the file', async ({ relaunch }) => {
  const { window: page, control } = await open(relaunch, [
    { channel: IPC.connectionConfigSetSecret, response: view() },
  ]);
  await page.locator('[data-testid="connection-password"]').fill('hunter2');
  await page.locator('[data-testid="connection-file-save"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.connectionConfigSetSecret)?.args)
    .toEqual({ kind: 'password', name: 'pg.password', value: 'hunter2' });
  expect(control.log().some((e) => e.channel === IPC.connectionConfigSaveFile)).toBe(false);
});
