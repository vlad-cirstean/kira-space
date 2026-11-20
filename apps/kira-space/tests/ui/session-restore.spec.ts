import { expect, type Page } from '@playwright/test';
import {
  type SessionRestoreResult,
  type SessionRestoreState,
  sessionRestoreResultSchema,
  sessionRestoreStateSchema,
} from '@shared/domain/sessionRestore';
import { test } from './fixtures';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// P701: the one dialog a restart opens for scripts, Docker containers and agents. Backend half:
// adeflow TestSessionRestore* and dockerflow TestSessionRestoreDocker store the offers, the
// confirmed args and the results this spec reads.

const quit = () =>
  contract('session-restore', 'SessionRestoreService.State#afterQuit', {
    schema: sessionRestoreStateSchema,
  });
const dockerState = () =>
  contract('session-restore-docker', 'SessionRestoreService.State#docker', {
    schema: sessionRestoreStateSchema,
  });
const mixedArgs = () =>
  contract<{ scripts: unknown[]; docker: unknown; agents: unknown[] }>(
    'session-restore',
    'args:SessionRestoreService.Restore#mixed',
  );
const mixedResult = () =>
  contract('session-restore', 'SessionRestoreService.Restore#mixed', {
    schema: sessionRestoreResultSchema,
  });
const partialResult = () =>
  contract('session-restore', 'SessionRestoreService.Restore#partial', {
    schema: sessionRestoreResultSchema,
  });

/** Scripts and agents from the quit offer, Docker from the engine offer. */
const full = (): SessionRestoreState => ({ ...quit(), docker: dockerState().docker });

const SCRIPT_TOKEN = 'tok-session-restore';
const withTerminalIds = (r: SessionRestoreResult): SessionRestoreResult => ({
  ...r,
  scripts: r.scripts.map((o) =>
    o.started?.terminal
      ? {
          ...o,
          started: { ...o.started, terminal: { ...o.started.terminal, token: SCRIPT_TOKEN } },
        }
      : o,
  ),
  // The contract masks terminal ids; each launch needs its own.
  agents: r.agents.map((a, i) =>
    a.launch ? { ...a, launch: { ...a.launch, terminalId: `term-restore-${i}` } } : a,
  ),
});

const plainScript = (s: SessionRestoreState) => {
  const item = s.scripts[0];
  if (!item) throw new Error('no script in the contract');
  return {
    id: item.scriptId,
    kind: 'script',
    params: [],
    smart: null,
    name: item.scriptName,
    command: item.command,
    workingDir: '',
    dirMode: 'kira',
    useAdeDir: false,
    color: item.color,
    collection: '',
    sortOrder: 0,
    createdAt: '2026-01-01T00:00:00.000Z',
    updatedAt: '2026-01-01T00:00:00.000Z',
  };
};

const base = (state: SessionRestoreState, claim = true) => [
  { channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } },
  { channel: IPC.scriptRunsList, response: [] },
  { channel: IPC.scriptRunsReadLog, response: { chunks: [], truncated: false } },
  {
    channel: IPC.customScriptsList,
    response: { collections: [], scripts: [plainScript(state)] },
  },
  { channel: IPC.sessionRestoreState, response: state },
  { channel: IPC.sessionRestoreClaim, response: claim },
  { channel: IPC.sessionRestoreClose, response: null },
];

const dialog = (page: Page) => page.locator('[data-testid="restore-dialog"]');
const row = (page: Page, id: string) =>
  page.locator(`[data-testid="restore-row"][data-id="${id}"]`);
const check = (page: Page, id: string) =>
  row(page, id).locator('[data-testid="restore-row-toggle"]');
const option = (page: Page, id: string) =>
  row(page, id).locator('[data-testid="restore-option-toggle"]');

test('contract: the dialog opens at launch with Scripts, Docker and Agents and the Continue defaults', async ({
  relaunch,
}) => {
  const state = full();
  const { window: page, control } = await relaunch({ control: base(state) });

  await expect(dialog(page)).toBeVisible();
  await expect(page.locator('[data-testid="restore-group"]')).toHaveCount(3);
  const groups = await page
    .locator('[data-testid="restore-group"]')
    .evaluateAll((els) => els.map((e) => e.getAttribute('data-group')));
  expect(groups).toEqual(['Scripts', 'Docker', 'Agents']);
  await expect(page.locator('[data-testid="restore-selection"]')).toHaveText('5 of 5 selected');

  for (const a of state.agents) {
    await expect(check(page, a.id)).toHaveAttribute('data-state', 'checked');
    await expect(option(page, a.id)).toHaveAttribute(
      'data-state',
      a.continueDefault ? 'checked' : 'unchecked',
    );
  }
  // P696: the X is the only way out, no Dismiss beside it.
  await expect(page.locator('[data-testid="restore-dismiss"]')).toHaveCount(0);
  expect(calls(control, IPC.sessionRestoreClaim)).toHaveLength(1);
  expect(calls(control, IPC.sessionRestoreRestore)).toHaveLength(0);
});

