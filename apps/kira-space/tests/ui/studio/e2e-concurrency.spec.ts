import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { calls } from '../support/ipcCalls';
import { IPC } from '../support/ipcChannels';
import { expectNoLeak } from './support/e2eLeak';

// Contract e2e-concurrency. Backend half: dbmcpflow TestE2EConcurrency. Five cards from three clients queue
// in arrival order: the dialog shows them one at a time, oldest first, and the card that needs a typed value
// blocks Approve until it has one. Deny all sends every id. The backend-only steps (script slots, the commit
// race, a mixed Answer for everything) have no UI surface: the dialog has no approve-all.

const SC = 'e2e-concurrency';
const TYPED = 'e13-typed-value-4K8';
const held = (round: 'five' | 'all') => ({
  channel: IPC.dbMcpPendingResponse,
  response: contract(SC, `DbMcpService.PendingResponse#${round}`),
});
const list = (round: 'five' | 'all') =>
  contract<ApprovalItem[]>(SC, `ApprovalsService.List#${round}`);

/** The queue as the dialog receives it: kept times live, creation order as recorded. */
function queue(round: 'five' | 'all'): ApprovalItem[] {
  const base = Date.now();
  return list(round).map((it, i) => {
    const payload = structuredClone(it.payload) as { kept?: Record<string, unknown>[] };
    if (payload.kept) {
      payload.kept = payload.kept.map((k) => ({
        ...k,
        createdAt: new Date(base - 60_000).toISOString(),
        expiresAt: new Date(base + 59 * 60_000).toISOString(),
      }));
    }
    return liveItem({ ...it, payload }, 600, base + i);
  });
}

const dialog = (page: import('@playwright/test').Page) =>
  page.locator('[data-testid="approvals-dialog"]');

test('contract: five cards show one at a time in arrival order and the typed-value card blocks Approve', async ({
  relaunch,
}) => {
  const items = queue('five');
  const ids = items.map((i) => i.id);
  const { window: page, control } = await relaunch({
    control: [held('five'), { channel: IPC.approvalsAnswer, response: answeredOk(ids) }],
  });
  const logs: string[] = [];
  page.on('console', (m) => logs.push(m.text()));
  // Pushed newest first: the dialog still lists by creation time.
  await pushApprovals(page, [...items].reverse());
  for (const [i, it] of items.entries()) {
    const left = items.slice(i);
    if (left.length > 1) {
      await expect(dialog(page).locator('[data-testid="approvals-position"]')).toHaveText(
        `1 of ${left.length}`,
      );
    }
    await expect(dialog(page).locator(`[data-testid="approval-item-${it.id}"]`)).toBeVisible();
    const approve = page.locator(`[data-testid="approval-approve-${it.id}"]`);
    if (i === 3) {
      await expect(approve).toBeDisabled();
      await page.locator('[data-testid="api-input-apikey"]').fill(TYPED);
      await expect(approve).toBeEnabled();
    }
    await approve.click();
    await expect.poll(() => calls(control, IPC.approvalsAnswer)).toHaveLength(i + 1);
    const sent = calls(control, IPC.approvalsAnswer)[i]?.args as {
      ids: string[];
      approve: boolean;
    };
    expect(sent.ids).toEqual([it.id]);
    expect(sent.approve).toBe(true);
    await pushApprovals(page, items.slice(i + 1));
  }
  await expect(dialog(page)).toHaveCount(0);
  await expectNoLeak(page, TYPED, logs);
});

test('contract: Deny all sends every id and never a typed value', async ({ relaunch }) => {
  const items = queue('all');
  const recorded = contract<{ ids: string[]; approve: boolean }>(
    SC,
    'args:ApprovalsService.Answer#all',
  );
  const { window: page, control } = await relaunch({
    control: [held('all'), { channel: IPC.approvalsAnswer, response: answeredOk(recorded.ids) }],
  });
  const logs: string[] = [];
  page.on('console', (m) => logs.push(m.text()));
  await pushApprovals(page, items);
  await expect(dialog(page).locator('[data-testid="approvals-position"]')).toHaveText('1 of 5');
  await page.click('[data-testid="approvals-deny-all"]');
  await expect
    .poll(() => {
      const args = calls(control, IPC.approvalsAnswer)[0]?.args as
        | { ids: string[]; approve: boolean }
        | undefined;
      return args && { ...args, ids: [...args.ids].sort() };
    })
    .toMatchObject({ ids: [...recorded.ids].sort(), approve: false });
  expect(JSON.stringify(calls(control, IPC.approvalsAnswer))).not.toContain(TYPED);
  await expectNoLeak(page, TYPED, logs);
});
