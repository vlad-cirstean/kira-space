import type { ScriptRun } from '@shared/domain/scriptRuns';

/** The run a task card's chip shows: the newest running one, else the newest failed or blocked one
 *  the person has not opened yet. */
export function chipRun(
  runs: readonly ScriptRun[],
  taskId: string,
  seen: readonly string[],
): ScriptRun | null {
  const mine = runs.filter((r) => r.taskId === taskId);
  const running = mine.find((r) => r.state === 'running');
  if (running) return running;
  return (
    mine.find((r) => (r.state === 'failed' || r.state === 'blocked') && !seen.includes(r.id)) ??
    null
  );
}

/** `chipRun` for every task that has one, grouped in one pass over the runs. */
export function chipRunsByTask(
  runs: readonly ScriptRun[],
  seen: readonly string[],
): Map<string, ScriptRun> {
  const byTask = new Map<string, ScriptRun[]>();
  for (const r of runs) {
    if (!r.taskId) continue;
    const list = byTask.get(r.taskId);
    if (list) list.push(r);
    else byTask.set(r.taskId, [r]);
  }
  const out = new Map<string, ScriptRun>();
  for (const [taskId, mine] of byTask) {
    const run = chipRun(mine, taskId, seen);
    if (run) out.set(taskId, run);
  }
  return out;
}

export function runReason(run: ScriptRun): string {
  return run.outcome?.reason || run.outcome?.summary || '';
}
