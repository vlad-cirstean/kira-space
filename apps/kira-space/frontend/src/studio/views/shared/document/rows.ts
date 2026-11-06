// The row model the virtualized document list reads: one memoized parse per page row, the
// per-path expansion set, and the exact row height. Plain Maps/Sets keyed by row and by path —
// never reactive — with `rowsVersion` as the one reactive surface, mirroring documents/page.ts's
// own `pageVersion`. A `reactive()` tree here would put a Proxy around every node of every document
// on the page, which is exactly the frame budget this phase exists to protect.

import { formatBytes } from '@workbench/behaviour/format';
import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { defineStore } from 'pinia';
import { reactive } from 'vue';
import { type BsonType, type DocNode, parseDocument, parseIdLabel } from './ejson';

// This module used to import documentRow from views/documents/page.ts directly — the one edge
// that blocked the move here, since views/console/ may not import views/documents/*
// (biome.json's views/<kind>/* rule). A registered source replaces it: the view that owns a
// scope (a document tab's id, or a console result's key) registers where its rows come from,
// and every function below still calls the same-shaped local documentRow().
type RowSource = (
  row: number,
) => { id: string; body: string; isTruncated?: boolean; bodyByteLength: number } | null;

export interface DocumentRowView {
  index: number;
  /** The raw EJSON id text — the mutation key, unchanged. */
  id: string;
  /** parseIdLabel().text — what the head shows and Copy _id copies. */
  idLabel: string;
  idType: BsonType;
  /** '7 fields' (F8's mockup badge). */
  fieldCount: number;
  byteLabel: string;
  isTruncated: boolean;
  /** null => D22's raw-text fallback. */
  root: DocNode | null;
}

export interface DocLine {
  node: DocNode;
  depth: number;
  expandable: boolean;
  expanded: boolean;
}

interface Parsed {
  root: DocNode | null;
  idLabel: string;
  idType: BsonType;
  byteLabel: string;
}

interface TabRows {
  parseCache: Map<number, Parsed>;
  expandedPaths: Map<number, Set<string>>;
  // `rowHeight`'s answer for an expanded row is a single
  // integer (the visible-line count) — a few bytes, against a whole DocNode tree in parseCache —
  // but it used to be derived from parseCache/visibleLines on every call, so `pruneRows` evicting
  // parseCache outside the visible window meant every rowHeights
  // recompute after a scroll re-parsed every expanded document on the page from scratch, to
  // answer a question whose answer hadn't changed. `lineCounts` is deliberately NOT touched by
  // pruneRows below — it survives exactly what parseCache doesn't — and is invalidated only where
  // the line count can change: togglePath (this row's own expansion set changed) and a
  // full resetRows/dropRows (the whole page, or scope, is gone). `null` caches the "no root, raw
  // fallback" case so a document that failed to parse doesn't retry parsing it every call either.
  lineCounts: Map<number, number | null>;
}

const HEAD_H = 26; // --kira-h-md
const LINE_H = 18; // --kira-h-xs, OperationsPanel's own row height
const BODY_PADDING_V = 8; // --kira-s-2 top + bottom, DocumentTree.vue's own body padding
const EDITING_H = 220; // the fixed editor panel height (unchanged from the pre-P27 row)

function countNodes(node: DocNode): number {
  let n = 1;
  for (const child of node.children) n += countNodes(child);
  return n;
}

