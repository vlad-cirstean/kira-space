import { type MessageKey, t } from '@theme/i18n';
import { runStateLabel } from '../runState';
import type {
  Branch,
  RunReview,
  RunSandboxBadge,
  RunState,
  RunsOn,
  StepKind,
  StepResult,
  Task,
  TaskStatus,
  Workflow,
  WorkflowStep,
} from '../wire';
import { defaultResults } from './stepResults';

// Pure port of mockup v2 `renderVals()` step aggregation (1423-1466), `phasePills` (1672-1702) and
// `branchProg` (1768-1787). Runs are the backend's: a stuck run arrives as `state: 'stuck'`.

/** `skipped`: a route went past the step, or the step is marked skip; it never runs. */
export type StepState = 'pending' | 'running' | 'review' | 'stuck' | 'failed' | 'done' | 'skipped';

export interface StepRun {
  branchId: string;
  state: RunState;
  loops: number;
  /** Times the step went back, up to and including this run. */
  returns: number;
  note: string;
  /** The stored outcome's reason, '' while none. */
  reason: string;
  /** The step result the agent reported, '' while none. */
  result: string;
  /** Whether that result was ok; null while none. */
  ok: boolean | null;
  /** Where the result sent the step: next, end, stop, a step id, `back:<id>` or `retry`; '' while none. */
  route: string;
  runId: string;
  sessionId: string;
  finishedAt: number | null;
  resumable: boolean;
  /** Review step run: where it stands; null on every other run. */
  review: RunReview | null;
  /** Sandbox badge; null when the run had none. */
  sandbox: RunSandboxBadge | null;
}

export interface StepProgress {
  id: string;
  /** 1-based position in the workflow. */
  n: number;
  name: string;
  kind: StepKind;
  /** The step is marked skip. */
  skip: boolean;
  /** Task status while the task is at this step. */
  status: TaskStatus;
  /** Manual step: opens a Claude Code session. */
  session: boolean;
  /** Script step: the shell command; '' on other kinds. */
  command: string;
  runsOn: RunsOn;
  before: 'auto' | 'approval';
  /** The step's results with their routes; empty on a manual step. */
  results: StepResult[];
  /** Returns the step may make before the run stops; 0 = no limit. */
  maxReturns: number;
  /** Name of the smart script the step runs, else ''. */
  smartScript: string;
  state: StepState;
  /** One entry per target branch, in task branch order; a branch without a run is `pending`. Empty on a manual step. */
  runs: StepRun[];
  /** Pending, gated on approval, and the previous step is done. */
  approval: boolean;
  /** 0..1: share of target runs done. */
  frac: number;
}

interface StepSegment {
  id: string;
  name: string;
  kind: StepKind;
  state: 'done' | 'current' | 'blocked' | 'todo' | 'skipped';
}

type ProgressTone = 'green' | 'red' | 'amber';

export interface TaskProgress {
  hasWorkflow: boolean;
  finished: boolean;
  /** The step the task is at; null when finished or the workflow file is unavailable. */
  step: StepProgress | null;
  /** Index of the current step in the workflow, -1 when finished or the workflow is unavailable. */
  stepIndex: number;
  /** Index of the step the task last entered; steps before it are history. */
  entryIndex: number;
  steps: StepProgress[];
  /** Automatic steps done or skipped. */
  doneSteps: number;
  /** Automatic steps of the workflow, skipped ones not counted. */
  totalSteps: number;
  /** The current step is stuck or failed. */
  bad: boolean;
  segments: StepSegment[];
  /** `Spec`, `Implement 2/5`, `Done`. */
  label: string;
  /** Precise bar while the current step is automatic. */
  showBar: boolean;
  percent: number;
  tone: ProgressTone;
  barTip: string;
  workflowTip: string;
}

export interface ProgressInput {
  task: Task;
  /** The task's workflow from the Workflows list; `null` when deleted or never valid. */
  workflow: Workflow | null;
  branch: (id: string) => Branch | undefined;
  /** Repo nickname, falling back to its name. */
  repoNick: (codeRepoId: string) => string;
}

/** The target of a step on a task with no branch: a run with no branch, in the task folder. */
export const TASK_TARGET = '';

/** Branches a step runs on: `once` is the first mine branch (D12), `each repo` every mine branch,
 *  `only <nick>` the mine branches in that repo. An unmatched `only` has no targets. A task with no
 *  mine branch runs on `TASK_TARGET`, except a review or `only` step, which has none; `pinned` (the
 *  step already ran without a branch in this pass) keeps `TASK_TARGET`. Mirrors Go `stepTargets`. */
