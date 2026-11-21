import type { Locator, Page } from '@playwright/test';
import { emitPrompts, routed } from '@workbench/testing/ui/prompts';
import { apiTest, expect, studioTest, test } from './fixtures';
import { modeTab, openHttpModeAndNewRequest } from './studio/support/apiMode';
import {
  connectAndExpand,
  connectionCreateArgs,
  openConsoleFromMenu,
} from './studio/support/connect';
import {
  BIG_ROWS_PATH,
  bigRowsFixture,
  postgresConnectionSummary,
} from './studio/support/postgresFixture';
import { findRow, openRowMenu } from './studio/support/tree';
import { adeV2Control, FIXED_NOW } from './support/adeV2';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import { repoRowById } from './support/repoRows';
import type { ControlSnapshot } from './support/types';

// P712: every control that runs a registry chord shows it. Frontend half; the native menu half is
// `TestBuildTemplateShowsEveryGlobalChord`. The table is the independent truth, written out by
// hand rather than read from `formatShortcut`: [mac, other].
const CHIPS: Record<string, readonly [string, string]> = {
  'app.settings': ['⌘,', 'Ctrl+,'],
  'app.new': ['⌘N', 'Ctrl+N'],
  'app.newAlternate': ['⌥⌘N', 'Ctrl+Alt+N'],
  'app.import': ['⌘O', 'Ctrl+O'],
  'api.environments': ['⌥⌘E', 'Ctrl+Alt+E'],
  'api.copyAsCurl': ['⇧⌘C', 'Ctrl+Shift+C'],
  'api.editRaw': ['⇧⌘E', 'Ctrl+Shift+E'],
  'view.toggleProjectPanel': ['⌘B', 'Ctrl+B'],
  'view.toggleOperationsPanel': ['⌘J', 'Ctrl+J'],
  'view.find': ['⌘F', 'Ctrl+F'],
  'panel.find': ['⌘F', 'Ctrl+F'],
  'view.refresh': ['F5', 'F5'],
  'view.run': ['⌘⏎', 'Ctrl+Enter'],
  'view.runAll': ['⇧⌘⏎', 'Ctrl+Shift+Enter'],
  'view.format': ['⌥⇧F', 'Shift+Alt+F'],
  'view.explain': ['⇧⌘X', 'Ctrl+Shift+X'],
  'data.generate': ['⇧⌘I', 'Ctrl+Shift+I'],
  'data.filterColumns': ['⇧⌘F', 'Ctrl+Shift+F'],
  'tab.close': ['⌘W', 'Ctrl+W'],
  'window.new': ['⇧⌘N', 'Ctrl+Shift+N'],
  'grid.deleteRows': ['⌘⌫', 'Delete'],
  'form.submit': ['⌘⏎', 'Ctrl+Enter'],
  'form.save': ['⌘S', 'Ctrl+S'],
  'form.cancel': ['⎋', 'Esc'],
  'prompt.deny': ['⌘⌫', 'Ctrl+Backspace'],
  'prompt.hide': ['⇧⎋', 'Shift+Esc'],
  'tree.open': ['⏎', 'Enter'],
  'tree.rename': ['F2', 'F2'],
  'tree.delete': ['⌘⌫', 'Delete'],
  'tree.copyName': ['⌘C', 'Ctrl+C'],
  'tree.copyUri': ['⌥⌘C', 'Shift+Alt+C'],
  'tree.duplicate': ['⌘D', 'Ctrl+D'],
  'grid.copy': ['⌘C', 'Ctrl+C'],
  'grid.paste': ['⌘V', 'Ctrl+V'],
  'grid.edit': ['⏎', 'Enter'],
  'grid.duplicateRows': ['⌘D', 'Ctrl+D'],
};

const LINUX_UA = 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/605.1.15 (KHTML, like Gecko)';
const PLATFORMS = [
  { name: 'Mac UA', col: 0, prepare: undefined },
  {
    name: 'Linux UA',
    col: 1,
    prepare: async (page: Page): Promise<void> => {
      await page.addInitScript((ua) => {
        Object.defineProperty(navigator, 'userAgent', { get: () => ua });
      }, LINUX_UA);
    },
  },
] as const;

const TIP = '[data-slot="tooltip-content"]';
const chip = (id: string) => `[data-shortcut="${id}"]`;
const itemChip = (itemId: string) => `[data-testid="menu-item-${itemId}-shortcut"]`;

const REPO = {
  id: 'repo-1',
  name: 'demo-repo',
  root: '/tmp/demo-repo',
  repoId: '/tmp/demo-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};
const GIT_CONTROL: ControlSnapshot[] = [{ channel: IPC.codeWorkspaceListRepos, response: [REPO] }];

const CONNECTION_ID = 'conn-hints';
const BIG = bigRowsFixture(CONNECTION_ID);
const DB_CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Hints DB', 'green'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Hints DB', 'green'),
  },
  ...BIG.control,
];

