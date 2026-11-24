import type {
  ProofDetail,
  ProofShotClipboard,
  ProofShotLocation,
  ProofStepDetail,
  ProofViewState,
  ProofViewStateEntry,
} from '@shared/domain/proof';
import { type UseQueryReturnType, useMutation, useQuery } from '@tanstack/vue-query';
import { t } from '@theme/i18n';
import { announceApp } from '@workbench/behaviour/liveRegion';
import { toCodedError } from '@workbench/bridge/codedError';
import { queryClient } from '@workbench/state/queryClient';
import { computed, type MaybeRefOrGetter, onUnmounted, toValue } from 'vue';
import { control } from '../bridge/control';
import { useProofWindowStore } from './state';

// Flat `[domain, ...ids]` keys (workbench/state/queryClient.ts). The window refetches only on Reload
// or its own accept and reject; `kira:proof:changed` only checks for staleness.
const proofKey = {
  all: ['proof'] as const,
  session: (id: string) => ['proof', 'session', id] as const,
  latest: (id: string) => ['proof', 'latest', id] as const,
  step: (id: string, rev: number, ordinal: number) => ['proof', 'step', id, rev, ordinal] as const,
  shotLocation: ['proof', 'shotLocation'] as const,
  shotClipboard: ['proof', 'shotClipboard'] as const,
  viewState: (id: string, rev: number) => ['proof', 'viewState', id, rev] as const,
};

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

export function useShotLocation() {
  return useQuery<ProofShotLocation>({
    queryKey: proofKey.shotLocation,
    queryFn: () => control.proofShotLocation(),
  });
}

/** Whether the system clipboard takes several images; fixed for the life of the app (default staleTime is Infinity). */
export function useShotClipboard() {
  return useQuery<ProofShotClipboard>({
    queryKey: proofKey.shotClipboard,
    queryFn: () => control.proofShotClipboard(),
  });
}

/** Opens the folder picker; a cancel leaves the cached location as is. */
export function useChooseShotLocation() {
  return useMutation({
    mutationFn: () => control.proofChooseShotLocation(),
    onSuccess: (result) => {
      if (!result.canceled) queryClient.setQueryData(proofKey.shotLocation, result.location);
    },
  });
}

export function useResetShotLocation() {
  return useMutation({
    mutationFn: () => control.proofResetShotLocation(),
    onSuccess: (location) => queryClient.setQueryData(proofKey.shotLocation, location),
  });
}

function refreshProof(): Promise<void> {
  return queryClient.invalidateQueries({
    queryKey: proofKey.all,
    predicate: (q) => q.queryKey[1] !== 'viewState' && q.queryKey[1] !== 'shotClipboard',
  });
}

export function useDeleteProof() {
  return useMutation({
    mutationFn: (sessionId: string) => control.proofDelete(sessionId),
    onSuccess: (_void, sessionId) => {
      queryClient.removeQueries({ queryKey: proofKey.session(sessionId) });
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

export type ProofStaleState = 'changed' | 'removed';

/** Tells whether the proof changed or vanished after the window loaded it. A `kira:proof:changed`
 *  naming this session reads the proof once more into a disabled query; a differing `version`, or
 *  not found, marks the window stale. Nothing reloads by itself: `reload` does. */
export function useProofStale(
  id: MaybeRefOrGetter<string | null>,
  shown: UseQueryReturnType<ProofDetail, Error>,
) {
  const latest = useQuery<ProofDetail>({
    queryKey: () => proofKey.latest(toValue(id) ?? ''),
    queryFn: () => control.proofSession(toValue(id) as string),
    enabled: false,
  });
  const off = control.onProofChanged((e) => {
    const mine = toValue(id);
    if (mine === null || (e.sessionId !== '' && e.sessionId !== mine)) return;
    void latest.refetch();
  });
  onUnmounted(off);
  const state = computed<ProofStaleState | null>(() => {
    if (shown.isFetching.value || latest.isFetching.value) return null;
    if (latest.error.value && toCodedError(latest.error.value).code === 'E_NOT_FOUND') {
      return 'removed';
    }
    const a = latest.data.value;
    const b = shown.data.value;
    return a && b && a.version !== b.version ? 'changed' : null;
  });
  return { state, reload: refreshProof };
}
