import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query';
import { control } from '../../../bridge/control';
import type { AgentContextSaveArgs } from './types';

export const agentContextKey = ['agentContext'] as const;

export function useAgentContext() {
  return useQuery({ queryKey: agentContextKey, queryFn: () => control.agentContextGet() });
}

export function useSaveAgentContext() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (args: AgentContextSaveArgs) => control.agentContextSave(args),
    onSuccess: (view) => queryClient.setQueryData(agentContextKey, view),
  });
}
