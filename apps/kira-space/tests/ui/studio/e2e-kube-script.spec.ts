import type { ApprovalItem } from '@shared/domain/approvals';
import type { DbMcpPendingResponse } from '@shared/domain/dbmcp';
import type { RevealResult } from '@shared/domain/variables';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { withTimes } from './support/keptTimes';

// Contract e2e-kube-script. Backend half: dbmcpflow TestE2EKubeScript, the same scenario. A kube list keeps
// a pod name and a token, the next logs call reads the pod in its prepare script, an API request reads it
// with {{kept.pod}}, and a reviewed scripted read previews the plain keep and reveals the secret behind the
// fingerprint prompt.

const SC = 'e2e-kube-script';
const TOKEN = 'tok-kube-web-1';
const item = (key: string) =>
  contract<ApprovalItem[]>(SC, `ApprovalsService.List#${key}`)[0] as ApprovalItem;
const reveal = (key: string) => contract<RevealResult>(SC, `KeptVarsService.${key}`);
const t = (id: string) => `[data-testid="${id}"]`;

test('contract: the list card marks the plain and the secret value its transform may keep', async ({
  relaunch,
}) => {
  const it = liveItem(item('list'));
  const { window: page, control } = await relaunch({});
  await pushApprovals(page, [it]);
  const c = page.locator(t(`approval-item-${it.id}`));
  await expect(c.locator(t('kube-approval-line'))).toContainText('pods');
  const mayKeep = c.locator(t('kube-approval-may-keep'));
  await expect(mayKeep).toContainText('pod');
  await expect(mayKeep).toContainText('token');
  await expect(mayKeep.locator(t('kube-approval-may-keep-token-secret'))).toHaveText('Secret');
  await expect(mayKeep.locator(t('kube-approval-may-keep-pod-plain'))).toHaveText('Plain');
  await expect(c.locator(t('kube-approval-kept-writes'))).toHaveText('Keeps: pod, token (secret)');
  await page.click(t(`approval-approve-${it.id}`));
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: true, options: null });
});

test('contract: the logs card shows the kept read, the prepared line and both scripts', async ({
  relaunch,
}) => {
  const it = withTimes(item('logs'));
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const c = page.locator(t(`approval-item-${it.id}`));
  await expect(c.locator(t('kube-approval-kept'))).toHaveText('Uses: pod');
  await expect(c.locator(t('kube-approval-kept-pod'))).toContainText('web-1');
  await expect(c.locator(t('kube-approval-kept-pod')).locator(t('kept-kind-plain'))).toBeVisible();
  const line = c.locator(t('kube-approval-line'));
  await expect(line).toContainText('logs');
  await expect(line).toContainText('web-1');
  await expect(line.locator(t('kube-approval-flag')).filter({ hasText: '2' })).toBeVisible();
  // The pod and tail the prepare script set carry the dotted underline.
  await expect(line.locator('[data-token="name"]')).toHaveClass(/decoration-dotted/);
  await expect(c.locator(t('kube-approval-script-prepare'))).toBeVisible();
  await page.click(t('approval-details-toggle'));
  const changed = c.locator(t('kube-approval-script-changed'));
  await expect(changed).toContainText('pod');
  await expect(changed).toContainText('tail');
  await expect(c.locator(t('kube-approval-detail-server'))).toBeVisible();
  await expect(page.locator('body')).not.toContainText(TOKEN);
});

test('contract: the API card reads the kube-kept pod', async ({ relaunch }) => {
  const it = withTimes(item('api'));
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const c = page.locator(t(`approval-item-${it.id}`));
  const pod = c.locator(t('api-approval-kept-pod'));
  await expect(pod).toContainText('pod');
  await expect(pod.locator(t('kept-kind-plain'))).toBeVisible();
  await expect(pod.locator(t('kept-value-preview'))).toContainText('web-1');
});

test('contract: the scripted review shows the envelope, the raw table and both kept kinds', async ({
  relaunch,
}) => {
  const it = liveItem(item('review'));
  const held = contract<DbMcpPendingResponse>(SC, 'DbMcpService.PendingResponse#review');
  const requestId = it.id.slice(it.kind.length + 1);
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.dbMcpPendingResponse, response: held },
      {
        channel: IPC.keptRevealStaged,
        args: { requestId, name: 'token', confirmed: false },
        response: reveal('RevealStaged#confirmation'),
      },
      {
        channel: IPC.keptRevealStaged,
        args: { requestId, name: 'token', confirmed: true },
        response: reveal('RevealStaged#review'),
      },
    ],
  });
  await pushApprovals(page, [it]);
  await expect(page.locator(t('kube-review-view-result'))).toBeVisible();
  await expect(page.locator(t('kube-review-result'))).toContainText('"first": "web-1"');
  await page.click(t('kube-review-view-table'));
  await expect(page.locator(t('kube-review-row'))).toHaveCount(3);
  await expect(page.locator(t('kube-review-script'))).toBeVisible();

  const block = page.locator(t('kube-review-keep'));
  await expect(block).toContainText('Will keep');
  const secret = block.locator(t('kube-review-keep-token'));
  const plain = block.locator(t('kube-review-keep-pod'));
  await expect(secret.locator(t('kept-kind-secret'))).toHaveText('Secret');
  await expect(plain.locator(t('kept-kind-plain'))).toHaveText('Plain');
  await expect(plain.locator(t('kept-value-preview'))).toContainText('web-1');
  await expect(secret.locator(t('kept-value-secret'))).toHaveText('••••••••');
  await expect(page.locator(t('kube-review-body'))).not.toContainText(TOKEN);

  await secret.locator(t('kept-value-reveal')).click();
  await expect(secret.locator(t('kept-value-revealed'))).toHaveCount(0);
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect(secret.locator(t('kept-value-revealed'))).toHaveText(TOKEN);
  await expect
    .poll(() =>
      control
        .log()
        .filter((e) => e.channel === IPC.keptRevealStaged)
        .map((e) => e.args),
    )
    .toEqual([
      { requestId, name: 'token', confirmed: false },
      { requestId, name: 'token', confirmed: true },
    ]);
  await secret.locator(t('kept-value-reveal')).click();
  await expect(secret.locator(t('kept-value-revealed'))).toHaveCount(0);

  await page.click(t(`approval-approve-${it.id}`));
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: true, options: null });
});
