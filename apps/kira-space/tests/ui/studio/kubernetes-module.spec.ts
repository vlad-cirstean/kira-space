import { expect, test } from './fixtures';
import {
  activePanel,
  idOf,
  k,
  kubeError,
  loc,
  openKind,
  openKubernetes,
  setupKubernetes,
  tabChip,
} from './support/kubernetesFixture';
import { emitWailsEvent } from './support/mockRuntime';

const POD = { namespace: 'default', name: 'web-1' };
const row = (page: Parameters<typeof loc>[0], name: string) =>
  page.locator(`[data-testid="kube-row"][data-name="${name}"]`);

test('contract: clusters connect lazily; only the opened one is probed', async ({ relaunch }) => {
  const { page, kube } = await setupKubernetes(relaunch);
  await openKubernetes(page);

  await expect(loc(page, 'kube-cluster-row')).toHaveCount(4);
  await expect(
    page.locator('[data-testid="kube-cluster-row"][data-context="kind-dev"]'),
  ).toContainText('Current context');
  expect(kube.calls('Status')).toHaveLength(0);

  await page.locator('[data-testid="kube-cluster-row"][data-context="kind-dev"]').click();
  await expect(
    page.locator('[data-testid="kube-cluster-row"][data-context="kind-dev"]'),
  ).toHaveAttribute('data-state', 'ok');
  expect(kube.calls('Status').map((c) => c.args.clusterId)).toEqual([idOf('kind-dev')]);
  await expect(
    page.locator('[data-testid="kube-kind-chip-v1_pods"][data-state="on"]'),
  ).toBeVisible();
  await loc(page, 'kube-kind-more').click();
  await expect(page.locator('[data-testid^="kube-kind-more-option-"]')).toHaveCount(8);
});

test('contract: missing and invalid kubeconfig show the start state', async ({ relaunch }) => {
  const missing = await setupKubernetes(relaunch, {
    Clusters: () => k('KubernetesService.Clusters:missing'),
  });
  await openKubernetes(missing.page);
  await expect(loc(missing.page, 'kube-config-missing')).toBeVisible();

  const invalid = await setupKubernetes(relaunch, {
    Clusters: () => k('KubernetesService.Clusters:invalid'),
  });
  await openKubernetes(invalid.page);
  await expect(loc(invalid.page, 'kube-config-invalid')).toBeVisible();
});

test('contract: unauthorized and plugin failures show the unavailable state; Retry refreshes', async ({
  relaunch,
}) => {
  const { page, kube } = await setupKubernetes(relaunch);
  await openKubernetes(page);

  await page.locator('[data-testid="kube-cluster-row"][data-context="bad-token"]').click();
  await expect(loc(page, 'kube-unavailable')).toHaveAttribute('data-reason', 'unauthorized');
  await loc(page, 'kube-retry').click();
  await expect.poll(() => kube.calls('Status').some((c) => c.args.refresh === true)).toBe(true);

  await page.locator('[data-testid="kube-cluster-row"][data-context="plugin-ctx"]').click();
  await expect(loc(page, 'kube-unavailable')).toHaveAttribute('data-reason', 'plugin');
});

test('contract: pod list, filters, namespace switch and truncation', async ({ relaunch }) => {
  const { page, kube } = await setupKubernetes(relaunch);
  await openKubernetes(page);
  await openKind(page, 'kind-dev', 'v1/pods');

  await expect(loc(page, 'kube-row')).toHaveCount(3);
  expect(kube.calls('List').at(-1)?.args).toMatchObject({
    clusterId: idOf('kind-dev'),
    kind: 'v1/pods',
    namespace: 'default',
    labelSelector: '',
  });

  await loc(page, 'kube-list-filter').fill('web');
  await expect(loc(page, 'kube-row')).toHaveCount(2);
  await expect(row(page, 'web-1').locator('mark').first()).toHaveText('web');
  await loc(page, 'kube-list-filter').fill('');

  await loc(page, 'kube-label-selector').fill('app=web');
  await loc(page, 'kube-label-selector').press('Enter');
  await expect.poll(() => kube.calls('List').at(-1)?.args.labelSelector).toBe('app=web');

  await loc(page, 'kube-label-selector').fill('!!');
  await loc(page, 'kube-label-selector').press('Enter');
  await expect(loc(page, 'kube-selector-error')).toContainText('invalid label selector');
  await loc(page, 'kube-label-selector').fill('');
  await loc(page, 'kube-label-selector').press('Enter');
  await expect(loc(page, 'kube-row')).toHaveCount(3);

  await loc(page, 'kube-namespace-picker').click();
  await page.getByRole('option', { name: 'All namespaces' }).click();
  await expect.poll(() => kube.calls('List').at(-1)?.args.namespace).toBe('');
  await expect(loc(page, 'kube-table-head')).toContainText('Namespace');
});

