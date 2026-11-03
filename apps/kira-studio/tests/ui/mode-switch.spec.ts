import type { Locator, Page } from '@playwright/test';
import type { ControlSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { modeTab } from './support/apiMode';
import { installFakeTimers } from './support/clock';
import { createConnection } from './support/connect';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import {
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';

// The mode seam, observed from outside. Studio's own connect/expand/open flow is
// ported straight from tabs.spec.ts's own createAndConnect (Studio's rendered output doesn't
// change) — what's new here is Http mode existing at all, and the five properties: two mode tabs;
// Http is genuinely empty with its own left-panel title; switching back restores Studio's tab
// untouched; switching mode writes nothing; the left panel's width survives; ⌘B still works
// in either mode.

const CONNECTION_ID = 'conn-mode-switch';
const FIXTURE = orderItemsFixture(CONNECTION_ID);
const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Mode DB', 'blue');

// MODE_ORDER cannot be imported across the web/node tsconfig boundary; the DOM is the source.
const KNOWN_MODES = ['studio', 'api', 'automations', 'docker'];
const KNOWN_NAMES: Record<string, string> = {
  studio: 'Studio',
  api: 'API',
  automations: 'Runs',
  docker: 'Docker',
};

function anyModeTab(page: Page, mode: string): Locator {
  return page.locator(`[data-testid="mode-tab"][data-mode="${mode}"]`);
}

async function renderedModes(page: Page): Promise<string[]> {
  return page
    .locator('[data-testid="mode-tab"]')
    .evaluateAll((els) => els.map((e) => e.getAttribute('data-mode') ?? ''));
}

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: {
      name: 'Mode DB',
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

async function createAndConnect(page: import('@playwright/test').Page): Promise<void> {
  await createConnection(page, 'postgres', 'Mode DB');

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

test('mode switch — the mode tabs, an empty Http mode, and Studio state that survives the round trip', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: CONTROL });

  // 1. the known mode tabs in order (count-agnostic: further modes may follow), Studio active by default.
  const rendered = await renderedModes(page);
  expect(rendered.filter((m) => KNOWN_MODES.includes(m))).toEqual(KNOWN_MODES);
  await expect(modeTab(page, 'studio')).toHaveClass(/is-active/);
  await expect(modeTab(page, 'api')).not.toHaveClass(/is-active/);
  await expect(page.locator('[data-testid="project-panel"]')).toContainText('Connections');

  // Build a real Studio tab with non-default state (page size 1000) to prove it survives.
  await createAndConnect(page);
  const orderItemsRow = await findRow(page, ORDER_ITEMS_PATH);
  await orderItemsRow.dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await page.click('[data-testid="page-size-1000"]');
  await expect(page.locator('[data-testid="page-size-1000"]')).toHaveAttribute('data-state', 'on');

  const studioTab = page.locator('[data-testid="tab"]');
  await expect(studioTab).toHaveCount(1);
  const studioTabId = await studioTab.getAttribute('data-tab-id');

  // The 1s-debounced tabsSave for the page-size change must land before the baseline, or it
  // counts as a save caused by the mode switch under load.
  await expect
    .poll(() =>
      control
        .log()
        .some(
          (entry) =>
            entry.channel === IPC.tabsSave &&
            (entry.args as { tabs: { state: { pageSize?: number } }[] }).tabs.some(
              (t) => t.state.pageSize === 1000,
            ),
        ),
    )
    .toBe(true);

  const panelWidthBefore = (await page.locator('[data-testid="project-panel"]').boundingBox())
    ?.width;
  const tabsSaveCallsBeforeSwitch = control
    .log()
    .filter((entry) => entry.channel === IPC.tabsSave).length;

  // 2. clicking Http shows the empty tab-strip state and Http's own empty content, with the left
  //    panel no longer titled "Connections".
  await modeTab(page, 'api').click();
  await expect(modeTab(page, 'api')).toHaveClass(/is-active/);
  await expect(modeTab(page, 'studio')).not.toHaveClass(/is-active/);
  await expect(page.locator('[data-testid="tab-strip-empty"]')).toBeVisible();
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="api-start"]')).toBeVisible();
  await expect(page.locator('[data-testid="project-panel"]')).not.toContainText('Connections');
  await expect(page.locator('[data-testid="project-panel"]')).toContainText('Collections');

  // "mode switching writes nothing" (D5), observed: no tabsSave call happened just from the two
  // clicks above — clicking a mode tab is a plain selection, not a mutation.
  const tabsSaveCallsAfterSwitch = control
    .log()
    .filter((entry) => entry.channel === IPC.tabsSave).length;
  expect(tabsSaveCallsAfterSwitch).toBe(tabsSaveCallsBeforeSwitch);

  // 3. clicking Studio restores the same tab, still active, with its state intact.
  await modeTab(page, 'studio').click();
  await expect(modeTab(page, 'studio')).toHaveClass(/is-active/);
  const restoredTab = page.locator('[data-testid="tab"]');
  await expect(restoredTab).toHaveCount(1);
  await expect(restoredTab).toHaveAttribute('data-tab-id', studioTabId ?? '');
  await expect(restoredTab).toHaveClass(/is-active/);
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await expect(page.locator('[data-testid="page-size-1000"]')).toHaveAttribute('data-state', 'on');

  // 4. the left panel's width is preserved across the switch (P312: each mode keeps its own width).
  const panelWidthAfter = (await page.locator('[data-testid="project-panel"]').boundingBox())
    ?.width;
  expect(panelWidthAfter).toBe(panelWidthBefore);

  // 5. ⌘B still collapses/expands the panel in either mode. There is no real Wails window in
  //    tests/ui to dispatch the native ⌘B menu accelerator through, so this drives the status
  //    bar's own toggle button — the exact same toggleProjectPanel() the accelerator itself
  //    invokes (App.vue's control.onToggleProjectPanel(toggleProjectPanel)).
  await page.click('[data-testid="toggle-project-panel"]');
  await expect(page.locator('[data-testid="project-panel"]')).toHaveCount(0);
  await page.click('[data-testid="toggle-project-panel"]');
  await expect(page.locator('[data-testid="project-panel"]')).toBeVisible();

  await modeTab(page, 'api').click();
  await page.click('[data-testid="toggle-project-panel"]');
  await expect(page.locator('[data-testid="project-panel"]')).toHaveCount(0);
  await page.click('[data-testid="toggle-project-panel"]');
  await expect(page.locator('[data-testid="project-panel"]')).toBeVisible();
  await expect(page.locator('[data-testid="project-panel"]')).toContainText('Collections');
});

