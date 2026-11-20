import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeBoard, adeV2Control, openTask } from './support/adeV2';
import { IPC } from './support/ipcChannels';

// Leaving ADE must free its component tree: a Pinia store whose setup ran inside AdeShell would pin
// the unmounted shell through a watcher (activateAdeStores). Chromium only: GC and DOM counters
// come over CDP. No IPC change, so no flow half.

const TASKS = 20;
// The pre-fix duplicate on this fixture was 8098 nodes (back 16764 vs task 8666, P693-part2-measure.md);
// the tolerance is a tenth of it.
const TOLERANCE = 810;

const modeTab = (page: Page, mode: string) =>
  page.locator(`[data-testid="mode-tab"][data-mode="${mode}"]`);

test('leaving ADE collects the shell and a return adds no duplicate tree', async ({ relaunch }) => {
  const board = adeBoard((b) => {
    const base = b.tasks.slice();
    let k = 0;
    while (b.tasks.length < TASKS) {
      const src = base[k % base.length];
      const copy = structuredClone(src) as typeof src;
      copy.id = `${src.id}_x${k}`;
      copy.title = `${String(src.title)} #${k}`;
      b.tasks.push(copy);
      k++;
    }
  });
  const { window: page } = await relaunch({
    control: adeV2Control([{ channel: IPC.adeTaskBoard, response: board }]),
  });
  await expect(page.locator('[data-testid="ade-card"]').first()).toBeVisible();
  const cdp = await page.context().newCDPSession(page);
  const collect = async (): Promise<void> => {
    await cdp.send('HeapProfiler.collectGarbage');
    await cdp.send('HeapProfiler.collectGarbage');
  };
  const nodes = async (): Promise<number> => {
    await collect();
    return (await cdp.send('Memory.getDOMCounters')).nodes;
  };

  await openTask(page, board.tasks[0].id);
  await expect(page.locator('[data-testid="ade-shell"]')).toBeVisible();
  // The open task panel is restored on return, so the baseline holds it.
  const atTask = await nodes();
  await page.evaluate(() => {
    (window as unknown as { __adeShellRef: WeakRef<Element> }).__adeShellRef = new WeakRef(
      document.querySelector('[data-testid="ade-shell"]') as Element,
    );
  });

  await modeTab(page, 'git').click();
  await expect(page.locator('[data-testid="ade-shell"]')).toHaveCount(0);
  await collect();
  const collected = await page.evaluate(
    () =>
      (window as unknown as { __adeShellRef: WeakRef<Element> }).__adeShellRef.deref() ===
      undefined,
  );
  expect(collected).toBe(true);

  await modeTab(page, 'ade').click();
  await expect(page.locator('[data-testid="ade-card"]').first()).toBeVisible();
  const atBack = await nodes();
  console.log(`RETENTION nodes task=${atTask} back=${atBack} delta=${atBack - atTask}`);
  expect(atBack - atTask).toBeLessThanOrEqual(TOLERANCE);
});
