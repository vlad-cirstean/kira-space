import type { QueryClient } from '@tanstack/vue-query';
import {
  type ComputedRef,
  computed,
  type MaybeRefOrGetter,
  type Ref,
  ref,
  toValue,
  watch,
} from 'vue';
import { isConflict } from '../bridge/codedError';
import { useAsyncAction } from './asyncAction';

// One hash-guarded draft: the value being edited, the hash it was built on, a dirty flag and a
// save that sends both. A write refused with E_CONFLICT (the source moved) offers Reload and
// Overwrite. A clean draft follows its source; a dirty one never loses typed work to a push.

export interface DraftSource<T> {
  value: T;
  hash: string;
}

/** Draft value, base hash and dirty flag; a store that owns them passes them as `state`. */
export interface GuardedDraftState<T> {
  draft: Ref<T>;
  baseHash: Ref<string>;
  dirty: Ref<boolean>;
}

export function guardedDraftState<T>(initial: T): GuardedDraftState<T> {
  return { draft: ref(initial) as Ref<T>, baseHash: ref(''), dirty: ref(false) };
}

export interface GuardedDraftOptions<T> {
  /** The saved value as the query or prop holds it; undefined while loading. Followed while clean. */
  source: MaybeRefOrGetter<DraftSource<T> | undefined>;
  /** Fresh read for Reload and Overwrite. Default: `toValue(source)`. */
  refetch?: () => Promise<DraftSource<T> | undefined>;
  /** Writes `value` built on `baseHash`; resolves the new hash. */
  write: (value: T, baseHash: string) => Promise<{ hash: string }>;
  /** Initial draft when no `state` is passed. */
  initial?: T;
  state?: GuardedDraftState<T>;
  /** Copy taken from the source (default: identity; workflows: `cloneWorkflow`). */
  clone?: (v: T) => T;
  /** Compare key for "unchanged while the write ran" (default: the value; workflows: JSON). */
  key?: (v: T) => unknown;
  /** Discard and Reload: replaces the default state writes (store `open` resets its selection). */
  adopt?: (src: DraftSource<T> | undefined) => void;
  /** Runs before a save; false stops it (config: apply the form, refuse on a problem). */
  beforeSave?: () => Promise<boolean>;
  /** Writes riding the same Save after the file (typed secrets). Save runs when either is pending. */
  side?: { pending: () => boolean; flush: () => Promise<void> };
  /** After a successful file write (store clears its fresh-step set). */
  onSaved?: () => void;
  /** After follow and after resetTo wrote the state (config: decode the new text). */
  afterLoad?: () => void;
  /** Only a unit spec without an app. */
  queryClient?: QueryClient;
}

export function useGuardedDraft<T>(opts: GuardedDraftOptions<T>): GuardedDraftState<T> & {
  saving: ComputedRef<boolean>;
  saveError: ComputedRef<string | null>;
  conflict: ComputedRef<boolean>;
  canSave: ComputedRef<boolean>;
  edit: (next: T) => void;
  save: () => Promise<boolean>;
  resetTo: (src: DraftSource<T> | undefined) => void;
  discard: () => void;
  reload: () => Promise<void>;
  overwrite: () => Promise<void>;
  clearError: () => void;
} {
  const state = opts.state ?? guardedDraftState<T>(opts.initial as T);
  const { draft, baseHash, dirty } = state;
  const clone = opts.clone ?? ((v: T) => v);
  const key = opts.key ?? ((v: T) => v);
  const refetch = opts.refetch ?? (async () => toValue(opts.source));
  const sidePending = () => opts.side?.pending() ?? false;

  const action = useAsyncAction(
    async () => {
      if (dirty.value) {
        const sent = key(draft.value);
        const next = await opts.write(draft.value, baseHash.value);
        baseHash.value = next.hash;
        if (key(draft.value) === sent) dirty.value = false;
        opts.onSaved?.();
      }
      await opts.side?.flush();
    },
    { queryClient: opts.queryClient },
  );

  const saving = action.pending;
  const saveError = action.error;
  const conflict = computed(() => (action.cause.value ? isConflict(action.cause.value) : false));
  const canSave = computed(() => (dirty.value || sidePending()) && !saving.value);

  watch(
    () => toValue(opts.source),
    (src) => {
      if (!src || dirty.value) return;
      draft.value = clone(src.value);
      baseHash.value = src.hash;
      opts.afterLoad?.();
    },
    { immediate: true },
  );

  function edit(next: T): void {
    draft.value = next;
    dirty.value = true;
  }

  async function save(): Promise<boolean> {
    if (saving.value) return false;
    if (opts.beforeSave && !(await opts.beforeSave())) return false;
    if (!dirty.value && !sidePending()) return false;
    return action.run();
  }

  function resetTo(src: DraftSource<T> | undefined): void {
    action.reset();
    if (opts.adopt) {
      opts.adopt(src);
    } else {
      draft.value = src ? clone(src.value) : (opts.initial as T);
      baseHash.value = src?.hash ?? '';
      dirty.value = false;
    }
    opts.afterLoad?.();
  }

  function discard(): void {
    resetTo(toValue(opts.source));
  }

  async function reload(): Promise<void> {
    resetTo(await refetch());
  }

  async function overwrite(): Promise<void> {
    const fresh = await refetch();
    if (!fresh) return;
    baseHash.value = fresh.hash;
    dirty.value = true;
    await save();
  }

  return {
    draft,
    baseHash,
    dirty,
    saving,
    saveError,
    conflict,
    canSave,
    edit,
    save,
    resetTo,
    discard,
    reload,
    overwrite,
    clearError: action.reset,
  };
}
