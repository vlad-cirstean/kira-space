import type { ControlSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { modeTab } from './support/apiMode';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// TERMINAL_OPEN_OK's shape uses control.log() polling rather than call-count
// assertions. The mocked DefaultCwd below is '/home/test' — mockRuntime.ts's own boot default
// (IPC.terminalDefaultCwd) — so an unscoped terminal's title falls out of its basename, 'test'
// (tabKinds.ts's terminal kind: `s.label || basename(s.cwd) || 'Terminal'`).
//
// ScriptDialog.vue adds or edits one command; the panel header `+` is the only add control,
// a row's context menu "Edit…" the only edit entry.

const SCRIPT = {
  id: 'script-1',
  kind: 'script',
  params: [],
  smart: null,
  name: 'Dev server',
  command: 'npm run dev',
  workingDir: '/tmp/demo-repo/frontend',
  dirMode: 'fixed',
  useAdeDir: false,
  color: 'green',
  collectionId: null,
  sortOrder: 0,
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
};

const RESOLVED_DIR: ControlSnapshot = {
  channel: IPC.scriptRunsResolveDir,
  response: {
    path: SCRIPT.workingDir,
    mode: 'fixed',
    base: '',
    blocker: '',
    branch: '',
    pending: false,
  },
};

// terminalId is a client-generated UUID — no `args` here, relying on mockRuntime.ts's
// single-snapshot shortcut so any terminalOpen call resolves.
const TERMINAL_OPEN_OK: ControlSnapshot = {
  channel: IPC.terminalOpen,
  response: { shell: '/bin/zsh' },
};

function tab(page: import('@playwright/test').Page) {
  return page.locator('[data-testid="tab-strip-wrapper"] [data-testid="tab"]');
}

function dialog(page: import('@playwright/test').Page) {
  return page.locator('[data-testid="script-dialog"]');
}

async function openTerminalModule(page: import('@playwright/test').Page): Promise<void> {
  await modeTab(page, 'automations').click();
  await expect(modeTab(page, 'automations')).toHaveClass(/is-active/);
}

test('the module exists and opens', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: [] });

  await expect(modeTab(page, 'automations')).toBeVisible();
  await openTerminalModule(page);

  await expect(page.locator('[data-testid="automations-panel"]')).toBeVisible();
  await expect(page.locator('[data-testid="automations-add"]')).toBeVisible();
  await expect(page.locator('[data-testid="script-empty-add"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="automations-start"]')).toBeVisible();
});

test('an unscoped terminal opens from the tab strip at the resolved home directory', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: [TERMINAL_OPEN_OK] });

  await openTerminalModule(page);
  await page.locator('[data-testid="tab-strip-new"]').click();
  const menu = page.locator('[data-testid="context-menu"]');
  await expect(menu).toBeVisible();
  // Terminal only — no repos configured in this test (§8.3: no Claude Code, no scripts here).
  await expect(menu.locator('[data-testid^="menu-item-"]')).toHaveCount(1);
  await menu.locator('[data-testid="menu-item-new-terminal"]').click();

  const terminalTab = tab(page);
  await expect(terminalTab).toHaveCount(1);
  // mockRuntime.ts's boot default for terminalDefaultCwd is '/home/test' — basename 'test'.
  await expect(terminalTab).toContainText('test');

  await expect(page.locator('.xterm-rows')).toBeVisible();
  await expect
    .poll(() =>
      control
        .log()
        .some(
          (e) =>
            e.channel === IPC.terminalOpen &&
            (e.args as { cwd?: string; command?: string } | undefined)?.cwd === '/home/test' &&
            (e.args as { cwd?: string; command?: string } | undefined)?.command === '',
        ),
    )
    .toBe(true);

  // Staying in the Automations module's own workspace the whole time.
  await expect(modeTab(page, 'automations')).toHaveClass(/is-active/);
});

