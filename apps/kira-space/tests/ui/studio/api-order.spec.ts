import type { Locator, Page } from '@playwright/test';
import { sortableHover } from '@workbench/testing/ui/drag';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openHttpMode } from './support/apiMode';
import { collectionRow } from './support/apiRows';
import { dragRow, settleFrames } from './support/tree';

// Collections and environments keep a manual order (drag, Move up / Move down, Alt+Arrow); items
// still reorder inside a collection. The Go half is apiflow TestApiOrder; contract api-order fixes
// the call shapes and the trees. Seed order is file-load order: Zeta, Alpha, Mid.

const KEY = 'api-order';

interface Row {
  id: string;
  name: string;
  sortOrder: number;
}
interface Tree {
  collections: Row[];
  items: Row[];
}
const seed = contract<Tree>(KEY, 'tree#seed');
const idOf = (list: Row[], name: string): string => {
  const hit = list.find((x) => x.name === name);
  if (!hit) throw new Error(`no ${name} in the contract`);
  return hit.id;
};
const Z = idOf(seed.collections, 'Zeta');
const A = idOf(seed.collections, 'Alpha');
const M = idOf(seed.collections, 'Mid');
const R1 = idOf(seed.items, 'r1');
const R2 = idOf(seed.items, 'r2');
const F = idOf(seed.items, 'f');

/** The tree with the collections in `order`, the answer to the refetch after a reorder. */
function treeIn(order: string[]): Tree {
  return {
    ...seed,
    collections: order.map((id, i) => ({
      ...(seed.collections.find((c) => c.id === id) as Row),
      sortOrder: i,
    })),
  };
}
const list = (...trees: Tree[]): ControlSnapshot[] =>
  trees.map((response) => ({ channel: IPC.collectionsList, response }));

type Log = { log(): { channel: string; args?: unknown }[] };
const calls = (control: Log, channel: string) => control.log().filter((e) => e.channel === channel);
const rowIds = (page: Page) =>
  page
    .locator('[data-testid="collection-row"]')
    .evaluateAll((els) => els.map((el) => el.getAttribute('data-id')));
const indicator = (page: Page) => page.locator('[data-testid="tree-drop-indicator"]');
const menuItem = (page: Page, id: string) => page.locator(`[data-testid="menu-item-${id}"]`);
const twisty = (row: Locator) => row.locator('[data-testid="tree-twisty"]');

test('drag a collection above another: line on its header, one call, new order', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [
      ...list(seed, treeIn([M, Z, A])),
      { channel: IPC.collectionsReorderCollections, response: null },
    ],
  });
  await openHttpMode(page);
  await expect.poll(() => rowIds(page)).toEqual([Z, A, M]);

  await dragRow(page, collectionRow(page, M), collectionRow(page, Z), 0.25);
  await expect(indicator(page)).toHaveCount(1);
  await expect(
    collectionRow(page, Z).locator('[data-testid="tree-drop-indicator"]'),
  ).toHaveAttribute('data-zone', 'before');
  await page.mouse.up();

  await expect.poll(() => calls(control, IPC.collectionsReorderCollections)).toHaveLength(1);
  expect(calls(control, IPC.collectionsReorderCollections)[0].args).toEqual({
    ...contract<object>(KEY, 'args:CollectionsService.ReorderCollections#drag'),
    ids: [M, Z, A],
  });
  await expect.poll(() => rowIds(page)).toEqual([M, Z, A]);
  await expect(indicator(page)).toHaveCount(0);
});

test('drag a collection onto a row inside an open collection: line under the last row of its block', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [...list(seed), { channel: IPC.collectionsReorderCollections, response: null }],
  });
  await openHttpMode(page);
  await twisty(collectionRow(page, Z)).click();
  await expect.poll(() => rowIds(page)).toEqual([Z, R1, R2, F, A, M]);

  await dragRow(page, collectionRow(page, M), collectionRow(page, R1), 0.5);
  await expect(indicator(page)).toHaveCount(1);
  await expect(
    collectionRow(page, F).locator('[data-testid="tree-drop-indicator"]'),
  ).toHaveAttribute('data-zone', 'after');
  await page.mouse.up();

  await expect.poll(() => calls(control, IPC.collectionsReorderCollections)).toHaveLength(1);
  expect(calls(control, IPC.collectionsReorderCollections)[0].args).toMatchObject({
    ids: [Z, M, A],
  });
});