for (const platform of PLATFORMS) {
  const expected = (id: string): string => {
    const row = CHIPS[id];
    if (!row) throw new Error(`CHIPS has no row for ${id}`);
    return row[platform.col];
  };
  const launch = { prepare: platform.prepare };

  // Every chip on the page matches the table: a hint added without a table row fails here.
  async function sweep(page: Page): Promise<void> {
    const found = await page
      .locator('[data-shortcut]')
      .evaluateAll((els) =>
        els.map((e) => [e.getAttribute('data-shortcut') ?? '', e.textContent ?? '']),
      );
    for (const [id, text] of found) expect(text, id).toBe(expected(id));
  }

  // Hover shows the label then the chip; the chip is the tooltip's last element.
  async function hint(page: Page, trigger: Locator, id: string): Promise<void> {
    // A disabled button hovers through its wrapper span, the tooltip trigger.
    const target = (await trigger.isDisabled())
      ? page.locator('[data-slot="tooltip-trigger"]').filter({ has: trigger })
      : trigger;
    await target.scrollIntoViewIfNeeded();
    await target.hover();
    const tip = page
      .locator(TIP)
      .filter({ has: page.locator(chip(id)) })
      .first();
    await expect(tip.locator(chip(id))).toHaveText(expected(id));
    const text = (await tip.innerText()).replace(expected(id), '').trim();
    expect(text.length, `${id} tooltip has a label before the chip`).toBeGreaterThan(0);
    // reka appends a visually hidden role=tooltip copy for assistive tech; skip it.
    expect(
      await tip.evaluate((el) =>
        [...el.children]
          .filter((c) => c.getAttribute('role') !== 'tooltip')
          .at(-1)
          ?.hasAttribute('data-shortcut'),
      ),
    ).toBe(true);
    await sweep(page);
    await page.mouse.move(0, 0);
    await expect(page.locator(TIP)).toHaveCount(0);
  }

  async function menuHint(page: Page, itemId: string, id: string): Promise<void> {
    await expect(page.locator(itemChip(itemId))).toHaveText(expected(id));
    await sweep(page);
  }

  test.describe(`contract: shortcut hints, ${platform.name}`, () => {
    test('title bar buttons: tooltip on hover and on keyboard focus, aria-keyshortcuts', async ({
      relaunch,
    }) => {
      const { window: page } = await relaunch({ control: [], ...launch });
      const bar = [
        ['toggle-project-panel', 'view.toggleProjectPanel', 'B'],
        ['toggle-operations-panel', 'view.toggleOperationsPanel', 'J'],
        ['open-settings', 'app.settings', ','],
        ['new-window', 'window.new', 'N'],
      ] as const;
      const mod = platform.col === 0 ? 'Meta' : 'Control';
      for (const [testid, id, key] of bar) {
        const button = page.locator(`[data-testid="${testid}"]`);
        await hint(page, button, id);
        await page.mouse.move(0, 0);
        await button.focus();
        await expect(page.locator(TIP).locator(chip(id)).first()).toHaveText(expected(id));
        const shift = id === 'window.new' ? 'Shift+' : '';
        await expect(button).toHaveAttribute('aria-keyshortcuts', `${mod}+${shift}${key}`);
        await page.locator('body').click({ position: { x: 5, y: 5 } });
      }
    });

    studioTest('studio: the new-connection button shows the module chord', async ({ relaunch }) => {
      const { window: page } = await relaunch({ control: [], ...launch });
      await hint(page, page.locator('[data-testid="add-connection"]'), 'app.new');
    });

    apiTest(
      'api: panel buttons, start screen and menus show New, Import and Environments',
      async ({ relaunch }) => {
        const tree = {
          collections: [
            {
              id: 'col-1',
              name: 'Orders API',
              sortOrder: 0,
              createdAt: '2026-01-01T00:00:00.000Z',
              updatedAt: '2026-01-01T00:00:00.000Z',
            },
          ],
          items: [],
        };
        const { window: page } = await relaunch({
          control: [{ channel: IPC.collectionsList, response: tree }],
          ...launch,
        });
        await hint(page, page.locator('[data-testid="new-request"]'), 'app.new');
        await hint(page, page.locator('[data-testid="api-environments"]'), 'api.environments');
        await hint(page, page.locator('[data-testid="new-request-start"]'), 'app.new');
        await hint(
          page,
          page.locator('[data-testid="new-grpc-request-start"]'),
          'app.newAlternate',
        );
        await hint(page, page.locator('[data-testid="import-collection-start"]'), 'app.import');

        await page.locator('[data-testid="tab-strip-new"]').click();
        await menuHint(page, 'new-request', 'app.new');
        await menuHint(page, 'new-grpc-request', 'app.newAlternate');
        await page.keyboard.press('Escape');

        await page.locator('[data-testid="tree-background"]').click({ button: 'right' });
        await menuHint(page, 'import-collection', 'app.import');
        await menuHint(page, 'environments', 'api.environments');
      },
    );

    test('automations, ADE and git: the module chord shows on each New control', async ({
      relaunch,
    }) => {
      const { window: page } = await relaunch({ control: GIT_CONTROL, ...launch });
      await modeTab(page, 'automations').click();
      await page.locator('[data-testid="automations-add"]').click();
      await menuHint(page, 'new-script', 'app.new');
      await menuHint(page, 'new-smart-script', 'app.newAlternate');
      await page.keyboard.press('Escape');

      await page.locator('[data-testid="mode-tab"][data-mode="git"]').click();
      await repoRowById(page, REPO.id).click();
      await hint(page, page.locator('[data-testid="tab-strip-new"]'), 'app.new');

      await modeTab(page, 'docker').click();
      await expect(page.locator('[data-testid="tab-strip-new"]')).toHaveCount(0);
    });

    test('ADE add button shows the new chord', async ({ relaunch }) => {
      const { window: page } = await relaunch({
        control: adeV2Control(),
        clockTime: FIXED_NOW,
        ...launch,
      });
      await page.locator('[data-testid="ade-plan"]').waitFor();
      await hint(page, page.locator('[data-testid="ade-add"]'), 'app.new');
    });

    studioTest('data view and console controls show their chords', async ({ relaunch }) => {
      const { window: page } = await relaunch({ control: DB_CONTROL, stream: BIG.port, ...launch });
      await connectAndExpand(page, 'Hints DB', 'green');
      await (await findRow(page, BIG_ROWS_PATH)).dblclick();
      await hint(page, page.locator('[data-testid="toolbar-refresh"]'), 'view.refresh');
      await hint(page, page.locator('[data-testid="toolbar-search"]'), 'view.find');
      await hint(page, page.locator('[data-testid="toolbar-generate-data"]'), 'data.generate');
      // Disabled with nothing selected: the tooltip trigger is the wrapper span around the button.
      await hint(
        page,
        page
          .locator('[data-slot="tooltip-trigger"]')
          .filter({ has: page.locator('[data-testid="toolbar-delete-row"]') }),
        'grid.deleteRows',
      );

      // The field hint shows while the field is empty and goes once it has text.
      const filter = page.locator('[data-testid="column-filter-input"]');
      const field = page.locator('[data-slot="search-field"]').filter({ has: filter });
      await expect(field.locator(chip('data.filterColumns'))).toHaveText(
        expected('data.filterColumns'),
      );
      await filter.fill('id');
      await expect(field.locator(chip('data.filterColumns'))).toHaveCount(0);
      await filter.fill('');

      await openConsoleFromMenu(page, BIG_ROWS_PATH);
      await hint(page, page.locator('[data-testid="console-run-statement"]'), 'view.run');
      await hint(page, page.locator('[data-testid="console-run-all"]'), 'view.runAll');
      await hint(page, page.locator('[data-testid="console-format"]'), 'view.format');
      await hint(page, page.locator('[data-testid="console-explain"]'), 'view.explain');
      await hint(page, page.locator('[data-testid="console-search"]'), 'view.find');
    });

    apiTest(
      'HTTP request controls and the active tab close button show their chords',
      async ({ relaunch }) => {
        const { window: page } = await relaunch({ control: [], ...launch });
        await openHttpModeAndNewRequest(page);
        await hint(page, page.locator('[data-testid="http-send"]'), 'view.run');
        await hint(page, page.locator('[data-testid="http-save"]'), 'form.save');
        await hint(page, page.locator('[data-testid="http-request-refresh"]'), 'view.refresh');
        await hint(page, page.locator('[data-testid="http-copy-as-curl"]'), 'api.copyAsCurl');
        await hint(page, page.locator('[data-testid="http-edit-raw"]'), 'api.editRaw');

        // tab.close closes the active tab: only its button prints the chord.
        await page.locator('[data-testid="tab-strip-new"]').click();
        await page.locator('[data-testid="menu-item-new-request"]').click();
        const tabs = page.locator('[data-testid="tab"]');
        await expect(tabs).toHaveCount(2);
        const inactive = tabs.filter({ hasNot: page.locator('[data-active="true"]') }).first();
        void inactive;
        await hint(page, tabs.nth(1).locator('[data-testid="tab-close"]'), 'tab.close');
        await tabs.nth(0).hover();
        await tabs.nth(0).locator('[data-testid="tab-close"]').hover();
        await expect(page.locator(TIP).first()).toBeVisible();
        await expect(page.locator(TIP).locator('[data-shortcut]')).toHaveCount(0);
      },
    );

    studioTest(
      'dialog header X and commit buttons show Esc, Shift+Esc and the commit chord',
      async ({ relaunch }) => {
        const { window: page } = await relaunch({ control: DB_CONTROL, ...launch });
        await page.click('[data-testid="add-connection"]');
        const dialog = page.locator('[data-testid="new-connection-dialog"]');
        await hint(page, dialog.locator('[data-dialog-dismiss]'), 'form.cancel');
        await page.click('[data-testid="connection-kind-postgres"]');
        await page.fill('[data-testid="new-connection-name"]', 'Hints DB');
        await page.click('[data-testid="new-connection-create"]');
        const file = page.locator('[data-testid="connection-file-dialog"]');
        await expect(file).toBeVisible();
        await expect(
          file.locator(`[data-testid="connection-file-save"] ${chip('form.submit')}`),
        ).toHaveText(expected('form.submit'));
        await sweep(page);
        await page.click('[data-testid="connection-file-close"]');

        const pairing = contract<{ pending: Record<string, unknown> & { requestId: string } }>(
          'mobile-pairing',
          'event:kira:mobile:pairing#request',
        ).pending;
        pairing.expiresAtMs = Date.now() + 120_000;
        await emitWailsEvent(page, IPC.mobilePairing, { pending: pairing, queued: 1 });
        await emitPrompts(page, [routed('mobile-pairing', pairing.requestId)]);
        const prompt = page.locator('[data-testid="mobile-pairing-dialog"]');
        await expect(prompt).toBeVisible();
        await expect(
          prompt.locator(`[data-testid="mobile-pairing-approve"] ${chip('form.submit')}`),
        ).toHaveText(expected('form.submit'));
        await expect(
          prompt.locator(`[data-testid="mobile-pairing-deny"] ${chip('prompt.deny')}`),
        ).toHaveText(expected('prompt.deny'));
        await hint(page, prompt.locator('[data-dialog-dismiss]'), 'prompt.hide');
      },
    );

    studioTest('dialog X: open focus shows no chord, Tab focus does', async ({ relaunch }) => {
      const { window: page } = await relaunch({ control: DB_CONTROL, ...launch });
      await page.click('[data-testid="add-connection"]');
      const dialog = page.locator('[data-testid="new-connection-dialog"]');
      const close = dialog.locator('[data-dialog-dismiss]');
      await expect(close).toBeFocused();
      await expect(page.locator(TIP)).toHaveCount(0);
      await page.keyboard.press('Tab');
      await page.keyboard.press('Shift+Tab');
      await expect(close).toBeFocused();
      await expect(page.locator(TIP).locator(chip('form.cancel')).first()).toHaveText(
        expected('form.cancel'),
      );
    });

    // Chips sit in one right-aligned column at the menu item's inner right edge.
    async function expectRightAligned(page: Page): Promise<void> {
      const menu = page.locator('[data-testid="context-menu"]');
      await expect(menu.locator('[data-slot="dropdown-menu-shortcut"]').first()).toBeVisible();
      const rows = await menu.locator('[data-slot="dropdown-menu-shortcut"]').evaluateAll((els) =>
        els.map((el) => {
          const item = el.closest('[role="menuitem"]') as HTMLElement;
          const box = el.getBoundingClientRect();
          const itemBox = item.getBoundingClientRect();
          const padRight = Number.parseFloat(getComputedStyle(item).paddingRight);
          return {
            right: box.right,
            innerRight: itemBox.right - padRight,
            left: box.left,
            itemLeft: itemBox.left,
            width: itemBox.width,
          };
        }),
      );
      expect(rows.length).toBeGreaterThan(0);
      for (const r of rows) {
        expect(Math.abs(r.right - r.innerRight)).toBeLessThanOrEqual(1);
        expect(r.left).toBeGreaterThan(r.itemLeft + r.width / 3);
        expect(Math.abs(r.right - (rows[0]?.right ?? 0))).toBeLessThanOrEqual(1);
      }
    }

    test('git repo row menu: Open, Rename and Remove show their chords right-aligned', async ({
      relaunch,
    }) => {
      const { window: page } = await relaunch({ control: GIT_CONTROL, ...launch });
      await repoRowById(page, REPO.id).click({ button: 'right' });
      await menuHint(page, 'open', 'tree.open');
      await menuHint(page, 'rename', 'tree.rename');
      await menuHint(page, 'remove', 'tree.delete');
      await expectRightAligned(page);
    });

    studioTest('studio project row menu shows right-aligned chords', async ({ relaunch }) => {
      const { window: page } = await relaunch({ control: DB_CONTROL, stream: BIG.port, ...launch });
      await connectAndExpand(page, 'Hints DB', 'green');
      await openRowMenu(page, BIG_ROWS_PATH);
      await expectRightAligned(page);
      await sweep(page);
    });
  });
}
