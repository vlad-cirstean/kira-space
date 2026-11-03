import { useMutation, useQuery } from '@tanstack/vue-query';
import { queryClient } from '@workbench/state/queryClient';
import { type MaybeRefOrGetter, toValue } from 'vue';
import { control } from '../../bridge/control';
import type { RepoConfigFileArgs, RepoConfigSaveArgs } from './repoConfigTypes';

export const repoConfigKey = ['repoConfig'] as const;
const repoConfigListKey = [...repoConfigKey, 'list'] as const;

const fileKey = (args: RepoConfigFileArgs) => [...repoConfigKey, 'file', args.file, args.path];

export function useRepoConfigList() {
  return useQuery({ queryKey: repoConfigListKey, queryFn: () => control.repoConfigList() });
}

export function useRepoConfigFile(args: MaybeRefOrGetter<RepoConfigFileArgs>) {
  return useQuery({
    queryKey: () => fileKey(toValue(args)),
    queryFn: () => control.repoConfigFile(toValue(args)),
  });
}

export function useSaveRepoConfigFile() {
  return useMutation({
    mutationFn: (args: RepoConfigSaveArgs) => control.repoConfigSaveFile(args),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: repoConfigKey }),
  });
}
