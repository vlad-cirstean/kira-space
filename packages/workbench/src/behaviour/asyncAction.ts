import { type QueryClient, type UseMutationReturnType, useMutation } from '@tanstack/vue-query';
import { type ComputedRef, computed } from 'vue';
import { type CodedError, errorText, toCodedError } from '../bridge/codedError';

/**
 * A write's pending and error come from its TanStack mutation, never a hand ref:
 * `useMutationAction(m)` for a mutation from a queries file, `useAsyncAction(fn)` for any other
 * awaited call, `useTaskAction()` for one error surface fed by several writes. `describe` maps or
 * hides the text, `cause` gives the coded error for branching. A component never wraps one
 * mutation call in `useAsyncAction` (two observers for one write). Needs a Vue setup (or a
 * `queryClient` option) and the app's `VueQueryPlugin`.
 */
export interface AsyncAction<Args extends unknown[]> {
  /** `false` after a failure; never throws. */
  run: (...args: Args) => Promise<boolean>;
  pending: ComputedRef<boolean>;
  /** `describe(cause)`; null when none or hidden. */
  error: ComputedRef<string | null>;
  /** `toCodedError` of the failure, for code branches. */
  cause: ComputedRef<CodedError | null>;
  reset: () => void;
}

export interface AsyncActionOptions {
  /** Default `errorText`; null hides the failure (a cancel). */
  describe?: (err: unknown) => string | null;
  /** Only a Pinia store unit-tested without an app. */
  queryClient?: QueryClient;
}

function derive(
  failure: () => unknown,
  failed: () => boolean,
  describe: (err: unknown) => string | null,
): Pick<AsyncAction<never[]>, 'error' | 'cause'> {
  return {
    error: computed(() => (failed() ? describe(failure()) : null)),
    cause: computed(() => (failed() ? toCodedError(failure()) : null)),
  };
}

export function useAsyncAction<Args extends unknown[]>(
  action: (...args: Args) => Promise<unknown>,
  opts: AsyncActionOptions = {},
): AsyncAction<Args> & { fail: (err: unknown) => void } {
  // The variable is a thunk so typed values (secrets) are not kept as plain mutation `variables`.
  // `always`: this wrapper runs the thunk at once; the calls inside carry the client's network mode.
  // Paused here instead, the phone's pending state (set by the inner write) would never show.
  const m = useMutation<unknown, unknown, () => Promise<unknown>>(
    { mutationFn: (task) => task(), networkMode: 'always', gcTime: 0 },
    opts.queryClient,
  );
  async function run(...args: Args): Promise<boolean> {
    try {
      await m.mutateAsync(() => action(...args));
      return true;
    } catch {
      return false;
    }
  }
  return {
    run,
    pending: computed(() => m.isPending.value),
    ...derive(
      () => m.error.value,
      () => m.isError.value,
      opts.describe ?? errorText,
    ),
    reset: m.reset,
    // `mutate` swallows the rejection; records a failure that happened outside the action.
    fail: (err) => m.mutate(() => Promise.reject(err)),
  };
}

export function useTaskAction(
  opts?: AsyncActionOptions,
): AsyncAction<[task: () => Promise<unknown>]> & { fail: (err: unknown) => void } {
  return useAsyncAction((task: () => Promise<unknown>) => task(), opts);
}

export function useMutationAction<TData, TVars>(
  mutation: UseMutationReturnType<TData, unknown, TVars, unknown>,
  opts: Pick<AsyncActionOptions, 'describe'> = {},
): AsyncAction<[vars: TVars]> {
  async function run(vars: TVars): Promise<boolean> {
    try {
      await mutation.mutateAsync(vars);
      return true;
    } catch {
      return false;
    }
  }
  return {
    run,
    pending: computed(() => mutation.isPending.value),
    ...derive(
      () => mutation.error.value,
      () => mutation.isError.value,
      opts.describe ?? errorText,
    ),
    reset: mutation.reset,
  };
}
