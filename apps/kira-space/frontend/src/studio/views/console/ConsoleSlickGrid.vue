<script setup lang="ts">
import type { TabularPage } from '@shared/protocol/page';
import { t } from '@theme/i18n';
import { copyOrReportError } from '@workbench/behaviour/clipboard';
import { announceApp } from '@workbench/behaviour/liveRegion';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { shortcutFor } from '@workbench/shortcuts/keys';
import { useContextMenuStore } from '@workbench/state/contextMenu';
import type {
  Column,
  CustomDataView,
  FormatterResultWithText,
  OnClickEventArgs,
  OnHeaderClickEventArgs,
  SlickEventData,
} from 'slickgrid';
import { SlickEventHandler, type SlickHybridSelectionModel, type SlickRange } from 'slickgrid';
import { onMounted, onUnmounted, ref, watch } from 'vue';
import { useCellSelectionStore } from '../../state/cellSelection';
import { useSettingsStore } from '../../state/settings';
import { classesFrom } from '../../theme/cellClass';
import { categoryForTypeClass } from '../../theme/icons';
import { disambiguateNames, type RowSnapshot } from '../shared/clipboardFormats';
import { nullCellText, truncatedCellTip } from '../shared/page/cellText';
import {
  alignmentFor,
  columnHeaderTooltip,
  DEFAULT_COLUMN_WIDTH,
  headerAwareMinWidth,
  initialWidthsByIndex,
  resetMeasureCtx,
  tooltipAttrs,
} from '../shared/page/columns';
import { setVisibleRows } from '../shared/page/visibleRows';
import { searchCellLayers } from '../shared/slick/cssLayers';
import {
  createGridDataSource,
  type DisplayRowIndex,
  displayPositionOf,
  type GridDataSourceState,
  type RowHandle,
} from '../shared/slick/dataSource';
import {
  computeCellFillHash,
  gutterColumn,
  renderedPageRowBand,
  subscribeRangeSelecting,
  subscribeRowHeight,
} from '../shared/slick/gridHostShared';
import { KiraSlickGrid } from '../shared/slick/kiraSlickGrid';
import {
  rowsForColumnOps as sharedRowsForColumnOps,
  visibleRowsInSpan as sharedVisibleRowsInSpan,
} from '../shared/slick/rowVisibility';
import { createScrollVelocityTracker } from '../shared/slick/scrollVelocity';
import {
  rangesFromSelection,
  type Selection,
  selectionCovers,
  selectionFromRanges,
} from '../shared/slick/selection';
import { type CopyFormat, type CopySource, selectionText } from '../shared/slick/selectionCopy';
import { attachScrollProbes, createHybridSelectionModel } from '../shared/slick/selectionModel';
import '../shared/slick/slickTheme.css';
import 'slickgrid/dist/styles/css/slick.grid.css';
import { tabularCellMenu, tabularColumnMenu, tabularRangeMenu, tabularRowMenu } from './menu';
import { cell, getPage, setVisibleWindow } from './resultPages';
import type { ResultViewState } from './resultViewState';
import { type Match, matchedRows, searchState } from './search';
import { useConsoleViewStore } from './state';

const cellSelectionStore = useCellSelectionStore();
const contextMenuStore = useContextMenuStore();
const settingsStore = useSettingsStore();
const consoleViewStore = useConsoleViewStore();

// P30 §3 — the console result grid's tabular branch, migrated off @tanstack/vue-virtual onto the
// same KiraSlickGrid/dataSource.ts/slickTheme.css layer views/grid/SlickGridHost.vue already uses
// (F1's own table: that host itself is bound to a data tab and cannot be reused directly, but the
// layer beneath it has no such dependency). A console result has no pager, no sort, no
// pending-changes and no persisted column widths/order — every one of SlickGridHost.vue's other
// features exists to serve those, so this file only wires the handful a read-only result set
// actually needs: gutter, cell colour/alignment, search, and the decode-window report.
//
// P19 D8: the selection model is the one exception — P43 iter2 D22's "never more than one cell
// selected at once" finding is superseded here, configured identically to SlickGridHost.vue's own
// SlickHybridSelectionModel (rows/columns/free-form ranges, ⌘C and a context menu per kind,
// console/menu.ts) so the console doesn't reinvent a different selection model than the app's main
// grid uses. What stays console-specific: no pending-changes-aware rowSnapshot, no Copy as INSERT
// (no addressable table), no paste/delete/duplicate (read-only by construction), and a filter
// change clears the selection outright rather than remapping it (D8 point 1).
//
// §3.5 rule 1: the scroll mechanism (KiraSlickGrid's runway/budget/chase) is inherited, never
// re-derived — `grid.velocity`/`lastScrollEventAt`/`scrollEventSeq` are wired the same four-field
// way SlickGridHost.vue's own onMounted does, off the shared sampler in shared/slick/
// scrollVelocity.ts (P107 T2-17 — SlickGridHost.vue uses the same instance-per-host tracker).
// §3.5 rule 2: no Vue reactivity on row data — grid/dataSource/viewport/handlers below are plain
// `let`s, never `ref`/`shallowRef`/`reactive`; every imperative call into the grid happens from a
// `watch` callback or a DOM event handler, never from inside a `computed`.
// §3.5 rule 3: a formatter returns text, never DOM.
const props = defineProps<{
  pageKey: string;
  tabId: string;
  connectionId: string | null;
  path: string;
  /** Opt-in view-state persistence (the proof window): restored on mount, reported on every
   *  scroll, selection and column resize. `undefined` for the console, which keeps none. */
  viewState?: ResultViewState | null;
}>();
const emit = defineEmits<{
  copyError: [message: string];
  'update:viewState': [state: ResultViewState];
  ready: [];
}>();
const onCopyError = (message: string): void => emit('copyError', message);

