import { fetchRequiredServices, needsStart } from '../services/requiredServices';
import { useAdeServicesGateStore } from './adeServicesGate';

/** Asks about the task's required services before a user start. */
export function useServicesGate() {
  const gate = useAdeServicesGateStore();

  /** True when the start may go on: nothing is missing, the task was skipped, or the user continues. */
  async function ensureServices(taskId: string): Promise<boolean> {
    if (taskId === '' || gate.skipped.has(taskId)) return true;
    const { items } = await fetchRequiredServices(taskId);
    return !items.some(needsStart) || gate.ask(taskId);
  }

  return { ensureServices };
}
