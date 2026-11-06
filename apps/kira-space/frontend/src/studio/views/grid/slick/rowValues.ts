import type { ObjectMeta } from '@shared/domain/tree';
import type { TabularPage } from '@shared/protocol/page';
import type { MenuItem } from '@workbench/state/contextMenu';
import type { RowSnapshot } from '../../shared/clipboardFormats';
import { pageColumnIndexFor } from '../../shared/page/columns';
import { rowsForColumnOps, visibleRowsInSpan } from '../../shared/slick/rowVisibility';
import type { Selection } from '../../shared/slick/selection';
import type { SqlDialect } from '../../shared/sqlIdent';
import { type FkNavContext, foreignKeyNavItems, referencedByItems } from '../menu';
import { cell } from '../page';
import { usePendingChangesStore } from '../pendingChanges';

// The small module DataGrid.vue kept inline, extracted so both the three context menus and the
// clipboard and the FK/PK nav button can share one copy of it, and so DataGrid.vue can be deleted
// in one commit later rather than picked apart. No SlickGrid API appears anywhere in this file —
// every function here is content, the same behaviour DataGrid.vue's own identically-named methods
// had, taking the page/column order/tabId it used to close over as plain parameters instead.

export interface DisplayCellView {
  text: string;
  isNull: boolean;
  truncated: boolean;
  staged: boolean;
}

/** Merges a staged edit over the real page value for display — never touches the underlying
 *  page/decode cache, which stays the server's own last-read value until commit/discard. */
export function displayCell(
  tabId: string,
  page: TabularPage | null,
  order: readonly string[],
  row: number,
  displayCol: number,
): DisplayCellView {
  const name = order[displayCol];
  const staged = name ? usePendingChangesStore().stagedValue(tabId, row, name) : undefined;
  if (staged !== undefined) {
    return { text: staged ?? '', isNull: staged === null, truncated: false, staged: true };
  }
  if (!page) return { text: '', isNull: true, truncated: false, staged: false };
  const pageCol = pageColumnIndexFor(page, order as string[], displayCol);
  if (pageCol < 0) return { text: '', isNull: true, truncated: false, staged: false };
  const view = cell(tabId, row, pageCol);
  return { ...view, staged: false };
}

/** The row's effective values across the whole display column order — reused by row copy and
 *  Duplicate row. */
export function rowSnapshot(
  tabId: string,
  page: TabularPage | null,
  order: readonly string[],
  row: number,
): RowSnapshot {
  const values: Record<string, string | null> = {};
  const truncatedSet = new Set<string>();
  for (let c = 0; c < order.length; c++) {
    const name = order[c] as string;
    const dc = displayCell(tabId, page, order, row, c);
    values[name] = dc.isNull ? null : dc.text;
    // F2/P21 round 1: `.text` is only the 64 KiB prefix the engine sent for a truncated cell —
    // carried here so a format that generates something meant to be *run* (rowsToInsert) can
    // refuse to treat it as the real value.
    if (dc.truncated) truncatedSet.add(name);
  }
  return {
    columns: [...order],
    values,
    truncated: truncatedSet.size > 0 ? truncatedSet : undefined,
  };
}

// P107 T2-17: rowsForColumnOps/visibleRowsInSpan moved to ../../shared/slick/rowVisibility.ts —
// they carry no grid-specific dependency, unlike the rest of this file — re-exported here so
// every existing `from './slick/rowValues'` site (SlickGridHost.vue) is unchanged.
export { rowsForColumnOps, visibleRowsInSpan };

/** Paste's own version of the same rule: a paste starting at `startRow`
 *  writes `count` clipboard rows in order, but `startRow + ri` (the old, purely arithmetic target)
 *  can land on a row the active filter is hiding. The ri-th clipboard row instead lands on the
 *  ri-th *visible* page row at or after `startRow` — `displayRows` only ever holds real, loaded
 *  page rows (never a pending-insert index), so once the walk runs past the last of them it
 *  continues contiguously from `Math.max(startRow, rowCount)`, the pending-insert region, which is
 *  never filtered (mirrors `isRowVisible`'s own pending-insert-is-always-visible rule). */
export function pasteTargetRows(
  displayRows: readonly number[] | null,
  rowCount: number,
  startRow: number,
  count: number,
): number[] {
  if (count <= 0) return [];
  if (!displayRows) return Array.from({ length: count }, (_, i) => startRow + i);
  const out: number[] = [];
  for (const r of displayRows) {
    if (out.length >= count) break;
    if (r >= startRow) out.push(r);
  }
  let next = Math.max(startRow, rowCount);
  while (out.length < count) out.push(next++);
  return out;
}

/** Real-interaction fix (reported bug family — right-click Copy on a multi-row selection was a
 *  no-op, Delete did nothing for a column or whole-table selection, and the keyboard shortcut and
 *  the right-click menu could each reach a different answer for the same selection): the single
 *  conversion every "act on whatever rows the current selection covers" call site must go through
 *  — SlickGridHost.vue's own onKeydown (Delete/Duplicate), its onGridContextMenu (right-click,
 *  routes to rowMenu() the same way the gutter path always has), and DataToolbar.vue's own
 *  toolbar Delete button. Before this, each of those three hand-rolled its own row/cell/range
 *  switch, and none of the three ever grew a `column` branch — a `Selection.kind` this file (and
 *  onCopy, which already handles all four kinds) has covered,
 *  never plumbed through to Delete. One switch, covering every `Selection` kind that exists,
 *  is the fix: a kind newly needs coverage here exactly once, not in three call sites that can
 *  silently drift out of sync with each other.
 *  `displayRows`/`rowCount` mirror `visibleRowsInSpan`'s own contract (real page rows only,
 *  honouring an active search filter); ascending order, no duplicates. */
