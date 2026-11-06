import type { MaskKind, MaskRule } from '@shared/domain/mask';
import type { ForeignKeyMeta, ObjectMeta } from '@shared/domain/tree';
import { decodePath } from '@shared/domain/tree';
import { type MessageKey, t } from '@theme/i18n';
import { copyAndAnnounce } from '@workbench/behaviour/clipboard';
import type { MenuItem } from '@workbench/state/contextMenu';
import { maskRulesFor, removeMaskRule, upsertMaskRule } from '../../state/maskRules';
import { useTabsStore } from '../../state/tabs';
import type { RowSnapshot } from '../shared/clipboardFormats';
import type { Selection } from '../shared/slick/selection';
import { type CopyFormat, rowsText } from '../shared/slick/selectionCopy';
import { identNeedsQuoting, quoteIdent, quoteLiteral, type SqlDialect } from '../shared/sqlIdent';
import { requestCellFocus } from './focusRequest';
import { usePendingChangesStore } from './pendingChanges';
import { useGridViewStore } from './state';

// The pure half of ColumnsMenu.vue's own close() — "None" seeds `selected` from
// the table/view's primary-key columns, which is empty for any relation with no primary key (every
// Postgres/MySQL view or matview, and any PK-less table). Zero selected columns falls back to null
// ("all columns") rather than a projection selecting nothing, the same fallback
// nextProjectionAfterHidingColumn below uses.
export function nextProjectionFromSelectedColumns(
  selectedColumnNames: readonly string[],
  allColumnNames: readonly string[],
): string[] | null {
  if (selectedColumnNames.length === 0 || selectedColumnNames.length === allColumnNames.length) {
    return null;
  }
  return [...selectedColumnNames];
}

// The pure half of "Hide column" — split out so the boundary case (hiding the
// last remaining column) is covered without exercising setProjection's own real load() pipeline.
// Returns null ("all columns", ResolveProjection's own meaning for it) rather than an empty array
// when nothing would be left selected; every SQL adapter turns a non-nil empty projection into
// `SELECT  FROM ...`, a syntax error that then persists in tabs.state_json across a reload.
export function nextProjectionAfterHidingColumn(
  currentProjection: string[] | null,
  allColumnNames: string[],
  columnName: string,
): string[] | null {
  const current = currentProjection ?? allColumnNames;
  const next = current.filter((c) => c !== columnName);
  return next.length === 0 ? null : next;
}

// Produced locally from the path, never round-tripped to the engine for a string join — the same
// discipline SlickGridHost.vue's own qualifiedName() and project/menus.ts's qualifiedNameFor use.
// Exported — the preview popover needs the referenced table's display name for its header.
const QUALIFIED_KINDS = new Set(['schema', 'table', 'view', 'matview']);
export function qualifiedNameForPath(connectionId: string, path: string): string {
  return decodePath(connectionId, path)
    .segments.filter((s) => QUALIFIED_KINDS.has(s.kind))
    .map((s) => s.name)
    .join('.');
}

const PLAIN_NUMBER_RE = /^-?(?:0|[1-9]\d*)(?:\.\d+)?$/;

export interface FkNavContext {
  connectionId: string;
  dialect: SqlDialect | undefined;
  rowValues: Record<string, string | null>;
  /** Source-row columns the page types as numbers: their values go bare into the filter. */
  numericColumns: ReadonlySet<string>;
}

