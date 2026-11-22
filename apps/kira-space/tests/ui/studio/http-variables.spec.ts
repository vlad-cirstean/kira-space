import type { Page } from '@playwright/test';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openHttpMode } from './support/apiMode';
import {
  collectionRow,
  matrixCell,
  openVariableHistory,
  openVariablesTab,
} from './support/apiRows';
import { acceptConfirm } from './support/dialogs';

// P5 §6.3: five tests, own file (§0.3 — nothing appended to collections.spec.ts or the mixed
// parity spec). Modelled on credential-reveal.spec.ts — mocking the reveal *outcome* per scenario
// rather than driving a real LocalAuthentication prompt this tier cannot reach at all.

const NOW = '2026-01-01T00:00:00.000Z';

// One collection, one request item directly under its root (no folder — one twisty click reaches
// it), reused by every test below.
const TREE = {
  collections: [
    {
      id: 'col-1',
      name: 'Orders API',
      file: 'orders-api.yaml',
      sortOrder: 0,
      createdAt: NOW,
      updatedAt: NOW,
    },
  ],
  items: [
    {
      id: 'item-1',
      collectionId: 'col-1',
      parentId: null,
      kind: 'request',
      name: 'Health check',
      sortOrder: 0,
      method: 'GET',
      url: 'https://api.example.com/healthz',
      createdAt: NOW,
      updatedAt: NOW,
    },
  ],
};
const HEALTH_REQUEST = {
  method: 'GET',
  url: 'https://api.example.com/healthz',
  headers: [],
  bodyMode: 'none',
  body: '',
  code: '',
  codeLanguage: 'json',
  urlEncoded: [],
  formData: [],
  binaryFile: null,
};

async function openVariablesDialog(page: Page): Promise<void> {
  await openHttpMode(page);
  await expect(collectionRow(page, 'col-1')).toBeVisible();
  await openVariablesTab(page);
  await expect(page.locator('[data-testid="variables-matrix"]')).toBeVisible();
}

// ---- 1. A secret is masked, and revealing it is gated ----

