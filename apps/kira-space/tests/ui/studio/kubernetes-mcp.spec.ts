import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { idOf, kubeError, loc, openKubernetes, setupKubernetes } from './support/kubernetesFixture';
import { openSettings } from './support/settings';

// Contract dbmcp-kube. Backend half: dbmcpflow TestKubeMcp. A cluster's MCP settings save as one
// SetMcp call; an AI client's Kubernetes read parks on the shared approval dialog.

const SCENARIO = 'dbmcp-kube';
const setMcpArgs = () =>
  contract<{ clusterId: string; mcp: Record<string, string> }>(
    SCENARIO,
    'args:KubernetesService.SetMcp',
  );

async function openMcpDialog(page: Parameters<typeof loc>[0]): Promise<void> {
  await openKubernetes(page);
  await page
    .locator('[data-testid="kube-cluster-row"][data-context="kind-dev"]')
    .click({ button: 'right' });
  await page.locator('[data-testid="menu-item-kube-cluster-mcp"]').click();
  await expect(loc(page, 'kube-mcp-dialog')).toBeVisible();
}

test('contract: the cluster menu edits MCP settings and Save sends them', async ({ relaunch }) => {
  const args = setMcpArgs();
  const { page, kube } = await setupKubernetes(relaunch);
  await openMcpDialog(page);

  for (const group of ['discover', 'list', 'describe', 'logs']) {
    await expect(loc(page, `kube-mcp-${group}-prompt`)).toHaveAttribute('data-state', 'on');
  }
  await loc(page, 'kube-mcp-description').fill(args.mcp.description ?? '');
  await loc(page, `kube-mcp-discover-${args.mcp.discover}`).click();
  await loc(page, `kube-mcp-list-${args.mcp.list}`).click();
  await loc(page, `kube-mcp-describe-${args.mcp.describe}`).click();
  await loc(page, `kube-mcp-logs-${args.mcp.logs}`).click();
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

test('contract: Database MCP settings list the clusters and edit one', async ({ relaunch }) => {
  const { page } = await setupKubernetes(relaunch);
  await openSettings(page);
  await page.click('[data-testid="settings-section-Database MCP"]');

  const dev = idOf('kind-dev');
  await expect(loc(page, 'db-mcp-kube-list')).toBeVisible();
  await expect(loc(page, `db-mcp-kube-row-${dev}`)).toContainText(
    'Discover prompt · list prompt · describe prompt · logs prompt',
  );
  await loc(page, `db-mcp-kube-edit-${dev}`).click();
  await expect(loc(page, 'kube-mcp-dialog')).toBeVisible();
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
  await expect(loc(page, 'db-mcp-approval-kube-namespace')).toHaveText(payload.namespace);
  await expect(loc(page, 'approvals-dialog')).toContainText('wants to list resources');
  await expect(loc(page, 'db-mcp-approval-kube')).toContainText(payload.statement);

  await page.click(`[data-testid="approval-approve-${item.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual(answer);
});

test('contract: the Operations panel shows an MCP Kubernetes read', async ({ relaunch }) => {
  const ops = contract<Record<string, unknown>[]>(SCENARIO, 'StudioOpsService.Recent');
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [] },
      { channel: IPC.studioOpsRecent, response: ops },
    ],
  });

  await page.click('[data-testid="toggle-operations-panel"]');
  await expect(loc(page, 'op-row')).toHaveCount(1);
  await expect(loc(page, 'op-source-cell')).toHaveText('MCP');
  await expect(loc(page, 'op-row')).toContainText(String(ops[0]?.command));
  await loc(page, 'op-row').click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-re-run"]')).toBeDisabled();
});
