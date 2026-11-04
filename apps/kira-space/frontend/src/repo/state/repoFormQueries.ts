import { useMutation } from '@tanstack/vue-query';
import type { FormEdit } from '@workbench/config/configForm';
import { type MaybeRefOrGetter, toValue } from 'vue';
import { control } from '../../bridge/control';
import type { RepoFormResult } from './repoFormTypes';

/** Applies form edits to a draft text and decodes it; a stateless call, nothing is written. */
export function useRepoForm(file: MaybeRefOrGetter<string>) {
  const mutation = useMutation({
    mutationFn: (args: { text: string; edits: FormEdit[] }): Promise<RepoFormResult> =>
      control.repoConfigForm({ file: toValue(file), text: args.text, edits: args.edits }),
  });
  return (text: string, edits: FormEdit[]) => mutation.mutateAsync({ text, edits });
}
