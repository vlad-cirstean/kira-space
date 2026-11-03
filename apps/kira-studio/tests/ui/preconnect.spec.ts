import type { ConnectionSummary } from '@shared/domain/connection';
import type { ControlSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { IPC } from './support/ipcChannels';
import { connectionRow } from './support/tree';

// Ported from tests/e2e/preconnect.spec.ts. The sidecar lifecycle half spawns a real shell process
// and has no equivalent in a tier with no real backend process; apps/kira-studio/internal/preconnect
// supervisor_test.go and tail_test.go cover it. The dialog half (field visibility, warning, delay
// range, null normalisation) went with the connection dialog: `preconnect` and
// `preconnect_delay_sec` are file fields now, checked by the connfiles schema and
// dbflow preconnect_test. What stays here is the renderer's part: a failing script aborts the
// connect and its output reaches the tree row.

const FAILING: ConnectionSummary = {
  file: 'preconnect-test.yaml',
  id: 'conn-preconnect',
  name: 'Preconnect Test',
  kind: 'postgres',
  color: 'none',
  mode: 'fields',
  readOnly: false,
  host: '127.0.0.1',
  port: 1,
  database: 'testdb',
  username: 'testuser',
  uri: null,
  options: {},
  preconnect: 'echo nope >&2; exit 3',
  preconnectSidecar: false,
  preconnectDelaySec: 0,
  autoExplain: false,
  throttlePerSec: 0,
  mcpEnabled: false,
  mcpDescription: '',
  mcpReadMode: 'allow',
  mcpWriteMode: 'prompt',
  mcpDdlMode: 'deny',
  mcpAutoExplain: true,
  mcpReviewResults: 'inherit',
  sortOrder: 0,
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
};

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [FAILING] },
  {
    channel: IPC.connectionsConnect,
    args: { id: FAILING.id },
    response: {
      connectionId: FAILING.id,
      status: 'error',
      serverVersion: null,
      error: 'Pre-connect script exited (echo nope >&2; exit 3): exit 3\nnope',
      since: 1735689600000,
      caps: null,
    },
  },
];

test('preconnect — a failing script aborts the connect before the adapter is contacted', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL });

  const failRow = await connectionRow(page, 'Preconnect Test');
  await failRow.click({ button: 'right' });
  await page.click('[data-testid="menu-item-connect"]');
  await expect(failRow.locator('.status-dot')).toHaveAttribute('data-status', 'error', {
    timeout: 10_000,
  });
  // The status dot's hint is a real Tooltip, so the dot must be hovered to render it.
  await failRow.locator('.status-dot').hover();
  const statusTooltip = page.locator('[data-slot="tooltip-content"]').first();
  await expect(statusTooltip).toContainText(/exit 3/);
  await expect(statusTooltip).toContainText(/nope/);
});
