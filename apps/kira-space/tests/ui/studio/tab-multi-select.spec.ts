// UI half of windowflow.TestTabMoveMany (contract tab-move-many): select several tabs with
// Cmd/Ctrl-click and Shift-click, close them or move them to another window through one
// TabsService.Move call (menu, drag-out, in-strip block drag), and receive a set from another window.
import type { Page } from '@playwright/test';
import { dragTab } from '@workbench/testing/ui/drag';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { calls } from '../support/ipcCalls';
import { IPC } from '../support/ipcChannels';
import { emitWailsEvent } from '../support/mockRuntime';
import type { ControlSnapshot } from '../support/types';
import { modeTab } from './support/apiMode';
import { acceptConfirm, cancelConfirm } from './support/dialogs';
import {
  activePanel,
  openKind,
  openKubernetes,
  setupKubernetes,
  tabChip,
} from './support/kubernetesFixture';
import { dataTab, holdOutside, httpTab, lastSave, sentMoves } from './support/tabMove';
import { tabLocator } from './support/tabs';

const MANY = 'tab-move-many';
const moved = contract<{ windowKey: string; created: boolean }>(MANY, 'TabsService.Move');
const inbound = contract<{ incognito: boolean; focus: boolean; tab: Record<string, unknown> }[]>(
  MANY,
  'TabsService.Inbound',
);
const dragOnly = contract('tab-move', 'TabsService.Capabilities:dragOut');
const hold = contract<{ end: number; live: boolean }>('terminal-move', 'TerminalService.Hold');

const TARGETS = [
  { key: 'win-b', number: 2 },
  { key: 'win-c', number: 3 },
];
const MOD = 'ControlOrMeta';

const FOUR = [dataTab('a', 0, true), dataTab('b', 1), dataTab('c', 2), dataTab('d', 3)];

function setup(tabs: unknown[], extra: ControlSnapshot[] = []): ControlSnapshot[] {
  return [
    { channel: IPC.tabsList, response: tabs },
    { channel: IPC.tabsCapabilities, response: dragOnly },
    { channel: IPC.tabsTargets, response: TARGETS },
    { channel: IPC.tabsMove, response: moved },
    ...extra,
  ];
}

const ids = (page: Page) =>
  page
    .locator('[data-testid="tab"]')
    .evaluateAll((els) => els.map((e) => e.getAttribute('data-tab-id')));
const selectedIds = (page: Page) =>
  page
    .locator('[data-testid="tab"][data-selected="true"]')
    .evaluateAll((els) => els.map((e) => e.getAttribute('data-tab-id')));
const addTo = (page: Page, id: string) => tabLocator(page, id).click({ modifiers: [MOD] });

test('Cmd/Ctrl-click toggles, Shift-click ranges, Esc and a plain click clear', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: setup(FOUR) });
  await addTo(page, 'c');
  expect(await selectedIds(page)).toEqual(['a', 'c']);
  await expect(tabLocator(page, 'a')).toHaveAttribute('data-active', 'true');
  await expect(tabLocator(page, 'c')).toContainText('Selected');

  await tabLocator(page, 'd').click({ modifiers: ['Shift'] });
  expect(await selectedIds(page)).toEqual(['c', 'd']);

  await tabLocator(page, 'b').locator('button').first().focus();
  await page.keyboard.press('Escape');
  expect(await selectedIds(page)).toEqual([]);

  await addTo(page, 'c');
  expect(await selectedIds(page)).toEqual(['a', 'c']);
  await tabLocator(page, 'b').click();
  expect(await selectedIds(page)).toEqual([]);
  await expect(tabLocator(page, 'b')).toHaveAttribute('data-active', 'true');
});

test('the menu closes the selected tabs with one save', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: setup(FOUR) });
  await addTo(page, 'c');
  await tabLocator(page, 'c').click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-close-others"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="menu-item-close-selected"]')).toHaveText(/Close 2 tabs/);
  await page.locator('[data-testid="menu-item-close-selected"]').click();
  await expect.poll(() => ids(page)).toEqual(['b', 'd']);
  await expect.poll(() => lastSave(control)).toEqual(['b', 'd']);
});

test('a dirty API tab in the set asks its own question; Cancel closes nothing', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: setup([httpTab('h1', 0, true)]) });
  await modeTab(page, 'api').click();
  await page.click('[data-testid="new-request"]');
  await page.getByRole('combobox', { name: 'Request URL' }).fill('https://api.example.com/draft');
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(2);
  await addTo(page, 'h1');
  const draft = page.locator('[data-testid="tab"][data-active="true"]');
  await draft.click({ button: 'right' });
  await page.locator('[data-testid="menu-item-close-selected"]').click();
  await cancelConfirm(page);
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(2);

  await draft.click({ button: 'right' });
  await page.locator('[data-testid="menu-item-close-selected"]').click();
  await acceptConfirm(page);
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(0);
});

