import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { modeTab, openHttpModeAndNewRequest } from './support/apiMode';
import { diffEditorText, editorText } from './support/editorText';

// P8 §6.3: three tests, each seeding historyList (and historyGet where needed) rather than
// sending twice — F12's whole point (P2 §8 OQ-8's predicted one-snapshot-per-channel limitation
// does not bite here, because history never arrives through a second httpSend).

// A minimal, always-present request half for a snapshot's own `request` field — no test below
// asserts on it, only on the response side.
const EMPTY_REQUEST = {
  method: 'GET',
  url: 'https://api.example.com/orders',
  headers: [],
  body: {
    mode: 'none',
    raw: '',
    code: '',
    codeLanguage: '',
    urlEncoded: [],
    formData: [],
    file: '',
  },
};

test('Http history — browse a request’s past responses', async ({ relaunch }) => {
  const OLDEST = {
    id: 'e1',
    itemId: null,
    tabId: 't1',
    sentAt: '2024-06-01T10:00:00.000Z',
    method: 'GET',
    url: 'https://api.example.com/orders',
    environment: '',
    status: 200,
    statusText: 'OK',
    elapsedMs: 100,
    bodyBytes: 50,
    storedBytes: 140,
  };
  const MIDDLE = {
    id: 'e2',
    itemId: null,
    tabId: 't1',
    sentAt: '2024-06-01T10:05:00.000Z',
    method: 'GET',
    url: 'https://api.example.com/orders',
    environment: 'Staging',
    status: 404,
    statusText: 'Not Found',
    elapsedMs: 80,
    bodyBytes: 22,
    storedBytes: 96,
  };
  const NEWEST = {
    id: 'e3',
    itemId: null,
    tabId: 't1',
    sentAt: '2024-06-01T10:10:00.000Z',
    method: 'GET',
    url: 'https://api.example.com/orders',
    environment: '',
    status: 200,
    statusText: 'OK',
    elapsedMs: 60,
    bodyBytes: 40,
    storedBytes: 110,
  };

  const MIDDLE_SNAPSHOT = {
    entry: MIDDLE,
    request: EMPTY_REQUEST,
    response: {
      status: 404,
      statusText: 'Not Found',
      proto: 'HTTP/1.1',
      headers: [{ name: 'Content-Type', value: 'application/json' }],
      body: '{"error":"not found"}',
      bodyEncoding: 'utf8',
      bodyBytes: 22,
      bodyTruncated: false,
      elapsedMs: 80,
      finalUrl: 'https://api.example.com/orders',
      redirects: [],
    },
    bodyStored: true,
    bodyStorageTruncated: false,
    requestBodyStorageTruncated: false,
  };

  const CONTROL: ControlSnapshot[] = [
    // Newest first — the same order Record's own per-scope trim/List orders by.
    { channel: IPC.historyList, response: [NEWEST, MIDDLE, OLDEST] },
    { channel: IPC.historyGet, response: MIDDLE_SNAPSHOT },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  await page.click('[data-testid="http-response-pane-history"]');

  const rows = page.locator('[data-testid="http-history-row"]');
  await expect(rows).toHaveCount(3);
  await expect(rows.nth(0)).toContainText('200');
  await expect(rows.nth(0)).toContainText('60ms');
  await expect(rows.nth(1)).toContainText('404');
  await expect(rows.nth(1)).toContainText('Staging');
  await expect(rows.nth(2)).toContainText('200');
  await expect(rows.nth(2)).toContainText('100ms');

  // P22b D1: a secondary status surface (the history list) now carries the code's meaning too —
  // not as a bare chip, since a dense list row has no room for a full sentence on its own line.
  await rows.nth(1).locator('[data-testid="http-history-status-chip"]').hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(
    'the server has no resource at this URL',
  );

  await rows.nth(1).click();

  const band = page.locator('[data-testid="http-history-band"]');
  await expect(band).toBeVisible();
  await expect(band).toContainText('GET');
  await expect(band).toContainText('https://api.example.com/orders');

  const status = page.locator('[data-testid="http-status"]');
  await expect(status).toContainText('404');
  await expect(status).toHaveClass(/err/);
  // P28 D1 reverses D1/F1's "the hint stays an inline line": the standing caption is gone and the
  // meaning is the chip's own tooltip, the same way ResponseHistoryList/ResponseDiffDialog/
  // TimelinePane have always shown it.
  await status.hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(
    /no resource at this URL/,
  );
  await expect(page.locator('[data-testid="http-status-hint"]')).toHaveCount(0);

  const bodyEditor = page.locator('[data-testid="http-response-pane"] .response-body');
  await expect(bodyEditor.locator('.monaco-host')).toBeVisible();
  expect(await editorText(bodyEditor)).toBe('{\n  "error": "not found"\n}');

  await page.click('[data-testid="http-history-back"]');
  await expect(band).toHaveCount(0);
});

test('Http history — restore, and the storage notices', async ({ relaunch }) => {
  const RESTORED_TAB = {
    id: 'tab-hist-1',
    connectionId: null,
    path: 'request',
    kind: 'http-request',
    order: 0,
    active: true,
    state: {
      method: 'GET',
      url: 'https://api.example.com/export',
      headers: [],
      bodyMode: 'none',
      body: '',
      requestPane: 'params',
      // A restored tab that last had the History pane open (P8 D10/D11: a pane choice persists
      // exactly like the two that already did).
      responsePane: 'history',
      responseView: 'pretty',
      requestPaneHeight: 0,
    },
  };

  const TRUNCATED = {
    id: 'trunc-1',
    itemId: null,
    tabId: 'tab-hist-1',
    sentAt: '2024-06-01T09:05:00.000Z',
    method: 'GET',
    url: 'https://api.example.com/export',
    environment: '',
    status: 200,
    statusText: 'OK',
    elapsedMs: 300,
    bodyBytes: 10 * 1024 * 1024,
    storedBytes: 262_144,
  };
  const BINARY = {
    id: 'bin-1',
    itemId: null,
    tabId: 'tab-hist-1',
    sentAt: '2024-06-01T09:00:00.000Z',
    method: 'GET',
    url: 'https://api.example.com/export',
    environment: '',
    status: 200,
    statusText: 'OK',
    elapsedMs: 900,
    bodyBytes: 412 * 1024,
    storedBytes: 300,
  };

  const TRUNCATED_SNAPSHOT = {
    entry: TRUNCATED,
    request: EMPTY_REQUEST,
    response: {
      status: 200,
      statusText: 'OK',
      proto: 'HTTP/1.1',
      headers: [],
      body: 'x'.repeat(10),
      bodyEncoding: 'utf8',
      bodyBytes: 10 * 1024 * 1024,
      // F9: two independent booleans — the transfer was ALSO truncated (a genuinely huge
      // response), so both notices must render together, not one instead of the other.
      bodyTruncated: true,
      elapsedMs: 300,
      finalUrl: 'https://api.example.com/export',
      redirects: [],
    },
    bodyStored: true,
    bodyStorageTruncated: true,
    requestBodyStorageTruncated: false,
    requestFieldsElided: true,
  };
  const BINARY_SNAPSHOT = {
    entry: BINARY,
    request: EMPTY_REQUEST,
    response: {
      status: 200,
      statusText: 'OK',
      proto: 'HTTP/1.1',
      headers: [],
      body: '',
      bodyEncoding: 'base64',
      bodyBytes: 412 * 1024,
      bodyTruncated: false,
      elapsedMs: 900,
      finalUrl: 'https://api.example.com/export',
      redirects: [],
    },
    bodyStored: false,
    bodyStorageTruncated: false,
    requestBodyStorageTruncated: false,
  };

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [RESTORED_TAB] },
    { channel: IPC.historyList, response: [TRUNCATED, BINARY] },
    { channel: IPC.historyGet, args: { id: 'trunc-1' }, response: TRUNCATED_SNAPSHOT },
    { channel: IPC.historyGet, args: { id: 'bin-1' }, response: BINARY_SNAPSHOT },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  // hydrateTabs derives the boot mode from the restored active tab's own kind.
  await expect(modeTab(page, 'api')).toHaveClass(/is-active/);
  const view = page.locator('[data-testid="http-request-view"]');
  await expect(view).toBeVisible();

  // D6/P2's own property, still true: no reconnect gate (nothing to reconnect), and no live
  // response — a restored tab never had a send in this session.
  await expect(view).not.toContainText('Reconnect');
  await expect(page.locator('[data-testid="http-status"]')).toHaveCount(0);

  const rows = page.locator('[data-testid="http-history-row"]');
  await expect(rows).toHaveCount(2);

  // View the binary entry: the dedicated note renders, and no editor is mounted for it.
  await rows.nth(1).click();
  await expect(page.locator('[data-testid="http-history-binary-note"]')).toBeVisible();
  await expect(page.locator('[data-testid="http-history-fields-elided"]')).toHaveCount(0);
  await expect(
    page.locator('[data-testid="http-response-pane"] .response-body .monaco-host'),
  ).toHaveCount(0);

  // Back to History, then the storage-truncated entry: both notices render together (F9).
  await page.click('[data-testid="http-response-pane-history"]');
  await rows.nth(0).click();
  await expect(page.locator('[data-testid="http-history-truncated"]')).toBeVisible();
  await expect(page.locator('[data-testid="http-body-truncated"]')).toBeVisible();
  await expect(page.locator('[data-testid="http-history-fields-elided"]')).toHaveText(
    'Request field values weren’t stored because they were too large for history.',
  );
});

