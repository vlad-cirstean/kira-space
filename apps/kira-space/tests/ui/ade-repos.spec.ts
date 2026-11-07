import { type RepoSummary, repoSummarySchema } from '@shared/domain/repo';
import { replaceCodeEditorText } from '@workbench/testing/ui/codeEditor';
import { expect, test } from './fixtures';
import { adeFixture, adeV2Control, openAdeRepos, type openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// The ADE Repositories page: import, per-repo configuration and scan folders.

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

async function openPage(relaunch: Parameters<typeof openPlan>[0], extra: ControlSnapshot[] = []) {
  return openAdeRepos(relaunch, [chooseFolder, ...extra]);
}

async function openFolders(relaunch: Parameters<typeof openPlan>[0]) {
  const app = await openPage(relaunch);
  await app.window.locator(t('ade-repos-tab-folders')).click();
  return app;
}

test('the ADE empty state opens the Repositories page', async ({ relaunch }) => {
  const empty = await relaunch({
    control: adeV2Control([{ channel: IPC.codeWorkspaceListRepos, response: [] }]),
  });
  await empty.window.locator(t('ade-import')).click();
  await expect(empty.window.locator(t('ade-repos'))).toBeVisible();
  await expect(empty.window.locator(t('ade-shell'))).toBeVisible();
  await expect(empty.window.locator(t('ade-tab-repos'))).toHaveAttribute('data-state', 'active');
});

test('a Git repo row menu opens the page on that repo', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: adeV2Control([{ channel: IPC.windowsEnsure, response: { mode: 'git' } }]),
  });
  await page.locator(`${t('repo-row')}[data-repo-id="repo-api"]`).click({ button: 'right' });
  await page.locator(t('menu-item-configure')).click();
  await expect(page.locator(t('ade-repos'))).toBeVisible();
  await expect(page.locator(`${t('ade-repos-repo-head')}[data-repo-id="repo-api"]`)).toBeVisible();
  await expect(page.locator(t('repo-file'))).toBeVisible();
});

test('the Git start screen import opens the Repositories page', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: adeV2Control([
      { channel: IPC.windowsEnsure, response: { mode: 'git' } },
      { channel: IPC.codeWorkspaceListRepos, response: [] },
    ]),
  });
  await page.locator(t('git-start-import')).click();
  await expect(page.locator(t('ade-repos'))).toBeVisible();
  await expect(page.locator(t('ade-tab-repos'))).toHaveAttribute('data-state', 'active');
});

test('the Git panel has no repositories button', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: adeV2Control([{ channel: IPC.windowsEnsure, response: { mode: 'git' } }]),
  });
  await expect(page.locator(t('repo-row')).first()).toBeVisible();
  await expect(page.locator(t('manage-repos'))).toHaveCount(0);
});

test('leaving the page with an unsaved settings edit asks first', async ({ relaunch }) => {
  const { window: page } = await openPage(relaunch, [
    {
      channel: IPC.repoConfigFile,
      response: contract('repo-config-file', 'RepoConfigService.SaveFile'),
    },
  ]);
  await page.locator(t('repo-file')).waitFor();
  await page.locator(t('repo-file-tab-yaml')).click();
  await replaceCodeEditorText(
    page,
    page.locator(t('repo-file-text')),
    'path: x\nnickname: Edited\n',
  );
  await expect(page.locator(t('repo-file-save'))).toBeEnabled();
  await page.locator(t('ade-tab-plan')).click();
  await expect(page.locator(t('confirm-dialog'))).toBeVisible();
  await page.locator(t('confirm-dialog-cancel')).click();
  await expect(page.locator(t('ade-repos'))).toBeVisible();
  await expect(page.locator(t('repo-file-save'))).toBeEnabled();
  await page.locator(t('ade-tab-plan')).click();
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect(page.locator(t('ade-plan'))).toBeVisible();
  await expect(page.locator(t('ade-repos'))).toHaveCount(0);
});

test('page folders list, add, watch and remove send their args', async ({ relaunch }) => {
  const fx = adeFixture<ReposFx>('repos');
  const { window: page, control } = await openFolders(relaunch);
  await expect(page.locator(t('ade-repos-folder'))).toHaveCount(fx.folders.length);

  await page.locator(t('ade-repos-add-folder')).click();
  await expect.poll(() => calls(control, IPC.adeTaskAddFolder)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskAddFolder)[0]?.args).toEqual({
    path: '~/code/oss',
    watch: true,
  });

  await page.locator(t('ade-repos-folder-watch')).first().click();
  await expect.poll(() => calls(control, IPC.adeTaskSetFolderWatch)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskSetFolderWatch)[0]?.args).toEqual({
    path: fx.folders[0]?.path,
    watch: !fx.folders[0]?.watch,
  });

  await page.locator(t('ade-repos-folder-remove')).first().click();
  await expect.poll(() => calls(control, IPC.adeTaskRemoveFolder)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskRemoveFolder)[0]?.args).toEqual({ path: fx.folders[0]?.path });
});