test('running a script opens a terminal titled with its name, at its own working dir', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [
      TERMINAL_OPEN_OK,
      RESOLVED_DIR,
      { channel: IPC.customScriptsList, response: { collections: [], scripts: [SCRIPT] } },
    ],
  });

  await openTerminalModule(page);
  const nameBox = await page
    .locator(`[data-testid="script-${SCRIPT.id}"]`)
    .getByText(SCRIPT.name, { exact: true })
    .boundingBox();
  const slotBox = await page
    .locator(`[data-testid="script-${SCRIPT.id}"] [data-testid="script-run-slot"]`)
    .boundingBox();
  expect(slotBox?.x ?? 0).toBeGreaterThan((nameBox?.x ?? 0) + (nameBox?.width ?? 0));
  await page.locator(`[data-testid="script-${SCRIPT.id}"] [data-testid="script-play"]`).click();

  const terminalTab = tab(page);
  await expect(terminalTab).toHaveCount(1);
  await expect(terminalTab).toContainText(SCRIPT.name);

  await expect
    .poll(() =>
      control
        .log()
        .some(
          (e) =>
            e.channel === IPC.terminalOpen &&
            (e.args as { cwd?: string; command?: string } | undefined)?.cwd === SCRIPT.workingDir &&
            (e.args as { cwd?: string; command?: string } | undefined)?.command ===
              SCRIPT.command &&
            (e.args as { scriptId?: string }).scriptId === SCRIPT.id,
        ),
    )
    .toBe(true);
});

test('the header + is the only add control and opens the dialog with the name focused', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.customScriptsList, response: { collections: [], scripts: [SCRIPT] } }],
  });

  await openTerminalModule(page);
  await expect(page.locator('[data-testid="script-empty-add"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="automations-add"]')).toHaveCount(1);
  await page.locator('[data-testid="automations-add"]').click();
  await page.locator('[data-testid="menu-item-new-script"]').click();
  await expect(dialog(page)).toBeVisible();
  await expect(dialog(page).locator('[data-testid="script-dialog-name"]')).toBeFocused();
});

test('a multiline script is saved as written with Cmd/Ctrl+Enter; the panel row shows its first line', async ({
  relaunch,
}) => {
  const MULTI = 'echo one\necho two';
  const { window: page, control } = await relaunch({
    control: [
      RESOLVED_DIR,
      {
        channel: IPC.customScriptsList,
        response: { collections: [], scripts: [{ ...SCRIPT, command: MULTI }] },
      },
      { channel: IPC.customScriptsCreate, response: SCRIPT },
    ],
  });

  await openTerminalModule(page);
  const row = page.locator(`[data-testid="script-${SCRIPT.id}"]`);
  await expect(row).toContainText('echo one…');
  await expect(row).not.toContainText('echo two');
  await row.hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(MULTI);

  await page.locator('[data-testid="automations-add"]').click();

  await page.locator('[data-testid="menu-item-new-script"]').click();
  await dialog(page).locator('[data-testid="script-dialog-name"]').fill('Two lines');
  const script = dialog(page).locator('[data-testid="script-dialog-command"]');
  await script.focus();
  await page.keyboard.type('echo one');
  await page.keyboard.press('Enter');
  await page.keyboard.type('echo two');
  const box = await script.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(150);
  // The app matches the platform chord from the page's own user agent (WebKit reports a Mac).
  const mac = await page.evaluate(() => navigator.userAgent.includes('Mac'));
  await page.keyboard.press(mac ? 'Meta+Enter' : 'Control+Enter');
  await expect
    .poll(() => control.log().find((entry) => entry.channel === IPC.customScriptsCreate)?.args)
    .toEqual({
      fields: {
        name: 'Two lines',
        command: MULTI,
        kind: 'script',
        params: [],
        schedule: null,
        smart: null,
        dirMode: 'kira',
        useAdeDir: true,
        workingDir: '',
        color: 'none',
        collectionId: null,
      },
      secretValues: null,
    });
  await expect(dialog(page)).toHaveCount(0);
});

