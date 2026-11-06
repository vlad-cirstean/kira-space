// rowHeight (for an expanded row) used to derive its answer by
// re-running parseRow -> parseDocument (a full EJSON parse) and visibleLines (a tree walk) on
// *every* call, and pruneRows (window pruning) evicts parseCache outside the visible
// window on every scroll — so bumping rowsVersion for one row's own togglePath re-parsed every
// *other* expanded document on the page from scratch, only to recompute a line count that hadn't
// changed. lineCounts fixes that: it survives pruneRows and is invalidated only by togglePath (for
// the one row whose expansion changed) and resetRows/dropRows (the whole scope). This file proves
// that end to end by counting real parseDocument calls through registerDocumentRows' own RowSource
// contract — the same seam documents/page.ts and console/resultPages.ts register against.
//
// rows.ts imports vue's reactive() and state/tabRuntime.ts (registerTabRuntimeCleanup), neither of
// which reach window/bridge, so no window stub is needed here.
import { describe, expect, test } from 'bun:test';
import { setActivePinia } from 'pinia';
import { pinia } from '../../../frontend/src/studio/state/pinia';
import { useDocumentRowsStore } from '../../../frontend/src/studio/views/shared/document/rows';

setActivePinia(pinia);
const {
  dropRows,
  pruneRows,
  registerDocumentRows,
  resetRows,
  rowHeight,
  togglePath,
  unregisterDocumentRows,
} = useDocumentRowsStore();

let parseCalls = 0;
const originalJSONParse = JSON.parse;

function withParseCounting<T>(fn: () => T): T {
  parseCalls = 0;
  // rows.ts's own parseDocument (ejson.ts) is built on JSON.parse, with no exported hook of its
  // own to spy on, so this instruments the built-in directly for the duration of one call.
  // biome-ignore lint/suspicious/noExplicitAny: instrumenting a built-in for one assertion
  (JSON as any).parse = (...args: Parameters<typeof JSON.parse>) => {
    parseCalls++;
    return originalJSONParse(...args);
  };
  try {
    return fn();
  } finally {
    JSON.parse = originalJSONParse;
  }
}

const HEAD_H = 26;
const LINE_H = 18;
const BODY_PADDING_V = 8;

// A 2-field object: two top-level lines when expanded, matching HEAD_H + 2*LINE_H + BODY_PADDING_V.
const TWO_FIELD_BODY = JSON.stringify({ a: 1, b: 2 });
// One scalar field plus one nested object field (2 children of its own) — collapsed, `b` is a
// single line like `a`; expanding `b`'s own path adds its two children as two more lines. Exercises
// the actual "a recount, not just a cache hit" behaviour togglePath's own invalidation exists for.
const NESTED_BODY = JSON.stringify({ a: 1, b: { c: 2, d: 3 } });
// parseRow calls JSON.parse twice per row: once for the id label (parseIdLabel), once for the body
// (parseDocument) — both go through this file's own JSON.parse spy.
const PARSES_PER_ROW = 2;

function scope(name: string, body = TWO_FIELD_BODY): string {
  const id = `${name}-${Math.random().toString(36).slice(2)}`;
  registerDocumentRows(id, (row) => {
    if (row < 0 || row >= 5) return null;
    return { id: `"row-${row}"`, body, bodyByteLength: body.length };
  });
  return id;
}

