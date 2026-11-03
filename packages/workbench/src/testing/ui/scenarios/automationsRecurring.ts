import { expect, type Page } from '@playwright/test';
import type { RoutedPrompt } from '@shared/domain/prompts';
import { scriptRunSchema } from '@shared/domain/scriptRuns';
import { customScriptSchema } from '@shared/domain/scripts';
import { type ControlSnapshot, emitWailsEvent } from '../mockRuntime';
import { emitPrompts, routed } from '../prompts';
import type { ScenarioEnv } from './env';

type Env = ScenarioEnv<
  | 'customScriptsList'
  | 'promptsMainWindow'
  | 'scriptRunsChanged'
  | 'scriptRunsConfirmAccept'
  | 'scriptRunsConfirmDecline'
  | 'scriptRunsNextFires'
  | 'scriptRunsReadLog'
  | 'scriptRunsResolveDir'
  | 'scriptRunsRunScheduleNow'
  | 'scriptRunsSchedulePreview'
>;

export interface Options {
  /** Snapshots an app needs ahead of the shared ones. */
  extraBase: readonly ControlSnapshot[];
}

export type Ctx = {
  BASE: ControlSnapshot[];
  SCRIPT: Record<string, unknown> & { id: string };
  openPanel: (page: Page) => Promise<void>;
};

