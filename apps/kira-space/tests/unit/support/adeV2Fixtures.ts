import fs from 'node:fs';
import path from 'node:path';
import { defaultResults } from '../../../frontend/src/ade/v2/board/stepResults';
import { defaultSandbox } from '../../../frontend/src/ade/v2/board/stepSandbox';
import type {
  Branch,
  Plan,
  Run,
  StatusDefaults,
  Task,
  TaskStatusDef,
  WorkflowStep,
} from '../../../frontend/src/ade/v2/wire';

// Builders for the pure-logic specs: every field defaulted, a spec overrides only what it asserts on.

export const DEFAULT_STATUSES: TaskStatusDef[] = [
  { id: 'todo', name: 'To do', color: 'grey' },
  { id: 'in-progress', name: 'In progress', color: 'amber' },
  { id: 'in-review', name: 'In review', color: 'blue' },
  { id: 'done', name: 'Done', color: 'green' },
];

export const DEFAULT_STATUS_DEFAULTS: StatusDefaults = {
  new: 'todo',
  step: 'in-progress',
  review: 'in-review',
  finished: 'done',
};

export function mkBranch(over: Partial<Branch> & Pick<Branch, 'id' | 'taskId'>): Branch {
  return {
    codeRepoId: 'repo-web',
    name: `feat/${over.id}`,
    kind: 'mine',
    owner: '',
    base: 'main',
    baseBranchId: '',
    baseOwner: '',
    tip: 'aaaaaaa',
    ahead: 1,
    behind: 0,
    upstream: '',
    upstreamAhead: 0,
    upstreamBehind: 0,
    mergedIntoMain: false,
    mergedAt: null,
    worktree: '',
    setup: null,
    integration: [],
    deployments: [],
    conflictsIfRebased: [],
    conflictCheck: 'done',
    conflictCheckReason: '',
    dirty: [],
    lastCommitAt: null,
    addedAt: 0,
    origin: '',
    baseMissing: false,
    basePendingFrom: '',
    rebaseInProgress: false,
    ...over,
  };
}

export function mkTask(over: Partial<Task> & Pick<Task, 'id'>): Task {
  return {
    kind: 'task',
    title: '',
    owner: '',
    jira: null,
    hostUrl: '',
    hostLink: null,
    workflowId: '',
    entryStepId: '',
    since: {},
    workflow: null,
    workflowOutdated: false,
    est: '',
    notes: '',
    color: 0,
    branchIds: [],
    runs: [],
    createdAt: 0,
    statusId: 'todo',
    statusBy: 'auto',
    statusStep: '',
    handoverIn: null,
    handoverDrafts: [],
    branchRequestId: '',
    ...over,
  };
}

export function mkRun(over: Partial<Run> & Pick<Run, 'taskId' | 'stepId' | 'branchId'>): Run {
  return {
    id: `${over.taskId}:${over.stepId}:${over.branchId}:${over.attempt ?? 1}`,
    attempt: 1,
    state: 'pending',
    loops: 0,
    returns: 0,
    note: '',
    summary: '',
    sessionId: '',
    exitCode: null,
    startedAt: null,
    finishedAt: null,
    outcome: null,
    purpose: '',
    resumable: false,
    review: null,
    sandbox: null,
    ...over,
  };
}

export function mkPlan(over: Partial<Plan> = {}): Plan {
  return { day: {}, order: [], queuedAfter: {}, unpushed: {}, backlog: [], ...over };
}

export function mkStep(over: Partial<WorkflowStep> & Pick<WorkflowStep, 'id'>): WorkflowStep {
  return {
    name: over.id,
    kind: 'agent',
    status: 'in-progress',
    skip: false,
    session: false,
    command: '',
    runsOn: 'each repo',
    outsideRepo: false,
    before: 'auto',
    results: defaultResults(),
    maxReturns: 3,
    timeout: '1h',
    prompt: 'p',
    allowedTools: [],
    dbMcp: false,
    smartScript: '',
    params: {},
    review: null,
    sandbox: defaultSandbox('agent'),
    agent: '',
    settings: {},
    ...over,
  };
}

/** A manual step: no runs, no results. */
export function mkManual(over: Partial<WorkflowStep> & Pick<WorkflowStep, 'id'>): WorkflowStep {
  return mkStep({
    kind: 'manual',
    runsOn: '',
    before: '',
    results: [],
    maxReturns: 0,
    timeout: '',
    prompt: '',
    ...over,
  });
}

/** Loads `tests/fixtures/ade-v2/<name>.json` typed as its wire shape. JSON imports widen string
 *  unions, so the cast lives here; the Go decode test already guards the shape. */
export function loadFixture<T>(name: string): T {
  const file = path.join(import.meta.dir, '../../fixtures/ade-v2', `${name}.json`);
  return JSON.parse(fs.readFileSync(file, 'utf8')) as T;
}
