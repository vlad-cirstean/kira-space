// Drop rules of the collections tree: zone thresholds, which selected rows ride along with an
// ancestor, and where a drop lands. Pure arithmetic over the tree list, so no store is mounted.
import { describe, expect, test } from 'bun:test';
import type { CollectionItemSummary } from '@shared/domain/collections';
import { placementFor, topmost, zoneFor } from '../../frontend/src/api/collectionDrop';
import type { CollectionRowVm } from '../../frontend/src/api/state/collections';

function item(
  id: string,
  kind: 'folder' | 'request',
  parentId: string | null,
  sortOrder: number,
  collectionId = 'c1',
): CollectionItemSummary {
  return {
    id,
    collectionId,
    parentId,
    kind,
    name: id,
    sortOrder,
    method: kind === 'request' ? 'GET' : '',
    url: '',
    protocol: 'http',
    createdAt: '',
    updatedAt: '',
  };
}

function row(
  kind: CollectionRowVm['kind'],
  id: string,
  extra: Partial<CollectionRowVm> = {},
): CollectionRowVm {
  return {
    key: `${kind === 'collection' ? 'c' : 'i'}:${id}`,
    depth: 1,
    hasChildren: false,
    expanded: false,
    kind,
    id,
    collectionId: 'c1',
    parentId: null,
    name: id,
    method: '',
    url: '',
    protocol: 'http',
    matched: false,
    ...extra,
  };
}

// c1: F (a, G (g1)), b, c      c2: d
const ITEMS = [
  item('F', 'folder', null, 0),
  item('a', 'request', 'F', 0),
  item('G', 'folder', 'F', 1),
  item('g1', 'request', 'G', 0),
  item('b', 'request', null, 1),
  item('c', 'request', null, 2),
  item('d', 'request', null, 0, 'c2'),
];

describe('zoneFor', () => {
  test('a collection row is always into', () => {
    expect(zoneFor(row('collection', 'c1'), 0.05, false)).toBe('into');
  });

  test('a request splits at the middle', () => {
    expect(zoneFor(row('request', 'b'), 0.49, false)).toBe('before');
    expect(zoneFor(row('request', 'b'), 0.5, false)).toBe('after');
  });

  test('a folder is into between its quarters', () => {
    const folder = row('folder', 'F');
    expect(zoneFor(folder, 0.24, false)).toBe('before');
    expect(zoneFor(folder, 0.25, false)).toBe('into');
    expect(zoneFor(folder, 0.74, false)).toBe('into');
    expect(zoneFor(folder, 0.75, false)).toBe('after');
  });

  test('an expanded folder with children has no after zone', () => {
    const open = row('folder', 'F', { expanded: true, hasChildren: true });
    expect(zoneFor(open, 0.9, false)).toBe('into');
    expect(zoneFor(open, 0.1, false)).toBe('before');
  });

  test('a search leaves only into, and none for a request', () => {
    expect(zoneFor(row('folder', 'F'), 0.1, true)).toBe('into');
    expect(zoneFor(row('request', 'b'), 0.1, true)).toBeNull();
  });
});

describe('topmost', () => {
  test('drops rows under a selected folder or collection', () => {
    const rows = [row('folder', 'F'), row('request', 'a'), row('folder', 'G'), row('request', 'b')];
    expect(topmost(rows, ITEMS).map((r) => r.id)).toEqual(['F', 'b']);
    const withCollection = [
      row('collection', 'c1'),
      row('request', 'b'),
      row('request', 'd', { collectionId: 'c2' }),
    ];
    expect(topmost(withCollection, ITEMS).map((r) => r.id)).toEqual(['c1', 'd']);
  });
});

describe('placementFor', () => {
  const into = (key: string) => ({ key, zone: 'into' as const });

  test('into a collection appends to its root', () => {
    expect(placementFor(into('c:c2'), new Set(['b']), ITEMS)).toEqual({
      collectionId: 'c2',
      parentId: null,
      beforeId: null,
    });
  });

  test('before and after resolve to the sibling to insert before', () => {
    expect(placementFor({ key: 'i:b', zone: 'before' }, new Set(['c']), ITEMS)?.beforeId).toBe('b');
    expect(placementFor({ key: 'i:b', zone: 'after' }, new Set(['a']), ITEMS)?.beforeId).toBe('c');
    expect(placementFor({ key: 'i:c', zone: 'after' }, new Set(['a']), ITEMS)?.beforeId).toBeNull();
  });

  test('after skips forward past moved siblings', () => {
    const place = placementFor({ key: 'i:F', zone: 'after' }, new Set(['b', 'a']), ITEMS);
    expect(place).toEqual({ collectionId: 'c1', parentId: null, beforeId: 'c' });
  });

  test('refuses a drop onto a moved item or its descendant at depth', () => {
    expect(placementFor(into('i:F'), new Set(['F']), ITEMS)).toBeNull();
    expect(placementFor(into('i:G'), new Set(['F']), ITEMS)).toBeNull();
    expect(placementFor({ key: 'i:g1', zone: 'before' }, new Set(['F']), ITEMS)).toBeNull();
  });

  test('refuses into a request', () => {
    expect(placementFor(into('i:b'), new Set(['c']), ITEMS)).toBeNull();
  });

  test('refuses a drop that changes nothing', () => {
    expect(placementFor({ key: 'i:c', zone: 'before' }, new Set(['b']), ITEMS)).toBeNull();
    expect(placementFor({ key: 'i:b', zone: 'after' }, new Set(['b']), ITEMS)).toBeNull();
    expect(placementFor(into('i:F'), new Set(['a', 'G']), ITEMS)).toBeNull();
    // Same parent, different order is a move.
    expect(placementFor({ key: 'i:F', zone: 'before' }, new Set(['c']), ITEMS)).not.toBeNull();
  });
});
