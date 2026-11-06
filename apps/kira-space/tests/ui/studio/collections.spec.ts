import { CHANNEL } from '@shared/protocol/events';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, test } from './fixtures';
import { openHttpMode } from './support/apiMode';
import { collectionRow } from './support/apiRows';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// P4 §6.3: four tests, one snapshot per channel (mockRuntime.ts: a channel with exactly one
// snapshot answers args-blind, which is what lets a test assert on a call's args afterwards
// without having to make them matchable up front).

// Located by row id rather than by label: a `hasText` filter would make "Orders" match the
// "Orders API" collection row too, and the id is what the row model actually keys on.

const NOW = '2026-01-01T00:00:00.000Z';

// One collection, one folder inside it, and two requests — one at the root, one in the folder.
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
      id: 'item-folder',
      collectionId: 'col-1',
      parentId: null,
      kind: 'folder',
      name: 'Orders',
      sortOrder: 0,
      method: '',
      url: '',
      createdAt: NOW,
      updatedAt: NOW,
    },
    {
      id: 'item-create',
      collectionId: 'col-1',
      parentId: 'item-folder',
      kind: 'request',
      name: 'Create order',
      sortOrder: 0,
      method: 'POST',
      url: 'https://api.example.com/v2/orders',
      createdAt: NOW,
      updatedAt: NOW,
    },
    {
      id: 'item-health',
      collectionId: 'col-1',
      parentId: null,
      kind: 'request',
      name: 'Health check',
      sortOrder: 1,
      method: 'GET',
      url: 'https://api.example.com/healthz',
      createdAt: NOW,
      updatedAt: NOW,
    },
  ],
};

const CREATE_ORDER_REQUEST = {
  method: 'POST',
  url: 'https://api.example.com/v2/orders',
  headers: [{ name: 'Content-Type', value: 'application/json', enabled: true }],
  bodyMode: 'code',
  body: '',
  code: '{"sku":"widget"}',
  codeLanguage: 'json',
  urlEncoded: [],
  formData: [],
  binaryFile: null,
};

test('collections — the tree renders and a request opens into the existing tab kind', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: TREE },
    { channel: IPC.collectionsGetRequest, response: CREATE_ORDER_REQUEST },
  ];
  const { window: page } = await relaunch({ control: CONTROL });
  await openHttpMode(page);

  // A collection renders collapsed until it is opened — nothing auto-expands, so the shape the
  // user left the tree in is the shape it comes back as.
  await expect(collectionRow(page, 'col-1')).toBeVisible();
  await expect(collectionRow(page, 'col-1')).toHaveText('Orders API');
  await expect(collectionRow(page, 'item-folder')).toHaveCount(0);

  await collectionRow(page, 'col-1').locator('[data-testid="tree-twisty"]').click();
  await expect(collectionRow(page, 'item-folder')).toBeVisible();
  await expect(collectionRow(page, 'item-health')).toBeVisible();
  // The folder is collapsed in turn, so the request inside it is not rendered yet.
  await expect(collectionRow(page, 'item-create')).toHaveCount(0);

  // Depths come from the row model, not from DOM nesting — every row is a sibling in the virtual
  // list, indented by padding.
  await expect(collectionRow(page, 'col-1')).toHaveAttribute('data-depth', '0');
  await expect(collectionRow(page, 'item-health')).toHaveAttribute('data-depth', '1');

  // A request row carries its method chip; a folder and a collection do not.
  await expect(collectionRow(page, 'item-health').locator('.method')).toHaveText('GET');
  await expect(collectionRow(page, 'item-folder').locator('.method')).toHaveCount(0);

  // Expanding the folder reveals the request at the next depth; collapsing hides it again.
  await collectionRow(page, 'item-folder').locator('[data-testid="tree-twisty"]').click();
  await expect(collectionRow(page, 'item-create')).toBeVisible();
  await expect(collectionRow(page, 'item-create')).toHaveAttribute('data-depth', '2');
  await expect(collectionRow(page, 'item-create').locator('.method')).toHaveText('POST');
  await collectionRow(page, 'item-folder').locator('[data-testid="tree-twisty"]').click();
  await expect(collectionRow(page, 'item-create')).toHaveCount(0);

  // Double-clicking a request opens the existing 'http-request' tab kind (D14), carrying the
  // **saved name** rather than the URL-derived title, and the saved body.
  await collectionRow(page, 'item-folder').locator('[data-testid="tree-twisty"]').click();
  await collectionRow(page, 'item-create').dblclick();
  const view = page.locator('[data-testid="http-request-view"]');
  await expect(view).toBeVisible();
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(1);
  await expect(page.locator('[data-testid="tab"]')).toContainText('Create order');
  await expect(page.locator('[data-testid="http-request-target"]')).toContainText('Create order');
  // P17 D18: the method select is an app-drawn button now, not a native <select> — the tab's
  // state still shows through data-value, the same fact toHaveValue used to assert.
  await expect(page.locator('[data-testid="http-method-select"]')).toHaveAttribute(
    'data-value',
    'POST',
  );
  await expect(page.locator('[data-testid="http-url"]')).toHaveValue(
    'https://api.example.com/v2/orders',
  );

  // Opening the same row again reuses the bound tab rather than making a second one (D14).
  await collectionRow(page, 'item-create').dblclick();
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(1);
});

