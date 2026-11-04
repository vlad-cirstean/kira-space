import type { Locator, Page } from '@playwright/test';
import { defaultSettings } from '../../frontend/src/state/settingsDomain';
import type { ControlSnapshot, PortSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { modeTab, openHttpModeAndNewRequest } from './support/apiMode';
import {
  connectAndExpand,
  connectionCreateArgs,
  createConnection,
  openConsoleFromMenu,
} from './support/connect';
import { executeSnap, numberPage } from './support/consolePages';
import { typeInto } from './support/editor';
import { IPC } from './support/ipcChannels';
import {
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';

// The row's premise ("the page-number input is visibly taller than the controls beside
// it") is not reproducible from the stylesheet — every control in the pager's own toolbar
// row already resolves to one height under one box-sizing. This measures that directly rather
// than guess a pixel change against an unmeasured complaint.
//
// The control-role layer (--kira-control-h/-lg/-sm) is a pure alias over the existing
// --kira-h-* scale — every control keeps today's rendered height. The one deliberate exception is
// .p-view-head, which moves onto the --kira-bar-h band to close the split the bar-height
// family left between it and the now-taller .p-toolbar.

const CONNECTION_ID = 'conn-control-sizing';
const FIXTURE = orderItemsFixture(CONNECTION_ID);
const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Sizing DB', 'blue');

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: {
      name: 'Sizing DB',
      kind: 'postgres',
      color: 'blue',
      mode: 'fields',
      readOnly: false,
      host: '127.0.0.1',
      port: 5432,
      database: 'kira_test',
      username: 'postgres',
      password: null,
      uri: null,
      options: {},
      preconnect: null,
      preconnectSidecar: false,
      preconnectDelaySec: 0,
      autoExplain: false,
      throttlePerSec: 0,
    },
    response: CONNECTION_SUMMARY,
  },
  ...FIXTURE.control,
];

async function connectAndOpenGrid(page: Page): Promise<void> {
  await createConnection(page, 'postgres', 'Sizing DB');

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

  const row = await findRow(page, ORDER_ITEMS_PATH);
  await row.dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
}

function rootVar(page: Page, name: string): Promise<number> {
  return page.evaluate((n) => {
    // getComputedStyle's own custom-property value keeps a calc() expression literal (only var()
    // references resolve) -- --kira-t-sm/-lg (calc(var(--kira-font-size) ± 1px)) need a real
    // property to force calc reduction. A detached, unpainted probe does that without touching
    // layout: any length-typed property works, `width` chosen to keep it independent of font
    // metrics.
    const probe = document.createElement('div');
    probe.style.cssText = `position:absolute;visibility:hidden;width:var(${n})`;
    document.documentElement.appendChild(probe);
    const value = Number.parseFloat(getComputedStyle(probe).width);
    probe.remove();
    return value;
  }, name);
}

// The one shared focus ring (packages/theme/src/base.css's `focus-ring`) -- 1px solid
// --kira-focus, laid inset over the element's own 1px edge, no box-shadow halo. Asserted on the
// actual computed style rather than the class string: the value must render identically whether
// the base `:focus-visible` rule, a `has-[...]:focus-visible:focus-ring` box, or an explicit
// `focus-within:focus-ring` painted it.
async function expectFocusRing(el: Locator): Promise<void> {
  const readStyle = () =>
    el.evaluate((node) => {
      const cs = getComputedStyle(node);
      return {
        outlineStyle: cs.outlineStyle,
        outlineWidth: cs.outlineWidth,
        outlineOffset: cs.outlineOffset,
        outlineColor: cs.outlineColor,
        boxShadow: cs.boxShadow,
      };
    });
  // Every shadcn primitive carries its own transition-colors/transition-all -- which animates
  // outline-width/-color/-offset along with everything else -- so a one-shot read right after
  // focus can catch it mid-transition. Poll past the (150ms default) transition instead of
  // sleeping a fixed amount.
  await expect.poll(async () => (await readStyle()).outlineColor).toBe('rgb(90, 138, 230)');
  const style = await readStyle();
  expect(style.outlineStyle).toBe('solid');
  expect(style.outlineWidth).toBe('1px');
  expect(style.outlineOffset).toBe('-1px');
  expect(style.boxShadow).toBe('none');
}

