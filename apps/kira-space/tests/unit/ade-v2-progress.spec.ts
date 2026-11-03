import { describe, expect, test } from 'bun:test';
import {
  branchProgress,
  buildTaskProgress,
  type ProgressInput,
  walkPath,
} from '../../frontend/src/ade/v2/board/progress';
import { blocked as isBlocked } from '../../frontend/src/ade/v2/board/status';
import type { Branch, Run, Task, Workflow } from '../../frontend/src/ade/v2/wire';
import { mkBranch, mkManual, mkRun, mkStep, mkTask } from './support/adeV2Fixtures';

const workflow: Workflow = {
  id: 'wf',
  name: 'Feature',
  steps: [
    mkManual({ id: 'spec', name: 'Spec', status: 'in-progress' }),
    mkStep({ id: 'plan', runsOn: 'once' }),
    mkStep({ id: 'code' }),
    mkStep({ id: 'pr', before: 'approval' }),
    mkStep({ id: 'release', kind: 'script', command: './r.sh', status: 'in-review' }),
  ],
};

const NICK: Record<string, string> = { 'repo-api': 'api', 'repo-web': 'web-app' };
const branches: Branch[] = [
  mkBranch({ id: 'b_api', taskId: 'T', codeRepoId: 'repo-api' }),
  mkBranch({ id: 'b_web', taskId: 'T', codeRepoId: 'repo-web' }),
];

function input(
  runs: Run[],
  over: Partial<Task> = {},
  wf: Workflow | null = workflow,
): ProgressInput {
  const task = mkTask({
    id: 'T',
    workflowId: 'wf',
    entryStepId: 'plan',
    branchIds: ['b_api', 'b_web'],
    runs,
    ...over,
  });
  return {
    task,
    workflow: wf,
    branch: (id) => branches.find((b) => b.id === id),
    repoNick: (id) => NICK[id] ?? id,
  };
}

const run = (stepId: string, branchId: string, over: Partial<Run> = {}): Run =>
  mkRun({ taskId: 'T', stepId, branchId, ...over });
const doneOn = (stepId: string): Run[] =>
  ['b_api', 'b_web'].map((b) => run(stepId, b, { state: 'done' }));

describe('step targets and percent', () => {
  test('averages done runs as 1 and others as 0, over automatic steps and repos', () => {
    const p = buildTaskProgress(
      input([
        run('plan', 'b_api', { state: 'done' }),
        run('code', 'b_api', { state: 'done' }),
        run('code', 'b_web', { state: 'running' }),
      ]),
    );
    expect(p.steps.map((s) => s.frac)).toEqual([0, 1, 0.5, 0, 0]);
    expect(p.percent).toBe(38);
    expect(p.totalSteps).toBe(4);
    expect(p.label).toBe('code 1/4');
    expect(p.barTip.split('\n')[0]).toBe('2. plan: api ✓');
  });

  test('only <repo> matches the nickname; an unmatched only has no targets', () => {
    const wf: Workflow = {
      ...workflow,
      steps: [
        mkStep({ id: 'a', runsOn: 'only web-app' }),
        mkStep({ id: 'b', runsOn: 'only nope' }),
      ],
    };
    const p = buildTaskProgress(input([], { entryStepId: 'a' }, wf));
    expect(p.steps[0]?.runs.map((r) => r.branchId)).toEqual(['b_web']);
    expect(p.steps[1]?.runs).toEqual([]);
    expect(p.steps[1]?.state).toBe('pending');
  });

  test('a task with no mine branch runs steps on the task folder, except review and only', () => {
    const wf: Workflow = {
      ...workflow,
      steps: [
        mkStep({ id: 'a', runsOn: 'once' }),
        mkStep({ id: 'b', runsOn: 'only web-app' }),
        mkStep({ id: 'c', kind: 'review' }),
      ],
    };
    const p = buildTaskProgress(input([], { entryStepId: 'a', branchIds: [] }, wf));
    expect(p.steps.map((s) => s.runs.map((r) => r.branchId))).toEqual([[''], [], []]);
  });

  test('a step that ran without a branch keeps the task folder once repos exist', () => {
    const p = buildTaskProgress(
      input([run('plan', '', { state: 'done' }), run('code', 'b_api', { state: 'running' })], {
        entryStepId: 'plan',
      }),
    );
    expect(p.steps[1]?.runs.map((r) => r.branchId)).toEqual(['']);
    expect(p.steps[1]?.state).toBe('done');
    expect(p.steps[2]?.runs.map((r) => r.branchId)).toEqual(['b_api', 'b_web']);
  });

  test('review branches are never targets', () => {
    const was = branches[1];
    branches[1] = mkBranch({ id: 'b_web', taskId: 'T', codeRepoId: 'repo-web', kind: 'review' });
    expect(buildTaskProgress(input([])).steps[2]?.runs.map((r) => r.branchId)).toEqual(['b_api']);
    if (was) branches[1] = was;
  });
});

