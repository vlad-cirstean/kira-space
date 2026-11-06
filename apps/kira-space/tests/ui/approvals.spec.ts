import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest, test } from './fixtures';
import { FIXED_NOW, openPlan } from './support/adeV2';
import { seedApprovals } from './support/approvals';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Contract approvals-space. Backend half: flows/adeflow TestBranchRequestMerge. Concurrent branch calls
// of one task arrive as one approvals item with a group per call; the person picks per item.

const SCENARIO = 'approvals-space';
const t = (id: string) => `[data-testid="${id}"]`;

function merged(): ApprovalItem {
  const [it] = contract<ApprovalItem[]>(SCENARIO, 'ApprovalsService.List');
  if (!it) throw new Error('contract has no item');
  return liveItem(it, 1800, FIXED_NOW);
}

test('contract: three calls show as three groups and Approve sends the picked items', async ({
  relaunch,
}) => {
  const it = merged();
  const answer = contract<{ ids: string[]; options: Record<string, unknown> }>(
    SCENARIO,
    'args:ApprovalsService.Answer',
  );
  const { window: page, control } = await openPlan(relaunch, [
    ...seedApprovals([it]),
    { channel: IPC.approvalsAnswer, response: answeredOk(answer.ids) },
  ]);
  const dialog = page.locator(t('approvals-dialog'));
  await expect(dialog.locator(t('approvals-count'))).toHaveText('1');
  await expect(dialog.locator('[data-testid^="ade-branch-request-call-"]')).toHaveCount(3);

  // Drop feat/c (call 1) and also rename the remote of the rename in call 2.
  await dialog.locator(t('ade-branch-request-item-c1i0')).getByRole('checkbox').click();
  await dialog.locator(t('ade-branch-request-remote-c2i0')).click();
  await dialog.locator(t(`approval-approve-${it.id}`)).click();
  await expect.poll(() => calls(control, IPC.approvalsAnswer)).toHaveLength(1);
  expect(calls(control, IPC.approvalsAnswer)[0]?.args).toEqual({ ...answer });
});

test('a push that joins a call keeps the person’s choices on the item', async ({ relaunch }) => {
  const it = merged();
  const { window: page } = await openPlan(relaunch, seedApprovals([it]));
  const dialog = page.locator(t('approvals-dialog'));
  const box = dialog.locator(t('ade-branch-request-item-c1i0')).getByRole('checkbox');
  await box.click();
  await expect(box).not.toBeChecked();
  await pushApprovals(page, [{ ...it, expiresAt: it.expiresAt + 60_000 }]);
  await expect(box).not.toBeChecked();
});

studioTest.describe('databases module', () => {
  // Contract approvals-studio. Backend half: dbmcpflow TestApprovalsPerItem (three concurrent writes). One dialog holds every
  // pending agent approval; X or Escape minimises to a toast and never answers.

  const SCENARIO = 'approvals-studio';

  function queue(n: number): ApprovalItem[] {
    const items = contract<ApprovalItem[]>(SCENARIO, 'ApprovalsService.List');
    if (items.length < n) throw new Error(`contract has ${items.length} items, want ${n}`);
    return items.slice(0, n).map((item, i) => liveItem(item, 600 - i * 60, Date.now() + i));
  }

  studioTest(
    'contract: X minimises to a toast without answering; Review reopens',
    async ({ relaunch }) => {
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
    },
  );

  studioTest('contract: Deny all and Approve all answer every visible id', async ({ relaunch }) => {
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

  studioTest(
    'contract: an item answered by Approve shows a refusal inline',
    async ({ relaunch }) => {
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
    },
  );

  studioTest('an item of an unknown kind offers Deny only', async ({ relaunch }) => {
    const [item] = queue(1);
    if (!item) throw new Error('no item');
    const { window: page } = await relaunch({ control: [] });
    const unknown = { ...item, kind: 'future.kind' };
    await pushApprovals(page, [unknown]);
    await expect(page.locator(`[data-testid="approval-deny-${unknown.id}"]`)).toBeVisible();
    await expect(page.locator(`[data-testid="approval-approve-${unknown.id}"]`)).toHaveCount(0);
  });

  studioTest('contract: a per-item Deny answers only that id', async ({ relaunch }) => {
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
});
