import type { ControlSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { modeTab, openHttpRequestView } from './support/apiMode';
import { installFakeTimers } from './support/clock';
import { contract } from './support/contract';
import { editorText } from './support/editorText';
import { IPC } from './support/ipcChannels';

// Six scenarios against the real built bundle, none needing a mockRuntime.ts change —
// variablesReveal is already registered, and every other bound call these scenarios touch
// (collectionsList, variablesList, variablesListEnvironments) is an existing channel.
// Own file.

const NOW = '2026-01-01T00:00:00.000Z';
const UUID_RE = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i;

// ---- 1. Import populates a new tab ----

test('import populates a new tab, and the first tab is left intact', async ({ relaunch }) => {
  const TREE = {
    collections: [
      { id: 'col-1', name: 'Orders API', sortOrder: 0, createdAt: NOW, updatedAt: NOW },
    ],
    items: [],
  };
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.collectionsList, response: TREE }];
  const { window: page } = await relaunch({ control: CONTROL });

  await modeTab(page, 'api').click();
  await expect(page.locator('[data-testid="collection-row"][data-id="col-1"]')).toBeVisible();

  // A first tab, open and edited, before the import ever happens.
  await page.click('[data-testid="new-request"]');
  await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();
  await page.fill('[data-testid="http-url"]', 'https://original.example.com/keep-me');

  await page
    .locator('[data-testid="tree-background"]')
    .click({ button: 'right', position: { x: 10, y: 400 } });
  await page.click('[data-testid="menu-item-import-curl"]');
  await expect(page.locator('[data-testid="import-curl-dialog"]')).toBeVisible();

  const CURL =
    "curl -X POST 'https://api.example.com/v1/orders' -H 'Authorization: Bearer abc' -H 'Content-Type: application/json' -d '{\"id\":1}'";
  await page.fill('[data-testid="import-curl-textarea"]', CURL);
  const summary = page.locator('[data-testid="import-curl-summary"]');
  await expect(summary).toContainText('POST');
  await expect(summary).toContainText('api.example.com/v1/orders');
  await expect(page.locator('[data-testid="import-curl-warnings"] li')).toHaveCount(0);

  await page.click('[data-testid="import-curl-submit"]');
  await expect(page.locator('[data-testid="import-curl-dialog"]')).toHaveCount(0);

  const tabs = page.locator('[data-testid="tab"]');
  await expect(tabs).toHaveCount(2);

  // The now-active (second, imported) tab.
  await expect(page.locator('[data-testid="http-method-chip"]')).toHaveText('POST');
  await expect(page.locator('[data-testid="http-url"]')).toHaveValue(
    'https://api.example.com/v1/orders',
  );

  await page.click('[data-testid="http-request-pane-headers"]');
  const headerRows = page.locator('[data-testid="http-header-row"]');
  await expect(headerRows.nth(0).locator('[data-testid="http-header-name"]')).toHaveValue(
    'Authorization',
  );
  await expect(headerRows.nth(0).locator('[data-testid="http-header-value"]')).toHaveValue(
    'Bearer abc',
  );
  await expect(headerRows.nth(1).locator('[data-testid="http-header-name"]')).toHaveValue(
    'Content-Type',
  );
  await expect(headerRows.nth(1).locator('[data-testid="http-header-value"]')).toHaveValue(
    'application/json',
  );

  // P15 D6: a JSON body is presented as the top-level JSON segment now, not "Body (code)" — same
  // stored state (bodyMode: 'code', codeLanguage: 'json'), new presentation.
  await expect(page.locator('[data-testid="http-request-pane-body"]')).toContainText('Body (JSON)');
  await page.click('[data-testid="http-request-pane-body"]');
  const pane = page.locator('[data-testid="http-request-pane"]');
  await expect(pane.locator('.monaco-host')).toBeVisible();
  expect(await editorText(pane)).toContain('{"id":1}');

  // The first tab's own contents are untouched by the import.
  await tabs.nth(0).click();
  await expect(page.locator('[data-testid="http-url"]')).toHaveValue(
    'https://original.example.com/keep-me',
  );
});

// ---- 2. Warnings are shown before Import ----

