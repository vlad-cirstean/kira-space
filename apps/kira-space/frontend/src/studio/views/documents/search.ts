import {
  eachMatch,
  REGEX_SCAN_TEXT_CAP,
  runPageScan,
  type SearchHandle,
  type SearchQuery,
} from '../shared/page/scan';
import { createPageSearch } from '../shared/page/search';
import { documentRow, getPage, pageVersion } from './page';

export interface Match {
  row: number;
  start: number;
  end: number;
}

// DocumentView.vue's collapsed row shows only the `_id` until expanded —
// exported so the view can build the *same* string a search matched against, and wrap the
// matched substring using search.ts's own start/end offsets without the two ever disagreeing.
// This searches the whole document body regardless, whitespace collapsed and never truncated, so
// a match can exist even though the un-searched row shows none of that text.
export function previewLineFor(body: string): string {
  return body.replace(/\s+/g, ' ').trim();
}

// Scoped to the loaded page only, never the server —
// and to each document's rendered preview line specifically, not the full raw EJSON body: a
// document's "columns" are dynamic, so there is no per-field grid to search field-by-field the
// way views/grid/search.ts does, and full-text-over-EJSON would be new scope this task doesn't
// ask for.
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
  // runPageScan scans the rows DocumentView.vue's VirtualList currently has on screen first.
  // A user regex like `(a+)+$` against one long document body still blocks the main thread. Same
  // partial mitigation grid/search.ts already applies, same condition (regex-only).
  const regexTextCap = q.regex ? REGEX_SCAN_TEXT_CAP : undefined;
  return runPageScan(
    page,
    tabId,
    () => (row, pattern, out) => {
      const doc = documentRow(tabId, row);
      if (!doc) return;
      const text = previewLineFor(doc.body);
      eachMatch(pattern, text, (start, end) => out.push({ row, start, end }), regexTextCap);
    },
    q,
    onProgress,
  );
}

// Mirrors views/grid/search.ts's searchState, narrowed to one column (a document has no columns
// to disambiguate a match's position within).
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
