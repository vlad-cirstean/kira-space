import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { connectAndExpand, connectionCreateArgs } from './support/connect';
import { gridCell } from './support/grid';
import {
  COMPOSITE_PK_COLUMNS,
  COMPOSITE_PK_PATH,
  compositePkConnectAndOpen,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { findRow } from './support/tree';

// Edits made while a commit is in flight used to be staged and then dropped by the post-commit
// clearPending; an inline editor left open at Commit was staged only after the plan was built.

const CONNECTION_ID = 'conn-commit-in-flight';
const FIXTURE = compositePkConnectAndOpen(CONNECTION_ID);
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('In-flight DB', 'green'),
    response: postgresConnectionSummary(CONNECTION_ID, 'In-flight DB', 'green'),
  },
  ...FIXTURE.control,
];

const POSITION = {
  offset: 0,
  pageSize: 100,
  hasMore: false,
  nextToken: null,
  prevToken: null,
  strategy: 'keyset' as const,
};
const FIRST_UPDATE = {
  kind: 'update',
  key: { tenant_id: '1', entity_id: '1' },
  changes: { name: 'committed value' },
} as const;
const SECOND_UPDATE = {
  kind: 'update',
  key: { tenant_id: '1', entity_id: '2' },
  changes: { name: 'open edit' },
} as const;

function portFor(ops: readonly unknown[], secondName: string): PortSnapshot[] {
  return [
    ...FIXTURE.port,
    {
      op: DATA_OP.mutate,
      payload: {
        connectionId: CONNECTION_ID,
        path: COMPOSITE_PK_PATH,
        ops,
      },
      response: { kind: 'mutate', affectedRows: 1 },
      delayMs: 3000,
    },
    {
      op: DATA_OP.invalidate,
      payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, scope: 'pages' },
      response: { kind: 'invalidate' },
    },
    {
      op: DATA_OP.read,
      payload: {
        connectionId: CONNECTION_ID,
        path: COMPOSITE_PK_PATH,
        projection: null,
        filter: null,
        sort: null,
        pageSize: 100,
        cursor: { mode: 'offset', offset: 0 },
      },
      response: {
        kind: 'read',
        page: {
          kind: 'tabular',
          columns: COMPOSITE_PK_COLUMNS,
          rows: [
            ['1', '1', 'committed value'],
            ['1', '2', secondName],
            ['2', '1', 'tenant 2 / entity 1'],
          ],
          position: POSITION,
          truncatedCells: 0,
        },
        source: 'server',
      },
    },
  ];
}

async function openGrid(page: import('@playwright/test').Page): Promise<void> {
  await connectAndExpand(page, 'In-flight DB', 'green');
  await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await expect(gridCell(page, 0, 'name')).toBeVisible();
}

test('an inline edit open at Commit is part of the commit', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: CONTROL,
    stream: portFor([FIRST_UPDATE, SECOND_UPDATE], 'open edit'),
  });
  await openGrid(page);

  await gridCell(page, 0, 'name').dblclick();
  await page.locator('[data-testid="grid-cell-input"]').fill('committed value');
  await page.keyboard.press('Enter');
  await gridCell(page, 1, 'name').dblclick();
  await page.locator('[data-testid="grid-cell-input"]').fill('open edit');
  await page.click('[data-testid="toolbar-commit-changes"]');

  // The mutate snapshot matches only a plan carrying the still-open edit.
  await expect(gridCell(page, 1, 'name')).toHaveText('open edit', { timeout: 15_000 });
  await expect(page.locator('[data-testid="toolbar-commit-changes"]')).toHaveCount(0, {
    timeout: 15_000,
  });
});

test('a dock edit made while a commit is in flight is refused, not staged then dropped', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: CONTROL,
    stream: portFor([FIRST_UPDATE], 'tenant 1 / entity 2'),
  });
  await openGrid(page);

  await gridCell(page, 0, 'name').dblclick();
  await page.locator('[data-testid="grid-cell-input"]').fill('committed value');
  await page.keyboard.press('Enter');
  await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);

  // The dock publishes its stage closure before the commit starts; a programmatic click keeps the
  // dock focused, so the stage below happens strictly mid-flight.
  await gridCell(page, 1, 'name').click();
  const panel = page.locator('[data-testid="cell-editor-panel"]');
  await panel.waitFor();
  await panel.locator('.view-lines').click();
  await page.keyboard.press('ControlOrMeta+A');
  await page.keyboard.type('typed mid-flight');
  await page.evaluate(() =>
    (document.querySelector('[data-testid="toolbar-commit-changes"]') as HTMLButtonElement).click(),
  );
  await expect(page.locator('[data-testid="toolbar-commit-changes"]')).toBeDisabled();
  await page.keyboard.press('ControlOrMeta+Enter');

  // Without the guard the edit stages here and only vanishes when the commit returns.
  await page.waitForTimeout(300);
  await expect(page.locator('[data-testid="toolbar-commit-changes"]')).toBeDisabled();
  // Read once: toHaveClass would retry until the commit returns and the edit is dropped anyway.
  expect(await gridCell(page, 1, 'name').getAttribute('class')).not.toMatch(/pending-edit/);
  await expect(gridCell(page, 0, 'name')).toHaveText('committed value', { timeout: 15_000 });
  await expect(page.locator('[data-testid="toolbar-commit-changes"]')).toHaveCount(0, {
    timeout: 15_000,
  });
});
