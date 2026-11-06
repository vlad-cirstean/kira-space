import { cellText, isNull, type TabularPage, type TextColumnChunk } from '@shared/protocol/page';
import { compileSearchPattern } from '../../../editor/searchPattern';
import { visibleRowsOf } from './visibleRows';

// grid/search.ts, documents/search.ts and keyvalue/search.ts declared the same
// SearchQuery/SearchHandle/CHUNK_ROWS/escapeRegExp and the same rAF-chunked driver with the same
// cancel/zero-width-match/onProgress/resolve semantics, differing only in the per-row scan body.
// stream/search.ts is deliberately NOT built on this — it is a simpler, different scanner
// (one case-insensitive substring match across five fixed columns, no case/word/regex toggles).

export interface SearchQuery {
  text: string;
  matchCase: boolean;
  wholeWord: boolean;
  regex: boolean;
}

interface ScanResult<M> {
  matches: M[];
  /** The true, uncapped count — `matches.length` once `found <= MAX_SCAN_MATCHES`, `MAX_SCAN_MATCHES`
   *  otherwise (D4/F6). */
  found: number;
  /** Ascending rows with at least one match, uncapped by `MAX_SCAN_MATCHES` — filter mode shows
   *  every matching row even when `matches` was truncated. */
  matchedRows: number[];
}

export interface SearchHandle<M> {
  cancel(): void;
  done: Promise<ScanResult<M>>;
}

const CHUNK_ROWS = 2000;

// CHUNK_ROWS above budgets by *row count* alone, but the real
// per-frame cost of a tabular scan is CHUNK_ROWS × columnCount — a wide table (docs/PERF.md's
// `app.scroll_grid` fixture is 60 columns) did 2 000 × 60 = 120 000 decode+regex pairs inside one
// requestAnimationFrame callback, five to fifteen times the 8-12 ms frame budget gates scroll on.
// chunkRowsForColumns turns the row budget into a cell budget so a wide page walks fewer rows
// per frame and a narrow one (keyValueRowScanner's fixed 2-column shape, or a 1-2 column table) is
// unaffected — Math.min keeps it from ever exceeding the original CHUNK_ROWS.
const CELL_BUDGET_PER_FRAME = CHUNK_ROWS * 2; // unaffected columnCount<=2, e.g. keyValueRowScanner
export function chunkRowsForColumns(columnCount: number): number {
  if (columnCount <= 0) return CHUNK_ROWS;
  return Math.min(CHUNK_ROWS, Math.max(1, Math.ceil(CELL_BUDGET_PER_FRAME / columnCount)));
}

// A find's match set had no cap — ~101 B retained per match, 38.8 MB at 400 000 matches,
// 96.2 MB at a million, held until the find is cleared or the tab closes. 50 000 is generous for
// anything a person navigates (Prev/Next through a highlight list) and bounded for a machine —
// ~5 MB by the per-match figure, two orders of magnitude past realistic use.
const MAX_SCAN_MATCHES = 50_000;

/** The compiler itself moved down to editor/searchPattern.ts, so the request/response
 *  find bar's own case/word/regex toggles mean exactly what these three mean here rather than
 *  growing a second, subtly different implementation. This wrapper keeps runChunkedScan's call
 *  site (and its SearchQuery shape) unchanged. Still throws SyntaxError synchronously for an
 *  invalid regex, before any scan starts. */
function compilePattern(q: SearchQuery): RegExp {
  return compileSearchPattern(q.text, q);
}

// A user regex with a nested quantifier (e.g. `(a+)+$`) exhibits catastrophic
// backtracking against one long cell — chunking bounds rows *between* animation frames, not the
// single `exec` call already in flight, so the main thread blocks with no escape (Stop can't run
// either, since the event loop it needs is what's blocked). A full fix moves regex-mode scanning
// into a worker with its own per-scan timeout; this constant is the documented, cheaper partial
// mitigation instead — capping how much text a regex-mode scan runs against per cell bounds the
// worst case blocking duration for any single `exec`, without eliminating the risk entirely. A
// literal/whole-word search (SearchQuery.regex: false) compiles to escapeRegExp's own output,
// which has no user-controlled quantifiers to backtrack on, so only regex-mode scans apply it —
// grid/search.ts and keyvalue/search.ts's own runSearch pass it conditionally on `q.regex`.
export const REGEX_SCAN_TEXT_CAP = 10_000;

