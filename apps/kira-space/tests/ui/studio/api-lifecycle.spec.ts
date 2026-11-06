import type { Page } from '@playwright/test';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openHttpMode } from './support/apiMode';
import { collectionRow } from './support/apiRows';
import { acceptConfirm, cancelConfirm } from './support/dialogs';
import { dragRow, settleFrames } from './support/tree';

// Rename, move, delete and duplicate on the Api tree and the environments tab, each an edit of a
// collection or environment file. The Go half is apiflow TestApiLifecycle; contract api-lifecycle
// fixes the call shapes, the trees and the answers.

const KEY = 'api-lifecycle';

interface Row {
  id: string;
  name: string;
}
interface Tree {
  collections: Row[];
  items: Row[];
}
const tree = (name: string) => contract<Tree>(KEY, name);
const seed = tree('tree#seed');
const nested = tree('tree#nested');
const moved = tree('tree#moved');
const idOf = (list: Row[], name: string): string => {
  const hit = list.find((x) => x.name === name);
  if (!hit) throw new Error(`no ${name} in the contract tree`);
  return hit.id;
};

const calls = (control: { log(): { channel: string; args?: unknown }[] }, channel: string) =>
  control.log().filter((e) => e.channel === channel);
const menuItem = (page: Page, id: string) => page.locator(`[data-testid="menu-item-${id}"]`);
const twisty = (page: Page, id: string) =>
  collectionRow(page, id).locator('[data-testid="tree-twisty"]');
const list = (...trees: Tree[]): ControlSnapshot[] =>
  trees.map((response) => ({ channel: IPC.collectionsList, response }));

// The app reads the platform chord from the page's user agent.
const primary = (page: Page) =>
  page.evaluate(() =>
    navigator.userAgent.includes('Mac') ? ('Meta' as const) : ('Control' as const),
  );

test('F2 renames a request in place: one call with the new name', async ({ relaunch }) => {
  const orders = idOf(seed.collections, 'Orders');
  const request = idOf(seed.items, 'List orders');
  const { window: page, control } = await relaunch({
    control: [...list(seed, seed), { channel: IPC.collectionsRename, response: null }],
  });
  await openHttpMode(page);
  await twisty(page, orders).click();

  await collectionRow(page, request).click();
  await page.keyboard.press('F2');
  const input = page.locator('[data-testid="collection-row"] input');
  await expect(input).toBeFocused();
  await input.fill('List all orders');
  await page.keyboard.press('Enter');

  await expect.poll(() => calls(control, IPC.collectionsRename)).toHaveLength(1);
  expect(calls(control, IPC.collectionsRename)[0].args).toEqual(
    contract(KEY, 'args:CollectionsService.Rename#request'),
  );
  await expect(input).toHaveCount(0);
});

test('a collection renames from its menu', async ({ relaunch }) => {
  const orders = idOf(seed.collections, 'Orders');
  const { window: page, control } = await relaunch({
    control: [...list(seed, seed), { channel: IPC.collectionsRename, response: null }],
  });
  await openHttpMode(page);

  await collectionRow(page, orders).click({ button: 'right' });
  await menuItem(page, 'rename').click();
  const input = page.locator('[data-testid="collection-row"] input');
  await expect(input).toBeFocused();
  await input.fill('Orders v2');
  await page.keyboard.press('Enter');

  await expect.poll(() => calls(control, IPC.collectionsRename)).toHaveLength(1);
  expect(calls(control, IPC.collectionsRename)[0].args).toMatchObject({
    id: orders,
    target: 'collection',
    name: 'Orders v2',
  });
});

test('drag a request above a sibling, then into a folder: placements follow the drop zone', async ({
  relaunch,
}) => {
  const orders = idOf(seed.collections, 'Orders');
  const first = idOf(seed.items, 'List orders');
  const last = idOf(seed.items, 'Get order');
  const folder = idOf(seed.items, 'Admin');
  const { window: page, control } = await relaunch({
    control: [
      ...list(seed, seed, nested),
      { channel: IPC.collectionsMoveItems, response: contract(KEY, 'result#before') },
    ],
  });
  await openHttpMode(page);
  await twisty(page, orders).click();

  await dragRow(page, collectionRow(page, last), collectionRow(page, first), 0.25);
  await expect(
    collectionRow(page, first).locator('[data-testid="tree-drop-indicator"]'),
  ).toHaveAttribute('data-zone', 'before');
  await page.mouse.up();
  await expect.poll(() => calls(control, IPC.collectionsMoveItems)).toHaveLength(1);
  expect(calls(control, IPC.collectionsMoveItems)[0].args).toEqual({
    ...contract<object>(KEY, 'args:CollectionsService.MoveItems#before'),
    itemIds: [last],
    collectionId: orders,
    parentId: null,
    beforeId: first,
  });

  await dragRow(page, collectionRow(page, first), collectionRow(page, folder), 0.5);
  await expect(collectionRow(page, folder)).toHaveAttribute('data-drop-zone', 'into');
  await page.mouse.up();
  await expect.poll(() => calls(control, IPC.collectionsMoveItems)).toHaveLength(2);
  expect(calls(control, IPC.collectionsMoveItems)[1].args).toEqual({
    ...contract<object>(KEY, 'args:CollectionsService.MoveItems#into'),
    itemIds: [first],
    collectionId: orders,
    parentId: folder,
    beforeId: null,
  });
});

