import { type MessageKey, t } from '@theme/i18n';
import type { TextPart } from '@theme/varText';
import { formatCompactDuration } from '@workbench/behaviour/format';
import type { Branch, Plan, Run, Session, StatusColor, Task, TaskStatusDef } from '../wire';
import { type BranchGraph, shortBranchName } from './branchGraph';
import { branchParts } from './catalogParts';
import type { TaskProgress } from './progress';
import { type RebaseAct, rebaseActs } from './rebaseActions';

// Left action column: task step action and branch tag + action, both first-match tables
// (SPEC2 §4.2, mockup v2 lines 1855-1856 and 1934-1957).

export type Tone = 'amber' | 'red' | 'green' | 'blue' | 'purple' | 'grey';

// ---- task cell

type TaskActionKind =
  | 'archive'
  | 'session'
  | 'done'
  | 'finish'
  | 'takeOver'
  | 'approve'
  | 'run'
  | 'retry';

export interface TaskAction {
  kind: TaskActionKind;
  label: string;
  tip: string;
  tone: 'agent' | 'red' | 'amber' | 'green' | 'purple';
  /** Step the action applies to (`approve`, `retry`, `takeOver`). */
  stepId?: string;
  /** Session to open for `takeOver`. */
  sessionId?: string;
}

export interface TaskCell {
  tag: { label: string; tone?: Tone; color?: StatusColor; tip: string };
  action: TaskAction | null;
}

export interface TaskCellInput {
  task: Task;
  progress: TaskProgress;
  status: TaskStatusDef;
  /** A stuck or failed run or setup: its tag wins over the status. */
  blocked: boolean;
  /** The task's branches. */
  branches: readonly Branch[];
  /** The task's sessions. */
  sessions: readonly Session[];
}

export const archiveTip = (): string => t('space.ade.board.taskAction.archive.hint');

function doneAction(p: TaskProgress): TaskAction {
  const rest = p.segments.slice(p.stepIndex + 1).filter((s) => s.state !== 'skipped');
  const last = p.stepIndex >= 0 && rest.length === 0;
  const step = p.step?.name ?? '';
  if (last) {
    return {
      kind: 'finish',
      label: t('space.ade.board.taskAction.finish.label'),
      tip: t('space.ade.board.taskAction.finish.tooltip'),
      tone: 'green',
    };
  }
  const next = rest[0]?.name;
  return {
    kind: 'done',
    label: t('space.ade.board.taskAction.done.label'),
    tip: next
      ? t('space.ade.board.taskAction.done.tooltip', { step, next })
      : t('space.ade.board.taskAction.doneLast.tooltip', { step }),
    tone: 'green',
  };
}

function manualAction(i: TaskCellInput): TaskAction {
  const { progress: p, sessions } = i;
  const step = p.step;
  // The review agent is the step's session only on the review step.
  const talking = sessions.some(
    (s) =>
      s.mode === 'tui' &&
      s.state === 'running' &&
      (s.purpose !== 'review' || step?.id === 'review'),
  );
  if (step?.session && !talking) {
    return {
      kind: 'session',
      label: t('space.ade.board.taskAction.session.label', { step: step.name }),
      tip: t('space.ade.board.taskAction.session.tooltip', { step: step.name }),
      tone: 'agent',
    };
  }
  return doneAction(p);
}

function automaticAction(p: TaskProgress): TaskAction | null {
  const s = p.step;
  if (!s?.runs.length) return null; // no target: the step needs a repository
  const session = s.runs.find((r) => r.sessionId !== '')?.sessionId;
  if ((s.state === 'stuck' || s.state === 'failed') && s.kind === 'agent' && session) {
    return {
      kind: 'takeOver',
      label: t('common.action.takeOver.label'),
      tip: t('space.ade.board.taskAction.takeOver.tooltip', { step: s.name }),
      tone: 'red',
      stepId: s.id,
      sessionId: session,
    };
  }
  if (s.state === 'failed' || s.state === 'stuck') {
    if (s.kind === 'agent') return null;
    return {
      kind: 'retry',
      label: t('common.action.retry.label'),
      tip: t(
        s.state === 'failed'
          ? 'space.ade.board.taskAction.retryFailed.tooltip'
          : 'space.ade.board.taskAction.retryStuck.tooltip',
        { step: s.name },
      ),
      tone: 'red',
      stepId: s.id,
    };
  }
  if (s.approval) {
    return {
      kind: 'approve',
      label: t('common.action.approve.label'),
      tip: t('space.ade.board.taskAction.approve.tooltip', { step: s.name }),
      tone: 'amber',
      stepId: s.id,
    };
  }
  const untouched = !s.runs.some((r) => r.runId !== '');
  if (s.state === 'pending' && untouched && p.stepIndex === p.entryIndex) {
    return {
      kind: 'run',
      label: t('common.action.run.label'),
      tip:
        s.kind === 'script'
          ? t('space.ade.board.taskAction.runScript.tooltip', { command: s.command })
          : s.kind === 'review'
            ? t('space.ade.board.taskAction.runReview.tooltip', { step: s.name })
            : t('space.ade.board.taskAction.runAgents.tooltip', { step: s.name }),
      tone: 'agent',
      stepId: s.id,
    };
  }
  return null;
}

