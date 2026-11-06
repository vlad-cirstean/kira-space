import type { Page } from '@playwright/test';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { emitWailsEvent } from '../support/mockRuntime';
import {
  activePanel,
  clustersIn,
  idOf,
  k,
  loc,
  openCluster,
  openKind,
  openKubernetes,
  setupKubernetes,
  tabChip,
} from './support/kubernetesFixture';

// Contract kubernetes. The kubeflow contracts are the backend half; this spec drives the same
// scenarios through the UI: pods first, kind bar, resources in tabs, one log stream per pod tab.

const row = (page: Page, name: string, namespace?: string) =>
  page.locator(
    `[data-testid="kube-row"][data-name="${name}"]${namespace ? `[data-namespace="${namespace}"]` : ''}`,
  );
const chip = (page: Page, key: string) => loc(page, `kube-kind-chip-${key.replaceAll('/', '_')}`);
const tabs = (page: Page) => loc(page, 'kube-tabs').locator('[data-testid="tab"]');
const logLines = (page: Page) => activePanel(page).locator('[data-testid="kube-logs-view-line"]');

async function openPermanent(page: Page, name: string, namespace?: string): Promise<void> {
  // The first click opens the pane and narrows the list, so the dblclick lands on the row after it.
  await row(page, name, namespace).click();
  await row(page, name, namespace).dblclick();
  await expect(activePanel(page).locator('[data-testid="kube-detail-title"]')).toHaveText(name);
}

async function openLogs(page: Page): Promise<void> {
  await activePanel(page).locator('[data-testid="kube-detail-tab-logs"]').click();
}

async function runPaletteCommand(page: Page, id: string): Promise<void> {
  await emitWailsEvent(page, IPC.commandPalette, null);
  await page.locator(`[data-testid="command-palette-item"][data-command-id="${id}"]`).click();
}

test('contract: a cluster opens on its pods; the sidebar has no kind rows', async ({
  relaunch,
}) => {
  const { page, kube } = await setupKubernetes(relaunch);
  await openKubernetes(page);
  await openCluster(page, 'kind-dev');

  await expect(loc(page, 'kube-row')).toHaveCount(3);
  expect(kube.calls('List')[0].args).toMatchObject({
    clusterId: idOf('kind-dev'),
    kind: 'v1/pods',
    namespace: 'default',
  });
  await expect(chip(page, 'v1/pods')).toHaveAttribute('data-state', 'on');
  await expect(loc(page, 'kube-cluster-row')).toHaveCount(4);
  await expect(loc(page, 'kube-kind-row')).toHaveCount(0);
  await expect(loc(page, 'kube-tabs')).toHaveCount(0);

  const view = await loc(page, 'kubernetes-view').boundingBox();
  const resources = await loc(page, 'kube-resources').boundingBox();
  expect(resources?.width).toBeCloseTo(view?.width ?? 0, 0);
});

test('contract: other kinds open from the picker and stay as chips', async ({ relaunch }) => {
  const { page, kube } = await setupKubernetes(relaunch);
  await openKubernetes(page);
  await openCluster(page, 'kind-dev');

  await loc(page, 'kube-kind-more').click();
  await expect(page.locator('[data-testid^="kube-kind-more-option-"]')).toHaveCount(8);
  for (const heading of ['Workloads', 'Config', 'Custom resources']) {
    await expect(loc(page, 'kube-kind-more-content')).toContainText(heading);
  }
  await loc(page, 'kube-kind-more-option-v1_secrets').click();
  await expect.poll(() => kube.calls('List').at(-1)?.args.kind).toBe('v1/secrets');
  await expect(loc(page, 'kube-kind-bar').locator('[data-testid^="kube-kind-chip-"]')).toHaveCount(
    2,
  );

  for (const key of ['apps/v1/deployments', 'v1/services', 'v1/events']) {
    await loc(page, 'kube-kind-more').click();
    await loc(page, `kube-kind-more-option-${key.replaceAll('/', '_')}`).click();
  }
  const chips = loc(page, 'kube-kind-bar').locator('[data-testid^="kube-kind-chip-"]');
  await expect(chips).toHaveCount(4);
  await expect(chip(page, 'v1/secrets')).toHaveCount(0);
  await expect(chip(page, 'v1/events')).toHaveAttribute('data-state', 'on');

  await page.reload();
  await openKubernetes(page);
  await expect(chips).toHaveCount(4);
  await expect(chip(page, 'v1/events')).toHaveAttribute('data-state', 'on');
});

