import { DATA_OP } from '@shared/protocol/dataOps';
import {
  controlSnapshots as sqsControl,
  portSnapshots as sqsPorts,
} from '../../ipc/sqs/sqs.fixture';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { acceptConfirm } from './support/dialogs';
import { readOnlySqsSnapshots, SQS_QUEUE_PATH, SQS_REDRIVE_LIMIT } from './support/sqsFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';
import { wireOps } from './support/wireOps';

// P184: a read-only SQS tab sets its poll hide time in the toolbar; the value rides the read
// request's filter and the confirm names it.
test('read-only sqs poll hide time: validated field, confirm text, filter on the wire', async ({
  relaunch,
}) => {
  const { control, stream: port } = readOnlySqsSnapshots([30, 60]);
  const { window: page, stream } = await relaunch({ control, stream: port });

  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  await (await findRow(page, SQS_QUEUE_PATH)).dblclick();

  const view = page.locator(`[data-testid="stream-view"][data-path="${SQS_QUEUE_PATH}"]`);
  const field = view.locator('[data-testid="stream-poll-visibility"]');
  const error = view.locator('[data-testid="stream-poll-visibility-error"]');
  const poll = view.locator('[data-testid="stream-poll"]');
  const confirmMessage = page.locator('[data-testid="confirm-dialog-message"]');

  await expect(field).toHaveValue('1');
  await expect(view.locator('[data-testid="stream-poll-warning"]')).toContainText(
    'for 1 second and raises their receive count',
  );
  await expect(view.locator('[data-testid="stream-poll-warning"]')).toContainText(
    'A short hide can end a poll early',
  );

  for (const bad of ['0', '43201']) {
    await field.fill(bad);
    await expect(error).toBeVisible();
    await expect(poll).toBeDisabled();
  }

  await field.fill('30');
  await field.blur();
  await expect(error).toHaveCount(0);
  await expect(poll).toBeEnabled();

  await poll.click();
  await expect(confirmMessage).toContainText('for 30 seconds');
  await expect(confirmMessage).toContainText(`after ${SQS_REDRIVE_LIMIT} receives`);
  await acceptConfirm(page);
  await expect(view.locator('[data-testid="stream-row"]').first()).toBeVisible({ timeout: 15_000 });

  const reads = (await stream.ops()).filter((o) => o.op === DATA_OP.read);
  expect(reads).toHaveLength(1);
  expect((reads[0].payload as { filter: string }).filter).toBe('{"visibilityTimeoutSeconds":30}');

  await field.fill('60');
  await field.blur();
  await poll.click();
  await expect(confirmMessage).toContainText('for 1 minute');
  await acceptConfirm(page);
});

// J-STR-012 and decision 7, UI half. The Go half (TestSqsPollCountsReceivesAndRefusesStaleDeletes,
// TestSqsDuplicateAttributeNameRefusesCompose) proves the receive counts, the stale-receipt refusals
// and that a repeated attribute name lands nothing. Here: only Poll reads, and the engine's refusal
// texts reach the screen.
const SQS_ID = 'test-sqs';
const POLL = sqsPorts.find((p) => p.op === DATA_OP.read) as PortSnapshot;
const INVALIDATE: PortSnapshot = {
  op: DATA_OP.invalidate,
  payload: { connectionId: SQS_ID, path: SQS_QUEUE_PATH },
  response: { kind: 'invalidate' },
};
const mutateOf = (ops: unknown[], reply: Partial<PortSnapshot>): PortSnapshot =>
  ({
    op: DATA_OP.mutate,
    payload: { connectionId: SQS_ID, path: SQS_QUEUE_PATH, ops },
    ...reply,
  }) as PortSnapshot;

async function openQueue(page: import('@playwright/test').Page) {
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  await (await findRow(page, SQS_QUEUE_PATH)).dblclick();
  return page.locator(`[data-testid="stream-view"][data-path="${SQS_QUEUE_PATH}"]`);
}

