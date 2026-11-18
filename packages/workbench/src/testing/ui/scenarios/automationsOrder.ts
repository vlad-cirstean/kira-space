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
  const { test, ipc: IPC } = env;

  const LIST: ControlSnapshot = {
    channel: IPC.customScriptsList,
    response: SNAPSHOT,
  };
  const BASE = [...options.extraBase, LIST];
  async function openPanel(page: Page): Promise<void> {
    await options.modeTab(page).click();
    await expect(page.locator('[data-testid="automations-panel"]')).toBeVisible();
  }

  test('script and collection menus have no Move items; Alt+ArrowDown moves nothing', async ({
    relaunch,
  }) => {
    const { window: page, control } = await relaunch({ control: BASE });
    await openPanel(page);

    await row(page, 's1').click({ button: 'right' });
    await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
    await expect(menuItem(page, 'move-to-collection')).toBeVisible();
    await expect(menuItem(page, 'remove')).toBeVisible();
    await expect(menuItem(page, 'move-up')).toHaveCount(0);
    await expect(menuItem(page, 'move-down')).toHaveCount(0);
    await page.keyboard.press('Escape');

    await header(page, 'X').click({ button: 'right' });
    await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
    await expect(menuItem(page, 'delete')).toBeVisible();
    await expect(menuItem(page, 'move-up')).toHaveCount(0);
    await expect(menuItem(page, 'move-down')).toHaveCount(0);
    await page.keyboard.press('Escape');
    await expect(page.locator('[data-testid="context-menu"]')).toHaveCount(0);

    await row(page, 's1').focus();
    await page.keyboard.press('Alt+ArrowDown');
    await header(page, 'X').focus();
    await page.keyboard.press('Alt+ArrowDown');
    // Key events run in order, so a settled order proves both chords were handled.
    await expect(row(page, 's1')).toBeVisible();
    expect(calls(control, IPC.customScriptsReorder)).toHaveLength(0);
    expect(calls(control, IPC.customScriptsReorderCollections)).toHaveLength(0);
    expect(await groupIds(page)).toEqual(['X', 'Y']);
  });
}