test('the close-tab chord closes the set holding the active tab, else only the active tab', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: setup(FOUR) });
  await addTo(page, 'b');
  await addTo(page, 'c');
  await emitWailsEvent(page, IPC.tabClose, null);
  await expect.poll(() => ids(page)).toEqual(['d']);

  const second = await relaunch({ control: setup(FOUR) });
  const other = second.window;
  await addTo(other, 'b');
  await addTo(other, 'a');
  await addTo(other, 'c');
  expect(await selectedIds(other)).toEqual(['b', 'c']);
  await emitWailsEvent(other, IPC.tabClose, null);
  await expect.poll(() => ids(other)).toEqual(['b', 'c', 'd']);
});

test('the menu moves the selected tabs to a window, then to a new one', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: setup(FOUR) });
  await addTo(page, 'c');
  await addTo(page, 'd');
  await tabLocator(page, 'c').click({ button: 'right' });
  await page.locator('[data-testid="menu-item-move-selected-to-window"]').hover();
  await expect(page.locator('[data-testid="menu-item-move-selected-to-window-2"]')).toHaveText(
    'Window 2',
  );
  await page.locator('[data-testid="menu-item-move-selected-to-window-2"]').click();
  await expect.poll(() => ids(page)).toEqual(['b']);
  await expect(tabLocator(page, 'b')).toHaveAttribute('data-active', 'true');
  const [move] = sentMoves(control);
  expect(move.tabs.map((e: { tab: { id: string } }) => e.tab.id)).toEqual(['a', 'c', 'd']);
  expect(move.tabs.map((e: { incognito: boolean }) => e.incognito)).toEqual([false, false, false]);
  expect(move).toMatchObject({ target: 'win-b', newWindow: false });
  await expect.poll(() => lastSave(control)).toEqual(['b']);
});

test('the menu moves the selected tabs to a new window', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: setup(FOUR) });
  await addTo(page, 'b');
  await tabLocator(page, 'a').click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-move-selected-to-new-window"]')).toHaveText(
    /Move 2 tabs to new window/,
  );
  await page.locator('[data-testid="menu-item-move-selected-to-new-window"]').click();
  await expect.poll(() => ids(page)).toEqual(['c', 'd']);
  expect(sentMoves(control)[0]).toMatchObject({ newWindow: true, target: '' });
});

test('moving every tab closes the empty window once', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: setup([dataTab('a', 0, true), dataTab('b', 1)]),
  });
  await addTo(page, 'b');
  await tabLocator(page, 'a').click({ button: 'right' });
  await page.locator('[data-testid="menu-item-move-selected-to-new-window"]').click();
  await expect.poll(() => ids(page)).toEqual([]);
  await expect.poll(() => calls(control, IPC.tabsCloseEmptyWindow)).toHaveLength(1);
});

test('dragging a selected tab out moves the whole set; an unselected one moves alone', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: setup(FOUR) });
  await addTo(page, 'b');
  await addTo(page, 'c');
  const hint = page.locator('[data-testid="tab-tearoff-hint"]');
  await holdOutside(page, tabLocator(page, 'b'));
  await expect(hint).toHaveText('Release to move 3 tabs to another window.');
  await expect(tabLocator(page, 'a')).toHaveClass(/opacity-50/);
  await expect(tabLocator(page, 'c')).toHaveClass(/opacity-50/);
  await expect(tabLocator(page, 'd')).not.toHaveClass(/opacity-50/);
  await page.mouse.up();
  await expect.poll(() => ids(page)).toEqual(['d']);
  const [move] = sentMoves(control);
  expect(move.tabs.map((e: { tab: { id: string } }) => e.tab.id)).toEqual(['a', 'b', 'c']);
  expect(move.drop.clientX).toBeLessThan(0);
  expect(sentMoves(control)).toHaveLength(1);

  const second = await relaunch({ control: setup(FOUR) });
  const other = second.window;
  await addTo(other, 'b');
  await addTo(other, 'c');
  await holdOutside(other, tabLocator(other, 'd'));
  await expect(other.locator('[data-testid="tab-tearoff-hint"]')).toHaveText(
    'Release to move to another window.',
  );
  await other.mouse.up();
  await expect.poll(() => ids(other)).toEqual(['a', 'b', 'c']);
  expect(await selectedIds(other)).toEqual([]);
  expect(sentMoves(second.control)[0].tabs.map((e: { tab: { id: string } }) => e.tab.id)).toEqual([
    'd',
  ]);
});

test('dragging a selected tab inside the strip moves the set as one block', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: setup(FOUR) });
  await addTo(page, 'b');
  await dragTab(page, tabLocator(page, 'b'), tabLocator(page, 'd'));
  await expect.poll(() => ids(page)).toEqual(['c', 'd', 'a', 'b']);
  await expect.poll(() => lastSave(control)).toEqual(['c', 'd', 'a', 'b']);
  expect(sentMoves(control)).toHaveLength(0);
});

