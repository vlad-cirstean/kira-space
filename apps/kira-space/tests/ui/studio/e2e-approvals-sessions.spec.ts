import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';

// Contract e2e-approvals-sessions. Backend half: dbmcpflow TestE2EApprovalsSessions, the same scenario. Two
// agent sessions raise two cards each. The dialog groups the cards by session; Deny all answers the shown
// session's ids and no others.

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