export function rowsForSelection(
  sel: Selection,
  displayRows: readonly number[] | null,
  rowCount: number,
): number[] {
  switch (sel.kind) {
    case 'row':
      return [...sel.rows].sort((a, b) => a - b);
    case 'cell':
      return [sel.row];
    case 'range':
      return visibleRowsInSpan(displayRows, sel.anchorRow, sel.row);
    case 'column':
      // A column selection spans every currently-displayed row (the header-select-zone
      // semantics — clicking a header selects the whole column, not a row range within it), so
      // "the rows this selection covers" is every visible row, same as rowsForColumnOps' own
      // column-scoped-op rule already applies to Copy for this exact kind.
      return rowsForColumnOps(displayRows, rowCount);
    default:
      return [];
  }
}

/** The loaded page's values only for one column (§8.5's own scope boundary) — the header menu's
 *  "Copy column values". */
export function columnValuesFor(
  tabId: string,
  page: TabularPage | null,
  order: readonly string[],
  displayRows: readonly number[] | null,
  displayCol: number,
): string[] {
  if (!page) return [];
  const out: string[] = [];
  for (const r of rowsForColumnOps(displayRows, page.rowCount)) {
    const dc = displayCell(tabId, page, order, r, displayCol);
    out.push(dc.isNull ? '' : dc.text);
  }
  return out;
}

export interface NavColumns {
  fk: Set<string>;
  pk: Set<string>;
  valueNames: string[];
}

/** P29 D6: the cheap precheck that makes cellNavEntry affordable — the two predicates menu.ts's
 *  own foreignKeyNavItems/referencedByItems already apply, computed once (not per cell/hover). */
export function navColumnsFor(meta: ObjectMeta | null): NavColumns {
  const fk = new Set<string>();
  const pk = new Set<string>();
  const valueNames = new Set<string>();
  if (meta) {
    for (const edge of meta.foreignKeys) {
      for (const name of edge.columns) {
        fk.add(name);
        valueNames.add(name);
      }
    }
    if (meta.primaryKey && meta.referencedBy.length > 0) {
      for (const name of meta.primaryKey) pk.add(name);
      for (const edge of meta.referencedBy) {
        for (const name of edge.columns) valueNames.add(name);
      }
    }
  }
  return { fk, pk, valueNames: [...valueNames] };
}

/** Nav columns the page types as numbers (page `typeClass`): an FK filter writes their values
 *  bare, not as string literals. */
export function numericNavColumns(page: TabularPage | null, navColumns: NavColumns): Set<string> {
  const out = new Set<string>();
  if (!page) return out;
  for (const name of navColumns.valueNames) {
    if (page.columns.find((c) => c.name === name)?.typeClass === 'number') out.add(name);
  }
  return out;
}

/** rowSnapshot(row) narrowed to navColumns.valueNames, memoised per row for one render/hover via
 *  the optional cache the caller passes — a table with no FK and no inbound reference (the common
 *  case) never builds one at all, since valueNames is then empty. */
export function navValuesFor(
  tabId: string,
  page: TabularPage | null,
  order: readonly string[],
  navColumns: NavColumns,
  row: number,
  cache?: Map<number, Record<string, string | null>>,
): Record<string, string | null> {
  const cached = cache?.get(row);
  if (cached) return cached;
  const out: Record<string, string | null> = {};
  for (const name of navColumns.valueNames) {
    const c = order.indexOf(name);
    if (c < 0) continue;
    const dc = displayCell(tabId, page, order, row, c);
    out[name] = dc.isNull ? null : dc.text;
  }
  cache?.set(row, out);
  return out;
}

export interface CellNavEntry {
  kind: 'fk' | 'pk';
  items: MenuItem[];
}

/** The single source of truth for a cell's nav affordance — both the button
 *  and the cell menu's FK/PK items read this, so they can never disagree about what's showing.
 *  'fk' wins over 'pk' when a cell is somehow both; null before meta has loaded or when
 *  there's nothing navigable. Unlike DataGrid.vue's own version, this does not veto on "is this
 *  cell being edited" — that is a SlickGrid-editor concept, and this module carries no
 *  SlickGrid API by design; the placement call site is where that veto belongs. */
export function cellNavEntry(
  tabId: string,
  page: TabularPage | null,
  order: readonly string[],
  meta: ObjectMeta | null,
  connectionId: string | null,
  dialect: SqlDialect | undefined,
  navColumns: NavColumns,
  row: number,
  displayCol: number,
  navCache?: Map<number, Record<string, string | null>>,
): CellNavEntry | null {
  const name = order[displayCol];
  if (!name || !meta || !connectionId) return null;
  if (!navColumns.fk.has(name) && !navColumns.pk.has(name)) return null;
  const fkCtx: FkNavContext = {
    connectionId,
    dialect,
    rowValues: navValuesFor(tabId, page, order, navColumns, row, navCache),
    numericColumns: numericNavColumns(page, navColumns),
  };
  const fkItems = foreignKeyNavItems(name, meta, fkCtx).filter(
    (i) => i.type === 'item' && !i.disabled,
  );
  if (fkItems.length) return { kind: 'fk', items: fkItems };
  const refItems = referencedByItems(name, meta, fkCtx).filter(
    (i) => i.type === 'item' && !i.disabled,
  );
  if (refItems.length) return { kind: 'pk', items: refItems };
  return null;
}
