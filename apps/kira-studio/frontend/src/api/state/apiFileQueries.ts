import type { ApiFileDetail, ApiFormResult } from '@shared/domain/apiConfig';
import { useMutation, useQuery } from '@tanstack/vue-query';
import type { FormEdit } from '@workbench/config/configForm';
import { queryClient } from '@workbench/state/queryClient';
import { type MaybeRefOrGetter, toValue } from 'vue';
import { control } from '../../bridge/control';
import { apiConfigKey } from './apiQueries';

const apiFileKey = (file: string) => [...apiConfigKey, 'file', file] as const;

/** One collection or environment file's text and hash. */
export function useApiFile(file: MaybeRefOrGetter<string | null>) {
  return useQuery(() => {
    const name = toValue(file);
    return {
      queryKey: apiFileKey(name ?? ''),
      enabled: name !== null,
      queryFn: (): Promise<ApiFileDetail> => control.apiConfigFile(name ?? ''),
    };
  }, queryClient);
}

export function useSaveApiFile() {
  return useMutation(
    {
      mutationFn: (args: { file: string; text: string; baseHash: string }) =>
        control.apiConfigSaveFile(args.file, args.text, args.baseHash),
      onSuccess: (detail: ApiFileDetail) => {
        queryClient.setQueryData(apiFileKey(detail.file), detail);
        return queryClient.invalidateQueries({ queryKey: apiConfigKey, exact: true });
      },
    },
    queryClient,
  );
}

/** Applies form edits to a draft text and decodes it; a stateless call, nothing is written. */
export function useApiForm(file: MaybeRefOrGetter<string>) {
  const mutation = useMutation(
    {
      mutationFn: (args: { text: string; edits: FormEdit[] }): Promise<ApiFormResult> =>
        control.apiConfigForm(toValue(file), args.text, args.edits),
    },
    queryClient,
  );
  return (text: string, edits: FormEdit[]) => mutation.mutateAsync({ text, edits });
}
