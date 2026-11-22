// UI half of windowflow.TestTabMove and TestTerminalMove (contracts tab-move, terminal-move): a tab
// leaves the window by drag-out or menu through one TabsService.Move call, and a tab handed over by
// another window arrives through the inbox. Runs terminals carry their screen and live output.
import type { Page } from '@playwright/test';
import { dragTabOut } from '@workbench/testing/ui/drag';
import { expect, filesTest, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { calls } from '../support/ipcCalls';
import { IPC } from '../support/ipcChannels';
import { emitWailsEvent } from '../support/mockRuntime';
import { filesWindowControl } from '../support/repoFiles';
import type { ControlSnapshot } from '../support/types';
import { modeTab } from './support/apiMode';
import { cancelConfirm } from './support/dialogs';
import { dataTab, holdOutside, httpTab, lastSave, sentMoves } from './support/tabMove';
import { tabLocator } from './support/tabs';

const MOVE = 'tab-move';
const TERM = 'terminal-move';
const moved = contract<{ windowKey: string; created: boolean }>(MOVE, 'TabsService.Move');
const movedNew = contract<{ windowKey: string; created: boolean }>(
  MOVE,
  'TabsService.Move:created',
);
const dragOnly = contract(MOVE, 'TabsService.Capabilities:dragOut');
const noDrag = contract(MOVE, 'TabsService.Capabilities');
const inbound = contract<{ incognito: boolean; focus: boolean; tab: Record<string, unknown> }[]>(
  MOVE,
  'TabsService.Inbound',
);
const hold = contract<{ end: number; live: boolean }>(TERM, 'TerminalService.Hold');
const holdExited = contract<{ end: number; live: boolean }>(TERM, 'TerminalService.Hold:exited');
const adopt = contract<Record<string, unknown>>(TERM, 'TerminalService.Adopt');

const TARGETS = [
  { key: 'win-b', number: 2 },
  { key: 'win-c', number: 3 },
];

const ids = (page: Page) =>
  page
    .locator('[data-testid="tab"]')
    .evaluateAll((els) => els.map((e) => e.getAttribute('data-tab-id')));

function setup(opts: { dragOut: boolean; tabs: unknown[] }): ControlSnapshot[] {
  return [
    { channel: IPC.tabsList, response: opts.tabs },
    { channel: IPC.tabsCapabilities, response: opts.dragOut ? dragOnly : noDrag },
    { channel: IPC.tabsTargets, response: TARGETS },
    { channel: IPC.tabsMove, response: moved },
  ];
}

async function openApi(page: Page): Promise<void> {
  await modeTab(page, 'api').click();
  await expect(modeTab(page, 'api')).toHaveClass(/is-active/);
}

test('dragging a tab out of the window moves it with its drop point', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: setup({
      dragOut: true,
      tabs: [dataTab('a', 0), dataTab('b', 1, true), dataTab('c', 2)],
    }),
  });
  const hint = page.locator('[data-testid="tab-tearoff-hint"]');
  await expect(hint).toHaveCount(0);
  await holdOutside(page, tabLocator(page, 'b'));
  await expect(hint).toBeVisible();
  await page.mouse.up();
  await expect(hint).toHaveCount(0);

  await expect(tabLocator(page, 'b')).toHaveCount(0);
  const [move] = sentMoves(control);
  expect(sentMoves(control)).toHaveLength(1);
  expect(move.tabs.map((e: { tab: { id: string } }) => e.tab.id)).toEqual(['b']);
  expect(move.drop.clientX).toBeLessThan(0);
  await expect(page.locator('[data-testid="tab"][data-active="true"]')).toHaveCount(1);
  await expect.poll(() => lastSave(control)).toEqual(['a', 'c']);
});

test('Escape while the tab is held outside cancels the move', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: setup({ dragOut: true, tabs: [dataTab('a', 0), dataTab('b', 1, true)] }),
  });
  await holdOutside(page, tabLocator(page, 'b'));
  await page.keyboard.press('Escape');
  await page.mouse.up();
  await page.waitForTimeout(200);
  expect(sentMoves(control)).toHaveLength(0);
  expect(await ids(page)).toEqual(['a', 'b']);
});

test('the last tab out closes the empty window once', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: setup({ dragOut: true, tabs: [dataTab('only', 0, true)] }),
  });
  await dragTabOut(page, tabLocator(page, 'only'), { x: -60, y: 200 });
  await expect(tabLocator(page, 'only')).toHaveCount(0);
  await expect.poll(() => calls(control, IPC.tabsCloseEmptyWindow)).toHaveLength(1);
});

