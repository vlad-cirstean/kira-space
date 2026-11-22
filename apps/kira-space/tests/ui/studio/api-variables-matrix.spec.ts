import { CHANNEL } from '@shared/protocol/events';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { calls } from '../support/ipcCalls';
import { IPC } from '../support/ipcChannels';
import { emitWailsEvent } from '../support/mockRuntime';
import { openHttpMode } from './support/apiMode';
import { matrixCell, matrixRow, openVariablesTab } from './support/apiRows';
import { acceptConfirm, cancelConfirm } from './support/dialogs';

// The Variables tab: a grid of names by files. Each test performs one step of the backend scenario
// and asserts the UI sends the exact EditVariables args the Go flow test applied.
// Backend half: apiflow.TestApiVariablesMatrix writes the api-variables-matrix contract.

const KEY = 'api-variables-matrix';
const uid = (n: number) => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`;
const STAGING = uid(1);
const COLLECTION = uid(2);
const LOCAL = uid(3);
const NOW = '2026-01-01T00:00:00Z';

const args = (step: string) => contract(KEY, `args:ApiConfigService.EditVariables#${step}`);
const view = (step: string) => contract(KEY, `EditVariables#${step}`);
const unlockArgs = (step: string) => contract(KEY, `args:ApiConfigService.UnlockVariables#${step}`);
const unlockResult = (step: string) => contract(KEY, `UnlockVariables#${step}`);

const TREE = {
  collections: [
    {
      id: COLLECTION,
      name: 'Orders API',
      file: 'orders-api.collection.yaml',
      sortOrder: 0,
      createdAt: NOW,
      updatedAt: NOW,
    },
  ],
  items: [],
};
const env = (id: string, name: string, order: number, isActive: boolean) => ({
  id,
  name,
  sortOrder: order,
  isActive,
  color: order === 0 ? 'green' : 'blue',
  description: '',
  file: `${name.toLowerCase()}.environment.yaml`,
});
const ENVIRONMENTS = [env(STAGING, 'Staging', 0, true), env(LOCAL, 'Local', 1, false)];

function row(
  scope: 'collection' | 'environment',
  ownerId: string,
  name: string,
  over: Record<string, unknown> = {},
) {
  return {
    id: `${ownerId}-${name}`,
    scope,
    ownerId,
    name,
    value: '',
    isSecret: false,
    secretSet: false,
    sortOrder: 0,
    description: '',
    ...over,
  };
}
const list = (ownerId: string, scope: string, rows: unknown[]): ControlSnapshot => ({
  channel: IPC.variablesList,
  args: { scope, ownerId },
  response: rows,
});

interface Rows {
  collection?: unknown[];
  staging?: unknown[];
  local?: unknown[];
}
const base = (rows: Rows): ControlSnapshot[] => [
  { channel: IPC.collectionsList, response: TREE },
  { channel: IPC.variablesListEnvironments, response: ENVIRONMENTS },
  list(COLLECTION, 'collection', rows.collection ?? []),
  list(STAGING, 'environment', rows.staging ?? []),
  list(LOCAL, 'environment', rows.local ?? []),
];

const BASE_URL = (scope: 'collection' | 'environment', owner: string) =>
  row(scope, owner, 'base_url', { value: 'https://api.example.com' });
const PAGE_SIZE = row('environment', STAGING, 'page_size', { value: '50' });

async function open(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  rows: Rows,
  extra: ControlSnapshot[] = [],
) {
  const app = await relaunch({ control: [...base(rows), ...extra] });
  await openHttpMode(app.window);
  await openVariablesTab(app.window);
  await expect(app.window.locator('[data-testid="variables-matrix"]')).toBeVisible();
  return app;
}

const edits = (control: Parameters<typeof calls>[0]) =>
  calls(control, IPC.apiConfigEditVariables).map((c) => c.args);

test('cell edit: Escape cancels with no call, Enter saves the value', async ({ relaunch }) => {
  const { window: page, control } = await open(relaunch, { staging: [PAGE_SIZE] }, [
    { channel: IPC.apiConfigEditVariables, args: args('set'), response: view('set') },
  ]);
  const cell = matrixCell(page, 'page_size', STAGING);
  await expect(cell).toContainText('50');
  await cell.click();
  const input = page.locator('[data-testid="matrix-cell-input"]');
  await input.fill('77');
  await input.press('Escape');
  await expect(input).toHaveCount(0);
  expect(edits(control)).toHaveLength(0);

  await cell.click();
  await input.fill('100');
  await input.press('Enter');
  await expect.poll(() => edits(control)).toEqual([args('set')]);
});