test('scripts group under collapsible collection rows; empty collections show', async ({
  relaunch,
}) => {
  const collections = [
    {
      id: 'c-be',
      name: 'Backend',
      sortOrder: 0,
      createdAt: SCRIPT.createdAt,
      updatedAt: SCRIPT.createdAt,
    },
    {
      id: 'c-web',
      name: 'Web',
      sortOrder: 1,
      createdAt: SCRIPT.createdAt,
      updatedAt: SCRIPT.createdAt,
    },
    {
      id: 'c-empty',
      name: 'Empty',
      sortOrder: 2,
      createdAt: SCRIPT.createdAt,
      updatedAt: SCRIPT.createdAt,
    },
  ];
  const scripts = [
    { ...SCRIPT, id: 's-free', name: 'Loose', collectionId: null },
    { ...SCRIPT, id: 's-a1', name: 'Build', collectionId: 'c-be' },
    { ...SCRIPT, id: 's-a2', name: 'Test', collectionId: 'c-be' },
    { ...SCRIPT, id: 's-b1', name: 'Serve', collectionId: 'c-web' },
  ];
  const { window: page } = await relaunch({
    control: [{ channel: IPC.customScriptsList, response: { collections, scripts } }],
  });

  await openTerminalModule(page);
  const group = (id: string) => page.locator(`[data-testid="script-group"][data-id="${id}"]`);
  await expect(group('c-be')).toContainText('2');
  await expect(group('c-web')).toContainText('1');
  await expect(group('c-empty')).toContainText('0');
  await expect(page.locator('[data-testid="script-list"] [data-testid^="script-s-"]')).toHaveCount(
    4,
  );

  await expect(page.locator('[data-testid="script-s-free"]')).toHaveCSS('text-align', 'left');

  await group('c-be').locator('button:has-text("Backend")').click();
  await expect(page.locator('[data-testid="script-s-a1"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="script-s-b1"]')).toBeVisible();
  await group('c-be').locator('button:has-text("Backend")').click();
  await expect(page.locator('[data-testid="script-s-a1"]')).toBeVisible();

  await page.locator('[data-testid="toggle-search"]').click();
  await page.locator('[data-testid="tree-search"]').fill('serve');
  await expect(group('c-be')).toHaveCount(0);
  await expect(group('c-empty')).toHaveCount(0);
  await expect(page.locator('[data-testid="script-s-b1"]')).toBeVisible();
});

test('the dialog adds a script into a collection chosen from a select', async ({ relaunch }) => {
  const collection = {
    id: 'c-be',
    name: 'Backend',
    sortOrder: 0,
    createdAt: SCRIPT.createdAt,
    updatedAt: SCRIPT.createdAt,
  };
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.customScriptsList, response: { collections: [collection], scripts: [] } },
      { channel: IPC.customScriptsCreate, response: SCRIPT },
    ],
  });

  await openTerminalModule(page);
  await page.locator('[data-testid="automations-add"]').click();
  await page.locator('[data-testid="menu-item-new-script"]').click();
  const select = dialog(page).locator('[data-testid="script-dialog-collection"]');
  await expect(select.locator('option')).toHaveText(['No collection', 'Backend']);
  await dialog(page).locator('[data-testid="script-dialog-name"]').fill('Lint');
  await dialog(page).locator('[data-testid="script-dialog-command"]').fill('bun run lint');
  await select.selectOption('c-be');
  await dialog(page).locator('[data-testid="script-dialog-save"]').click();
  await expect
    .poll(() => control.log().find((entry) => entry.channel === IPC.customScriptsCreate)?.args)
    .toEqual({
      fields: {
        name: 'Lint',
        command: 'bun run lint',
        kind: 'script',
        params: [],
        schedule: null,
        smart: null,
        dirMode: 'kira',
        useAdeDir: true,
        workingDir: '',
        color: 'none',
        collectionId: 'c-be',
      },
      secretValues: null,
    });
});

