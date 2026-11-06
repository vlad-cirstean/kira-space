import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { modeTab, openHttpRequestView } from './support/apiMode';

// `tests/ui` drives the real built bundle in real WebKit with both wire planes mocked, so it
// exercises the real lazy chunk load through a real dynamic import() — the one tier that proves
// the split works at runtime rather than only at build time. Own file (nothing appended to
// http-variables.spec.ts or the mixed parity spec).
//
// No mockRuntime.ts/ipcChannels.ts change is needed: the boot path's collectionsList/
// variablesList/variablesListEnvironments wildcards already cover a tab opened with no fixture at
// all, and httpSend is an existing channel.

const NOW = '2026-01-01T00:00:00.000Z';
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

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

// ---- 1. A dynamic reference is generated, and reaches the wire ----

test('a dynamic reference is generated, and reaches the wire', async ({ relaunch }) => {
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.httpSend, response: RESPONSE }];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await openHttpRequestView(page);
  await page.fill(
    '[data-testid="http-url"]',
    'https://api.example.com/orders/{{$guid}}?t={{$timestamp}}',
  );
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');

  const sendCalls = control.log().filter((e) => e.channel === IPC.httpSend);
  expect(sendCalls).toHaveLength(1);
  const url = (sendCalls[0].args as { url: string }).url;
  expect(url).not.toContain('{{');
  const match = url.match(/^https:\/\/api\.example\.com\/orders\/([0-9a-f-]{36})\?t=(\d{10})$/i);
  expect(match).not.toBeNull();
  expect(match?.[1]).toMatch(UUID_RE);
});

// ---- 2. Per-occurrence freshness (D3) ----

test('per-occurrence freshness: two {{$guid}}s in one send differ, and a second send differs again', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.httpSend, response: RESPONSE }];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await openHttpRequestView(page);
  await page.fill('[data-testid="http-url"]', 'https://api.example.com/echo');
  await page.click('[data-testid="http-request-pane-body"]');
  await page.click('[data-testid="http-body-mode-raw"]');
  const editor = page.locator('[data-testid="http-request-pane"]').locator('.view-lines');
  await editor.click();
  await page.keyboard.insertText('{{$guid}} {{$guid}}');

  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');

  const sendCalls = control.log().filter((e) => e.channel === IPC.httpSend);
  expect(sendCalls).toHaveLength(2);

  const allValues: string[] = [];
  for (const call of sendCalls) {
    const raw = (call.args as { body: { raw: string } }).body.raw;
    const [first, second] = raw.split(' ');
    expect(first).toMatch(UUID_RE);
    expect(second).toMatch(UUID_RE);
    expect(first).not.toBe(second);
    allValues.push(first, second);
  }
  // Fresh on every send *and* fresh per occurrence — all four values distinct.
  expect(new Set(allValues).size).toBe(4);
});

// ---- 3. The preview never generates (§0.3's invariant) ----

test('the preview never generates: a catalogued name is not a warning and stays literal', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.httpSend, response: RESPONSE }];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await openHttpRequestView(page);
  await page.fill('[data-testid="http-url"]', '{{$guid}}');

  // A catalogued $name is not shown as unresolved (D8) — it will resolve fine at send time.
  await expect(page.locator('[data-testid="http-unresolved-chip"]')).toHaveCount(0);

  // Typing elsewhere must not have generated anything or rewritten the URL field's own text.
  await page.click('[data-testid="http-request-pane-headers"]');
  const firstHeaderRow = page.locator('[data-testid="http-header-row"]').first();
  await firstHeaderRow.locator('[data-testid="http-header-name"]').fill('X-Probe');
  await expect(page.locator('[data-testid="http-url"]')).toHaveValue('{{$guid}}');
  expect(control.log().filter((e) => e.channel === IPC.httpSend)).toHaveLength(0);

  // An uncatalogued $name still warns, naming itself as an unknown dynamic value.
  await page.fill('[data-testid="http-url"]', '{{$nope}}');
  const chip = page.locator('[data-testid="http-unresolved-chip"]');
  await expect(chip).toContainText('1 unresolved');
  await chip.hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(
    /\$nope is an unknown dynamic value/,
  );
});

// ---- 4. The reference dialog lists the vocabulary with live samples ----

test('the reference dialog lists 58 names with live samples, regenerated on reopen', async ({
  relaunch,
}) => {
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

  const openDialog = async (): Promise<void> => {
    await page
      .locator('[data-testid="tree-background"]')
      .click({ button: 'right', position: { x: 10, y: 400 } });
    await page.click('[data-testid="menu-item-dynamic-values"]');
    await expect(page.locator('[data-testid="dynamic-values-dialog"]')).toBeVisible();
  };

  await openDialog();
  // P28 D15(b): the dialog teaches one vocabulary now — the 57 `fake.` names. The 58 Postman
  // `$name` aliases still resolve everywhere they are already stored, they are simply no longer
  // listed here or offered in autocomplete.
  const rows = page.locator('[data-testid="dynamic-values-fake-row"]');
  await expect(rows).toHaveCount(57);
  await expect(page.locator('[data-testid="dynamic-values-row"]')).toHaveCount(0);

  const emailRow = page.locator(
    '[data-testid="dynamic-values-fake-row"][data-name="fake.internet.email"]',
  );
  await expect(emailRow.locator('[data-testid="dynamic-values-sample"]')).toContainText('@');

  const guidRow = page.locator(
    '[data-testid="dynamic-values-fake-row"][data-name="fake.string.uuid"]',
  );
  const firstSample = await guidRow.locator('[data-testid="dynamic-values-sample"]').innerText();
  expect(firstSample).toMatch(UUID_RE);

  await page.click('[data-testid="dynamic-values-dialog-close"]');
  await expect(page.locator('[data-testid="dynamic-values-dialog"]')).toHaveCount(0);

  await openDialog();
  const secondSample = await guidRow.locator('[data-testid="dynamic-values-sample"]').innerText();
  expect(secondSample).toMatch(UUID_RE);
  expect(secondSample).not.toBe(firstSample);
});