// Builds "<refCol> = <val> AND ..." for a (possibly composite) FK edge against the
// *target* table's own columns, sourcing values from this row via the edge's own columns. Shared
// by both directions (meta.foreignKeys and meta.referencedBy) since both use the same "my
// columns, their referencedPath/referencedColumns" convention regardless of which populated them.
// Returns null — never an IS NULL clause — when a needed source value is missing or NULL:
// there is no row to jump to, unlike the filter-by-value which treats NULL as a real predicate.
// Exported — the preview popover needs this same filter to scope its own tab-free
// `data.read`, and reuses this builder rather than re-deriving it (one shared builder, the
// "never threaded from the caller" rule applies here too).
export function foreignKeyValueFilter(
  dialect: SqlDialect | undefined,
  columns: string[],
  referencedColumns: string[],
  rowValues: Record<string, string | null>,
  numericColumns: ReadonlySet<string>,
): string | null {
  const parts: string[] = [];
  for (let i = 0; i < columns.length; i++) {
    const source = columns[i] as string;
    const value = rowValues[source];
    if (value === undefined || value === null) return null;
    const ref = referencedColumns[i] as string;
    const column = identNeedsQuoting(dialect, ref) ? quoteIdent(dialect, ref) : ref;
    const bare = numericColumns.has(source) && PLAIN_NUMBER_RE.test(value);
    parts.push(`${column} = ${bare ? value : quoteLiteral(dialect, value)}`);
  }
  return parts.join(' AND ');
}

// Always a *new* tab (§8.5: "spawns a new tab"), pre-filtered to this row's value(s) on the
// target. No-ops if foreignKeyValueFilter can't build a clause (P7 D2).
function navigateForeignKey(entry: ForeignKeyMeta, ctx: FkNavContext): void {
  const filter = foreignKeyValueFilter(
    ctx.dialect,
    entry.columns,
    entry.referencedColumns,
    ctx.rowValues,
    ctx.numericColumns,
  );
  if (filter === null) return;
  const opened = useTabsStore().openDataTab(ctx.connectionId, entry.referencedPath, {
    newTab: true,
  });
  if (!opened) return;
  const tabId = opened.id;
  void useGridViewStore().setFilter(tabId, filter);
}

/** The same jump navigateForeignKey performs, then a request to land the caret in the
 *  related record — the grid tab IS the edit surface (no second editor exists), so "edit the
 *  related record" is "open it there and select a cell in it".
 *
 *  `newTab: true`, same as navigateForeignKey: reusing an existing tab on the same target is
 *  unsafe — a filter change would re-scope the reused tab's page. A new tab cannot do that.
 *
 *  `await setFilter`, then request focus: `setFilter` resolves only after `load()` has called
 *  `setPage`, so by the time the request is made, the page in the store is the *filtered* one —
 *  requesting before the await would risk the target tab's own first, unfiltered mount-load
 *  consuming it and selecting the wrong record. */
async function editReferencedRow(entry: ForeignKeyMeta, ctx: FkNavContext): Promise<void> {
  const filter = foreignKeyValueFilter(
    ctx.dialect,
    entry.columns,
    entry.referencedColumns,
    ctx.rowValues,
    ctx.numericColumns,
  );
  if (filter === null) return;
  const opened = useTabsStore().openDataTab(ctx.connectionId, entry.referencedPath, {
    newTab: true,
  });
  if (!opened) return;
  const tabId = opened.id;
  await useGridViewStore().setFilter(tabId, filter);
  requestCellFocus(tabId, { row: 0, prefer: 'first-non-key', edit: true });
}

// P7 D9: ids derive from the constraint's own name (unique per table, stable across reloads) —
// matches the file's existing saved-filter-${id}/menu-item-${col} convention over a counter.
function fkNavItem(idPrefix: string, entry: ForeignKeyMeta, ctx: FkNavContext): MenuItem {
  const label = qualifiedNameForPath(ctx.connectionId, entry.referencedPath);
  return {
    type: 'item',
    id: `${idPrefix}-${entry.name}`,
    label:
      idPrefix === 'go-to-referenced'
        ? t('studio.grid.menu.goToReferenced.label', { table: label })
        : `${label}.${entry.referencedColumns.join(', ')}`,
    icon: idPrefix === 'go-to-referenced' ? 'arrow-right' : 'references',
    disabled:
      foreignKeyValueFilter(
        ctx.dialect,
        entry.columns,
        entry.referencedColumns,
        ctx.rowValues,
        ctx.numericColumns,
      ) === null,
    run: () => navigateForeignKey(entry, ctx),
  };
}