function stepAction(i: TaskCellInput): TaskAction | null {
  const step = i.progress.step;
  if (!step) return null;
  return step.kind === 'manual' ? manualAction(i) : automaticAction(i.progress);
}

/** Header cell of a task row, `null` for a review item (it has no task row). */
export function taskCell(i: TaskCellInput): TaskCell | null {
  const { task, progress } = i;
  if (task.kind === 'review') return null;
  if (task.kind === 'parked') {
    return {
      tag: {
        label: t('space.ade.board.notMerging.label'),
        tone: 'grey',
        tip: t('space.ade.board.taskStatus.tooltip'),
      },
      action: null,
    };
  }
  const mine = i.branches.filter((b) => b.kind === 'mine');
  const allMerged = mine.length > 0 && mine.every((b) => b.mergedIntoMain);
  if (progress.finished || (allMerged && !progress.hasWorkflow)) {
    return {
      tag: {
        label: t('common.merged.status'),
        tone: 'purple',
        tip: t('space.ade.board.taskMerged.tooltip'),
      },
      action: {
        kind: 'archive',
        label: t('space.common.archive.label'),
        tip: archiveTip(),
        tone: 'purple',
      },
    };
  }
  return {
    tag: {
      label: i.blocked ? t('space.common.status.blocked.status') : i.status.name,
      ...(i.blocked ? { tone: 'red' as const } : { color: i.status.color }),
      tip: t('space.ade.board.taskStatus.tooltip'),
    },
    action: stepAction(i),
  };
}

// ---- branch cell

type BranchActionKind =
  | 'seeError'
  | 'forcePush'
  | 'rebase'
  | 'queue'
  | 'changeBase'
  | 'abortRebase'
  | 'seeLog'
  | 'takeOver'
  | 'start';

export interface BranchAction {
  kind: BranchActionKind;
  label: string;
  tip: string;
  /** The tip with its base and branch names as variables, when it names any. */
  tipParts?: readonly TextPart[];
  disabled?: boolean;
  /** `rebase` / `queue` / `changeBase` / `abortRebase`: what the button runs. */
  act?: RebaseAct;
  /** `takeOver`: the rebase run's session. */
  sessionId?: string;
}

export interface BranchTag {
  label: string;
  tone: Tone;
  tip: string;
  /** The tip with its base names as variables, when it names any. */
  tipParts?: readonly TextPart[];
  actions: BranchAction[];
  /** Start of a running setup; the label's elapsed time re-reads the clock (`tagLabel`). */
  since?: number;
}

export function tagLabel(tag: { label: string; since?: number }, nowMs: number): string {
  return tag.since === undefined
    ? tag.label
    : t('space.ade.board.branchTag.preparing.label', {
        duration: formatCompactDuration(nowMs - tag.since),
      });
}

export interface BranchTagInput {
  branch: Branch;
  graph: BranchGraph;
  plan: Pick<Plan, 'unpushed'>;
  /** `TimelineView.after`: earlier-merging branch this one shares files with. */
  after: ReadonlyMap<string, { id: string; file: string }>;
  nowMs: number;
  /** The branch ever had a session (running or stopped). */
  hadSession: boolean;
  /** Short name of the repo's main branch, `''` when unresolved. */
  mainName: string;
  /** Runs of the branch's task. */
  runs: readonly Run[];
}

type Rule = (i: BranchTagInput) => BranchTag | null;

const tag = (label: string, tone: Tone, tip: string, actions: BranchAction[] = []): BranchTag => ({
  label,
  tone,
  tip,
  actions,
});

