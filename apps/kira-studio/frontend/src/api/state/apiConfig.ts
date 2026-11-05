import type { ApiConfigView } from '@shared/domain/apiConfig';
import { useMutation, useQuery } from '@tanstack/vue-query';
import { queryClient } from '@workbench/state/queryClient';
import { control } from '../../bridge/control';
import { apiAgentAccessQueryOptions, apiConfigKey, apiConfigQueryOptions } from './apiQueries';

// The Api config files' validity and the secret names they reference. Never carries a value.
export function useApiConfig() {
  return useQuery(apiConfigQueryOptions(), queryClient);
}

// The environments that share variables with agents: names and allowed hosts, never a value.
export function useApiAgentAccess() {
  return useQuery(apiAgentAccessQueryOptions(), queryClient);
}

export function useSetApiSecret() {
  return useMutation(
    {
      mutationFn: (args: { name: string; value: string }) =>
        control.apiConfigSetSecret(args.name, args.value),
      onSuccess: (view: ApiConfigView) => queryClient.setQueryData(apiConfigKey, view),
    },
    queryClient,
  );
}

export function useDeleteApiSecret() {
  return useMutation(
    {
      mutationFn: (name: string) => control.apiConfigDeleteSecret(name),
      onSuccess: (view: ApiConfigView) => queryClient.setQueryData(apiConfigKey, view),
    },
    queryClient,
  );
}