test('Http history — compare two responses', async ({ relaunch }) => {
  const ENTRY_A = {
    id: 'cmp-a',
    itemId: null,
    tabId: 't1',
    sentAt: '2024-06-01T08:00:00.000Z',
    method: 'GET',
    url: 'https://api.example.com/orders/1',
    environment: '',
    status: 200,
    statusText: 'OK',
    elapsedMs: 50,
    bodyBytes: 22,
    storedBytes: 140,
  };
  const ENTRY_B = {
    id: 'cmp-b',
    itemId: null,
    tabId: 't1',
    sentAt: '2024-06-01T08:05:00.000Z',
    method: 'GET',
    url: 'https://api.example.com/orders/1',
    environment: '',
    status: 404,
    statusText: 'Not Found',
    elapsedMs: 20,
    bodyBytes: 36,
    storedBytes: 150,
  };

  const SNAPSHOT_A = {
    entry: ENTRY_A,
    request: EMPTY_REQUEST,
    response: {
      status: 200,
      statusText: 'OK',
      proto: 'HTTP/1.1',
      headers: [
        { name: 'Content-Type', value: 'application/json' },
        { name: 'X-Total', value: '10' },
      ],
      body: '{"id":1,"name":"Ada"}',
      bodyEncoding: 'utf8',
      bodyBytes: 22,
      bodyTruncated: false,
      elapsedMs: 50,
      finalUrl: 'https://api.example.com/orders/1',
      redirects: [],
    },
    bodyStored: true,
    bodyStorageTruncated: false,
    requestBodyStorageTruncated: false,
  };
  const SNAPSHOT_B = {
    entry: ENTRY_B,
    request: EMPTY_REQUEST,
    response: {
      status: 404,
      statusText: 'Not Found',
      proto: 'HTTP/1.1',
      headers: [
        { name: 'Content-Type', value: 'application/json' },
        { name: 'X-Total', value: '12' },
        { name: 'X-New', value: 'yes' },
      ],
      body: '{"id":1,"name":"Ada","active":true}',
      bodyEncoding: 'utf8',
      bodyBytes: 36,
      bodyTruncated: false,
      elapsedMs: 20,
      finalUrl: 'https://api.example.com/orders/1',
      redirects: [],
    },
    bodyStored: true,
    bodyStorageTruncated: false,
    requestBodyStorageTruncated: false,
  };

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.historyList, response: [ENTRY_B, ENTRY_A] },
    { channel: IPC.historyGet, args: { id: 'cmp-a' }, response: SNAPSHOT_A },
    { channel: IPC.historyGet, args: { id: 'cmp-b' }, response: SNAPSHOT_B },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  await page.click('[data-testid="http-response-pane-history"]');

  const rows = page.locator('[data-testid="http-history-row"]');
  await expect(rows).toHaveCount(2);
  await rows.nth(0).locator('[data-testid="http-history-checkbox"]').check();
  await rows.nth(1).locator('[data-testid="http-history-checkbox"]').check();

  const compareButton = page.locator('[data-testid="http-history-compare"]');
  await expect(compareButton).toBeEnabled();
  await compareButton.click();

  const dialog = page.locator('[data-testid="http-diff-dialog"]');
  await expect(dialog).toBeVisible();

  // A is fixed as the older entry by sentAt regardless of click/check order (D12).
  const statusA = page.locator('[data-testid="http-diff-status-a"]');
  const statusB = page.locator('[data-testid="http-diff-status-b"]');
  await expect(statusA).toContainText('200');
  await expect(statusA).toHaveClass(/ok/);
  await expect(statusB).toContainText('404');
  await expect(statusB).toHaveClass(/err/);

  // One changed header (X-Total) and one added header (X-New) — Content-Type is unchanged and
  // collapsed behind the disclosure by default.
  const headerRows = dialog.locator('[data-testid="http-diff-header-row"]');
  await expect(headerRows).toHaveCount(2);
  await expect(headerRows.filter({ hasText: 'x-total' })).toContainText('Changed');
  await expect(headerRows.filter({ hasText: 'x-new' })).toContainText('Added');

  // Both bodies are pretty-printed (indented) before diffing, since both are JSON — the seeded
  // bodies above are minified, so an unindented render here is a failing assertion (D12).
  const mergeHost = page.locator('[data-testid="http-diff-merge"]');
  await expect(mergeHost.locator('.monaco-diff-editor')).toBeVisible({ timeout: 10_000 });
  const diffText = await diffEditorText(mergeHost);
  expect(diffText.original).toBe('{\n  "id": 1,\n  "name": "Ada"\n}');
  expect(diffText.modified).toBe('{\n  "id": 1,\n  "name": "Ada",\n  "active": true\n}');
});

