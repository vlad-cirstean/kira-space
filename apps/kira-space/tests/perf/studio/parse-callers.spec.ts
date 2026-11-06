import { execFileSync } from 'node:child_process';
import { readdirSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { resolve } from 'node:path';
import type { Locator, Page } from '@playwright/test';
import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, test } from '../../ui/studio/fixtures';
import { grpcTab } from '../../ui/studio/support/apiMode';
import { installClipboardSpy } from '../../ui/studio/support/clipboard';
import { connectAndExpand, connectionCreateArgs, openConsoleFromMenu } from '../../ui/studio/support/connect';
import { IPC } from '../../ui/studio/support/ipcChannels';
import { WIDGETS_PATH } from '../../ui/studio/support/mongoFixture';
import {
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from '../../ui/studio/support/postgresFixture';
import { type ActionMetrics, actionLine, actionSummaryLine, measureAction } from './blockMeter';
import {
  CONNECTION_ID,
  connectMongo,
  CONTROL as MONGO_CONTROL,
  FIXTURE as MONGO_FIXTURE,
  makeDocs,
  openWidgets,
  readSnapshot,
} from './documentsFixture';
import { load1 } from './perfProbe';

// P163: opt-in probe of every large-input parse caller (`bun run perf:parse:studio`), asserts
// nothing. Same gate as the HTTP probe: run only at load1 <= 1.0 (docs/DEV_ENVIRONMENT.md).

const MB = 1_048_576;
const RUNS = Number(process.env.KIRA_PERF_RUNS ?? 3);
const ONLY = process.env.KIRA_PERF_CASES?.split(',');
const want = (c: string): boolean => !ONLY || ONLY.includes(c);

function jsonOfSize(bytes: number): string {
  const rows: string[] = [];
  let len = 2;
  for (let n = 0; len < bytes; n++) {
    const r = `{"id":${n},"name":"user-${n}","email":"user-${n}@example.com","active":true,"score":12.5,"tags":["a","b"]}`;
    rows.push(r);
    len += r.length + 1;
  }
  // One record per line: Monaco inserts a multi-line paste in seconds but chokes on one huge line.
  return `[${rows.join(',\n')}]`;
}

function xmlOfSize(bytes: number): string {
  const rows: string[] = [];
  let len = 13;
  for (let n = 0; len < bytes; n++) {
    const r = `<item id="${n}"><name>user-${n}</name><v>${n}</v><tag>a</tag></item>`;
    rows.push(r);
    len += r.length;
  }
  return `<root>\n${rows.join('\n')}\n</root>`;
}

function sqlScriptOfSize(bytes: number): string {
  const stmts: string[] = [];
  let len = 0;
  for (let n = 0; len < bytes; n++) {
    const s = `select a${n},b from t${n} where a=${n} and b in (select x from y);`;
    stmts.push(s);
    len += s.length + 1;
  }
  return stmts.join('\n');
}

// Pretty output is short lines only; the seeded input has one 100+ char record per line, so the
// predicate flips exactly when Beautify has applied.
const PRETTY_DONE = `() => {
  const lines = [...document.querySelectorAll('.monaco-editor .view-lines .view-line')];
  if (lines.length < 5) return false;
  return lines.every((l) => (l.textContent || '').trim().length <= 40);
}`;

// Builds the text inside the page and pastes it with a synthetic paste event: WebKit's
// `keyboard.insertText` takes minutes on a 0.5 MB insert, and the Playwright WebKit transport
// breaks on any message over 512 KB, so the text never crosses it. `gen` must be self-contained.
async function pasteInto(
  view: Locator,
  page: Page,
  gen: (bytes: number) => string,
  bytes: number,
): Promise<void> {
  await view.locator('.view-lines').click();
  await page.evaluate(
    ({ src, n }) => {
      const text = (new Function(`return (${src})`)() as (b: number) => string)(n);
      const dt = new DataTransfer();
      dt.setData('text/plain', text);
      document.activeElement?.dispatchEvent(
        new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }),
      );
    },
    { src: gen.toString(), n: bytes },
  );
}