// Clicks `anchorTestid` (a mouse click, which never itself triggers :focus-visible on a button)
// when given, then presses Tab until the next tabbable element is `targetTestid`
// -- a real keyboard focus, the only way a button's own :focus-visible rule ever paints. Disabled
// controls in between are skipped by the browser's own native Tab order, so this is robust to
// which of the toolbar's own buttons happen to be enabled in the fixture. `anchorTestid: null`
// starts tabbing from whatever already has focus (e.g. a just-opened dialog's own initial focus).
async function focusViaTabFrom(
  page: Page,
  anchorTestid: string | null,
  targetTestid: string,
  maxTabs = 8,
): Promise<void> {
  if (anchorTestid !== null) {
    await page.locator(`[data-testid="${anchorTestid}"]`).click();
  }
  for (let i = 0; i < maxTabs; i++) {
    await page.keyboard.press('Tab');
    const active = await page.evaluate(
      () => document.activeElement?.getAttribute('data-testid') ?? null,
    );
    if (active === targetTestid) return;
  }
  throw new Error(
    `could not reach [data-testid="${targetTestid}"] via Tab from [data-testid="${anchorTestid}"] within ${maxTabs} presses`,
  );
}

test('every control in the pager toolbar row is one height, equal to --kira-control-h', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: FIXTURE.port });
  await connectAndOpenGrid(page);

  const controlH = await rootVar(page, '--kira-control-h');
  expect(controlH).toBeGreaterThan(0);

  const pageInputHeight = await page
    .locator('[data-testid="pager-page-input"]')
    .evaluate((el) => el.getBoundingClientRect().height);
  expect(pageInputHeight).toBeCloseTo(controlH, 0);

  for (const testid of ['pager-first', 'pager-prev', 'pager-next', 'pager-last']) {
    const height = await page
      .locator(`[data-testid="${testid}"]`)
      .evaluate((el) => el.getBoundingClientRect().height);
    expect(height).toBeCloseTo(controlH, 0);
  }

  const searchHeight = await page
    .locator('[data-testid="toolbar-search"]')
    .evaluate((el) => el.getBoundingClientRect().height);
  expect(searchHeight).toBeCloseTo(controlH, 0);
});

test('the .md control family in SettingsDialog is one height, equal to --kira-control-h-lg', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch();
  await page.click('[data-testid="open-settings"]');
  await expect(page.locator('[data-testid="settings-dialog"]')).toBeVisible();

  const controlHLg = await rootVar(page, '--kira-control-h-lg');
  expect(controlHLg).toBeGreaterThan(0);

  // settings-font-size is now InputGroupInput's own borderless leaf, nested one level
  // inside the bordered InputGroup box (the stepper recipe's own box-model split) -- the group's
  // own root is what renders at --kira-control-h-lg, so that's what this measures.
  // InputGroup's own root is a <fieldset> (implicit role="group"), not a <div>.
  const fontSizeInputHeight = await page
    .locator('[data-testid="settings-font-size"]')
    .locator('xpath=ancestor::fieldset[@data-slot="input-group"][1]')
    .evaluate((el) => el.getBoundingClientRect().height);
  expect(fontSizeInputHeight).toBeCloseTo(controlHLg, 0);

  const saveHeight = await page
    .locator('[data-testid="settings-save"]')
    .evaluate((el) => el.getBoundingClientRect().height);
  expect(saveHeight).toBeCloseTo(controlHLg, 0);
});

// D1's no-op guard: the role layer is a pure alias, so at the 12px default every family reports
// exactly the pre-P22 value it always has.
test('the role layer changes no rendered control height at the default font size', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: FIXTURE.port });
  await connectAndOpenGrid(page);
  await page.click('[data-testid="open-settings"]');
  await expect(page.locator('[data-testid="settings-dialog"]')).toBeVisible();

  const pInputHeight = await page
    .locator('[data-testid="pager-page-input"]')
    .evaluate((el) => el.getBoundingClientRect().height);
  expect(pInputHeight).toBeCloseTo(22, 0);

  const pIconbtnHeight = await page
    .locator('[data-testid="pager-first"]')
    .evaluate((el) => el.getBoundingClientRect().height);
  expect(pIconbtnHeight).toBeCloseTo(22, 0);

  const pDlgbtnHeight = await page
    .locator('[data-testid="settings-save"]')
    .evaluate((el) => el.getBoundingClientRect().height);
  expect(pDlgbtnHeight).toBeCloseTo(26, 0);

  const pChipHeight = await page
    .locator('[data-testid="grid-pk-chip"]')
    .evaluate((el) => el.getBoundingClientRect().height);
  expect(pChipHeight).toBeCloseTo(18, 0);
});

