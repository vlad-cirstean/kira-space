import { expect, test } from '../../ui/studio/fixtures';
import { assertTooltipShows } from '../../ui/studio/support/tooltip';
import { connectionRow, expandRow, findRow, openRowMenu } from '../../ui/studio/support/tree';
import type { ControlSnapshot } from '../support/types';
import { controlSnapshots, portSnapshots } from './kafka.fixture';

// P50 §4.4 — kafka's frontend half. The Add-Connection-dialog flow is left to
// tests/e2e/connections.spec.ts (kept, unchanged) — this spec starts from an already-listed
// connection.

interface TreeNodeLike {
  name: string;
  path: string;
  kind: string;
}

function nodePathByName(name: string, kind?: string): string {
  for (const snap of controlSnapshots as ControlSnapshot[]) {
    if (snap.channel !== 'kira:tree:children') continue;
    const nodes = (snap.response as { nodes?: TreeNodeLike[] } | undefined)?.nodes ?? [];
    const node = nodes.find((n) => n.name === name && (!kind || n.kind === kind));
    if (node) return node.path;
  }
  throw new Error(`no captured tree node named ${name} in kafka.fixture.ts`);
}

const ORDERS_TOPIC_PATH = nodePathByName('orders', 'topic');
const EMPTY_TOPIC_PATH = nodePathByName('empty-topic', 'topic');
const CONSUMER_GROUP_PATH = nodePathByName('kira-test-group', 'consumerGroup');