// P7 D1/D3: one item per outbound FK whose own columns include this cell's column — i.e. this
// cell is part of that FK. Shared verbatim by cellMenu() and the grid cell's own nav button
// (DataGrid.vue) so the two can never disagree about what's navigable for a given cell.
export function foreignKeyNavItems(
  columnName: string,
  meta: ObjectMeta | null,
  ctx: FkNavContext,
): MenuItem[] {
  if (!meta) return [];
  return meta.foreignKeys
    .filter((fk) => fk.columns.includes(columnName))
    .map((fk) => fkNavItem('go-to-referenced', fk, ctx));
}

// P67 §5.1: mirrors fkNavItem above one for one (same disabled predicate, same id convention).
// P89: the only edit entry point for a referenced row — the preview popover dropped its own.
function fkEditItem(entry: ForeignKeyMeta, ctx: FkNavContext): MenuItem {
  const label = qualifiedNameForPath(ctx.connectionId, entry.referencedPath);
  return {
    type: 'item',
    id: `edit-referenced-${entry.name}`,
    label: t('studio.grid.menu.editReferenced.label', { table: label }),
    icon: 'edit',
    disabled:
      foreignKeyValueFilter(
        ctx.dialect,
        entry.columns,
        entry.referencedColumns,
        ctx.rowValues,
        ctx.numericColumns,
      ) === null,
    run: () => void editReferencedRow(entry, ctx),
  };
}

// P67 §5.1: mirrors foreignKeyNavItems one for one (same edges) — spliced into cellMenu's own
// fkItems array right after the nav items (P7 D3's own invariant, now for two actions).
function foreignKeyEditItems(
  columnName: string,
  meta: ObjectMeta | null,
  ctx: FkNavContext,
): MenuItem[] {
  if (!meta) return [];
  return meta.foreignKeys
    .filter((fk) => fk.columns.includes(columnName))
    .map((fk) => fkEditItem(fk, ctx));
}

// Flat "<qualified referencing table>.<col(s)>" items, one per meta.referencedBy entry — shown
// only when columnName is part of the table's primary key. Flat (no submenu wrapper) so the cell
// button's own popup can use it directly; referencedByMenuItems below wraps it for the right-click
// menu.
export function referencedByItems(
  columnName: string,
  meta: ObjectMeta | null,
  ctx: FkNavContext,
): MenuItem[] {
  if (!meta?.primaryKey?.includes(columnName) || meta.referencedBy.length === 0) return [];
  return meta.referencedBy.map((fk) => fkNavItem('referenced-by', fk, ctx));
}

// P7 D3/D4: referencedByItems, wrapped in a "Referenced by ▸" submenu — [] (not a lone separator)
// when empty, so cellMenu() can always splice this in unconditionally.
function referencedByMenuItems(
  columnName: string,
  meta: ObjectMeta | null,
  ctx: FkNavContext,
): MenuItem[] {
  const items = referencedByItems(columnName, meta, ctx);
  if (items.length === 0) return [];
  return [
    {
      type: 'submenu',
      id: 'referenced-by',
      label: t('studio.definition.constraint.type.referencedBy.label'),
      icon: 'references',
      items,
    },
  ];
}