// F2(c)/D1: the identity band (.p-view-head, via ViewHeader) and the toolbar directly beneath it
// (.p-toolbar) used to disagree by 6px — every view opened with a 28px band above a 34px bar. D1
// closes the split; both now report --kira-bar-h.
test('the view-head band and the toolbar beneath it report the same height (F2(c) closed)', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: FIXTURE.port });
  await connectAndOpenGrid(page);

  const barH = await rootVar(page, '--kira-bar-h');
  expect(barH).toBeGreaterThan(0);

  const viewHeadHeight = await page
    .locator('[data-testid="grid-target"]')
    .evaluate((el) => el.closest('[data-testid="view-head"]')?.getBoundingClientRect().height);
  expect(viewHeadHeight).toBeCloseTo(barH, 0);

  const toolbarHeight = await page
    .locator('[data-testid="data-toolbar"]')
    .evaluate((el) => el.getBoundingClientRect().height);
  expect(toolbarHeight).toBeCloseTo(barH, 0);
});

// P117 A1/S5: every pane-switcher ToggleGroup moved to size="kira" so it renders at
// --kira-control-h alongside its bar's other controls, instead of the shorter stock toggle size.
test('the HTTP request pane toggle sits at --kira-control-h (P117 A1)', async ({ relaunch }) => {
  const { window: page } = await relaunch();
  await openHttpModeAndNewRequest(page);
  await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();

  const controlH = await rootVar(page, '--kira-control-h');
  expect(controlH).toBeGreaterThan(0);

  const height = await page
    .locator('[data-testid="http-request-pane-params"]')
    .evaluate((el) => el.getBoundingClientRect().height);
  expect(height).toBeCloseTo(controlH, 0);
});

// P117 A2: EnvironmentsView's filter InputGroup moved to variant="kira" -- stock rendered at 32px
// under this view's 22px row chrome.
test('the environments filter sits at --kira-control-h, and New environment never clips (P117 A2)', async ({
  relaunch,
}) => {
  const ENVIRONMENTS = [
    { id: 'env-1', name: 'Prod', sortOrder: 0, isActive: true, color: 'green' },
  ];
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: { collections: [], items: [] } },
    { channel: IPC.variablesListEnvironments, response: ENVIRONMENTS },
  ];
  const { window: page } = await relaunch({ control: CONTROL });
  await modeTab(page, 'api').click();
  await page.click('[data-testid="api-environments"]');
  await expect(page.locator('[data-testid="environments-dialog"]')).toBeVisible();

  const controlH = await rootVar(page, '--kira-control-h');
  const filterHeight = await page
    .locator('[data-testid="environments-filter"]')
    .locator('xpath=ancestor::fieldset[@data-slot="search-field"][1]')
    .evaluate((el) => el.getBoundingClientRect().height);
  expect(filterHeight).toBeCloseTo(controlH, 0);

  const newEnvBox = await page.locator('[data-testid="new-environment"]').boundingBox();
  const dialogBox = await page.locator('[data-testid="environments-dialog"]').boundingBox();
  if (newEnvBox === null || dialogBox === null) {
    throw new Error('expected both boxes to be measurable');
  }
  expect(newEnvBox.x + newEnvBox.width).toBeLessThanOrEqual(dialogBox.x + dialogBox.width + 1);
});

// P122 §6.2: the reference value itself -- the SQL data view's WHERE filter, `InputGroup
// variant="kira"`'s own `focus-within:focus-ring`. Unchanged by this phase; asserted here as the
// baseline every other row below is measured against.
test('the SQL filter input renders the reference 1px focus ring (P122)', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: FIXTURE.port });
  await connectAndOpenGrid(page);

  await page.click('[data-testid="filter-where-input"]');
  await expectFocusRing(
    page
      .locator('[data-testid="filter-where-input"]')
      .locator('xpath=ancestor::fieldset[@data-slot="input-group"][1]'),
  );
});

// P122 §6.2: stock `Input` (T1) -- used to paint a 3px 50%-alpha box-shadow halo on top of its own
// border; now inherits the shared base `:focus-visible` rule instead.
test('a stock Input renders the shared 1px focus ring, not the old halo (P122)', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: FIXTURE.port });
  await connectAndOpenGrid(page);

  await page.click('[data-testid="pager-page-input"]');
  await expectFocusRing(page.locator('[data-testid="pager-page-input"]'));
});

