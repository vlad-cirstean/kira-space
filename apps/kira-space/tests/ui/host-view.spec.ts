import { expect } from '@playwright/test';
import {
  azureDetail,
  azureTarget,
  calls,
  githubDetail,
  githubTarget,
  gone,
  hostViewControl,
  t,
  hostViewTest as test,
  unavailable,
} from './support/hostView';
import { IPC } from './support/ipcChannels';

// UI half of hostview_test.go: the Go flows write tests/contract/host-view.json (target and detail
// per provider, unavailable, gone); this spec renders them in the host view window.

const OPEN = { channel: IPC.gitHostOpenChangeRequestUrl, response: null };
const LINK = { channel: IPC.linkOpenExternal, response: null };

test('GitHub detail renders header, markdown, checks, people and conversation', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: hostViewControl(githubTarget, githubDetail, [LINK]),
  });
  await expect(page.locator(t('host-view-title'))).toHaveText('Add login rate limit');
  await expect(page.locator(t('host-view-ref'))).toContainText('PR7');
  await expect(page.locator(t('host-view-state'))).toHaveText('Open');
  await expect(page.locator(t('host-view-branches'))).toHaveText('feature/login into main');
  await expect(page.locator(t('host-view-meta'))).toContainText('Alice A opened');

  const body = page.locator(t('host-view-body')).locator(`main > ${t('host-view-markdown')}`);
  await expect(body.locator('h1')).toHaveText('Summary');
  await expect(body).not.toContainText('template comment');
  await expect(body.locator('script')).toHaveCount(0);
  await expect(body).toContainText('alert(1)');
  await expect(body.locator('img')).toHaveCount(0);
  const image = body.locator('[data-kira-image]');
  await expect(image).toHaveText('diagram');
  await expect(image).toHaveAttribute('href', 'https://example.com/d.png');
  await image.click();
  await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(1);
  expect(calls(control, IPC.linkOpenExternal)[0]?.args).toEqual({
    url: 'https://example.com/d.png',
  });

  const checks = page.locator(t('host-view-check'));
  await expect(checks).toHaveCount(9);
  await expect(checks.nth(0)).toHaveAttribute('data-state', 'success');
  await expect(checks.nth(0)).toContainText('Required');
  await expect(checks.nth(1)).toHaveAttribute('data-state', 'failure');
  await expect(checks.nth(3)).toHaveAttribute('data-state', 'skipped');
  await expect(page.locator(t('host-view-checks-truncated'))).toBeVisible();

  const reviewers = page.locator(t('host-view-reviewer'));
  await expect(reviewers).toHaveCount(4);
  await expect(reviewers.nth(0)).toHaveAttribute('data-state', 'changesRequested');
  await expect(reviewers.nth(2)).toContainText('Platform');
  await expect(page.locator(t('host-view-label'))).toHaveText(['bug', 'odd']);
  await expect(page.locator(t('host-view-assignee'))).toHaveText('Bob B');
  await expect(page.locator(t('host-view-stats'))).toContainText('3 commits, 6 files, +120 -14');
  await expect(page.locator(t('host-view-mergeable'))).toHaveAttribute(
    'data-mergeable',
    'conflicting',
  );

  const threads = page.locator(t('host-view-thread'));
  await expect(threads).toHaveCount(6);
  await expect(threads.nth(0)).toContainText('Looks good.');
  await expect(threads.nth(1).locator(t('host-view-thread-review'))).toHaveText(
    'Changes requested',
  );
  await expect(threads.nth(2).locator(t('host-view-thread-path'))).toHaveText('auth/login.go:42');
  await expect(threads.nth(2).locator(t('host-view-comments-truncated'))).toBeVisible();
  await expect(page.locator(t('host-view-threads-truncated'))).toBeVisible();

  const resolved = threads.nth(3);
  await expect(resolved).toContainText('Resolved');
  await expect(resolved).toContainText('Outdated');
  await expect(resolved.locator(t('host-view-comment'))).toHaveCount(0);
  await resolved.locator(t('host-view-thread-toggle')).click();
  await expect(resolved.locator(t('host-view-comment'))).toContainText('Nit.');
});

test('Open on host and Copy link use the target URL', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: hostViewControl(githubTarget, githubDetail, [OPEN]),
  });
  await page.evaluate(() => {
    const w = window as unknown as { __clipboard: string[] };
    w.__clipboard = [];
    navigator.clipboard.writeText = async (text: string) => {
      w.__clipboard.push(text);
    };
  });
  await page.locator(t('host-view-open-host')).click();
  await expect.poll(() => calls(control, IPC.gitHostOpenChangeRequestUrl)).toHaveLength(1);
  expect(calls(control, IPC.gitHostOpenChangeRequestUrl)[0]?.args).toEqual({
    url: githubTarget.url,
  });
  await page.locator(t('host-view-copy')).click();
  await expect
    .poll(() =>
      page.evaluate(() => (window as unknown as { __clipboard: string[] }).__clipboard[0]),
    )
    .toBe(githubTarget.url);
});

test('Azure detail has no stats and lists build check and rejected reviewer', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: hostViewControl(azureTarget, azureDetail) });
  await expect(page.locator(t('host-view-title'))).toHaveText('PROJ-7 Add login');
  await expect(page.locator(t('host-view-stats'))).not.toContainText('commits');
  const check = page.locator(t('host-view-check'));
  await expect(check).toHaveCount(1);
  await expect(check.locator('button')).toHaveText('CI build');
  await expect(page.locator(t('host-view-reviewer')).nth(1)).toHaveAttribute(
    'data-state',
    'changesRequested',
  );
  await expect(page.locator(t('host-view-mergeable'))).toHaveAttribute(
    'data-mergeable',
    'mergeable',
  );
});

test('unavailable shows the reason and Retry asks again', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: hostViewControl(azureTarget, unavailable),
  });
  await expect(page.locator(t('host-view-failure-reason'))).toContainText('az login');
  expect(calls(control, IPC.hostViewChangeRequest)).toHaveLength(1);
  await page.locator(t('host-view-retry')).click();
  await expect.poll(() => calls(control, IPC.hostViewChangeRequest)).toHaveLength(2);
  await expect(page.locator(t('host-view-failure'))).toBeVisible();
});

test('gone shows the host reason', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: hostViewControl(azureTarget, gone) });
  await expect(page.locator(t('host-view-failure-reason'))).toHaveText(
    'not found, or you cannot see it',
  );
});

test('Refresh fetches the detail again', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: hostViewControl(azureTarget, azureDetail),
  });
  await expect(page.locator(t('host-view-title'))).toBeVisible();
  expect(calls(control, IPC.hostViewChangeRequest)).toHaveLength(1);
  await page.locator(t('host-view-refresh')).click();
  await expect.poll(() => calls(control, IPC.hostViewChangeRequest)).toHaveLength(2);
});