test('warnings are shown before Import, and the coerced method sticks after importing', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: { collections: [], items: [] } },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await modeTab(page, 'api').click();
  await expect(page.locator('[data-testid="api-start"]')).toBeVisible();
  await page.click('[data-testid="import-curl-start"]');
  await expect(page.locator('[data-testid="import-curl-dialog"]')).toBeVisible();

  await page.fill(
    '[data-testid="import-curl-textarea"]',
    'curl -k -X PROPFIND https://api.example.com/x',
  );
  const warnings = page.locator('[data-testid="import-curl-warnings"] li');
  await expect(warnings).toHaveCount(2);
  await expect(warnings.nth(0)).toHaveAttribute('data-kind', 'unsupported-flag');
  await expect(warnings.nth(1)).toHaveAttribute('data-kind', 'method-coerced');

  await page.click('[data-testid="import-curl-submit"]');
  await expect(page.locator('[data-testid="http-method-chip"]')).toHaveText('GET');
});

// Finding 15: the preview used to re-lex the entire pasted text on every keystroke — typing
// character-by-character (unlike .fill's single input event) is what actually exercises the
// debounce this fix adds.
test('the curl preview is debounced, not re-lexed on every keystroke', async ({ relaunch }) => {
  // TIMING.preview — restated here rather than imported across the web/node
  // tsconfig boundary (this suite's existing convention, see `global.d.ts:1`-`5`).
  const PREVIEW_DEBOUNCE_MS = 250;
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: { collections: [], items: [] } },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await modeTab(page, 'api').click();
  await expect(page.locator('[data-testid="api-start"]')).toBeVisible();
  await page.click('[data-testid="import-curl-start"]');
  await expect(page.locator('[data-testid="import-curl-dialog"]')).toBeVisible();
  await installFakeTimers(page); // support/clock.ts — after boot, never before

  const summary = page.locator('[data-testid="import-curl-summary"]');
  await page.click('[data-testid="import-curl-textarea"]');
  await page.keyboard.type('curl https://api.example.com/orders');

  // Time has not moved, so no wall-clock budget is being asserted here: the debounce *cannot*
  // have elapsed, however slowly the harness typed (P81 §3.3/§6 — same shape as grpc-request's).
  await expect(summary).toHaveText('');
  await page.clock.runFor(PREVIEW_DEBOUNCE_MS - 1);
  await expect(summary).toHaveText('');
  await page.clock.runFor(2);

  await expect(summary).toContainText('api.example.com/orders');
});

// ---- 3. A parse error disables Import ----

test('a parse error disables Import and shows the message', async ({ relaunch }) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: { collections: [], items: [] } },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await modeTab(page, 'api').click();
  await page.click('[data-testid="import-curl-start"]');
  await expect(page.locator('[data-testid="import-curl-dialog"]')).toBeVisible();

  await page.fill('[data-testid="import-curl-textarea"]', "curl 'unterminated");
  await expect(page.locator('[data-testid="import-curl-error"]')).toContainText('quoted string');
  await expect(page.locator('[data-testid="import-curl-submit"]')).toBeDisabled();
});

// ---- 4. Copy as curl, no secrets ----

