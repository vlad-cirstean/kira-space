// Decision 3 (OQ-API-6). UI half of apiflow.TestSavedGrpcRequestTwoWindows (contract
// api-grpc-two-windows): a gRPC request another window saved is not overwritten unseen. The first
// Save carries the request and name this tab read; the refusal asks, declining keeps the edit
// unsaved, and confirming sends the same save again without its guard.
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openHttpMode } from './support/apiMode';
import { collectionRow } from './support/apiRows';
import { acceptConfirm, cancelConfirm } from './support/dialogs';

const scenario = 'api-grpc-two-windows';
const conflict = contract<{ code: string; message: string; details: { changed: boolean } }>(
  scenario,
  'SaveGrpcRequest#changed',
);
const guarded = contract<{
  expect: { message: string };
  expectName: string;
}>(scenario, 'args:SaveGrpcRequest#guarded');
const confirmed = contract<{ name: string; url: string }>(scenario, 'SaveGrpcRequest#confirmed');

const NOW = '2026-01-01T00:00:00.000Z';
const ITEM = {
  id: 'item-shared',
  collectionId: 'col-shared',
  parentId: null,
  kind: 'request',
  name: confirmed.name,
  sortOrder: 0,
  method: '',
  url: '',
  protocol: 'grpc',
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
  ...guarded.expect,
  importPaths: [],
  metadata: [],
};
const EDITED = { ...READ, message: `${READ.message} ` };
const SAVE_ARGS = { itemId: ITEM.id, name: ITEM.name, request: EDITED };
const error = { code: conflict.code, message: conflict.message, details: conflict.details };

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.collectionsList, response: TREE },
  { channel: IPC.variablesListEnvironments, response: [] },
  { channel: IPC.collectionsGetGrpcRequest, response: READ },
  { channel: IPC.grpcDescribe, response: { services: [], mode: 'reflection', warnings: [] } },
  // Both guarded saves are refused; the confirmed resend carries no guard and lands.
  {
    channel: IPC.collectionsSaveGrpcRequest,
    args: { ...SAVE_ARGS, expect: READ, expectName: guarded.expectName },
    error,
  },
  {
    channel: IPC.collectionsSaveGrpcRequest,
    args: { ...SAVE_ARGS, expect: READ, expectName: guarded.expectName },
    error,
  },
  {
    channel: IPC.collectionsSaveGrpcRequest,
    args: { ...SAVE_ARGS, expect: null, expectName: null },
    response: ITEM,
  },
];

test('a gRPC Save over a request another window saved asks first and writes only after confirm', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: CONTROL });
  await openHttpMode(page);
  await collectionRow(page, 'col-shared').locator('[data-testid="tree-twisty"]').click();
  await collectionRow(page, 'item-shared').dblclick();
  await expect(page.locator('[data-testid="grpc-request-view"]')).toBeVisible();

  const saves = () => control.log().filter((e) => e.channel === IPC.collectionsSaveGrpcRequest);
  await page.locator('[data-testid="grpc-message-editor"] .view-lines').click();
  await page.keyboard.press('End');
  await page.keyboard.type(' ');
  await expect(page.locator('[data-testid="grpc-dirty"]')).toBeVisible();
  await page.click('[data-testid="grpc-save"]');

  // The refusal names the conflict; declining leaves the edit unsaved.
  await expect(page.locator('[data-testid="confirm-dialog"]')).toContainText(conflict.message);
  await cancelConfirm(page);
  await expect(page.locator('[data-testid="grpc-dirty"]')).toBeVisible();
  expect(saves()).toHaveLength(1);
  expect(saves()[0].args).toMatchObject({
    expect: { message: guarded.expect.message },
    expectName: guarded.expectName,
  });

  // Saving again asks again; confirming resends the same request without the guard.
  await page.click('[data-testid="grpc-save"]');
  await acceptConfirm(page);
  await expect(page.locator('[data-testid="grpc-dirty"]')).toHaveCount(0);
  expect(saves()).toHaveLength(3);
  expect(saves()[2].args).toMatchObject({ expect: null, expectName: null });
});
