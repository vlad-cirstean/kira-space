import type { RunFormResult } from '@shared/domain/scripts';
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query';
import { computed, type MaybeRefOrGetter, toValue } from 'vue';
import type { FormEdit } from '../config/configForm';
import { type ScriptsSeam, useAutomationsModule } from './module';

// Server state for one run file: its text, hash and validity. Keys are flat [domain, file] tuples.
export const runFileKey = (file: string) => ['runFile', file] as const;

export function useRunFile(file: MaybeRefOrGetter<string>) {
  const { scripts } = useAutomationsModule();
  return useQuery({
    queryKey: computed(() => runFileKey(toValue(file))),
    queryFn: () => scripts.readFile(toValue(file)),
    staleTime: 0,
  });
}

export function useSaveRunFile() {
  const { scripts } = useAutomationsModule();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (args: { file: string; text: string; baseHash: string }) =>
      scripts.saveFile(args.file, args.text, args.baseHash),
    onSuccess: (view) => queryClient.setQueryData(runFileKey(view.file), view),
  });
}

/** Applies form edits to a draft text and decodes it; a stateless call, nothing is written. */
export function useRunForm(file: MaybeRefOrGetter<string>) {
  const { scripts } = useAutomationsModule();
  const mutation = useMutation({
    mutationFn: (args: { text: string; edits: FormEdit[] }): Promise<RunFormResult> =>
      scripts.form(toValue(file), args.text, args.edits),
  });
  return (text: string, edits: FormEdit[]) => mutation.mutateAsync({ text, edits });
}

/** Edits one run file without its dialog: read, apply the form edits, save on the read hash. A file
 *  with a problem is refused with its message, and nothing is written. */
export async function applyRunFileEdits(
  scripts: Pick<ScriptsSeam, 'readFile' | 'form' | 'saveFile'>,
  file: string,
  edits: FormEdit[],
): Promise<void> {
  const view = await scripts.readFile(file);
  const result = await scripts.form(file, view.text, edits);
  if (result.problem) throw new Error(result.problem.message);
  await scripts.saveFile(file, result.text, view.hash);
}
