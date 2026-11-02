import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query';
import { control } from '../../../bridge/control';
import type { AgentSetSecretArgs, AgentSettingsSaveArgs } from './types';

export const agentSettingsKey = ['agentSettings'] as const;

export function useAgentSettings() {
  return useQuery({ queryKey: agentSettingsKey, queryFn: () => control.agentSettingsGet() });
}

export function useSaveAgentSettings() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (args: AgentSettingsSaveArgs) => control.agentSettingsSave(args),
    onSuccess: (view) => queryClient.setQueryData(agentSettingsKey, view),
  });
}

export function useSetAgentSecret() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (args: AgentSetSecretArgs) => control.agentSettingsSetSecret(args),
    onSuccess: (view) => queryClient.setQueryData(agentSettingsKey, view),
  });
}

export function useDeleteAgentSecret() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => control.agentSettingsDeleteSecret(name),
    onSuccess: (view) => queryClient.setQueryData(agentSettingsKey, view),
  });
}