// Column<T>'s own `field` type can't be satisfied by RowHandle's shape (arbitrary db column
// names) — same escape hatch kiraSlickGrid.ts/SlickGridHost.vue already use.
// biome-ignore lint/suspicious/noExplicitAny: see comment above.
type KiraColumn = Column<any>;

const GUTTER_FIELD = '__kira_gutter';

// A console result comes from ad-hoc SQL (a JOIN where both tables have an
// `id` column, `SELECT 1 AS x, 2 AS x`, ...), so `page.columns[i].name` is routinely NOT unique.
// Every SlickGrid column needs a unique `id`/`field` regardless — addressing columns by an
// index-derived field (rather than by `col.name`, as the tabular branch this migrated off of never
// did: it addressed cells by column INDEX) keeps duplicate-named columns distinct throughout;
// `col.name` is used only for the header's own display text/tooltip below.
function colField(index: number): string {
  return `col_${index}`;
}

// The inverse of colField — parses the page-column index back out of a SlickGrid field/column id.
// Returns -1 for the gutter or anything else that isn't one of this file's own data-column fields.
function colIndexFromField(field: string): number {
  if (!field.startsWith('col_')) return -1;
  const idx = Number(field.slice(4));
  return Number.isInteger(idx) && idx >= 0 ? idx : -1;
}

function gutterFormatter(
  _row: number,
  _cell: number,
  _value: unknown,
  _columnDef: KiraColumn,
  dataContext: RowHandle,
): string {
  return String(dataContext.row + 1);
}

// NULL and truncated fold onto the cell's own className via
// `FormatterResultWithText.addClasses` rather than building a child node — mirrors
// SlickGridHost.vue's own cellFormatter, minus the FK/PK nav and pending-insert branches (a
// console result has neither).
function cellFormatter(
  _row: number,
  _cell: number,
  value: unknown,
  _columnDef: KiraColumn,
  _dataContext: RowHandle,
): string | FormatterResultWithText {
  const view = value as { text: string; isNull: boolean; truncated: boolean };
  if (view.isNull) return { text: nullCellText(), addClasses: 'cell-null' };
  if (view.truncated) {
    return { text: view.text, addClasses: 'cell-truncated', toolTip: truncatedCellTip() };
  }
  return view.text;
}

// The header tooltip has to be written as attributes by hand (SlickGrid's own
// `Column` shape carries no child markup) — tooltipAttrs (views/shared/page/columns.ts): the
// hover controller (`workbench/state/tooltip.ts`) triggers off `data-kira-tip` (the plain,
// newline-joined text) and only then reads `data-kira-tip-parts` for the structured content,
// so both attributes are required, not just the structured one.

