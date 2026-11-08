import type { Branch, Task, TaskStatusDef } from '../wire';
import { isWorkKind } from './kind';
import type { TaskProgress } from './progress';

/** The task's stored status; an id missing from the set (a transient cache state) shows as its id in grey. */
export function statusOf(task: Task, statuses: readonly TaskStatusDef[]): TaskStatusDef {
  return (
    statuses.find((s) => s.id === task.statusId) ?? {
      id: task.statusId,
      name: task.statusId,
      color: 'grey',
    }
  );
}

/** A stuck or failed run, or a failed worktree setup. Not a status: it shows beside the status. */
export function blocked(task: Task, progress: TaskProgress, branches: readonly Branch[]): boolean {
  if (!isWorkKind(task.kind) || !progress.hasWorkflow || progress.finished) return false;
  return progress.bad || branches.some((b) => b.setup?.state === 'failed');
}
