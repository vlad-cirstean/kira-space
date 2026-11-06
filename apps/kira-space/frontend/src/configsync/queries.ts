import { type QueryClient, useMutation, useQuery, useQueryClient } from '@tanstack/vue-query';
import { onScopeDispose } from 'vue';
import { control } from '../bridge/control';
import type { ConfigSyncChoice, ConfigSyncStatus } from './domain';

export const configSyncStatusKey = ['configSync', 'status'] as const;
export const configSyncPreviewKey = ['configSync', 'preview'] as const;
export const configSyncWindowKey = (windowKey: string) =>
  ['configSync', 'window', windowKey] as const;

const setStatus = (qc: QueryClient) => (status: ConfigSyncStatus) =>
  qc.setQueryData(configSyncStatusKey, status);

// The status follows the backend: every change arrives as an event, so the query never polls.
export function useConfigSyncStatus() {
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: configSyncStatusKey,
    queryFn: () => control.configSyncStatus(),
  });
  onScopeDispose(control.onConfigSyncChanged(setStatus(qc)));
  return query;
}

// Mutations that return the new status write it straight into the cache.
export function useConfigSyncMutations() {
  const qc = useQueryClient();
  const onSuccess = setStatus(qc);
  return {
    setRepo: useMutation({
      mutationFn: (a: { url: string; branch: string; windowKey: string }) =>
        control.configSyncSetRepo(a),
      onSuccess,
    }),
    removeRepo: useMutation({ mutationFn: () => control.configSyncRemoveRepo(), onSuccess }),
    setFetchMinutes: useMutation({
      mutationFn: (minutes: number) => control.configSyncSetFetchMinutes(minutes),
      onSuccess,
    }),
    setMode: useMutation({
      mutationFn: (a: { module: string; mode: string }) =>
        control.configSyncSetMode(a.module, a.mode),
      onSuccess,
    }),
    setDeactivated: useMutation({
      mutationFn: (a: { module: string; unit: string; off: boolean }) =>
        control.configSyncSetDeactivated(a.module, a.unit, a.off),
      onSuccess: (status: ConfigSyncStatus) => {
        onSuccess(status);
        void qc.invalidateQueries({ queryKey: configSyncPreviewKey });
      },
    }),
    fetchNow: useMutation({ mutationFn: () => control.configSyncFetchNow(), onSuccess }),
    dismiss: useMutation({
      mutationFn: (sha: string) => control.configSyncDismiss(sha),
      onSuccess,
    }),
    apply: useMutation({
      mutationFn: (a: { token: string; choices: Record<string, ConfigSyncChoice> }) =>
        control.configSyncApply(a.token, a.choices),
      onSuccess: () => qc.invalidateQueries({ queryKey: ['configSync'] }),
    }),
  };
}

// A preview is single use and time limited, so it never counts as fresh.
export function useConfigSyncPreview(enabled: () => boolean) {
  return useQuery({
    queryKey: configSyncPreviewKey,
    queryFn: () => control.configSyncPreview(),
    staleTime: 0,
    gcTime: 0,
    enabled,
  });
}
