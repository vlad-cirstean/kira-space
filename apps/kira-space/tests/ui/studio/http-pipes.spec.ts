import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { httpResponse, openHttpModeAndNewRequest } from './support/apiMode';
import { editorText } from './support/editorText';

// The end-to-end proof, which no unit test can give because it is a property
// of what reaches the wire — three cases, mirroring http-variables.spec.ts's own
// "substitution reaches the wire, and a secret does not" (its own line 185) and http-raw.spec.ts's
// "the inspector, http2 and masked", extended to a piped secret.
//
// This tier mocks the network layer, not Go itself (mockRuntime.ts's own header comment) — there
// is no real HTTP server here for a request to "arrive at". What this tier *can* and does prove:
// (a) the renderer's own stage 1 never attempts the transform on a secret at all — the literal
// `{{name | transform}}` text is what leaves the renderer, byte for byte, the same "never resolved
// client-side" property http-variables.spec.ts:185 already asserts for a bare secret; and
// (b) every copyable surface (Raw pane, Timeline hop, persisted history entry) renders exactly
// what a real Go backend's masking pipeline would have handed back, with neither the plaintext nor
// the transformed (piped) plaintext ever appearing — the properties the Go tests
// (bridge/http_test.go's TestMaskSecrets_MasksPipedSecretsBase64Form et al.) prove are what Go
// computes; this file proves the UI never adds a leak of its own on top.

// ---- 1. Stage 1 never resolves a piped secret — the literal reference is what leaves the renderer ----

test('a piped secret is never resolved client-side — the literal reference reaches httpSend, and the mocked Go response comes back masked', async ({
  relaunch,
}) => {
  const ENV = { id: 'env-1', name: 'Prod', sortOrder: 0, isActive: true, description: '' };
  const TOKEN_VAR = {
    id: 'var-token',
    scope: 'environment',
    ownerId: 'env-1',
    name: 'token',
    value: '',
    isSecret: true,
    sortOrder: 0,
    description: '',
  };
  const MASKED_REQUEST =
    'GET /v1 HTTP/1.1\r\nAuthorization: Bearer {{token | base64}}\r\nHost: api.example.com\r\n\r\n';
  const RESPONSE = httpResponse({
    wire: {
      request: MASKED_REQUEST,
      responseHead: 'HTTP/1.1 200 OK\r\n\r\n',
      fidelity: 'exact',
      maskedSecrets: 1,
      requestBodyElided: false,
    },
  });

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: { collections: [], items: [] } },
    { channel: IPC.variablesListEnvironments, response: [ENV] },
    {
      channel: IPC.variablesList,
      args: { scope: 'environment', ownerId: 'env-1' },
      response: [TOKEN_VAR],
    },
    { channel: IPC.httpSend, response: RESPONSE },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  await page.fill('[data-testid="http-url"]', 'https://api.example.com/v1');
  await page.click('[data-testid="http-request-pane-headers"]');
  const firstHeaderRow = page.locator('[data-testid="http-header-row"]').first();
  await firstHeaderRow.locator('[data-testid="http-header-name"]').fill('Authorization');
  await firstHeaderRow
    .locator('[data-testid="http-header-value"]')
    .fill('Bearer {{token | base64}}');

  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');

  // The renderer's own stage 1 (substitute.ts) classifies a secret reference 'deferred' and never
  // resolves — let alone transforms — it (D6's load-bearing corpus row, R1). What reaches httpSend
  // is the literal text the user typed, pipe and all.
  const sendCalls = control.log().filter((e) => e.channel === IPC.httpSend);
  expect(sendCalls).toHaveLength(1);
  expect(sendCalls[0].args).toMatchObject({
    headers: [{ name: 'Authorization', value: 'Bearer {{token | base64}}' }],
  });

  // The mocked response stands in for what Go's stage 2 actually computed (proven server-side by
  // R3's own Go tests) — the Raw pane shows exactly that masked form, and nothing else.
  await page.click('[data-testid="http-response-pane-raw"]');
  const requestEditor = page.locator('[data-testid="http-wire-request-editor"]');
  const requestText = await editorText(requestEditor);
  expect(requestText).toContain('{{token | base64}}');
  expect(requestText).not.toContain('sk_live_');
});

