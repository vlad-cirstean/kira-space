import { useMutation, useQuery } from '@tanstack/vue-query';
import { optimisticUpdate } from '@workbench/behaviour/optimistic';
import { queryClient } from '@workbench/state/queryClient';
import { control } from '../../../bridge/control';
import type { MarkTrackedSeenArgs, TrackedUpdatesResult } from '../wire';

export const trackedUpdatesKey = ['adetask', 'updates'] as const;

export function useTrackedUpdates() {
  return useQuery({
    queryKey: trackedUpdatesKey,
    queryFn: () => control.adeTaskTrackedUpdates(),
  });
}

function withoutSeen(prev: TrackedUpdatesResult, args: MarkTrackedSeenArgs): TrackedUpdatesResult {
  return {
    updates: prev.updates.filter(
      (u) =>
        u.taskId !== args.taskId ||
        (args.itemKeys.length > 0 && !args.itemKeys.includes(u.itemKey)),
    ),
  };
}

export function useMarkTrackedSeen() {
  return useMutation({
    mutationFn: (args: MarkTrackedSeenArgs) => control.adeTaskMarkTrackedSeen(args),
    ...optimisticUpdate<TrackedUpdatesResult, MarkTrackedSeenArgs>(
      queryClient,
      trackedUpdatesKey,
      withoutSeen,
      { invalidate: 'settled' },
    ),
  });
}
