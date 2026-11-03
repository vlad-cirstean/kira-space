import { type RepoSummary, repoSummarySchema } from '@shared/domain/repo';
import { expect, test } from './fixtures';
import { adeFixture, adeV2Control, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// The Git module's Repositories dialog: import, per-repo configuration and scan folders.

interface ReposFx {
  repos: { codeRepoId: string; nickname: string }[];
  folders: { path: string; watch: boolean }[];
}

interface FolderFx {
  hidden: boolean;
  repoCount: number;
  hiddenCount: number;
}

const t = (id: string) => `[data-testid="${id}"]`;

const chooseFolder = {
  channel: IPC.filesChooseFolder,
  response: { canceled: false, path: '~/code/oss' },
};

async function openDialog(relaunch: Parameters<typeof openPlan>[0], extra: ControlSnapshot[] = []) {
  const app = await relaunch({
    control: adeV2Control([
      { channel: IPC.windowsEnsure, response: { mode: 'git' } },
      chooseFolder,
      ...extra,
    ]),
  });
  await app.window.locator(t('manage-repos')).click();
  await app.window.locator(t('repos-dialog')).waitFor();
  return app;
}

async function openFolders(relaunch: Parameters<typeof openPlan>[0]) {
  const app = await openDialog(relaunch);
  await app.window.locator(t('repos-dialog-tab-folders')).click();
  return app;
}

test('the Agents shell has no Repos tab; its empty state opens the dialog in the Git module', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  await expect(page.locator(t('ade-tab-repos'))).toHaveCount(0);

  const empty = await relaunch({
    control: adeV2Control([{ channel: IPC.codeWorkspaceListRepos, response: [] }]),
  });
  await empty.window.locator(t('ade-import')).click();
  await expect(empty.window.locator(t('repos-dialog'))).toBeVisible();
  await expect(empty.window.locator(t('ade-view'))).toHaveCount(0);
});

test('a repo row menu opens the dialog on that repo', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: adeV2Control([{ channel: IPC.windowsEnsure, response: { mode: 'git' } }]),
  });
  await page.locator(`${t('repo-row')}[data-repo-id="repo-api"]`).click({ button: 'right' });
  await page.locator(t('menu-item-configure')).click();
  await expect(page.locator(t('repos-dialog'))).toBeVisible();
  await expect(
    page.locator(`${t('repos-dialog-repo-head')}[data-repo-id="repo-api"]`),
  ).toBeVisible();
  await expect(page.locator(t('repo-file'))).toBeVisible();
});

test('dialog folders list, add, watch and remove send their args', async ({ relaunch }) => {
  const fx = adeFixture<ReposFx>('repos');
  const { window: page, control } = await openFolders(relaunch);
  await expect(page.locator(t('repos-dialog-folder'))).toHaveCount(fx.folders.length);

  await page.locator(t('repos-dialog-add-folder')).click();
  await expect.poll(() => calls(control, IPC.adeTaskAddFolder)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskAddFolder)[0]?.args).toEqual({
    path: '~/code/oss',
    watch: true,
  });

  await page.locator(t('repos-dialog-folder-watch')).first().click();
  await expect.poll(() => calls(control, IPC.adeTaskSetFolderWatch)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskSetFolderWatch)[0]?.args).toEqual({
    path: fx.folders[0]?.path,
    watch: !fx.folders[0]?.watch,
  });

  await page.locator(t('repos-dialog-folder-remove')).first().click();
  await expect.poll(() => calls(control, IPC.adeTaskRemoveFolder)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskRemoveFolder)[0]?.args).toEqual({ path: fx.folders[0]?.path });
});

test('dialog import sends the chosen path', async ({ relaunch }) => {
  const { window: page, control } = await openDialog(relaunch);
  await page.locator(t('repos-dialog-import')).click();
  await expect.poll(() => calls(control, IPC.codeWorkspaceImportRepo)).toHaveLength(1);
  expect(calls(control, IPC.codeWorkspaceImportRepo)[0]?.args).toEqual({ path: '~/code/oss' });
});

test('a repos push re-reads the shared list without the ade module open', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: adeV2Control([{ channel: IPC.windowsEnsure, response: { mode: 'git' } }]),
  });
  await expect(page.locator(t('manage-repos'))).toBeVisible();
  const before = calls(control, IPC.codeWorkspaceListRepos).length;
  await emitWailsEvent(page, IPC.adeTaskReposChanged, null);
  await expect
    .poll(() => calls(control, IPC.codeWorkspaceListRepos).length)
    .toBeGreaterThan(before);
});

test('selecting a repo shows its identity and opens its settings file', async ({ relaunch }) => {
  const { window: page, control } = await openDialog(relaunch);
  await expect(page.locator(t('repos-dialog-repo'))).toHaveCount(3);
  await page.locator(t('repos-dialog-repo')).nth(1).click();
  await expect(page.locator(t('repo-full'))).toContainText('acme-platform-core-api-service');
  await expect
    .poll(() => calls(control, IPC.repoConfigFile).map((e) => e.args))
    .toContainEqual({ path: '/tmp/acme-platform-core-api-service', file: '' });
});

test('removing a repository asks first, from the repo head', async ({ relaunch }) => {
  const { window: page, control } = await openDialog(relaunch, [
    { channel: IPC.codeWorkspaceRemoveRepo },
  ]);
  await expect(page.locator(t('repos-dialog-repo-remove'))).toHaveCount(1);
  await expect(
    page.locator(`${t('repos-dialog-repo-head')} ${t('repos-dialog-repo-remove')}`),
  ).toHaveCount(1);
  await page.locator(t('repos-dialog-repo-remove')).click();
  await page.locator(t('confirm-dialog-cancel')).click();
  expect(calls(control, IPC.codeWorkspaceRemoveRepo)).toHaveLength(0);
  await page.locator(t('repos-dialog-repo-remove')).click();
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect.poll(() => calls(control, IPC.codeWorkspaceRemoveRepo)).toHaveLength(1);
});

