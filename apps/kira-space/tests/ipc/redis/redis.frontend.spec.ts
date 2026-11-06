import type { Locator, Page } from '@playwright/test';
import { expect, type Relaunch, studioTest as test } from '../../ui/fixtures';
import { acceptConfirm } from '../../ui/studio/support/dialogs';
import { connectionRow, expandRow, findRow, openRowMenu } from '../../ui/studio/support/tree';
import type { ControlSnapshot } from '../support/types';
import { controlSnapshots, portSnapshots } from './redis.fixture';

// The redis split's frontend half. Real Vue and real bridge/{control,port
// }.ts, both wire protocols mocked from the exact fixture redis.backend.spec.ts asserts against —
// no container, no Docker, and no Electron process either: a page against the static
// build, both mocks installed at the network layer before the one navigation `relaunch()` does.

interface TreeNodeLike {
  name: string;
  path: string;
}

/** Every path this spec's locators need comes from the fixture's own captured tree.ts:children()
 *  responses, never hand-typed — the same discipline the backend spec follows. */
function nodePathByName(name: string): string {
  for (const snap of controlSnapshots as ControlSnapshot[]) {
    if (snap.channel !== 'kira:tree:children') continue;
    const nodes = (snap.response as { nodes?: TreeNodeLike[] } | undefined)?.nodes ?? [];
    const node = nodes.find((n) => n.name === name);
    if (node) return node.path;
  }
  throw new Error(`no captured tree node named ${name} in redis.fixture.ts`);
}

const DB0_PATH = nodePathByName('db0');
const DB1_PATH = nodePathByName('db1');
const USER_NS_PATH = nodePathByName('user');
const USER_1_NS_PATH = nodePathByName('1');
const HASH_KEY_PATH = nodePathByName('user:1:profile');
const BIG_HASH_KEY_PATH = nodePathByName('user:1:bighash');
const QUEUE_NS_PATH = nodePathByName('queue');
const LIST_KEY_PATH = nodePathByName('queue:jobs');
const SESSION_NS_PATH = nodePathByName('session');
const DB1_OTHER_NS_PATH = nodePathByName('other-db');

async function setup(relaunch: Relaunch): Promise<Page> {
  const { window } = await relaunch({ control: controlSnapshots, stream: portSnapshots });

  const connRow = connectionRow(window);
  await expect(connRow).toBeVisible();
  await openRowMenu(window, '');
  await window.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  return window;
}

async function openLeaf(row: Locator): Promise<void> {
  await row.click({ button: 'right' });
  await row.page().click('[data-testid="menu-item-open"]');
}

