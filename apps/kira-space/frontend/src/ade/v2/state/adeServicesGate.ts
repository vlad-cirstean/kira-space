import { useScriptRunDialogStore } from '@workbench/automations/run/runDialog';
import { queryClient } from '@workbench/state/queryClient';
import { defineStore } from 'pinia';
import { reactive, ref } from 'vue';
import { needsStart, requiredServicesKey } from '../services/requiredServices';
import type { RequiredService, RequiredServices } from '../wire';

// The ask before work starts on a task whose repos need services that are not running (P697).
// `ask` opens the dialog and resolves true when the start may go on, false when it is cancelled or
// handed to the run dialog. One concern: that ask and its memory.
export const useAdeServicesGateStore = defineStore('adeServicesGate', () => {
  const runDialog = useScriptRunDialogStore();

  /** The task being asked about; `null` when the dialog is closed. */
  const request = ref<{ taskId: string } | null>(null);
  /** Tasks whose start went on without their services: not asked again until the app restarts. */
  const skipped = reactive(new Set<string>());
  let resolver: ((proceed: boolean) => void) | null = null;

  function close(proceed: boolean): void {
    const done = resolver;
    resolver = null;
    request.value = null;
    done?.(proceed);
  }

  function ask(taskId: string): Promise<boolean> {
    resolver?.(false); // a second ask replaces the open one
    request.value = { taskId };
    return new Promise<boolean>((resolve) => {
      resolver = resolve;
    });
  }

  /** Continue: the start goes on; a service still missing is not asked about again for this task. */
  function proceed(): void {
    const taskId = request.value?.taskId;
    if (!taskId) return;
    const live = queryClient.getQueryData<RequiredServices>(requiredServicesKey(taskId));
    if (live?.items.some(needsStart)) skipped.add(taskId);
    close(true);
  }

  function cancel(): void {
    close(false);
  }

  /** Closes the ask without starting work and opens the script's run dialog for the task. */
  function run(entry: RequiredService): void {
    const taskId = request.value?.taskId;
    if (!taskId) return;
    close(false);
    runDialog.open({
      scriptId: entry.scriptId,
      taskId,
      branchId: entry.branchId,
      prefill: entry.params,
      from: 'ade',
    });
  }

  return { request, skipped, ask, proceed, cancel, run };
});
