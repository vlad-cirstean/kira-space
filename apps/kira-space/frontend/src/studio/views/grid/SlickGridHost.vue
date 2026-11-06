<script setup lang="ts">
import { foldRulesByColumn, type MaskingRule } from '@shared/domain/mask';
import type { ColumnMeta, ForeignKeyMeta, ObjectMeta } from '@shared/domain/tree';
import { decodePath } from '@shared/domain/tree';
import type { ColumnDescriptor } from '@shared/protocol/page';
import { useQuery } from '@tanstack/vue-query';
import AttributeTooltip from '@theme/components/AttributeTooltip.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import { buttonVariants } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { wrapSelectionOnType } from '@theme/wrapSelection';
import { useDebounceFn } from '@vueuse/core';
import { copyAndAnnounce, readClipboardText } from '@workbench/behaviour/clipboard';
import { TIMING } from '@workbench/behaviour/timing';
import { errorText } from '@workbench/bridge/codedError';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { shortcutFor } from '@workbench/shortcuts/keys';
import { type MenuItem, runMenuShortcut, useContextMenuStore } from '@workbench/state/contextMenu';
import type {
  Column,
  CustomDataView,
  FormatterResultWithText,
  ItemMetadata,
  OnBeforeEditCellEventArgs,
  OnBeforeHeaderCellDestroyEventArgs,
  OnClickEventArgs,
  OnHeaderCellRenderedEventArgs,
  OnHeaderClickEventArgs,
  SlickEventData,
} from 'slickgrid';
import { SlickEventHandler, type SlickHybridSelectionModel, SlickRange } from 'slickgrid';
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { type SelectedCell, useCellSelectionStore } from '../../state/cellSelection';
import { useConnectionsStore } from '../../state/connections';
import {
  correlationKeyFor,
  loadMaskRules,
  maskRulesFor,
  maskRulesQueryKey,
} from '../../state/maskRules';
import { useSettingsStore } from '../../state/settings';
import { useTabsStore } from '../../state/tabs';
import { classesFrom } from '../../theme/cellClass';
import { categoryForTypeClass } from '../../theme/icons';
import { parseDelimited, type RowSnapshot } from '../shared/clipboardFormats';
import { nullCellText, truncatedCellTip } from '../shared/page/cellText';
import {
  alignmentFor,
  columnHeaderTooltip,
  DEFAULT_COLUMN_WIDTH,
  headerAwareMinWidth,
  initialWidths,
  pageColumnIndexFor,
  resetMeasureCtx,
  resolveColumnOrder,
  tooltipAttrs,
} from '../shared/page/columns';
import SearchFilterNotice from '../shared/page/SearchFilterNotice.vue';
import { type EdgeHash, searchCellLayers } from '../shared/slick/cssLayers';
import {
  computeCellFillHash,
  gutterColumn,
  renderedPageRowBand,
  subscribeRangeSelecting,
  subscribeRowHeight,
} from '../shared/slick/gridHostShared';
import { KiraSlickGrid } from '../shared/slick/kiraSlickGrid';
import { createScrollVelocityTracker } from '../shared/slick/scrollVelocity';
import { attachScrollProbes, createHybridSelectionModel } from '../shared/slick/selectionModel';
import { sqlDialectFor } from '../shared/sqlIdent';
import FkPreviewPopover from './FkPreviewPopover.vue';
import {
  type CellFocusRequest,
  consumeCellFocus,
  registerGridHost,
  requestCellFocus,
  unregisterGridHost,
} from './focusRequest';
import { buildMaskTagCache, createMaskPreviewTransform } from './maskPreview';
import { cellMenu, type FkNavContext, headerMenu, rowMenu } from './menu';
import { registerOpenEditFlush, unregisterOpenEditFlush } from './openEdit';
import { applyPastedCells, resolvePasteTarget } from './paste';
import { usePendingChangesStore } from './pendingChanges';
import { matchedRows, searchState } from './search';
import {
  createDisplayValueExtractor,
  createGridDataSource,
  displayPositionOf,
  type GridDataSourceState,
  isRowVisible,
  pendingRowClasses,
  type RowHandle,
  rowAtDisplayPosition,
} from './slick/dataSource';
import { editorCtx, KiraCellEditor } from './slick/editor';
import {
  type CellNavEntry,
  type DisplayCellView,
  type NavColumns,
  navColumnsFor,
  navValuesFor,
  numericNavColumns,
  rowsForSelection,
  cellNavEntry as rvCellNavEntry,
  columnValuesFor as rvColumnValuesFor,
  displayCell as rvDisplayCell,
  rowSnapshot as rvRowSnapshot,
  rowsForColumnOps as rvRowsForColumnOps,
  visibleRowsInSpan,
} from './slick/rowValues';
import '../shared/slick/slickTheme.css';
import 'slickgrid/dist/styles/css/slick.grid.css';
import { setVisibleRows } from '../shared/page/visibleRows';
import * as scrollTrace from '../shared/slick/scrollTrace';
import {
  rangesFromSelection,
  selectionCovers,
  selectionFromRanges,
} from '../shared/slick/selection';
import { type CopySource, selectionText } from '../shared/slick/selectionCopy';
import { getPage, pageVersion, setVisibleWindow } from './page';
import { parseTextSortTerms } from './sortTerms';
import { type Selection, useGridViewStore } from './state';

const contextMenuStore = useContextMenuStore();
const cellSelectionStore = useCellSelectionStore();
const pendingChangesStore = usePendingChangesStore();
const connectionsStore = useConnectionsStore();
const tabsStore = useTabsStore();
const settingsStore = useSettingsStore();
const gridViewStore = useGridViewStore();

// A from-scratch Vue host for SlickGrid, on editor/CodeMirrorHost.vue's own
// established shape for wrapping an imperative library: one ref root div, the instance held in a
// plain `let` (never a ref/shallowRef/reactive — see that file's own comment for why: Vue must not
// see the grid, its rowsCache or its DOM, or every internal object SlickGrid touches on every
// render gets proxied), constructed in onMounted, destroyed in onUnmounted.
const props = defineProps<{ tabId: string }>();

// See kiraSlickGrid.ts's own comment: Column<T>'s `field` type is a recursive
// PathsToStringProps<T>, which RowHandle's own shape can't satisfy for the app's arbitrary db
// column names — the column generic is deliberately widened to `any` here too, matching the
// grid class's own escape hatch.
// biome-ignore lint/suspicious/noExplicitAny: see comment above.
type KiraColumn = Column<any>;

function tab() {
  return tabsStore.findDataTab(props.tabId);
}

const { single: rowHeight } = useRowHeight();

// Same split DataGrid.vue's own displayRows/displayRowCount use — kept as a plain, non-reactive
// read at mount and on an explicit page reload (below), not a live `watch` on the search state:
// the scope is decode/bridge/gutter/colour/theme/runway, not live search-filter wiring. The
// un-filtered common case (the vast majority of a session) is unaffected either way.
function currentDisplayRows(): number[] | null {
  return matchedRows(props.tabId);
}

const GUTTER_FIELD = '__kira_gutter';

// Read fresh by the gutter formatter on every cell it builds — a plain object the host reassigns,
// never a Vue ref (§6 D1's own rule: the formatter runs *during* SlickGrid's own render). Mirrors
// dataSource.ts's own GridDataSourceState in spirit: one mutable snapshot, swapped wholesale.
const formatterCtx = { rowNumberBase: 0 };

function gutterFormatter(
  _row: number,
  _cell: number,
  _value: unknown,
  _columnDef: KiraColumn,
  dataContext: RowHandle,
): string {
  if (dataContext.insertId !== undefined) return '+';
  return String(formatterCtx.rowNumberBase + dataContext.row + 1);
}

// Structure is per-cell: a plain string for the common case (-> textContent).
// NULL and truncated no longer build a DocumentFragment — SlickGrid's own
// FormatterResultWithText.addClasses folds straight into the cell's own className
// (appendCellHtml, dist/esm/index.js:9962), so `.cell-null`/`.cell-truncated` land on the
// `.slick-cell` itself, and `.toolTip` becomes a `title` on the cell (:9969) — the truncation
// tooltip is kept, not lost. slickTheme.css's own `.cell-truncated::after` restores the muted
// ellipsis marker as a pseudo-element (zero DOM nodes) in place of the old child <span>. `value`
// is exactly what the grid's own dataItemColumnValueExtractor (below) returned for this cell — the
// CellView the app's existing decode/cache/staged-edit pipeline produced, never re-derived here.
function cellFormatter(
  _row: number,
  _cell: number,
  value: unknown,
  columnDef: KiraColumn,
  dataContext: RowHandle,
): string | FormatterResultWithText | HTMLElement {
  const view = value as { text: string; isNull: boolean; truncated: boolean; masked?: boolean };
  // The one place a formatter returns DOM, against `-iter2-pacing` the measured
  // "text, never DOM" rule: bounded to the insert region alone (typically 1-5 rows), and the
  // normal path just above/below is untouched. Self-contained once built: every keystroke stages
  // straight into pendingChanges via the grid root's own delegated `input` listener (onMounted,
  // below), never back through this formatter, so a value survives even if this cell is later
  // invalidated and this branch rebuilds a fresh element from the same staged state.
  if (dataContext.insertId !== undefined) {
    const input = document.createElement('input');
    input.className = 'cell-input';
    input.dataset.testid = 'grid-cell-insert-input';
    input.value = view.isNull ? '' : view.text;
    if (pendingChangesStore.isCommitting(props.tabId) || gridViewStore.isStale(props.tabId)) {
      input.readOnly = true;
    }
    // The server computes generated columns; the insert paths already skip them (P36 D28).
    if (isGeneratedColumn(String(columnDef.field))) {
      input.readOnly = true;
      input.classList.add('cell-generated');
    }
    return input;
  }
  // The *cheap* precheck only (a Set lookup, not the full `cellNavEntry` a click on
  // this column's own nav button computes lazily — navEntryAt, below): whether this column is
  // fk-or-pk and this cell isn't NULL. `placeNavButtonsForRenderedCells` rides on `.has-nav`
  // (below) to know which rendered cells get a button at all;
  // `.fk` no longer carries a colour of its own, only which icon glyph the button gets.
  const name = String(columnDef.field);
  const isFk = !view.isNull && navColumns.fk.has(name);
  const hasNav = isFk || (!view.isNull && navColumns.pk.has(name));
  const navClasses = hasNav ? (isFk ? 'fk has-nav' : 'has-nav') : '';
  if (view.isNull) return { text: nullCellText(), addClasses: 'cell-null' };
  // The one visual affordance keeping a user from misreading the preview as real data —
  // `.cell-masked` (slickTheme.css) styles the `#TAG` suffix muted. `view.masked` is only ever
  // true from maskPreview.ts's own transform, never from the plain decode path, so this is a no-op
  // (`addClasses` stays exactly what it was) whenever the preview is off.
  const maskedClass = view.masked ? 'cell-masked' : '';
  const classes = [navClasses, maskedClass].filter(Boolean).join(' ');
  if (view.truncated) {
    return {
      text: view.text,
      addClasses: classes ? `cell-truncated ${classes}` : 'cell-truncated',
      toolTip: truncatedCellTip(),
    };
  }
  return classes ? { text: view.text, addClasses: classes } : view.text;
}

function rt() {
  return gridViewStore.runtime[props.tabId];
}

// The row-shortcut/copy/paste plumbing needs the same writability/identity predicates
// DataGrid.vue's own computeds provide; plain functions here (not `computed`s) since nothing below
// reads them from inside the render path and every call site is already an event handler.
function hasPrimaryKey(): boolean {
  return pendingChangesStore.pageHasFullPrimaryKey(props.tabId);
}
function isWritable(): boolean {
  const t = tab();
  if (!t?.connectionId) return false;
  return (
    !connectionsStore.connectionRecord(t.connectionId)?.readOnly &&
    !pendingChangesStore.isCommitting(props.tabId) &&
    !gridViewStore.isStale(props.tabId)
  );
}
function caps() {
  const connectionId = tab()?.connectionId;
  return connectionId ? (connectionsStore.states[connectionId]?.caps ?? null) : null;
}
// M5 §6.4: this tab's own masking preview toggle. A plain function, not a computed — the same
// "every other call site is an event handler outside SlickGrid's own render path" reasoning
// hasPrimaryKey/isWritable already give, just above (D0).
function maskPreviewOn(): boolean {
  return !!rt()?.maskPreview;
}

// Gates whether double-click/Enter starts an inline edit — the toolbar's own add/preview/
// commit/discard buttons are gated on writability alone, never on hasPrimaryKey.
//
// `&& !maskPreviewOn()` is the whole edit lockout — the worst bug this feature could ship
// is a user committing a masked value into their database, and every write path already routes
// through this one predicate: the `editable` grid option (the `canEditTableReactive` watch,
// below), `SelectedCell.onEdit`/`onRevert` (the publication watch further down, which reads
// `canEditTable()` directly), `cellFormatter`'s insert-row `<input>` branch (no insert row can
// exist while masked, since it is add-row's own isWritable in DataToolbar.vue that gets one too),
// and `onPaste` (`if (!canEditTable()) return;`, above). A stale page (connection lost, or read
// under another session) locks the same paths through isWritable(); staged edits survive.
function canEditTable(): boolean {
  return isWritable() && hasPrimaryKey() && !!caps()?.canUpdate && !maskPreviewOn();
}
// Deliberately not folded into canEditTable — an engine could offer one of
// canUpdate/canDelete without the other.
//
// `&& !maskPreviewOn()` added defensively — a row delete is still a write path against rows the
// user cannot currently read clearly, and the same lockout principle applies: never let
// masked-preview state reach a real mutation.
function canDeleteRows(): boolean {
  return isWritable() && hasPrimaryKey() && !!caps()?.canDelete && !maskPreviewOn();
}

// A `computed` purely to trigger the `editable`-sync watch below via `grid.setOptions`
// (rowHeight's own computed+watch pair, just below, is the identical precedent); canEditTable()
// itself deliberately stays a plain function, not a computed, since every other call site is an
// event handler outside SlickGrid's synchronous render path.
// `pageVersion.n` is read explicitly (not just through hasPrimaryKey()'s own getPage() call)
// because `page/store.ts`'s `pages` map is a plain `Map`, not `reactive()` — `pageVersion.n` is
// that store's own dedicated reactive signal for "a page was set/dropped", the same one the
// `pageVersion` watch below already keys its own page-dependent rebuild off of; without this read
// here, a tab whose grid mounts before its first page arrives (the common case — connect, then
// load) would stay permanently `editable: false`, since nothing would ever re-run this computed.
const canEditTableReactive = computed(() => {
  void pageVersion.n;
  return canEditTable();
});

// The two empty states, overlaid as absolutely-positioned siblings over the always-
// mounted grid root (template, below) rather than swapping it out: this host's own root div IS
// what SlickGrid holds references to, and `v-if`-ing it away would unmount the element mid-life.
// Reactive `computed`s are fine here, unlike `currentDisplayRows()`'s own deliberately-plain twin
// above — the template is outside SlickGrid's synchronous render path entirely, the same reasoning
// the row-coloring class binding already relies on.
// `pageVersion.n` is read explicitly for the identical reason `canEditTableReactive` reads it,
// just above: `getPage()` itself is not reactive (`page/store.ts`'s own plain `Map`).
const showNoRows = computed(() => {
  void pageVersion.n;
  const p = getPage(props.tabId);
  return !!p && p.rowCount === 0;
});
// Filtering to zero matches is a distinct empty state from "the table is empty" — but a pending
// insert row is work in progress, not a search result, so its presence keeps the grid itself on
// screen even with zero real matches (DataGrid.vue's own identical guard, ported verbatim).
const showNoMatchingRows = computed(() => {
  void pageVersion.n;
  const p = getPage(props.tabId);
  if (!p) return false;
  const rows = matchedRows(props.tabId);
  return (
    rows !== null &&
    rows.length === 0 &&
    (pendingChangesStore.pendingFor(props.tabId)?.inserts.length ?? 0) === 0
  );
});

// Produced locally from the path, never round-tripped to the engine for a string join — the same
// discipline grid/menu.ts's qualifiedNameForPath and project/menus.ts's qualifiedNameFor use.
const QUALIFIED_KINDS = new Set(['schema', 'table', 'view', 'matview']);
function qualifiedName(): string {
  const t = tab();
  if (!t?.connectionId) return '';
  return decodePath(t.connectionId, t.path)
    .segments.filter((s) => QUALIFIED_KINDS.has(s.kind))
    .map((s) => s.name)
    .join('.');
}

// The current display column order, read fresh (not a computed: nothing here may create a Vue
// reactive dependency this file's own imperative calls could re-enter). Every menu/clipboard/nav
// function below takes it as a plain parameter, matching rowValues.ts's own signatures (content,
// no SlickGrid API).
function currentOrder(): string[] {
  const p = getPage(props.tabId);
  return p ? resolveColumnOrder(p, tab()?.state.columnOrder ?? null) : [];
}