test('collections — editing marks the request dirty, and Save clears it', async ({ relaunch }) => {
  const SAVED_ITEM = {
    id: 'item-create',
    collectionId: 'col-1',
    parentId: 'item-folder',
    kind: 'request',
    name: 'Create order',
    sortOrder: 0,
    method: 'POST',
    url: 'https://api.example.com/v2/orders/edited',
    createdAt: NOW,
    updatedAt: NOW,
  };
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: TREE },
    { channel: IPC.collectionsGetRequest, response: CREATE_ORDER_REQUEST },
    { channel: IPC.collectionsSaveRequest, response: SAVED_ITEM },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });
  await openHttpMode(page);

  await collectionRow(page, 'col-1').locator('[data-testid="tree-twisty"]').click();
  await collectionRow(page, 'item-folder').locator('[data-testid="tree-twisty"]').click();
  await collectionRow(page, 'item-create').dblclick();
  await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();

  // A freshly opened saved request is clean, and Save is disabled because there is nothing to do.
  await expect(page.locator('[data-testid="http-dirty"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="http-save"]')).toBeDisabled();

  // P22b D3: Save lives in #head-trailing now, to the right of the request's own title, and its
  // left edge does not move when the dirty mark appears beside the title (F5's own bug — Save
  // used to sit in #badges, mid-row, and shift position whenever a badge's width changed).
  const target = page.locator('[data-testid="http-request-target"]');
  const saveButton = page.locator('[data-testid="http-save"]');
  const targetBoxBefore = await target.boundingBox();
  const saveBoxBefore = await saveButton.boundingBox();
  if (!targetBoxBefore || !saveBoxBefore) throw new Error('target/save button has no box');
  expect(targetBoxBefore.x + targetBoxBefore.width).toBeLessThanOrEqual(saveBoxBefore.x);

  await page.fill('[data-testid="http-url"]', 'https://api.example.com/v2/orders/edited');
  await expect(page.locator('[data-testid="http-dirty"]')).toBeVisible();
  await expect(page.locator('[data-testid="http-save"]')).toBeEnabled();

  const saveBoxAfter = await saveButton.boundingBox();
  if (!saveBoxAfter) throw new Error('save button has no box');
  expect(saveBoxAfter.x).toBe(saveBoxBefore.x);

  await page.click('[data-testid="http-save"]');
  await expect(page.locator('[data-testid="http-dirty"]')).toHaveCount(0);

  // The call carries the edited request — and none of the four UI-only pane fields, which is what
  // stops resizing a pane from marking a request dirty (D15's toSavedRequest).
  const calls = control.log().filter((e) => e.channel === IPC.collectionsSaveRequest);
  expect(calls).toHaveLength(1);
  const args = calls[0].args as {
    itemId: string;
    name: string;
    request: Record<string, unknown>;
  };
  expect(args.itemId).toBe('item-create');
  expect(args.name).toBe('Create order');
  expect(args.request.url).toBe('https://api.example.com/v2/orders/edited');
  expect(args.request.code).toBe('{"sku":"widget"}');
  for (const uiOnly of ['requestPane', 'responsePane', 'responseView', 'requestPaneHeight']) {
    expect(args.request).not.toHaveProperty(uiOnly);
  }
  // Nor the tab's own identity fields — a saved request document is the request, nothing else.
  expect(args.request).not.toHaveProperty('itemId');
  expect(args.request).not.toHaveProperty('name');
});

