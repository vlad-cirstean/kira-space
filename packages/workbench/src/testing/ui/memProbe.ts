import { execFileSync } from 'node:child_process';
import { appendFileSync, mkdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import type { CDPSession, Page, TestInfo } from '@playwright/test';
import { HeapSampler } from './heapProbe';
import { captureSnapshot, type NamedRoot, type SnapshotReport } from './heapSnapshot';
import { webkitRssMb } from './perfProbe';

// P480 RAM probe. Report-only: prints `RAM <app> <world> <checkpoint> <json>` and appends the same
// record to test-results-perf/ram/<app>-<engine>.jsonl; asserts nothing on numbers.

const SETTLE_MS = 500;

export interface DomCounters {
  documents: number;
  nodes: number;
  jsEventListeners: number;
}

export interface Suspects {
  monacoEditors: number;
  monacoDiffEditors: number;
  xterms: number;
  slickGrids: number;
  imgs: number;
  imgDecodedBytes: number;
  canvases: number;
  largestScrollerChildren: number;
}

export interface QueryStats {
  total: number;
  noObservers: number;
  gcInfinity: number;
  gcDefault: number;
  noObserversInfinity: number;
  noObserversDefault: number;
}

export interface Introspection {
  piniaStores: string[];
  queries: QueryStats | null;
  i18nLocales: string[];
  hooks: Record<string, unknown>;
  missing: string[];
}

export interface Rss {
  rendererMb: number;
  totalMb: number;
  path: 'cdp' | 'ps' | 'webkit';
}

export interface MemPoint {
  app: string;
  world: string;
  checkpoint: string;
  engine: 'chromium' | 'webkit';
  heapMb?: number;
  dom?: DomCounters;
  rss: Rss;
  suspects: Suspects;
  intro?: Introspection;
}

export type Engine = 'chromium' | 'webkit';

export const SNAPSHOTS = process.env.KIRA_RAM_SNAPSHOTS === '1';
const KEEP = process.env.KIRA_RAM_KEEP_SNAPSHOTS === '1';

/** `KIRA_RAM_WORLD=a,b` narrows to worlds; unset runs all. */
export function worldEnabled(name: string): boolean {
  const only = process.env.KIRA_RAM_WORLD;
  return !only || only.split(',').includes(name);
}

/** P311 precedent: another suite on this host invalidates a run. Waits for load1 <= 1.0. */
export async function waitForQuietHost(maxMs = 120_000): Promise<number> {
  const t0 = Date.now();
  let load = Number.POSITIVE_INFINITY;
  while (Date.now() - t0 < maxMs) {
    load = Number.parseFloat(readFileSync('/proc/loadavg', 'utf8').split(' ')[0] ?? '0');
    if (load <= 1.0) return load;
    await new Promise((r) => setTimeout(r, 5000));
  }
  console.log(`RAMLOAD gave up waiting; load1=${load}`);
  return load;
}

function psRss(pids: readonly number[]): number {
  if (pids.length === 0) return 0;
  const out = execFileSync('ps', ['-o', 'rss=', '-p', pids.join(',')], { encoding: 'utf8' });
  return (
    out
      .split('\n')
      .map((l) => Number.parseInt(l.trim(), 10) || 0)
      .reduce((a, b) => a + b, 0) / 1024
  );
}

/** Fallback: the Playwright Chromium processes by command line. */
function psMatchRss(): { rendererMb: number; totalMb: number } {
  const out = execFileSync('ps', ['-axo', 'rss=,command='], { encoding: 'utf8' });
  let renderer = 0;
  let total = 0;
  for (const line of out.split('\n')) {
    if (!line.includes('/opt/pw-browsers') && !line.includes('chrom')) continue;
    const kb = Number.parseInt(line.trim().split(/\s+/, 1)[0] ?? '0', 10) || 0;
    total += kb;
    if (line.includes('--type=renderer')) renderer += kb;
  }
  return { rendererMb: renderer / 1024, totalMb: total / 1024 };
}

const SUSPECTS_FN = () => {
  const count = (s: string) => document.querySelectorAll(s).length;
  let decoded = 0;
  for (const img of Array.from(document.images))
    decoded += img.naturalWidth * img.naturalHeight * 4;
  let largest = 0;
  for (const el of Array.from(document.querySelectorAll<HTMLElement>('*'))) {
    if (el.scrollHeight <= el.clientHeight + 1) continue;
    const o = getComputedStyle(el).overflowY;
    if (o === 'auto' || o === 'scroll') largest = Math.max(largest, el.childElementCount);
  }
  return {
    monacoEditors: count('.monaco-editor'),
    monacoDiffEditors: count('.monaco-diff-editor'),
    xterms: count('.xterm'),
    slickGrids: count('.slick-pane-top.slick-pane-right'),
    imgs: document.images.length,
    imgDecodedBytes: decoded,
    canvases: count('canvas'),
    largestScrollerChildren: largest,
  };
};

/** Runs in the page; reads only what the built app already exposes. */
const INTROSPECT_FN = () => {
  const missing: string[] = [];
  // biome-ignore lint/suspicious/noExplicitAny: probing untyped app internals
  const w = window as any;
  const app = document.querySelector('#app')?.['__vue_app__' as never] as any;
  const out: {
    piniaStores: string[];
    queries: unknown;
    i18nLocales: string[];
    hooks: Record<string, unknown>;
    missing: string[];
  } = { piniaStores: [], queries: null, i18nLocales: [], hooks: {}, missing };
  if (!app) {
    missing.push('#app.__vue_app__');
    return out;
  }
  const pinia = app.config.globalProperties.$pinia;
  if (pinia?.state?.value) out.piniaStores = Object.keys(pinia.state.value);
  else missing.push('$pinia');
  const provides = app._context.provides ?? {};
  const key = Reflect.ownKeys(provides).find((k) => String(k).includes('VUE_QUERY_CLIENT'));
  const client = key === undefined ? undefined : provides[key];
  if (client) {
    const qs = client.getQueryCache().getAll();
    const s = {
      total: qs.length,
      noObservers: 0,
      gcInfinity: 0,
      gcDefault: 0,
      noObserversInfinity: 0,
      noObserversDefault: 0,
    };
    for (const q of qs) {
      const inf = q.gcTime === Number.POSITIVE_INFINITY;
      const idle = q.getObserversCount() === 0;
      if (inf) s.gcInfinity++;
      else s.gcDefault++;
      if (idle) {
        s.noObservers++;
        if (inf) s.noObserversInfinity++;
        else s.noObserversDefault++;
      }
    }
    out.queries = s;
  } else missing.push('VUE_QUERY_CLIENT');
  const i18n = app.config.globalProperties.$i18n;
  if (i18n?.messages) out.i18nLocales = Object.keys(i18n.messages);
  else missing.push('$i18n');
  for (const k of ['__kiraRetention', '__kiraRetainedBytes', '__kiraGridRetainedBytes']) {
    if (typeof w[k] === 'function') {
      try {
        out.hooks[k] = w[k]();
      } catch (e) {
        out.hooks[k] = `error: ${String(e)}`;
      }
    }
  }
  return out;
};

/** Expressions evaluated in the page to get a named root's remote object. */
const PINIA_ROOT = (id: string) =>
  `document.querySelector('#app').__vue_app__.config.globalProperties.$pinia.state.value[${JSON.stringify(id)}]`;
const QUERY_CLIENT = `(()=>{const p=document.querySelector('#app').__vue_app__._context.provides;const k=Reflect.ownKeys(p).find(k=>String(k).includes('VUE_QUERY_CLIENT'));return p[k]})()`;

export class MemSampler {
  private constructor(
    readonly app: string,
    readonly world: string,
    readonly page: Page,
    readonly engine: Engine,
    private readonly heap: HeapSampler | undefined,
    private readonly cdp: CDPSession | undefined,
    private readonly browserCdp: CDPSession | undefined,
    private readonly outFile: string,
    private readonly snapDir: string,
  ) {}

  static async attach(app: string, world: string, page: Page, info: TestInfo): Promise<MemSampler> {
    const browser = page.context().browser();
    const engine: Engine = browser?.browserType().name() === 'webkit' ? 'webkit' : 'chromium';
    let heap: HeapSampler | undefined;
    let cdp: CDPSession | undefined;
    let browserCdp: CDPSession | undefined;
    if (engine === 'chromium') {
      heap = await HeapSampler.attach(page);
      cdp = await page.context().newCDPSession(page);
      if (browser) browserCdp = await browser.newBrowserCDPSession();
    }
    const root = info.project.outputDir;
    const dir = join(root, 'ram');
    mkdirSync(dir, { recursive: true });
    return new MemSampler(
      app,
      world,
      page,
      engine,
      heap,
      cdp,
      browserCdp,
      join(dir, `${app}-${engine}.jsonl`),
      join(root, 'heap', app, world),
    );
  }

  private async rss(): Promise<Rss> {
    // Freed pages lag the GC: settle, then keep the minimum of three readings.
    await this.page.waitForTimeout(2000);
    const readings: Rss[] = [];
    for (let i = 0; i < 3; i++) {
      readings.push(await this.rssOnce());
      if (i < 2) await this.page.waitForTimeout(250);
    }
    return readings.reduce((a, b) => ({
      rendererMb: Math.min(a.rendererMb, b.rendererMb),
      totalMb: Math.min(a.totalMb, b.totalMb),
      path: a.path,
    }));
  }

  private async rssOnce(): Promise<Rss> {
    if (this.engine === 'webkit') {
      const mb = webkitRssMb();
      return { rendererMb: mb, totalMb: mb, path: 'webkit' };
    }
    if (!this.browserCdp) return { ...psMatchRss(), path: 'ps' };
    try {
      const { processInfo } = (await this.browserCdp.send('SystemInfo.getProcessInfo')) as {
        processInfo: { type: string; id: number }[];
      };
      const renderers = processInfo.filter((p) => p.type === 'renderer').map((p) => p.id);
      return {
        rendererMb: psRss(renderers),
        totalMb: psRss(processInfo.map((p) => p.id)),
        path: 'cdp',
      };
    } catch {
      return { ...psMatchRss(), path: 'ps' };
    }
  }

  /** One checkpoint: heap after two GCs, DOM counters, settled RSS, suspects, introspection. */
  async sample(checkpoint: string): Promise<MemPoint> {
    const heapMb = await this.heap?.sampleMb();
    const dom = this.cdp
      ? ((await this.cdp.send('Memory.getDOMCounters')) as DomCounters)
      : undefined;
    const suspects = await this.page.evaluate(SUSPECTS_FN);
    const intro = this.engine === 'chromium' ? await this.page.evaluate(INTROSPECT_FN) : undefined;
    const rss = await this.rss();
    const point: MemPoint = {
      app: this.app,
      world: this.world,
      checkpoint,
      engine: this.engine,
      heapMb,
      dom,
      rss,
      suspects,
      intro: intro as Introspection | undefined,
    };
    this.emit('RAM', point);
    return point;
  }

  private emit(tag: string, rec: unknown): void {
    const line = JSON.stringify(rec);
    console.log(
      `${tag} ${this.app} ${this.world} ${(rec as { checkpoint?: string }).checkpoint ?? ''} ${line}`,
    );
    if (tag === 'RAM') appendFileSync(this.outFile, `${line}\n`);
  }

  /** Named roots: pinia stores, query `state.data` and the i18n messages. */
  private async namedRoots(): Promise<{ name: string; expression: string }[]> {
    const meta = await this.page.evaluate(() => {
      // biome-ignore lint/suspicious/noExplicitAny: probing untyped app internals
      const app = (document.querySelector('#app') as any)?.__vue_app__;
      if (!app) return { stores: [] as string[], queries: [] as string[] };
      const p = app._context.provides;
      const k = Reflect.ownKeys(p).find((x) => String(x).includes('VUE_QUERY_CLIENT'));
      const qs = k === undefined ? [] : p[k].getQueryCache().getAll();
      return {
        stores: Object.keys(app.config.globalProperties.$pinia?.state?.value ?? {}),
        queries: qs.map((q: { queryHash: string }) => q.queryHash) as string[],
      };
    });
    const roots = meta.stores.map((id) => ({
      name: `pinia:${id}`,
      expression: PINIA_ROOT(id),
    }));
    meta.queries.forEach((hash, i) => {
      roots.push({
        name: `query:${hash.slice(0, 80)}`,
        expression: `${QUERY_CLIENT}.getQueryCache().getAll()[${i}]?.state.data`,
      });
    });
    roots.push({
      name: 'i18n:messages',
      expression: `document.querySelector('#app').__vue_app__.config.globalProperties.$i18n.messages`,
    });
    return roots;
  }

  /** Heap snapshot with named roots; pass B only. Returns undefined unless `KIRA_RAM_SNAPSHOTS=1`. */
  async snapshot(checkpoint: string, keep = false): Promise<SnapshotReport | undefined> {
    if (!SNAPSHOTS || !this.cdp) return undefined;
    await this.heap?.sampleMb();
    const named: NamedRoot[] = [];
    for (const r of await this.namedRoots()) {
      try {
        const obj = (await this.cdp.send('Runtime.evaluate', { expression: r.expression })) as {
          result: { objectId?: string };
        };
        if (!obj.result.objectId) continue;
        const { heapSnapshotObjectId } = (await this.cdp.send('HeapProfiler.getHeapObjectId', {
          objectId: obj.result.objectId,
        })) as { heapSnapshotObjectId: string };
        named.push({ name: r.name, id: Number(heapSnapshotObjectId) });
      } catch {
        // root missing in this build or gone: skip
      }
    }
    mkdirSync(this.snapDir, { recursive: true });
    const report = await captureSnapshot(
      this.cdp,
      join(this.snapDir, `${checkpoint}.heapsnapshot`),
      {
        named,
        keep: keep || KEEP,
      },
    );
    console.log(`HEAPSNAP ${this.app} ${this.world} ${checkpoint} ${JSON.stringify(report)}`);
    appendFileSync(
      join(this.snapDir, '..', '..', '..', 'ram', `${this.app}-snap.jsonl`),
      `${JSON.stringify({ world: this.world, checkpoint, ...report })}\n`,
    );
    return report;
  }

  /** Where `snapshot(checkpoint, true)` leaves its file. */
  snapshotFile(checkpoint: string): string {
    return join(this.snapDir, `${checkpoint}.heapsnapshot`);
  }
}

export interface MemWorld {
  app: string;
  world: string;
  page: Page;
  info: TestInfo;
  /** Units opened per cycle (tabs); 1 for a mode or a single-surface world. */
  units: number;
  /** Enters the world's mode; sampled as `mode`. Optional. */
  enter?: () => Promise<void>;
  /** Opens unit `i` (0-based). */
  open: (i: number) => Promise<void>;
  /** Activates every open unit once; defaults to clicking each tab. */
  activateAll?: () => Promise<void>;
  /** Closes everything opened (or leaves the mode). */
  closeAll: () => Promise<void>;
  /** Checkpoints that also take a heap snapshot in pass B (default mode, openN, closed). */
  snapshotAt?: readonly string[];
  /** Hook called with the sampler at `openN` and `closed`, e.g. for a TanStack `gcTime` sweep. */
  atCheckpoint?: (name: string, s: MemSampler) => Promise<void>;
  /** Extra cycles beyond the first (default 2: closed2, closed3). */
  extraCycles?: number;
}

export async function activateTabs(page: Page, count: number): Promise<void> {
  for (let i = 0; i < count; i++) {
    const tab = page.locator('[data-testid="tab"]').nth(i);
    await tab.click();
    await page.waitForFunction(
      (n) =>
        document.querySelectorAll('[data-testid="tab"]')[n]?.getAttribute('data-active') === 'true',
      i,
    );
    await page.waitForTimeout(SETTLE_MS);
  }
}

export interface WorldSummary {
  slopeMb: number | null;
  retentionMb: number | null;
  leakPerCycleMb: number | null;
}

/**
 * boot, mode, open1, openN, round, closed, then extra open-N/close cycles (closed2, closed3).
 * Derived: slope `(openN - open1) / (N - 1)`, retention `closed - mode`, leak per cycle
 * `(closed3 - closed) / 2`.
 */
export async function runMemWorld(w: MemWorld): Promise<MemSampler> {
  await waitForQuietHost();
  const s = await MemSampler.attach(w.app, w.world, w.page, w.info);
  await w.page.waitForTimeout(2000);
  await s.sample('boot');
  if (w.enter) {
    await w.enter();
    await w.page.waitForTimeout(SETTLE_MS * 2);
  }
  const mode = await s.sample('mode');
  const snapAt = new Set(w.snapshotAt ?? ['mode', 'openN', 'closed']);
  if (snapAt.has('mode')) await s.snapshot('mode');

  await w.open(0);
  await w.page.waitForTimeout(SETTLE_MS * 2);
  const open1 = await s.sample('open1');
  for (let i = 1; i < w.units; i++) {
    await w.open(i);
    await w.page.waitForTimeout(SETTLE_MS);
  }
  await w.page.waitForTimeout(SETTLE_MS * 2);
  const openN = await s.sample('openN');
  await w.atCheckpoint?.('openN', s);
  if (snapAt.has('openN')) await s.snapshot('openN');

  await (w.activateAll ?? (() => activateTabs(w.page, w.units)))();
  await s.sample('round');
  if (snapAt.has('round')) await s.snapshot('round');

  await w.closeAll();
  await w.page.waitForTimeout(SETTLE_MS * 2);
  const closed = await s.sample('closed');
  await w.atCheckpoint?.('closed', s);
  if (snapAt.has('closed')) await s.snapshot('closed');

  let last = closed;
  const cycles = w.extraCycles ?? 2;
  for (let c = 0; c < cycles; c++) {
    for (let i = 0; i < w.units; i++) {
      await w.open(i);
      await w.page.waitForTimeout(SETTLE_MS);
    }
    await w.closeAll();
    await w.page.waitForTimeout(SETTLE_MS * 2);
    last = await s.sample(`closed${c + 2}`);
  }

  const mb = (p: MemPoint) => p.heapMb ?? p.rss.rendererMb;
  const summary: WorldSummary = {
    slopeMb: w.units > 1 ? (mb(openN) - mb(open1)) / (w.units - 1) : null,
    retentionMb: mb(closed) - mb(mode),
    leakPerCycleMb: cycles > 0 ? (mb(last) - mb(closed)) / cycles : null,
  };
  console.log(`RAMSUM ${w.app} ${w.world} ${s.engine} ${JSON.stringify(summary)}`);
  return s;
}
