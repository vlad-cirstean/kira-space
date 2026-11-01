import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { expect, type Page } from '@playwright/test';
import type { ContractJson } from '@workbench/testing/ui/contract';
import type { KiraApp, RelaunchOptions } from '../fixtures';
import { contract } from './contract';
import { calls } from './ipcCalls';
import { IPC } from './ipcChannels';
import { emitWailsEvent } from './mockRuntime';
import type { ControlSnapshot } from './types';

// The ade v2 fixtures are the P143 captures; `FIXED_NOW` is the day they are anchored to
// (Tue 2026-09-22), so band labels and relative times match the committed data.
export const FIXED_NOW = 1_790_070_000_000;

const FIXTURE_DIR = resolve(__dirname, '../../fixtures/ade-v2');

export function adeFixture<T = unknown>(name: string): T {
  return JSON.parse(readFileSync(resolve(FIXTURE_DIR, `${name}.json`), 'utf8')) as T;
}

/** The task issue of a task no tracker recognises: the Task tab shows no issue row. */
const NO_TRACKER_ISSUE = {
  provider: '',
  label: '',
  key: '',
  url: '',
  title: '',
  status: '',
  statusCategory: '',
  summary: { known: false, count: 0, state: '', providers: [] },
  changeRequests: [],
  reason: 'noTracker',
  changeRequestsReason: '',
};

const REBASE_PREVIEW = {
  prompt: 'Rebase feat/billing onto origin/main.',
  suffix: 'Report the outcome with finish_step.',
  stack: [],
  blockers: [],
  noOp: false,
};

const REPO_BRANCHES = {
  mainName: 'main',
  previous: '',
  branches: [
    {
      name: 'develop',
      local: true,
      remote: true,
      branchId: '',
      taskId: '',
      taskTitle: '',
      draft: false,
      excluded: '',
    },
  ],
};

function repoRecord(id: string, name: string, order: number, color: string) {
  return {
    id,
    name,
    root: `/tmp/${name}`,
    repoId: `/tmp/${name}`,
    sortOrder: order,
    color,
    createdAt: '2026-01-01T00:00:00.000Z',
    hidden: false,
  };
}

const REPO_RECORDS = [
  repoRecord('repo-web-app', 'acme-customer-dashboard-web-frontend', 1, 'blue'),
  repoRecord('repo-api', 'acme-platform-core-api-service', 2, 'green'),
  repoRecord('repo-mobile', 'acme-mobile-react-native-app', 3, 'violet'),
];

function firstWorkflow(): unknown {
  return adeFixture<{ workflows: unknown[] }>('workflows').workflows[0];
}

/** `ade-archived` contract answer of `ArchivedTask` ('live' = host answered, 'snapshot' = stored PR),
 *  placed on the board's archived task `taskId`/`codeRepoId` and given real ids and times. */
export function archivedTaskFx(
  taskId = 'H_1',
  source: 'live' | 'snapshot' = 'live',
  codeRepoId = 'repo-api',
): ContractJson {
  const fx = contract<ContractJson>('ade-archived', `AdeTaskService.ArchivedTask#${source}`);
  fx.task.id = taskId;
  fx.task.createdAt = FIXED_NOW - 864e5;
  fx.archivedAt = FIXED_NOW - 36e5;
  fx.statuses = fx.statuses.map((s: ContractJson, i: number) => ({
    ...s,
    id: ['todo', 'in-progress', 'in-review', 'done'][i],
  }));
  fx.repos = fx.repos.map((r: ContractJson) => ({ ...r, codeRepoId }));
  fx.branches = fx.branches.map((b: ContractJson, i: number) => ({
    ...b,
    id: `${taskId}_b${i}`,
    codeRepoId,
    addedAt: FIXED_NOW - 864e5,
    archivedAt: FIXED_NOW - 36e5,
  }));
  fx.task.branchIds = fx.branches.map((b: ContractJson) => b.id);
  return fx;
}

/** The committed board with `edit` applied to a copy, for a spec that needs another task state. */
export function adeBoard(edit: (board: AdeBoardFx) => void): AdeBoardFx {
  const board = adeFixture<AdeBoardFx>('board');
  edit(board);
  return board;
}

