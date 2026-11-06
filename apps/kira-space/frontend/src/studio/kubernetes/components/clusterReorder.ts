import {
  type EdgeDrop,
  orderAfterDrop,
  useTreePointerDrag,
} from '@workbench/behaviour/treePointerDrag';
import { computed, type Ref } from 'vue';

const CLUSTER_ROW = '[data-kind="cluster"]';

/**
 * Pointer drag reorder for the cluster list, on the shared tree pointer drag. A drop lands before
 * or after the cluster row under the pointer; elsewhere is no target.
 */
export function useClusterReorder(opts: {
  container: Ref<HTMLElement | null>;
  ids: () => readonly string[];
  disabled: () => boolean;
  rowHeight: () => number;
  onReorder: (ids: string[]) => void;
}): {
  draggingId: Readonly<Ref<string | null>>;
  drop: Readonly<Ref<EdgeDrop | null>>;
} {
  let draggedId: string | null = null;
  const { dragging, drop } = useTreePointerDrag<EdgeDrop>({
    container: opts.container,
    pressable: (target) => target.closest<HTMLElement>(CLUSTER_ROW),
    disabled: opts.disabled,
    rowHeight: opts.rowHeight,
    onStart: (pressed) => {
      draggedId = pressed.dataset.clusterId ?? null;
      return draggedId !== null;
    },
    resolve: (el, y) => {
      const row = el?.closest<HTMLElement>(CLUSTER_ROW);
      const id = row?.dataset.clusterId;
      if (!row || !id || id === draggedId) return null;
      const rect = row.getBoundingClientRect();
      return { id, edge: y < rect.top + rect.height / 2 ? 'top' : 'bottom' };
    },
    onDrop: (source, target) => {
      const dragged = source.dataset.clusterId;
      if (!dragged) return;
      const current = opts.ids();
      const next = orderAfterDrop(current, dragged, target);
      if (next.some((id, i) => id !== current[i])) opts.onReorder(next);
    },
  });
  const draggingId = computed(() => dragging.value?.dataset.clusterId ?? null);
  return { draggingId, drop };
}
