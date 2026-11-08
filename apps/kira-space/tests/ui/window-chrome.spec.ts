import type { AppMetricsSample } from '@shared/protocol/events';
import { expect, test } from './fixtures';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// The sandbox-runnable proof of window-chrome parity — mirroring Kira Studio's own
// tests/ui/workbench.spec.ts:62-140 (New window button, keep-awake toggle, both directions) plus
// the app-metrics readout and the five menu-pushed commands this app's own Go menu now emits
// (internal/appshell/menu.go). Two unscoped terminal tabs stand in for "the tab set on screen" —
// GENERAL_WORKSPACE's own shared slot (state/workspace.ts), the cheapest tab kind that needs no
// imported repository and isn't pinned (state/tabKinds.ts has no `pinned: true` for 'terminal').
const TAB_A = {
  id: 'tab-a',
  connectionId: null,
  path: '',
  order: 0,
  active: true,
  workspaceId: null,
  kind: 'terminal',
  state: {
    cwd: '/tmp/a',
    codeRepoId: '',
    command: '',
    label: 'Tab A',
    color: 'none',
    launchKind: 'shell',
  },
};
const TAB_B = {
  id: 'tab-b',
  connectionId: null,
  path: '',
  order: 1,
  active: false,
  workspaceId: null,
  kind: 'terminal',
  state: {
    cwd: '/tmp/b',
    codeRepoId: '',
    command: '',
    label: 'Tab B',
    color: 'none',
    launchKind: 'shell',
  },
};
const TWO_TABS: ControlSnapshot[] = [{ channel: IPC.tabsList, response: [TAB_A, TAB_B] }];

test('the title bar has a New window button, rightmost of the action row, that calls WindowsService.OpenNew once per click', async ({
  relaunch,
}) => {
  const { window, control } = await relaunch({
    control: [{ channel: IPC.windowsOpenNew, response: null }],
  });
  const newWindow = window.locator('[data-testid="new-window"]');
  await expect(newWindow).toBeVisible();
  await expect(newWindow).toContainText('New window');

  // DOM order: Repositories, Operations, Settings, keep-awake, New window (TitleBar.vue's own
  // render order).
  const testIds = await window
    .locator(
      '[data-testid="toggle-project-panel"], [data-testid="toggle-operations-panel"], ' +
        '[data-testid="open-settings"], [data-testid="toggle-keep-awake"], ' +
        '[data-testid="new-window"]',
    )
    .evaluateAll((els) => els.map((el) => el.getAttribute('data-testid')));
  expect(testIds).toEqual([
    'toggle-project-panel',
    'toggle-operations-panel',
    'open-settings',
    'toggle-keep-awake',
    'new-window',
  ]);

  const openNewCalls = () => control.log().filter((e) => e.channel === IPC.windowsOpenNew);
  expect(openNewCalls()).toHaveLength(0);
  await newWindow.click();
  await expect.poll(() => openNewCalls().length).toBe(1);
});

