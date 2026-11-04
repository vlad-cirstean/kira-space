import type { Page } from '@playwright/test';
import { scriptRunSchema } from '@shared/domain/scriptRuns';
import { customScriptSchema } from '@shared/domain/scripts';
import { expect, test } from './fixtures';
import { adeBoard, adeFixture, openPlan, SCRIPTS, SMART } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// P242 Part 3: scripts in ADE on the mocked bridge: menus, run dialog, chip, panel block, step card.

const t = (id: string) => `[data-testid="${id}"]`;
const card = (id: string) => `[data-testid="ade-card"][data-task-id="${id}"]`;

const BRANCHES = [
  { id: 'd_alerts_api', label: 'acme-platform-core-api-service', disabled: false, why: '' },
  {
    id: 'd_alerts_web',
    label: 'acme-customer-dashboard-web-frontend',
    disabled: true,
    why: 'This branch has no worktree yet.',
  },
];

function preview(over: Record<string, unknown> = {}) {
  return {
    kind: 'smart',
    missing: [],
    blocker: '',
    dir: {
      path: '/wt/api/alerts',
      mode: 'worktree',
      base: '/kira',
      blocker: '',
      branch: 'api · feat/alerts',
      pending: false,
    },
    body: SMART.command,
    prompt: [
      { text: 'Review ', var: '', value: '' },
      { text: '', var: 'branch', value: 'feat/alerts' },
      { text: ' of ', var: '', value: '' },
      { text: '', var: 'task', value: 'Usage alerts by email' },
      { text: '.', var: '', value: '' },
    ],
    suffix: 'When you are finished, call the finish_step tool.',
    env: [],
    command: '',
    model: 'sonnet',
    maxBudgetUsd: 1,
    timeout: '15m',
    tools: ['Read'],
    allowedTools: ['Read'],
    mcpServers: [],
    hash: 'h1',
    needs: { tasks: [], branches: BRANCHES },
    ade: {
      taskId: 'T_alerts',
      taskTitle: 'Usage alerts by email',
      branchId: 'd_alerts_api',
      branchLabel: 'feat/alerts',
    },
    ...over,
  };
}

function run(state: string, extra: Record<string, unknown> = {}) {
  return {
    id: 'run-1',
    scriptId: SMART.id,
    scriptName: SMART.name,
    color: 'none',
    kind: 'smart',
    trigger: 'manual',
    state,
    terminalId: '',
    cwd: '/wt/api/alerts',
    command: '',
    outcome: null,
    createdAt: 1_000,
    startedAt: 1_000,
    finishedAt: state === 'running' ? null : 4_000,
    model: 'sonnet',
    sessionId: 's1',
    prompt: 'Review feat/alerts.',
    params: [],
    tools: { tools: [], allowedTools: [], mcpServers: [] },
    taskId: 'T_alerts',
    taskTitle: 'Usage alerts by email',
    branchId: 'd_alerts_api',
    branchLabel: 'feat/alerts',
    ...extra,
  };
}

const BASE: ControlSnapshot[] = [
  SCRIPTS,
  { channel: IPC.scriptRunsPreview, response: preview() },
  { channel: IPC.scriptRunsStart, response: { runId: 'run-1', terminal: null } },
  { channel: IPC.scriptRunsList, response: [] },
  { channel: IPC.scriptRunsReadLog, response: { chunks: [], truncated: false } },
];

/** BASE with `extra` snapshots replacing a default of the same channel. */
function withBase(...extra: ControlSnapshot[]): ControlSnapshot[] {
  const over = new Set(extra.map((e) => e.channel));
  return [...BASE.filter((e) => !over.has(e.channel)), ...extra];
}

async function rightClick(page: Page, taskId: string): Promise<void> {
  await page.locator(card(taskId)).click({ button: 'right', position: { x: 60, y: 30 } });
}

test('the task menu lists scripts, smart ones marked, and opens the run dialog on the task', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, BASE);
  await rightClick(page, 'T_alerts');
  await page.locator(t('menu-item-ade-task-automation')).click();
  await expect(page.locator(t('menu-item-ade-automation-plain-1'))).toBeVisible();
  await expect(page.locator(t('menu-item-ade-automation-smart-1'))).toBeVisible();
  await page.locator(t('menu-item-ade-automation-smart-1')).click();

  const dialog = page.locator(t('run-dialog'));
  await expect(dialog).toBeVisible();
  await expect
    .poll(() => (calls(control, IPC.scriptRunsPreview).at(-1)?.args as { taskId?: string })?.taskId)
    .toBe('T_alerts');
  await expect(
    dialog.locator('[data-testid="run-prompt-text"] [data-testid="var-chip"][data-var="branch"]'),
  ).toContainText('feat/alerts');
  await expect(
    dialog.locator('[data-testid="run-folder"] [data-chip="branch"][data-var="branch"]'),
  ).toContainText('api · feat/alerts');
  await expect(dialog.locator(t('run-folder'))).toContainText('Worktree of');
  await expect(dialog.locator(t('run-ade-context'))).toContainText('Usage alerts by email');
});

