import { expect, type Page } from '@playwright/test';
import { scriptRunSchema } from '@shared/domain/scriptRuns';
import { customScriptSchema } from '@shared/domain/scripts';
import { type ControlSnapshot, emitWailsEvent } from '../mockRuntime';
import type { ScenarioEnv } from './env';

type Env = ScenarioEnv<
  | 'agentInfo'
  | 'customScriptsCreate'
  | 'customScriptsList'
  | 'customScriptsUpdate'
  | 'scriptRunLog'
  | 'scriptRunsChanged'
  | 'scriptRunsMcpServers'
  | 'scriptRunsPreview'
  | 'scriptRunsReadLog'
  | 'scriptRunsResumeCommand'
  | 'scriptRunsStart'
  | 'terminalOpen'
>;

export type Ctx = { BASE: ControlSnapshot[]; openPanel: (page: Page) => Promise<void> };

export function registerSmartScenarios(env: Env, extra?: (ctx: Ctx) => void): void {
  const { test, contract, ipc: IPC } = env;

  // P242 Part 2: smart scripts on the mocked bridge: the editor, the run dialog, the run tab.

  const SMART = {
    id: 'smart-1',
    kind: 'smart',
    name: 'Ask Claude',
    command: 'Check the {topic} in {env} for {areas}.',
    params: [
      {
        name: 'topic',
        type: 'text',
        options: [],
        default: ['build'],
        required: true,
        secret: false,
      },
      {
        name: 'env',
        type: 'select',
        options: ['dev', 'prod'],
        default: ['dev'],
        required: false,
        secret: false,
      },
      {
        name: 'areas',
        type: 'multiselect',
        options: ['api', 'ui', 'db'],
        default: ['api'],
        required: false,
        secret: false,
      },
    ],
    smart: {
      model: 'sonnet',
      maxBudgetUsd: 1,
      timeout: '15m',
      tools: ['Read', 'Grep', 'Glob'],
      bashPatterns: [],
      mcp: [],
    },
    workingDir: '',
    dirMode: 'kira',
    useAdeDir: false,
    color: 'none',
    collectionId: null,
    sortOrder: 0,
    createdAt: '2026-01-01T00:00:00.000Z',
    updatedAt: '2026-01-01T00:00:00.000Z',
  };

  const DIR = {
    path: '/kira/automations/smart-1',
    mode: 'kira',
    base: '/kira',
    blocker: '',
    branch: '',
    pending: false,
  };
  const ALLOWED = ['Read', 'Grep', 'Glob', 'mcp__kira-ade__finish_step'];

  const PREVIEW = {
    kind: 'smart',
    missing: [],
    blocker: '',
    dir: DIR,
    body: SMART.command,
    prompt: [
      { text: 'Check the ', var: '', value: '' },
      { text: '', var: 'topic', value: 'build' },
      { text: ' in ', var: '', value: '' },
      { text: '', var: 'env', value: 'dev' },
      { text: ' for ', var: '', value: '' },
      { text: '', var: 'areas', value: 'api' },
      { text: '.', var: '', value: '' },
    ],
    suffix: 'When you are finished, call the finish_step tool.',
    env: [
      { name: 'KIRA_PARAM_TOPIC', value: 'build', secret: false, fromVar: 'topic' },
      { name: 'KIRA_PARAM_ENV', value: 'dev', secret: false, fromVar: 'env' },
    ],
    command: '',
    model: 'sonnet',
    maxBudgetUsd: 1,
    timeout: '15m',
    tools: ['Read', 'Grep', 'Glob'],
    allowedTools: ALLOWED,
    mcpServers: [],
    hash: 'h1',
    needs: { tasks: [], branches: [] },
    ade: null,
  };

  const BASE: ControlSnapshot[] = [
    { channel: IPC.customScriptsList, response: { collections: [], scripts: [SMART] } },
    { channel: IPC.scriptRunsPreview, response: PREVIEW },
    { channel: IPC.scriptRunsStart, response: { runId: 'run-9', terminal: null } },
    { channel: IPC.scriptRunsReadLog, response: { chunks: [], truncated: false } },
    { channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } },
  ];

  function smartRun(state: string, extra: Record<string, unknown> = {}) {
    const finished = state !== 'running';
    return {
      id: 'run-9',
      scriptId: SMART.id,
      scriptName: SMART.name,
      color: 'none',
      kind: 'smart',
      trigger: 'manual',
      state,
      terminalId: '',
      cwd: DIR.path,
      command: '',
      outcome: null,
      createdAt: 1_000,
      startedAt: 1_000,
      finishedAt: finished ? 4_000 : null,
      taskId: '',
      taskTitle: '',
      branchId: '',
      branchLabel: '',
      model: 'sonnet',
      sessionId: 'sess-1',
      prompt: 'Check the build in dev for api.',
      params: [],
      tools: { tools: ['Read', 'Grep', 'Glob'], allowedTools: ALLOWED, mcpServers: [] },
      ...extra,
    };
  }

  const modeTab = (page: Page) => page.locator('[data-testid="mode-tab"][data-mode="automations"]');

  async function openPanel(page: Page): Promise<void> {
    await modeTab(page).click();
    await expect(page.locator('[data-testid="automations-panel"]')).toBeVisible();
  }

  test('New smart script: tools, Bash patterns, MCP and params in the editor', async ({
    relaunch,
  }) => {
    const { window: page, control } = await relaunch({
      control: [
        { channel: IPC.customScriptsList, response: { collections: [], scripts: [] } },
        {
          channel: IPC.scriptRunsMcpServers,
          response: {
            source: 'agent',
            servers: [
              { name: 'fake', transport: 'stdio', target: 'fake-mcp', status: 'connected' },
            ],
          },
        },
        { channel: IPC.customScriptsCreate, response: SMART },
      ],
    });
    await openPanel(page);
    await page.locator('[data-testid="automations-add"]').click();
    await page.locator('[data-testid="menu-item-new-smart-script"]').click();

    const dialog = page.locator('[data-testid="script-dialog"]');
    await expect(dialog).toContainText('New smart script');
    await expect(dialog.locator('[data-testid="script-ai-badge"]')).toBeVisible();
    await expect(dialog.locator('[data-testid="smart-tools-default"]')).toHaveText('Default tools');
    await expect(page.getByText(/read-only/i)).toHaveCount(0);
    await expect(dialog.locator('[data-testid="smart-model"]')).toHaveValue('sonnet');
    await expect(dialog.locator('[data-testid="smart-allowed-line"]')).toHaveText(
      '--allowedTools Read Grep Glob',
    );

    await expect(dialog.locator('[data-testid="smart-bash-patterns"]')).toHaveCount(0);
    await dialog.locator('[data-testid="smart-tool-Bash"]').click();
    await expect(dialog.locator('[data-testid="smart-bash-patterns"]')).toBeVisible();
    await dialog.locator('[data-testid="smart-bash-input"]').fill('git status:*');
    await dialog.locator('[data-testid="smart-bash-add"]').click();
    await expect(dialog.locator('[data-testid="smart-allowed-line"]')).toHaveText(
      '--allowedTools Read Grep Glob Bash(git status:*)',
    );
    await dialog.locator('[data-testid="smart-tools-default"]').click();
    await expect(dialog.locator('[data-testid="smart-bash-patterns"]')).toHaveCount(0);

    const sw = dialog.locator('[data-testid="smart-mcp-switch-fake"]');
    await expect(sw).toHaveAttribute('aria-checked', 'false');
    await expect(dialog.locator('[data-testid="smart-mcp-tools"]')).toHaveCount(0);
    await expect(dialog.locator('[data-testid="smart-mcp-status"]')).toHaveText('Online');
    await sw.click();
    const all = dialog.locator('[data-testid="smart-mcp-all"]');
    await expect(all).toHaveAttribute('aria-checked', 'true');
    await expect(dialog.locator('[data-testid="smart-allowed-line"]')).toContainText('mcp__fake');
    await all.click();
    await dialog.locator('[data-testid="smart-mcp-add-input"]').fill('echo');
    await dialog.locator('[data-testid="smart-mcp-add"]').click();
    await expect(dialog.locator('[data-testid="smart-allowed-line"]')).toContainText(
      'mcp__fake__echo',
    );

    await dialog.locator('[data-testid="script-dialog-tab-params"]').click();
    await dialog.locator('[data-testid="param-add"]').click();
    await dialog.locator('[data-testid="param-name-0"]').fill('topic');
    await dialog.locator('[data-testid="param-type-0-select"]').click();
    await dialog.locator('[data-testid="param-option-add-0"]').click();
    await dialog.locator('[data-testid="param-option-0-0"]').fill('dev');

    await dialog.locator('[data-testid="script-dialog-tab-script"]').click();
    await dialog.locator('[data-testid="script-dialog-name"]').fill('Ask Claude');
    await dialog.locator('[data-testid="script-dialog-command"]').fill('Look at {topic}');
    await expect(
      dialog.locator('[data-testid="script-dialog-uses"] [data-var="topic"]'),
    ).toBeVisible();
    await dialog.locator('[data-testid="script-dialog-save"]').click();

    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.customScriptsCreate)?.args)
      .toMatchObject({
        fields: {
          name: 'Ask Claude',
          kind: 'smart',
          command: 'Look at {topic}',
          params: [{ name: 'topic', type: 'select', options: ['dev'] }],
          smart: {
            model: 'sonnet',
            maxBudgetUsd: 1,
            timeout: '15m',
            tools: ['Read', 'Grep', 'Glob'],
            mcp: [{ server: 'fake', allTools: false, tools: ['echo'] }],
          },
        },
      });
  });

  test('contract: hermetic launches list the app agent settings servers, no status', async ({
    relaunch,
  }) => {
    const { window: page } = await relaunch({
      control: [
        { channel: IPC.customScriptsList, response: { collections: [], scripts: [] } },
        {
          channel: IPC.scriptRunsMcpServers,
          response: contract('automations-mcp-app', 'ScriptRunsService.McpServers'),
        },
      ],
    });
    await openPanel(page);
    await page.locator('[data-testid="automations-add"]').click();
    await page.locator('[data-testid="menu-item-new-smart-script"]').click();
    const dialog = page.locator('[data-testid="script-dialog"]');
    await expect(dialog.locator('[data-testid="smart-mcp-switch-app-srv"]')).toBeVisible();
    await expect(dialog.locator('[data-testid="smart-mcp-status"]')).toHaveCount(0);
    await expect(dialog.locator('[data-testid="smart-mcp"]')).toContainText(
      'Servers from Kira’s own agent settings',
    );
  });

  test('the smart model select lists the active provider models', async ({ relaunch }) => {
    const provider = {
      id: 'fake',
      label: 'Fake agent',
      productLabel: 'Fake agent',
      command: 'kira-fake-agent',
      headlessLabel: 'kira-fake-agent headless',
      models: [
        { id: 'fast', label: 'fast' },
        { id: 'deep', label: 'deep' },
      ],
      defaultModel: 'fast',
      hooks: false,
      usage: false,
    };
    const { window: page } = await relaunch({
      control: [
        { channel: IPC.customScriptsList, response: { collections: [], scripts: [] } },
        { channel: IPC.agentInfo, response: provider },
      ],
    });
    await openPanel(page);
    await page.locator('[data-testid="automations-add"]').click();
    await page.locator('[data-testid="menu-item-new-smart-script"]').click();
    const model = page.locator('[data-testid="script-dialog"] [data-testid="smart-model"]');
    await expect(model).toHaveValue('fast');
    const values = await model
      .locator('option')
      .evaluateAll((els) => els.map((e) => (e as HTMLOptionElement).value));
    expect(values).toEqual(['fast', 'deep']);
  });

  test('a smart script row has the AI badge and opens the run dialog', async ({ relaunch }) => {
    const { window: page, control } = await relaunch({ control: BASE });
    await openPanel(page);
    const row = page.locator(`[data-testid="script-${SMART.id}"]`);
    await expect(row.locator('[data-testid="script-ai-badge"]')).toBeVisible();
    await row.locator('[data-testid="script-play"]').click();

    const dialog = page.locator('[data-testid="run-dialog"]');
    await expect(dialog).toBeVisible();
    await expect(dialog.locator('[data-testid="run-param-input-topic"]')).toHaveValue('build');
    await expect(dialog.locator('[data-testid="run-param-option-env-dev"]')).toHaveAttribute(
      'data-state',
      'checked',
    );
    await expect(dialog.locator('[data-testid="run-param-option-areas-api"]')).toHaveAttribute(
      'data-state',
      'checked',
    );
    await expect(
      dialog.locator('[data-testid="run-prompt-text"] [data-testid="var-chip"][data-var="env"]'),
    ).toBeVisible();
    await expect(dialog.locator('[data-testid="run-allowed-tools"]')).toHaveText(
      `--allowedTools ${ALLOWED.join(' ')}`,
    );
    await expect(dialog.locator('[data-testid="run-env-row"]')).toHaveCount(2);
    await expect(dialog).toContainText('Your Claude settings files are not loaded.');
    await expect(dialog.locator('[data-testid="run-start"]')).toBeEnabled();

    await dialog.locator('[data-testid="run-param-option-areas-ui"]').click();
    await expect
      .poll(
        () =>
          control
            .log()
            .filter((e) => e.channel === IPC.scriptRunsPreview)
            .at(-1)?.args,
      )
      .toMatchObject({ params: { areas: ['api', 'ui'] } });
  });

  test('a one-off prompt edit reaches Start and is never saved', async ({ relaunch }) => {
    const { window: page, control } = await relaunch({ control: BASE });
    await openPanel(page);
    await page.locator(`[data-testid="script-${SMART.id}"] [data-testid="script-play"]`).click();
    const dialog = page.locator('[data-testid="run-dialog"]');
    await dialog.locator('[data-testid="run-prompt-edit"]').click();
    await expect(dialog).toContainText('Not saved to the script');
    await dialog.locator('[data-testid="run-prompt-input"]').fill('Only for today: {topic}');
    await expect(dialog.locator('[data-testid="run-start"]')).toBeEnabled();
    await dialog.locator('[data-testid="run-start"]').click();

    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.scriptRunsStart)?.args)
      .toMatchObject({ scriptId: SMART.id, prompt: 'Only for today: {topic}', hash: 'h1' });
    expect(control.log().some((e) => e.channel === IPC.customScriptsUpdate)).toBe(false);
    await expect(page.locator('[data-testid="script-run-view"]')).toBeVisible();
  });

  test('play on a script with a running run reopens its tab and starts nothing', async ({
    relaunch,
  }) => {
    const { window: page, control } = await relaunch({ control: BASE });
    await openPanel(page);
    const play = page.locator(`[data-testid="script-${SMART.id}"] [data-testid="script-play"]`);
    await play.click();
    await page.locator('[data-testid="run-start"]').click();
    await emitWailsEvent(page, IPC.scriptRunsChanged, smartRun('running'));
    const view = page.locator('[data-testid="script-run-view"]');
    await expect(view).toBeVisible();

    await page
      .locator('[data-testid="tab"][data-tab-kind="script-run"] [data-testid="tab-close"]')
      .click();
    await expect(view).toHaveCount(0);
    await play.click();
    await expect(view).toBeVisible();
    await expect(page.locator('[data-testid="run-dialog"]')).toHaveCount(0);
    expect(control.log().filter((e) => e.channel === IPC.scriptRunsStart)).toHaveLength(1);
  });

  test('the run tab follows a run: Running, a pushed log line, then its result', async ({
    relaunch,
  }) => {
    const { window: page } = await relaunch({ control: BASE });
    await openPanel(page);
    await page.clock.install();
    await page.clock.pauseAt(Date.now() + 10_000);
    const now = await page.evaluate(() => Date.now());

    await page.locator(`[data-testid="script-${SMART.id}"] [data-testid="script-play"]`).click();
    await page.locator('[data-testid="run-start"]').click();
    await emitWailsEvent(
      page,
      IPC.scriptRunsChanged,
      smartRun('running', { startedAt: now, createdAt: now }),
    );

    const view = page.locator('[data-testid="script-run-view"]');
    await expect(view.locator('[data-testid="run-status"]')).toHaveText('Running');
    await page.clock.runFor(3000);
    await expect(view.locator('[data-testid="run-elapsed"]')).toHaveText('0:03');

    await emitWailsEvent(page, IPC.scriptRunLog, {
      runId: 'run-9',
      chunks: [{ seq: 1, stream: 'stdout', text: '▸ Read README.md\n' }],
    });
    await expect(view.locator('[data-testid="run-log-line"]')).toHaveText('▸ Read README.md');

    await emitWailsEvent(
      page,
      IPC.scriptRunsChanged,
      smartRun('done', {
        startedAt: now,
        createdAt: now,
        finishedAt: now + 5000,
        outcome: {
          status: 'done',
          reason: 'Finished',
          source: 'agent',
          reported: true,
          summary: 'All good',
          costUsd: 0.12,
        },
      }),
    );
    await expect(view.locator('[data-testid="run-status"]')).toHaveText('Succeeded');
    await expect(view.locator('[data-testid="run-outcome-summary"]')).toHaveText('All good');
    await expect(view.locator('[data-testid="run-outcome-cost"]')).toContainText('Cost $0.12');
    await expect(view.locator('[data-testid="run-continue"]')).toHaveCount(0);
  });

  test('a failed run shows its reason and Continue in terminal resumes the session', async ({
    relaunch,
  }) => {
    const { window: page, control } = await relaunch({
      control: [
        ...BASE,
        {
          channel: IPC.scriptRunsResumeCommand,
          response: contract('smart', 'ScriptRunsService.ResumeCommand'),
        },
      ],
    });
    await openPanel(page);
    await page.locator(`[data-testid="script-${SMART.id}"] [data-testid="script-play"]`).click();
    await page.locator('[data-testid="run-start"]').click();
    await emitWailsEvent(
      page,
      IPC.scriptRunsChanged,
      smartRun('failed', {
        outcome: {
          status: 'failed',
          reason: 'Stopped at the 1 USD budget',
          source: 'budget',
          reported: false,
          costUsd: 1.01,
          permissionDenials: ['Bash'],
        },
      }),
    );
    const view = page.locator('[data-testid="script-run-view"]');
    await expect(view.locator('[data-testid="run-outcome-reason"]')).toHaveText(
      'Stopped at the 1 USD budget',
    );
    await expect(view.locator('[data-testid="run-outcome-denials"]')).toContainText('Bash');
    await view.locator('[data-testid="run-continue"]').click();
    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.terminalOpen)?.args)
      .toMatchObject({
        command: contract<{ command: string }>('smart', 'ScriptRunsService.ResumeCommand').command,
        launchKind: 'agent',
      });
  });

  test('a normal script with params starts through a launch token', async ({ relaunch }) => {
    const normal = {
      ...SMART,
      id: 'plain-1',
      kind: 'script',
      name: 'Deploy',
      command: 'deploy.sh',
      smart: null,
      params: [SMART.params[0]],
    };
    const { window: page, control } = await relaunch({
      control: [
        { channel: IPC.customScriptsList, response: { collections: [], scripts: [normal] } },
        {
          channel: IPC.scriptRunsPreview,
          response: { ...PREVIEW, kind: 'script', command: 'deploy.sh' },
        },
        {
          channel: IPC.scriptRunsStart,
          response: { runId: 'run-3', terminal: { token: 'tok-1', cwd: DIR.path } },
        },
        { channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } },
      ],
    });
    await openPanel(page);
    await page.locator('[data-testid="script-plain-1"] [data-testid="script-play"]').click();
    await expect(page.locator('[data-testid="run-command"]')).toHaveText('deploy.sh');
    await page.locator('[data-testid="run-start"]').click();
    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.terminalOpen)?.args)
      .toMatchObject({ scriptId: 'plain-1', scriptLaunchToken: 'tok-1', launchKind: 'script' });
  });

  // Contract smart. Backend half: termflow TestSmartScript (Studio) / TestSmartScriptSpace (Space) ("done run", "outcomes") stores the same
  // fixture against the fake claude.
  for (const outcome of [
    { key: 'done', label: 'Succeeded', summary: 'summary-done' },
    { key: 'failed', label: 'Failed', summary: 'summary-failed' },
  ] as const) {
    test(`contract: a smart script run ends ${outcome.label} with the agent summary`, async ({
      relaunch,
    }) => {
      const suffix = outcome.key === 'done' ? '' : '#failed';
      const script = contract('smart', `CustomScriptsService.Create${suffix}`, {
        schema: customScriptSchema,
      });
      const finished = contract('smart', `ScriptRunsService.Get#${outcome.key}`, {
        schema: scriptRunSchema,
      });
      const preview = contract<{ dir: unknown }>('smart', 'ScriptRunsService.Preview');
      const { window: page, control } = await relaunch({
        control: [
          { channel: IPC.customScriptsList, response: { collections: [], scripts: [script] } },
          { channel: IPC.scriptRunsPreview, response: preview },
          { channel: IPC.scriptRunsStart, response: { runId: finished.id, terminal: null } },
          { channel: IPC.scriptRunsReadLog, response: { chunks: [], truncated: false } },
          { channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } },
        ],
      });
      await openPanel(page);
      await page.locator(`[data-testid="script-${script.id}"] [data-testid="script-play"]`).click();
      await expect(page.locator('[data-testid="run-start"]')).toBeEnabled();
      await page.locator('[data-testid="run-start"]').click();
      await expect
        .poll(() => control.log().find((e) => e.channel === IPC.scriptRunsStart)?.args)
        .toMatchObject({ scriptId: script.id });

      const times = { createdAt: 1_000, startedAt: 1_000 };
      await emitWailsEvent(page, IPC.scriptRunsChanged, {
        ...finished,
        ...times,
        state: 'running',
        outcome: null,
        finishedAt: null,
      });
      await emitWailsEvent(page, IPC.scriptRunsChanged, {
        ...finished,
        ...times,
        finishedAt: 4_000,
      });
      const view = page.locator('[data-testid="script-run-view"]');
      await expect(view.locator('[data-testid="run-status"]')).toHaveText(outcome.label);
      await expect(view.locator('[data-testid="run-outcome-summary"]')).toHaveText(outcome.summary);
    });
  }

  extra?.({ BASE, openPanel });
}
