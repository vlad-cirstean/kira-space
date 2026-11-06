import type { Page } from '@playwright/test';
import { load1, RssSampler } from './perfProbe';

// P163: shared in-page main-thread block meter (setTimeout-0 gap list) for action probes.

export interface ActionMetrics {
  load1: number;
  actionMs: number | null;
  longestBlockMs: number;
  tbtMs: number;
  rssPeakDeltaMb: number | null;
}

/**
 * Clicks `clickSelector` and waits until `doneFn` (a function source string evaluated in the page,
 * returning truthy) holds. Meters block gaps from the click to one second after completion.
 */
export async function measureAction(
  page: Page,
  clickSelector: string,
  doneFn: string,
  timeoutMs = 120_000,
): Promise<ActionMetrics> {
  const load = load1();
  const rss = new RssSampler();
  rss.start();
  const t = await page.evaluate(
    async ({ sel, done, timeout }) => {
      const gaps: Array<[number, number]> = [];
      let last = performance.now();
      let stop = false;
      const tick = () => {
        const n = performance.now();
        gaps.push([n, n - last]);
        last = n;
        if (!stop) setTimeout(tick, 0);
      };
      setTimeout(tick, 0);
      const isDone = new Function(`return (${done})()`) as () => boolean;
      const t0 = performance.now();
      (document.querySelector(sel) as HTMLElement).click();
      let doneAt: number | null = null;
      while (performance.now() < t0 + timeout) {
        if (isDone()) {
          doneAt = performance.now();
          break;
        }
        await new Promise((r) => setTimeout(r, 20));
      }
      const end = (doneAt ?? performance.now()) + 1000;
      while (performance.now() < end) await new Promise((r) => setTimeout(r, 50));
      stop = true;
      const inWin = gaps.filter(([at]) => at >= t0 && at <= end);
      return {
        actionMs: doneAt === null ? null : doneAt - t0,
        longest: inWin.reduce((m, [, g]) => Math.max(m, g), 0),
        tbt: inWin.reduce((s, [, g]) => s + (g > 50 ? g - 50 : 0), 0),
      };
    },
    { sel: clickSelector, done: doneFn, timeout: timeoutMs },
  );
  rss.stop();
  return {
    load1: load,
    actionMs: t.actionMs,
    longestBlockMs: t.longest,
    tbtMs: t.tbt,
    rssPeakDeltaMb:
      rss.peakMb !== null && rss.baselineMb !== null ? rss.peakMb - rss.baselineMb : null,
  };
}

const med = (xs: Array<number | null>): number | null => {
  const v = xs.filter((x): x is number => x !== null && Number.isFinite(x)).sort((a, b) => a - b);
  return v.length ? (v[Math.floor(v.length / 2)] ?? null) : null;
};
const fmt = (n: number | null): string => (n === null ? 'n/a' : String(Math.round(n * 10) / 10));

export function actionLine(c: string, size: string, run: number, m: ActionMetrics): string {
  return (
    `case=${c} size=${size} run=${run} load1=${fmt(m.load1)} actionMs=${fmt(m.actionMs)} ` +
    `longestBlockMs=${fmt(m.longestBlockMs)} tbtMs=${fmt(m.tbtMs)} rssPeakDeltaMb=${fmt(m.rssPeakDeltaMb)}`
  );
}

export function actionSummaryLine(c: string, size: string, runs: ActionMetrics[]): string {
  const loads = runs.map((r) => r.load1).filter(Number.isFinite);
  return (
    `summary case=${c} size=${size} runs=${runs.length} ` +
    `load1=${fmt(loads.length ? Math.min(...loads) : null)}..${fmt(loads.length ? Math.max(...loads) : null)} ` +
    `actionMs=${fmt(med(runs.map((r) => r.actionMs)))} longestBlockMs=${fmt(med(runs.map((r) => r.longestBlockMs)))} ` +
    `tbtMs=${fmt(med(runs.map((r) => r.tbtMs)))} rssPeakDeltaMb=${fmt(med(runs.map((r) => r.rssPeakDeltaMb)))}`
  );
}