// Seeds the editor with `gen(bytes)` and waits until a long line is on screen.
async function seedEditor(
  page: Page,
  view: Locator,
  gen: (bytes: number) => string,
  bytes: number,
): Promise<void> {
  await expect(view.locator('.monaco-editor').first()).toBeVisible({ timeout: 30_000 });
  await pasteInto(view, page, gen, bytes);
  await expect
    .poll(
      () =>
        view
          .locator('.view-line')
          .evaluateAll((ls) => Math.max(...ls.map((l) => (l.textContent ?? '').length))),
      { timeout: 120_000 },
    )
    .toBeGreaterThan(50);
}

// The WebKit transport mangles request bodies over 512 KiB, which the control mock cannot parse;
// answer such calls (tab-state saves of the seeded buffer) with `null` before it sees them.
async function answerOversizedCalls(page: Page): Promise<void> {
  await page.route('**/wails/runtime', async (route) => {
    const req = route.request();
    if (req.method() === 'POST') {
      try {
        JSON.parse(req.postData() ?? '{}');
      } catch {
        await route.fulfill({ status: 200, contentType: 'application/json', body: 'null' });
        return;
      }
    }
    await route.fallback();
  });
}

async function measure(
  name: string,
  size: string,
  open: () => Promise<Page>,
  click: string,
  done: string,
): Promise<void> {
  const results: ActionMetrics[] = [];
  for (let run = 0; run <= RUNS; run++) {
    const page = await open();
    await page.waitForTimeout(500);
    const m = await measureAction(page, click, done);
    // run 0 is the discarded warm-up.
    if (run > 0) {
      results.push(m);
      console.log(actionLine(name, size, run, m));
    }
    await page.close();
  }
  console.log(actionSummaryLine(name, size, results));
  expect(results.length).toBe(RUNS);
}

test.describe.configure({ mode: 'serial' });

const BODY_SIZES = (process.env.KIRA_PERF_SIZES_MB ?? '0.5,5').split(',').map(Number);

for (const [kind, gen, lang] of [
  ['json', jsonOfSize, 'json'],
  ['xml', xmlOfSize, 'xml'],
] as const) {
  const name = `req-body-${kind}`;
  if (want(name)) {
    for (const mb of BODY_SIZES) {
      test(`${name} ${mb}MB`, async ({ relaunch }) => {
        test.setTimeout(300_000);
        const bytes = Math.round(mb * MB);
        const tab = {
          id: 'tab-http-1',
          connectionId: null,
          path: 'request',
          kind: 'http-request',
          order: 0,
          active: true,
          state: {
            method: 'POST',
            url: 'https://api.example.com/upload',
            headers: [],
            bodyMode: 'code',
            code: '',
            codeLanguage: lang,
            requestPane: 'body',
            responsePane: 'body',
            responseView: 'pretty',
            requestPaneHeight: 0,
          },
        };
        await measure(
          name,
          `${mb}MB`,
          async () => {
            const { window } = await relaunch({
              control: [{ channel: IPC.tabsList, response: [tab] }],
            });
            await answerOversizedCalls(window);
            await expect(window.locator('[data-testid="http-request-view"]')).toBeVisible();
            await seedEditor(
              window,
              window.locator('[data-testid="http-request-pane"]'),
              gen,
              bytes,
            );
            return window;
          },
          '[data-testid="http-body-beautify"]',
          PRETTY_DONE,
        );
      });
    }
  }
}

if (want('grpc-json')) {
  for (const mb of BODY_SIZES) {
    test(`grpc-json ${mb}MB`, async ({ relaunch }) => {
      test.setTimeout(300_000);
      const bytes = Math.round(mb * MB);
      await measure(
        'grpc-json',
        `${mb}MB`,
        async () => {
          const { window } = await relaunch({
            control: [
              {
                channel: IPC.tabsList,
                response: [grpcTab({ message: '' }, { responsePane: 'messages' })],
              },
            ],
          });
          await answerOversizedCalls(window);
          await expect(window.locator('[data-testid="grpc-request-view"]')).toBeVisible();
          await seedEditor(
            window,
            window.locator('[data-testid="grpc-message-editor"]'),
            jsonOfSize,
            bytes,
          );
          return window;
        },
        '[data-testid="grpc-beautify"]',
        PRETTY_DONE,
      );
    });
  }
}

