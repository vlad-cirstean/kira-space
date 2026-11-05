import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// Contract dbmcp-approval-script. Backend half: dbmcpflow TestDbMcpScriptApproval. An agent's
// run_script in prompt mode parks in the approvals dialog as a script.run item: the script, who
// asked, the command and every value it would see, with the secret masked.

type Payload = { command: string; description: string };

function setup() {
  const [item] = contract<ApprovalItem[]>('dbmcp-approval-script', 'ApprovalsService.List');
  const answer = contract<{ ids: string[] }>(
    'dbmcp-approval-script',
    'args:ApprovalsService.Answer',
  );
  if (!item) throw new Error('contract has no item');
  return { item, answer, payload: item.payload as Payload };
}

test('contract: a script request shows the script, who asked and the masked secret', async ({
  relaunch,
}) => {
  const { item, payload } = setup();
  const { window: page } = await relaunch({});

  await pushApprovals(page, [liveItem(item)]);
  const card = page.locator(`[data-testid="approval-item-${item.id}"]`);
  await expect(card.locator('[data-testid="approval-target-name"]')).toHaveText(item.target.name);
  await expect(card.locator('[data-testid="approval-operation"]')).toBeVisible();
  await expect(card.locator('[data-testid="approval-origin"]')).toContainText(item.origin.client);
  await expect(card.locator('[data-testid="db-mcp-approval-script-description"]')).toHaveText(
    payload.description,
  );
  await expect(card.locator('[data-testid="db-mcp-approval-script-command"]')).toContainText(
    'KIRA_PARAM_REGION',
  );
  await expect(card.locator('[data-testid="db-mcp-approval-script-param-region"]')).toContainText(
    'eu',
  );
  const token = card.locator('[data-testid="db-mcp-approval-script-env-KIRA_PARAM_TOKEN"]');
  await expect(token).toContainText('Secret');
  await expect(token).toContainText('Saved value, not shown');
  await expect(card.locator('[data-testid="db-mcp-approval-statement"]')).toHaveCount(0);
});

test('contract: Deny is focused first and denies by item id; Approve answers it too', async ({
  relaunch,
}) => {
  const { item, answer } = setup();
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(answer.ids) }],
  });

  await pushApprovals(page, [liveItem(item)]);
  await expect(page.locator(`[data-testid="approval-deny-${item.id}"]`)).toBeFocused();
  await page.click(`[data-testid="approval-deny-${item.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [item.id], approve: false, options: null });

  await pushApprovals(page, [liveItem(item)]);
  await page.click(`[data-testid="approval-approve-${item.id}"]`);
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.approvalsAnswer).length)
    .toBe(2);
});