test('an active search or Escape starts no collection reorder; a mixed selection starts none either', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [...list(seed), { channel: IPC.collectionsReorderCollections, response: null }],
  });
  await openHttpMode(page);
  await twisty(collectionRow(page, Z)).click();

  await collectionRow(page, M).click();
  await collectionRow(page, R1).click({
    modifiers: [
      await page.evaluate(() =>
        navigator.userAgent.includes('Mac') ? ('Meta' as const) : ('Control' as const),
      ),
    ],
  });
  await dragRow(page, collectionRow(page, M), collectionRow(page, Z), 0.25);
  await expect(indicator(page)).toHaveCount(0);
  await page.mouse.up();

  await collectionRow(page, M).click();
  await dragRow(page, collectionRow(page, M), collectionRow(page, Z), 0.25);
  await expect(indicator(page)).toHaveCount(1);
  await page.keyboard.press('Escape');
  await expect(indicator(page)).toHaveCount(0);
  await page.mouse.up();

  await page.click('[data-testid="toggle-search"]');
  await page.fill('[data-testid="tree-search"]', 'r');
  await expect(collectionRow(page, R1)).toBeVisible();
  await collectionRow(page, Z).click();
  await dragRow(page, collectionRow(page, Z), collectionRow(page, R2), 0.75);
  await expect(indicator(page)).toHaveCount(0);
  await page.mouse.up();
  await settleFrames(page);

  expect(calls(control, IPC.collectionsReorderCollections)).toHaveLength(0);
  expect(calls(control, IPC.collectionsMoveItems)).toHaveLength(0);
});

test('collection menu: Move up is disabled on the first; Move down, then Alt+ArrowDown, send the order', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [
      ...list(seed, treeIn([A, Z, M]), treeIn([A, M, Z])),
      { channel: IPC.collectionsReorderCollections, response: null },
    ],
  });
  await openHttpMode(page);

  await collectionRow(page, Z).click({ button: 'right' });
  await expect(menuItem(page, 'move-up')).toHaveAttribute('data-disabled', '');
  await expect(menuItem(page, 'move-down')).not.toHaveAttribute('data-disabled', '');
  await menuItem(page, 'move-down').click();
  await expect.poll(() => rowIds(page)).toEqual([A, Z, M]);
  expect(calls(control, IPC.collectionsReorderCollections)[0].args).toMatchObject({
    ids: [A, Z, M],
  });

  await collectionRow(page, Z).focus();
  await page.keyboard.press('Alt+ArrowDown');
  await expect.poll(() => rowIds(page)).toEqual([A, M, Z]);
  expect(calls(control, IPC.collectionsReorderCollections)).toHaveLength(2);
  expect(calls(control, IPC.collectionsReorderCollections)[1].args).toMatchObject({
    ids: [A, M, Z],
  });
  await expect(collectionRow(page, Z)).toBeFocused();
});

test('dragging a request above its sibling still sends MoveItems', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [...list(seed), { channel: IPC.collectionsMoveItems, response: null }],
  });
  await openHttpMode(page);
  await twisty(collectionRow(page, Z)).click();

  await dragRow(page, collectionRow(page, R2), collectionRow(page, R1), 0.25);
  await expect(indicator(page)).toHaveCount(1);
  await page.mouse.up();

  await expect.poll(() => calls(control, IPC.collectionsMoveItems)).toHaveLength(1);
  expect(calls(control, IPC.collectionsMoveItems)[0].args).toMatchObject({
    itemIds: [R2],
    collectionId: Z,
    parentId: null,
    beforeId: R1,
  });
  expect(calls(control, IPC.collectionsReorderCollections)).toHaveLength(0);
});

test('a rejected collection reorder shows the error and keeps the server order', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [
      ...list(seed, seed),
      {
        channel: IPC.collectionsReorderCollections,
        error: { code: 'E_BAD_REQUEST', message: 'That order no longer matches the list. Retry.' },
      },
    ],
  });
  await openHttpMode(page);

  await dragRow(page, collectionRow(page, M), collectionRow(page, Z), 0.25);
  await page.mouse.up();
  await expect.poll(() => calls(control, IPC.collectionsReorderCollections)).toHaveLength(1);
  await expect(page.locator('[data-testid="collections-error"]')).toBeVisible();
  await expect.poll(() => rowIds(page)).toEqual([Z, A, M]);
});

