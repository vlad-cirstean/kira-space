import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { useOpenChangeRequest } from '../../../hostview/queries';
import { useMarkTrackedSeen } from '../updates/queries';
import { useTaskUpdates } from '../updates/useTaskUpdates';

// Opens a branch's change request in the host view and marks its pending update seen.
export function useOpenBranchPr() {
  const { pending } = useTaskUpdates();
  const markSeen = useMarkTrackedSeen();
  const openView = useOpenChangeRequest();

  function open(taskId: string, branchId: string, url: string): void {
    const itemKey = `cr:${branchId}`;
    if (pending(taskId, itemKey)) markSeen.mutate({ taskId, itemKeys: [itemKey] });
    fireAndForget(openView.mutateAsync(url), 'hostView.open');
  }

  return { open };
}
