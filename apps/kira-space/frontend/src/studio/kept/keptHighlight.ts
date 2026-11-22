import type { KeptSite, KeptUse } from '@shared/domain/kept';
import type { RangeHighlight } from '@workbench/editor/ranges';
import { indentJson } from '../beautify';

// One mark colour for every saved variable; the name inside the token says which.
export const KEPT_MARK_CLASS = 'bg-kept-mark';
// The same tint inside Monaco, where a class name is all a decoration takes (edDecorations.css).
const KEPT_EDITOR_CLASS = 'kira-ed-kept';

export interface KeptRange {
  from: number;
  to: number;
}

interface KeptRef extends KeptRange {
  name: string;
  secret: boolean;
}

const KEPT_PREFIXES = ['plain.', 'secret.'] as const;

/** `{{plain.<name>}}` and `{{secret.<name>}}` spans of a text, same grammar as internal/keptvars RefSpans: `{{` to the next
 *  `}}`, optional `| transform`, offsets in UTF-16 units. */
export function keptRefs(text: string): KeptRef[] {
  const out: KeptRef[] = [];
  let i = 0;
  while (i < text.length) {
    const open = text.indexOf('{{', i);
    if (open < 0) break;
    const end = text.indexOf('}}', open + 2);
    if (end < 0) break;
    i = end + 2;
    const inner = text.slice(open + 2, end).trim();
    const prefix = KEPT_PREFIXES.find((p) => inner.startsWith(p));
    if (!prefix) continue;
    const name = inner.slice(prefix.length).split('|')[0]?.trim() ?? '';
    out.push({ name, secret: prefix === 'secret.', from: open, to: end + 2 });
  }
  return out;
}

/** The ranges of one field's text where the request reads a saved variable. */
export function keptRanges(
  uses: readonly KeptUse[],
  field: KeptSite['field'],
  index?: number,
  part?: string,
): KeptRange[] {
  return uses
    .flatMap((u) => u.sites)
    .filter(
      (s) =>
        s.field === field &&
        (index === undefined || s.index === index) &&
        (part === undefined || s.part === part),
    )
    .map((s) => ({ from: s.from, to: s.to }))
    .sort((a, b) => a.from - b.from);
}

const SCRIPT_VAR_RE =
  /\b(?:plain|secret)\s*(?:\.\s*[A-Za-z_$][\w$]*|\[\s*(?:"[^"\\]*"|'[^'\\]*')\s*\])/g;

/** Monaco ranges of every `plain.x` and `secret.x` token of a script, found in the text shown: same
 *  grammar as internal/keptvars ScanScript. */
export function scriptVarRanges(doc: string): readonly RangeHighlight[] {
  return Array.from(doc.matchAll(SCRIPT_VAR_RE), (m) => ({
    from: m.index,
    to: m.index + m[0].length,
    class: KEPT_EDITOR_CLASS,
  }));
}

/** Splits text on ranges for `HighlightedText`. */
export function keptParts(
  text: string,
  ranges: readonly KeptRange[],
): { text: string; match: boolean }[] {
  const out: { text: string; match: boolean }[] = [];
  let at = 0;
  for (const r of ranges) {
    if (r.from < at || r.to > text.length || r.from >= r.to) continue;
    if (r.from > at) out.push({ text: text.slice(at, r.from), match: false });
    out.push({ text: text.slice(r.from, r.to), match: true });
    at = r.to;
  }
  if (at < text.length) out.push({ text: text.slice(at), match: false });
  return out.length ? out : [{ text, match: false }];
}

/** Monaco ranges for the references of the given names in `doc`. Offsets are re-derived from the
 *  text itself: pretty-printing moves them. */
export function keptEditorRanges(names: ReadonlySet<string>) {
  return (doc: string): readonly RangeHighlight[] =>
    keptRefs(doc)
      .filter((r) => names.has(r.name))
      .map((r) => ({ from: r.from, to: r.to, class: KEPT_EDITOR_CLASS }));
}

/** Monaco ranges for the placeholders a SQL statement binds kept variables to. The offsets are of the
 *  statement as sent: any other text (the formatted copy) gets none. */
export function keptStatementRanges(uses: readonly KeptUse[], statement: string) {
  const ranges = keptRanges(uses, 'statement').map((r) => ({
    from: r.from,
    to: r.to,
    class: KEPT_EDITOR_CLASS,
  }));
  return (doc: string): readonly RangeHighlight[] => (doc === statement ? ranges : []);
}

// A number the sentinel swap uses: below 2^53, so JSON round-trips it exactly.
const SENTINEL_BASE = 7_000_000_000_000_000;

/** Pretty-prints a JSON body that holds references. An unquoted reference is not valid JSON, so each
 *  one becomes a number for the parse and the reference text returns after. Null when the body is
 *  not JSON either way. */
export function prettyKeptJson(body: string): string | null {
  const refs = keptRefs(body);
  if (refs.length === 0 || body.includes(String(SENTINEL_BASE).slice(0, 13)))
    return indentJson(body);
  let swapped = '';
  let at = 0;
  refs.forEach((r, i) => {
    swapped += body.slice(at, r.from) + String(SENTINEL_BASE + i);
    at = r.to;
  });
  swapped += body.slice(at);
  const pretty = indentJson(swapped);
  if (pretty === null) return null;
  return pretty.replace(/7000000000000\d{3}/g, (m) => {
    const ref = refs[Number(m) - SENTINEL_BASE];
    return ref ? body.slice(ref.from, ref.to) : m;
  });
}
