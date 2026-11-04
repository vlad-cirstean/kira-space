import { expect, type Page } from '@playwright/test';
import { scriptRunSchema } from '@shared/domain/scriptRuns';
import { type ControlSnapshot, emitWailsEvent } from '../mockRuntime';
import type { ScenarioEnv } from './env';

type Env = ScenarioEnv<
  | 'customScriptsFile'
  | 'customScriptsList'
  | 'scriptRunsChanged'
  | 'scriptRunsList'
  | 'scriptRunsPreview'
  | 'scriptRunsReadLog'
  | 'scriptRunsResolveDir'
  | 'scriptRunsStart'
  | 'terminal'
  | 'terminalOpen'
>;

export interface Options {
  extraBase: readonly ControlSnapshot[];
  modeTab: (page: Page) => ReturnType<Page['locator']>;
}

// One tab per script. Backend half: termflow TestScriptRunTwice, whose contract
// ScriptRunsService.List#twice is the run list these tabs key on.
export function registerTabScenarios(env: Env, options: Options): void {
  const { test, contract, ipc: IPC } = env;

  const common = {
    params: [],
    smart: null,
    workingDir: '',
    dirMode: 'kira',
    useAdeDir: false,
    collection: '',
    sortOrder: 0,
    createdAt: '2026-01-01T00:00:00.000Z',
    updatedAt: '2026-01-01T00:00:00.000Z',
  };
  const PLAIN = {
    ...common,
    id: 'plain-1',
    kind: 'script',
    name: 'Build all',
    command: 'make build',
    color: 'blue',
  };
  const SMART = {
    ...common,
    id: 'smart-1',
    kind: 'smart',
    name: 'Ask Claude',
    command: 'Check the build.',
    color: 'none',
    smart: {
      model: 'sonnet',
      maxBudgetUsd: 1,
      noCostCap: false,
      timeout: '15m',
      tools: ['Read'],
      bashPatterns: [],
      mcp: [],
    },
  };
  const DIR = {
    path: '/kira/automations/plain-1',
    mode: 'kira',
    base: '/kira',
    blocker: '',
    branch: '',
    pending: false,
  };
  const PREVIEW = {
    kind: 'smart',
    missing: [],
    blocker: '',
    dir: DIR,
    body: SMART.command,
    prompt: [{ text: SMART.command, var: '', value: '' }],
    suffix: '',
    env: [],
    command: '',
    model: 'sonnet',
    maxBudgetUsd: 1,
    timeout: '15m',
    tools: ['Read'],
    allowedTools: ['Read'],
    mcpServers: [],
    hash: 'h1',
    needs: { tasks: [], branches: [] },
    ade: null,
  };

  const START_ARGS = {
    scriptId: SMART.id,
    params: {},
    taskId: '',
    branchId: '',
    listTasks: false,
    hash: 'h1',
  };

  const BASE: ControlSnapshot[] = [
    ...options.extraBase,
    { channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } },
    { channel: IPC.scriptRunsResolveDir, response: DIR },
    { channel: IPC.scriptRunsList, response: [] },
    { channel: IPC.scriptRunsPreview, response: PREVIEW },
    { channel: IPC.scriptRunsReadLog, response: { chunks: [], truncated: false } },
    {
      channel: IPC.scriptRunsStart,
      args: { ...START_ARGS, prompt: null },
      response: { runId: 'run-1', terminal: null },
    },
    {
      channel: IPC.scriptRunsStart,
      args: { ...START_ARGS, prompt: 'again' },
      response: { runId: 'run-2', terminal: null },
    },
    { channel: IPC.customScriptsList, response: { collections: [], scripts: [PLAIN, SMART] } },
    {
      channel: IPC.customScriptsFile,
      response: {
        file: `${PLAIN.id}.yaml`,
        id: PLAIN.id,
        path: `/kira/runs/${PLAIN.id}.yaml`,
        text: 'name: Lint\n',
        hash: 'h1',
        problem: null,
        lastGood: false,
        editable: true,
      },
    },
  ];

  const row = (page: Page, id: string) => page.locator(`[data-testid="script-${id}"]`);
  const play = (page: Page, id: string) =>
    page.locator(`[data-testid="script-${id}"] [data-testid="script-play"]`);
  const tabs = (page: Page) =>
    page.locator('[data-testid="tab-strip-wrapper"] [data-testid="tab"]');

  async function openPanel(page: Page): Promise<void> {
    await options.modeTab(page).click();
    await expect(page.locator('[data-testid="automations-panel"]')).toBeVisible();
  }

  function opens(control: { log(): { channel: string; args?: unknown }[] }): string[] {
    return control
      .log()
      .filter((e) => e.channel === IPC.terminalOpen)
      .map((e) => (e.args as { terminalId: string }).terminalId);
  }
  const starts = (control: { log(): { channel: string }[] }) =>
    control.log().filter((e) => e.channel === IPC.scriptRunsStart).length;

  function plainRun(terminalId: string, id: string, state: string, createdAt: number) {
    const finished = state !== 'running';
    return {
      id,
      scriptId: PLAIN.id,
      scriptName: PLAIN.name,
      color: PLAIN.color,
      kind: 'script',
      trigger: 'terminal',
      state,
      terminalId,
      cwd: DIR.path,
      command: PLAIN.command,
      outcome: finished
        ? { status: state, reason: 'finished', source: 'exit', reported: false, exitCode: 0 }
        : null,
      createdAt,
      startedAt: createdAt,
      finishedAt: finished ? createdAt + 1_000 : null,
      taskId: '',
      taskTitle: '',
      branchId: '',
      branchLabel: '',
    };
  }

  function smartRun(id: string, state: string, createdAt: number, summary = '') {
    const finished = state !== 'running';
    return {
      ...plainRun('', id, state, createdAt),
      scriptId: SMART.id,
      scriptName: SMART.name,
      color: 'none',
      kind: 'smart',
      trigger: 'manual',
      command: '',
      model: 'sonnet',
      sessionId: 's1',
      prompt: SMART.command,
      tools: { tools: ['Read'], allowedTools: ['Read'], mcpServers: [] },
      outcome: finished
        ? { status: state, reason: 'Finished', source: 'agent', reported: true, summary }
        : null,
    };
  }

  async function exit(page: Page, terminalId: string): Promise<void> {
    await emitWailsEvent(page, IPC.terminal, { terminalId, data: '', exited: true, exitCode: 0 });
  }

  test('a row click selects it; double-click and Enter open the file; nothing starts', async ({
    relaunch,
  }) => {
    const { window: page, control } = await relaunch({ control: BASE });
    await openPanel(page);
    await row(page, PLAIN.id).click();
    await expect(row(page, PLAIN.id)).toHaveAttribute('aria-selected', 'true');
    await expect(tabs(page)).toHaveCount(0);
    await expect(page.locator('[data-testid="run-file-dialog"]')).toHaveCount(0);

    await row(page, PLAIN.id).dblclick();
    await expect(page.locator('[data-testid="run-file-dialog"]')).toBeVisible();
    await page.locator('[data-testid="run-file-cancel"]').click();
    await expect(page.locator('[data-testid="run-file-dialog"]')).toHaveCount(0);

    await row(page, PLAIN.id).focus();
    await page.keyboard.press('Enter');
    await expect(page.locator('[data-testid="run-file-dialog"]')).toBeVisible();
    await page.locator('[data-testid="run-file-cancel"]').click();

    expect(opens(control)).toHaveLength(0);
    expect(starts(control)).toBe(0);
    await expect(tabs(page)).toHaveCount(0);
  });

  test('play while the script runs focuses its tab; after it ends the tab is replaced', async ({
    relaunch,
  }) => {
    const { window: page, control } = await relaunch({ control: BASE });
    await openPanel(page);
    await play(page, PLAIN.id).click();
    await expect(tabs(page)).toHaveCount(1);
    await expect.poll(() => opens(control).length).toBe(1);
    const [first] = opens(control);
    await emitWailsEvent(
      page,
      IPC.scriptRunsChanged,
      plainRun(first ?? '', 'r1', 'running', 1_000),
    );

    await play(page, PLAIN.id).click();
    await expect(tabs(page)).toHaveCount(1);
    expect(opens(control)).toHaveLength(1);
    await expect(tabs(page).first()).toHaveAttribute('data-active', 'true');

    await exit(page, first ?? '');
    await emitWailsEvent(page, IPC.scriptRunsChanged, plainRun(first ?? '', 'r1', 'done', 1_000));
    await play(page, PLAIN.id).click();
    await expect.poll(() => opens(control).length).toBe(2);
    await expect(tabs(page)).toHaveCount(1);
    const second = opens(control)[1] ?? '';
    expect(second).not.toBe(first);

    // The run list of TestScriptRunTwice: the new run running, the old one done.
    const twice = contract('automations', 'ScriptRunsService.List#twice', {
      schema: scriptRunSchema.array(),
    });
    const [newest, oldest] = twice;
    await emitWailsEvent(page, IPC.scriptRunsChanged, { ...oldest, terminalId: first });
    await emitWailsEvent(page, IPC.scriptRunsChanged, {
      ...newest,
      scriptId: PLAIN.id,
      terminalId: second,
    });
    await expect(
      page.locator('[data-testid="script-run-strip"] [data-testid="run-status"]'),
    ).toHaveText('Running');
    await expect(tabs(page)).toHaveCount(1);
  });

  test('the context menu Run goes through the same gate', async ({ relaunch }) => {
    const { window: page, control } = await relaunch({ control: BASE });
    await openPanel(page);
    await play(page, PLAIN.id).click();
    const [first] = opens(control);
    await emitWailsEvent(
      page,
      IPC.scriptRunsChanged,
      plainRun(first ?? '', 'r1', 'running', 1_000),
    );
    await row(page, PLAIN.id).click({ button: 'right' });
    await page.locator('[data-testid="menu-item-run"]').click();
    await expect(tabs(page)).toHaveCount(1);
    expect(opens(control)).toHaveLength(1);
  });

  test('closing the tab lets the next run open a new one', async ({ relaunch }) => {
    const { window: page, control } = await relaunch({ control: BASE });
    await openPanel(page);
    await play(page, PLAIN.id).click();
    await expect(tabs(page)).toHaveCount(1);
    await expect.poll(() => opens(control)).toHaveLength(1);
    const [first] = opens(control);
    await exit(page, first ?? '');
    await emitWailsEvent(page, IPC.scriptRunsChanged, plainRun(first ?? '', 'r1', 'done', 1_000));
    await tabs(page).first().locator('[data-testid="tab-close"]').click();
    await expect(tabs(page)).toHaveCount(0);
    await play(page, PLAIN.id).click();
    await expect(tabs(page)).toHaveCount(1);
    await expect.poll(() => opens(control)).toHaveLength(2);
  });

  test('a smart script keeps one run tab; Operations opens an older run in it', async ({
    relaunch,
  }) => {
    const { window: page, control } = await relaunch({ control: BASE });
    await openPanel(page);
    const dialog = page.locator('[data-testid="run-dialog"]');
    await play(page, SMART.id).click();
    await expect(dialog.locator('[data-testid="run-start"]')).toBeEnabled();
    await dialog.locator('[data-testid="run-start"]').click();
    await expect(page.locator('[data-testid="script-run-view"]')).toBeVisible();
    await emitWailsEvent(page, IPC.scriptRunsChanged, smartRun('run-1', 'running', 1_000));

    await play(page, SMART.id).click();
    await expect(dialog).toHaveCount(0);
    expect(starts(control)).toBe(1);
    await expect(tabs(page)).toHaveCount(1);

    await emitWailsEvent(page, IPC.scriptRunsChanged, smartRun('run-1', 'done', 1_000, 'first'));
    await play(page, SMART.id).click();
    await dialog.locator('[data-testid="run-prompt-edit"]').click();
    await dialog.locator('[data-testid="run-prompt-input"]').fill('again');
    await expect(dialog.locator('[data-testid="run-start"]')).toBeEnabled();
    await dialog.locator('[data-testid="run-start"]').click();
    await expect.poll(() => starts(control)).toBe(2);
    await emitWailsEvent(page, IPC.scriptRunsChanged, smartRun('run-2', 'done', 2_000, 'second'));
    await expect(tabs(page)).toHaveCount(1);
    await expect(page.locator('[data-testid="run-outcome-summary"]')).toHaveText('second');

    // Finished runs live in the Operations panel.
    await expect(page.locator('[data-testid="run-row"]')).toHaveCount(0);
    await page.locator('[data-testid="toggle-operations-panel"]').click();
    await page.locator('[data-op-id="run:run-1"]').click({ button: 'right' });
    await page.locator('[data-testid="menu-item-open-run"]').click();
    await expect(tabs(page)).toHaveCount(1);
    await expect(page.locator('[data-testid="run-outcome-summary"]')).toHaveText('first');
  });
}
