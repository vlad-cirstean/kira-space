import { type Ref, watch } from 'vue';

/**
 * Keeps the first visible row where it is while row heights above it change (the worker's counts
 * replacing estimates, a row above toggled). `heights` must be the same array the virtualizer
 * lays rows out with; runs after the virtualizer's own measure, before paint.
 */
export function useHeightAnchor(
  scrollEl: Ref<HTMLElement | null>,
  heights: () => readonly number[] | undefined,
): void {
  watch(
    heights,
    (next, prev) => {
      const el = scrollEl.value;
      if (!el || !next || !prev || next.length !== prev.length) return;
      const top = el.scrollTop;
      if (top === 0) return;
      let start = 0;
      let index = 0;
      while (index < prev.length && start + prev[index] <= top) start += prev[index++];
      if (index >= prev.length) return;
      let moved = 0;
      for (let i = 0; i < index; i++) moved += next[i];
      if (moved !== start) el.scrollTop = moved + (top - start);
    },
    { flush: 'post' },
  );
}