// Thin, tabId/page/order-bound wrappers over rowValues.ts's own pure functions — every call site
// in this file already has props.tabId and currentOrder() on hand, so binding them here once
// keeps those call sites reading exactly like DataGrid.vue's own did.
//
// The ONE place this file's own `displayCell` masks — every call site (the
// SelectedCell publish watch below, so the cell editor panel shows the masked text; the context
// menus' own `text`/copy paths, so "copy follows display") reads through here, so a
// future call site inherits the masking rule for free rather than needing its own opt-in. A
// staged edit (`view.staged`) is never masked — moot in practice (the toggle is disabled while
// `hasPending`, so nothing can be staged while masked), kept as an explicit early return rather
// than relying on that invariant holding here too.
function displayCell(row: number, displayCol: number): DisplayCellView {
  const view = rvDisplayCell(props.tabId, getPage(props.tabId), currentOrder(), row, displayCol);
  if (view.staged || !maskPreviewOn() || maskRulesByColumn.size === 0) return view;
  const field = currentOrder()[displayCol];
  if (!field) return view;
  const masked = maskTransform(view, field);
  return { ...view, text: masked.text, isNull: masked.isNull, truncated: masked.truncated };
}
// "copy follows display" — row copy must not paste real values while the preview is showing masked
// ones. Post-processes rvRowSnapshot's own values through the same transform `displayCell` above
// uses. (Duplicate row is a write path, not a read one — `duplicateAsInsert`, menu.ts's own `run`,
// reads `cell()` directly rather than through this wrapper at all, but it never runs while masked
// regardless: menu.ts's own `disabled: !ctx.canEdit` already blocks it, since `canEditTable()`
// folds in `!maskPreviewOn()`.)
function rowSnapshot(row: number): RowSnapshot {
  const snap = rvRowSnapshot(props.tabId, getPage(props.tabId), currentOrder(), row);
  if (!maskPreviewOn() || maskRulesByColumn.size === 0) return snap;
  const values: Record<string, string | null> = {};
  for (const name of snap.columns) {
    const raw = snap.values[name];
    if (raw === null) {
      values[name] = null;
      continue;
    }
    const masked = maskTransform(
      { text: raw, isNull: false, truncated: snap.truncated?.has(name) ?? false },
      name,
    );
    values[name] = masked.text;
  }
  return { ...snap, values };
}

function currentDialect() {
  return sqlDialectFor(connectionsStore.connectionRecord(tab()?.connectionId)?.kind);
}

function isDeleted(row: number): boolean {
  return pendingChangesStore.isPendingDelete(props.tabId, row);
}

// C7/§5 D7 — rowsForColumnOps/columnValuesFor bound to this file's own displayRows/tabId/page/
// order, mirroring the displayCell/rowSnapshot wrappers above.
function rowsForColumnOps(rowCount: number): number[] {
  return rvRowsForColumnOps(currentDisplayRows(), rowCount);
}
// M5 §6.6: "copy follows display", the copy-column-values path — mirrors rowSnapshot's own
// post-processing above, for the identical reason (rvColumnValuesFor reads through rowValues.ts's
// own unmasked `displayCell`, not this file's masking-aware wrapper).
function columnValuesFor(displayCol: number): string[] {
  const values = rvColumnValuesFor(
    props.tabId,
    getPage(props.tabId),
    currentOrder(),
    currentDisplayRows(),
    displayCol,
  );
  if (!maskPreviewOn() || maskRulesByColumn.size === 0) return values;
  const field = currentOrder()[displayCol];
  if (!field) return values;
  const rule = maskRulesByColumn.get(field.toLowerCase());
  if (!rule) return values;
  // rvColumnValuesFor already collapsed NULL to '' (its own contract, matching every other
  // column-values consumer) — masking that empty string is a no-op regardless (Apply's own
  // "empty stays empty" universal rule), so no NULL-awareness is lost by transforming it anyway.
  return values.map((text) => maskTransform({ text, isNull: false, truncated: false }, field).text);
}

// FIX-8: PK/FK stated as a label, never inferred from colour alone — mirrors DataGrid.vue's own
// foreignKeyColumnNames/keyLabelFor, folded to a Set built once per buildColumns call (not once
// per column) since it's the same answer for every column of one call.
function foreignKeyNamesFor(meta: ObjectMeta | null): Set<string> {
  const names = new Set<string>();
  for (const fk of meta?.foreignKeys ?? []) for (const c of fk.columns) names.add(c);
  return names;
}
function keyLabelFor(
  descriptor: ColumnDescriptor | undefined,
  name: string,
  foreignKeyNames: Set<string>,
): 'PK' | 'FK' | null {
  if (descriptor?.isPrimaryKey) return 'PK';
  if (foreignKeyNames.has(name)) return 'FK';
  return null;
}

// headerCellAttrs is a plain static attribute bag, so tooltipAttrs (views/shared/page/columns.ts)
// writes the same shape AttributeTooltip.vue's own bridge (packages/theme/src/components/) reads by
// hand: data-kira-tip (the plain, newline-joined a11y text) and data-kira-tip-parts (the
// structured JSON a real TooltipContent would render) plus aria-label — since no Vue component
// tree mounts over SlickGrid-owned DOM.

// The DESCRIBE-derived dataType (meta.columns) when it has loaded, else the page's own
// ColumnDescriptor — mirrors DataGrid.vue's own dataTypeFor, so the header tooltip can never show
// a column two different type strings depending on which one asked first.
function dataTypeFor(
  name: string,
  descriptor: ColumnDescriptor | undefined,
  metaByName: Map<string, ColumnMeta>,
): string {
  return metaByName.get(name)?.dataType ?? descriptor?.dataType ?? '';
}

/** Colour and alignment are per-column and static: a `tc-<category>` class plus
 *  `kira-align-right` where the column's own descriptor says numeric. Nine categories collapse to
 *  the five `categoryForTypeClass` can return; slickTheme.css carries all five anyway
 *  (that file's own comment says why). */
function buildColumns(
  page: ReturnType<typeof getPage>,
  order: string[],
  storedWidths: Record<string, number>,
  settledWidths: Record<string, number>,
  meta: ObjectMeta | null,
): KiraColumn[] {
  // D2 — changed from Pass A's false/false: F1's row-select-on-gutter (§5 D4, C4) requires
  // canCellBeActive(row, 0), which `handleClick`'s row branch checks
  // (slick.hybridselectionmodel.ts:497). Tab/Left-arrow landing on the gutter is the one side
  // effect (D8 vetoes editing there; a gutter active cell becomes a row selection).
  // P22 Pass B, C1/§5 D10 — F3: cellAttrs/headerCellAttrs are per-column static attribute bags,
  // applied once per cell/header BUILD (alongside SlickGrid's own role/tabIndex/aria-describedby),
  // never per retained row — the whole `data-testid`/`data-*` surface this app's tests/ui/ suite
  // needs is free at this granularity; only the row's own `data-row` correction (below,
  // tagRenderedRows) needs an actual per-render pass, and rows are ~200, not ~2 400 cells.
  const cols: KiraColumn[] = [
    gutterColumn(GUTTER_FIELD, gutterFormatter, 'grid-gutter-cell', {
      'data-testid': 'grid-select-all',
      role: 'button',
      'aria-label': t('studio.grid.selectAll.aria'),
    }),
  ];
  if (!page) return cols;
  const byName = new Map(page.columns.map((c) => [c.name, c]));
  const metaByName = new Map(meta?.columns.map((c) => [c.name, c]) ?? []);
  const fkNames = foreignKeyNamesFor(meta);
  order.forEach((name, displayIndex) => {
    const descriptor = byName.get(name);
    const classes = [`tc-${descriptor ? categoryForTypeClass(descriptor.typeClass) : 'other'}`];
    if (descriptor && alignmentFor(descriptor) === 'right') classes.push('kira-align-right');
    const keyLabel = keyLabelFor(descriptor, name, fkNames);
    const m = metaByName.get(name);
    const tooltip = columnHeaderTooltip(
      descriptor ?? { name, typeClass: 'other' },
      dataTypeFor(name, descriptor, metaByName),
      {
        comment: m?.comment,
        key: keyLabel,
        nullable: m?.nullable ?? null,
        generated: descriptor?.generated ?? false,
      },
    );
    // A per-column floor that knows what the header renders — padding, the sort
    // indicator (every column here is sortable, below) and the PK/FK badge (keyLabelFor is the
    // same call onHeaderCellRendered makes for the badge itself, so a column widens for its badge
    // at the same moment the badge appears). Fed into both `width` and `minWidth` below: fixing
    // only the initial width would leave a drag able to crop the header again.
    const floor = headerAwareMinWidth(name, {
      padding: 16,
      sortControl: 16, // P28 D4 — was 22; see HeaderChrome.sortControl's own comment.
      keyBadge: keyLabel ? 20 : 0,
    });
    cols.push({
      id: name,
      field: name,
      name,
      // Clamped to `floor` even for a stored width (a width read back from a prior resize/session
      // was never re-clamped, only an interactive drag was, so a column persisted at a pre-floor
      // width — as narrow as one character for something as short as "id" — forever, across every
      // reload, until manually widened again).
      width: Math.max(floor, storedWidths[name] ?? settledWidths[name] ?? DEFAULT_COLUMN_WIDTH),
      // F9 — the app's own resize floor; onColumnsResized persists the drag (below). Header-aware
      // (D4) rather than the flat columns.ts MIN_WIDTH — the mismatch between a flat floor and a
      // header's real furniture was what let a manual drag crop the header text again.
      minWidth: floor,
      resizable: true,
      // F8 — creates the sort indicator divs; tristateMultiColumnSort/multiColumnSort (grid
      // options, below) make a click cycle asc -> desc -> none, this app's own header cycle.
      sortable: true,
      cssClass: classes.join(' '),
      formatter: cellFormatter,
      // D2/D8 — always present, gated by onBeforeEditCell's own veto, not by presence: a column
      // this table can't currently write to still needs the same editor class the moment
      // writability changes (a caps probe resolving after mount), never a rebuild to add one.
      editor: KiraCellEditor,
      cellAttrs: {
        'data-testid': 'grid-cell',
        'data-column': name,
        'data-col-index': String(displayIndex),
      },
      headerCellAttrs: {
        'data-testid': 'grid-header-cell',
        'data-column': name,
        'data-col-index': String(displayIndex),
        ...tooltipAttrs(tooltip),
      },
    });
  });
  return cols;
}

// C9/§5 D9 — dataSource.ts's own `GridDataSourceState.rowColumns` header comment explains the
// shape and the cost story; built once per column-order rebuild (matches
// `createDisplayValueExtractor`'s own `fieldToPageCol` — cheap, but no reason to redo it per row).
function insertRowColumns(order: readonly string[]): NonNullable<ItemMetadata['columns']> {
  const columns: NonNullable<ItemMetadata['columns']> = {};
  for (const name of order) columns[name] = { editor: null, focusable: false };
  return columns;
}

// This tab's own folded mask rules (by lowercased column name) and precomputed tag
// cache. Plain module-scope-shaped `let`s, not `ref`/`reactive` — the same rule everything else
// on this synchronous render path follows (`formatterCtx`, `editorCtx`): `dataSourceState` reads
// them synchronously, and the two refresh functions below (async, since resolving the correlation
// key and computing HMAC tags both are) are the only writers, always followed by a `setState` +
// `invalidateAllRows` + `render` to show the result — never awaited inline from
// `dataSourceState` itself, which must stay synchronous.
let maskRulesByColumn = new Map<string, MaskingRule>();
let maskTagCache = new Map<string, string>();
// Perf: one shared transform instance per (maskRulesByColumn, maskTagCache)
// pair — rebuilt only here and in refreshMaskTagCache below, whenever either changes —
// instead of every displayCell/rowSnapshot/columnValuesFor/dataSourceState call constructing (and
// memoizing nothing in) its own. displayCell alone runs once per visible cell on every render.
let maskTransform = createMaskPreviewTransform(maskRulesByColumn, maskTagCache);

// The sole reactive trigger for a refold — proactively enabled (not deferred to the
// first preview toggle) so `refreshMaskFolding()` has real data the instant the user turns preview
// on, and TanStack Query's own cache means a tab that's reactivating (remounting on
// tab-switch, not a genuinely new table) does not re-fetch rules already cached for this
// connection. `foldMaskRulesOnly` below still reads via the plain, non-reactive `maskRulesFor` —
// only the watch below needs this ref's reactivity, the render path never does.
const maskRulesQuery = useQuery(() => ({
  queryKey: maskRulesQueryKey(tab()?.connectionId ?? ''),
  queryFn: () => loadMaskRules(tab()?.connectionId ?? ''),
  enabled: !!tab()?.connectionId,
}));

// Re-resolves maskRulesByColumn from this tab's own connection — cheap (state/maskRules.ts's own
// TanStack Query cache, no IPC round trip when already loaded). Split out from refreshMaskFolding
// below so a caller about to rebuild maskTagCache too (refreshMaskTagCache always reassigns
// maskTransform itself) does not also construct one here, only to discard it unused a line later.
function foldMaskRulesOnly(): void {
  const connectionId = tab()?.connectionId;
  maskRulesByColumn = connectionId ? foldRulesByColumn(maskRulesFor(connectionId)) : new Map();
}

function refreshMaskFolding(): void {
  foldMaskRulesOnly();
  maskTransform = createMaskPreviewTransform(maskRulesByColumn, maskTagCache);
}

// Rebuilds the tag cache for the CURRENT page against the CURRENT folding — "precompute
// tags per page, not per render". Async (resolving the key and signing each distinct value both
// are); callers re-render once this resolves. A page with nothing to correlate (no key yet, or no
// correlating rule) clears the cache rather than leaving a prior page's tags behind.
let maskTagSeq = 0;
async function refreshMaskTagCache(): Promise<void> {
  const seq = ++maskTagSeq;
  const connectionId = tab()?.connectionId;
  const p = getPage(props.tabId);
  if (!connectionId || !p || maskRulesByColumn.size === 0) {
    maskTagCache = new Map();
    maskTransform = createMaskPreviewTransform(maskRulesByColumn, maskTagCache);
    return;
  }
  const key = await correlationKeyFor(connectionId);
  const cache = await buildMaskTagCache(p, maskRulesByColumn, key);
  // A later refresh or page swap owns the cache now; installing this one would show old-page tags.
  if (seq !== maskTagSeq || getPage(props.tabId) !== p) return;
  maskTagCache = cache;
  maskTransform = createMaskPreviewTransform(maskRulesByColumn, maskTagCache);
}

// refreshMaskTagCache's own correlationKeyFor call can reject (an IPC failure) —
// every call site below used to leave that unhandled, which "fails open": the preview stays on
// (toggle active, grid-writable-badge still "read-only") but the grid's own dataSource is never
// updated to apply the mask transform, since the render tail after the await never runs. Turn the
// preview back off on failure (setMaskPreview, not a direct write to rt.maskPreview — its own
// pending-changes guard is irrelevant here since this only ever turns preview OFF) and surface the
// failure the same way every other async grid action in this file does (setActionError), then still
// re-render so the toggle-off state reaches the grid rather than leaving the last, already-
// stale render in place.
function refreshMaskTagsAndRerender(): void {
  void refreshMaskTagCache().then(
    () => {
      if (!grid || !dataSource) return;
      dataSource.setState(dataSourceState(getPage(props.tabId), currentOrder()));
      grid.invalidateAllRows();
      grid.render();
    },
    (err: unknown) => {
      gridViewStore.setMaskPreview(props.tabId, false);
      gridViewStore.setActionError(props.tabId, errorText(err));
      if (!grid || !dataSource) return;
      dataSource.setState(dataSourceState(getPage(props.tabId), currentOrder()));
      grid.invalidateAllRows();
      grid.render();
    },
  );
}

// The `GridDataSourceState` builder itself, factored out once this stopped being the single
// mount-time-only object: the pageVersion watch already rebuilt it wholesale on every reload, and
// now an insert-count change (below) needs the identical shape for a narrower reason. `p`/`order`
// are passed in rather than re-derived, since every call site has already resolved them for its
// own other purposes (formatterCtx, buildColumns, ...).
function dataSourceState(p: ReturnType<typeof getPage>, order: string[]): GridDataSourceState {
  const inserts = pendingChangesStore.pendingFor(props.tabId)?.inserts ?? [];
  const insertColumns = inserts.length > 0 ? insertRowColumns(order) : undefined;
  // M5 §6.2: `undefined` when mask preview is off (or this tab has no masked columns at all) —
  // the extractor is then byte-for-byte what it was before M5.
  const activeMaskTransform =
    maskPreviewOn() && maskRulesByColumn.size > 0 ? maskTransform : undefined;
  return {
    index: { displayRows: currentDisplayRows(), pageRowCount: p?.rowCount ?? 0 },
    inserts,
    rowClasses: (row) => pendingRowClasses(props.tabId, row, p?.rowCount ?? 0),
    rowColumns: insertColumns
      ? (handle) => (handle.insertId !== undefined ? insertColumns : undefined)
      : undefined,
    extractValue: p
      ? createDisplayValueExtractor(props.tabId, p, order, activeMaskTransform)
      : () => ({ text: '', isNull: true, truncated: false }),
  };
}

const rootRef = ref<HTMLElement | null>(null);

// P67 §3/§4.3: the open preview popover for this tab, if any — v-if'd in the template (:2329's
// root div). A plain ref (not part of `runtime[tabId]`, §4.1's own "never touches runtime" rule):
// this is purely a transient UI overlay, never persisted, never read by anything outside this file.
interface FkPreviewOpen {
  x: number;
  y: number;
  entry: ForeignKeyMeta;
  ctx: FkNavContext;
  openInNewTab: () => void;
}
const fkPreview = ref<FkPreviewOpen | null>(null);
function closeFkPreview(): void {
  fkPreview.value = null;
}

