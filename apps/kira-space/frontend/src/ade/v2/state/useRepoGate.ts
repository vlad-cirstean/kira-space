import { useAdeRepoGateStore } from './adeRepoGate';

/** The bridge refuses a work start on a task with no repo with this code. */
const isTaskNoRepo = (err: unknown): boolean =>
  typeof err === 'object' && err !== null && (err as { code?: unknown }).code === 'E_TASK_NO_REPO';

/** Asks for a repo before a start, and again when the bridge refuses a start the board thought fine. */
export function useRepoGate() {
  const gate = useAdeRepoGateStore();

  /** Runs `call`; on E_TASK_NO_REPO (a stale board) asks for a repo and runs it once more. Cancel or any other error rethrows. */
  async function retryAfterRepo<T>(taskId: string, call: () => Promise<T>): Promise<T> {
    try {
      return await call();
    } catch (err) {
      if (!isTaskNoRepo(err) || !(await gate.ask(taskId))) throw err;
      return call();
    }
  }

  /** With `needsRepo` asks for a repo first; false when the user cancels. */
  async function ensureRepo(taskId: string, needsRepo: boolean): Promise<boolean> {
    return !needsRepo || (await gate.ask(taskId));
  }

  /** Runs `start`; with `needsRepo` asks for a repo first and stops on Cancel. */
  async function gated(
    taskId: string,
    needsRepo: boolean,
    start: () => Promise<void>,
  ): Promise<void> {
    if (!(await ensureRepo(taskId, needsRepo))) return;
    await retryAfterRepo(taskId, start);
  }

  return { gated, ensureRepo, retryAfterRepo };
}
