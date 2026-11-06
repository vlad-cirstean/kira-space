import type { Locator } from '@playwright/test';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { modeTab, openHttpModeAndNewRequest } from './support/apiMode';
import { editorText } from './support/editorText';

// Three tests, one httpSend snapshot each (a channel with more than one snapshot matches on args,
// and the send's renderer-minted opId makes those args unmatchable across two sends in one test;
// the contained fix, a per-snapshot matchIgnoreKeys, is handed forward for whichever phase needs
// two responses on one channel).

test('Http request — send, view a JSON response, and Params-table <-> URL sync', async ({
  relaunch,
}) => {
  const RESPONSE_BODY = '{"id":1,"name":"Ada"}';
  const JSON_RESPONSE = {
    status: 200,
    statusText: 'OK',
    proto: 'HTTP/1.1',
    headers: [
      { name: 'Content-Type', value: 'application/json' },
      { name: 'X-Request-Id', value: 'abc123' },
    ],
    body: RESPONSE_BODY,
    bodyEncoding: 'utf8',
    bodyBytes: RESPONSE_BODY.length,
    bodyTruncated: false,
    elapsedMs: 42,
    finalUrl: 'https://api.example.com/users?limit=20',
    redirects: [],
  };

  const CONTROL: ControlSnapshot[] = [{ channel: IPC.httpSend, response: JSON_RESPONSE }];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(1);
  const view = page.locator('[data-testid="http-request-view"]');
  await expect(view).toBeVisible();

  // P22 D13 (F22): the request/response splitter draws a visible divider at rest — it used to be
  // 4px of nothing, with no line and no grab affordance until the pointer crossed it.
  await expect
    .poll(() => view.locator('.request-splitter').evaluate((el) => getComputedStyle(el).boxShadow))
    .not.toBe('none');

  // Typing in the URL updates the Params table without rewriting the URL (D9).
  await page.fill('[data-testid="http-url"]', 'https://api.example.com/users?limit=10');
  const paramRows = page.locator('[data-testid="http-param-row"]');
  const firstParamRow = paramRows.first();
  await expect(firstParamRow.locator('[data-testid="http-param-name"]')).toHaveValue('limit');
  await expect(firstParamRow.locator('[data-testid="http-param-value"]')).toHaveValue('10');
  await expect(page.locator('[data-testid="http-url"]')).toHaveValue(
    'https://api.example.com/users?limit=10',
  );

  // Editing the table rewrites the URL (D9's other half).
  await firstParamRow.locator('[data-testid="http-param-value"]').fill('20');
  await expect(page.locator('[data-testid="http-url"]')).toHaveValue(
    'https://api.example.com/users?limit=20',
  );

  // A header the builder shows should reach the send args verbatim.
  await page.click('[data-testid="http-request-pane-headers"]');
  const firstHeaderRow = page.locator('[data-testid="http-header-row"]').first();
  await firstHeaderRow.locator('[data-testid="http-header-name"]').fill('Accept');
  await firstHeaderRow.locator('[data-testid="http-header-value"]').fill('application/json');

  await page.click('[data-testid="http-send"]');

  const status = page.locator('[data-testid="http-status"]');
  await expect(status).toContainText('200');
  await expect(status).toHaveClass(/ok/);

  // The request's own args carried the method/URL/headers the builder shows.
  const httpSendCalls = control.log().filter((e) => e.channel === IPC.httpSend);
  expect(httpSendCalls).toHaveLength(1);
  expect(httpSendCalls[0].args).toMatchObject({
    method: 'GET',
    url: 'https://api.example.com/users?limit=20',
    headers: [{ name: 'Accept', value: 'application/json' }],
    body: { mode: 'none' },
  });

  // Opening a tab persists it.
  expect(control.log().some((e) => e.channel === IPC.tabsSave)).toBe(true);

  // Pretty (default) shows the indented form; Raw shows the compact bytes exactly as sent.
  const bodyEditor = page.locator('[data-testid="http-response-pane"] .response-body');
  await expect(bodyEditor.locator('.monaco-host')).toBeVisible();
  expect(await editorText(bodyEditor)).toBe('{\n  "id": 1,\n  "name": "Ada"\n}');

  await page.click('[data-testid="http-response-view-raw"]');
  expect(await editorText(bodyEditor)).toBe(RESPONSE_BODY);

  // The Headers pane shows a known header row.
  await page.click('[data-testid="http-response-pane-headers"]');
  const responseHeaders = page.locator('[data-testid="http-response-headers"]');
  await expect(responseHeaders).toContainText('Content-Type');
  await expect(responseHeaders).toContainText('application/json');
});