test('a secret is masked, and revealing it is gated', async ({ relaunch }) => {
  const PLAIN = {
    id: 'var-plain',
    scope: 'collection',
    ownerId: 'col-1',
    name: 'host',
    value: 'api.example.com',
    isSecret: false,
    secretSet: false,
    sortOrder: 0,
  };
  const SECRET_OK = {
    id: 'var-secret-ok',
    scope: 'collection',
    ownerId: 'col-1',
    name: 'apiKey',
    value: '',
    isSecret: true,
    secretSet: true,
    sortOrder: 1,
  };
  const SECRET_CANCEL = {
    id: 'var-secret-cancel',
    scope: 'collection',
    ownerId: 'col-1',
    name: 'cancelled',
    value: '',
    isSecret: true,
    secretSet: true,
    sortOrder: 2,
  };
  const SECRET_ERROR = {
    id: 'var-secret-error',
    scope: 'collection',
    ownerId: 'col-1',
    name: 'errored',
    value: '',
    isSecret: true,
    secretSet: true,
    sortOrder: 3,
  };

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: TREE },
    {
      channel: IPC.variablesList,
      args: { scope: 'collection', ownerId: 'col-1' },
      response: [PLAIN, SECRET_OK, SECRET_CANCEL, SECRET_ERROR],
    },
    {
      channel: IPC.variablesReveal,
      args: { variableId: 'var-secret-ok', confirmed: false },
      response: { value: null, error: null, outcome: 'confirmation-required' },
    },
    {
      channel: IPC.variablesReveal,
      args: { variableId: 'var-secret-ok', confirmed: true },
      response: { value: 's3cr3t', error: null, outcome: 'revealed' },
    },
    {
      channel: IPC.variablesReveal,
      args: { variableId: 'var-secret-cancel', confirmed: false },
      response: { value: null, error: null, outcome: 'cancelled' },
    },
    {
      channel: IPC.variablesReveal,
      args: { variableId: 'var-secret-error', confirmed: false },
      response: {
        value: null,
        error: 'The stored credential could not be decrypted.',
        outcome: 'error',
      },
    },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await openVariablesDialog(page);

  // The plaintext row shows its value; every secret row is masked; not one Reveal call has been
  // made yet, and no bound call's own log carries the secret's plaintext before a reveal.
  await expect(matrixCell(page, 'host', 'col-1')).toContainText('api.example.com');
  await expect(
    matrixCell(page, 'apiKey', 'col-1').locator('[data-slot="masked-value"]'),
  ).toHaveText('••••••••');
  expect(control.log().filter((e) => e.channel === IPC.variablesReveal)).toHaveLength(0);
  for (const entry of control.log()) {
    expect(JSON.stringify(entry)).not.toContain('s3cr3t');
  }

  // Confirmation-required -> the app's own confirm dialog -> accept -> a second Reveal call with
  // confirmed: true, and the value now renders.
  await page.click('[data-testid="matrix-reveal-apiKey-col-1"]');
  await expect(page.locator('[data-testid="confirm-dialog-message"]')).toHaveText(
    'This variable’s value will be displayed in plain text.',
  );
  await acceptConfirm(page);
  await expect(matrixCell(page, 'apiKey', 'col-1')).toContainText('s3cr3t');
  expect(control.log().filter((e) => e.channel === IPC.variablesReveal)).toHaveLength(2);

  // A cancelled reveal leaves the row masked, with no further call than the one Reveal itself.
  await page.click('[data-testid="matrix-reveal-cancelled-col-1"]');
  await expect(
    matrixCell(page, 'cancelled', 'col-1').locator('[data-slot="masked-value"]'),
  ).toHaveText('••••••••');

  // An error leaves the row masked too, and surfaces in the dialog's own error strip.
  await page.click('[data-testid="matrix-reveal-errored-col-1"]');
  await expect(
    matrixCell(page, 'errored', 'col-1').locator('[data-slot="masked-value"]'),
  ).toHaveText('••••••••');
  await expect(page.locator('[data-testid="variables-error"]')).toContainText(
    'could not be decrypted',
  );
});

// ---- 2. Substitution reaches the wire, and a secret does not ----

test('substitution reaches the wire, and a secret does not', async ({ relaunch }) => {
  const ENV = { id: 'env-1', name: 'Prod', sortOrder: 0, isActive: true };
  const HOST_VAR = {
    id: 'var-host',
    scope: 'environment',
    ownerId: 'env-1',
    name: 'host',
    value: 'api.example.com',
    isSecret: false,
    secretSet: false,
    sortOrder: 0,
  };
  const TOKEN_VAR = {
    id: 'var-token',
    scope: 'environment',
    ownerId: 'env-1',
    name: 'token',
    value: '',
    isSecret: true,
    secretSet: true,
    sortOrder: 1,
  };
  const RESPONSE = {
    status: 200,
    statusText: 'OK',
    proto: 'HTTP/1.1',
    headers: [],
    body: '',
    bodyEncoding: 'utf8',
    bodyBytes: 0,
    bodyTruncated: false,
    elapsedMs: 1,
    finalUrl: '',
    redirects: [],
  };

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: { collections: [], items: [] } },
    { channel: IPC.variablesListEnvironments, response: [ENV] },
    {
      channel: IPC.variablesList,
      args: { scope: 'environment', ownerId: 'env-1' },
      response: [HOST_VAR, TOKEN_VAR],
    },
    { channel: IPC.httpSend, response: RESPONSE },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await openHttpMode(page);
  await page.click('[data-testid="new-request"]');
  await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();

  await page.fill('[data-testid="http-url"]', 'https://{{host}}/v1/x?k={{missing}}');
  await page.click('[data-testid="http-request-pane-headers"]');
  const firstHeaderRow = page.locator('[data-testid="http-header-row"]').first();
  await firstHeaderRow.locator('[data-testid="http-header-name"]').fill('Authorization');
  await firstHeaderRow.locator('[data-testid="http-header-value"]').fill('Bearer {{token}}');

  // The chip is live, before any send: 1 unresolved reference, naming the unknown one — never the
  // deferred secret, which will resolve fine at send time.
  const chip = page.locator('[data-testid="http-unresolved-chip"]');
  await expect(chip).toContainText('1 unresolved');
  await chip.hover();
  const tip = page.locator('[data-slot="tooltip-content"]').first();
  await expect(tip).toContainText(/missing/);
  await expect(tip).not.toContainText(/token/);

  // OQ-API-2: the unresolved name blocks the send; Go is never called.
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-send-error"]')).toContainText('missing');
  expect(control.log().filter((e) => e.channel === IPC.httpSend)).toHaveLength(0);

  await page.fill('[data-testid="http-url"]', 'https://{{host}}/v1/x?k=1');
  await expect(chip).toHaveCount(0);
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');

  const sendCalls = control.log().filter((e) => e.channel === IPC.httpSend);
  expect(sendCalls).toHaveLength(1);
  expect(sendCalls[0].args).toMatchObject({
    url: 'https://api.example.com/v1/x?k=1',
    headers: [{ name: 'Authorization', value: 'Bearer {{token}}' }],
    environmentId: 'env-1',
    collectionId: '',
  });
});

