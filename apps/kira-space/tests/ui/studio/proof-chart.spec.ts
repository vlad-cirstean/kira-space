import type { Locator, Page } from '@playwright/test';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, test } from './fixtures';
import { installClipboardSpy } from './support/clipboard';
import { IPC } from './support/ipcChannels';
import {
  CHART_KEYS,
  chartControl,
  chartPendingSession,
  chartStep,
  chartViewStates,
  diffClipboardImage,
  openProofWindow,
  PENDING_ID,
  pngHeight,
  shotFolder,
} from './support/proof';

// UI half of the proof-chart scenario; the Go half is proofflow TestProofCharts. Every answer is
// that test's recorded result, so a payload the backend stores is what the window draws here.

const CAPTURE_MS = 60_000;

// biome-ignore lint/suspicious/noExplicitAny: logged bridge args
type Json = Record<string, any>;

const TITLES = [
  '#1 Errors per minute around the deploy',
  '#2 Responses by service',
  '#3 Response time histogram',
  '#4 Requests by region',
  '#5 Latency vs payload size',
  '#6 Share of 5xx by endpoint',
];
const TYPES = ['line', 'bar', 'bar', 'area', 'scatter', 'pie'];

const tab = (page: Page, n: number) =>
  page.locator('[data-testid="tab"]', { hasText: TITLES[n - 1] });
const chart = (page: Page) => page.locator('[data-testid="proof-chart"]');
const stepReady = (page: Page) =>
  expect(page.locator('[data-testid="proof-step"][data-ready="true"]').first()).toBeVisible();
const copiedImageCount = (page: Page) =>
  page.evaluate(
    () => (window as unknown as { __clipboardImages: Blob[] }).__clipboardImages.length,
  );
const viewStateCalls = (log: { channel: string; args: unknown }[]) =>
  log.filter((c) => c.channel === IPC.proofSetViewState).map((c) => c.args as Json);

async function open(
  relaunch: (
    o: Json,
  ) => Promise<{ window: Page; control: { log(): { channel: string; args: unknown }[] } }>,
  url: string,
  extra: ControlSnapshot[] = [],
) {
  const out = await relaunch({ control: chartControl(extra), timezoneId: 'UTC' });
  await openProofWindow(out.window, url);
  await stepReady(out.window);
  return out;
}

/** Centre of the first drawn path whose fill is the series colour token; bars have no ids. */
async function seriesShapes(root: Locator, token: string): Promise<{ x: number; y: number }[]> {
  return root.evaluate((root, name) => {
    const probe = document.createElement('span');
    probe.style.color = `var(${name})`;
    document.body.append(probe);
    const want = getComputedStyle(probe).color;
    probe.remove();
    const top = root.getBoundingClientRect().y;
    const out: { x: number; y: number }[] = [];
    for (const el of root.querySelectorAll('svg path')) {
      const fill = getComputedStyle(el).fill;
      if (fill !== want) continue;
      const r = el.getBoundingClientRect();
      // The legend swatch shares the series colour; shapes start below the legend row.
      if (r.width > 4 && r.height > 4 && r.y > top + 40)
        out.push({ x: r.x + r.width / 2, y: r.y + r.height / 2 });
    }
    return out.sort((a, b) => a.x - b.x);
  }, token);
}

