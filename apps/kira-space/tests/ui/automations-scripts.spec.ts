import type { Page } from '@playwright/test';
import { scriptsSnapshotSchema } from '@shared/domain/scripts';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

const SCRIPT = {
  id: 'script-1',
  kind: 'script',
  params: [],
  smart: null,
  name: 'Build all',
  command: 'echo one\necho two',
  workingDir: '',
  dirMode: 'kira',
  useAdeDir: false,
  color: 'none',
  collectionId: null,
  sortOrder: 0,
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
};

const TERMINAL_OPEN_OK: ControlSnapshot = {
  channel: IPC.terminalOpen,
  response: { shell: '/bin/zsh' },
};

const RESOLVED_DIR: ControlSnapshot = {
  channel: IPC.scriptRunsResolveDir,
  response: {
    path: '/kira/automations/script-1',
    mode: 'kira',
    base: '/kira',
    blocker: '',
    branch: '',
    pending: false,
  },
};

function modeTab(page: Page) {
  return page.locator('[data-testid="mode-tab"][data-mode="automations"]');
}

test('the header + adds a two-line script, sent with its newline', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.windowsEnsure, response: { mode: 'automations' } },
      TERMINAL_OPEN_OK,
      RESOLVED_DIR,
      { channel: IPC.customScriptsCreate, response: SCRIPT },
    ],
  });
  await expect(modeTab(page)).toHaveClass(/is-active/);
  await expect(page.locator('[data-testid="automations-panel"]')).toContainText('No scripts');

  await page.locator('[data-testid="automations-add"]').click();

  await page.locator('[data-testid="menu-item-new-script"]').click();
  const dialog = page.locator('[data-testid="script-dialog"]');
  await expect(dialog).toBeVisible();
  await dialog.locator('[data-testid="script-dialog-name"]').fill(SCRIPT.name);
  const script = dialog.locator('[data-testid="script-dialog-command"]');
  await script.focus();
  await page.keyboard.type('echo one');
  await page.keyboard.press('Enter');
  await page.keyboard.type('echo two');
  await dialog.locator('[data-testid="script-dialog-save"]').click();

  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.customScriptsCreate)?.args)
    .toEqual({
      fields: {
        name: SCRIPT.name,
        command: SCRIPT.command,
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
});

test('clicking a script opens a terminal tab titled with its name', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.windowsEnsure, response: { mode: 'automations' } },
      TERMINAL_OPEN_OK,
      RESOLVED_DIR,
      { channel: IPC.customScriptsList, response: { collections: [], scripts: [SCRIPT] } },
    ],
  });

  const row = page.locator(`[data-testid="script-${SCRIPT.id}"]`);
  await expect(row).toContainText('echo one…');
  await row.locator('[data-testid="script-play"]').click();

  await expect(page.locator('[data-testid="tab-strip-wrapper"] [data-testid="tab"]')).toContainText(
    SCRIPT.name,
  );
  await expect
    .poll(() =>
      control
        .log()
        .some(
          (e) =>
            e.channel === IPC.terminalOpen &&
            (e.args as { command?: string; scriptId?: string } | undefined)?.command ===
              SCRIPT.command &&
            (e.args as { scriptId?: string }).scriptId === SCRIPT.id,
        ),
    )
    .toBe(true);
});

test('contract: Choose folder… switches the script to a fixed folder', async ({ relaunch }) => {
  const fixed = contract<{ dirMode: string }>(
    'script-folders',
    'CustomScriptsService.Create#fixed',
  );
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.windowsEnsure, response: { mode: 'automations' } },
      RESOLVED_DIR,
      { channel: IPC.filesChooseFolder, response: { canceled: false, path: '/tmp/picked' } },
      {
        channel: IPC.customScriptsCreate,
        response: { ...SCRIPT, dirMode: fixed.dirMode, workingDir: '/tmp/picked' },
      },
    ],
  });
  await page.locator('[data-testid="automations-add"]').click();
  await page.locator('[data-testid="menu-item-new-script"]').click();
  const dialog = page.locator('[data-testid="script-dialog"]');
  const preview = dialog.locator('[data-testid="script-dialog-dir-preview"]');
  const folder = preview.locator('[data-chip="path"][data-var="folder"]');
  await expect(folder).toContainText('/kira');
  await expect(folder).toContainText('Set on save');

  await dialog.locator('[data-testid="script-dialog-dir-fixed"]').click();
  await dialog.locator('[data-testid="script-dialog-workingdir-choose"]').click();
  await expect(dialog.locator('[data-testid="script-dialog-workingdir"]')).toHaveText(
    '/tmp/picked',
  );
  await expect(preview).toHaveCount(0);

  await dialog.locator('[data-testid="script-dialog-name"]').fill('x');
  await dialog.locator('[data-testid="script-dialog-command"]').fill('true');
  await dialog.locator('[data-testid="script-dialog-save"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.customScriptsCreate)?.args)
    .toMatchObject({ fields: { dirMode: fixed.dirMode, workingDir: '/tmp/picked' } });
});

// Contract commands. Backend half: termflow TestCollectionsMoveAndDelete.
test('contract: a collection with its scripts shows after boot', async ({ relaunch }) => {
  const library = contract('commands', 'CustomScriptsService.List#grouped', {
    schema: scriptsSnapshotSchema,
  });
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.windowsEnsure, response: { mode: 'automations' } },
      { channel: IPC.customScriptsList, response: library },
    ],
  });
  const group = page.locator('[data-testid="script-group"][data-name="Build"]');
  await expect(group).toBeVisible();
  await expect(group).toContainText(String(library.scripts.length));
  for (const script of library.scripts) {
    await expect(page.locator(`[data-testid="script-${script.id}"]`)).toBeVisible();
  }
});