/** Walks every match of `pattern` in `text`, calling `emit(start, end)` for each — the zero-width
 *  match guard (a pattern like `x*` or `(?:)` that can match an empty string) that
 *  grid/documents/keyvalue's per-row scan bodies each wrote out identically. `pattern.lastIndex`
 *  is reset first so a shared RegExp scans from the start. `maxLength`, when given, truncates
 *  `text` before scanning (the partial ReDoS mitigation, REGEX_SCAN_TEXT_CAP above) — omitted by
 *  every caller not passing a user-authored regex against untrusted cell content. */
export function eachMatch(
  pattern: RegExp,
  text: string,
  emit: (start: number, end: number) => void,
  maxLength?: number,
): void {
  const scanned =
    maxLength !== undefined && text.length > maxLength ? text.slice(0, maxLength) : text;
  pattern.lastIndex = 0;
  let m = pattern.exec(scanned);
  while (m) {
    emit(m.index, m.index + m[0].length);
    if (m[0].length === 0) pattern.lastIndex++; // never loop forever on a zero-width match
    m = pattern.exec(scanned);
  }
}

// Searches the loaded page only, never the server. Iterates in chunks of 2 000 rows
// per animation frame; a new keystroke cancels and restarts.
//
// `opts.priority`, when given, is scanned first — in its own frame, reported through
// `onProgress` — before the ordinary ascending pass starts from row 0. That ascending pass always
// rebuilds `matches` from scratch and is what `done` resolves to, so the final array is strictly
// ascending regardless of where the priority window sat; the window's own rows
// are therefore scanned twice, which is free next to a page of thousands. `onProgress`'s 4th
// argument is the matches found so far — for the priority tick, that window's own matches only
// (never folded into `matches` itself); for every tick after, `matches` itself, as a fresh
// ascending slice. Omitting `opts` is byte-for-byte today's behaviour, modulo the cap below.
//
// Both passes append through `appendCapped`, which stops growing the returned array
// once it holds `MAX_SCAN_MATCHES` (or `opts.cap`, tests only) but keeps counting — `found`
// (the first `onProgress` argument, and `ScanResult.found`) is always the true, uncapped total, so
// the toolbar can say "N of `<found>` (first `<matches.length>` shown)". Each row's own matches are
// collected into a reused scratch buffer first and appended as one ascending run, so capping never
// reorders a row's own matches relative to each other or to an earlier row's.
export function runChunkedScan<M>(
  totalRows: number,
  scanRow: (row: number, pattern: RegExp, out: M[]) => void,
  q: SearchQuery,
  onProgress: (found: number, rowsScanned: number, totalRows: number, soFar: readonly M[]) => void,
  opts?: { priority?: { from: number; to: number }; cap?: number; chunkRows?: number },
): SearchHandle<M> {
  const pattern = compilePattern(q);
  const cap = opts?.cap ?? MAX_SCAN_MATCHES;
  const chunkRows = opts?.chunkRows ?? CHUNK_ROWS;
  let cancelled = false;
  const matches: M[] = [];
  let found = 0;
  const matchedRows: number[] = [];
  const rowBuf: M[] = [];

  function appendCapped(into: M[], rowMatches: readonly M[]): void {
    if (into === matches) found += rowMatches.length;
    if (into.length >= cap) return;
    const room = cap - into.length;
    for (let i = 0; i < rowMatches.length && i < room; i++) into.push(rowMatches[i]);
  }

  const done = new Promise<ScanResult<M>>((resolve) => {
    function runMainPass(): void {
      let row = 0;
      function step(): void {
        if (cancelled) {
          resolve({ matches, found, matchedRows });
          return;
        }
        const chunkEnd = Math.min(totalRows, row + chunkRows);
        for (; row < chunkEnd; row++) {
          rowBuf.length = 0;
          scanRow(row, pattern, rowBuf);
          if (rowBuf.length > 0) matchedRows.push(row);
          appendCapped(matches, rowBuf);
        }
        onProgress(found, row, totalRows, matches);
        if (row < totalRows) requestAnimationFrame(step);
        else resolve({ matches, found, matchedRows });
      }
      requestAnimationFrame(step);
    }

    const priority = opts?.priority;
    const from = priority ? Math.max(0, priority.from) : 0;
    const to = priority ? Math.min(totalRows, priority.to) : 0;
    if (to > from) {
      requestAnimationFrame(() => {
        if (cancelled) {
          resolve({ matches, found, matchedRows });
          return;
        }
        const priorityMatches: M[] = [];
        let priorityFound = 0;
        for (let row = from; row < to; row++) {
          rowBuf.length = 0;
          scanRow(row, pattern, rowBuf);
          priorityFound += rowBuf.length;
          appendCapped(priorityMatches, rowBuf);
        }
        onProgress(priorityFound, 0, totalRows, priorityMatches);
        runMainPass();
      });
    } else {
      runMainPass();
    }
  });

  return {
    cancel() {
      cancelled = true;
    },
    done,
  };
}

