import type { CollectionItemSummary } from '@shared/domain/collections';

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