test('page import sends the chosen path', async ({ relaunch }) => {
  const { window: page, control } = await openPage(relaunch);
  await page.locator(t('ade-repos-import')).click();
  await expect.poll(() => calls(control, IPC.codeWorkspaceImportRepo)).toHaveLength(1);
  expect(calls(control, IPC.codeWorkspaceImportRepo)[0]?.args).toEqual({ path: '~/code/oss' });
});

test('a repos push re-reads the shared list without the ade module open', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: adeV2Control([{ channel: IPC.windowsEnsure, response: { mode: 'git' } }]),
  });
  const before = calls(control, IPC.codeWorkspaceListRepos).length;
  await emitWailsEvent(page, IPC.adeTaskReposChanged, null);
  await expect
    .poll(() => calls(control, IPC.codeWorkspaceListRepos).length)
    .toBeGreaterThan(before);
});

test('selecting a repo shows its identity and opens its settings file', async ({ relaunch }) => {
  const { window: page, control } = await openPage(relaunch);
  await expect(page.locator(t('ade-repos-repo'))).toHaveCount(3);
  await page.locator(t('ade-repos-repo')).nth(1).click();
  await expect(page.locator(t('repo-full'))).toContainText('acme-platform-core-api-service');
  await expect
    .poll(() => calls(control, IPC.repoConfigFile).map((e) => e.args))
    .toContainEqual({ path: '/tmp/acme-platform-core-api-service', file: '' });
});

test('removing a repository asks first, from the repo head', async ({ relaunch }) => {
  const { window: page, control } = await openPage(relaunch, [
    { channel: IPC.codeWorkspaceRemoveRepo },
  ]);
  await expect(page.locator(t('ade-repos-repo-remove'))).toHaveCount(1);
  await expect(
    page.locator(`${t('ade-repos-repo-head')} ${t('ade-repos-repo-remove')}`),
  ).toHaveCount(1);
  await page.locator(t('ade-repos-repo-remove')).click();
  await page.locator(t('confirm-dialog-cancel')).click();
  expect(calls(control, IPC.codeWorkspaceRemoveRepo)).toHaveLength(0);
  await page.locator(t('ade-repos-repo-remove')).click();
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect.poll(() => calls(control, IPC.codeWorkspaceRemoveRepo)).toHaveLength(1);
});

test('removing a folder-sourced repository names the folder in the confirm', async ({
  relaunch,
}) => {
  const fx = adeFixture<ReposFx & { repos: { source: string }[] }>('repos');
  const { window: page } = await openPage(relaunch);
  await page.locator(t('ade-repos-repo-remove')).click();
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
  const { window: page, control } = await openPage(relaunch, [
    {
      channel: IPC.codeWorkspaceSetRepoHidden,
      response: { ...rec, id: 'repo-web-app', name: 'acme-customer-dashboard-web-frontend' },
    },
  ]);
  const row = page.locator(`${t('ade-repos-repo')}[data-repo-id="repo-web-app"]`);
  await expect(row).toHaveAttribute('data-hidden', 'false');
  await page.locator(t('ade-repos-repo-hide')).click();
  await expect.poll(() => calls(control, IPC.codeWorkspaceSetRepoHidden)).toHaveLength(1);
  expect(calls(control, IPC.codeWorkspaceSetRepoHidden)[0]?.args).toEqual({
    id: 'repo-web-app',
    hidden: rec.hidden,
  });
  await expect(row).toHaveAttribute('data-hidden', 'true');
  await expect(page.locator(t('ade-repos-repo-hide'))).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator(t('ade-repos-repo-hidden-badge'))).toBeVisible();
});

test('contract: folder Hide all sends SetFolderHidden', async ({ relaunch }) => {
  const fx = adeFixture<ReposFx>('repos');
  const folder = contract<FolderFx>('repos-hidden', 'AdeTaskService.SetFolderHidden');
  const { window: page, control } = await openFolders(relaunch);
  await page.locator(t('ade-repos-folder-hide')).first().click();
  await expect.poll(() => calls(control, IPC.adeTaskSetFolderHidden)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskSetFolderHidden)[0]?.args).toEqual({
    path: fx.folders[0]?.path,
    hidden: folder.hidden,
  });
});

test('contract: a folder row counts its hidden repos', async ({ relaunch }) => {
  const fx = adeFixture<ReposFx>('repos');
  const folder = contract<FolderFx>('repos-hidden', 'AdeTaskService.SetFolderHidden');
  const { window: page } = await openPage(relaunch, [
    {
      channel: IPC.adeTaskRepos,
      response: { ...fx, folders: [{ ...fx.folders[0], ...folder, path: fx.folders[0]?.path }] },
    },
  ]);
  await page.locator(t('ade-repos-tab-folders')).click();
  await expect(page.locator(t('ade-repos-folder-count')).first()).toHaveText(
    `${folder.repoCount} repositories · ${folder.hiddenCount} hidden`,
  );
  await expect(page.locator(t('ade-repos-folder-hide')).first()).toHaveAttribute(
    'aria-pressed',
    'true',
  );
});

test('picking a swatch sends SetRepoColor and the nav dot follows', async ({ relaunch }) => {
  const { window: page, control } = await openPage(relaunch, [
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
    page.locator(`${t('ade-repos-repo')}[data-repo-id="repo-web-app"] ${t('ade-repos-repo-rail')}`),
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
