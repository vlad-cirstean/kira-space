import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';

// Contract approvals-api-kept-script. Backend half: dbmcpflow TestKeptVarsFromScript. A script that reads
// one kept variable in prepare and keeps another in transform: the card lists the read, marks both calls
// in the scripts and names what the transform may keep.

const base = () =>
  contract<ApprovalItem[]>('approvals-api-kept-script', 'ApprovalsService.List')[0] as ApprovalItem;
const card = (page: Page, it: ApprovalItem) =>
  page.locator(`[data-testid="approval-item-${it.id}"]`);

/** The recorded item with real kept times: created 5 minutes ago, 55 minutes left. */
function timed(it: ApprovalItem) {
  const payload = structuredClone(it.payload) as { kept: { [k: string]: unknown }[] };
  payload.kept = payload.kept.map((k) => ({
    ...k,
    createdAt: new Date(Date.now() - 5 * 60_000).toISOString(),
    expiresAt: new Date(Date.now() + 55 * 60_000).toISOString(),
  }));
  return liveItem({ ...it, payload });
}

test('contract: the card lists the script read and where the prepare script makes it', async ({
  relaunch,
}) => {
  const it = timed(base());
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const row = card(page, it).locator('[data-testid="api-approval-kept-region"]');
  await expect(row).toContainText('Text');
  await expect(row.locator('[data-testid="kept-value-preview"]')).toHaveText('eu');
  await expect(row.locator('[data-testid="kept-used-in"]')).toHaveText('Prepare script');
  await expect(card(page, it).locator('[data-testid="api-approval-kept"]')).toHaveText(
    'Uses kept: region',
  );
  // The read is in prepare: the prepare block marks it, the transform block has no reads note.
  await expect(card(page, it).locator('[data-testid="api-approval-script-reads"]')).toHaveCount(0);
});

test('contract: the kept.get and kept.set calls are marked in their scripts', async ({
  relaunch,
}) => {
  const it = timed(base());
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const c = card(page, it);
  await expect(c.locator('[data-testid="api-approval-script-prepare"] .kira-ed-kept')).toHaveCount(
    1,
  );
  await expect(
    c.locator('[data-testid="api-approval-script-transform"] .kira-ed-kept'),
  ).toHaveCount(1);
});

test('contract: the card names what the transform may keep', async ({ relaunch }) => {
  const it = timed(base());
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const c = card(page, it);
  await expect(c.locator('[data-testid="api-approval-may-keep"]')).toHaveText(
    /This script may keep:\s*idsPlain/,
  );
  await expect(c.locator('[data-testid="api-approval-kept-writes"]')).toHaveText('May keep: ids');
});

test('a plain request shows none of the script kept notes', async ({ relaunch }) => {
  const it = base();
  const payload = structuredClone(it.payload) as {
    kept: unknown[];
    script: { transform: { mayKeep: unknown[] } };
  };
  payload.kept = [];
  payload.script.transform.mayKeep = [];
  const live = liveItem({ ...it, payload });
  const { window: page } = await relaunch({});
  await pushApprovals(page, [live]);
  const c = card(page, live);
  await expect(c.locator('[data-testid="api-approval-may-keep"]')).toHaveCount(0);
  await expect(c.locator('[data-testid="api-approval-script-reads"]')).toHaveCount(0);
  await expect(c.locator('[data-testid="api-approval-kept-panel"]')).toHaveCount(0);
});