// Never a ref/shallowRef/reactive (CodeMirrorHost.vue's own rule, restated here — D3).
let grid: KiraSlickGrid | null = null;
let eventHandler: SlickEventHandler | null = null;
let dataSource: ReturnType<typeof createGridDataSource> | null = null;
let viewportEl: HTMLElement | null = null;
// The grid root, set at mount for the insert region's own delegated listeners' teardown
// (below); `rootRef.value` is not used there since a template ref is not guaranteed to still
// point at the mounted element by the time `onUnmounted` runs (`viewportEl`'s own identical
// mount-time-capture pattern, just above, is why this follows it rather than `rootRef.value`).
let gridRootEl: HTMLElement | null = null;
// C4/§5 D0 rule 2 — never a ref/shallowRef/reactive, same as `grid` itself.
let selectionModel: SlickHybridSelectionModel | null = null;
// AttributeTooltip.vue's own `container` prop: a ref (not a plain let) since
// AttributeTooltip's `useEventListener(computed(() => props.container), ...)` re-binds reactively
// once this is set, after init()'s synchronous header build below. An ARRAY, not a
// single element: SlickGrid always splits the header row into two sibling DOM panes — `_headerL`
// (frozen columns) and `_headerR` (the rest) — appended directly under the grid's own root `el`,
// with no narrower common ancestor than `el` itself wrapping both (confirmed reading
// initialize() in slick.grid.js). A single `querySelector('.slick-header-columns')` only ever
// grabs `_headerL` (first DOM match), so a table with any frozen column silently lost header
// tooltips for every OTHER column — hovering `tenant_id` on a
// composite-PK table produced zero pointermove events on the bound container even though the
// browser's own `:hover` state was set (confirmed via CDP trace), because the pointer was over
// `_headerR`, never bound. `useEventListener` accepts `Arrayable<HTMLElement>` natively, so both
// panes get one real listener each — no widening to the whole grid body needed.
const headerRowEls = ref<HTMLElement[]>([]);
// P22 postscript §14.2's "cell-editor dock panel is sometimes missing its header" investigation
// (below, onMounted) surfaced a real, adjacent bug while chasing it: SlickGrid never self-observes
// its own container's size (confirmed reading slick.grid.ts — no ResizeObserver anywhere in the
// library; `resizeCanvas()` is the caller's job entirely). This app never called it when the
// container's *own* size changed post-mount, only implicitly at construction — so opening/closing
// the cell-editor dock (a sibling flex item that shrinks `.grid-area`, CellEditorDock.vue) left the
// grid's internal `.slick-pane`/`.slick-viewport` elements at their stale, pre-shrink inline height,
// genuinely overlapping the dock panel now painted below and intercepting its own clicks — the
// concrete mechanism behind `cell-editor.spec.ts`'s pre-existing "Target page ... has been closed"
// timeouts (a `.grid-canvas` element, still full-height, sat on top of the dock's own controls).
// P99 §9.3: not VueUse's useResizeObserver — this file's own onUnmounted has an explicit "Order
// matters (§6 D3)" teardown sequence (resizeObserver.disconnect() before grid.destroy(), listener
// removal in a specific order relative to both); a composable's onScopeDispose-driven cleanup can't
// be slotted into that hand-ordered sequence without risking the exact class of timing bug P22
// fixed. Declined, named per CLAUDE.md's library rule.
let resizeObserver: ResizeObserver | null = null;
// D4's own one-shot flag, set by the header select zone immediately before it pushes ranges into
// the model — mirrors DataGrid.vue's own `dragProducedRange` shape.
let pendingSelectionKind: 'column' | null = null;
// P139: the corner's select-all paints via the `.kira-select-all` host class, not the model's range.
const selectAllActive = ref(false);
// Shift-range anchor for the header select zone's own click cycle — DataGrid.vue's own `colAnchor`
// ref, kept as a plain variable here since nothing renders from it.
let colAnchor: number | null = null;

// D11c's own cheap precheck's Set membership, cached across the whole meta lifetime — recomputed
// only where `rt()?.meta`'s own watch already rebuilds columns (rebuildAndSetColumns's own call
// site, below), never per cell or per hover.
let navColumns: NavColumns = navColumnsFor(null);
// See `placeNavButtonsForRenderedCells`' own comment: a real button per nav-eligible rendered
// cell, always visible, no hover tracking left to own.

// Incremented by every native `scroll` event on the viewport, and read by
// KiraSlickGrid's own per-frame chase gate (see `scheduleChase` there). A counter, not a
// timestamp: the wall-clock gate it joins cannot tell "no scroll is driving this frame" from
// "the scroll that is driving this frame arrived in the previous, long, frame".
let scrollEventSeq = 0;
// The gesture-onset fix and its dedupe/seed/lastScrollEventAt plumbing moved into
// views/shared/slick/scrollVelocity.ts, byte-identical to ConsoleSlickGrid.vue's own copy — see
// that module's own comments for why the fix is needed and why the dedupe makes pulling from
// velocity() itself safe.
const scrollVelocityTracker = createScrollVelocityTracker(() => viewportEl);

// The host's own viewport scroll listener. The test-only scroll-work mark
// lives in KiraSlickGrid's `render()` override, not here: SlickGrid's
// `scrollRenderThrottling` defers a far-jump render past this listener, so a mark here times the
// throttle wait, not render work. Render timing is reported by
// scrollTrace.noteRenderMs() from that same override — not from getRenderedRange, which only
// computes the range and runs *before* the work that override times has happened.
function onViewportScroll(): void {
  const el = viewportEl;
  if (!el) return;
  // P67 §4.3: the preview popover is anchored to a viewport point over a virtualized row — a
  // scroll makes that point mean nothing (a different row, maybe no row) — reuses this existing
  // listener rather than binding a second one to the same element.
  if (fkPreview.value) closeFkPreview();
  const now = performance.now();
  scrollTrace.noteScrollEvent(el.scrollTop, now);
  scrollEventSeq++;
  // P22 iter2-onset D1: still the sampler's other driver, unchanged in effect — but now a no-op
  // whenever velocity() already pulled this very position a moment earlier, from inside the
  // render this same event drove (see scrollVelocity.ts's own comment).
  scrollVelocityTracker.recordOffsetSample(el.scrollTop, now);
}

// SchemaDialog.vue's own useDebounceFn precedent (Part 2) — cancel() in onUnmounted replaces the
// clearTimeout below. TIMING.scrollPersist, not TIMING.persist: the patch lands mid-scroll at 150
// ms and breaks cell-editor.spec's column scan.
const persistScroll = useDebounceFn(() => {
  const el = viewportEl;
  if (!el) return;
  tabsStore.patchDataTabState(props.tabId, { scrollTop: el.scrollTop, scrollLeft: el.scrollLeft });
}, TIMING.scrollPersist);
function onViewportScrollPersist(): void {
  void persistScroll();
}

// The visible-window report drives the pruning. KiraSlickGrid.lastRenderedRowBounds is
// the *rendered* (overscanned) range, not the strictly-visible one grid.onRendered's own
// {startRow, endRow} carries (see kiraSlickGrid.ts's own comment on why the difference matters) —
// this reads the wider one so a row still inside the runway keeps its decode cache alive.
// The one per-render DOM pass this migration still needs: a row's own
// `data-row` (SlickGrid writes the *display position* there, `src/slick.grid.ts`'s own
// `appendRowHtml`) has to be corrected to the *page* row every other subsystem (selection,
// pending changes, search, the gutter number) addresses a row by. The two differ only while a
// search filter is hiding non-matching rows — a trap commented at both ends.
//
// Idempotent by construction, not by re-deriving "did the rendered range change": a `.slick-row`
// already carrying `data-kira-row-tagged` is skipped outright, so a sub-row scroll (no new row
// entering the DOM) touches nothing at all — `slick-grid.spec.ts`'s existing zero-mutation gate
// enforces that for free. A row div SlickGrid later discards and rebuilds (`invalidateRow`) comes
// back with no marker, correctly re-tagged from its own fresh (display-position) `data-row`.
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
    // F4 — the row div is cloned per frozen pane; only the right (data) pane's clone gets
    // `data-testid="grid-row"` (or `"grid-row-insert"`), so `[data-testid="grid-row"]` counts
    // stay exactly what they were under the incumbent (one per rendered row, not two).
    if (el.closest('.grid-canvas-right')) {
      el.dataset.testid = handle.insertId !== undefined ? 'grid-row-insert' : 'grid-row';
    }
    if (handle.insertId !== undefined) {
      el.dataset.insertId = handle.insertId;
      // `cellAttrs` (buildColumns, D2) is a per-COLUMN constant — it cannot vary per row, so an
      // insert row's own cells (D9's own region) are the one place a per-row cell pass still
      // earns its keep, matching the incumbent's own `data-testid="grid-cell-insert"`.
      for (const cellEl of el.querySelectorAll<HTMLElement>(
        '.slick-cell[data-testid="grid-cell"]',
      )) {
        cellEl.dataset.testid = 'grid-cell-insert';
      }
    }
  }
}

// C5/§5 D5 — a display column index -> the SlickGrid column *id* (field name) it corresponds to,
// i.e. the key setCellCssStyles' own hash wants (F2's own `hash[j][this.columns[k].id]`).
// `getColumns()` includes the gutter at index 0, hence the +1.
function fieldAtDisplayCol(displayCol: number): string | undefined {
  const c = grid?.getColumns()[displayCol + 1];
  return c ? String(c.field) : undefined;
}

// D4 (fix) — live visual feedback for a cell-range drag. SlickHybridSelectionModel's own
// `handleCellRangeSelected` intentionally no-ops for a CELL-mode `onCellRangeSelecting` call
// (only `onCellRangeSelected`, at drag end, calls `setSelectedRanges` — §4.1 item 3's own
// documented reasoning, but that reasoning is about the cell-editor dock's flicker specifically,
// never about the grid's own selection highlight); `dragToSelect: true` (needed for the gutter's
// own *row*-range drag — see its own branch below, which has no such early return and so already
// updates live) also zeroes the stock `SlickCellRangeDecorator`'s own border
// (`selectionCss: {border: 'none'}`), so nothing at all painted during a cell-range drag until
// mouseup — confirmed live with a real mousedown -> mousemove(x2) -> mouseup sequence, not a
// single programmatic selection call.
//
// This paints the SAME fill/edge layers the committed selection uses, from the drag's own
// in-progress `SlickRange` — via `selectionFromRanges`/`toPageRowSelection` (identical translation
// `onSelectedRangesChanged` itself uses) — but deliberately never touches `rt().selection` or the
// cell-editor dock: writing `rt().selection` on every drag tick is exactly what DataGrid.vue's own
// deleted `cellDragActive` flag existed to guard against (a drag passing back over its own anchor
// cell transiently looks like a completed one-cell selection, flashing the dock open and shut),
// and §4.1 item 3 is right that nothing needs it once `rt().selection` itself stays untouched until
// the real commit. The committed `onSelectedRangesChanged` handler (unchanged) supersedes every
// layer this writes the moment the drag actually ends.
function onCellRangeSelecting(_e: unknown, args: { range: SlickRange }): void {
  selectAllActive.value = false;
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
      fieldAtDisplayCol,
    ),
  );
}

/** C5/§5 D5 — bounded by what the user staged (`pendingFor(tabId).edits`, a handful of rows in
 *  any real session), clipped to the rendered range the same way the edges layer is. */
function computeStagedHash(): Record<number, Record<string, string>> {
  const hash: Record<number, Record<string, string>> = {};
  if (!grid || !dataSource) return hash;
  const { edits } = pendingChangesStore.pendingOnPage(props.tabId);
  if (edits.size === 0) return hash;
  const { start, end } = grid.lastRenderedRowBounds;
  if (end < start) return hash;
  const cls = classesFrom({ pendingEdit: true })[0] ?? 'pending-edit';
  for (let pos = start; pos <= end; pos++) {
    const pageRow = dataSource.getItem(pos).row;
    const changes = edits.get(pageRow);
    if (!changes) continue;
    const row: Record<string, string> = {};
    for (const column of Object.keys(changes)) row[column] = cls;
    hash[pos] = row;
  }
  return hash;
}

function refreshStagedLayer(): void {
  grid?.setCellCssStyles('kira-staged', computeStagedHash());
}

// `kira-search`'s own row: "every match", not clipped to the rendered range the way
// `kira-staged` is (a handful of staged rows is
// cheap to re-walk on every render-band change; a search result is not bounded that way, so
// this only recomputes on the result or current match changing, plus a live filter shifting every
// match's own display position, both watched below). With 10 000 matches active, a selection
// change must still land inside the same 150ms sandbox bound this file's other cost gates use;
// the fallback if it doesn't is a rendered-range ± hysteresis band.
function computeSearchHashes(): [EdgeHash, EdgeHash] {
  if (!grid || !dataSource) return [{}, {}];
  const entry = searchState[props.tabId];
  if (!entry || entry.matches.length === 0) return [{}, {}];
  const p = getPage(props.tabId);
  if (!p) return [{}, {}];
  const orderSet = new Set(currentOrder());
  const idx = { displayRows: currentDisplayRows(), pageRowCount: p.rowCount };
  return searchCellLayers(
    entry.matches,
    entry.index,
    (col) => p.columns[col]?.name,
    (name) => orderSet.has(name),
    (row) => displayPositionOf(idx, row),
    classesFrom({ searchMatch: true })[0] ?? 'search-match',
    classesFrom({ searchMatchCurrent: true })[0] ?? 'search-match-current',
  );
}

function refreshSearchLayer(): void {
  if (!grid) return;
  const [matchHash, currentHash] = computeSearchHashes();
  grid.setCellCssStyles('kira-search', matchHash);
  grid.setCellCssStyles('kira-search-current', currentHash);
}

// The rendered row *band* (not every render — a sub-row scroll re-renders nothing new, and the
// edges/staged layers only ever depend on what's mounted) leaving the previous one is
// the other trigger for the edges layer, beside a selection change; folded into
// onGridRendered since it already runs on every grid.onRendered and already reads
// lastRenderedRowBounds.
let lastCssLayerBand = { start: 0, end: -1 };
let lastAppliedPage: ReturnType<typeof getPage> = null;
let lastAppliedOrderKey = '';

function onGridRendered(): void {
  if (!grid || !dataSource) return;
  const band = renderedPageRowBand(grid, dataSource);
  if (!band) return;
  setVisibleWindow(props.tabId, band.lo, band.hi + 1);
  setVisibleRows(props.tabId, band.lo, band.hi + 1);

  if (band.start !== lastCssLayerBand.start || band.end !== lastCssLayerBand.end) {
    lastCssLayerBand = { start: band.start, end: band.end };
    refreshStagedLayer();
  }
  // Places an always-visible button per nav-eligible rendered cell (see
  // placeNavButtonsForRenderedCells' own comment). Still called on every render: a render can
  // rebuild the very cell node a button lives in (`invalidateRow`), silently dropping it, so this
  // has to re-scan rather than run once.
  placeNavButtonsForRenderedCells();
}

// `pos`/`cellIdx` are SlickGrid's own display position / column index (incl. the gutter at 0) —
// `getCellFromEvent`'s and `getActiveCell`'s own shape, not page-row/display-col (D7's own split);
// translated to the latter only here, at the one call site `rowValues.ts`'s `cellNavEntry` needs.
function navEntryAt(pos: number, cellIdx: number): CellNavEntry | null {
  if (!dataSource || cellIdx === 0) return null; // the gutter itself is never navigable
  const handle = dataSource.getItem(pos);
  if (handle.insertId !== undefined) return null; // D9's insert region has no real row to jump to
  return rvCellNavEntry(
    props.tabId,
    getPage(props.tabId),
    currentOrder(),
    rt()?.meta ?? null,
    tab()?.connectionId ?? null,
    currentDialect(),
    navColumns,
    handle.row,
    cellIdx - 1,
  );
}