// ---- 2b. An unresolved variable in URL, header and body blocks the send (contract: api-http-unresolved) ----

// Backend half: httpflow.TestUnresolvedVariableBlocksSend sends the same request to Go, which refuses
// it with the same text and no request reaches the recorder.
test('an unresolved variable in URL, header and body blocks the send and lists the names', async ({
  relaunch,
}) => {
  const origins = { http: 'http://flow.test' };
  const sent = contract<{
    url: string;
    headers: { name: string; value: string }[];
    body: { raw: string };
  }>('api-http-unresolved', 'args:HttpService.Send', { origins });
  const message = contract<string>('api-http-unresolved', 'error:HttpService.Send');
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: { collections: [], items: [] } },
    { channel: IPC.httpSend, response: { status: 200 } },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await openHttpMode(page);
  await page.click('[data-testid="new-request"]');
  await page.fill('[data-testid="http-url"]', sent.url);
  await page.click('[data-testid="http-request-pane-headers"]');
  const row = page.locator('[data-testid="http-header-row"]').first();
  await row.locator('[data-testid="http-header-name"]').fill(sent.headers[0].name);
  await row.locator('[data-testid="http-header-value"]').fill(sent.headers[0].value);
  await page.click('[data-testid="http-request-pane-body"]');
  await page.click('[data-testid="http-body-mode-raw"]');
  await page.locator('[data-testid="http-request-pane"]').locator('.view-lines').click();
  await page.keyboard.insertText(sent.body.raw);

  await expect(page.locator('[data-testid="http-unresolved-chip"]')).toContainText('3 unresolved');
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-send-error"]')).toHaveText(message);
  expect(control.log().filter((e) => e.channel === IPC.httpSend)).toHaveLength(0);
});

// ---- 3. Precedence is environment-over-collection ----