// By deliberate design — a console result has no tab state the way SlickGridHost.vue's own
// DataTabState.columnWidths does. What changed is where the memory lives.
// `consoleColumnWidths`/`setConsoleColumnWidths` (state.ts) key it by column *name* on the tab's
// own runtime record instead — the first time this tab ever measures a given
// column name (this tab's first query, or any later query whose result introduces a column this
// tab hasn't shown before), the measured width is committed as that name's sticky width for the
// rest of the session; every subsequent build (`:key="pageKey"` in ConsoleResultGrid.vue remounts
// this component on every run) reuses it verbatim, exactly like SlickGridHost.vue's own
// `buildColumns`'s `storedWidths[name] ?? measured[name]` already does for the SQL data view.
// A manual resize (onColumnsResized, below) is committed the same way. Every column still gets
// `sortable: false` (no re-query path). The gutter is now focusable/selectable —
// `rowSelectColumnIds` (below) needs it to compute a row selection when the active cell lands
// there; Tab/Left-arrow landing on the gutter is the one side effect (SlickGridHost.vue's own
// finding, verbatim).
function buildColumns(page: TabularPage): KiraColumn[] {
  const cols: KiraColumn[] = [
    gutterColumn(GUTTER_FIELD, gutterFormatter, 'console-result-gutter-cell'),
  ];
  // Indexed, not name-keyed (`initialWidths` would silently collide on a duplicate column name,
  // e.g. `SELECT 1 AS x, 2 AS x`) — the one remaining name-keyed assumption in an otherwise fully
  // index-addressed console path (colField(i)).
  // The session-scoped sticky-width store (below) is the one deliberate exception: it is keyed by
  // name because that is the identity a *later, different* result set's columns must match against
  // to inherit a width at all — a duplicate-named result shares one sticky width across its
  // same-named columns, same tradeoff SlickGridHost.vue's own tab.state.columnWidths already
  // accepts (it just never sees a duplicate, a real table's columns being unique by construction).
  const measured = initialWidthsByIndex(page);
  const stored = consoleViewStore.consoleColumnWidths(props.tabId);
  const nextStored: Record<string, number> = { ...stored };
  let storedChanged = false;
  page.columns.forEach((col, i) => {
    const classes = [`tc-${categoryForTypeClass(col.typeClass)}`];
    if (alignmentFor(col) === 'right') classes.push('kira-align-right');
    const tooltip = columnHeaderTooltip(col, col.dataType);
    // Every column here is `sortable: false` (no re-query path, below) and renders no PK/FK badge,
    // so this reduces to `max(MIN_WIDTH, ceil(headerText + 16))` — the console grid's header
    // furniture is 16px of padding only, which measuredWidths' own CELL_PADDING already covered, so
    // this is provably unchanged for every name short enough to matter.
    const floor = headerAwareMinWidth(col.name, { padding: 16, sortControl: 0, keyBadge: 0 });
    // This tab's own sticky width for this column name wins over a fresh measurement — the first
    // time a name is ever seen in this tab (a brand new session, or a query returning a column
    // this tab hasn't shown before) measures it and commits the result below so every later query
    // in the same tab finds it here instead of re-measuring.
    const width = stored[col.name] ?? measured[i] ?? DEFAULT_COLUMN_WIDTH;
    if (stored[col.name] === undefined) {
      nextStored[col.name] = width;
      storedChanged = true;
    }
    cols.push({
      id: colField(i),
      field: colField(i),
      name: col.name,
      // measured[i] already carries columns.ts's own MIN_WIDTH floor (measuredWidths' own
      // Math.max clamp), and DEFAULT_COLUMN_WIDTH (96) is above it too — width itself needs no
      // extra clamp here, unlike SlickGridHost.vue's storedWidths path. minWidth still needs to
      // match, so an interactive drag can't undercut what a fresh render already guarantees.
      width,
      minWidth: floor,
      resizable: true,
      sortable: false,
      cssClass: classes.join(' '),
      formatter: cellFormatter,
      cellAttrs: {
        'data-testid': 'console-result-cell',
        'data-column': col.name,
        'data-col-index': String(i),
      },
      headerCellAttrs: {
        'data-testid': 'console-result-header-cell',
        'data-column': col.name,
        ...tooltipAttrs(tooltip),
      },
    });
  });
  if (storedChanged) consoleViewStore.setConsoleColumnWidths(props.tabId, nextStored);
  return cols;
}

const { single: rowHeight } = useRowHeight();

const rootRef = ref<HTMLElement | null>(null);

// §3.5 rule 2 — never a ref/shallowRef/reactive (CodeMirrorHost.vue's/SlickGridHost.vue's own
// rule, restated here).
let grid: KiraSlickGrid | null = null;
let eventHandler: SlickEventHandler | null = null;
let dataSource: ReturnType<typeof createGridDataSource> | null = null;
let viewportEl: HTMLElement | null = null;
let resizeObserver: ResizeObserver | null = null;
let page: TabularPage | null = null;

// P107 T2-17: sampler itself moved to shared/slick/scrollVelocity.ts (byte-identical to
// SlickGridHost.vue's own) — scrollEventSeq stays local, a separate simpler counter the tracker
// doesn't own.
let scrollEventSeq = 0;
const scrollVelocityTracker = createScrollVelocityTracker(() => viewportEl);

function onViewportScroll(): void {
  const el = viewportEl;
  if (!el) return;
  scrollEventSeq++;
  scrollVelocityTracker.recordOffsetSample(el.scrollTop, performance.now());
  emitViewState();
}

// Set while a saved state is applied, so the scroll and selection events it causes are not
// reported back as the user's own.
let restoring = false;

function emitViewState(): void {
  if (props.viewState === undefined || restoring) return;
  emit('update:viewState', {
    resultScrollTop: viewportEl?.scrollTop ?? 0,
    resultScrollLeft: viewportEl?.scrollLeft ?? 0,
    selection: currentSelection,
    selectedRow: null,
    expanded: [],
    columnWidths: { ...consoleViewStore.consoleColumnWidths(props.tabId) },
  });
}

// The one per-render DOM pass this migration still needs: a row's own `data-row` (SlickGrid writes
// the *display position* there) is corrected to the *page* row every other subsystem (the cell
// editor panel, search) addresses a row by. Idempotent by construction: a `.slick-row` already
// carrying `data-kira-row-tagged` is skipped. The row div is cloned per frozen pane; only the right
// (data) pane's clone gets `data-testid="console-result-row"`, matching the incumbent VirtualList's
// own one-testid-per-row.
function tagRenderedRows(): void {
  const root = rootRef.value;
  if (!root || !dataSource) return;
  const rowEls = root.querySelectorAll<HTMLElement>('.slick-row:not([data-kira-row-tagged])');
  for (const el of rowEls) {
    const pos = Number(el.dataset.row);
    if (!Number.isFinite(pos)) continue;
    const handle = dataSource.getItem(pos);
    el.dataset.row = String(handle.row);
    el.dataset.kiraRowTagged = '1';
    if (el.closest('.grid-canvas-right')) {
      el.dataset.testid = 'console-result-row';
    }
  }
}

