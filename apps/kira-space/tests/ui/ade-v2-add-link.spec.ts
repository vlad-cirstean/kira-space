import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeFixture, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Paste a PR or branch link in Add > Existing branch. Backend half: adeflow TestHostLink
// (contract ade-host-link).

const t = (id: string) => `[data-testid="${id}"]`;

interface Target {
  branch: string;
  [key: string]: unknown;
}

const added = () => ({
  ...adeFixture<{ task: unknown }>('add-existing-branch'),
  task: { ...adeFixture<Record<string, unknown>>('task'), id: 'T_deps' },
});

async function openLinkTab(page: Page): Promise<void> {
  await page.locator(t('ade-add')).click();
  await page.locator(t('ade-add-tab-branch')).click();
}

test('contract: pasting a PR link resolves and adds it', async ({ relaunch }) => {
  const resolveArgs = contract<{ url: string }>(
    'ade-host-link',
    'args:AdeTaskService.ResolveHostLink',
  );
  const addArgs = contract<{ url: string }>('ade-host-link', 'args:AdeTaskService.AddHostLink');
  const target = contract<Target>('ade-host-link', 'AdeTaskService.ResolveHostLink#pr');
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskCandidateBranches, response: adeFixture('candidates') },
    { channel: IPC.adeTaskResolveHostLink, response: target },
    { channel: IPC.adeTaskAddHostLink, response: added() },
  ]);
  await openLinkTab(page);
  await page.locator(t('ade-branch-search')).fill(resolveArgs.url);
  await expect(page.locator(t('ade-link-target'))).toContainText(target.branch);
  await expect(page.locator(t('ade-link-target'))).toContainText('#11');
  // One line: the Add button sits level with the pill.
  const pillTop = (await page.locator(t('ade-link-target-pill')).boundingBox())?.y ?? -100;
  const addTop = (await page.locator(t('ade-link-add')).boundingBox())?.y ?? 100;
  expect(Math.abs(addTop - pillTop)).toBeLessThanOrEqual(4);
  expect(calls(control, IPC.adeTaskResolveHostLink)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskResolveHostLink)[0]?.args).toEqual(resolveArgs);
  await page.locator(t('ade-link-add')).click();
  await expect.poll(() => calls(control, IPC.adeTaskAddHostLink)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskAddHostLink)[0]?.args).toEqual({
    ...addArgs,
    url: resolveArgs.url,
  });
  await expect(page.locator(t('ade-add-popover'))).toHaveCount(0);
  await expect(page.locator('[data-testid="ade-task"][data-task-id="T_deps"]')).toHaveAttribute(
    'data-selected',
    'true',
  );
});

test('contract: a fork PR shows its owner', async ({ relaunch }) => {
  const fork = contract<Target>('ade-host-link', 'AdeTaskService.ResolveHostLink#fork');
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskCandidateBranches, response: adeFixture('candidates') },
    { channel: IPC.adeTaskResolveHostLink, response: fork },
  ]);
  await openLinkTab(page);
  await page.locator(t('ade-branch-search')).fill('https://github.com/acme/api/pull/12');
  await expect(page.locator(t('ade-link-fork'))).toContainText('forker');
  await expect(page.locator(t('ade-link-target'))).toContainText('forker/main');
});

test('a branch already on a task cannot be added again', async ({ relaunch }) => {
  const target = {
    ...contract<Target>('ade-host-link', 'AdeTaskService.ResolveHostLink#pr'),
    onTaskId: 'T_deps',
  };
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskCandidateBranches, response: adeFixture('candidates') },
    { channel: IPC.adeTaskResolveHostLink, response: target },
  ]);
  await openLinkTab(page);
  await page.locator(t('ade-branch-search')).fill('https://github.com/acme/api/pull/11');
  await expect(page.locator(t('ade-link-on-task'))).toBeVisible();
  await expect(page.locator(t('ade-link-add'))).toBeDisabled();
});

test('a resolve error shows and blocks add', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskCandidateBranches, response: adeFixture('candidates') },
    {
      channel: IPC.adeTaskResolveHostLink,
      error: { code: 'E_BAD_REQUEST', message: 'An issue link names no branch.' },
    },
  ]);
  await openLinkTab(page);
  await page.locator(t('ade-branch-search')).fill('https://github.com/acme/api/issues/5');
  await expect(page.locator(t('ade-link-error'))).toContainText('names no branch');
  await expect(page.locator(t('ade-link-add'))).toHaveCount(0);
});

test('plain text still filters candidates', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskCandidateBranches, response: adeFixture('candidates') },
  ]);
  await openLinkTab(page);
  await page.locator(t('ade-branch-search')).fill('invoice');
  await expect(page.locator('[data-testid^="ade-branch-option-"]')).toHaveCount(1);
  expect(calls(control, IPC.adeTaskResolveHostLink)).toHaveLength(0);
});

test('attach mode sends the task id', async ({ relaunch }) => {
  const target = contract<Target>('ade-host-link', 'AdeTaskService.ResolveHostLink#pr');
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskCandidateBranches, response: adeFixture('candidates') },
    { channel: IPC.adeTaskResolveHostLink, response: target },
    { channel: IPC.adeTaskAddHostLink, response: added() },
  ]);
  await page
    .locator('[data-testid="ade-card"][data-task-id="T_bill"] [data-testid="ade-card-head"]')
    .click();
  await expect(page.locator(t('ade-panel'))).toBeVisible();
  await page.locator(t('ade-add-branch')).click();
  const url = 'https://github.com/acme/api/pull/11';
  await page.locator(t('ade-branch-search')).fill(url);
  await page.locator(t('ade-link-add')).click();
  await expect.poll(() => calls(control, IPC.adeTaskAddHostLink)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskAddHostLink)[0]?.args).toEqual({ url, taskId: 'T_bill' });
});
