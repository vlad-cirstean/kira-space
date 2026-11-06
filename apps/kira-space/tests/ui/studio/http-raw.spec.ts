import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, test } from './fixtures';
import { httpResponse as buildHttpResponse, openHttpModeAndNewRequest } from './support/apiMode';
import { editorText } from './support/editorText';
import { IPC } from './support/ipcChannels';

// P9 §6.4: the real built bundle, real WebKit, both wire planes mocked — the inspector (D12/D14,
// exact and http2/masked) and the editor (D8/D9/D10), each seeded through the same one-snapshot-
// per-channel discipline http-request.spec.ts's own header comment states.

const REQUEST_TEXT =
  'GET /v2/orders?a=1 HTTP/1.1\r\nHost: api.example.com\r\nUser-Agent: Kira Studio/1.2.3\r\nAccept-Encoding: gzip\r\n\r\n';
const RESPONSE_HEAD = 'HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n';
const RESPONSE_BODY = '{"id":1}';

// This file's own fixture defaults diverge from the shared canonical shape (exact body/headers/
// elapsedMs/finalUrl below) — its "exact fidelity" tests rely on them without overriding every
// field explicitly, so this passes its own `base` rather than changing the shared default.
function httpResponse(overrides: Record<string, unknown>): Record<string, unknown> {
  return buildHttpResponse(overrides, {
    status: 200,
    statusText: 'OK',
    proto: 'HTTP/1.1',
    headers: [{ name: 'Content-Type', value: 'application/json' }],
    body: RESPONSE_BODY,
    bodyEncoding: 'utf8',
    bodyBytes: RESPONSE_BODY.length,
    bodyTruncated: false,
    elapsedMs: 12,
    finalUrl: 'https://api.example.com/v2/orders?a=1',
    redirects: [],
  });
}

test('Http raw — the inspector, exact fidelity', async ({ relaunch }) => {
  const RESPONSE = httpResponse({
    wire: {
      request: REQUEST_TEXT,
      responseHead: RESPONSE_HEAD,
      fidelity: 'exact',
      maskedSecrets: 0,
      requestBodyElided: false,
    },
  });
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.httpSend, response: RESPONSE }];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  await page.fill('[data-testid="http-url"]', 'https://api.example.com/v2/orders?a=1');
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');

  await page.click('[data-testid="http-response-pane-raw"]');

  const fidelity = page.locator('[data-testid="http-wire-fidelity"]');
  await expect(fidelity).toContainText('exact bytes this app wrote to the connection');

  const requestEditor = page.locator('[data-testid="http-wire-request-editor"]');
  await expect(requestEditor).toBeVisible();
  const requestEditorText = await editorText(requestEditor);
  expect(requestEditorText).toContain('GET /v2/orders?a=1 HTTP/1.1');
  expect(requestEditorText).toContain('Host: api.example.com');

  // D5: the response section concatenates the seeded responseHead with the response's own body —
  // a seeded body that does not appear is a failing assertion, not a cosmetic one.
  const responseEditor = page.locator('[data-testid="http-wire-response-editor"]');
  const responseText = await editorText(responseEditor);
  expect(responseText).toContain('HTTP/1.1 200 OK');
  expect(responseText).toContain(RESPONSE_BODY);
  expect(responseText.indexOf('HTTP/1.1 200 OK')).toBeLessThan(responseText.indexOf(RESPONSE_BODY));
});

test('Http raw — the inspector, http2 and masked', async ({ relaunch }) => {
  const MASKED_REQUEST =
    'GET /v2/orders HTTP/1.1\r\nAuthorization: Bearer {{token}}\r\nHost: api.example.com\r\n\r\n';
  const RESPONSE = httpResponse({
    wire: {
      request: MASKED_REQUEST,
      responseHead: RESPONSE_HEAD,
      fidelity: 'http2',
      maskedSecrets: 2,
      requestBodyElided: false,
    },
  });
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.httpSend, response: RESPONSE }];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  await page.fill('[data-testid="http-url"]', 'https://api.example.com/v2/orders');
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');

  await page.click('[data-testid="http-response-pane-raw"]');

  const fidelity = page.locator('[data-testid="http-wire-fidelity"]');
  await expect(fidelity).toContainText('HTTP/2');
  await expect(fidelity).toHaveClass(/warn/);

  const maskingNote = page.locator('[data-testid="http-wire-masking-note"]');
  await expect(maskingNote).toContainText('2 secret values are shown as');

  const requestEditor = page.locator('[data-testid="http-wire-request-editor"]');
  const requestText = await editorText(requestEditor);
  expect(requestText).toContain('{{token}}');
  // The masked form is what the mocked bridge sent — this pane never has a real secret value to
  // begin with, so the negative assertion is that no plaintext-shaped credential ever appears.
  expect(requestText).not.toContain('sk_live_');
});

