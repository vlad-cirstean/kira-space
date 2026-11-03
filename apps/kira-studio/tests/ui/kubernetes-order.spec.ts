import type { Locator, Page } from '@playwright/test';
import { expect, test } from './fixtures';
import {
  clustersIn,
  idOf,
  k,
  kubeError,
  loc,
  openKubernetes,
  setupKubernetes,
} from './support/kubernetesFixture';
import { emitWailsEvent } from './support/mockRuntime';
import { settleFrames } from './support/tree';

// Contract kubernetes. Backend half: kubeflow TestClusterOrder. Clusters reorder by pointer drag
// (kubernetes/components/clusterReorder.ts), Alt+Up/Down and the context menu.

const DEFAULT = ['kind-dev', 'bad-token', 'down', 'plugin-ctx'];
const MOVED = ['down', 'kind-dev', 'bad-token', 'plugin-ctx'];

const row = (page: Page, context?: string): Locator =>
  page.locator(`[data-testid="kube-cluster-row"]${context ? `[data-context="${context}"]` : ''}`);
const orderOf = (page: Page) =>
  row(page).evaluateAll((els) => els.map((el) => el.getAttribute('data-context')));
const indicator = (page: Page) => loc(page, 'kube-drop-indicator');

async function open(page: Page): Promise<void> {
  await openKubernetes(page);
  await expect(row(page)).toHaveCount(4);
  await expect.poll(() => orderOf(page)).toEqual(DEFAULT);
}

/** Press on `from`, cross the drag tolerance, then park over `to`'s top or bottom quarter. */
async function dragTo(page: Page, from: Locator, to: Locator, edge: 'top' | 'bottom') {
  const src = await from.boundingBox();
  const dst = await to.boundingBox();
  if (!src || !dst) throw new Error('row has no box');
  const x = src.x + src.width * 0.6;
  await page.mouse.move(x, src.y + src.height / 2);
  await page.mouse.down();
  await page.mouse.move(x, src.y + src.height / 2 + 10, { steps: 4 });
  await page.mouse.move(x, dst.y + dst.height * (edge === 'top' ? 0.25 : 0.75), { steps: 10 });
  await settleFrames(page);
}

test('contract: drag a cluster above another: drop line, one Reorder call, new order', async ({
  relaunch,
}) => {
  const { page, kube } = await setupKubernetes(relaunch);
  await open(page);

  await dragTo(page, row(page, 'down'), row(page, 'kind-dev'), 'top');
  await expect(row(page, 'kind-dev').locator('[data-testid="kube-drop-indicator"]')).toBeVisible();
  await page.mouse.up();

  await expect.poll(() => orderOf(page)).toEqual(MOVED);
  await expect(indicator(page)).toHaveCount(0);
  const calls = kube.calls('Reorder');
  expect(calls).toHaveLength(1);
  expect(calls[0].args).toEqual(k('args:KubernetesService.Reorder'));
});

test('dropping outside a cluster row, or pressing Escape, reorders nothing', async ({
  relaunch,
}) => {
  const { page, kube } = await setupKubernetes(relaunch);
  await open(page);

  await dragTo(page, row(page, 'down'), loc(page, 'kube-cluster-list'), 'bottom');
  await expect(indicator(page)).toHaveCount(0);
  await page.mouse.up();

  await dragTo(page, row(page, 'down'), row(page, 'kind-dev'), 'top');
  await expect(indicator(page)).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(indicator(page)).toHaveCount(0);
  await page.mouse.up();

  expect(kube.calls('Reorder')).toHaveLength(0);
  await expect.poll(() => orderOf(page)).toEqual(DEFAULT);
});

test('menu Move up is disabled on the first cluster; Alt+ArrowDown moves and keeps focus', async ({
  relaunch,
}) => {
  const order = [idOf('bad-token'), idOf('kind-dev'), idOf('down'), idOf('plugin-ctx')];
  const { page, kube } = await setupKubernetes(relaunch, { Reorder: () => clustersIn(order) });
  await open(page);

  await row(page, 'kind-dev').click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-move-up"]')).toHaveAttribute(
    'data-disabled',
    '',
  );
  await expect(page.locator('[data-testid="menu-item-move-down"]')).not.toHaveAttribute(
    'data-disabled',
    '',
  );
  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="context-menu"]')).toHaveCount(0);

  await row(page, 'kind-dev').focus();
  await page.keyboard.press('Alt+ArrowDown');
  await expect.poll(() => orderOf(page)).toEqual(['bad-token', 'kind-dev', 'down', 'plugin-ctx']);
  await expect(row(page, 'kind-dev')).toBeFocused();
  expect(kube.calls('Reorder')).toHaveLength(1);
  expect(kube.calls('Reorder')[0].args).toEqual({ ids: order });
});

test('a search disables reorder', async ({ relaunch }) => {
  const { page, kube } = await setupKubernetes(relaunch);
  await open(page);
  await page.click('[data-testid="toggle-search"]');
  await page.fill('[data-testid="tree-search"]', 'd');
  await expect(row(page, 'down')).toBeVisible();
  await expect(row(page, 'kind-dev')).toBeVisible();

  await dragTo(page, row(page, 'down'), row(page, 'kind-dev'), 'top');
  await expect(indicator(page)).toHaveCount(0);
  await page.mouse.up();
  expect(kube.calls('Reorder')).toHaveLength(0);

  await row(page, 'down').click({ button: 'right' });
  for (const id of ['move-up', 'move-down']) {
    await expect(page.locator(`[data-testid="menu-item-${id}"]`)).toHaveAttribute(
      'data-disabled',
      '',
    );
  }
});

test('a failed Reorder restores the stored order', async ({ relaunch }) => {
  const { page, kube } = await setupKubernetes(relaunch, {
    Reorder: () => kubeError('E_NOT_FOUND', 'cluster not found'),
  });
  await open(page);

  await dragTo(page, row(page, 'down'), row(page, 'kind-dev'), 'top');
  await page.mouse.up();
  await expect.poll(() => kube.calls('Reorder').length).toBe(1);
  await expect.poll(() => orderOf(page)).toEqual(DEFAULT);
});

test('contract: a clusters event refetches the list', async ({ relaunch }) => {
  let current: unknown = k('KubernetesService.Clusters');
  const { page, kube } = await setupKubernetes(relaunch, { Clusters: () => current });
  await open(page);

  current = k('KubernetesService.Reorder');
  await emitWailsEvent(page, 'kira:kubernetes:clusters', {});
  await expect.poll(() => orderOf(page)).toEqual(MOVED);
  expect(kube.calls('Reorder')).toHaveLength(0);
});