// P122 §6.2: InputGroup `default` (T3) -- the number-stepper box's own `has-[...]:focus-visible:
// ring-3` halo, now `has-[...]:focus-visible:focus-ring`.
test('the InputGroup default box renders the shared 1px focus ring, not the old halo (P122)', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch();
  await page.click('[data-testid="open-settings"]');
  await expect(page.locator('[data-testid="settings-dialog"]')).toBeVisible();

  await page.click('[data-testid="settings-font-size"]');
  await expectFocusRing(
    page
      .locator('[data-testid="settings-font-size"]')
      .locator('xpath=ancestor::fieldset[@data-slot="input-group"][1]'),
  );
});

// P122 §6.2: `Button` (T4) -- keyboard-focus only (a mouse click never matches :focus-visible on a
// button), the shadcn registry's own ring-3 halo replaced by the shared base rule.
test('a Button renders the shared 1px focus ring on keyboard focus, not the old halo (P122)', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: FIXTURE.port });
  await connectAndOpenGrid(page);

  await focusViaTabFrom(page, 'toolbar-add-row', 'toolbar-search');
  await expectFocusRing(page.locator('[data-testid="toolbar-search"]'));
});

// P122 §6.2/U1: a raw <button> (SettingsShell.vue's own section nav) carried no outline utility at
// all, so it fell back to the browser's own UA focus ring (WebKit: outline: auto 5px
// -webkit-focus-ring-color) -- now painted by the same shared base rule, no template edit needed.
test('a raw <button> with no outline utility renders the shared 1px focus ring (P122)', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch();
  await page.click('[data-testid="open-settings"]');
  await expect(page.locator('[data-testid="settings-dialog"]')).toBeVisible();

  // Reka UI's own dialog auto-focus lands on the content itself, so the first Tab already reaches
  // the section nav's first button -- no anchor to click first.
  await focusViaTabFrom(page, null, 'settings-section-Appearance');
  await expectFocusRing(page.locator('[data-testid="settings-section-Appearance"]'));
});

test('the page-size picker renders as one connected segmented row (P121 S23, P262)', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: FIXTURE.port });
  await connectAndOpenGrid(page);

  const items = [
    page.locator('[data-testid="page-size-10"]'),
    page.locator('[data-testid="page-size-100"]'),
    page.locator('[data-testid="page-size-1000"]'),
    page.locator('[data-testid="page-size-10000"]'),
  ];
  // Segmented picker (P262): one connected row, only the outer corners rounded.
  const radii = await Promise.all(
    items.map((item) => item.evaluate((el) => getComputedStyle(el).borderRadius)),
  );
  expect(radii[0]).toBe('4px 0px 0px 4px');
  expect(radii[1]).toBe('0px');
  expect(radii[2]).toBe('0px');
  expect(radii[3]).toBe('0px 4px 4px 0px');
  const [firstBox, secondBox] = await Promise.all([items[0].boundingBox(), items[1].boundingBox()]);
  if (!firstBox || !secondBox) throw new Error('expected both page-size chips to be measurable');
  expect(Math.abs(secondBox.x - (firstBox.x + firstBox.width))).toBeLessThanOrEqual(1);
});

// P121 §6.2: same shape as S23, a different default-variant ToggleGroup (P117 A1's own "kira"
// size, unaffected by this phase's height/font work -- only radius/spacing changed).
test('the HTTP request pane toggle items are rounded, at 22px (P121 S14)', async ({ relaunch }) => {
  const { window: page } = await relaunch();
  await openHttpModeAndNewRequest(page);
  await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();

  for (const testid of [
    'http-request-pane-params',
    'http-request-pane-headers',
    'http-request-pane-body',
    'http-request-pane-settings',
    'http-request-pane-cookies',
  ]) {
    const item = page.locator(`[data-testid="${testid}"]`);
    const style = await item.evaluate((el) => ({
      radius: getComputedStyle(el).borderRadius,
      height: el.getBoundingClientRect().height,
    }));
    expect(style.radius).toBe('4px');
    expect(style.height).toBeCloseTo(22, 0);
  }
});