export interface CellMenuContext {
  tabId: string;
  row: number;
  columnName: string;
  /** Display text — the masking-aware wrapper's own value (M5 §6.6: copy follows display). Used
   *  only by Copy with header below; never for the filter or FK literals (F6,
   *  P108 Part 10) — see `rowValues`. */
  isNull: boolean;
  text: string;
  /** What this cell copies as `format`: the same text the keyboard copy writes. */
  copy: (format: CopyFormat) => string;
  dialect: SqlDialect | undefined;
  canEdit: boolean;
  /** P36 D26: separate from canEdit — an engine can offer update without delete (or vice versa). */
  canDelete: boolean;
  isDeleted: boolean;
  /** F3 (P108 Part 10): the server computes a generated column's value — Edit/Set NULL must
   *  refuse it the same way onBeforeEditCell does, not just the insert paths. */
  isGenerated: boolean;
  /** This exact cell's own raw value is truncated (the engine's 64 KiB cap) —
   *  "Filter by this value" would then build a predicate against only the stored prefix, matching
   *  nothing real. A staged, uncommitted edit does NOT get this treatment (interaction.spec.ts's
   *  own "stage NULL, then filter by it" scenario): `rowValues` below already comes from
   *  `displayCell`'s staged-over-real merge (rowValues.ts), so a staged value here is exactly the
   *  value that would land on commit, unmasked and precise — not an unreliable guess. Filtering
   *  by it is a well-defined `IS NULL`/`= <literal>` query; that it may currently match 0 committed
   *  rows is the correct, expected answer, not a symptom of an unknowable raw value. */
  truncated: boolean;
  startEdit: () => void;
  /** P21 D12: DataGrid.vue's own onPaste — an existing, guarded handler this menu had no row for. */
  onPaste: () => void;
  meta: ObjectMeta | null;
  connectionId: string;
  /** F6 (P108 Part 10): raw page values (`cell()`), never the masking-aware display wrapper — a
   *  masked or truncated *display* value must not end up as a filter or FK-join literal, where it
   *  would either match nothing (masked/truncated) or describe the wrong row entirely. */
  rowValues: Record<string, string | null>;
  numericColumns: ReadonlySet<string>;
}

