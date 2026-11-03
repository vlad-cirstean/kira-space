import { t } from '@theme/i18n';
import { reviewPhaseLabel, runStateLabel } from '../runState';
import type { Workflow } from '../wire';
import type { Tone } from './actions';
import type { StepProgress, StepRun, StepState, TaskProgress } from './progress';
import { defaultRoute } from './stepResults';
import { runsOnLabel } from './workflowForm';

// Task panel Workflow block (SPEC2 section 7, mockup `phaseBlocks`): one block per step with its
// per-repo run lines. Pure, so the card model carries it ready to render.

interface RunLine {
  run: StepRun;
  glyph: string;
  tone: Tone;
  status: string;
  /** The run's own note, else `fix round n of max` for a fix run (`fix round n` without a budget). */
  note: string;
  /** `↩ 2/3` (`↩ 2` with no limit) once the step went back; '' before. */
  returns: string;
  returnsTip: string;
  /** The run stopped because the step used all its returns. */
  returnsSpent: boolean;
  /** Log (agent) / Output (script) opens the run's log: shown once the run started. */
  hasLog: boolean;
  canRetry: boolean;
  canResume: boolean;
  /** Review step run waiting on or between rounds: the Review button opens the window. */
  canReview: boolean;
}

export interface StepBlock {
  step: StepProgress;
  state: 'past' | 'now' | 'next' | 'skipped';
  statusText: string;
  tone: Tone;
  /** Agent: the `runsOn` text; script: the command; manual: ''. */
  scope: string;
  gated: boolean;
  routeText: string;
  runs: RunLine[];
  /** Kind line: `Agent · background claude -p`, `Script`, `Review`, `Manual`. */
  mode: string;
  /** The Release block lists the task's branches under its row. */
  release: boolean;
}

const RUN_GLYPH: Record<StepRun['state'], { glyph: string; tone: Tone }> = {
  done: { glyph: '✓', tone: 'green' },
  running: { glyph: '●', tone: 'amber' },
  back: { glyph: '↩', tone: 'amber' },
  stuck: { glyph: '!', tone: 'red' },
  failed: { glyph: '✕', tone: 'red' },
  pending: { glyph: '○', tone: 'grey' },
  review: { glyph: '◐', tone: 'blue' },
};

const STEP_TONE: Record<StepState, Tone> = {
  done: 'green',
  running: 'amber',
  review: 'blue',
  stuck: 'red',
  failed: 'red',
  pending: 'grey',
  skipped: 'grey',
};

function stepStatusText(s: StepProgress, now: boolean): string {
  if (s.kind === 'manual') {
    if (s.state === 'skipped') return runStateLabel('pending');
    return now ? t('automations.run.state.waiting.status') : runStateLabel('pending');
  }
  if (s.state === 'stuck') return t('space.ade.allSessions.stuck.status');
  if (now && s.state === 'pending' && !s.runs.length)
    return t('space.ade.stepBlock.needsRepo.status');
  if (s.state === 'pending' && s.approval) return t('space.ade.step.waitingApproval.status');
  return runStateLabel(s.state);
}

/** The step's non-default routes, `changes ↩ Implement`, `trivial → end`; '' when it has none. */
function routeText(s: StepProgress, steps: readonly StepProgress[]): string {
  const at = new Map(steps.map((x, i) => [x.id, i]));
  const own = at.get(s.id) ?? -1;
  const nameOf = (id: string): string => steps.find((x) => x.id === id)?.name ?? id;
  const parts: string[] = [];
  for (const r of s.results) {
    if (r.next === defaultRoute(r.ok)) continue;
    if (r.next === 'end') parts.push(t('space.ade.step.routeEnd.label', { id: r.id }));
    else if (r.next === 'stop') parts.push(t('space.ade.step.routeStop.label', { id: r.id }));
    else if (r.next === s.id) parts.push(t('space.ade.step.routeRetry.label', { id: r.id }));
    else if ((at.get(r.next) ?? Number.POSITIVE_INFINITY) <= own)
      parts.push(t('space.ade.step.routeBack.label', { id: r.id, step: nameOf(r.next) }));
    else parts.push(t('space.ade.step.routeGoto.label', { id: r.id, step: nameOf(r.next) }));
  }
  return parts.join(' · ');
}

