import { useMutation, useQuery } from '@tanstack/vue-query';
import { queryClient } from '@workbench/state/queryClient';
import type { MaybeRefOrGetter } from 'vue';
import { toValue } from 'vue';
import { control } from '../../../bridge/control';
import { artifactsPrefix, diskPrefix, handoversPrefix } from '../queries';
import type { DiskResolveArgs } from '../wire';

// The task folder against what the app holds. One stat pass per open, window focus, Refresh and board push.
// Kira applies a live agent's edits on save, so a push follows them; a user's edits show on the next trigger.

export function useDiskChanges(
  taskId: MaybeRefOrGetter<string>,
  enabled?: MaybeRefOrGetter<boolean>,
) {
  return useQuery({
    queryKey: [...diskPrefix, taskId] as const,
    queryFn: () => control.adeTaskDetailsDiskChanges({ taskId: toValue(taskId) }),
    enabled: () => toValue(enabled) ?? true,
    staleTime: 0,
    refetchOnWindowFocus: true,
  });
}

export const refreshDisk = () => queryClient.invalidateQueries({ queryKey: diskPrefix });

function settled() {
  return Promise.all([
    refreshDisk(),
    queryClient.invalidateQueries({ queryKey: handoversPrefix }),
    queryClient.invalidateQueries({ queryKey: artifactsPrefix }),
  ]);
}

export function useAcceptDisk() {
  return useMutation({
    mutationFn: (args: DiskResolveArgs) => control.adeTaskDetailsAcceptDisk(args),
    onSuccess: settled,
  });
}

export function useDiscardDisk() {
  return useMutation({
    mutationFn: (args: DiskResolveArgs) => control.adeTaskDetailsDiscardDisk(args),
    onSuccess: settled,
  });
}
