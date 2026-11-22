import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';

// Contract e2e-approvals-sessions. Backend half: dbmcpflow TestE2EApprovalsSessions, the same scenario. Two
// agent sessions raise two cards each. The dialog groups the cards by session; Deny all answers the shown
// session's ids and no others.

const sessionKeyOf = (it: ApprovalItem) => `session:${it.origin.session}`;

const items = () =>
  contract<ApprovalItem[]>('e2e-approvals-sessions', 'ApprovalsService.List').map((it) =>
    liveItem(it),
  );

test('contract: the dialog groups the cards by agent session and denies one group at a time', async ({
  relaunch,
}) => {
  const all = items();
  const idsA = [all[0]?.id, all[1]?.id] as string[];
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(idsA) }],
  });
  await pushApprovals(page, all);
  const tabs = page.locator('[data-testid^="approvals-session-"]');
  await expect(tabs).toHaveCount(2);
  await expect(page.locator('[data-testid="approvals-position"]')).toBeVisible();
  await page.click('[data-testid="approvals-deny-all"]');
  await expect
    .poll(() => {
      const args = control
        .log()
        .filter((e) => e.channel === IPC.approvalsAnswer)
        .at(-1)?.args as { ids: string[]; approve: boolean } | undefined;
      return args && { ...args, ids: [...args.ids].sort() };
    })
    .toMatchObject({ ids: [...idsA].sort(), approve: false });
});

test('contract: same-folder sessions get distinct tab labels', async ({ relaunch }) => {
  const { window: page } = await relaunch({});
  await pushApprovals(page, items());
  const tabs = page.locator('[data-testid^="approvals-session-"]');
  await expect(tabs).toHaveCount(2);
  const [first, second] = await tabs.allInnerTexts();
  expect(first).not.toEqual(second);
});

test('contract: answering stays on the picked session and shows its next card', async ({
  relaunch,
}) => {
  const all = items();
  const [a1, a2, b1, b2] = all as [ApprovalItem, ApprovalItem, ApprovalItem, ApprovalItem];
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk([b1.id]) }],
  });
  await pushApprovals(page, all);
  await expect(page.getByTestId(`approval-approve-${a1.id}`)).toBeVisible();
  await page.click(`[data-testid="approvals-session-${sessionKeyOf(b1)}"]`);
  await expect(page.getByTestId(`approval-approve-${b1.id}`)).toBeVisible();
  await page.getByTestId(`approval-approve-${b1.id}`).click();
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.approvalsAnswer).length)
    .toBe(1);
  await pushApprovals(page, [a1, a2, b2]);
  await expect(page.getByTestId(`approval-approve-${b2.id}`)).toBeVisible();
  await expect(page.getByTestId(`approval-approve-${a1.id}`)).toHaveCount(0);
  // Last card of the picked session answered: the other session's next card shows.
  await pushApprovals(page, [a1, a2]);
  await expect(page.getByTestId(`approval-approve-${a1.id}`)).toBeVisible();
});