describe('latest attempt, history and worst-of', () => {
  test('the highest attempt wins per (step, branch)', () => {
    const p = buildTaskProgress(
      input([
        run('code', 'b_api', { attempt: 1, state: 'failed' }),
        run('code', 'b_api', { attempt: 2, state: 'running' }),
        run('code', 'b_web', { attempt: 1, state: 'done' }),
      ]),
    );
    expect(p.steps[2]?.runs.map((r) => [r.state, r.runId.endsWith(':2')])).toEqual([
      ['running', true],
      ['done', false],
    ]);
    expect(p.bad).toBe(false);
  });

  test('runs at or below since are history', () => {
    const runs = [run('code', 'b_api', { attempt: 1, state: 'done' })];
    expect(buildTaskProgress(input(runs)).steps[2]?.runs[0]?.state).toBe('done');
    const moved = buildTaskProgress(input(runs, { since: { 'code/b_api': 1 } }));
    expect(moved.steps[2]?.runs[0]?.state).toBe('pending');
  });

  test.each([
    [['done', 'stuck'], 'stuck'],
    [['failed', 'stuck'], 'stuck'],
    [['running', 'failed'], 'failed'],
    [['back', 'pending'], 'running'],
    [['running', 'done'], 'running'],
    [['done', 'pending'], 'pending'],
    [['done', 'done'], 'done'],
  ] as const)('runs %j aggregate to %s', (states, expected) => {
    const p = buildTaskProgress(
      input([
        run('code', 'b_api', { state: states[0] }),
        run('code', 'b_web', { state: states[1] }),
      ]),
    );
    expect(p.steps[2]?.state).toBe(expected);
  });

  test('stuck turns the current segment and the bar red', () => {
    const p = buildTaskProgress(
      input([...doneOn('plan'), run('code', 'b_web', { state: 'stuck' })]),
    );
    expect(p.bad).toBe(true);
    expect(p.tone).toBe('red');
    expect(p.segments.map((s) => s.state)).toEqual(['done', 'done', 'blocked', 'todo', 'todo']);
  });
});

describe('approval gate', () => {
  test('flags a gated pending step only when the previous step is done', () => {
    const ready = buildTaskProgress(input([...doneOn('plan'), ...doneOn('code')]));
    expect(ready.steps[3]?.approval).toBe(true);
    expect(ready.step?.id).toBe('pr');
    const notYet = buildTaskProgress(
      input([...doneOn('plan'), run('code', 'b_api', { state: 'running' })]),
    );
    expect(notYet.steps[3]?.approval).toBe(false);
  });

  test('a gated entry step never needs approval', () => {
    const wf: Workflow = { ...workflow, steps: [mkStep({ id: 'a', before: 'approval' })] };
    expect(buildTaskProgress(input([], { entryStepId: 'a' }, wf)).steps[0]?.approval).toBe(false);
  });
});