test('contract: click previews, double-click keeps, so several pods stay open', async ({
  relaunch,
}) => {
  const { page, kube } = await setupKubernetes(relaunch);
  await openKubernetes(page);
  await openKind(page, 'kind-dev', 'v1/pods');

  await row(page, 'web-1').click();
  await expect(tabs(page)).toHaveCount(1);
  await expect(tabs(page)).toHaveAttribute('data-preview', 'true');
  await expect.poll(() => kube.calls('Describe').length).toBe(1);

  await row(page, 'web-2').click();
  await expect(tabs(page)).toHaveCount(1);
  await expect(tabChip(page, 'web-2')).toHaveCount(1);

  await row(page, 'web-2').dblclick();
  await expect(tabChip(page, 'web-2')).toHaveAttribute('data-preview', 'false');

  await row(page, 'db-0').click();
  await expect(tabs(page)).toHaveCount(2);
  await expect(tabChip(page, 'web-2')).toHaveAttribute('data-preview', 'false');
  await expect(tabChip(page, 'db-0')).toHaveAttribute('data-preview', 'true');

  await row(page, 'web-2').click();
  await expect(tabs(page)).toHaveCount(2);
  await expect(tabChip(page, 'web-2')).toHaveAttribute('data-active', 'true');
  await expect(row(page, 'web-2')).toHaveAttribute('aria-selected', 'true');
});

test('contract: each pod tab keeps its own log stream', async ({ relaunch }) => {
  const { page, kube } = await setupKubernetes(relaunch);
  await openKubernetes(page);
  await openKind(page, 'kind-dev', 'v1/pods');

  await openPermanent(page, 'web-1');
  await openLogs(page);
  await expect.poll(() => kube.calls('LogsOpen').length).toBe(1);
  await openPermanent(page, 'web-2');
  await openLogs(page);
  await expect.poll(() => kube.calls('LogsOpen').length).toBe(2);
  const [first, second] = kube
    .calls('LogsOpen')
    .map((c) => c.args as { streamId: string; pod: string });
  expect(first.pod).toBe('web-1');
  expect(second.pod).toBe('web-2');
  expect(first.streamId).not.toBe(second.streamId);

  await emitWailsEvent(page, 'kira:kubernetes:logs', {
    streamId: first.streamId,
    lines: [{ text: 'from web-1' }],
    ended: false,
  });
  await tabChip(page, 'web-1').locator('button').first().click();
  await expect(logLines(page)).toHaveCount(1);
  await expect(logLines(page)).toContainText('from web-1');
  expect(kube.calls('LogsOpen')).toHaveLength(2);

  await runPaletteCommand(page, 'tab-close');
  await expect
    .poll(() => kube.calls('LogsClose').map((c) => c.args.streamId))
    .toEqual([first.streamId]);
  await expect(tabs(page)).toHaveCount(1);
  await expect(tabChip(page, 'web-2')).toHaveAttribute('data-active', 'true');
});

test('contract: Open logs from the row menu', async ({ relaunch }) => {
  const { page, kube } = await setupKubernetes(relaunch);
  await openKubernetes(page);
  await openKind(page, 'kind-dev', 'v1/pods');

  await row(page, 'db-0').click({ button: 'right' });
  await page.locator('[data-testid="menu-item-kube-open-logs"]').click();
  await expect(tabChip(page, 'db-0')).toHaveAttribute('data-preview', 'false');
  await expect(activePanel(page).locator('[data-testid="kube-detail-tab-logs"]')).toHaveAttribute(
    'data-state',
    'on',
  );
  await expect.poll(() => kube.calls('LogsOpen').length).toBe(1);
  expect(kube.calls('LogsOpen')[0].args).toMatchObject({ pod: 'db-0' });

  await openKind(page, 'kind-dev', 'v1/secrets');
  await row(page, 'db-creds').click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-kube-open"]')).toBeVisible();
  await expect(page.locator('[data-testid="menu-item-kube-open-logs"]')).toHaveCount(0);
});

