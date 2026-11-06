import { expect, test } from '../../ui/studio/fixtures';
import { httpResponse, openHttpModeAndNewRequest } from '../../ui/studio/support/apiMode';
import { IPC } from '../../ui/studio/support/ipcChannels';
import { load1, RssSampler, type RunMetrics, runLine, summaryLine } from './perfProbe';

// P160: HTTP response viewer probe. Opt-in (`bun run perf:http:studio`), asserts nothing. Run on
// a quiet machine (load1 <= 1.0); runs print load1 so a noisy one can be discarded.

const MB = 1_048_576;
const SIZES = [2.4, 5, 12];
const RUNS = Number(process.env.KIRA_PERF_RUNS ?? 3);
const ONLY = process.env.KIRA_PERF_CASES?.split(',');

type Gen = (bytes: number) => { body: string; contentType: string };

function repeatTo(unit: (i: number) => string, bytes: number): string {
  const parts: string[] = [];
  let len = 0;
  for (let i = 0; len < bytes; i++) {
    const s = unit(i);
    parts.push(s);
    len += s.length;
  }
  return parts.join('');
}

const CASES: Record<string, Gen> = {
  json: (bytes) => {
    const rows: string[] = [];
    let len = 2;
    for (let n = 0; len < bytes; n++) {
      const r = `{"id":${n},"name":"user-${n}","email":"user-${n}@example.com","active":true,"score":12.5,"tags":["a","b"]}`;
      rows.push(r);
      len += r.length + 1;
    }
    return { body: `[${rows.join(',')}]`, contentType: 'application/json' };
  },
  'text-80col': (bytes) => ({
    body: repeatTo((i) => `${String(i).padStart(7, '0')} ${'x'.repeat(71)}\n`, bytes),
    contentType: 'text/plain',
  }),
  'text-short-lines': (bytes) => ({
    body: repeatTo((i) => `line ${i}\n`, bytes),
    contentType: 'text/plain',
  }),
  'text-1line': (bytes) => ({
    body: repeatTo((i) => `word${i} `, bytes).trimEnd(),
    contentType: 'text/plain',
  }),
};

test.describe.configure({ mode: 'serial' });

for (const [name, gen] of Object.entries(CASES)) {
  if (ONLY && !ONLY.includes(name)) continue;
  for (const size of SIZES) {
    test(`${name} ${size}MB`, async ({ relaunch }) => {
      test.setTimeout(600_000);
      const { body, contentType } = gen(Math.round(size * MB));
      const results: RunMetrics[] = [];
      for (let run = 1; run <= RUNS; run++) {
        const { window: page } = await relaunch({
          control: [
            {
              channel: IPC.httpSend,
              response: httpResponse({
                body,
                bodyBytes: body.length,
                headers: [{ name: 'Content-Type', value: contentType }],
              }),
            },
          ],
        });
        await openHttpModeAndNewRequest(page);
        await page.fill('[data-testid="http-url"]', 'https://api.example.com/v1');
        const warm = await page
          .waitForSelector('.monaco-editor', { timeout: 3000 })
          .then(() => true)
          .catch(() => false);
        await page.waitForTimeout(500);

        const load = load1();
        const rss = new RssSampler();
        rss.start();
        const t = await page.evaluate(async () => {
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

          let firstText: number | null = null;
          let shown: number | null = null;
          const check = () => {
            const now = performance.now();
            if (firstText === null) {
              const pre = document.querySelector('.response-body .monaco-host pre');
              const line = document.querySelector('.response-body .view-line');
              if ((pre?.textContent ?? '').length > 0 || (line?.textContent ?? '').length > 0) {
                firstText = now;
              }
            }
            if (shown === null) {
              const lines = document.querySelectorAll(
                '.response-body .monaco-host .view-lines .view-line',
              );
              for (const l of lines) {
                if ((l.textContent ?? '').trim().length > 0) {
                  requestAnimationFrame(() => {
                    shown = performance.now();
                  });
                  shown = -1;
                  break;
                }
              }
            }
          };
          const mo = new MutationObserver(check);
          mo.observe(document.body, { subtree: true, childList: true, characterData: true });

          const t0 = performance.now();
          (document.querySelector('[data-testid="http-send"]') as HTMLElement).click();
          const deadline = t0 + 120_000;
          while ((shown === null || shown < 0) && performance.now() < deadline) {
            await new Promise((r) => setTimeout(r, 20));
          }
          const shownAt = shown !== null && shown > 0 ? shown : null;
          const end = (shownAt ?? performance.now()) + 1000;
          while (performance.now() < end) await new Promise((r) => setTimeout(r, 50));
          stop = true;
          mo.disconnect();
          const inWin = gaps.filter(([at]) => at >= t0 && at <= end);
          const longest = inWin.reduce((m, [, g]) => Math.max(m, g), 0);
          const tbt = inWin.reduce((s, [, g]) => s + (g > 50 ? g - 50 : 0), 0);
          return {
            firstText: firstText === null ? null : firstText - t0,
            shown: shownAt === null ? null : shownAt - t0,
            longest,
            tbt,
          };
        });
        rss.stop();
        const m: RunMetrics = {
          load1: load,
          warm,
          firstTextMs: t.firstText,
          shownMs: t.shown,
          longestBlockMs: t.longest,
          tbtMs: t.tbt,
          rssPeakDeltaMb:
            rss.peakMb !== null && rss.baselineMb !== null ? rss.peakMb - rss.baselineMb : null,
          rssPeakMb: rss.peakMb,
        };
        results.push(m);
        console.log(runLine(name, size, run, m));
        await page.close();
      }
      console.log(summaryLine(name, size, results));
      expect(results.length).toBe(RUNS);
    });
  }
}
