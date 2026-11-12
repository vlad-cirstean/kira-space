import { useMutation, useQuery } from '@tanstack/vue-query';
import { queryClient } from '@workbench/state/queryClient';
import { control } from '../../../bridge/control';
import type { LeaseQueueArgs, LeaseQueuesArgs } from '../wire';

export const leasesKey = ['ade', 'leases'] as const;

/** Every lease queue with its holder and waiters; the `adeLeases` push invalidates it. */
export function useLeases() {
  return useQuery({
    queryKey: leasesKey,
    queryFn: () => control.leasesSnapshot(),
  });
}

const settled = {
  onSettled: () => queryClient.invalidateQueries({ queryKey: leasesKey, exact: true }),
};

export function useSaveLeaseQueues() {
  return useMutation({
    mutationFn: (args: LeaseQueuesArgs) => control.leasesSaveQueues(args),
    ...settled,
  });
}

export function useRequestLeaseRelease() {
  return useMutation({
    mutationFn: (args: LeaseQueueArgs) => control.leasesRequestRelease(args),
    ...settled,
  });
}

export function useForceLeaseRelease() {
  return useMutation({
    mutationFn: (args: LeaseQueueArgs) => control.leasesForceRelease(args),
    ...settled,
  });
}
