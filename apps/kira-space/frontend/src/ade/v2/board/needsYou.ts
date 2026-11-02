import { type MessageKey, t } from '@theme/i18n';
import { formatDuration } from '@workbench/behaviour/format';
import { reviewPhaseLabel } from '../runState';
import type { Board, Branch, Run, Session } from '../wire';
import type { StepProgress, TaskProgress } from './progress';
import { runsOnLabel } from './workflowForm';

// Needs-you list (SPEC2 §11), mockup v2 lines 2490-2520. No CI item: D9 has no CI badge.

export type NeedsKind =
  | 'stuck run'
  | 'failed'
  | 'rebase'
  | 'question'
  | 'approval'
  | 'review'
  | 'setup failed'
  | 'stale merge';

/** Most urgent first (SPEC2 §11); ties break oldest first. */
export const NEEDS_RANK: readonly NeedsKind[] = [
  'stuck run',
  'failed',
  'rebase',
  'question',
  'approval',
  'review',
  'setup failed',
  'stale merge',
];

type NeedsAction =
  | 'Take over'
  | 'Retry'
  | 'Resume'
  | 'Open'
  | 'Approve'
  | 'Review'
  | 'See error'
  | 'Re-merge';

const ACTION: Record<NeedsKind, NeedsAction> = {
  'stuck run': 'Take over',
  failed: 'Retry',
  rebase: 'Open',
  question: 'Open',
  approval: 'Approve',
  review: 'Review',
  'setup failed': 'See error',
  'stale merge': 'Re-merge',
};

const KIND_LABEL = {
  'stuck run': 'space.ade.needs.kind.stuckRun.label',
  failed: 'common.state.failed.status',
  rebase: 'common.rebase.label',
  question: 'space.ade.needs.kind.question.label',
  approval: 'space.ade.needs.kind.approval.label',
  review: 'space.ade.board.branchTag.review.label',
  'setup failed': 'space.ade.needs.kind.setupFailed.label',
  'stale merge': 'space.ade.needs.kind.staleMerge.label',
} as const satisfies Record<NeedsKind, MessageKey>;

const ACTION_LABEL = {
  'Take over': 'common.action.takeOver.label',
  Retry: 'common.action.retry.label',
  Resume: 'common.action.resume.label',
  Open: 'common.action.open.label',
  Approve: 'common.action.approve.label',
  Review: 'space.ade.board.branchTag.review.label',
  'See error': 'space.ade.needs.action.seeError.label',
  'Re-merge': 'space.ade.needs.action.reMerge.label',
} as const satisfies Record<NeedsAction, MessageKey>;

/** Shown name of a kind; the kind itself stays the id. */
export function needsKindLabel(kind: NeedsKind): string {
  return t(KIND_LABEL[kind]);
}

/** Shown name of an item's action; the action itself stays the id. */
export function needsActionLabel(action: NeedsItem['action']): string {
  return t(ACTION_LABEL[action]);
}

const TONE: Record<NeedsKind, 'red' | 'amber' | 'grey'> = {
  'stuck run': 'red',
  failed: 'red',
  rebase: 'red',
  question: 'amber',
  approval: 'amber',
  review: 'amber',
  'setup failed': 'red',
  'stale merge': 'grey',
};

export interface NeedsItem {
  /** Stable key: kind plus the ids it points at. */
  id: string;
  kind: NeedsKind;
  tone: 'red' | 'amber' | 'grey';
  action: NeedsAction;
  taskId: string;
  /** `''` for a task-level item (spec session, failed step). */
  branchId: string;
  stepId: string;
  sessionId: string;
  /** `stale merge`: the integration branch it is stale in. */
  target: string;
  /** `Retry`: the latest failed or stuck runs to re-run. */
  runIds: string[];
  /** Repo nickname, `spec`, or the step's `runs_on`. */
  scope: string;
  what: string;
  /** Extra fact, e.g. `took 1m 12s`; `''` when none. */
  detail: string;
  /** Milliseconds since it started needing you; `null` when unknown. */
  ageMs: number | null;
}

export interface NeedsYou {
  items: NeedsItem[];
  empty: boolean;
  /** Tab badge: the item count. */
  badge: number;
  backgroundRuns: number;
  interactiveSessions: number;
  footer: string;
}

export interface NeedsYouInput {
  board: Pick<Board, 'tasks' | 'branches'>;
  sessions: readonly Session[];
  progress: ReadonlyMap<string, TaskProgress>;
  repoNick: (codeRepoId: string) => string;
  nowMs: number;
}

type Extra = Partial<NeedsItem> & Pick<NeedsItem, 'id' | 'taskId' | 'what'>;

