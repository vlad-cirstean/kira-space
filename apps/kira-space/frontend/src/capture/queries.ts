import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query';
import { tryOnScopeDispose } from '@vueuse/core';
import { control } from '../bridge/control';

export const captureItemKey = (windowKey: string) => ['capture', 'item', windowKey] as const;
const captureStatusKey = ['capture', 'status'] as const;

/** The hotkey binding state; the backend pushes every change into the cache. */
export function useCaptureStatus() {
  const queryClient = useQueryClient();
  const off = control.onCaptureStatus((state) => queryClient.setQueryData(captureStatusKey, state));
  tryOnScopeDispose(off);
  return useQuery({
    queryKey: captureStatusKey,
    queryFn: () => control.captureStatus(),
  });
}

export interface ConfirmVars {
  windowKey: string;
  itemId: string;
  title: string;
  /** Plan day (YYYY-MM-DD) the task lands on; null lands in Later. */
  day: string | null;
}

/** Adds the task; a rejection keeps the popup open, and so does a notice in the result. */
export function useConfirmCapture() {
  return useMutation({ mutationFn: (vars: ConfirmVars) => control.captureConfirm(vars) });
}

export function useDismissCapture() {
  return useMutation({ mutationFn: (windowKey: string) => control.captureDismiss(windowKey) });
}
