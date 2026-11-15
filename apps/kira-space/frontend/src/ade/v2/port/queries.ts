import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query';
import { computed } from 'vue';
import { control } from '../../../bridge/control';
import { agentContextKey } from '../context/queries';
import { agentSettingsKey } from '../settings/queries';
import type { ExportApplyArgs, ImportApplyArgs, RestoreArgs } from './types';

const agentPortKey = ['agentPort'] as const;
const backupsKey = ['agentPort', 'backups'] as const;

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

// An export takes backups, so the list reloads.
export function useExportApply() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (args: ExportApplyArgs) => control.agentPortExportApply(args),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: backupsKey }),
  });
}

export function usePortBackups(provider: () => string, enabled: () => boolean) {
  return useQuery({
    queryKey: computed(() => [...backupsKey, provider()]),
    queryFn: () => control.agentPortBackups({ provider: provider() }),
    enabled: computed(enabled),
  });
}

// A restore writes a native file and takes a backup of the one it replaces.
export function useRestore() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (args: RestoreArgs) => control.agentPortRestore(args),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: backupsKey }),
  });
}

export function useCancelPort() {
  return useMutation({ mutationFn: (token: string) => control.agentPortCancel({ token }) });
}