// Finding 16: buildQuery used to encodeURIComponent a query param whole, including any literal
// {{name}} reference inside it — turning it into %7B%7Bname%7D%7D, a form neither substitution
// engine recognises any more.
test('Http request — adding a query param leaves an existing {{variable}} reference untouched', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: [] });

  await openHttpModeAndNewRequest(page);
  await page.fill('[data-testid="http-url"]', 'https://api.example.com/orders?region={{region}}');

  const paramRows = page.locator('[data-testid="http-param-row"]');
  await expect(paramRows.first().locator('[data-testid="http-param-name"]')).toHaveValue('region');
  await expect(paramRows.first().locator('[data-testid="http-param-value"]')).toHaveValue(
    '{{region}}',
  );

  // Add a second param via the table's own trailing blank row.
  const trailing = paramRows.nth(1);
  await trailing.locator('[data-testid="http-param-name"]').fill('limit');
  await trailing.locator('[data-testid="http-param-value"]').fill('10');

  // The existing {{region}} reference must survive un-mangled, not %7B%7Bregion%7D%7D.
  await expect(page.locator('[data-testid="http-url"]')).toHaveValue(
    'https://api.example.com/orders?region={{region}}&limit=10',
  );
});

test('Http request — a 404 shows its own hint', async ({ relaunch }) => {
  const NOT_FOUND_RESPONSE = {
    status: 404,
    statusText: 'Not Found',
    proto: 'HTTP/1.1',
    headers: [],
    body: '',
    bodyEncoding: 'utf8',
    bodyBytes: 0,
    bodyTruncated: false,
    elapsedMs: 5,
    finalUrl: 'https://api.example.com/missing',
    redirects: [],
  };
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.httpSend, response: NOT_FOUND_RESPONSE }];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  await page.fill('[data-testid="http-url"]', 'https://api.example.com/missing');
  await page.click('[data-testid="http-send"]');

  const status = page.locator('[data-testid="http-status"]');
  await expect(status).toContainText('404');
  await expect(status).toHaveClass(/err/);
  // P28 D1: the code's meaning is the chip's tooltip now, not a standing caption below the row.
  await status.hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(
    /no resource at this URL/,
  );
});

// A header's value cell now completes from a vocabulary keyed by the row's own name —
// a whole-value entry for Content-Type (replaces the field) and a prefix entry for Authorization
// (inserts `Bearer ` and leaves the caret after it, so {{variable}} completion can take over for
// the credential itself).
test('Http request — a header value completes from its own name', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: [] });

  await openHttpModeAndNewRequest(page);
  await page.click('[data-testid="http-request-pane-headers"]');
  const rows = page.locator('[data-testid="http-header-row"]');
  const suggestions = page.locator('.autocomplete-suggestions li');

  const firstRow = rows.first();
  await firstRow.locator('[data-testid="http-header-name"]').fill('Content-Type');
  const firstValue = firstRow.locator('[data-testid="http-header-value"]');
  await firstValue.click();
  await firstValue.pressSequentially('appl');
  await expect(suggestions.filter({ hasText: 'application/json' })).toBeVisible({
    timeout: 5_000,
  });
  await suggestions.filter({ hasText: 'application/json' }).click();
  await expect(firstValue).toHaveValue('application/json');

  const secondRow = rows.nth(1);
  await secondRow.locator('[data-testid="http-header-name"]').fill('Authorization');
  const secondValue = secondRow.locator('[data-testid="http-header-value"]');
  await secondValue.click();
  await secondValue.pressSequentially('Bea');
  await expect(suggestions.filter({ hasText: 'Bearer' })).toBeVisible({ timeout: 5_000 });
  await suggestions.filter({ hasText: 'Bearer' }).click();
  await expect(secondValue).toHaveValue('Bearer ');
  // The caret lands right after the inserted space — typing `{{` there opens the {{variable}}
  // source, not a second round of the Authorization prefix vocabulary. templateToken only opens
  // once there is a non-empty word inside the reference (api-ui-consistency.spec.ts's own rule).
  await secondValue.pressSequentially('{{');
  await expect(secondValue).toHaveValue('Bearer {{}}');
  // P28 D15(b): `fake.` is the only dynamic vocabulary offered now — the Postman `$name`
  // spellings still resolve wherever they are already stored, but are no longer suggested.
  await secondValue.pressSequentially('fake.string');
  await expect(suggestions.filter({ hasText: 'fake.string.uuid' })).toBeVisible({ timeout: 5_000 });
});

