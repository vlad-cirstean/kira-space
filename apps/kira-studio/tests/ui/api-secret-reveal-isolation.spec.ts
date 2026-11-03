import type { ControlSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { openHttpMode } from './support/apiMode';
import { collectionRow, variableRow } from './support/apiRows';
import { IPC } from './support/ipcChannels';

// revealedValues (state/variables.ts) is shared between the Variables
// tab and the Copy-as-curl dialog's own revealSecretValues loop (curl.ts), but only
// closeVariablesDialog used to clear it — closeCopyAsCurlDialog cleared only its own, separate
// revealedSecretValues map. A secret revealed via Copy-as-curl left a stale entry that the
// Variables tab trusted in place of its own re-auth gate, showing the secret with no reveal call
// — no re-auth — at all. Separately,
// revealedHistoryValues (VariableRow.vue's own per-row history popover) leaked across rows for the
// same reason: switching from one row's popover to another's unmounts the first via `v-if` without
// ever firing its own `@close`.
//
// Both are fixed by clearing the shared map at every point that can populate it (or supersede it),
// not only its own dialog's clean-close path — this pins that a reveal from one dialog/popover
// never lets a *different*, later one skip a real reveal call.

const NOW = '2026-01-01T00:00:00.000Z';

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

test('a secret revealed via Copy as curl does not skip re-auth in the later-opened Variables dialog', async ({
  relaunch,
}) => {
  const SECRET_VAR = {
    id: 'var-apikey',
    scope: 'collection',
    ownerId: 'col-1',
    name: 'apiKey',
    value: '',
    isSecret: true,
    sortOrder: 0,
  };

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: TREE },
    { channel: IPC.collectionsGetRequest, response: HEALTH_REQUEST },
    { channel: IPC.variablesListEnvironments, response: [] },
    {
      channel: IPC.variablesList,
      args: { scope: 'collection', ownerId: 'col-1' },
      response: [SECRET_VAR],
    },
    {
      channel: IPC.variablesReveal,
      args: { variableId: 'var-apikey', confirmed: false },
      response: { value: 's3cr3t-key', error: null, outcome: 'revealed' },
    },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await openHttpMode(page);
  await collectionRow(page, 'col-1').locator('[data-testid="tree-twisty"]').click();
  await page.locator('[data-testid="collection-row"][data-id="item-1"]').dblclick();
  await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();

  await page.click('[data-testid="http-request-pane-headers"]');
  const firstHeaderRow = page.locator('[data-testid="http-header-row"]').first();
  await firstHeaderRow.locator('[data-testid="http-header-name"]').fill('Authorization');
  await firstHeaderRow.locator('[data-testid="http-header-value"]').fill('Bearer {{apiKey}}');

  // Reveal the secret through Copy as curl, then close it — the shared revealedValues map now
  // holds var-apikey's plaintext, populated by a dialog other than the one about to check it.
  await page.click('[data-testid="http-copy-as-curl"]');
  await expect(page.locator('[data-testid="copy-as-curl-dialog"]')).toBeVisible();
  await page.click('[data-testid="copy-as-curl-reveal"]');
  await expect(page.locator('[data-testid="copy-as-curl-command"]')).toHaveValue(/s3cr3t-key/);
  expect(control.log().filter((e) => e.channel === IPC.variablesReveal)).toHaveLength(1);
  await page.click('[data-testid="copy-as-curl-dialog-close"]');
  await expect(page.locator('[data-testid="copy-as-curl-dialog"]')).toBeHidden();

  // Open the Variables tab for the same variable and reveal it — this must run its own real reveal
  // call, not silently trust the stale entry left by Copy as curl.
  await collectionRow(page, 'col-1').click({ button: 'right' });
  await page.click('[data-testid="menu-item-variables"]');
  await expect(page.locator('[data-testid="variables-dialog"]')).toBeVisible();

  await variableRow(page, 'var-apikey').locator('[data-testid="variable-reveal"]').click();
  await expect(
    variableRow(page, 'var-apikey').locator('[data-testid="variable-value"]'),
  ).toHaveText('s3cr3t-key');

  expect(control.log().filter((e) => e.channel === IPC.variablesReveal)).toHaveLength(2);
});