// P30 §3.6 C3 — the decode-window/retention report (P5 C1): the same setVisibleRows +
// setVisibleWindow pair ConsoleResultGrid.vue's own onVisibleRangeIndices made from
// VirtualList's `visible-range` emit, now driven from grid.onRendered/lastRenderedRowBounds the
// way SlickGridHost.vue's own onGridRendered does — the *rendered* (overscanned) band, not the
// narrower strictly-visible one, so a row still inside the runway keeps its decode cache alive.
function onGridRendered(): void {
  if (!grid || !dataSource) return;
  const band = renderedPageRowBand(grid, dataSource);
  if (!band) return;
  setVisibleRows(props.tabId, band.lo, band.hi + 1);
  setVisibleWindow(props.pageKey, band.lo, band.hi + 1);
}

function fieldAtCol(colIdx: number): string | undefined {
  const c = grid?.getColumns()[colIdx];
  return c ? String(c.field) : undefined;
}

// The drag-in-progress twin of the fill SlickGrid draws at commit through
// `selectedCellCssClass` (grid options below) — written to that same 'kira-cell-selected' layer key
// so a commit's real write and this preview's writes replace one another cleanly rather than both
// existing at once. Bounded by the rendered band, exactly like the edge layer.
//
// SlickGridHost.vue has had this; this file never did, which is the whole
// of the reported "selection highlight appears only on mouse-up". SlickHybridSelectionModel's
// `handleCellRangeSelected` deliberately no-ops for a CELL-mode `onCellRangeSelecting` call (only
// the drag-END `onCellRangeSelected` reaches `setSelectedRanges`), and `dragToSelect: true` —
// needed for the gutter's own row-range drag, which has no such early return and so already
// painted live — zeroes the stock decorator's border, so a cell-range drag painted nothing at all
// until mouseup.
// Deliberately never writes `currentSelection`: a drag passing back over its own anchor cell
// transiently looks like a completed one-cell selection, and the committed
// `onSelectedRangesChanged` supersedes every layer this writes the moment the drag ends.
function onCellRangeSelecting(_e: unknown, args: { range: SlickRange }): void {
  if (!grid || !dataSource || !selectionModel || selectionModel.currentSelectionModeIsRow()) return;
  const posSel = selectionFromRanges([args.range], false, null);
  const pageSel = posSel ? toPageRowSelection(posSel) : null;
  const ds = dataSource;
  grid.setCellCssStyles(
    'kira-cell-selected',
    computeCellFillHash(
      pageSel,
      grid.lastRenderedRowBounds,
      (pos) => ds.getItem(pos).row,
      (c) => colField(c),
    ),
  );
}

// A real SlickHybridSelectionModel, configured identically to SlickGridHost.vue's own
// ("so the query console doesn't reinvent a different selection model than the app's main grid
// uses"). `currentSelection` is this file's own `rt().selection` equivalent: plain, non-reactive
// (rule 2), page-row space (never a display position), populated by onSelectedRangesChanged below
// and read by onCopy/onKeydown/onContextMenu.
let selectionModel: SlickHybridSelectionModel | null = null;
// One-shot flag consumed by the next onSelectedRangesChanged call — selection.ts's own documented
// shape (selectionFromRanges' own `pendingKind` parameter).
let pendingSelectionKind: 'column' | null = null;
let currentSelection: Selection | null = null;

// The translation `selection.ts` deliberately leaves to its caller: a `SlickRange`'s rows are
// DISPLAY positions, `Selection`'s are PAGE rows — SlickGridHost.vue's own toPageRowSelection,
// using this file's existing dataSource.getItem(pos).row rather than a second index structure.
function toPageRowSelection(sel: Selection): Selection {
  if (!dataSource) return sel;
  const toPageRow = (pos: number) => dataSource?.getItem(Math.max(0, pos)).row ?? pos;
  switch (sel.kind) {
    case 'cell':
      return { ...sel, row: toPageRow(sel.row) };
    case 'range':
      return { ...sel, anchorRow: toPageRow(sel.anchorRow), row: toPageRow(sel.row) };
    case 'row':
      return { ...sel, rows: sel.rows.map(toPageRow) };
    default:
      return sel;
  }
}

// Fires on every setSelectedRanges call: a click, a drag end, the gutter's own row-select click,
// this file's own header-click column push (below).
function onSelectedRangesChanged(_e: unknown, ranges: SlickRange[]): void {
  const rowMode = selectionModel?.currentSelectionModeIsRow() ?? false;
  const kind = pendingSelectionKind;
  pendingSelectionKind = null;
  const posSel = selectionFromRanges(ranges, rowMode, kind);
  currentSelection = posSel ? toPageRowSelection(posSel) : null;
  emitViewState();
}

// Mirrors SlickGridHost.vue's own onColumnsResized: SlickGrid drags the
// handle and persists nothing on its own, so a manual resize is read straight off getColumns() and
// committed into this tab's session-scoped sticky-width store (state.ts's consoleColumnWidths),
// the same place buildColumns' own first-measurement commit (above) writes to — a manual resize
// and an auto-measured width are stored identically, so a later query in the same tab can't tell
// (and doesn't need to) which one produced the width it's now honouring.
function onColumnsResized(): void {
  if (!grid) return;
  const widths: Record<string, number> = { ...consoleViewStore.consoleColumnWidths(props.tabId) };
  for (const col of grid.getColumns()) {
    if (col.id === GUTTER_FIELD || col.width === undefined) continue;
    widths[String(col.name)] = col.width;
  }
  consoleViewStore.setConsoleColumnWidths(props.tabId, widths);
  emitViewState();
}

