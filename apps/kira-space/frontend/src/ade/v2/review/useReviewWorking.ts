import type { AgentEvent } from '@shared/domain/agent';
import { queryOptions, useMutation, useQuery } from '@tanstack/vue-query';
import { tryOnScopeDispose, useDebounceFn, useWindowFocus } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { queryClient } from '@workbench/state/queryClient';
import { type MaybeRefOrGetter, toValue, watch } from 'vue';
import { control } from '../../../bridge/control';
import { useBoard } from '../readQueries';
import type { Board, ReviewWindowTarget, ReviewWorkingMarkArgs } from '../wire';

// The uncommitted files of a review window's branch worktree, and each file's diff. List and diffs
// share one key prefix so a single invalidation re-reads them all.
const reviewWorkingPrefix = ['adetask', 'reviewWorking'] as const;

const reviewWorkingKey = (branchId: string) => [...reviewWorkingPrefix, branchId] as const;

/** Re-reads the uncommitted list and every open diff; a refused write calls it to show the disk. */
export function invalidateReviewWorking(): Promise<void> {
  return queryClient.invalidateQueries({ queryKey: reviewWorkingPrefix });
}

export function reviewWorkingDiffOptions(branchId: string, path: string) {
  return queryOptions({
    queryKey: [...reviewWorkingKey(branchId), 'diff', path] as const,
    queryFn: () => control.adeTaskReviewWorkingDiff({ branchId, path }),
    staleTime: 0,
  });
}

export function useReviewWorking(branchId: MaybeRefOrGetter<string>) {
  return useQuery({
    queryKey: [...reviewWorkingPrefix, branchId] as const,
    queryFn: () => control.adeTaskReviewWorking({ branchId: toValue(branchId) }),
  });
}

export function useReviewWorkingDiff(
  branchId: MaybeRefOrGetter<string>,
  path: MaybeRefOrGetter<string>,
) {
  return useQuery({
    queryKey: [...reviewWorkingPrefix, branchId, 'diff', path] as const,
    queryFn: () =>
      control.adeTaskReviewWorkingDiff({ branchId: toValue(branchId), path: toValue(path) }),
    enabled: () => toValue(branchId) !== '',
  });
}

/** Marks uncommitted files reviewed or not; the list and every diff re-read afterwards. */
export function useReviewWorkingMark() {
  return useMutation({
    mutationFn: (args: ReviewWorkingMarkArgs) => control.adeTaskReviewWorkingMark(args),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: reviewWorkingPrefix }),
  });
}

/** Most paths one stamp covers; the server refuses more. */
const MAX_STAMP_PATHS = 5000;

/** Polls a stat stamp of the listed files; a changed stamp re-reads the uncommitted list, so an edit
 *  no agent hook announced still shows. Outside `reviewWorkingPrefix`: a list invalidation never
 *  refetches it, and a refetched list over the same paths yields the same stamp. */
export function useReviewWorkingStamp(
  branchId: MaybeRefOrGetter<string>,
  paths: MaybeRefOrGetter<readonly string[]>,
  enabled: MaybeRefOrGetter<boolean>,
): void {
  const stamp = useQuery({
    queryKey: ['adetask', 'reviewStamp', branchId] as const,
    queryFn: () =>
      control.adeTaskReviewWorkingStamp({
        branchId: toValue(branchId),
        paths: toValue(paths).slice(0, MAX_STAMP_PATHS),
      }),
    refetchInterval: TIMING.reviewStamp,
    enabled: () => toValue(branchId) !== '' && toValue(enabled),
  });
  watch(
    () => stamp.data.value?.stamp,
    (now, was) => {
      if (was !== undefined && now !== was) void invalidateReviewWorking();
    },
  );
}

/** Agent hook events that follow a file write or the end of a turn. */
const SETTLE_EVENTS = new Set(['PostToolUse', 'Stop']);

function isInside(cwd: string, root: string): boolean {
  return root !== '' && (cwd === root || cwd.startsWith(`${root}/`));
}

/** Tip and dirty paths of the window's branch: the part of a board push that moves its uncommitted list. */
function branchSignature(board: Board | undefined, branchId: string | undefined): string | null {
  const branch = board?.branches.find((b) => b.id === branchId);
  if (!branch) return null;
  return `${branch.tip}\n${branch.dirty.map((d) => `${d.code} ${d.path}`).join('\n')}`;
}

/** Keeps the uncommitted queries fresh: a change to this branch's tip or dirty paths (commit, stage,
 *  checkout), window focus, and agent activity inside the worktree. Other tasks' board pushes are
 *  ignored. Edits no hook announces are caught by `useReviewWorkingStamp`. */
export function useReviewWorkingSync(target: () => ReviewWindowTarget | null): void {
  const board = useBoard();
  const invalidate = (): void => {
    void invalidateReviewWorking();
  };
  const invalidateSoon = useDebounceFn(invalidate, TIMING.fileSettle);
  const focused = useWindowFocus();
  watch(focused, (now, was) => {
    if (now && !was) invalidate();
  });
  watch(
    () => branchSignature(board.data.value, target()?.branchId),
    (now, was) => {
      if (was !== null && now !== was) void invalidateSoon();
    },
  );
  const stops = [
    control.onAgentEvent((event: AgentEvent) => {
      const root = target()?.worktree ?? '';
      if (SETTLE_EVENTS.has(event.event) && isInside(event.cwd, root)) void invalidateSoon();
    }),
  ];
  tryOnScopeDispose(() => {
    for (const stop of stops) stop();
  });
}
