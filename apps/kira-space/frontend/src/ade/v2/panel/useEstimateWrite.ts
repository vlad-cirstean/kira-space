import { useMutationAction } from '@workbench/behaviour/asyncAction';
import { type MaybeRefOrGetter, toValue } from 'vue';
import { taskPatch } from '../board/panelFacts';
import { useUpdateTask } from '../queries';

// Estimate write shared by the Task tab row and the plan card chip.
export function useEstimateWrite(taskId: MaybeRefOrGetter<string>) {
  const writer = useMutationAction(useUpdateTask());

  async function save(value: string): Promise<void> {
    await writer.run({ taskId: toValue(taskId), patch: taskPatch({ est: value }) });
  }

  return { error: writer.error, save };
}