test('precedence is environment-over-collection', async ({ relaunch }) => {
  const ENV = { id: 'env-1', name: 'Prod', sortOrder: 0, isActive: true };
  const ENV_REGION = {
    id: 'var-env-region',
    scope: 'environment',
    ownerId: 'env-1',
    name: 'region',
    value: 'us-environment',
    isSecret: false,
    secretSet: false,
    sortOrder: 0,
  };
  const COL_REGION = {
    id: 'var-col-region',
    scope: 'collection',
    ownerId: 'col-1',
    name: 'region',
    value: 'us-collection',
    isSecret: false,
    secretSet: false,
    sortOrder: 0,
  };
  const RESPONSE = {
    status: 200,
    statusText: 'OK',
    proto: 'HTTP/1.1',
    headers: [],
    body: '',
    bodyEncoding: 'utf8',
    bodyBytes: 0,
    bodyTruncated: false,
    elapsedMs: 1,
    finalUrl: '',
    redirects: [],
  };

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: TREE },
    { channel: IPC.collectionsGetRequest, response: HEALTH_REQUEST },
    // Two snapshots sharing the same (args-less) key, on purpose — the initial mount's own load,
    // then the reload SetActiveEnvironment('') triggers, reflecting the switch to "No environment".
    { channel: IPC.variablesListEnvironments, response: [ENV] },
    { channel: IPC.variablesListEnvironments, response: [{ ...ENV, isActive: false }] },
    { channel: IPC.variablesSetActiveEnvironment, args: { id: '' } },
    {
      channel: IPC.variablesList,
      args: { scope: 'environment', ownerId: 'env-1' },
      response: [ENV_REGION],
    },
    {
      channel: IPC.variablesList,
      args: { scope: 'collection', ownerId: 'col-1' },
      response: [COL_REGION],
    },
    { channel: IPC.httpSend, response: RESPONSE },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await openHttpMode(page);
  await collectionRow(page, 'col-1').locator('[data-testid="tree-twisty"]').click();
  await page.locator('[data-testid="collection-row"][data-id="item-1"]').dblclick();
  await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();

  await page.fill('[data-testid="http-url"]', 'https://api.example.com/{{region}}');
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');

  // Falls back to the collection's value with no environment active.
  await page.click('[data-testid="api-environment-select"]');
  await page.click('[data-testid="api-environment-option-none"]');
  await expect(page.locator('[data-testid="api-environment-select"]')).toHaveAttribute(
    'data-value',
    '',
  );
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');

  const sendCalls = control.log().filter((e) => e.channel === IPC.httpSend);
  expect(sendCalls).toHaveLength(2);
  expect(sendCalls[0].args).toMatchObject({ url: 'https://api.example.com/us-environment' });
  expect(sendCalls[1].args).toMatchObject({ url: 'https://api.example.com/us-collection' });
});

// ---- 5. History lists prior values, read-only ----

test('history lists prior values and offers no restore', async ({ relaunch }) => {
  const CURRENT = {
    id: 'var-1',
    scope: 'collection',
    ownerId: 'col-1',
    name: 'baseUrl',
    value: 'v3',
    isSecret: false,
    secretSet: false,
    sortOrder: 0,
  };
  const HISTORY = [
    {
      id: 'h3',
      variableId: 'var-1',
      value: 'v3',
      isSecret: false,
      recordedAt: '2026-01-03T00:00:00.000Z',
    },
    {
      id: 'h2',
      variableId: 'var-1',
      value: 'v2',
      isSecret: false,
      recordedAt: '2026-01-02T00:00:00.000Z',
    },
    {
      id: 'h1',
      variableId: 'var-1',
      value: 'v1',
      isSecret: false,
      recordedAt: '2026-01-01T00:00:00.000Z',
    },
  ];

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: TREE },
    {
      channel: IPC.variablesList,
      args: { scope: 'collection', ownerId: 'col-1' },
      response: [CURRENT],
    },
    { channel: IPC.variablesHistory, args: { variableId: 'var-1' }, response: HISTORY },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await openVariablesDialog(page);
  await openVariableHistory(page, CURRENT.name);

  const entries = page.locator('[data-testid="variable-history-entry"]');
  await expect(entries).toHaveCount(3);
  // Newest first.
  await expect(entries.nth(0).locator('[data-testid="variable-history-value"]')).toHaveText('v3');
  await expect(entries.nth(1).locator('[data-testid="variable-history-value"]')).toHaveText('v2');
  await expect(entries.nth(2).locator('[data-testid="variable-history-value"]')).toHaveText('v1');

  await expect(page.locator('[data-testid="variable-history-restore"]')).toHaveCount(0);
});