/** Fields of `board.json` the specs edit; the rest passes through untyped. */
export interface AdeBoardFx {
  tasks: ({
    id: string;
    entryStepId: string;
    workflowId: string;
    runs: Record<string, unknown>[];
  } & Record<string, unknown>)[];
  branches: ({ id: string; setup: Record<string, unknown> | null } & Record<string, unknown>)[];
  [key: string]: unknown;
}

/** Boots straight into the ade mode with the committed board, PR, workflow and repo fixtures;
 *  `extra` snapshots replace a default of the same channel. */
export function adeV2Control(extra: readonly ControlSnapshot[] = []): ControlSnapshot[] {
  const defaults: ControlSnapshot[] = [
    { channel: IPC.windowsEnsure, response: { mode: 'ade' } },
    { channel: IPC.codeWorkspaceListRepos, response: REPO_RECORDS },
    { channel: IPC.adeTaskBoard, response: adeFixture('board') },
    { channel: IPC.adeTaskChangeRequests, response: adeFixture('change-requests') },
    { channel: IPC.gitHostProviders, response: adeFixture('host-providers') },
    { channel: IPC.adeTaskWorkflows, response: adeFixture('workflows') },
    { channel: IPC.adeTaskRepos, response: adeFixture('repos') },
    { channel: IPC.adeTaskSetPlan },
    { channel: IPC.adeTaskShelveTask },
    { channel: IPC.adeTaskCaptureTask, response: adeFixture('task') },
    { channel: IPC.adeTaskUpdateTask, response: adeFixture('task') },
    { channel: IPC.adeTaskAddTaskRepo, response: adeFixture('branch') },
    { channel: IPC.adeTaskWorkflowYaml, response: adeFixture('workflow-yaml') },
    { channel: IPC.adeTaskValidateWorkflowYaml, response: adeFixture('workflow-validation') },
    { channel: IPC.configFilesOpen },
    { channel: IPC.configFilesReveal },
    { channel: IPC.adeTaskSaveWorkflow, response: firstWorkflow() },
    { channel: IPC.adeTaskSaveWorkflowYaml, response: firstWorkflow() },
    { channel: IPC.adeTaskImportWorkflow, response: firstWorkflow() },
    { channel: IPC.adeTaskNewWorkflow, response: firstWorkflow() },
    {
      channel: IPC.adeTaskUpdateRepo,
      response: adeFixture<{ repos: unknown[] }>('repos').repos[0],
    },
    { channel: IPC.adeTaskAddFolder, response: adeFixture('folder-import') },
    {
      channel: IPC.adeTaskSetFolderWatch,
      response: adeFixture<{ folders: unknown[] }>('repos').folders[0],
    },
    { channel: IPC.adeTaskRemoveFolder },
    { channel: IPC.codeWorkspaceImportRepo, response: REPO_RECORDS[0] },
    { channel: IPC.adeTaskStartRun, response: adeFixture('start-run') },
    { channel: IPC.adeTaskSetTaskWorkflow, response: adeFixture('task') },
    { channel: IPC.adeTaskApprove },
    { channel: IPC.adeTaskRetryRun },
    { channel: IPC.adeTaskStepDone, response: adeFixture('task') },
    { channel: IPC.adeTaskSetTaskStep, response: adeFixture('task') },
    { channel: IPC.adeTaskHandovers, response: { handovers: [] } },
    { channel: IPC.adeTaskUpdateHandover },
    { channel: IPC.adeTaskDismissHandover },
    { channel: IPC.adeTaskArtifacts, response: { artifacts: [], gaps: [] } },
    { channel: IPC.adeTaskIssue, response: NO_TRACKER_ISSUE },
    { channel: IPC.adeTaskAddArtifact },
    { channel: IPC.adeTaskRemoveArtifact },
    {
      channel: IPC.adeTaskSaveStatuses,
      response: (() => {
        const b = adeFixture<{ statuses: unknown[]; statusDefaults: unknown }>('board');
        return { statuses: b.statuses, defaults: b.statusDefaults };
      })(),
    },
    { channel: IPC.adeTaskRetrySetup },
    { channel: IPC.adeTaskReadLog, response: adeFixture('log-page') },
    { channel: IPC.adeTaskSessions, response: adeFixture('sessions') },
    { channel: IPC.adeTaskStopRun },
    { channel: IPC.adeTaskTakeOver, response: adeFixture('launch') },
    { channel: IPC.adeTaskRestartSession, response: adeFixture('launch') },
    { channel: IPC.adeTaskCloseSession, response: null },
    { channel: IPC.adeTaskLaunchStep, response: adeFixture('launch') },
    { channel: IPC.adeTaskStartBranch, response: adeFixture('launch') },
    { channel: IPC.adeTaskStartAdHoc, response: adeFixture('launch') },
    { channel: IPC.adeTaskSend },
    { channel: IPC.adeTaskFocusSession, response: true },
    { channel: IPC.adeTaskArchiveRisk, response: adeFixture('archive-risk') },
    { channel: IPC.adeTaskArchiveTask },
    { channel: IPC.adeTaskArchivedTask, response: archivedTaskFx() },
    { channel: IPC.adeTaskRestoreTask, response: adeFixture('task') },
    { channel: IPC.adeTaskRecordMerge },
    { channel: IPC.adeTaskSetQueuedAfter },
    { channel: IPC.adeTaskRebasePreview, response: REBASE_PREVIEW },
    { channel: IPC.adeTaskRebase, response: { runId: 'run-rebase', noOp: false } },
    { channel: IPC.adeTaskAbortRebase },
    { channel: IPC.adeTaskSetBranchBase },
    { channel: IPC.adeTaskSetTaskBranch, response: adeFixture('branch') },
    { channel: IPC.adeTaskUpdateTaskRepo, response: adeFixture('branch') },
    { channel: IPC.adeTaskRemoveTaskBranch },
    { channel: IPC.adeTaskRemoveTaskRepo },
    { channel: IPC.adeTaskTaskBranchRisk, response: adeFixture('branch-risk-clean') },
    { channel: IPC.adeTaskRepoBranches, response: REPO_BRANCHES },
    { channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } },
  ];
  const overridden = new Set(extra.map((s) => s.channel));
  return [...defaults.filter((s) => !overridden.has(s.channel)), ...extra];
}

