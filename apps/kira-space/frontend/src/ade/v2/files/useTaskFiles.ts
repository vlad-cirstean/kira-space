import type { FileListing, FileRead } from '@shared/domain/repo';
import { useQuery, useQueryClient } from '@tanstack/vue-query';
import { type MaybeRefOrGetter, toValue } from 'vue';
import { control } from '../../../bridge/control';

const filesKey = (branchId: string) => ['adetask', 'files', branchId] as const;

/** The branch worktree's tracked and untracked files. Refetches on mount and on Refresh. */
export function useTaskFilesListing(branchId: MaybeRefOrGetter<string>) {
  return useQuery<FileListing>(() => {
    const id = toValue(branchId);
    return {
      queryKey: [...filesKey(id), 'list'],
      queryFn: () => control.adeTaskFiles({ branchId: id }),
      enabled: id !== '',
    };
  });
}

/** One worktree file. A refetch sends the previous stat; an unchanged answer keeps the prior text. */
export function useTaskFileRead(
  branchId: MaybeRefOrGetter<string>,
  path: MaybeRefOrGetter<string>,
) {
  const queryClient = useQueryClient();
  return useQuery<FileRead>(() => {
    const id = toValue(branchId);
    const p = toValue(path);
    const queryKey = [...filesKey(id), 'read', p];
    return {
      queryKey,
      queryFn: async () => {
        const prev = queryClient.getQueryData<FileRead>(queryKey);
        const ifStat = prev?.kind === 'found' ? prev.stat : undefined;
        const next = await control.adeTaskFileRead(id, p, ifStat);
        return next.kind === 'unchanged' && prev ? prev : next;
      },
      enabled: id !== '' && p !== '',
      // A remount re-reads: the editor is rebuilt each time, so nothing is kept past unmount.
      gcTime: 0,
    };
  });
}

/** Editor registry id of one worktree file's viewer; shared by the viewer and a search reveal. */
export const taskFileTabId = (branchId: string, path: string): string =>
  `ade-file:${branchId}:${path}`;
