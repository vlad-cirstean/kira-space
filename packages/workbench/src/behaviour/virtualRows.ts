import { defaultRangeExtractor, type Range, useVirtualizer } from '@tanstack/vue-virtual';
import { useResizeObserver } from '@vueuse/core';
import { computed, onMounted, type Ref, ref, watch } from 'vue';

// The 9 consuming files' own scoped `.virtual-row`/`.sticky-row` copies (each byte-identical,
// restored per-file after base.css's shared `@utility virtual-row`/`sticky-row` was reverted --
// an unlayered scoped rule is required to beat a row's own unlayered
// `.tree-row { position: relative }`; a `@utility` is Tailwind-layered and always loses that
// fight) -- now that every row's own unlayered `position: relative` is dropped in favour of a
// plain `relative` utility class merged through `cn()`/`tailwind-merge`, a parent can bind these
// two literal strings directly as the row's `class` prop: `cn()` resolves the position conflict
// itself (last one wins on the same property, so `absolute` always beats `relative`), and a plain
// `<div>` row has no `relative` to conflict with in the first place. The sticky row's own
// background (`bg-bg`) moved onto the row's own `stateClass` ternary -- never here, so this stays
// position-only, unlike the old `.sticky-row { @apply absolute left-0 right-0 z-1; background:
// var(--kira-bg); }`.
export const VIRTUAL_ROW_CLASS = 'absolute top-0 left-0 w-full';
export const STICKY_ROW_CLASS = 'absolute left-0 right-0 z-1';

// The old hand-rolled virtual list's own recipe, rebuilt on @tanstack/vue-virtual directly at
// each call site rather than kept as a wrapper component -- this is the shared *behaviour*
// (windowing, scrollstate, scrollToIndex), never a re-rendered wrapper; the
// scroll container and row markup still live in each caller's own template.
export interface UseVirtualRowsOptions {
  count: () => number;
  rowHeight: () => number;
  /** Exact per-row heights, already known (never measured) -- VirtualList's own `rowHeights`
   *  escape hatch for rows whose height varies (a document's collapsed head vs. expanded body).
   *  Falls back to the uniform `rowHeight` when absent or when an index has none. */
  rowHeights?: () => readonly number[] | undefined;
  scrollElement: Ref<HTMLElement | null>;
  /** Rows rendered beyond the viewport on each side; a getter lets a caller size it from row heights. Default 8. */
  overscan?: number | (() => number);
  /** An index that stays mounted outside the window, so a focused row keeps focus while it scrolls away. */
  pinned?: () => number | null;
  /** Offset of the first row from the scroll element's content top, when other content sits above the list. */
  scrollMargin?: () => number;
}

export function useVirtualRows(opts: UseVirtualRowsOptions) {
  const virtualizer = useVirtualizer({
    get count() {
      return opts.count();
    },
    getScrollElement: () => opts.scrollElement.value,
    estimateSize: (index: number) => opts.rowHeights?.()?.[index] ?? opts.rowHeight(),
    get scrollMargin() {
      return opts.scrollMargin?.() ?? 0;
    },
    get overscan() {
      return typeof opts.overscan === 'function' ? opts.overscan() : (opts.overscan ?? 8);
    },
    get rangeExtractor() {
      const pin = opts.pinned?.();
      if (pin === undefined) return undefined;
      return (range: Range): number[] => {
        const indexes = defaultRangeExtractor(range);
        if (pin === null || pin < 0 || pin >= range.count || indexes.includes(pin)) return indexes;
        return [...indexes, pin].sort((a, b) => a - b);
      };
    },
  });

  // `estimateSize` isn't one of virtual-core's own memo deps (only `count` and the handful listed
  // in `Virtualizer#maybeNotify` are) -- a row-height change with the same item count (a document
  // row expanding, the row-density toggle) never re-triggers a measure on its own, so rows overlap
  // or mis-lay-out until the count next changes. Force it explicitly on either input changing.
  watch([() => opts.rowHeight(), () => opts.rowHeights?.()], () => virtualizer.value.measure());

  const virtualItems = computed(() => virtualizer.value.getVirtualItems());
  const totalSize = computed(() => virtualizer.value.getTotalSize());

  // Published for callers that need raw scroll geometry (the sticky ancestor band's own
  // stickyBand.ts) -- VirtualList's own `scrollstate` emit, republished as refs instead since
  // there is no wrapper component left to emit an event from.
  const scrollTop = ref(0);
  const viewportHeight = ref(0);
  function onScroll(): void {
    scrollTop.value = opts.scrollElement.value?.scrollTop ?? 0;
  }
  onMounted(() => {
    viewportHeight.value = opts.scrollElement.value?.clientHeight ?? 0;
  });
  useResizeObserver(opts.scrollElement, ([entry]) => {
    if (entry) viewportHeight.value = entry.contentRect.height;
  });

  // Scrolls just enough to bring `index` into view -- top-aligned if it's above the viewport,
  // bottom-aligned if below, a no-op if already visible. `inset` (VirtualList's own) keeps that
  // many px clear above the row when it is top-aligned, so a caller's own sticky band does not
  // occlude the revealed row. Ported verbatim from VirtualList.vue's own scrollToIndex.
  function scrollToIndex(index: number, inset = 0): void {
    const el = opts.scrollElement.value;
    if (!el) return;
    const heights = opts.rowHeights?.();
    const rowH = opts.rowHeight();
    let rowTop = (opts.scrollMargin?.() ?? 0) + index * rowH;
    if (heights) {
      rowTop = opts.scrollMargin?.() ?? 0;
      for (let i = 0; i < index; i++) rowTop += heights[i] ?? rowH;
    }
    const thisRowH = heights?.[index] ?? rowH;
    const rowBottom = rowTop + thisRowH;
    if (rowTop - inset < el.scrollTop) {
      el.scrollTop = rowTop - inset;
    } else if (rowBottom > el.scrollTop + el.clientHeight) {
      el.scrollTop = rowBottom - el.clientHeight;
    }
  }

  return {
    virtualizer,
    virtualItems,
    totalSize,
    scrollTop,
    viewportHeight,
    onScroll,
    scrollToIndex,
  };
}
