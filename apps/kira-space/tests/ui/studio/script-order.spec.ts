import type { Locator, Page } from '@playwright/test';
import { sortableHover } from '@workbench/testing/ui/drag';
import { registerOrderScenarios } from '@workbench/testing/ui/scenarios/automationsOrder';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { modeTab } from './support/apiMode';

// P308/P381: scripts reorder within their own set (ungrouped or one collection) and collections reorder
// among themselves, by drag or Alt+arrows. The Go half is termflow TestScriptOrder; its contract
// fixes the three call shapes.

const NOW = '2026-01-01T00:00:00.000Z';

function script(id: string, collection: string, sortOrder: number) {
  return {
    id,
    kind: 'script',
    params: [],
    smart: null,
    name: `Job ${id}`,
    command: `echo ${id}`,
    workingDir: '/tmp/demo-repo',
    dirMode: 'fixed',
    useAdeDir: false,
    color: 'green',
    collection,
    schedule: null,
    savedSecrets: [],
    sortOrder,
    createdAt: NOW,
    updatedAt: NOW,
  };
}
const collection = (name: string, sortOrder: number) => ({ name, sortOrder });

const SNAPSHOT = {
  collections: [collection('X', 0), collection('Y', 1)],
  scripts: [
    script('s1', '', 0),
    script('s2', '', 1),
    script('s3', '', 2),
    script('t1', 'X', 3),
    script('t2', 'X', 4),
  ],
};
const LIST = { channel: IPC.customScriptsList, response: SNAPSHOT };

const scriptIds = (page: Page, scope: string) =>
  page
    .locator(`${scope} [data-nav-key^="script:"]`)
    .evaluateAll((els) => els.map((el) => el.getAttribute('data-nav-key')));
const groupIds = (page: Page) =>
  page
    .locator('[data-testid="script-group"]')
    .evaluateAll((els) => els.map((el) => el.getAttribute('data-name')));
const calls = (
  control: { log(): { channel: string; args?: unknown }[] },
  channel: string,
): unknown[] =>
  control
    .log()
    .filter((e) => e.channel === channel)
    .map((e) => e.args);

async function open(page: Page): Promise<void> {
  await modeTab(page, 'automations').click();
  await expect(page.locator('[data-testid="automations-panel"]')).toBeVisible();
}

// Sortable swaps from its own sampling of the pointer and, once a target is its last one, wants more
// travel than the approach gave. `landed` asserts the dragged row reached its slot; when it did not,
// the pointer leaves the target and re-enters it, as a hand would.
async function drag(
  page: Page,
  from: Locator,
  to: Locator,
  landed?: () => Promise<void>,
): Promise<void> {
  const src = await from.boundingBox();
  const dst = await to.boundingBox();
  if (!src || !dst) throw new Error('row has no box');
  const home = { x: src.x + src.width / 2, y: src.y + src.height / 2 };
  const drop = { x: dst.x + dst.width / 2, y: dst.y + dst.height * 0.25 };
  await sortableHover(page, home, drop);
  if (landed) {
    await expect(async () => {
      try {
        await landed();
      } catch (err) {
        await page.mouse.move(home.x, home.y, { steps: 5 });
        await page.mouse.move(drop.x, drop.y, { steps: 15 });
        throw err;
      }
    }).toPass({ timeout: 15_000 });
  }
  await page.mouse.up();
}

const row = (page: Page, id: string) => page.locator(`[data-testid="script-${id}"]`);
const header = (page: Page, name: string) =>
  page.locator(`[data-testid="script-group"][data-name="${name}"]`);

test('drag reorders ungrouped scripts, a collection own scripts and the collections', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [
      LIST,
      { channel: IPC.customScriptsReorder, args: { collection: '', ids: ['s3', 's1', 's2'] } },
      { channel: IPC.customScriptsReorder, args: { collection: 'X', ids: ['t2', 't1'] } },
      { channel: IPC.customScriptsReorderCollections, args: { names: ['Y', 'X'] } },
    ],
  });
  await open(page);

  await drag(page, row(page, 's3'), row(page, 's1'));
  await expect
    .poll(() => scriptIds(page, '[data-testid="script-list"] > div:first-child'))
    .toEqual(['script:s3', 'script:s1', 'script:s2']);
  expect(calls(control, IPC.customScriptsReorder)).toEqual([
    {
      ...contract<object>('script-order', 'args:CustomScriptsService.Reorder#ungrouped'),
      collection: '',
      ids: ['s3', 's1', 's2'],
    },
  ]);

  await drag(page, row(page, 't2'), row(page, 't1'));
  await expect
    .poll(() => calls(control, IPC.customScriptsReorder)[1])
    .toEqual({
      ...contract<object>('script-order', 'args:CustomScriptsService.Reorder#collection'),
      collection: 'X',
      ids: ['t2', 't1'],
    });

  await drag(page, header(page, 'Y'), header(page, 'X'), () =>
    expect(page.locator('[data-group-entry]').first()).toHaveClass(/opacity-50/, { timeout: 500 }),
  );
  await expect.poll(() => groupIds(page)).toEqual(['Y', 'X']);
  expect(calls(control, IPC.customScriptsReorderCollections)).toEqual([
    {
      ...contract<object>('script-order', 'args:CustomScriptsService.ReorderCollections'),
      names: ['Y', 'X'],
    },
  ]);
});

test('Alt+Arrow moves within the set; the first script of a collection never moves the collection', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [
      LIST,
      { channel: IPC.customScriptsReorder, args: { collection: 'X', ids: ['t2', 't1'] } },
      { channel: IPC.customScriptsReorderCollections, args: { names: ['Y', 'X'] } },
    ],
  });
  await open(page);

  await row(page, 't1').focus();
  await page.keyboard.press('Alt+ArrowUp');

  // The Alt+ArrowDown call is the barrier: exactly one reorder call proves the Alt+ArrowUp above
  // sent none.
  await page.keyboard.press('Alt+ArrowDown');
  await expect.poll(() => calls(control, IPC.customScriptsReorder)).toHaveLength(1);
  expect(calls(control, IPC.customScriptsReorderCollections)).toHaveLength(0);
  expect(await groupIds(page)).toEqual(['X', 'Y']);
  await expect(row(page, 't1')).toBeFocused();

  await header(page, 'X').focus();
  await page.keyboard.press('Alt+ArrowDown');
  await expect.poll(() => groupIds(page)).toEqual(['Y', 'X']);
  expect(calls(control, IPC.customScriptsReorderCollections)).toHaveLength(1);
});

test('an active search pauses drag', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: [LIST] });
  await open(page);

  await page.click('[data-testid="toggle-search"]');
  await page.fill('[data-testid="tree-search"]', 'echo');
  await expect(row(page, 't2')).toBeVisible();
  await drag(page, row(page, 's3'), row(page, 's1'));
  await drag(page, row(page, 't2'), row(page, 't1'));
  await expect(row(page, 's3')).toBeVisible();
  expect(calls(control, IPC.customScriptsReorder)).toHaveLength(0);
});

// P381: the Move up/down menu items; shared with Space.
registerOrderScenarios(
  { test, contract, ipc: IPC },
  { extraBase: [], modeTab: (page) => modeTab(page, 'automations') },
);
