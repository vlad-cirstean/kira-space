import { CHANNEL } from '@shared/protocol/events';
import { answeredOk, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from './fixtures';
import { apiRequestItem, dbQueryItem } from './support/approvalItems';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// The dialog shows one approval at a time with a fixed footer; only a long body scrolls.

const t = (id: string) => `[data-testid="${id}"]`;
const query = dbQueryItem('db.query:1', 3);
// Newest last: the dialog lists by creation time.
const request = { ...apiRequestItem('api.request:2', 60), createdAt: query.createdAt + 1 };

test.use({ viewport: { width: 1400, height: 900 } });

test('one item shows at a time, the footer stays put and a long body scrolls', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk([query.id]) }],
  });
  await pushApprovals(page, [query, request]);
  const dialog = page.locator(t('approvals-dialog'));
  await expect(dialog.locator('[data-testid^="approval-item-"]')).toHaveCount(1);
  await expect(dialog.locator(t('approvals-position'))).toHaveText('1 of 2');
  await expect(dialog.locator(t(`approval-item-${query.id}`))).toBeVisible();

  const approve = dialog.locator('[data-testid^="approval-approve-"]');
  const bodyOf = () =>
    dialog.locator('[data-slot="dialog-body"]').evaluate((e) => e.scrollHeight > e.clientHeight);
  await expect.poll(bodyOf).toBe(false);
  const first = await approve.boundingBox();

  await dialog.locator(t(`approval-approve-${query.id}`)).click();
  await expect.poll(() => calls(control, IPC.approvalsAnswer)).toHaveLength(1);
  await pushApprovals(page, [request]);
  await expect(dialog.locator(t(`approval-item-${request.id}`))).toBeVisible();
  await expect(dialog.locator(t('approvals-position'))).toHaveCount(0);
  await expect.poll(bodyOf).toBe(true);
  const lines = dialog.locator(t('api-approval-body-text')).locator('.view-line');
  await expect.poll(() => lines.count()).toBeGreaterThan(12);
  const second = await approve.boundingBox();
  // Labels differ per kind, so the right edge is the stable anchor.
  expect((second?.x ?? 0) + (second?.width ?? 0)).toBeCloseTo(
    (first?.x ?? -1) + (first?.width ?? 0),
    0,
  );
  expect(second?.y).toBeCloseTo(first?.y ?? -1, 0);
});

test('a reveal of the second item shows it first', async ({ relaunch }) => {
  const { window: page } = await relaunch({});
  await pushApprovals(page, [query, request]);
  const dialog = page.locator(t('approvals-dialog'));
  await expect(dialog.locator(t(`approval-item-${query.id}`))).toBeVisible();
  await emitWailsEvent(page, CHANNEL.promptsReveal, { id: `approval:${request.id}` });
  await expect(dialog.locator(t(`approval-item-${request.id}`))).toBeVisible();
  await expect(dialog.locator(t('approvals-position'))).toHaveText('2 of 2');
});
