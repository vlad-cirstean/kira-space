// kira:api:dataChanged is what another window's edit reaches this one through — these three specs
// drive it directly with emitWailsEvent (the same push-channel mechanism grpc-request.spec.ts's own
// call streaming already exercises), rather than performing a second mutation in this window, so
// each one pins the cross-window path on its own: an environments change reaches the selector, a
// variables change reaches a URL's unresolved chip and Copy as cURL, and a tree change reaches an
// open tab's orphan/Save-as fallback.
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, test } from './fixtures';
import { modeTab, openHttpModeAndNewRequest } from './support/apiMode';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

const NOW = '2026-01-01T00:00:00.000Z';

// ---- 1. An environments change adds an environment — the selector lists it ----

test('an environments change adds an environment, and the selector lists it without a reload', async ({
  relaunch,
}) => {
  const ENV_STAGING = {
    id: 'env-1',
    name: 'Staging',
    sortOrder: 0,
    isActive: false,
    description: '',
    color: 'none',
  };
  const ENV_PRODUCTION = {
    id: 'env-2',
    name: 'Production',
    sortOrder: 1,
    isActive: false,
    description: '',
    color: 'none',
  };
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: { collections: [], items: [] } },
    { channel: IPC.variablesListEnvironments, response: [ENV_STAGING] },
    { channel: IPC.variablesListEnvironments, response: [ENV_STAGING, ENV_PRODUCTION] },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);

  const trigger = page.locator('[data-testid="api-environment-select"]');
  await trigger.click();
  await expect(page.locator('[data-testid="api-environment-menu"]')).toBeVisible();
  await expect(page.locator('[data-testid="api-environment-option"]')).toHaveCount(1);
  await trigger.click();
  await expect(page.locator('[data-testid="api-environment-menu"]')).toHaveCount(0);

  // Another window created "Production" — no local mutation here, only the push.
  await emitWailsEvent(page, IPC.apiDataChanged, { changes: [{ kind: 'environments' }] });

  await trigger.click();
  const options = page.locator('[data-testid="api-environment-option"]');
  await expect(options).toHaveCount(2);
  await expect(options.nth(1)).toContainText('Production');
});

// ---- 2. A variables change for the active environment — the unresolved chip and Copy as cURL ----

test('a variables change for the active environment resolves the URL chip and Copy as cURL', async ({
  relaunch,
}) => {
  const ENV = {
    id: 'env-1',
    name: 'Prod',
    sortOrder: 0,
    isActive: true,
    description: '',
    color: 'none',
  };
  const HOST_VAR = {
    id: 'var-host',
    scope: 'environment',
    ownerId: 'env-1',
    name: 'host',
    value: 'api.example.com',
    isSecret: false,
    sortOrder: 0,
    description: '',
  };
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: { collections: [], items: [] } },
    { channel: IPC.variablesListEnvironments, response: [ENV] },
    {
      channel: IPC.variablesList,
      args: { scope: 'environment', ownerId: 'env-1' },
      response: [],
    },
    {
      channel: IPC.variablesList,
      args: { scope: 'environment', ownerId: 'env-1' },
      response: [HOST_VAR],
    },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  await page.fill('[data-testid="http-url"]', 'https://{{host}}/v1/x');

  const chip = page.locator('[data-testid="http-unresolved-chip"]');
  await expect(chip).toContainText('1 unresolved');

  await page.click('[data-testid="http-copy-as-curl"]');
  await expect(page.locator('[data-testid="copy-as-curl-dialog"]')).toBeVisible();
  await expect(page.locator('[data-testid="copy-as-curl-command"]')).toHaveValue(/\{\{host\}\}/);
  await page.click('[data-testid="copy-as-curl-close"]');
  await expect(page.locator('[data-testid="copy-as-curl-dialog"]')).toHaveCount(0);

  // Another window added "host" to the same environment's own variables — no local mutation here.
  await emitWailsEvent(page, IPC.apiDataChanged, {
    changes: [{ kind: 'variables', scope: 'environment', ownerId: 'env-1' }],
  });

  await expect(chip).toHaveCount(0);
  await page.click('[data-testid="http-copy-as-curl"]');
  await expect(page.locator('[data-testid="copy-as-curl-command"]')).toHaveValue(
    /api\.example\.com/,
  );
});

// ---- 3. A tree change removes an open request's item — Save falls back to Save as (orphan) ----

test('a tree change removes an open request item, and Save falls back to Save as (D14 orphan)', async ({
  relaunch,
}) => {
  const TREE_WITH_ITEM = {
    collections: [
      { id: 'col-1', name: 'Orders API', sortOrder: 0, createdAt: NOW, updatedAt: NOW },
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
        protocol: 'http',
        createdAt: NOW,
        updatedAt: NOW,
      },
    ],
  };
  const TREE_ITEM_REMOVED = { collections: TREE_WITH_ITEM.collections, items: [] as unknown[] };
  const SAVED_REQUEST = {
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
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: TREE_WITH_ITEM },
    { channel: IPC.collectionsList, response: TREE_ITEM_REMOVED },
    { channel: IPC.collectionsGetRequest, response: SAVED_REQUEST },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL });

  await modeTab(page, 'api').click();
  // A collection renders collapsed until it is opened (collections.spec.ts's own note) — expand it
  // before the item row underneath it is visible at all.
  const collectionRow = page.locator('[data-testid="collection-row"][data-id="col-1"]');
  await expect(collectionRow).toBeVisible();
  await collectionRow.locator('[data-testid="tree-twisty"]').click();
  const row = page.locator('[data-testid="collection-row"][data-id="item-1"]');
  await expect(row).toBeVisible();
  await row.dblclick();
  await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();
  // CollectionsTree.vue's own onOpen awaits loadSavedRequest before ever opening the tab, so the
  // saved row's fields are already on screen the moment the view mounts — clean and not yet
  // orphaned, so Save stays disabled (canSave && !dirty, collections.spec.ts's own precedent for a
  // freshly opened, untouched saved request) until the tree change below makes it orphaned.
  await expect(page.locator('[data-testid="http-url"]')).toHaveValue(
    'https://api.example.com/healthz',
  );
  await expect(page.locator('[data-testid="http-dirty"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="http-save"]')).toBeDisabled();

  // Another window deleted "Health check" — no local mutation here, only the push. The tab stays
  // open (a request delete never closes an open tab), but the row itself follows the tree, and
  // reconcileTree nulls the tab's own saved-request cache entry — which is what flips Save from
  // "disabled, nothing to save" to the orphan/Save-as case.
  await emitWailsEvent(page, IPC.apiDataChanged, { changes: [{ kind: 'tree' }] });
  await expect(row).toHaveCount(0);
  await expect(page.locator('[data-testid="http-save"]')).toBeEnabled();

  await page.click('[data-testid="http-save"]');
  await expect(page.locator('[data-testid="save-request-dialog"]')).toBeVisible();
  expect(control.log().filter((e) => e.channel === IPC.collectionsSaveRequest)).toHaveLength(0);
});
