import type { ApprovalItem } from '@shared/domain/approvals';
import type { ConnectionSummary } from '@shared/domain/connection';
import { CHANNEL } from '@shared/protocol/events';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { emitWailsEvent } from '../support/mockRuntime';
import { connectionRow } from './support/tree';

// An MCP statement against a connection that stopped answering returns the connection error and
// raises no approval; the connection turns to the error state. Contract dbmcp-unreachable; backend
// half: dbmcpflow TestDbMcpUnreachableRaisesNoApproval. The renderer's part: the lost state shows on
// the tree row and the approvals queue stays empty.

const SCENARIO = 'dbmcp-unreachable';
const ID = 'conn-unreachable';

const CONNECTION: ConnectionSummary = {
  file: 'unreachable-test.yaml',
  id: ID,
  name: 'Unreachable Test',
  kind: 'sqlite',
  color: 'none',
  mode: 'fields',
  readOnly: false,
  host: null,
  port: null,
  database: '/data/mcp.db',
  username: null,
  uri: null,
  options: {},
  preconnect: null,
  preconnectSidecar: false,
  preconnectDelaySec: 0,
  autoExplain: false,
  explainThresholdRows: 0,
  throttlePerSec: 0,
  mcpEnabled: true,
  mcpDescription: '',
  mcpReadMode: 'allow',
  mcpWriteMode: 'prompt',
  mcpDdlMode: 'deny',
  mcpAutoExplain: true,
  mcpReview: { read: 'inherit', write: 'inherit', ddl: 'inherit', masked: 'inherit' },
  mcpMaskedMode: 'inherit',
  mcpStatementTimeoutSec: 10,
  mcpTables: null,
  maskReviewOn: false,
  sortOrder: 0,
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
};

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [CONNECTION] },
  {
    channel: IPC.approvalsList,
    response: contract<ApprovalItem[]>(SCENARIO, 'ApprovalsService.List'),
  },
];

test('dbmcp unreachable — the row shows the lost connection and no approval is queued', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL });
  const row = await connectionRow(page, CONNECTION.name);

  const lost = contract<Record<string, unknown>>(SCENARIO, 'event:kira:connection:state#lost');
  await emitWailsEvent(page, CHANNEL.connectionState, { ...lost, connectionId: ID });

  await expect(row.locator('.status-dot')).toHaveAttribute('data-status', 'error', {
    timeout: 10_000,
  });
  await row.locator('.status-dot').hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(
    /Connection lost/,
  );
  await expect(page.locator('[data-testid="approvals-dialog"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="approvals-toast"]')).toHaveCount(0);
});