test('redis (frontend, mocked IPC) — connect, tree, keyvalue tabs, console', async ({
  relaunch,
  consoleErrors,
}) => {
  const page = await setup(relaunch);

  // --- 1: two logical dbs, both leaves (no key-browsing twisty) ---------------------------
  await expandRow(page, '');
  const db0Row = await findRow(page, DB0_PATH);
  await expect(db0Row).toHaveAttribute('data-kind', 'database');
  await expect(db0Row.locator('[data-testid="tree-twisty"]')).toHaveClass(/invisible/);
  const db1Row = await findRow(page, DB1_PATH);
  await expect(db1Row).toBeVisible();
  await expect(db1Row.locator('[data-testid="tree-twisty"]')).toHaveClass(/invisible/);

  const browseView = page.locator('[data-testid="browse-view"]');

  // --- 2: db1's own Browse tab ------------------------------------------------------------
  await db1Row.dblclick();
  await expect(browseView).toBeVisible();
  await expect(browseView).toHaveAttribute('data-level', DB1_PATH);
  const otherNsRow = browseView.locator(
    `[data-testid="browse-row"][data-path="${DB1_OTHER_NS_PATH}"]`,
  );
  await expect(otherNsRow).toBeVisible();
  await expect(otherNsRow).toHaveAttribute('data-kind', 'namespace');
  await expect(browseView.locator('[data-testid="browse-truncated"]')).toHaveCount(0);

  // --- 3: db0 -> user -> 1 -> the hash key -------------------------------------------------
  await db0Row.dblclick();
  await expect(browseView).toBeVisible();
  await expect(browseView).toHaveAttribute('data-level', DB0_PATH);
  const userRow = browseView.locator(`[data-testid="browse-row"][data-path="${USER_NS_PATH}"]`);
  await expect(userRow).toHaveAttribute('data-kind', 'namespace');
  await userRow.dblclick();
  await expect(browseView).toHaveAttribute('data-level', USER_NS_PATH);
  const user1Row = browseView.locator(`[data-testid="browse-row"][data-path="${USER_1_NS_PATH}"]`);
  await expect(user1Row).toHaveAttribute('data-kind', 'namespace');
  await user1Row.dblclick();
  await expect(browseView).toHaveAttribute('data-level', USER_1_NS_PATH);
  const hashKeyRow = browseView.locator(`[data-testid="browse-row"][data-path="${HASH_KEY_PATH}"]`);
  await expect(hashKeyRow).toBeVisible();
  await expect(hashKeyRow).toHaveAttribute('data-kind', 'key');

  // --- 4: hash key tab — type badge, field/value rows -------------------------------------
  await openLeaf(hashKeyRow);
  const view = page.locator('[data-testid="keyvalue-view"]');
  await expect(view).toBeVisible();
  await expect(view.locator('[data-testid="keyvalue-type"]')).toHaveText('hash');
  await expect(page.locator('[data-testid="keyvalue-row"]')).toHaveCount(2, { timeout: 15_000 });

  // --- 5: a reload clears the cell editor (frontend-only, P43 iter2 F20/D27) --------------
  await view.locator('[data-testid="keyvalue-row"]').first().click();
  const cellEditorPanel = page.locator('[data-testid="cell-editor-panel"]');
  await expect(cellEditorPanel).toBeVisible();
  await page.click('[data-testid="keyvalue-refresh"]');
  await expect(cellEditorPanel).toHaveCount(0);

  // --- 6/7: big hash — virtualised rows, cursor Refresh returns to page one ---------------
  await (await findRow(page, DB0_PATH)).dblclick();
  await expect(browseView).toBeVisible();
  await browseView.locator('[data-testid="browse-crumb"]').first().click();
  await expect(browseView).toHaveAttribute('data-level', DB0_PATH);
  await userRow.dblclick();
  await user1Row.dblclick();
  const bigHashRow = browseView.locator(
    `[data-testid="browse-row"][data-path="${BIG_HASH_KEY_PATH}"]`,
  );
  await expect(bigHashRow).toBeVisible();
  await openLeaf(bigHashRow);
  const bigHashView = page.locator(
    `[data-testid="keyvalue-view"][data-path="${BIG_HASH_KEY_PATH}"]`,
  );
  await expect(bigHashView).toBeVisible();
  const bigHashFirstField = bigHashView.locator('[data-testid="keyvalue-field"]').first();
  await expect(bigHashFirstField).toBeVisible({ timeout: 15_000 });
  // P49 F7/D5: this page holds 100 rows — with rows virtualized, the DOM should hold far fewer.
  const bigHashRowCount = await bigHashView.locator('[data-testid="keyvalue-row"]').count();
  expect(bigHashRowCount).toBeLessThan(100);
  const firstFieldAtPageOne = await bigHashFirstField.innerText();
  await bigHashView.locator('[data-testid="keyvalue-next"]').click();
  await bigHashView.locator('[data-testid="keyvalue-refresh"]').click();
  await expect
    .poll(() => bigHashFirstField.innerText(), { timeout: 15_000 })
    .toBe(firstFieldAtPageOne);

  // --- 8: list key — one page holds every seeded job --------------------------------------
  await (await findRow(page, DB0_PATH)).dblclick();
  await expect(browseView).toBeVisible();
  await browseView.locator('[data-testid="browse-crumb"]').first().click();
  await expect(browseView).toHaveAttribute('data-level', DB0_PATH);
  const queueRow = browseView.locator(`[data-testid="browse-row"][data-path="${QUEUE_NS_PATH}"]`);
  await expect(queueRow).toHaveAttribute('data-kind', 'namespace');
  await queueRow.dblclick();
  await expect(browseView).toHaveAttribute('data-level', QUEUE_NS_PATH);
  const listKeyRow = browseView.locator(`[data-testid="browse-row"][data-path="${LIST_KEY_PATH}"]`);
  await expect(listKeyRow).toBeVisible();
  await openLeaf(listKeyRow);
  const listView = page.locator(`[data-testid="keyvalue-view"][data-path="${LIST_KEY_PATH}"]`);
  await expect(listView).toBeVisible();
  await expect(listView.locator('[data-testid="keyvalue-type"]')).toHaveText('list');
  await expect(listView.locator('[data-testid="keyvalue-status"]')).toContainText('30 loaded', {
    timeout: 15_000,
  });
  await expect(listView.locator('[data-testid="keyvalue-prev"]')).toBeDisabled();
  await expect(listView.locator('[data-testid="keyvalue-next"]')).toBeDisabled();

  // --- 9/10: TTL key — badges populated; delete refreshes the Browse tab with no manual Refresh
  await (await findRow(page, DB0_PATH)).dblclick();
  await expect(browseView).toBeVisible();
  await browseView.locator('[data-testid="browse-crumb"]').first().click();
  await expect(browseView).toHaveAttribute('data-level', DB0_PATH);
  const sessionRow = browseView.locator(
    `[data-testid="browse-row"][data-path="${SESSION_NS_PATH}"]`,
  );
  await expect(sessionRow).toHaveAttribute('data-kind', 'namespace');
  await sessionRow.dblclick();
  await expect(browseView).toHaveAttribute('data-level', SESSION_NS_PATH);
  const ttlKeyRow = browseView.locator('[data-testid="browse-row"][data-kind="key"]', {
    hasText: 'session:abc',
  });
  await expect(ttlKeyRow).toBeVisible();
  await openLeaf(ttlKeyRow);
  await expect(page.locator('[data-testid="keyvalue-type"]').last()).toHaveText('string');
  await expect(page.locator('[data-testid="keyvalue-ttl"]').last()).not.toContainText('No expiry');
  await expect(page.locator('[data-testid="keyvalue-memory"]').last()).not.toContainText('unknown');

  await page.locator('[data-testid="keyvalue-delete"]').last().click();
  await acceptConfirm(page);
  await (await findRow(page, DB0_PATH)).dblclick();
  await expect(browseView).toBeVisible();
  await expect(browseView).toHaveAttribute('data-level', SESSION_NS_PATH);
  await expect(
    browseView.locator('[data-testid="browse-row"][data-kind="key"]', { hasText: 'session:abc' }),
  ).toHaveCount(0);

  // --- 11: console — DBSIZE against db0 ----------------------------------------------------
  await openRowMenu(page, DB0_PATH);
  await page.click('[data-testid="menu-item-open-console"]');
  const consoleView = page.locator('[data-testid="console-view"]');
  await expect(consoleView).toBeVisible();
  await consoleView.locator('.view-lines').click();
  await page.keyboard.type('DBSIZE');
  await page.click('[data-testid="console-run-statement"]');
  const consoleResult = consoleView.locator('[data-testid="console-result-grid"]');
  await expect(consoleResult).toHaveCount(1);
  await expect(consoleResult.locator('[data-testid="console-result-kv-row"]')).toHaveCount(1);

  expect(consoleErrors).toEqual([]);
});