// ---- 8. The active environment's base variable reaches the wire (contract: api-boot) ----

// Backend half: apiflow.TestActiveEnvironmentResolvesBase stores the same fixture.
test("the active environment's base variable reaches the wire", async ({ relaunch }) => {
  const origins = { http: 'http://flow.test' };
  const envs = contract<{ id: string; isActive: boolean }[]>(
    'api-boot',
    'VariablesService.ListEnvironments',
  );
  const vars = contract('api-boot', 'VariablesService.List', { origins });
  const sent = contract<{ url: string; environmentId: string }>(
    'api-boot',
    'args:HttpService.Send',
    {
      origins,
    },
  );
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: { collections: [], items: [] } },
    { channel: IPC.variablesListEnvironments, response: envs },
    {
      channel: IPC.variablesList,
      args: { scope: 'environment', ownerId: envs[0].id },
      response: vars,
    },
    { channel: IPC.httpSend, response: contract('api-boot', 'HttpService.Send', { origins }) },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await openHttpMode(page);
  await page.click('[data-testid="new-request"]');
  await page.fill('[data-testid="http-url"]', '{{base}}/echo?from=ui');
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');

  const sendCalls = control.log().filter((e) => e.channel === IPC.httpSend);
  expect(sendCalls).toHaveLength(1);
  expect(sendCalls[0].args).toMatchObject({
    url: sent.url,
    environmentId: sent.environmentId,
  });
});

// ---- 9. A secret reference stays literal in the send args and the history list (contract: api-http) ----

// Backend half: httpflow.TestSendResolvesSecretsOnlyOnTheWire stores the same fixture and proves the
// server gets the plaintext while history and the op log keep the placeholder.
test('a secret reference stays literal in the send args and the history list', async ({
  relaunch,
}) => {
  const origins = { http: 'http://flow.test' };
  const envs = contract<{ id: string }[]>('api-http', 'VariablesService.ListEnvironments');
  const vars = contract('api-http', 'VariablesService.List');
  const sent = contract<{ url: string; headers: unknown[]; environmentId: string }>(
    'api-http',
    'args:HttpService.Send',
    { origins },
  );
  const history = contract<{ url: string }[]>('api-http', 'ResponseHistoryService.List', {
    origins,
  });
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: { collections: [], items: [] } },
    { channel: IPC.variablesListEnvironments, response: envs },
    {
      channel: IPC.variablesList,
      args: { scope: 'environment', ownerId: envs[0].id },
      response: vars,
    },
    { channel: IPC.httpSend, response: contract('api-http', 'HttpService.Send') },
    { channel: IPC.historyList, response: history },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await openHttpMode(page);
  await page.click('[data-testid="new-request"]');
  await page.fill('[data-testid="http-url"]', sent.url);
  await page.click('[data-testid="http-request-pane-headers"]');
  const row = page.locator('[data-testid="http-header-row"]').first();
  const header = sent.headers[0] as { name: string; value: string };
  await row.locator('[data-testid="http-header-name"]').fill(header.name);
  await row.locator('[data-testid="http-header-value"]').fill(header.value);
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');

  const sendCalls = control.log().filter((e) => e.channel === IPC.httpSend);
  expect(sendCalls).toHaveLength(1);
  expect(sendCalls[0].args).toMatchObject({
    url: sent.url,
    headers: sent.headers,
    environmentId: sent.environmentId,
  });

  await page.click('[data-testid="http-response-pane-history"]');
  const rows = page.locator('[data-testid="http-history-row"]');
  await expect(rows).toHaveCount(history.length);
  await expect(rows.first()).toContainText('200');
  await expect(rows.first()).not.toContainText('sekret');
});