export function registerRecurringScenarios(
  env: Env,
  options: Options,
  extra?: (ctx: Ctx) => void,
): void {
  const { test, contract, ipc: IPC } = env;

  // P242 Part 4: recurring scripts on the mocked bridge: the editor's schedule, rows, the confirm popup.

  const SCHEDULE = {
    cron: '0 9 * * 1-5',
    timezone: 'UTC',
    enabled: true,
    confirm: true,
    timeout: '30m',
    params: {},
    taskId: '',
    branchId: '',
  };

  const SCRIPT = {
    id: 'rec-1',
    kind: 'script',
    name: 'Nightly build',
    command: 'make build',
    params: [],
    smart: null,
    schedule: SCHEDULE,
    workingDir: '',
    dirMode: 'kira',
    useAdeDir: false,
    color: 'blue',
    collection: '',
    sortOrder: 0,
    createdAt: '2026-01-01T00:00:00.000Z',
    updatedAt: '2026-01-01T00:00:00.000Z',
  };

  const DIR = {
    path: '/kira/automations/rec-1',
    mode: 'kira',
    base: '/kira',
    blocker: '',
    branch: '',
    pending: false,
  };

  const FIRES = [1_900_000_000_000, 1_900_086_400_000, 1_900_172_800_000];

  const PREVIEW = {
    kind: 'script',
    missing: [],
    blocker: '',
    dir: DIR,
    body: SCRIPT.command,
    prompt: [],
    suffix: '',
    env: [],
    command: 'make build',
    model: '',
    maxBudgetUsd: 0,
    timeout: '30m',
    tools: [],
    allowedTools: [],
    mcpServers: [],
    hash: 'sched-h1',
    needs: { tasks: [], branches: [] },
    ade: null,
  };

  function run(state: string, extra: Record<string, unknown> = {}) {
    const ended = state !== 'running' && state !== 'waiting';
    return {
      id: 'run-w',
      scriptId: SCRIPT.id,
      scriptName: SCRIPT.name,
      color: 'blue',
      kind: 'script',
      trigger: 'scheduled',
      state,
      terminalId: '',
      cwd: DIR.path,
      command: 'make build',
      outcome: ended ? { status: state, reason: 'done', source: 'exit', reported: false } : null,
      createdAt: 2_000,
      startedAt: state === 'waiting' ? null : 2_000,
      finishedAt: ended ? 3_000 : null,
      model: '',
      sessionId: '',
      prompt: '',
      params: [],
      tools: { tools: [], allowedTools: [], mcpServers: [] },
      taskId: '',
      taskTitle: '',
      branchId: '',
      branchLabel: '',
      ...extra,
    };
  }

  const BASE: ControlSnapshot[] = [
    ...options.extraBase,
    { channel: IPC.customScriptsList, response: { collections: [], scripts: [SCRIPT] } },
    { channel: IPC.scriptRunsResolveDir, response: DIR },
    { channel: IPC.scriptRunsNextFires, response: FIRES },
    { channel: IPC.scriptRunsSchedulePreview, response: PREVIEW },
    { channel: IPC.scriptRunsConfirmAccept, response: { runId: 'run-w', terminal: null } },
    { channel: IPC.scriptRunsRunScheduleNow, response: { runId: 'run-n', terminal: null } },
    { channel: IPC.scriptRunsReadLog, response: { chunks: [], truncated: false } },
  ];

  const modeTab = (page: Page) => page.locator('[data-testid="mode-tab"][data-mode="automations"]');

  async function openPanel(page: Page): Promise<void> {
    await modeTab(page).click();
    await expect(page.locator('[data-testid="automations-panel"]')).toBeVisible();
  }

  test('a recurring row shows the clock and next fire', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: BASE });
    await openPanel(page);
    const row = page.locator(`[data-testid="script-${SCRIPT.id}"]`);
    await expect(row.locator('[data-testid="script-schedule-line"]')).toBeVisible();
    await expect(row.locator('[data-testid="script-next"]')).toContainText('Next');
  });

  test('Run now opens the confirm popup and Run starts the schedule run', async ({ relaunch }) => {
    const { window: page, control } = await relaunch({ control: BASE });
    await openPanel(page);
    await page.locator(`[data-testid="script-${SCRIPT.id}"]`).click({ button: 'right' });
    await page.locator('[data-testid="menu-item-run-now"]').click();
    const popup = page.locator('[data-testid="schedule-confirm"]');
    await expect(popup).toContainText('Run Nightly build?');
    await expect(popup.locator('[data-testid="schedule-confirm-due"]')).toHaveText('Now');
    await expect(popup.locator('[data-testid="run-folder"] [data-var="folder"]')).toBeVisible();
    await popup.locator('[data-testid="schedule-confirm-run"]').click();
    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.scriptRunsRunScheduleNow)?.args)
      .toMatchObject({ scriptId: SCRIPT.id, hash: 'sched-h1' });
    await expect(popup).toHaveCount(0);
  });

  test('a waiting run shows the popup in the main window; Run accepts, Not now declines', async ({
    relaunch,
  }) => {
    const { window: page, control } = await relaunch({
      control: [...BASE, { channel: IPC.scriptRunsConfirmDecline }],
    });
    await openPanel(page);
    await emitWailsEvent(page, IPC.scriptRunsChanged, run('waiting'));
    await emitPrompts(page, [routed('schedule', 'run-w')]);
    const popup = page.locator('[data-testid="schedule-confirm"]');
    await expect(popup).toBeVisible();
    await expect(popup.locator('[data-testid="schedule-confirm-due"]')).toContainText('Due');
    await popup.locator('[data-testid="schedule-confirm-run"]').click();
    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.scriptRunsConfirmAccept)?.args)
      .toMatchObject({ runId: 'run-w', hash: 'sched-h1' });
    await expect(popup).toHaveCount(0);

    await emitWailsEvent(page, IPC.scriptRunsChanged, run('waiting', { id: 'run-x' }));
    await emitPrompts(page, [routed('schedule', 'run-x')]);
    await expect(popup).toBeVisible();
    await popup.locator('[data-testid="schedule-confirm-decline"]').click();
    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.scriptRunsConfirmDecline)?.args)
      .toMatchObject({ id: 'run-x' });
  });

  test('contract: a waiting run shows no popup in a window the router did not target', async ({
    relaunch,
  }) => {
    const [entry] = contract<RoutedPrompt[]>('prompts-route', 'PromptsService.List#two-waiting');
    if (!entry) throw new Error('prompts-route has no entries');
    const { window: page } = await relaunch({ control: BASE });
    await openPanel(page);
    await emitWailsEvent(page, IPC.scriptRunsChanged, run('waiting', { id: entry.ref }));
    await emitPrompts(page, [{ ...entry, target: 'other-window' }]);
    await expect(page.locator('[data-testid="run-row"][data-state="waiting"]')).toHaveCount(1);
    await expect(page.locator('[data-testid="schedule-confirm"]')).toHaveCount(0);
  });

  test('contract: Operations shows Skipped with its reason; a headless run opens its tab', async ({
    relaunch,
  }) => {
    const overlap = contract<{ outcome: { reason: string } }>(
      'schedule-overlap',
      'ScriptRunsService.Get#skipped',
    );
    const { window: page } = await relaunch({ control: BASE });
    await openPanel(page);
    await emitWailsEvent(
      page,
      IPC.scriptRunsChanged,
      run('skipped', {
        id: 'run-s',
        outcome: overlap.outcome,
      }),
    );
    await expect(page.locator('[data-testid="run-row"][data-state="skipped"]')).toHaveCount(0);
    await page.locator('[data-testid="toggle-operations-panel"]').click();
    const skipped = page.locator('[data-op-id="run:run-s"]');
    await expect(skipped.locator('[data-testid="op-status-note"]')).toHaveText('Skipped');
    await expect(skipped.locator('[data-testid="op-source-cell"]')).toContainText('Scheduled');
    await skipped.click();
    await expect(page.locator('[data-testid="operations-panel"]')).toContainText(
      overlap.outcome.reason,
    );

    await emitWailsEvent(page, IPC.scriptRunsChanged, run('running', { id: 'run-h' }));
    await page.locator('[data-testid="run-row"][data-state="running"] button').first().click();
    const view = page.locator('[data-testid="script-run-view"]');
    await expect(view).toBeVisible();
    await expect(view.locator('[data-testid="script-run-command"]')).toContainText('make build');
    await expect(view.locator('[data-testid="run-log"]')).toBeVisible();
  });

  // Contract schedule-confirm and schedule-now. Backend half: termflow TestScheduleConfirm and
  // TestRunScheduleNow (fake clock) store the same fixtures.
  const CONTRACT_TIMES = { createdAt: 2_000, startedAt: 2_000 };

  test('contract: a waiting run asks, Run starts it headless and it ends Succeeded', async ({
    relaunch,
  }) => {
    const script = contract('schedule-confirm', 'CustomScriptsService.Create', {
      schema: customScriptSchema,
    });
    const preview = contract<{ command: string; dir: { path: string } }>(
      'schedule-confirm',
      'ScriptRunsService.SchedulePreview',
    );
    const waiting = contract('schedule-confirm', 'ScriptRunsService.Get#waiting', {
      schema: scriptRunSchema,
    });
    const done = contract('schedule-confirm', 'ScriptRunsService.Get#done', {
      schema: scriptRunSchema,
    });
    const accept = contract<{ runId: string }>(
      'schedule-confirm',
      'args:ScriptRunsService.ConfirmAccept',
    );
    const log = contract<{ chunks: { text: string }[] }>(
      'schedule-confirm',
      'ScriptRunsService.ReadLog',
    );
    const { window: page, control } = await relaunch({
      control: [
        { channel: IPC.customScriptsList, response: { collections: [], scripts: [script] } },
        { channel: IPC.scriptRunsResolveDir, response: preview.dir },
        { channel: IPC.scriptRunsNextFires, response: FIRES },
        { channel: IPC.promptsMainWindow, response: 'main' },
        { channel: IPC.scriptRunsSchedulePreview, response: preview },
        {
          channel: IPC.scriptRunsConfirmAccept,
          response: contract('schedule-confirm', 'ScriptRunsService.ConfirmAccept'),
        },
        { channel: IPC.scriptRunsReadLog, response: log },
      ],
    });
    await openPanel(page);
    await emitWailsEvent(page, IPC.scriptRunsChanged, {
      ...waiting,
      ...CONTRACT_TIMES,
      startedAt: null,
    });
    await emitPrompts(page, [routed('schedule', waiting.id)]);
    const popup = page.locator('[data-testid="schedule-confirm"]');
    await expect(popup).toContainText(`Run ${script.name}?`);
    await expect(popup.locator('[data-testid="run-command"]')).toContainText(preview.command);
    await popup.locator('[data-testid="schedule-confirm-run"]').click();
    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.scriptRunsConfirmAccept)?.args)
      .toMatchObject({ runId: accept.runId });

    await emitWailsEvent(page, IPC.scriptRunsChanged, {
      ...done,
      ...CONTRACT_TIMES,
      finishedAt: 3_000,
    });
    await expect(page.locator('[data-testid="run-row"][data-state="done"]')).toHaveCount(0);
    await page.locator('[data-testid="toggle-operations-panel"]').click();
    await expect(page.locator(`[data-op-id="run:${done.id}"]`)).toHaveAttribute(
      'data-status',
      'ok',
    );
    await page.locator(`[data-op-id="run:${done.id}"]`).click({ button: 'right' });
    await page.locator('[data-testid="menu-item-open-run"]').click();
    await expect(
      page.locator('[data-testid="script-run-view"] [data-testid="run-log"]'),
    ).toContainText(log.chunks[0].text);
  });

  test('contract: Run now starts the schedule run at once', async ({ relaunch }) => {
    const script = contract('schedule-now', 'CustomScriptsService.Create', {
      schema: customScriptSchema,
    });
    const preview = contract<{ dir: { path: string } }>(
      'schedule-now',
      'ScriptRunsService.SchedulePreview',
    );
    const sent = contract<{ scriptId: string }>(
      'schedule-now',
      'args:ScriptRunsService.RunScheduleNow',
    );
    const { window: page, control } = await relaunch({
      control: [
        { channel: IPC.customScriptsList, response: { collections: [], scripts: [script] } },
        { channel: IPC.scriptRunsResolveDir, response: preview.dir },
        { channel: IPC.scriptRunsNextFires, response: FIRES },
        { channel: IPC.promptsMainWindow, response: 'main' },
        { channel: IPC.scriptRunsSchedulePreview, response: preview },
        {
          channel: IPC.scriptRunsRunScheduleNow,
          response: contract('schedule-now', 'ScriptRunsService.RunScheduleNow'),
        },
      ],
    });
    await openPanel(page);
    await page.locator(`[data-testid="script-${script.id}"]`).click({ button: 'right' });
    await page.locator('[data-testid="menu-item-run-now"]').click();
    const popup = page.locator('[data-testid="schedule-confirm"]');
    await expect(popup.locator('[data-testid="schedule-confirm-due"]')).toHaveText('Now');
    await popup.locator('[data-testid="schedule-confirm-run"]').click();
    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.scriptRunsRunScheduleNow)?.args)
      .toMatchObject({ scriptId: sent.scriptId });
    await expect(popup).toHaveCount(0);
  });

  extra?.({ BASE, SCRIPT, openPanel });
}
