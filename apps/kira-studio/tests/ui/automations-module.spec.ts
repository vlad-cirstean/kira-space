import type { ControlSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { modeTab } from './support/apiMode';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// TERMINAL_OPEN_OK's shape uses control.log() polling rather than call-count
// assertions. The mocked DefaultCwd below is '/home/test' — mockRuntime.ts's own boot default
// (IPC.terminalDefaultCwd) — so an unscoped terminal's title falls out of its basename, 'test'
// (tabKinds.ts's terminal kind: `s.label || basename(s.cwd) || 'Terminal'`).
//
// The panel header `+` is the only add control; it writes a run file and opens it as text.

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
  collection: '',
  schedule: null,
  savedSecrets: [],
  sortOrder: 0,
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
};

const FILE = {
  file: 'script-1.yaml',
  id: 'script-1',
  path: '/kira/runs/script-1.yaml',
  text: 'name: Dev server\n',
  hash: 'h1',
  problem: null,
  lastGood: false,
  editable: true,
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
  return page.locator('[data-testid="run-file-dialog"]');
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

test('the header + is the only add control and writes a run file, then opens it', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.customScriptsList, response: { collections: [], scripts: [SCRIPT] } },
      { channel: IPC.customScriptsCreate, response: SCRIPT },
      { channel: IPC.customScriptsFile, response: FILE },
    ],
  });

  await openTerminalModule(page);
  await expect(page.locator('[data-testid="script-empty-add"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="automations-add"]')).toHaveCount(1);
  await page.locator('[data-testid="automations-add"]').click();
  await page.locator('[data-testid="menu-item-new-script"]').click();
  await expect(dialog(page)).toBeVisible();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.customScriptsCreate)?.args)
    .toMatchObject({ fields: { name: 'New script', kind: 'script', dirMode: 'kira' } });
});

test('scripts group under collapsible collection rows; empty collections show', async ({
  relaunch,
}) => {
  const collections = [
    { name: 'Backend', sortOrder: 0 },
    { name: 'Web', sortOrder: 1 },
    { name: 'Empty', sortOrder: 2 },
  ];
  const scripts = [
    { ...SCRIPT, id: 's-free', name: 'Loose', collection: '' },
    { ...SCRIPT, id: 's-a1', name: 'Build', collection: 'Backend' },
    { ...SCRIPT, id: 's-a2', name: 'Test', collection: 'Backend' },
    { ...SCRIPT, id: 's-b1', name: 'Serve', collection: 'Web' },
  ];
  const { window: page } = await relaunch({
    control: [{ channel: IPC.customScriptsList, response: { collections, scripts } }],
  });

  await openTerminalModule(page);
  const group = (name: string) => page.locator(`[data-testid="script-group"][data-name="${name}"]`);
  await expect(group('Backend')).toContainText('2');
  await expect(group('Web')).toContainText('1');
  await expect(group('Empty')).toContainText('0');
  await expect(page.locator('[data-testid="script-list"] [data-testid^="script-s-"]')).toHaveCount(
    4,
  );

  await expect(page.locator('[data-testid="script-s-free"]')).toHaveCSS('text-align', 'left');

  await group('Backend').locator('button:has-text("Backend")').click();
  await expect(page.locator('[data-testid="script-s-a1"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="script-s-b1"]')).toBeVisible();
  await group('Backend').locator('button:has-text("Backend")').click();
  await expect(page.locator('[data-testid="script-s-a1"]')).toBeVisible();

  await page.locator('[data-testid="toggle-search"]').click();
  await page.locator('[data-testid="tree-search"]').fill('serve');
  await expect(group('Backend')).toHaveCount(0);
  await expect(group('Empty')).toHaveCount(0);
  await expect(page.locator('[data-testid="script-s-b1"]')).toBeVisible();
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
