import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../../ui/fixtures';
import { postgresConnectionSummary } from '../../ui/studio/support/postgresFixture';
import { IPC } from '../../ui/support/ipcChannels';
import { settleFonts } from './support/settle';

// P432: the Operations dock at rest, then with one failed op expanded.

function op(overrides: Record<string, unknown>): Record<string, unknown> {
  return {
    id: 'op-1',
    connectionId: 'conn-vis-ops',
    tabId: null,
    startedAt: '2026-01-01T00:03:00.000Z',
    durationMs: 12,
    kind: 'read',
    status: 'ok',
    rows: 3,
    command: 'select id, name from customers order by id',
    error: null,
    source: 'user',
    ...overrides,
  };
}

const CONTROL: ControlSnapshot[] = [
  {
    channel: IPC.connectionsList,
    response: [postgresConnectionSummary('conn-vis-ops', 'Warehouse DB', 'blue')],
  },
  {
    channel: IPC.studioOpsRecent,
    response: [
      op({ id: 'op-a' }),
      op({
        id: 'op-b',
        startedAt: '2026-01-01T00:02:00.000Z',
        status: 'error',
        kind: 'write',
        rows: null,
        durationMs: 480,
        command: 'update orders set total = total * 2 where id = 7',
        error: 'relation "orders" does not exist\nLINE 1: update orders set total = total * 2',
      }),
      op({
        id: 'op-c',
        connectionId: null,
        startedAt: '2026-01-01T00:01:00.000Z',
        kind: 'http',
        rows: null,
        source: 'mcp',
        durationMs: 3,
        command: 'GET /health',
      }),
    ],
  },
];

test('operations dock: list and expanded failed op', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await page.setViewportSize({ width: 1400, height: 800 });
  await page.click('[data-testid="toggle-operations-panel"]');
  const handle = page.getByRole('separator', { name: 'Resize operations panel' });
  await handle.focus();
  for (let i = 0; i < 20; i++) await page.keyboard.press('ArrowUp');
  const dock = page.locator('[data-testid="operations-panel"]');
  await expect(page.locator('[data-testid="op-row"]')).toHaveCount(3);
  await settleFonts(page);
  await expect(dock).toHaveScreenshot('operations-list.png');
  await page.locator('[data-testid="op-row"][data-op-id="op-b"]').click();
  await expect(page.locator('[data-testid="op-detail"]')).toBeVisible();
  await settleFonts(page);
  await expect(dock).toHaveScreenshot('operations-detail.png');
});
