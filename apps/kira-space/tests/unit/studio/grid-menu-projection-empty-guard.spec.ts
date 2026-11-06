// P21 round 1 functional finding F1: the column projection could be driven to an empty (but
// non-nil) slice via three separate menu paths — Columns▸None on a PK-less table, repeated
// Hide-column with no PK guard, and "Add to projection" from an empty starting state —
// and ResolveProjection treats non-nil-empty as "zero columns", so every SQL adapter then emits
// `SELECT  FROM ...`. These are the three pure decision functions the fix routes through, tested
// at their own boundary rather than through setProjection's real load() pipeline.
//
// menu.ts transitively reaches bridge/control/bridge/data (via ./pendingChanges/./state), so this
// has to be a dynamic import(), after @workbench/testing/unit/window's mock.module registration has run — the
// same pattern row-values-visible-span.spec.ts already uses.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';

const { nextProjectionAfterHidingColumn, nextProjectionFromSelectedColumns } = await import(
  '../../../frontend/src/studio/views/grid/menu'
);
const { nextProjectionAfterAddingColumn } = await import(
  '../../../frontend/src/studio/views/definition/menu'
);

describe('nextProjectionFromSelectedColumns (ColumnsMenu.vue "None"/"All")', () => {
  test('selecting every column resolves to null ("all columns"), not an explicit full list', () => {
    expect(nextProjectionFromSelectedColumns(['a', 'b'], ['a', 'b'])).toBeNull();
  });

  test('selecting nothing (a PK-less relation\'s "None") also resolves to null, never []', () => {
    expect(nextProjectionFromSelectedColumns([], ['a', 'b', 'c'])).toBeNull();
  });

  test('a genuine narrower selection is returned as-is', () => {
    expect(nextProjectionFromSelectedColumns(['a'], ['a', 'b', 'c'])).toEqual(['a']);
  });
});

describe('nextProjectionAfterHidingColumn ("Hide column")', () => {
  test('hiding the only remaining column falls back to null, never []', () => {
    expect(nextProjectionAfterHidingColumn(['only'], ['only', 'b'], 'only')).toBeNull();
  });

  test('hiding one column of several narrows the projection', () => {
    expect(nextProjectionAfterHidingColumn(['a', 'b'], ['a', 'b', 'c'], 'a')).toEqual(['b']);
  });

  test('starting from an implicit "all columns" (null) hides down from the full set', () => {
    expect(nextProjectionAfterHidingColumn(null, ['a', 'b'], 'a')).toEqual(['b']);
  });

  test('hiding columns one at a time on a PK-less two-column relation eventually falls back to null', () => {
    expect(nextProjectionAfterHidingColumn(['b'], ['a', 'b'], 'b')).toBeNull();
  });
});

describe('nextProjectionAfterAddingColumn ("Add to projection")', () => {
  test('starting from "all columns" (null) is a no-op — the column is already shown', () => {
    // This is the exact inversion bug: the old code turned "everything" into "just this column".
    expect(nextProjectionAfterAddingColumn(null, 'name')).toBeNull();
  });

  test('a column already in a narrower projection is a no-op', () => {
    expect(nextProjectionAfterAddingColumn(['id'], 'id')).toBeNull();
  });

  test('a column not yet in a narrower projection is appended', () => {
    expect(nextProjectionAfterAddingColumn(['id'], 'name')).toEqual(['id', 'name']);
  });
});
