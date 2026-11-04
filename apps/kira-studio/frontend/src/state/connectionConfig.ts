import type {
  ConnectionConfigView,
  ConnectionFileDetail,
  ConnectionFormResult,
  ConnectionSecretKind,
} from '@shared/domain/connectionConfig';
import { useMutation, useQuery } from '@tanstack/vue-query';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import type { FormEdit } from '@workbench/config/configForm';
import { KEEP_WHILE_OPEN, queryClient } from '@workbench/state/queryClient';
import { type MaybeRefOrGetter, toValue } from 'vue';
import { control } from '../bridge/control';

const connectionConfigKey = ['connectionConfig', 'view'] as const;
const connectionFileKey = (file: string) => ['connectionConfig', 'file', file] as const;

// The connection files' validity and the secret names they reference. Never carries a value.
export function useConnectionConfig() {
  return useQuery(
    {
      queryKey: connectionConfigKey,
      gcTime: KEEP_WHILE_OPEN,
      queryFn: (): Promise<ConnectionConfigView> => control.connectionConfigView(),
    },
    queryClient,
  );
}

export function useConnectionFile(file: MaybeRefOrGetter<string | null>) {
  return useQuery(() => {
    const name = toValue(file);
    return {
      queryKey: connectionFileKey(name ?? ''),
      enabled: name !== null,
      queryFn: (): Promise<ConnectionFileDetail> => control.connectionConfigFile(name ?? ''),
    };
  }, queryClient);
}

export function useSaveConnectionFile() {
  return useMutation(
    {
      mutationFn: (args: { file: string; text: string; baseHash: string }) =>
        control.connectionConfigSaveFile(args.file, args.text, args.baseHash),
      onSuccess: (detail: ConnectionFileDetail) => {
        queryClient.setQueryData(connectionFileKey(detail.file), detail);
        return queryClient.invalidateQueries({ queryKey: connectionConfigKey });
      },
    },
    queryClient,
  );
}

/** Applies form edits to a draft text and decodes it; a stateless call, nothing is written. */
export function useConnectionForm(file: MaybeRefOrGetter<string>) {
  const mutation = useMutation(
    {
      mutationFn: (args: { text: string; edits: FormEdit[] }): Promise<ConnectionFormResult> =>
        control.connectionConfigForm(toValue(file), args.text, args.edits),
    },
    queryClient,
  );
  return (text: string, edits: FormEdit[]) => mutation.mutateAsync({ text, edits });
}

function refreshAfterSecret(view: ConnectionConfigView): Promise<void> {
  queryClient.setQueryData(connectionConfigKey, view);
  return queryClient.invalidateQueries({ queryKey: ['connectionConfig', 'file'] });
}

export function useSetConnectionSecret() {
  return useMutation(
    {
      mutationFn: (args: { kind: ConnectionSecretKind; name: string; value: string }) =>
        control.connectionConfigSetSecret(args.kind, args.name, args.value),
      onSuccess: refreshAfterSecret,
    },
    queryClient,
  );
}

export function useDeleteConnectionSecret() {
  return useMutation(
    {
      mutationFn: (args: { kind: ConnectionSecretKind; name: string }) =>
        control.connectionConfigDeleteSecret(args.kind, args.name),
      onSuccess: refreshAfterSecret,
    },
    queryClient,
  );
}

export function useTestConnection() {
  return useMutation({ mutationFn: (id: string) => control.connectionsTest(id) }, queryClient);
}

let unsubscribeConfigChanged: (() => void) | null = null;

/** Called once in main.ts's bootstrap(): a file edit anywhere refreshes the view and open files. */
export function initConnectionConfigSync(): void {
  unsubscribeConfigChanged?.();
  unsubscribeConfigChanged = control.onConnectionConfigChanged(() => {
    fireAndForget(
      queryClient.invalidateQueries({ queryKey: ['connectionConfig'] }),
      'connectionConfigChanged',
    );
  });
}
