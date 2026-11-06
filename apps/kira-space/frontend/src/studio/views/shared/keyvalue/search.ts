import {
  keyValueRowScanner,
  REGEX_SCAN_TEXT_CAP,
  runPageScan,
  type SearchHandle,
  type SearchQuery,
} from '../page/scan';
import { createPageSearch } from '../page/search';
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
function runSearch(
  tabId: string,
  q: SearchQuery,
  onProgress: (
    found: number,
    rowsScanned: number,
    totalRows: number,
    soFar: readonly Match[],
  ) => void,
): SearchHandle<Match> {
  const page = getPage(tabId);
  // P42 D39/P49 D5: KeyValueView.vue reports its own visible window (VirtualList's visible-range
  // emit) — runPageScan scans that window first instead of always starting cold at row 0.
  return runPageScan(
    page,
    tabId,
    // F16 (P108 Part 10): same partial ReDoS mitigation as grid/search.ts — capped only for a
    // user-authored regex, never for a literal/whole-word scan.
    (p) =>
      keyValueRowScanner(
        p,
        ['field', 'value'],
        (row, col, start, end) => ({
          row,
          col,
          start,
          end,
        }),
        q.regex ? REGEX_SCAN_TEXT_CAP : undefined,
      ),
    q,
    onProgress,
  );
}

const {
  searchState,
  matchedRows,
  api: pageSearchApi,
} = createPageSearch<Match>({
  runSearch,
  pageVersion,
  pageOf: getPage,
  loadedRowCount: (tabId) => getPage(tabId)?.rowCount ?? 0,
});

export { matchedRows, pageSearchApi, searchState };