test('Http request — restore from saved state, no reconnect gate', async ({ relaunch }) => {
  const RESTORED_TAB = {
    id: 'tab-restore-1',
    connectionId: null,
    path: 'request',
    kind: 'http-request',
    order: 0,
    active: true,
    state: {
      method: 'POST',
      url: 'https://api.example.com/widgets',
      headers: [{ name: 'Accept', value: 'application/json', enabled: true }],
      bodyMode: 'json',
      body: '{"name":"gizmo"}',
      requestPane: 'body',
      responsePane: 'body',
      responseView: 'pretty',
      requestPaneHeight: 0,
    },
  };
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.tabsList, response: [RESTORED_TAB] }];
  const { window: page } = await relaunch({ control: CONTROL });

  // hydrateTabs derives the boot mode from the restored active tab's own kind.
  await expect(modeTab(page, 'api')).toHaveClass(/is-active/);
  const view = page.locator('[data-testid="http-request-view"]');
  await expect(view).toBeVisible();

  // P17 D18: the method select is an app-drawn button now, not a native <select>.
  await expect(page.locator('[data-testid="http-method-select"]')).toHaveAttribute(
    'data-value',
    'POST',
  );
  await expect(page.locator('[data-testid="http-url"]')).toHaveValue(
    'https://api.example.com/widgets',
  );
  // requestPane restored to 'body' — the JSON body shows immediately, byte-identical to what was
  // saved (no beautify is ever applied on restore).
  const bodyEditor = page.locator('[data-testid="http-request-pane"]');
  await expect(bodyEditor.locator('.monaco-host')).toBeVisible();
  expect(await editorText(bodyEditor)).toBe('{"name":"gizmo"}');

  await page.click('[data-testid="http-request-pane-headers"]');
  const headerRow = page.locator('[data-testid="http-header-row"]').first();
  await expect(headerRow.locator('[data-testid="http-header-name"]')).toHaveValue('Accept');
  await expect(headerRow.locator('[data-testid="http-header-value"]')).toHaveValue(
    'application/json',
  );

  // D6: no reconnect gate (there is nothing to reconnect) and no response content — a restored
  // tab never had a send in this session.
  await expect(view).not.toContainText('Reconnect');
  await expect(page.locator('[data-testid="http-status"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="http-response-pane"]')).toContainText(
    'Send a request to see the response',
  );
});

// P22b D6/D7: the description toggle reveals a description cell on every configurable row —
// headers, params, urlencoded and form-data — off by default (a fourth AutocompleteField-width
// column is unusable for the majority of rows that have none).
test('Http request — the description toggle reveals a description cell on every row kind', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: [] });
  await openHttpModeAndNewRequest(page);

  await expect(page.locator('[data-testid="http-header-description"]')).toHaveCount(0);
  await page.click('[data-testid="http-field-descriptions-toggle"]');

  // Params: needs a real row, which only exists once the URL carries a query string (D9's
  // URL-is-authoritative design — F10's own reason there is no persisted params array at all).
  await page.fill('[data-testid="http-url"]', 'https://api.example.com/x?limit=10');
  const paramDescription = page.locator('[data-testid="http-param-description"]').first();
  await expect(paramDescription).toBeVisible();
  await paramDescription.fill('page size');
  await expect(paramDescription).toHaveValue('page size');

  // Headers.
  await page.click('[data-testid="http-request-pane-headers"]');
  const headerDescription = page.locator('[data-testid="http-header-description"]').first();
  await expect(headerDescription).toBeVisible();
  await headerDescription.fill('bearer token');
  await expect(headerDescription).toHaveValue('bearer token');

  // Urlencoded body.
  await page.click('[data-testid="http-request-pane-body"]');
  await page.click('[data-testid="http-body-mode-urlencoded"]');
  const urlencodedDescription = page.locator('[data-testid="http-urlencoded-description"]').first();
  await expect(urlencodedDescription).toBeVisible();
  await urlencodedDescription.fill('form field');
  await expect(urlencodedDescription).toHaveValue('form field');

  // Form-data body.
  await page.click('[data-testid="http-body-mode-formdata"]');
  const formdataDescription = page.locator('[data-testid="http-formdata-description"]').first();
  await expect(formdataDescription).toBeVisible();
  await formdataDescription.fill('upload field');
  await expect(formdataDescription).toHaveValue('upload field');
});