test('opening the tab and moving window focus never polls; only Poll reads', async ({
  relaunch,
}) => {
  const { window: page, stream } = await relaunch({
    control: sqsControl as ControlSnapshot[],
    stream: [INVALIDATE, POLL, ...sqsPorts],
  });
  const view = await openQueue(page);
  await expect(view.locator('[data-testid="stream-poll"]')).toBeEnabled();
  await page.evaluate(() => {
    window.dispatchEvent(new Event('blur'));
    window.dispatchEvent(new Event('focus'));
    document.dispatchEvent(new Event('visibilitychange'));
  });
  await page.bringToFront();
  const reads = async () => (await stream.ops()).filter((o) => o.op === DATA_OP.read).length;
  expect(await reads()).toBe(0);

  await view.locator('[data-testid="stream-poll"]').click();
  await expect(view.locator('[data-testid="stream-row"]').first()).toBeVisible({ timeout: 15_000 });
  expect(await reads()).toBe(1);
  await page.evaluate(() => {
    window.dispatchEvent(new Event('blur'));
    window.dispatchEvent(new Event('focus'));
  });
  expect(await reads()).toBe(1);
});

test('deleting a message whose receipt lapsed shows the engine refusal and keeps the row', async ({
  relaunch,
}) => {
  const EXPIRED = contract<{ Code: string; Message: string }>(
    'studio-sqs',
    'mutate:receiptExpiredError',
  );
  const firstId = (POLL.response as { page: { keys: string[] } }).page.keys[0];
  const { window: page } = await relaunch({
    control: sqsControl as ControlSnapshot[],
    stream: [
      INVALIDATE,
      POLL,
      mutateOf([{ kind: 'delete', key: { messageId: firstId } }], {
        error: { code: EXPIRED.Code, message: EXPIRED.Message },
      }),
      ...sqsPorts,
    ],
  });
  const view = await openQueue(page);
  await view.locator('[data-testid="stream-poll"]').click();
  const rows = view.locator('[data-testid="stream-row"]');
  await expect(rows.first()).toBeVisible({ timeout: 15_000 });
  await rows.first().click();
  await page.click('[data-testid="stream-delete-message"]');
  await acceptConfirm(page);
  await expect(page.locator('[data-testid="stream-action-error"]')).toContainText(EXPIRED.Message);
  await expect(rows).toHaveCount(5);
});

test('a repeated attribute name is refused with the engine text and nothing is sent twice', async ({
  relaunch,
}) => {
  const OPS = contract<{ ops: unknown[] }>('studio-sqs', 'mutate:duplicateAttributeOps').ops;
  const REFUSAL = contract<{ Code: string; Message: string }>(
    'studio-sqs',
    'mutate:duplicateAttributeError',
  );
  const values = (OPS[0] as { values: Record<string, string> }).values;
  const { window: page, stream } = await relaunch({
    control: sqsControl as ControlSnapshot[],
    stream: [
      mutateOf(wireOps(OPS, ['$body', '$headers']), {
        error: { code: REFUSAL.Code, message: REFUSAL.Message },
      }),
      ...sqsPorts,
    ],
  });
  const view = await openQueue(page);
  await view.locator('[data-testid="stream-poll"]').click();
  await expect(view.locator('[data-testid="stream-row"]').first()).toBeVisible({ timeout: 15_000 });
  await page.click('[data-testid="stream-add-message"]');
  await page.fill('[data-testid="stream-add-message-body"]', values.$body);
  await page.fill('[data-testid="stream-add-message-headers"]', values.$headers);
  await page.click('[data-testid="stream-add-message-submit"]');
  await expect(page.locator('[data-testid="stream-add-message-error"]')).toContainText(
    REFUSAL.Message,
  );
  await expect(page.locator('[data-testid="stream-add-message-headers"]')).toHaveValue(
    values.$headers,
  );
  expect((await stream.ops()).filter((o) => o.op === DATA_OP.mutate)).toHaveLength(1);
});