// F15: no `.header-select-zone`, no `onHeaderCellRendered` subscription — a console result has no
// sort at all (every column is `sortable: false`) and no re-query path, so a plain header body
// click is free to mean "select this column" outright.
function onGridHeaderClick(_e: unknown, args: OnHeaderClickEventArgs): void {
  if (!grid || !selectionModel || !page) return;
  if (args.column.id === GUTTER_FIELD) return;
  const displayCol = colIndexFromField(String(args.column.field));
  if (displayCol < 0) return;
  const displayRowCount = matchedRows(props.tabId)?.length ?? page.rowCount;
  const colCount = grid.getColumns().length - 1; // minus the gutter
  pendingSelectionKind = 'column';
  selectionModel.setSelectedRanges(
    rangesFromSelection({ kind: 'column', cols: [displayCol] }, displayRowCount, colCount),
  );
}

// The click handler's other job (unchanged): publishing into the read-only cell-editor dock. The
// one-cell `kira-cell-selected` bookkeeping this used to carry (selectedRow/selectedField/
// setCellCssStyles) is gone — `selectedCellCssClass` now makes SlickGrid itself paint the
// highlight straight off `selectionModel`'s own ranges.
function onGridClick(_e: SlickEventData, args: OnClickEventArgs): void {
  if (!grid || !dataSource || !page) return;
  if (args.cell === 0) return; // the gutter selects a row via rowSelectColumnIds, not a publish
  const field = fieldAtCol(args.cell);
  if (!field) return;
  const pageCol = colIndexFromField(field);
  if (pageCol < 0) return;
  const column = page.columns[pageCol];
  if (!column) return;
  const handle = dataSource.getItem(args.row);
  const view = cell(props.pageKey, handle.row, pageCol);
  cellSelectionStore.publishSelectedCell({
    tabId: props.tabId,
    connectionId: props.connectionId,
    path: props.path,
    columnIndex: pageCol,
    column,
    row: handle.row,
    value: view.isNull ? null : view.text,
    truncated: view.truncated,
    hasPrimaryKey: column.isPrimaryKey,
    // No onEdit/onRevert: a console result has no addressable row/table to write a change back
    // to, so this stays view-only in the cell editor panel — same as ConsoleResultGrid.vue's own
    // former `selectTabularCell`.
  });
}

function cellAt(row: number, col: number): { text: string; isNull: boolean } {
  return cell(props.pageKey, row, col);
}

// Keyboard copy and every Copy menu item read through this one source. Row snapshots memoize per
// source, so a menu's items share one pass over the selected rows.
function copySource(): CopySource {
  // A console result can repeat a column name (self-joins, `SELECT 1 AS x, 2 AS x`) -- disambiguated
  // so `values` never loses an occurrence to a same-named later one.
  const names = disambiguateNames(page?.columns.map((c) => c.name) ?? []);
  const snapshots = new Map<number, RowSnapshot>();
  return {
    cellAt,
    rowSnapshot: (row) => {
      let snap = snapshots.get(row);
      if (!snap) {
        const values: Record<string, string | null> = {};
        names.forEach((name, i) => {
          const dc = cellAt(row, i);
          values[name] = dc.isNull ? null : dc.text;
        });
        snap = { columns: names, values };
        snapshots.set(row, snap);
      }
      return snap;
    },
    columnName: (col) => page?.columns[col]?.name ?? '',
    rowsInSpan: visibleRowsInSpan,
    allRows: rowsForColumnOps,
  };
}

// D9's own column-scoped rule — views/shared/slick/rowVisibility.ts's own pure functions (P107
// T2-17: split out of grid/slick/rowValues.ts, whose other exports console can't import, F13),
// bound to this file's own displayRows/page.
function rowsForColumnOps(): number[] {
  return sharedRowsForColumnOps(matchedRows(props.tabId), page?.rowCount ?? 0);
}

function visibleRowsInSpan(r0: number, r1: number): number[] {
  return sharedVisibleRowsInSpan(matchedRows(props.tabId), r0, r1);
}

// D9: ⌘/Ctrl+C over the current selection.
function onCopy(): void {
  const sel = currentSelection;
  if (!sel || !page) return;
  void copyOrReportError(selectionText(sel, 'tsv', copySource()), onCopyError, () =>
    announceApp(t('studio.copy.selection.announce')),
  );
}

// Right-click acts on what is shown selected: the current selection when it covers the hit, else
// the hit itself, which becomes the selection first (the data grid's rule).
function menuTarget(hitCell: number, pageRow: number): Selection {
  const sel = currentSelection;
  const idx: DisplayRowIndex = {
    displayRows: matchedRows(props.tabId),
    pageRowCount: page?.rowCount ?? 0,
  };
  const pos = displayPositionOf(idx, pageRow);
  if (hitCell === 0) {
    if (sel?.kind === 'row' && sel.rows.includes(pageRow)) return sel;
    grid?.setActiveCell(pos, 0, false, false, false);
    return { kind: 'row', rows: [pageRow] };
  }
  const displayCol = hitCell - 1;
  if (sel && selectionCovers(sel, pageRow, displayCol)) return sel;
  if (sel?.kind === 'cell' && sel.row === pageRow && sel.col === displayCol) return sel;
  grid?.setActiveCell(pos, hitCell, false, false, false);
  return { kind: 'cell', row: pageRow, col: displayCol };
}