// A value typed into a description cell survives a tab reload — asserted the same way this file's
// own "restore from saved state" test above proves persistence for every other field: a tab
// restored with the description already in its saved state renders it immediately, with no
// further interaction.
test('Http request — a header description survives a tab reload', async ({ relaunch }) => {
  const RESTORED_TAB = {
    id: 'tab-restore-desc',
    connectionId: null,
    path: 'request',
    kind: 'http-request',
    order: 0,
    active: true,
    state: {
      method: 'GET',
      url: 'https://api.example.com/widgets',
      headers: [
        {
          name: 'Authorization',
          value: 'Bearer {{token}}',
          enabled: true,
          description: 'auth token',
        },
      ],
      bodyMode: 'none',
      body: '',
      requestPane: 'headers',
      responsePane: 'body',
      responseView: 'pretty',
      requestPaneHeight: 0,
      fieldDescriptions: true,
    },
  };
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.tabsList, response: [RESTORED_TAB] }];
  const { window: page } = await relaunch({ control: CONTROL });

  const headerRow = page.locator('[data-testid="http-header-row"]').first();
  await expect(headerRow.locator('[data-testid="http-header-description"]')).toHaveValue(
    'auth token',
  );
});

// The trailing blank row IS the add affordance (FieldRowsTable.vue's own displayRows comment), so
// the row a user needs next is the one that appears BELOW the one they just filled in — off the
// fold on any table long enough to scroll. Guarded on growth only: deleting a row must never move
// the scroll position.
test('Http request — a new header row scrolls into view when it appears (D16)', async ({
  relaunch,
}) => {
  const headers = Array.from({ length: 30 }, (_, i) => ({
    name: `X-Header-${i}`,
    value: `v${i}`,
    enabled: true,
  }));
  const RESTORED_TAB = {
    id: 'tab-scroll-1',
    connectionId: null,
    path: 'request',
    kind: 'http-request',
    order: 0,
    active: true,
    state: {
      method: 'GET',
      url: '',
      headers,
      bodyMode: 'none',
      body: '',
      requestPane: 'headers',
      responsePane: 'body',
      responseView: 'pretty',
      requestPaneHeight: 0,
    },
  };
  const { window: page } = await relaunch({
    control: [{ channel: IPC.tabsList, response: [RESTORED_TAB] }],
  });

  const table = page.locator('[data-testid="http-headers-table"]');
  await expect(table).toBeVisible();
  const rows = table.locator('[data-testid="http-header-row"]');
  await expect(rows).toHaveCount(31); // 30 real rows plus the trailing blank

  const trailing = rows.last();
  await trailing.locator('[data-testid="http-header-name"]').fill('X-New');
  await expect(rows).toHaveCount(32);

  const newTrailing = rows.last();
  const tableBox = await table.boundingBox();
  const rowBox = await newTrailing.boundingBox();
  if (!tableBox || !rowBox) throw new Error('table/row has no box');
  // "inside the scroller's client rect": the new row's own box is fully within the container's.
  expect(rowBox.y).toBeGreaterThanOrEqual(tableBox.y);
  expect(rowBox.y + rowBox.height).toBeLessThanOrEqual(tableBox.y + tableBox.height + 1);
});