function nameOf(i: BranchTagInput, id: string): string {
  return i.graph.byBranch.get(id)?.name ?? id;
}

function rootIsMain(i: BranchTagInput): boolean {
  const p = i.graph.parentOf.get(i.branch.id);
  return p === undefined || i.graph.byBranch.get(p)?.kind !== 'mine';
}

function baseLabel(i: BranchTagInput): string {
  const b = i.branch;
  return b.baseBranchId === '' && (b.base === '' || b.base === i.mainName) ? 'main' : b.base;
}

function rebaseConflict(i: BranchTagInput): string[] {
  return i.branch.conflictCheck === 'done' ? i.branch.conflictsIfRebased : [];
}

const notCreated: Rule = (i) =>
  i.branch.name === ''
    ? tag(
        t('space.ade.board.branchTag.notCreated.label'),
        'grey',
        t('space.ade.board.branchTag.notCreated.tooltip'),
      )
    : null;

const preparing: Rule = (i) => {
  const s = i.branch.setup;
  if (s?.state !== 'running') return null;
  return {
    ...tag(
      t('space.ade.board.branchTag.preparing.label', {
        duration: formatCompactDuration(i.nowMs - s.startedAt),
      }),
      'blue',
      t('space.ade.board.branchTag.preparing.tooltip'),
    ),
    since: s.startedAt,
  };
};

const setupFailed: Rule = (i) =>
  i.branch.setup?.state === 'failed'
    ? tag(
        t('space.ade.needs.kind.setupFailed.label'),
        'red',
        i.branch.setup.note || t('space.ade.board.branchTag.setupFailed.tooltip'),
        [
          {
            kind: 'seeError',
            label: t('space.ade.needs.action.seeError.label'),
            tip: t('space.ade.board.branchTag.setupLog.tooltip'),
          },
        ],
      )
    : null;

const merged: Rule = (i) =>
  i.branch.mergedIntoMain
    ? tag(t('common.merged.status'), 'purple', t('space.ade.board.branchTag.merged.tooltip'))
    : null;

const parked: Rule = (i) =>
  i.branch.kind === 'parked'
    ? tag(
        t('space.ade.board.notMerging.label'),
        'grey',
        t('space.ade.board.branchTag.parked.tooltip'),
      )
    : null;

const review: Rule = (i) => {
  if (i.branch.kind !== 'review') return null;
  const conf = i.graph.conflicts.get(i.branch.id)?.[0];
  return conf
    ? tag(
        t('space.ade.board.branchTag.conflict.label'),
        'red',
        t('space.ade.board.branchTag.reviewOverlap.tooltip', {
          branch: nameOf(i, conf.with),
          files: conf.files.join(', '),
        }),
      )
    : tag(
        t('space.ade.board.branchTag.review.label'),
        'blue',
        t('space.ade.board.branchTag.review.tooltip'),
      );
};

const conflict: Rule = (i) => {
  const conf = i.graph.conflicts.get(i.branch.id)?.[0];
  if (!conf) return null;
  const name = nameOf(i, conf.with);
  return tag(
    t('space.ade.board.branchTag.conflict.label'),
    'red',
    t('space.ade.rebase.conflictWith.tooltip', { base: name, files: conf.files.join(', ') }),
    actsOf(i, 'queue'),
  );
};

const SHORT = {
  rebase: 'common.rebase.label',
  queue: 'space.ade.rebase.queueAfterShort.label',
  changeBase: 'space.ade.rebase.changeBase.label',
  abortRebase: 'space.ade.rebase.abort.label',
} as const satisfies Record<RebaseAct['kind'], MessageKey>;

function actionOf(act: RebaseAct): BranchAction {
  const label =
    act.kind === 'rebase' && act.isRetry ? t('space.ade.rebase.retry.label') : t(SHORT[act.kind]);
  return {
    kind: act.kind,
    label,
    tip: plain(act.tip),
    tipParts: act.tip,
    disabled: act.disabled,
    act,
  };
}

const plain = (parts: readonly TextPart[]): string =>
  parts.map((p) => (typeof p === 'string' ? p : p.value)).join('');

/** The rebase acts of the branch whose ids are listed, as tag buttons. */
function actsOf(i: BranchTagInput, ...ids: string[]): BranchAction[] {
  return rebaseActs({
    branch: i.branch,
    graph: i.graph,
    after: i.after,
    runs: i.runs,
    mainName: i.mainName,
  })
    .filter((a) => ids.includes(a.id))
    .map(actionOf);
}