function runStatus(r: StepRun): string {
  if (r.state === 'done') {
    return r.ok === false ? t('space.ade.step.failedContinued.status') : runStateLabel('done');
  }
  if (r.state === 'back') {
    return r.route === 'retry'
      ? t('common.action.retry.label')
      : t('space.ade.outcome.back.status');
  }
  if (r.state === 'review' && r.review) return reviewPhaseLabel(r.review.phase);
  return runStateLabel(r.state);
}

/** Limit of the steps that send the task back to step `id`: the largest `maxReturns`, 0 when none. */
function backLimit(steps: readonly StepProgress[], id: string): number {
  let limit = 0;
  for (const s of steps) {
    if (s.id !== id && s.results.some((r) => r.next === id)) limit = Math.max(limit, s.maxReturns);
  }
  return limit;
}

function fixRound(loops: number, max: number): string {
  if (!loops) return '';
  return max
    ? t('space.ade.step.fixRoundMax.status', { count: loops, max })
    : t('space.ade.step.fixRound.status', { count: loops });
}

function returnsChip(
  r: StepRun,
  limit: number,
): Pick<RunLine, 'returns' | 'returnsTip' | 'returnsSpent'> {
  if (r.returns === 0) return { returns: '', returnsTip: '', returnsSpent: false };
  const vars = { count: r.returns, limit };
  const spent = r.state === 'failed' && r.route === 'stop' && limit > 0 && r.returns >= limit;
  return limit > 0
    ? {
        returns: t('space.ade.step.returns.label', vars),
        returnsTip: t('space.ade.step.returns.tooltip', vars),
        returnsSpent: spent,
      }
    : {
        returns: t('space.ade.step.returnsUnlimited.label', vars),
        returnsTip: t('space.ade.step.returnsUnlimited.tooltip', vars),
        returnsSpent: false,
      };
}

function runLine(r: StepRun, max: number, limit: number): RunLine {
  const g = RUN_GLYPH[r.state];
  const started = r.state !== 'pending' && r.runId !== '';
  return {
    run: r,
    glyph: g.glyph,
    tone: g.tone,
    status: runStatus(r),
    note: r.reason || r.note || fixRound(r.loops, max),
    ...returnsChip(r, limit),
    hasLog: started,
    canRetry: started && (r.state === 'failed' || r.state === 'stuck'),
    canResume: r.resumable,
    canReview: r.state === 'review',
  };
}

function showRuns(s: StepProgress): boolean {
  const started = s.runs.some((r) => r.state !== 'pending' || r.note);
  return (
    started &&
    (s.runs.length > 1 ||
      s.kind === 'script' ||
      s.runs.some((r) => r.state !== s.state || r.note || r.loops > 0))
  );
}

function modeText(s: StepProgress): string {
  switch (s.kind) {
    case 'agent':
      return t('space.ade.step.modeAgent.label');
    case 'script':
      return t('common.script.label');
    case 'review':
      return t('space.ade.step.modeReview.label');
    case 'manual':
      return s.session ? t('space.ade.step.modeManual.label') : t('space.ade.step.modeOwn.label');
  }
}

/** One block per workflow step; `p` is the task's progress, `workflow` supplies script commands. */
export function buildStepBlocks(p: TaskProgress, workflow: Workflow | null): StepBlock[] {
  const idx = p.finished ? p.steps.length : p.stepIndex;
  return p.steps.map((s, k): StepBlock => {
    const state: StepBlock['state'] =
      s.state === 'skipped' ? 'skipped' : k < idx ? 'past' : k === idx ? 'now' : 'next';
    const command =
      s.kind === 'script' ? (workflow?.steps.find((x) => x.id === s.id)?.command ?? '') : '';
    return {
      step: s,
      state,
      statusText: stepStatusText(s, state === 'now'),
      tone: STEP_TONE[s.state],
      scope: s.kind === 'script' ? command : s.kind === 'manual' ? '' : runsOnLabel(s.runsOn),
      gated: s.kind !== 'manual' && s.before === 'approval',
      routeText: routeText(s, p.steps),
      runs: showRuns(s)
        ? s.runs.map((r) => runLine(r, backLimit(p.steps, s.id), s.maxReturns))
        : [],
      mode: modeText(s),
      release: s.id === 'release' && k <= idx,
    };
  });
}

/** Step a skip or Next from `fromId` lands on: the next step not skipped, else `done`. */
export function nextStepId(blocks: readonly StepBlock[], fromId: string): string {
  const at = blocks.findIndex((b) => b.step.id === fromId);
  return blocks.slice(at + 1).find((b) => b.state !== 'skipped' && !b.step.skip)?.step.id ?? 'done';
}