test('kafka (frontend, mocked IPC) — tree, partition filter, stream tab (offsetWindow), definitions', async ({
  relaunch,
  consoleErrors,
}) => {
  const { window: page } = await relaunch({ control: controlSnapshots, stream: portSnapshots });

  const connRow = connectionRow(page);
  await expect(connRow).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  const statusDot = connRow.locator('.status-dot');
  await expect(statusDot).toHaveAttribute('data-status', 'connected', { timeout: 10_000 });
  // P104 §6: the status-dot hint moved off `data-kira-tip` onto the real Tooltip system.
  await assertTooltipShows(page, statusDot, 'Kafka');

  // --- tree: topics ungrouped at root; "Consumer groups" is a frontend-only folder over the
  // real consumerGroup-kind node (D15/mysql's "Routines" precedent) --------------------------
  await expandRow(page, '');
  const ordersTopicRow = await findRow(page, ORDERS_TOPIC_PATH);
  await expect(ordersTopicRow).toHaveAttribute('data-kind', 'topic');
  const emptyTopicRow = await findRow(page, EMPTY_TOPIC_PATH);
  await expect(emptyTopicRow).toHaveAttribute('data-kind', 'topic');
  const groupsFolder = await findRow(page, '#consumerGroup');
  await expect(groupsFolder).toBeVisible();
  await expect(groupsFolder).toContainText('Consumer groups');
  await groupsFolder.locator('[data-testid="tree-twisty"]').click();
  const groupRow = await findRow(page, CONSUMER_GROUP_PATH);
  await expect(groupRow).toHaveAttribute('data-kind', 'consumerGroup');

  // --- P23 D3: a topic no longer expands — its twisty is hidden -------------------------------
  await expect(ordersTopicRow.locator('[data-testid="tree-twisty"]')).not.toBeVisible();

  // --- open the orders topic: offsetWindow auto-loads on mount, no Poll button ----------------
  await ordersTopicRow.dblclick();
  const view = page.locator(`[data-testid="stream-view"][data-path="${ORDERS_TOPIC_PATH}"]`);
  await expect(view).toBeVisible();
  await expect(view.locator('[data-testid="stream-target"]')).toHaveText('orders');
  await expect(view.locator('[data-testid="stream-poll"]')).toHaveCount(0);
  await expect(view.locator('[data-testid="stream-row"]')).toHaveCount(7, { timeout: 15_000 });
  await expect(view.locator('[data-testid="stream-next"]')).toBeDisabled();

  // kafkaCaps has no maxPageSize — all four page-size choices stay present (P43 iter3 D46/F29).
  await expect(view.locator('[data-testid="stream-page-size-10"]')).toBeVisible();
  await expect(view.locator('[data-testid="stream-page-size-100"]')).toBeVisible();
  await expect(view.locator('[data-testid="stream-page-size-1000"]')).toBeVisible();
  await expect(view.locator('[data-testid="stream-page-size-10000"]')).toBeVisible();

  const firstRow = view.locator('[data-testid="stream-row"]').first();
  // toHaveText with a RegExp matches raw textContent, unlike a string (no whitespace
  // normalization) — the cell's conditional Badge (v-if, hidden here) still leaves Vue's own
  // inter-element whitespace before the key text, confirmed pre-existing (unrelated to the
  // tombstone addition below).
  await expect(firstRow.locator('[data-testid="stream-key"]')).toHaveText(/^\s*key-\d\s*$/);
  await expect(firstRow.locator('[data-testid="stream-headers"]')).toContainText('seed');
  await expect(firstRow.locator('[data-testid="stream-body"]')).toContainText('seq');

  // --- a tombstone (nil Value) renders the NULL marker, not an empty body (P168 Part 12 F17) ---
  const tombstoneRow = view.locator('[data-testid="stream-row"]').last();
  await expect(tombstoneRow.locator('[data-testid="stream-key"]')).toHaveText('key-6');
  await expect(tombstoneRow.locator('[data-testid="stream-body"]')).toHaveAttribute(
    'aria-label',
    'Null (tombstone)',
  );

  // --- the stream view's cell editor dock is a read-only viewer, no write affordances ---------
  await firstRow.locator('[data-testid="stream-body"]').click();
  const cellPanel = page.locator('[data-testid="cell-editor-panel"]');
  await expect(cellPanel).toHaveAttribute('data-read-only', 'true');
  await expect(cellPanel.locator('[data-testid="cell-editor-generate"]')).toHaveCount(0);

  // --- the partition filter popover's own live children() call ---------------------------------
  await view.locator('[data-testid="stream-filter-partition"]').click();
  const partitionMenu = page.locator('[data-testid="stream-partition-menu"]');
  await expect(partitionMenu).toBeVisible();
  await expect(partitionMenu.locator('.partition-option')).toHaveCount(2);
  await page.keyboard.press('Escape');

  // --- row context menu: copy-key + copy-body, no delete/edit ----------------------------------
  await firstRow.click({ button: 'right' });
  const menu = page.locator('[data-testid="context-menu"]');
  await expect(menu).toBeVisible();
  await expect(menu.locator('[data-testid="menu-item-copy-key"]')).toBeVisible();
  await expect(menu.locator('[data-testid="menu-item-copy-body"]')).toBeVisible();
  await page.keyboard.press('Escape');

  // --- an empty topic renders the placeholder, not an error ------------------------------------
  await emptyTopicRow.dblclick();
  const emptyView = page.locator(`[data-testid="stream-view"][data-path="${EMPTY_TOPIC_PATH}"]`);
  await expect(emptyView).toBeVisible();
  await expect(emptyView.locator('[data-testid="stream-row"]')).toHaveCount(0, { timeout: 15_000 });
  await expect(emptyView.locator('.no-rows')).toContainText('No messages');

  // --- Kafka has no console (D13); a topic's definition shows Partitions + Configuration -------
  await openRowMenu(page, CONSUMER_GROUP_PATH);
  await expect(page.locator('[data-testid="menu-item-open-console"]')).toHaveCount(0);
  await page.keyboard.press('Escape');

  await openRowMenu(page, ORDERS_TOPIC_PATH);
  await page.click('[data-testid="menu-item-open-definition"]');
  const topicDef = page.locator('[data-testid="definition-view"]');
  await expect(topicDef).toBeVisible();
  const partitionsSection = topicDef.locator(
    '[data-testid="definition-properties"][data-title="Partitions"]',
  );
  await expect(partitionsSection).toBeVisible({ timeout: 10_000 });
  await expect(partitionsSection.locator('[data-testid="definition-row"]')).toHaveCount(2);
  const configSection = topicDef.locator(
    '[data-testid="definition-properties"][data-title="Configuration"]',
  );
  await expect(configSection).toBeVisible();
  // P58e E11 / P58f cutover: the Go adapter's DescribeTopicConfigs call actually works, unlike the
  // deleted engine's kafkajs binding — the fixture now carries the topic's real config rows.
  await expect(configSection.locator('[data-testid="definition-row"]')).toHaveCount(33);

  // --- a consumer group's definition shows Group/Members/Committed offsets --------------------
  await openRowMenu(page, CONSUMER_GROUP_PATH);
  await page.click('[data-testid="menu-item-open-definition"]');
  const groupDef = page.locator('[data-testid="definition-view"]');
  await expect(groupDef).toBeVisible();
  const offsetsSection = groupDef.locator(
    '[data-testid="definition-properties"][data-title="Committed offsets"]',
  );
  await expect(offsetsSection).toBeVisible({ timeout: 10_000 });
  await expect(offsetsSection.locator('[data-testid="definition-row"]')).toHaveCount(2);

  expect(consoleErrors).toEqual([]);
});
