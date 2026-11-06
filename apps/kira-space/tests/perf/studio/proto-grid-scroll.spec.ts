import { resolve } from 'node:path';
import { test } from '@playwright/test';
import {
  attachResults,
  FLICK_LADDER,
  type FlickResult,
  measureFlick,
  RssSampler,
} from '@workbench/testing/ui/perfProbe';
import { startServer, type UiServer } from '@workbench/testing/ui/server';
import type { TraceResult } from '../../../frontend/proto/grid/trace';

// Frame cost and late data of the prototype grids, same data and viewport as the app's own
// `NCOLS=20 grid-scroll` probe. `bun run perf:probe:proto`; PERF_HEADED=1 for headed WebKitGTK;
// GRID=cheetah,slick-stock,slick-kira narrows the variants. Frame cost runs on the release build
// (`dist-proto`, no hooks); late data on the hooks build (`dist-proto-hooks`) because its readback
// distorts timing. Report numbers, assert nothing.

const FRONTEND = resolve(__dirname, '../../../frontend');
const VIEWPORT = { width: 1104, height: 729 };
const PAGES = {
  cheetah: { path: 'cheetah.html?', scroller: '[data-testid="data-grid"] .grid-scrollable' },
  'slick-stock': {
    path: 'slick.html?variant=stock&',
    scroller: '.slick-viewport-top.slick-viewport-right',
  },
  'slick-kira': {
    path: 'slick.html?variant=kira&',
    scroller: '.slick-viewport-top.slick-viewport-right',
  },
} as const;
type Variant = keyof typeof PAGES;

const VARIANTS = (process.env.GRID ?? 'cheetah,slick-stock,slick-kira')
  .split(',')
  .filter((v): v is Variant => v in PAGES);
const BANDS = [40, 100, 200];
const FRAMES_PER_BAND = 60;

let release: UiServer;
let hooks: UiServer;
test.beforeAll(async () => {
  release = await startServer(resolve(FRONTEND, 'dist-proto'));
  hooks = await startServer(resolve(FRONTEND, 'dist-proto-hooks'));
});
test.afterAll(async () => {
  await release.close();
  await hooks.close();
});

function url(server: UiServer, variant: Variant): string {
  return `${server.url}proto/grid/${PAGES[variant].path}w=${VIEWPORT.width}&h=${VIEWPORT.height}&rowHeight=28`;
}

function summarise(values: number[]): string {
  if (values.length === 0) return 'no moving frames';
  const sorted = [...values].sort((a, b) => a - b);
  const at = (q: number): number =>
    sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * q))] as number;
  const gap = values.filter((v) => v > 0).length;
  return `p50 ${at(0.5).toFixed(0)}, p95 ${at(0.95).toFixed(0)}, max ${sorted[sorted.length - 1]?.toFixed(0)}, gap frames ${gap}/${values.length} (${((gap * 100) / values.length).toFixed(0)}%)`;
}

for (const variant of VARIANTS) {
  test(`perf: ${variant} momentum flicks`, async ({ page }, info) => {
    test.setTimeout(180_000);
    await page.setViewportSize({ width: 1440, height: 960 });
    await page.goto(url(release, variant));
    const scroller = page.locator(PAGES[variant].scroller);
    await scroller.waitFor();
    const box = await scroller.boundingBox();
    if (!box) throw new Error('no scroller box');
    await page.mouse.move(box.x + 200, box.y + 200);
    console.log(`PERF ${variant} viewport ${Math.round(box.width)}x${Math.round(box.height)}`);

    const rss = new RssSampler();
    rss.start();
    const results: FlickResult[] = [];
    for (const { label, opts } of FLICK_LADDER) {
      results.push(await measureFlick(page, rss, `${variant} ${label}`, opts));
    }
    rss.stop();
    console.log(
      `PERF ${variant} scrollTop after ladder ${await scroller.evaluate((el) => Math.round(el.scrollTop))}`,
    );
    await attachResults(info, results);
  });

  test(`perf: ${variant} late data`, async ({ page }) => {
    test.setTimeout(180_000);
    await page.setViewportSize({ width: 1440, height: 960 });
    await page.goto(url(hooks, variant));
    const selector = PAGES[variant].scroller;
    await page.locator(selector).waitFor();
    await page.waitForFunction(() => window.__kiraProtoTrace !== undefined);

    for (const px of BANDS) {
      await page.evaluate((sel) => {
        const el = document.querySelector(sel) as HTMLElement;
        el.scrollTop = 0;
      }, selector);
      await page.waitForTimeout(800);
      const trace = await page.evaluate(
        async ({ sel, step, frames, blank }) => {
          const el = document.querySelector(sel) as HTMLElement;
          const api = window.__kiraProtoTrace;
          if (!api) throw new Error('no trace');
          api.start({ blank });
          for (let i = 0; i < frames; i++) {
            el.scrollTop += step;
            await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
          }
          await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
          return api.stop();
        },
        { sel: selector, step: px, frames: FRAMES_PER_BAND, blank: variant === 'cheetah' },
      );
      reportLate(variant, px, trace);
    }
  });
}

function reportLate(variant: Variant, px: number, trace: TraceResult): void {
  const moving = trace.frames.filter((f) => f.pxPerFrame > 0);
  const gaps = moving.map((f) => f.gapPx);
  const label = variant === 'cheetah' ? 'lagPx' : 'uncoveredPx';
  let line = `PERF ${variant} late-${px}px/frame ${label} ${summarise(gaps)}`;
  if (variant === 'cheetah') {
    const blank = moving.map((f) => f.blankRows ?? 0);
    const worst = Math.max(0, ...blank);
    const frames = blank.filter((b) => b > 0).length;
    line += ` | blankRows max ${worst}, frames with blank ${frames}/${moving.length}`;
  }
  console.log(line);
}