// The row-density ToggleGroup is `variant="outline"`, so it stays connected (spacing 0,
// the KuiSegmented precedent) -- only its two outer corners round, and the shared inner edge
// keeps a single 1px divider (the second/last item's own border-left drops to 0, rather than
// doubling the first item's border-right).
test('the row-density ToggleGroup keeps a connected outline with 4px outer corners (P121 S4)', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch();
  await page.click('[data-testid="open-settings"]');
  await expect(page.locator('[data-testid="settings-dialog"]')).toBeVisible();

  const compact = page.locator('[data-testid="settings-appearance-rowDensity-compact"]');
  const comfortable = page.locator('[data-testid="settings-appearance-rowDensity-comfortable"]');

  const compactRadii = await compact.evaluate((el) => {
    const cs = getComputedStyle(el);
    return { tl: cs.borderTopLeftRadius, tr: cs.borderTopRightRadius };
  });
  expect(compactRadii.tl).toBe('4px');
  expect(compactRadii.tr).toBe('0px');

  const comfortableRadii = await comfortable.evaluate((el) => {
    const cs = getComputedStyle(el);
    return { tl: cs.borderTopLeftRadius, tr: cs.borderTopRightRadius };
  });
  expect(comfortableRadii.tl).toBe('0px');
  expect(comfortableRadii.tr).toBe('4px');

  expect(await comfortable.evaluate((el) => getComputedStyle(el).borderLeftWidth)).toBe('0px');
});

// The console's own result chips ("Result 1", "Result 2") were the one tab-chip copy
// off the shared scale (22px/10px, a 4px strip gap) -- now tab-strip size (26px/12px/2px,
// 12px not the original 11px: tabChipVariants moved onto the control-text role).
test('console result tabs render at tab-strip size, 2px strip gap (P121 T4)', async ({
  relaunch,
}) => {
  const CONNECTION_ID = 'conn-p121-console';
  const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Console Sizing DB', 'blue');
  const CONSOLE_FIXTURE = orderItemsFixture(CONNECTION_ID);
  const CONSOLE_CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: connectionCreateArgs('Console Sizing DB', 'blue'),
      response: CONNECTION_SUMMARY,
    },
    ...CONSOLE_FIXTURE.control,
  ];
  const CONSOLE_PORT: PortSnapshot[] = [
    ...CONSOLE_FIXTURE.port,
    executeSnap(CONNECTION_ID, ['SELECT 1 AS n'], [numberPage('n', '1')]),
    executeSnap(CONNECTION_ID, ['SELECT 2 AS n'], [numberPage('n', '2')]),
  ];

  const { window: page } = await relaunch({ control: CONSOLE_CONTROL, stream: CONSOLE_PORT });
  await connectAndExpand(page, 'Console Sizing DB', 'blue');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);

  const consoleView = page.locator('[data-testid="console-view"]');
  await typeInto(consoleView, page, 'SELECT 1 AS n;');
  await page.click('[data-testid="console-run-statement"]');
  await expect(consoleView.locator('[data-testid="console-result-tab"]')).toHaveCount(1);

  await typeInto(consoleView, page, '\nSELECT 2 AS n;');
  await page.click('[data-testid="console-run-statement"]');
  const resultTabs = consoleView.locator('[data-testid="console-result-tab"]');
  await expect(resultTabs).toHaveCount(2);

  const style = await resultTabs.nth(0).evaluate((el) => ({
    height: el.getBoundingClientRect().height,
    fontSize: getComputedStyle(el).fontSize,
  }));
  expect(style.height).toBeCloseTo(26, 0);
  expect(style.fontSize).toBe('12px');

  const [firstBox, secondBox] = await Promise.all([
    resultTabs.nth(0).boundingBox(),
    resultTabs.nth(1).boundingBox(),
  ]);
  if (!firstBox || !secondBox) throw new Error('expected both result tabs to be measurable');
  expect(secondBox.x - (firstBox.x + firstBox.width)).toBeCloseTo(2, 0);
});

// P121 §6.2: T1's own `pt-0.5` removal -- the scrolling tab now centres in its 34px `h-tabbar`
// bar (`tab-strip`, minus its own 1px `border-b`) instead of sitting 2px low.
test('the first document tab sits vertically centred in the tab-strip bar (P121 T1)', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: FIXTURE.port });
  await connectAndOpenGrid(page);

  const bar = page.locator('[data-testid="tab-strip"]');
  const firstTab = page.locator('[data-testid="tab-strip-row"] [data-testid="tab"]').first();

  const [barBox, tabBox] = await Promise.all([bar.boundingBox(), firstTab.boundingBox()]);
  if (!barBox || !tabBox)
    throw new Error('expected both the bar and its first tab to be measurable');

  const barBottomInterior = barBox.y + barBox.height - 1; // 1px border-b
  const topGap = tabBox.y - barBox.y;
  const bottomGap = barBottomInterior - (tabBox.y + tabBox.height);
  expect(Math.abs(topGap - bottomGap)).toBeLessThanOrEqual(0.5);
});