test('header New collection creates one and names it inline', async ({ relaunch }) => {
  const created = {
    id: 'c-new',
    name: 'New collection',
    sortOrder: 0,
    createdAt: SCRIPT.createdAt,
    updatedAt: SCRIPT.createdAt,
  };
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.customScriptsCreateCollection, response: created },
      { channel: IPC.customScriptsRenameCollection, response: null },
    ],
  });

  await openTerminalModule(page);
  await page.locator('[data-testid="automations-new-collection"]').click();
  await emitWailsEvent(page, IPC.customScriptsChanged, { collections: [created], scripts: [] });
  const input = page.locator('[data-testid="script-collection-rename-input"]');
  await expect(input).toBeFocused();
  await input.fill('Backend');
  await input.press('Enter');
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.customScriptsRenameCollection)?.args)
    .toEqual({ id: 'c-new', name: 'Backend' });
  await expect(input).toHaveCount(0);
});

test('Escape cancels an inline collection rename without renaming', async ({ relaunch }) => {
  const created = {
    id: 'c-esc',
    name: 'New collection',
    sortOrder: 0,
    createdAt: SCRIPT.createdAt,
    updatedAt: SCRIPT.createdAt,
  };
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.customScriptsCreateCollection, response: created },
      { channel: IPC.customScriptsRenameCollection, response: null },
    ],
  });

  await openTerminalModule(page);
  await page.locator('[data-testid="automations-new-collection"]').click();
  await emitWailsEvent(page, IPC.customScriptsChanged, { collections: [created], scripts: [] });
  const input = page.locator('[data-testid="script-collection-rename-input"]');
  await expect(input).toBeFocused();
  await input.fill('Discarded');
  await input.press('Escape');
  await expect(input).toHaveCount(0);
  await expect(page.locator('[data-testid="script-group"][data-id="c-esc"]')).toContainText(
    'New collection',
  );
  expect(control.log().some((e) => e.channel === IPC.customScriptsRenameCollection)).toBe(false);
});

test('the collection menu Delete confirms and deletes it', async ({ relaunch }) => {
  const collection = {
    id: 'c-be',
    name: 'Backend',
    sortOrder: 0,
    createdAt: SCRIPT.createdAt,
    updatedAt: SCRIPT.createdAt,
  };
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.customScriptsList, response: { collections: [collection], scripts: [] } },
      { channel: IPC.customScriptsDeleteCollection, response: null },
    ],
  });

  await openTerminalModule(page);
  await page.locator('[data-testid="script-group"][data-id="c-be"]').click({ button: 'right' });
  await page.locator('[data-testid="menu-item-delete"]').click();
  await expect(page.locator('[data-testid="confirm-dialog-message"]')).toHaveText(
    'Delete “Backend” and everything inside it.',
  );
  await page.locator('[data-testid="confirm-dialog-confirm"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.customScriptsDeleteCollection)?.args)
    .toEqual({ id: 'c-be' });
});