/** Opens the ade Plan on the fixture day; the clock is pinned before the first navigation. */
export async function openPlan(
  relaunch: (options?: RelaunchOptions) => Promise<KiraApp>,
  extra: readonly ControlSnapshot[] = [],
): Promise<KiraApp> {
  const app = await relaunch({ control: adeV2Control(extra), clockTime: FIXED_NOW });
  await app.window.locator('[data-testid="ade-plan"]').waitFor();
  return app;
}

/** Opens the Sessions tab of task `taskId`'s panel. */
export async function openSessions(page: Page, taskId: string): Promise<void> {
  await page
    .locator(`[data-testid="ade-card"][data-task-id="${taskId}"] [data-testid="ade-card-head"]`)
    .click();
  await page.locator('[data-testid="ade-panel-tab-sessions"]').click();
  await expect(page.locator('[data-testid="ade-sessions-tab"]')).toBeVisible();
}

export function bandOf(page: Page, day: number) {
  return page.locator(`[data-testid="ade-day-band"][data-ade-day="${day}"]`);
}

/** Pushes `runs` as an `ade:task:runs` event. */
export async function emitRuns(page: Page, runs: Record<string, unknown>[]): Promise<void> {
  await emitWailsEvent(page, IPC.adeTaskRunsChanged, { runs });
}

/** Pushes log `chunks` of one run or setup. */
export async function emitLog(
  page: Page,
  kind: 'run' | 'setup',
  id: string,
  chunks: { seq: number; at: number; stream: string; text: string }[],
): Promise<void> {
  await emitWailsEvent(page, IPC.adeTaskLogChanged, { kind, id, chunks });
}

/** Pushes an `ade:task:sessions` event; the renderer refetches the (static) sessions snapshot. */
export async function emitSessions(page: Page): Promise<void> {
  await emitWailsEvent(page, IPC.adeTaskSessionsChanged, null);
}

