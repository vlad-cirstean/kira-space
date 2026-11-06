import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';

// Contract approvals-exemption. Backend half: dbmcpflow TestDbMcpExemptionApproved and
// TestDbMcpExemptionUnion. An agent asks to lift a limit for one statement: the dialog names each limit
// with the measured value, shows the agent's reason and the statement, and approves only on a click on
// that item.

const item = (key = 'ApprovalsService.List') => {
  const found =
    key === 'ApprovalsService.List'
      ? contract<ApprovalItem[]>('approvals-exemption', key)[0]
      : contract<ApprovalItem>('approvals-exemption', key);
  if (!found) throw new Error(`contract has no ${key}`);
  return found;
};
const card = (page: import('@playwright/test').Page, it: ApprovalItem) =>
  page.locator(`[data-testid="approval-item-${it.id}"]`);

test('contract: a rows limit shows measured value, reason and statement', async ({ relaunch }) => {
  const it = item();
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  const c = card(page, it);
  await expect(c.locator('[data-testid="approval-target-name"]')).toHaveText('notes db');
  await expect(c.locator('[data-testid="approval-operation"]')).toHaveText('Read');
  await expect(c.locator('[data-testid="db-mcp-exemption-lede"]')).toContainText(
    'asks to run one statement past a limit',
  );
  const limits = c.locator('[data-testid="db-mcp-exemption-limit"]');
  await expect(limits).toHaveCount(1);
  await expect(limits).toHaveText('Estimated rows read: 5,000, limit 1,000');
  await expect(c.locator('[data-testid="db-mcp-exemption-reason"]')).toContainText(
    'the report needs every row',
  );
  const statement = c.locator('[data-testid="db-mcp-approval-statement"]');
  await expect(statement).toContainText('select');
  await expect(statement).toContainText('/* big */');
  await expect(c.locator('[data-testid="db-mcp-approval-plan"]')).toHaveCount(0);
});

test('contract: two lifted limits list both, the timeout as a duration', async ({ relaunch }) => {
  const it = item('item#union');
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  const limits = card(page, it).locator('[data-testid="db-mcp-exemption-limit"]');
  await expect(limits).toHaveCount(2);
  await expect(limits.nth(0)).toHaveAttribute('data-limit', 'expensive_query');
  await expect(limits.nth(1)).toHaveAttribute('data-limit', 'statement_timeout');
  await expect(limits.nth(1)).toHaveText('Statement timeout: 5.2s, limit 5.0s');
});

test('contract: Lift for this query answers the item id, Keep the limit denies it', async ({
  relaunch,
}) => {
  const it = item();
  const answer = contract<{ ids: string[] }>('approvals-exemption', 'args:ApprovalsService.Answer');
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(answer.ids) }],
  });
  await pushApprovals(page, [liveItem(it)]);
  await expect(page.locator(`[data-testid="approval-approve-${it.id}"]`)).toHaveText(
    'Lift for this query',
  );
  await expect(page.locator(`[data-testid="approval-deny-${it.id}"]`)).toHaveText('Keep the limit');
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: true, options: null });
});

test('contract: Approve all leaves an exemption for its own click; Deny all covers it', async ({
  relaunch,
}) => {
  const exemption = liveItem(item());
  const [other] = contract<ApprovalItem[]>('approvals-studio', 'ApprovalsService.List');
  if (!other) throw new Error('approvals-studio contract has no item');
  const query = liveItem(other);
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk([query.id, exemption.id]) }],
  });
  await pushApprovals(page, [query, exemption]);

  const approveAll = page.locator('[data-testid="approvals-approve-all"]');
  await expect(approveAll).toHaveText('Approve all (1)');
  await approveAll.click();
  await expect
    .poll(
      () =>
        control
          .log()
          .filter((e) => e.channel === IPC.approvalsAnswer)
          .at(-1)?.args,
    )
    .toEqual({ ids: [query.id], approve: true, options: null });

  await page.click('[data-testid="approvals-deny-all"]');
  await expect
    .poll(() => {
      const args = control
        .log()
        .filter((e) => e.channel === IPC.approvalsAnswer)
        .at(-1)?.args as { ids: string[]; approve: boolean } | undefined;
      return args && { ...args, ids: [...args.ids].sort() };
    })
    .toMatchObject({ ids: [exemption.id, query.id].sort(), approve: false });
});

test('contract: Approve all is absent when only exemptions are pending', async ({ relaunch }) => {
  const a = liveItem(item());
  const b = liveItem(item('item#union'));
  const { window: page } = await relaunch({});
  await pushApprovals(page, [a, b]);
  await expect(page.locator('[data-testid="approvals-deny-all"]')).toBeVisible();
  await expect(page.locator('[data-testid="approvals-approve-all"]')).toHaveCount(0);
});