// This test used to be named "no raw view for a stored entry" and asserted the empty
// state — the Raw pane could show nothing at all for a stored entry, since a stored
// entry never has a `wire` (nulled before persisting) and nothing reconstructed one from
// what *is* stored. It now reconstructs both documents from the snapshot's own stage-1 request/
// response fields, so this asserts that reconstruction instead of an empty state.
test('Http raw — a stored entry reconstructs its raw view (P18 D8)', async ({ relaunch }) => {
  const ENTRY = {
    id: 'e1',
    itemId: null,
    tabId: 't1',
    sentAt: '2024-06-01T10:00:00.000Z',
    method: 'GET',
    url: 'https://api.example.com/orders',
    environment: '',
    status: 200,
    statusText: 'OK',
    elapsedMs: 40,
    bodyBytes: 22,
    storedBytes: 140,
  };
  const SNAPSHOT = {
    entry: ENTRY,
    request: {
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
    },
    // No `wire` key at all — D7's own stripped-before-storage shape, exactly what Record produces.
    response: {
      status: 200,
      statusText: 'OK',
      proto: 'HTTP/1.1',
      headers: [{ name: 'Content-Type', value: 'application/json' }],
      body: '{"id":1,"name":"Ada"}',
      bodyEncoding: 'utf8',
      bodyBytes: 22,
      bodyTruncated: false,
      elapsedMs: 40,
      finalUrl: 'https://api.example.com/orders',
      redirects: [],
    },
    bodyStored: true,
    bodyStorageTruncated: false,
    requestBodyStorageTruncated: false,
  };
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.historyList, response: [ENTRY] },
    { channel: IPC.historyGet, response: SNAPSHOT },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  await page.click('[data-testid="http-response-pane-history"]');
  await page.locator('[data-testid="http-history-row"]').first().click();
  await expect(page.locator('[data-testid="http-history-band"]')).toBeVisible();

  await page.click('[data-testid="http-response-pane-raw"]');
  await expect(page.locator('[data-testid="http-raw-reconstructed"]')).toBeVisible();
  const requestEditor = page.locator('[data-testid="http-wire-request-editor"]');
  expect(await editorText(requestEditor)).toContain('GET https://api.example.com/orders HTTP/1.1');
  const responseEditor = page.locator('[data-testid="http-wire-response-editor"]');
  expect(await editorText(responseEditor)).toContain('{"id":1,"name":"Ada"}');

  // Switching back to Body still renders the stored entry — the fourth segment did not disturb
  // P8's source swap.
  await page.click('[data-testid="http-response-pane-body"]');
  const bodyEditor = page.locator('[data-testid="http-response-pane"] .response-body');
  await expect(bodyEditor.locator('.monaco-host')).toBeVisible();
  expect(await editorText(bodyEditor)).toBe('{\n  "id": 1,\n  "name": "Ada"\n}');
});