test('contract: six chart tabs draw their data literally', async ({ relaunch, uiServer }) => {
  const { window: page } = await open(relaunch, uiServer.url);
  const icons = page.locator('[data-testid="proof-tabs"] [data-testid="tab"] .codicon-graph');
  await expect(icons).toHaveCount(6);

  for (let n = 1; n <= 6; n++) {
    await tab(page, n).click();
    await expect(chart(page)).toHaveAttribute('data-chart-type', TYPES[n - 1] as string);
    await expect(page.locator('[data-testid="proof-step"][data-ready="true"]')).toBeVisible();
  }

  await tab(page, 2).click();
  const svg = chart(page).locator('svg');
  for (const label of ['api', 'web', 'worker', '2xx', '5xx']) {
    await expect(svg.locator('text', { hasText: label }).first()).toBeVisible();
  }
  await tab(page, 3).click();
  await expect(chart(page).locator('svg text', { hasText: /^ms$/ })).toBeVisible();
  await tab(page, 5).click();
  await expect(chart(page).locator('svg text', { hasText: /^bytes$/ })).toBeVisible();
  await tab(page, 6).click();
  await expect(chart(page).locator('svg text', { hasText: '/checkout: 62.5%' })).toBeVisible();

  await tab(page, 1).click();
  await expect(chart(page).locator('svg text', { hasText: 'after <b>fix</b>' })).toBeVisible();
  await expect(page.locator('[data-testid="proof-step-result"] b')).toHaveCount(0);
  const box = await chart(page).boundingBox();
  await page.mouse.move(
    (box?.x ?? 0) + (box?.width ?? 0) / 2,
    (box?.y ?? 0) + (box?.height ?? 0) / 2,
  );
  const tip = page.locator('.kira-proof-chart-tooltip');
  await expect(tip).toContainText('after <b>fix</b>');
  await expect(tip.locator('b')).toHaveCount(0);
});

test('contract: legend, clicked point and zoom persist and restore', async ({
  relaunch,
  uiServer,
}) => {
  const states = chartViewStates();
  const bar = states.find((s) => s.ordinal === 2)?.state.chart;
  const line = states.find((s) => s.ordinal === 1)?.state.chart;
  const { window: page, control } = await open(relaunch, uiServer.url);

  await tab(page, 2).click();
  await expect(chart(page)).toHaveAttribute('data-hidden', '[]');
  await chart(page).locator('svg text', { hasText: /^5xx$/ }).click({ force: true });
  await expect(chart(page)).toHaveAttribute('data-hidden', JSON.stringify(bar?.hidden));
  await expect
    .poll(() => viewStateCalls(control.log()).at(-1)?.state.chart.hidden)
    .toEqual(bar?.hidden);

  const shapes = await seriesShapes(chart(page), '--kira-conn-blue');
  expect(shapes.length).toBe(3);
  await page.mouse.click(shapes[1]?.x ?? 0, shapes[1]?.y ?? 0);
  await expect(chart(page)).toHaveAttribute('data-selected', '0:1');
  await expect
    .poll(() => viewStateCalls(control.log()).at(-1)?.state.chart.selected)
    .toEqual(bar?.selected);

  await tab(page, 3).click();
  await tab(page, 2).click();
  await expect(chart(page)).toHaveAttribute('data-hidden', '["5xx"]');
  await expect(chart(page)).toHaveAttribute('data-selected', '0:1');

  await tab(page, 1).click();
  const box = await chart(page).boundingBox();
  await page.mouse.move(
    (box?.x ?? 0) + (box?.width ?? 0) / 2,
    (box?.y ?? 0) + (box?.height ?? 0) / 2,
  );
  await page.mouse.wheel(0, -300);
  await expect(chart(page)).not.toHaveAttribute('data-zoom', '');
  await expect
    .poll(() => viewStateCalls(control.log()).some((c) => c.ordinal === 1 && c.state.chart.zoom))
    .toBe(true);

  const { window: reloaded } = await relaunch({
    control: chartControl([{ channel: IPC.proofViewState, response: states }]),
    timezoneId: 'UTC',
  });
  await openProofWindow(reloaded, uiServer.url);
  await stepReady(reloaded);
  await expect(chart(reloaded)).toHaveAttribute(
    'data-zoom',
    `${line?.zoom?.start}-${line?.zoom?.end}`,
  );
  await tab(reloaded, 2).click();
  await expect(chart(reloaded)).toHaveAttribute('data-hidden', JSON.stringify(bar?.hidden));
  await expect(chart(reloaded)).toHaveAttribute('data-selected', '0:1');
});

