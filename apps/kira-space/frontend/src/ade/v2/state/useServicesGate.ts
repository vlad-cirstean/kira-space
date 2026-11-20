import { fetchRequiredServices, needsStart } from '../services/requiredServices';
import type { RequiredService } from '../wire';
import { useAdeServicesGateStore } from './adeServicesGate';

// The ask is advisory: a failed read of the services never blocks a start (P697 review R1-3).
/** Asks about the task's required services before a user start. */
export function useServicesGate() {
  const gate = useAdeServicesGateStore();

  /** True when the start may go on: nothing is missing, the task was skipped, or the user continues. */
  async function ensureServices(taskId: string): Promise<boolean> {
    if (taskId === '' || gate.skipped.has(taskId)) return true;
    let items: RequiredService[];
    try {
      ({ items } = await fetchRequiredServices(taskId));
    } catch {
      return true;
    }
    return !items.some(needsStart) || gate.ask(taskId);
  }

  return { ensureServices };
}
