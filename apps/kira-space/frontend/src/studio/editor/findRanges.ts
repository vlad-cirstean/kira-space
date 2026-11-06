import type { RangeHighlight } from '@workbench/editor/ranges';
import { REGEX_SCAN_TEXT_CAP } from '../views/shared/page/scan';
import { compileSearchPattern, type SearchPatternOptions } from './searchPattern';

// The find bar's own "find" — not @codemirror/search (that package is not a dependency,
// and the requirement here is this app's own toolbar chrome and its own --kira-search-match token
// pair, not a re-skinned vendor panel). Returns ranges in the shape rangeHighlightPlugin already
// consumes, so a find bar is another `rangeHighlights` source.
//
// The three options the data views' own SearchToolbar has always had (match case, whole
// word, regex) now apply here too, and they are compiled by `editor/searchPattern.ts`'s own
// `compileSearchPattern` rather than by a second implementation — so "whole word" and "regex" mean
// exactly the same thing in a request/response find as in a grid search.

/** P28 D11: all-false is the pre-P28 behaviour exactly — a case-insensitive substring walk. */
export type FindOptions = SearchPatternOptions;

/** Exported so a consumer that has not yet mounted its find bar (a pane reading a null bar ref)
 *  uses the identical defaults rather than spelling three falses of its own. */
export const DEFAULT_FIND_OPTIONS: FindOptions = {
  matchCase: false,
  wholeWord: false,
  regex: false,
};

interface MatchPosition {
  from: number;
  to: number;
}

// On one keystroke, ResponseFindBar.vue's own matchCounts calls
// this once per target (1-2), scrollToCurrent calls it again for the target the current match
// lands in, and the pane's own per-target highlighters call it again per target just to derive a
// running cursor — every one of those for the *same* (doc, query) pair, and every call re-walked
// the whole document. `doc` is an immutable string reference each render, so the match *positions*
// for a given (doc, query, options) never change between those calls — only `currentIndex` (which
// range gets the "current" class) does, and that is a cheap O(matches) relabelling. A tiny bounded
// cache turns 4-5 full-document walks per keystroke into at most one per distinct document.
const POSITION_CACHE_SIZE = 4;
const positionCache: { key: string; doc: string; positions: MatchPosition[] }[] = [];

/** The options half of the cache key. Kept separate from `doc` (compared by reference) so a large
 *  document is never concatenated into a string key. */
function optionsKey(query: string, o: FindOptions): string {
  return `${o.matchCase ? 1 : 0}${o.wholeWord ? 1 : 0}${o.regex ? 1 : 0}\0${query}`;
}

function matchPositions(doc: string, query: string, o: FindOptions): readonly MatchPosition[] {
  const key = optionsKey(query, o);
  const cached = positionCache.find((e) => e.doc === doc && e.key === key);
  if (cached) return cached.positions;

  const positions: MatchPosition[] = [];
  let pattern: RegExp;
  try {
    pattern = compileSearchPattern(query, o);
  } catch {
    // An in-progress regex ("[", "(?") is a normal intermediate state while typing, not an error
    // to throw at a render — zero matches, and the bar marks its own input invalid. Deliberately
    // not cached: the next keystroke usually makes it valid again.
    return positions;
  }
  // A user regex runs against a response/request body this app doesn't control —
  // `(a+)+$` still blocked the main thread here, the same ReDoS shape scan.ts's own
  // REGEX_SCAN_TEXT_CAP mitigates for a grid/console/document cell. A literal/whole-word search has
  // no user-controlled quantifiers to backtrack on, so only regex mode truncates. `scanned` is a
  // prefix of `doc`, so every offset below still indexes correctly into the real document.
  const scanned =
    o.regex && doc.length > REGEX_SCAN_TEXT_CAP ? doc.slice(0, REGEX_SCAN_TEXT_CAP) : doc;
  pattern.lastIndex = 0;
  for (let m = pattern.exec(scanned); m !== null; m = pattern.exec(scanned)) {
    // A zero-width match (`x*`, `(?:)`) would otherwise never advance lastIndex — the same guard
    // scan.ts's own walker carries, for the same reason.
    if (m[0] === '') {
      pattern.lastIndex += 1;
      continue;
    }
    positions.push({ from: m.index, to: m.index + m[0].length });
  }

  positionCache.push({ key, doc, positions });
  if (positionCache.length > POSITION_CACHE_SIZE) positionCache.shift();
  return positions;
}

/** True when `query` cannot compile under `options` — the bar's own "invalid input" signal. Only
 *  ever true with `regex` on; a literal search escapes its input and can never fail. */
export function findQueryIsInvalid(
  query: string,
  options: FindOptions = DEFAULT_FIND_OPTIONS,
): boolean {
  if (!query || !options.regex) return false;
  try {
    compileSearchPattern(query, options);
    return false;
  } catch {
    return true;
  }
}

/** Every occurrence of `query` in `doc` under `options`, as `{from, to, class}` ranges — an empty
 *  query, no match, or a query that cannot compile returns `[]`. `currentIndex`, when it names one
 *  of this document's own matches (0-based), gets `'kira-ed-find-match-current'` instead of the
 *  plain match class — the caller (ResponseFindBar.vue) numbers matches across every target
 *  document it searches and only ever passes the index that lands in *this* document. */
export function findRanges(
  doc: string,
  query: string,
  currentIndex?: number,
  options: FindOptions = DEFAULT_FIND_OPTIONS,
): readonly RangeHighlight[] {
  if (!query) return [];
  return matchPositions(doc, query, options).map((p, index) => ({
    from: p.from,
    to: p.to,
    class: index === currentIndex ? 'kira-ed-find-match-current' : 'kira-ed-find-match',
  }));
}