// The growth guard (the reasoning: "without it, deleting the second row of forty would
// jump to the fortieth"): removing a row is a shrink, and must never scroll anywhere. Scrolled to
// the top, removing a row further down the (still off-screen) list must leave the top in view —
// deliberately not reusing the scroll-into-view test above, whose own scroll-to-bottom state
// would fold a real browser reflow (removing DOM above the viewport shifts scrollTop on its own)
// into the same assertion as the watcher's own behaviour.
test('Http request — deleting a header row does not scroll (D16 guard)', async ({ relaunch }) => {
  const headers = Array.from({ length: 30 }, (_, i) => ({
    name: `X-Header-${i}`,
    value: `v${i}`,
    enabled: true,
  }));
  const RESTORED_TAB = {
    id: 'tab-scroll-2',
    connectionId: null,
    path: 'request',
    kind: 'http-request',
    order: 0,
    active: true,
    state: {
      method: 'GET',
      url: '',
      headers,
      bodyMode: 'none',
      body: '',
      requestPane: 'headers',
      responsePane: 'body',
      responseView: 'pretty',
      requestPaneHeight: 0,
    },
  };
  const { window: page } = await relaunch({
    control: [{ channel: IPC.tabsList, response: [RESTORED_TAB] }],
  });

  const table = page.locator('[data-testid="http-headers-table"]');
  await expect(table).toBeVisible();
  await expect(table.evaluate((el) => el.scrollTop)).resolves.toBe(0);

  const rows = table.locator('[data-testid="http-header-row"]');
  // A plain `.click()` would first scroll the (currently off-screen) target into view — an
  // artifact of the test driver, not of the app — which is exactly the browser-reflow noise this
  // test exists to avoid. Dispatching the click via JS clicks the real button with no scroll.
  await rows
    .nth(20)
    .locator('[data-testid="http-header-remove"]')
    .evaluate((el: HTMLElement) => el.click());
  await expect(rows).toHaveCount(30);
  await expect(table.evaluate((el) => el.scrollTop)).resolves.toBe(0);
});

// The claim, stated as an assertion — toggling incognito then sending
// carries `incognito: true` on the httpSend args, and no tabsSave call ever carries this tab, not
// even the one the toggle itself fires (setIncognito's own on-transition flush) to drop the
// tab's existing row immediately.
test('Http request — an incognito tab sends with incognito:true and is never in a tabsSave', async ({
  relaunch,
}) => {
  const RESPONSE_BODY = '{"ok":true}';
  const JSON_RESPONSE = {
    status: 200,
    statusText: 'OK',
    proto: 'HTTP/1.1',
    headers: [],
    body: RESPONSE_BODY,
    bodyEncoding: 'utf8',
    bodyBytes: RESPONSE_BODY.length,
    bodyTruncated: false,
    elapsedMs: 5,
    finalUrl: 'https://api.example.com/ping',
    redirects: [],
  };
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.httpSend, response: JSON_RESPONSE }];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  const tabId = await page.locator('[data-testid="tab"]').first().getAttribute('data-tab-id');
  await page.fill('[data-testid="http-url"]', 'https://api.example.com/ping');

  // Everything up to here is ordinary, persisted tab activity — the assertion below is scoped to
  // what happens *after* the toggle (the "prospective, not retroactive" rule), not to the tab's
  // whole history in this test.
  // Wait out the URL edit's debounced save so it cannot land after the toggle and count as one.
  await expect
    .poll(() =>
      control
        .log()
        .some(
          (e) =>
            e.channel === IPC.tabsSave && JSON.stringify(e.args).includes('api.example.com/ping'),
        ),
    )
    .toBe(true);
  const savesBeforeToggle = control.log().filter((e) => e.channel === IPC.tabsSave).length;

  await page.click('[data-testid="http-incognito-toggle"]');
  await expect(page.locator('[data-testid="http-incognito-chip"]')).toBeVisible();

  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');

  const sendCalls = control.log().filter((e) => e.channel === IPC.httpSend);
  expect(sendCalls).toHaveLength(1);
  expect(sendCalls[0].args).toMatchObject({ incognito: true });

  // Not "no tabsSave at all" — turning incognito on itself fires one immediately (setIncognito's
  // own on-transition flush) to drop the tab's existing row right away. The claim is that none of
  // those calls, from the toggle on, ever carries this tab.
  const savesAfterToggle = control
    .log()
    .filter((e) => e.channel === IPC.tabsSave)
    .slice(savesBeforeToggle);
  expect(savesAfterToggle.length).toBeGreaterThan(0);
  for (const save of savesAfterToggle) {
    const args = save.args as { tabs: { id: string }[] };
    expect(args.tabs.some((t) => t.id === tabId)).toBe(false);
  }
});

