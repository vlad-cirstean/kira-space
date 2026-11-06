import type { CollectionItemSummary } from '@shared/domain/collections';

/** `order` with `id` swapped one place toward `step`, or null at the end. */
export function collectionStepOrder(
  order: readonly string[],
  id: string,
  step: -1 | 1,
): string[] | null {
  const at = order.indexOf(id);
  const to = at + step;
  if (at < 0 || to < 0 || to >= order.length) return null;
  const next = [...order];
  [next[at], next[to]] = [next[to], next[at]];
  return next;
}

/** The `beforeId` that moves `id` one place within its (sorted) siblings; undefined at the end. */
export function itemStepBefore(
  siblings: readonly CollectionItemSummary[],
  id: string,
  step: -1 | 1,
): string | null | undefined {
  const at = siblings.findIndex((s) => s.id === id);
  const to = at + step;
  if (at < 0 || to < 0 || to >= siblings.length) return undefined;
  return step < 0 ? siblings[at - 1].id : (siblings[at + 2]?.id ?? null);
}
