import { t } from '@theme/i18n';
import type { TextPart } from '@theme/varText';
import { taskColor } from '../palette';
import type { Branch } from '../wire';
import { type BranchGraph, isDraft, shortBranchName } from './branchGraph';
import { branchParts } from './catalogParts';

export interface BaseMarker {
  /** `⑂` for a base inside the same task, else `⑂ <base without its first path segment>`. */
  label: TextPart[];
  /** Blue when the base is someone else's branch, else grey. */
  tone: 'grey' | 'blue';
  tip: TextPart[];
  /** The base changed and no verified rebase has moved the branch onto it yet. */
  pending: boolean;
  /** Work colour of the parent task's branch, `null` for trunk. */
  color: string | null;
}

const base = (value: string): TextPart => ({ name: 'base', value, kind: 'branch' });

function withVariant(tip: TextPart[], tone: 'grey' | 'blue', pending: boolean): TextPart[] {
  const variant = pending ? 'warn' : tone === 'blue' ? 'info' : 'default';
  return tip.map((p) => (typeof p === 'string' || p.kind !== 'branch' ? p : { ...p, variant }));
}

/** Mockup v2 line 1818: nothing when the branch starts from main. */
export function baseMarker(b: Branch, graph: BranchGraph, mainName: string): BaseMarker | null {
  const pending = b.basePendingFrom !== '';
  const parent = graph.parentOf.get(b.id);
  const p = parent === undefined ? undefined : graph.byBranch.get(parent);
  if (p) {
    const outside = p.taskId !== b.taskId;
    const vars = { base: p.name };
    const tip =
      p.kind === 'review'
        ? branchParts('space.ade.baseMarker.startsFromOwner.tooltip', vars, { owner: p.owner })
        : branchParts(
            outside
              ? 'space.ade.baseMarker.startsFromOtherTask.tooltip'
              : 'space.ade.baseMarker.startsFromNotMain.tooltip',
            vars,
          );
    const tone = p.kind === 'review' ? 'blue' : 'grey';
    const parentTask = graph.byTask.get(p.taskId);
    return {
      label: outside ? ['⑂ ', base(shortBranchName(p.name))] : ['⑂'],
      tone,
      tip: withVariant(tip, tone, pending),
      pending,
      color: parentTask ? taskColor(parentTask.color) : null,
    };
  }
  if (b.base === '' || b.base === mainName) {
    return pending
      ? {
          label: ['⑂ ', base(b.base || mainName)],
          tone: 'grey',
          tip: withVariant(
            branchParts('space.ade.baseMarker.startsFrom.tooltip', { base: b.base || mainName }),
            'grey',
            pending,
          ),
          pending,
          color: null,
        }
      : null;
  }
  const owner = b.baseOwner;
  const tone = owner ? 'blue' : 'grey';
  const tip = owner
    ? branchParts('space.ade.baseMarker.startsFromOwner.tooltip', { base: b.base }, { owner })
    : branchParts('space.ade.baseMarker.startsFromNotMain.tooltip', { base: b.base });
  return {
    label: ['⑂ ', base(shortBranchName(b.base))],
    tone,
    tip: withVariant(tip, tone, pending),
    pending,
    color: null,
  };
}

/** Second-line context of a branch not created yet: `Forks from <base>`. */
export function baseContext(b: Branch, graph: BranchGraph): string {
  if (!isDraft(b)) return '';
  const parent = graph.parentOf.get(b.id);
  const base =
    (parent !== undefined ? graph.byBranch.get(parent)?.name : undefined) || b.base || 'main';
  return t('space.ade.baseMarker.draft.label', { base });
}
