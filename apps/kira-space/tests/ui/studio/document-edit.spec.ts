import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { installClipboardSpy, lastClipboardWrite } from './support/clipboard';
import { createConnection } from './support/connect';
import { editorText } from './support/editorText';
import {
  DB_PATH as MONGO_DB_PATH,
  mongoConnectionSummary,
  WIDGETS_PATH,
  widgetsFixture,
} from './support/mongoFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';
import { wireOps } from './support/wireOps';

// J-DOC-010 and decisions 3 and 4, UI half. The Go half (TestDocumentEditKeepsTypesAndGuards) saves a
// document holding ObjectId, Int64, Decimal128, a pre-1970 date, Binary subtype 4 and nested arrays
// through the same shell text, and proves the saved BSON with a second client. Here the editor must
// load that text, send those ops, ask before overwriting a changed document and show the refusal of
// an edited _id.

const CONNECTION_ID = 'conn-doc-edit';
const SCENARIO = 'studio-document';
const COLUMNS = ['_id', '$document'];
// The contract keeps the ISODate's time without its T so the timestamp mask leaves it alone.
const unmask = (v: unknown): unknown =>
  JSON.parse(JSON.stringify(v).replaceAll('1960-05-04 03:02:01.000Z', '1960-05-04T03:02:01.000Z'));
const opsOf = (key: string): unknown[] =>
  wireOps(unmask(contract<{ ops: unknown[] }>(SCENARIO, key).ops) as unknown[], COLUMNS);
const bodyOf = (op: unknown, field: 'changes' | 'expect'): string =>
  (op as Record<string, Record<string, string>>)[field].$document;

const ROW = contract<{ id: string; body: string }>(SCENARIO, 'read:row');
const SAVE_OPS = opsOf('mutate:ops');
const GUARDED_OPS = opsOf('mutate:guarded-ops');
const OVERWRITE_OPS = opsOf('mutate:overwrite-ops');
const ID_EDIT_OPS = opsOf('mutate:idEdit-ops');
const CONFLICT = contract<{ Code: string; Message: string; Details: string }>(
  SCENARIO,
  'mutate:conflict',
);
const ID_EDIT_ERROR = contract<{ Code: string; Message: string }>(SCENARIO, 'mutate:idEditError');

const PROJECTED = contract<{ rows: [string, string][] }>(SCENARIO, 'read:projected').rows;

const FIXTURE = widgetsFixture(CONNECTION_ID);
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: {
      name: 'Mongo Edit',
      kind: 'mongodb',
      color: 'green',
      mode: 'fields',
      readOnly: false,
      host: '127.0.0.1',
      port: 27017,
      database: 'kira_test',
      username: 'kira',
      password: null,
      uri: null,
      options: {},
      preconnect: null,
      preconnectSidecar: false,
      preconnectDelaySec: 0,
      autoExplain: false,
      throttlePerSec: 0,
    },
    response: mongoConnectionSummary(CONNECTION_ID, 'Mongo Edit', 'green'),
  },
  ...FIXTURE.control,
];

const READ_PAYLOAD = {
  connectionId: CONNECTION_ID,
  path: WIDGETS_PATH,
  projection: null,
  filter: null,
  sort: null,
  pageSize: 100,
  cursor: { mode: 'offset', offset: 0 },
};
const POSITION = {
  offset: 0,
  pageSize: 100,
  hasMore: false,
  nextToken: null,
  prevToken: null,
  strategy: 'keyset' as const,
};
const readOf = (body: string): PortSnapshot => ({
  op: DATA_OP.read,
  payload: READ_PAYLOAD,
  response: {
    kind: 'read',
    page: { kind: 'document', ids: [ROW.id], bodies: [body], position: POSITION },
    source: 'server',
  },
});
const mutateOf = (ops: unknown[], reply: Partial<PortSnapshot>): PortSnapshot =>
  ({
    op: DATA_OP.mutate,
    payload: { connectionId: CONNECTION_ID, path: WIDGETS_PATH, ops },
    ...reply,
  }) as PortSnapshot;
const INVALIDATE: PortSnapshot = {
  op: DATA_OP.invalidate,
  payload: { connectionId: CONNECTION_ID, path: WIDGETS_PATH },
  response: { kind: 'invalidate' },
};

async function openEditor(page: import('@playwright/test').Page): Promise<void> {
  await createConnection(page, 'mongodb', 'Mongo Edit');
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  await expandRow(page, MONGO_DB_PATH);
  await (await findRow(page, WIDGETS_PATH)).dblclick();
  await expect(page.locator('[data-testid="document-row"]').first()).toBeVisible({
    timeout: 15_000,
  });
  await page.locator('[data-testid="document-edit"]').first().click();
  await expect(page.locator('[data-testid="document-edit-save"]')).toBeEnabled();
}

// Monaco re-indents pasted multi-line text, so the edit is made in place: select the word, type over it.
async function replaceWord(
  page: import('@playwright/test').Page,
  from: string,
  to: string,
): Promise<void> {
  const point = await page
    .locator('[data-testid="document-body"] .view-lines')
    .evaluate((el, word) => {
      const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
      for (let node = walker.nextNode(); node; node = walker.nextNode()) {
        const idx = (node.textContent ?? '').indexOf(word);
        if (idx >= 0) {
          const range = document.createRange();
          range.setStart(node, idx);
          range.setEnd(node, idx + word.length);
          const rect = range.getBoundingClientRect();
          return { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 };
        }
      }
      return null;
    }, from);
  if (!point) throw new Error(`"${from}" not found in the editor`);
  await page.mouse.dblclick(point.x, point.y);
  await page.keyboard.type(to);
}

