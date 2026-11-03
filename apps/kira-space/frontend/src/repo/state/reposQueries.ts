import { useMutation, useQuery } from '@tanstack/vue-query';
import { queryClient } from '@workbench/state/queryClient';
import type { FolderArgs, FolderHiddenArgs, PathArgs } from '../../ade/v2/wire';
import { control } from '../../bridge/control';

export const reposKey = ['adetask', 'repos'] as const;

export function useRepos() {
  return useQuery({
    queryKey: reposKey,
    queryFn: () => control.adeTaskRepos(),
  });
}

function invalidateRepos(): Promise<void> {
  return queryClient.invalidateQueries({ queryKey: reposKey, exact: true });
}

export function useAddFolder() {
  return useMutation({
    mutationFn: (args: FolderArgs) => control.adeTaskAddFolder(args),
    onSettled: invalidateRepos,
  });
}

export function useSetFolderWatch() {
  return useMutation({
    mutationFn: (args: FolderArgs) => control.adeTaskSetFolderWatch(args),
    onSettled: invalidateRepos,
  });
}

export function useSetFolderHidden() {
  return useMutation({
    mutationFn: (args: FolderHiddenArgs) => control.adeTaskSetFolderHidden(args),
    onSettled: invalidateRepos,
  });
}

export function useRemoveFolder() {
  return useMutation({
    mutationFn: (args: PathArgs) => control.adeTaskRemoveFolder(args),
    onSettled: invalidateRepos,
  });
}
