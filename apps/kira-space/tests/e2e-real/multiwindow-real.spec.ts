import { expect, test } from './fixtures';
import { createConnection } from './support/connections';
import { installPassthrough } from './support/passthrough';
import { stubNativeDialogs } from './support/routes';
import {
  SQLITE_UNAVAILABLE_MESSAGE,
  type SqliteFixture,
  sqliteAvailable,
  startSqlite,
} from './support/sqlite';

// Two pages, two window keys, one server: a terminal started in one page streams only to it.
test('terminal output reaches only the page that owns the terminal', async ({ kira, browser }) => {
  const context = await browser.newContext({ viewport: { width: 1440, height: 960 } });
  const other = await context.newPage();
  await stubNativeDialogs(other);
  await other.goto(`${kira.baseURL}/?window=${crypto.randomUUID()}`);
  await other.waitForSelector('[data-testid="status-bar"]');

  const owner = kira.window;
  await owner.locator('[data-testid="mode-tab"][data-mode="automations"]').click();
  await owner.locator('[data-testid="automations-start-new"]').click();
  await expect(owner.locator('.xterm-rows')).toBeVisible();
  await owner.locator('.xterm-helper-textarea').focus();
  await owner.keyboard.type('echo only-mine-$((20+22))');
  await owner.keyboard.press('Enter');
  await expect(owner.locator('.xterm-rows')).toContainText('only-mine-42');

  await other.locator('[data-testid="mode-tab"][data-mode="automations"]').click();
  await expect(other.locator('.xterm-rows')).toHaveCount(0);
  await expect(other.locator('body')).not.toContainText('only-mine-42');
  await context.close();
});

// Real multi-window proof on SQLite (no container): each window keeps only its own tabs.
let sqlite: SqliteFixture | null = null;

test.describe('tabs per window', () => {
  test.beforeAll(async () => {
    if (!(await sqliteAvailable())) {
      test.skip(true, SQLITE_UNAVAILABLE_MESSAGE);
      return;
    }
    sqlite = await startSqlite();
  });

  test.afterAll(async () => {
    await sqlite?.stop();
  });

  // Connections are app-wide (D1): connecting from one page broadcasts kira:connection:state to
  // every other page too, so only the first call here actually needs to drive the Connect context
  // menu item — a second page's own attempt would find "Disconnect" already showing instead, since
  // by the time it looks the connection is already live from the first page's own action.
  async function selectStudio(page: import('@playwright/test').Page): Promise<void> {
    await page.locator('[data-testid="mode-tab"][data-mode="studio"]').click();
  }

  async function openTable(
    page: import('@playwright/test').Page,
    tableName: string,
    opts: { connect: boolean },
  ): Promise<void> {
    const connRow = page
      .locator('[data-testid="tree-row"][data-kind="connection"]')
      .filter({ hasText: 'Real SQLite' });
    await expect(connRow).toBeVisible();

    if (opts.connect) {
      await connRow.click({ button: 'right' });
      await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
      await page.click('[data-testid="menu-item-connect"]');
    }
    await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
      timeout: 15_000,
    });

    await connRow.locator('[data-testid="tree-twisty"]').click();
    const dbRow = page.locator('[data-testid="tree-row"][data-path="database:main"]');
    await expect(dbRow).toBeVisible();
    await dbRow.locator('[data-testid="tree-twisty"]').click();

    const tableRow = page.locator(
      `[data-testid="tree-row"][data-path="database:main/table:${tableName}"]`,
    );
    await expect(tableRow).toBeVisible();
    await tableRow.dblclick();
    await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  }

  async function expectOnlyOwnTab(
    page: import('@playwright/test').Page,
    wantTitle: string,
  ): Promise<void> {
    await expect(page.locator('[data-testid="tab"]')).toHaveCount(1);
    await expect(page.locator('[data-testid="tab"] .tab-title')).toHaveText(wantTitle);
  }

  test('two windows, one backend: each keeps only its own tabs (P8 F6)', async ({
    kira,
    browser,
  }) => {
    if (!sqlite) throw new Error('sqlite fixture did not start');

    // Two independent pages against the same real backend, each its own workbench — kira.window
    // (no `?window=`, the "main" fallback, D2) is left untouched and unused by this spec.
    const pageOne = await browser.newPage();
    const pageTwo = await browser.newPage();
    await installPassthrough(pageOne);
    await installPassthrough(pageTwo);
    await pageOne.goto(`${kira.baseURL}/?window=w-one`);
    await pageOne.waitForSelector('[data-testid="status-bar"]');
    await selectStudio(pageOne);
    await pageTwo.goto(`${kira.baseURL}/?window=w-two`);
    await pageTwo.waitForSelector('[data-testid="status-bar"]');
    await selectStudio(pageTwo);

    // The connection itself is app-wide (D1) — added once, from page one.
    await createConnection(kira, { name: 'Real SQLite', kind: 'sqlite', database: sqlite.path });

    // A reload re-hydrates from a plain ConnectionsService.List() call — real backend state, not a
    // UI row count (sqlite-real.spec.ts's own precedent, §5) — so both pages see the same
    // connection without depending on the create event's own delivery timing.
    await pageOne.reload();
    await pageOne.waitForSelector('[data-testid="status-bar"]');
    await selectStudio(pageOne);
    await pageTwo.reload();
    await pageTwo.waitForSelector('[data-testid="status-bar"]');
    await selectStudio(pageTwo);

    // Open a *different* table in each window — page one drives the actual Connect action; page
    // two's own connection state is already live by the time it looks (app-wide, D1).
    await openTable(pageOne, 'customers', { connect: true });
    await openTable(pageTwo, 'products', { connect: false });

    // The real proof: each page's own tab strip shows only its own tab, not the other's.
    await expectOnlyOwnTab(pageOne, 'customers');
    await expectOnlyOwnTab(pageTwo, 'products');

    // Reload both: each restores only its own tab. Against the pre-C4 tree, TabsRepo.Save's
    // unscoped `DELETE FROM tabs` means whichever page's tabsSave landed last would win outright —
    // the other page's own reload would then show that same tab (or none), never its own.
    await pageOne.reload();
    await pageOne.waitForSelector('[data-testid="status-bar"]');
    await selectStudio(pageOne);
    await pageTwo.reload();
    await pageTwo.waitForSelector('[data-testid="status-bar"]');
    await selectStudio(pageTwo);

    await expectOnlyOwnTab(pageOne, 'customers');
    await expectOnlyOwnTab(pageTwo, 'products');

    await pageOne.close();
    await pageTwo.close();
  });
});