// P90 §6.3: turning off Inherit on one leaf in the Settings segment and changing it must reach
// httpSend's own `options` carrying exactly that one leaf — buildSettingsWire drops every null
// (still-inherited) leaf rather than sending all seven.
test('Http request — a Settings-segment override reaches the wire as the one leaf that changed', async ({
  relaunch,
}) => {
  const RESPONSE_BODY = '{"ok":true}';
  const JSON_RESPONSE = {
    status: 200,
    statusText: 'OK',
    proto: 'HTTP/1.1',
    headers: [],
    body: RESPONSE_BODY,
    bodyEncoding: 'utf8',
    bodyBytes: RESPONSE_BODY.length,
    bodyTruncated: false,
    elapsedMs: 5,
    finalUrl: 'https://api.example.com/ping',
    redirects: [],
  };
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.httpSend, response: JSON_RESPONSE }];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  await page.fill('[data-testid="http-url"]', 'https://api.example.com/ping');
  await page.click('[data-testid="http-request-pane-settings"]');

  // Unchecking Inherit stages the current global value (true) first — a no-op change on its own —
  // then the checkbox itself flips it to false, the one leaf this request overrides.
  await page.click('[data-testid="http-settings-followRedirects-inherit"]');
  await page.click('[data-testid="http-settings-followRedirects"]');

  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');

  const sendCalls = control.log().filter((e) => e.channel === IPC.httpSend);
  expect(sendCalls).toHaveLength(1);
  const args = sendCalls[0].args as { options: unknown };
  expect(args.options).toEqual({ followRedirects: false });
});

// P90 §6.3: the override staged above is tab state — it must come back exactly as saved, with the
// segment's own count badge reflecting it, with no further interaction.
test('Http request — a Settings-segment override survives a tab restore', async ({ relaunch }) => {
  const RESTORED_TAB = {
    id: 'tab-settings-restore',
    connectionId: null,
    path: 'request',
    kind: 'http-request',
    order: 0,
    active: true,
    state: {
      method: 'GET',
      url: 'https://api.example.com/ping',
      headers: [],
      bodyMode: 'none',
      body: '',
      requestPane: 'settings',
      responsePane: 'body',
      responseView: 'pretty',
      requestPaneHeight: 0,
      settings: {
        httpVersion: null,
        requestTimeoutMs: null,
        maxResponseMb: null,
        sslVerify: null,
        followRedirects: false,
        maxRedirects: null,
        disableCookieJar: null,
      },
    },
  };
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.tabsList, response: [RESTORED_TAB] }];
  const { window: page } = await relaunch({ control: CONTROL });

  await expect(page.locator('[data-testid="http-request-pane-settings"]')).toContainText(
    /Settings\s*1/,
  );
  await expect(
    page.locator('[data-testid="http-settings-followRedirects-inherit"]'),
  ).not.toBeChecked();
  await expect(page.locator('[data-testid="http-settings-followRedirects"]')).not.toBeChecked();
});

// P90 §6.3: sent/received cookies from a mocked response, grouped and rendering their attributes.
test('Http request — the response Cookies tab renders sent and received cookies', async ({
  relaunch,
}) => {
  const JSON_RESPONSE = {
    status: 200,
    statusText: 'OK',
    proto: 'HTTP/1.1',
    headers: [],
    body: '',
    bodyEncoding: 'utf8',
    bodyBytes: 0,
    bodyTruncated: false,
    elapsedMs: 5,
    finalUrl: 'https://api.example.com/me',
    redirects: [],
    sentCookies: [
      {
        name: 'session',
        value: 'abc123',
        domain: '',
        path: '',
        expires: '',
        maxAge: 0,
        secure: false,
        httpOnly: false,
        sameSite: '',
        hop: 0,
      },
    ],
    receivedCookies: [
      {
        name: 'csrf',
        value: 'xyz789',
        domain: 'api.example.com',
        path: '/',
        expires: '',
        maxAge: 0,
        secure: true,
        httpOnly: true,
        sameSite: 'lax',
        hop: 0,
      },
    ],
  };
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.httpSend, response: JSON_RESPONSE }];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  await page.fill('[data-testid="http-url"]', 'https://api.example.com/me');
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');

  await page.click('[data-testid="http-response-pane-cookies"]');
  const cookies = page.locator('[data-testid="http-response-cookies"]');
  await expect(cookies).toContainText('session');
  await expect(cookies).toContainText('abc123');
  await expect(cookies).toContainText('csrf');
  await expect(cookies).toContainText('xyz789');
  await expect(cookies).toContainText('HttpOnly');
});

