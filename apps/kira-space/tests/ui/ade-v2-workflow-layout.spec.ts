import type { Locator, Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// P255: the workflow graph is locked: one direction, one node size, vertical scroll only, no zoom.

const t = (id: string) => `[data-testid="${id}"]`;

async function open(
  relaunch: Parameters<typeof openPlan>[0],
  extra: Parameters<typeof openPlan>[1] = [],
): Promise<Page> {
  const { window: page } = await openPlan(relaunch, extra);
  await page.setViewportSize({ width: 1400, height: 700 });
  await page.locator(t('ade-tab-workflows')).click();
  await page.locator(t('ade-wf-form')).waitFor();
  await page.locator(t('ade-wf-node')).first().waitFor();
  return page;
}

async function box(l: Locator): Promise<{ x: number; y: number; width: number; height: number }> {
  const b = await l.boundingBox();
  if (!b) throw new Error('no box');
  return b;
}
const node = (page: Page, step: string) =>
  page.locator(`${t('ade-wf-node')}[data-step-id="${step}"]`);
const scale = (page: Page) =>
  page
    .locator('.vue-flow__transformationpane')
    .evaluate((el) => new DOMMatrix(getComputedStyle(el).transform).a);

test('nodes cannot be dragged', async ({ relaunch }) => {
  const page = await open(relaunch);
  const n = node(page, 'plan');
  const before = await box(n);
  await page.mouse.move(before.x + 40, before.y + 20);
  await page.mouse.down();
  await page.mouse.move(before.x + 190, before.y + 20, { steps: 8 });
  await page.mouse.up();
  expect(await box(n)).toEqual(before);
});

test('every node has the same size', async ({ relaunch }) => {
  const page = await open(relaunch);
  const picks = [
    ...(await page.locator(t('ade-wf-node')).all()),
    page.locator(t('ade-wf-end-node')).first(),
  ];
  const sizes = new Set<string>();
  for (const el of picks) {
    const b = await box(el);
    sizes.add(`${Math.round(b.width)}x${Math.round(b.height)}`);
  }
  expect(sizes.size).toBe(1);
});

test('steps run top to bottom and the spine shares one column', async ({ relaunch }) => {
  const page = await open(relaunch);
  const steps = page.locator(t('ade-wf-node'));
  const ys: number[] = [];
  for (let i = 0; i < (await steps.count()); i += 1) ys.push((await box(steps.nth(i))).y);
  expect(ys).toEqual([...ys].sort((a, b) => a - b));
  const centres: number[] = [];
  for (const id of ['plan', 'impl', 'tests', 'ci', 'pr']) {
    const b = await box(node(page, id));
    centres.push(Math.round(b.x + b.width / 2));
  }
  expect(Math.max(...centres) - Math.min(...centres)).toBeLessThanOrEqual(1);
});

test('the canvas scrolls vertically and never zooms or pans sideways', async ({ relaunch }) => {
  const page = await open(relaunch);
  const graph = page.locator(t('ade-wf-graph'));
  const z0 = await scale(page);
  const m = await box(graph);
  await page.mouse.move(m.x + m.width / 2, m.y + m.height / 2);
  await page.mouse.wheel(0, 400);
  await expect.poll(() => graph.evaluate((el) => el.scrollTop)).toBeGreaterThan(0);
  await page.keyboard.down('Control');
  await page.mouse.wheel(0, -200);
  await page.keyboard.up('Control');
  await page.mouse.dblclick(m.x + 4, m.y + 4);
  expect(await scale(page)).toBe(z0);
  const sizes = await graph.evaluate((el) => [el.scrollWidth, el.clientWidth]);
  expect(sizes[0]).toBeLessThanOrEqual(sizes[1] as number);
  for (const id of ['ade-wf-zoom-in', 'ade-wf-zoom-out', 'ade-wf-fit'])
    await expect(page.locator(t(id))).toHaveCount(0);
  await expect(page.locator('.vue-flow__minimap')).toHaveCount(0);
});

test('loop edges run in a gutter left of the nodes', async ({ relaunch }) => {
  const page = await open(relaunch);
  const loops = page.locator(`${t('ade-wf-edge')}[data-loop="true"]`);
  expect(await loops.count()).toBeGreaterThan(0);
  const left = await box(node(page, 'plan'));
  for (let i = 0; i < (await loops.count()); i += 1) {
    const edge = await box(loops.nth(i).locator('path').first());
    expect(edge.x).toBeLessThan(left.x);
  }
});

test('an edit lays the graph out again', async ({ relaunch }) => {
  const page = await open(relaunch);
  const end = page.locator(t('ade-wf-end-node')).first();
  await page.locator(t('ade-wf-add-step')).click();
  await page.locator(t('ade-wf-add-step-agent')).click();
  const fresh = page.locator(`${t('ade-wf-node')}[data-step-id^="stp_"]`);
  await expect(fresh).toHaveCount(1);
  const last = await box(node(page, 'release'));
  const added = await box(fresh);
  expect(added.y).toBeGreaterThan(last.y);
  expect(Math.round(added.x + added.width / 2)).toBe(Math.round(last.x + last.width / 2));
  expect((await box(end)).y).toBeGreaterThan(added.y);
});

test('contract: a backend workflow with a result loop lays out on one spine', async ({
  relaunch,
}) => {
  const page = await open(relaunch, [
    {
      channel: IPC.adeTaskWorkflows,
      response: contract('ade-branching', 'AdeTaskService.Workflows'),
    },
  ]);
  await expect(page.locator(t('ade-wf-node'))).toHaveCount(2);

  const sizes = new Set<string>();
  for (const el of await page.locator(t('ade-wf-node')).all()) {
    const b = await box(el);
    sizes.add(`${Math.round(b.width)}x${Math.round(b.height)}`);
  }
  expect(sizes.size).toBe(1);

  const impl = await box(node(page, 'impl'));
  const review = await box(node(page, 'review'));
  expect(impl.y).toBeLessThan(review.y);
  expect(Math.abs(impl.x + impl.width / 2 - (review.x + review.width / 2))).toBeLessThanOrEqual(1);

  const loops = page.locator(`${t('ade-wf-edge')}[data-loop="true"]`);
  await expect(loops).toHaveCount(1);
  const edge = await box(loops.first().locator('path').first());
  expect(edge.x).toBeLessThan(Math.min(impl.x, review.x));
  expect(await scale(page)).toBe(1);
});
