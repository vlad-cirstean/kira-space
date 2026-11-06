import type { Page } from '@playwright/test';
import { defaultLayout } from '@shared/domain/layout';
import { center, openDock } from '../../../../../packages/workbench/src/testing/ui/panelResize';
import { expect, test } from './fixtures';
import { modeTab } from './support/apiMode';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// P312: panel sizes are per module; the flow test (windowflow) covers the same scenario in Go.

const PROJECT_HANDLE = '.workbench-shell [data-slot="resizable-handle"]';
const DOCK_HANDLE = '.workbench-shell [role="separator"][aria-label="Resize operations panel"]';

async function drag(page: Page, selector: string, dx: number, dy: number): Promise<void> {
  const at = await center(page.locator(selector).first());
  await page.mouse.move(at.x, at.y);
  await page.mouse.down();
  await page.mouse.move(at.x + dx, at.y + dy, { steps: 6 });
  await page.mouse.up();
}

const panelWidth = async (page: Page) =>
  (await page.locator('[data-testid="project-panel"]').boundingBox())?.width ?? 0;
const dockHeight = async (page: Page) =>
  (await page.locator('[data-testid="operations-panel"]').boundingBox())?.height ?? 0;

test('resizing a panel in one module leaves every other module alone', async ({ relaunch }) => {
  const { window: page, control } = await relaunch();
  const writes = () =>
    control
      .log()
      .filter((e) => e.channel === IPC.layoutSet)
      .map((e) => (e.args as { patch: { modes?: Record<string, unknown> } }).patch.modes);

  const defaultWidth = await panelWidth(page);
  await drag(page, PROJECT_HANDLE, 100, 0);
  await expect.poll(() => panelWidth(page)).toBeGreaterThan(defaultWidth + 90);
  const studioWidth = await panelWidth(page);
  await expect.poll(() => writes().length).toBeGreaterThan(0);
  const modes = writes().at(-1) ?? {};
  expect(Object.keys(modes)).toEqual(['studio']);

  await modeTab(page, 'api').click();
  await expect.poll(() => panelWidth(page)).toBeCloseTo(defaultWidth, -1);
  await modeTab(page, 'studio').click();
  await expect.poll(() => panelWidth(page)).toBeCloseTo(studioWidth, -1);

  await openDock(page);
  const defaultHeight = await dockHeight(page);
  await drag(page, DOCK_HANDLE, 0, -60);
  await expect.poll(() => dockHeight(page)).toBeGreaterThan(defaultHeight + 50);
  const studioHeight = await dockHeight(page);

  await modeTab(page, 'api').click();
  await expect.poll(() => dockHeight(page)).toBeCloseTo(defaultHeight, -1);
  await drag(page, DOCK_HANDLE, 0, -40);
  await expect.poll(() => dockHeight(page)).toBeGreaterThan(defaultHeight + 30);
  await modeTab(page, 'studio').click();
  await expect.poll(() => dockHeight(page)).toBeCloseTo(studioHeight, -1);
});

test('a pushed layout for one module does not move the active module', async ({ relaunch }) => {
  const { window: page } = await relaunch();
  await modeTab(page, 'api').click();
  const apiWidth = await panelWidth(page);

  await emitWailsEvent(page, IPC.layoutChanged, {
    ...defaultLayout,
    modes: {
      studio: { project: { width: 420 }, operations: { height: 200 } },
    },
  });
  await expect.poll(() => panelWidth(page)).toBeCloseTo(apiWidth, -1);

  await modeTab(page, 'studio').click();
  await expect.poll(() => panelWidth(page)).toBeCloseTo(420, -1);
});
