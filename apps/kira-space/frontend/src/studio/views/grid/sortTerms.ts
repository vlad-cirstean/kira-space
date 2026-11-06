// queries.ts's own doc comment: typing in the ORDER BY box always produces a `text` sort and
// "clears the header indicators" — true for the machine-driven distinction that keeps pagination
// on `offset`, but a user who types "a asc, b desc" still expects the headers to reflect it.
// This best-effort, display-only parse recovers per-column direction + position for exactly that
// case; a term that doesn't match a real column name (an expression, a typo) is silently skipped
// rather than guessed at. It never feeds back into `state.sort` — the text/structured split for
// pagination purposes is untouched.
export function parseTextSortTerms(
  text: string,
  knownColumns: readonly string[],
): { column: string; direction: 'asc' | 'desc' }[] {
  const known = new Set(knownColumns);
  const terms: { column: string; direction: 'asc' | 'desc' }[] = [];
  for (const raw of text.split(',')) {
    const trimmed = raw.trim();
    if (!trimmed) continue;
    const match = trimmed.match(/^(?:"([^"]+)"|`([^`]+)`|(\w+)\b)\s*(desc|asc)?/i);
    const name = match?.[1] ?? match?.[2] ?? match?.[3];
    if (!name || !known.has(name)) continue;
    terms.push({ column: name, direction: match?.[4]?.toLowerCase() === 'desc' ? 'desc' : 'asc' });
  }
  return terms;
}
