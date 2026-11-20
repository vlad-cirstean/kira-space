import { DATA_OP } from '@shared/protocol/dataOps';
import type { PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { OBJECT_PATH, openS3Browse, S3_ID, s3Control } from './support/s3Fixture';
import { openBrowseLeaf } from './support/tree';
import { wireOps } from './support/wireOps';

// J-KV-012 and decision 6, UI half. The Go half (TestS3EditKeepsMetadataAndRefusesChangedObject,
// TestS3EditWarnsWhenACLUnreadable) proves the saved object and that an unreadable ACL waits for a
// confirm. Here the object Save must send those ops: the opened ETag as the guard, then the
// $aclReset marker only after the user confirms the engine's warning.

const SCENARIO = 'studio-s3-edit';
const COLUMNS = ['_key', '$etag', '$aclReset', '$value'];
// The contract masks the ETag (a real one is random); the page's ETag stands in for it.
const ETAG = 'etag-opened';
const KEY_NAME = 'a.txt';
const withKey = (ops: unknown[]): unknown[] =>
  wireOps(
    (ops as { key: Record<string, string> }[]).map((o) => ({
      ...o,
      key: { ...o.key, _key: KEY_NAME, $etag: ETAG },
    })),
    COLUMNS,
  );
const opsOf = (key: string): unknown[] => withKey(contract<{ ops: unknown[] }>(SCENARIO, key).ops);

const GUARDED_OPS = opsOf('mutate:acl-guarded-ops');
const CONFIRMED_OPS = opsOf('mutate:acl-confirmed-ops');
const CONFLICT = contract<{ Code: string; Message: string; Details: string }>(
  SCENARIO,
  'mutate:acl-conflict',
);
const CHANGED_ERROR = { code: 'E_QUERY', message: 'The object was replaced after it was opened.' };

const READ_PAYLOAD = {
  connectionId: S3_ID,
  path: OBJECT_PATH,
  projection: null,
  filter: null,
  sort: null,
  pageSize: 100,
  cursor: { mode: 'offset', offset: 0 },
};
const readOf = (body: string): PortSnapshot => ({
  op: DATA_OP.read,
  payload: READ_PAYLOAD,
  response: {
    kind: 'read',
    page: {
      kind: 'keyvalue',
      redisType: 'object',
      ttlMs: null,
      memoryBytes: body.length,
      fields: ['ETag', 'Body'],
      values: [ETAG, body],
      position: {
        offset: 0,
        pageSize: 2,
        hasMore: false,
        nextToken: null,
        prevToken: null,
        strategy: 'offset',
      },
    },
    source: 'server',
  },
});
const mutateOf = (ops: unknown[], reply: Partial<PortSnapshot>): PortSnapshot =>
  ({
    op: DATA_OP.mutate,
    payload: { connectionId: S3_ID, path: OBJECT_PATH, ops },
    ...reply,
  }) as PortSnapshot;
const INVALIDATE: PortSnapshot = {
  op: DATA_OP.invalidate,
  payload: { connectionId: S3_ID, path: OBJECT_PATH },
  response: { kind: 'invalidate' },
};

async function stageEdit(page: import('@playwright/test').Page): Promise<void> {
  await openS3Browse(page);
  await openBrowseLeaf(page, OBJECT_PATH);
  const view = page.locator(`[data-testid="keyvalue-view"][data-path="${OBJECT_PATH}"]`);
  await expect(view.locator('[data-testid="keyvalue-row"]').first()).toBeVisible({
    timeout: 15_000,
  });
  await page.click('[data-testid="keyvalue-edit"]');
  await page.locator('[data-testid="cell-editor-panel"] .view-lines').click();
  await page.keyboard.press('ControlOrMeta+A');
  await page.keyboard.type('new');
  // Leaving the editor stages the draft.
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
  await expect(page.locator('[data-testid="keyvalue-object-edit-pending"]')).toBeVisible();
}

test('an object whose ACL cannot be read asks before the save resets it', async ({ relaunch }) => {
  const { window: page, stream } = await relaunch({
    control: s3Control,
    stream: [
      readOf('old'),
      readOf('old'),
      mutateOf(GUARDED_OPS, {
        error: { code: CONFLICT.Code, message: CONFLICT.Message, details: CONFLICT.Details },
      }),
      mutateOf(CONFIRMED_OPS, { response: { kind: 'mutate', affectedRows: 1 } }),
      INVALIDATE,
      readOf('new'),
    ],
  });
  await stageEdit(page);
  await page.click('[data-testid="keyvalue-object-edit-save"]');
  await expect(page.locator('[data-testid="confirm-dialog"]')).toContainText(CONFLICT.Message);

  // Cancel keeps the staged edit and sends nothing more.
  await page.click('[data-testid="confirm-dialog-close"]');
  await expect(page.locator('[data-testid="keyvalue-object-edit-pending"]')).toBeVisible();
  expect((await stream.ops()).filter((o) => o.op === DATA_OP.mutate)).toHaveLength(1);

  await page.click('[data-testid="keyvalue-object-edit-save"]');
  await page.click('[data-testid="confirm-dialog-confirm"]');
  await expect(page.locator('[data-testid="keyvalue-object-edit-pending"]')).toHaveCount(0);
  const sent = (await stream.ops()).filter((o) => o.op === DATA_OP.mutate);
  expect(sent.map((o) => (o.payload as { ops: unknown[] }).ops)).toEqual([
    GUARDED_OPS,
    GUARDED_OPS,
    CONFIRMED_OPS,
  ]);
});

test('an object replaced after it was opened refuses the save and keeps the edit', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: s3Control,
    // The browse preview and the opened tab each read the object once.
    stream: [readOf('old'), readOf('old'), mutateOf(GUARDED_OPS, { error: CHANGED_ERROR })],
  });
  await stageEdit(page);
  await page.click('[data-testid="keyvalue-object-edit-save"]');
  await expect(page.locator('[data-testid="keyvalue-object-edit-note"]')).toContainText(
    CHANGED_ERROR.message,
  );
  await expect(page.locator('[data-testid="confirm-dialog"]')).toHaveCount(0);
});
