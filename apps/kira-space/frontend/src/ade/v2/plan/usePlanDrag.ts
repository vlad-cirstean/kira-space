import { useMouse, useRafFn } from '@vueuse/core';
import { type Ref, ref } from 'vue';
import type { DropTarget } from '../board/dropPlan';

// Shared drag state for the day bands (each band owns a SortableJS list, see AdeDayBand). While a
// drag runs the target is resolved from live card geometry, not the element under the pointer, so
// gaps, headings and padding all land on the nearest slot; the caller turns the released drag into
// one plan write.

function bandOf(root: HTMLElement, y: number): HTMLElement | null {
  let best: HTMLElement | null = null;
  let bestDist = Number.POSITIVE_INFINITY;
  for (const band of root.querySelectorAll<HTMLElement>('[data-ade-band]')) {
    const r = band.getBoundingClientRect();
    const dist = y < r.top ? r.top - y : y > r.bottom ? y - r.bottom : 0;
    if (dist < bestDist) {
      best = band;
      bestDist = dist;
    }
  }
  return best;
}

/** Slot under the pointer: before the first card whose midpoint is below it, else the band's end. */
export function resolveSlot(
  root: HTMLElement,
  x: number,
  y: number,
  draggedId: string,
): DropTarget | null {
  const rr = root.getBoundingClientRect();
  if (x < rr.left || x > rr.right || y < rr.top || y > rr.bottom) return null;
  const band = bandOf(root, y);
  if (!band || band.dataset.adeDay === undefined) return null;
  const cards = [...band.querySelectorAll<HTMLElement>('[data-ade-task]')];
  const at = cards.findIndex((c) => {
    if (c.dataset.taskId === draggedId) return false;
    const r = c.getBoundingClientRect();
    return r.top + r.height / 2 > y;
  });
  const dragged = cards.findIndex((c) => c.dataset.taskId === draggedId);
  const before = at >= 0 ? cards[at] : undefined;
  // Dropping where the card already sits is a no-op: no placeholder, no write.
  if (dragged >= 0) {
    if (before === undefined && dragged === cards.length - 1) return null;
    if (before !== undefined && at === dragged + 1) return null;
  }
  if (before?.dataset.taskId) return { kind: 'card', taskId: before.dataset.taskId };
  return { kind: 'band', day: Number(band.dataset.adeDay) };
}

function keyOf(t: DropTarget | null): string {
  if (!t) return 'none';
  return t.kind === 'card' ? `card:${t.taskId}` : `band:${t.day}`;
}

export function usePlanDrag(
  root: Ref<HTMLElement | null>,
  onDrop: (taskId: string, target: DropTarget | null) => void,
) {
  const draggedId = ref<string | null>(null);
  const target = ref<DropTarget | null>(null);
  const { x, y } = useMouse({ type: 'client' });
  // Re-resolved every frame so auto-scroll under a still pointer keeps the slot right; the ref
  // changes only when the slot does.
  const { pause, resume } = useRafFn(
    () => {
      const el = root.value;
      const id = draggedId.value;
      if (!el || !id) return;
      const next = resolveSlot(el, x.value, y.value, id);
      if (keyOf(next) !== keyOf(target.value)) target.value = next;
    },
    { immediate: false },
  );

  function begin(taskId: string): void {
    draggedId.value = taskId;
    target.value = null;
    resume();
  }

  function end(): void {
    const id = draggedId.value;
    const t = target.value;
    pause();
    draggedId.value = null;
    target.value = null;
    if (id) onDrop(id, t);
  }

  return { draggedId, target, begin, end };
}