test('the branch radio disables a branch without a worktree and says why', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, BASE);
  await rightClick(page, 'T_alerts');
  await page.locator(t('menu-item-ade-task-automation')).click();
  await page.locator(t('menu-item-ade-automation-smart-1')).click();
  const dialog = page.locator(t('run-dialog'));
  await dialog.locator(t('run-branch')).click();
  const options = page.locator(t('run-branch-content'));
  await expect(options.locator(t('run-branch-option-d_alerts_api'))).not.toHaveAttribute(
    'data-disabled',
  );
  const web = options.locator(t('run-branch-option-d_alerts_web'));
  await expect(web).toHaveAttribute('data-disabled');
  await expect(web).toContainText('no worktree');

  await options.locator(t('run-branch-option-d_alerts_api')).click();
  await expect
    .poll(
      () => (calls(control, IPC.scriptRunsPreview).at(-1)?.args as { branchId?: string })?.branchId,
    )
    .toBe('d_alerts_api');
  await expect(dialog.locator(t('run-start'))).toBeEnabled();
  await dialog.locator(t('run-start')).click();
  await expect.poll(() => calls(control, IPC.scriptRunsStart)).toHaveLength(1);
  expect(calls(control, IPC.scriptRunsStart)[0]?.args).toMatchObject({
    scriptId: 'smart-1',
    taskId: 'T_alerts',
    branchId: 'd_alerts_api',
    hash: 'h1',
  });
});

test('a blocked preview keeps Run disabled', async ({ relaunch }) => {
  const { window: page } = await openPlan(
    relaunch,
    withBase({
      channel: IPC.scriptRunsPreview,
      response: preview({ blocker: 'an ADE run is working in that branch' }),
    }),
  );
  await rightClick(page, 'T_alerts');
  await page.locator(t('menu-item-ade-task-automation')).click();
  await page.locator(t('menu-item-ade-automation-smart-1')).click();
  const dialog = page.locator(t('run-dialog'));
  await expect(dialog.locator(t('run-blocker'))).toContainText('working in that branch');
  await expect(dialog.locator(t('run-start'))).toBeDisabled();
});

test('a task chip shows a running automation and a failed one until its run is opened', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(
    relaunch,
    withBase({ channel: IPC.scriptRunsList, response: [run('running')] }),
  );
  const chip = page.locator(`${card('T_alerts')} ${t('ade-automation-chip')}`);
  await expect(chip).toHaveAttribute('data-state', 'running');
  await expect(chip).toContainText('Review diff');

  const runs = page.locator(t('status-runs'));
  await expect(runs).toHaveText('1 running');
  await runs.hover();
  await expect(page.locator(t('tooltip-details-title'))).toHaveText('Script runs');
  await expect(page.locator(`${t('tooltip-details-row')}[data-label="Running"]`)).toHaveText(
    /Running\s*1/,
  );
  await expect(page.locator(t('tooltip-details-hint'))).toHaveText('Click to show the runs list.');

  await emitWailsEvent(
    page,
    IPC.scriptRunsChanged,
    run('failed', { outcome: { status: 'failed', reason: 'no diff to review' } }),
  );
  await expect(chip).toHaveAttribute('data-state', 'failed');
  await expect(chip).toContainText('failed');

  await chip.click();
  await expect(chip).toHaveCount(0);
});

test('the task panel lists the task automations and stops a running one', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(
    relaunch,
    withBase(
      { channel: IPC.scriptRunsList, response: [run('running'), run('done', { id: 'run-0' })] },
      { channel: IPC.scriptRunsStop },
    ),
  );
  await page.locator(`${card('T_alerts')} ${t('ade-card-head')}`).click();
  const block = page.locator(t('ade-automations'));
  await expect(block.locator(t('ade-automation-row'))).toHaveCount(2);
  await expect(block.locator(`${t('ade-automation-row')}[data-state="running"]`)).toContainText(
    'feat/alerts',
  );
  await block.locator(t('ade-automation-stop')).click();
  await expect.poll(() => calls(control, IPC.scriptRunsStop)).toHaveLength(1);
  expect(calls(control, IPC.scriptRunsStop)[0]?.args).toEqual({ id: 'run-1' });
});

