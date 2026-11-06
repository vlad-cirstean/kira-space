import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { emitWailsEvent } from '../support/mockRuntime';
import {
  activePanel,
  idOf,
  k,
  loc,
  openKind,
  openKubernetes,
  setupKubernetes,
} from './support/kubernetesFixture';

// Contract kubernetes. Backend half: kubeflow TestOpsRecordEveryAction. Every cluster call is one
// Operations panel row; refetches of loaded data skip the log.

type Op = Record<string, unknown>;
// The contract masks id, startedAt and durationMs; the panel needs distinct real ones.
const hydrate = (op: Op, i: number): Op => ({
  ...op,
  id: `kube-op-${i}`,
  startedAt: new Date(Date.UTC(2026, 0, 1, 0, 0, 60 - i)).toISOString(),
  durationMs: op.status === 'running' ? null : 40 + i,
  error: op.status === 'error' ? `cluster call ${i} failed` : op.error,
});
const recent = () => k<Op[]>('StudioOpsService.Recent').map(hydrate);
const running = () => hydrate(k<Op[]>('StudioOpsService.Recent:running')[0], 99);

test('contract: the Operations panel lists every Kubernetes action', async ({ relaunch }) => {
  const ops = recent();
  const { page } = await setupKubernetes(relaunch, {}, [
    { channel: IPC.connectionsList, response: [] },
    { channel: IPC.studioOpsRecent, response: ops },
  ]);

  await page.click('[data-testid="toggle-operations-panel"]');
  await expect(loc(page, 'op-row')).toHaveCount(ops.length);
  for (const [i, op] of ops.entries()) {
    await expect(loc(page, 'op-row').nth(i)).toContainText(
      String(op.status === 'error' ? op.error : op.command),
    );
  }
  await expect(loc(page, 'op-source-cell')).toHaveCount(ops.length);
  for (const cell of await loc(page, 'op-source-cell').all()) await expect(cell).toHaveText('User');

  const byStatus = new Map<string, number>();
  for (const op of ops) byStatus.set(String(op.status), (byStatus.get(String(op.status)) ?? 0) + 1);
  const labels = await loc(page, 'op-status-icon').evaluateAll((els) =>
    els.map((el) => el.getAttribute('aria-label')),
  );
  const byLabel = new Map<string | null, number>();
  for (const l of labels) byLabel.set(l, (byLabel.get(l) ?? 0) + 1);
  expect([...byLabel.values()].sort()).toEqual([...byStatus.values()].sort());

  await loc(page, 'op-row').first().click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-re-run"]')).toBeDisabled();
});

test('contract: Cancel in the panel ends a running log stream', async ({ relaunch }) => {
  const live = running();
  const { page, kube, control } = await setupKubernetes(relaunch, {}, [
    { channel: IPC.connectionsList, response: [] },
    { channel: IPC.studioOpsRecent, response: [] },
  ]);
  await openKubernetes(page);
  await openKind(page, 'kind-dev', 'v1/pods');
  await loc(page, 'kube-row').first().click();
  await activePanel(page).locator('[data-testid="kube-detail-tab-logs"]').click();
  await expect.poll(() => kube.calls('LogsOpen').length).toBe(1);
  const { streamId } = kube.calls('LogsOpen')[0].args as { streamId: string };

  await page.click('[data-testid="toggle-operations-panel"]');
  await emitWailsEvent(page, IPC.studioOpUpdate, live);
  await expect(loc(page, 'op-row')).toHaveCount(1);
  await loc(page, 'op-row').click({ button: 'right' });
  await page.locator('[data-testid="menu-item-cancel"]').click();
  await expect
    .poll(() =>
      control
        .log()
        .filter((e) => e.channel === IPC.studioOpsCancel)
        .map((e) => e.args),
    )
    .toEqual([{ opId: live.id }]);

  await emitWailsEvent(page, IPC.studioOpUpdate, {
    ...live,
    status: 'cancelled',
    durationMs: 1200,
  });
  await emitWailsEvent(page, 'kira:kubernetes:logs', { streamId, lines: [], ended: true });
  await expect(loc(page, 'kube-logs-ended')).toBeVisible();
  await expect(loc(page, 'kube-logs-reconnect')).toBeVisible();
});

test('contract: a refetch is background; first load and a namespace switch are not', async ({
  relaunch,
}) => {
  const { page, kube } = await setupKubernetes(relaunch);
  // Installed before any query runs, so the refetch timer belongs to the fake clock.
  await page.clock.install();
  await page.clock.pauseAt(new Date(Date.now() + 1000));
  await openKubernetes(page);
  await openKind(page, 'kind-dev', 'v1/pods');
  await expect(loc(page, 'kube-row')).toHaveCount(3);

  expect(kube.calls('Status')[0].args).toMatchObject({
    clusterId: idOf('kind-dev'),
    background: false,
  });
  expect(kube.calls('List')).toHaveLength(1);
  expect(kube.calls('List')[0].args).toMatchObject({ background: false });

  await expect
    .poll(async () => {
      await page.clock.runFor(11_000);
      return kube.calls('List').length;
    })
    .toBeGreaterThan(1);
  expect(kube.calls('List')[1].args).toMatchObject({ namespace: 'default', background: true });

  await loc(page, 'kube-namespace-picker').click();
  await page.getByRole('option', { name: 'All namespaces' }).click();
  await expect.poll(() => kube.calls('List').at(-1)?.args.namespace).toBe('');
  expect(kube.calls('List').at(-1)?.args).toMatchObject({ background: false });
});