test('collections — import reports what it did, and no file contents cross the bridge', async ({
  relaunch,
}) => {
  const REPORT = {
    collectionId: 'col-1',
    name: 'Orders API',
    folders: 1,
    requests: 2,
    warnings: [
      {
        kind: 'scripts_inert',
        count: 4,
        detail:
          '4 pre-request/test scripts were kept but are not run — they survive an export unchanged.',
      },
      {
        kind: 'auth_inert',
        count: 2,
        detail:
          '2 requests or folders carry an auth block. It is kept but not applied — those requests will need an Authorization header.',
      },
      {
        kind: 'graphql_body',
        count: 1,
        detail: '1 GraphQL bodies were imported as JSON bodies carrying the same query.',
      },
    ],
  };
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: TREE },
    {
      channel: IPC.filesChooseOpenMany,
      response: {
        canceled: false,
        files: [
          {
            path: '/Users/someone/Orders.postman_collection.json',
            name: 'Orders.postman_collection.json',
            size: 4_200_000,
          },
        ],
      },
    },
    {
      channel: IPC.collectionsImport,
      response: {
        files: [
          {
            path: '/Users/someone/Orders.postman_collection.json',
            fileName: 'Orders.postman_collection.json',
            kind: 'collection',
            collection: REPORT,
            environment: null,
            error: null,
          },
        ],
      },
    },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });
  await openHttpMode(page);

  // P28 D18: the panel button moved to the menu bar (App → Import Postman Collection…).
  await emitWailsEvent(page, CHANNEL.importPostman, undefined);

  const strip = page.locator('[data-testid="import-report"]');
  await expect(strip).toBeVisible();
  await expect(page.locator('[data-testid="import-report-summary"]')).toHaveText(
    'Imported “Orders API”: 2 requests, 1 folder.',
  );
  await expect(strip.locator('li[data-kind="scripts_inert"]')).toContainText('are not run');
  await expect(strip.locator('li[data-kind="auth_inert"]')).toContainText('not applied');
  await expect(strip.locator('li[data-kind="graphql_body"]')).toContainText('JSON bodies');

  // The tree is re-listed after an import, so the new collection is visible without a reload.
  const listCalls = control.log().filter((e) => e.channel === IPC.collectionsList);
  expect(listCalls.length).toBeGreaterThanOrEqual(2);

  // Import carries the **path only** (D11/F16) — Go opens the file. This is the load-bearing
  // assertion of the test: the same shape P3's own form-data test asserts for a request body.
  const importCalls = control.log().filter((e) => e.channel === IPC.collectionsImport);
  expect(importCalls).toHaveLength(1);
  expect(importCalls[0].args).toEqual({
    paths: ['/Users/someone/Orders.postman_collection.json'],
  });
  // And nothing else in the whole call log is file-sized either.
  for (const entry of control.log()) {
    const size = entry.args === undefined ? 0 : JSON.stringify(entry.args).length;
    expect(size).toBeLessThan(2000);
  }

  // The strip is dismissible — it reports, it does not persist.
  await page.click('[data-testid="import-report-dismiss"]');
  await expect(strip).toHaveCount(0);
});

test('collections — search filters the tree, keeps ancestors, and restores the expansion state', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.collectionsList, response: TREE }];
  const { window: page } = await relaunch({ control: CONTROL });
  await openHttpMode(page);

  // Expand the collection only — the folder stays collapsed, which is the state search must
  // restore afterwards.
  await collectionRow(page, 'col-1').locator('[data-testid="tree-twisty"]').click();
  await expect(collectionRow(page, 'item-folder')).toBeVisible();
  await expect(collectionRow(page, 'item-create')).toHaveCount(0);

  await page.click('[data-testid="toggle-search"]');
  await page.fill('[data-testid="tree-search"]', 'Create');

  // Only the matching branch survives, and its ancestors render so the match is reachable.
  await expect(collectionRow(page, 'item-create')).toBeVisible();
  await expect(collectionRow(page, 'col-1')).toBeVisible();
  await expect(collectionRow(page, 'item-folder')).toBeVisible();
  await expect(collectionRow(page, 'item-health')).toHaveCount(0);
  // The matched substring is highlighted, not the whole label.
  await expect(collectionRow(page, 'item-create').locator('mark')).toHaveText('Create');

  // A URL-only match still surfaces the row, since the URL is searched even though it is not shown.
  await page.fill('[data-testid="tree-search"]', 'healthz');
  await expect(collectionRow(page, 'item-health')).toBeVisible();
  await expect(collectionRow(page, 'item-create')).toHaveCount(0);

  // Clearing restores exactly the shape the user had: the collection expanded, the folder not —
  // D13's "renders expanded without mutating `expanded`".
  await page.fill('[data-testid="tree-search"]', '');
  await expect(collectionRow(page, 'item-folder')).toBeVisible();
  await expect(collectionRow(page, 'item-health')).toBeVisible();
  await expect(collectionRow(page, 'item-create')).toHaveCount(0);
});