// Deliberate redesign, not a bug fix (item 12, a later coordinator round): "make the PK/FK nav
// button always visible (not hover-only), and have it overlay on top of the cell's text... rather
// than reserving padding". Supersedes D11b's single host-owned, JS-moved-on-hover button —
// one real <button> per rendered nav-eligible cell instead, built once per cell and left in
// place (idempotent: a cell already carrying `.cell-nav-btn` is skipped) until its row is
// rebuilt, the same per-render-pass discipline `tagRenderedRows`' own `data-kira-row-tagged`
// idempotency already uses, and bounded the identical way that pass already is: the rendered
// window's rows × however many of this table's columns are nav-eligible, never the whole table.
// This is a deliberate departure from §4.1 item 5's own cost note (D11b's comment above used to
// cite it) — the trade this makes explicitly, at the user's own request: `cellNavEntry`'s real,
// row-value-dependent lookup (a referencedBy query, `navEntryAt` above) still only ever runs on
// click, never per render — this only needs the CHEAP, column-level precheck `cellFormatter`
// already classes every cell with (`.fk`/`.has-nav`) to pick which icon glyph to show, so it
// cannot know per-row whether a specific PK value actually has any referencing rows the way the
// old hover path's real `navEntryAt` call did before showing anything — a PK cell in a nav-
// eligible COLUMN always gets a button now, even on a row that turns out (only discoverable by
// clicking) to have no real referencing rows. Accepted trade for "always visible": the button
// still resolves and no-ops harmlessly on click in that case (`onGridClick` below, unchanged).
// A useful side effect, not the primary motivation: this also structurally removes the whole
// class of hover-driven append/remove flicker the old single-button design had (item 4, this same
// coordinator round) — there is no more DOM node being moved on hover at all.
// A6/P21 round 1: `onGridRendered` fires once per render() — once per scroll frame during a fling,
// plus every self-scheduled chase render — so the `:not([data-kira-nav-placed])` selector (mirrors
// `tagRenderedRows`' own `data-kira-row-tagged` marker, right above) is what makes a sub-row scroll
// (nothing new mounted) match zero elements. The previous idempotency check (`cellEl.querySelector
// ('.cell-nav-btn')`) was correct but still paid for a full `querySelectorAll` over the grid
// subtree, plus one `querySelector` per matched cell, on every frame regardless of whether
// anything new had mounted — a constant per-frame DOM sweep on the exact path
// docs/PERF.md §2.1a/§2.1c have spent three phases chasing.
function placeNavButtonsForRenderedCells(): void {
  const root = rootRef.value;
  if (!root) return;
  const cells = root.querySelectorAll<HTMLElement>(
    '.grid-canvas-right .slick-cell.has-nav:not([data-kira-nav-placed])',
  );
  for (const cellEl of cells) {
    cellEl.dataset.kiraNavPlaced = '1';
    const isFk = cellEl.classList.contains('fk');
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = NAV_BTN_CLASS;
    btn.dataset.testid = 'cell-nav-button';
    btn.dataset.navKind = isFk ? 'fk' : 'pk';
    btn.setAttribute(
      'aria-label',
      isFk
        ? t('studio.grid.nav.goTo.aria')
        : t('studio.definition.constraint.type.referencedBy.label'),
    );
    const icon = document.createElement('span');
    icon.setAttribute('aria-hidden', 'true');
    icon.style.fontSize = '13px';
    icon.className = `codicon codicon-${isFk ? 'arrow-right' : 'references'}`;
    btn.appendChild(icon);
    cellEl.appendChild(btn);
  }
}

// `cell-nav-btn`/`header-key` stay as marker names (click delegation, test selectors).
const NAV_BTN_CLASS = `cell-nav-btn absolute left-1 top-1/2 z-1 size-4 -translate-y-1/2 bg-elevated ${buttonVariants({ variant: 'toolbar', size: 'inline' })}`;
const HEADER_KEY_CLASS = 'header-key ml-1 shrink-0 font-data text-warn';
const HEADER_KEY_FK_CLASS = 'header-key is-fk ml-1 shrink-0 font-data text-info';

// A click on a `.cell-nav-btn` (a real child of whichever `.slick-cell` it lives in, one
// per nav-eligible rendered cell — placeNavButtonsForRenderedCells' own comment) resolves through
// SlickGrid's own `getCellFromEvent` exactly like any other cell click, so `args.row`/`args.cell`
// already name the right cell without this file tracking anything of its own;
// `stopImmediatePropagation` (checked by `handleClick` right after triggering `onClick`,
// `slick.grid.ts:4611-4614`) is what stops the click from *also* moving the active cell there.
// An FK-kind click ('fk' — an outbound edge) now opens the preview popover instead of
// navigating straight there; a PK-kind click ('pk' — "Referenced by") is untouched (it names
// *tables*, plural, with no single record to preview). `entry.items` is already the enabled-edges-
// only list `cellNavEntry` built (disabled edges filtered out); `fkEdgesFor` re-derives the same
// `meta.foreignKeys` filter `foreignKeyNavItems` uses so each item can be matched back to its raw
// `ForeignKeyMeta` by id (`go-to-referenced-<fk.name>`, the stable convention) — the popover
// needs the raw edge (referencedPath/columns), not just the resolved MenuItem.
function fkEdgesFor(meta: ObjectMeta | null, columnName: string): ForeignKeyMeta[] {
  return meta?.foreignKeys.filter((fk) => fk.columns.includes(columnName)) ?? [];
}
function fkEdgeForItem(edges: ForeignKeyMeta[], item: MenuItem): ForeignKeyMeta | null {
  if (item.type !== 'item') return null;
  return edges.find((fk) => `go-to-referenced-${fk.name}` === item.id) ?? null;
}

function onGridClick(e: SlickEventData, args: OnClickEventArgs): void {
  if (!e.target?.closest('.cell-nav-btn')) return;
  e.stopImmediatePropagation();
  const entry = navEntryAt(args.row, args.cell);
  if (!entry) return;
  const native = e.getNativeEvent<MouseEvent>();

  if (entry.kind === 'fk') {
    const order = currentOrder();
    const columnName = order[args.cell - 1] ?? '';
    const handle = dataSource?.getItem(args.row);
    const meta = rt()?.meta ?? null;
    const connectionId = tab()?.connectionId ?? null;
    if (handle === undefined || !connectionId) return;
    const ctx: FkNavContext = {
      connectionId,
      dialect: currentDialect(),
      rowValues: navValuesFor(props.tabId, getPage(props.tabId), order, navColumns, handle.row),
      numericColumns: numericNavColumns(getPage(props.tabId), navColumns),
    };
    const edges = fkEdgesFor(meta, columnName);
    const openPreview = (fk: ForeignKeyMeta, openInNewTab: () => void): void => {
      fkPreview.value = { x: native.clientX, y: native.clientY, entry: fk, ctx, openInNewTab };
    };
    // Exactly one candidate opens the preview directly; more than one opens the same
    // ContextMenu popup the right-click cell menu uses, but each item now opens the preview for
    // its own edge rather than navigating (the "no capability lost" — the popover's own
    // "Open in new tab" is that item's original navigateForeignKey run, one click further in).
    if (entry.items.length === 1) {
      const item = entry.items[0];
      const fk = item ? fkEdgeForItem(edges, item) : null;
      if (item?.type === 'item' && fk) openPreview(fk, () => void item.run());
      return;
    }
    const previewItems: MenuItem[] = entry.items.map((item) => {
      const fk = fkEdgeForItem(edges, item);
      if (item.type !== 'item' || !fk) return item;
      return { ...item, run: () => openPreview(fk, () => void item.run()) };
    });
    contextMenuStore.openContextMenu(native, previewItems);
    return;
  }

  // kind === 'pk' ("Referenced by") — unchanged (§9): exactly one candidate navigates immediately;
  // more than one opens the same ContextMenu popup the right-click cell menu uses.
  if (entry.items.length === 1) {
    const only = entry.items[0];
    if (only?.type === 'item') void only.run();
    return;
  }
  contextMenuStore.openContextMenu(native, entry.items);
}

function currentWidths(): Record<string, number> {
  return tab()?.state.columnWidths ?? {};
}

// Re-measuring on every reload resized columns the user never dragged; measure only when the
// page's column set changes.
function settledWidthsFor(page: ReturnType<typeof getPage>): Record<string, number> {
  if (!page) return {};
  const key = page.columns.map((c) => c.name).join('\u0000');
  const settled = gridViewStore.settledWidthsFor(props.tabId);
  if (settled?.key === key) return settled.widths;
  const measured = initialWidths(page);
  // An empty page measures headers only; settling on that would pin later data to it.
  if (page.rowCount > 0) gridViewStore.setSettledWidths(props.tabId, key, measured);
  return measured;
}

function rebuildAndSetColumns(): void {
  if (!grid) return;
  const p = getPage(props.tabId);
  const order = p ? resolveColumnOrder(p, tab()?.state.columnOrder ?? null) : [];
  grid.setColumns(buildColumns(p, order, currentWidths(), settledWidthsFor(p), rt()?.meta ?? null));
  // setColumns rebuilds every header from scratch (F8's own indicator divs included) — restore
  // the sort chevrons the fresh headers just lost.
  syncSortIndicators();
}

// The PK/FK badge and the header select zone are the two pieces of header DOM
// SlickGrid's own Column shape can't express (a static `name` string, no child markup), so they're
// appended imperatively once per header cell BUILD. `onHeaderCellRendered` fires once per column
// per `setColumns` call (never per scroll frame — headers aren't virtualized), matching the
// gutter/data-column split cellAttrs/headerCellAttrs already draw.
function onHeaderCellRendered(_e: unknown, args: OnHeaderCellRenderedEventArgs): void {
  const name = String(args.column.field ?? '');
  if (name === GUTTER_FIELD) return;
  // The fresh-render half of the `data-sort` mirror; `syncSortIndicators`'s own direct
  // write (below) covers a plain sort change, which never rebuilds this header cell at all
  // (`setSortColumns` toggles SlickGrid's own indicator classes without a
  // `setColumns` rebuild) — this half only fires when a rebuild (a page reload, a meta/appearance
  // change) discards whatever this node already had and needs it set fresh.
  const sortTerm = currentSortTerms().find((t) => t.column === name);
  if (sortTerm) args.node.dataset.sort = sortTerm.direction;
  const p = getPage(props.tabId);
  const descriptor = p?.columns.find((c) => c.name === name);
  const label = keyLabelFor(descriptor, name, foreignKeyNamesFor(rt()?.meta ?? null));
  if (label) {
    const badge = document.createElement('span');
    badge.className = label === 'FK' ? HEADER_KEY_FK_CLASS : HEADER_KEY_CLASS;
    badge.textContent = label;
    args.node.appendChild(badge);
  }
  // Deliberate redesign, not the original port: "left-
  // click on a header's body selects that column instead of sorting", "right-click also selects
  // the column" (already true — onHeaderContextMenuHandler, below), and "sorting only happens via
  // a dedicated arrow control" on the right. The zone used to be a narrow 10px strip at the
  // cell's left edge, leaving the rest of the header body to SlickGrid's own native
  // handleHeaderClick/setupColumnSort (slick.grid.ts) — that native click-to-sort binding is
  // UNCHANGED (still reads `column.sortable`, still fires `grid.onSort`), but now it never
  // runs from a header-body click: the zone below covers the ENTIRE header cell
  // (`inset: 0`, not just a strip) and stops propagation before a body click can ever bubble up
  // to that delegated listener. The one thing still allowed to sort is the dedicated arrow button
  // built further down, whose own listener stops propagation before THIS zone's listener runs
  // (DOM order: the zone is a lower sibling in paint order via z-index, not later in the
  // listener chain, so both need their own stopPropagation independently).
  // Not useEventListener — `zone` (and `sortIndicator` below) is a node SlickGrid itself
  // creates and destroys per header-cell render (onHeaderCellRendered fires repeatedly, well after
  // setup()'s effect scope is captured), not a stable Vue-tracked ref; its own destruction is what
  // tears the listener down (onBeforeHeaderCellDestroy's own comment). Declined, named exception.
  const zone = document.createElement('span');
  zone.className = 'header-select-zone absolute inset-0 z-0';
  zone.dataset.testid = 'grid-header-select';
  zone.dataset.column = name;
  zone.dataset.colIndex = args.node.dataset.colIndex ?? '';
  zone.setAttribute('role', 'button');
  zone.setAttribute('aria-label', t('studio.grid.selectColumn.aria'));
  zone.addEventListener('click', (ev) => {
    ev.stopPropagation();
    const displayCol = Number(zone.dataset.colIndex);
    if (Number.isInteger(displayCol)) onHeaderSelectClick(displayCol, ev as MouseEvent);
  });
  args.node.appendChild(zone);

  // Dedicated sort control — SlickGrid always builds a `.slick-sort-indicator` div
  // per column regardless of sort state (the comment, above), so this reuses that existing
  // node as the click target rather than adding a second element: it already sits at the header
  // cell's right edge (slickTheme.css's own margin/positioning, unchanged) and `setSortColumns`
  // already drives its `-asc`/`-desc` glyph classes on every sort change (syncSortIndicators) —
  // nothing about that display wiring changes, only that clicking it now also DOES something.
  // `slickTheme.css` gives it a small permanent width so it has a real hit target even at rest
  // (an inactive column's arrow is invisible — no glyph — but still clickable, the same "hidden
  // affordance" trade this whole redesign makes explicit).
  const sortIndicator = args.node.querySelector<HTMLElement>('.slick-sort-indicator');
  if (sortIndicator) {
    sortIndicator.dataset.testid = 'grid-header-sort';
    sortIndicator.setAttribute('role', 'button');
    sortIndicator.setAttribute('aria-label', t('studio.grid.sortBy.aria', { name }));
    sortIndicator.tabIndex = 0;
    sortIndicator.addEventListener('click', (ev) => {
      ev.stopPropagation();
      cycleSortFor(name);
    });
  }
}

// Nothing to clean up beyond the header cell's own subtree (destroyed with it, badge/zone and the
// zone's listener included) — subscribed anyway so a future addition (e.g. a listener registered
// *outside* this node) has an obvious place to land, per §5 D2's own header-cell lifecycle pair.
function onBeforeHeaderCellDestroy(_e: unknown, _args: OnBeforeHeaderCellDestroyEventArgs): void {}

// The app's own asc -> desc -> none cycle, mirrored from `sortTerms`/tab.state.sort (the
// text-sort parse included) into SlickGrid's own indicator DOM via setSortColumns, which redraws
// only the header — no row touched, no setColumns rebuild. Called from a watch callback, not
// a computed: this makes an imperative call into the grid.
function currentSortTerms(): { column: string; direction: 'asc' | 'desc' }[] {
  const t = tab();
  const sort = t?.state.sort;
  if (!sort) return [];
  const p = getPage(props.tabId);
  const order = p ? resolveColumnOrder(p, t.state.columnOrder ?? null) : [];
  if (sort.kind === 'structured') return sort.terms;
  return parseTextSortTerms(sort.text, order);
}
function syncSortIndicators(): void {
  if (!grid) return;
  const terms = currentSortTerms();
  grid.setSortColumns(
    terms.map((term) => ({ columnId: term.column, sortAsc: term.direction === 'asc' })),
  );
  // The direct-write half of the `data-sort` mirror (`onHeaderCellRendered`'s own comment
  // explains the split): `setSortColumns` only toggles the chevron's own CSS classes, never
  // rebuilds the header cell, so a plain sort change (no page reload) needs this file to write
  // the attribute itself rather than waiting for a render that isn't coming.
  for (const col of grid.getColumns()) {
    if (col.id === GUTTER_FIELD) continue;
    const node = grid.getHeaderColumn(col.id as string);
    if (!node) continue;
    const term = terms.find((t) => t.column === col.id);
    if (term) node.dataset.sort = term.direction;
    else delete node.dataset.sort;
  }
}

// Deliberate redesign, not the original port — supersedes the `onSort`/`grid.onSort` wiring. That
// handler translated SlickGrid's own NATIVE
// click-to-sort event (`tristateMultiColumnSort` + `multiColumnSort: false`'s own asc -> desc ->
// none cycle, fired from `setupColumnSort`'s delegated header-click listener, slick.grid.ts) into
// `setSort` — but a header body click no longer reaches that native listener at all
// (onHeaderCellRendered's own `header-select-zone` now covers the whole cell and stops
// propagation before it can bubble there), so `grid.onSort` can no longer fire from a click; the
// header context menu's own Sort asc/Sort desc/Clear sort items (menu.ts) already call `setSort`
// directly and were never routed through `onSort` either. This function is what the new dedicated
// sort-arrow control (onHeaderCellRendered, above) calls instead — the identical single-column
// asc -> desc -> none tristate cycle `onSort` used to derive from SlickGrid's own bookkeeping,
// computed here directly from `currentSortTerms()` (the same source `syncSortIndicators` already
// reads), so no SlickGrid-internal sort state needs to exist for this to work correctly.
function cycleSortFor(name: string): void {
  const current = currentSortTerms().find((t) => t.column === name);
  if (!current) {
    void gridViewStore.setSort(props.tabId, {
      kind: 'structured',
      terms: [{ column: name, direction: 'asc' }],
    });
  } else if (current.direction === 'asc') {
    void gridViewStore.setSort(props.tabId, {
      kind: 'structured',
      terms: [{ column: name, direction: 'desc' }],
    });
  } else {
    void gridViewStore.setSort(props.tabId, null);
  }
}

// The corner cell selects everything, as a single `range` (never a `row` selection, which
// `isSelected` would resolve with `Array.includes`, the O(rows) cost). Pushing the full range
// through the selection model cost 80-130 ms on spike_grid (1 000 × 61) — SlickGrid's
// O(rows × cols) `kira-cell-selected` hash — against the 150 ms gate. So the model holds no
// range: `rt().selection` owns the meaning and `.kira-select-all` on the host root paints
// every cell.
function onSelectAll(): void {
  if (!grid || !selectionModel) return;
  // Display row count, not the page's own row count — `SlickRange` below is position-space, and
  // the two only coincide when nothing is filtered (same fix as onHeaderSelectClick/
  // onHeaderContextMenuHandler's own `displayRowCount`): under an active search filter this
  // used to run the range past the last displayed row into the pending-insert-row
  // address space (`rowAtDisplayPosition` in dataSource.ts maps an out-of-range position to
  // `pageRowCount + (pos - count)`), producing spurious extra lines on copy.
  const displayRowCount = currentDisplayRows()?.length ?? getPage(props.tabId)?.rowCount ?? 0;
  const colCount = grid.getColumns().length - 1; // minus the gutter
  if (displayRowCount <= 0 || colCount <= 0) return;
  // Clears the model's ranges, SlickGrid's fill hash and `rt().selection`; also resets the flag.
  selectionModel.setSelectedRanges([]);
  const all = selectionFromRanges(
    [new SlickRange(0, 1, displayRowCount - 1, colCount)],
    false,
    null,
  );
  const entry = rt();
  if (!entry || !all) return;
  entry.selection = toPageRowSelection(all);
  selectAllActive.value = true;
}

