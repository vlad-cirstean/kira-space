import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';

// Contract approvals-api-response. Backend half: dbmcpflow TestApiResponseReview and
// TestApiResponseReviewScripted. A held api_request response parks as an api.response item; its body
// comes from the pending-response query, never the payload. A script adds the raw response and the
// script as views beside the result.

interface Held {
  requestId: string;
  text: string;
  raw: string;
  isError: boolean;
  script: { source: string; hash: string } | null;
}

const SC = 'approvals-api-response';
const item = (key: string) => contract<ApprovalItem[]>(SC, `ApprovalsService.List#${key}`)[0];
const held = (key: string) => contract<Held>(SC, `DbMcpService.PendingResponse#${key}`);

const mocks = (key: string) => [
  { channel: IPC.dbMcpPendingResponse, response: held(key) },
  {
    channel: IPC.approvalsAnswer,
    response: answeredOk(
      contract<{ ids: string[] }>(SC, `args:ApprovalsService.Answer#${key}`).ids,
    ),
  },
];

test('contract: an unscripted response shows its raw view and Send answers by id', async ({
  relaunch,
}) => {
  const it = item('plain');
  const { window: page, control } = await relaunch({ control: mocks('plain') });
  await pushApprovals(page, [liveItem(it)]);
  const body = page.locator('[data-testid="api-review-body"]');
  await expect(body.locator('[data-testid="api-review-raw-status"]')).toContainText('200');
  await expect(body.locator('[data-testid="api-review-raw-headers"]')).toBeVisible();
  await expect(body.locator('[data-testid="api-review-raw-body"]')).toContainText('a@x.test');
  // An integer above 2^53 keeps its digits and the body is indented (not the compact original).
  const rawBody = body.locator('[data-testid="api-review-raw-body"]');
  await expect(rawBody).toContainText('"id": 9007199254740993');
  await expect(rawBody).not.toContainText('"total":2');
  await expect(body.locator('[data-testid="api-review-raw-status"]')).toContainText('200 OK');
  await expect(body.locator('[data-testid="api-review-raw-status"]')).toHaveClass(/text-ok/);
  await expect(page.locator('[data-testid="api-review-view-raw"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="api-review-script"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="api-review-sent"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="api-review-request"]')).toContainText('GET');
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: true, options: null });
});

test('contract: a scripted response shows the result, the raw response and the script', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: mocks('scripted') });
  await pushApprovals(page, [liveItem(item('scripted'))]);
  const source = held('scripted').script?.source ?? '';
  const result = page.locator('[data-testid="api-review-result"]');
  // A big integer would change on re-indent, so the text shows verbatim.
  await expect(result).toContainText('{"id":9007199254740993,"domain":"x.test"}');
  await expect(page.locator('[data-testid="api-review-script"]')).toContainText(
    source.split('\n')[0] ?? '',
  );
  await page.click('[data-testid="api-review-view-raw"]');
  await expect(page.locator('[data-testid="api-review-raw-status"]')).toContainText('200');
  await expect(page.locator('[data-testid="api-review-raw-body"]')).toContainText('a@x.test');
  await page.click('[data-testid="api-review-view-result"]');
  await expect(result).toContainText('9007199254740993');
});

test('contract: a write response says the request was already sent', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: mocks('scriptedWrite') });
  await pushApprovals(page, [liveItem(item('scriptedWrite'))]);
  await expect(page.locator('[data-testid="api-review-sent"]')).toBeVisible();
});

test('contract: a failed script shows the failure beside the raw response', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: mocks('scriptFailed') });
  await pushApprovals(page, [liveItem(item('scriptFailed'))]);
  await expect(page.locator('[data-testid="api-review-script-failed"]')).toBeVisible();
  await expect(page.locator('[data-testid="api-review-result"]')).toContainText(
    'transform script failed',
  );
  await page.click('[data-testid="api-review-view-raw"]');
  await expect(page.locator('[data-testid="api-review-raw-body"]')).toContainText('a@x.test');
});

test('a binary or cut body reads as the HTTP pane words it, sizes through formatBytes', async ({
  relaunch,
}) => {
  const it = item('plain');
  const base = held('plain');
  const response = JSON.parse(base.text) as Record<string, unknown>;
  const text = JSON.stringify({
    ...response,
    status: 404,
    statusText: 'Not Found',
    body: '',
    binary: true,
    truncated: true,
    bodyBytes: 12288,
  });
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.dbMcpPendingResponse, response: { ...base, text } },
      ...mocks('plain').slice(1),
    ],
  });
  await pushApprovals(page, [liveItem(it)]);
  const body = page.locator('[data-testid="api-review-body"]');
  await expect(body.locator('[data-testid="api-review-raw-body"]')).toContainText(
    'Binary body, 12.0 KB not shown',
  );
  await expect(body).toContainText('Response truncated at 12.0 KB.');
  await expect(body.locator('[data-testid="api-review-raw-status"]')).toContainText(
    '404 Not Found',
  );
  // statusClass tone: 4xx is error red, never the warn amber of a redirect.
  await expect(body.locator('[data-testid="api-review-raw-status"]')).toHaveClass(/text-error/);
});
