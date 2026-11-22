import type { Page } from '@playwright/test';
import { expect, filesTest, test } from './fixtures';
import { openPlan } from './support/adeV2';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// Frontend half of the module chords (⌘N, ⌥⌘N, ⌘O, ⌥⌘E): the menu item emits one channel and the
// active module maps it to its own "+" / import control. Playwright cannot press a native
// accelerator, so each case emits the channel. Backend half: appshell TestBuildTemplateGlobalChordItems.

const modeTab = (page: Page, mode: string) =>
  page.locator(`[data-testid="mode-tab"][data-mode="${mode}"]`);
const panel = (id: string) => `[data-testid="${id}"]`;

const REPO = {
  id: 'repo-1',
  name: 'demo-repo',
  root: '/tmp/demo-repo',
  repoId: '/tmp/demo-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};

const SCRIPT = {
  id: 'script-1',
  kind: 'script',
  params: [],
  smart: null,
  schedule: null,
  savedSecrets: [],
  name: 'New script',
  command: 'npm run build',
  workingDir: '',
  dirMode: 'kira',
  useAdeDir: false,
  color: 'none',
  collection: '',
  sortOrder: 0,
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
};

const SCRIPT_FILE = {
  file: 'script-1.yaml',
  id: 'script-1',
  path: '/kira/runs/script-1.yaml',
  text: 'name: New script\n',
  hash: 'h1',
  problem: null,
  lastGood: false,
  editable: true,
};

const SCRIPTS: ControlSnapshot[] = [
  { channel: IPC.windowsEnsure, response: { mode: 'automations' } },
  { channel: IPC.customScriptsCreate, response: SCRIPT },
  { channel: IPC.customScriptsFile, response: SCRIPT_FILE },
  { channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } },
];

const createdKinds = (control: { log(): { channel: string; args?: unknown }[] }) =>
  control
    .log()
    .filter((e) => e.channel === IPC.customScriptsCreate)
    .map((e) => (e.args as { fields: { kind: string } }).fields.kind);

test('New in Databases opens the new connection dialog', async ({ relaunch }) => {
  const { window: page } = await relaunch();
  await modeTab(page, 'studio').click();
  await emitWailsEvent(page, IPC.new, undefined);
  await expect(page.locator(panel('new-connection-dialog'))).toBeVisible();
});

test('New in API opens an HTTP request tab; New Alternate opens a gRPC one', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch();
  await modeTab(page, 'api').click();
  await emitWailsEvent(page, IPC.new, undefined);
  await expect(page.locator(panel('http-request-view'))).toBeVisible();
  await emitWailsEvent(page, IPC.newAlternate, undefined);
  await expect(page.locator(panel('grpc-request-view'))).toBeVisible();
  await expect(page.locator(panel('tab'))).toHaveCount(2);
});

test('Environments in API opens the Variables tab', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.collectionsList, response: { collections: [], items: [] } },
      { channel: IPC.variablesListEnvironments, response: [] },
    ],
  });
  await modeTab(page, 'api').click();
  await emitWailsEvent(page, IPC.apiEnvironments, undefined);
  await expect(page.locator(panel('variables-matrix'))).toBeVisible();
});

test('Import in API picks Postman files; in Databases it scans a DataGrip project', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.collectionsList, response: { collections: [], items: [] } },
      { channel: IPC.filesChooseOpenMany, response: { canceled: true, files: [] } },
      { channel: IPC.filesChooseFolder, response: { canceled: true, path: '' } },
    ],
  });
  const calls = (channel: string) => control.log().filter((e) => e.channel === channel);

  await modeTab(page, 'api').click();
  await emitWailsEvent(page, IPC.import, undefined);
  await expect.poll(() => calls(IPC.filesChooseOpenMany)).toHaveLength(1);
  expect(calls(IPC.filesChooseFolder)).toHaveLength(0);

  await modeTab(page, 'studio').click();
  await emitWailsEvent(page, IPC.import, undefined);
  await expect.poll(() => calls(IPC.filesChooseFolder)).toHaveLength(1);
  expect(calls(IPC.filesChooseOpenMany)).toHaveLength(1);
});

test('New in Automations creates a script and opens its file; New Alternate a smart one', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: SCRIPTS });
  await expect(page.locator(panel('automations-panel'))).toBeVisible();

  await emitWailsEvent(page, IPC.new, undefined);
  await expect(page.locator(panel('run-file-dialog'))).toBeVisible();
  expect(createdKinds(control)).toEqual(['script']);

  await page.keyboard.press('Escape');
  await expect(page.locator(panel('run-file-dialog'))).toHaveCount(0);
  await emitWailsEvent(page, IPC.newAlternate, undefined);
  await expect(page.locator(panel('run-file-dialog'))).toBeVisible();
  expect(createdKinds(control)).toEqual(['script', 'smart']);
});

test('New in Automations with the project panel hidden shows the panel and creates the script', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: SCRIPTS });
  await expect(page.locator(panel('automations-panel'))).toBeVisible();
  await emitWailsEvent(page, IPC.toggleProjectPanel, undefined);
  await expect(page.locator(panel('automations-panel'))).toHaveCount(0);

  await emitWailsEvent(page, IPC.new, undefined);
  await expect(page.locator(panel('automations-panel'))).toBeVisible();
  await expect(page.locator(panel('run-file-dialog'))).toBeVisible();
  expect(createdKinds(control)).toEqual(['script']);
});

test('New in Agents opens the Add popover', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await emitWailsEvent(page, IPC.new, undefined);
  await expect(page.locator(panel('ade-add-popover'))).toBeVisible();
});

test('New in Git opens a terminal tab at the active repository root', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
      { channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } },
    ],
  });
  await page.locator(`${panel('repo-row')}[data-repo-id="${REPO.id}"]`).click();
  await emitWailsEvent(page, IPC.new, undefined);
  await expect(page.locator('.xterm-rows')).toBeVisible();
  await expect
    .poll(() =>
      control
        .log()
        .some(
          (e) =>
            e.channel === IPC.terminalOpen &&
            (e.args as { cwd?: string } | undefined)?.cwd === REPO.root,
        ),
    )
    .toBe(true);
});

test('New in Git with no repository active opens nothing', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.codeWorkspaceListRepos, response: [REPO] }],
  });
  await emitWailsEvent(page, IPC.new, undefined);
  await expect(page.locator('.xterm-rows')).toHaveCount(0);
  expect(control.log().filter((e) => e.channel === IPC.terminalOpen)).toHaveLength(0);
});

test('New in Docker does nothing', async ({ relaunch }) => {
  const { window: page } = await relaunch();
  await modeTab(page, 'docker').click();
  await emitWailsEvent(page, IPC.new, undefined);
  await emitWailsEvent(page, IPC.newAlternate, undefined);
  await expect(page.locator(panel('new-connection-dialog'))).toHaveCount(0);
  await expect(page.locator(panel('tab'))).toHaveCount(0);
});

filesTest('New in the opened-files window does nothing', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.openedFilesIsFilesWindow, response: true },
      { channel: IPC.openedFilesPending, response: [] },
      { channel: IPC.windowsEnsure, response: { mode: 'studio' } },
    ],
  });
  await emitWailsEvent(page, IPC.new, undefined);
  await expect(page.locator(panel('new-connection-dialog'))).toHaveCount(0);
});
