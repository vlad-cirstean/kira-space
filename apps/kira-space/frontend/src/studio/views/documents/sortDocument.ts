import type { SortSpec } from '@shared/domain/queries';

// The Mongo dialect of FilterToolbar.vue's ORDER BY box: unlike SQL's free-text sort, Mongo's
// read.ts explicitly rejects a `{kind:'text'}` sort (a free-text expression has no server-side
// meaning here) — so this box parses straight into the structured form itself, never the text
// variant, mirroring how clicking a grid column header builds structured terms. The box's own
// syntax is Mongo's own sort-document shape (`{ field: 1, field2: -1 }`, `1` ascending / `-1`
// descending — a `db.collection.find().sort(...)` argument, not SQL's `ORDER BY field ASC`) so a
// Mongo user can type what they already know; sortSpecToText/parseSortText are this box's own
// serializer/parser and must round-trip each other exactly.
// P21 round 3 functional finding 5: the bare-key alternative below (SORT_TERM_RE) only matches
// `[A-Za-z0-9_.$]+` — the same set this function used to emit unconditionally, regardless of what
// the field name actually was. A field name outside that set (a hyphen, a space, `@`, non-ASCII)
// round-tripped through its own quoted form exactly once (typed by the user, or produced by this
// function while it still matched the field), but the very next re-serialisation dropped the
// quotes — the header comment's own "must round-trip each other exactly" contract, violated by
// this function alone. JSON.stringify both decides whether quoting is needed by construction (it
// always quotes) and produces a form parseSortText's own `"([^"]+)"` alternative can read back
// unless the name itself contains a literal `"` (not supported by the parser's regex either way).
function formatSortKey(column: string): string {
  return /^[A-Za-z0-9_.$]+$/.test(column) ? column : JSON.stringify(column);
}

export function sortSpecToText(sort: SortSpec | null): string {
  if (sort?.kind !== 'structured' || sort.terms.length === 0) return '';
  const body = sort.terms
    .map((t) => `${formatSortKey(t.column)}: ${t.direction === 'desc' ? -1 : 1}`)
    .join(', ');
  return `{ ${body} }`;
}

// Lenient by design (matches how a Mongo shell user types a sort document): braces are
// optional, keys may be bare or quoted (single or double), and `asc`/`desc` are tolerated
// alongside Mongo's own `1`/`-1` for anyone still transitioning off the old ORDER BY-style box.
// Anything that isn't a `key: value` pair is skipped rather than rejecting the whole
// string, since a half-typed sort document while the user is still editing is not an error.
const SORT_TERM_RE = /(?:"([^"]+)"|'([^']+)'|([A-Za-z0-9_.$]+))\s*:\s*(-?1|asc|desc)/gi;

export function parseSortText(text: string): SortSpec | null {
  const terms: { column: string; direction: 'asc' | 'desc' }[] = [];
  for (const match of text.matchAll(SORT_TERM_RE)) {
    const column = match[1] ?? match[2] ?? match[3];
    if (!column) continue;
    const value = match[4].toLowerCase();
    const direction: 'asc' | 'desc' = value === '-1' || value === 'desc' ? 'desc' : 'asc';
    terms.push({ column, direction });
  }
  return terms.length > 0 ? { kind: 'structured', terms } : null;
}