test('Http raw — the editor', async ({ relaunch }) => {
  const { window: page } = await relaunch({});

  await openHttpModeAndNewRequest(page);
  await page.fill('[data-testid="http-url"]', 'https://{{base_url}}/v2/orders');

  await page.click('[data-testid="http-edit-raw"]');
  const dialog = page.locator('[data-testid="edit-raw-dialog"]');
  await expect(dialog).toBeVisible();

  const editorHost = dialog.locator('[data-testid="edit-raw-textarea"]');
  await expect(editorHost).toBeVisible();
  // D9: pre-substitution — {{base_url}} appears literally in the generated buffer.
  const initialText = await editorText(editorHost);
  expect(initialText).toContain('{{base_url}}');
  expect(initialText).toContain('GET https://{{base_url}}/v2/orders HTTP/1.1');

  // Replace the whole buffer: a changed method and one added header.
  const editor = editorHost.locator('.view-lines');
  await editor.click();
  await page.keyboard.press('ControlOrMeta+a');
  await page.keyboard.insertText('PUT https://{{base_url}}/v2/orders HTTP/1.1\nX-Added: yes\n\n');

  await page.click('[data-testid="edit-raw-apply"]');
  await expect(dialog).toHaveCount(0);

  // P17 D18: the method select is an app-drawn button now, not a native <select>.
  await expect(page.locator('[data-testid="http-method-select"]')).toHaveAttribute(
    'data-value',
    'PUT',
  );
  await page.click('[data-testid="http-request-pane-headers"]');
  await expect(page.locator('[data-testid="http-header-name"]').first()).toHaveValue('X-Added');
  await expect(page.locator('[data-testid="http-header-value"]').first()).toHaveValue('yes');
  await expect(page.locator('[data-testid="http-url"]')).toHaveValue(
    'https://{{base_url}}/v2/orders',
  );

  // D10's own stated consequence: a urlencoded body has no raw-HTTP representation, so opening
  // the dialog over one shows the mode-change warning *before* Apply is pressed — raw HTTP has
  // nowhere for `urlencoded` to land, so the parse always folds it into `raw`.
  await page.click('[data-testid="http-request-pane-body"]');
  await page.click('[data-testid="http-body-mode-urlencoded"]');
  await page.locator('[data-testid="http-urlencoded-name"]').first().fill('q');
  await page.locator('[data-testid="http-urlencoded-value"]').first().fill('1');
  await page.click('[data-testid="http-edit-raw"]');
  await expect(dialog).toBeVisible();
  const modeChanged = page.locator('[data-testid="edit-raw-mode-changed"]');
  await expect(modeChanged).toBeVisible();
  await expect(modeChanged).toContainText('urlencoded');
  await expect(modeChanged).toContainText('raw');
  await page.click('[data-testid="edit-raw-cancel"]');
  await expect(dialog).toHaveCount(0);

  // D10: disabled with its tooltip for a formdata body.
  await page.click('[data-testid="http-body-mode-formdata"]');
  const editRawButton = page.locator('[data-testid="http-edit-raw"]');
  await expect(editRawButton).toBeDisabled();
  // Disabled control: pointer-events:none on the button itself, so the §6.3 focusable wrapper span
  // is the real hover target.
  await page.locator('[data-slot="tooltip-trigger"]').filter({ has: editRawButton }).hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(
    /has no text form that can be edited and parsed back/,
  );
});

// P175 D6 (P168 F17): a no-edit Apply keeps disabled header rows and every description.
test('Http raw — a no-edit Apply keeps disabled header rows and descriptions', async ({
  relaunch,
}) => {
  const RESTORED_TAB = {
    id: 'tab-raw-merge',
    connectionId: null,
    path: 'request',
    kind: 'http-request',
    order: 0,
    active: true,
    state: {
      method: 'GET',
      url: 'https://api.example.com/ping',
      headers: [
        { name: 'X-On', value: '1', enabled: true, description: 'kept note' },
        { name: 'X-Off', value: '2', enabled: false, description: '' },
      ],
      bodyMode: 'none',
      body: '',
      requestPane: 'headers',
      responsePane: 'body',
      responseView: 'pretty',
      requestPaneHeight: 0,
    },
  };
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.tabsList, response: [RESTORED_TAB] }];
  const { window: page } = await relaunch({ control: CONTROL });

  await page.click('[data-testid="http-field-descriptions-toggle"]');
  await page.click('[data-testid="http-edit-raw"]');
  const dialog = page.locator('[data-testid="edit-raw-dialog"]');
  await expect(dialog).toBeVisible();
  await page.click('[data-testid="edit-raw-apply"]');
  await expect(dialog).toHaveCount(0);

  const names = page.locator('[data-testid="http-header-name"]');
  await expect(names.nth(0)).toHaveValue('X-On');
  await expect(names.nth(1)).toHaveValue('X-Off');
  await expect(page.locator('[data-testid="http-header-description"]').first()).toHaveValue(
    'kept note',
  );
  await expect(page.locator('[data-testid="http-header-enabled"]').nth(1)).not.toBeChecked();
});
