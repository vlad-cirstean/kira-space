import {
  type MaybeComputedElementRef,
  unrefElement,
  useDraggable,
  useEventListener,
} from '@vueuse/core';
import { computed, type MaybeRefOrGetter, onScopeDispose, type Ref, toValue } from 'vue';
import {
  RESIZE_KEY_STEP,
  type ResizeAxis,
  type ResizeBounds,
  type ResizeDirection,
  type ResizeUnit,
  sizeAfterDrag,
  sizeAfterKey,
} from './paneResizeMath';

export interface PaneResizeOptions {
  /** The handle element. */
  target: Ref<HTMLElement | null>;
  /** Current size of the resized pane, in `unit`. */
  size: MaybeRefOrGetter<number>;
  min: MaybeRefOrGetter<number>;
  /** Unset: no upper bound. */
  max?: MaybeRefOrGetter<number | undefined>;
  axis: ResizeAxis;
  unit?: ResizeUnit;
  /** Element whose length along `axis` is 100%; required for `unit: 'percent'`. */
  container?: MaybeComputedElementRef;
  direction?: ResizeDirection;
  step?: number;
  /** Size change per pixel of pointer travel; 2 for a centred box whose edge moves half the change. */
  scale?: number;
  /** Every pointer move and keyboard step: render this size. */
  onLive?: (size: number) => void;
  /** Pointer release, keyboard step, Home or End: persist this size. */
  onCommit?: (size: number) => void;
  /** Drag aborted (pointercancel, window blur, unmount): drop any live preview. */
  onCancel?: () => void;
}

/** One pane-resize gesture for every splitter and column handle. Pointer wiring is VueUse
 *  `useDraggable`; the rest is clamp arithmetic in `paneResizeMath`. */
export function usePaneResize(options: PaneResizeOptions) {
  const axis = options.axis;
  const unit = options.unit ?? 'px';
  const direction = options.direction ?? 'normal';
  const step = options.step ?? RESIZE_KEY_STEP;
  const scale = options.scale ?? 1;

  function containerPx(): number {
    if (unit === 'px') return 0;
    const el = unrefElement(options.container);
    if (!el) return 0;
    const box = el.getBoundingClientRect();
    return axis === 'x' ? box.width : box.height;
  }

  function bounds(): ResizeBounds {
    return {
      min: toValue(options.min),
      max: toValue(options.max) ?? Number.POSITIVE_INFINITY,
      unit,
      containerPx: containerPx(),
    };
  }

  // Shell-wide resize cursor for the length of the drag (base.css `kira-resizing-*`).
  const cursorClass = `kira-resizing-${axis}`;
  const setCursor = (on: boolean): void => {
    document.documentElement.classList.toggle(cursorClass, on);
  };

  let origin = 0;
  let startSize = 0;
  // Plain flag, not `isDragging.value`: abort runs on unmount inside the parent's render, where a
  // reactive read would subscribe that parent to this handle's computed and pin the unmounted tree.
  let active = false;
  let last: number | undefined;
  const pointerOf = (e: PointerEvent): number => (axis === 'x' ? e.clientX : e.clientY);

  const { isDragging } = useDraggable(options.target, {
    axis,
    preventDefault: true,
    onStart: (_pos, e) => {
      active = true;
      setCursor(true);
      origin = pointerOf(e);
      startSize = toValue(options.size);
      last = undefined;
    },
    onMove: (_pos, e) => {
      last = sizeAfterDrag(startSize, (pointerOf(e) - origin) * scale, direction, bounds());
      options.onLive?.(last);
    },
    onEnd: (_pos, e) => {
      active = false;
      const done = last;
      last = undefined;
      setCursor(false);
      if (e.type === 'pointercancel') options.onCancel?.();
      else if (done !== undefined) options.onCommit?.(done);
    },
  });

  function abort(): void {
    if (!active) return;
    active = false;
    last = undefined;
    setCursor(false);
    options.onCancel?.();
  }
  // A release can vanish with the window itself (focus moved to another app); end the drag.
  useEventListener(window, 'blur', abort);
  onScopeDispose(abort);

  function onKeydown(e: KeyboardEvent): void {
    if (e.ctrlKey || e.metaKey || e.altKey || e.shiftKey) return;
    const next = sizeAfterKey(e.key, toValue(options.size), axis, direction, step, bounds());
    if (next === null) return;
    e.preventDefault();
    options.onLive?.(next);
    options.onCommit?.(next);
  }

  const handleProps = computed(() => {
    const b = bounds();
    return {
      role: 'separator' as const,
      tabindex: 0,
      'aria-orientation': axis === 'x' ? ('vertical' as const) : ('horizontal' as const),
      'aria-valuenow': Math.round(toValue(options.size)),
      'aria-valuemin': b.min,
      'aria-valuemax': Number.isFinite(b.max) ? b.max : undefined,
      'data-state': isDragging.value ? 'drag' : undefined,
      onKeydown,
    };
  });

  return { handleProps, dragging: isDragging as Readonly<Ref<boolean>> };
}
