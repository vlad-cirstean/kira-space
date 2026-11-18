import type { Locator, Page } from '@playwright/test';

export interface FreezeReading {
  maxGapMs: number;
  blockedMs: number;
  longTasks: number;
  bodyPointerEvents: string;
}

interface ProbeWindow {
  __kiraFreeze: { reset(): void; read(): FreezeReading };
}

/** Installs a rAF-gap probe on every navigation. WebKit has no `longtask` entries, so the frame
 *  gap is the assertion; long tasks are Chromium-only diagnostics. */
export async function installFreezeProbe(page: Page): Promise<void> {
  await page.addInitScript(() => {
    let last = performance.now();
    let maxGap = 0;
    let blocked = 0;
    let longTasks = 0;
    const tick = (now: number) => {
      const gap = now - last;
      last = now;
      if (gap > maxGap) maxGap = gap;
      if (gap > 50) blocked += gap;
      requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
    try {
      if (PerformanceObserver.supportedEntryTypes.includes('longtask')) {
        new PerformanceObserver((l) => {
          longTasks += l.getEntries().length;
        }).observe({ entryTypes: ['longtask'] });
      }
    } catch {
      // diagnostics only
    }
    (globalThis as unknown as ProbeWindow).__kiraFreeze = {
      reset() {
        last = performance.now();
        maxGap = 0;
        blocked = 0;
        longTasks = 0;
      },
      read() {
        // Count the still-running frame: a freeze in progress has no next rAF yet.
        const open = performance.now() - last;
        return {
          maxGapMs: Math.round(Math.max(maxGap, open)),
          blockedMs: Math.round(blocked + (open > 50 ? open : 0)),
          longTasks,
          bodyPointerEvents: getComputedStyle(document.body).pointerEvents,
        };
      },
    };
  });
}

export function resetFreezeProbe(page: Page): Promise<void> {
  return page.evaluate(() => (globalThis as unknown as ProbeWindow).__kiraFreeze.reset());
}

export function readFreezeProbe(page: Page): Promise<FreezeReading> {
  return page.evaluate(() => (globalThis as unknown as ProbeWindow).__kiraFreeze.read());
}

/** Fails with the probe's numbers when any frame gap since the last reset exceeded `budgetMs`. */
export async function expectResponsive(page: Page, label: string, budgetMs: number): Promise<void> {
  const r = await readFreezeProbe(page);
  console.log(
    `freeze-probe ${label}: maxGapMs=${r.maxGapMs} blockedMs=${r.blockedMs} longTasks=${r.longTasks}`,
  );
  if (r.bodyPointerEvents === 'none') throw new Error(`${label}: body pointer-events is none`);
  if (r.maxGapMs > budgetMs) {
    throw new Error(
      `${label}: frame gap ${r.maxGapMs} ms over ${budgetMs} ms budget (blocked ${r.blockedMs} ms, ${r.longTasks} long tasks)`,
    );
  }
}

/** Click, then wait for the next frame in which `changed` holds; returns the elapsed ms. */
export async function measureClickLatency(
  page: Page,
  target: Locator,
  changed: () => Promise<boolean>,
  timeoutMs = 30_000,
): Promise<number> {
  const start = Date.now();
  await target.click({ timeout: timeoutMs });
  while (!(await changed())) {
    if (Date.now() - start > timeoutMs) throw new Error('click had no visible effect');
    await page.waitForTimeout(10);
  }
  return Date.now() - start;
}