// A window remembers which module it was closed in and reopens into it. This
// tier's own relaunch() mocks both wire protocols from scratch each call (fixtures.ts's own header
// comment) — there is nothing to persist to, so a real write-then-reboot round trip cannot be
// proven here. What can: (a) boot honours whatever mode windowsEnsure's own snapshot answers with
// (the hydration half — real cross-process persistence is Go's own windows_test.go/migration
// coverage), and (b) switching mode eventually reaches windowsSetMode, debounced rather than
// synchronous (the invariant — the mode click itself schedules no tabsSave, still proven
// unchanged by the existing case above).
test('a window boots into whatever mode windowsEnsure answers with (P22 D12)', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.windowsEnsure, response: { mode: 'api' } }],
  });

  await expect(modeTab(page, 'api')).toHaveClass(/is-active/);
  await expect(modeTab(page, 'studio')).not.toHaveClass(/is-active/);
  await expect(page.locator('[data-testid="api-start"]')).toBeVisible();
});

test('a window-close flush saves pending tab state at once, then acks (P168 Part 13 F21)', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: CONTROL });
  await createAndConnect(page);
  await (await findRow(page, ORDER_ITEMS_PATH)).dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await page.click('[data-testid="page-size-1000"]');
  await expect(page.locator('[data-testid="page-size-1000"]')).toHaveAttribute('data-state', 'on');

  // A save carrying the new page size must land before the ack; the debounce may or may not have
  // fired first, so no exact count.
  await emitWailsEvent(page, IPC.windowFlushBeforeClose, {});
  await expect.poll(() => control.log().some((e) => e.channel === IPC.windowFlushed)).toBe(true);
  const log = control.log();
  const ackAt = log.findIndex((e) => e.channel === IPC.windowFlushed);
  const saveAt = log.findIndex(
    (e, i) =>
      i < ackAt &&
      e.channel === IPC.tabsSave &&
      (e.args as { tabs: { state: { pageSize?: number } }[] }).tabs.some(
        (t) => t.state.pageSize === 1000,
      ),
  );
  expect(saveAt).toBeGreaterThanOrEqual(0);
});