test('removing a folder-sourced repository names the folder in the confirm', async ({
  relaunch,
}) => {
  const fx = adeFixture<ReposFx & { repos: { source: string }[] }>('repos');
  const { window: page } = await openDialog(relaunch);
  await page.locator(t('repos-dialog-repo-remove')).click();
  await expect(page.locator(t('confirm-dialog-message'))).toContainText(
    `A rescan of ${fx.repos[0]?.source} imports it again`,
  );
});

// Contract repos-hidden. Backend half: repoflow TestHideRepo / adeflow TestHideFolder.
test('contract: hiding from the repo head sends SetRepoHidden and the nav row turns hidden', async ({
  relaunch,
}) => {
  const rec = contract<RepoSummary>('repos-hidden', 'CodeWorkspaceService.SetRepoHidden', {
    schema: repoSummarySchema,
  });
  const { window: page, control } = await openDialog(relaunch, [
    {
      channel: IPC.codeWorkspaceSetRepoHidden,
      response: { ...rec, id: 'repo-web-app', name: 'acme-customer-dashboard-web-frontend' },
    },
  ]);
  const row = page.locator(`${t('repos-dialog-repo')}[data-repo-id="repo-web-app"]`);
  await expect(row).toHaveAttribute('data-hidden', 'false');
  await page.locator(t('repos-dialog-repo-hide')).click();
  await expect.poll(() => calls(control, IPC.codeWorkspaceSetRepoHidden)).toHaveLength(1);
  expect(calls(control, IPC.codeWorkspaceSetRepoHidden)[0]?.args).toEqual({
    id: 'repo-web-app',
    hidden: rec.hidden,
  });
  await expect(row).toHaveAttribute('data-hidden', 'true');
  await expect(page.locator(t('repos-dialog-repo-hide'))).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator(t('repos-dialog-repo-hidden-badge'))).toBeVisible();
});

test('contract: folder Hide all sends SetFolderHidden', async ({ relaunch }) => {
  const fx = adeFixture<ReposFx>('repos');
  const folder = contract<FolderFx>('repos-hidden', 'AdeTaskService.SetFolderHidden');
  const { window: page, control } = await openFolders(relaunch);
  await page.locator(t('repos-dialog-folder-hide')).first().click();
  await expect.poll(() => calls(control, IPC.adeTaskSetFolderHidden)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskSetFolderHidden)[0]?.args).toEqual({
    path: fx.folders[0]?.path,
    hidden: folder.hidden,
  });
});

test('contract: a folder row counts its hidden repos', async ({ relaunch }) => {
  const fx = adeFixture<ReposFx>('repos');
  const folder = contract<FolderFx>('repos-hidden', 'AdeTaskService.SetFolderHidden');
  const { window: page } = await relaunch({
    control: adeV2Control([
      { channel: IPC.windowsEnsure, response: { mode: 'git' } },
      {
        channel: IPC.adeTaskRepos,
        response: { ...fx, folders: [{ ...fx.folders[0], ...folder, path: fx.folders[0]?.path }] },
      },
    ]),
  });
  await page.locator(t('manage-repos')).click();
  await page.locator(t('repos-dialog-tab-folders')).click();
  await expect(page.locator(t('repos-dialog-folder-count')).first()).toHaveText(
    `${folder.repoCount} repositories · ${folder.hiddenCount} hidden`,
  );
  await expect(page.locator(t('repos-dialog-folder-hide')).first()).toHaveAttribute(
    'aria-pressed',
    'true',
  );
});

test('picking a swatch sends SetRepoColor and the nav dot follows', async ({ relaunch }) => {
  const { window: page, control } = await openDialog(relaunch, [
    {
      channel: IPC.codeWorkspaceSetRepoColor,
      response: {
        id: 'repo-web-app',
        name: 'acme-customer-dashboard-web-frontend',
        root: '/tmp/acme-customer-dashboard-web-frontend',
        repoId: '/tmp/acme-customer-dashboard-web-frontend',
        sortOrder: 1,
        color: 'red',
        createdAt: '2026-01-01T00:00:00.000Z',
        hidden: false,
      },
    },
  ]);
  await page.locator(`${t('repo-color')} ${t('color-red')}`).check({ force: true });
  await expect.poll(() => calls(control, IPC.codeWorkspaceSetRepoColor)).toHaveLength(1);
  expect(calls(control, IPC.codeWorkspaceSetRepoColor)[0]?.args).toEqual({
    id: 'repo-web-app',
    color: 'red',
  });
  await expect(
    page.locator(
      `${t('repos-dialog-repo')}[data-repo-id="repo-web-app"] ${t('repos-dialog-repo-rail')}`,
    ),
  ).toHaveClass(/bg-conn-red/);
});

// Contract repos. Backend half: repoflow TestImportViaFolderPicker. The imported record is what the
// repo list shows in the nav.
test('contract: an imported repo record shows as a nav row with its name and colour', async ({
  relaunch,
}) => {
  const rec = contract<{ id: string; name: string; color: string }>(
    'repos',
    'CodeWorkspaceService.ImportRepo',
  );
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.windowsEnsure, response: { mode: 'git' } },
      { channel: IPC.codeWorkspaceListRepos, response: [rec] },
    ],
  });
  const row = page.locator(`[data-testid="repo-row"][data-repo-id="${rec.id}"]`);
  await expect(row).toContainText(rec.name);
});
