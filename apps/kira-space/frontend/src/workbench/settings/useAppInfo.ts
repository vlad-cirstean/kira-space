import { useQuery } from '@tanstack/vue-query';
import { control } from '../../bridge/control';

// Constant per process: the client default (never stale) means one fetch.
export function useAppInfo() {
  return useQuery({ queryKey: ['appInfo'], queryFn: () => control.appInfo() });
}