// ---- P18 D1: the live-update bug, and its exact repro ----

const SEND_RESPONSE = {
  status: 200,
  statusText: 'OK',
  proto: 'HTTP/1.1',
  headers: [],
  body: '',
  bodyEncoding: 'utf8',
  bodyBytes: 0,
  bodyTruncated: false,
  elapsedMs: 5,
  finalUrl: 'https://api.example.com/orders',
  redirects: [],
};

test('Http history — the list refreshes after a send made while another pane was showing (P18 D1)', async ({
  relaunch,
}) => {
  const NEWEST = {
    id: 'live-2',
    itemId: null,
    tabId: 't1',
    sentAt: '2024-06-01T10:05:00.000Z',
    method: 'GET',
    url: 'https://api.example.com/orders',
    environment: '',
    status: 200,
    statusText: 'OK',
    elapsedMs: 5,
    bodyBytes: 0,
    storedBytes: 60,
  };
  const OLDEST = {
    id: 'live-1',
    itemId: null,
    tabId: 't1',
    sentAt: '2024-06-01T10:00:00.000Z',
    method: 'GET',
    url: 'https://api.example.com/orders',
    environment: '',
    status: 200,
    statusText: 'OK',
    elapsedMs: 50,
    bodyBytes: 0,
    storedBytes: 60,
  };

  const CONTROL: ControlSnapshot[] = [
    // Two historyList answers for the same (itemId: '') scope, consumed in order (mockRuntime.ts
    // strips tabId from the match key, so a scratch tab's own random id never has to be
    // predicted): the first is ResponsePane.vue's own "does this tab have any history at all"
    // mount-time probe, before either send — empty. The second is what a real reopen of
    // the History pane fetches after both sends. Pre-fix, this second answer is never asked for
    // at all: `entries` is already non-null (from the first, empty answer) and `stale` has no
    // reader, so ensureLoaded's own guard silently returns without refetching.
    { channel: IPC.historyList, args: { itemId: '' }, response: [] },
    { channel: IPC.historyList, args: { itemId: '' }, response: [NEWEST, OLDEST] },
    // One snapshot only (F16's own rule in this file's header comment): a send's opId is
    // renderer-minted, so two sends in one test can only share one httpSend answer.
    { channel: IPC.httpSend, response: SEND_RESPONSE },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  await page.fill('[data-testid="http-url"]', 'https://api.example.com/orders');

  // Both sends happen with the Body pane showing (the default, and where a user is after every
  // send) — never opening History in between, so both take the lazy branch (F3's exact repro).
  await expect(page.locator('[data-testid="http-response-pane-body"]')).toHaveAttribute(
    'data-state',
    'on',
  );
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');

  // Only now does the pane switch to History — this is the one call site (ResponseHistoryList's
  // own onMounted) the bug lived in.
  await page.click('[data-testid="http-response-pane-history"]');

  const rows = page.locator('[data-testid="http-history-row"]');
  await expect(rows).toHaveCount(2);
  // Newest first — the same order Record's own per-scope trim/List orders by (and the order this
  // test's own List answer above is already sorted in, matching every other test in this file).
  await expect(rows.nth(0)).toContainText('5ms');
  await expect(rows.nth(1)).toContainText('50ms');
});

test('Http history — sending while viewing a stored response shows the new one (P18 D3)', async ({
  relaunch,
}) => {
  const STORED = {
    id: 'stored-1',
    itemId: null,
    tabId: 't1',
    sentAt: '2024-06-01T09:00:00.000Z',
    method: 'GET',
    url: 'https://api.example.com/orders',
    environment: '',
    status: 404,
    statusText: 'Not Found',
    elapsedMs: 20,
    bodyBytes: 0,
    storedBytes: 60,
  };
  const STORED_SNAPSHOT = {
    entry: STORED,
    request: EMPTY_REQUEST,
    response: {
      status: 404,
      statusText: 'Not Found',
      proto: 'HTTP/1.1',
      headers: [],
      body: '',
      bodyEncoding: 'utf8',
      bodyBytes: 0,
      bodyTruncated: false,
      elapsedMs: 20,
      finalUrl: 'https://api.example.com/orders',
      redirects: [],
    },
    bodyStored: true,
    bodyStorageTruncated: false,
    requestBodyStorageTruncated: false,
  };

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.historyList, response: [STORED] },
    { channel: IPC.historyGet, response: STORED_SNAPSHOT },
    { channel: IPC.httpSend, response: SEND_RESPONSE },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  await page.fill('[data-testid="http-url"]', 'https://api.example.com/orders');
  await page.click('[data-testid="http-response-pane-history"]');
  await page.click('[data-testid="http-history-row"]');

  const band = page.locator('[data-testid="http-history-band"]');
  await expect(band).toBeVisible();
  await expect(page.locator('[data-testid="http-status"]')).toContainText('404');

  // D10's own click also switches the pane back to Body — send from there, the same place a user
  // actually is after clicking a history row.
  await page.click('[data-testid="http-send"]');

  await expect(band).toHaveCount(0);
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');
});