/** Pushes an `ade:task:open-session` event (the backend asking the window to show a session). */
export async function emitOpenSession(
  page: Page,
  event: { taskId: string; branchId: string; sessionId: string },
): Promise<void> {
  await emitWailsEvent(page, IPC.adeTaskOpenSession, event);
}

/** Pushes one Claude Code hook event for a terminal (`UserPromptSubmit`, `Stop`, `SessionEnd`, ...). */
export async function emitAgentEvent(
  page: Page,
  terminalId: string,
  event: string,
  cwd = '',
): Promise<void> {
  await emitWailsEvent(page, IPC.agentEvent, {
    terminalId,
    event,
    sessionId: '',
    cwd,
    toolName: '',
    toolUseId: '',
    notificationType: '',
    message: '',
    source: '',
    reason: '',
    lastAssistantMessage: '',
  });
}

/** Pushes the live agent-session list (one entry per Claude Code terminal). */
export async function emitAgentSessions(page: Page, terminalIds: readonly string[]): Promise<void> {
  await emitWailsEvent(page, IPC.agentSessions, {
    sessions: terminalIds.map((terminalId) => ({ terminalId, cwd: '' })),
  });
}

/** Opens the bill task's panel from its card. */
export async function openBillTask(page: Page): Promise<void> {
  await page
    .locator('[data-testid="ade-card"][data-task-id="T_bill"] [data-testid="ade-card-head"]')
    .click();
  await page.locator('[data-testid="ade-panel"]').waitFor();
}

/** Boots into the Workflows tab. */
export async function openWorkflows(
  relaunch: Parameters<typeof openPlan>[0],
  extra: readonly ControlSnapshot[] = [],
) {
  const app = await openPlan(relaunch, extra);
  await app.window.locator('[data-testid="ade-tab-workflows"]').click();
  await app.window.locator('[data-testid="ade-workflows"]').waitFor();
  return app;
}

/** A custom script record as `customScriptsList` returns it. */
function script(id: string, name: string, kind: 'script' | 'smart', command: string) {
  return {
    id,
    kind,
    name,
    command,
    params: [],
    smart:
      kind === 'smart'
        ? { model: 'sonnet', maxBudgetUsd: 1, timeout: '15m', tools: [], bashPatterns: [], mcp: [] }
        : null,
    workingDir: '',
    dirMode: 'kira',
    useAdeDir: true,
    color: 'none',
    collectionId: null,
    sortOrder: 0,
    createdAt: '2026-01-01T00:00:00.000Z',
    updatedAt: '2026-01-01T00:00:00.000Z',
  };
}

export const SMART = script('smart-1', 'Review diff', 'smart', 'Review {branch} of {task}.');
export const PLAIN = script('plain-1', 'Lint', 'script', 'bun run lint');

export const SCRIPTS: ControlSnapshot = {
  channel: IPC.customScriptsList,
  response: { collections: [], scripts: [PLAIN, SMART] },
};

/** Selects the card of `id` and waits for its Task tab. */
export async function openTask(page: Page, id: string): Promise<void> {
  await page
    .locator(`[data-testid="ade-card"][data-task-id="${id}"] [data-testid="ade-card-head"]`)
    .click();
  await page.locator('[data-testid="ade-task-tab"]').waitFor();
}

/** Opens the inspector's collapsed Advanced section. */
export const openAdvanced = async (page: Page): Promise<void> => {
  const toggle = page.locator('[data-testid="ade-wf-step-advanced"]');
  if ((await toggle.getAttribute('aria-expanded')) !== 'true') await toggle.click();
};

/** Clicks Save and returns the args of the nth SaveWorkflow call. */
export async function saved<T>(
  page: Page,
  control: Parameters<typeof calls>[0],
  n: number,
): Promise<T> {
  await page.locator('[data-testid="ade-wf-save"]').click();
  await expect.poll(() => calls(control, IPC.adeTaskSaveWorkflow)).toHaveLength(n);
  return calls(control, IPC.adeTaskSaveWorkflow)[n - 1]?.args as T;
}
