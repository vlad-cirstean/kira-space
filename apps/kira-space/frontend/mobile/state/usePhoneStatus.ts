import { type PhoneRun, phoneRunSchema } from '@shared/domain/mobile';
import { CHANNEL } from '@shared/protocol/events';
import { type QueryClient, useQuery } from '@tanstack/vue-query';
import { useDocumentVisibility } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { onChannel } from '../api/events';
import { getPhoneComputer, getPhoneRuns } from '../api/status';

// Script runs and the computer summary: the two reads the board's pushes do not cover. Runs are
// push-fed; Docker has no push (its events are per desktop window), so it polls.

export const phoneKey = ['phone'] as const;
export const phoneRunsKey = [...phoneKey, 'runs'] as const;
export const phoneComputerKey = [...phoneKey, 'computer'] as const;

const RUN_LIMIT = 30;

/** Merges each pushed run into the cached list, newest first, so a run flips state without a
 *  refetch. Without a cached list the push is dropped: the first fetch will include it. */
export function installPhoneReadSignals(queryClient: QueryClient): void {
  onChannel(CHANNEL.scriptRunsChanged, (data) => {
    const parsed = phoneRunSchema.safeParse(data);
    if (!parsed.success) return;
    const run = parsed.data;
    queryClient.setQueryData<PhoneRun[]>(phoneRunsKey, (prev) => {
      if (!prev) return prev;
      const rest = prev.filter((r) => r.id !== run.id);
      return [run, ...rest].sort((a, b) => b.createdAt - a.createdAt).slice(0, RUN_LIMIT);
    });
  });
}

export function usePhoneRuns() {
  return useQuery({ queryKey: phoneRunsKey, queryFn: getPhoneRuns });
}

/** Polls only while a consumer is mounted and the page is visible. */
export function useComputer() {
  const visibility = useDocumentVisibility();
  return useQuery({
    queryKey: phoneComputerKey,
    queryFn: getPhoneComputer,
    refetchInterval: () => (visibility.value === 'visible' ? TIMING.pollMedium : false),
  });
}

const DAY_MS = 86_400_000;

/** A run that wants the user: waiting or blocked on them, or failed within the last day. */
export function runNeedsYou(run: PhoneRun, nowMs: number): boolean {
  if (run.state === 'blocked' || run.state === 'waiting') return true;
  return run.state === 'failed' && nowMs - (run.finishedAt ?? run.createdAt) < DAY_MS;
}