test('clear from the cell menu unsets the value', async ({ relaunch }) => {
  const { window: page, control } = await open(relaunch, { staging: [PAGE_SIZE] }, [
    { channel: IPC.apiConfigEditVariables, args: args('unset'), response: view('unset') },
  ]);
  await matrixCell(page, 'page_size', STAGING).click({ button: 'right' });
  await page.click('[data-testid="menu-item-clear"]');
  await expect.poll(() => edits(control)).toEqual([args('unset')]);
});

test('lock turns a plain variable into a secret', async ({ relaunch }) => {
  const { window: page, control } = await open(
    relaunch,
    { staging: [row('environment', STAGING, 'token', { value: 'abc' })] },
    [{ channel: IPC.apiConfigEditVariables, args: args('lock'), response: view('lock') }],
  );
  await page.click('[data-testid="matrix-secret-token"]');
  await expect.poll(() => edits(control)).toEqual([args('lock')]);
});

test('an unset secret cell takes a value, sealed by the backend', async ({ relaunch }) => {
  const { window: page, control } = await open(
    relaunch,
    { staging: [row('environment', STAGING, 'token', { isSecret: true })] },
    [{ channel: IPC.apiConfigEditVariables, args: args('setSecret'), response: view('setSecret') }],
  );
  await page.click(`[data-testid="matrix-secret-set-token-${STAGING}"]`);
  const input = page.locator('[data-testid="matrix-cell-input"]');
  await input.fill('sealed-token-2');
  await input.press('Enter');
  await expect.poll(() => edits(control)).toEqual([args('setSecret')]);
});

const unlocks = (control: Parameters<typeof calls>[0]) =>
  calls(control, IPC.apiConfigUnlockVariables).map((c) => c.args);

const UNLOCK_STAGING = {
  staging: [row('environment', STAGING, 'token', { isSecret: true, secretSet: true })],
};

test('unlock asks the danger confirm first, then goes through the reveal gate', async ({
  relaunch,
}) => {
  const { window: page, control } = await open(relaunch, UNLOCK_STAGING, [
    {
      channel: IPC.apiConfigUnlockVariables,
      args: unlockArgs('unlock'),
      response: unlockResult('unlock'),
    },
  ]);
  await page.click('[data-testid="matrix-secret-token"]');
  expect(unlocks(control)).toEqual([]);
  await acceptConfirm(page);
  await expect.poll(() => unlocks(control)).toEqual([unlockArgs('unlock')]);
  expect(edits(control)).toEqual([]);
});

test('unlock stops when the confirm is declined', async ({ relaunch }) => {
  const { window: page, control } = await open(relaunch, UNLOCK_STAGING, []);
  await page.click('[data-testid="matrix-secret-token"]');
  await cancelConfirm(page);
  expect(unlocks(control)).toEqual([]);
});

test('unlock stops when the OS prompt is declined', async ({ relaunch }) => {
  const { window: page, control } = await open(relaunch, UNLOCK_STAGING, [
    {
      channel: IPC.apiConfigUnlockVariables,
      args: unlockArgs('cancelled'),
      response: unlockResult('cancelled'),
    },
  ]);
  await page.click('[data-testid="matrix-secret-token"]');
  await acceptConfirm(page);
  await expect.poll(() => unlocks(control)).toEqual([unlockArgs('cancelled')]);
  await expect(page.locator('[data-testid="confirm-dialog"]')).toHaveCount(0);
});

test('rename renames the name in every file, then offers Find usages', async ({ relaunch }) => {
  const { window: page, control } = await open(
    relaunch,
    {
      collection: [BASE_URL('collection', COLLECTION)],
      staging: [BASE_URL('environment', STAGING)],
      local: [BASE_URL('environment', LOCAL)],
    },
    [{ channel: IPC.apiConfigEditVariables, args: args('rename'), response: view('rename') }],
  );
  await page.click('[data-testid="matrix-row-menu-base_url"]');
  await page.click('[data-testid="menu-item-rename"]');
  const input = page.locator('[data-testid="matrix-row-input"]');
  await input.fill('host');
  await input.press('Enter');
  await expect.poll(() => edits(control)).toEqual([args('rename')]);
  await expect(page.locator('[data-testid="variables-renamed"]')).toContainText('base_url');
  await expect(page.locator('[data-testid="variables-find-usages"]')).toBeVisible();
});