test('contract: a script moves between collections from its context menu', async ({ relaunch }) => {
  const collections = [
    {
      id: 'c-be',
      name: 'Backend',
      sortOrder: 0,
      createdAt: SCRIPT.createdAt,
      updatedAt: SCRIPT.createdAt,
    },
    {
      id: 'c-web',
      name: 'Web',
      sortOrder: 1,
      createdAt: SCRIPT.createdAt,
      updatedAt: SCRIPT.createdAt,
    },
  ];
  const script = { ...SCRIPT, collectionId: 'c-be' };
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.customScriptsList, response: { collections, scripts: [script] } },
      { channel: IPC.customScriptsMove, response: null },
      { channel: IPC.customScriptsCreateCollection, response: collections[1] },
    ],
  });
  const toCollection = contract<object>(
    'script-collections',
    'args:CustomScriptsService.Move#to-collection',
  );
  const toNone = contract<object>('script-collections', 'args:CustomScriptsService.Move#to-none');
  const moves = () =>
    control
      .log()
      .filter((e) => e.channel === IPC.customScriptsMove)
      .map((e) => e.args);
  const openMoveMenu = async () => {
    await page.locator(`[data-testid="script-${script.id}"]`).click({ button: 'right' });
    await page.locator('[data-testid="menu-item-move-to-collection"]').hover();
  };

  await openTerminalModule(page);
  await openMoveMenu();
  await expect(page.locator('[data-testid="menu-item-move-to-c-be"]')).toHaveAttribute(
    'data-disabled',
    '',
  );
  await page.locator('[data-testid="menu-item-move-to-c-web"]').click();
  await expect.poll(moves).toEqual([{ ...toCollection, id: script.id, collectionId: 'c-web' }]);

  await openMoveMenu();
  await page.locator('[data-testid="menu-item-move-to-none"]').click();
  await expect.poll(moves).toEqual([
    { ...toCollection, id: script.id, collectionId: 'c-web' },
    { ...toNone, id: script.id, collectionId: null },
  ]);

  await openMoveMenu();
  await page.locator('[data-testid="menu-item-move-to-new"]').click();
  await expect
    .poll(() => control.log().some((e) => e.channel === IPC.customScriptsCreateCollection))
    .toBe(true);
  await expect
    .poll(() => moves().at(-1))
    .toEqual({ ...toCollection, id: script.id, collectionId: 'c-web' });
  await expect(page.locator('[data-testid="script-collection-rename-input"]')).toBeFocused();
});

const CHOOSE = '[data-testid="script-dialog-workingdir-choose"]';

async function pickFixed(page: import('@playwright/test').Page): Promise<void> {
  await dialog(page).locator('[data-testid="script-dialog-dir-fixed"]').click();
}

test('Choose… fills the folder from the folder dialog', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [
      RESOLVED_DIR,
      { channel: IPC.filesChooseFolder, response: { canceled: false, path: '/tmp/picked' } },
    ],
  });

  await openTerminalModule(page);
  await page.locator('[data-testid="automations-add"]').click();
  await page.locator('[data-testid="menu-item-new-script"]').click();
  await pickFixed(page);
  await dialog(page).locator(CHOOSE).click();
  await expect(dialog(page).locator('[data-testid="script-dialog-workingdir"]')).toHaveText(
    '/tmp/picked',
  );
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.filesChooseFolder)?.args)
    .toEqual({ title: 'Choose a run folder' });
});

test('a cancelled folder dialog leaves the folder unchanged', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [
      RESOLVED_DIR,
      { channel: IPC.filesChooseFolder, response: { canceled: true, path: null } },
    ],
  });

  await openTerminalModule(page);
  await page.locator('[data-testid="automations-add"]').click();
  await page.locator('[data-testid="menu-item-new-script"]').click();
  await pickFixed(page);
  await dialog(page).locator(CHOOSE).click();
  await expect
    .poll(() => control.log().some((e) => e.channel === IPC.filesChooseFolder))
    .toBe(true);
  await expect(dialog(page).locator('[data-testid="script-dialog-workingdir"]')).toHaveText(
    'No folder chosen',
  );
});

