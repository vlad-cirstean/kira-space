import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query';
import { control } from '../../../bridge/control';
import { agentContextKey } from '../context/queries';
import { agentSettingsKey } from '../settings/queries';
import type { ExportApplyArgs, ImportApplyArgs } from './types';

export const agentPortKey = ['agentPort'] as const;

export function usePortProviders() {
  return useQuery({ queryKey: agentPortKey, queryFn: () => control.agentPortList() });
}

export function useImportPreview() {
  return useMutation({
    mutationFn: (provider: string) => control.agentPortImportPreview({ provider }),
  });
}

export function useExportPreview() {
  return useMutation({
    mutationFn: (provider: string) => control.agentPortExportPreview({ provider }),
  });
}

// An apply changes the shared settings and context, so both reload.
function useRefresh() {
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: agentSettingsKey }),
      queryClient.invalidateQueries({ queryKey: agentContextKey }),
    ]);
}

export function useImportApply() {
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (args: ImportApplyArgs) => control.agentPortImportApply(args),
    onSuccess: refresh,
  });
}

export function useExportApply() {
  return useMutation({ mutationFn: (args: ExportApplyArgs) => control.agentPortExportApply(args) });
}

export function useCancelPort() {
  return useMutation({ mutationFn: (token: string) => control.agentPortCancel({ token }) });
}