// D9: right-click opens the menu that matches the selection it acts on (see menuTarget).
function onGridContextMenu(e: SlickEventData): void {
  if (!grid || !dataSource || !page) return;
  e.preventDefault();
  const hit = grid.getCellFromEvent(e);
  if (!hit) return;
  const nativeLike = e as unknown as MouseEvent;
  const pageRow = dataSource.getItem(hit.row).row;
  const field = hit.cell === 0 ? null : fieldAtCol(hit.cell);
  const pageCol = field ? colIndexFromField(field) : -1;
  if (hit.cell !== 0 && pageCol < 0) return;
  const target = menuTarget(hit.cell, pageRow);
  const src = copySource();
  const ctx = {
    onError: onCopyError,
    copy: (format: CopyFormat) => selectionText(target, format, src),
  };
  const columnName = page.columns[pageCol]?.name ?? '';
  switch (target.kind) {
    case 'row':
      contextMenuStore.openContextMenu(nativeLike, tabularRowMenu(ctx));
      break;
    case 'range':
      contextMenuStore.openContextMenu(nativeLike, tabularRangeMenu(ctx));
      break;
    case 'column':
      contextMenuStore.openContextMenu(nativeLike, tabularColumnMenu({ ...ctx, columnName }));
      break;
    case 'cell':
      contextMenuStore.openContextMenu(nativeLike, tabularCellMenu({ ...ctx, columnName }));
      break;
  }
}

// A header right-click never changes the selection (it only opens the column menu), so it copies
// the clicked column, not the current selection.
function onGridHeaderContextMenu(e: SlickEventData, args: { column: KiraColumn }): void {
  if (!grid || !page) return;
  if (args.column.id === GUTTER_FIELD) return;
  e.preventDefault();
  const displayCol = colIndexFromField(String(args.column.field));
  if (displayCol < 0) return;
  const column = page.columns[displayCol];
  if (!column) return;
  const src = copySource();
  const target: Selection = { kind: 'column', cols: [displayCol] };
  contextMenuStore.openContextMenu(
    e as unknown as MouseEvent,
    tabularColumnMenu({
      onError: onCopyError,
      columnName: column.name,
      copy: (format) => selectionText(target, format, src),
    }),
  );
}

// D1: local, DOM-focus-scoped copy — never a native Electron accelerator. F6 — `handleKeyDown`
// triggers `onKeyDown` first and honours `stopImmediatePropagation`, checked right after this
// fires — the same contract SlickGridHost.vue's own onKeydown relies on.
function onKeydown(e: SlickEventData): void {
  if (shortcutFor(e.getNativeEvent<KeyboardEvent>(), ['grid.copy']) !== null) {
    e.preventDefault();
    e.stopImmediatePropagation();
    onCopy();
  }
}

// P40 D10/D17: the same "hide non-matching rows" toggle grid/documents/keyvalue share (P24 D2) —
// `matchedRows(tabId)` is the filter *and* the data source (C12/§5 D12's own precedent in
// SlickGridHost.vue): a filtered row keeps its real page-row number in the gutter.
function dataSourceState(): GridDataSourceState {
  return {
    index: { displayRows: matchedRows(props.tabId), pageRowCount: page?.rowCount ?? 0 },
    inserts: [],
    extractValue: (item, field) => cell(props.pageKey, item.row, colIndexFromField(field)),
  };
}

// "the highlight is two keyed setCellCssStyles layers" — every match, and (if any) the
// current one, via views/shared/slick/cssLayers.ts's own searchCellLayers so this isn't
// written a second time from SlickGridHost.vue's own computeSearchHashes. Not clipped to the
// rendered band (unlike the one-cell selection layer's neighbour — SlickGridHost.vue's own
// `kira-search` is deliberately unclipped too): a search result is not bounded the
// way a rendered band is. `isVisibleColumn` is always `true` — a console result never reorders or
// hides a column, unlike the data grid's own column-menu-driven set.
function refreshSearchLayer(): void {
  if (!grid || !page) return;
  const entry = searchState[props.tabId];
  const idx: DisplayRowIndex = {
    displayRows: matchedRows(props.tabId),
    pageRowCount: page.rowCount,
  };
  const [matchHash, currentHash] = searchCellLayers(
    entry?.matches ?? [],
    entry?.index ?? -1,
    (col) => (page && col >= 0 && col < page.columns.length ? colField(col) : undefined),
    () => true,
    (row) => displayPositionOf(idx, row),
    classesFrom({ searchMatch: true })[0] ?? 'search-match',
    classesFrom({ searchMatchCurrent: true })[0] ?? 'search-match-current',
  );
  grid.setCellCssStyles('kira-search', matchHash);
  grid.setCellCssStyles('kira-search-current', currentHash);
}