test('without drag-out the menu moves a tab to a new or another window', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: setup({ dragOut: false, tabs: [dataTab('a', 0, true), dataTab('b', 1)] }),
  });
  await dragTabOut(page, tabLocator(page, 'a'), { x: -60, y: 200 });
  await page.waitForTimeout(200);
  expect(sentMoves(control)).toHaveLength(0);
  await expect(tabLocator(page, 'a')).toHaveCount(1);

  control.respond(IPC.tabsMove, movedNew);
  await tabLocator(page, 'a').click({ button: 'right' });
  await page.locator('[data-testid="menu-item-move-to-new-window"]').click();
  await expect(tabLocator(page, 'a')).toHaveCount(0);
  expect(sentMoves(control)[0]).toMatchObject({ newWindow: true, target: '' });

  await tabLocator(page, 'b').click({ button: 'right' });
  await page.locator('[data-testid="menu-item-move-to-window"]').hover();
  await expect(page.locator('[data-testid="menu-item-move-to-window-2"]')).toHaveText('Window 2');
  await page.locator('[data-testid="menu-item-move-to-window-3"]').click();
  await expect(tabLocator(page, 'b')).toHaveCount(0);
  expect(sentMoves(control)[1]).toMatchObject({ newWindow: false, target: 'win-c' });
});

test('a tab handed over by another window arrives last and active', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: setup({ dragOut: true, tabs: [httpTab('a', 0, true)] }),
  });
  await openApi(page);
  control.respond(IPC.tabsInbound, inbound);
  await emitWailsEvent(page, IPC.tabsInboundEvent, undefined);

  const strip = page.locator('[data-testid="tab"]');
  await expect(strip).toHaveCount(2);
  expect(await ids(page)).toEqual(['a', 'h1']);
  await expect(tabLocator(page, 'h1')).toHaveAttribute('data-active', 'true');
  await expect(modeTab(page, 'api')).toHaveClass(/is-active/);
  await expect.poll(() => lastSave(control)).toContain('h1');
});

test('an incognito tab arrives marked and is never saved here', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: setup({ dragOut: true, tabs: [httpTab('a', 0, true)] }),
  });
  await openApi(page);
  control.respond(
    IPC.tabsInbound,
    inbound.map((entry) => ({ ...entry, incognito: true })),
  );
  await emitWailsEvent(page, IPC.tabsInboundEvent, undefined);

  await expect(tabLocator(page, 'h1')).toBeVisible();
  await expect(tabLocator(page, 'h1').locator('.codicon-eye-closed')).toBeVisible();
  await expect.poll(() => calls(control, IPC.tabsSave).length).toBeGreaterThan(0);
  expect(lastSave(control)).not.toContain('h1');
});

test('a tab with unsaved work asks before leaving; Cancel keeps it', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: setup({ dragOut: false, tabs: [] }),
  });
  await modeTab(page, 'api').click();
  await page.click('[data-testid="new-request-start"]');
  const chip = page.locator('[data-testid="tab"]').first();
  await expect(chip).toBeVisible();
  await page.getByRole('combobox', { name: 'Request URL' }).fill('https://api.example.com/draft');
  await chip.click({ button: 'right' });
  await page.locator('[data-testid="menu-item-move-to-new-window"]').click();
  await cancelConfirm(page);
  expect(sentMoves(control)).toHaveLength(0);
  await expect(chip).toBeVisible();
});

test('the keyboard opens the tab menu with the move item', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: setup({ dragOut: false, tabs: [dataTab('a', 0, true)] }),
  });
  await tabLocator(page, 'a').locator('button').first().focus();
  await page.keyboard.press('Shift+F10');
  await expect(page.locator('[data-testid="menu-item-move-to-new-window"]')).toBeVisible();
});

const REPO = {
  id: 'repo-1',
  name: 'demo-repo',
  root: '/tmp/demo-repo',
  repoId: '/tmp/demo-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};

filesTest('an opened-files tab offers no move and does not tear off', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: filesWindowControl(
      REPO,
      ['a.ts'],
      [
        { channel: IPC.tabsCapabilities, response: dragOnly },
        { channel: IPC.tabsTargets, response: TARGETS },
        {
          channel: IPC.codeWorkspaceReadFile,
          response: {
            kind: 'found',
            text: 'package a\n',
            bytes: 10,
            limitBytes: 1 << 23,
            language: 'go',
          },
        },
      ],
    ),
  });
  const chip = page.locator('[data-testid="tab"]').first();
  await expect(chip).toBeVisible();
  await dragTabOut(page, chip, { x: -60, y: 200 });
  await page.waitForTimeout(200);
  expect(sentMoves(control)).toHaveLength(0);
  await chip.click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-duplicate-tab"]')).toBeVisible();
  await expect(page.locator('[data-testid="menu-item-move-to-new-window"]')).toHaveCount(0);
});