test('Http history — a full list says only the last thirty are kept (P18 D6)', async ({
  relaunch,
}) => {
  function entryAt(i: number): unknown {
    return {
      id: `cap-${i}`,
      itemId: null,
      tabId: 't1',
      sentAt: `2024-06-01T10:${String(i).padStart(2, '0')}:00.000Z`,
      method: 'GET',
      url: 'https://api.example.com/orders',
      environment: '',
      status: 200,
      statusText: 'OK',
      elapsedMs: 5,
      bodyBytes: 0,
      storedBytes: 60,
    };
  }
  const THIRTY = Array.from({ length: 30 }, (_, i) => entryAt(i));
  const TWENTY_NINE = THIRTY.slice(1);

  const FULL_CONTROL: ControlSnapshot[] = [{ channel: IPC.historyList, response: THIRTY }];
  {
    const { window: page } = await relaunch({ control: FULL_CONTROL });
    await openHttpModeAndNewRequest(page);
    await page.click('[data-testid="http-response-pane-history"]');
    await expect(page.locator('[data-testid="http-history-row"]')).toHaveCount(30);
    await expect(page.locator('[data-testid="http-history-cap-note"]')).toBeVisible();
    await expect(page.locator('[data-testid="http-history-cap-note"]')).toContainText(
      'the last 30',
    );
  }

  const UNDER_CAP_CONTROL: ControlSnapshot[] = [
    { channel: IPC.historyList, response: TWENTY_NINE },
  ];
  {
    const { window: page } = await relaunch({ control: UNDER_CAP_CONTROL });
    await openHttpModeAndNewRequest(page);
    await page.click('[data-testid="http-response-pane-history"]');
    await expect(page.locator('[data-testid="http-history-row"]')).toHaveCount(29);
    await expect(page.locator('[data-testid="http-history-cap-note"]')).toHaveCount(0);
  }
});