if (want('console-format')) {
  for (const kb of [64, 236, 1024]) {
    test(`console-format ${kb}KB`, async ({ relaunch }) => {
      test.setTimeout(300_000);
      const id = 'conn-perf-format';
      const summary = postgresConnectionSummary(id, 'Perf DB', 'green');
      const fixture = orderItemsFixture(id);
      const control: ControlSnapshot[] = [
        { channel: IPC.connectionsList, response: [] },
        {
          channel: IPC.connectionsCreate,
          args: connectionCreateArgs('Perf DB', 'green'),
          response: summary,
        },
        ...fixture.control,
      ];
      await measure(
        'console-format',
        `${kb}KB`,
        async () => {
          const { window } = await relaunch({ control });
          await answerOversizedCalls(window);
          await connectAndExpand(window, 'Perf DB', 'green');
          await openConsoleFromMenu(window, ORDER_ITEMS_PATH);
          const view = window.locator('[data-testid="console-view"]');
          await expect(view).toBeVisible();
          await pasteInto(view, window, sqlScriptOfSize, kb * 1024);
          await expect(view.locator('.view-line').first()).toContainText('select');
          return window;
        },
        '[data-testid="console-format"]',
        `() => {
          const lines = document.querySelectorAll('[data-testid="console-view"] .view-lines .view-line');
          if (lines.length < 3) return false;
          return [...lines].some((l) => (l.textContent || '').trim() === 'select');
        }`,
      );
    });
  }
}

// Row 10: Documents page of 10 000 documents loads; fieldNamesOnPage runs from four computeds.
if (want('doc-fieldnames')) {
  for (const [rows, pad] of [
    [10_000, 0],
    [2000, 8192],
  ] as const) {
    test(`doc-fieldnames ${rows}x${pad}B`, async ({ relaunch }) => {
      test.setTimeout(600_000);
      const docs = makeDocs(rows, pad);
      await measure(
        'doc-fieldnames',
        `${rows}x${400 + pad}B`,
        async () => {
          const { window } = await relaunch({
            control: MONGO_CONTROL,
            stream: [
              readSnapshot(100, docs, true),
              readSnapshot(10000, docs, false),
              ...MONGO_FIXTURE.port.slice(1),
            ],
          });
          await openWidgets(window);
          return window;
        },
        '[data-testid="document-page-size-10000"]',
        `() => {
          const l = document.querySelector('[data-testid="document-list"] [data-testid="virtual-list"]');
          return !!l && l.scrollHeight >= ${rows * 100};
        }`,
      );
    });
  }
}

// Row 12: console Mongo result, "Copy all" over every displayed document.
if (want('console-copy-all')) {
  for (const [rows, pad] of [
    [10_000, 0],
    [2000, 8192],
  ] as const) {
    for (const [fmt, item] of [
      ['json', 'copy-all-as-json'],
      ['shell', 'copy-all-shell'],
    ] as const) {
      test(`console-copy-all-${fmt} ${rows}x${pad}B`, async ({ relaunch }) => {
        test.setTimeout(600_000);
        const docs = makeDocs(rows, pad);
        await measure(
          `console-copy-all-${fmt}`,
          `${rows}x${400 + pad}B`,
          async () => {
            const stream: PortSnapshot[] = [
              {
                op: DATA_OP.execute,
                payload: {
                  connectionId: CONNECTION_ID,
                  path: WIDGETS_PATH,
                  statements: ['db.widgets.find()'],
                },
                response: {
                  kind: 'execute',
                  pages: [
                    {
                      kind: 'document',
                      ids: docs.ids,
                      bodies: docs.bodies,
                      position: {
                        offset: 0,
                        pageSize: rows,
                        hasMore: false,
                        nextToken: null,
                        prevToken: null,
                        strategy: 'offset',
                      },
                    },
                  ],
                },
              },
            ];
            const { window } = await relaunch({ control: MONGO_CONTROL, stream });
            await installClipboardSpy(window, { lazy: true });
            await connectMongo(window);
            await openConsoleFromMenu(window, WIDGETS_PATH);
            const view = window.locator('[data-testid="console-view"]');
            await expect(view).toBeVisible();
            await pasteInto(view, window, () => 'db.widgets.find()', 0);
            await window.click('[data-testid="console-run-statement"]');
            const first = view.locator('[data-testid="console-result-doc-row"]').first();
            await expect(first).toBeVisible({ timeout: 60_000 });
            await first.click({ button: 'right' });
            await window.locator('[data-testid="menu-item-copy-all-submenu"]').hover();
            await expect(window.locator('[data-testid="context-submenu"]')).toBeVisible();
            return window;
          },
          `[data-testid="menu-item-${item}"]`,
          `() => ((window).__clipboard || []).length > 0`,
        );
      });
    }
  }
}