function lastRebase(i: BranchTagInput): Run | null {
  let best: Run | null = null;
  for (const r of i.runs) {
    if (
      r.purpose === 'rebase' &&
      r.branchId === i.branch.id &&
      (!best || r.attempt >= best.attempt)
    ) {
      best = r;
    }
  }
  return best;
}

const seeLog = (): BranchAction => ({
  kind: 'seeLog',
  label: t('space.ade.board.branchTag.seeLog.label'),
  tip: t('space.ade.board.branchTag.seeLog.tooltip'),
});

function takeOverOf(run: Run | null): BranchAction[] {
  return run && run.sessionId !== ''
    ? [
        {
          kind: 'takeOver',
          label: t('common.action.takeOver.label'),
          tip: t('space.ade.board.branchTag.takeOver.tooltip'),
          sessionId: run.sessionId,
        },
      ]
    : [];
}

const rebasing: Rule = (i) => {
  const run = lastRebase(i);
  return run && (run.state === 'running' || run.state === 'pending')
    ? tag(
        t('git.op.inProgress.rebase.label'),
        'blue',
        t('space.ade.board.branchTag.rebasing.tooltip'),
        [seeLog()],
      )
    : null;
};

const rebasePending: Rule = (i) => {
  const b = i.branch;
  if (b.kind !== 'mine' || b.basePendingFrom === '') return null;
  const run = lastRebase(i);
  const reason = run?.outcome?.reason ? `\n${run.outcome.reason}` : '';
  const parts: TextPart[] = [
    ...branchParts('space.ade.board.branchTag.rebasePendingNote.hint', {
      from: b.basePendingFrom,
      to: b.base,
    }),
    ...(reason ? [reason] : []),
  ];
  return {
    ...tag(t('space.ade.board.branchTag.rebasePending.label'), 'amber', plain(parts), [
      ...actsOf(i, 'rebase-retry', 'abort-rebase'),
      ...(run ? [seeLog()] : []),
      ...takeOverOf(run && (run.state === 'failed' || run.state === 'stuck') ? run : null),
    ]),
    tipParts: parts,
  };
};

const rebaseRunFailed: Rule = (i) => {
  const run = lastRebase(i);
  if (!run || (run.state !== 'failed' && run.state !== 'stuck')) return null;
  const stuck = run.state === 'stuck';
  return tag(
    t(
      stuck
        ? 'space.ade.board.branchTag.rebaseNeedsYou.label'
        : 'space.ade.board.branchTag.rebaseFailed.label',
    ),
    'red',
    run.outcome?.reason ||
      t(
        stuck
          ? 'space.ade.board.branchTag.rebaseNeedsYou.tooltip'
          : 'space.ade.board.branchTag.rebaseFailed.tooltip',
      ),
    [
      ...actsOf(i, 'rebase-retry', 'rebase-base', 'rebase-self', 'abort-rebase'),
      seeLog(),
      ...takeOverOf(run),
    ],
  );
};

const rebaseInProgress: Rule = (i) =>
  i.branch.kind === 'mine' && i.branch.rebaseInProgress
    ? tag(
        t('space.ade.board.branchTag.rebaseInProgress.label'),
        'amber',
        t('space.ade.board.branchTag.rebaseInProgress.tooltip'),
        actsOf(i, 'abort-rebase'),
      )
    : null;

const baseMissing: Rule = (i) => {
  const b = i.branch;
  if (b.kind !== 'mine' || !b.baseMissing) return null;
  const parts = branchParts('space.ade.board.branchTag.baseMissing.tooltip', { base: b.base });
  return {
    ...tag(
      t('space.ade.board.branchTag.baseMissing.label'),
      'red',
      plain(parts),
      actsOf(i, 'change-base'),
    ),
    tipParts: parts,
  };
};

const rebaseConflicts: Rule = (i) => {
  const paths = rebaseConflict(i);
  if (!paths.length) return null;
  const base = baseLabel(i);
  return tag(
    t('space.ade.board.branchTag.conflict.label'),
    'red',
    t('space.ade.board.branchTag.rebaseConflicts.tooltip', { base, files: paths.join(', ') }),
    actsOf(i, 'rebase-base', 'rebase-self'),
  );
};

