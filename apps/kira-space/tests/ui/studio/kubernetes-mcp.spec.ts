import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import {
  clusters,
  idOf,
  kubeError,
  loc,
  openKubernetes,
  setupKubernetes,
} from './support/kubernetesFixture';
import { openSettings } from './support/settings';

// Contract dbmcp-kube. Backend half: dbmcpflow TestKubeMcp. A cluster's MCP switch, description and
// approval save as one SetMcp call; an AI client's Kubernetes read parks on the shared approval dialog.

const SCENARIO = 'dbmcp-kube';
interface SetMcpArgs {
  clusterId: string;
  mcp: { description: string; enabled: boolean; approval: string };
}
const setMcpArgs = (key = 'args:KubernetesService.SetMcp') => contract<SetMcpArgs>(SCENARIO, key);

async function openMcpDialog(page: Parameters<typeof loc>[0]): Promise<void> {
  await openKubernetes(page);
  await page
    .locator('[data-testid="kube-cluster-row"][data-context="kind-dev"]')
    .click({ button: 'right' });
  await page.locator('[data-testid="menu-item-kube-cluster-mcp"]').click();
  await expect(loc(page, 'kube-mcp-dialog')).toBeVisible();
}

test('contract: the cluster menu turns MCP on, sets description and approval, Save sends them', async ({
  relaunch,
}) => {
  const args = setMcpArgs();
  const { page, kube } = await setupKubernetes(relaunch);
  await openMcpDialog(page);

  await expect(loc(page, 'kube-mcp-enabled')).toHaveAttribute('data-state', 'unchecked');
  await expect(loc(page, 'kube-mcp-description')).toHaveCount(0);
  await expect(loc(page, 'kube-mcp-approval')).toHaveCount(0);
  await loc(page, 'kube-mcp-enabled').click();
  await expect(loc(page, 'kube-mcp-prompt')).toHaveAttribute('data-state', 'checked');
  await loc(page, 'kube-mcp-description').fill(args.mcp.description);
  await loc(page, `kube-mcp-${args.mcp.approval}`).click();
  await loc(page, 'kube-mcp-save').click();

  await expect(loc(page, 'kube-mcp-dialog')).toHaveCount(0);
  expect(kube.calls('SetMcp').map((c) => c.args)).toEqual([
    { ...args, clusterId: idOf('kind-dev') },
  ]);
});

test('contract: a rejected Save keeps the dialog open with the error', async ({ relaunch }) => {
  const { page } = await setupKubernetes(relaunch, {
    SetMcp: () => kubeError('E_BAD_REQUEST', 'The MCP description is too long.'),
  });
  await openMcpDialog(page);
  await loc(page, 'kube-mcp-save').click();

  await expect(loc(page, 'kube-mcp-error')).toHaveText('The MCP description is too long.');
  await expect(loc(page, 'kube-mcp-dialog')).toBeVisible();
});

async function openDbMcpSettings(page: Parameters<typeof loc>[0]): Promise<void> {
  await openSettings(page);
  await page.click('[data-testid="settings-section-Studio MCP clusters"]');
  await expect(loc(page, 'db-mcp-kube-list')).toBeVisible();
}

test('contract: the Studio MCP switch turns a cluster on with one SetMcp call', async ({
  relaunch,
}) => {
  const args = setMcpArgs('args:KubernetesService.SetMcp:enable');
  const { page, kube } = await setupKubernetes(relaunch);
  await openDbMcpSettings(page);

  const dev = idOf('kind-dev');
  await expect(loc(page, `db-mcp-kube-${dev}`)).toHaveText('Off');
  await loc(page, `db-mcp-kube-toggle-${dev}`).click();

  await expect
    .poll(() => kube.calls('SetMcp').map((c) => c.args))
    .toEqual([{ ...args, clusterId: dev }]);
});

test('contract: an enabled cluster shows its approval in Studio MCP settings', async ({
  relaunch,
}) => {
  const { page } = await setupKubernetes(relaunch, {
    Clusters: () => {
      // The flow contract masks ids; swap in the kubernetes.json ids by context.
      const res = contract<{ clusters: { id: string; context: string }[] }>(
        SCENARIO,
        'KubernetesService.Clusters:enabled',
      );
      return { ...res, clusters: res.clusters.map((c) => ({ ...c, id: idOf(c.context) })) };
    },
  });
  await openDbMcpSettings(page);

  const dev = idOf('kind-dev');
  const row = loc(page, `db-mcp-kube-row-${dev}`);
  await expect(loc(page, `db-mcp-kube-${dev}`)).toHaveText('Exposed');
  await expect(row).toContainText('Reads straight away');
  await expect(row).toContainText('dev cluster, safe to read');
  const others = clusters().filter((c) => c.id !== dev);
  for (const c of others) await expect(loc(page, `db-mcp-kube-${c.id}`)).toHaveText('Off');

  await loc(page, `db-mcp-kube-edit-${dev}`).click();
  await expect(loc(page, 'kube-mcp-dialog')).toBeVisible();
  await expect(loc(page, 'kube-mcp-enabled')).toHaveAttribute('data-state', 'checked');
  await expect(loc(page, 'kube-mcp-allow')).toHaveAttribute('data-state', 'checked');
});

test('contract: a Kubernetes read shows the cluster and answers by item id', async ({
  relaunch,
}) => {
  const [item] = contract<ApprovalItem[]>(SCENARIO, 'ApprovalsService.List');
  const answer = contract<{ ids: string[] }>(SCENARIO, 'args:ApprovalsService.Answer');
  if (!item) throw new Error('contract has no item');
  const payload = item.payload as { statement: string; namespace: string };
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(answer.ids) }],
  });

  await pushApprovals(page, [liveItem(item)]);
  await expect(loc(page, 'approval-target-name')).toHaveText(item.target.name);
  await expect(loc(page, `approval-item-${item.id}`)).toContainText(item.target.location);
  await expect(loc(page, 'approval-target-kind')).toContainText(payload.namespace);
  await expect(loc(page, 'db-mcp-approval-kube')).toContainText(payload.statement);

  await page.click(`[data-testid="approval-approve-${item.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual(answer);
});

test('contract: the Operations panel shows an MCP Kubernetes read', async ({ relaunch }) => {
  const ops = contract<Record<string, unknown>[]>(SCENARIO, 'OpsService.Recent');
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [] },
      { channel: IPC.opsRecent, response: ops },
    ],
  });

  await page.click('[data-testid="toggle-operations-panel"]');
  await expect(loc(page, 'op-row')).toHaveCount(1);
  await expect(loc(page, 'op-source-cell')).toHaveText('MCP');
  await expect(loc(page, 'op-row')).toContainText(String(ops[0]?.command));
  await loc(page, 'op-row').click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-re-run"]')).toBeDisabled();
});