test("switching the history popover to a different row clears the previous row's revealed value", async ({
  relaunch,
}) => {
  const VAR_A = {
    id: 'var-a',
    scope: 'collection',
    ownerId: 'col-1',
    name: 'tokenA',
    value: 'current-a',
    isSecret: false,
    sortOrder: 0,
  };
  const VAR_B = {
    id: 'var-b',
    scope: 'collection',
    ownerId: 'col-1',
    name: 'tokenB',
    value: 'current-b',
    isSecret: false,
    sortOrder: 1,
  };
  const HIST_A = {
    id: 'hist-a-1',
    variableId: 'var-a',
    value: '',
    isSecret: true,
    recordedAt: NOW,
  };
  const HIST_B = {
    id: 'hist-b-1',
    variableId: 'var-b',
    value: '',
    isSecret: true,
    recordedAt: NOW,
  };

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: TREE },
    {
      channel: IPC.variablesList,
      args: { scope: 'collection', ownerId: 'col-1' },
      response: [VAR_A, VAR_B],
    },
    { channel: IPC.variablesHistory, args: { variableId: 'var-a' }, response: [HIST_A] },
    { channel: IPC.variablesHistory, args: { variableId: 'var-b' }, response: [HIST_B] },
    {
      channel: IPC.variablesRevealHistory,
      args: { historyId: 'hist-a-1', confirmed: false },
      response: { value: 'old-a-secret', error: null, outcome: 'revealed' },
    },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpMode(page);
  await collectionRow(page, 'col-1').click({ button: 'right' });
  await page.click('[data-testid="menu-item-variables"]');
  await expect(page.locator('[data-testid="variables-dialog"]')).toBeVisible();

  // Reveal row A's one history entry.
  await variableRow(page, 'var-a').locator('[data-testid="variable-history"]').click();
  await expect(page.locator('[data-testid="variable-history-entry"]')).toBeVisible();
  await page.click('[data-testid="variable-history-reveal"]');
  await expect(page.locator('[data-testid="variable-history-value"]')).toHaveText('old-a-secret');

  // Switch straight to row B's popover — row A's own is unmounted by `v-if`, with no `@close`.
  // reka Popover's dismissable layer treats a pointer click outside as close-this-one-first, not
  // reach-the-target — a keyboard `focus()` + Enter dispatches a real `click` event straight at
  // row B's own button instead, bypassing that outside-pointer-down handling entirely.
  await variableRow(page, 'var-b').locator('[data-testid="variable-history"]').focus();
  await page.keyboard.press('Enter');
  await expect(page.locator('[data-testid="variable-history-entry"]')).toBeVisible();
  await expect(page.locator('[data-testid="variable-history-masked"]')).toBeVisible();

  // Reopening row A must show its entry masked again — the earlier reveal must not have survived
  // the switch away and back.
  await variableRow(page, 'var-a').locator('[data-testid="variable-history"]').focus();
  await page.keyboard.press('Enter');
  await expect(page.locator('[data-testid="variable-history-masked"]')).toBeVisible();
  await expect(page.locator('[data-testid="variable-history-value"]')).toHaveCount(0);
});

// P16 §5: the variables filter matches the row's `name` and nothing else — this is the test
// that pins §5's own invariant, that a filter box can never become an oracle a user probes a
// masked secret's plaintext with.
test('the variables filter matches a secret’s name, never its plaintext', async ({ relaunch }) => {
  const SECRET_VAR = {
    id: 'var-apikey',
    scope: 'collection',
    ownerId: 'col-1',
    name: 'apiKey',
    value: '',
    isSecret: true,
    sortOrder: 0,
  };
  const PLAIN_VAR = {
    id: 'var-region',
    scope: 'collection',
    ownerId: 'col-1',
    name: 'region',
    value: 'us-east-1',
    isSecret: false,
    sortOrder: 1,
  };
  // The mock backend's own record of what this secret decrypts to — never sent to the renderer
  // for an unrevealed row (D5's projection guarantee, SECRET_VAR.value above is ''), so this
  // string exists in this test only to prove the filter can't be used to guess it.
  const SECRET_PLAINTEXT = 's3cr3t-key';

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: TREE },
    {
      channel: IPC.variablesList,
      args: { scope: 'collection', ownerId: 'col-1' },
      response: [SECRET_VAR, PLAIN_VAR],
    },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpMode(page);
  await collectionRow(page, 'col-1').click({ button: 'right' });
  await page.click('[data-testid="menu-item-variables"]');
  await expect(page.locator('[data-testid="variables-dialog"]')).toBeVisible();

  const realRows = page.locator('[data-testid="variable-row"]');
  await expect(realRows).toHaveCount(2);

  // Typing the secret's own plaintext into the filter matches nothing — not the secret row (whose
  // name is "apiKey", not this string) and not the other row either.
  await page.fill('[data-testid="variables-filter"]', SECRET_PLAINTEXT);
  await expect(realRows).toHaveCount(0);

  // The variable's own *name* matches, as a name filter should.
  await page.fill('[data-testid="variables-filter"]', 'apiKey');
  await expect(realRows).toHaveCount(1);
  await expect(variableRow(page, 'var-apikey')).toBeVisible();

  // Clearing the filter restores every row.
  await page.fill('[data-testid="variables-filter"]', '');
  await expect(realRows).toHaveCount(2);
});