function stepTargets(
  i: ProgressInput,
  runsOn: RunsOn | '',
  kind: StepKind,
  pinned: boolean,
): string[] {
  if (pinned) return [TASK_TARGET];
  const mine = i.task.branchIds.filter((id) => i.branch(id)?.kind === 'mine');
  if (!mine.length) return kind === 'review' || runsOn.startsWith('only ') ? [] : [TASK_TARGET];
  if (runsOn === 'once') return mine.slice(0, 1);
  if (runsOn.startsWith('only ')) {
    const nick = runsOn.slice(5);
    return mine.filter((id) => i.repoNick(i.branch(id)?.codeRepoId ?? '') === nick);
  }
  return mine;
}

function aggregate(states: readonly RunState[]): StepState {
  if (!states.length) return 'pending';
  if (states.includes('stuck')) return 'stuck';
  if (states.includes('failed')) return 'failed';
  if (states.includes('review')) return 'review';
  if (states.includes('running') || states.includes('back')) return 'running';
  return states.every((s) => s === 'done') ? 'done' : 'pending';
}

function runFraction(r: StepRun): number {
  return r.state === 'done' ? 1 : 0;
}

/** The step defaults for display: kind-fixed fields filled in. */
function stepDef(
  s: WorkflowStep,
): Omit<StepProgress, 'n' | 'state' | 'runs' | 'approval' | 'frac'> {
  const manual = s.kind === 'manual';
  return {
    id: s.id,
    name: s.name,
    kind: s.kind,
    skip: s.skip,
    status: s.status,
    session: s.session,
    command: s.command,
    runsOn: s.runsOn || 'once',
    before: s.before === 'approval' ? 'approval' : 'auto',
    results: manual ? [] : s.results?.length ? s.results : defaultResults(),
    maxReturns: s.maxReturns,
    smartScript: s.smartScript,
  };
}

const emptyRun = (branchId: string): StepRun => ({
  branchId,
  state: 'pending',
  loops: 0,
  returns: 0,
  note: '',
  reason: '',
  result: '',
  ok: null,
  route: '',
  runId: '',
  sessionId: '',
  finishedAt: null,
  resumable: false,
  review: null,
  sandbox: null,
});

/** Latest-attempt run of `(step, branch)` in the task's current pass (W12): runs at or below the
 *  watermark are history. */
function latestRun(task: Task, stepId: string, branchId: string): StepRun | null {
  const since = task.since[`${stepId}/${branchId}`] ?? 0;
  let best: Task['runs'][number] | null = null;
  for (const r of task.runs) {
    if (r.stepId !== stepId || r.branchId !== branchId || r.attempt <= since) continue;
    if (!best || r.attempt > best.attempt) best = r;
  }
  if (!best) return null;
  return {
    branchId,
    state: best.state,
    loops: best.loops,
    returns: best.returns,
    note: best.note,
    reason: best.outcome?.reason ?? '',
    result: best.outcome?.result ?? '',
    ok: best.outcome?.result ? best.outcome.status !== 'failed' : null,
    route: best.outcome?.route ?? '',
    runId: best.id,
    sessionId: best.sessionId,
    finishedAt: best.finishedAt,
    resumable: best.resumable,
    review: best.review,
    sandbox: best.sandbox,
  };
}

/** Index of the step after `i` for a done step: the earliest forward route among its runs. Mirrors Go
 *  `forwardTarget`; a run without a stored route counts as `next`. */
function forwardTarget(ids: readonly string[], i: number, runs: readonly StepRun[]): number {
  let next = ids.length;
  for (const r of runs) {
    let to = i + 1;
    if (r.route === 'end') to = ids.length;
    else if (r.route && r.route !== 'next') {
      const j = ids.indexOf(r.route);
      if (j > i) to = j;
    }
    next = Math.min(next, to);
  }
  return Math.max(next, i + 1);
}

/** The route path of a task: from the entry step, a skipped step is passed over, a done step moves
 *  to its forward target and the first step not done (a manual step never is) ends the path.
 *  `skipped` marks the steps the route went past. Mirrors Go `walkPath`. */
