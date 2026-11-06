// sortSpecToText always emitted bare, unquoted keys, while
// parseSortText's bare-key alternative only matches [A-Za-z0-9_.$]+ — the file's own header
// comment states the two "must round-trip each other exactly", but a field name outside that set
// (a hyphen, a space, `@`, non-ASCII) survived parsing only in its quoted form and was then
// re-emitted unquoted, so the *next* re-parse silently matched a different, shorter bare key and
// committed a sort by a field no document has.
import { describe, expect, test } from 'bun:test';
import type { SortSpec } from '@shared/domain/queries';
import {
  parseSortText,
  sortSpecToText,
} from '../../../frontend/src/studio/views/documents/sortDocument';

function structured(column: string, direction: 'asc' | 'desc' = 'desc'): SortSpec {
  return { kind: 'structured', terms: [{ column, direction }] };
}

describe('sortSpecToText / parseSortText round-trip (finding 5)', () => {
  test('a bare-safe key is emitted unquoted, as before', () => {
    expect(sortSpecToText(structured('first_name'))).toBe('{ first_name: -1 }');
  });

  test('a key containing a hyphen is quoted on output', () => {
    expect(sortSpecToText(structured('first-name'))).toBe('{ "first-name": -1 }');
  });

  test('a key containing a space is quoted on output', () => {
    expect(sortSpecToText(structured('display name'))).toBe('{ "display name": -1 }');
  });

  test('re-parsing the serialized text recovers the exact same sort — the round-trip the header comment promises', () => {
    for (const column of [
      'first-name',
      'display name',
      'e@mail',
      'first_name',
      'a.b.c',
      '$field',
    ]) {
      const spec = structured(column, 'desc');
      const text = sortSpecToText(spec);
      expect(parseSortText(text)).toEqual(spec);
    }
  });

  test("the exact failure scenario: typing a quoted sort, then re-parsing the box's own re-rendered text, sorts by the same field — not a truncated bare match", () => {
    // 1. The user types a quoted sort for a hyphenated field.
    const typed = parseSortText('{ "first-name": -1 }');
    expect(typed).toEqual(structured('first-name', 'desc'));

    // 2. The box re-renders itself from the committed sort (DocumentView.vue's own watcher).
    const rerendered = sortSpecToText(typed);

    // 3. The user presses Enter again (or edits anything else) — re-parsing the box's own
    //    rendered text must recover the same field, not silently match a shorter bare key
    //    (the pre-fix bug: "{ first-name: -1 }" re-parsed as column "name").
    const reparsed = parseSortText(rerendered);
    expect(reparsed).toEqual(structured('first-name', 'desc'));
  });
});