test('contract: truncated list shows the cap notice', async ({ relaunch }) => {
  const { page } = await setupKubernetes(relaunch, {
    List: () => k('KubernetesService.List:truncated'),
  });
  await openKubernetes(page);
  await openKind(page, 'kind-dev', 'v1/pods');
  await expect(loc(page, 'kube-truncated')).toBeVisible();
});

test('contract: restricted account falls back to the allowed namespace', async ({ relaunch }) => {
  const { page, kube } = await setupKubernetes(relaunch, {
    Namespaces: () => k('KubernetesService.Namespaces:restricted'),
  });
  await openKubernetes(page);
  await openKind(page, 'kind-dev', 'v1/pods');
  await expect.poll(() => kube.calls('List').at(-1)?.args.namespace).toBe('default');
});

test('contract: describe filters across sections; Secret values stay redacted', async ({
  relaunch,
}) => {
  const { page, kube } = await setupKubernetes(relaunch);
  await openKubernetes(page);
  await openKind(page, 'kind-dev', 'v1/pods');
  await row(page, 'web-1').click();

  await expect(activePanel(page)).toBeVisible();
  expect(kube.calls('Describe').at(-1)?.args).toMatchObject({ ...POD, kind: 'v1/pods' });
  await expect(
    activePanel(page).locator('[data-testid="kube-describe-section-conditions"]'),
  ).toBeVisible();
  await expect(
    activePanel(page).locator('[data-testid="kube-describe-section-events"]'),
  ).toBeVisible();

  await loc(page, 'kube-describe-filter').fill('Scheduled');
  await expect(
    activePanel(page).locator('[data-testid="kube-describe-section-conditions"]'),
  ).toBeVisible();
  await expect(
    activePanel(page).locator('[data-testid="kube-describe-section-events"]'),
  ).toHaveCount(0);
  await loc(page, 'kube-describe-filter').fill('zzz-nothing');
  await expect(loc(page, 'kube-describe-no-match')).toBeVisible();
  await tabChip(page, 'web-1').locator('[data-testid="tab-close"]').click();
  await expect(loc(page, 'kube-detail')).toHaveCount(0);

  await openKind(page, 'kind-dev', 'v1/secrets');
  await row(page, 'db-creds').click();
  await expect(loc(page, 'kube-manifest')).toContainText('<14 bytes>');
  await expect(loc(page, 'kube-manifest')).not.toContainText('hunter2');
});

test('contract: pod logs stream, filter, and reopen on option change', async ({ relaunch }) => {
  const { page, kube } = await setupKubernetes(relaunch);
  await openKubernetes(page);
  await openKind(page, 'kind-dev', 'v1/pods');
  await row(page, 'web-1').click();
  await activePanel(page).locator('[data-testid="kube-detail-tab-logs"]').click();

  await expect.poll(() => kube.calls('LogsOpen').length).toBe(1);
  const first = kube.calls('LogsOpen')[0].args as { streamId: string };
  expect(first).toMatchObject({
    namespace: POD.namespace,
    pod: 'web-1',
    container: 'app',
    tail: 1000,
    previous: false,
  });

  await emitWailsEvent(page, 'kira:kubernetes:logs', {
    streamId: first.streamId,
    lines: [{ text: 'hello world' }, { text: 'boom failure' }],
    ended: false,
  });
  const lines = page.locator('[data-testid="kube-logs-view-line"]');
  await expect(lines).toHaveCount(2);

  await loc(page, 'kube-logs-filter').fill('boom');
  await expect(lines).toHaveCount(1);
  await expect(loc(page, 'kube-logs-matches')).toContainText('1 of 2');
  await loc(page, 'kube-logs-filter').fill('');

  await loc(page, 'kube-logs-previous').click();
  await expect.poll(() => kube.calls('LogsOpen').length).toBe(2);
  expect(kube.calls('LogsOpen')[1].args).toMatchObject({ previous: true });
  expect(kube.calls('LogsClose').length).toBeGreaterThan(0);

  const second = kube.calls('LogsOpen')[1].args as { streamId: string };
  await emitWailsEvent(page, 'kira:kubernetes:logs', {
    streamId: second.streamId,
    lines: [],
    ended: true,
  });
  await expect(loc(page, 'kube-logs-ended')).toBeVisible();
  await expect(loc(page, 'kube-logs-reconnect')).toBeVisible();
});

test('contract: a list error shows the load-failed state', async ({ relaunch }) => {
  const { page } = await setupKubernetes(relaunch, { List: () => kubeError('E_INTERNAL', 'boom') });
  await openKubernetes(page);
  await openKind(page, 'kind-dev', 'v1/pods');
  await expect(loc(page, 'query-error')).toBeVisible();
});