test('Add stays disabled until name and script are filled; it sends the trimmed fields', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [
      RESOLVED_DIR,
      { channel: IPC.filesChooseFolder, response: { canceled: false, path: SCRIPT.workingDir } },
      { channel: IPC.customScriptsList, response: { collections: [], scripts: [] } },
      { channel: IPC.customScriptsCreate, response: SCRIPT },
    ],
  });

  await openTerminalModule(page);
  await page.locator('[data-testid="automations-add"]').click();
  await page.locator('[data-testid="menu-item-new-script"]').click();
  await expect(dialog(page)).toBeVisible();

  const save = dialog(page).locator('[data-testid="script-dialog-save"]');
  const created = () => control.log().filter((e) => e.channel === IPC.customScriptsCreate);
  await save.click();
  await expect(dialog(page).locator('[data-testid="script-dialog-name-error"]')).toBeVisible();
  await expect(dialog(page).locator('[data-testid="script-dialog-name"]')).toBeFocused();
  expect(created()).toHaveLength(0);

  await dialog(page).locator('[data-testid="script-dialog-name"]').fill(`  ${SCRIPT.name}  `);
  await save.click();
  await expect(dialog(page).locator('[data-testid="script-dialog-name-error"]')).toHaveCount(0);
  expect(created()).toHaveLength(0);

  await dialog(page).locator('[data-testid="script-dialog-command"]').fill(`  ${SCRIPT.command}  `);
  await pickFixed(page);
  await dialog(page).locator(CHOOSE).click();
  await dialog(page).locator('[data-testid="color-green"]').click();
  await expect(save).toBeEnabled();
  await save.click();

  await expect
    .poll(() => control.log().find((entry) => entry.channel === IPC.customScriptsCreate)?.args)
    .toEqual({
      fields: {
        name: SCRIPT.name,
        command: SCRIPT.command,
        kind: 'script',
        params: [],
        schedule: null,
        smart: null,
        dirMode: 'fixed',
        useAdeDir: true,
        workingDir: SCRIPT.workingDir,
        color: 'green',
        collectionId: null,
      },
      secretValues: null,
    });
});

test('a rejected folder keeps the dialog open and shows the backend error', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [
      RESOLVED_DIR,
      { channel: IPC.filesChooseFolder, response: { canceled: false, path: 'relative/dir' } },
      { channel: IPC.customScriptsList, response: { collections: [], scripts: [] } },
      {
        channel: IPC.customScriptsCreate,
        error: {
          code: 'E_BAD_REQUEST',
          message: 'scripts: working directory must be an absolute path',
        },
      },
    ],
  });

  await openTerminalModule(page);
  await page.locator('[data-testid="automations-add"]').click();
  await page.locator('[data-testid="menu-item-new-script"]').click();
  await dialog(page).locator('[data-testid="script-dialog-name"]').fill(SCRIPT.name);
  await dialog(page).locator('[data-testid="script-dialog-command"]').fill(SCRIPT.command);
  await pickFixed(page);
  await dialog(page).locator(CHOOSE).click();
  await dialog(page).locator('[data-testid="script-dialog-save"]').click();

  await expect(dialog(page).locator('[data-testid="script-dialog-error"]')).toHaveText(
    'scripts: working directory must be an absolute path',
  );
  await expect(dialog(page)).toBeVisible();
});

