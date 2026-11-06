import {
  chunkRowsForColumns,
  REGEX_SCAN_TEXT_CAP,
  runPageScan,
  type SearchHandle,
  type SearchQuery,
  tabularRowScanner,
} from '../shared/page/scan';
import { createPageSearch } from '../shared/page/search';
import { getPage, pageVersion } from './page';

export interface Match {
  row: number;
  col: number; // index into the page's own columns/chunks, not the display order
  start: number;
  end: number;
}

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
  return runPageScan(
    page,
    tabId,
    // F16 (P108 Part 10): a user regex runs against untrusted cell content — cap the text length
    // scanned per cell, the documented partial ReDoS mitigation (scan.ts's own REGEX_SCAN_TEXT_CAP
    // comment). A literal/whole-word search has no user-controlled quantifiers to backtrack on.
    (p) =>
      tabularRowScanner(
        p,
        (row, col, start, end) => ({ row, col, start, end }),
        q.regex ? REGEX_SCAN_TEXT_CAP : undefined,
      ),
    q,
    onProgress,
    // P21 round 3 performance finding 1: a cell-based chunk budget, not a flat 2 000 rows — see
    // scan.ts's own comment on chunkRowsForColumns.
    { chunkRows: page ? chunkRowsForColumns(page.columns.length) : undefined },
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