// D4: Copy / Copy with header / Copy as JSON / Paste / Edit / Set NULL / Filter by this value / Go
// to referenced row / Referenced by (P7).
export function cellMenu(ctx: CellMenuContext): MenuItem[] {
  const editDisabled = !ctx.canEdit || ctx.isDeleted || ctx.isGenerated;
  // Built from the raw rowValues, never ctx.text/ctx.isNull (the masked/display pair) — a
  // masked or truncated cell under preview used to compare against text nothing in the table
  // matches; the FK nav button already used raw values (unaffected, still correct) while this
  // menu's own filter/FK items did not.
  const rawValue = ctx.rowValues[ctx.columnName] ?? null;
  const filterExpr =
    rawValue === null
      ? `${quoteIdent(ctx.dialect, ctx.columnName)} IS NULL`
      : `${quoteIdent(ctx.dialect, ctx.columnName)} = ${quoteLiteral(ctx.dialect, rawValue)}`;
  // F6: a truncated cell has no reliable raw value to filter on — refuse rather than build a
  // predicate against only the stored prefix. A staged cell is NOT included here — see the
  // `truncated` field doc above.
  const filterDisabled = ctx.truncated;
  const fkCtx: FkNavContext = {
    connectionId: ctx.connectionId,
    dialect: ctx.dialect,
    rowValues: ctx.rowValues,
    numericColumns: ctx.numericColumns,
  };
  // P7 D4: always-present array, empty sub-arrays disappear on their own — same shape as
  // rowMenu()'s always-present copy-rows submenu below, no extra "anything to show" branch.
  const fkItems = [
    ...foreignKeyNavItems(ctx.columnName, ctx.meta, fkCtx),
    ...foreignKeyEditItems(ctx.columnName, ctx.meta, fkCtx),
    ...referencedByMenuItems(ctx.columnName, ctx.meta, fkCtx),
  ];

  return [
    {
      type: 'item',
      id: 'copy',
      label: t('common.action.copy.label'),
      icon: 'copy',
      // P21 D5: display-only — DataGrid.vue's onKeydown already binds Cmd/Ctrl+C, but its
      // behavior branches on selection kind (cell/range/row/column) in a way one menu row can't
      // express, so this is tagged for its printed key without being dispatched through here.
      shortcut: 'grid.copy',
      run: () => void copyAndAnnounce(ctx.copy('tsv'), t('studio.copy.cell.announce')),
    },
    {
      type: 'item',
      id: 'copy-with-header',
      label: t('studio.copy.withHeader.label'),
      icon: 'copy',
      run: () =>
        void copyAndAnnounce(
          `${ctx.columnName}\n${ctx.isNull ? '' : ctx.text}`,
          t('studio.copy.withHeader.announce'),
        ),
    },
    {
      type: 'item',
      id: 'copy-as-json',
      label: t('common.action.copyAsJson.label'),
      icon: 'copy',
      run: () => void copyAndAnnounce(ctx.copy('json'), t('common.copy.copiedJson.announce')),
    },
    {
      type: 'item',
      id: 'paste',
      label: t('common.action.paste.label'),
      icon: 'clippy',
      disabled: !ctx.canEdit,
      shortcut: 'grid.paste',
      run: () => ctx.onPaste(),
    },
    { type: 'separator' },
    {
      type: 'item',
      id: 'edit',
      label: t('common.action.edit.label'),
      icon: 'edit',
      disabled: editDisabled,
      shortcut: 'grid.edit',
      run: () => ctx.startEdit(),
    },
    {
      type: 'item',
      id: 'set-null',
      label: t('studio.grid.menu.setNull.label'),
      disabled: editDisabled,
      run: () => usePendingChangesStore().stageNull(ctx.tabId, ctx.row, ctx.columnName),
    },
    {
      // Singular "row" (not rowMenu's "row(s)") — a cell selection is one row by
      // construction, so the plural label rowMenu correctly uses for its own multi-row case would
      // be a lie here. Same shortcut/danger/disabled as rowMenu's own Delete row(s) so the printed
      // key and DataGrid.vue's keyboard handler can't drift apart.
      type: 'item',
      id: 'delete-row',
      label: t('studio.grid.deleteRows.label', { count: 1 }),
      icon: 'trash',
      danger: true,
      disabled: !ctx.canDelete,
      shortcut: 'grid.deleteRows',
      run: () => usePendingChangesStore().stageDelete(ctx.tabId, [ctx.row]),
    },
    {
      type: 'item',
      id: 'filter-by-value',
      label: t('studio.grid.menu.filterByValue.label'),
      icon: 'filter',
      disabled: filterDisabled,
      // Replaces (not appends to) the current filter — a deliberate narrowing action, not an
      // accumulating AND-chain (D5).
      run: () => void useGridViewStore().setFilter(ctx.tabId, filterExpr),
    },
    ...(fkItems.length ? [{ type: 'separator' } as const, ...fkItems] : []),
  ];
}

export interface RowMenuContext {
  tabId: string;
  rows: number[]; // the acted-on selection — the clicked row alone if it wasn't already selected (D3)
  qualifiedName: string;
  snapshot: (row: number) => RowSnapshot;
  canEdit: boolean;
  /** P36 D26: separate from canEdit — an engine can offer update without delete (or vice versa). */
  canDelete: boolean;
  // F3/P21 round 1: Copy row(s) ▸ INSERT needs the dialect to quote both identifiers and literals
  // correctly — undefined for a connection with no SQL surface (that submenu item never runs there).
  dialect: SqlDialect | undefined;
  /** Kind of the selection this menu acts on. ⌘C is printed on Copy rows ▸ TSV only for `'row'`,
   *  the one kind where ⌘C copies whole rows. */
  selectionKind?: Selection['kind'];
}

// A1/P21 round 1: memoized once per rowMenu() call so the four Copy row(s) submenu items share one
// pass over ctx.rows — building the menu itself must never pay for a decode only one of those four
// items (if any) will ever ask for.
function snapshotsThunk(ctx: RowMenuContext): () => RowSnapshot[] {
  let cached: RowSnapshot[] | null = null;
  return () => (cached ??= ctx.rows.map(ctx.snapshot));
}

function hasPendingChange(ctx: RowMenuContext): boolean {
  const store = usePendingChangesStore();
  if (store.isCommitting(ctx.tabId)) return false;
  return ctx.rows.some((row) => store.hasRowChange(ctx.tabId, row));
}