test('describe and share an environment variable with agents', async ({ relaunch }) => {
  const { window: page, control } = await open(
    relaunch,
    { staging: [row('environment', STAGING, 'host', { value: 'api.example.com' })] },
    [{ channel: IPC.apiConfigEditVariables, response: view('describeShare') }],
  );
  await page.click('[data-testid="matrix-row-menu-host"]');
  await page.click('[data-testid="menu-item-describe"]');
  const input = page.locator('[data-testid="matrix-row-input"]');
  await input.fill('API root');
  await input.press('Enter');
  await page.click('[data-testid="matrix-row-menu-host"]');
  // Keyboard: the submenu sits at the window edge, a mouse path to it crosses closing siblings.
  await page.locator('[data-testid="menu-item-share"]').focus();
  await page.keyboard.press('ArrowRight');
  await expect(page.locator(`[data-testid="menu-item-share-${STAGING}"]`)).toBeFocused();
  await page.keyboard.press('Enter');
  const sent = edits(control) as { edits: unknown[] }[];
  await expect.poll(() => edits(control)).toHaveLength(2);
  const want = args('describeShare') as { edits: unknown[] };
  expect(sent[0].edits[0]).toMatchObject(want.edits[0] as Record<string, unknown>);
  expect(sent[1].edits[0]).toMatchObject(want.edits[1] as Record<string, unknown>);
});

test('a new variable is added from the new-row inputs by keyboard', async ({ relaunch }) => {
  const { window: page, control } = await open(relaunch, { staging: [PAGE_SIZE] }, [
    { channel: IPC.apiConfigEditVariables, args: args('set'), response: view('set') },
  ]);
  await page.click('[data-testid="matrix-add"]');
  const name = page.locator('[data-testid="matrix-new-name"]');
  await expect(name).toBeFocused();
  await name.fill('page_size');
  await page.locator('[data-testid="matrix-new-value"]').fill('100');
  await page.locator('[data-testid="matrix-new-value"]').press('Enter');
  await expect.poll(() => edits(control)).toEqual([args('set')]);
  await expect(matrixRow(page, 'page_size')).toBeVisible();
});

test('the active environment switches from its column radio', async ({ relaunch }) => {
  const { window: page, control } = await open(relaunch, { staging: [PAGE_SIZE] }, [
    { channel: IPC.variablesSetActiveEnvironment, args: { id: LOCAL }, response: null },
  ]);
  await page.click(`[data-testid="matrix-activate-${LOCAL}"]`);
  await expect
    .poll(() => calls(control, IPC.variablesSetActiveEnvironment).map((c) => c.args))
    .toEqual([{ id: LOCAL }]);
});

test('narrow mode shows one column picked in the toolbar', async ({ relaunch }) => {
  const app = await open(relaunch, {
    collection: [BASE_URL('collection', COLLECTION)],
    staging: [PAGE_SIZE],
  });
  const page = app.window;
  await page.setViewportSize({ width: 900, height: 600 });
  await expect(page.locator('[data-testid="matrix-env-picker"]')).toBeVisible();
  await expect(page.locator(`[data-testid="matrix-col-${LOCAL}"]`)).toHaveCount(0);
  await page.click('[data-testid="matrix-env-picker"]');
  await page.locator('[data-testid="matrix-env-option"]', { hasText: 'Local' }).click();
  await expect(page.locator(`[data-testid="matrix-col-${LOCAL}"]`)).toBeVisible();
  await expect(page.locator(`[data-testid="matrix-col-${STAGING}"]`)).toHaveCount(0);
});

test('the request popover adds a missing variable to the active environment', async ({
  relaunch,
}) => {
  const ITEM = {
    id: 'item-1',
    collectionId: COLLECTION,
    parentId: null,
    kind: 'request',
    name: 'List',
    sortOrder: 0,
    method: 'GET',
    url: 'https://{{page_size}}.example.com',
    createdAt: NOW,
    updatedAt: NOW,
  };
  const app = await relaunch({
    control: [
      { channel: IPC.collectionsList, response: { ...TREE, items: [ITEM] } },
      {
        channel: IPC.collectionsGetRequest,
        response: {
          method: 'GET',
          url: ITEM.url,
          headers: [],
          bodyMode: 'none',
          body: '',
          code: '',
          codeLanguage: 'json',
          urlEncoded: [],
          formData: [],
          binaryFile: null,
        },
      },
      { channel: IPC.variablesListEnvironments, response: ENVIRONMENTS },
      list(COLLECTION, 'collection', []),
      list(STAGING, 'environment', []),
      list(LOCAL, 'environment', []),
      { channel: IPC.apiConfigEditVariables, args: args('set'), response: view('set') },
    ],
  });
  const page = app.window;
  await openHttpMode(page);
  await page
    .locator(`[data-testid="collection-row"][data-id="${COLLECTION}"]`)
    .locator('[data-testid="tree-twisty"]')
    .click();
  await page.locator('[data-testid="collection-row"][data-id="item-1"]').dblclick();
  await page.click('[data-testid="http-unresolved-chip"]');
  await expect(page.locator('[data-testid="request-var-page_size"]')).toHaveAttribute(
    'data-state',
    'missing',
  );
  await page.locator('[data-testid="request-var-fix-input"]').fill('100');
  await page.click('[data-testid="request-var-fix-add"]');
  await expect.poll(() => edits(app.control)).toEqual([args('set')]);
});

