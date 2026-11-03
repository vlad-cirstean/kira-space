import { useElementByPoint, useEventListener, useMouse, useRafFn } from '@vueuse/core';
import { type Ref, shallowRef, watch } from 'vue';
import { DRAG_TOLERANCE_PX } from './sortableReorder';

/** A drop on the top or bottom edge of the row with `id`. */
export interface EdgeDrop {
  id: string;
  edge: 'top' | 'bottom';
}

/** Order after dropping `dragged` on `target`'s top or bottom edge; the input when nothing moves. */
export function orderAfterDrop(ids: readonly string[], dragged: string, drop: EdgeDrop): string[] {
  const rest = ids.filter((id) => id !== dragged);
  const at = rest.indexOf(drop.id);
  if (at < 0 || !ids.includes(dragged)) return [...ids];
  rest.splice(drop.edge === 'top' ? at : at + 1, 0, dragged);
  return rest;
}

/**
 * Pointer drag for a virtualized tree. Sortable cannot serve one: only the visible window is
 * mounted, rows are absolutely positioned, and a drag has to survive its source row unmounting
 * during edge auto-scroll. The caller decides what a press may grab (`pressable`), where the
 * pointer would drop (`resolve`) and what a drop does (`onDrop`); this owns press tolerance, hit
 * testing, edge auto-scroll, Escape/cancel and swallowing the click that ends a drag.
 */
export function useTreePointerDrag<D>(opts: {
  container: Ref<HTMLElement | null>;
  /** The row a press may start a drag from, or null. */
  pressable: (target: HTMLElement) => HTMLElement | null;
  disabled: () => boolean;
  rowHeight: () => number;
  /** Runs once the pointer leaves the press tolerance; false cancels the drag. */
  onStart?: (pressed: HTMLElement) => boolean | undefined;
  /** The drop under the pointer for the hovered element and client y, or null. */
  resolve: (hit: HTMLElement | null, y: number) => D | null;
  onDrop: (pressed: HTMLElement, drop: D) => void;
}): {
  dragging: Readonly<Ref<HTMLElement | null>>;
  drop: Readonly<Ref<D | null>>;
} {
  const dragging = shallowRef<HTMLElement | null>(null);
  const drop = shallowRef<D | null>(null);
  let pressed: HTMLElement | null = null;
  let pressAt = { x: 0, y: 0 };
  let swallowClick = false;

  const { x, y } = useMouse({ type: 'client' });
  const hit = useElementByPoint({
    x,
    y,
    scheduler: (cb) => useRafFn(cb, { immediate: false }),
  });
  const scroller = useRafFn(
    () => {
      const el = opts.container.value;
      if (!el) return;
      const rect = el.getBoundingClientRect();
      const zone = opts.rowHeight();
      const step = zone / 4;
      if (y.value < rect.top + zone) el.scrollTop -= step;
      else if (y.value > rect.bottom - zone) el.scrollTop += step;
    },
    { immediate: false },
  );

  watch([hit.element, y], ([el]) => {
    if (!dragging.value) return;
    drop.value = opts.resolve((el as HTMLElement | null) ?? null, y.value);
  });

  function reset(): void {
    pressed = null;
    dragging.value = null;
    drop.value = null;
    hit.pause();
    scroller.pause();
  }

  function finish(commit: boolean): void {
    const source = dragging.value;
    const target = drop.value;
    reset();
    if (commit && source && target !== null) opts.onDrop(source, target);
  }

  useEventListener(opts.container, 'pointerdown', (e: PointerEvent) => {
    swallowClick = false;
    if (e.button !== 0 || !e.isPrimary || opts.disabled()) return;
    const row = opts.pressable(e.target as HTMLElement);
    if (!row) return;
    pressed = row;
    pressAt = { x: e.clientX, y: e.clientY };
  });

  useEventListener(document, 'pointermove', (e: PointerEvent) => {
    if (pressed === null || dragging.value !== null) return;
    if (Math.hypot(e.clientX - pressAt.x, e.clientY - pressAt.y) < DRAG_TOLERANCE_PX) return;
    if (opts.onStart?.(pressed) === false) {
      pressed = null;
      return;
    }
    dragging.value = pressed;
    swallowClick = true;
    hit.resume();
    scroller.resume();
  });
  useEventListener(document, 'pointerup', () => {
    if (dragging.value) finish(true);
    else pressed = null;
  });
  useEventListener(document, 'pointercancel', () => finish(false));
  useEventListener(document, 'keydown', (e: KeyboardEvent) => {
    if (dragging.value && e.key === 'Escape') {
      e.preventDefault();
      finish(false);
    }
  });
  // A finished drag must not also select or open the row the pointer came up on.
  useEventListener(
    opts.container,
    'click',
    (e: MouseEvent) => {
      if (!swallowClick) return;
      swallowClick = false;
      e.stopPropagation();
    },
    { capture: true },
  );

  return { dragging, drop };
}
