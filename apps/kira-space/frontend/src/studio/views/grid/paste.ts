// SlickGridHost.vue's onPaste's pure target-resolution and cell-staging logic, lifted out with
// everything it needs passed in explicitly — never reaching back into the SFC's own reactive scope.
// The SFC keeps the guard clauses, the clipboard read, and these two calls.
import type { TabularPage } from '@shared/protocol/page';
import type { Selection } from '../shared/slick/selection';
import { usePendingChangesStore } from './pendingChanges';
import { pasteTargetRows } from './slick/rowValues';

export interface PasteTarget {
  startCol: number;
  /** The page row a pasted row `ri` (0-based, within the clipboard's own rows) lands on, or
   *  `undefined`/negative to skip it (a `range` paste's own target list can run out before the
   *  clipboard does, `pasteTargetRows`'s own contract). */
  rowAt: (ri: number) => number | undefined;
}

/** Target resolution: every kind now goes through the same filter/selection-aware mapping `range`
 *  already used — `row` and `cell` used to fall back to plain `startRow + ri` arithmetic, which is
 *  wrong on two counts: for `row`, it ignored which rows were selected (a ctrl-click selection of
 *  rows 2, 7, 9 pasted into 2, 3, 4); for both, `startRow + ri` can land on a row the active
 *  search filter hides, staging an invisible write the user never saw until the filter cleared.
 *
 *  `row` kind: clipboard row `ri` maps onto the `ri`-th selected row in ascending order. When the
 *  clipboard has fewer rows than the selection, later selected rows are never reached (the
 *  caller's own loop only ever calls `rowAt` for `ri < pastedRowCount`) — nothing to explicitly
 *  stop. When the clipboard has *more* rows than the selection, the remainder continues into the
 *  insert region right after the last real row, exactly like `range`'s own overflow.
 *
 *  `cell` kind: same filtered walk as `range`, starting from the one anchor cell/row — a multi-row
 *  clipboard pasted from a single cell must skip hidden rows the same way a range paste already
 *  does, not walk past them. */
export function resolvePasteTarget(
  sel: Extract<Selection, { kind: 'cell' | 'range' | 'row' }>,
  page: TabularPage,
  displayRows: readonly number[] | null,
  pastedRowCount: number,
): PasteTarget {
  if (sel.kind === 'row') {
    const sortedRows = [...sel.rows].sort((a, b) => a - b);
    const lastSelected = sortedRows[sortedRows.length - 1] ?? -1;
    return {
      startCol: 0,
      rowAt: (ri) =>
        ri < sortedRows.length
          ? sortedRows[ri]
          : Math.max(lastSelected + 1, page.rowCount) + (ri - sortedRows.length),
    };
  }
  const startRow = sel.kind === 'range' ? sel.anchorRow : sel.row;
  const startCol = sel.kind === 'range' ? sel.anchorCol : sel.col;
  const targetRows = pasteTargetRows(displayRows, page.rowCount, startRow, pastedRowCount);
  return {
    startCol,
    rowAt: (ri) => targetRows[ri],
  };
}

function columnIsGenerated(page: TabularPage, name: string): boolean {
  return page.columns.find((c) => c.name === name)?.generated ?? false;
}

/** One clipboard row's own cells, staged at `startCol..` — an existing page row via `stageEdit`,
 *  a new one (`isNewRow`) via `stageInsertValue`, skipping a generated column either way. */
function stagePastedRow(
  tabId: string,
  page: TabularPage,
  columns: readonly string[],
  startCol: number,
  row: number,
  isNewRow: boolean,
  insertId: string | undefined,
  cols: readonly string[],
): void {
  const pendingChangesStore = usePendingChangesStore();
  for (let ci = 0; ci < cols.length; ci++) {
    const name = columns[startCol + ci];
    if (!name) continue;
    if (isNewRow) {
      if (insertId && !columnIsGenerated(page, name)) {
        pendingChangesStore.stageInsertValue(tabId, insertId, name, cols[ci] as string);
      }
    } else if (!columnIsGenerated(page, name)) {
      // This branch used to stage unconditionally — the server computes a
      // generated column's value, so an existing-row paste into one staged an UPDATE the engine
      // refuses at commit, the one paste path the doc comment above already (wrongly) claimed
      // skipped it.
      pendingChangesStore.stageEdit(tabId, row, name, cols[ci] as string);
    }
  }
}

/** Stages every pasted cell (`parsed`, one string[] per clipboard row) at `target`'s rows/
 *  columns — an existing page row via `stageEdit`, a new row (`row >= page.rowCount`) via
 *  `addInsertRow`/`stageInsertValue`. A paste landing on a display row that already has a
 *  staged insert updates that insert's `values` in place rather than always appending a sibling
 *  — `pending.inserts` is positional (`inserts[row - page.rowCount]`). Returns every display row
 *  whose insert (new or already-staged) this call touched — the caller's own render/invalidate
 *  pass needs both (a reused insert's `<input>` needs invalidating too). */
export function applyPastedCells(
  tabId: string,
  page: TabularPage,
  columns: readonly string[],
  target: PasteTarget,
  parsed: readonly (readonly string[])[],
): ReadonlySet<number> {
  // P36 D28: the server computes a generated column's value — an explicit paste into one is
  // silently dropped rather than staged into an insert the server would then reject outright.
  const insertColumns = columns.filter((name) => !columnIsGenerated(page, name));
  const insertIds = new Map<number, string>();
  const pendingChangesStore = usePendingChangesStore();
  const pending = pendingChangesStore.pendingFor(tabId);

  for (let ri = 0; ri < parsed.length; ri++) {
    const row = target.rowAt(ri);
    if (row === undefined || row < 0) continue;
    const isNewRow = row >= page.rowCount;
    let insertId = insertIds.get(row);
    if (isNewRow && insertId === undefined) {
      insertId =
        pending?.inserts[row - page.rowCount]?.id ??
        pendingChangesStore.addInsertRow(tabId, insertColumns);
      insertIds.set(row, insertId);
    }
    stagePastedRow(tabId, page, columns, target.startCol, row, isNewRow, insertId, parsed[ri]);
  }
  return new Set(insertIds.keys());
}