test('a set handed over by another window arrives last, in order, the focused one active', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: setup([httpTab('a', 0, true)]),
  });
  await modeTab(page, 'api').click();
  control.respond(
    IPC.tabsInbound,
    inbound.map((entry, i) => ({
      ...entry,
      focus: i === 1,
      tab: httpTab(`i${i + 1}`, 0),
    })),
  );
  await emitWailsEvent(page, IPC.tabsInboundEvent, undefined);
  await expect.poll(() => ids(page)).toEqual(['a', 'i1', 'i2', 'i3']);
  await expect(tabLocator(page, 'i2')).toHaveAttribute('data-active', 'true');
  await expect(page.locator('[data-testid="tab"][data-active="true"]')).toHaveCount(1);
  await expect(modeTab(page, 'api')).toHaveClass(/is-active/);
  await expect.poll(() => lastSave(control)).toEqual(['a', 'i1', 'i2']);
});

// Runs terminals ---------------------------------------------------------------------------

const b64 = (s: string) => Buffer.from(s, 'utf8').toString('base64');
const TERMINAL_OPEN: ControlSnapshot = {
  channel: IPC.terminalOpen,
  response: { shell: '/bin/zsh' },
};

async function openTerminal(page: Page): Promise<string> {
  const before = await ids(page);
  await page.locator('[data-testid="tab-strip-new"]').click();
  await page.locator('[data-testid="menu-item-new-terminal"]').click();
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(before.length + 1);
  const now = await ids(page);
  const id = now.find((x) => !before.includes(x));
  if (!id) throw new Error('terminal tab has no id');
  await emitWailsEvent(page, IPC.terminal, {
    terminalId: id,
    data: b64(`out-${id.slice(0, 4)}\r\n`),
    end: 9,
    exited: false,
  });
  return id;
}

test('two live Runs terminals move together; a refused move releases both', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: setup(
      [],
      [
        TERMINAL_OPEN,
        { channel: IPC.terminalHold, response: hold },
        { channel: IPC.terminalRelease, response: null },
        {
          channel: IPC.tabsMove,
          error: { code: 'E_CONTRACT', message: 'terminal is already moving' },
        },
      ],
    ),
  });
  await modeTab(page, 'automations').click();
  const first = await openTerminal(page);
  const second = await openTerminal(page);
  await addTo(page, first);
  await tabLocator(page, second).click({ button: 'right' });
  await page.locator('[data-testid="menu-item-move-selected-to-new-window"]').click();
  await expect.poll(() => calls(control, IPC.terminalRelease)).toHaveLength(2);
  expect(calls(control, IPC.terminalHold)).toHaveLength(2);
  await expect(tabLocator(page, first)).toHaveCount(1);
  await expect(tabLocator(page, second)).toHaveCount(1);
  expect(calls(control, IPC.terminalClose)).toHaveLength(0);

  control.respond(IPC.tabsMove, moved);
  await tabLocator(page, second).click({ button: 'right' });
  await page.locator('[data-testid="menu-item-move-selected-to-new-window"]').click();
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(0);
  const order = control
    .log()
    .map((e) => e.channel)
    .filter((c) => c === IPC.terminalHold || c === IPC.tabsMove);
  expect(order.slice(-3)).toEqual([IPC.terminalHold, IPC.terminalHold, IPC.tabsMove]);
  const move = sentMoves(control).at(-1);
  expect(move?.tabs).toHaveLength(2);
  for (const entry of move?.tabs ?? []) {
    expect(entry.terminal.status).toBe('running');
    expect(entry.terminal.snapshot).toContain('out-');
  }
  expect(calls(control, IPC.terminalClose)).toHaveLength(0);
});

// Kubernetes -------------------------------------------------------------------------------

test('the Kubernetes strip selects resource tabs and closes them, with no move items', async ({
  relaunch,
}) => {
  const { page } = await setupKubernetes(relaunch);
  await openKubernetes(page);
  await openKind(page, 'kind-dev', 'v1/pods');
  for (const name of ['web-1', 'web-2']) {
    await page.locator(`[data-testid="kube-row"][data-name="${name}"]`).click();
    await page.locator(`[data-testid="kube-row"][data-name="${name}"]`).dblclick();
    await expect(activePanel(page).locator('[data-testid="kube-detail-title"]')).toHaveText(name);
  }
  await tabChip(page, 'web-1').click({ modifiers: [MOD] });
  await expect(page.locator('[data-testid="tab"][data-selected="true"]')).toHaveCount(2);
  await tabChip(page, 'web-1').click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-close-selected"]')).toHaveText(/Close 2 tabs/);
  await expect(page.locator('[data-testid^="menu-item-move-selected"]')).toHaveCount(0);
  await page.locator('[data-testid="menu-item-close-selected"]').click();
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(0);
});