test('the title bar shows Kira Space right after the window controls, inside the drag region', async ({
  relaunch,
}) => {
  const { window } = await relaunch();
  const title = window.locator('[data-testid="titlebar-app-title"]');
  await expect(title).toHaveText('Kira Space');

  const info = await title.evaluate((el) => {
    const bar = el.parentElement as HTMLElement;
    return {
      first: bar.firstElementChild === el,
      offset: el.getBoundingClientRect().left - bar.getBoundingClientRect().left,
      inset: parseFloat(getComputedStyle(bar).paddingLeft),
      drag: getComputedStyle(el).getPropertyValue('--wails-draggable').trim(),
    };
  });
  expect(info.first).toBe(true);
  expect(info.inset).toBe(78);
  expect(info.offset).toBe(info.inset);
  expect(info.drag).toBe('drag');

  // Window floor: title, mode switcher and action row must not overlap or clip the title.
  await window.setViewportSize({ width: 1024, height: 640 });
  const box = async (sel: string, parent: boolean) =>
    window
      .locator(sel)
      .first()
      .evaluate((el, up) => {
        const r = ((up ? el.parentElement : el) as HTMLElement).getBoundingClientRect();
        return { left: r.left, right: r.right };
      }, parent);
  const titleBox = await box('[data-testid="titlebar-app-title"]', false);
  const switcher = await box('[data-testid="mode-tab"]', true);
  const actions = await box('[data-testid="toggle-project-panel"]', true);
  expect(titleBox.right).toBeLessThanOrEqual(switcher.left);
  expect(switcher.right).toBeLessThanOrEqual(actions.left);
  expect(await title.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
});

test('the keep-awake button is hidden when unsupported, shown and toggleable when supported', async ({
  relaunch,
}) => {
  const unsupported = await relaunch({
    control: [
      { channel: IPC.keepAwakeStatus, response: { manual: false, supported: false, error: '' } },
    ],
  });
  await expect(unsupported.window.locator('[data-testid="toggle-keep-awake"]')).toHaveCount(0);

  const { window, control } = await relaunch({
    control: [
      {
        channel: IPC.keepAwakeSetManual,
        response: { manual: true, supported: true, error: '' },
      },
    ],
  });
  const button = window.locator('[data-testid="toggle-keep-awake"]');
  await expect(button).toBeVisible();
  await expect(button).toHaveAttribute('aria-pressed', 'false');

  const setManualCalls = () => control.log().filter((e) => e.channel === IPC.keepAwakeSetManual);
  await button.click();
  await expect.poll(() => setManualCalls().length).toBe(1);
  expect(setManualCalls()[0]?.args).toEqual({ enabled: true });
  await expect(button).toHaveAttribute('aria-pressed', 'true');
});

test('the keep-awake button turns on from a pushed kira:keepAwake:changed event with no click', async ({
  relaunch,
}) => {
  const { window } = await relaunch();
  const button = window.locator('[data-testid="toggle-keep-awake"]');
  await expect(button).toHaveAttribute('aria-pressed', 'false');

  await emitWailsEvent(window, IPC.keepAwake, { manual: true, supported: true, error: '' });

  await expect(button).toHaveAttribute('aria-pressed', 'true');
});

test('a pushed kira:app:metrics sample renders CPU and memory in the status bar', async ({
  relaunch,
}) => {
  const { window } = await relaunch();
  await expect(window.locator('[data-testid="app-metrics"]')).toHaveCount(0);

  const sample: AppMetricsSample = {
    cpuPercent: 3.4,
    memoryBytes: 512 * 1024 * 1024,
    logicalCPUs: 8,
    processCount: 2,
  };
  await emitWailsEvent(window, IPC.appMetrics, sample);

  await expect(window.locator('[data-testid="app-metrics"]')).toBeVisible();
  await expect(window.locator('[data-testid="app-metrics-cpu"]')).toHaveText('3.4%');

  const family = (selector: string) =>
    window
      .locator(selector)
      .first()
      .evaluate((el) => getComputedStyle(el).fontFamily);
  expect(await family('[data-testid="app-metrics-cpu"]')).toBe(await family('body'));

  const trigger = window.locator('[data-testid="app-metrics"]');
  await trigger.hover();
  await expect(window.locator('[data-testid="tooltip-details-title"]')).toHaveText(
    'Kira Space resource use',
  );
  const row = (label: string) =>
    window.locator(`[data-testid="tooltip-details-row"][data-label="${label}"]`);
  await expect(row('CPU')).toContainText('3.4% of total capacity');
  await expect(row('Cores')).toContainText('8');
  await expect(row('Memory')).toContainText('512');
  await expect(row('Processes')).toContainText('2');
  await expect(window.locator('[data-testid="tooltip-details-hint"]')).toHaveText(
    'Includes helper processes. Updates every 5 seconds.',
  );

  // The tooltip sits on the elevated ground, draws no arrow and keeps off its trigger.
  const tipSel = '[data-slot="tooltip-content"][data-variant="default"]';
  const ground = await window.evaluate((sel) => {
    const probe = document.createElement('div');
    probe.style.backgroundColor = 'var(--kira-bg-elevated)';
    document.body.appendChild(probe);
    const elevated = getComputedStyle(probe).backgroundColor;
    probe.remove();
    const el = document.querySelector(sel);
    return { elevated, actual: el ? getComputedStyle(el).backgroundColor : '' };
  }, tipSel);
  expect(ground.actual).toBe(ground.elevated);
  await expect(window.locator('[data-slot="tooltip-content"] svg')).toHaveCount(0);
  const [tip, box] = await Promise.all([
    window.locator(tipSel).boundingBox(),
    trigger.boundingBox(),
  ]);
  if (!(tip && box)) throw new Error('tooltip or trigger not laid out');
  const gap = Math.max(box.y - (tip.y + tip.height), tip.y - (box.y + box.height));
  expect(gap).toBeGreaterThanOrEqual(4);
});

test('kira:menu:openSettings opens the Settings dialog', async ({ relaunch }) => {
  const { window } = await relaunch();
  await expect(window.locator('[data-testid="settings-dialog"]')).toHaveCount(0);

  await emitWailsEvent(window, IPC.openSettings, undefined);

  await expect(window.locator('[data-testid="settings-dialog"]')).toBeVisible();
});

// P117 S2: same NativeSelect binding fix as Kira Studio's own settings-apply-on-save.spec.ts --
// this app's own DateFormatField.vue used the identical pattern (P120: this field moved from the
// former shared packages/workbench into this app), so the regression and its guard both apply here.
test('the date-format select shows its real value on open (P117 S2)', async ({ relaunch }) => {
  const { window } = await relaunch();
  await window.click('[data-testid="open-settings"]');
  await expect(window.locator('[data-testid="settings-dialog"]')).toBeVisible();

  await expect(window.locator('[data-testid="settings-date-format"]')).toHaveValue('relative');
});

test('kira:menu:toggleProjectPanel toggles the project panel', async ({ relaunch }) => {
  const { window } = await relaunch();
  const panel = window.locator('[data-testid="project-panel"]');
  await expect(panel).toBeVisible();

  await emitWailsEvent(window, IPC.toggleProjectPanel, undefined);
  await expect(panel).toHaveCount(0);

  await emitWailsEvent(window, IPC.toggleProjectPanel, undefined);
  await expect(panel).toBeVisible();
});

test('kira:menu:toggleOperationsPanel toggles the operations dock', async ({ relaunch }) => {
  const { window } = await relaunch();
  const dock = window.locator('[data-testid="operations-panel"]');
  await expect(dock).toHaveCount(0);

  await emitWailsEvent(window, IPC.toggleOperationsPanel, undefined);
  await expect(dock).toBeVisible();

  await emitWailsEvent(window, IPC.toggleOperationsPanel, undefined);
  await expect(dock).toHaveCount(0);
});

test('kira:menu:tabNext activates the next tab in the active workspace', async ({ relaunch }) => {
  const { window } = await relaunch({ control: TWO_TABS });
  const activeTab = () => window.locator('[data-testid="tab"][data-active="true"]');
  await expect(activeTab()).toHaveAttribute('data-tab-id', 'tab-a');

  await emitWailsEvent(window, IPC.tabNext, undefined);

  await expect(activeTab()).toHaveAttribute('data-tab-id', 'tab-b');
});

test('kira:menu:tabClose closes the active tab', async ({ relaunch }) => {
  const { window } = await relaunch({ control: TWO_TABS });
  await expect(window.locator('[data-testid="tab"]')).toHaveCount(2);

  await emitWailsEvent(window, IPC.tabClose, undefined);

  const remaining = window.locator('[data-testid="tab"]');
  await expect(remaining).toHaveCount(1);
  await expect(remaining).toHaveAttribute('data-tab-id', 'tab-b');
});
