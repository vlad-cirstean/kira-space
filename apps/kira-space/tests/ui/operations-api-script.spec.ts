import { expect, studioTest as test } from './fixtures';
import { contract } from './support/contract';
import { unmaskedOps } from './support/dbOps';
import { IPC } from './support/ipcChannels';

// Contract api-script-ops. Backend half: dbmcpflow TestApiRequestScripted and TestApiScriptFailures, the
// same rows. A scripted request logs jsPrepare, http, jsTransform; a failed script row shows the text
// the agent got.

test('contract: the script rows show their source highlighted', async ({ relaunch }) => {
  const ops = unmaskedOps(
    contract<Record<string, unknown>[]>('api-script-ops', 'OpsService.Recent'),
  );
  const { window: page } = await relaunch({ control: [{ channel: IPC.opsRecent, response: ops }] });
  await page.click('[data-testid="toggle-operations-panel"]');
  const rows = page.locator('[data-testid="op-row"]');
  await expect(rows).toHaveCount(3);
  await expect(rows.filter({ hasText: 'jsPrepare' })).toHaveCount(1);
  await expect(rows.filter({ hasText: 'jsTransform' })).toHaveCount(1);
  await expect(rows.filter({ hasText: 'http' }).first()).toBeVisible();

  await rows.filter({ hasText: 'jsTransform' }).click();
  const command = page.locator('[data-testid="op-detail-command"]');
  await expect(command).toContainText('response.json.users');
  await expect(command.locator('span[class^="mtk"]').first()).toBeVisible();
});

test('contract: a failed transform row shows the text the agent got', async ({ relaunch }) => {
  const row = contract<{ kind: string; command: string }>(
    'api-script-ops',
    'failure:transformRuntime',
  );
  const text = contract<string>('api-script-ops', 'failureText:transformRuntime');
  const op = unmaskedOps([
    {
      ...contract<Record<string, unknown>[]>('api-script-ops', 'OpsService.Recent')[2],
      kind: row.kind,
      status: 'error',
      command: row.command,
      error: text,
    },
  ]);
  const { window: page } = await relaunch({ control: [{ channel: IPC.opsRecent, response: op }] });
  await page.click('[data-testid="toggle-operations-panel"]');
  await page.locator('[data-testid="op-row"]').click();
  await expect(page.locator('[data-testid="op-detail-error"]')).toHaveText(text);
});