// P90 §6.3: with disableCookieJar at its global default (true — the jar is off), the request
// Cookies tab shows the jar-off empty state and never calls httpCookies.
test('Http request — the request Cookies tab shows the jar-off state by default', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: [] });

  await openHttpModeAndNewRequest(page);
  await page.fill('[data-testid="http-url"]', 'https://api.example.com/ping');
  await page.click('[data-testid="http-request-pane-cookies"]');

  await expect(page.locator('[data-testid="http-cookies-jar-off"]')).toBeVisible();
  expect(control.log().some((e) => e.channel === IPC.httpCookies)).toBe(false);
});

// P175 D2 (P168 F12): Remove deletes by the exact domain and path, so two same-name cookies on one
// URL are removed independently.
test('Http request — Remove sends the exact domain and path of the cookie', async ({
  relaunch,
}) => {
  const RESTORED_TAB = {
    id: 'tab-cookie-remove',
    connectionId: null,
    path: 'request',
    kind: 'http-request',
    order: 0,
    active: true,
    state: {
      method: 'GET',
      url: 'https://api.example.com/a/b',
      headers: [],
      bodyMode: 'none',
      body: '',
      requestPane: 'cookies',
      responsePane: 'body',
      responseView: 'pretty',
      requestPaneHeight: 0,
      settings: {
        httpVersion: null,
        requestTimeoutMs: null,
        maxResponseMb: null,
        sslVerify: null,
        followRedirects: null,
        maxRedirects: null,
        disableCookieJar: false,
      },
    },
  };
  const cookie = (path: string, value: string) => ({
    name: 'sid',
    value,
    domain: 'api.example.com',
    path,
    expires: '',
    maxAge: 0,
    secure: false,
    httpOnly: false,
    sameSite: '',
    hop: 0,
  });
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.tabsList, response: [RESTORED_TAB] },
    { channel: IPC.httpCookies, response: [cookie('/a', 'deep'), cookie('/', 'root')] },
    { channel: IPC.httpDeleteCookie, response: [cookie('/', 'root')] },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await expect(
    page.locator('[data-testid="http-cookies-remove-sid-api.example.com/a"]'),
  ).toBeVisible();
  await page.click('[data-testid="http-cookies-remove-sid-api.example.com/a"]');

  const calls = control.log().filter((e) => e.channel === IPC.httpDeleteCookie);
  expect(calls).toHaveLength(1);
  expect(calls[0].args).toMatchObject({ name: 'sid', domain: 'api.example.com', path: '/a' });
  await expect(page.locator('[data-testid^="http-cookies-remove-"]')).toHaveCount(1);
  await expect(
    page.locator('[data-testid="http-cookies-remove-sid-api.example.com/"]'),
  ).toBeVisible();
});

// The overlay's own Range client rects give exact per-character line boundaries, with no font
// metric hard-coded — reused both to size the fixture text (stay within the 4-row clamp) and to
// drive the click assertion below.
interface GrowMeasurement {
  textareaHeight: number;
  overlayHeight: number;
  scrollHeight: number;
  clientHeight: number;
  lineStarts: number[];
  lineCount: number;
}

function measureGrowField(wrap: Locator): Promise<GrowMeasurement> {
  return wrap.evaluate((wrapEl) => {
    const textarea = wrapEl.querySelector('textarea') as HTMLTextAreaElement;
    const overlay = wrapEl.querySelector('.highlight-overlay') as HTMLElement;
    const textareaRect = textarea.getBoundingClientRect();
    const overlayRect = overlay.getBoundingClientRect();

    const walker = document.createTreeWalker(overlay, NodeFilter.SHOW_TEXT);
    const textNodes: Text[] = [];
    for (let n = walker.nextNode(); n; n = walker.nextNode()) textNodes.push(n as Text);
    function locate(index: number): { node: Text; offset: number } {
      let remaining = index;
      for (const node of textNodes) {
        if (remaining <= node.length) return { node, offset: remaining };
        remaining -= node.length;
      }
      const last = textNodes[textNodes.length - 1];
      return { node: last, offset: last.length };
    }
    const text = textarea.value;
    const tops: number[] = [];
    for (let i = 0; i < text.length; i++) {
      const range = document.createRange();
      const loc = locate(i);
      const end = Math.min(loc.offset + 1, loc.node.length);
      range.setStart(loc.node, loc.offset);
      range.setEnd(loc.node, end);
      const rect = range.getClientRects()[0];
      tops.push(rect ? rect.top : Number.NaN);
    }
    const lineStarts = [0];
    for (let i = 1; i < tops.length; i++) {
      if (Math.abs(tops[i] - tops[i - 1]) > 1) lineStarts.push(i);
    }

    return {
      textareaHeight: textareaRect.height,
      overlayHeight: overlayRect.height,
      scrollHeight: textarea.scrollHeight,
      clientHeight: textarea.clientHeight,
      lineStarts,
      lineCount: lineStarts.length,
    };
  });
}

