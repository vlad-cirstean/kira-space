// ProjectionMenu.vue's own close-time decision — an untouched close silently
// cleared an active projection whenever the projected page's own (already-narrowed) field set
// happened to match it exactly, and "None" wrote `[]` rather than null even though Mongo's own
// empty-projection behaviour returns every field regardless. nextDocumentProjectionOnClose is the
// pure decision the fix routes through, tested at its own boundary — mirrors grid/menu.ts's own
// nextProjectionFromSelectedColumns test shape (grid-menu-projection-empty-guard.spec.ts).
import { describe, expect, test } from 'bun:test';
import { nextDocumentProjectionOnClose } from '../../../frontend/src/studio/views/documents/projection';

describe('nextDocumentProjectionOnClose', () => {
  test('an untouched close (selected still equals the opening projection) is a no-op', () => {
    // The exact bug: a projection to [a, b] leaves the page showing only _id, a, b, so
    // fieldNames/initialSelected are also [a, b] — set-equal to the active projection by
    // coincidence, not because the user selected "everything".
    const initial = new Set(['a', 'b']);
    const selected = new Set(['a', 'b']);
    expect(nextDocumentProjectionOnClose(selected, initial, 2, false)).toBeUndefined();
  });

  test('an untouched close while unprojected (fieldNames is the real full set) is also a no-op', () => {
    const initial = new Set(['a', 'b', 'c']);
    const selected = new Set(['a', 'b', 'c']);
    expect(nextDocumentProjectionOnClose(selected, initial, 3, false)).toBeUndefined();
  });

  test('a genuine narrowing (deselecting one field) is returned as an explicit list', () => {
    const initial = new Set(['a', 'b', 'c']);
    const selected = new Set(['a', 'b']);
    expect(nextDocumentProjectionOnClose(selected, initial, 3, false)).toEqual(['a', 'b']);
  });

  test('an explicit All press clears the projection even when its own value is set-equal to the opening one', () => {
    const initial = new Set(['a', 'b']);
    const selected = new Set(['a', 'b']);
    expect(nextDocumentProjectionOnClose(selected, initial, 2, true)).toBeNull();
  });

  test('a set-size match with no explicit All press is never read as "everything"', () => {
    const initial = new Set(['a', 'b']);
    const selected = new Set(['a', 'c']); // same size as fieldNames, different members, not All
    expect(nextDocumentProjectionOnClose(selected, initial, 2, false)).toEqual(['a', 'c']);
  });

  test('an empty selection resolves to null, never []', () => {
    const initial = new Set(['a', 'b']);
    const selected = new Set<string>();
    expect(nextDocumentProjectionOnClose(selected, initial, 2, false)).toBeNull();
  });

  test('an empty selection resolves to null even via an explicit None after an explicit All', () => {
    const initial = new Set(['a', 'b']);
    const selected = new Set<string>();
    expect(nextDocumentProjectionOnClose(selected, initial, 2, true)).toBeNull();
  });
});
