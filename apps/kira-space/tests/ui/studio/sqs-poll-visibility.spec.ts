import { DATA_OP } from '@shared/protocol/dataOps';
import { expect, test } from './fixtures';
import { acceptConfirm } from './support/dialogs';
import { readOnlySqsSnapshots, SQS_QUEUE_PATH, SQS_REDRIVE_LIMIT } from './support/sqsFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';

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
