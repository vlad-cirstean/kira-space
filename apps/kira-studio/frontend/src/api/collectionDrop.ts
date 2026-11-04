import type { CollectionItemSummary } from '@shared/domain/collections';
import type { CollectionRowVm } from './state/collections';

export type DropZone = 'before' | 'into' | 'after';

export interface DropTarget {
  key: string;
  zone: DropZone;
}

/** Where moved items land. `beforeId` null appends. */
export interface Placement {
  collectionId: string;
  parentId: string | null;
  beforeId: string | null;
}

/** Zone under the pointer, `fraction` being its height from the row top (0..1). While a search
 *  filters the tree only `into` is allowed: reordering against a filtered list hides siblings. */
export function zoneFor(
  row: CollectionRowVm,
  fraction: number,
  searching: boolean,
): DropZone | null {
  if (row.kind === 'collection') return 'into';
  if (searching) return row.kind === 'folder' ? 'into' : null;
  if (row.kind === 'request') return fraction < 0.5 ? 'before' : 'after';
  if (fraction < 0.25) return 'before';
  // An expanded folder's next row is its first child, so "after" would draw in the wrong place.
  if (fraction >= 0.75 && !(row.expanded && row.hasChildren)) return 'after';
  return 'into';
}

function byIdMap(items: readonly CollectionItemSummary[]): Map<string, CollectionItemSummary> {
  return new Map(items.map((item) => [item.id, item]));
}

function hasAncestorIn(
  item: CollectionItemSummary,
  set: ReadonlySet<string>,
  byId: ReadonlyMap<string, CollectionItemSummary>,
): boolean {
  for (let p = item.parentId; p; p = byId.get(p)?.parentId ?? null) {
    if (set.has(p)) return true;
  }
  return false;
}

/** Rows that survive after dropping any whose ancestor (folder or collection) is also in `rows`:
 *  moving or deleting the ancestor carries them. Keeps `rows` order. */
export function topmost(
  rows: readonly CollectionRowVm[],
  items: readonly CollectionItemSummary[],
): CollectionRowVm[] {
  const byId = byIdMap(items);
  const collections = new Set(rows.filter((r) => r.kind === 'collection').map((r) => r.id));
  const picked = new Set(rows.filter((r) => r.kind !== 'collection').map((r) => r.id));
  return rows.filter((row) => {
    if (row.kind === 'collection') return true;
    if (collections.has(row.collectionId)) return false;
    const item = byId.get(row.id);
    return !item || !hasAncestorIn(item, picked, byId);
  });
}

function siblingsOf(
  items: readonly CollectionItemSummary[],
  collectionId: string,
  parentId: string | null,
): CollectionItemSummary[] {
  return items
    .filter((i) => i.collectionId === collectionId && i.parentId === parentId)
    .sort((a, b) => a.sortOrder - b.sortOrder);
}

/** True when every moved item already sits where `place` would put it, in the same order. */
function isNoop(
  place: Placement,
  moved: ReadonlySet<string>,
  byId: ReadonlyMap<string, CollectionItemSummary>,
  items: readonly CollectionItemSummary[],
): boolean {
  for (const id of moved) {
    const item = byId.get(id);
    if (!item || item.collectionId !== place.collectionId || item.parentId !== place.parentId) {
      return false;
    }
  }
  const current = siblingsOf(items, place.collectionId, place.parentId).map((i) => i.id);
  const result: string[] = [];
  let placed = false;
  const drop = (): void => {
    result.push(...moved);
    placed = true;
  };
  for (const id of current) {
    if (moved.has(id)) continue;
    if (id === place.beforeId) drop();
    result.push(id);
  }
  if (!placed) drop();
  return result.every((id, i) => id === current[i]);
}

/** Where `moved` (item ids, topmost, in tree order) land when dropped on `target`. Null when the
 *  drop is refused: onto a moved item or its descendant, into a request, or a move that changes
 *  nothing. */
export function placementFor(
  target: DropTarget,
  moved: ReadonlySet<string>,
  items: readonly CollectionItemSummary[],
): Placement | null {
  const byId = byIdMap(items);
  let place: Placement;
  if (target.key.startsWith('c:')) {
    if (target.zone !== 'into') return null;
    place = { collectionId: target.key.slice(2), parentId: null, beforeId: null };
  } else {
    const item = byId.get(target.key.slice(2));
    if (!item || moved.has(item.id) || hasAncestorIn(item, moved, byId)) return null;
    if (target.zone === 'into') {
      if (item.kind !== 'folder') return null;
      place = { collectionId: item.collectionId, parentId: item.id, beforeId: null };
    } else {
      const siblings = siblingsOf(items, item.collectionId, item.parentId);
      let i = siblings.findIndex((s) => s.id === item.id) + (target.zone === 'after' ? 1 : 0);
      while (i < siblings.length && moved.has(siblings[i].id)) i++;
      place = {
        collectionId: item.collectionId,
        parentId: item.parentId,
        beforeId: siblings[i]?.id ?? null,
      };
    }
  }
  return isNoop(place, moved, byId, items) ? null : place;
}
