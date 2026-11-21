import type { ApprovalItem } from '@shared/domain/approvals';
import type { DbMcpPendingResponse } from '@shared/domain/dbmcp';
import type { KeptList } from '@shared/domain/kept';
import type { RevealResult } from '@shared/domain/variables';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { withTimes } from './support/keptTimes';
import { openKeptPane } from './support/settings';

// Contract e2e-kept-secret. Backend half: dbmcpflow TestE2EKeptSecret, the same scenario. A login script
// keeps a session token as a secret and the account as a plain value. The card marks both kinds, the review
// previews the plain one and reveals the secret behind the local-auth gate, the follow-up card shows the
// secret as dots beside the plain preview, and the old chain panel is gone.

const SC = 'e2e-kept-secret';
const TOKEN = 'tok-e2e-session-7Q2xK';
const item = (key: string) =>
  contract<ApprovalItem[]>(SC, `ApprovalsService.List#${key}`)[0] as ApprovalItem;
const reveal = (key: string) => contract<RevealResult>(SC, `KeptVarsService.${key}`);

function liveKinds(): KeptList {
  const list = contract<KeptList>(SC, 'KeptVarsService.List#kinds');
  return {
    ...list,
    entries: list.entries.map((e) => ({
      ...e,
      createdAt: new Date(Date.now() - 60_000).toISOString(),
      expiresAt: new Date(Date.now() + 59 * 60_000).toISOString(),
    })),
  };
}

test('contract: the login card marks the secret and the plain value its script may keep', async ({
  relaunch,
}) => {
  const it = liveItem(item('login'));
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const c = page.locator(`[data-testid="approval-item-${it.id}"]`);
  const mayKeep = c.locator('[data-testid="api-approval-may-keep"]');
  await expect(mayKeep).toContainText('session_token');
  await expect(mayKeep).toContainText('account_id');
  await expect(
    mayKeep.locator('[data-testid="api-approval-may-keep-session_token-secret"]'),
  ).toHaveText('Secret');
  await expect(
    mayKeep.locator('[data-testid="api-approval-may-keep-account_id-plain"]'),
  ).toHaveText('Plain');
  await expect(c.locator('[data-testid="api-approval-kept-writes"]')).toContainText(
    'session_token (secret)',
  );
  await expect(c.locator('[data-testid="approval-chain"]')).toHaveCount(0);
});

test('contract: the review previews the plain value and reveals the secret behind the gate', async ({
  relaunch,
}) => {
  const it = liveItem(item('review'));
  const held = contract<DbMcpPendingResponse>(SC, 'DbMcpService.PendingResponse#review');
  const requestId = it.id.slice(it.kind.length + 1);
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.dbMcpPendingResponse, response: held },
      {
        channel: IPC.keptRevealStaged,
        args: { requestId, name: 'session_token', confirmed: false },
        response: reveal('RevealStaged#confirmation'),
      },
      {
        channel: IPC.keptRevealStaged,
        args: { requestId, name: 'session_token', confirmed: true },
        response: reveal('RevealStaged#review'),
      },
    ],
  });
  await pushApprovals(page, [it]);
  const block = page.locator('[data-testid="api-review-keep"]');
  await expect(block).toContainText('Will keep');
  const secret = block.locator('[data-testid="api-review-keep-session_token"]');
  const plain = block.locator('[data-testid="api-review-keep-account_id"]');
  await expect(secret.locator('[data-testid="kept-kind-secret"]')).toHaveText('Secret');
  await expect(plain.locator('[data-testid="kept-kind-plain"]')).toHaveText('Plain');
  await expect(plain.locator('[data-testid="kept-value-preview"]')).toContainText('acct_42');
  await expect(secret.locator('[data-testid="kept-value-secret"]')).toHaveText('••••••••');
  await expect(page.locator('[data-testid="api-review-body"]')).not.toContainText(
    `"token":"${TOKEN}"`,
  );
  // The result the agent gets carries the account and no token; the raw response is the user's view only.
  await secret.locator('[data-testid="kept-value-reveal"]').click();
  await expect(secret.locator('[data-testid="kept-value-revealed"]')).toHaveCount(0);
  await page.locator('[data-testid="confirm-dialog-confirm"]').click();
  await expect(secret.locator('[data-testid="kept-value-revealed"]')).toHaveText(TOKEN);
  await expect
    .poll(() =>
      control
        .log()
        .filter((e) => e.channel === IPC.keptRevealStaged)
        .map((e) => e.args),
    )
    .toEqual([
      { requestId, name: 'session_token', confirmed: false },
      { requestId, name: 'session_token', confirmed: true },
    ]);
  await secret.locator('[data-testid="kept-value-reveal"]').click();
  await expect(secret.locator('[data-testid="kept-value-revealed"]')).toHaveCount(0);
});

test('contract: the follow-up card shows the secret as dots beside the plain preview', async ({
  relaunch,
}) => {
  const it = withTimes(item('followup'));
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const c = page.locator(`[data-testid="approval-item-${it.id}"]`);
  await expect(c.locator('[data-testid="api-approval-kept"]')).toContainText('session_token');
  const secret = c.locator('[data-testid="api-approval-kept-session_token"]');
  const plain = c.locator('[data-testid="api-approval-kept-account_id"]');
  await expect(secret.locator('[data-testid="kept-kind-secret"]')).toBeVisible();
  await expect(secret.locator('[data-testid="kept-value-secret"]')).toBeVisible();
  await expect(plain.locator('[data-testid="kept-kind-plain"]')).toBeVisible();
  await expect(plain.locator('[data-testid="kept-value-preview"]')).toContainText('acct_42');
  await expect(c.locator('[data-testid="approval-chain"]')).toHaveCount(0);
  await expect(page.locator('body')).not.toContainText(TOKEN);
});

test('contract: Settings lists both kinds and reveals the secret', async ({ relaunch }) => {
  const list = liveKinds();
  const secretRow = list.entries.find((e) => e.secret);
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.keptList, response: list },
      {
        channel: IPC.keptReveal,
        args: { id: secretRow?.id, confirmed: true },
        response: reveal('Reveal#kinds'),
      },
      {
        channel: IPC.keptReveal,
        args: { id: secretRow?.id, confirmed: false },
        response: reveal('Reveal#kinds'),
      },
    ],
  });
  await openKeptPane(page);
  const rows = page.locator('[data-testid="settings-kept-entry"]');
  await expect(rows).toHaveCount(2);
  const secret = rows.filter({ has: page.locator('[data-testid="kept-entry-secret"]') });
  const plain = rows.filter({ has: page.locator('[data-testid="kept-entry-plain"]') });
  await expect(secret).toHaveCount(1);
  await expect(plain).toHaveCount(1);
  await expect(plain.locator('[data-testid="kept-value-preview"]')).toContainText('acct_42');
  await expect(secret.locator('[data-testid="kept-value-secret"]')).toBeVisible();
  await secret.locator('[data-testid="kept-value-reveal"]').click();
  await expect(secret.locator('[data-testid="kept-value-revealed"]')).toHaveText(TOKEN);
});