// ---- 2. Every copyable surface shows the masked form, never the plaintext or its piped form ----

test('the Raw pane, the timeline hop, and the persisted history entry all show the masked reference and never a value (D9)', async ({
  relaunch,
}) => {
  const MASKED_URL = 'https://api.example.com/v1?token=%7B%7Btoken%20%7C%20base64%7D%7D';
  const RESPONSE = httpResponse({
    finalUrl: MASKED_URL,
    wire: {
      request: `GET ${MASKED_URL} HTTP/1.1\r\nHost: api.example.com\r\n\r\n`,
      responseHead: 'HTTP/1.1 200 OK\r\n\r\n',
      fidelity: 'exact',
      maskedSecrets: 1,
      requestBodyElided: false,
    },
    timeline: {
      hops: [
        {
          index: 0,
          method: 'GET',
          url: MASKED_URL,
          status: 200,
          statusText: 'OK',
          proto: 'HTTP/1.1',
          reused: false,
          connAttempts: 1,
          startOffsetMs: 0,
          totalMs: 5,
        },
      ],
      totalMs: 5,
    },
  });
  const HISTORY_ENTRY = {
    id: 'hist-1',
    itemId: null,
    tabId: 't1',
    sentAt: '2026-01-01T00:00:00.000Z',
    method: 'GET',
    url: MASKED_URL,
    environment: 'Prod',
    status: 200,
    statusText: 'OK',
    elapsedMs: 5,
    bodyBytes: 0,
    storedBytes: 0,
  };

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.httpSend, response: RESPONSE },
    { channel: IPC.historyList, response: [HISTORY_ENTRY] },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  await page.fill(
    '[data-testid="http-url"]',
    'https://api.example.com/v1?token={{token | base64}}',
  );
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');

  await page.click('[data-testid="http-response-pane-raw"]');
  const requestText = await editorText(page.locator('[data-testid="http-wire-request-editor"]'));
  expect(requestText).toContain('%7B%7Btoken%20%7C%20base64%7D%7D');
  expect(requestText).not.toContain('sk_live_');

  await page.click('[data-testid="http-response-pane-timeline"]');
  const hopCaption = page.locator('[data-testid="http-timeline-hop-caption"]');
  await expect(hopCaption).toContainText('%7B%7Btoken%20%7C%20base64%7D%7D');
  const hopText = await hopCaption.innerText();
  expect(hopText).not.toContain('sk_live_');

  await page.click('[data-testid="http-response-pane-history"]');
  const historyRow = page.locator('[data-testid="http-history-row"]').first();
  await expect(historyRow).toContainText('%7B%7Btoken%20%7C%20base64%7D%7D');
});

// ---- 3. Both spellings of one secret mask both, counted as one distinct secret (D9(d)) ----

test('a request using both {{secret}} and {{secret | base64}} masks both forms, counted as one secret (D9(d))', async ({
  relaunch,
}) => {
  const MASKED_REQUEST =
    'GET /v1 HTTP/1.1\r\nX-Plain: {{token}}\r\nX-Piped: {{token | base64}}\r\nHost: api.example.com\r\n\r\n';
  const RESPONSE = httpResponse({
    wire: {
      request: MASKED_REQUEST,
      responseHead: 'HTTP/1.1 200 OK\r\n\r\n',
      fidelity: 'exact',
      // One distinct secret NAME, even though it appears in two rendered forms on the wire — the
      // property a name-keyed model (Reference has no field for "which form") could never pass by
      // construction (D9(d)), and exactly what UsedSecret's dedup-by-(Name,Placeholder) computes.
      maskedSecrets: 1,
      requestBodyElided: false,
    },
  });

  const CONTROL: ControlSnapshot[] = [{ channel: IPC.httpSend, response: RESPONSE }];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  await page.fill('[data-testid="http-url"]', 'https://api.example.com/v1');
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');

  await page.click('[data-testid="http-response-pane-raw"]');
  const maskingNote = page.locator('[data-testid="http-wire-masking-note"]');
  await expect(maskingNote).toContainText('1 secret value is shown as');

  const requestText = await editorText(page.locator('[data-testid="http-wire-request-editor"]'));
  expect(requestText).toContain('{{token}}');
  expect(requestText).toContain('{{token | base64}}');
});
