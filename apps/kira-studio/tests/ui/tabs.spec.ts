import { dragTab } from '@workbench/testing/ui/drag';
import type { ControlSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { connectionCreateArgs, createConnection } from './support/connect';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import {
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { tabLocator } from './support/tabs';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';

// Ported from tests/e2e/tabs.spec.ts. Its "session restore" section (roughly the back
// half of the first scenario) asserts three tabs across two connections surviving a real
// relaunch(), including which ones reconnect and which persisted page/page-size values come back
// — none of that has an equivalent here (tests/ui/fixtures.ts's own header comment: no backing
// store, nothing to persist to), so it's dropped, same category as workbench.spec.ts's five.
// Everything before it — independent per-tab state, the tab context menu (exact item list, then
// each action), and tab colour — needs no relaunch and ports against real captured Postgres data
// (tests/ui/support/postgresFixture.ts). The second scenario (tab-strip overflow scrolling) is
// pure layout and ports unchanged.

const CONNECTION_ID = 'conn-tabs';
const FIXTURE = orderItemsFixture(CONNECTION_ID);
const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Tabs DB', 'blue');

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Tabs DB'),
    response: CONNECTION_SUMMARY,
  },
  ...FIXTURE.control,
];

async function menuItemIds(page: import('@playwright/test').Page): Promise<string[]> {
  const menu = page.locator('[data-testid="context-menu"]');
  return menu
    .locator(
      '[data-testid^="menu-item-"]:not([data-testid$="-shortcut"]), [data-slot="dropdown-menu-separator"]',
    )
    .evaluateAll((els) =>
      els.map((el) =>
        el.getAttribute('data-slot') === 'dropdown-menu-separator'
          ? '--separator--'
          : (el.getAttribute('data-testid') ?? '').replace('menu-item-', ''),
      ),
    );
}

async function createAndConnect(page: import('@playwright/test').Page): Promise<void> {
  await createConnection(page, 'postgres', 'Tabs DB');

  const connRow = connectionRow(page);
  await expect(connRow).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, 'database:kira_test');
  await expandRow(page, 'database:kira_test/schema:app');
}

test('tabs — independent state, context menu, colours', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await createAndConnect(page);

  // --- open the same table twice: independent state (§8.4, §9.2's own example) -----------
  const orderItemsRow = await findRow(page, ORDER_ITEMS_PATH);
  await orderItemsRow.dblclick(); // tab 1

  await openRowMenu(page, ORDER_ITEMS_PATH);
  await page.click('[data-testid="menu-item-open-data-new-tab"]'); // tab 2, now active

  const tabs = page.locator('[data-testid="tab"]');
  await expect(tabs).toHaveCount(2);
  const tab1Id = await tabs.nth(0).getAttribute('data-tab-id');
  const tab2Id = await tabs.nth(1).getAttribute('data-tab-id');
  if (!tab1Id || !tab2Id) throw new Error('expected two tab ids');

  // tab 2 is active: change its page size to 1000.
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await page.click('[data-testid="page-size-1000"]');
  await expect(page.locator('[data-testid="page-size-1000"]')).toHaveAttribute('data-state', 'on');

  // Switch to tab 1: untouched (page size 100).
  await tabLocator(page, tab1Id).click();
  await expect(page.locator('[data-testid="page-size-100"]')).toHaveAttribute('data-state', 'on');

  // --- tab context menu: exact item id list, then exercise each action --------------------
  await tabLocator(page, tab1Id).click({ button: 'right' });
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  expect(await menuItemIds(page)).toEqual([
    'close',
    'close-others',
    'close-to-the-right',
    'close-all',
    '--separator--',
    'duplicate-tab',
    'copy-name',
    'reveal-in-project-panel',
  ]);
  await page.keyboard.press('Escape');

  // Duplicate tab 1: a third tab appears, sharing tab 1's target but fresh state.
  await tabLocator(page, tab1Id).click({ button: 'right' });
  await page.click('[data-testid="menu-item-duplicate-tab"]');
  await expect(tabs).toHaveCount(3);
  const tab3Id = await tabs.nth(2).getAttribute('data-tab-id');
  if (!tab3Id) throw new Error('expected a third tab id');
  await expect(page.locator('[data-testid="page-size-100"]')).toHaveAttribute('data-state', 'on');

  // Reveal in project panel: collapse the tree, then assert the action re-expands and selects
  // the originating row.
  await (await findRow(page, '')).locator('[data-testid="tree-twisty"]').click();
  await tabLocator(page, tab3Id).click({ button: 'right' });
  await page.click('[data-testid="menu-item-reveal-in-project-panel"]');
  // P110 I2-14: `.tree-row`'s literal `selected` class is gone -- `cn()` now folds selected into
  // `bg-select` via a `stateClass` computed, so `aria-selected` is the stable selected-state check.
  await expect(await findRow(page, ORDER_ITEMS_PATH)).toHaveAttribute('aria-selected', 'true', {
    timeout: 10_000,
  });

  // Close others: only tab 3 (the one right-clicked) survives.
  await tabLocator(page, tab3Id).click({ button: 'right' });
  await page.click('[data-testid="menu-item-close-others"]');
  await expect(tabs).toHaveCount(1);
  await expect(tabLocator(page, tab3Id)).toBeVisible();

  // Rebuild two tabs to exercise close-to-the-right and close-all.
  const orderItemsRowAgain = await findRow(page, ORDER_ITEMS_PATH);
  await orderItemsRowAgain.dblclick(); // reuses/activates the same-target tab (tab 3)
  await openRowMenu(page, ORDER_ITEMS_PATH);
  await page.click('[data-testid="menu-item-open-data-new-tab"]');
  await expect(tabs).toHaveCount(2);
  await tabs.first().click({ button: 'right' });
  await page.click('[data-testid="menu-item-close-to-the-right"]');
  await expect(tabs).toHaveCount(1);

  await tabs.first().click({ button: 'right' });
  await page.click('[data-testid="menu-item-close-all"]');
  await expect(tabs).toHaveCount(0);
  await expect(page.locator('[data-testid="tab-strip-empty"]')).toBeVisible();
  await expect(page.locator('[data-testid="main-view"]')).toContainText('Kira Studio');

  // --- colours: the tab and toolbar rail both carry the connection colour token -----------
  await (await findRow(page, ORDER_ITEMS_PATH)).dblclick();
  const tab = page.locator('[data-testid="tab"]');
  await expect(tab).toHaveAttribute('data-color', 'blue');
  await expect(tab.locator('span').first()).toHaveClass(/\bbg-conn-blue\b/);
  await expect(page.locator('[data-testid="toolbar-rail"]')).toHaveClass(/\bbg-conn-blue\b/);

  // The colour lives in the connection's file: an edit there reaches the open tab.
  await emitWailsEvent(page, IPC.connectionsChanged, [{ ...CONNECTION_SUMMARY, color: 'magenta' }]);
  await expect(tab).toHaveAttribute('data-color', 'magenta');
  await expect(tab.locator('span').first()).toHaveClass(/\bbg-conn-magenta\b/);
  await expect(page.locator('[data-testid="toolbar-rail"]')).toHaveClass(/\bbg-conn-magenta\b/);
});