test('copy as curl with no secrets never reveals or sends anything', async ({ relaunch }) => {
  const ENV = { id: 'env-1', name: 'Prod', sortOrder: 0, isActive: true };
  const HOST_VAR = {
    id: 'var-host',
    scope: 'environment',
    ownerId: 'env-1',
    name: 'host',
    value: 'api.example.com',
    isSecret: false,
    sortOrder: 0,
  };
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: { collections: [], items: [] } },
    { channel: IPC.variablesListEnvironments, response: [ENV] },
    {
      channel: IPC.variablesList,
      args: { scope: 'environment', ownerId: 'env-1' },
      response: [HOST_VAR],
    },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await openHttpRequestView(page);
  await page.fill('[data-testid="http-url"]', 'https://{{host}}/v1/x');
  await page.click('[data-testid="http-copy-as-curl"]');
  await expect(page.locator('[data-testid="copy-as-curl-dialog"]')).toBeVisible();

  const command = page.locator('[data-testid="copy-as-curl-command"]');
  await expect(command).toHaveValue(/api\.example\.com/);
  await expect(command).not.toHaveValue(/\{\{/);
  await expect(page.locator('[data-testid="copy-as-curl-strip"]')).toHaveCount(0);

  expect(control.log().filter((e) => e.channel === IPC.variablesReveal)).toHaveLength(0);
  expect(control.log().filter((e) => e.channel === IPC.httpSend)).toHaveLength(0);
  for (const entry of control.log().filter((e) => e.channel === IPC.tabsSave)) {
    expect(JSON.stringify(entry)).not.toContain('curl -L');
  }
});

// ---- 5. Copy as curl, with a secret — the gate ----

function secretRequestSetup(): { ENV: object; SECRET_VAR: object } {
  return {
    ENV: { id: 'env-1', name: 'Prod', sortOrder: 0, isActive: true },
    SECRET_VAR: {
      id: 'var-apikey',
      scope: 'environment',
      ownerId: 'env-1',
      name: 'apiKey',
      value: '',
      isSecret: true,
      sortOrder: 0,
    },
  };
}

test('copy as curl with a secret: masked by default, revealed by one gated action', async ({
  relaunch,
}) => {
  const { ENV, SECRET_VAR } = secretRequestSetup();
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: { collections: [], items: [] } },
    { channel: IPC.variablesListEnvironments, response: [ENV] },
    {
      channel: IPC.variablesList,
      args: { scope: 'environment', ownerId: 'env-1' },
      response: [SECRET_VAR],
    },
    {
      channel: IPC.variablesReveal,
      args: { variableId: 'var-apikey', confirmed: false },
      response: { value: 's3cr3t-key', error: null, outcome: 'revealed' },
    },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await openHttpRequestView(page);
  await page.fill('[data-testid="http-url"]', 'https://api.example.com/x');
  await page.click('[data-testid="http-request-pane-headers"]');
  const firstHeaderRow = page.locator('[data-testid="http-header-row"]').first();
  await firstHeaderRow.locator('[data-testid="http-header-name"]').fill('Authorization');
  await firstHeaderRow.locator('[data-testid="http-header-value"]').fill('Bearer {{apiKey}}');

  await page.click('[data-testid="http-copy-as-curl"]');
  await expect(page.locator('[data-testid="copy-as-curl-dialog"]')).toBeVisible();

  const command = page.locator('[data-testid="copy-as-curl-command"]');
  await expect(command).toHaveValue(/\{\{apiKey\}\}/);
  const strip = page.locator('[data-testid="copy-as-curl-strip"]');
  await expect(strip).toContainText('1 secret value is not shown');
  expect(control.log().filter((e) => e.channel === IPC.variablesReveal)).toHaveLength(0);

  await page.click('[data-testid="copy-as-curl-reveal"]');
  await expect(command).toHaveValue(/s3cr3t-key/);
  await expect(command).not.toHaveValue(/\{\{apiKey\}\}/);
  await expect(strip).toContainText('real secret values');
  expect(control.log().filter((e) => e.channel === IPC.variablesReveal)).toHaveLength(1);

  // Hide re-masks the command and the strip; the reveal button is back.
  await page.click('[data-testid="copy-as-curl-hide"]');
  await expect(command).toHaveValue(/\{\{apiKey\}\}/);
  await expect(command).not.toHaveValue(/s3cr3t-key/);
  await expect(strip).toContainText('1 secret value is not shown');
  await expect(page.locator('[data-testid="copy-as-curl-hide"]')).toHaveCount(0);
  await page.click('[data-testid="copy-as-curl-reveal"]');
  await expect(command).toHaveValue(/s3cr3t-key/);

  expect(control.log().filter((e) => e.channel === IPC.httpSend)).toHaveLength(0);
  for (const entry of control.log().filter((e) => e.channel === IPC.tabsSave)) {
    expect(JSON.stringify(entry)).not.toContain('s3cr3t-key');
  }
});

test('copy as curl with a secret: a cancelled reveal leaves the reference literal', async ({
  relaunch,
}) => {
  const { ENV, SECRET_VAR } = secretRequestSetup();
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: { collections: [], items: [] } },
    { channel: IPC.variablesListEnvironments, response: [ENV] },
    {
      channel: IPC.variablesList,
      args: { scope: 'environment', ownerId: 'env-1' },
      response: [SECRET_VAR],
    },
    {
      channel: IPC.variablesReveal,
      args: { variableId: 'var-apikey', confirmed: false },
      response: { value: null, error: null, outcome: 'cancelled' },
    },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpRequestView(page);
  await page.fill('[data-testid="http-url"]', 'https://api.example.com/x');
  await page.click('[data-testid="http-request-pane-headers"]');
  const firstHeaderRow = page.locator('[data-testid="http-header-row"]').first();
  await firstHeaderRow.locator('[data-testid="http-header-name"]').fill('Authorization');
  await firstHeaderRow.locator('[data-testid="http-header-value"]').fill('Bearer {{apiKey}}');

  await page.click('[data-testid="http-copy-as-curl"]');
  await page.click('[data-testid="copy-as-curl-reveal"]');

  const command = page.locator('[data-testid="copy-as-curl-command"]');
  await expect(command).toHaveValue(/\{\{apiKey\}\}/);
  await expect(page.locator('[data-testid="copy-as-curl-strip"]')).toContainText('apiKey');
});

