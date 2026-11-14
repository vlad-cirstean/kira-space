import { DATA_OP } from '@shared/protocol/dataOps';
import { controlSnapshots as redisControl } from '../../ipc/redis/redis.fixture';
import type { PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { openRedisBrowse, REDIS_DB0 } from './support/redisFixture';
import { openBrowseLeaf } from './support/tree';
import { wireOps } from './support/wireOps';

// J-KV-010, UI half. The Go half (TestRedisEditKeepsTTLAndNeverResurrects, TestRedisKeyTypesReadAndRefuseEdit)
// proves the saved bytes, the kept TTL, the refusal of non-string keys and that an expired key is not
// recreated. Here the popover must send the same ops and show the engine's messages.

const SCENARIO = 'studio-redis-edit';
const CONNECTION_ID = 'test-redis';
const KEY_NAME = 'session:abc';
const NS = `${REDIS_DB0}/namespace:session`;
const KEY_PATH = `${NS}/key:session%3Aabc`;
const UI_OPS = wireOps(
  (contract<{ ops: { key: object }[] }>(SCENARIO, 'mutate:ui-ops').ops ?? []).map((o) => ({
    ...o,
    key: { _key: KEY_NAME },
  })),
  ['_key', '$value'],
);
const UI_VALUE = (UI_OPS[0] as { changes: { $value: string } }).changes.$value;
const EXPIRED = contract<{ Code: string; Message: string }>(SCENARIO, 'mutate:expiredError');

const readOf = (redisType: 'string' | 'hash', values: string[]): PortSnapshot => ({
  op: DATA_OP.read,
  payload: {
    connectionId: CONNECTION_ID,
    path: KEY_PATH,
    projection: null,
    filter: null,
    sort: null,
    pageSize: 100,
    cursor: { mode: 'offset', offset: 0 },
  },
  response: {
    kind: 'read',
    page: {
      kind: 'keyvalue',
      redisType,
      ttlMs: 9_999_000,
      memoryBytes: 64,
      fields: redisType === 'string' ? ['value'] : ['f'],
      values,
      position: {
        offset: 0,
        pageSize: 1,
        hasMore: false,
        nextToken: null,
        prevToken: null,
        strategy: 'offset',
      },
    },
    source: 'server',
  },
});
const mutateOf = (reply: Partial<PortSnapshot>): PortSnapshot =>
  ({
    op: DATA_OP.mutate,
    payload: { connectionId: CONNECTION_ID, path: KEY_PATH, ops: UI_OPS },
    ...reply,
  }) as PortSnapshot;
const INVALIDATE: PortSnapshot = {
  op: DATA_OP.invalidate,
  payload: { connectionId: CONNECTION_ID, path: KEY_PATH },
  response: { kind: 'invalidate' },
};

async function openKey(page: import('@playwright/test').Page): Promise<void> {
  await openRedisBrowse(page);
  const browse = page.locator('[data-testid="browse-view"]');
  await browse.locator(`[data-testid="browse-row"][data-path="${NS}"]`).dblclick();
  await openBrowseLeaf(page, KEY_PATH);
  await expect(
    page
      .locator(`[data-testid="keyvalue-view"][data-path="${KEY_PATH}"]`)
      .locator('[data-testid="keyvalue-row"]')
      .first(),
  ).toBeVisible({ timeout: 15_000 });
}

test('editing a string sends the contract ops and reloads the key', async ({ relaunch }) => {
  const { window: page, stream } = await relaunch({
    control: redisControl,
    // The browse preview and the opened tab each read the key once.
    stream: [
      readOf('string', ['token-abc']),
      readOf('string', ['token-abc']),
      mutateOf({ response: { kind: 'mutate', affectedRows: 1 } }),
      INVALIDATE,
      readOf('string', [UI_VALUE]),
    ],
  });
  await openKey(page);
  await page.click('[data-testid="keyvalue-edit"]');
  await page.fill('[data-testid="keyvalue-edit-input"]', UI_VALUE);
  await page.click('[data-testid="keyvalue-edit-save"]');
  await expect(page.locator('[data-testid="keyvalue-edit-popover"]')).toHaveCount(0);
  const sent = (await stream.ops()).filter((o) => o.op === DATA_OP.mutate);
  expect(sent.map((o) => (o.payload as { ops: unknown[] }).ops)).toEqual([UI_OPS]);
  await expect(page.locator('[data-testid="keyvalue-row"]').first()).toContainText('ünï');
});

test('saving a key that expired shows the engine failure and keeps the draft', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: redisControl,
    stream: [
      readOf('string', ['token-abc']),
      readOf('string', ['token-abc']),
      mutateOf({ error: { code: EXPIRED.Code, message: EXPIRED.Message } }),
    ],
  });
  await openKey(page);
  await page.click('[data-testid="keyvalue-edit"]');
  await page.fill('[data-testid="keyvalue-edit-input"]', UI_VALUE);
  await page.click('[data-testid="keyvalue-edit-save"]');
  await expect(page.locator('[data-testid="keyvalue-edit-error"]')).toContainText(EXPIRED.Message);
  await expect(page.locator('[data-testid="keyvalue-edit-input"]')).toHaveValue(UI_VALUE);
});

test('a key of another type has its edit disabled and sends nothing', async ({ relaunch }) => {
  const { window: page, stream } = await relaunch({
    control: redisControl,
    stream: [readOf('hash', ['1']), readOf('hash', ['1'])],
  });
  await openKey(page);
  await expect(page.locator('[data-testid="keyvalue-edit"]')).toBeDisabled();
  expect((await stream.ops()).filter((o) => o.op === DATA_OP.mutate)).toHaveLength(0);
});
