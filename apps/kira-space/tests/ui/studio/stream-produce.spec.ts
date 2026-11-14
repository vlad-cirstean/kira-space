import { DATA_OP } from '@shared/protocol/dataOps';
import {
  controlSnapshots as kafkaControl,
  portSnapshots as kafkaPorts,
} from '../../ipc/kafka/kafka.fixture';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';
import { wireOps } from './support/wireOps';

// J-STR-011, UI half. The Go half (TestKafkaProduceKeepsHeadersKeysAndBodies, TestKafkaProducePartialBatchAndResend)
// proves what the broker received. Here the composer must send the contract's insert and show the
// engine's refusal text. The composer needs a non-empty body, so the empty-body and absent-body
// cases are engine-only.

const TOPIC_PATH = 'topic:orders';
const COLUMNS = ['$key', '$body', '$headers'];
const OPS = contract<{ ops: unknown[] }>('studio-kafka-produce', 'mutate:ops').ops;
const SEND = wireOps(OPS.slice(0, 1), COLUMNS);
const SEND_NO_KEY = wireOps(OPS.slice(1, 2), COLUMNS);
const MALFORMED = contract<{ Code: string; Message: string }>(
  'studio-kafka-produce',
  'mutate:malformedHeadersError',
);
const values = (op: unknown) => (op as { values: Record<string, string | null> }).values;

const mutateOf = (ops: unknown[], reply: Partial<PortSnapshot>): PortSnapshot =>
  ({
    op: DATA_OP.mutate,
    payload: { connectionId: 'test-kafka', path: TOPIC_PATH, ops },
    ...reply,
  }) as PortSnapshot;

const INVALIDATE: PortSnapshot = {
  op: DATA_OP.invalidate,
  payload: { connectionId: 'test-kafka', path: TOPIC_PATH },
  response: { kind: 'invalidate' },
};
const READ = kafkaPorts.find(
  (s) => s.op === DATA_OP.read && (s.payload as { path: string }).path === TOPIC_PATH,
) as PortSnapshot;

async function openTopic(page: import('@playwright/test').Page) {
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  await (await findRow(page, TOPIC_PATH)).dblclick();
  const view = page.locator(`[data-testid="stream-view"][data-path="${TOPIC_PATH}"]`);
  await expect(view.locator('[data-testid="stream-row"]').first()).toBeVisible({
    timeout: 15_000,
  });
  await page.click('[data-testid="stream-add-message"]');
}

async function compose(
  page: import('@playwright/test').Page,
  fields: { key?: string; body: string; headers?: string },
) {
  if (fields.key) await page.fill('[data-testid="stream-add-message-key"]', fields.key);
  await page.fill('[data-testid="stream-add-message-body"]', fields.body);
  if (fields.headers) {
    await page.fill('[data-testid="stream-add-message-headers"]', fields.headers);
  }
}

test('composing a message with repeated headers sends the contract insert', async ({
  relaunch,
}) => {
  const { window: page, stream } = await relaunch({
    control: kafkaControl as ControlSnapshot[],
    stream: [
      mutateOf(SEND, { response: { kind: 'mutate', affectedRows: 1 } }),
      INVALIDATE,
      READ,
      mutateOf(SEND_NO_KEY, { response: { kind: 'mutate', affectedRows: 1 } }),
      INVALIDATE,
      READ,
      ...kafkaPorts,
    ],
  });
  await openTopic(page);
  const first = values(OPS[0]);
  await compose(page, {
    key: first.$key as string,
    body: first.$body as string,
    headers: first.$headers as string,
  });
  await page.click('[data-testid="stream-add-message-submit"]');
  await expect(page.locator('[data-testid="stream-add-message-panel"]')).toHaveCount(0);

  await page.click('[data-testid="stream-add-message"]');
  await compose(page, { body: values(OPS[1]).$body as string });
  await page.click('[data-testid="stream-add-message-submit"]');
  await expect(page.locator('[data-testid="stream-add-message-panel"]')).toHaveCount(0);

  const sent = (await stream.ops()).filter((o) => o.op === DATA_OP.mutate);
  expect(sent.map((o) => (o.payload as { ops: unknown[] }).ops)).toEqual([SEND, SEND_NO_KEY]);
});

test('a malformed headers object is refused with the engine text and nothing is sent twice', async ({
  relaunch,
}) => {
  const bad = '{"a":';
  const { window: page, stream } = await relaunch({
    control: kafkaControl as ControlSnapshot[],
    stream: [
      mutateOf(
        wireOps([{ kind: 'insert', values: { $key: null, $body: 'b', $headers: bad } }], COLUMNS),
        { error: { code: MALFORMED.Code, message: MALFORMED.Message } },
      ),
      ...kafkaPorts,
    ],
  });
  await openTopic(page);
  await compose(page, { body: 'b', headers: bad });
  await page.click('[data-testid="stream-add-message-submit"]');
  await expect(page.locator('[data-testid="stream-add-message-error"]')).toContainText(
    MALFORMED.Message,
  );
  // The panel stays open with the typed message for a resend.
  await expect(page.locator('[data-testid="stream-add-message-body"]')).toHaveValue('b');
  expect((await stream.ops()).filter((o) => o.op === DATA_OP.mutate)).toHaveLength(1);
});
