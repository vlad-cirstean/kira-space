import { t } from '@theme/i18n';
import type { TextPart } from '@theme/varText';
import type { BranchRowModel, CardModel } from '../plan/usePlanModel';
import type { Task } from '../wire';
import type { BranchGraph } from './branchGraph';
import { branchParts } from './catalogParts';

// One rule for every Review code control: hidden, disabled with a reason, or enabled with a preview.

export interface ReviewChoice {
  branchId: string;
  /** `repo · branch`, the row's own menu label. */
  label: string;
  disabled: boolean;
  /** The reason when disabled, else what the click opens. */
  tip: readonly TextPart[];
}

/** The branch the review diffs against: its stacking parent, else its base, else `main`. */
export function baseNameOf(row: BranchRowModel, graph: BranchGraph): string {
  const parent = graph.parentOf.get(row.branch.id);
  return (parent ? graph.byBranch.get(parent)?.name : '') || row.branch.base || 'main';
}

function enabledTip(
  ahead: number,
  dirty: number,
  vars: { base: string; branch: string },
): TextPart[] {
  if (ahead === 0) {
    return branchParts('space.ade.board.reviewChoice.reviewUncommitted.tooltip', vars, {
      count: dirty,
    });
  }
  if (dirty > 0) {
    return branchParts('space.ade.board.reviewChoice.reviewMixed.tooltip', vars, { count: dirty });
  }
  return branchParts('space.ade.board.reviewChoice.review.tooltip', vars);
}

export function reviewChoice(
  row: BranchRowModel,
  task: Task,
  graph: BranchGraph,
): ReviewChoice | null {
  const b = row.branch;
  if (b.kind === 'parked') return null;
  const vars = { base: baseNameOf(row, graph), branch: b.name };
  const choice = (disabled: boolean, tip: readonly TextPart[]): ReviewChoice => ({
    branchId: row.id,
    label: `${row.repo} · ${row.name}`,
    disabled,
    tip,
  });
  if (b.name === '')
    return choice(true, branchParts('space.ade.board.reviewChoice.createFirst.tooltip'));
  if (b.baseMissing) {
    return choice(true, branchParts('space.ade.rebase.baseMissing.tooltip', vars));
  }
  const dirty = b.dirty.length;
  if (b.ahead === 0 && dirty === 0) {
    return choice(true, branchParts('space.ade.board.reviewChoice.nothing.tooltip', vars));
  }
  const working = task.runs.some((r) => r.branchId === row.id && r.state === 'running');
  const tip = enabledTip(b.ahead, dirty, vars);
  if (working) tip.push(`\n${t('space.ade.board.reviewChoice.working.tooltip')}`);
  return choice(false, tip);
}

export function reviewChoices(card: CardModel, graph: BranchGraph): ReviewChoice[] {
  return card.rows.flatMap((row) => reviewChoice(row, card.task, graph) ?? []);
}

/** Tooltip of a task-level control: the branch's own tip when there is one, else a prompt to pick. */
export function taskReviewTip(choices: readonly ReviewChoice[]): readonly TextPart[] {
  return choices.length === 1
    ? choices[0].tip
    : branchParts('space.ade.board.reviewChoice.pick.tooltip');
}
