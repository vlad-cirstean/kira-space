import type {
  ProofDetail,
  ProofStepDetail,
  ProofSummary,
  ProofViewState,
  ProofViewStateEntry,
} from '@shared/domain/proof';
import { useMutation, useQuery } from '@tanstack/vue-query';
import { t } from '@theme/i18n';
import { announceApp } from '@workbench/behaviour/liveRegion';
import { queryClient } from '@workbench/state/queryClient';
import { type MaybeRefOrGetter, onUnmounted, toValue } from 'vue';
import { control } from '../bridge/control';
import { useProofWindowStore } from './state';

// Flat `[domain, ...ids]` keys (workbench/state/queryClient.ts). Go pushes `kira:proof:changed`,
// so every read stays fresh until the broadcast invalidates it.
const proofKey = {
  all: ['proof'] as const,
  sessions: ['proof', 'sessions'] as const,
  session: (id: string) => ['proof', 'session', id] as const,
  step: (id: string, rev: number, ordinal: number) => ['proof', 'step', id, rev, ordinal] as const,
  viewState: (id: string, rev: number) => ['proof', 'viewState', id, rev] as const,
};

export function useProofSessions() {
  return useQuery<ProofSummary[]>({
    queryKey: proofKey.sessions,
    queryFn: () => control.proofSessions(),
  });
}

export function useProofSession(id: MaybeRefOrGetter<string | null>) {
  return useQuery<ProofDetail>({
    queryKey: () => proofKey.session(toValue(id) ?? ''),
    queryFn: () => control.proofSession(toValue(id) as string),
    enabled: () => toValue(id) !== null,
  });
}

export function useProofStep(
  id: MaybeRefOrGetter<string>,
  rev: MaybeRefOrGetter<number>,
  ordinal: MaybeRefOrGetter<number>,
) {
  return useQuery<ProofStepDetail>({
    queryKey: () => proofKey.step(toValue(id), toValue(rev), toValue(ordinal)),
    queryFn: () => control.proofStep(toValue(id), toValue(rev), toValue(ordinal)),
  });
}

export function useProofViewStates(id: MaybeRefOrGetter<string>, rev: MaybeRefOrGetter<number>) {
  return useQuery<ProofViewStateEntry[]>({
    queryKey: () => proofKey.viewState(toValue(id), toValue(rev)),
    queryFn: () => control.proofViewState(toValue(id), toValue(rev)),
  });
}

/** Writes one step's view state: the cache updates at once so a remount restores it; the
 *  backend write is the caller's debounced step. */
export function cacheViewState(
  sessionId: string,
  rev: number,
  ordinal: number,
  state: ProofViewState,
): void {
  queryClient.setQueryData<ProofViewStateEntry[]>(proofKey.viewState(sessionId, rev), (old) => [
    ...(old ?? []).filter((e) => e.ordinal !== ordinal),
    { ordinal, state },
  ]);
}

function refreshProof(): Promise<void> {
  return queryClient.invalidateQueries({
    queryKey: proofKey.all,
    predicate: (q) => q.queryKey[1] !== 'viewState',
  });
}

export function useDeleteProof() {
  return useMutation({
    mutationFn: (sessionId: string) => control.proofDelete(sessionId),
    onSuccess: (_void, sessionId) => {
      queryClient.removeQueries({ queryKey: proofKey.session(sessionId) });
      return queryClient.invalidateQueries({ queryKey: proofKey.sessions });
    },
  });
}

/** Accept or reject the pending edit; both land back on the (new) current revision. */
export function useReviewEdit() {
  const store = useProofWindowStore();
  const settle = async (message: string): Promise<void> => {
    store.side = null;
    store.compare = false;
    store.activeId = null;
    await refreshProof();
    announceApp(message);
  };
  const accept = useMutation({
    mutationFn: (sessionId: string) => control.proofAcceptEdit(sessionId),
    onSuccess: () => settle(t('studio.proof.change.accepted.announce')),
  });
  const reject = useMutation({
    mutationFn: (sessionId: string) => control.proofRejectEdit(sessionId),
    onSuccess: () => settle(t('studio.proof.change.rejected.announce')),
  });
  return { accept, reject };
}

/** Go broadcasts: refetch whatever changed, and follow a `select` push to a session. */
export function useProofSync(): void {
  const store = useProofWindowStore();
  const offs = [
    control.onProofChanged(() => {
      void refreshProof();
    }),
    control.onProofSelect(({ sessionId }) => {
      store.selectSession(sessionId);
    }),
  ];
  onUnmounted(() => {
    for (const off of offs) off();
  });
}
