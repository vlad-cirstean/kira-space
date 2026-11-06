import { execFileSync } from 'node:child_process';
import type { Page, TestInfo } from '@playwright/test';

// Shared by both apps' tests/perf probes. Probes report numbers and assert nothing: a hard bound
// would be flaky across machines. On macOS `ps` RSS omits CoreAnimation/IOSurface graphics memory,
// the dominant scroll cost there; read footprint with
// apps/kira-space/frontend/proto/grid/mac/wkhost.swift instead. Process RSS here is the
// DOM/JS/malloc side only.

const WEBKIT_PROCESS =
  /WPEWebProcess|WPENetworkProcess|WPEGraphicsProcess|WebKitWebProcess|WebKitNetworkProcess|WebKitGPUProcess|MiniBrowser\/bin|com\.apple\.WebKit|Playwright\.app/;

/** Sum of resident set size over the WebKit processes of this machine, in MB. */
export function webkitRssMb(): number {
  const out = execFileSync('ps', ['-axo', 'rss=,command='], { encoding: 'utf8' });
  let kb = 0;
  for (const line of out.split('\n')) {
    if (!WEBKIT_PROCESS.test(line)) continue;
    kb += Number.parseInt(line.trim().split(/\s+/, 1)[0] ?? '0', 10) || 0;
  }
  return kb / 1024;
}

export interface RssStats {
  startMb: number;
  peakMb: number;
  endMb: number;
  riseMb: number;
}

/** Samples process RSS on a timer; `tag` buckets samples into named phases. */
export class RssSampler {
  private readonly samples: { tag: string; mb: number }[] = [];
  private current = 'idle';
  private timer: ReturnType<typeof setInterval> | undefined;

  start(intervalMs = 25): void {
    this.timer = setInterval(() => {
      this.samples.push({ tag: this.current, mb: webkitRssMb() });
    }, intervalMs);
  }

  phase(tag: string): void {
    this.current = tag;
  }

  stats(tag: string): RssStats {
    const mb = this.samples.filter((s) => s.tag === tag).map((s) => s.mb);
    if (mb.length === 0) return { startMb: 0, peakMb: 0, endMb: 0, riseMb: 0 };
    const peakMb = Math.max(...mb);
    return { startMb: mb[0], peakMb, endMb: mb[mb.length - 1], riseMb: peakMb - mb[0] };
  }

  stop(): void {
    if (this.timer) clearInterval(this.timer);
  }
}

export interface FrameStats {
  frames: number;
  avgFps: number;
  p50Ms: number;
  p95Ms: number;
  maxMs: number;
  over50Ms: number;
  over100Ms: number;
}

interface FrameWindow {
  __perfFrames?: number[];
  __perfFramesOn?: boolean;
}

/** Starts recording requestAnimationFrame intervals in the page. */
export function startFrames(page: Page): Promise<void> {
  return page.evaluate(() => {
    const w = window as unknown as FrameWindow;
    w.__perfFrames = [];
    w.__perfFramesOn = true;
    let last = performance.now();
    const tick = (now: number): void => {
      if (!w.__perfFramesOn) return;
      w.__perfFrames?.push(now - last);
      last = now;
      requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  });
}

/** Stops the recording and summarises it. A frame over 50 ms is under 20 fps. */
export function stopFrames(page: Page): Promise<FrameStats> {
  return page.evaluate(() => {
    const w = window as unknown as FrameWindow;
    w.__perfFramesOn = false;
    const all = w.__perfFrames ?? [];
    const sorted = [...all].sort((a, b) => a - b);
    const at = (q: number): number => sorted[Math.floor(sorted.length * q)] ?? 0;
    const totalMs = all.reduce((a, b) => a + b, 0);
    return {
      frames: all.length,
      avgFps: totalMs > 0 ? (all.length * 1000) / totalMs : 0,
      p50Ms: at(0.5),
      p95Ms: at(0.95),
      maxMs: sorted[sorted.length - 1] ?? 0,
      over50Ms: all.filter((x) => x > 50).length,
      over100Ms: all.filter((x) => x > 100).length,
    };
  });
}

export interface FlickOptions {
  /** First wheel delta in px. 120 is a brisk trackpad flick, 250 a hard one. */
  startDelta: number;
  /** Per-event delta multiplier (momentum decay). */
  decay?: number;
  direction?: 1 | -1;
  /** Gap between wheel events; 8 ms is about 120 Hz. */
  stepMs?: number;
}

/** Replays a momentum scroll as a decaying burst of wheel events at the current mouse position. */
export async function flick(
  page: Page,
  opts: FlickOptions,
): Promise<{ events: number; distancePx: number }> {
  const { startDelta, decay = 0.985, direction = 1, stepMs = 8 } = opts;
  let delta = startDelta;
  let events = 0;
  let distancePx = 0;
  while (delta > 1) {
    await page.mouse.wheel(0, direction * delta);
    distancePx += delta;
    delta *= decay;
    events++;
    await page.waitForTimeout(stepMs);
  }
  return { events, distancePx };
}

export interface FlickResult {
  label: string;
  distancePx: number;
  rss: RssStats;
  frames: FrameStats;
}

/** Runs one flick with RSS sampling and frame recording around it, then logs one line. */
export async function measureFlick(
  page: Page,
  rss: RssSampler,
  label: string,
  opts: FlickOptions,
): Promise<FlickResult> {
  await page.waitForTimeout(1500);
  rss.phase(label);
  await startFrames(page);
  const { distancePx } = await flick(page, opts);
  const frames = await stopFrames(page);
  await page.waitForTimeout(400);
  const result = { label, distancePx, rss: rss.stats(label), frames };
  console.log(formatResult(result));
  return result;
}

export function formatResult(r: FlickResult): string {
  return (
    `PERF ${r.label}: ${Math.round(r.distancePx)}px | ` +
    `RSS ${r.rss.startMb.toFixed(0)} -> peak ${r.rss.peakMb.toFixed(0)} MB (+${r.rss.riseMb.toFixed(0)}) | ` +
    `${r.frames.avgFps.toFixed(0)} fps avg, p50 ${r.frames.p50Ms.toFixed(0)} ms, ` +
    `p95 ${r.frames.p95Ms.toFixed(0)} ms, max ${r.frames.maxMs.toFixed(0)} ms, ` +
    `>50ms ${r.frames.over50Ms}, >100ms ${r.frames.over100Ms}`
  );
}

/** Attaches the run's results to the Playwright report as JSON. */
export async function attachResults(info: TestInfo, results: readonly unknown[]): Promise<void> {
  await info.attach('perf-results', {
    body: JSON.stringify(results, null, 2),
    contentType: 'application/json',
  });
}

/** Standard flick ladder: forward, back, forward again, then a hard flick. */
export const FLICK_LADDER: readonly { label: string; opts: FlickOptions }[] = [
  { label: 'flick-down', opts: { startDelta: 120 } },
  { label: 'flick-up', opts: { startDelta: 120, direction: -1 } },
  { label: 'flick-down-2', opts: { startDelta: 120 } },
  { label: 'flick-up-2', opts: { startDelta: 120, direction: -1 } },
  { label: 'flick-hard-down', opts: { startDelta: 250 } },
];
