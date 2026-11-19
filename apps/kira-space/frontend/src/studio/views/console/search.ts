import { cellText, isNull } from '@shared/protocol/page';
import {
  cellMatch,
  chunkRowsForColumns,
  eachMatch,
  emptyScan,
  keyValueRowScanner,
  regexTextCap,
  runChunkedScan,
  tabularRowScanner,
} from '../shared/page/scan';
import { createPageSearch, type RunSearch } from '../shared/page/search';
import { visibleRowsOf } from '../shared/page/visibleRows';
import { pageVersion } from './resultPages';
import { useConsoleViewStore } from './state';

// The search scope stays the *tab id*, same as grid/documents/keyvalue's own
// PageSearchApi — this file alone resolves "which of the tab's N result sets is active" via
// state.ts's activePage, so views/shared/page/'s two cleanup handlers never have to learn a
// per-result-set key scheme. `col` is a page column index for a tabular result; a document
// row has no per-column projection so it is always 0, and a key/value row is 0 for the field, 1
// for the value — mirroring keyvalue/search.ts's own fixed-column convention.
export interface Match {
  row: number;
  col: number;
  start: number;
  end: number;
}

// Searches the active result set's loaded page only, never the server (§8.5's D28, every other
// view's own precedent).
const runSearch: RunSearch<Match> = (tabId, q, onProgress) => {
  const page = useConsoleViewStore().activePage(tabId);
  if (!page || q.text === '') return emptyScan();

  // P42 D39: the rows ConsoleResultGrid.vue's VirtualList currently has on screen (D37) — keyed
  // by tabId like every other search.ts/PageSearchApi entry point here (D9), not by result key,
  // since only the active result set is ever rendered (and therefore reportable) at a time.
  const priority = { priority: visibleRowsOf(tabId) ?? undefined };

  // Only grid/search.ts and keyvalue/search.ts were capped, deliberately leaving these three
  // branches uncapped — a user regex like `(a+)+$` against one long console cell still blocked
  // the main thread. Same partial mitigation, same condition: only a regex search has
  // user-controlled quantifiers to backtrack on.
  const cap = regexTextCap(q);

  if (page.kind === 'tabular') {
    return runChunkedScan<Match>(
      page.rowCount,
      tabularRowScanner(page, cellMatch, cap),
      q,
      onProgress,
      // P21 round 3 performance finding 1: cell-based chunk budget for the tabular branch only —
      // the document/keyvalue branches below have no per-row column multiplier to correct for.
      { ...priority, chunkRows: chunkRowsForColumns(page.columns.length) },
    );
  }

  if (page.kind === 'document') {
    const decoder = new TextDecoder();
    const bodies = page.bodies;
    return runChunkedScan<Match>(
      page.rowCount,
      (row, pattern, out) => {
        if (isNull(bodies, row)) return;
        const text = cellText(bodies, row, decoder);
        eachMatch(pattern, text, (start, end) => out.push({ row, col: 0, start, end }), cap);
      },
      q,
      onProgress,
      priority,
    );
  }

  // A console result is only ever tabular, document or keyvalue (ConsoleResultGrid.vue's own
  // three template branches) — never StreamPage, so this narrows the same way resultPages.ts's
  // keyValueRow() does rather than assuming the else branch away.
  if (page.kind !== 'keyvalue') return emptyScan();
  return runChunkedScan<Match>(
    page.rowCount,
    keyValueRowScanner(page, [0, 1], cellMatch, cap),
    q,
    onProgress,
    priority,
  );
};

export const { searchState, matchedRows, pageSearchApi } = createPageSearch<Match>({
  runSearch,
  pageVersion,
  loadedRowCount: (tabId) => useConsoleViewStore().activePage(tabId)?.rowCount ?? 0,
  // Page identity, not the counter every console tab's load/close bumps (Part 11 F4 pattern).
  pageOf: (tabId) => useConsoleViewStore().activePage(tabId),
});
