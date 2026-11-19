import { cellMatch, keyValueRowScanner, regexTextCap, runPageScan } from '../page/scan';
import { createSinglePageSearch, type RunSearch } from '../page/search';
import { getPage, pageVersion } from './page';

// Mirrors views/grid/search.ts exactly, narrowed to KeyValuePage's two fixed semantic columns
// (`fields`/`values`) instead of a tabular page's caller-defined column set — 'field'/
// 'value' are close enough to 'col' that this stays `col: 'field' | 'value'` rather than an
// index, since there is no columns/chunks array to index into here.
export interface Match {
  row: number;
  col: 'field' | 'value';
  start: number;
  end: number;
}

// Searches the loaded page only, never the server — same discipline as grid/search.ts (§8.5's
// D28), applied to keyvalue's own two-column shape.
const runSearch: RunSearch<Match> = (tabId, q, onProgress) => {
  const page = getPage(tabId);
  // P42 D39/P49 D5: KeyValueView.vue reports its own visible window (VirtualList's visible-range
  // emit) — runPageScan scans that window first instead of always starting cold at row 0.
  return runPageScan(
    page,
    tabId,
    // F16 (P108 Part 10): same partial ReDoS mitigation as grid/search.ts — capped only for a
    // user-authored regex, never for a literal/whole-word scan.
    (p) => keyValueRowScanner(p, ['field', 'value'], cellMatch, regexTextCap(q)),
    q,
    onProgress,
  );
};

export const { searchState, matchedRows, pageSearchApi } = createSinglePageSearch<Match>(
  runSearch,
  {
    getPage,
    pageVersion,
  },
);
