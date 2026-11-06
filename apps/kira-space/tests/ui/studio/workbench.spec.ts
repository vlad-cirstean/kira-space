import { expect, test } from './fixtures';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// Ported from tests/e2e/workbench.spec.ts. Five of its seven scenarios asserted real
// persistence across a relaunch (panel visibility, settings appearance/Advanced sections, word
// wrap, a narrowed-patch write leaving other sections untouched) — real writes surviving a real
// process restart, backed by src/main's own storage. This tier's `relaunch()` has no backing
// store at all (a fresh page + fresh mocks every time, tests/ui/fixtures.ts's own doc comment),
// so those five scenarios have no equivalent here and are not ported; they are also not covered
// by tests/e2e/sqlite.spec.ts (the one full-stack anchor kept, and the rule is that its
// assertions do not grow to cover what this port drops), so this is a real, acknowledged
// coverage loss — see docs/CLAUDE.md's findings. Only the two
// scenarios below asserted pure rendering with no relaunch-persistence claim.

test('the workbench is inset from the window edge on two sides, none from the title bar (P31 D8)', async ({
  kira,
}) => {
  const { window } = kira;
  const shell = window.locator('.workbench-shell');
  const padding = await shell.evaluate((el) => {
    const style = getComputedStyle(el);
    return {
      top: style.paddingTop,
      right: style.paddingRight,
      bottom: style.paddingBottom,
      left: style.paddingLeft,
    };
  });
  // No top inset any more: a title-bar-height session fix moved that 6px into
  // --kira-titlebar-h instead, since a gap here between the title bar and this shell's own
  // content read as the bar's content being off-centre relative to the panel right below it.
  expect(padding.top).toBe('0px');
  expect(padding.right).toBe('6px');
  expect(padding.left).toBe('6px');
  expect(padding.bottom).toBe('2px');

  const [shellBox, projectBox] = await Promise.all([
    shell.boundingBox(),
    window.locator('[data-testid="project-panel"]').boundingBox(),
  ]);
  if (!shellBox || !projectBox) throw new Error('bounding boxes not found');
  expect(projectBox.x - shellBox.x).toBeGreaterThanOrEqual(6);
});

// P42 F17: Chromium's own unstyled default for this pseudo-element is opaque white, which
// survives every theme (it uses none of this app's own tokens) until base.css overrides it.
test("the scrollbar corner is not left at Chromium's opaque-white default (P42 F17)", async ({
  kira,
}) => {
  const { window } = kira;
  const corner = await window.evaluate(
    () => getComputedStyle(document.documentElement, '::-webkit-scrollbar-corner').backgroundColor,
  );
  expect(corner).not.toBe('rgb(255, 255, 255)');
});

// The title bar's own action button for shell.BuildMenu's ItemNewWindow, previously reachable only
// from the native menu — nothing in the renderer could reach it before. It moved to the rightmost
// position, inserting the keep-awake button immediately to its left — this test's own title and
// DOM-order assertion are updated to match, not ported unchanged.
test('the title bar has a New window button, rightmost of the action row, that calls WindowsService.OpenNew once per click', async ({
  relaunch,
}) => {
  const { window, control } = await relaunch({
    control: [{ channel: IPC.windowsOpenNew, response: null }],
  });
  const newWindow = window.locator('[data-testid="new-window"]');
  await expect(newWindow).toBeVisible();
  await expect(newWindow).toContainText('New window');

  // DOM order: Connections, Operations, Settings, keep-awake, New window (P87's own reorder).
  const testIds = await window
    .locator(
      '[data-testid="toggle-project-panel"], [data-testid="toggle-operations-panel"], ' +
        '[data-testid="open-settings"], [data-testid="toggle-keep-awake"], [data-testid="new-window"]',
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

// The titlebar keep-awake toggle — renders (mockRuntime.ts's own default keepAwakeStatus
// has supported: true), one click issues exactly one KeepAwakeService.SetManual carrying
// {enabled: true}, and the button gains aria-pressed="true". This file's `.is-on` assertions are
// dropped as stale, aria-pressed alone is the toggle signal now.
test('the keep-awake button toggles on click, calling KeepAwakeService.SetManual once with {enabled: true}', async ({
  relaunch,
}) => {
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
  expect(setManualCalls()).toHaveLength(0);
  await button.click();
  await expect.poll(() => setManualCalls().length).toBe(1);
  expect(setManualCalls()[0]?.args).toEqual({ enabled: true });

  await expect(button).toHaveAttribute('aria-pressed', 'true');
});

// The cross-window broadcast: a ChannelKeepAwake event with no click at all still turns the
// button on — nothing else in this file covers a push-driven (not click-driven) title-bar state.
test('the keep-awake button turns on from a ChannelKeepAwake broadcast with no click', async ({
  relaunch,
}) => {
  const { window } = await relaunch();
  const button = window.locator('[data-testid="toggle-keep-awake"]');
  await expect(button).toBeVisible();
  await expect(button).toHaveAttribute('aria-pressed', 'false');

  await emitWailsEvent(window, IPC.keepAwake, { manual: true, supported: true, error: '' });

  await expect(button).toHaveAttribute('aria-pressed', 'true');
});
