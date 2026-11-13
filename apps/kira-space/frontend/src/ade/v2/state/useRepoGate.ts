import { OpAborted } from './adeOps';
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

  /**
   * Runs `call` as an op (`startOp` is `ops.run` with its id). A stale board that lets the bridge refuse
   * with E_TASK_NO_REPO ends that op silently, then asks for a repo outside any op (no chip or loading
   * toast while the user picks) and starts a fresh op. False on failure, or when the user cancels the pick.
   */
  async function opAfterRepo(
    taskId: string,
    call: () => Promise<unknown>,
    startOp: (fn: () => Promise<unknown>) => Promise<boolean>,
  ): Promise<boolean> {
    let refused = false;
    const ok = await startOp(async () => {
      try {
        await call();
      } catch (err) {
        if (!isTaskNoRepo(err)) throw err;
        refused = true;
        throw new OpAborted();
      }
    });
    if (!refused) return ok;
    if (!(await gate.ask(taskId))) return false;
    return startOp(call);
  }

  return { ensureRepo, retryAfterRepo, opAfterRepo };
}
