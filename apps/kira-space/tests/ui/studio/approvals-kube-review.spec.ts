import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';

// Contract kube-review. Backend half: dbmcpflow TestKubeResponseReview, TestKubeResponseWithhold and
// TestKubeReadReviewOption. A held kube_list_resources, kube_describe or kube_pod_logs result parks as a
// kube.response item; its bytes come from the pending-response query, never the payload.

interface Held {
  requestId: string;
  text: string;
  isError: boolean;
}

const SC = 'kube-review';
const item = (key: string) => contract<ApprovalItem[]>(SC, `ApprovalsService.List#${key}`)[0];
const held = (key: string) => contract<Held>(SC, `DbMcpService.PendingResponse#${key}`);
const answerIds = (key: string) =>
  contract<{ ids: string[] }>(SC, `args:ApprovalsService.Answer#${key}`).ids;
const t = (id: string) => `[data-testid="${id}"]`;

const mocks = (key: string, text?: string) => [
  { channel: IPC.dbMcpPendingResponse, response: { ...held(key), ...(text ? { text } : {}) } },
  { channel: IPC.approvalsAnswer, response: answeredOk(answerIds(key)) },
];

test('contract: a held list shows as a table, expands a row, and Send answers by id', async ({
  relaunch,
}) => {
  const it = item('list');
  const { window: page, control } = await relaunch({ control: mocks('list') });
  await pushApprovals(page, [liveItem(it)]);
  await expect(page.locator(t('approval-operation'))).toContainText('Read result');
  await expect(page.locator(t('kube-review-column'))).toHaveText([
    'Name',
    'Ready',
    'Status',
    'Restarts',
    'Age',
  ]);
  await expect(page.locator(t('kube-review-row'))).toHaveCount(3);
  await expect(page.locator(t('kube-review-row')).first()).toContainText('web-1');

  const row = page.locator(t('kube-review-row')).first();
  await row.click();
  await expect(row).toHaveAttribute('data-expanded', '');
  await row.hover();
  await row.locator(t('kube-review-expand')).first().click();
  await expect(page.locator(t('kube-review-cell'))).toBeVisible();
  await page.keyboard.press('Escape');

  await page.click(t('kube-review-view-json'));
  await expect(page.locator(t('kube-review-json'))).toContainText('"rowCount": 3');
  await page.click(t('kube-review-view-table'));
  await expect(page.locator(t('kube-review-row'))).toHaveCount(3);

  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: true, options: null });
});

test('contract: Withhold answers the item with a deny', async ({ relaunch }) => {
  const it = item('list');
  const { window: page, control } = await relaunch({ control: mocks('list') });
  await pushApprovals(page, [liveItem(it)]);
  await page.click(`[data-testid="approval-deny-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: false, options: null });
});

test('contract: describing a Secret shows its manifest and says the values are redacted', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: mocks('describeSecret') });
  await pushApprovals(page, [liveItem(item('describeSecret'))]);
  const manifest = page.locator(t('kube-review-manifest'));
  await expect(manifest).toContainText('kind: Secret');
  await expect(manifest).toContainText('<14 bytes>');
  await expect(page.locator(t('kube-review-mask'))).toHaveText(
    'Secret values are redacted by Kira.',
  );
  await page.click(t('kube-review-view-json'));
  await expect(page.locator(t('kube-review-json'))).toContainText('"kind": "Secret"');
  await expect(page.locator(t('kube-review-mask'))).toBeVisible();
});

test('contract: describing a pod shows the manifest and a JSON view, with no redaction note', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: mocks('describePod') });
  await pushApprovals(page, [liveItem(item('describePod'))]);
  await expect(page.locator(t('kube-review-manifest'))).toContainText('kind: Pod');
  await expect(page.locator(t('kube-review-mask'))).toHaveCount(0);
  await page.click(t('kube-review-view-json'));
  await expect(page.locator(t('kube-review-json'))).toContainText('"name": "web-1"');
  await expect(page.locator(t('kube-review-manifest'))).toHaveCount(0);
});

test('contract: held logs show as lines', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: mocks('logs') });
  await pushApprovals(page, [liveItem(item('logs'))]);
  await expect(page.locator(t('kube-review-log-line'))).toHaveText(['line two', 'line three']);
  await expect(page.locator(t('kube-review-summary'))).toHaveCount(0);
  await page.click(t('kube-review-view-json'));
  await expect(page.locator(t('kube-review-json'))).toContainText('"line three"');
});

test('truncated results say so, and a timestamped log gets its column', async ({ relaunch }) => {
  const text = JSON.stringify({
    lines: [
      { ts: '2026-01-02T03:04:05Z', text: 'line two' },
      { ts: '2026-01-02T03:04:06Z', text: 'line three' },
    ],
    truncated: true,
  });
  const { window: page } = await relaunch({ control: mocks('logs', text) });
  await pushApprovals(page, [liveItem(item('logs'))]);
  await expect(page.locator(t('kube-review-log-line')).first()).toContainText(
    '2026-01-02T03:04:05Z',
  );
  await expect(page.locator(t('kube-review-summary'))).toHaveText(
    'Older lines were cut off at the size limit',
  );
});

test('a result that does not parse falls back to its verbatim JSON view', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: mocks('list', '{"unexpected":true}') });
  await pushApprovals(page, [liveItem(item('list'))]);
  await expect(page.locator(t('kube-review-json'))).toContainText('"unexpected": true');
  await expect(page.locator(t('kube-review-view-table'))).toHaveCount(0);
});

test('contract: an ask cluster offers the review, and a tick rides the approval', async ({
  relaunch,
}) => {
  const it = item('readOption');
  const answer = contract<{ options: Record<string, unknown> }>(
    SC,
    'args:ApprovalsService.Answer#tick',
  );
  const [options] = Object.values(answer.options);
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk([it.id]) }],
  });
  await pushApprovals(page, [liveItem(it)]);
  const box = page.locator(t('kube-approval-review-result'));
  await expect(box).toHaveAttribute('data-state', 'unchecked');
  await box.click();
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: true, options: { [it.id]: options } });
});

test('contract: a cluster that already reviews shows the option checked and locked', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(item('readOptionOn'))]);
  const box = page.locator(t('kube-approval-review-result'));
  await expect(box).toHaveAttribute('data-state', 'checked');
  await expect(box).toBeDisabled();
});