// Pure caller functions timed in-page (WebKit), median over 5 runs; rows with no UI path worth
// driving: find, compare, cell editor, byte label, row menu, lint scan, EXPLAIN parse, grid copy.
if (want('pure-fns')) {
  test('pure-fns', async ({ relaunch }) => {
    test.setTimeout(600_000);
    const out = resolve(tmpdir(), `kira-pure-${process.pid}.js`);
    execFileSync(
      'bun',
      [
        'build',
        resolve(__dirname, 'pureFns.entry.ts'),
        '--outfile',
        out,
        '--target=browser',
        '--format=iife',
      ],
      { stdio: 'inherit' },
    );
    for (let run = 1; run <= RUNS; run++) {
      const { window: page } = await relaunch({ control: [] });
      await page.waitForTimeout(500);
      const load = load1();
      await page.route('**/__pure.js', (route) =>
        route.fulfill({ path: out, contentType: 'text/javascript' }),
      );
      await page.addScriptTag({ url: '/__pure.js' });
      const res = await page.evaluate(() =>
        (window as unknown as { __pure: (n: number) => Record<string, number> }).__pure(5),
      );
      console.log(`case=pure-fns run=${run} load1=${load} ${JSON.stringify(res)}`);
      await page.close();
    }
  });
}

// Worker lifecycle costs: cold start, respawn after terminate, and structured-clone cost of a
// 12 MB string posted to the worker. Needs the built `parse.worker-*.js` asset.
if (want('worker-costs')) {
  test('worker-costs', async ({ relaunch }) => {
    test.setTimeout(300_000);
    const asset = readdirSync(resolve(__dirname, '../../../frontend/dist/assets')).find((f) =>
      /^parse\.worker-.*\.js$/.test(f),
    );
    if (!asset) throw new Error('no parse.worker asset in dist');
    for (let run = 1; run <= RUNS; run++) {
      const { window: page } = await relaunch({ control: [] });
      await page.waitForTimeout(500);
      const load = load1();
      const r = await page.evaluate(async (url) => {
        const roundTrip = (w: Worker, kind: string, input: unknown): Promise<number> =>
          new Promise((res) => {
            const t0 = performance.now();
            w.onmessage = () => res(performance.now() - t0);
            w.postMessage({ id: 0, kind, input });
          });
        const small = { body: '{"a":1}', wantText: false };
        let w = new Worker(url);
        const coldMs = await roundTrip(w, 'body.format', small);
        const warmMs = await roundTrip(w, 'body.format', small);
        w.terminate();
        w = new Worker(url);
        const respawnMs = await roundTrip(w, 'body.format', small);
        const big = 'x'.repeat(12 * 1024 * 1024);
        const t0 = performance.now();
        w.postMessage({ id: 1, kind: 'body.format', input: { body: big, wantText: false } });
        const postMs = performance.now() - t0;
        await new Promise((res) => {
          w.onmessage = () => res(null);
        });
        const out = await roundTrip(w, 'json.beautify', {
          text: `[${Array.from({ length: 200_000 }, (_, i) => `{"id":${i},"name":"user-${i}"}`).join(',')}]`,
          mode: 'indented',
        });
        w.terminate();
        return { coldMs, warmMs, respawnMs, postMs, beautifyRoundTripMs: out };
      }, `/assets/${asset}`);
      console.log(
        `case=worker-costs run=${run} load1=${load} coldMs=${r.coldMs.toFixed(1)} warmMs=${r.warmMs.toFixed(1)} ` +
          `respawnMs=${r.respawnMs.toFixed(1)} post12MbMs=${r.postMs.toFixed(1)} beautify200kRoundTripMs=${r.beautifyRoundTripMs.toFixed(1)}`,
      );
      await page.close();
    }
  });
}
