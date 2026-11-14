// Decision 3 (OQ-API-6). UI half of apiflow.TestSavedRequestTwoWindows (contract
// api-request-two-windows): a request another window saved is not overwritten unseen. The first
// Save carries the request this tab read; the refusal asks, declining keeps the edit unsaved, and
// confirming sends the same save again without its guard.
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openHttpMode } from './support/apiMode';
import { collectionRow } from './support/apiRows';
import { acceptConfirm, cancelConfirm } from './support/dialogs';

const scenario = 'api-request-two-windows';
const conflict = contract<{ code: string; message: string; details: { changed: boolean } }>(
  scenario,
  'SaveRequest#changed',
);
const guarded = contract<{ expect: { url: string }; request: { url: string } }>(
  scenario,
  'args:SaveRequest#guarded',
);
const confirmed = contract<{ name: string; url: string }>(scenario, 'SaveRequest#confirmed');

const NOW = '2026-01-01T00:00:00.000Z';
const ITEM = {
  id: 'item-shared',
  collectionId: 'col-shared',
  parentId: null,
  kind: 'request',
  name: confirmed.name,
  sortOrder: 0,
  method: 'GET',
  url: guarded.expect.url,
  protocol: 'http',
  createdAt: NOW,
  updatedAt: NOW,
};
const TREE = {
  collections: [
    {
      id: 'col-shared',
      name: 'Two windows',
      file: 'two-windows.collection.yaml',
      sortOrder: 0,
      createdAt: NOW,
      updatedAt: NOW,
    },
  ],
  items: [ITEM],
};
const READ = {
  method: 'GET',
  url: guarded.expect.url,
  headers: [],
  bodyMode: 'none',
  body: '',
  code: '',
  codeLanguage: 'json',
  urlEncoded: [],
  formData: [],
  binaryFile: null,
};
const error = { code: conflict.code, message: conflict.message, details: conflict.details };

const SAVE_ARGS = {
  itemId: ITEM.id,
  name: ITEM.name,
  request: { ...READ, url: guarded.request.url, paramDescriptions: {} },
};
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.collectionsList, response: TREE },
  { channel: IPC.variablesListEnvironments, response: [] },
  { channel: IPC.collectionsGetRequest, response: READ },
  // Both guarded saves are refused; the confirmed resend carries no guard and lands.
  {
    channel: IPC.collectionsSaveRequest,
    args: { ...SAVE_ARGS, expect: { ...READ, paramDescriptions: {} } },
    error,
  },
  {
    channel: IPC.collectionsSaveRequest,
    args: { ...SAVE_ARGS, expect: { ...READ, paramDescriptions: {} } },
    error,
  },
  {
    channel: IPC.collectionsSaveRequest,
    args: { ...SAVE_ARGS, expect: null },
    response: { ...ITEM, url: guarded.request.url },
  },
];

test('a Save over a request another window saved asks first and writes only after confirm', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: CONTROL });
  await openHttpMode(page);
  await collectionRow(page, 'col-shared').locator('[data-testid="tree-twisty"]').click();
  await collectionRow(page, 'item-shared').dblclick();
  await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();

  const saves = () => control.log().filter((e) => e.channel === IPC.collectionsSaveRequest);
  await page.fill('[data-testid="http-url"]', guarded.request.url);
  await page.click('[data-testid="http-save"]');

  // The refusal names the conflict; declining leaves the edit unsaved.
  await expect(page.locator('[data-testid="confirm-dialog"]')).toContainText(conflict.message);
  await cancelConfirm(page);
  await expect(page.locator('[data-testid="http-dirty"]')).toBeVisible();
  expect(saves()).toHaveLength(1);
  expect(saves()[0].args).toMatchObject({ expect: { url: guarded.expect.url } });

  // Saving again asks again; confirming resends the same request without the guard.
  await page.click('[data-testid="http-save"]');
  await acceptConfirm(page);
  await expect(page.locator('[data-testid="http-dirty"]')).toHaveCount(0);
  expect(saves()).toHaveLength(3);
});
