import { errorText } from '@workbench/bridge/codedError';
import { type MaybeRefOrGetter, ref, toValue } from 'vue';
import { taskPatch } from '../board/panelFacts';
import { useUpdateTask } from '../queries';

// Estimate write shared by the Task tab row and the plan card chip.
export function useEstimateWrite(taskId: MaybeRefOrGetter<string>) {
  const update = useUpdateTask();
  const error = ref<string | null>(null);

  async function save(value: string): Promise<void> {
    error.value = null;
    try {
      await update.mutateAsync({ taskId: toValue(taskId), patch: taskPatch({ est: value }) });
    } catch (err) {
      error.value = errorText(err);
    }
  }

  return { error, save };
}