describe('rowHeight line-count cache (finding 4)', () => {
  test('an expanded row is parsed once, not once per rowHeight call', () => {
    const tabId = scope('once');
    try {
      withParseCounting(() => {
        const h1 = rowHeight(tabId, 0, null, true);
        const h2 = rowHeight(tabId, 0, null, true);
        const h3 = rowHeight(tabId, 0, null, true);
        expect(h1).toBe(HEAD_H + 2 * LINE_H + BODY_PADDING_V);
        expect(h2).toBe(h1);
        expect(h3).toBe(h1);
      });
      expect(parseCalls).toBe(PARSES_PER_ROW);
    } finally {
      unregisterDocumentRows(tabId);
      dropRows(tabId);
    }
  });

  test('pruneRows evicting parseCache does not force a re-parse for rowHeight (the core of finding 4)', () => {
    const tabId = scope('prune');
    try {
      rowHeight(tabId, 0, null, true); // populate lineCounts[0] and parseCache[0]
      rowHeight(tabId, 1, null, true); // populate lineCounts[1] and parseCache[1]

      // Simulate the scroll DocumentView.vue's onVisibleRange drives: row 0 falls out of the
      // visible window and its parseCache entry is evicted, exactly as P5 C3/F4 intends.
      pruneRows(tabId, 1, 5);

      withParseCounting(() => {
        const h0 = rowHeight(tabId, 0, null, true); // parseCache[0] is gone; lineCounts[0] is not
        expect(h0).toBe(HEAD_H + 2 * LINE_H + BODY_PADDING_V);
      });
      expect(parseCalls).toBe(0); // answered from lineCounts alone — no re-parse
    } finally {
      unregisterDocumentRows(tabId);
      dropRows(tabId);
    }
  });

  test("togglePath invalidates only its own row's cached line count, and the recount actually reflects the new expansion", () => {
    const tabId = scope('toggle', NESTED_BODY);
    try {
      const collapsedH0 = rowHeight(tabId, 0, null, true); // 'a', 'b' collapsed — 2 lines
      const collapsedH1 = rowHeight(tabId, 1, null, true); // never toggled — stays 2 lines
      expect(collapsedH0).toBe(HEAD_H + 2 * LINE_H + BODY_PADDING_V);

      let parsesDuringToggleAndRecount = 0;
      withParseCounting(() => {
        togglePath(tabId, 0, 'b'); // expand 'b' — row 0 now has 2 + 2 = 4 visible lines
        rowHeight(tabId, 0, null, true);
        parsesDuringToggleAndRecount = parseCalls;
      });
      const expandedH0 = rowHeight(tabId, 0, null, true);
      expect(expandedH0).toBe(HEAD_H + 4 * LINE_H + BODY_PADDING_V);
      expect(expandedH0).toBeGreaterThan(collapsedH0);
      // The recount reused parseCache's already-parsed root (untouched by togglePath) — only
      // visibleLines re-walks it, so this needed no fresh JSON.parse at all.
      expect(parsesDuringToggleAndRecount).toBe(0);

      // Row 1's own cached line count was never invalidated — expanding row 0's 'b' must not
      // change what row 1 (a completely different row) reports.
      const h1Again = rowHeight(tabId, 1, null, true);
      expect(h1Again).toBe(collapsedH1);
    } finally {
      unregisterDocumentRows(tabId);
      dropRows(tabId);
    }
  });

  test('resetRows clears the line-count cache along with everything else', () => {
    const tabId = scope('reset');
    try {
      rowHeight(tabId, 0, null, true);
      resetRows(tabId);
      withParseCounting(() => {
        rowHeight(tabId, 0, null, true);
      });
      expect(parseCalls).toBe(PARSES_PER_ROW);
    } finally {
      unregisterDocumentRows(tabId);
      dropRows(tabId);
    }
  });

  test('a collapsed row never parses at all', () => {
    const tabId = scope('collapsed');
    try {
      withParseCounting(() => {
        const h = rowHeight(tabId, 0, null, false);
        expect(h).toBe(HEAD_H);
      });
      expect(parseCalls).toBe(0);
    } finally {
      unregisterDocumentRows(tabId);
      dropRows(tabId);
    }
  });

  test('a row with no document (past the end) falls back to the editing-height allowance, cached too', () => {
    const tabId = scope('missing');
    try {
      withParseCounting(() => {
        const h1 = rowHeight(tabId, 999, null, true);
        const h2 = rowHeight(tabId, 999, null, true);
        expect(h1).toBe(HEAD_H + 220);
        expect(h2).toBe(h1);
      });
      // documentRow() returns null for row 999, so parseRow bails before ever calling
      // parseDocument — this just confirms the cache doesn't spuriously invent a parse either.
      expect(parseCalls).toBe(0);
    } finally {
      unregisterDocumentRows(tabId);
      dropRows(tabId);
    }
  });
});
