import type { SessionRestoreArgs, SessionRestoreState } from '@shared/domain/sessionRestore';
import { useMutation, useQuery } from '@tanstack/vue-query';
import { TIMING } from '@workbench/behaviour/timing';
import { queryClient } from '@workbench/state/queryClient';
import { control } from '../bridge/control';

// Server state for the restart dialog. The offer is per boot and Go pushes nothing for it, so the
// query refetches on mount and focus, and polls while the window is open with nothing ready yet.
const KEY = ['sessionRestore'] as const;

/** Whether the state holds anything the user can restore. */
export function hasReadyItem(s: SessionRestoreState): boolean {
  return (
    s.scripts.some((i) => i.status === 'ready') ||
    (s.docker.state === 'pending' && s.docker.items.some((i) => i.status === 'restorable')) ||
    s.agents.some((a) => a.state === 'ready')
  );
}

export function useSessionRestoreState() {
  return useQuery(
    {
      queryKey: KEY,
      queryFn: () => control.sessionRestoreState(),
      staleTime: 0,
      refetchOnMount: 'always',
      refetchOnWindowFocus: true,
      // A Docker engine that starts after Kira shows up within the offer window.
      refetchInterval: (q) => {
        const s = q.state.data;
        return s?.open && !hasReadyItem(s) ? TIMING.pollFast : false;
      },
    },
    queryClient,
  );
}

// Restored items leave the offers and the run, Docker and board lists gain them.
const settle = () =>
  Promise.all([
    queryClient.invalidateQueries({ queryKey: KEY }),
    queryClient.invalidateQueries({ queryKey: ['scriptRuns'] }),
    queryClient.invalidateQueries({ queryKey: ['docker-restore'] }),
    queryClient.invalidateQueries({ queryKey: ['adetask'] }),
  ]);

export function useClaimSessionRestore() {
  return useMutation(
    { mutationKey: [...KEY, 'claim'], mutationFn: () => control.sessionRestoreClaim() },
    queryClient,
  );
}

export function useRestoreSession() {
  return useMutation(
    {
      mutationKey: [...KEY, 'restore'],
      mutationFn: (args: SessionRestoreArgs) => control.sessionRestoreRestore(args),
      onSettled: settle,
    },
    queryClient,
  );
}

export function useCloseSessionRestore() {
  return useMutation(
    {
      mutationKey: [...KEY, 'close'],
      mutationFn: () => control.sessionRestoreClose(),
      onSettled: settle,
    },
    queryClient,
  );
}
