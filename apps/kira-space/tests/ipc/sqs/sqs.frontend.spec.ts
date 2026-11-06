import { expect, test } from '../../ui/studio/fixtures';
import { connectionRow, expandRow, findRow, openRowMenu } from '../../ui/studio/support/tree';
import type { ControlSnapshot } from '../support/types';
import { controlSnapshots, portSnapshots } from './sqs.fixture';

// P50 §4.4 — sqs's frontend half. The connectionsCreate (uri mode) flow is left to
// tests/e2e/connections.spec.ts (kept, unchanged) — this spec starts from an already-listed
// connection.

interface TreeNodeLike {
  name: string;
  path: string;
  kind: string;
}

function nodePathByName(name: string): string {
  for (const snap of controlSnapshots as ControlSnapshot[]) {
    if (snap.channel !== 'kira:tree:children') continue;
    const nodes = (snap.response as { nodes?: TreeNodeLike[] } | undefined)?.nodes ?? [];
    const node = nodes.find((n) => n.name === name);
    if (node) return node.path;
  }
  throw new Error(`no captured tree node named ${name} in sqs.fixture.ts`);
}

const DRAIN_QUEUE_PATH = nodePathByName('drain-queue');
const EMPTY_QUEUE_PATH = nodePathByName('empty-queue');
const ORDERS_QUEUE_PATH = nodePathByName('orders-queue');

