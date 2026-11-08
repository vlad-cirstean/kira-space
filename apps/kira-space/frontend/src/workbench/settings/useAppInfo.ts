import { useQuery } from '@tanstack/vue-query';
import type { MaybeRefOrGetter } from 'vue';
import { control } from '../../bridge/control';

// Constant per process: the client default (never stale) means one fetch, made on first open of About.
export function useAppInfo(enabled: MaybeRefOrGetter<boolean>) {
  return useQuery({ queryKey: ['appInfo'], queryFn: () => control.appInfo(), enabled });
}
