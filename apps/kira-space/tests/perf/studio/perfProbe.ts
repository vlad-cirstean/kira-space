import { readdirSync, readFileSync } from 'node:fs';

// P160/P161 probe helpers. Opt-in, asserts nothing (docs/DEV_ENVIRONMENT.md).

export interface RunMetrics {
  load1: number;
  warm: boolean;
  firstTextMs: number | null;
  shownMs: number | null;
  longestBlockMs: number | null;
  tbtMs: number | null;
  rssPeakDeltaMb: number | null;
  rssPeakMb: number | null;
}

export function load1(): number {
  try {
    return Number.parseFloat(readFileSync('/proc/loadavg', 'utf8').split(' ')[0] ?? 'NaN');
  } catch {
    return Number.NaN;
  }
}

function readProc(pid: number): { ppid: number; rssKb: number } | null {
  try {
    const stat = readFileSync(`/proc/${pid}/stat`, 'utf8');
    const ppid = Number(stat.slice(stat.lastIndexOf(')') + 2).split(' ')[1]);
    const status = readFileSync(`/proc/${pid}/status`, 'utf8');
    const m = /^VmRSS:\s+(\d+) kB/m.exec(status);
    return { ppid, rssKb: m ? Number(m[1]) : 0 };
  } catch {
    return null;
  }
}

/** Sum of VmRSS (MB) over every descendant of this process (WebKit UI, web and network processes). */
export function descendantsRssMb(rootPid: number = process.pid): number | null {
  if (process.platform !== 'linux') return null;
  const procs = new Map<number, { ppid: number; rssKb: number }>();
  for (const name of readdirSync('/proc')) {
    if (!/^\d+$/.test(name)) continue;
    const p = readProc(Number(name));
    if (p) procs.set(Number(name), p);
  }
  const inTree = new Set<number>([rootPid]);
  let grew = true;
  while (grew) {
    grew = false;
    for (const [pid, p] of procs) {
      if (!inTree.has(pid) && inTree.has(p.ppid)) {
        inTree.add(pid);
        grew = true;
      }
    }
  }
  let kb = 0;
  for (const pid of inTree) if (pid !== rootPid) kb += procs.get(pid)?.rssKb ?? 0;
  return kb / 1024;
}

export class RssSampler {
  private timer: ReturnType<typeof setInterval> | null = null;
  peakMb: number | null = null;
  baselineMb: number | null = descendantsRssMb();

  start(): void {
    this.baselineMb = descendantsRssMb();
    this.peakMb = this.baselineMb;
    this.timer = setInterval(() => {
      const v = descendantsRssMb();
      if (v !== null && (this.peakMb === null || v > this.peakMb)) this.peakMb = v;
    }, 100);
  }

  stop(): void {
    if (this.timer) clearInterval(this.timer);
    this.timer = null;
  }
}

export function median(xs: Array<number | null>): number | null {
  const v = xs.filter((x): x is number => x !== null && Number.isFinite(x)).sort((a, b) => a - b);
  if (v.length === 0) return null;
  return v[Math.floor(v.length / 2)] ?? null;
}

export const fmt = (n: number | null): string =>
  n === null ? 'n/a' : String(Math.round(n * 10) / 10);

export function runLine(c: string, size: number, run: number, m: RunMetrics): string {
  return (
    `case=${c} size=${size} run=${run} load1=${fmt(m.load1)} warm=${m.warm} ` +
    `firstTextMs=${fmt(m.firstTextMs)} shownMs=${fmt(m.shownMs)} longestBlockMs=${fmt(m.longestBlockMs)} ` +
    `tbtMs=${fmt(m.tbtMs)} rssPeakDeltaMb=${fmt(m.rssPeakDeltaMb)} rssPeakMb=${fmt(m.rssPeakMb)}`
  );
}

