import type { ApprovalItem } from '@shared/domain/approvals';
import type { KeptList } from '@shared/domain/kept';
import { answeredOk, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { withTimes } from './support/keptTimes';
import { openKeptPane } from './support/settings';

// Contract e2e-kept-chain. Backend half: dbmcpflow TestE2EKeptChain. A script keeps a list with an
// integer above 2^53; a later request names it and parks as a card that shows the kept value and the
// body; a script on that request rewrites the body; the kept pane lists the value per scope.

const SC = 'e2e-kept-chain';
const item = (key: string) =>
  contract<ApprovalItem[]>(SC, `ApprovalsService.List#${key}`)[0] as ApprovalItem;

function live(key: string): KeptList {
  const list = contract<KeptList>(SC, `KeptVarsService.List#${key}`);
  return {
    ...list,
    entries: list.entries.map((e) => ({
      ...e,
      createdAt: new Date(Date.now() - 60_000).toISOString(),
      expiresAt: new Date(Date.now() + 59 * 60_000).toISOString(),
    })),
  };
}

test('contract: the card shows the kept list with its digits and marks the reference; Send answers by id', async ({
  relaunch,
}) => {
  const it = withTimes(item('step2'));
  const answer = contract<{ ids: string[]; approve: boolean; options: unknown }>(
    SC,
    'args:ApprovalsService.Answer#step2',
  );
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(answer.ids) }],
  });
  await pushApprovals(page, [it]);
  const c = page.locator(`[data-testid="approval-item-${it.id}"]`);
  await expect(c.locator('[data-testid="api-approval-kept"]')).toHaveText('Uses kept: ids');
  const row = c.locator('[data-testid="api-approval-kept-ids"]');
  await expect(row).toContainText('List of 2');
  await expect(row.locator('[data-testid="kept-value-preview"]')).toContainText('9007199254740993');
  await expect(c.locator('[data-testid="api-approval-body-text"] .kira-ed-kept')).toHaveCount(1);
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual(answer);
});

test('contract: a script rewrites the body and the card shows the result the request will send', async ({
  relaunch,
}) => {
  const it = withTimes(item('step3'));
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const c = page.locator(`[data-testid="approval-item-${it.id}"]`);
  await expect(c.locator('[data-testid="api-approval-body-text"]')).toContainText(
    '"id": 9007199254740993',
  );
  await expect(c.locator('[data-testid="approval-chain"]')).toHaveCount(0);
  await c.locator('[data-testid="approval-details-toggle"]').click();
  await expect(c.locator('[data-testid="api-approval-script-changed"]')).toBeVisible();
});

test('contract: the pane lists a session value beside the external ones', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.keptList, response: live('scopes') }],
  });
  await openKeptPane(page);
  const rows = page.locator('[data-testid="settings-kept-entry"]');
  await expect(rows).toHaveCount(3);
  await expect(rows.filter({ hasText: /\[\s*5\s*\]/ })).toHaveCount(1);
  await expect(rows.filter({ hasText: '9007199254740993' })).toHaveCount(1);
});

test('contract: after the session ends only the external values remain', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.keptList, response: live('ended') }],
  });
  await openKeptPane(page);
  const rows = page.locator('[data-testid="settings-kept-entry"]');
  await expect(rows).toHaveCount(2);
  await expect(rows.filter({ hasText: /\[\s*5\s*\]/ })).toHaveCount(0);
});