test('sqs (frontend, mocked IPC) — flat queue tree, stream tab (batch, Poll-only), definition', async ({
  relaunch,
  consoleErrors,
}) => {
  const { window: page, stream } = await relaunch({
    control: controlSnapshots,
    stream: portSnapshots,
  });

  const connRow = connectionRow(page);
  await expect(connRow).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });

  // --- tree: a flat queue list, no nested level under any queue ---------------------------
  await expandRow(page, '');
  const drainRow = await findRow(page, DRAIN_QUEUE_PATH);
  await expect(drainRow).toBeVisible();
  await expect(drainRow).toHaveAttribute('data-kind', 'queue');
  await expect(drainRow.locator('[data-testid="tree-twisty"]')).toHaveClass(/invisible/);
  const emptyQueueRow = await findRow(page, EMPTY_QUEUE_PATH);
  await expect(emptyQueueRow).toBeVisible();
  const ordersQueueRow = await findRow(page, ORDERS_QUEUE_PATH);
  await expect(ordersQueueRow).toBeVisible();

  // --- open the orders queue: batch pagination never auto-loads --------------------------
  await ordersQueueRow.dblclick();
  const view = page.locator(`[data-testid="stream-view"][data-path="${ORDERS_QUEUE_PATH}"]`);
  await expect(view).toBeVisible();
  await expect(view.locator('[data-testid="stream-next"]')).toHaveCount(0);
  await expect(view.locator('[data-testid="stream-poll"]')).toBeVisible();
  await expect(view.locator('[data-testid="stream-poll-warning"]')).toContainText(
    'consumes messages',
  );
  await expect(view.locator('.no-rows')).toContainText('Poll to fetch messages');
  await expect(view.locator('[data-testid="stream-row"]')).toHaveCount(0);

  // --- P2 R1 regression: refresh must never itself consume from the queue — a batch tab's
  //     Refresh affordance is inert (disabled), unlike every other view kind ---------------
  await expect(view.locator('[data-testid="stream-refresh"]')).toBeDisabled();
  const opsBeforeRefresh = await stream.ops();
  await view.locator('[data-testid="stream-refresh"]').click({ force: true });
  await expect(view.locator('[data-testid="stream-row"]')).toHaveCount(0);
  expect(await stream.ops()).toEqual(opsBeforeRefresh);

  // sqs has no caps.maxPageSize — all four sizes stay present (the guard that would otherwise
  // filter them down applies only to an adapter that actually exposes a real cap, P50 §4.4).
  await expect(view.locator('[data-testid="stream-page-size-10"]')).toBeVisible();
  await expect(view.locator('[data-testid="stream-page-size-100"]')).toBeVisible();
  await expect(view.locator('[data-testid="stream-page-size-1000"]')).toBeVisible();
  await expect(view.locator('[data-testid="stream-page-size-10000"]')).toBeVisible();

  // --- Poll: fetches a batch, populates the visibility-timeout badge and rows ---------------
  await view.locator('[data-testid="stream-poll"]').click();
  await expect(view.locator('[data-testid="stream-row"]').first()).toBeVisible({
    timeout: 15_000,
  });
  await expect(view.locator('[data-testid="stream-visibility-timeout"]')).toContainText(
    /Visibility \d+s/,
  );
  const firstRow = view.locator('[data-testid="stream-row"]').first();
  await expect(firstRow.locator('[data-testid="stream-key"]')).not.toHaveText('None');
  await expect(firstRow.locator('[data-testid="stream-headers"]')).toContainText('seed');
  await expect(firstRow.locator('[data-testid="stream-body"]')).toContainText('seq');

  // --- a Poll clears the cell editor (frontend-only, P43 iter2 F20/D27) --------------------
  await firstRow.locator('[data-testid="stream-body"]').click();
  const streamCellEditorPanel = page.locator('[data-testid="cell-editor-panel"]');
  await expect(streamCellEditorPanel).toBeVisible();
  await view.locator('[data-testid="stream-poll"]').click();
  await expect(streamCellEditorPanel).toHaveCount(0);

  // --- Regression: SQS has no addressable cursor, so a second Poll's data:read carries
  //     the identical connectionId/path/pageSize/cursor as the first — without invalidating the
  //     cache first, the second Poll would silently be served the first Poll's now-stale page
  //     instead of ever reaching a fresh ReceiveMessage --------------------------------------
  const opsSoFar = await stream.ops();
  expect(opsSoFar.filter((o) => o.op === 'data:read').length).toBeGreaterThanOrEqual(2);
  expect(opsSoFar).toContainEqual({
    op: 'data:invalidate',
    payload: { connectionId: 'test-sqs', path: ORDERS_QUEUE_PATH },
  });

  // --- row context menu: copy-key + copy-body, read-only -----------------------------------
  await firstRow.click({ button: 'right' });
  const menu = page.locator('[data-testid="context-menu"]');
  await expect(menu).toBeVisible();
  await expect(menu.locator('[data-testid="menu-item-copy-key"]')).toBeVisible();
  await expect(menu.locator('[data-testid="menu-item-copy-body"]')).toBeVisible();
  await page.keyboard.press('Escape');

  // --- approximate count carries a "~" -----------------------------------------------------
  await emptyQueueRow.dblclick();
  const emptyView = page.locator(`[data-testid="stream-view"][data-path="${EMPTY_QUEUE_PATH}"]`);
  await expect(emptyView).toBeVisible();
  await emptyView.locator('[data-testid="stream-count"]').click();
  await expect(emptyView.locator('[data-testid="stream-status"]')).toContainText('~0 total', {
    timeout: 10_000,
  });

  // --- sqs is read-only with no console; Open definition is offered instead ----------------
  await openRowMenu(page, ORDERS_QUEUE_PATH);
  await expect(page.locator('[data-testid="menu-item-open-console"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="menu-item-open-definition"]')).toBeVisible();

  // --- the queue's definition shows its attributes, no console button, and a Refresh reuses
  //     the same request shape --------------------------------------------------------------
  await page.click('[data-testid="menu-item-open-definition"]');
  const queueDef = page.locator('[data-testid="definition-view"]');
  await expect(queueDef).toBeVisible();
  await expect(queueDef.locator('[data-testid="definition-open-console"]')).toHaveCount(0);
  const attributesSection = queueDef.locator(
    '[data-testid="definition-properties"][data-title="Attributes"]',
  );
  await expect(attributesSection).toBeVisible({ timeout: 10_000 });
  await expect(attributesSection.locator('[data-testid="definition-row"]')).not.toHaveCount(0);
  await queueDef.locator('[data-testid="definition-refresh"]').click();
  await expect(attributesSection).toBeVisible({ timeout: 10_000 });

  expect(consoleErrors).toEqual([]);
});
