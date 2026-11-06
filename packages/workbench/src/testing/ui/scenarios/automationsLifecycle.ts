import { expect, type Page } from '@playwright/test';
import { type ControlSnapshot, emitWailsEvent } from '../mockRuntime';
import type { ScenarioEnv } from './env';

type Env = ScenarioEnv<
  | 'customScriptsChanged'
  | 'customScriptsDeleteCollection'
  | 'customScriptsList'
  | 'customScriptsMove'
  | 'customScriptsRenameCollection'
>;

export interface Options {
  extraBase: readonly ControlSnapshot[];
  modeTab: (page: Page) => ReturnType<Page['locator']>;
}

const NOW = '2026-01-01T00:00:00.000Z';

function run(id: string, collection: string, sortOrder: number) {
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
  dir: '/tmp/runs',
  problems: [],
  collections: [collection('A', 0), collection('B', 1)],
  scripts: [run('u1', '', 0), run('a1', 'A', 1), run('a2', 'A', 2), run('b1', 'B', 3)],
};

const row = (page: Page, id: string) => page.locator(`[data-testid="script-${id}"]`);
const header = (page: Page, name: string) =>
  page.locator(`[data-testid="script-group"][data-name="${name}"]`);
const menuItem = (page: Page, id: string) => page.locator(`[data-testid="menu-item-${id}"]`);
const calls = (control: { log(): { channel: string; args?: unknown }[] }, channel: string) =>
  control
    .log()
    .filter((e) => e.channel === channel)
    .map((e) => e.args);

// Run collections are names in run files. Backend half: termflow TestRunsLifecycle stores the runs-lifecycle contract.
export function registerLifecycleScenarios(env: Env, options: Options): void {
  const { test, contract, ipc: IPC } = env;
  const args = (name: string) =>
    contract<object>('runs-lifecycle', `args:CustomScriptsService.${name}`);

  const control = [
    ...options.extraBase,
    { channel: IPC.customScriptsList, response: SNAPSHOT },
    { channel: IPC.customScriptsMove },
    { channel: IPC.customScriptsRenameCollection },
    { channel: IPC.customScriptsDeleteCollection },
  ];
  async function openPanel(page: Page): Promise<void> {
    await options.modeTab(page).click();
    await expect(page.locator('[data-testid="automations-panel"]')).toBeVisible();
  }

  test('a collection header renames inline through its menu', async ({ relaunch }) => {
    const { window: page, control: calllog } = await relaunch({ control });
    await openPanel(page);

    await header(page, 'A').click({ button: 'right' });
    await menuItem(page, 'rename').click();
    const input = page.locator('[data-testid="script-collection-rename-input"]');
    await expect(input).toBeVisible();
    await input.fill('Ops');
    await input.press('Enter');
    await expect
      .poll(() => calls(calllog, IPC.customScriptsRenameCollection))
      .toEqual([{ ...args('RenameCollection'), name: 'A', newName: 'Ops' }]);
    await expect(input).toHaveCount(0);
  });

  test('deleting a collection asks first and names it', async ({ relaunch }) => {
    const { window: page, control: calllog } = await relaunch({ control });
    await openPanel(page);

    await header(page, 'B').click({ button: 'right' });
    await menuItem(page, 'delete').click();
    await expect(page.locator('[data-testid="confirm-dialog-message"]')).toContainText('“B”');
    await page.click('[data-testid="confirm-dialog-cancel"]');
    expect(calls(calllog, IPC.customScriptsDeleteCollection)).toHaveLength(0);

    await header(page, 'B').click({ button: 'right' });
    await menuItem(page, 'delete').click();
    await page.click('[data-testid="confirm-dialog-confirm"]');
    await expect
      .poll(() => calls(calllog, IPC.customScriptsDeleteCollection))
      .toEqual([{ ...args('DeleteCollection'), name: 'B' }]);
  });

  test('Move to collection and Move to new collection from the run menu', async ({ relaunch }) => {
    const { window: page, control: calllog } = await relaunch({ control });
    await openPanel(page);

    await row(page, 'u1').click({ button: 'right' });
    await menuItem(page, 'move-to-collection').hover();
    await page
      .locator(`[data-testid="context-submenu"] [data-testid="menu-item-move-to-B"]`)
      .click();
    await expect
      .poll(() => calls(calllog, IPC.customScriptsMove))
      .toEqual([{ ...args('Move'), id: 'u1', collection: 'B', beforeId: null }]);

    await row(page, 'a1').click({ button: 'right' });
    await menuItem(page, 'move-to-collection').hover();
    await page
      .locator(`[data-testid="context-submenu"] [data-testid="menu-item-move-to-new"]`)
      .click();
    await expect
      .poll(() => calls(calllog, IPC.customScriptsMove)[1])
      .toEqual({ ...args('Move'), id: 'a1', collection: 'New collection', beforeId: null });

    // The file change comes back as a snapshot; the new collection then shows its name field.
    await emitWailsEvent(page, IPC.customScriptsChanged, {
      ...SNAPSHOT,
      collections: [...SNAPSHOT.collections, collection('New collection', 2)],
      scripts: [
        run('u1', '', 0),
        run('a1', 'New collection', 1),
        run('a2', 'A', 2),
        run('b1', 'B', 3),
      ],
    });
    await expect(page.locator('[data-testid="script-collection-rename-input"]')).toBeVisible();
  });

  test('dragging a run onto a collection moves it into that collection at the drop position', async ({
    relaunch,
  }) => {
    const { window: page, control: calllog } = await relaunch({ control });
    await openPanel(page);

    const src = await row(page, 'u1').boundingBox();
    const dst = await row(page, 'b1').boundingBox();
    if (!src || !dst) throw new Error('row has no box');
    const home = { x: src.x + src.width / 2, y: src.y + src.height / 2 };
    const end = { x: dst.x + dst.width / 2, y: dst.y + dst.height * 0.25 };
    await page.mouse.move(home.x, home.y);
    await page.mouse.down();
    await page.mouse.move(home.x, home.y + 10, { steps: 5 });
    await page.mouse.move(end.x, end.y, { steps: 15 });
    // Sortable decides a cross-list insert from its own pointer sampling: nudge until it landed.
    let nudge = 0;
    let ids: (string | null)[] = [];
    await expect(async () => {
      nudge += 1;
      await page.mouse.move(end.x + (nudge % 4), end.y, { steps: 2 });
      await page.waitForTimeout(80);
      ids = await page
        .locator('[data-group-entry]:last-child [data-nav-key^="script:"]')
        .evaluateAll((els) => els.map((el) => el.getAttribute('data-nav-key')));
      expect(ids).toContain('script:u1');
    }).toPass({ timeout: 10_000 });
    // The drop lands where the placeholder sits: before the run that follows it.
    const next = ids[ids.indexOf('script:u1') + 1]?.replace('script:', '') ?? null;
    await page.mouse.up();
    await expect
      .poll(() => calls(calllog, IPC.customScriptsMove))
      .toEqual([{ ...args('Move'), id: 'u1', collection: 'B', beforeId: next }]);
  });
}