test('a folder moves to another collection from its menu, and its open request tab follows the new row', async ({
  relaunch,
}) => {
  const billing = idOf(nested.collections, 'Billing');
  const orders = idOf(nested.collections, 'Orders v2');
  const folder = idOf(nested.items, 'Operations');
  const request = idOf(nested.items, 'List all orders');
  const map = contract<Record<string, string>>(KEY, 'result#across');
  const newRequest = map[request];
  const savedRequest = {
    method: 'GET',
    url: 'https://api.example.com/list',
    headers: [],
    bodyMode: 'none',
    body: '',
    code: '',
    codeLanguage: 'json',
    urlEncoded: [],
    formData: [],
    binaryFile: null,
  };
  const { window: page, control } = await relaunch({
    control: [
      ...list(nested, moved),
      { channel: IPC.collectionsGetRequest, response: savedRequest },
      { channel: IPC.collectionsMoveItems, response: map },
    ],
  });
  await openHttpMode(page);
  await twisty(page, orders).click();
  await twisty(page, folder).click();
  await collectionRow(page, request).dblclick();
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(1);

  await collectionRow(page, folder).click({ button: 'right' });
  await menuItem(page, 'move-to-collection').hover();
  await page
    .locator(`[data-testid="context-submenu"] [data-testid="menu-item-move-to-${billing}"]`)
    .click();
  await expect.poll(() => calls(control, IPC.collectionsMoveItems)).toHaveLength(1);
  expect(calls(control, IPC.collectionsMoveItems)[0].args).toEqual({
    ...contract<object>(KEY, 'args:CollectionsService.MoveItems#across'),
    itemIds: [folder],
    collectionId: billing,
    parentId: null,
    beforeId: null,
  });

  // The moved request is a new row; opening it reuses the tab instead of making a second one.
  await twisty(page, map[folder]).click();
  await expect(collectionRow(page, newRequest)).toBeVisible();
  await collectionRow(page, newRequest).dblclick();
  await settleFrames(page);
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(1);
});

test('Delete on a mixed selection counts the topmost rows and confirms before one call', async ({
  relaunch,
}) => {
  const billing = idOf(moved.collections, 'Billing');
  const archive = idOf(moved.items, 'Archive');
  const folder = idOf(moved.items, 'Operations');
  const child = idOf(moved.items, 'Purge');
  const { window: page, control } = await relaunch({
    control: [...list(moved, moved), { channel: IPC.collectionsDeleteMany, response: null }],
  });
  await openHttpMode(page);
  await twisty(page, billing).click();
  await twisty(page, folder).click();
  await expect(collectionRow(page, child)).toBeVisible();

  await collectionRow(page, archive).click();
  await collectionRow(page, folder).click({ modifiers: [await primary(page)] });
  await collectionRow(page, child).click({ modifiers: [await primary(page)] });
  await collectionRow(page, folder).click({ button: 'right' });
  await expect(menuItem(page, 'delete')).toContainText('2');
  await menuItem(page, 'delete').click();

  const dialog = page.locator('[data-testid="confirm-dialog"]');
  await expect(dialog).toContainText('2');
  await cancelConfirm(page);
  expect(calls(control, IPC.collectionsDeleteMany)).toHaveLength(0);

  await collectionRow(page, folder).click({ button: 'right' });
  await menuItem(page, 'delete').click();
  await acceptConfirm(page);
  await expect.poll(() => calls(control, IPC.collectionsDeleteMany)).toHaveLength(1);
  const args = calls(control, IPC.collectionsDeleteMany)[0].args as {
    collectionIds: string[];
    itemIds: string[];
  };
  expect(args.collectionIds).toEqual([]);
  expect([...args.itemIds].sort()).toEqual([archive, folder].sort());
});

test('environments: Duplicate adds the copy, Delete asks first and removes the file', async ({
  relaunch,
}) => {
  const seedEnvs = contract<Array<{ id: string; name: string }>>(KEY, 'environments#seed');
  const duplicated = contract<Array<{ id: string; name: string }>>(KEY, 'environments#duplicated');
  const deleted = contract<Array<{ id: string; name: string }>>(KEY, 'environments#deleted');
  const production = seedEnvs[0].id;
  const copy = duplicated.find((e) => e.id !== production);
  if (!copy) throw new Error('no copy in the contract');
  const { window: page, control } = await relaunch({
    control: [
      ...list(seed),
      { channel: IPC.variablesListEnvironments, response: seedEnvs },
      { channel: IPC.variablesListEnvironments, response: duplicated },
      { channel: IPC.variablesListEnvironments, response: deleted },
      { channel: IPC.variablesDuplicateEnvironment, response: copy },
      { channel: IPC.variablesDeleteEnvironment, response: null },
    ],
  });
  await openHttpMode(page);
  await page.click('[data-testid="api-environments"]');
  const rows = page.locator('[data-testid="environment-row"]');
  await expect(rows).toHaveCount(1);

  await rows.first().locator('[data-testid="environment-duplicate"]').click();
  await expect.poll(() => calls(control, IPC.variablesDuplicateEnvironment)).toHaveLength(1);
  expect(calls(control, IPC.variablesDuplicateEnvironment)[0].args).toEqual({ id: production });
  await expect(rows).toHaveCount(2);

  await page
    .locator(`[data-testid="environment-row"][data-id="${production}"]`)
    .locator('[data-testid="environment-remove"]')
    .click();
  await cancelConfirm(page);
  expect(calls(control, IPC.variablesDeleteEnvironment)).toHaveLength(0);

  await page
    .locator(`[data-testid="environment-row"][data-id="${production}"]`)
    .locator('[data-testid="environment-remove"]')
    .click();
  await acceptConfirm(page);
  await expect.poll(() => calls(control, IPC.variablesDeleteEnvironment)).toHaveLength(1);
  expect(calls(control, IPC.variablesDeleteEnvironment)[0].args).toEqual({ id: production });
  await expect(rows).toHaveCount(1);
});
