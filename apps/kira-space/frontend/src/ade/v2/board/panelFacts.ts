import { t } from '@theme/i18n';
import type { RemoteOpError, Task, TaskPatch } from '../wire';
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

/** A remote operation's failure as one line: the remote's own words when it sent any. */
export function remoteErrorText(e: RemoteOpError): string {
  return e.remoteMessage
    ? t('common.dashPair.label', { first: e.message, second: e.remoteMessage })
    : e.message;
}

/** The branch a force push dialog is open for; `taskId` is the owning task, which the op's chip and status match on. */
export interface ForcePushTarget {
  branchId: string;
  branchName: string;
  repo: string;
  taskId: string;
}

/** Rejects with the remote's failure line, so an op raises its error toast. */
export function throwOnRemoteError<T extends { error: RemoteOpError | null }>(res: T): T {
  if (res.error) throw new Error(remoteErrorText(res.error));
  return res;
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
