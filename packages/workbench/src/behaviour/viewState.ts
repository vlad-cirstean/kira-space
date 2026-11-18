import { useEventListener } from '@vueuse/core';
import {
  computed,
  hasInjectionContext,
  inject,
  type MaybeRefOrGetter,
  type Ref,
  ref,
  toValue,
  type WritableComputedRef,
  watch,
} from 'vue';
import { KEPT_VIEW_STATE_KEY, type KeptViewState, useViewStateStore } from '../state/viewState';

// A git-ui mount is its own Vue app without Pinia; its host provides the store (P660 Part 2).
function keptViewState(): KeptViewState {
  return (hasInjectionContext() ? inject(KEPT_VIEW_STATE_KEY, null) : null) ?? useViewStateStore();
}

// Ref backed by the session view-state store; a reactive key reads the new entry (per repo, per container).
export function useKeptState<T>(
  key: MaybeRefOrGetter<string>,
  fallback: T,
): WritableComputedRef<T> {
  const store = keptViewState();
  return computed({
    get: () => store.read(toValue(key), fallback),
    set: (value) => store.write(toValue(key), value),
  });
}

// Saves an element's scroll and restores it once per mount and key. `ready` gates restore until
// the content is tall enough (virtual lists size after their rows arrive).
export function useKeptScroll(
  key: MaybeRefOrGetter<string | null>,
  el: Readonly<Ref<HTMLElement | null | undefined>>,
  opts?: { ready?: () => boolean },
): void {
  const store = keptViewState();
  let settledKey: string | null = null;

  function save(): void {
    const k = toValue(key);
    const node = el.value;
    if (k === null || !node) return;
    settledKey = k;
    store.setScroll(k, { top: node.scrollTop, left: node.scrollLeft });
  }

  useEventListener(el, 'scroll', save, { passive: true });

  watch(
    [el, () => toValue(key), () => opts?.ready?.() ?? true],
    ([node, k, ready]) => {
      if (!node || k === null || !ready || settledKey === k) return;
      const pos = store.scroll(k);
      if (!pos) return;
      settledKey = k;
      node.scrollTop = pos.top;
      node.scrollLeft = pos.left;
    },
    { immediate: true, flush: 'post' },
  );
}

// Monaco: `initial` seeds the `scrollTop` prop, `save` is wired to `update:scrollTop`. `initial` is
// a snapshot per key, taken before the editor's own first scroll event writes 0 over the store.
export function useKeptEditorScroll(key: MaybeRefOrGetter<string>): {
  readonly initial: number;
  save: (top: number) => void;
} {
  const store = keptViewState();
  const initial = ref(store.scroll(toValue(key))?.top ?? 0);
  watch(
    () => toValue(key),
    (k) => {
      initial.value = store.scroll(k)?.top ?? 0;
    },
  );
  return {
    get initial() {
      return initial.value;
    },
    save: (top) => store.setScroll(toValue(key), { top, left: 0 }),
  };
}
