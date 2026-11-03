import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query';
import { computed, type MaybeRefOrGetter, toValue } from 'vue';
import { useAutomationsModule } from './module';

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
