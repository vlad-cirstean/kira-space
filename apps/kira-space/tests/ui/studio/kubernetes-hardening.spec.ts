import { expect, studioTest as test } from '../fixtures';
import { emitWailsEvent } from '../support/mockRuntime';
import { installClipboardSpy, lastClipboardWrite } from './support/clipboard';
import {
  activePanel,
  k,
  loc,
  openKind,
  openKubernetes,
  setupKubernetes,
} from './support/kubernetesFixture';

// Contract kubernetes (written by flows/kubeflow): the same truncated list the Go cap test pages.
test('contract: truncated list says sort and filter cover loaded rows only', async ({
  relaunch,
}) => {
  const { page } = await setupKubernetes(relaunch, {
    List: () => k('KubernetesService.List:truncated'),
  });
  await openKubernetes(page);
  await openKind(page, 'kind-dev', 'v1/pods');
  await expect(loc(page, 'kube-row')).toHaveCount(3);
  await expect(loc(page, 'kube-truncated')).toHaveText(
    'Showing the first 3 rows; sort and filter cover loaded rows only',
  );
});

// Contract kubernetes `event:kira:kubernetes:logs:restart`: the lines the Go restart test followed.
test('contract: copy log writes the followed lines in order', async ({ relaunch }) => {
  const { page, kube } = await setupKubernetes(relaunch);
  await openKubernetes(page);
  await openKind(page, 'kind-dev', 'v1/pods');
  await page.locator('[data-testid="kube-row"][data-name="web-1"]').click();
  await activePanel(page).locator('[data-testid="kube-detail-tab-logs"]').click();
  await expect.poll(() => kube.calls('LogsOpen').length).toBe(1);
  const { streamId } = kube.calls('LogsOpen')[0].args as { streamId: string };

  const ev = k<{ lines: { text: string }[] }>('event:kira:kubernetes:logs:restart');
  await emitWailsEvent(page, 'kira:kubernetes:logs', { streamId, lines: ev.lines, ended: false });
  await expect(page.locator('[data-testid="kube-logs-view-line"]')).not.toHaveCount(0);
  await installClipboardSpy(page);
  await loc(page, 'kube-logs-copy').click();
  await expect.poll(() => lastClipboardWrite(page)).toBe(ev.lines.map((l) => l.text).join('\n'));
});