// Runs terminals ---------------------------------------------------------------------------

const b64 = (s: string) => Buffer.from(s, 'utf8').toString('base64');
const TERMINAL_OPEN: ControlSnapshot = {
  channel: IPC.terminalOpen,
  response: { shell: '/bin/zsh' },
};

async function openRunsTerminal(page: Page): Promise<string> {
  await modeTab(page, 'automations').click();
  await page.locator('[data-testid="tab-strip-new"]').click();
  await page.locator('[data-testid="menu-item-new-terminal"]').click();
  const chip = page.locator('[data-testid="tab"]');
  await expect(chip).toHaveCount(1);
  const id = await chip.getAttribute('data-tab-id');
  if (!id) throw new Error('terminal tab has no id');
  await expect(page.locator('.xterm-rows')).toBeVisible();
  return id;
}

const termMove = (extra: ControlSnapshot[]): ControlSnapshot[] => [
  { channel: IPC.tabsCapabilities, response: dragOnly },
  { channel: IPC.tabsTargets, response: TARGETS },
  TERMINAL_OPEN,
  ...extra,
];

test('a live Runs terminal leaves with its screen and is never closed', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: termMove([
      { channel: IPC.terminalHold, response: hold },
      { channel: IPC.tabsMove, response: moved },
    ]),
  });
  const id = await openRunsTerminal(page);
  await emitWailsEvent(page, IPC.terminal, {
    terminalId: id,
    data: b64('hello-moved\r\n'),
    end: 13,
    exited: false,
  });
  await expect(page.locator('.xterm-rows')).toContainText('hello-moved');

  await dragTabOut(page, tabLocator(page, id), { x: -60, y: 200 });
  await expect(tabLocator(page, id)).toHaveCount(0);

  const order = control
    .log()
    .map((e) => e.channel)
    .filter((c) => c === IPC.terminalHold || c === IPC.tabsMove);
  expect(order).toEqual([IPC.terminalHold, IPC.tabsMove]);
  const [move] = sentMoves(control);
  expect(move.tabs[0].terminal.snapshot).toContain('hello-moved');
  expect(move.tabs[0].terminal.status).toBe('running');
  expect(calls(control, IPC.terminalClose)).toHaveLength(0);
});

test('a Runs terminal that exited before the hold leaves as exited, with its code', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: termMove([
      { channel: IPC.terminalHold, response: holdExited },
      { channel: IPC.tabsMove, response: moved },
    ]),
  });
  const id = await openRunsTerminal(page);
  await emitWailsEvent(page, IPC.terminal, {
    terminalId: id,
    data: b64('a1\r\n'),
    end: 4,
    exited: false,
  });
  await expect(page.locator('.xterm-rows')).toContainText('a1');

  await dragTabOut(page, tabLocator(page, id), { x: -60, y: 200 });
  await expect.poll(() => calls(control, IPC.terminalHold)).toHaveLength(1);
  // The exit event the source saw reaches it while the move waits for it.
  await emitWailsEvent(page, IPC.terminal, { terminalId: id, end: 4, exited: true, exitCode: 3 });
  await expect(tabLocator(page, id)).toHaveCount(0);

  const [move] = sentMoves(control);
  expect(move.tabs[0].terminal.status).toBe('exited');
  expect(move.tabs[0].terminal.exitCode).toBe(3);
  expect(calls(control, IPC.terminalRelease)).toHaveLength(0);
});

test('a refused Runs terminal move releases the hold and keeps the tab', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: termMove([
      { channel: IPC.terminalHold, response: hold },
      { channel: IPC.terminalRelease, response: null },
      {
        channel: IPC.tabsMove,
        error: { code: 'E_CONTRACT', message: 'terminal is already moving' },
      },
    ]),
  });
  const id = await openRunsTerminal(page);
  await emitWailsEvent(page, IPC.terminal, {
    terminalId: id,
    data: b64('x'),
    end: 1,
    exited: false,
  });
  await dragTabOut(page, tabLocator(page, id), { x: -60, y: 200 });
  await expect.poll(() => calls(control, IPC.terminalRelease)).toHaveLength(1);
  await expect(tabLocator(page, id)).toHaveCount(1);
  expect(calls(control, IPC.terminalClose)).toHaveLength(0);
});

