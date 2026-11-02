import { expect } from '@playwright/test';
import {
  calls,
  issueControl,
  issueFetchFresh,
  issueFetchStale,
  issueTarget,
  t,
  hostViewTest as test,
} from './support/hostView';
import { IPC } from './support/ipcChannels';

// UI half of issueview_fetch_test.go: a saved copy arrives marked stale and the window asks for a
// fresh read at once; Refresh asks for a fresh read too.

const REF = issueTarget.url;
const PLAIN = { channel: IPC.hostViewIssue, args: { ref: REF, fresh: false } };
const FRESH = { channel: IPC.hostViewIssue, args: { ref: REF, fresh: true } };

test('a stale copy is labeled until the fresh read replaces it', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: issueControl([
      { ...PLAIN, response: issueFetchStale },
      { ...FRESH, response: issueFetchFresh, hold: true },
    ]),
  });
  await expect(page.locator(t('host-view-title'))).toHaveText('Export report as CSV');
  await expect(page.locator(t('issue-stale'))).toBeVisible();
  await expect
    .poll(() => calls(control, IPC.hostViewIssue).map((c) => c.args))
    .toEqual([
      { ref: REF, fresh: false },
      { ref: REF, fresh: true },
    ]);
  control.release(IPC.hostViewIssue);
  await expect(page.locator(t('host-view-title'))).toHaveText('Export report as CSV v2');
  await expect(page.locator(t('issue-stale'))).toHaveCount(0);
});

test('Refresh asks for a fresh read', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: issueControl([
      { ...PLAIN, response: issueFetchFresh },
      { ...FRESH, response: issueFetchFresh },
    ]),
  });
  await expect(page.locator(t('host-view-title'))).toBeVisible();
  expect(calls(control, IPC.hostViewIssue)).toHaveLength(1);
  await page.locator(t('host-view-refresh')).click();
  await expect.poll(() => calls(control, IPC.hostViewIssue)).toHaveLength(2);
  expect(calls(control, IPC.hostViewIssue)[1]?.args).toEqual({ ref: REF, fresh: true });
});