interface Env {
  id: string;
  name: string;
}
const envSeed = contract<Env[]>(KEY, 'environments#seed');
const envReordered = contract<Env[]>(KEY, 'environments#reordered').filter((e) =>
  envSeed.some((s) => s.id === e.id),
);
const envId = (name: string): string => {
  const hit = envSeed.find((e) => e.name === name);
  if (!hit) throw new Error(`no environment ${name}`);
  return hit.id;
};
const DEV = envId('dev');
const PROD = envId('prod');
const STAGE = envId('stage');
const envRows = (page: Page) => page.locator('[data-testid="environment-row"]');
const envRow = (page: Page, id: string) =>
  page.locator(`[data-testid="environment-row"][data-id="${id}"]`);
const envIds = (page: Page) =>
  envRows(page).evaluateAll((els) => els.map((el) => el.getAttribute('data-id')));

async function openEnvironments(page: Page): Promise<void> {
  await openHttpMode(page);
  await page.click('[data-testid="api-environments"]');
  await expect(envRows(page)).toHaveCount(3);
}

test('environments: dragging the grip sends the order and renders the refetched list', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [
      ...list(seed),
      { channel: IPC.variablesListEnvironments, response: envSeed },
      { channel: IPC.variablesListEnvironments, response: envReordered },
      { channel: IPC.variablesReorderEnvironments, response: null },
    ],
  });
  await openEnvironments(page);
  await expect.poll(() => envIds(page)).toEqual([DEV, PROD, STAGE]);

  const grip = (id: string) => envRow(page, id).locator('[data-testid="environment-grip"]');
  const from = await grip(STAGE).boundingBox();
  const to = await envRow(page, DEV).boundingBox();
  if (!from || !to) throw new Error('row has no box');
  await sortableHover(
    page,
    { x: from.x + from.width / 2, y: from.y + from.height / 2 },
    { x: to.x + to.width / 2, y: to.y + to.height * 0.25 },
  );
  await page.mouse.up();

  await expect.poll(() => calls(control, IPC.variablesReorderEnvironments)).toHaveLength(1);
  expect(calls(control, IPC.variablesReorderEnvironments)[0].args).toEqual({
    ...contract<object>(KEY, 'args:VariablesService.ReorderEnvironments#drag'),
    ids: [STAGE, DEV, PROD],
  });
  await expect.poll(() => envIds(page)).toEqual(envReordered.map((e) => e.id));
});

test('environments: Alt+ArrowUp on a focused grip sends one reorder', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [
      ...list(seed),
      { channel: IPC.variablesListEnvironments, response: envSeed },
      { channel: IPC.variablesReorderEnvironments, response: null },
    ],
  });
  await openEnvironments(page);

  await envRow(page, PROD).locator('[data-testid="environment-grip"]').focus();
  await page.keyboard.press('Alt+ArrowUp');
  await expect.poll(() => calls(control, IPC.variablesReorderEnvironments)).toHaveLength(1);
  expect(calls(control, IPC.variablesReorderEnvironments)[0].args).toEqual({
    ids: [PROD, DEV, STAGE],
  });
});

test('environments: while filtered the grip explains and a drag sends nothing', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [
      ...list(seed),
      { channel: IPC.variablesListEnvironments, response: envSeed },
      { channel: IPC.variablesReorderEnvironments, response: null },
    ],
  });
  await openEnvironments(page);
  await page.fill('[data-testid="environments-filter"]', 'd');
  await expect(envRows(page)).toHaveCount(2);

  const grip = envRow(page, DEV).locator('[data-testid="environment-grip"]');
  await grip.hover();
  await expect(page.getByText('Clear the filter to reorder.')).toBeVisible();

  const from = await grip.boundingBox();
  const to = await envRow(page, PROD).boundingBox();
  if (!from || !to) throw new Error('row has no box');
  await sortableHover(
    page,
    { x: from.x + from.width / 2, y: from.y + from.height / 2 },
    { x: to.x + to.width / 2, y: to.y + to.height * 0.75 },
  );
  await page.mouse.up();
  await settleFrames(page);
  expect(calls(control, IPC.variablesReorderEnvironments)).toHaveLength(0);
});