const terminalTab = (id: string) => ({
  id,
  connectionId: null,
  path: '',
  kind: 'terminal',
  order: 0,
  active: true,
  workspaceId: 'automations',
  state: {
    codeRepoId: '',
    cwd: '/home/test',
    command: '',
    label: '',
    color: 'none',
    launchKind: 'shell',
    scriptId: '',
    launchToken: '',
    taskId: '',
    branchId: '',
  },
});

test('a live Runs terminal arrives with snapshot, held bytes, then live output', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [
      ...termMove([
        { channel: IPC.terminalAdopt, response: { ...adopt, data: b64('held-bytes\r\n'), end: 5 } },
      ]),
    ],
  });
  control.respond(IPC.tabsInbound, [
    {
      incognito: false,
      focus: true,
      tab: terminalTab('t1'),
      terminal: {
        snapshot: 'moved-screen\r\n',
        status: 'running',
        cols: 80,
        rows: 24,
        shell: '/bin/zsh',
      },
    },
  ]);
  await emitWailsEvent(page, IPC.tabsInboundEvent, undefined);

  await expect(modeTab(page, 'automations')).toHaveClass(/is-active/);
  await expect(tabLocator(page, 't1')).toHaveAttribute('data-active', 'true');
  const rows = page.locator('.xterm-rows');
  await expect(rows).toContainText('held-bytes');
  await emitWailsEvent(page, IPC.terminal, {
    terminalId: 't1',
    data: b64('live-after\r\n'),
    end: 17,
    exited: false,
  });
  await expect(rows).toContainText('live-after');
  const text = (await rows.innerText()).replace(/\s+/g, ' ');
  expect(text.indexOf('moved-screen')).toBeGreaterThanOrEqual(0);
  expect(text.indexOf('moved-screen')).toBeLessThan(text.indexOf('held-bytes'));
  expect(text.indexOf('held-bytes')).toBeLessThan(text.indexOf('live-after'));
  expect(calls(control, IPC.terminalOpen)).toHaveLength(0);
  expect(calls(control, IPC.terminalAdopt)[0]?.args).toMatchObject({ terminalId: 't1' });
});

test('an exited Runs terminal arrives with its stored status and adopts nothing', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: termMove([]) });
  control.respond(IPC.tabsInbound, [
    {
      incognito: false,
      focus: true,
      tab: terminalTab('t2'),
      terminal: {
        snapshot: 'done-screen\r\n',
        status: 'exited',
        exitCode: 3,
        cols: 80,
        rows: 24,
        shell: '/bin/zsh',
      },
    },
  ]);
  await emitWailsEvent(page, IPC.tabsInboundEvent, undefined);
  await expect(page.locator('.xterm-rows')).toContainText('done-screen');
  await expect(page.locator('[data-testid="repo-terminal-footer"]')).toContainText('code 3');
  expect(calls(control, IPC.terminalAdopt)).toHaveLength(0);
  expect(calls(control, IPC.terminalOpen)).toHaveLength(0);
});

test('a script-run tab moves by menu without terminal calls and arrives as a run view', async ({
  relaunch,
}) => {
  const runTab = (id: string) => ({
    id,
    connectionId: null,
    path: '',
    kind: 'script-run',
    order: 0,
    active: true,
    workspaceId: 'automations',
    state: { runId: 'run1', label: 'Lint', scriptId: '', taskId: '', branchId: '' },
  });
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.tabsList, response: [runTab('sr1')] },
      { channel: IPC.tabsCapabilities, response: noDrag },
      { channel: IPC.tabsTargets, response: TARGETS },
      { channel: IPC.tabsMove, response: moved },
    ],
  });
  await modeTab(page, 'automations').click();
  await tabLocator(page, 'sr1').click({ button: 'right' });
  await page.locator('[data-testid="menu-item-move-to-new-window"]').click();
  await expect(tabLocator(page, 'sr1')).toHaveCount(0);
  const [move] = sentMoves(control);
  expect(move.tabs[0].tab.kind).toBe('script-run');
  expect(move.tabs[0].terminal ?? null).toBeNull();
  expect(calls(control, IPC.terminalHold)).toHaveLength(0);

  control.respond(IPC.tabsInbound, [{ incognito: false, focus: true, tab: runTab('sr2') }]);
  await emitWailsEvent(page, IPC.tabsInboundEvent, undefined);
  await expect(page.locator('[data-testid="script-run-view"]')).toBeVisible();
});