// ---- P18 D8: a stored entry's Raw pane shows what was sent ----

test('Http history — a stored entry’s Raw view shows what was sent (P18 D8)', async ({
  relaunch,
}) => {
  const ENTRY = {
    id: 'raw-1',
    itemId: null,
    tabId: 't1',
    sentAt: '2024-06-01T09:00:00.000Z',
    method: 'POST',
    url: 'https://api.example.com/orders',
    environment: '',
    status: 201,
    statusText: 'Created',
    elapsedMs: 30,
    bodyBytes: 2,
    storedBytes: 200,
  };
  const SNAPSHOT = {
    entry: ENTRY,
    request: {
      method: 'POST',
      url: 'https://api.example.com/orders',
      headers: [
        { name: 'Authorization', value: 'Bearer {{token}}' },
        { name: 'Content-Type', value: 'application/json' },
      ],
      body: {
        mode: 'raw',
        raw: '{}',
        code: '',
        codeLanguage: '',
        urlEncoded: [],
        formData: [],
        file: '',
      },
    },
    response: {
      status: 201,
      statusText: 'Created',
      proto: 'HTTP/1.1',
      headers: [{ name: 'Content-Type', value: 'application/json' }],
      body: '{"id":1}',
      bodyEncoding: 'utf8',
      bodyBytes: 8,
      bodyTruncated: false,
      elapsedMs: 30,
      finalUrl: 'https://api.example.com/orders',
      redirects: [],
    },
    bodyStored: true,
    bodyStorageTruncated: false,
    requestBodyStorageTruncated: true,
    requestFieldsElided: true,
  };

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.historyList, response: [ENTRY] },
    { channel: IPC.historyGet, response: SNAPSHOT },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  await page.click('[data-testid="http-response-pane-history"]');
  await page.click('[data-testid="http-history-row"]');

  await page.click('[data-testid="http-response-pane-raw"]');

  await expect(page.locator('[data-testid="http-raw-reconstructed"]')).toBeVisible();
  await expect(page.locator('[data-testid="http-history-request-truncated"]')).toBeVisible();
  await expect(page.locator('[data-testid="http-history-request-fields-elided"]')).toBeVisible();

  const requestEditor = page.locator('[data-testid="http-wire-request-editor"]');
  await expect(requestEditor).toBeVisible();
  const requestText = await editorText(requestEditor);
  expect(requestText).toContain('POST https://api.example.com/orders HTTP/1.1');
  expect(requestText).toContain('Authorization: Bearer {{token}}');
  expect(requestText).toContain('{}');

  const responseEditor = page.locator('[data-testid="http-wire-response-editor"]');
  const responseText = await editorText(responseEditor);
  expect(responseText).toContain('201 Created');
  expect(responseText).toContain('{"id":1}');
});
