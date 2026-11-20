import { useQuery } from '@tanstack/vue-query';
import { tryOnScopeDispose } from '@vueuse/core';
import { queryClient } from '@workbench/state/queryClient';
import { type MaybeRefOrGetter, toValue } from 'vue';
import { control } from '../../../bridge/control';
import type { RequiredService, RequiredServices } from '../wire';

// The repo services a task needs and whether Kira runs each (P697). The status is computed per call
// and goes stale with any script run or repo file change, so those two pushes invalidate it.
const prefix = ['adetask', 'requiredServices'] as const;
export const requiredServicesKey = (taskId: string) => [...prefix, taskId] as const;
const options = (taskId: string) => ({
  queryKey: requiredServicesKey(taskId),
  queryFn: (): Promise<RequiredServices> => control.adeTaskRequiredServices({ taskId }),
  staleTime: 0,
});

/** A service that needs the user: not running, or a reference that cannot run. */
export const needsStart = (s: RequiredService): boolean =>
  s.state === 'stopped' || s.state === 'invalid';

/** A fresh read for the start gate; never the cache. */
export function fetchRequiredServices(taskId: string): Promise<RequiredServices> {
  return queryClient.fetchQuery(options(taskId));
}

/** The open gate dialog's live rows. */
export function useRequiredServices(taskId: MaybeRefOrGetter<string>) {
  const refresh = (): void => void queryClient.invalidateQueries({ queryKey: prefix });
  const offs = [control.onScriptRunsChanged(refresh), control.onRepoConfigChanged(refresh)];
  tryOnScopeDispose(() => {
    for (const off of offs) off();
  });
  return useQuery(() => ({
    ...options(toValue(taskId)),
    enabled: toValue(taskId) !== '',
  }));
}