export const useDocumentRowsStore = defineStore('documentRows', () => {
  const rowSources = new Map<string, RowSource>();
  const tabRows = new Map<string, TabRows>();

  /** The only reactive thing in this module — bumped on every parse-cache/expansion-set change. */
  const rowsVersion = reactive({ n: 0 });

  /** Where `scope`'s rows come from — the document tab registers documents/page.ts's own
   *  documentRow on mount and unregisters on unmount; a console result set does the same with
   *  console/resultPages.ts's documentRow (P42 D11). */
  function registerDocumentRows(scope: string, source: RowSource): void {
    rowSources.set(scope, source);
  }

  function unregisterDocumentRows(scope: string): void {
    rowSources.delete(scope);
  }

  function documentRow(
    tabId: string,
    row: number,
  ): { id: string; body: string; isTruncated: boolean; bodyByteLength: number } | null {
    const result = rowSources.get(tabId)?.(row);
    return result
      ? {
          id: result.id,
          body: result.body,
          isTruncated: result.isTruncated ?? false,
          bodyByteLength: result.bodyByteLength,
        }
      : null;
  }

  function ensureTabRows(tabId: string): TabRows {
    let entry = tabRows.get(tabId);
    if (!entry) {
      entry = { parseCache: new Map(), expandedPaths: new Map(), lineCounts: new Map() };
      tabRows.set(tabId, entry);
    }
    return entry;
  }

  function parseRow(tabId: string, row: number): Parsed | null {
    const doc = documentRow(tabId, row);
    if (!doc) return null;
    const entry = ensureTabRows(tabId);
    const cached = entry.parseCache.get(row);
    if (cached) return cached;
    const idLabel = parseIdLabel(doc.id);
    const parsed: Parsed = {
      root: parseDocument(doc.body),
      idLabel: idLabel.text,
      idType: idLabel.bsonType,
      // P2 R2 (task #99): bodyByteLength comes straight from the wire's own chunk offsets (the
      // RowSource's own doc comment) — this used to re-encode the already-decoded `doc.body` string
      // back to UTF-8 with a fresh TextEncoder just to read its byte length.
      byteLabel: formatBytes(doc.bodyByteLength),
    };
    entry.parseCache.set(row, parsed);
    return parsed;
  }

  function rowView(tabId: string, row: number): DocumentRowView | null {
    const doc = documentRow(tabId, row);
    const parsed = parseRow(tabId, row);
    if (!doc || !parsed) return null;
    return {
      index: row,
      id: doc.id,
      idLabel: parsed.idLabel,
      idType: parsed.idType,
      fieldCount: parsed.root ? parsed.root.children.length : 0,
      byteLabel: parsed.byteLabel,
      isTruncated: doc.isTruncated,
      root: parsed.root,
    };
  }

  function expandedPathsFor(tabId: string, row: number): Set<string> {
    const entry = ensureTabRows(tabId);
    let set = entry.expandedPaths.get(row);
    if (!set) {
      set = new Set();
      entry.expandedPaths.set(row, set);
    }
    return set;
  }

  function isPathExpanded(tabId: string, row: number, path: string): boolean {
    return expandedPathsFor(tabId, row).has(path);
  }

  function togglePath(tabId: string, row: number, path: string): void {
    const set = expandedPathsFor(tabId, row);
    if (set.has(path)) set.delete(path);
    else set.add(path);
    // This row's own visible-line count just changed — the one cached answer that must not survive
    // this call, unlike every other row's on the page (finding 4).
    ensureTabRows(tabId).lineCounts.delete(row);
    rowsVersion.n++;
  }

  function walk(tabId: string, row: number, node: DocNode, depth: number, out: DocLine[]): void {
    for (const child of node.children) {
      const expandable = child.kind !== 'scalar';
      const expanded = expandable && isPathExpanded(tabId, row, child.path);
      out.push({ node: child, depth, expandable, expanded });
      if (expanded) walk(tabId, row, child, depth + 1, out);
    }
  }

  /**
   * Ascending, flattened, exactly what the expanded body renders: the first layer always, plus the
   * descendants of every path in the expansion set. `depth` drives the indent.
   */
  function visibleLines(tabId: string, row: number): readonly DocLine[] {
    const parsed = parseRow(tabId, row);
    if (!parsed?.root) return [];
    const out: DocLine[] = [];
    walk(tabId, row, parsed.root, 0, out);
    return out;
  }

  /** Called from `state.ts`'s `load()` after `setPage` — a new page has new rows; every parse and
   *  every path is stale. */
  function resetRows(tabId: string): void {
    const entry = tabRows.get(tabId);
    if (entry) {
      entry.parseCache.clear();
      entry.expandedPaths.clear();
      entry.lineCounts.clear();
    }
    rowsVersion.n++;
  }

  /** Prunes `parseCache` to the visible window (widened by VirtualList's own overscan —
   *  already folded into the caller's `start`/`end`, `DocumentView.vue`'s `onVisibleRange`). The
   *  O(rowCount) parse pass a page's initial load needs — VirtualList's own exact-height math needs
   *  every *expanded* row's line count, and every document defaults to expanded — no
   *  longer stays retained for the whole page once the window has moved past a row.
   *
   *  `expandedPaths` is pruned too, but only for a row whose set is *empty* — a row the user
   *  drilled a nested field into keeps its expansion memory regardless of scroll position,
   *  so expanding a document, scrolling far away and scrolling back still shows it expanded.
   *  `lineCounts` is never pruned here at all — it is the answer `rowHeight` needs for
   *  every row on the page, not just the visible window, and costs a few bytes/row against a whole
   *  `DocNode` tree. */
  function pruneRows(scope: string, start: number, end: number): void {
    const entry = tabRows.get(scope);
    if (!entry) return;
    for (const row of entry.parseCache.keys()) {
      if (row < start || row >= end) entry.parseCache.delete(row);
    }
    for (const [row, paths] of entry.expandedPaths) {
      if ((row < start || row >= end) && paths.size === 0) entry.expandedPaths.delete(row);
    }
  }

  /** Releases everything this module holds for one scope — a closed document
   *  tab, or a closed console result set whose key nothing can ever match again (state.ts's own
   *  `nextSeq`). Distinct from `resetRows` above, which keeps the entry because the same scope is
   *  about to hold a *new* page — `dropRows` is for a scope that is never coming back. Registered
   *  as a tab-runtime cleanup: a document tab's own scope is its tab id, so the same close path
   *  every other per-tab store already uses (state/tabRuntime.ts) covers it for free; console
   *  result scopes are dropped explicitly by console/state.ts instead, since a result set's own
   *  lifetime is shorter than its tab's. */
  function dropRows(scope: string): void {
    tabRows.delete(scope);
    rowsVersion.n++;
  }

  registerTabRuntimeCleanup(dropRows);

  /** Playwright-only (main.ts's `window.__kiraRetention`, C1) — F4's own subject: `parseCache`'s
   *  row count and the total `DocNode` count across every cached tree, neither of which
   *  `totalRetainedBytes()` (page store's own `byteSize` sum) can see. */
  function retentionSnapshot(): {
    tabScopes: number;
    parseCacheRows: number;
    docNodeCount: number;
  } {
    let parseCacheRows = 0;
    let docNodeCount = 0;
    for (const entry of tabRows.values()) {
      parseCacheRows += entry.parseCache.size;
      for (const parsed of entry.parseCache.values()) {
        if (parsed.root) docNodeCount += countNodes(parsed.root);
      }
    }
    return { tabScopes: tabRows.size, parseCacheRows, docNodeCount };
  }

  /**
   * The exact pixel height of row `i`, with no measurement (D20): a head plus, when expanded, one
   * line per visible node — or the fixed editor height while this row is the one being edited, or
   * while it fell back to raw text (D22 — an arbitrarily wrapped `<pre>` isn't exactly measurable
   * either, so it gets the same fixed allowance rather than a false claim of precision).
   *
   * `isExpanded` is supplied by the caller (state.ts's `isDocumentExpanded`) rather than looked up
   * here — this module and state.ts would otherwise import each other (documents/page.ts already does,
   * below, for `resetRows`), and one circular edge is enough. `hasSearchPreview` is the same kind
   * of caller-supplied flag (P31 D20): DocumentView.vue's own documents/search.ts-derived state, adding
   * one line's worth of height for the collapsed row's `<mark>`-highlighted preview line.
   *
   * P5 C2/F4: `editingRow` is a page-row index, not a document id — the caller already knows which
   * row it started editing (it's the one the Edit click came from), so this never has to decode a
   * row's id purely to answer "is this the one being edited". Combined with `isExpanded`, a
   * collapsed, unedited, non-preview row — the common case for every row outside the rendered
   * window — returns `HEAD_H` with no `documentRow` and no `parseRow` call at all.
   *
   * P21 round 3 performance finding 4: an expanded row's line count is read from `lineCounts` first
   * — populated the first time this row is asked about after its expansion set last changed, kept
   * (unlike `parseCache`) across `pruneRows` — so a `rowsVersion` bump from toggling *one* row's
   * expansion (or from a scroll, which changes nothing about any row's own line count) no longer
   * re-parses every other expanded document on the page just to answer the same question again.
   */
  function rowHeight(
    tabId: string,
    row: number,
    editingRow: number | null,
    isExpanded: boolean,
    hasSearchPreview = false,
  ): number {
    if (row === editingRow) return HEAD_H + EDITING_H;
    if (!isExpanded) return hasSearchPreview ? HEAD_H + LINE_H : HEAD_H;
    const entry = ensureTabRows(tabId);
    let lines = entry.lineCounts.get(row);
    if (lines === undefined) {
      const parsed = parseRow(tabId, row);
      lines = parsed?.root ? visibleLines(tabId, row).length : null;
      entry.lineCounts.set(row, lines);
    }
    if (lines === null) return HEAD_H + EDITING_H;
    return HEAD_H + lines * LINE_H + BODY_PADDING_V;
  }

  return {
    rowsVersion,
    registerDocumentRows,
    unregisterDocumentRows,
    rowView,
    togglePath,
    visibleLines,
    resetRows,
    pruneRows,
    dropRows,
    retentionSnapshot,
    rowHeight,
  };
});