// P41: the Browse panel's own filter and Up affordances — frontend-only (P50 §4.3 row 12): every
// assertion here is over an already-loaded level, which the fixture's already-captured user/1
// listing (four rows) already provides.
test('redis (frontend, mocked IPC) — browse tab: filter and Up', async ({
  relaunch,
  consoleErrors,
}) => {
  const page = await setup(relaunch);

  await expandRow(page, '');
  const db0Row = await findRow(page, DB0_PATH);
  await db0Row.dblclick();
  const browseView = page.locator('[data-testid="browse-view"]');
  await expect(browseView).toBeVisible();
  await expect(browseView).toHaveAttribute('data-level', DB0_PATH);

  const userRow = browseView.locator(`[data-testid="browse-row"][data-path="${USER_NS_PATH}"]`);
  await userRow.dblclick();
  await expect(browseView).toHaveAttribute('data-level', USER_NS_PATH);
  const user1Row = browseView.locator(`[data-testid="browse-row"][data-path="${USER_1_NS_PATH}"]`);
  await user1Row.dblclick();
  await expect(browseView).toHaveAttribute('data-level', USER_1_NS_PATH);

  // `data-level` and the row list itself update off two separate reactive triggers
  // (project/state/tree.ts's own level ref vs. its children fetch resolving) — a gap real
  // Electron IPC's own round-trip latency happened to always outlast, but this tier's HTTP mock
  // sometimes doesn't. `expect.poll` waits for the count to settle before this test reads
  // it as a plain number to reuse below, rather than reading `data-level` matching as a proxy for
  // "the rows are here too".
  await expect.poll(() => browseView.locator('[data-testid="browse-row"]').count()).toBe(4);
  const totalRows = await browseView.locator('[data-testid="browse-row"]').count();
  // P22b D15: the filter is a toggle now (browse-filter-toggle), not an always-visible box.
  await browseView.locator('[data-testid="browse-filter-toggle"]').click();
  await browseView.locator('[data-testid="browse-filter"]').fill('profile');
  await expect(browseView.locator('[data-testid="browse-row"]')).toHaveCount(1);
  await expect(browseView.locator('[data-testid="browse-count"]')).toContainText(
    `1 of ${totalRows}`,
  );

  await db0Row.dblclick();
  await expect(browseView).toBeVisible();
  await expect(browseView).toHaveAttribute('data-level', USER_1_NS_PATH);
  await browseView.locator('[data-testid="browse-filter"]').fill('');
  await expect(browseView.locator('[data-testid="browse-row"]')).toHaveCount(totalRows);

  const upButton = browseView.locator('[data-testid="browse-up"]');
  await upButton.click();
  await expect(browseView).toHaveAttribute('data-level', USER_NS_PATH);
  await upButton.click();
  await expect(browseView).toHaveAttribute('data-level', DB0_PATH);
  await expect(upButton).toBeDisabled();

  expect(consoleErrors).toEqual([]);
});
