import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { editorText } from './support/editorText';

// Contract approvals-api-script. Backend half: dbmcpflow TestApiRequestScripted, the same scenario.
// A request with a prepare and a transform script parks with both sources read-only and highlighted,
// the prepared body, what the script set, and its input.

interface ScriptPayload {
  script: {
    prepare: { source: string; changed: string[]; input: string };
    transform: { source: string };
  };
  body: { text: string };
}

const item = () => {
  const found = contract<ApprovalItem[]>('approvals-api-script', 'ApprovalsService.List')[0];
  if (!found) throw new Error('contract has no item');
  return found;
};

test('contract: both scripts show highlighted next to the prepared request', async ({
  relaunch,
}) => {
  const it = item();
  const payload = it.payload as unknown as ScriptPayload;
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  const card = page.locator(`[data-testid="approval-item-${it.id}"]`);

  await expect(
    card.locator('[data-testid="approval-fact"]').filter({ hasText: 'Runs 2 scripts' }),
  ).toBeVisible();
  const prepare = card.locator('[data-testid="api-approval-script-prepare"]');
  const transform = card.locator('[data-testid="api-approval-script-transform"]');
  await expect.poll(() => editorText(prepare)).toBe(payload.script.prepare.source);
  await expect.poll(() => editorText(transform)).toBe(payload.script.transform.source);

  // Monaco's javascript tokenizer colours a keyword apart from an identifier.
  await expect
    .poll(async () =>
      transform
        .locator('.view-line span[class*="mtk"]')
        .evaluateAll((els) => new Set(els.map((e) => e.className)).size),
    )
    .toBeGreaterThanOrEqual(2);

  await expect(card.locator('[data-testid="api-approval-body-text"]')).toBeVisible();
  await expect
    .poll(() => editorText(card.locator('[data-testid="api-approval-body-text"]')))
    .toContain('"items"');
  await card.locator('[data-testid="approval-details-toggle"]').click();
  await expect(card.locator('[data-testid="api-approval-script-changed"]')).toContainText(
    'body, contentType',
  );
  await expect(card.locator('[data-testid="api-approval-script-input"]')).toContainText(
    payload.script.prepare.input,
  );
});

test('contract: Send answers the item id', async ({ relaunch }) => {
  const it = item();
  const answer = contract<{ ids: string[] }>(
    'approvals-api-script',
    'args:ApprovalsService.Answer',
  );
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(answer.ids) }],
  });
  await pushApprovals(page, [liveItem(it)]);
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: true, options: null });
});
