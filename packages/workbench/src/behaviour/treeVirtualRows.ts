import { STICKY_MAX_ROWS, type StickyRowLike, stickyBand, stickyInsetFor } from '@theme/stickyBand';
import { computed, type MaybeRefOrGetter, type Ref } from 'vue';
import { useVirtualRows } from './virtualRows';

// P104 §3.4: the old hand-rolled tree host's own recipe (virtualization + the pinned ancestor
// band + reveal-scroll) rebuilt on useVirtualRows -- stickyBand.ts itself is a logic file, not a
// component primitive, and stays untouched (§1.7's own distinction).
export function useTreeVirtualRows<T extends StickyRowLike & { key: string }>(opts: {
  rows: () => readonly T[];
  rowHeight: () => number;
  scrollElement: Ref<HTMLElement | null>;
  keepScroll?: MaybeRefOrGetter<string | null>;
}) {
  const virtual = useVirtualRows({
    count: () => opts.rows().length,
    rowHeight: opts.rowHeight,
    scrollElement: opts.scrollElement,
    keepScroll: opts.keepScroll,
  });

  // D5: three rows, further clamped so a deliberately short panel never spends more of its own
  // height on the band than it has rows to spare.
  const stickyMaxRows = computed(() =>
    Math.max(
      0,
      Math.min(STICKY_MAX_ROWS, Math.floor(virtual.viewportHeight.value / opts.rowHeight()) - 2),
    ),
  );

  const band = computed(() =>
    stickyBand(opts.rows(), virtual.scrollTop.value, opts.rowHeight(), stickyMaxRows.value),
  );

  // A caller sets its own pending-reveal key, waits a tick for its rows to reflect any expansion,
  // then calls this.
  async function revealKey(key: string): Promise<void> {
    await new Promise((resolve) => requestAnimationFrame(resolve));
    const rows = opts.rows();
    const index = rows.findIndex((row) => row.key === key);
    if (index < 0) return;
    const inset = stickyInsetFor(rows, index, opts.rowHeight(), stickyMaxRows.value);
    virtual.scrollToIndex(index, inset);
  }

  return { ...virtual, band, revealKey };
}
