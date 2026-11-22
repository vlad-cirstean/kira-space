import { CAPTURE_LAUNCH, CAPTURE_NOW } from '@workbench/testing/screenshots/capture';
import { expect, test as plainTest, studioTest as test } from '../../ui/fixtures';
import {
  activePanel,
  loc,
  openCluster,
  openKubernetes,
  setupKubernetes,
} from '../../ui/studio/support/kubernetesFixture';
import { seedApprovals } from '../../ui/support/approvals';
import { IPC } from '../../ui/support/ipcChannels';
import { emitWailsEvent } from '../../ui/support/mockRuntime';
import { storyBoard } from '../support/buildStory';
import { expandApprovalsDialog, shoot } from '../support/shoot';
import { storyKubeRead, storyKubeReview } from '../support/storyApi';
import { ACME_KUBE, NAMESPACE, POD, POD_LOG, STAGING } from './support/acmeKube';

// P734: the Kubernetes module: clusters, the pods of one namespace, a pod's description and its log.
// Run by `bun run screenshots`; output goes to /screenshots.

test('kubernetes module: pods, description and logs', async ({ relaunch, consoleErrors }) => {
  const { page, kube } = await setupKubernetes(relaunch, ACME_KUBE, [], CAPTURE_LAUNCH);
  await openKubernetes(page);
  await openCluster(page, STAGING);
  await expect(
    page.locator('[data-testid="kube-kind-chip-v1_pods"][data-state="on"]'),
  ).toBeVisible();
  await page.locator(`[data-testid="kube-row"][data-name="${POD}"]`).click();
  await expect(activePanel(page)).toContainText(NAMESPACE);
  await page.clock.setFixedTime(CAPTURE_NOW + 1000);
  await shoot(page, 'studio-kube-pods', consoleErrors);

  await activePanel(page).locator('[data-testid="kube-detail-tab-logs"]').click();
  await expect.poll(() => kube.calls('LogsOpen').length).toBe(1);
  const { streamId } = kube.calls('LogsOpen')[0].args as { streamId: string };
  await emitWailsEvent(page, 'kira:kubernetes:logs', {
    streamId,
    lines: POD_LOG.map((text) => ({ text })),
    ended: true,
  });
  await expect(loc(page, 'kube-logs-view-line')).toHaveCount(POD_LOG.length);
  // Wider and taller than the window shot: one-row toolbars and every log line in view.
  await page.setViewportSize({ width: 1400, height: 1000 });
  await shoot(page, 'studio-kube-logs', consoleErrors);
});

const t = (id: string) => `[data-testid="${id}"]`;
const TALL = { ...CAPTURE_LAUNCH, viewport: { width: 1024, height: 1100 } } as const;

plainTest('kubernetes approvals: read with a script', async ({ relaunch, consoleErrors }) => {
  const item = storyKubeRead();
  const { window: win } = await relaunch({ ...TALL, control: storyBoard(seedApprovals([item])) });
  const card = win.locator(`[data-testid="approval-item-${item.id}"]`);
  await expect(card).toContainText('acme-staging');
  await expect(card.locator(t('kube-approval-script-transform'))).toBeVisible();
  await win.clock.setFixedTime(CAPTURE_NOW + 1000);
  await expandApprovalsDialog(win);
  await shoot(win, 'studio-kube-read', consoleErrors);
});

plainTest('kubernetes approvals: held pod list', async ({ relaunch, consoleErrors }) => {
  const { item, held } = storyKubeReview();
  const { window: win } = await relaunch({
    ...TALL,
    control: storyBoard([
      ...seedApprovals([item]),
      { channel: IPC.dbMcpPendingResponse, response: held },
    ]),
  });
  const card = win.locator(`[data-testid="approval-item-${item.id}"]`);
  await expect(card).toContainText('Read result');
  await win.clock.setFixedTime(CAPTURE_NOW + 1000);
  await expandApprovalsDialog(win);
  await shoot(win, 'studio-kube-review', consoleErrors);
});
