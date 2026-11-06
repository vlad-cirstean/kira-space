import { computed, ref } from 'vue';

interface RowTip {
  label: string;
  left: number;
  top: number;
  width: number;
  height: number;
}

/** One tooltip for every row's action buttons: anchored to a fixed, invisible span moved over the
 *  hovered or focused `[data-tip]` element, driven by delegated listeners on the list. */
export function useRowTip() {
  const rowTip = ref<RowTip | null>(null);
  const rowTipStyle = computed(() => ({
    left: `${rowTip.value?.left ?? 0}px`,
    top: `${rowTip.value?.top ?? 0}px`,
    width: `${rowTip.value?.width ?? 0}px`,
    height: `${rowTip.value?.height ?? 0}px`,
  }));
  function tipSource(e: Event): HTMLElement | null {
    return e.target instanceof Element ? e.target.closest<HTMLElement>('[data-tip]') : null;
  }
  function showRowTip(e: Event): void {
    const el = tipSource(e);
    if (!el) return;
    const r = el.getBoundingClientRect();
    rowTip.value = {
      label: el.dataset.tip ?? '',
      left: r.left,
      top: r.top,
      width: r.width,
      height: r.height,
    };
  }
  function hideRowTip(e: PointerEvent | FocusEvent): void {
    const el = tipSource(e);
    if (el && !(e.relatedTarget instanceof Node && el.contains(e.relatedTarget)))
      rowTip.value = null;
  }
  function clearRowTip(): void {
    rowTip.value = null;
  }
  return { rowTip, rowTipStyle, showRowTip, hideRowTip, clearRowTip };
}
