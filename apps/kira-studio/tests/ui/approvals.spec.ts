import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// Contract approvals-studio. Backend half: dbmcpflow TestApprovalsPerItem (three concurrent writes). One dialog holds every
// pending agent approval; X or Escape minimises to a toast and never answers.

const SCENARIO = 'approvals-studio';

function queue(n: number): ApprovalItem[] {
  const items = contract<ApprovalItem[]>(SCENARIO, 'ApprovalsService.List');
  if (items.length < n) throw new Error(`contract has ${items.length} items, want ${n}`);
  return items.slice(0, n).map((item, i) => liveItem(item, 600 - i * 60, Date.now() + i));
}

test('contract: X minimises to a toast without answering; Review reopens', async ({ relaunch }) => {
  const items = queue(2);
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: [] }],
  });

  await pushApprovals(page, items);
  const dialog = page.locator('[data-testid="approvals-dialog"]');
  const toast = page.locator('[data-testid="approvals-toast"]');
  await expect(dialog).toBeVisible();
  await expect(page.locator('[data-testid="approvals-count"]')).toHaveText('2');

  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
  await expect(toast).toBeVisible();
  await expect(page.locator('[data-testid="approvals-toast-count"]')).toHaveText(
    '2 things awaiting your approval',
  );
  await expect(page.locator('[data-testid="approvals-toast-expires"]')).toContainText(
    'Next expires in',
  );
  expect(control.log().some((e) => e.channel === IPC.approvalsAnswer)).toBe(false);

  await pushApprovals(page, queue(3));
  await expect(dialog).toHaveCount(0);
  await expect(page.locator('[data-testid="approvals-toast-count"]')).toHaveText(
    '3 things awaiting your approval',
  );

  await page.click('[data-testid="approvals-toast-review"]');
  await expect(dialog).toBeVisible();
  await expect(toast).toHaveCount(0);
});

test('contract: Deny all and Approve all answer every visible id', async ({ relaunch }) => {
  const items = queue(2);
  const ids = items.map((i) => i.id);
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(ids) }],
  });

  await pushApprovals(page, items);
  await page.click('[data-testid="approvals-deny-all"]');
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids, approve: false, options: null });

  await pushApprovals(page, []);
  await expect(page.locator('[data-testid="approvals-dialog"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="approvals-toast"]')).toHaveCount(0);
});

test('contract: an item answered by Approve shows a refusal inline', async ({ relaunch }) => {
  const [item] = queue(1);
  if (!item) throw new Error('no item');
  const { window: page } = await relaunch({
    control: [
      {
        channel: IPC.approvalsAnswer,
        response: [{ id: item.id, code: 'E_CONFLICT', error: 'Already answered elsewhere' }],
      },
    ],
  });

  await pushApprovals(page, [item]);
  await page.click(`[data-testid="approval-approve-${item.id}"]`);
  await expect(page.locator('[data-testid="approval-error"]')).toHaveText(
    'Already answered elsewhere',
  );
});

test('an item of an unknown kind offers Deny only', async ({ relaunch }) => {
  const [item] = queue(1);
  if (!item) throw new Error('no item');
  const { window: page } = await relaunch({ control: [] });
  const unknown = { ...item, kind: 'future.kind' };
  await pushApprovals(page, [unknown]);
  await expect(page.locator(`[data-testid="approval-deny-${unknown.id}"]`)).toBeVisible();
  await expect(page.locator(`[data-testid="approval-approve-${unknown.id}"]`)).toHaveCount(0);
});

test('contract: a per-item Deny answers only that id', async ({ relaunch }) => {
  const items = queue(3);
  const answer = contract<{ ids: string[] }>(SCENARIO, 'args:ApprovalsService.Answer');
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(answer.ids) }],
  });

  await pushApprovals(page, items);
  await page.click(`[data-testid="approval-deny-${answer.ids[0]}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ...answer, options: null });
});