// P48 F9: grid/documents/keyvalue/console's search.ts each repeated this same four-line early-out
// for "no page yet, or an empty query" — a scan that never starts.
export function emptyScan<M>(): SearchHandle<M> {
  return { cancel() {}, done: Promise.resolve({ matches: [], found: 0, matchedRows: [] }) };
}

// grid/search.ts, documents/search.ts and shared/keyvalue/search.ts each wrapped
// runChunkedScan in the same "no page or empty query -> emptyScan; else scan with this tab's
// visible rows as scan priority" shell around their own per-row scanner. `visibleRowsOf` needs no
// import at each call site any more; only the scanner itself (a plain row scan body, or one built
// from tabularRowScanner/keyValueRowScanner) stays per view.
export function runPageScan<M, Pg extends { rowCount: number }>(
  page: Pg | null | undefined,
  tabId: string,
  makeScanRow: (page: Pg) => (row: number, pattern: RegExp, out: M[]) => void,
  q: SearchQuery,
  onProgress: (found: number, rowsScanned: number, totalRows: number, soFar: readonly M[]) => void,
  opts?: { chunkRows?: number },
): SearchHandle<M> {
  if (!page || q.text === '') return emptyScan();
  return runChunkedScan<M>(page.rowCount, makeScanRow(page), q, onProgress, {
    priority: visibleRowsOf(tabId) ?? undefined,
    ...opts,
  });
}

/** The tabular per-row scan body grid/search.ts and console/search.ts's tabular branch
 *  each wrote out — every column, skipping a null cell. `make` builds the caller's own Match
 *  shape from the (row, col, start, end) the scan found. `regexTextCap`, when given, is
 *  forwarded to eachMatch's own `maxLength` — a caller scanning with a user-authored regex passes
 *  REGEX_SCAN_TEXT_CAP; a literal/whole-word caller omits it. */
export function tabularRowScanner<M>(
  page: Pick<TabularPage, 'columns' | 'chunks'>,
  make: (row: number, col: number, start: number, end: number) => M,
  regexTextCap?: number,
): (row: number, pattern: RegExp, out: M[]) => void {
  const decoder = new TextDecoder();
  const colCount = page.columns.length;
  const chunks = page.chunks; // a definite alias — narrowing does not persist into the closure
  return (row, pattern, out) => {
    for (let col = 0; col < colCount; col++) {
      const chunk = chunks[col];
      if (isNull(chunk, row)) continue;
      const text = cellText(chunk, row, decoder);
      eachMatch(pattern, text, (start, end) => out.push(make(row, col, start, end)), regexTextCap);
    }
  };
}

/** The two-chunk field/value scan keyvalue/search.ts and console/search.ts's keyvalue
 *  branch each wrote out, differing only in whether `col` is spelled `'field'|'value'` or `0|1`
 *  — `cols` supplies whichever pair the caller's own Match shape uses. Neither chunk is ever
 *  null by construction (KeyValuePageBuilder.push takes plain strings), so — unlike
 *  tabularRowScanner — there is no isNull check to share. `regexTextCap`: same as
 *  tabularRowScanner's own — forwarded to eachMatch's `maxLength`, omitted for a literal/
 *  whole-word scan. */
export function keyValueRowScanner<M, C>(
  page: { fields: TextColumnChunk; values: TextColumnChunk },
  cols: [C, C],
  make: (row: number, col: C, start: number, end: number) => M,
  regexTextCap?: number,
): (row: number, pattern: RegExp, out: M[]) => void {
  const decoder = new TextDecoder();
  return (row, pattern, out) => {
    const fieldText = cellText(page.fields, row, decoder);
    eachMatch(
      pattern,
      fieldText,
      (start, end) => out.push(make(row, cols[0], start, end)),
      regexTextCap,
    );
    const valueText = cellText(page.values, row, decoder);
    eachMatch(
      pattern,
      valueText,
      (start, end) => out.push(make(row, cols[1], start, end)),
      regexTextCap,
    );
  };
}