function onHeaderClick(_e: unknown, args: OnHeaderClickEventArgs): void {
  if (args.column.id === GUTTER_FIELD) onSelectAll();
}

// SlickGrid drags the handle and persists nothing on its own; this app reads the resulting
// widths straight off getColumns() and patches them into tab state. The echo guard is the
// same shape DataGrid.vue's own dragProducedRange uses: without it, the columnWidths watch below
// would call setColumns and rebuild every header (and every rendered row) mid-drag.
let suppressWidthEcho = false;
function onColumnsResized(): void {
  if (!grid) return;
  const t = tab();
  if (!t) return;
  const widths: Record<string, number> = { ...t.state.columnWidths };
  for (const col of grid.getColumns()) {
    if (col.id === GUTTER_FIELD || col.width === undefined) continue;
    widths[String(col.id)] = col.width;
  }
  suppressWidthEcho = true;
  tabsStore.patchDataTabState(props.tabId, { columnWidths: widths });
  suppressWidthEcho = false;
}

// `selectionFromRanges`/`rangesFromSelection` (shared/slick/selection.ts) are deliberately
// row-space-agnostic; these two glue functions are the one place that translates between a
// `SlickRange`'s own display-position rows and `Selection`'s page rows, via dataSource.ts's own
// rowAtDisplayPosition/displayPositionOf (identity today).
function toPageRowSelection(sel: Selection): Selection {
  if (!dataSource) return sel;
  const idx = {
    displayRows: currentDisplayRows(),
    pageRowCount: getPage(props.tabId)?.rowCount ?? 0,
  };
  const toPageRow = (pos: number) => rowAtDisplayPosition(idx, Math.max(0, pos));
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
function toPositionSelection(sel: Selection): Selection {
  const idx = {
    displayRows: currentDisplayRows(),
    pageRowCount: getPage(props.tabId)?.rowCount ?? 0,
  };
  const toPos = (row: number) => displayPositionOf(idx, row);
  switch (sel.kind) {
    case 'cell':
      return { ...sel, row: toPos(sel.row) };
    case 'range':
      return { ...sel, anchorRow: toPos(sel.anchorRow), row: toPos(sel.row) };
    case 'row':
      return { ...sel, rows: sel.rows.map(toPos) };
    default:
      return sel;
  }
}

// `selectedCellCssClass: 'kira-cell-selected'` makes SlickGrid itself paint
// the highlight straight off `selectionModel`'s own `SlickRange[]`, which lives in DISPLAY-POSITION
// space and is only recomputed inside `setSelectedRanges` (slick.grid.ts). The filter-toggle watch
// below swaps the data source and re-renders, but a display-position range built under the OLD
// filter keeps pointing at whatever page row now happens to sit at that same position under the
// NEW one — the highlight can visibly land on the wrong row while `rt().selection` (page-row space,
// unaffected) still correctly targets the original one, so a subsequent Ctrl+C/Delete hits the
// right data while the highlight lies about it. Same defect class as ConsoleSlickGrid.vue's own
// fix (its `refreshSelectionLayer`) — here the model owns the geometry, so the fix is to
// re-push freshly-translated ranges rather than repaint a CSS layer by hand.
//
// A row/corner that fell out of the new filter (`isRowVisible` — never `displayPositionOf`'s own
// nearest-match fallback, which would just as silently point the highlight at a neighboring row)
// drops out of a 'row' selection, or clears a 'cell'/'range' selection entirely (a 'range' whose
// anchor or focus row is no longer visible can't be re-expressed as one rectangle in the new
// space). 'column' selections need no row-visibility check — they already span every display row.
function refreshSelectionForFilterChange(): void {
  if (!grid || !selectionModel) return;
  const runtimeEntry = rt();
  const sel = runtimeEntry?.selection ?? null;
  if (!sel) return;
  const idx = {
    displayRows: currentDisplayRows(),
    pageRowCount: getPage(props.tabId)?.rowCount ?? 0,
  };

  let visibleSel: Selection | null;
  switch (sel.kind) {
    case 'cell':
      visibleSel = isRowVisible(idx, sel.row) ? sel : null;
      break;
    case 'range':
      visibleSel = isRowVisible(idx, sel.anchorRow) && isRowVisible(idx, sel.row) ? sel : null;
      break;
    case 'row': {
      const rows = sel.rows.filter((row) => isRowVisible(idx, row));
      visibleSel = rows.length > 0 ? { kind: 'row', rows } : null;
      break;
    }
    default:
      visibleSel = sel;
  }

  const displayRowCount = currentDisplayRows()?.length ?? getPage(props.tabId)?.rowCount ?? 0;
  const colCount = grid.getColumns().length - 1; // minus the gutter
  pendingSelectionKind = visibleSel?.kind === 'column' ? 'column' : null;
  // `setSelectedRanges([])`/`setSelectedRanges(ranges)` both fire `onSelectedRangesChanged`
  // (below), which re-derives `rt().selection` from the ranges just pushed — no need to assign
  // `runtimeEntry.selection` here too, only to push the geometry.
  selectionModel.setSelectedRanges(
    visibleSel
      ? rangesFromSelection(toPositionSelection(visibleSel), displayRowCount, colCount)
      : [],
  );
}

// The selection model owns the geometry, `rt().selection` owns the meaning. Fires on every
// `setSelectedRanges` call: a click, a drag frame (cell mode: only on drag END —
// `handleCellRangeSelected` returns early for `caller === 'onCellRangeSelecting'`),
// shift/ctrl accumulation, keyboard extension, and this host's own header-select-zone push below.
function onSelectedRangesChanged(_e: unknown, ranges: SlickRange[]): void {
  const runtimeEntry = rt();
  if (!runtimeEntry) return;
  selectAllActive.value = false;
  const rowMode = selectionModel?.currentSelectionModeIsRow() ?? false;
  const kind = pendingSelectionKind;
  pendingSelectionKind = null;
  const posSel = selectionFromRanges(ranges, rowMode, kind);
  runtimeEntry.selection = posSel ? toPageRowSelection(posSel) : null;
}

// The header select zone's own click semantics (DataGrid.vue's onHeaderSelectClick, content —
// mirrored verbatim): shift extends a contiguous range from the last anchor, ctrl/cmd toggles one
// column into a disjoint set, a plain click replaces the selection with just that column. Pushes
// the resulting range(s) into the selection model (via the pendingKind flag) so the model and
// rt().selection can never disagree about what's painted.
function onHeaderSelectClick(displayCol: number, e: MouseEvent): void {
  if (!grid || !selectionModel) return;
  const sel = rt()?.selection;
  let cols: number[];
  if (e.shiftKey && colAnchor !== null) {
    const [a, b] = [colAnchor, displayCol].sort((x, y) => x - y);
    cols = [];
    for (let c = a; c <= b; c++) cols.push(c);
  } else if ((e.ctrlKey || e.metaKey) && sel?.kind === 'column') {
    cols = sel.cols.includes(displayCol)
      ? sel.cols.filter((c) => c !== displayCol)
      : [...sel.cols, displayCol];
    colAnchor = displayCol;
  } else {
    cols = [displayCol];
    colAnchor = displayCol;
  }
  // Display row count, not the page's own row count — `setSelectedRanges` below is
  // position-space, and the two only coincide because nothing filters yet (C12).
  const displayRowCount = currentDisplayRows()?.length ?? getPage(props.tabId)?.rowCount ?? 0;
  const colCount = grid.getColumns().length - 1; // minus the gutter
  pendingSelectionKind = 'column';
  selectionModel.setSelectedRanges(
    rangesFromSelection({ kind: 'column', cols }, displayRowCount, colCount),
  );
}

// C8/D8 — the cell menu's own "Edit" item and double-click/Enter (SlickGrid's own default
// `editable: true` handling, unported here) now share one real editor, gated by onBeforeEditCell
// below rather than by `editable: false`'s old no-op.
function startEditCell(row: number, displayCol: number): void {
  if (!grid || !dataSource) return;
  const idx = {
    displayRows: currentDisplayRows(),
    pageRowCount: getPage(props.tabId)?.rowCount ?? 0,
  };
  const pos = displayPositionOf(idx, row);
  grid.setActiveCell(pos, displayCol + 1, false, false, true);
  grid.editActiveCell();
}

// This tab's own `focusRequest.ts` registry entry — registered in onMounted, called
// either immediately by `requestCellFocus` (a host already mounted with a matching page) or from
// the `pageVersion` watch below once a fresh, filtered page lands. Deliberately NOT `startEditCell`
// above: that helper's own `setActiveCell` call suppresses `onActiveCellChanged`, which is fine for
// its existing callers (the cell menu's "Edit" item, always run on a cell an ordinary prior click
// already selected) but wrong here — this is the FIRST selection this brand-new tab's cell ever
// gets, so the event must fire for `SlickHybridSelectionModel.handleActiveCellChange` to populate
// `rt().selection` at all (the publish watch below reads only that; with it suppressed, the cell
// editor dock never appears). `onBeforeEditCell`'s own veto still gates `editActiveCell` below — no
// second editability rule is introduced here.
function applyCellFocusRequest(req: CellFocusRequest): boolean {
  const p = getPage(props.tabId);
  if (!grid || !p || req.row >= p.rowCount) return false;
  const order = currentOrder();
  let displayCol = 0;
  for (let i = 0; i < order.length; i++) {
    const pageCol = pageColumnIndexFor(p, order, i);
    const col = pageCol >= 0 ? p.columns[pageCol] : undefined;
    if (col && !col.isPrimaryKey) {
      displayCol = i;
      break;
    }
  }
  const idx = { displayRows: currentDisplayRows(), pageRowCount: p.rowCount };
  grid.setActiveCell(displayPositionOf(idx, req.row), displayCol + 1, false, false, false);
  if (req.edit) grid.editActiveCell();
  return true;
}

// Exactly `startEdit`'s own guards (DataGrid.vue:810-818 mirror: not writable, row
// deleted, value truncated) plus the two new predicates (the gutter column; a pending-insert row) —
// one function, so this veto and the cell menu's `Edit` item
// (`disabled: editDisabled`, menu.ts) can never drift apart on what's editable.
function onBeforeEditCell(_e: SlickEventData, args: OnBeforeEditCellEventArgs): boolean {
  if (args.column.id === GUTTER_FIELD) return false;
  const item = args.item as RowHandle | undefined;
  if (item?.insertId !== undefined) return false;
  const row = item?.row ?? args.row ?? -1;
  if (!canEditTable() || isDeleted(row)) return false;
  const order = currentOrder();
  const displayCol = order.indexOf(String(args.column.field));
  if (displayCol < 0) return false;
  // F3 (P108 Part 10): the server computes a generated column's value — an UPDATE into one is
  // refused at commit (Postgres/MySQL). The insert paths already skip generated columns
  // (duplicateAsInsert, applyPastedCells, fake data); this existing-row inline-edit path didn't.
  const p = getPage(props.tabId);
  const pageCol = p ? pageColumnIndexFor(p, order, displayCol) : -1;
  if (pageCol >= 0 && p?.columns[pageCol]?.generated) return false;
  // P24 D27: a value the engine truncated is not editable — committing the buffer verbatim
  // (stageEdit's own contract) would write the truncated text over the real value.
  if (displayCell(row, displayCol).truncated) return false;
  return true;
}

// One delegated listener each on the grid root for the insert region's own `<input>`s
// (`cellFormatter`'s insert branch, above), rather than a per-element listener.
// `data-column` already exists on the cell (`cellAttrs` — a per-column
// constant, unrelated to this row being an insert) and `data-insert-id` on the row
// (`tagRenderedRows`), so no extra data attribute needs baking onto the input itself beyond the
// testid this scopes the delegation to.
function insertInputTarget(
  e: Event,
): { insertId: string; column: string; input: HTMLInputElement } | null {
  const input = e.target;
  if (!(input instanceof HTMLInputElement) || input.dataset.testid !== 'grid-cell-insert-input') {
    return null;
  }
  const insertId = input.closest<HTMLElement>('[data-insert-id]')?.dataset.insertId;
  const column = input.closest<HTMLElement>('.slick-cell[data-column]')?.dataset.column;
  return insertId && column ? { insertId, column, input } : null;
}

// The dock stages by page-row index; after a reload that index names a different record.
function dockPageStale(published: ReturnType<typeof getPage>): boolean {
  if (getPage(props.tabId) === published) return false;
  gridViewStore.setActionError(props.tabId, t('studio.grid.editNotStaged.error'));
  return true;
}

function isGeneratedColumn(name: string): boolean {
  return getPage(props.tabId)?.columns.some((c) => c.name === name && c.generated) ?? false;
}

function onInsertGridInput(e: Event): void {
  const hit = insertInputTarget(e);
  if (!hit || isGeneratedColumn(hit.column)) return;
  pendingChangesStore.stageInsertValue(props.tabId, hit.insertId, hit.column, hit.input.value);
}

function onInsertGridKeydown(e: KeyboardEvent): void {
  if (!insertInputTarget(e)) return;
  wrapSelectionOnType(e);
}

// Right-clicking a row already in the selection acts on the whole selection; right-clicking
// outside it replaces the selection with just that row first (the "replace the selection first"
// rule). Cell/header menus have no multi-target actions, so those two always collapse to a
// single-item selection. Pushed via `grid.setActiveCell` on the gutter column, not a direct
// `rangesFromSelection`/`setSelectedRanges` call — landing the active cell on a
// `rowSelectColumnIds` column is what makes `SlickHybridSelectionModel` itself compute and notify
// the single-row range (`handleActiveCellChange`, the `selectActiveRow` branch), which is
// also what keeps `_activeSelectionIsRow` (and so `onSelectedRangesChanged`'s own `rowMode` read)
// correct — calling `setSelectedRanges` directly here, before anything told the model this is a
// *row* push, would leave it reading whatever mode the *previous* selection left behind.
function onGutterContextMenu(row: number, e: MouseEvent): void {
  const p = getPage(props.tabId);
  if (!p || row >= p.rowCount) return; // pending insert rows have no row menu yet (D9/C9)
  const sel = rt()?.selection;
  const inSelection = sel?.kind === 'row' && sel.rows.includes(row);
  if (!inSelection && grid) {
    const idx = { displayRows: currentDisplayRows(), pageRowCount: p.rowCount };
    grid.setActiveCell(displayPositionOf(idx, row), 0, false, false, false);
  }
  const rows = inSelection && sel.kind === 'row' ? sel.rows : [row];
  contextMenuStore.openContextMenu(
    e,
    rowMenu({
      tabId: props.tabId,
      rows,
      qualifiedName: qualifiedName(),
      snapshot: rowSnapshot,
      canEdit: canEditTable(),
      canDelete: canDeleteRows(),
      dialect: currentDialect(),
      selectionKind: 'row',
    }),
  );
}

// `selectionCovers` now lives in shared/slick/selection.ts — moved
// so it can be unit-tested directly and shared verbatim rather than redefined; `setActiveCell`
// below is not cosmetic — SlickHybridSelectionModel.handleActiveCellChange turns *any* active-cell
// change into `setSelectedRanges([one cell])`, wiping whatever was selected, which then leaves the
// toolbar's own copy/delete acting on one cell (or nothing) instead of the rows the user picked.

function onCellContextMenu(row: number, displayCol: number, e: MouseEvent): void {
  const order = currentOrder();
  const priorSel = rt()?.selection;
  const covers = selectionCovers(priorSel, row, displayCol);
  // D8: only when the click lands OUTSIDE the current selection — the "replace the selection
  // first" rule (§5 D7) is about a right-click on something the user has not selected, and was
  // never meant to discard a selection the click is already inside.
  if (grid && dataSource && !covers) {
    const idx = {
      displayRows: currentDisplayRows(),
      pageRowCount: getPage(props.tabId)?.rowCount ?? 0,
    };
    grid.setActiveCell(displayPositionOf(idx, row), displayCol + 1, false, false, false);
  } else if (priorSel && priorSel.kind !== 'cell') {
    // Real-interaction fix (§4a/§4d — "select multiple rows, right-click Copy does not work",
    // "all interactions ... should work the same ... for the selected row all the time"): the
    // click landed *inside* a selection that already covers more than this one cell (a multi-row
    // 'range' from a cell-to-cell drag, a gutter 'row' selection, or a 'column' selection) — this
    // must act on that whole selection via the same rowMenu() the gutter's own right-click and the
    // Delete/Duplicate keyboard shortcuts already build from `rowsForSelection` (the one shared
    // conversion, slick/rowValues.ts), never the single cell under the pointer. Before this fix,
    // onCellContextMenu always opened cellMenu() here regardless of what was actually selected, so
    // right-click Copy on a multi-row range silently copied one cell, and right-click Delete on a
    // column/whole-table selection deleted (at most) one row.
    contextMenuStore.openContextMenu(
      e,
      rowMenu({
        tabId: props.tabId,
        rows: rowsForSelection(priorSel, currentDisplayRows(), getPage(props.tabId)?.rowCount ?? 0),
        qualifiedName: qualifiedName(),
        snapshot: rowSnapshot,
        canEdit: canEditTable(),
        canDelete: canDeleteRows(),
        dialect: currentDialect(),
        selectionKind: priorSel.kind,
      }),
    );
    return;
  }
  const dc = displayCell(row, displayCol);
  // F6 (P108 Part 10): raw (non-masked) values for the filter/FK literals this menu builds —
  // `dc` above (the masking-aware wrapper) stays the source for `text`/`isNull` (Copy).
  const rawDc = rvDisplayCell(props.tabId, getPage(props.tabId), order, row, displayCol);
  const rawSnap = rvRowSnapshot(props.tabId, getPage(props.tabId), order, row);
  const name = order[displayCol] ?? '';
  const t = tab();
  const p = getPage(props.tabId);
  const pageCol = p ? pageColumnIndexFor(p, order, displayCol) : -1;
  contextMenuStore.openContextMenu(
    e,
    cellMenu({
      tabId: props.tabId,
      row,
      columnName: name,
      isNull: dc.isNull,
      text: dc.text,
      copy: (format) => selectionText({ kind: 'cell', row, col: displayCol }, format, copySource()),
      dialect: currentDialect(),
      canEdit: canEditTable(),
      canDelete: canDeleteRows(),
      isDeleted: isDeleted(row),
      isGenerated: pageCol >= 0 ? (p?.columns[pageCol]?.generated ?? false) : false,
      truncated: rawDc.truncated,
      startEdit: () => startEditCell(row, displayCol),
      onPaste: () => void onPaste(),
      meta: rt()?.meta ?? null,
      connectionId: t?.connectionId ?? '',
      rowValues: rawSnap.values,
      numericColumns: numericNavColumns(p, navColumns),
    }),
  );
}

// Unlike a body-cell/gutter right-click, which follows the deliberate
// "replace the selection first" rule, a header right-click has never been specified to do that —
// headerMenu()'s own items (sort/hide/copy) all close over `displayCol` directly and read no live
// selection state, so there was never a functional reason for this to push a range through the
// selection model at all. It used to do so unconditionally on every right-click, wiping out
// whatever the user had selected (rows, a range, other columns) just to paint a highlight
// nobody asked for. Right-click here now only opens the menu.
function onHeaderContextMenuHandler(displayCol: number, e: MouseEvent): void {
  const order = currentOrder();
  const name = order[displayCol] ?? '';
  contextMenuStore.openContextMenu(
    e,
    headerMenu({
      tabId: props.tabId,
      columnName: name,
      currentSort: currentSortTerms().find((t) => t.column === name)?.direction ?? null,
      currentProjection: tab()?.state.projection ?? null,
      allColumnNames: getPage(props.tabId)?.columns.map((c) => c.name) ?? [],
      columnValues: () => columnValuesFor(displayCol),
      // M5 §6.7: the "Mark column as PII" submenu's own inputs.
      connectionId: tab()?.connectionId ?? '',
      tablePath: tab()?.path ?? '',
    }),
  );
}

// F7 — `handleContextMenu` resolves nothing itself and does not prevent the native menu; the
// event args are `{}` (resolve the cell with getCellFromEvent), and a right-click on the cell
// currently being edited is deliberately swallowed by SlickGrid itself before this ever fires.
function onGridContextMenu(e: SlickEventData): void {
  if (!grid || !dataSource) return;
  e.preventDefault();
  const hit = grid.getCellFromEvent(e);
  if (!hit) return;
  const pageRow = dataSource.getItem(hit.row).row;
  const nativeLike = e as unknown as MouseEvent;
  if (hit.cell === 0) onGutterContextMenu(pageRow, nativeLike);
  else onCellContextMenu(pageRow, hit.cell - 1, nativeLike);
}

function onGridHeaderContextMenu(e: SlickEventData, args: { column: KiraColumn }): void {
  if (args.column.id === GUTTER_FIELD) return;
  e.preventDefault();
  const displayCol = currentOrder().indexOf(String(args.column.field));
  if (displayCol < 0) return;
  onHeaderContextMenuHandler(displayCol, e as unknown as MouseEvent);
}

// Keyboard copy and every Copy menu item read through this one source.
function copySource(): CopySource {
  const order = currentOrder();
  return {
    cellAt: displayCell,
    rowSnapshot,
    columnName: (col) => order[col] ?? '',
    rowsInSpan: (r0, r1) => visibleRowsInSpan(currentDisplayRows(), r0, r1),
    allRows: () => rowsForColumnOps(getPage(props.tabId)?.rowCount ?? 0),
    insertTarget: { qualifiedName: qualifiedName(), dialect: currentDialect() },
  };
}

// D1: local, DOM-focus-scoped copy/paste — never a native Electron accelerator.
function onCopy(): void {
  const sel = rt()?.selection;
  if (!sel || !getPage(props.tabId)) return;
  void copyAndAnnounce(
    selectionText(sel, 'tsv', copySource()),
    t('studio.copy.selection.announce'),
  );
}

// D13: TSV-if-tab-else-CSV, applied column-by-column from the selection's anchor across the
// current display column order — existing rows become stageEdit calls, rows past the loaded page
// become pending inserts (reusing one already staged at that row, else a fresh addInsertRow).
async function onPaste(): Promise<void> {
  if (!canEditTable()) return;
  const sel = rt()?.selection;
  const p = getPage(props.tabId);
  if (!sel || !p) return;
  if (sel.kind !== 'cell' && sel.kind !== 'range' && sel.kind !== 'row') return;

  let clipboardText: string;
  try {
    clipboardText = await readClipboardText();
  } catch {
    return;
  }
  if (!clipboardText) return;
  // The readText await can outlast a reload, selection change, mask toggle or tab close; staging
  // by the captured page-row indices would then hit the wrong record.
  if (!grid || getPage(props.tabId) !== p || rt()?.selection !== sel || !canEditTable()) return;

  const parsed = parseDelimited(clipboardText);
  const columns = currentOrder();
  const target = resolvePasteTarget(sel, p, currentDisplayRows(), parsed.length);
  const insertRows = applyPastedCells(props.tabId, p, columns, target, parsed);

  // A paste landing on an *existing* staged insert row updates its `values` in
  // place via `stageInsertValue`, which never changes `inserts.length` and so never reaches the
  // insert-count watch below; that row's own `<input>` would otherwise keep showing its
  // pre-paste text forever. This is exactly the "the change came from outside the grid"
  // case (a paste, not a keystroke into that row's own input), so invalidating it here is safe —
  // nothing about a paste could be holding focus inside the very input being overwritten.
  // `dataSource.setState` covers the one edge case rebuilding wouldn't otherwise reach until the
  // insert-count watch's own next tick: the tab's *first* ever insert, created moments ago by
  // `addInsertRow` above, whose `state.inserts` this closure captured as a plain `[]` before that
  // row's tab-scoped `TabPending` (and its own real, reactive `inserts` array) existed at all.
  if (insertRows.size > 0 && grid && dataSource) {
    dataSource.setState(dataSourceState(p, currentOrder()));
    grid.updateRowCount();
    const idx = { displayRows: currentDisplayRows(), pageRowCount: p.rowCount };
    for (const row of insertRows) grid.invalidateRow(displayPositionOf(idx, row));
    grid.render();
  }
}

// `handleKeyDown` triggers `onKeyDown` first and honours `stopImmediatePropagation`
// (`isImmediatePropagationStopped()`, checked right after this fires): the app's own handler runs
// before SlickGrid's own default key handling and wins by calling it on the branches it
// owns. `enableCellNavigation: true` (above) is what gives the arrow-key/Enter-to-edit path for
// free — DataGrid.vue's own onKeydown arrow block (:1762-1779) has no counterpart here.
function onKeydown(e: SlickEventData): void {
  const runtimeEntry = rt();
  if (!runtimeEntry) return;

  const _key = (e.key ?? '').toLowerCase();
  // The cell editor and insert-row inputs sit under SlickGrid's keydown handler; copy and paste
  // there stay native.
  const native = e.getNativeEvent<KeyboardEvent>();
  const target = native.target;
  const inTextControl =
    target instanceof HTMLInputElement ||
    target instanceof HTMLTextAreaElement ||
    (target instanceof HTMLElement && target.isContentEditable);
  if (shortcutFor(native, ['grid.copy']) !== null && !inTextControl) {
    e.preventDefault();
    e.stopImmediatePropagation();
    onCopy();
    return;
  }
  if (shortcutFor(native, ['grid.paste']) !== null && !inTextControl) {
    e.preventDefault();
    e.stopImmediatePropagation();
    void onPaste();
    return;
  }

  // shortcutFor's own matchesShortcut (keys.ts) reads `.code` for any single-letter chord (F12:
  // Option+letter on macOS composes a different `.key`, so a letter binding matches on the
  // physical `.code` instead) — SlickEventData's constructor copies `key`/`ctrlKey`/`metaKey`/etc.
  // off the wrapped native event onto itself (slick.core.ts), but `code` is not in that copied
  // list, so a plain field reinterpretation of `e` leaves `.code` `undefined` and every
  // letter-keyed shortcut reached through this path (grid.duplicateRows, the only one — copy/paste
  // above are matched on `.key` directly, never routed through shortcutFor) silently never matches.
  // `getNativeEvent()` is SlickEventData's own accessor for the real, unmodified KeyboardEvent it
  // wraps, `.code` included.
  const nativeLike = e.getNativeEvent<KeyboardEvent>();

  // Dispatched through rowMenu() itself (the same builder the row/gutter context menu
  // will call) so the printed shortcut and the executed action can't drift, and
  // `disabled: !canEdit` is honoured for free — inert on a read-only table without restating that
  // guard here. Duplicate stays row-selection-only (there is no sensible "duplicate this column"
  // or "duplicate this range" — a row is what gets duplicated, and only a gutter/row selection
  // names an unambiguous set of them).
  const duplicateShortcut = shortcutFor(nativeLike, ['grid.duplicateRows']);
  if (duplicateShortcut && runtimeEntry.selection?.kind === 'row') {
    const { rows } = runtimeEntry.selection;
    const ran = runMenuShortcut(
      rowMenu({
        tabId: props.tabId,
        rows,
        qualifiedName: qualifiedName(),
        snapshot: rowSnapshot,
        canEdit: canEditTable(),
        canDelete: canDeleteRows(),
        dialect: currentDialect(),
        selectionKind: 'row',
      }),
      duplicateShortcut,
    );
    if (ran) {
      e.preventDefault();
      e.stopImmediatePropagation();
    }
    return;
  }

  // Delete/Cmd+Backspace used to only recognise a 'cell' or 'range' selection here —
  // a 'row' selection was handled by the block above (fine), but a 'column' selection
  // (click a header) reached neither block, so the shortcut silently did nothing.
  // `rowsForSelection` (slick/rowValues.ts) is the same conversion onGridContextMenu's right-click
  // routing now uses (below) — every Selection kind that exists is covered in exactly one place,
  // so the keyboard shortcut and a right-click Delete on the identical selection can never again
  // disagree about which rows are acted on.
  const deleteShortcut = shortcutFor(nativeLike, ['grid.deleteRows']);
  const sel = runtimeEntry.selection;
  if (deleteShortcut && sel) {
    const rows = rowsForSelection(sel, currentDisplayRows(), getPage(props.tabId)?.rowCount ?? 0);
    if (rows.length === 0) return;
    const ran = runMenuShortcut(
      rowMenu({
        tabId: props.tabId,
        rows,
        qualifiedName: qualifiedName(),
        snapshot: rowSnapshot,
        canEdit: canEditTable(),
        canDelete: canDeleteRows(),
        dialect: currentDialect(),
        selectionKind: sel.kind,
      }),
      deleteShortcut,
    );
    if (ran) {
      e.preventDefault();
      e.stopImmediatePropagation();
    }
  }
}

onMounted(() => {
  const el = rootRef.value;
  if (!el) return;
  gridRootEl = el;

  // M5 §6.2: the proactive load itself now lives in `maskRulesQuery` above (enabled the instant
  // this tab has a connectionId) — nothing to do here.

  const t = tab();
  const p = getPage(props.tabId);
  const order = p ? resolveColumnOrder(p, t?.state.columnOrder ?? null) : [];
  lastAppliedPage = p;
  lastAppliedOrderKey = order.join('\u0000');
  formatterCtx.rowNumberBase = t ? t.state.pageIndex * t.state.pageSize : 0;
  // C11/§5 D11c — meta may already be loaded by mount time (a reopened tab); the `rt()?.meta`
  // watch (below) only fires on a *change*, so the first value needs setting here too.
  navColumns = navColumnsFor(rt()?.meta ?? null);

  // This component remounts on every tab switch (`:key="activeTab.id"` in
  // MainView.vue), which resets maskRulesByColumn/maskTagCache/maskTransform — all component-
  // scoped `let`s above — back to empty. Without this call, the dataSourceState() built just below
  // would silently bake in `undefined` for activeMaskTransform even when mask preview is already
  // on for this tab, rendering real values while the toggle still reads "on". Cheap in the common
  // remount case: state/maskRules.ts's own in-memory store already has this connection's rules
  // loaded from the earlier mount, so this is a synchronous re-fold, not a re-fetch.
  refreshMaskFolding();

  dataSource = createGridDataSource(dataSourceState(p, order));

  // slickTheme.css's own `.slick-header-column` rule reads this
  // custom property for its `height`/`line-height` — set here (mount) and in the `rowHeight` watch
  // below (density changing later) so the header row tracks the same 28px/22px toggle the body
  // rows do, matching the incumbent's own `.header-row`'s `:style="{height: rowHeight+'px'}"`.
  // Set on `el` itself (the mount SlickGrid builds its own DOM inside) — ordinary CSS custom-
  // property inheritance carries it down to every header column, wherever in that subtree it ends
  // up.
  el.style.setProperty('--kira-header-row-height', `${rowHeight.value}px`);

  // editor.ts's own `formatterCtx`-style reassigned plain object (its own file-level
  // comment says why: a grid-constructed `KiraCellEditor` cannot close over tabId/displayCell
  // itself). Both callbacks re-resolve the display column / read state fresh on every call, so —
  // unlike `dataItemColumnValueExtractor`'s own captured-closure trap noted just above — this
  // assignment is correct for the tab's whole lifetime and needs no pageVersion-watch counterpart.
  editorCtx.readValue = (row, name) => displayCell(row, currentOrder().indexOf(name));
  editorCtx.commit = (row, name, value) =>
    pendingChangesStore.stageEdit(props.tabId, row, name, value);

  // getCellValue's return type is a compatibility shim only (the insurance, never the real
  // render path — dataItemColumnValueExtractor, below, is) so it deliberately returns `unknown`
  // rather than CustomDataView<RowHandle>'s own narrower `T[keyof T]`; the cast reflects that.
  // dataItemColumnValueExtractor is bound to `dataSource.extractValue`, not a locally captured
  // closure — see GridDataSourceState's own comment on why a captured closure went stale after the
  // first page reload (every cell after it kept reading this construction's original, often
  // `page === null`, "everything is NULL" extractor, since this grid option is fixed at
  // construction and a captured closure can't see a later `dataSource.setState` call).
  grid = new KiraSlickGrid(
    el,
    dataSource as CustomDataView<RowHandle>,
    buildColumns(p, order, currentWidths(), settledWidthsFor(p), rt()?.meta ?? null),
    {
      rowHeight: rowHeight.value,
      // tristateMultiColumnSort + multiColumnSort: false is exactly this app's own header
      // click cycle (asc -> desc -> none), replacing DataGrid.vue's own onHeaderClick entirely;
      // numberedMultiColumnSort + sortColNumberInSeparateSpan renders the order badge for the
      // ORDER BY box's own multi-term sort even though a header click alone never produces one.
      tristateMultiColumnSort: true,
      multiColumnSort: false,
      numberedMultiColumnSort: true,
      sortColNumberInSeparateSpan: true,
      // F7 — Sortable.js is never loaded or bundled; this app reorders columns via ColumnsMenu.vue,
      // never by dragging a header, so the default (true, which hard-throws without a global
      // `Sortable`) must be off.
      enableColumnReorder: false,
      // F6 — cell text is untrusted database content; this removes the innerHTML branch entirely.
      enableHtmlRendering: false,
      // F1 — this app measures column widths itself (columns.ts's own canvas-based initialWidths);
      // leaving autosizeColumns() unused also keeps getCellValue off the render path entirely.
      autosizeColsMode: 'LegacyOff',
      // C4/§5 D4 — SlickGrid's own selection-highlight layer now IS the app's own selection
      // paint (replacing Pass A's '' — F2's O(area) hash cost is gated at C6, not avoided here).
      selectedCellCssClass: 'kira-cell-selected',
      // F1 — enables ctrl/shift disjoint *row* selection in SlickHybridSelectionModel's own row
      // branch (handleClick, :539-556) — the grid's own default, stated explicitly.
      multiSelect: true,
      // §5 item 5 — the sticky row-number gutter: SlickGrid's frozen column, whose canvas
      // KiraSlickGrid re-homes into the scroll viewport as one sticky column (P182) rather than one
      // position:sticky box per mounted row (the per-frame cost P22-…-iter2-rendering.md F12 flagged).
      frozenColumn: 0,
      // SlickGrid's own wheel handler quantizes every wheel/trackpad tick to `deltaY * rowHeight`
      // in JS, discarding WebKit's native momentum physics; that's what read on macOS as the fluid
      // trackpad scroll going away. The viewport's already-native `overflow:auto` plus the
      // already-bound native `scroll` listener are sufficient on their own — frozen-pane sync and
      // this host's own velocity/runway logic both already run off native scroll,
      // never off this handler.
      enableMouseWheelScrollHandler: false,
      // Safe, and only safe, now that kiraSlickGrid.ts's getRenderedRange bounds a single
      // render() call's own synchronous cost independent of fling distance. Without the batch cap
      // this would be a regression: SlickGrid's default `_handleScroll` defers to a 10ms-windowed
      // `scrollThrottle.enqueue()` whenever a single frame's delta exceeds one full viewport
      // height (`dy >= this.viewportH`), which at least caps how *often* the old unbounded batch
      // ran; forcing every large-delta scroll to call render() immediately, before the batch cap
      // existed, would have run that same unbounded batch on every such frame instead.
      //
      // Real-hardware update: unconditional `true` coupled main-thread render work to *every*
      // native scroll-event tick during a fling, which real-macOS testing found produces visible
      // stutter (the motion itself hitching) — a less forgivable failure than the incumbent
      // tanstack grid's own "content lags, motion stays smooth" gap symptom.
      // Defaulted back to `false` (the batch cap alone) pending a real A/B on whether
      // forceSyncScrolling is the cause; kept overridable from the console (see main.ts's own doc
      // comment on `forceSyncScrollingOverride`) so both variants can be compared without a rebuild
      // per variant. Read once here, at construction — this option is construction-time only,
      // same as `frozenColumn`/`enableColumnReorder`/etc. above.
      forceSyncScrolling: window.__kiraGridTuning?.forceSyncScrollingOverride ?? false,
      // Display-position-correct arrows for free: the app's own onKeydown no longer needs to juggle
      // displayPositionOf/rowAtDisplayPosition (DataGrid.vue:1762-1779) because the data source
      // already indexes display positions; navigateUp/Down/Left/Right and the Enter -> edit path
      // come from this option, unported.
      enableCellNavigation: true,
      enableAddRow: false,
      // Bound to canEditTable() at construction; the `editable` watch below (after
      // writability-affecting reactive state can change post-mount, e.g. a caps probe resolving)
      // keeps it live via `grid.setOptions`, the same pattern `rowHeight`'s own watch already uses.
      // `isCellPotentiallyEditable`/`makeActiveCellEditable` both read `this._options.editable`
      // fresh on every edit attempt (`slick.grid.ts`) rather than caching it, so `setOptions` alone
      // is sufficient — no `invalidateAllRows`/`render()` call is needed alongside it.
      editable: canEditTable(),
      // F5 — a selected cell never opens its editor just by typing over it; only double-click or
      // Enter does (SlickGrid's own default handling, unported here), matching DataGrid.vue's own
      // `startEdit` trigger set exactly.
      autoEdit: false,
      autoCommitEdit: true,
      asyncEditorLoading: false,
      editorCellNavOnLRKeys: false,
      explicitInitialization: true,
      dataItemColumnValueExtractor: (item: RowHandle, columnDef: KiraColumn) =>
        dataSource?.extractValue(item, String(columnDef.field)),
    },
  );
  attachScrollProbes(grid, scrollVelocityTracker, () => scrollEventSeq);

  selectionModel = createHybridSelectionModel(GUTTER_FIELD);
  grid.setSelectionModel(selectionModel);

  eventHandler = new SlickEventHandler();
  eventHandler.subscribe(grid.onRendered, tagRenderedRows);
  eventHandler.subscribe(grid.onRendered, onGridRendered);
  eventHandler.subscribe(grid.onHeaderCellRendered, onHeaderCellRendered);
  eventHandler.subscribe(grid.onBeforeHeaderCellDestroy, onBeforeHeaderCellDestroy);
  // No grid.onSort subscription any more — see cycleSortFor's own comment: a header body click
  // can no longer reach SlickGrid's native click-to-sort handler at all, so that
  // event never fires from a click, and the header context menu's own sort items already call
  // `setSort` directly without going through it either.
  eventHandler.subscribe(grid.onHeaderClick, onHeaderClick);
  eventHandler.subscribe(grid.onContextMenu, onGridContextMenu);
  eventHandler.subscribe(grid.onHeaderContextMenu, onGridHeaderContextMenu);
  eventHandler.subscribe(grid.onColumnsResized, onColumnsResized);
  eventHandler.subscribe(grid.onKeyDown, onKeydown);
  eventHandler.subscribe(grid.onBeforeEditCell, onBeforeEditCell);
  eventHandler.subscribe(grid.onClick, onGridClick);
  eventHandler.subscribe(selectionModel.onSelectedRangesChanged, onSelectedRangesChanged);
  subscribeRangeSelecting(eventHandler, selectionModel, onCellRangeSelecting);

  // Explicit init: the header build fires onHeaderCellRendered, so it must run after the
  // subscriptions above or the first (only) header set misses its badges and select zones.
  grid.init();

  // P104 §6.4 — both `.slick-header-columns` panes exist in `el` once init ran — `querySelectorAll`
  // (not `querySelector`), see `headerRowEls`' own comment above.
  headerRowEls.value = Array.from(el.querySelectorAll<HTMLElement>('.slick-header-columns'));

  viewportEl = grid.getViewports()[1] ?? grid.getViewports()[0] ?? null;
  if (viewportEl && t) {
    viewportEl.scrollTop = t.state.scrollTop;
    viewportEl.scrollLeft = t.state.scrollLeft;
  }
  if (viewportEl) {
    // Seed the sampler's baseline at mount (after the restored scroll position
    // is applied, before the first render), so the *first* gesture in a tab's life has a `prev`
    // sample to diff against. Without it `prevOffsetT` is still 0 when that gesture's first render
    // pulls, velocity() takes its own `!prevOffsetT` branch, and the exact defect this fix exists
    // for survives on one gesture per tab. `grid.render()` just below pulls the same position and
    // dedupes, so this seeding costs nothing and changes nothing at rest.
    scrollVelocityTracker.seed(viewportEl.scrollTop, performance.now());
  }
  grid.render();
  syncSortIndicators();
  // A tab switch (not a fresh connect) can remount this host onto a tab that already has a
  // completed search from before it was last hidden; searchState isn't cleared until the tab
  // itself closes (registerTabRuntimeCleanup), so this needs the same initial sync `onMounted`
  // already gives every other CSS layer via its own grid.render() call, above.
  refreshSearchLayer();

  // P99 §9.3: not useEventListener, here and below — recordOffsetSample's own comment documents
  // that registration order against SlickGrid's own internal scroll listener is load-bearing for
  // the velocity sampler, and onUnmounted's "Order matters (§6 D3)" sequence removes these in a
  // specific hand-ordered position relative to resizeObserver/eventHandler/grid.destroy(). Declined,
  // named per CLAUDE.md's library rule — same reasoning as resizeObserver above.
  if (viewportEl) {
    scrollTrace.registerGrid(viewportEl, '.grid-canvas-right .slick-row');
    viewportEl.addEventListener('scroll', onViewportScroll, { passive: true });
    viewportEl.addEventListener('scroll', onViewportScrollPersist, { passive: true });
  }

  // The insert region's own delegated listeners, on the grid root rather than the
  // viewport (an insert row's `<input>` needs `keydown` regardless of scroll position, and `el`
  // is stable across a reload the way `viewportEl` — reassigned from `grid.getViewports()` — is
  // not).
  // Not useEventListener — the removal two lines into onUnmounted's own "Order matters" sequence
  // (between the viewportEl scroll cleanup and eventHandler.unsubscribeAll()) is the same
  // hand-ordered teardown the viewportEl listeners above are declined for. Declined, named per
  // CLAUDE.md's library rule.
  el.addEventListener('input', onInsertGridInput);
  el.addEventListener('keydown', onInsertGridKeydown);

  // See `resizeObserver`'s own declaration for why this exists. `el` (`.slick-grid-mount`) is
  // exactly the container SlickGrid measures itself against
  // (`getViewportHeight`/`getViewportWidth`, `resizeCanvas` — slick.grid.ts), so observing it
  // directly, rather than `.grid-area` a level up, needs no extra plumbing to find the right
  // element. `resizeCanvas()` itself is cheap to call unconditionally (a real height/width
  // remeasure plus one render pass) and is guarded by SlickGrid itself (`!this.initialized`
  // returns immediately) — no debounce needed for a ResizeObserver callback, which already
  // coalesces synchronous layout thrash into one notification per frame.
  resizeObserver = new ResizeObserver(() => {
    grid?.resizeCanvas();
  });
  resizeObserver.observe(el);

  // Registers this tab's own focus-request apply — a pending
  // editReferencedRow()-driven request (menu.ts) may already be waiting for this exact tab. A
  // brand-new tab's *first* load is the one `editReferencedRow`'s own `setFilter` call triggers —
  // DataView.vue's own mount-time `load()` is skipped, since `ensureRuntime` (called synchronously
  // inside `setFilter`, well before this component gets a chance to mount) has already created
  // `runtime[tabId]` by the time DataView checks for it. So there is no SECOND page load left to
  // fire the `pageVersion` watch below for a request that was already pending by the time this
  // component mounts — `requestCellFocus` ran before `registerGridHost` could, found no host, and
  // left the request pending. Consuming it here, right after this mount's own synchronous
  // `grid.render()` above, is what closes that gap: the page this mount just rendered is
  // very likely the filtered one the request was waiting for. If it genuinely isn't loaded yet (a
  // real backend slower than this mount), `applyCellFocusRequest` returns false and the request is
  // put right back pending — the `pageVersion` watch then has a real future bump to catch it on.
  registerGridHost(props.tabId, applyCellFocusRequest);
  registerOpenEditFlush(props.tabId, () => grid?.getEditorLock().commitCurrentEdit());
  const pendingFocus = consumeCellFocus(props.tabId);
  if (pendingFocus && !applyCellFocusRequest(pendingFocus) && !getPage(props.tabId)) {
    requestCellFocus(props.tabId, pendingFocus);
  }

  // M7 finding #5 (cont'd): refreshMaskFolding() above fixes the synchronous fold, but a
  // correlating rule's tags still need this connection's own key plus a fresh HMAC pass over the
  // current page — maskTagCache was reset to empty by the remount too. Mirrors the pageVersion
  // watch's own async tail (below) exactly: render synchronously (already done, correct but
  // tag-less, by the `grid.render()` above), then fill tags in once the async pass resolves.
  // refreshMaskTagsAndRerender (finding #11) turns preview back off on a correlation-key failure.
  if (maskPreviewOn()) refreshMaskTagsAndRerender();
});

onUnmounted(() => {
  // Order matters (§6 D3): stop everything that could still fire into a half-torn-down grid before
  // tearing it down.
  // P67 §4.3: an open preview popover has nothing left to anchor to once this host is gone.
  closeFkPreview();
  unregisterGridHost(props.tabId);
  unregisterOpenEditFlush(props.tabId);
  resizeObserver?.disconnect();
  resizeObserver = null;
  persistScroll.cancel();
  if (viewportEl) {
    viewportEl.removeEventListener('scroll', onViewportScroll);
    viewportEl.removeEventListener('scroll', onViewportScrollPersist);
    scrollTrace.unregisterGrid(viewportEl);
  }
  gridRootEl?.removeEventListener('input', onInsertGridInput);
  gridRootEl?.removeEventListener('keydown', onInsertGridKeydown);
  gridRootEl = null;
  headerRowEls.value = [];
  eventHandler?.unsubscribeAll();
  eventHandler = null;
  // `grid.destroy()` never calls `this.selectionModel?.destroy()` itself (only its own
  // registered `_selector` plugin, via the plugins-unregister loop it does run) — its own
  // `_eventHandler.unsubscribeAll()` (grid.onActiveCellChanged/onClick/onKeyDown) needs this
  // explicit call, before the grid it's subscribed against goes away.
  selectionModel?.destroy();
  selectionModel = null;
  // `true` also nulls SlickGrid's own ~60 internal element references; its own destroy()
  // unbinds every listener it registered, unregisters every plugin and cancels any in-flight edit.
  // Column rules live in kiraSlickGrid.ts's shared append-only sheet, so there is no per-grid
  // <style> to leak. The only real risk was ever this app forgetting to call it —
  // this call is the named, gated acceptance item.
  grid?.destroy(true);
  grid = null;
  dataSource = null;
  viewportEl = null;
  // `editorCtx` (editor.ts) is a module-level singleton; this mount's own closures stay assigned to
  // it after teardown otherwise, retained for nothing — bounded (the next grid to mount overwrites
  // both before a `KiraCellEditor` it constructs could ever call either), but reset explicitly
  // rather than leave a stale closure hanging off a singleton once the last data tab closes.
  editorCtx.readValue = () => ({ text: '', isNull: true });
  editorCtx.commit = () => {};
});

watch(
  () => pageVersion.n,
  () => {
    if (!grid || !dataSource) return;
    const p = getPage(props.tabId);
    const t = tab();
    const order = p ? resolveColumnOrder(p, t?.state.columnOrder ?? null) : [];
    // pageVersion counts every tab's loads and closes; rebuilding for one that is not ours would
    // destroy an open inline editor and mis-consume a pending focus request.
    const orderKey = order.join('\u0000');
    if (p === lastAppliedPage && orderKey === lastAppliedOrderKey) return;
    lastAppliedPage = p;
    lastAppliedOrderKey = orderKey;
    formatterCtx.rowNumberBase = t ? t.state.pageIndex * t.state.pageSize : 0;
    dataSource.setState(dataSourceState(p, order));
    grid.setColumns(
      buildColumns(p, order, currentWidths(), settledWidthsFor(p), rt()?.meta ?? null),
    );
    grid.updateRowCount();
    grid.invalidateAllRows();
    // C5 — the rendered band's own numbers can coincidentally match the pre-reload band (a page
    // reload commonly lands back at scrollTop 0), which would fool onGridRendered's own
    // band-change check into skipping a refresh a genuinely new page needs. The sentinel forces it.
    lastCssLayerBand = { start: 0, end: -1 };
    grid.render();
    syncSortIndicators();
    refreshSearchLayer();
    // P67 §5.2: this is exactly when a freshly loaded page is rendered and its columns are built
    // — the point a pending focus request (a host that wasn't registered yet, or one that was but
    // had no matching page at request time) can finally be satisfied.
    const pendingFocus = consumeCellFocus(props.tabId);
    // A page that landed without the referenced row (it was deleted) drops the request, so a later
    // unrelated load cannot consume it.
    if (pendingFocus && !applyCellFocusRequest(pendingFocus) && !getPage(props.tabId)) {
      requestCellFocus(props.tabId, pendingFocus);
    }

    // M5 §6.3: "on every pageVersion bump while it is on" — a freshly loaded page has an entirely
    // different set of distinct values, so the previous tag cache no longer applies. The redaction
    // above already rendered synchronously (correct, tag-less) the instant this watch fired; this
    // fills the tags in a moment later, once the async HMAC pass resolves. refreshMaskTagsAndRerender
    // (finding #11) turns preview back off on a correlation-key failure.
    if (maskPreviewOn()) refreshMaskTagsAndRerender();
  },
);

// The filter *is* the data source: `matchedRows(tabId)` (via `currentDisplayRows()`,
// already what `dataSourceState()`'s own `index.displayRows` reads) tracks both the search
// toolbar's own scan result and its "filter to matches" toggle reactively (both read a reactive
// `searchState`/`searchFilterState` entry internally), so this watch's getter needs nothing of
// its own beyond calling the same function every other read site already uses.
// `getLength`/`getItem` already do the rest (`dataSource.ts`) once the state is swapped in — the
// same three-call shape the `pageVersion` watch above uses for a full reload, minus rebuilding
// columns (a filter never changes which columns exist, only which rows do). Match positions shift
// under a live filter exactly like selection/staged positions do, so `kira-search` needs its own
// explicit refresh here too — unlike those two, it is not band-scoped, so `onGridRendered`'s own
// band-change check (which the `lastCssLayerBand` sentinel forces below) would never catch it.
watch(
  () => currentDisplayRows(),
  () => {
    if (!grid || !dataSource) return;
    const p = getPage(props.tabId);
    dataSource.setState(dataSourceState(p, currentOrder()));
    grid.updateRowCount();
    grid.invalidateAllRows();
    lastCssLayerBand = { start: 0, end: -1 };
    grid.render();
    refreshSearchLayer();
    // Finding 1 (round 2) — the selection model's own display-position ranges go stale the
    // instant the filter renumbers rows; re-push them (or clear) before anything else reads
    // `rt().selection`/the CSS-painted highlight against the new display space.
    refreshSelectionForFilterChange();
  },
);

// `kira-search`'s own trigger: a scan publishing a new/updated result, or goNext/
// goPrev moving the current match. The signature is result identity + current index + pending,
// not the matches themselves — rebuilding the whole hash is `computeSearchHashes`' own job once
// this fires, not this getter's.
watch(
  () => {
    const entry = searchState[props.tabId];
    return entry ? `${entry.matches.length}:${entry.index}:${entry.pending ? 1 : 0}` : '';
  },
  () => refreshSearchLayer(),
);

// An insert row's own count changes independent of `pageVersion` (Add Row, a discard,
// a bulk paste past the loaded page's end, `duplicateAsInsert`), so this needs its own trigger.
// Safe to fully invalidate the touched region unconditionally: `stageInsertValue` (every keystroke
// into an insert's own input) mutates only that insert's `values` in place, never `p.inserts`
// itself, so this watch never fires mid-typing — the only thing that CAN change `inserts.length`
// is a user action outside any insert row's own input (a toolbar click, a menu item), so the
// "never invalidate a focused insert row" rule has nothing to protect against on this path.
let lastInsertCount = 0;
watch(
  () => pendingChangesStore.pendingFor(props.tabId)?.inserts.length ?? 0,
  (count) => {
    if (!grid || !dataSource) return;
    const p = getPage(props.tabId);
    dataSource.setState(dataSourceState(p, currentOrder()));
    grid.updateRowCount();
    const base = currentDisplayRows()?.length ?? p?.rowCount ?? 0;
    const end = base + Math.max(count, lastInsertCount) - 1;
    const lastInsertCountBefore = lastInsertCount;
    lastInsertCount = count;
    for (let pos = base; pos <= end; pos++) grid.invalidateRow(pos);
    grid.render();
    // The insert is staged at the end of the virtual list, past the loaded page — on
    // any page taller than the viewport, "Add a row" previously staged a row nobody could see.
    // Safe on this path specifically, for the reason this watcher's own comment above already
    // gives: inserts.length can only change from a deliberate user action (a toolbar click, a
    // menu item), never from a keystroke inside an insert row, so this can never yank the
    // viewport away from someone who is typing. Guarded on growth only, same as the watchers —
    // a discard shrinks the count and must not scroll.
    if (count > lastInsertCountBefore) grid.scrollRowIntoView(end);
  },
);

subscribeRowHeight(rowHeight, () => grid, rootRef);

// Keeps the grid's own `editable` option live across a writability change that happens
// after mount (a caps probe resolving, a connection flipping read-only). No invalidate/render
// needed alongside it: `isCellPotentiallyEditable`/`makeActiveCellEditable` both read
// `this._options.editable` fresh on every edit attempt, never a cached value.
watch(canEditTableReactive, (editable) => {
  grid?.setOptions({ editable });
});
// Insert inputs are plain DOM the formatter built before the commit started.
watch(
  () => pendingChangesStore.isCommitting(props.tabId) || gridViewStore.isStale(props.tabId),
  (locked) => {
    for (const input of gridRootEl?.querySelectorAll<HTMLInputElement>(
      'input[data-testid="grid-cell-insert-input"]',
    ) ?? []) {
      const column = input.closest<HTMLElement>('.slick-cell[data-column]')?.dataset.column ?? '';
      input.readOnly = locked || isGeneratedColumn(column);
    }
  },
);

// The mask preview toggle itself — `runtime` is a real `reactive()` map (the
// note on `canEditTableReactive`, above, restated: `rt()?.maskPreview` needs no `pageVersion.n`
// read the way plain-`Map`-backed page state does), so this getter re-fires on every toggle.
// `dataSource.setState` swaps the extractor closure; `invalidateAllRows` + `render` are what
// `shared/slick/dataSource.ts:174-183`'s own header comment says `setState` exists to be followed
// by — the page store itself is never touched (a mask preview is a display-layer transform only).
//
// Also watches `maskRulesQuery.data` (this tab's own connection entry in the
// `['maskRules', connectionId]` cache) — `refreshMaskFolding` was previously only ever re-run from
// this same watch, so marking (or un-marking) a column PII via the header menu while preview was
// already on left `maskRulesByColumn` stale: a newly-masked column kept showing real values, a
// newly-unmasked one kept showing bulleted output, despite the header menu's own checkbox and the
// toolbar's "masked" banner both already reflecting the change. `loadMaskRules` writes a fresh
// array to the cache on every write, so watching the query's data ref (rather than deep-watching
// its contents) is enough to catch every upsert/remove for the tab's current connection.
watch([() => rt()?.maskPreview, () => maskRulesQuery.data.value], async () => {
  foldMaskRulesOnly();
  try {
    await refreshMaskTagCache();
  } catch (err) {
    // A correlation-key fetch failure must not leave the preview claiming "on"
    // while the grid quietly never applied it (a fail-open) — turn it back off and surface why,
    // then still fall through to the render below so that toggle-off state reaches the
    // grid. setMaskPreview(false) is a no-op guard-wise (its pending-changes guard only blocks
    // turning preview ON), and reassigning this watch's own `rt().maskPreview` dependency to the
    // same false-vs-already-false value it may already hold means this fires at most once more.
    gridViewStore.setMaskPreview(props.tabId, false);
    gridViewStore.setActionError(props.tabId, errorText(err));
  }
  if (!grid || !dataSource) return;
  const p = getPage(props.tabId);
  dataSource.setState(dataSourceState(p, currentOrder()));
  grid.invalidateAllRows();
  grid.render();
});

// DataGrid.vue's own `selectionTarget()`/publish watch (its own comment: "the cell editor's
// target"), ported: `onSelectedRangesChanged`/`onGridActiveCellChanged` above already
// keep `rt().selection` current for the grid's own visual selection layer, but never told the cell
// editor dock (`state/cellSelection.ts`) what's selected — a gap this migration left open, not a
// deliberate omission (nothing DataGrid.vue-specific in the logic below; it was never
// carried over when this host's own selection wiring was built). `selectionTarget()` reads
// only `rt()?.selection`, which both engines populate identically, so it ports unchanged.
function selectionTarget(): { row: number; col: number } | null {
  const sel = rt()?.selection;
  if (!sel) return null;
  if (sel.kind === 'cell') return { row: sel.row, col: sel.col };
  if (sel.kind === 'range' && sel.anchorRow === sel.row && sel.anchorCol === sel.col) {
    return { row: sel.row, col: sel.col };
  }
  return null;
}
watch(
  // `rt()?.maskPreview` added so toggling the preview republishes the current selection
  // (and its `masked` flag/`value`) even when the selection itself hasn't changed.
  // The pending-delete source below republishes the same way when a row already
  // selected gets marked (or unmarked) for delete without the selection itself moving — the row
  // menu's/toolbar's own stageDelete/discardRowChange never touch `selection` or `pageVersion`, so
  // without this the dock kept offering Save against a row the grid would already refuse it for.
  [
    () => rt()?.selection,
    () => pageVersion.n,
    () => props.tabId,
    () => rt()?.maskPreview,
    () => {
      const t = selectionTarget();
      return t ? isDeleted(t.row) : false;
    },
    () => pendingChangesStore.isCommitting(props.tabId),
    () => gridViewStore.isStale(props.tabId),
    // Staging or discarding the selected cell republishes, so the dock's value and diff base follow it.
    () => {
      const p = getPage(props.tabId);
      const t = selectionTarget();
      if (!p || !t) return undefined;
      const column = p.columns[pageColumnIndexFor(p, currentOrder(), t.col)];
      return column ? pendingChangesStore.stagedValue(props.tabId, t.row, column.name) : undefined;
    },
  ],
  () => {
    const p = getPage(props.tabId);
    const t = tab();
    const target = selectionTarget();
    if (!p || !t || !target || target.row < 0 || target.row >= p.rowCount) {
      cellSelectionStore.clearSelectedCellFor(props.tabId);
      return;
    }
    const order = currentOrder();
    const pageCol = pageColumnIndexFor(p, order, target.col);
    if (pageCol < 0) {
      cellSelectionStore.clearSelectedCellFor(props.tabId);
      return;
    }
    const view = displayCell(target.row, target.col);
    const column = p.columns[pageCol];
    if (!column) {
      cellSelectionStore.clearSelectedCellFor(props.tabId);
      return;
    }
    const targetRow = target.row;
    // M5 §6.5: `displayCell()` (this file's own wrapper, above) already applies the mask
    // transform when the preview is on — `view.text` here is already the masked text, so the
    // panel's own display needs no change beyond the `masked` flag itself.
    const masked =
      !view.isNull && maskPreviewOn() && maskRulesByColumn.has(column.name.toLowerCase());
    const selected: SelectedCell = {
      tabId: props.tabId,
      connectionId: t.connectionId,
      path: t.path,
      columnIndex: pageCol,
      column,
      row: targetRow,
      value: view.isNull ? null : view.text,
      original: masked
        ? undefined
        : pendingChangesStore.stagedOriginal(props.tabId, targetRow, column.name),
      truncated: view.truncated,
      masked,
      hasPrimaryKey: hasPrimaryKey(),
      // F3: the server computes a generated column — readOnlyReasonFor blocks the dock the same
      // way onBeforeEditCell below blocks the grid's own inline edit.
      generated: column.generated,
      pendingDelete: isDeleted(targetRow),
      stale: gridViewStore.isStale(props.tabId),
      // Same eligibility as the grid's own inline (double-click) edit: writable connection,
      // a primary key to identify the row, and the row isn't already staged for delete. Stages into
      // the exact same pending-change set the inline editor already feeds, so the panel's save and
      // the grid's own inline edit can never disagree about a cell's value. `canEditTable()`
      // already folds in `!maskPreviewOn()`, so onEdit/onRevert are undefined for free
      // whenever `masked` above is true — the panel's own read-only lockout and this cell's own
      // masked-ness can never disagree. `!column.generated` too, matching onBeforeEditCell —
      // `isEditable` already blocks this via `readOnlyReasonFor` above, this is defense in depth.
      onEdit:
        canEditTable() && !isDeleted(targetRow) && !column.generated
          ? (newValue: string) => {
              if (dockPageStale(p)) return;
              pendingChangesStore.stageEdit(props.tabId, targetRow, column.name, newValue);
            }
          : undefined,
      onRevert:
        canEditTable() && !isDeleted(targetRow)
          ? () => {
              if (dockPageStale(p)) return;
              pendingChangesStore.discardCellEdit(props.tabId, targetRow, column.name);
            }
          : undefined,
    };
    cellSelectionStore.publishSelectedCell(selected);
  },
  { immediate: true },
);

watch(
  () => settingsStore.appearanceVersion.n,
  () => {
    resetMeasureCtx();
    rebuildAndSetColumns();
    grid?.render();
  },
);

// A columnWidths change that did NOT originate from this host's own resize drag
// (onColumnsResized's echo guard) rebuilds the header/cell width bag; loadMeta (state.ts) resolves
// after the page itself, so the header tooltip/PK-FK badges need their own watch too, independent
// of pageVersion.
// `flush: 'sync'` (not the default 'pre') is load-bearing for the echo guard: `patchDataTabState`
// mutates reactive state synchronously inside onColumnsResized's `suppressWidthEcho = true; ...;
// suppressWidthEcho = false` bracket, but a default-flush watcher runs on Vue's next pre-render
// microtask — strictly after that synchronous reset — so `suppressWidthEcho` always reads back
// `false` by the time this callback runs. That made the guard a no-op: every column-resize drag
// rebuilt the whole grid (setColumns, header/CSS rebuild, resizeCanvas, closing any open inline
// editor) on every echoed width patch. Sync flush runs this callback from inside that same
// Object.assign, while the flag is still `true`.
watch(
  () => tab()?.state.columnWidths,
  () => {
    if (suppressWidthEcho) return;
    rebuildAndSetColumns();
    grid?.render();
  },
  { flush: 'sync' },
);
watch(
  () => rt()?.meta,
  () => {
    // The cheap precheck's own Set membership, recomputed exactly on a meta change
    // and nowhere else (not per cell, not per hover): `navColumnsFor` is a plain function, so this
    // is the one place that needs to call it, not `rebuildAndSetColumns` itself (also called by
    // the appearance/columnWidths watches below, neither of which changes what's navigable).
    navColumns = navColumnsFor(rt()?.meta ?? null);
    rebuildAndSetColumns();
    grid?.render();
  },
);

// Every row this watch has ever seen staged (edited OR marked for delete), so a discard (which
// clears `p.edits`/`p.deletes` entirely, outside any SlickGrid commit path) knows which
// *previously* staged rows must be invalidated back to their real page value/rails, not only the
// newly-staged ones a plain diff against the current map/set would catch.
let lastPendingRows = new Set<number>();

// The `kira-staged` layer's own trigger: a cell staged or un-staged. This extends the same watch
// to `p.deletes` too, since `pendingRowClasses`' rail/strike-through classes need the same
// re-render a staged edit already gets — one watch, not two, since both `p.edits` and `p.deletes`
// land on the same row-invalidation need and `stageDelete` already keeps them mutually exclusive
// per row.
// `pendingFor(tabId)?.edits`/`.deletes` are a reactive Map/Set (pendingChanges.ts's own
// `pendingState`, a Vue `reactive()`), so both track reactively without a reference-identity trap
// the way the TabPending object itself would be (created once per tab, mutated in place, never
// reassigned). The signature must be value-aware, not just row -> staged column count: this same
// watch also drives the `grid.invalidateRow()`/`grid.render()` call below, which repaints the
// cell's *text* (via `dataItemColumnValueExtractor`), not only the
// `kira-staged` CSS layer. Re-editing an already-staged cell with a new value (same column, so
// the key count is unchanged) is a Map `.set()` on an existing key — Vue still re-runs this
// getter, but a count-only signature comes back byte-identical, so `watch()`'s equality check
// skips the callback and the grid keeps showing the previous staged value forever. Keying on
// `JSON.stringify(edit.changes)` instead makes any staged-value change, not just a staged-key
// change, produce a different signature.
watch(
  () => {
    const { edits, deletes } = pendingChangesStore.pendingOnPage(props.tabId);
    let sig = '';
    for (const [row, changes] of edits) sig += `e${row}:${JSON.stringify(changes)};`;
    for (const row of deletes) sig += `d${row};`;
    return sig;
  },
  () => {
    refreshStagedLayer();
    // `dataItemColumnValueExtractor` already merges `stagedValue` over the page, so a
    // committed *edit* renders correctly for free (SlickGrid's own `commitCurrentEdit` calls
    // `updateRow` after `applyValue`, `slick.grid.ts:4136`). Nothing calls that for a *discard*,
    // a delete toggle, or a commit's own reload-free path, though — those all change
    // `pendingRowClasses`' own answer for a row from outside any SlickGrid edit-commit path, so
    // without this the row keeps showing its stale rails/strike-through/text until something else
    // happens to re-render it. Invalidate the union of this row's newly- and previously-staged
    // state (not just the new set — a discard's new set is empty) and re-render.
    if (!grid || !dataSource) return;
    const { edits, deletes } = pendingChangesStore.pendingOnPage(props.tabId);
    const rows = new Set<number>([...edits.keys(), ...deletes]);
    const touched = new Set<number>([...lastPendingRows, ...rows]);
    lastPendingRows = rows;
    if (touched.size === 0) return;
    const idx = {
      displayRows: currentDisplayRows(),
      pageRowCount: getPage(props.tabId)?.rowCount ?? 0,
    };
    for (const row of touched) grid.invalidateRow(displayPositionOf(idx, row));
    grid.render();
  },
);

defineExpose({
  // DataView.vue's own contract (matching DataGrid.vue's identical export): `row` is a *page* row
  // index, `col` a *display* column index. SlickGrid's own scrollCellIntoView wants its own
  // display *position* and a column index that accounts for the frozen gutter occupying slot 0.
  // The real translation a live filter would need: `displayPositionOf` collapses to identity while
  // nothing is filtered (unchanged behaviour for the common case) and only shifts anything once the
  // filter watch can make page row and display position diverge.
  scrollCellIntoView(row: number, col: number): void {
    if (!grid) return;
    const idx = {
      displayRows: currentDisplayRows(),
      pageRowCount: getPage(props.tabId)?.rowCount ?? 0,
    };
    grid.scrollCellIntoView(displayPositionOf(idx, row), col + 1);
  },
});
</script>

<template>
  <!-- The outer div is now the empty-states' own positioning context
       (`.no-rows { position: absolute; inset: 0 }`, slickTheme.css) and `data-testid="data-grid"`'s
       new home; `rootRef` moves to the inner `.slick-grid-mount`, the one div SlickGrid itself ever
       touches, so an empty state overlaying it can never be wiped out by SlickGrid's own DOM writes
       the way a *child* of `rootRef` would be. The rowColoring setting is one
       class toggle on the host root — Vue's own reactivity on this binding (not a watch) is what
       keeps it live, since settingsStore is a reactive store and this is the template's own
       ordinary :class binding. -->
  <div
    class="slick-grid-host relative h-full font-data text-kira-md text-fg"
    data-testid="data-grid"
    :class="{
      'kira-grid--row-coloring': settingsStore.appearance.rowColoring,
      'kira-select-all': selectAllActive,
    }"
  >
    <div ref="rootRef" class="slick-grid-mount h-full w-full"></div>
    <AttributeTooltip :container="headerRowEls" />
    <EmptyState
      v-if="showNoRows"
      class="no-rows absolute inset-0"
      data-testid="grid-no-rows"
      icon="table"
      :title="t('studio.common.noRows.empty')"
    />
    <SearchFilterNotice
      v-else-if="showNoMatchingRows"
      class="no-rows absolute inset-0"
      data-testid="grid-no-matching-rows"
      :filter-key="props.tabId"
      show-all-testid="grid-show-all-rows"
    />
    <FkPreviewPopover
      v-if="fkPreview"
      :x="fkPreview.x"
      :y="fkPreview.y"
      :entry="fkPreview.entry"
      :ctx="fkPreview.ctx"
      :source-tab-id="props.tabId"
      :open-in-new-tab="fkPreview.openInNewTab"
      @close="closeFkPreview"
    />
  </div>
</template>
