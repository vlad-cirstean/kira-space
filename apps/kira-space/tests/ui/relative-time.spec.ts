import { expect } from '@playwright/test';
import { issueControl, issueOk, t, hostViewTest as test } from './support/hostView';

// Renderer-only: no value crosses IPC. The issue sidebar renders createdAt and updatedAt through
// the shared RelativeTime component; the page clock drives the ticks.

const CREATED = Date.UTC(2026, 0, 15, 9, 0, 0);
const NOW = CREATED + 30_000;
const MINUTE = 60_000;

const issue = {
  ...issueOk,
  detail: {
    ...issueOk.detail,
    createdAt: CREATED,
    // 5m10s ahead of NOW: floors to "in 5m".
    updatedAt: NOW + 5 * MINUTE + 10_000,
  },
};

test('a relative time ticks from "just now" to "1m ago" and shows the absolute time on hover', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: issueControl(issue),
    clockTime: NOW,
    timezoneId: 'UTC',
    locale: 'en-US',
  });
  const created = page.locator(t('issue-side-created'));
  await expect(created).toHaveText('just now');
  await page.clock.runFor(30_000);
  await expect(created).toHaveText('1m ago');
  await expect(created.locator('time')).toHaveAttribute(
    'datetime',
    new Date(CREATED).toISOString(),
  );

  await created.locator('time').hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(
    /Jan 15, 2026, 9:00\sAM/,
  );
});

test('a future instant reads "in 5m"', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: issueControl(issue),
    clockTime: NOW,
    timezoneId: 'UTC',
    locale: 'en-US',
  });
  await expect(page.locator(t('issue-side-updated'))).toHaveText('in 5m');
});