test('an import report lists unset secrets and opens the Variables tab', async ({ relaunch }) => {
  const importArgs = contract<{ paths: string[] }>(KEY, 'args:CollectionsService.Import');
  const app = await relaunch({
    control: [
      ...base({}),
      {
        channel: IPC.filesChooseOpenMany,
        response: {
          canceled: false,
          files: importArgs.paths.map((path) => ({ name: 'prod.json', path, size: 0 })),
        },
      },
      {
        channel: IPC.collectionsImport,
        args: importArgs,
        response: contract(KEY, 'CollectionsService.Import'),
      },
    ],
  });
  const page = app.window;
  await openHttpMode(page);
  await emitWailsEvent(page, CHANNEL.importPostman, undefined);
  await expect(page.locator('[data-testid="import-report-secrets"]')).toContainText(
    'client_secret',
  );
  await page.click('[data-testid="import-report-open-variables"]');
  await expect(page.locator('[data-testid="variables-matrix"]')).toBeVisible();
});

// 20 environments: each column keeps its minimum width, the whole grid scrolls sideways with the Name
// column pinned, and the footer counts agree in number.
const MIN_ENV_COL_PX = 176;
const envId = (i: number) => uid(100 + i);

function manyEnvironments(count: number, withoutToken: number, token = true): ControlSnapshot[] {
  const envs = Array.from({ length: count }, (_, i) =>
    env(envId(i), `Environment ${String(i + 1).padStart(2, '0')}`, i, i === 0),
  );
  return [
    { channel: IPC.collectionsList, response: TREE },
    { channel: IPC.variablesListEnvironments, response: envs },
    list(COLLECTION, 'collection', [BASE_URL('collection', COLLECTION)]),
    ...envs.map((e, i) =>
      list(
        e.id,
        'environment',
        i < withoutToken || !token
          ? []
          : [row('environment', e.id, 'token', { isSecret: true, secretSet: true })],
      ),
    ),
  ];
}

async function openMany(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  count: number,
  withoutToken: number,
  token = true,
) {
  const app = await relaunch({ control: manyEnvironments(count, withoutToken, token) });
  await openHttpMode(app.window);
  await openVariablesTab(app.window);
  await expect(app.window.locator('[data-testid="variables-matrix"]')).toBeVisible();
  return app.window;
}

test('20 environments keep a minimum column width and scroll sideways with Name pinned', async ({
  relaunch,
}) => {
  const page = await openMany(relaunch, 20, 0);
  const scroller = page.locator('[data-testid="variables-matrix-scroll"]');
  await expect(page.locator(`[data-testid="matrix-col-${envId(19)}"]`)).toBeAttached();
  for (let i = 0; i < 20; i++) {
    const box = await page.locator(`[data-testid="matrix-col-${envId(i)}"]`).boundingBox();
    expect(box?.width ?? 0).toBeGreaterThanOrEqual(MIN_ENV_COL_PX - 1);
  }
  const overflow = await scroller.evaluate((el) => el.scrollWidth - el.clientWidth);
  expect(overflow).toBeGreaterThan(0);

  const name = page.locator('[data-testid="matrix-name-token"]');
  const frame = await scroller.boundingBox();
  const before = await name.boundingBox();
  expect(before?.x).toBeCloseTo(frame?.x ?? -1, 0);
  await scroller.evaluate((el) => {
    el.scrollLeft = el.scrollWidth;
  });
  await expect.poll(async () => scroller.evaluate((el) => el.scrollLeft)).toBeGreaterThan(0);
  const after = await name.boundingBox();
  expect(after?.x).toBeCloseTo(frame?.x ?? -1, 0);
});

test('the footer counts agree in number', async ({ relaunch }) => {
  const one = await openMany(relaunch, 20, 1);
  await expect(one.locator('[data-testid="matrix-footer-missing"]')).toContainText(
    '1 value missing',
  );
  await expect(one.locator('[data-testid="matrix-footer-missing"]')).not.toContainText('values');
  await expect(one.locator('[data-testid="matrix-footer-counts"]')).toHaveText(
    '2 variables, 20 environments',
  );
});

test('the footer says 2 values missing', async ({ relaunch }) => {
  const page = await openMany(relaunch, 20, 2);
  await expect(page.locator('[data-testid="matrix-footer-missing"]')).toContainText(
    '2 values missing',
  );
});

test('one variable and one environment read in the singular', async ({ relaunch }) => {
  const page = await openMany(relaunch, 1, 0, false);
  await expect(page.locator('[data-testid="matrix-footer-counts"]')).toHaveText(
    '1 variable, 1 environment',
  );
});