// P28 D16(d) removes P22b D8's Environments category from the collections panel — the user asked
// for the environment list to stop sharing the panel with the collections tree. What replaced it as
// the way in is the panel header's own Environments action, which opens the environments *tab*
// (D16c) — and each row there still opens the same openVariableSetTab('environment', …) the
// category's rows used to, which is what the tail of this test still checks.
const ENVIRONMENTS = [
  {
    id: 'env-prod',
    name: 'Prod',
    sortOrder: 0,
    isActive: true,
    color: 'green',
  },
  {
    id: 'env-staging',
    name: 'Staging',
    sortOrder: 1,
    isActive: false,
    color: 'amber',
  },
];

test('collections — no environments category in the panel; the header action opens the environments tab, and a row there opens one as a variables tab', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: TREE },
    { channel: IPC.variablesListEnvironments, response: ENVIRONMENTS },
  ];
  const { window: page } = await relaunch({ control: CONTROL });
  await openHttpMode(page);

  await expect(collectionRow(page, 'col-1')).toBeVisible();

  // D16(d): gone outright — neither the category's toggle nor any of its rows exists any more.
  await expect(page.locator('[data-testid="environments-category-toggle"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="environments-category-row"]')).toHaveCount(0);

  // D16(c)/(e): the panel header's action — server-environment, not the old settings gear — opens
  // the environments tab rather than a dialog.
  await page.click('[data-testid="api-environments"]');
  const environments = page.locator('[data-testid="environments-dialog"]');
  await expect(environments).toBeVisible();
  await expect(page.locator('[data-testid="environment-row"]')).toHaveCount(2);
  await expect(page.locator('[data-testid="environment-row"][data-id="env-prod"]')).toBeVisible();

  // The same openVariableSetTab('environment', …) the removed category's rows used to make — and,
  // unlike the dialog this replaced, opening it no longer closes the list behind it.
  await page
    .locator('[data-testid="environment-row"][data-id="env-prod"]')
    .locator('[data-testid="environment-edit-variables"]')
    .click();
  const view = page.locator('[data-testid="variables-dialog"][data-scope="environment"]');
  await expect(view).toBeVisible();
  // Two tabs now, and that is the point: the environments tab stays open behind the variables
  // tab it opened, where the dialog it replaced used to close itself. Assert the ACTIVE one.
  await expect(page.locator('[data-testid="tab"][data-active="true"]')).toContainText('Prod');
  await expect(page.locator('[data-testid="tab"][data-tab-kind="environments"]')).toHaveCount(1);
});

test('collections — Postman import builds the contract tree and export asks for a save path', async ({
  relaunch,
}) => {
  const tree = contract<{ collections: { id: string; name: string }[] }>(
    'api-postman',
    'CollectionsService.List',
  );
  const result = contract<{
    files: {
      collection: { name: string; requests: number; folders: number };
    }[];
  }>('api-postman', 'CollectionsService.Import');
  const report = result.files[0].collection;
  const importArgs = contract('api-postman', 'args:CollectionsService.Import');
  const exportArgs = contract<{ collectionId: string }>(
    'api-postman',
    'args:CollectionsService.Export',
  );
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: tree },
    {
      channel: IPC.filesChooseOpenMany,
      response: contract('api-postman', 'FilesService.ChooseOpenMany'),
    },
    { channel: IPC.collectionsImport, response: result },
    {
      channel: IPC.filesChooseSave,
      response: contract('api-postman', 'FilesService.ChooseSave'),
    },
    {
      channel: IPC.collectionsExport,
      response: contract('api-postman', 'CollectionsService.Export'),
    },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });
  await openHttpMode(page);

  await emitWailsEvent(page, CHANNEL.importPostman, undefined);
  await expect(page.locator('[data-testid="import-report-summary"]')).toHaveText(
    `Imported “${report.name}”: ${report.requests} requests, ${report.folders} folder.`,
  );
  const importCalls = control.log().filter((e) => e.channel === IPC.collectionsImport);
  expect(importCalls).toHaveLength(1);
  expect(importCalls[0].args).toEqual(importArgs);

  const root = collectionRow(page, tree.collections[0].id);
  await expect(root).toContainText(tree.collections[0].name);
  await root.click({ button: 'right' });
  await page.click('[data-testid="menu-item-export"]');
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.collectionsExport))
    .toHaveLength(1);
  expect(control.log().find((e) => e.channel === IPC.collectionsExport)?.args).toEqual(exportArgs);
});

