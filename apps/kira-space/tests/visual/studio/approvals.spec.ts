import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../../ui/fixtures';
import { withTimes } from '../../ui/studio/support/keptTimes';
import { contract } from '../../ui/support/contract';
import { IPC } from '../../ui/support/ipcChannels';
import { settleFonts } from './support/settle';

// P520: the approvals dialog with three queued writes, then minimised to the toast.

test('approvals dialog and toast', async ({ relaunch }) => {
  const items = contract<ApprovalItem[]>('approvals-studio', 'ApprovalsService.List').map(
    (item, i) => liveItem(item, 600, Date.now() + i),
  );
  const { window: page } = await relaunch({ control: [] });
  await page.setViewportSize({ width: 1400, height: 900 });
  await pushApprovals(page, items);
  const dialog = page.locator('[data-testid="approvals-dialog"]');
  await expect(dialog).toBeVisible();
  await expect(page.locator('[data-testid="db-mcp-approval-statement"]').first()).toBeVisible();
  await settleFonts(page);
  await expect(dialog).toHaveScreenshot('approvals-dialog.png', {
    mask: [page.locator('[data-testid="approval-countdown"]')],
  });

  await page.keyboard.press('Shift+Escape');
  const toast = page.locator('[data-testid="approvals-toast"]');
  await expect(toast).toBeVisible();
  await settleFonts(page);
  await expect(toast).toHaveScreenshot('approvals-toast.png', {
    mask: [page.locator('[data-testid="approvals-toast-expires"]')],
  });
});

// P728: API request with one value still needed and one stored, and the Kept for line.

test('api approval with a needed and a stored value row', async ({ relaunch }) => {
  const [stored] = contract<ApprovalItem[]>('e2e-user-value', 'ApprovalsService.List#stored');
  const base = withTimes(stored as ApprovalItem);
  const payload = structuredClone(base.payload) as { inputs: Record<string, unknown>[] };
  payload.inputs = [
    {
      name: 'otp',
      label: 'One-time code',
      state: 'needed',
      origin: 'http://127.0.0.1:1',
      keepMinutes: 60,
    },
    ...payload.inputs,
  ];
  const { window: page } = await relaunch({ control: [] });
  await page.setViewportSize({ width: 1400, height: 900 });
  await pushApprovals(page, [liveItem({ ...base, payload }, 600, Date.now())]);
  const dialog = page.locator('[data-testid="approvals-dialog"]');
  await expect(dialog).toBeVisible();
  await expect(page.locator('[data-testid="api-input-otp"]')).toBeVisible();
  await settleFonts(page);
  await expect(dialog).toHaveScreenshot('approvals-api-input.png', {
    mask: [
      page.locator('[data-testid="approval-countdown"]'),
      page.locator('[data-testid="kept-expiry"]'),
    ],
  });
});

// P718: kube reads with scripts. Contract e2e-kube-script; behaviour in ui/studio/e2e-kube-script.spec.ts.

const KUBE = 'e2e-kube-script';
const kubeItem = (key: string) =>
  contract<ApprovalItem[]>(KUBE, `ApprovalsService.List#${key}`)[0] as ApprovalItem;
const countdown = (page: import('@playwright/test').Page) => [
  page.locator('[data-testid="approval-countdown"]'),
];

test('kube read with prepare and transform scripts', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: [] });
  await page.setViewportSize({ width: 1400, height: 900 });
  const it = withTimes(kubeItem('logs'));
  await pushApprovals(page, [liveItem(it, 600, Date.now())]);
  await expect(page.locator('[data-testid="kube-approval-script-prepare"]')).toBeVisible();
  await settleFonts(page);
  await expect(page.locator('[data-testid="approvals-dialog"]')).toHaveScreenshot(
    'approvals-kube-read-script.png',
    { mask: [...countdown(page), page.locator('[data-testid="kept-expiry"]')] },
  );
});

test('kube list read whose transform keeps a plain and a secret value', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: [] });
  await page.setViewportSize({ width: 1400, height: 900 });
  await pushApprovals(page, [liveItem(kubeItem('list'), 600, Date.now())]);
  await expect(page.locator('[data-testid="kube-approval-script-transform"]')).toBeVisible();
  await settleFonts(page);
  await expect(page.locator('[data-testid="approvals-dialog"]')).toHaveScreenshot(
    'approvals-kube-read-list.png',
    { mask: countdown(page) },
  );
});

test('kube scripted review: envelope, raw table and Will keep', async ({ relaunch }) => {
  const it = liveItem(kubeItem('review'), 600, Date.now());
  const held = contract<Record<string, unknown>>(KUBE, 'DbMcpService.PendingResponse#review');
  const { window: page } = await relaunch({
    control: [{ channel: IPC.dbMcpPendingResponse, response: held }],
  });
  await page.setViewportSize({ width: 1400, height: 900 });
  await pushApprovals(page, [it]);
  await expect(page.locator('[data-testid="kube-review-keep"]')).toBeVisible();
  await settleFonts(page);
  await expect(page.locator('[data-testid="approvals-dialog"]')).toHaveScreenshot(
    'approvals-kube-review-script.png',
    { mask: countdown(page) },
  );
});
