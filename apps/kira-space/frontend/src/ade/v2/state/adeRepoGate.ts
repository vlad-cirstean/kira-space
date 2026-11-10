import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import { useAddTaskRepo } from '../queries';

// The repo choice before work starts on a task with no repo (P598). `ask` opens the dialog and
// resolves true once at least one repo is on the task, false when the person cancels.
export const useAdeRepoGateStore = defineStore('adeRepoGate', () => {
  const addRepo = useAddTaskRepo();

  /** The task being asked about; `null` when the dialog is closed. */
  const request = ref<{ taskId: string } | null>(null);
  let resolver: ((chosen: boolean) => void) | null = null;
  // Repos added by an attempt that failed halfway: a retry must not add them twice.
  let added = new Set<string>();

  function close(chosen: boolean): void {
    const done = resolver;
    resolver = null;
    request.value = null;
    adding.error.value = null;
    done?.(chosen);
  }

  function ask(taskId: string): Promise<boolean> {
    resolver?.(false); // a second ask replaces the open one
    added = new Set();
    adding.error.value = null;
    request.value = { taskId };
    return new Promise<boolean>((resolve) => {
      resolver = resolve;
    });
  }

  // Adds each repo to the task in order; a failure keeps the dialog open with its message.
  const adding = useAsyncAction(async (codeRepoIds: readonly string[]) => {
    const taskId = request.value?.taskId;
    if (!taskId) return;
    for (const codeRepoId of codeRepoIds) {
      if (added.has(codeRepoId)) continue;
      await addRepo.mutateAsync({ taskId, codeRepoId, base: null });
      added.add(codeRepoId);
    }
    close(true);
  });
  const error = computed(() => adding.error.value ?? '');
  const pending = adding.pending;

  function confirm(codeRepoIds: readonly string[]): void {
    if (pending.value || codeRepoIds.length === 0) return;
    void adding.run(codeRepoIds);
  }

  function cancel(): void {
    if (!pending.value) close(false);
  }

  return { request, error, pending, ask, confirm, cancel };
});