test('switching mode reaches windowsSetMode eventually, never synchronously (P22 D12/F20)', async ({
  relaunch,
}) => {
  // state/mode.ts's own constant — restated here rather than imported across the web/node
  // tsconfig boundary (this suite's existing convention, see `global.d.ts:1`-`5`).
  const MODE_WRITE_DEBOUNCE_MS = 150;
  const { window: page, control } = await relaunch({ control: [] });
  await installFakeTimers(page); // support/clock.ts — after boot, never before

  const setModeCalls = () => control.log().filter((e) => e.channel === IPC.windowsSetMode);

  await modeTab(page, 'api').click();
  // Nothing yet — the click itself must not fire a synchronous IPC (F20's own invariant). Time
  // has not moved, so this isn't a wall-clock budget: the debounce *cannot* have elapsed, however
  // slowly the harness's own click landed (P81 §3.3/§6 — same shape as grpc-request's).
  expect(setModeCalls()).toHaveLength(0);
  await page.clock.runFor(MODE_WRITE_DEBOUNCE_MS - 1);
  expect(setModeCalls()).toHaveLength(0);
  await page.clock.runFor(2);
  await expect.poll(() => setModeCalls().length).toBe(1);
  expect(setModeCalls()[0]?.args).toMatchObject({ mode: 'api' });

  // Switching again before the first write's debounce would have fired collapses to the latest
  // value, the same coalescing behaviour state/layout.ts's own patchLayout has.
  await modeTab(page, 'studio').click();
  await modeTab(page, 'api').click();
  await page.clock.runFor(MODE_WRITE_DEBOUNCE_MS + 1);
  await expect.poll(() => setModeCalls().length).toBe(2);
  expect(setModeCalls()[1]?.args).toMatchObject({ mode: 'api' });
});

test('every mode tab is icon-only, named, with a tooltip', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: [] });

  const modes = await renderedModes(page);
  expect(modes.length).toBeGreaterThanOrEqual(KNOWN_MODES.length);
  for (const mode of modes) {
    const tab = anyModeTab(page, mode);
    expect((await tab.innerText()).trim()).toBe('');
    await expect(tab).toHaveAccessibleName(/\S/);
    const expected = KNOWN_NAMES[mode];
    if (expected) await expect(tab).toHaveAccessibleName(expected);
  }

  await modeTab(page, 'automations').hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText('Runs');
  await expect(modeTab(page, 'studio')).toHaveClass(/is-active/);
  await expect(modeTab(page, 'studio')).toHaveAttribute('aria-pressed', 'true');
});