test('the Runs entry asks for the task and the run dialog lists tasks', async ({ relaunch }) => {
  const { window: page } = await openPlan(
    relaunch,
    withBase({
      channel: IPC.scriptRunsPreview,
      response: preview({
        ade: null,
        dir: {
          path: '/kira',
          mode: 'kira',
          base: '/kira',
          blocker: '',
          branch: '',
          pending: false,
        },
        needs: {
          tasks: [
            { id: 'T_alerts', title: 'Usage alerts by email' },
            { id: 'T_bill', title: 'Billing' },
          ],
          branches: [],
        },
      }),
    }),
  );
  await page.locator('[data-testid="mode-tab"][data-mode="automations"]').click();
  await page.locator(t('automations-panel')).waitFor();
  await page.locator(`${t('script-smart-1')} [data-testid="script-play"]`).click();
  const dialog = page.locator(t('run-dialog'));
  await dialog.locator(t('run-task')).click();
  await expect(page.locator(t('run-task-option-T_bill'))).toBeVisible();
  await page.locator(t('run-task-option-T_alerts')).click();
  await expect(dialog.locator(t('run-task'))).toContainText('Usage alerts by email');
});

function smartStepBoard(): ControlSnapshot {
  const wf = structuredClone(
    adeFixture<{ workflows: { workflow: { steps: Record<string, unknown>[] } }[] }>('workflows')
      .workflows[0]?.workflow,
  );
  const step = wf?.steps.find((x) => x.id === 'impl');
  if (!wf || !step) throw new Error('fixture workflow missing');
  Object.assign(step, { smartScript: SMART.name, prompt: '' });
  return {
    channel: IPC.adeTaskBoard,
    response: adeBoard((b) => {
      const alerts = b.tasks.find((x) => x.id === 'T_alerts');
      if (!alerts) throw new Error('fixture task missing');
      alerts.entryStepId = 'impl';
      alerts.workflowId = 'standard';
      alerts.workflow = wf as never;
      alerts.runs = [];
    }),
  };
}

test('the ADE Run dialog shows a smart step body read-only', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [SCRIPTS, smartStepBoard()]);
  await page
    .locator(
      `[data-testid="ade-task"][data-task-id="T_alerts"] ${t('ade-card-head')} ${t('ade-task-action-run')}`,
    )
    .click();
  const dialog = page.locator(t('ade-run-dialog'));
  await expect(dialog.locator(t('ade-run-smart'))).toContainText('Review diff');
  await expect(
    dialog.locator(`${t('ade-run-smart-body')} [data-testid="var-chip"][data-var="branch"]`),
  ).toBeVisible();
  await expect(dialog.locator(t('ade-run-message'))).toHaveCount(0);
});

test('the step inspector toggles to a smart script and saves smart_script with params', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [SCRIPTS]);
  await page.locator(t('ade-tab-workflows')).click();
  await page.locator(t('ade-workflows')).waitFor();
  await page.locator(`${t('ade-wf-node')}[data-step-id="plan"]`).click();
  const step = page.locator(t('ade-wf-step'));
  await expect(step.locator(t('ade-wf-step-prompt'))).toBeVisible();
  await step.locator(t('ade-wf-step-mode-smart')).click();
  await expect(step.locator(t('ade-wf-step-script'))).toHaveValue('Review diff');
  await expect(step.locator(t('ade-wf-step-prompt'))).toHaveCount(0);
  await expect(
    step.locator(`${t('ade-wf-step-body')} [data-testid="var-chip"][data-var="branch"]`),
  ).toBeVisible();
  await page.locator(t('ade-wf-save')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSaveWorkflow)).toHaveLength(1);
  const args = calls(control, IPC.adeTaskSaveWorkflow)[0]?.args as {
    workflow: { steps: { smartScript: string; prompt: string }[] };
  };
  expect(args.workflow.steps.filter((x) => x.smartScript === 'Review diff')).toHaveLength(1);
});

test('the step inspector hides secret params and names them', async ({ relaunch }) => {
  const param = (name: string, secret: boolean) => ({
    name,
    type: 'text',
    required: true,
    secret,
    default: '',
    options: [],
  });
  const withParams = {
    channel: IPC.customScriptsList,
    response: {
      collections: [],
      scripts: [{ ...SMART, params: [param('level', false), param('token', true)] }],
    },
  } as unknown as ControlSnapshot;
  const { window: page } = await openPlan(relaunch, [withParams]);
  await page.locator(t('ade-tab-workflows')).click();
  await page.locator(t('ade-workflows')).waitFor();
  await page.locator(`${t('ade-wf-node')}[data-step-id="plan"]`).click();
  const step = page.locator(t('ade-wf-step'));
  await step.locator(t('ade-wf-step-mode-smart')).click();
  await expect(step.locator(t('ade-wf-step-secret-params'))).toContainText('token');
  await expect(step.locator('input[type="password"]')).toHaveCount(0);
});