// D6: Copy row(s) ▸ TSV/CSV/JSON/INSERT, Duplicate row(s), Revert row(s), Delete row(s) — all act
// on the full row selection.
export function rowMenu(ctx: RowMenuContext): MenuItem[] {
  // Building the menu used to eagerly decode every selected row across every
  // column (ctx.rows.map(ctx.snapshot)) whether or not any of the four items below ever runs —
  // including on every Delete keypress, which routes through this same builder (the "printed
  // shortcut and executed action can't drift" rule) purely to dispatch, never reading snapshots at
  // all. headerMenu's own columnValues already takes this shape (a thunk, invoked inside run());
  // this mirrors it, memoized once so the four Copy row(s) items share a single pass if more than
  // one is ever used.
  const snapshots = snapshotsThunk(ctx);
  return [
    {
      type: 'submenu',
      id: 'copy-rows',
      label: t('studio.copy.rows.label'),
      icon: 'copy',
      items: [
        {
          type: 'item',
          id: 'copy-rows-tsv',
          label: t('studio.copy.format.tsv.label'),
          // Display-only (P21 D5): ⌘C on a row selection writes this exact text.
          shortcut: ctx.selectionKind === 'row' ? 'grid.copy' : undefined,
          run: () =>
            void copyAndAnnounce(rowsText(snapshots(), 'tsv'), t('studio.copy.tsv.announce')),
        },
        {
          type: 'item',
          id: 'copy-rows-csv',
          label: t('studio.copy.format.csv.label'),
          run: () =>
            void copyAndAnnounce(rowsText(snapshots(), 'csv'), t('studio.copy.csv.announce')),
        },
        {
          type: 'item',
          id: 'copy-rows-json',
          label: t('studio.copy.format.json.label'),
          run: () =>
            void copyAndAnnounce(
              rowsText(snapshots(), 'json'),
              t('common.copy.copiedJson.announce'),
            ),
        },
        {
          type: 'item',
          id: 'copy-rows-insert',
          label: t('studio.copy.insert.label'),
          run: () =>
            void copyAndAnnounce(
              rowsText(snapshots(), 'insert', {
                qualifiedName: ctx.qualifiedName,
                dialect: ctx.dialect,
              }),
              t('studio.copy.insert.announce'),
            ),
        },
      ],
    },
    {
      type: 'item',
      id: 'duplicate-row',
      label: t('studio.grid.menu.duplicateRows.label', { count: ctx.rows.length }),
      icon: 'copy',
      disabled: !ctx.canEdit,
      shortcut: 'grid.duplicateRows',
      run: () => {
        const pendingChangesStore = usePendingChangesStore();
        for (const row of ctx.rows) pendingChangesStore.duplicateAsInsert(ctx.tabId, row);
      },
    },
    {
      type: 'item',
      id: 'revert-row',
      label: t('studio.grid.menu.revertRows.label', { count: ctx.rows.length }),
      icon: 'discard',
      // Only offered when at least one row in the selection actually has something staged
      // (an edit and/or a pending delete) — otherwise "Revert" would be a no-op every time.
      disabled: !hasPendingChange(ctx),
      run: () => {
        const pendingChangesStore = usePendingChangesStore();
        for (const row of ctx.rows) pendingChangesStore.discardRowChange(ctx.tabId, row);
      },
    },
    {
      type: 'item',
      id: 'delete-row',
      label: t('studio.grid.deleteRows.label', { count: ctx.rows.length }),
      icon: 'trash',
      danger: true,
      disabled: !ctx.canDelete,
      shortcut: 'grid.deleteRows',
      run: () => usePendingChangesStore().stageDelete(ctx.tabId, ctx.rows),
    },
  ];
}