export function summaryLine(c: string, size: number, runs: RunMetrics[]): string {
  const loads = runs.map((r) => r.load1).filter(Number.isFinite);
  return (
    `summary case=${c} size=${size} runs=${runs.length} ` +
    `load1=${fmt(loads.length ? Math.min(...loads) : null)}..${fmt(loads.length ? Math.max(...loads) : null)} ` +
    `firstTextMs=${fmt(median(runs.map((r) => r.firstTextMs)))} shownMs=${fmt(median(runs.map((r) => r.shownMs)))} ` +
    `longestBlockMs=${fmt(median(runs.map((r) => r.longestBlockMs)))} tbtMs=${fmt(median(runs.map((r) => r.tbtMs)))} ` +
    `rssPeakDeltaMb=${fmt(median(runs.map((r) => r.rssPeakDeltaMb)))}`
  );
}

/** One rAF frame during a scroll gesture: `t` is the rAF timestamp, `uncoveredPx` the viewport
 *  height not covered by a mounted row at that frame. */
export interface Frame {
  t: number;
  uncoveredPx: number;
}

export interface FrameMetrics {
  load1: number;
  frames: number;
  fps: number | null;
  frameP50Ms: number | null;
  frameP95Ms: number | null;
  frameMaxMs: number | null;
  over50: number;
  over33: number;
  uncoveredMaxPx: number | null;
}

function percentile(sorted: number[], p: number): number | null {
  if (sorted.length === 0) return null;
  return sorted[Math.min(sorted.length - 1, Math.ceil((p / 100) * sorted.length) - 1)] ?? null;
}

/** Stats over frame deltas between consecutive rAF timestamps. */
export function frameStats(frames: Frame[], load: number): FrameMetrics {
  const deltas: number[] = [];
  for (let i = 1; i < frames.length; i++)
    deltas.push((frames[i]?.t ?? 0) - (frames[i - 1]?.t ?? 0));
  const sorted = [...deltas].sort((a, b) => a - b);
  const span = deltas.reduce((s, d) => s + d, 0);
  return {
    load1: load,
    frames: deltas.length,
    fps: span > 0 ? (deltas.length / span) * 1000 : null,
    frameP50Ms: percentile(sorted, 50),
    frameP95Ms: percentile(sorted, 95),
    frameMaxMs: sorted.length ? (sorted[sorted.length - 1] ?? null) : null,
    over50: deltas.filter((d) => d > 50).length,
    over33: deltas.filter((d) => d > 33.4).length,
    uncoveredMaxPx: frames.length ? Math.max(...frames.map((f) => f.uncoveredPx)) : null,
  };
}

export function frameRunLine(c: string, run: number, params: string, m: FrameMetrics): string {
  return (
    `case=${c} run=${run} ${params} load1=${fmt(m.load1)} frames=${m.frames} fps=${fmt(m.fps)} ` +
    `frameP50Ms=${fmt(m.frameP50Ms)} frameP95Ms=${fmt(m.frameP95Ms)} frameMaxMs=${fmt(m.frameMaxMs)} ` +
    `over50=${m.over50} over33=${m.over33} uncoveredMaxPx=${fmt(m.uncoveredMaxPx)}`
  );
}

export function frameSummaryLine(c: string, params: string, runs: FrameMetrics[]): string {
  const loads = runs.map((r) => r.load1).filter(Number.isFinite);
  const med = (f: (r: FrameMetrics) => number | null) => fmt(median(runs.map(f)));
  return (
    `summary case=${c} runs=${runs.length} ${params} ` +
    `load1=${fmt(loads.length ? Math.min(...loads) : null)}..${fmt(loads.length ? Math.max(...loads) : null)} ` +
    `frames=${med((r) => r.frames)} fps=${med((r) => r.fps)} frameP50Ms=${med((r) => r.frameP50Ms)} ` +
    `frameP95Ms=${med((r) => r.frameP95Ms)} frameMaxMs=${med((r) => r.frameMaxMs)} ` +
    `over50=${med((r) => r.over50)} over33=${med((r) => r.over33)} ` +
    `uncoveredMaxPx=${med((r) => r.uncoveredMaxPx)}`
  );
}
