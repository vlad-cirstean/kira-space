import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { control } from '../../../bridge/control';
import { useMarkTrackedSeen } from '../updates/queries';
import { useTaskUpdates } from '../updates/useTaskUpdates';

// Opens a branch's change request in the browser and marks its pending update seen.
export function useOpenBranchPr() {
  const { pending } = useTaskUpdates();
  const markSeen = useMarkTrackedSeen();

  function open(taskId: string, branchId: string, url: string): void {
    const itemKey = `cr:${branchId}`;
    if (pending(taskId, itemKey)) markSeen.mutate({ taskId, itemKeys: [itemKey] });
    fireAndForget(control.openExternal(url), 'ade.branch');
  }

  return { open };
}
