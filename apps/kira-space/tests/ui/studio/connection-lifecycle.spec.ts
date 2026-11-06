import type { ConnectionSummary } from '@shared/domain/connection';
import type { ConnectionConfigView, ConnectionFileDetail } from '@shared/domain/connectionConfig';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { connectionRow } from './support/tree';

// Duplicate and Delete from the connection dialog. Backend half: dbflow TestConnectionLifecycle
// writes the connection-lifecycle contract (the call shapes) beside connection-files.

const KEY = 'connection-lifecycle';
const dlg = '[data-testid="connection-file-dialog"]';

const list = () => contract<ConnectionSummary[]>('connection-files', 'ConnectionsService.List');
const view = () =>
  contract<ConnectionConfigView>('connection-files', 'ConnectionConfigService.View');
const copy = () => contract<ConnectionSummary>(KEY, 'duplicate');
// The source file's text and form calls come from the connection-form contract (recorded on pg.yaml).
const source = (): ConnectionFileDetail => ({
  ...contract<ConnectionFileDetail>('connection-form', 'file#read'),
  file: 'local.yaml',
  id: list()[0]?.id ?? '',
});
const copyDetail = (): ConnectionFileDetail => ({
  ...contract<ConnectionFileDetail>(KEY, 'file#copy'),
  id: copy().id,
});

const formCall = (name: string): ControlSnapshot => ({
  channel: IPC.connectionConfigForm,
  args: {
    ...(contract('connection-form', `args:ConnectionConfigService.Form#${name}`) as object),
    file: 'local.yaml',
  },
  response: contract('connection-form', `form#${name}`),
});

const calls = (control: { log(): { channel: string; args?: unknown }[] }, channel: string) =>
  control
    .log()
    .filter((e) => e.channel === channel)
    .map((e) => e.args);

async function open(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  extra: ControlSnapshot[] = [],
) {
  const app = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: list() },
      { channel: IPC.connectionConfigView, response: view() },
      { channel: IPC.connectionConfigFile, args: { file: 'local.yaml' }, response: source() },
      formCall('read'),
      formCall('rename'),
      ...extra,
    ],
  });
  await (await connectionRow(app.window, 'Local')).click({ button: 'right' });
  await app.window.click('[data-testid="menu-item-edit"]');
  await expect(app.window.locator(dlg)).toBeVisible();
  return app;
}

test('contract: Duplicate switches the dialog to the copy and waits for a save', async ({
  relaunch,
}) => {
  const { window: page, control } = await open(relaunch, [
    {
      channel: IPC.connectionsDuplicate,
      args: { ...contract<object>(KEY, 'args:ConnectionsService.Duplicate'), id: list()[0].id },
      response: copy(),
    },
    { channel: IPC.connectionConfigFile, args: { file: copy().file }, response: copyDetail() },
  ]);
  const duplicate = page.locator('[data-testid="connection-duplicate"]');
  await expect(duplicate).toBeEnabled();

  await page.locator('[data-testid="connection-name"]').fill('Pg staging');
  await expect(duplicate).toBeDisabled();
  await expect(page.locator('[data-testid="connection-save-first"]')).toBeVisible();
  await page.locator('[data-testid="connection-file-discard"]').click();
  await expect(duplicate).toBeEnabled();

  await duplicate.click();
  await expect
    .poll(() => calls(control, IPC.connectionsDuplicate))
    .toEqual([{ ...contract<object>(KEY, 'args:ConnectionsService.Duplicate'), id: list()[0].id }]);
  await expect
    .poll(() => calls(control, IPC.connectionConfigFile))
    .toContainEqual({ file: copy().file });
});

test('Delete asks first, then removes the connection and closes the dialog', async ({
  relaunch,
}) => {
  const { window: page, control } = await open(relaunch, [
    { channel: IPC.connectionsDelete, args: { id: list()[0].id }, response: null },
  ]);
  await page.locator('[data-testid="connection-delete"]').click();
  await expect(page.locator('[data-testid="confirm-dialog-message"]')).toContainText('Local');
  await page.click('[data-testid="confirm-dialog-cancel"]');
  expect(calls(control, IPC.connectionsDelete)).toHaveLength(0);

  await page.locator('[data-testid="connection-delete"]').click();
  await page.click('[data-testid="confirm-dialog-confirm"]');
  await expect.poll(() => calls(control, IPC.connectionsDelete)).toEqual([{ id: list()[0].id }]);
  await expect(page.locator(dlg)).toHaveCount(0);
});

test('contract: a file that never held a connection is deleted by file', async ({ relaunch }) => {
  const broken = contract<ConnectionFileDetail>(KEY, 'file#broken');
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [] },
      {
        channel: IPC.connectionConfigView,
        response: {
          ...view(),
          files: [
            {
              file: broken.file,
              id: '',
              name: '',
              problem: broken.problem,
              lastGood: false,
            },
          ],
        },
      },
      { channel: IPC.connectionConfigFile, response: broken },
      {
        channel: IPC.connectionConfigDeleteFile,
        args: contract<object>(KEY, 'args:ConnectionConfigService.DeleteFile'),
        response: null,
      },
    ],
  });
  await page.locator(`[data-testid="tree-connection-broken-${broken.file}"]`).click();
  await expect(page.locator('[data-testid="connection-duplicate"]')).toBeDisabled();
  await page.locator('[data-testid="connection-delete"]').click();
  await page.click('[data-testid="confirm-dialog-confirm"]');
  await expect
    .poll(() => calls(control, IPC.connectionConfigDeleteFile))
    .toEqual([contract<object>(KEY, 'args:ConnectionConfigService.DeleteFile')]);
  await expect(page.locator(dlg)).toHaveCount(0);
});