test('contract: the copied screenshot holds the chart and never a tooltip', async ({
  relaunch,
  uiServer,
}) => {
  test.setTimeout(180_000);
  const { window: page, control } = await open(relaunch, uiServer.url, [
    { channel: IPC.proofViewState, response: chartViewStates() },
    { channel: IPC.proofChooseShotFolder, response: shotFolder() },
  ]);
  await tab(page, 2).click();
  await expect(chart(page)).toHaveAttribute('data-selected', '0:1');
  await installClipboardSpy(page, { image: true });

  const box = await chart(page).boundingBox();
  await page.mouse.move(
    (box?.x ?? 0) + (box?.width ?? 0) / 2,
    (box?.y ?? 0) + (box?.height ?? 0) / 2,
  );
  await expect(page.locator('.kira-proof-chart-tooltip')).toBeVisible();
  const button = page.locator('[data-testid="proof-shot"]');
  await button.hover();
  await button.click();
  await expect.poll(() => copiedImageCount(page), { timeout: CAPTURE_MS }).toBe(1);

  await page.mouse.move(0, 0);
  await expect(page.locator('.kira-proof-chart-tooltip')).toBeHidden();
  const shot = await page.locator('[data-testid="proof-centre"]').screenshot();
  const diff = await diffClipboardImage(page, shot);
  expect(diff.over).toBeLessThan(0.02);
  expect(diff.mean).toBeLessThan(6);

  await page.locator('[data-testid="proof-shot-more"]').click();
  await page.locator('[data-testid="proof-shot-save-all"]').click();
  await expect
    .poll(() => control.log().filter((c) => c.channel === IPC.proofSavePngTo).length, {
      timeout: CAPTURE_MS,
    })
    .toBe(6);
  const centre = await page.locator('[data-testid="proof-centre"]').boundingBox();
  const dpr = await page.evaluate(() => window.devicePixelRatio);
  for (const c of control.log().filter((c) => c.channel === IPC.proofSavePngTo)) {
    const height = pngHeight((c.args as Json).pngBase64);
    expect(Math.abs(height - (centre?.height ?? 0) * dpr)).toBeLessThanOrEqual(3);
  }
});

test('contract: a pending edit compares the old and new chart', async ({ relaunch, uiServer }) => {
  const pending = chartPendingSession();
  const rev = pending.pending.rev as number;
  const steps = CHART_KEYS.map((key, i) =>
    chartStep(PENDING_ID, rev, i + 1, i === 1 ? 'bar-new' : key),
  ).concat(CHART_KEYS.map((key, i) => chartStep(PENDING_ID, pending.current.rev, i + 1, key)));
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.proofWindowTarget, response: { sessionId: PENDING_ID } },
      {
        channel: IPC.proofSessions,
        response: [
          { id: PENDING_ID, name: pending.name, closedAt: 0, steps: 6, pending: true, seen: true },
        ],
      },
      { channel: IPC.proofSession, response: pending },
      ...steps,
    ],
    timezoneId: 'UTC',
  });
  await openProofWindow(page, uiServer.url);
  await stepReady(page);

  await expect(page.locator('[data-testid="proof-change-step-2"]')).toHaveAttribute(
    'data-diff',
    'changed',
  );
  await page.locator('[data-testid="proof-view-compare"]').click();
  await tab(page, 2).click();
  const oldSide = page.locator('[data-testid="proof-compare-old"] [data-testid="proof-chart"]');
  const newSide = page.locator('[data-testid="proof-compare-new"] [data-testid="proof-chart"]');
  await expect(oldSide).toHaveAttribute('data-chart-type', 'bar');
  await expect(newSide).toHaveAttribute('data-chart-type', 'bar');
  for (const [name, side, want] of [
    ['old', oldSide, '5xx: 6'],
    ['new', newSide, '5xx: 9'],
  ] as const) {
    const bars = await seriesShapes(side, '--kira-conn-blue');
    await page.mouse.move(bars[1]?.x ?? 0, bars[1]?.y ?? 0);
    await expect(
      page.locator(`[data-testid="proof-compare-${name}"] .kira-proof-chart-tooltip`),
    ).toContainText(want);
  }
});

test('a malformed chart payload shows the load-failed state', async ({ relaunch, uiServer }) => {
  const { window: page } = await relaunch({
    control: chartControl([], { 1: { data: { type: 'heatmap' } } }),
    timezoneId: 'UTC',
  });
  await openProofWindow(page, uiServer.url);
  await expect(page.getByText('Couldn’t load').first()).toBeVisible();
  await expect(chart(page)).toHaveCount(0);
});
