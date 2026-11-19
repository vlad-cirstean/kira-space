import {
  cellMatch,
  chunkRowsForColumns,
  regexTextCap,
  runPageScan,
  tabularRowScanner,
} from '../shared/page/scan';
import { createSinglePageSearch, type RunSearch } from '../shared/page/search';
import { getPage, pageVersion } from './page';

export interface Match {
  row: number;
  col: number; // index into the page's own columns/chunks, not the display order
  start: number;
  end: number;
}

const runSearch: RunSearch<Match> = (tabId, q, onProgress) => {
  const page = getPage(tabId);
  return runPageScan(
    page,
    tabId,
    // F16 (P108 Part 10): a user regex runs against untrusted cell content — cap the text length
    // scanned per cell, the documented partial ReDoS mitigation (scan.ts's own REGEX_SCAN_TEXT_CAP
    // comment). A literal/whole-word search has no user-controlled quantifiers to backtrack on.
    (p) => tabularRowScanner(p, cellMatch, regexTextCap(q)),
    q,
    onProgress,
    // P21 round 3 performance finding 1: a cell-based chunk budget, not a flat 2 000 rows — see
    // scan.ts's own comment on chunkRowsForColumns.
    { chunkRows: page ? chunkRowsForColumns(page.columns.length) : undefined },
  );
};

export const { searchState, matchedRows, pageSearchApi } = createSinglePageSearch<Match>(
  runSearch,
  {
    getPage,
    pageVersion,
  },
);