test('Edit… opens the dialog filled in; Save sends every edited field in one update', async ({
  relaunch,
}) => {
  const editedName = `${SCRIPT.name} edited`;
  const { window: page, control } = await relaunch({
    control: [
      RESOLVED_DIR,
      { channel: IPC.customScriptsList, response: { collections: [], scripts: [SCRIPT] } },
      { channel: IPC.customScriptsUpdate, response: { ...SCRIPT, name: editedName } },
    ],
  });

  await openTerminalModule(page);
  await page.locator(`[data-testid="script-${SCRIPT.id}"]`).click({ button: 'right' });
  await page.locator('[data-testid="menu-item-edit"]').click();
  await expect(dialog(page)).toBeVisible();

  const name = dialog(page).locator('[data-testid="script-dialog-name"]');
  await expect(name).toHaveValue(SCRIPT.name);
  await expect(dialog(page).locator('[data-testid="script-dialog-command"]')).toHaveValue(
    SCRIPT.command,
  );
  await expect(dialog(page).locator('[data-testid="script-dialog-workingdir"]')).toHaveText(
    SCRIPT.workingDir,
  );

  await name.fill(`  ${editedName}  `);
  await dialog(page).locator('[data-testid="color-blue"]').click();
  await dialog(page).locator('[data-testid="script-dialog-save"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.customScriptsUpdate)?.args)
    .toEqual({
      id: SCRIPT.id,
      fields: {
        name: editedName,
        command: SCRIPT.command,
        kind: 'script',
        params: [],
        schedule: null,
        smart: null,
        dirMode: 'fixed',
        useAdeDir: false,
        workingDir: SCRIPT.workingDir,
        color: 'blue',
        collectionId: null,
      },
      secretValues: null,
    });
});

// Contract terminal. Backend half: termflow TestShellTab and TestScriptInPickedFolder.
test('contract: an unscoped terminal opens at the default directory', async ({ relaunch }) => {
  const home = contract<{ path: string }>('terminal', 'TerminalService.DefaultCwd');
  const opened = contract<{ cwd: string; command: string }>(
    'terminal',
    'args:TerminalService.Open',
  );
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.terminalDefaultCwd, response: home }, TERMINAL_OPEN_OK],
  });

  await openTerminalModule(page);
  await page.locator('[data-testid="tab-strip-new"]').click();
  await page.locator('[data-testid="menu-item-new-terminal"]').click();
  await expect
    .poll(
      () =>
        control.log().find((e) => e.channel === IPC.terminalOpen)?.args as
          | { cwd?: string; command?: string }
          | undefined,
    )
    .toMatchObject({ cwd: opened.cwd, command: opened.command });
});

test('contract: Choose… fills a fixed folder and the command saves it', async ({ relaunch }) => {
  const picked = contract<{ canceled: boolean; path: string }>(
    'terminal',
    'FilesService.ChooseFolder#picked',
  );
  const saved = contract<{ name: string; command: string; dirMode: string; workingDir: string }>(
    'terminal',
    'CustomScriptsService.Create#fixed',
  );
  const { window: page, control } = await relaunch({
    control: [
      RESOLVED_DIR,
      { channel: IPC.filesChooseFolder, response: picked },
      { channel: IPC.customScriptsCreate, response: { ...SCRIPT, ...saved } },
    ],
  });

  await openTerminalModule(page);
  await page.locator('[data-testid="automations-add"]').click();
  await page.locator('[data-testid="menu-item-new-script"]').click();
  await dialog(page).locator('[data-testid="script-dialog-name"]').fill(saved.name);
  await dialog(page).locator('[data-testid="script-dialog-command"]').fill(saved.command);
  await pickFixed(page);
  await dialog(page).locator(CHOOSE).click();
  await expect(dialog(page).locator('[data-testid="script-dialog-workingdir"]')).toHaveText(
    saved.workingDir,
  );
  await dialog(page).locator('[data-testid="script-dialog-save"]').click();
  await expect
    .poll(
      () =>
        (
          control.log().find((e) => e.channel === IPC.customScriptsCreate)?.args as {
            fields?: Record<string, unknown>;
          }
        )?.fields,
    )
    .toMatchObject({
      dirMode: saved.dirMode,
      workingDir: saved.workingDir,
      command: saved.command,
    });
});

test('contract: a cancelled folder dialog returns no path', async ({ relaunch }) => {
  const cancelled = contract('terminal', 'FilesService.ChooseFolder#cancelled');
  const { window: page } = await relaunch({
    control: [RESOLVED_DIR, { channel: IPC.filesChooseFolder, response: cancelled }],
  });
  await openTerminalModule(page);
  await page.locator('[data-testid="automations-add"]').click();
  await page.locator('[data-testid="menu-item-new-script"]').click();
  await pickFixed(page);
  await dialog(page).locator(CHOOSE).click();
  await expect(dialog(page).locator('[data-testid="script-dialog-workingdir"]')).toHaveText(
    'No folder chosen',
  );
});