export interface HeaderMenuContext {
  tabId: string;
  columnName: string;
  currentSort: 'asc' | 'desc' | null;
  currentProjection: string[] | null;
  allColumnNames: string[];
  columnValues: () => string[]; // the loaded page's values only (§8.5's own scope boundary)
  // M5 §6.7: the "Mark column as PII" submenu's own inputs — the grid already holds both at the
  // point this menu opens, so no new metadata path is needed to resolve the qualified table name
  // Upsert wants (qualifiedNameForPath, above, does that).
  connectionId: string;
  tablePath: string;
}

// The six kinds offered from the header menu. Deliberately not exhaustive of every field a rule
// carries (keepHint/correlate stay at their sensible defaults here — the Privacy tab is where those
// get tuned) — "mark this column PII" is meant to be one click, not a form.
const MASK_KIND_CHOICES: { kind: MaskKind; labelKey: MessageKey }[] = [
  { kind: 'name', labelKey: 'common.field.name.label' },
  { kind: 'email', labelKey: 'common.field.email.label' },
  { kind: 'text', labelKey: 'common.field.text.label' },
  { kind: 'number', labelKey: 'common.field.number.label' },
  { kind: 'date', labelKey: 'common.field.date.label' },
  { kind: 'redact', labelKey: 'studio.project.connectionDialog.redact.label' },
];

// §2.3: number never correlates (a bucket is many-to-one); date defaults off ("a masked date is a
// poor join key"). Every other kind defaults on — the join-preserving behaviour is the point.
function defaultCorrelateFor(kind: MaskKind): boolean {
  return kind !== 'number' && kind !== 'date';
}

// The header menu's own scoping choice: matches by column name alone (case-insensitive) — so
// "Not PII" and the checked kind reflect whatever rule is masking this column today, even one
// authored in the Privacy tab against a different table_name (or '*'). A rule this menu itself
// writes always uses THIS tab's own qualified table name (qualifiedNameForPath, below) — so
// repeat visits from the same tab's header menu edit the same row rather than accumulating
// duplicates; only a rule already present under a different table_name is left in place by
// "Not PII" here (removing every rule matching a column name across every table would reach
// outside this menu's own tab-scoped intent).
function existingMaskRule(ctx: HeaderMenuContext): MaskRule | undefined {
  const lower = ctx.columnName.toLowerCase();
  const rules = maskRulesFor(ctx.connectionId).filter((r) => r.columnName.toLowerCase() === lower);
  if (rules.length === 0) return undefined;
  const tableName = qualifiedNameForPath(ctx.connectionId, ctx.tablePath) || '*';
  return rules.find((r) => r.tableName === tableName) ?? rules[0];
}

async function markColumnMaskKind(ctx: HeaderMenuContext, kind: MaskKind): Promise<void> {
  const existing = existingMaskRule(ctx);
  const tableName =
    existing?.tableName ?? qualifiedNameForPath(ctx.connectionId, ctx.tablePath) ?? '*';
  await upsertMaskRule(ctx.connectionId, {
    tableName: tableName || '*',
    columnName: ctx.columnName,
    kind,
    keepHint: true,
    correlate: defaultCorrelateFor(kind),
  });
  // "the effect is immediately visible" — toggling preview on for this tab is what makes
  // marking a column PII feel like it did something, right where the user was looking. Blocked
  // only while an edit or insert row is staged (setMaskPreview's own pending-changes guard); the
  // rule itself is still saved either way, so say why the preview did not also turn on rather than
  // leaving it looking like the menu action had no effect.
  if (!useGridViewStore().setMaskPreview(ctx.tabId, true)) {
    useGridViewStore().setActionError(ctx.tabId, t('studio.grid.menu.markedSensitive.error'));
  }
}

async function clearColumnMask(ctx: HeaderMenuContext): Promise<void> {
  const existing = existingMaskRule(ctx);
  if (existing) await removeMaskRule(ctx.connectionId, existing.id);
}

