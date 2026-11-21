import { useEventListener } from '@vueuse/core';
import { computed, nextTick, type Ref, shallowRef, watch } from 'vue';
import { useDraggable } from 'vue-draggable-plus';
import { shortcutFor } from '../shortcuts/keys';

/** The one drag-ghost class; Sortable toggles a single class name. */
export const SORTABLE_GHOST_CLASS = 'opacity-50';

/** Pointer travel before a press becomes a drag; shared by Sortable and the virtualized tree. */
export const DRAG_TOLERANCE_PX = 4;

/** Moves fromId to toId's pre-splice index — createTabsStore.moveTab's own rule, so a drop onto a
 *  later item lands after it and onto an earlier one lands before it. */
export function moveId(ids: readonly string[], fromId: string, toId: string): string[] {
  const from = ids.indexOf(fromId);
  const to = ids.indexOf(toId);
  if (from < 0 || to < 0 || from === to) return [...ids];
  const next = [...ids];
  const [moved] = next.splice(from, 1);
  next.splice(to, 0, moved);
  return next;
}

// One drag at a time. Shared, not per list: the list a cross-list drop lands in never saw the
// drag start, yet must honour an Esc pressed during it.
let dragCancelled = false;

const VERTICAL_IDS = ['list.moveUp', 'list.moveDown'] as const;
const HORIZONTAL_IDS = ['list.moveLeft', 'list.moveRight'] as const;

interface SortableReorderOptions {
  /** Sortable `draggable` selector for items. */
  draggable: string;
  handle?: string;
  /** Presses here never start a drag; their click still fires. */
  filter?: string;
  direction?: 'horizontal' | 'vertical';
  /** Default true. False keeps the drag clone inside a portalled layer (popover). */
  fallbackOnBody?: boolean;
  disabled?: () => boolean;
  /** Alt+arrow moves the focused item one place (`list.move*`); items must be focusable. */
  keyboard?: boolean;
  /** With `keyboard`: a move fires only when focus sits in this selector's element inside the
   *  item. For items that nest other focusable rows (a collection holds script rows). */
  keyboardHandle?: string;
  /** Lists sharing a name exchange items. Omitted keeps the list isolated. */
  group?: string;
  /** With `group`: the id of a dragged element, read on the list it lands in. */
  itemId?: (el: HTMLElement) => string | null;
  /** With `group`: an item from another list was dropped here, before `beforeId` (null = last). */
  onAdd?: (id: string, beforeId: string | null) => void;
  /** Whether a drop outside the viewport hands the item to `onDropOutside` (tab tear-off). */
  canDropOutside?: (id: string) => boolean;
  /** The item was released outside the viewport. `grab*` is where the press that started the drag
   *  landed; both points are client coordinates. */
  onDropOutside?: (id: string, drop: OutsideDrop) => void;
}

/** A release outside the viewport: client coordinates (CSS px), possibly negative. */
export interface OutsideDrop {
  clientX: number;
  clientY: number;
  grabX: number;
  grabY: number;
}

function isOutside(x: number, y: number): boolean {
  return x < 0 || y < 0 || x >= window.innerWidth || y >= window.innerHeight;
}

/** P137's TabStrip pattern, shared: vue-draggable-plus in forceFallback mode bound to an id mirror
 *  (the library reverts its own DOM move only when a list is bound), one onMove per drop. */