test('the list hides, resizes and compacts beside tabs', async ({ relaunch }) => {
  const { page } = await setupKubernetes(relaunch);
  await openKubernetes(page);
  await openKind(page, 'kind-dev', 'v1/pods');
  await row(page, 'web-1').click();
  await expect(loc(page, 'kube-tabs')).toBeVisible();

  await loc(page, 'kube-list-toggle').click();
  await expect(loc(page, 'kube-resources')).toBeHidden();
  await loc(page, 'kube-list-toggle').click();
  await expect(loc(page, 'kube-resources')).toBeVisible();

  await loc(page, 'kube-list-toggle').click();
  await page.addInitScript(() => localStorage.setItem('kira.kubernetes.listWidth', '300'));
  await page.reload();
  await openKubernetes(page);
  await expect(loc(page, 'kube-table')).toBeVisible();
  await expect(loc(page, 'kube-table')).toHaveAttribute('data-compact', 'false');
  await row(page, 'web-1').click();
  await expect(loc(page, 'kube-resources')).toBeHidden();

  await loc(page, 'kube-list-toggle').click();
  await expect(loc(page, 'kube-table')).toHaveAttribute('data-compact', 'true');
  const head = loc(page, 'kube-table-head');
  await expect(head).toContainText('Name');
  await expect(head).toContainText('Ready');
  await expect(head).toContainText('Status');
  await expect(head).not.toContainText('Restarts');
  await expect(head).not.toContainText('Age');
});

test('tabs outlive a cluster switch; a removed context drops its tabs', async ({ relaunch }) => {
  let current: unknown = k('KubernetesService.Clusters');
  const { page } = await setupKubernetes(relaunch, { Clusters: () => current });
  await openKubernetes(page);
  await openKind(page, 'kind-dev', 'v1/pods');
  await row(page, 'web-1').click();
  await expect(tabs(page)).toHaveCount(1);

  await page.locator('[data-testid="kube-cluster-row"][data-context="plugin-ctx"]').click();
  await expect(loc(page, 'kube-unavailable')).toBeVisible();
  await expect(tabs(page)).toHaveCount(1);

  current = clustersIn([idOf('bad-token'), idOf('down'), idOf('plugin-ctx')]);
  await emitWailsEvent(page, 'kira:kubernetes:clusters', {});
  await expect(loc(page, 'kube-cluster-row')).toHaveCount(3);
  await expect(loc(page, 'kube-tabs')).toHaveCount(0);
});

test('same name in two namespaces gets a qualified title', async ({ relaunch }) => {
  const columns = [
    { name: 'Namespace', type: 'string' },
    { name: 'Name', type: 'string' },
    { name: 'Ready', type: 'string' },
    { name: 'Status', type: 'string' },
  ];
  const pod = (namespace: string) => ({
    key: `uid-${namespace}-web-1`,
    name: 'web-1',
    namespace,
    cells: [namespace, 'web-1', '1/1', 'Running'],
  });
  const { page } = await setupKubernetes(relaunch, {
    List: () => ({ columns, rows: [pod('default'), pod('staging')], truncated: false }),
  });
  await openKubernetes(page);
  await openKind(page, 'kind-dev', 'v1/pods');

  await openPermanent(page, 'web-1', 'default');
  await openPermanent(page, 'web-1', 'staging');
  await expect(tabs(page)).toHaveCount(2);
  await expect(tabChip(page, 'web-1 · kind-dev/default')).toHaveCount(1);
  await expect(tabChip(page, 'web-1 · kind-dev/staging')).toHaveCount(1);
});

test('the close tab command closes the active Kubernetes tab', async ({ relaunch }) => {
  const { page } = await setupKubernetes(relaunch);
  await openKubernetes(page);
  await openKind(page, 'kind-dev', 'v1/pods');
  await openPermanent(page, 'web-1');
  await openPermanent(page, 'web-2');
  await expect(tabs(page)).toHaveCount(2);

  await runPaletteCommand(page, 'tab-close');
  await expect(tabs(page)).toHaveCount(1);
  await expect(tabChip(page, 'web-1')).toHaveAttribute('data-active', 'true');
});

test('the Window menu tab channels step and close Kubernetes tabs', async ({ relaunch }) => {
  const { page } = await setupKubernetes(relaunch);
  await openKubernetes(page);
  await openKind(page, 'kind-dev', 'v1/pods');
  await openPermanent(page, 'web-1');
  await openPermanent(page, 'web-2');
  await expect(tabChip(page, 'web-2')).toHaveAttribute('data-active', 'true');

  await emitWailsEvent(page, IPC.tabPrev, null);
  await expect(tabChip(page, 'web-1')).toHaveAttribute('data-active', 'true');
  await emitWailsEvent(page, IPC.tabNext, null);
  await expect(tabChip(page, 'web-2')).toHaveAttribute('data-active', 'true');

  await emitWailsEvent(page, IPC.tabClose, null);
  await expect(tabs(page)).toHaveCount(1);
  await expect(tabChip(page, 'web-1')).toHaveAttribute('data-active', 'true');
});
