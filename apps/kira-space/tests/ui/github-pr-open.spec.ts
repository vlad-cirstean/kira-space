import { expect } from '@playwright/test';
import { contract } from './support/contract';
import { calls, hostViewControl, t, hostViewTest as test, type Wire } from './support/hostView';
import { IPC } from './support/ipcChannels';

// Frontend half of githostflow TestOpenGitHubPullRequest: the Go flow opens a GitHub PR through the
// real provider and writes tests/contract/github-pr-open.json; this spec renders the same target and
// detail, and "Open on GitHub" sends the same URL.

const SCENARIO = 'github-pr-open';
type Target = Parameters<typeof hostViewControl>[0];
const target = contract<Target>(SCENARIO, 'HostViewService.WindowTarget');
const detail = contract<Wire>(SCENARIO, 'HostViewService.ChangeRequest');
const openArgs = contract<{ url: string }>(SCENARIO, 'args:GitHostService.OpenChangeRequestURL');
const OPEN = { channel: IPC.gitHostOpenChangeRequestUrl, response: null };

test('contract: an opened GitHub PR renders with its team reviewer, no failure', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: hostViewControl(target, detail, [OPEN]) });
  await expect(page.locator(t('host-view-title'))).toHaveText('Add login rate limit');
  await expect(page.locator(t('host-view-body'))).toBeVisible();
  await expect(page.locator(t('host-view-failure'))).toHaveCount(0);
  await expect(page.locator(t('host-view-reviewer')).filter({ hasText: 'Platform' })).toHaveCount(
    1,
  );
});

test('contract: Open on GitHub sends the PR URL', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: hostViewControl(target, detail, [OPEN]),
  });
  await page.locator(t('host-view-open-host')).click();
  await expect.poll(() => calls(control, IPC.gitHostOpenChangeRequestUrl)).toHaveLength(1);
  expect(calls(control, IPC.gitHostOpenChangeRequestUrl)[0]?.args).toEqual(openArgs);
});