function item(kind: NeedsKind, o: Extra): NeedsItem {
  return {
    kind,
    tone: TONE[kind],
    action: ACTION[kind],
    branchId: '',
    stepId: '',
    sessionId: '',
    target: '',
    runIds: [],
    scope: '',
    detail: '',
    ageMs: null,
    ...o,
  };
}

interface Ctx {
  i: NeedsYouInput;
  nick(branchId: string): string;
  nameOf(branchId: string): string;
  age(at: number | null | undefined): number | null;
  sessionsById: ReadonlyMap<string, Session>;
}

/** `Step "x" failed`, or `ended <result>` when the step declared its own result. */
function failedWhat(name: string, result: string): string {
  return result && result !== 'failed' && result !== 'done'
    ? t('space.ade.needs.what.stepEnded.label', { name, result })
    : t('space.ade.needs.what.stepFailed.label', { name });
}

/** A review step run waiting for the user, or whose agent stopped. */
function reviewItems(c: Ctx, taskId: string, step: StepProgress): NeedsItem[] {
  return step.runs
    .filter(
      (x) => x.state === 'review' && (x.review?.phase === 'waiting' || x.review?.phase === 'stuck'),
    )
    .map((r) =>
      item('review', {
        id: `review:${taskId}:${step.id}:${r.branchId}`,
        taskId,
        branchId: r.branchId,
        stepId: step.id,
        scope: c.nick(r.branchId),
        what: t('space.ade.needs.what.review.label', {
          name: step.name,
          branch: c.nameOf(r.branchId),
        }),
        detail: r.review?.phase === 'stuck' ? reviewPhaseLabel('stuck') : '',
        ageMs: c.age(r.finishedAt),
      }),
    );
}

function stepItems(c: Ctx): NeedsItem[] {
  return c.i.board.tasks.flatMap((task) => {
    const current = c.i.progress.get(task.id)?.step;
    return current ? currentStepItems(c, task, current) : [];
  });
}

function currentStepItems(c: Ctx, task: Board['tasks'][number], step: StepProgress): NeedsItem[] {
  const out: NeedsItem[] = [];
  for (const r of step.runs.filter((x) => x.state === 'stuck')) {
    out.push(
      item('stuck run', {
        id: `stuck:${task.id}:${step.id}:${r.branchId}`,
        taskId: task.id,
        branchId: r.branchId,
        stepId: step.id,
        sessionId: r.sessionId,
        // A script run has no session to take over: it is retried. A restart-stopped one resumes.
        action: r.resumable ? 'Resume' : r.sessionId ? 'Take over' : 'Retry',
        runIds: r.runId ? [r.runId] : [],
        scope: c.nick(r.branchId),
        what: t('space.ade.needs.what.stuck.label', {
          name: step.name,
          branch: c.nameOf(r.branchId),
        }),
        detail: r.note,
        ageMs: c.age(c.sessionsById.get(r.sessionId)?.lastActiveAt ?? r.finishedAt),
      }),
    );
  }
  const base = { taskId: task.id, stepId: step.id, scope: runsOnLabel(step.runsOn) };
  if (step.state === 'failed') {
    const failed = step.runs.filter((x) => x.state === 'failed' || x.state === 'stuck');
    out.push(
      item('failed', {
        ...base,
        id: `failed:${task.id}:${step.id}`,
        what: failedWhat(step.name, failed.find((x) => x.result)?.result ?? ''),
        runIds: failed.flatMap((x) => (x.runId ? [x.runId] : [])),
        detail: failed.find((x) => x.note)?.note ?? '',
        ageMs: c.age(Math.max(0, ...failed.map((x) => x.finishedAt ?? 0))),
      }),
    );
  }
  out.push(...reviewItems(c, task.id, step));
  if (step.approval) {
    out.push(
      item('approval', {
        ...base,
        id: `approval:${task.id}:${step.id}`,
        what: t('space.ade.needs.what.approval.label', { name: step.name }),
      }),
    );
  }
  return out;
}

function questionItems(c: Ctx): NeedsItem[] {
  const taskIds = new Set(c.i.board.tasks.map((t) => t.id));
  return c.i.sessions
    .filter(
      (s) =>
        s.mode === 'tui' &&
        s.state === 'running' &&
        s.activity === 'input' &&
        taskIds.has(s.taskId),
    )
    .map((s) =>
      item('question', {
        id: `question:${s.id}`,
        taskId: s.taskId,
        branchId: s.branchId,
        sessionId: s.id,
        scope: s.branchId ? c.nick(s.branchId) : t('space.ade.needs.scope.spec.label'),
        what: s.branchId
          ? t('space.ade.needs.what.question.label', { branch: c.nameOf(s.branchId) })
          : t('space.ade.needs.what.questionSpec.label'),
        ageMs: c.age(s.lastActiveAt),
      }),
    );
}