export function useSortableReorder(
  container: Ref<HTMLElement | null>,
  ids: () => readonly string[],
  onMove: (fromId: string, toId: string) => void,
  options: SortableReorderOptions,
): { dragging: Readonly<Ref<boolean>>; outside: Readonly<Ref<boolean>> } {
  const rowIds = shallowRef<string[]>([]);
  watch(
    ids,
    (v) => {
      rowIds.value = [...v];
    },
    { immediate: true },
  );
  const dragging = shallowRef(false);
  // True while a drag that may leave the window (canDropOutside) has its pointer outside the viewport.
  const outside = shallowRef(false);
  let grab = { x: 0, y: 0, pointerId: 0 };
  let last = { x: 0, y: 0 };
  const canLeave = (id: string | null | undefined) =>
    !!id && !!options.onDropOutside && (options.canDropOutside?.(id) ?? true);
  let draggedId: string | null = null;
  const tearing = () => canLeave(draggedId);
  const idOf = (el: HTMLElement) => options.itemId?.(el) ?? el.dataset.tabId ?? null;

  const sortable = useDraggable(
    container,
    rowIds,
    computed(() => ({
      // Bound below once the element exists: a portalled container is absent at mount.
      immediate: false,
      draggable: options.draggable,
      handle: options.handle,
      filter: options.filter,
      preventOnFilter: false,
      direction: options.direction,
      disabled: options.disabled?.() ?? false,
      group: options.group
        ? { name: options.group, pull: true, put: true }
        : { name: 'sortable-reorder', pull: false, put: false },
      forceFallback: true,
      fallbackOnBody: options.fallbackOnBody ?? true,
      fallbackTolerance: DRAG_TOLERANCE_PX,
      ghostClass: SORTABLE_GHOST_CLASS,
      onStart: (evt) => {
        dragging.value = true;
        dragCancelled = false;
        draggedId = idOf(evt.item);
        last = { x: grab.x, y: grab.y };
        // Keeps pointermove/pointerup flowing once the pointer leaves the viewport, so a drag always ends.
        try {
          evt.item.setPointerCapture(grab.pointerId);
        } catch {
          // The pointer is already gone, or the element cannot capture; the drag still ends.
        }
      },
      onEnd: (evt) => {
        const wasOutside = outside.value;
        dragging.value = false;
        outside.value = false;
        // Sortable swallows the click after a fallback drag; a release outside the viewport makes
        // none, so spend it here or the next real click is eaten.
        if (isOutside(last.x, last.y))
          document.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }));
        const id = idOf(evt.item);
        if (dragCancelled || !wasOutside || !id || !canLeave(id)) return;
        rowIds.value = [...ids()];
        options.onDropOutside?.(id, {
          clientX: last.x,
          clientY: last.y,
          grabX: grab.x,
          grabY: grab.y,
        });
      },
      // The library has already moved its id mirror and DOM; the real order only changes when the
      // backend snapshot lands, so put both back and let that snapshot re-render.
      onAdd: (evt) => {
        const id = options.itemId?.(evt.item);
        const beforeId = ids()[evt.newDraggableIndex ?? -1] ?? null;
        rowIds.value = [...ids()];
        if (!dragCancelled && id) options.onAdd?.(id, beforeId);
      },
      onRemove: () => {
        rowIds.value = [...ids()];
      },
      onUpdate: (evt) => {
        const current = ids();
        if (dragCancelled || outside.value) {
          rowIds.value = [...current];
          return;
        }
        const from = current[evt.oldDraggableIndex ?? -1];
        const to = current[evt.newDraggableIndex ?? -1];
        if (from && to && from !== to) onMove(from, to);
      },
    })),
  );

  useEventListener(container, 'pointerdown', (e: PointerEvent) => {
    grab = { x: e.clientX, y: e.clientY, pointerId: e.pointerId };
  });
  useEventListener(document, 'pointermove', (e: PointerEvent) => {
    if (!dragging.value) return;
    last = { x: e.clientX, y: e.clientY };
    outside.value = tearing() && isOutside(e.clientX, e.clientY);
  });
  // The release itself may land past the last move; its coordinates win.
  useEventListener(
    document,
    'pointerup',
    (e: PointerEvent) => {
      if (!dragging.value) return;
      last = { x: e.clientX, y: e.clientY };
      outside.value = tearing() && isOutside(e.clientX, e.clientY);
    },
    { capture: true },
  );

  // Esc during a drag drops the item back where it started.
  useEventListener(document, 'keydown', (e: KeyboardEvent) => {
    if (dragging.value && e.key === 'Escape') dragCancelled = true;
  });

  if (options.keyboard) {
    const axisIds = options.direction === 'horizontal' ? HORIZONTAL_IDS : VERTICAL_IDS;
    useEventListener(container, 'keydown', (e: KeyboardEvent) => {
      if (e.isComposing || e.defaultPrevented || options.disabled?.()) return;
      const id = shortcutFor(e, axisIds);
      const el = (e.target as HTMLElement | null)?.closest<HTMLElement>(options.draggable);
      if (id === null || !el || !container.value?.contains(el)) return;
      if (options.keyboardHandle) {
        const handle = (e.target as HTMLElement).closest(options.keyboardHandle);
        if (!handle || !el.contains(handle)) return;
      }
      const items = [...container.value.querySelectorAll<HTMLElement>(options.draggable)];
      const current = ids();
      const from = current[items.indexOf(el)];
      const step = id === 'list.moveUp' || id === 'list.moveLeft' ? -1 : 1;
      const to = current[items.indexOf(el) + step];
      if (!from || !to) return;
      e.preventDefault();
      onMove(from, to);
      // A keyed row keeps its element across the reorder; the move can drop its focus. Refocus the
      // element that held it (a chip's inner button, not the unfocusable chip).
      const focused = e.target as HTMLElement;
      void nextTick().then(() => {
        if (focused.isConnected) focused.focus();
      });
    });
  }

  watch(
    container,
    (el) => {
      if (el) sortable.start(el);
    },
    { immediate: true, flush: 'post' },
  );

  return { dragging, outside };
}