test('the automation submenu shows a disabled hint without scripts', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.customScriptsList, response: { collections: [], scripts: [] } },
  ]);
  await rightClick(page, 'T_alerts');
  await page.locator(t('menu-item-ade-task-automation')).click();
  await expect(page.locator(t('menu-item-ade-automation-none'))).toBeDisabled();
});

// Contract ade-automation. Backend half: adeflow TestAutomationRun ("variables, tools and folder",
// "gate both ways"). The task and branch ids are the fixture board's; every other field is the
// backend's.
const ALERTS = { taskId: 'T_alerts', taskTitle: 'Usage alerts by email', branchId: 'd_alerts_api' };

function contractRun(key: string, extra: Record<string, unknown> = {}) {
  const script = contract('ade-automation', 'CustomScriptsService.Create', {
    schema: customScriptSchema,
  });
  return {
    script,
    run: {
      ...contract('ade-automation', `ScriptRunsService.${key}`, { schema: scriptRunSchema }),
      ...ALERTS,
      createdAt: 1_000,
      startedAt: 1_000,
      ...extra,
    },
  };
}

test('contract: a smart script runs in the task worktree, Running chip then Succeeded', async ({
  relaunch,
}) => {
  const { script, run: running } = contractRun('Get#running');
  const { run: done } = contractRun('Get#done', { finishedAt: 4_000 });
  const { window: page } = await openPlan(
    relaunch,
    withBase(
      { channel: IPC.customScriptsList, response: { collections: [], scripts: [script] } },
      { channel: IPC.scriptRunsList, response: [running] },
    ),
  );
  const chip = page.locator(`${card('T_alerts')} ${t('ade-automation-chip')}`);
  await expect(chip).toHaveAttribute('data-state', 'running');
  await expect(chip).toContainText(script.name);
  await chip.click();
  await expect(page.locator(t('script-run-header')).locator(t('run-status'))).toHaveText('Running');

  await emitWailsEvent(page, IPC.scriptRunsChanged, done);
  await expect(page.locator(t('script-run-header')).locator(t('run-status'))).toHaveText(
    'Succeeded',
  );
});

test('contract: the run dialog shows the backend preview for the task branch', async ({
  relaunch,
}) => {
  const { script } = contractRun('Get#running');
  const pv = contract<{
    prompt: { var: string; value: string }[];
    dir: { branch: string };
    ade: Record<string, string>;
  }>('ade-automation', 'ScriptRunsService.Preview');
  const branch = pv.prompt.find((p) => p.var === 'branch')?.value ?? '';
  const { window: page } = await openPlan(
    relaunch,
    withBase(
      { channel: IPC.customScriptsList, response: { collections: [], scripts: [script] } },
      { channel: IPC.scriptRunsPreview, response: { ...pv, ade: { ...pv.ade, ...ALERTS } } },
    ),
  );
  await rightClick(page, 'T_alerts');
  await page.locator(t('menu-item-ade-task-automation')).click();
  await page.locator(t(`menu-item-ade-automation-${script.id}`)).click();
  const dialog = page.locator(t('run-dialog'));
  await expect(
    dialog.locator('[data-testid="run-prompt-text"] [data-testid="var-chip"][data-var="branch"]'),
  ).toContainText(branch);
  await expect(
    dialog.locator('[data-testid="run-folder"] [data-chip="branch"][data-var="branch"]'),
  ).toContainText(pv.dir.branch);
  await expect(dialog.locator(t('run-start'))).toBeEnabled();
});

test('contract: a step run waits behind a running automation', async ({ relaunch }) => {
  const waiting = contract<Record<string, unknown>>('ade-automation', 'AdeTaskService.Run#waiting');
  const { window: page } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskBoard,
      response: adeBoard((b) => {
        const bill = b.tasks.find((x) => x.id === 'T_bill');
        if (!bill) throw new Error('fixture task missing');
        bill.runs = bill.runs.map((r) =>
          r.id === 'r_T_bill_impl_b_meter'
            ? {
                ...waiting,
                id: r.id,
                taskId: r.taskId,
                stepId: r.stepId,
                branchId: r.branchId,
              }
            : r,
        );
      }),
    },
  ]);
  await page
    .locator('[data-testid="ade-card"][data-task-id="T_bill"] [data-testid="ade-card-head"]')
    .click();
  await expect(page.locator(t('ade-panel'))).toContainText(String(waiting.note));
});
