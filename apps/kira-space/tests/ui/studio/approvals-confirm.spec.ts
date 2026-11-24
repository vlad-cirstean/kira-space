import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';

// Contract approvals-confirm. Backend half: dbmcpflow TestDbMcpConfirmAllowedQuery and
// TestDbMcpConfirmOnPrompt. An agent asks for a prompt on an operation the settings allow: the card
// shows the agent's sentence and says the settings allow it; when the settings ask anyway the card shows
// the sentence only.

const item = (key = 'ApprovalsService.List') => {
  const found =
    key === 'ApprovalsService.List'
      ? contract<ApprovalItem[]>('approvals-confirm', key)[0]
      : contract<ApprovalItem>('approvals-confirm', key);
  if (!found) throw new Error(`contract has no ${key}`);
  return found;
};
const card = (page: import('@playwright/test').Page, it: ApprovalItem) =>
  page.locator(`[data-testid="approval-item-${it.id}"]`);

test('contract: a confirm item shows the agent sentence and that settings allow it', async ({
  relaunch,
}) => {
  const it = item();
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  const c = card(page, it);
  await expect(c.locator('[data-testid="approval-agent-confirm"]')).toContainText(it.agentNote);
  await expect(c.locator('[data-testid="approval-agent-requested"]')).toBeVisible();
});

test('contract: a permission item shows the sentence without the allowed notice', async ({
  relaunch,
}) => {
  const it = item('item#permission');
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  const c = card(page, it);
  await expect(c.locator('[data-testid="approval-agent-confirm"]')).toContainText(it.agentNote);
  await expect(c.locator('[data-testid="approval-agent-requested"]')).toHaveCount(0);
});

test('contract: Approve answers the confirm item id', async ({ relaunch }) => {
  const it = item();
  const answer = contract<{ ids: string[] }>('approvals-confirm', 'args:ApprovalsService.Answer');
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(answer.ids) }],
  });
  await pushApprovals(page, [liveItem(it)]);
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: answer.ids, approve: true, options: null });
});
