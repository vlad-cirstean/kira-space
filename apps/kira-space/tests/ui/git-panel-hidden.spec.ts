import type { Page } from '@playwright/test';
import { type RepoSummary, repoSummarySchema } from '@shared/domain/repo';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// P259: hidden repos leave the Git panel's Repos list; Show hidden brings them back dimmed. Backend
// half: repoflow TestHideRepo.

const t = (id: string) => `[data-testid="${id}"]`;

function record(id: string, name: string, order: number, hidden: boolean) {
  return {
    id,
    name,
    root: `/tmp/${name}`,
    repoId: `/tmp/${name}`,
    sortOrder: order,
    color: 'none',
    createdAt: '2026-01-01T00:00:00.000Z',
    hidden,
  };
}

const SHOWN = record('repo-shown', 'shown-repo', 1, false);
const HIDDEN = record('repo-hidden', 'hidden-repo', 2, true);

function listRepos(...records: ReturnType<typeof record>[]): ControlSnapshot {
  return { channel: IPC.codeWorkspaceListRepos, response: records };
}

const repoRow = (page: Page, id: string) => page.locator(`${t('repo-row')}[data-repo-id="${id}"]`);

test('a hidden repo is absent until Show hidden reveals it, marked', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: [listRepos(SHOWN, HIDDEN)] });
  await expect(repoRow(page, SHOWN.id)).toBeVisible();
  await expect(repoRow(page, HIDDEN.id)).toHaveCount(0);

  await page.locator(t('repos-show-hidden')).click();
  await expect(page.locator(t('repos-show-hidden'))).toHaveAttribute('aria-pressed', 'true');
  await expect(repoRow(page, HIDDEN.id)).toBeVisible();
  await expect(repoRow(page, HIDDEN.id).locator(t('repo-hidden-mark'))).toBeVisible();
  await expect(repoRow(page, SHOWN.id).locator(t('repo-hidden-mark'))).toHaveCount(0);
});

test('the Show hidden toggle is absent while nothing is hidden', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: [listRepos(SHOWN)] });
  await expect(repoRow(page, SHOWN.id)).toBeVisible();
  await expect(page.locator(t('repos-show-hidden'))).toHaveCount(0);
});

test('contract: menu Hide sends SetRepoHidden and closes the open workspace', async ({
  relaunch,
}) => {
  const rec = contract<RepoSummary>('repos-hidden', 'CodeWorkspaceService.SetRepoHidden', {
    schema: repoSummarySchema,
  });
  const { window: page, control } = await relaunch({
    control: [
      listRepos(SHOWN, HIDDEN),
      {
        channel: IPC.codeWorkspaceSetRepoHidden,
        response: { ...SHOWN, hidden: rec.hidden },
      },
    ],
  });
  await repoRow(page, SHOWN.id).click();
  await expect(page.locator(t('tab-strip-wrapper'))).toBeVisible();
  await page.locator(t('git-panel-tab-repos')).click();
  await repoRow(page, SHOWN.id).click({ button: 'right' });
  await page.locator(t('menu-item-hide')).click();

  await expect.poll(() => calls(control, IPC.codeWorkspaceSetRepoHidden)).toHaveLength(1);
  expect(calls(control, IPC.codeWorkspaceSetRepoHidden)[0]?.args).toEqual({
    id: SHOWN.id,
    hidden: rec.hidden,
  });
  await expect(repoRow(page, SHOWN.id)).toHaveCount(0);
  await expect(page.locator(t('repos-all-hidden'))).toBeVisible();
  await expect(page.locator(`${t('tab-strip-wrapper')} ${t('tab')}`)).toHaveCount(0);
});

test('menu Show sends hidden: false', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [
      listRepos(SHOWN, HIDDEN),
      { channel: IPC.codeWorkspaceSetRepoHidden, response: { ...HIDDEN, hidden: false } },
    ],
  });
  await page.locator(t('repos-show-hidden')).click();
  await repoRow(page, HIDDEN.id).click({ button: 'right' });
  await page.locator(t('menu-item-show')).click();
  await expect.poll(() => calls(control, IPC.codeWorkspaceSetRepoHidden)).toHaveLength(1);
  expect(calls(control, IPC.codeWorkspaceSetRepoHidden)[0]?.args).toEqual({
    id: HIDDEN.id,
    hidden: false,
  });
  await expect(repoRow(page, HIDDEN.id).locator(t('repo-hidden-mark'))).toHaveCount(0);
});

test('menu Remove asks first', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [listRepos(SHOWN), { channel: IPC.codeWorkspaceRemoveRepo }],
  });
  await repoRow(page, SHOWN.id).click({ button: 'right' });
  await page.locator(t('menu-item-remove')).click();
  await page.locator(t('confirm-dialog-close')).click();
  expect(calls(control, IPC.codeWorkspaceRemoveRepo)).toHaveLength(0);
  await repoRow(page, SHOWN.id).click({ button: 'right' });
  await page.locator(t('menu-item-remove')).click();
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect.poll(() => calls(control, IPC.codeWorkspaceRemoveRepo)).toHaveLength(1);
});

test('RepoHeads never asks for a hidden repo while Show hidden is off', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: [listRepos(SHOWN, HIDDEN)] });
  await expect(repoRow(page, SHOWN.id)).toBeVisible();
  await expect.poll(() => calls(control, IPC.codeWorkspaceRepoHeads).length).toBeGreaterThan(0);
  const asked = () =>
    calls(control, IPC.codeWorkspaceRepoHeads).flatMap((e) => (e.args as { ids: string[] }).ids);
  expect(asked()).not.toContain(HIDDEN.id);
  expect(asked()).toContain(SHOWN.id);

  await page.locator(t('repos-show-hidden')).click();
  await expect.poll(asked).toContain(HIDDEN.id);
});

test('all repos hidden shows the empty state without a button; the header toggle shows them', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: [listRepos(HIDDEN)] });
  await expect(page.locator(t('repos-all-hidden'))).toContainText('All repositories are hidden');
  await expect(page.locator(t('repo-row'))).toHaveCount(0);
  expect(calls(control, IPC.codeWorkspaceRepoHeads)).toHaveLength(0);
  await expect(page.locator(t('repos-all-hidden')).locator('button')).toHaveCount(0);
  await page.locator(t('repos-show-hidden')).click();
  await expect(repoRow(page, HIDDEN.id)).toBeVisible();
});