// The grow-field fix, the one assertion a browser is needed for. A textarea/overlay pair
// must wrap on the same boundaries and occupy the same box, the textarea must not
// scroll internally once its track has grown to fit, and a click near the end of the
// visible 3rd line must place the caret in that line, not the 2nd (the reported symptom). Line
// boundaries are read off the overlay's own Range client rects rather than assumed from a
// character count, so this does not encode a font metric. Content stays within the field's own
// 4-row clamp (grown incrementally, not a fixed length guessed from a font metric) — past the
// clamp the textarea is *meant* to scroll internally (by design), which is a different case from
// the bug this test guards (a box that scrolled before it finished growing, well under the clamp).
test('Http request — a grow field wraps its textarea and overlay onto the same lines', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: [] });
  await openHttpModeAndNewRequest(page);
  await page.click('[data-testid="http-request-pane-headers"]');

  const firstRow = page.locator('[data-testid="http-header-row"]').first();
  await firstRow.locator('[data-testid="http-header-name"]').fill('X-Long');
  const valueField = firstRow.locator('[data-testid="http-header-value"]');
  const wrap = valueField.locator('xpath=..');

  let length = 40;
  await valueField.fill('x'.repeat(length));
  await expect(wrap.locator('.highlight-overlay')).toBeVisible();
  let measurement = await measureGrowField(wrap);
  while (measurement.lineCount < 3 && length < 1000) {
    length += 20;
    await valueField.fill('x'.repeat(length));
    measurement = await measureGrowField(wrap);
  }

  expect(measurement.lineCount).toBeGreaterThanOrEqual(3);
  expect(measurement.lineCount).toBeLessThanOrEqual(4); // still inside the clamp — see comment above
  // Defect B: same box.
  expect(Math.abs(measurement.textareaHeight - measurement.overlayHeight)).toBeLessThanOrEqual(1);
  // Defect C: the box grew to fit — no internal scroll left over.
  expect(measurement.scrollHeight).toBeLessThanOrEqual(measurement.clientHeight);

  // The reported symptom: click near the end of the visible 3rd line (overlay line index 2) and
  // the caret must land within that line's own character range.
  const line3Start = measurement.lineStarts[2];
  const line4Start = measurement.lineStarts[3] as number | undefined;
  const nearEndIndex = (line4Start ?? measurement.lineStarts[2] + 1) - 1;

  const click = await wrap.evaluate((wrapEl, charIndex: number) => {
    const overlay = wrapEl.querySelector('.highlight-overlay') as HTMLElement;
    const walker = document.createTreeWalker(overlay, NodeFilter.SHOW_TEXT);
    const textNodes: Text[] = [];
    for (let n = walker.nextNode(); n; n = walker.nextNode()) textNodes.push(n as Text);
    let remaining = charIndex;
    let node = textNodes[0];
    let offset = 0;
    for (const n of textNodes) {
      if (remaining <= n.length) {
        node = n;
        offset = remaining;
        break;
      }
      remaining -= n.length;
    }
    const range = document.createRange();
    const end = Math.min(offset + 1, node.length);
    range.setStart(node, offset);
    range.setEnd(node, end);
    const rect = range.getClientRects()[0];
    return { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 };
  }, nearEndIndex);

  await page.mouse.click(click.x, click.y);

  const selectionStart = await valueField.evaluate(
    (el: HTMLTextAreaElement) => el.selectionStart ?? 0,
  );
  expect(selectionStart).toBeGreaterThanOrEqual(line3Start);
  if (line4Start !== undefined) expect(selectionStart).toBeLessThan(line4Start);
});
