import { useEventListener } from '@vueuse/core';
import {
  computed,
  type MaybeRefOrGetter,
  type Ref,
  toValue,
  type WritableComputedRef,
  watch,
} from 'vue';
import { useViewStateStore } from '../state/viewState';

// Ref backed by the session view-state store; a reactive key reads the new entry (per repo, per container).
export function useKeptState<T>(
  key: MaybeRefOrGetter<string>,
  fallback: T,
): WritableComputedRef<T> {
  const store = useViewStateStore();
  return computed({
    get: () => store.read(toValue(key), fallback),
    set: (value) => store.write(toValue(key), value),
  });
}

// Saves an element's scroll and restores it once per mount and key. `ready` gates restore until
// the content is tall enough (virtual lists size after their rows arrive).
export function useKeptScroll(
  key: MaybeRefOrGetter<string | null>,
  el: Ref<HTMLElement | null | undefined>,
  opts?: { ready?: () => boolean },
): void {
  const store = useViewStateStore();
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

// Monaco: `initial` seeds the `scrollTop` prop once, `save` is wired to `update:scrollTop`.
export function useKeptEditorScroll(key: MaybeRefOrGetter<string>): {
  initial: number;
  save: (top: number) => void;
} {
  const store = useViewStateStore();
  return {
    get initial() {
      return store.scroll(toValue(key))?.top ?? 0;
    },
    save: (top) => store.setScroll(toValue(key), { top, left: 0 }),
  };
}