export function walkPath(
  ids: readonly string[],
  steps: readonly { state: StepState; kind: StepKind; skip: boolean; runs: readonly StepRun[] }[],
  entry: number,
): { path: number[]; skipped: boolean[] } {
  const path: number[] = [];
  const skipped = ids.map(() => false);
  let i = Math.max(entry, 0);
  while (i < ids.length) {
    const step = steps[i];
    if (step?.skip) {
      skipped[i] = true;
      i++;
      continue;
    }
    path.push(i);
    if (!step || step.kind === 'manual' || step.state !== 'done') break;
    const next = forwardTarget(ids, i, step.runs);
    for (let j = i + 1; j < next && j < ids.length; j++) skipped[j] = true;
    i = next;
  }
  return { path, skipped };
}

/** Every step of the workflow with per-branch runs, step state and approval flag. */
function buildSteps(i: ProgressInput, workflow: Workflow): StepProgress[] {
  const built = workflow.steps.map((s, idx): StepProgress => {
    const def = stepDef(s);
    const runs =
      s.kind === 'manual'
        ? []
        : stepTargets(i, s.runsOn, s.kind, latestRun(i.task, s.id, TASK_TARGET) !== null).map(
            (branchId): StepRun => latestRun(i.task, s.id, branchId) ?? emptyRun(branchId),
          );
    const state = aggregate(runs.map((r) => r.state));
    const frac = runs.length ? runs.reduce((acc, r) => acc + runFraction(r), 0) / runs.length : 0;
    return { ...def, n: idx + 1, state, runs, approval: false, frac };
  });
  const entry = entryIndex(i.task, built);
  const { path, skipped } = walkPath(
    built.map((s) => s.id),
    built,
    entry,
  );
  const end = path[path.length - 1] ?? -1;
  return built.map((s, idx) => {
    if (s.skip || skipped[idx]) return { ...s, state: 'skipped', frac: 1 };
    const started = s.runs.some((r) => r.runId !== '');
    const approval =
      s.kind !== 'manual' &&
      s.state === 'pending' &&
      s.before === 'approval' &&
      !started &&
      idx === end &&
      idx !== path[0];
    return { ...s, approval };
  });
}

function entryIndex(task: Task, steps: readonly { id: string }[]): number {
  return Math.max(
    steps.findIndex((s) => s.id === task.entryStepId),
    0,
  );
}

export function buildTaskProgress(i: ProgressInput): TaskProgress {
  const { task, workflow } = i;
  const finished = task.workflowId !== '' && task.entryStepId === 'done';
  const live = !finished && workflow !== null && task.entryStepId !== '';
  const hasWorkflow = finished || live;
  if (!hasWorkflow) {
    return {
      hasWorkflow: false,
      finished: false,
      step: null,
      stepIndex: -1,
      entryIndex: -1,
      steps: [],
      doneSteps: 0,
      totalSteps: 0,
      bad: false,
      segments: [],
      label: '',
      showBar: false,
      percent: 0,
      tone: 'amber',
      barTip: '',
      workflowTip: '',
    };
  }
  const steps = workflow ? buildSteps(i, workflow) : [];
  const { path } = walkPath(
    steps.map((s) => s.id),
    steps,
    entryIndex(task, steps),
  );
  const idx = finished ? steps.length : (path[path.length - 1] ?? -1);
  const step = !finished && idx >= 0 ? (steps[idx] ?? null) : null;
  const auto = steps.filter((s) => s.kind !== 'manual' && !s.skip);
  const bad = step !== null && (step.state === 'stuck' || step.state === 'failed');
  const doneSteps = auto.filter((s) => s.state === 'done' || s.state === 'skipped').length;
  const frac = auto.length ? auto.reduce((acc, s) => acc + s.frac, 0) / auto.length : 0;
  const segments = steps.map((s, k): StepSegment => {
    let state: StepSegment['state'] = 'todo';
    if (s.state === 'skipped') state = 'skipped';
    else if (k < idx) state = 'done';
    else if (k === idx) state = s.state === 'done' ? 'done' : bad ? 'blocked' : 'current';
    return { id: s.id, name: s.name, kind: s.kind, state };
  });
  const label =
    finished || !step
      ? t('common.done.status')
      : step.name + (step.kind === 'manual' ? '' : ` ${doneSteps}/${auto.length}`);
  const nick = (id: string): string =>
    id === TASK_TARGET
      ? t('space.ade.stepBlock.taskFolder.label')
      : i.repoNick(i.branch(id)?.codeRepoId ?? '');
  return {
    hasWorkflow: true,
    finished,
    step,
    stepIndex: workflow && !finished ? idx : -1,
    entryIndex: workflow && !finished ? entryIndex(task, steps) : -1,
    steps,
    doneSteps,
    totalSteps: auto.length,
    bad,
    segments,
    label,
    showBar: !finished && step !== null && step.kind !== 'manual',
    percent: Math.round(frac * 100),
    tone: finished ? 'green' : bad ? 'red' : 'amber',
    barTip: auto
      .map((s) =>
        t('space.ade.progress.tipLine.tooltip', {
          n: s.n,
          name: s.name,
          detail: s.runs
            .map((r) =>
              t('space.ade.progress.barTipRun.label', {
                nick: nick(r.branchId),
                state: r.state === 'done' ? '✓' : runStateLabel(r.state),
              }),
            )
            .join(', '),
        }),
      )
      .join('\n'),
    workflowTip: t('theme.varText.aria', {
      name: workflow?.name ?? task.workflowId,
      value: steps
        .map((s, k) => `${k < idx ? '✓ ' : k === idx ? '▸ ' : ''}${stepKindLabel(s)}`)
        .join(' › '),
    }),
  };
}

