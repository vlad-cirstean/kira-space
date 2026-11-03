import { expect, type Locator, type Page } from '@playwright/test';
import type { ControlSnapshot } from '../mockRuntime';
import type { ScenarioEnv } from './env';

type Env = ScenarioEnv<
  'customScriptsList' | 'customScriptsReorder' | 'customScriptsReorderCollections'
>;

export interface Options {
  extraBase: readonly ControlSnapshot[];
  modeTab: (page: Page) => Locator;
}

const NOW = '2026-01-01T00:00:00.000Z';

function scenarioScript(id: string, collection: string, sortOrder: number) {
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
const scenarioCollection = (name: string, sortOrder: number) => ({ name, sortOrder });

const SNAPSHOT = {
  collections: [scenarioCollection('X', 0), scenarioCollection('Y', 1)],
  scripts: [
    scenarioScript('s1', '', 0),
    scenarioScript('s2', '', 1),
    scenarioScript('s3', '', 2),
    scenarioScript('t1', 'X', 3),
    scenarioScript('t2', 'X', 4),
  ],
};

const groupIds = (page: Page) =>
  page
    .locator('[data-testid="script-group"]')
    .evaluateAll((els) => els.map((el) => el.getAttribute('data-name')));
const calls = (control: { log(): { channel: string; args?: unknown }[] }, channel: string) =>
  control
    .log()
    .filter((e) => e.channel === channel)
    .map((e) => e.args);
const row = (page: Page, id: string) => page.locator(`[data-testid="script-${id}"]`);
const header = (page: Page, name: string) =>
  page.locator(`[data-testid="script-group"][data-name="${name}"]`);
const menuItem = (page: Page, id: string) => page.locator(`[data-testid="menu-item-${id}"]`);

// Backend half: termflow TestScriptOrder (Reorder, ReorderCollections); its contract fixes the call
// shapes.
export function registerOrderScenarios(env: Env, options: Options): void {
  const { test, contract, ipc: IPC } = env;

  const LIST: ControlSnapshot = {
    channel: IPC.customScriptsList,
    response: SNAPSHOT,
  };
  const BASE = [...options.extraBase, LIST];
  async function openPanel(page: Page): Promise<void> {
    await options.modeTab(page).click();
    await expect(page.locator('[data-testid="automations-panel"]')).toBeVisible();
  }

  test('menu Move up and Move down reorder a script and a collection, disabled at the ends', async ({
    relaunch,
  }) => {
    const { window: page, control } = await relaunch({
      control: [
        ...BASE,
        {
          channel: IPC.customScriptsReorder,
          args: { collection: '', ids: ['s2', 's1', 's3'] },
        },
        {
          channel: IPC.customScriptsReorderCollections,
          args: { names: ['Y', 'X'] },
        },
      ],
    });
    await openPanel(page);

    await row(page, 's1').click({ button: 'right' });
    await expect(menuItem(page, 'move-up')).toHaveAttribute('data-disabled', '');
    await expect(menuItem(page, 'move-down')).not.toHaveAttribute('data-disabled', '');
    await menuItem(page, 'move-down').click();
    await expect
      .poll(() => calls(control, IPC.customScriptsReorder))
      .toEqual([
        {
          ...contract<object>('script-order', 'args:CustomScriptsService.Reorder#ungrouped'),
          collection: '',
          ids: ['s2', 's1', 's3'],
        },
      ]);

    await row(page, 's3').click({ button: 'right' });
    await expect(menuItem(page, 'move-down')).toHaveAttribute('data-disabled', '');
    await page.keyboard.press('Escape');

    await header(page, 'Y').click({ button: 'right' });
    await expect(menuItem(page, 'move-down')).toHaveAttribute('data-disabled', '');
    await page.keyboard.press('Escape');

    await header(page, 'X').click({ button: 'right' });
    await expect(menuItem(page, 'move-up')).toHaveAttribute('data-disabled', '');
    await menuItem(page, 'move-down').click();
    await expect
      .poll(() => calls(control, IPC.customScriptsReorderCollections))
      .toEqual([
        {
          ...contract<object>('script-order', 'args:CustomScriptsService.ReorderCollections'),
          names: ['Y', 'X'],
        },
      ]);
    await expect.poll(() => groupIds(page)).toEqual(['Y', 'X']);
  });

  test('an active search disables the menu moves', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: BASE });
    await openPanel(page);

    await page.click('[data-testid="toggle-search"]');
    await page.fill('[data-testid="tree-search"]', 'echo');
    await expect(row(page, 't2')).toBeVisible();
    await row(page, 's2').click({ button: 'right' });
    await expect(menuItem(page, 'move-up')).toHaveAttribute('data-disabled', '');
    await expect(menuItem(page, 'move-down')).toHaveAttribute('data-disabled', '');
  });
}