/** The latest rebase run of each branch, by branch id. */
function latestRebases(board: Pick<Board, 'tasks'>): Map<string, Run> {
  const out = new Map<string, Run>();
  for (const t of board.tasks) {
    for (const r of t.runs) {
      const cur = out.get(r.branchId);
      if (r.purpose === 'rebase' && (!cur || r.attempt >= cur.attempt)) out.set(r.branchId, r);
    }
  }
  return out;
}

function rebaseItem(c: Ctx, b: Branch, run: Run | undefined): NeedsItem | null {
  const running = run?.state === 'running' || run?.state === 'pending';
  if (running) return null;
  const failed = run?.state === 'failed' || run?.state === 'stuck';
  if (!failed && b.basePendingFrom === '') return null;
  const what =
    run?.state === 'stuck'
      ? 'space.ade.needs.what.rebaseNeedsYou.label'
      : failed
        ? 'space.ade.needs.what.rebaseFailed.label'
        : 'space.ade.needs.what.rebasePending.label';
  return item('rebase', {
    id: `rebase:${b.id}`,
    taskId: b.taskId,
    branchId: b.id,
    scope: c.nick(b.id),
    what: t(what, { branch: c.nameOf(b.id) }),
    detail: run?.outcome?.reason ?? '',
    ageMs: c.age(run?.finishedAt),
  });
}

function branchItems(c: Ctx): NeedsItem[] {
  const out: NeedsItem[] = [];
  const rebases = latestRebases(c.i.board);
  for (const b of c.i.board.branches) {
    const r = b.kind === 'mine' ? rebaseItem(c, b, rebases.get(b.id)) : null;
    if (r) out.push(r);
    if (b.setup?.state === 'failed') {
      out.push(
        item('setup failed', {
          id: `setup:${b.id}`,
          taskId: b.taskId,
          branchId: b.id,
          scope: c.nick(b.id),
          what: t('space.ade.needs.what.setupFailed.label', { branch: c.nameOf(b.id) }),
          detail:
            b.setup.note ||
            (b.setup.finishedAt
              ? t('space.ade.needs.detail.setupTook.label', {
                  duration: formatDuration(b.setup.finishedAt - b.setup.startedAt),
                })
              : ''),
          ageMs: c.age(b.setup.finishedAt),
        }),
      );
    }
    for (const g of b.integration.filter((x) => x.status === 'stale')) {
      out.push(
        item('stale merge', {
          id: `stale:${b.id}:${g.target}`,
          target: g.target,
          taskId: b.taskId,
          branchId: b.id,
          scope: c.nick(b.id),
          what: t('space.ade.needs.what.staleMerge.label', {
            branch: c.nameOf(b.id),
            target: g.target,
          }),
        }),
      );
    }
  }
  return out;
}

export function buildNeedsYou(i: NeedsYouInput): NeedsYou {
  const branches = new Map(i.board.branches.map((b) => [b.id, b]));
  const c: Ctx = {
    i,
    nick: (id) =>
      id
        ? i.repoNick(branches.get(id)?.codeRepoId ?? '')
        : t('space.ade.stepBlock.taskFolder.label'),
    nameOf: (id) =>
      id
        ? branches.get(id)?.name || t('common.newBranch.label')
        : t('space.ade.stepBlock.taskFolder.label'),
    age: (at) => (at ? Math.max(0, i.nowMs - at) : null),
    sessionsById: new Map(i.sessions.map((s) => [s.id, s])),
  };
  const rank = (k: NeedsKind): number => NEEDS_RANK.indexOf(k);
  const items = [...stepItems(c), ...questionItems(c), ...branchItems(c)].sort(
    (a, b) => rank(a.kind) - rank(b.kind) || (b.ageMs ?? -1) - (a.ageMs ?? -1),
  );
  const running = i.sessions.filter((s) => s.state === 'running');
  const backgroundRuns = running.filter((s) => s.mode === 'headless').length;
  const interactiveSessions = running.filter((s) => s.mode === 'tui').length;
  return {
    items,
    empty: items.length === 0,
    badge: items.length,
    backgroundRuns,
    interactiveSessions,
    footer: t('common.dotPair.label', {
      first: t('space.ade.needs.backgroundRuns.label', { count: backgroundRuns }),
      second: t('space.ade.needs.interactiveSessions.label', { count: interactiveSessions }),
    }),
  };
}
