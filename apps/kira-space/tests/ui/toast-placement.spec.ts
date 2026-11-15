import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, test } from './fixtures';
import { adeFixture, FIXED_NOW, openPlan, openTask } from './support/adeV2';
import { seedApprovals } from './support/approvals';
import { pick } from './support/branchMenu';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// UI only: placement is frontend-only, no backend half. Toasts and corner chips share one
// top-right column: chips first, Sonner toasts below, 8px under the title bar, 16px from the edge.

const t = (id: string) => `[data-testid="${id}"]`;
const GAP = 8;
const EDGE = 16;
const SHA = 'a1b2c3d4e5f60718293a4b5c6d7e8f9012345678';

type Box = { x: number; y: number; width: number; height: number };

const dirty = { channel: IPC.adeTaskTaskBranchRisk, response: adeFixture('branch-risk-dirty') };
const syncToast: ControlSnapshot = {
  channel: IPC.configSyncStatus,
  response: {
    ...contract<Record<string, unknown>>('config-sync', 'ConfigSyncService.Status'),
    url: 'https://example.com/team/kira-config.git',
    fetched: SHA,
    fetchedAt: '2026-01-01T10:00:00Z',
    appliedAt: '',
    showToast: true,
  },
};

function item(): ApprovalItem {
  const [it] = contract<ApprovalItem[]>('approvals-space', 'ApprovalsService.List');
  if (!it) throw new Error('contract has no item');
  return liveItem(it, 1800, FIXED_NOW);
}

async function box(page: Page, selector: string): Promise<Box> {
  const b = await page.locator(selector).first().boundingBox();
  if (!b) throw new Error(`no box for ${selector}`);
  return b;
}

// Sonner's enter animation moves the toast; wait until its box stops changing.
async function settledToast(page: Page): Promise<Box> {
  let prev = '';
  await expect
    .poll(async () => {
      const cur = JSON.stringify(await box(page, '[data-sonner-toast]'));
      const same = cur === prev;
      prev = cur;
      return same;
    })
    .toBe(true);
  return box(page, '[data-sonner-toast]');
}

const titlebarBottom = async (page: Page) => {
  const b = await page.locator(t('titlebar-app-title')).locator('..').boundingBox();
  if (!b) throw new Error('no title bar');
  return b.y + b.height;
};

// Removing a branch holds, so the loading toast stays on screen.
async function holdToast(page: Page) {
  await openTask(page, 'T_cart');
  await pick(page, 'b_cart', 'ade-task-branch-remove');
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect(page.locator('[data-sonner-toast]')).toContainText('Removing');
}

const holdControl: ControlSnapshot[] = [
  dirty,
  { channel: IPC.adeTaskRemoveTaskBranch, hold: true },
];

const overlap = (a: Box, b: Box) =>
  a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height;

test('a toast anchors top right under the title bar', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, holdControl);
  await holdToast(page);
  const toaster = page.locator('[data-sonner-toaster]');
  await expect(toaster).toHaveAttribute('data-y-position', 'top');
  await expect(toaster).toHaveAttribute('data-x-position', 'right');
  const vp = page.viewportSize();
  const b = await settledToast(page);
  expect(Math.abs(b.y - ((await titlebarBottom(page)) + GAP))).toBeLessThanOrEqual(1);
  expect(Math.abs(b.x + b.width - ((vp?.width ?? 0) - EDGE))).toBeLessThanOrEqual(1);
});

test('chips and toasts stack in one column and follow chip removal', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    ...holdControl,
    ...seedApprovals([item()]),
    syncToast,
  ]);
  await expect(page.locator(t('approvals-dialog'))).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator(t('approvals-toast'))).toBeVisible();
  await expect(page.locator(t('config-sync-toast'))).toBeVisible();
  await holdToast(page);

  const vp = page.viewportSize();
  const tb = await titlebarBottom(page);
  const approvals = await box(page, t('approvals-toast'));
  const sync = await box(page, t('config-sync-toast'));
  const sonner = await settledToast(page);
  expect(Math.abs(approvals.y - (tb + GAP))).toBeLessThanOrEqual(1);
  expect(sync.y).toBeGreaterThanOrEqual(approvals.y + approvals.height + GAP - 1);
  expect(sonner.y).toBeGreaterThanOrEqual(sync.y + sync.height + GAP - 1);
  for (const b of [approvals, sync, sonner]) {
    expect(Math.abs(b.x + b.width - ((vp?.width ?? 0) - EDGE))).toBeLessThanOrEqual(1);
  }
  expect(overlap(approvals, sync)).toBe(false);
  expect(overlap(sync, sonner)).toBe(false);
  expect(overlap(approvals, sonner)).toBe(false);

  await pushApprovals(page, []);
  await expect(page.locator(t('approvals-toast'))).toHaveCount(0);
  await expect
    .poll(async () => (await box(page, '[data-sonner-toast]')).y)
    .toBeLessThanOrEqual(sync.y + sync.height + GAP + 1);
});

test('a narrow window keeps the toast under the title bar and inside the viewport', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch, holdControl);
  await page.setViewportSize({ width: 520, height: 800 });
  await holdToast(page);
  const min = (await titlebarBottom(page)) + GAP - 1;
  // Enter animation settles first.
  await expect
    .poll(async () => (await box(page, '[data-sonner-toast]')).y)
    .toBeGreaterThanOrEqual(min);
  const b = await box(page, '[data-sonner-toast]');
  expect(b.x).toBeGreaterThanOrEqual(0);
  expect(b.x + b.width).toBeLessThanOrEqual(520);
});

test('clicks pass through the column outside the chips', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [...seedApprovals([item()]), syncToast]);
  await expect(page.locator(t('approvals-dialog'))).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator(t('approvals-toast'))).toBeVisible();
  await expect(page.locator(t('config-sync-toast'))).toBeVisible();
  const stack = await box(page, t('corner-stack'));
  const approvals = await box(page, t('approvals-toast'));
  const sync = await box(page, t('config-sync-toast'));
  // Gap strip between the two chips.
  const y = Math.min(approvals.y + approvals.height, sync.y) + GAP / 2;
  const x = stack.x + stack.width / 2;
  const inside = await page.evaluate(
    ([px, py]) =>
      !!document.elementFromPoint(px ?? 0, py ?? 0)?.closest('[data-testid="corner-stack"]'),
    [x, y],
  );
  expect(inside).toBe(false);
});