// D7: Sort asc/desc/Clear sort, Hide column/Show all columns, Copy column name/values.
export function headerMenu(ctx: HeaderMenuContext): MenuItem[] {
  // Computed once: the mark-pii submenu below reads it once per mask kind plus once for "Not
  // PII" (7 reads total) — existingMaskRule does a fresh maskRulesFor()/filter()/find() scan
  // each time, and ctx (and the rules it reads) cannot change between them within one menu build.
  const currentMaskRule = existingMaskRule(ctx);
  return [
    {
      type: 'item',
      id: 'sort-asc',
      label: t('studio.grid.menu.sortAsc.label'),
      run: () =>
        void useGridViewStore().setSort(ctx.tabId, {
          kind: 'structured',
          terms: [{ column: ctx.columnName, direction: 'asc' }],
        }),
    },
    {
      type: 'item',
      id: 'sort-desc',
      label: t('studio.grid.menu.sortDesc.label'),
      run: () =>
        void useGridViewStore().setSort(ctx.tabId, {
          kind: 'structured',
          terms: [{ column: ctx.columnName, direction: 'desc' }],
        }),
    },
    {
      type: 'item',
      id: 'clear-sort',
      label: t('studio.grid.menu.clearSort.label'),
      disabled: ctx.currentSort === null,
      run: () => void useGridViewStore().setSort(ctx.tabId, null),
    },
    { type: 'separator' },
    {
      type: 'item',
      id: 'hide-column',
      label: t('studio.grid.menu.hideColumn.label'),
      // Reuses the same setProjection() ColumnsMenu.vue calls — no second, competing
      // "which columns are shown" mechanism. This deliberately does not lock
      // out every primary-key column the way ColumnsMenu.vue's own checkbox does — hiding one
      // column of a *composite* key while the others stay visible is legitimate, tested behaviour
      // (interaction.spec.ts's own composite-PK coverage), and the real bug (walking the
      // projection down to zero columns) is closed below by falling back to null instead, not by
      // forbidding every PK column outright.
      run: () => {
        void useGridViewStore().setProjection(
          ctx.tabId,
          nextProjectionAfterHidingColumn(
            ctx.currentProjection,
            ctx.allColumnNames,
            ctx.columnName,
          ),
        );
      },
    },
    {
      type: 'item',
      id: 'show-all-columns',
      label: t('studio.grid.menu.showAllColumns.label'),
      run: () => void useGridViewStore().setProjection(ctx.tabId, null),
    },
    { type: 'separator' },
    {
      type: 'item',
      id: 'copy-column-name',
      label: t('studio.copy.columnName.label'),
      icon: 'copy',
      run: () => void copyAndAnnounce(ctx.columnName, t('studio.copy.columnName.announce')),
    },
    {
      type: 'item',
      id: 'copy-column-values',
      label: t('studio.copy.columnValues.label'),
      icon: 'copy',
      // Display-only (P21 D5): onHeaderContextMenu sets a `column` selection before this menu
      // opens, and onCopy's final branch already copies that column's loaded values on
      // Cmd/Ctrl+C — a binding that worked before this phase but was never shown anywhere.
      shortcut: 'grid.copy',
      run: () =>
        void copyAndAnnounce(ctx.columnValues().join('\n'), t('studio.copy.columnValues.announce')),
    },
    { type: 'separator' },
    {
      type: 'submenu',
      id: 'mark-pii',
      label: t('studio.grid.menu.markSensitive.label'),
      icon: 'shield',
      items: [
        ...MASK_KIND_CHOICES.map(
          ({ kind, labelKey }): MenuItem => ({
            type: 'item',
            id: `mask-${kind}`,
            label: t(labelKey),
            checked: currentMaskRule?.kind === kind,
            run: () => markColumnMaskKind(ctx, kind),
          }),
        ),
        { type: 'separator' },
        {
          type: 'item',
          id: 'mask-none',
          label: t('studio.grid.menu.notSensitive.label'),
          checked: !currentMaskRule,
          run: () => clearColumnMask(ctx),
        },
      ],
    },
  ];
}
