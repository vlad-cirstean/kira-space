import type { Page } from '@playwright/test';
import {
  runFormResultSchema,
  scriptFileViewSchema,
  scriptsSnapshotSchema,
} from '@shared/domain/scripts';
import { expect, test } from './fixtures';
import { modeTab } from './studio/support/apiMode';
import { contract } from './support/contract';
import { gitStreamParams } from './support/gitStreamMock';
import {
  openPortGraph,
  PICKER_RESULTS,
  rootRow,
  SHA_A,
  singleRowChunks,
} from './support/gitUiPortFixtures';
import { IPC } from './support/ipcChannels';
import { expectOn, expectStaysOnAfterClick } from './support/toggleGroup';

// P668: single-select radios became toggle groups. UI half only: each group keeps a value selected
// and sends the same IPC args as before. RunFolderTab, WorktreeDialog and StashDialog have no
// other spec.

const tid = (page: Page, id: string) => page.locator(`[data-testid="${id}"]`);

const form = (key: string) => contract('runs-form', `form#${key}`, { schema: runFormResultSchema });
const file = contract('runs-form', 'file#read', { schema: scriptFileViewSchema });

test('RunFolderTab: Kira folder is on, picking a fixed folder sends dir once, a re-click changes nothing', async ({
  relaunch,
}) => {
  const read = form('read');
  const valid = contract('runs-file', 'snapshot#valid', { schema: scriptsSnapshotSchema });
  const { window: page, control } = await relaunch({
    control: [
      {
        channel: IPC.customScriptsList,
        response: {
          ...valid,
          scripts: valid.scripts.map((s) => ({
            ...s,
            id: 'ship',
            name: 'Ship',
            collection: 'Ops',
          })),
        },
      },
      { channel: IPC.customScriptsFile, response: file },
      { channel: IPC.customScriptsForm, response: read },
    ],
  });
  await modeTab(page, 'automations').click();
  await tid(page, 'script-ship').dblclick();
  const dialog = tid(page, 'run-file-dialog');
  await expect(dialog).toBeVisible();
  await dialog.locator('[data-testid="run-file-tab-folder"]').click();

  const kira = dialog.locator('[data-testid="script-dialog-dir-kira"]');
  const fixed = dialog.locator('[data-testid="script-dialog-dir-fixed"]');
  await expectOn(kira);
  control.respond(IPC.customScriptsForm, {
    ...read,
    value: { ...(read.value as object), dir: 'fixed' },
  });
  const dirEdits = () =>
    control
      .log()
      .filter((e) => e.channel === IPC.customScriptsForm)
      .flatMap((e) => (e.args as { edits: { path: string[]; value?: unknown }[] }).edits)
      .filter((e) => e.path.join('.') === 'dir');
  await fixed.click();
  await expect(dialog.locator('[data-testid="script-dialog-workingdir-choose"]')).toBeVisible();
  await expect.poll(dirEdits).toEqual([expect.objectContaining({ value: 'fixed' })]);
  await expectStaysOnAfterClick(fixed);
  await page.waitForTimeout(500);
  expect(dirEdits()).toHaveLength(1);
});

test('WorktreeDialog: the start-from mode stays selected and drives the preflight', async ({
  relaunch,
}) => {
  const page = await openPortGraph(relaunch, {
    chunks: singleRowChunks([rootRow(SHA_A, 'A commit')]),
    results: {
      ...PICKER_RESULTS,
      'preflight.worktreeAdd': {
        path: '/tmp/port-repo-worktree',
        mode: 'detach',
        branch: undefined,
        blockers: [],
        notes: [],
        verdict: 'clean',
        routes: [],
      },
    },
  });
  await page.locator('.slick-row[data-row="0"] [data-testid="message-subject"]').click({
    button: 'right',
  });
  await tid(page, 'menu-item-createWorktreeHere').click();
  await expectOn(tid(page, 'worktree-mode-detach'));
  const newBranch = tid(page, 'worktree-mode-newBranch');
  await newBranch.click();
  await expectOn(newBranch);
  await expect
    .poll(async () => (await gitStreamParams(page, 'preflight.worktreeAdd')).at(-1))
    .toMatchObject({ mode: 'newBranch' });
  await expectStaysOnAfterClick(newBranch);
});

test('StashDialog: the save source keeps one value selected and the save sends it', async ({
  relaunch,
}) => {
  const page = await openPortGraph(relaunch, {
    chunks: singleRowChunks([rootRow(SHA_A, 'A commit')]),
    results: PICKER_RESULTS,
  });
  await tid(page, 'branch-trigger').click();
  await page.getByRole('button', { name: /^Stashes/ }).click();
  await page
    .locator('[data-testid="branch-section"][aria-label="Stashes"] [data-testid="branch-row"]')
    .getByRole('button', { name: /More actions/ })
    .click();
  await tid(page, 'menu-item-stashSaveGlobal').click();

  await expectOn(tid(page, 'stash-save-source-entry'));
  const working = tid(page, 'stash-save-source-workingTree');
  await working.click();
  await expectStaysOnAfterClick(working);
  await tid(page, 'stash-save-source-entry').click();
  await expectOn(tid(page, 'stash-save-source-entry'));
  await working.click();
  await page.getByLabel(/label/i).first().fill('keep this');
  await page.getByRole('button', { name: /^Save/ }).click();
  await expect
    .poll(async () => (await gitStreamParams(page, 'op.run')).at(-1))
    .toMatchObject({ op: { kind: 'globalStashSave' } });
});
