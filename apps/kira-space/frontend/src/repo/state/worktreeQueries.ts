import { orderWorktrees } from '@kira/git-core';
import type { WorktreeEntry } from '@kira/git-ipc';
import { useQueries } from '@tanstack/vue-query';
import { errorText } from '@workbench/bridge/codedError';
import { KEEP_WHILE_OPEN } from '@workbench/state/queryClient';
import { computed } from 'vue';
import { gitCall } from '../git/gitCall';

/** A repo row whose worktrees are expanded; `ready` settles once the window holds its repo. */
export interface ExpandedWorktreeRow {
  readonly codeRepoId: string;
  readonly gitRepoId: string;
  readonly ready: () => Promise<unknown>;
}

/** Keeps a key per expanded row; `worktrees.ts` removes it on collapse and invalidates it on `refsChanged`. */
export const worktreesQueryKey = (gitRepoId: string) =>
  ['git', 'worktree.list', gitRepoId] as const;

/** `worktree.list` of every expanded row. Earlier entries stay visible while a refetch runs. */
export function useRepoWorktreeLists(rows: () => readonly ExpandedWorktreeRow[]) {
  const results = useQueries({
    queries: computed(() =>
      rows().map((row) => ({
        queryKey: worktreesQueryKey(row.gitRepoId),
        gcTime: KEEP_WHILE_OPEN,
        queryFn: async ({ signal }: { signal: AbortSignal }): Promise<readonly WorktreeEntry[]> => {
          await row.ready();
          const { worktrees } = await gitCall('worktree.list', { repoId: row.gitRepoId }, signal);
          return worktrees;
        },
      })),
    ),
  });
  const resultOf = (codeRepoId: string) => {
    const index = rows().findIndex((row) => row.codeRepoId === codeRepoId);
    return index < 0 ? undefined : results.value[index];
  };
  return {
    entries: (codeRepoId: string): readonly WorktreeEntry[] =>
      orderWorktrees(resultOf(codeRepoId)?.data ?? []),
    loading: (codeRepoId: string): boolean => resultOf(codeRepoId)?.isFetching ?? false,
    error: (codeRepoId: string): string | null => {
      const error = resultOf(codeRepoId)?.error;
      return error ? errorText(error) : null;
    },
  };
}