// The find toolbar's go-to-match — ConsoleResultGrid.vue delegates here for the tabular
// branch. `rowIndices`' own position lookup (the incumbent VirtualList's approach) has no
// counterpart here: SlickGrid addresses a row by display *position*, so this goes straight through
// the same displayPositionOf arithmetic the search layer above uses.
function goToMatch(match: Match): void {
  if (!grid || !page) return;
  const idx: DisplayRowIndex = {
    displayRows: matchedRows(props.tabId),
    pageRowCount: page.rowCount,
  };
  const pos = displayPositionOf(idx, match.row);
  grid.scrollRowIntoView(pos);
  grid.setActiveCell(pos, match.col + 1); // +1: the frozen gutter occupies display column 0
}
defineExpose({ goToMatch });

onMounted(() => {
  const el = rootRef.value;
  if (!el) return;

  const p = getPage(props.pageKey);
  // The caller only mounts this component for a tabular result (ConsoleResultGrid.vue's own
  // `page.kind === 'tabular'` gate) — this narrows the type, it never fires in practice.
  if (p?.kind !== 'tabular') return;
  page = p;

  dataSource = createGridDataSource(dataSourceState());
  const saved = props.viewState;
  if (saved && Object.keys(saved.columnWidths).length > 0) {
    consoleViewStore.setConsoleColumnWidths(props.tabId, { ...saved.columnWidths });
  }

  el.style.setProperty('--kira-header-row-height', `${rowHeight.value}px`);

  grid = new KiraSlickGrid(el, dataSource as CustomDataView<RowHandle>, buildColumns(p), {
    rowHeight: rowHeight.value,
    // F7 — Sortable.js is never loaded; this app never drags a header to reorder.
    enableColumnReorder: false,
    // F6 — cell text is untrusted database content.
    enableHtmlRendering: false,
    // F1 — this app measures column widths itself (columns.ts's own canvas-based initialWidths).
    autosizeColsMode: 'LegacyOff',
    // §5 item 5 (SlickGridHost.vue) — the sticky row-number gutter: a frozen column whose canvas
    // KiraSlickGrid re-homes into the scroll viewport as one sticky column (P182), F4's own fix for
    // the incumbent VirtualList's row-number-scrolls-away defect.
    frozenColumn: 0,
    // F3 addendum — the viewport's native `overflow:auto` plus its native `scroll` listener are
    // sufficient on their own; SlickGrid's own wheel handler would discard native momentum.
    enableMouseWheelScrollHandler: false,
    // §3.4 — the one *addition* over the incumbent: keyboard cell navigation, free once a data
    // column's own default `focusable`/`selectable` (both true) let it hold the active cell.
    enableCellNavigation: true,
    enableAddRow: false,
    // §3.4: read-only, no editor, no inline edit trigger.
    editable: false,
    autoEdit: false,
    autoCommitEdit: true,
    asyncEditorLoading: false,
    editorCellNavOnLRKeys: false,
    explicitInitialization: false,
    // P19 D8: SlickGrid's own selection layer, straight off selectionModel's ranges — the same
    // swap SlickGridHost.vue's own grid options make.
    selectedCellCssClass: 'kira-cell-selected',
    multiSelect: true,
    dataItemColumnValueExtractor: (item: RowHandle, columnDef: KiraColumn) =>
      dataSource?.extractValue(item, String(columnDef.field)),
  });
  attachScrollProbes(grid, scrollVelocityTracker, () => scrollEventSeq);

  // P19 D8: identical configuration to SlickGridHost.vue's own — the row's own words are "rows,
  // columns, or an arbitrary free-form cell range", exactly the four-kind Selection model the data
  // grid already has.
  selectionModel = createHybridSelectionModel(GUTTER_FIELD);
  grid.setSelectionModel(selectionModel);

  eventHandler = new SlickEventHandler();
  eventHandler.subscribe(grid.onRendered, tagRenderedRows);
  eventHandler.subscribe(grid.onRendered, onGridRendered);
  eventHandler.subscribe(grid.onClick, onGridClick);
  eventHandler.subscribe(grid.onHeaderClick, onGridHeaderClick);
  eventHandler.subscribe(grid.onContextMenu, onGridContextMenu);
  eventHandler.subscribe(grid.onHeaderContextMenu, onGridHeaderContextMenu);
  eventHandler.subscribe(grid.onColumnsResized, onColumnsResized);
  eventHandler.subscribe(grid.onKeyDown, onKeydown);
  eventHandler.subscribe(selectionModel.onSelectedRangesChanged, onSelectedRangesChanged);
  subscribeRangeSelecting(eventHandler, selectionModel, onCellRangeSelecting);

  // Not useEventListener/useResizeObserver — same declined reasoning as
  // SlickGridHost.vue's own identical pair: registration order against SlickGrid's internal scroll
  // listener is load-bearing for the velocity sampler this mirrors, and onUnmounted below tears
  // these down in a specific hand-ordered position relative to eventHandler/grid.destroy().
  viewportEl = grid.getViewports()[1] ?? grid.getViewports()[0] ?? null;
  if (viewportEl) {
    scrollVelocityTracker.seed(viewportEl.scrollTop, performance.now());
    viewportEl.addEventListener('scroll', onViewportScroll, { passive: true });
  }
  grid.render();
  // C12/§5 D12 precedent (SlickGridHost.vue) — a tab switch (not a fresh run) can remount this
  // component onto a result that already has a completed search from before it was last hidden.
  refreshSearchLayer();

  resizeObserver = new ResizeObserver(() => grid?.resizeCanvas());
  resizeObserver.observe(el);
  restoreViewState(saved);
});

