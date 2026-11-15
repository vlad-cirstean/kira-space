// UI half of apiflow.TestTabCloseSaveOrDiscard (contract api-tab-close): closing an API tab that
// holds a new request or unsaved edits asks Save, Discard or Cancel, and the tab closes only after
// Save or Discard. Save writes through the contract's SaveRequest / CreateItem args; Discard and
// Cancel write nothing.
import type { Page } from '@playwright/test';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { emitWailsEvent } from '../support/mockRuntime';
import { openHttpMode, openHttpModeAndNewRequest } from './support/apiMode';
import { collectionRow } from './support/apiRows';
import { acceptConfirm, cancelConfirm, chooseSecondary } from './support/dialogs';

const scenario = 'api-tab-close';
interface Req {
  url: string;
}
const saveArgs = contract<{ name: string; expectName: string; expect: Req; request: Req }>(
  scenario,
  'args:SaveRequest#close',
);
const createArgs = contract<{ name: string; kind: string; request: Req }>(
  scenario,
  'args:CreateItem#close',
);

const NOW = '2026-01-01T00:00:00.000Z';
function item(id: string, protocol: 'http' | 'grpc') {
  return {
    id,
    collectionId: 'col-shared',
    parentId: null,
    kind: 'request',
    name: id === 'item-shared' ? saveArgs.name : id,
    sortOrder: 0,
    method: protocol === 'http' ? 'GET' : '',
    url: protocol === 'http' ? saveArgs.expect.url : '',
    protocol,
    createdAt: NOW,
    updatedAt: NOW,
  };
}
const ITEM = item('item-shared', 'http');
const TREE = {
  collections: [
    {
      id: 'col-shared',
      name: 'Tab close',
      file: 'tab-close.collection.yaml',
      sortOrder: 0,
      createdAt: NOW,
      updatedAt: NOW,
    },
  ],
  items: [ITEM, item('item-other', 'http'), item('item-grpc', 'grpc')],
};
const READ = {
  method: 'GET',
  url: saveArgs.expect.url,
  headers: [],
  bodyMode: 'none',
  body: '',
  code: '',
  codeLanguage: 'json',
  urlEncoded: [],
  formData: [],
  binaryFile: null,
};
const GRPC_READ = {
  target: 'demo.example.com:443',
  tlsMode: 'tls',
  caFile: '',
  serverName: '',
  descriptorMode: 'reflection',
  protoPath: '',
  importPaths: [],
  service: '',
  method: '',
  message: '{"a":1}',
  metadata: [],
};
const BASE: ControlSnapshot[] = [
  { channel: IPC.collectionsList, response: TREE },
  { channel: IPC.variablesListEnvironments, response: [] },
  { channel: IPC.collectionsGetRequest, response: READ },
  { channel: IPC.collectionsGetGrpcRequest, response: GRPC_READ },
  { channel: IPC.grpcDescribe, response: { services: [], mode: 'reflection', warnings: [] } },
];
const SAVED: ControlSnapshot[] = [
  ...BASE,
  { channel: IPC.collectionsSaveRequest, response: { ...ITEM, url: saveArgs.request.url } },
  {
    channel: IPC.collectionsCreateItem,
    response: { ...item('item-fresh', 'http'), name: createArgs.name, url: createArgs.request.url },
  },
];

const view = (page: Page) => page.locator('[data-testid="http-request-view"]');
const dialog = (page: Page) => page.locator('[data-testid="confirm-dialog"]');
const tabs = (page: Page) => page.locator('[data-testid="tab"]');
const closeButton = (page: Page, index = 0) =>
  tabs(page).nth(index).locator('[data-testid="tab-close"]');
const logOf = (control: { log(): { channel: string; args?: unknown }[] }, channel: string) =>
  control.log().filter((e) => e.channel === channel);

async function expandCollection(page: Page): Promise<void> {
  await openHttpMode(page);
  await collectionRow(page, 'col-shared').locator('[data-testid="tree-twisty"]').click();
}

async function openAndEdit(page: Page, itemId: string, url: string): Promise<void> {
  await collectionRow(page, itemId).dblclick();
  await expect(view(page)).toBeVisible();
  await page.fill('[data-testid="http-url"]', url);
  await expect(page.locator('[data-testid="http-dirty"]')).toBeVisible();
}

test('a modified saved tab asks on close: Cancel keeps it, Discard drops the edit, Save writes it', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: SAVED });
  await expandCollection(page);
  await openAndEdit(page, 'item-shared', saveArgs.request.url);

  await closeButton(page).click();
  await expect(dialog(page)).toContainText(`Save changes to ${saveArgs.name}`);
  await expect(dialog(page).locator('[data-testid="confirm-dialog-message"]')).toContainText(
    'unsaved changes',
  );
  await expect(page.locator('[data-testid="confirm-dialog-confirm"]')).toHaveText('Save');
  await expect(page.locator('[data-testid="confirm-dialog-secondary"]')).toHaveText('Discard');
  await expect(page.locator('[data-testid="confirm-dialog-cancel"]')).toBeVisible();

  await cancelConfirm(page);
  await expect(tabs(page)).toHaveCount(1);
  await expect(page.locator('[data-testid="http-dirty"]')).toBeVisible();
  expect(logOf(control, IPC.collectionsSaveRequest)).toHaveLength(0);

  await closeButton(page).click();
  await chooseSecondary(page);
  await expect(tabs(page)).toHaveCount(0);
  expect(logOf(control, IPC.collectionsSaveRequest)).toHaveLength(0);

  await openAndEdit(page, 'item-shared', saveArgs.request.url);
  await closeButton(page).click();
  await acceptConfirm(page);
  await expect(tabs(page)).toHaveCount(0);
  const saves = logOf(control, IPC.collectionsSaveRequest);
  expect(saves).toHaveLength(1);
  expect(saves[0].args).toMatchObject({
    name: saveArgs.name,
    expectName: saveArgs.expectName,
    expect: { url: saveArgs.expect.url },
    request: { url: saveArgs.request.url },
  });
});