// ---- 6. A dynamic value is frozen across the reveal ----

test('a dynamic value is frozen across the reveal, and stays distinct per occurrence', async ({
  relaunch,
}) => {
  const { ENV, SECRET_VAR } = secretRequestSetup();
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: { collections: [], items: [] } },
    { channel: IPC.variablesListEnvironments, response: [ENV] },
    {
      channel: IPC.variablesList,
      args: { scope: 'environment', ownerId: 'env-1' },
      response: [SECRET_VAR],
    },
    {
      channel: IPC.variablesReveal,
      args: { variableId: 'var-apikey', confirmed: false },
      response: { value: 's3cr3t-key', error: null, outcome: 'revealed' },
    },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpRequestView(page);
  await page.fill(
    '[data-testid="http-url"]',
    'https://api.example.com/orders/{{$guid}}?trace={{$guid}}',
  );
  await page.click('[data-testid="http-request-pane-headers"]');
  const firstHeaderRow = page.locator('[data-testid="http-header-row"]').first();
  await firstHeaderRow.locator('[data-testid="http-header-name"]').fill('Authorization');
  await firstHeaderRow.locator('[data-testid="http-header-value"]').fill('Bearer {{apiKey}}');

  await page.click('[data-testid="http-copy-as-curl"]');
  const command = page.locator('[data-testid="copy-as-curl-command"]');
  await expect(page.locator('[data-testid="copy-as-curl-dynamic-note"]')).toBeVisible();

  const before = await command.inputValue();
  const uuids = [...before.matchAll(new RegExp(UUID_RE, 'gi'))].map((m) => m[0]);
  expect(uuids).toHaveLength(2);
  expect(uuids[0]).not.toBe(uuids[1]);

  await page.click('[data-testid="copy-as-curl-reveal"]');
  await expect(command).toHaveValue(/s3cr3t-key/);
  const after = await command.inputValue();
  const uuidsAfter = [...after.matchAll(new RegExp(UUID_RE, 'gi'))].map((m) => m[0]);
  expect(uuidsAfter).toEqual(uuids);
});

// ---- 9. Import, send, copy as curl (contract: api-curl) ----

// Backend half: httpflow.TestCurlReplayMatchesSend sends the same request, replays the generated
// command with real curl and asserts the server saw the same request twice.
test('an imported curl sends as written, and copy-as-curl yields the contract command', async ({
  relaunch,
}) => {
  const origins = { http: 'http://flow.test' };
  const importText = contract<string>('api-curl', 'curl:import', { origins });
  const generated = contract<string>('api-curl', 'curl:generated', { origins });
  const sent = contract<{ method: string; url: string; headers: unknown[]; body: unknown }>(
    'api-curl',
    'args:HttpService.Send',
    { origins },
  );
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: { collections: [], items: [] } },
    { channel: IPC.httpSend, response: contract('api-curl', 'HttpService.Send', { origins }) },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await modeTab(page, 'api').click();
  await page.click('[data-testid="import-curl-start"]');
  await page.fill('[data-testid="import-curl-textarea"]', importText);
  await page.click('[data-testid="import-curl-submit"]');
  await expect(page.locator('[data-testid="http-method-chip"]')).toHaveText(sent.method);

  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');
  const sendCalls = control.log().filter((e) => e.channel === IPC.httpSend);
  expect(sendCalls).toHaveLength(1);
  expect(sendCalls[0].args).toMatchObject({
    method: sent.method,
    url: sent.url,
    headers: sent.headers,
    body: sent.body,
  });

  await page.click('[data-testid="http-copy-as-curl"]');
  await expect(page.locator('[data-testid="copy-as-curl-command"]')).toHaveValue(generated);
});