// The selection must fit the page: a stale index would paint nothing or throw in SlickGrid.
function selectionFits(sel: Selection, rowCount: number, colCount: number): boolean {
  switch (sel.kind) {
    case 'cell':
      return sel.row < rowCount && sel.col < colCount;
    case 'range':
      return (
        Math.max(sel.row, sel.anchorRow) < rowCount && Math.max(sel.col, sel.anchorCol) < colCount
      );
    case 'row':
      return sel.rows.every((r) => r < rowCount);
    default:
      return sel.cols.every((c) => c < colCount);
  }
}

// Applies the saved selection, then the scroll offsets, and reports `ready` once SlickGrid has
// mounted the rows at that offset: the strictly visible rows always render in the one synchronous
// pass below, and the two frames let a queued chase settle before a screenshot reads the DOM.
function restoreViewState(saved: ResultViewState | null | undefined): void {
  if (saved && grid && viewportEl && selectionModel && page) {
    restoring = true;
    try {
      const colCount = grid.getColumns().length - 1;
      const sel = saved.selection;
      if (sel && selectionFits(sel, page.rowCount, colCount)) {
        if (sel.kind === 'column') pendingSelectionKind = 'column';
        selectionModel.setSelectedRanges(rangesFromSelection(sel, page.rowCount, colCount));
        currentSelection = sel;
      }
      viewportEl.scrollTop = saved.resultScrollTop;
      viewportEl.scrollLeft = saved.resultScrollLeft;
      grid.syncToViewportScroll();
    } finally {
      restoring = false;
    }
  }
  requestAnimationFrame(() => requestAnimationFrame(() => emit('ready')));
}

onUnmounted(() => {
  resizeObserver?.disconnect();
  resizeObserver = null;
  if (viewportEl) viewportEl.removeEventListener('scroll', onViewportScroll);
  eventHandler?.unsubscribeAll();
  eventHandler = null;
  // C4 (SlickGridHost.vue) — grid.destroy() never calls selectionModel.destroy() itself.
  selectionModel?.destroy();
  selectionModel = null;
  currentSelection = null;
  // `true` also nulls SlickGrid's own internal element references and unbinds every listener the
  // library itself registered; the app forgetting this call was the whole risk (KiraSlickGrid's own
  // `destroy` override additionally fixes the library's capture-flag `bindAncestorScrollEvents`
  // leak — inherited here for free).
  grid?.destroy(true);
  grid = null;
  dataSource = null;
  viewportEl = null;
  page = null;
});

subscribeRowHeight(rowHeight, () => grid, rootRef);

// P31 D11/F13 — a font change leaves every unstored column sized for whatever font was active
// when columns.ts's shared measuring context was first created, for the rest of the session.
watch(
  () => settingsStore.appearanceVersion.n,
  () => {
    if (!grid || !page) return;
    resetMeasureCtx();
    grid.setColumns(buildColumns(page));
    grid.render();
  },
);

// C12/§5 D12 precedent — the filter *is* the data source: `matchedRows(tabId)` tracks both the
// search toolbar's own scan result and its "filter to matches" toggle reactively.
watch(
  () => matchedRows(props.tabId),
  () => {
    if (!grid || !dataSource) return;
    dataSource.setState(dataSourceState());
    grid.updateRowCount();
    grid.invalidateAllRows();
    grid.render();
    refreshSearchLayer();
    // A selection identifies page rows via a display-position range built under
    // the OLD filter; under the NEW one that same range can silently point at a different row.
    // Unlike SlickGridHost.vue's own refreshSelectionForFilterChange (which remaps), this
    // clears — the same reasoning ConsoleResultGrid.vue's own one-click highlight already uses on
    // a page swap ("a row index into a page that has been replaced identifies nothing").
    selectionModel?.setSelectedRanges([]);
  },
);

// C12/§5 D5 precedent — `kira-search`'s own trigger: a scan publishing a new/updated result, or
// goNext/goPrev moving the current match. The signature is result identity + current index +
// pending, not the matches themselves — rebuilding the whole hash is refreshSearchLayer's own job.
watch(
  () => {
    const entry = searchState[props.tabId];
    return entry ? `${entry.matches.length}:${entry.index}:${entry.pending ? 1 : 0}` : '';
  },
  () => refreshSearchLayer(),
);
</script>

<template>
  <!-- Type-based cell colour is new for the console — the same class toggle SlickGridHost.vue's
       own root carries, extending the rowColoring setting to console results (this migration's
       own recommendation). `.slick-grid-host` is what scopes every rule in slickTheme.css —
       imported globally by this file, not relied on from SlickGridHost.vue, so a console panel
       never depends on a data tab having been opened first. -->
  <div
    class="slick-grid-host relative h-full font-data text-kira-md text-fg"
    :class="{ 'kira-grid--row-coloring': settingsStore.appearance.rowColoring }"
  >
    <div ref="rootRef" class="slick-grid-mount h-full w-full"></div>
  </div>
</template>