// Contract api-postman-multi. Backend half: apiflow.TestPostmanImportCollectionAndEnvironment stores
// the same fixture.
test('collections — Postman import of a collection, an environment and a bad file', async ({
  relaunch,
}) => {
  const result = contract<{ files: { kind: string }[] }>(
    'api-postman-multi',
    'CollectionsService.Import',
  );
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.filesChooseOpenMany,
      response: contract('api-postman-multi', 'FilesService.ChooseOpenMany'),
    },
    { channel: IPC.collectionsImport, response: result },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });
  await openHttpMode(page);

  await emitWailsEvent(page, CHANNEL.importPostman, undefined);

  const files = page.locator('[data-testid="import-report-file"]');
  await expect(files).toHaveCount(4);
  await expect(files.nth(0)).toHaveAttribute('data-kind', 'collection');
  await expect(files.nth(0)).toContainText('Imported “Multi”: 2 requests, 0 folders.');
  await expect(files.nth(0).locator('li[data-kind="empty_values"]')).toContainText(
    '1 variables were imported without a value. Postman exports only shared values; fill them in here.',
  );
  await expect(files.nth(0).locator('li[data-kind="unnamed_variables"]')).toContainText(
    '1 variables had no name and were imported as unnamed_1, unnamed_2 and so on.',
  );
  await expect(files.nth(0).locator('li[data-kind="missing_request"]')).toContainText(
    '1 items had no request and were imported as empty GET requests.',
  );
  await expect(files.nth(1)).toHaveAttribute('data-kind', 'environment');
  await expect(files.nth(1)).toContainText('Imported environment “Staging”: 6 variables.');
  await expect(files.nth(1).locator('li[data-kind="disabled_variables"]')).toContainText(
    '1 disabled variables were skipped.',
  );
  await expect(files.nth(1).locator('li[data-kind="empty_values"]')).toContainText(
    '1 variables were imported without a value.',
  );
  await expect(files.nth(2)).toHaveAttribute('data-kind', 'failed');
  await expect(files.nth(2)).toContainText(
    'Couldn’t import bad.json: postman: not a Postman collection or environment',
  );
  await expect(files.nth(3)).toHaveAttribute('data-kind', 'environment');
  await expect(files.nth(3)).toContainText('Imported environment “Wrapped”: 3 variables.');
  await expect(files.nth(3).locator('li[data-kind="empty_values"]')).toContainText(
    '1 variables were imported without a value.',
  );
  await expect(files.nth(3).locator('li[data-kind="unnamed_variables"]')).toContainText(
    '1 variables had no name',
  );
  const importCalls = control.log().filter((e) => e.channel === IPC.collectionsImport);
  expect(importCalls[0].args).toEqual(
    contract('api-postman-multi', 'args:CollectionsService.Import'),
  );
});

// Contract api-restart. Backend half: apiflow.TestRestartKeepsApiState stores the post-restart state.
test('collections — after a restart the tree and the active environment come back', async ({
  relaunch,
}) => {
  const tree = contract<{
    collections: { id: string }[];
    items: { id: string; name: string }[];
  }>('api-restart', 'CollectionsService.List');
  const envs = contract<{ id: string; name: string; isActive: boolean }[]>(
    'api-restart',
    'VariablesService.ListEnvironments',
  );
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: tree },
    { channel: IPC.variablesListEnvironments, response: envs },
  ];
  const { window: page } = await relaunch({ control: CONTROL });
  await openHttpMode(page);

  await expect(collectionRow(page, tree.collections[0].id)).toBeVisible();
  await page.click('[data-testid="api-environments"]');
  const active = envs.find((e) => e.isActive);
  expect(active).toBeDefined();
  const activeRow = page.locator(`[data-testid="environment-row"][data-id="${active?.id}"]`);
  await expect(activeRow.locator('[role="radio"]')).toBeChecked();
});