test('the X closes the dialog and restores nothing', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: base(full()) });
  await expect(dialog(page)).toBeVisible();

  await page.locator('[data-testid="restore-close"]').click();

  await expect(dialog(page)).toHaveCount(0);
  expect(calls(control, IPC.sessionRestoreClose)).toHaveLength(1);
  expect(calls(control, IPC.sessionRestoreRestore)).toHaveLength(0);
});

test('unchecking Resume unchecks and disables Continue; a required Continue stays checked', async ({
  relaunch,
}) => {
  const state = full();
  const [run, busy] = state.agents;
  if (!run || !busy) throw new Error('contract agents missing');
  const review = { ...run, continue: 'required' as const };
  const { window: page } = await relaunch({
    control: base({ ...state, agents: [review, busy, ...state.agents.slice(2)] }),
  });
  await expect(dialog(page)).toBeVisible();

  await expect(option(page, review.id)).toHaveAttribute('data-state', 'checked');
  await expect(option(page, review.id)).toBeDisabled();
  await expect(option(page, busy.id)).toBeEnabled();

  await check(page, busy.id).click();
  await expect(option(page, busy.id)).toHaveAttribute('data-state', 'unchecked');
  await expect(option(page, busy.id)).toBeDisabled();
  await check(page, busy.id).click();
  await expect(option(page, busy.id)).toBeEnabled();
});

test('contract: Restore sends the chosen items, opens the script tab and the agent terminals, then closes', async ({
  relaunch,
}) => {
  const state = full();
  const [run] = state.agents;
  if (!run) throw new Error('contract agents missing');
  const result = withTerminalIds(mixedResult());
  const { window: page, control } = await relaunch({
    control: [
      ...base(state),
      { channel: IPC.sessionRestoreRestore, response: result },
      { channel: IPC.windowsEnsure, response: { mode: 'git' } },
    ],
  });
  await expect(dialog(page)).toBeVisible();

  // The contract's args: the run resumes without Continue, Docker stays out.
  await option(page, run.id).click();
  for (const c of state.docker.items) {
    const id = c.composeProject ? `project:${c.composeProject}` : c.id;
    await check(page, id).click();
  }
  await page.locator('[data-testid="restore-submit"]').click();

  await expect(dialog(page)).toHaveCount(0);
  expect(calls(control, IPC.sessionRestoreRestore)[0]?.args).toEqual(mixedArgs());
  await expect
    .poll(() =>
      calls(control, IPC.terminalOpen).map((c) => (c.args as { terminalId?: string }).terminalId),
    )
    .toEqual(expect.arrayContaining(['term-restore-1', 'term-restore-2']));
});

test('contract: a partial result keeps the dialog open with the row errors and no toast', async ({
  relaunch,
}) => {
  const state = full();
  const [run, busy, idle] = state.agents;
  const script = state.scripts[0];
  const partial = partialResult();
  const scriptFail = partial.scripts[0];
  const agentFail = partial.agents[1];
  if (!run || !busy || !idle || !script || !scriptFail || !agentFail)
    throw new Error('contract fixtures missing');
  const opened = withTerminalIds(partial).agents[0]?.launch;
  const result: SessionRestoreResult = {
    scripts: [{ ...scriptFail, runId: script.runId }],
    docker: null,
    agents: [
      { id: busy.id, launch: opened ?? null, runId: '', code: '', error: '' },
      { ...agentFail, id: idle.id },
    ],
  };
  const { window: page, control } = await relaunch({
    control: [...base(state), { channel: IPC.sessionRestoreRestore, response: result }],
  });
  await expect(dialog(page)).toBeVisible();

  await page.locator('[data-testid="restore-submit"]').click();

  await expect(row(page, script.runId).locator('[data-testid="restore-row-error"]')).toContainText(
    scriptFail.error,
  );
  await expect(row(page, idle.id).locator('[data-testid="restore-row-error"]')).toContainText(
    agentFail.error,
  );
  await expect(dialog(page)).toBeVisible();
  await page.waitForTimeout(2000);
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(0);
  expect(
    calls(control, IPC.terminalOpen).filter(
      (c) => (c.args as { terminalId?: string }).terminalId === opened?.terminalId,
    ),
  ).toHaveLength(1);
});

test('nothing ready: no claim and no dialog', async ({ relaunch }) => {
  const state = full();
  const idle: SessionRestoreState = {
    ...state,
    scripts: state.scripts.map((s) => ({ ...s, status: 'gone' as const, reason: 'Gone.' })),
    docker: { ...state.docker, state: 'none', items: [] },
    agents: state.agents.map((a) => ({ ...a, state: 'unavailable' as const, reason: 'Gone.' })),
  };
  const { window: page, control } = await relaunch({ control: base(idle) });
  await expect(page.locator('[data-testid="mode-tab"]').first()).toBeVisible();

  await expect(dialog(page)).toHaveCount(0);
  expect(calls(control, IPC.sessionRestoreClaim)).toHaveLength(0);
});

test('another window won the claim: no dialog here', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: base(full(), false) });
  await expect(page.locator('[data-testid="mode-tab"]').first()).toBeVisible();

  await expect.poll(() => calls(control, IPC.sessionRestoreClaim)).toHaveLength(1);
  await expect(dialog(page)).toHaveCount(0);
});
