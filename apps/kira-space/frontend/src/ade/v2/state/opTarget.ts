import { compareNatural } from '@workbench/behaviour/match';

/**
 * Op target of a retry or resume, shared by every surface (step card, panel, Needs you) so the same
 * run dedupes across them: the sorted run ids, or the task and step when no run has an id yet.
 */
export function runsTarget(runIds: readonly string[], taskId: string, stepId: string): string {
  const ids = runIds.filter((r) => r !== '');
  return ids.length > 0 ? [...ids].sort(compareNatural).join('+') : `${taskId}:${stepId}`;
}
