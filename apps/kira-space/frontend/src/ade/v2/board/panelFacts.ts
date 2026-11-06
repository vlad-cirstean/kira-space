import { t } from '@theme/i18n';
import type { MergedInto, RemoteOpError, RepoRefresh, Task, TaskPatch } from '../wire';
import type { TaskProgress } from './progress';

/** Why the status chip reads as it does: the user set it, or the workflow position did. */
export function statusWhy(task: Task, blocked: boolean, progress: TaskProgress): string {
  if (task.kind === 'review') return t('space.ade.board.branchTag.review.tooltip');
  if (task.kind === 'parked') return t('space.ade.board.branchTag.parked.tooltip');
  if (task.statusBy === 'user') return t('space.ade.panelFacts.why.manual.hint');
  if (progress.finished) return t('space.ade.panelFacts.why.finished.hint');
  if (blocked) return t('space.ade.panelFacts.why.stuck.hint');
  if (task.statusStep === '') return t('space.ade.panelFacts.why.notStarted.hint');
  return progress.step ? t('space.ade.panelFacts.why.step.hint', { step: progress.step.name }) : '';
}

/** `Today–Wed 23 · 3 branches in api, web-app · 1/5 steps`. */
export function taskFacts(i: {
  span: string;
  branches: number;
  repos: string[];
  doneSteps: number;
  steps: number;
}): string {
  const parts = [
    i.span,
    i.repos.length
      ? t('space.ade.panelFacts.branchesIn.label', { count: i.branches, repos: i.repos.join(', ') })
      : t('space.ade.panelFacts.branches.label', { count: i.branches }),
  ];
  if (i.steps > 0)
    parts.push(t('space.ade.panelFacts.steps.label', { count: i.steps, done: i.doneSteps }));
  return parts.join(' · ');
}

/** A remote operation's failure as one line: the remote's own words when it sent any. */
export function remoteErrorText(e: RemoteOpError): string {
  return e.remoteMessage
    ? t('common.dashPair.label', { first: e.message, second: e.remoteMessage })
    : e.message;
}

function countByTarget(merged: readonly MergedInto[]): Map<string, number> {
  const byTarget = new Map<string, number>();
  for (const m of merged) byTarget.set(m.target, (byTarget.get(m.target) ?? 0) + 1);
  return byTarget;
}

/** `3 refs changed · 1 merged into develop`; `No changes` when the fetch moved nothing. */
export function refreshNote(r: RepoRefresh): string {
  const byTarget = countByTarget(r.mergedInto);
  if (r.refsChanged === 0 && byTarget.size === 0) return t('space.ade.refresh.noChanges.status');
  const parts = [t('space.ade.refresh.refsChanged.status', { count: r.refsChanged })];
  for (const [target, n] of byTarget)
    parts.push(t('space.ade.refresh.mergedInto.status', { count: n, target }));
  return parts.join(' · ');
}

/** A patch that changes nothing; spread the fields to write over it (null = unchanged). */
export function taskPatch(over: Partial<TaskPatch>): TaskPatch {
  return {
    title: null,
    jira: null,
    clearJira: false,
    hostUrl: null,
    est: null,
    color: null,
    kind: null,
    statusId: null,
    ...over,
  };
}
