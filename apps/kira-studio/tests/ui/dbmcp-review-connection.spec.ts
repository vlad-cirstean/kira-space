import type { ConnectionSummary } from '@shared/domain/connection';
import { emitPrompts, routed } from '@workbench/testing/ui/prompts';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import { openSettings } from './support/settings';

// Contract dbmcp-review-connection. Backend half: dbmcpflow TestDbMcpReviewPerConnection, which sets
// the review setting in the connection's file. A connection's review setting overrides the app switch:
// its results are held with the app switch off.

interface Snapshot {
  pending: { requestId: string; target: { database: string } };
  queued: number;
}
const ID = 'conn-review';
const OTHER_ID = 'conn-inherit';

const reviewed = () =>
  contract<ConnectionSummary>('dbmcp-review-connection', 'ConnectionsService.List#review');

const inheriting = (): ConnectionSummary => ({
  ...reviewed(),
  id: ID,
  mcpReviewResults: 'inherit',
  sortOrder: 0,
});
const other = (): ConnectionSummary => ({
  ...inheriting(),
  id: OTHER_ID,
  name: 'Plain DB',
  sortOrder: 1,
});

test('contract: a file edit that sets the review override reaches the settings glance', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.connectionsList, response: [inheriting()] }],
  });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Database MCP"]');
  await expect(page.locator(`[data-testid="db-mcp-connection-review-${ID}"]`)).toHaveCount(0);

  await emitWailsEvent(page, IPC.connectionsChanged, [{ ...reviewed(), id: ID }]);
  await expect(page.locator(`[data-testid="db-mcp-connection-review-${ID}"]`)).toHaveText(
    'Results always reviewed',
  );
});

test('the settings glance opens the connection file', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.connectionsList, response: [inheriting()] }],
  });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Database MCP"]');
  await page.click(`[data-testid="db-mcp-connection-edit-${ID}"]`);
  await expect(page.locator('[data-testid="connection-file-dialog"]')).toBeVisible();
});

test('the settings glance flags a connection with an override and skips one that inherits', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [
      {
        channel: IPC.connectionsList,
        response: [{ ...inheriting(), mcpReviewResults: 'review' }, other()],
      },
    ],
  });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Database MCP"]');
  await expect(page.locator(`[data-testid="db-mcp-connection-review-${ID}"]`)).toHaveText(
    'Results always reviewed',
  );
  await expect(page.locator(`[data-testid="db-mcp-connection-review-${OTHER_ID}"]`)).toHaveCount(0);
});

test('contract: a result held by a connection override opens the review dialog', async ({
  relaunch,
}) => {
  const snap = contract<Snapshot>('dbmcp-review-connection', 'DbMcpService.PendingApprovals');
  const held = contract<{ requestId: string }>(
    'dbmcp-review-connection',
    'DbMcpService.PendingResponse',
  );
  const { window: page, control } = await relaunch({
    control: [
      {
        channel: IPC.dbMcpPendingApprovals,
        response: { ...snap, pending: { ...snap.pending, expiresAtMs: Date.now() + 60_000 } },
      },
      { channel: IPC.dbMcpPendingResponse, response: held },
      { channel: IPC.dbMcpApproveQuery, response: { pending: null, queued: 0 } },
    ],
  });
  await emitPrompts(page, [routed('dbmcp', snap.pending.requestId)]);
  await expect(page.locator('[data-testid="db-mcp-preview-send"]')).toBeVisible();
  await page.click('[data-testid="db-mcp-preview-send"]');
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.dbMcpApproveQuery)?.args)
    .toEqual({ requestId: snap.pending.requestId });
});