// Ink measurement: screenshot the icon, find its painted pixels (differing from its own sampled
// background) rather than trust the fixed box it sits in, and compare the ink itself.
async function inkBounds(
  locator: Locator,
): Promise<{ top: number; bottom: number; left: number; right: number } | null> {
  const box = await locator.boundingBox();
  if (!box) return null;
  const base64 = (await locator.screenshot()).toString('base64');
  const rel = await locator.page().evaluate(async (b64) => {
    const img = new Image();
    const loaded = new Promise<void>((resolve, reject) => {
      img.onload = () => resolve();
      img.onerror = () => reject(new Error('mode-tab ink screenshot failed to decode'));
    });
    img.src = `data:image/png;base64,${b64}`;
    await loaded;
    const canvas = document.createElement('canvas');
    canvas.width = img.naturalWidth;
    canvas.height = img.naturalHeight;
    const ctx = canvas.getContext('2d');
    if (!ctx) throw new Error('mode-tab ink measurement: no 2d context');
    ctx.drawImage(img, 0, 0);
    const { data, width, height } = ctx.getImageData(0, 0, canvas.width, canvas.height);
    // The crop's own corner pixel is its background — the icon box paints no fill of its own, so this
    // is whatever sits behind it (the tab's own ground).
    const bg = [data[0], data[1], data[2]];
    const THRESHOLD = 24; // per-channel delta that counts as "ink", tolerant of anti-aliasing
    let minX = Number.POSITIVE_INFINITY;
    let maxX = Number.NEGATIVE_INFINITY;
    let minY = Number.POSITIVE_INFINITY;
    let maxY = Number.NEGATIVE_INFINITY;
    for (let y = 0; y < height; y++) {
      for (let x = 0; x < width; x++) {
        const i = (y * width + x) * 4;
        const dr = Math.abs(data[i] - bg[0]);
        const dg = Math.abs(data[i + 1] - bg[1]);
        const db = Math.abs(data[i + 2] - bg[2]);
        if (dr > THRESHOLD || dg > THRESHOLD || db > THRESHOLD) {
          if (x < minX) minX = x;
          if (x > maxX) maxX = x;
          if (y < minY) minY = y;
          if (y > maxY) maxY = y;
        }
      }
    }
    if (minX === Number.POSITIVE_INFINITY) return null;
    return { minX, maxX, minY, maxY, width, height };
  }, base64);
  if (!rel) return null;
  const scaleX = box.width / rel.width;
  const scaleY = box.height / rel.height;
  return {
    left: box.x + rel.minX * scaleX,
    right: box.x + (rel.maxX + 1) * scaleX,
    top: box.y + rel.minY * scaleY,
    bottom: box.y + (rel.maxY + 1) * scaleY,
  };
}

async function modeTabInk(
  page: Page,
  mode: string,
): Promise<{ centreOffsetX: number; centreOffsetY: number; iconRightInset: number }> {
  const tab = anyModeTab(page, mode);
  const tabBox = await tab.boundingBox();
  const iconBoxLocator = tab.locator('[data-testid="mode-tab-icon"]');
  const iconBox = await iconBoxLocator.boundingBox();
  const iconInk = await inkBounds(iconBoxLocator);
  if (!tabBox || !iconBox || !iconInk) {
    throw new Error(`mode tab "${mode}" has no measurable icon ink`);
  }
  return {
    centreOffsetX: Math.abs((iconInk.left + iconInk.right) / 2 - (tabBox.x + tabBox.width / 2)),
    centreOffsetY: Math.abs((iconInk.top + iconInk.bottom) / 2 - (tabBox.y + tabBox.height / 2)),
    // Icon box trailing edge to the glyph's rightmost ink: how much of the 16px box the glyph fills.
    iconRightInset: iconBox.x + iconBox.width - iconInk.right,
  };
}

test('a mode tab’s icon renders at its own design size, centred in its tab (P22 D5/D6, P438)', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: [] });

  for (const mode of await renderedModes(page)) {
    const { centreOffsetX, centreOffsetY, iconRightInset } = await modeTabInk(page, mode);
    expect(iconRightInset).toBeLessThanOrEqual(3.5);
    // Tabs are icon-only: the glyph's ink sits at the tab box centre on both axes.
    expect(centreOffsetX).toBeLessThanOrEqual(2.5);
    expect(centreOffsetY).toBeLessThanOrEqual(2.5);
  }
});