const notPushed: Rule = (i) =>
  i.plan.unpushed[i.branch.id]
    ? tag(
        t('space.ade.board.branchTag.notPushed.label'),
        'amber',
        t('space.ade.board.branchTag.notPushed.tooltip'),
        [
          {
            kind: 'forcePush',
            label: t('common.forcePush.label'),
            tip: 'git push --force-with-lease',
          },
        ],
      )
    : null;

const behindMain: Rule = (i) =>
  rootIsMain(i) && i.branch.behind > 0
    ? tag(
        t('space.ade.board.branchTag.behind.label', {
          behind: i.branch.behind,
          base: baseLabel(i),
        }),
        'amber',
        t('space.ade.board.branchTag.behind.tooltip', {
          count: i.branch.behind,
          base: baseLabel(i),
        }),
        actsOf(i, 'rebase-base'),
      )
    : null;

const sharesFiles: Rule = (i) => {
  const a = i.after.get(i.branch.id);
  if (!a) return null;
  const name = nameOf(i, a.id);
  return tag(
    t('space.ade.board.branchTag.sharesFiles.label', { branch: shortBranchName(name) }),
    'amber',
    t('space.ade.board.branchTag.sharesFiles.tooltip', { file: a.file, branch: name }),
    actsOf(i, 'rebase-onto'),
  );
};

const waitingOnReview: Rule = (i) => {
  const pid = i.graph.parentOf.get(i.branch.id);
  const p = pid === undefined ? undefined : i.graph.byBranch.get(pid);
  return p?.kind === 'review'
    ? tag(
        t('space.ade.board.branchTag.waiting.label', { owner: p.owner }),
        'blue',
        t('space.ade.board.branchTag.waiting.tooltip', { branch: p.name }),
      )
    : null;
};

const baseNeedsAttention: Rule = (i) => {
  const anc = i.graph.ancestors(i.branch.id).map((id) => i.graph.byBranch.get(id));
  const conflicted = anc.some(
    (a) =>
      a &&
      ((i.graph.conflicts.get(a.id)?.length ?? 0) > 0 ||
        (a.conflictCheck === 'done' && a.conflictsIfRebased.length > 0)),
  );
  const label = conflicted
    ? t('space.ade.board.branchTag.baseConflict.label')
    : anc.some((a) => (a?.behind ?? 0) > 0)
      ? t('space.ade.board.branchTag.baseBehind.label')
      : '';
  return label ? tag(label, 'amber', t('space.ade.board.branchTag.baseAttention.tooltip')) : null;
};

const checking: Rule = (i) =>
  i.branch.conflictCheck === 'checking'
    ? tag(
        t('space.ade.board.branchTag.checking.loading'),
        'grey',
        t('space.ade.board.branchTag.checking.tooltip'),
      )
    : null;

const checkFailed: Rule = (i) =>
  i.branch.conflictCheck === 'failed'
    ? tag(
        t('space.ade.board.branchTag.checkFailed.label'),
        'amber',
        i.branch.conflictCheckReason || t('space.ade.board.branchTag.checkFailed.label'),
      )
    : null;

const clean: Rule = () =>
  tag(
    t('space.ade.board.branchTag.clean.label'),
    'green',
    t('space.ade.board.branchTag.clean.tooltip'),
  );

// First match wins; `clean` is the unconditional last rung.
const BRANCH_RULES: readonly Rule[] = [
  notCreated,
  preparing,
  setupFailed,
  merged,
  parked,
  review,
  rebasing,
  rebasePending,
  rebaseRunFailed,
  rebaseInProgress,
  baseMissing,
  conflict,
  rebaseConflicts,
  notPushed,
  behindMain,
  sharesFiles,
  waitingOnReview,
  baseNeedsAttention,
  checking,
  checkFailed,
  clean,
];

export function branchTag(i: BranchTagInput): BranchTag {
  let result: BranchTag = tag(t('space.ade.board.branchTag.clean.label'), 'green', '');
  for (const rule of BRANCH_RULES) {
    const found = rule(i);
    if (found) {
      result = found;
      break;
    }
  }
  const b = i.branch;
  const setupOk = b.setup === null || b.setup.state === 'ready';
  if (b.kind === 'mine' && b.name !== '' && !b.mergedIntoMain && !i.hadSession && setupOk) {
    result.actions.push({
      kind: 'start',
      label: t('common.action.start.label'),
      tip: t('space.ade.board.branchTag.start.tooltip'),
    });
  }
  return result;
}
