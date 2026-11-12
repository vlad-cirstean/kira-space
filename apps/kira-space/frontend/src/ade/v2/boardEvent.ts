import type { QueryClient } from '@tanstack/vue-query';
import type { BoardChangedEvent } from './wire';

/** A board push with every field settled: `refs` true and `taskIds` null (all) unless the payload says otherwise. */
export interface BoardChange {
  refs: boolean;
  taskIds: string[] | null;
}

/** Reads a board push payload; none, or an unknown shape, means everything changed. */
export function normalizeBoardEvent(raw: BoardChangedEvent | null | undefined): BoardChange {
  return {
    refs: raw?.refs !== false,
    taskIds: Array.isArray(raw?.taskIds) ? raw.taskIds : null,
  };
}

/**
 * Invalidates a per-task query family (`[...prefix, taskId, ...]`): only the pushed tasks when the
 * push names them, the whole family otherwise.
 */
export function invalidateTaskFamily(
  queryClient: QueryClient,
  prefix: readonly unknown[],
  change: BoardChange,
): void {
  if (change.taskIds === null) {
    void queryClient.invalidateQueries({ queryKey: prefix });
    return;
  }
  for (const taskId of change.taskIds) {
    void queryClient.invalidateQueries({ queryKey: [...prefix, taskId] });
  }
}