describe('current step', () => {
  test('a manual entry step is current and carries no bar', () => {
    const p = buildTaskProgress(input([], { entryStepId: 'spec' }));
    expect([p.label, p.showBar, p.stepIndex, p.step?.id]).toEqual(['Spec', false, 0, 'spec']);
  });

  test('a finished task is Done on a green bar-less line', () => {
    const p = buildTaskProgress(input([], { entryStepId: 'done' }));
    expect([p.label, p.showBar, p.tone, p.finished]).toEqual(['Done', false, 'green', true]);
    expect(p.segments.every((s) => s.state === 'done')).toBe(true);
  });

  test('a task without a workflow has no progress', () => {
    expect(
      buildTaskProgress(input([], { workflowId: '', entryStepId: '' }, null)).hasWorkflow,
    ).toBe(false);
  });

  test('a done manual-free path walks to the first step not done', () => {
    const p = buildTaskProgress(input([...doneOn('plan'), ...doneOn('code')]));
    expect(p.step?.id).toBe('pr');
    expect(p.entryIndex).toBe(1);
  });

  test('workflow tooltip marks done and current steps', () => {
    expect(buildTaskProgress(input([...doneOn('plan')])).workflowTip).toBe(
      'Feature: ✓ Spec (manual) › ✓ plan (agent) › ▸ code (agent) › pr (agent) › release (script)',
    );
  });
});

describe('branch progress line', () => {
  const lineOf = (runs: Run[], branchId = 'b_web') =>
    branchProgress(buildTaskProgress(input(runs)), branchId);

  test('names the first unfinished step with fix round and state suffix', () => {
    expect(
      lineOf([
        run('plan', 'b_api', { state: 'done' }),
        run('code', 'b_web', { state: 'running', loops: 1 }),
      ])?.label,
    ).toBe('code · fix 1');
    expect(lineOf([run('code', 'b_web', { state: 'stuck' })])?.label).toBe('code · stuck');
    expect(lineOf([run('code', 'b_web', { state: 'back' })])?.label).toBe('code ↩ sent back');
    expect(lineOf([])?.label).toBe('code · waiting');
  });

  test('is absent without a step on the branch', () => {
    const wf: Workflow = { ...workflow, steps: [mkStep({ id: 'a', runsOn: 'only api' })] };
    expect(
      branchProgress(buildTaskProgress(input([], { entryStepId: 'a' }, wf)), 'b_web'),
    ).toBeNull();
  });
});

describe('blocked', () => {
  const blocked = (runs: Run[], over: Partial<Task> = {}) => {
    const i = input(runs, over);
    return isBlocked(i.task, buildTaskProgress(i), branches);
  };

  test('a stuck or failed run blocks a task in a workflow', () => {
    expect(blocked([], { entryStepId: 'done' })).toBe(false);
    expect(blocked([...doneOn('plan'), run('code', 'b_api', { state: 'failed' })])).toBe(true);
    expect(blocked([run('code', 'b_api', { state: 'running' })])).toBe(false);
  });

  test('a parked task is never blocked', () => {
    expect(blocked([run('code', 'b_api', { state: 'failed' })], { kind: 'parked' })).toBe(false);
  });
});

describe('walkPath (mirrors Go walkPath)', () => {
  const r = (route: string) => ({ route }) as never;
  const done = (route: string) => ({
    state: 'done' as const,
    kind: 'agent' as const,
    skip: false,
    runs: [r(route)],
  });
  const pending = { state: 'pending' as const, kind: 'agent' as const, skip: false, runs: [] };
  const skipped = { ...pending, skip: true };
  const manual = { ...pending, kind: 'manual' as const };

  test('a forward route skips the steps between', () => {
    expect(walkPath(['a', 'b', 'c'], [done('c'), pending, pending], 0)).toEqual({
      path: [0, 2],
      skipped: [false, true, false],
    });
  });
  test('end stops the path and skips the rest', () => {
    expect(walkPath(['a', 'b'], [done('end'), pending], 0)).toEqual({
      path: [0, 2].slice(0, 1),
      skipped: [false, true],
    });
  });
  test('the earliest forward target across branches wins', () => {
    const two = { ...done('c'), runs: [r('c'), r('next')] };
    expect(walkPath(['a', 'b', 'c'], [two, pending, pending], 0).path).toEqual([0, 1]);
  });
  test('a run without a stored route counts as next', () => {
    expect(walkPath(['a', 'b'], [done(''), pending], 0).path).toEqual([0, 1]);
  });
  test('starts at the entry and passes over skip steps', () => {
    expect(walkPath(['a', 'b', 'c'], [done('next'), skipped, pending], 1).path).toEqual([2]);
  });
  test('a manual step ends the path', () => {
    expect(walkPath(['a', 'b'], [manual, pending], 0).path).toEqual([0]);
  });
});
