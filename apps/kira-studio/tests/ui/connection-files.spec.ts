import type { ConnectionSummary } from '@shared/domain/connection';
import type { ConnectionConfigView, ConnectionFileDetail } from '@shared/domain/connectionConfig';
import { expectHighlighted, replaceCodeEditorText } from '@workbench/testing/ui/codeEditor';
import type { ControlSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import { connectionRow } from './support/tree';

// Contract connection-files. Backend half: dbflow TestConnectionFiles (a file edit changes the
// row and the tree filters, a bad edit keeps the last valid version and reports its line).

const FILE = 'local.yaml';
const SCENARIO = 'connection-files';

const listed = () => contract<ConnectionSummary[]>(SCENARIO, 'ConnectionsService.List');
const edited = () => contract<ConnectionSummary[]>(SCENARIO, 'ConnectionsService.List#edited');
const goodView = () => contract<ConnectionConfigView>(SCENARIO, 'ConnectionConfigService.View');
const badView = () => contract<ConnectionConfigView>(SCENARIO, 'ConnectionConfigService.View#bad');

const GOOD_TEXT = 'name: Local\nkind: sqlite\ndatabase: /data/local.db\ncolor: blue\n';
const BAD_TEXT = 'name: Local (edited)\nkind: nope\ndatabase: /data/local.db\n';

function detail(over: Partial<ConnectionFileDetail> = {}): ConnectionFileDetail {
  return {
    file: FILE,
    id: listed()[0]?.id ?? '',
    path: '/kira/connections/local.yaml',
    text: GOOD_TEXT,
    hash: 'hash-1',
    problem: null,
    lastGood: false,
    editable: true,
    secrets: [],
    ...over,
  };
}

function boot(extra: ControlSnapshot[] = []): ControlSnapshot[] {
  return [
    { channel: IPC.connectionsList, response: listed() },
    { channel: IPC.connectionConfigView, response: goodView() },
    ...extra,
  ];
}

test('contract: the tree shows the file-backed connection with its color rail', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: boot() });
  const row = await connectionRow(page, 'Local');
  await expect(row).toBeVisible();
  await expect(row.locator('[data-testid="tree-rail"]')).toHaveClass(/\bbg-conn-blue\b/);
  await expect(row.locator('[data-testid="tree-connection-problem"]')).toHaveCount(0);
});

test('contract: a file edit repaints the row name and color', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: boot() });
  await expect(await connectionRow(page, 'Local')).toBeVisible();

  await emitWailsEvent(page, IPC.connectionsChanged, edited());
  const row = await connectionRow(page, edited()[0]?.name ?? '');
  await expect(row.locator('[data-testid="tree-rail"]')).toHaveClass(/\bbg-conn-green\b/);
});

test('contract: a bad edit marks the row with its line and keeps the last valid name and color', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: listed() },
      { channel: IPC.connectionConfigView, response: goodView() },
      { channel: IPC.connectionConfigView, response: badView() },
    ],
  });
  const row = await connectionRow(page, 'Local');
  await expect(row).toBeVisible();
  await expect(row.locator('[data-testid="tree-connection-problem"]')).toHaveCount(0);

  await emitWailsEvent(page, IPC.connectionConfigChanged, null);
  const marker = row.locator('[data-testid="tree-connection-problem"]');
  await expect(marker).toBeVisible();
  await expect(marker).toContainText('Line 3');
  await expect(row.locator('[data-testid="tree-rail"]')).toHaveClass(/\bbg-conn-blue\b/);
  await expect(await connectionRow(page, 'Local')).toBeVisible();
});

test('a file that never held a valid connection is listed and opens its dialog', async ({
  relaunch,
}) => {
  const broken: ConnectionConfigView = {
    ...goodView(),
    files: [
      {
        file: 'broken.yaml',
        id: '',
        name: '',
        problem: { line: 2, message: 'bad' },
        lastGood: false,
      },
    ],
  };
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [] },
      { channel: IPC.connectionConfigView, response: broken },
      {
        channel: IPC.connectionConfigFile,
        response: detail({
          file: 'broken.yaml',
          id: '',
          text: 'name: [\n',
          problem: { line: 2, message: 'bad' },
        }),
      },
    ],
  });
  const entry = page.locator('[data-testid="tree-connection-broken-broken.yaml"]');
  await expect(entry).toBeVisible();
  await expect(entry).toContainText('Line 2');
  await entry.click();
  await expect(page.locator('[data-testid="connection-file-dialog"]')).toBeVisible();
  await expect(page.locator('[data-testid="config-file-status"]')).toContainText('Line 2');
});

test('Edit file shows the file error and Save sends the text with its base hash', async ({
  relaunch,
}) => {
  const fixed = detail({ text: GOOD_TEXT.replace('Local', 'Local 2'), hash: 'hash-2' });
  const { window: page, control } = await relaunch({
    control: boot([
      {
        channel: IPC.connectionConfigFile,
        response: detail({
          text: BAD_TEXT,
          problem: { line: 2, message: 'kind must be valid' },
          lastGood: true,
        }),
      },
      { channel: IPC.connectionConfigSaveFile, response: fixed },
    ]),
  });
  await (await connectionRow(page, 'Local')).click({ button: 'right' });
  await page.click('[data-testid="menu-item-edit"]');

  const dialog = page.locator('[data-testid="connection-file-dialog"]');
  await expect(dialog).toBeVisible();
  await expect(dialog.locator('[data-testid="config-file-status"]')).toContainText('Line 2');
  await expect(dialog.locator('[data-testid="connection-file-last-good"]')).toBeVisible();
  await dialog.locator('[data-testid="connection-tab-yaml"]').click();
  await expectHighlighted(dialog.locator('[data-testid="connection-file-text"]'), 'yaml');

  await replaceCodeEditorText(
    page,
    dialog.locator('[data-testid="connection-file-text"]'),
    fixed.text,
  );
  await dialog.locator('[data-testid="connection-file-save"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.connectionConfigSaveFile)?.args)
    .toEqual({ file: FILE, text: fixed.text, baseHash: 'hash-1' });
});

test('Open in editor from the menu opens the connection file', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: boot([{ channel: IPC.configFilesOpen, response: null }]),
  });
  await (await connectionRow(page, 'Local')).click({ button: 'right' });
  await page.click('[data-testid="menu-item-open-file"]');
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.configFilesOpen)?.args)
    .toEqual({ folder: 'connections', file: FILE });
});

test('the connection menu has no color or read-only item', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: boot() });
  await (await connectionRow(page, 'Local')).click({ button: 'right' });
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await expect(page.locator('[data-testid="menu-item-edit"]')).toBeVisible();
  await expect(page.locator('[data-testid="menu-item-color"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="menu-item-readonly"]')).toHaveCount(0);
});
