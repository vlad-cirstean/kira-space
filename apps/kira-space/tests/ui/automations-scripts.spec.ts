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
  schedule: null,
  savedSecrets: [],
  name: 'Build all',
  command: 'echo one\necho two',
  workingDir: '',
  dirMode: 'kira',
  useAdeDir: false,
  color: 'none',
  collection: '',
  sortOrder: 0,
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
};

const FILE = {
  file: 'script-1.yaml',
  id: 'script-1',
  path: '/kira/runs/script-1.yaml',
  text: 'name: Build all\n',
  hash: 'h1',
  problem: null,
  lastGood: false,
  editable: true,
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

test('the header + writes a new run file and opens it', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.windowsEnsure, response: { mode: 'automations' } },
      TERMINAL_OPEN_OK,
      RESOLVED_DIR,
      { channel: IPC.customScriptsCreate, response: SCRIPT },
      { channel: IPC.customScriptsFile, response: FILE },
    ],
  });
  await expect(modeTab(page)).toHaveClass(/is-active/);
  await expect(page.locator('[data-testid="automations-panel"]')).toContainText('No scripts');

  await page.locator('[data-testid="automations-add"]').click();
  await page.locator('[data-testid="menu-item-new-script"]').click();
  await expect(page.locator('[data-testid="run-file-dialog"]')).toBeVisible();
  await expect(page.locator('[data-testid="config-file-path"]')).toContainText('script-1.yaml');

  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.customScriptsCreate)?.args)
    .toEqual({
      fields: {
        name: 'New script',
        command: 'npm run build',
        kind: 'script',
        params: [],
        smart: null,
        dirMode: 'kira',
        useAdeDir: false,
        workingDir: '',
        color: 'none',
        collection: '',
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
