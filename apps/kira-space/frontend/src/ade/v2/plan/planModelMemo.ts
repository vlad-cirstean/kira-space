import { replaceEqualDeep } from '@tanstack/vue-query';
import type { TaskProgress } from '../board/progress';
import type { Task, Workflow } from '../wire';

// Pure caches behind the plan model, so a push that touches one task leaves every other task's
// derived objects identical and Vue skips their render.

interface ProgressEntry {
  workflow: Workflow | null;
  facts: string;
  progress: TaskProgress;
}

export type ProgressCache = WeakMap<Task, ProgressEntry>;

/**
 * A task's progress, reused while the task object (structurally shared by the query cache), its
 * workflow and `facts` (what progress reads from branches and repo names, as one string) are
 * unchanged. Runs live on the task, so a run change gives a new task and a rebuild.
 */
export function cachedProgress(
  cache: ProgressCache,
  task: Task,
  workflow: Workflow | null,
  facts: string,
  build: () => TaskProgress,
): TaskProgress {
  const hit = cache.get(task);
  if (hit && hit.workflow === workflow && hit.facts === facts) return hit.progress;
  const progress = build();
  cache.set(task, { workflow, facts, progress });
  return progress;
}

/**
 * Builds a model for each id and keeps the previous object for any whose build is deeply equal, so
 * unchanged cards keep their identity across rebuilds. Ids no longer listed drop out. A built model
 * that cannot be compared (non-plain values) simply replaces the old one: never stale, only unshared.
 */
export function reuseModels<T>(
  previous: ReadonlyMap<string, T>,
  ids: Iterable<string>,
  build: (id: string) => T | null,
): Map<string, T> {
  const next = new Map<string, T>();
  for (const id of ids) {
    const built = build(id);
    if (built === null) continue;
    const before = previous.get(id);
    next.set(id, before === undefined ? built : replaceEqualDeep(before, built));
  }
  return next;
}