async function expectEditorText(
  page: import('@playwright/test').Page,
  text: string,
): Promise<void> {
  await expect
    .poll(async () => editorText(page.locator('[data-testid="document-body"]')))
    .toBe(text);
}

test('editing a rich document sends the contract ops with the loaded body as the guard', async ({
  relaunch,
}) => {
  const edited = bodyOf(SAVE_OPS[0], 'changes');
  const { window: page, stream } = await relaunch({
    control: CONTROL,
    stream: [
      readOf(ROW.body),
      mutateOf(SAVE_OPS, { response: { kind: 'mutate', affectedRows: 1 } }),
      INVALIDATE,
      readOf(ROW.body.replace('"before"', '"after"')),
    ],
  });
  await openEditor(page);
  // The editor opens on the shell text the Go half also saved: the loaded body is the guard.
  expect(await editorText(page.locator('[data-testid="document-body"]'))).toBe(
    bodyOf(SAVE_OPS[0], 'expect'),
  );
  await replaceWord(page, 'before', 'after');
  await expectEditorText(page, edited);
  await page.click('[data-testid="document-edit-save"]');
  await expect(page.locator('[data-testid="document-edit-save"]')).toHaveCount(0);
  const sent = (await stream.ops()).filter((o) => o.op === DATA_OP.mutate);
  expect(sent).toHaveLength(1);
  expect((sent[0].payload as { ops: unknown[] }).ops).toEqual(SAVE_OPS);
});

test('a document changed on the server asks before it is overwritten', async ({ relaunch }) => {
  const mine = bodyOf(GUARDED_OPS[0], 'changes');
  const { window: page, stream } = await relaunch({
    control: CONTROL,
    stream: [
      readOf(ROW.body),
      mutateOf(GUARDED_OPS, {
        error: { code: CONFLICT.Code, message: CONFLICT.Message, details: CONFLICT.Details },
      }),
      mutateOf(OVERWRITE_OPS, { response: { kind: 'mutate', affectedRows: 1 } }),
      INVALIDATE,
      readOf(ROW.body.replace('"before"', '"mine"')),
    ],
  });
  await openEditor(page);
  await replaceWord(page, 'before', 'mine');
  await expectEditorText(page, mine);
  await page.click('[data-testid="document-edit-save"]');
  const dialog = page.locator('[data-testid="confirm-dialog"]');
  await expect(dialog).toContainText(CONFLICT.Message);

  // Cancel keeps the editor open with the typed text and sends nothing more.
  await page.click('[data-testid="confirm-dialog-cancel"]');
  await expect(page.locator('[data-testid="document-edit-save"]')).toBeEnabled();
  expect(await editorText(page.locator('[data-testid="document-body"]'))).toBe(mine);
  expect((await stream.ops()).filter((o) => o.op === DATA_OP.mutate)).toHaveLength(1);

  await page.click('[data-testid="document-edit-save"]');
  await page.click('[data-testid="confirm-dialog-confirm"]');
  await expect(page.locator('[data-testid="document-edit-save"]')).toHaveCount(0);
  const sent = (await stream.ops()).filter((o) => o.op === DATA_OP.mutate);
  expect(sent.map((o) => (o.payload as { ops: unknown[] }).ops)).toEqual([
    GUARDED_OPS,
    GUARDED_OPS,
    OVERWRITE_OPS,
  ]);
});

test('an edited _id is refused with the engine message and the editor stays open', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: CONTROL,
    stream: [
      readOf(ROW.body),
      mutateOf(ID_EDIT_OPS, {
        error: { code: ID_EDIT_ERROR.Code, message: ID_EDIT_ERROR.Message },
      }),
    ],
  });
  await openEditor(page);
  await replaceWord(page, 'before', 'after');
  await replaceWord(page, '65f000000000000000000001', '000000000000000000000001');
  await expectEditorText(page, bodyOf(ID_EDIT_OPS[0], 'changes'));
  await page.click('[data-testid="document-edit-save"]');
  await expect(page.locator('[data-testid="document-action-error"]')).toContainText(
    ID_EDIT_ERROR.Message,
  );
  await expect(page.locator('[data-testid="document-edit-save"]')).toBeEnabled();
});

test('copying a document from a projected read copies only the projected fields', async ({
  relaunch,
}) => {
  // OQ-DOC-3: the page holds what the server returned for the projection, and Copy reads the page.
  const { window: page } = await relaunch({
    control: CONTROL,
    stream: [
      {
        op: DATA_OP.read,
        payload: READ_PAYLOAD,
        response: {
          kind: 'read',
          page: {
            kind: 'document',
            ids: PROJECTED.map((r) => r[0]),
            bodies: PROJECTED.map((r) => r[1]),
            position: POSITION,
          },
          source: 'server',
        },
      },
    ],
  });
  await createConnection(page, 'mongodb', 'Mongo Edit');
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  await expandRow(page, MONGO_DB_PATH);
  await (await findRow(page, WIDGETS_PATH)).dblclick();
  const rows = page.locator('[data-testid="document-row"]');
  await expect(rows).toHaveCount(PROJECTED.length);
  await installClipboardSpy(page);
  await rows.first().click({ button: 'right' });
  await page.locator('[data-testid="menu-item-copy-document-submenu"]').hover();
  await page.click('[data-testid="menu-item-copy-plain-json"]');
  const copied = await lastClipboardWrite(page);
  expect(JSON.parse(copied)).toEqual({ _id: 1, a: 'shown', nested: { k: 'v' } });
  expect(copied).not.toMatch(/secret|hidden|hid/);
});