test('a new tab with content asks on close: Save opens Save as, Discard writes nothing', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: SAVED });
  await openHttpMode(page);
  await openHttpModeAndNewRequest(page);
  await page.fill('[data-testid="http-url"]', createArgs.request.url);

  // Ctrl+W reaches the app as the native menu's tabClose event.

  await emitWailsEvent(page, IPC.tabClose, undefined);
  await expect(dialog(page).locator('[data-testid="confirm-dialog-message"]')).toContainText(
    'never saved',
  );

  // Dismissing Save as… leaves the tab open.
  await acceptConfirm(page);
  await expect(page.locator('[data-testid="save-request-dialog"]')).toBeVisible();
  await page.click('[data-testid="save-request-cancel"]');
  await expect(page.locator('[data-testid="save-request-dialog"]')).toHaveCount(0);
  await expect(tabs(page)).toHaveCount(1);
  expect(logOf(control, IPC.collectionsCreateItem)).toHaveLength(0);

  // Saving as creates the row, then the tab closes.
  await emitWailsEvent(page, IPC.tabClose, undefined);
  await acceptConfirm(page);
  await page.fill('[data-testid="save-request-name"]', createArgs.name);
  await page.click('[data-testid="save-request-submit"]');
  await expect(page.locator('[data-testid="save-request-dialog"]')).toHaveCount(0);
  await expect(tabs(page)).toHaveCount(0);
  const creates = logOf(control, IPC.collectionsCreateItem);
  expect(creates).toHaveLength(1);
  expect(creates[0].args).toMatchObject({
    name: createArgs.name,
    kind: createArgs.kind,
    request: { url: createArgs.request.url },
  });

  // Discard on a second new tab: closes, creates nothing more.
  await openHttpModeAndNewRequest(page);
  await page.fill('[data-testid="http-url"]', createArgs.request.url);
  await closeButton(page).click();
  await chooseSecondary(page);
  await expect(tabs(page)).toHaveCount(0);
  expect(logOf(control, IPC.collectionsCreateItem)).toHaveLength(1);
});

test('an untouched new tab closes without asking', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: SAVED });
  await openHttpMode(page);
  await openHttpModeAndNewRequest(page);
  await expect(tabs(page)).toHaveCount(1);
  await closeButton(page).click();
  await expect(tabs(page)).toHaveCount(0);
  await expect(dialog(page)).toHaveCount(0);
});

test('middle click on a modified tab asks', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: SAVED });
  await expandCollection(page);
  await openAndEdit(page, 'item-shared', saveArgs.request.url);
  await tabs(page).first().click({ button: 'middle' });
  await expect(dialog(page)).toBeVisible();
  await cancelConfirm(page);
  await expect(tabs(page)).toHaveCount(1);
});

test('Close all with two edited tabs: Cancel closes nothing, Discard closes both', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: SAVED });
  await expandCollection(page);
  await openAndEdit(page, 'item-shared', saveArgs.request.url);
  await openAndEdit(page, 'item-other', saveArgs.request.url);
  await expect(tabs(page)).toHaveCount(2);

  const closeAll = async () => {
    await tabs(page).first().click({ button: 'right' });
    await page.click('[data-testid="menu-item-close-all"]');
  };
  await closeAll();
  await expect(dialog(page)).toBeVisible();
  await cancelConfirm(page);
  await expect(tabs(page)).toHaveCount(2);

  await closeAll();
  // The second question follows at once, so the first click cannot wait for the dialog to go.
  await expect(dialog(page)).toBeVisible();
  await page.click('[data-testid="confirm-dialog-secondary"]');
  await chooseSecondary(page);
  await expect(tabs(page)).toHaveCount(0);
});

test('a Save that fails keeps the tab open with the error shown', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [
      ...BASE,
      {
        channel: IPC.collectionsSaveRequest,
        error: { code: 'E_INTERNAL', message: 'disk is full' },
      },
    ],
  });
  await expandCollection(page);
  await openAndEdit(page, 'item-shared', saveArgs.request.url);
  await closeButton(page).click();
  await acceptConfirm(page);
  await expect(page.locator('[data-testid="collections-error"]')).toContainText('disk is full');
  await expect(tabs(page)).toHaveCount(1);
  await expect(page.locator('[data-testid="http-dirty"]')).toBeVisible();
});

test('a modified gRPC tab asks on close and Discard closes it', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: BASE });
  await expandCollection(page);
  await collectionRow(page, 'item-grpc').dblclick();
  await expect(page.locator('[data-testid="grpc-request-view"]')).toBeVisible();
  await page.fill('[data-testid="grpc-target"]', 'other.example.com:443');
  await expect(page.locator('[data-testid="grpc-dirty"]')).toBeVisible();
  await closeButton(page).click();
  await expect(dialog(page)).toBeVisible();
  await chooseSecondary(page);
  await expect(tabs(page)).toHaveCount(0);
});
