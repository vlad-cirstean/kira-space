import { useMutation, useQuery } from '@tanstack/vue-query';
import { tryOnScopeDispose } from '@vueuse/core';
import { queryClient } from '@workbench/state/queryClient';
import { type MaybeRefOrGetter, toValue } from 'vue';
import { control } from '../bridge/control';

// Kept variables are server state held in the backend's memory. The push from the store (`kira:kept:changed`)
// is the only signal that the set moved, so each observer listens while it lives.
const KEPT_KEY = ['keptVars'] as const;

export function useKeptVars() {
  const off = control.onKeptChanged(
    () => void queryClient.invalidateQueries({ queryKey: KEPT_KEY }),
  );
  tryOnScopeDispose(off);
  return useQuery(
    () => ({ queryKey: [...KEPT_KEY, 'list'], queryFn: () => control.keptList() }),
    queryClient,
  );
}

/** The full value, fetched only once `enabled` turns on. Not cached past the view: a value is read on demand. */
export function useKeptValue(id: MaybeRefOrGetter<string>, enabled: MaybeRefOrGetter<boolean>) {
  return useQuery(
    () => ({
      queryKey: [...KEPT_KEY, 'value', toValue(id)],
      queryFn: () => control.keptValue(toValue(id)),
      enabled: toValue(enabled),
      gcTime: 0,
    }),
    queryClient,
  );
}

export function useClearKept() {
  return useMutation(
    {
      mutationFn: (ids: string[] | 'all') =>
        ids === 'all' ? control.keptClearAll() : control.keptClear(ids),
      onSettled: () => queryClient.invalidateQueries({ queryKey: KEPT_KEY }),
    },
    queryClient,
  );
}
