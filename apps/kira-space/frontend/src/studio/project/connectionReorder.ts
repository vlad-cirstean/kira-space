import {
  type EdgeDrop,
  orderAfterDrop,
  useTreePointerDrag,
} from '@workbench/behaviour/treePointerDrag';
import { computed, type Ref } from 'vue';

const CONNECTION_ROW = '[role="treeitem"][data-kind="connection"]';

export type ConnectionDrop = EdgeDrop;

/**
 * Pointer drag reorder for the virtualized project tree, on the shared tree pointer drag. A drop
 * lands before or after the connection row under the pointer.
 */
export function useConnectionReorder(opts: {
  container: Ref<HTMLElement | null>;
  ids: () => readonly string[];
  disabled: () => boolean;
  rowHeight: () => number;
  onReorder: (ids: string[]) => void;
}): {
  draggingId: Readonly<Ref<string | null>>;
  drop: Readonly<Ref<ConnectionDrop | null>>;
} {
  let draggedId: string | null = null;
  const { dragging, drop } = useTreePointerDrag<ConnectionDrop>({
    container: opts.container,
    pressable: (target) => target.closest<HTMLElement>(CONNECTION_ROW),
    disabled: opts.disabled,
    rowHeight: opts.rowHeight,
    onStart: (pressed) => {
      draggedId = pressed.dataset.connectionId ?? null;
      return draggedId !== null;
    },
    resolve: (el, y) => {
      const row = el?.closest<HTMLElement>(CONNECTION_ROW);
      const id = row?.dataset.connectionId;
      if (!row || !id || id === draggedId) return null;
      const rect = row.getBoundingClientRect();
      return { id, edge: y < rect.top + rect.height / 2 ? 'top' : 'bottom' };
    },
    onDrop: (source, target) => {
      const dragged = source.dataset.connectionId;
      if (!dragged) return;
      const current = opts.ids();
      const next = orderAfterDrop(current, dragged, target);
      if (next.some((id, i) => id !== current[i])) opts.onReorder(next);
    },
  });
  const draggingId = computed(() => dragging.value?.dataset.connectionId ?? null);
  return { draggingId, drop };
}