// WorkbenchShell's own class name used to collide with TabStrip's identically named root under
// scoped CSS's "child inherits the parent's scope id too" rule, so the outer overflow: hidden
// always won regardless of what TabStrip.vue itself declared. Renames the outer wrapper; adds a
// wheel handler and a visible thin scrollbar.
test('the tab strip scrolls once tabs overflow it (P31 D6/D7)', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await createAndConnect(page);
  const orderItemsRow = await findRow(page, ORDER_ITEMS_PATH);
  // Twelve independent tabs of the same table (§8.4: "the same table opens any number of
  // times") — enough at any reasonable window width to overflow a 900px-wide strip.
  for (let i = 0; i < 12; i++) {
    await orderItemsRow.click({ button: 'right' });
    await page.click('[data-testid="menu-item-open-data-new-tab"]');
  }
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(12);

  const strip = page.locator('[data-testid="tab-strip-row"]');
  const overflowX = await strip.evaluate((el) => getComputedStyle(el).overflowX);
  expect(overflowX).not.toBe('hidden');
  const { scrollWidth, clientWidth } = await strip.evaluate((el) => ({
    scrollWidth: el.scrollWidth,
    clientWidth: el.clientWidth,
  }));
  expect(scrollWidth).toBeGreaterThan(clientWidth);

  const scrollLeftBefore = await strip.evaluate((el) => el.scrollLeft);
  await strip.hover();
  await page.mouse.wheel(0, 300);
  await expect.poll(() => strip.evaluate((el) => el.scrollLeft)).toBeGreaterThan(scrollLeftBefore);

  // Activating the first tab again scrolls it back into view (TabStrip.vue's own watch, unchanged).
  await page.locator('[data-testid="tab"]').first().click();
  await expect
    .poll(async () => {
      const box = await page.locator('[data-testid="tab"]').first().boundingBox();
      const stripBox = await strip.boundingBox();
      if (!box || !stripBox) return false;
      return box.x >= stripBox.x;
    })
    .toBe(true);
});

test('tab strip: drag reorders tabs; click, middle-click and close still work (P137)', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: CONTROL });
  await createAndConnect(page);
  const orderItemsRow = await findRow(page, ORDER_ITEMS_PATH);
  for (let i = 0; i < 3; i++) {
    await orderItemsRow.click({ button: 'right' });
    await page.click('[data-testid="menu-item-open-data-new-tab"]');
  }
  const chips = page.locator('[data-testid="tab-strip-row"] [data-testid="tab"]');
  await expect(chips).toHaveCount(3);
  const rowOrder = () =>
    chips.evaluateAll((els) => els.map((el) => el.getAttribute('data-tab-id')));
  const [a, b, c] = (await rowOrder()) as string[];

  await dragTab(page, tabLocator(page, c), tabLocator(page, a));
  await expect.poll(rowOrder).toEqual([c, a, b]);
  await expect(page.locator('[data-testid="tab"].opacity-50')).toHaveCount(0);
  await expect
    .poll(() => {
      const saves = control.log().filter((e) => e.channel === IPC.tabsSave);
      const args = saves[saves.length - 1]?.args as { tabs: { id: string }[] } | undefined;
      return args?.tabs.map((t) => t.id) ?? null;
    })
    .toEqual([c, a, b]);

  await dragTab(page, tabLocator(page, c), tabLocator(page, b));
  await expect.poll(rowOrder).toEqual([a, b, c]);
  await expect(page.locator('[data-testid="tab"].opacity-50')).toHaveCount(0);

  await tabLocator(page, a).locator('button').first().click();
  await expect(tabLocator(page, a)).toHaveAttribute('data-active', 'true');
  await tabLocator(page, b).locator('button').first().click({ button: 'middle' });
  await expect(tabLocator(page, b)).toHaveCount(0);
  await expect.poll(rowOrder).toEqual([a, c]);
  await tabLocator(page, a).locator('[data-testid="tab-close"]').click();
  await expect(tabLocator(page, a)).toHaveCount(0);
  await expect.poll(rowOrder).toEqual([c]);
});
