import { emitPrompts, routed } from '@workbench/testing/ui/prompts';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// Contract dbmcp-approval-script. Backend half: dbmcpflow TestDbMcpScriptApproval. An agent's
// run_script in prompt mode parks on the approval dialog: the script, who asked, the command and
// every value it would see, with the secret masked.

type Pending = {
  requestId: string;
  subject: string;
  origin: { client: string; version: string };
  script: {
    name: string;
    description: string;
    command: string;
    params: { name: string; value: string; secret: boolean; saved: boolean }[];
    env: { name: string; value: string; secret: boolean; saved: boolean }[];
  };
};

function setup() {
  const snap = contract<{ pending: Pending }>(
    'dbmcp-approval-script',
    'DbMcpService.PendingApprovals',
  );
  const approve = contract<{ requestId: string }>(
    'dbmcp-approval-script',
    'args:DbMcpService.ApproveQuery',
  );
  const deny = contract<{ requestId: string }>(
    'dbmcp-approval-script',
    'args:DbMcpService.DenyQuery',
  );
  const pending = { ...snap.pending, expiresAtMs: Date.now() + 60_000 };
  return { snap, approve, deny, response: { ...snap, pending } };
}

test('contract: a script request shows the script, who asked and the masked secret', async ({
  relaunch,
}) => {
  const { snap, response } = setup();
  const { window: page } = await relaunch({
    control: [{ channel: IPC.dbMcpPendingApprovals, response }],
  });
  await emitPrompts(page, [routed('dbmcp', snap.pending.requestId)]);
  const dialog = page.locator('[data-testid="db-mcp-approval-dialog"]');
  await expect(dialog.locator('[data-testid="db-mcp-approval-script-name"]')).toHaveText(
    snap.pending.script.name,
  );
  await expect(dialog.locator('[data-testid="db-mcp-approval-script-operation"]')).toBeVisible();
  await expect(dialog.locator('[data-testid="db-mcp-approval-origin"]')).toContainText(
    `${snap.pending.origin.client} ${snap.pending.origin.version}`,
  );
  await expect(dialog.locator('[data-testid="db-mcp-approval-script-description"]')).toHaveText(
    snap.pending.script.description,
  );
  await expect(dialog.locator('[data-testid="db-mcp-approval-script-command"]')).toContainText(
    'KIRA_PARAM_REGION',
  );
  const region = dialog.locator('[data-testid="db-mcp-approval-script-param-region"]');
  await expect(region).toContainText('eu');
  const token = dialog.locator('[data-testid="db-mcp-approval-script-env-KIRA_PARAM_TOKEN"]');
  await expect(token).toContainText('Secret');
  await expect(token).toContainText('Saved value, not shown');
  await expect(dialog.locator('[data-testid="db-mcp-approval-statement"]')).toHaveCount(0);
  await expect(dialog.locator('[data-testid="db-mcp-approval-target"]')).toHaveCount(0);
});

test('contract: Deny is focused and sends the request id; Approve sends it too', async ({
  relaunch,
}) => {
  const { snap, deny, approve, response } = setup();
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.dbMcpPendingApprovals, response },
      { channel: IPC.dbMcpDenyQuery, response: { pending: null, queued: 0 } },
      { channel: IPC.dbMcpApproveQuery, response: { pending: null, queued: 0 } },
    ],
  });
  await emitPrompts(page, [routed('dbmcp', snap.pending.requestId)]);
  await expect(page.locator('[data-testid="db-mcp-approval-deny"]')).toBeFocused();
  await page.click('[data-testid="db-mcp-approval-deny"]');
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.dbMcpDenyQuery)?.args)
    .toEqual(deny);

  await emitPrompts(page, [routed('dbmcp', snap.pending.requestId)]);
  await page.click('[data-testid="db-mcp-approval-approve"]');
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.dbMcpApproveQuery)?.args)
    .toEqual(approve);
});