const STEP_KIND = {
  agent: 'space.ade.progress.stepAgent.label',
  script: 'space.ade.progress.stepScript.label',
  review: 'space.ade.progress.stepReview.label',
  manual: 'space.ade.progress.stepManual.label',
} as const satisfies Record<StepKind, MessageKey>;
const stepKindLabel = (s: { kind: StepKind; name: string }): string =>
  t(STEP_KIND[s.kind], { name: s.name });

export type BranchSegState = 'done' | 'running' | 'bad' | 'pending';

export interface BranchProgress {
  segments: BranchSegState[];
  /** `Implement`, `Implement · stuck`, `Tests ↩ sent back`, `Implement ✓`. */
  label: string;
  tone: 'amber' | 'red' | 'green' | 'grey';
  tip: string;
}

/** One branch's own line while the task is at an automatic step; `null` otherwise or without a step on it. */
export function branchProgress(p: TaskProgress, branchId: string): BranchProgress | null {
  const current = p.step;
  if (!current || current.kind === 'manual') return null;
  const own: { step: StepProgress; run: StepRun }[] = [];
  for (const step of p.steps.slice(Math.max(p.entryIndex, 0))) {
    if (step.state === 'skipped') continue;
    const run = step.runs.find((r) => r.branchId === branchId);
    if (run) own.push({ step, run });
  }
  if (!own.length) return null;
  const segments = own.map(({ run }): BranchSegState => {
    if (run.state === 'done') return 'done';
    if (run.state === 'running' || run.state === 'back' || run.state === 'review') return 'running';
    return run.state === 'stuck' || run.state === 'failed' ? 'bad' : 'pending';
  });
  const pos = own.find(({ run }) => run.state !== 'done');
  let label: string;
  let tone: BranchProgress['tone'] = 'amber';
  if (!pos) {
    label = t('space.ade.progress.stepDone.label', { name: current.name });
    tone = 'green';
  } else if (pos.run.state === 'back') {
    label =
      pos.run.route === 'retry'
        ? t('common.dotPair.label', { first: pos.step.name, second: pos.run.note })
        : t('space.ade.progress.sentBack.label', { step: pos.step.name });
  } else {
    const base = pos.run.loops
      ? t('space.ade.progress.fix.label', { step: pos.step.name, count: pos.run.loops })
      : pos.step.name;
    if (pos.run.state === 'stuck') label = t('space.ade.progress.stuck.label', { step: base });
    else if (pos.run.state === 'failed') label = t('space.common.nameFailed.label', { name: base });
    else if (pos.run.state === 'pending')
      label = t('space.ade.progress.waiting.label', { step: base });
    else label = base;
    if (pos.run.state === 'stuck' || pos.run.state === 'failed') tone = 'red';
    else if (pos.run.state === 'pending') tone = 'grey';
  }
  const tip = own
    .map(({ step, run }) =>
      run.note
        ? t('space.ade.progress.tipLineNote.tooltip', {
            n: step.n,
            name: step.name,
            state: runStateLabel(run.state),
            note: run.note,
          })
        : t('space.ade.progress.tipLine.tooltip', {
            n: step.n,
            name: step.name,
            detail: runStateLabel(run.state),
          }),
    )
    .join('\n');
  return { segments, label, tone, tip };
}
