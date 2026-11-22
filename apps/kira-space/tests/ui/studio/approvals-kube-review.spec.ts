import type { Page } from '@playwright/test';
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

// The list grid sizes its columns to content and fills the dialog width; a header handle resizes one
// column, a double click restores it, and expanding a row leaves the other columns alone.
async function columnWidths(page: Page): Promise<number[]> {
  return page
    .locator(t('kube-review-column'))
    .evaluateAll((ths) => ths.map((th) => th.getBoundingClientRect().width));
}

test('list columns fill the dialog, resize by handle and keep width when a row expands', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: mocks('list') });
  await pushApprovals(page, [liveItem(item('list'))]);
  const frame = page.locator(`${t('kube-review-table')} > div`).first();
  await expect(page.locator(t('kube-review-row'))).toHaveCount(3);
  const [tableWidth, frameWidth] = await frame.evaluate((el) => [
    el.querySelector('table')?.getBoundingClientRect().width ?? 0,
    el.clientWidth,
  ]);
  expect(tableWidth).toBeGreaterThanOrEqual(frameWidth - 1);

  const initial = await columnWidths(page);
  expect(initial.every((w) => w >= 64)).toBe(true);

  const handle = page.locator(t('kube-review-column-resize')).first();
  const box = await handle.boundingBox();
  if (!box) throw new Error('no resize handle');
  const x = box.x + box.width / 2;
  const y = box.y + box.height / 2;
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.mouse.move(x + 60, y, { steps: 4 });
  await page.mouse.up();
  await expect.poll(async () => (await columnWidths(page))[0]).toBeGreaterThan(initial[0] + 40);

  await handle.dblclick();
  await expect.poll(async () => (await columnWidths(page))[0]).toBeCloseTo(initial[0], 0);

  await page.locator(t('kube-review-row')).first().click();
  await expect(page.locator(t('kube-review-row')).first()).toHaveAttribute('data-expanded', '');
  expect(await columnWidths(page)).toEqual(initial);
});

test('a held log reads as terminal lines: dense, levelled, optional numbers, wrap toggle', async ({
  relaunch,
}) => {
  const ts = '2026-01-02T03:04:05Z';
  const text = JSON.stringify({
    lines: [
      { ts, text: 'INFO listening on :8080' },
      { ts, text: 'ERROR connection refused' },
      { ts, text: 'level=warn msg="slow upstream"' },
    ],
    truncated: false,
  });
  const { window: page } = await relaunch({ control: mocks('logs', text) });
  await pushApprovals(page, [liveItem(item('logs'))]);
  const lines = page.locator(t('kube-review-log-line'));
  await expect(lines).toHaveCount(3);

  const boxes = await Promise.all([0, 1, 2].map((i) => lines.nth(i).boundingBox()));
  for (const [i, box] of boxes.entries()) {
    expect(box?.height).toBeCloseTo(18, 0);
    if (i > 0) expect((box?.y ?? 0) - (boxes[i - 1]?.y ?? 0)).toBeCloseTo(18, 0);
  }
  await expect(lines.nth(1)).toHaveClass(/text-error/);
  await expect(lines.nth(2)).toHaveClass(/text-warn/);
  await expect(lines.nth(0)).toHaveClass(/text-fg/);
  await expect(lines.first().locator('span').first()).toHaveClass(/text-muted-foreground/);

  await expect(lines.first()).not.toHaveAttribute('data-n', /.*/);
  await page.click(t('kube-review-log-line-numbers'));
  await expect(lines.nth(1)).toHaveAttribute('data-n', '2');
  const gutter = await lines.nth(1).evaluate((el) => getComputedStyle(el, '::before').content);
  expect(gutter).toBe('"2"');

  await expect(page.locator(t('kube-review-log-wrap'))).toHaveAttribute('data-state', 'on');
  await expect(lines.first()).toHaveClass(/whitespace-pre-wrap/);
  await page.click(t('kube-review-log-wrap'));
  await expect(lines.first()).toHaveClass(/whitespace-pre(?!-)/);
  await page.click(t('kube-review-log-timestamps'));
  await expect(lines.first()).not.toContainText(ts);
});