function computedFontSize(el: Locator): Promise<number> {
  return el.evaluate((node) => Number.parseFloat(getComputedStyle(node).fontSize));
}

// The four-value chrome type scale itself, read from the live --kira-t-sm/md/lg
// custom properties (not a hard-coded pixel value, since Data font size can move what "12px"
// means) and asserted on one representative of each role. Button/tab/Label are the changed
// rows -- 11px (sm) before, 12px (md) after. Status-bar item and the dialog title were already
// on their assigned role and stay unchanged.
test('the chrome type scale renders --kira-t-sm/md/lg on their assigned roles (P123)', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: FIXTURE.port });
  await connectAndOpenGrid(page);

  const sm = await rootVar(page, '--kira-t-sm');
  const md = await rootVar(page, '--kira-t-md');
  const lg = await rootVar(page, '--kira-t-lg');
  expect(sm).toBeGreaterThan(0);
  expect(md).toBeGreaterThan(sm);
  expect(lg).toBeGreaterThan(md);

  // FilterToolbar's "Clear" button (size="kira") -- control text, §1.2's md role. No testid on the
  // button itself, so located by role and name as the plan states.
  const clearSize = await computedFontSize(
    page.getByRole('button', { name: 'Clear', exact: true }),
  );
  expect(clearSize).toBeCloseTo(md, 1);

  // The workbench tab this grid opened -- control text, md.
  const tabSize = await computedFontSize(page.locator('[data-testid="tab"]').first());
  expect(tabSize).toBeCloseTo(md, 1);

  await page.click('[data-testid="open-settings"]');
  const settingsDialog = page.locator('[data-testid="settings-dialog"]');
  await expect(settingsDialog).toBeVisible();

  // The Label for settings-font-size -- a label, md.
  const labelSize = await computedFontSize(
    settingsDialog.locator('label').filter({ hasText: 'Data font size' }),
  );
  expect(labelSize).toBeCloseTo(md, 1);

  // The dialog title -- a heading, lg. Already lg before this phase (DialogTitle's base was never
  // touched, §3.2); asserted here as the "unchanged" half of the proof.
  const titleSize = await computedFontSize(settingsDialog.locator('[data-slot="dialog-title"]'));
  expect(titleSize).toBeCloseTo(lg, 1);

  await page.click('[data-testid="settings-dialog-close"]');
  await expect(settingsDialog).toHaveCount(0);
});

// P123 §6.3 Test 2: data stays customizable -- chrome moving onto a fixed four-value scale must
// not stop the SQL grid from tracking the user's own Data font size setting. Unchanged by this
// phase; asserted here as the "data views are untouched" proof, expected to pass at both commits.
test('the SQL grid still tracks Data font size after the chrome scale lands (P123)', async ({
  relaunch,
}) => {
  // mockRuntime.ts echoes IPC.settingsSet back as the untouched default unless a spec scripts its
  // own response (settings-apply-on-save.spec.ts's own precedent) -- this scripts the one leaf this
  // test changes.
  const savedSettings = {
    ...defaultSettings,
    appearance: { ...defaultSettings.appearance, fontSize: 16 },
  };
  const { window: page } = await relaunch({
    control: [...CONTROL, { channel: IPC.settingsSet, response: savedSettings }],
    stream: FIXTURE.port,
  });
  await connectAndOpenGrid(page);

  await page.click('[data-testid="open-settings"]');
  const settingsDialog = page.locator('[data-testid="settings-dialog"]');
  await expect(settingsDialog).toBeVisible();

  await page.locator('[data-testid="settings-font-size"]').fill('16');
  await page.click('[data-testid="settings-save"]');
  await expect(settingsDialog).toHaveCount(0);

  // `.kira-gutter` is also a `.slick-cell` (the row-number column) and keeps its own fixed
  // `--kira-t-xs` size regardless of Data font size (§3.8) -- excluded so this reads an actual data
  // cell.
  const cellSize = await computedFontSize(page.locator('.slick-cell:not(.kira-gutter)').first());
  expect(cellSize).toBeCloseTo(16, 0);

  // slickTheme.css's own header rule names var(--kira-t-sm) directly -- one step below the 16px
  // base, same -1 derivation as the chrome scale (§1.3), never a fixed value of its own.
  const headerSize = await computedFontSize(page.locator('.slick-header-column').first());
  expect(headerSize).toBeCloseTo(15, 0);
});
