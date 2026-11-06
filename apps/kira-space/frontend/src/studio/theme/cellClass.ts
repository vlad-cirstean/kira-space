// Design system's tabular-cell vocabulary (see primitives.css's "tabular body shared by
// grid / kv / stream / console" section) is a fixed set of per-cell states — null value, numeric
// alignment, selection, a staged edit, a foreign-key reference — that DataGrid.vue,
// ConsoleResultGrid.vue and StreamView.vue each computed into a `:class` object ad hoc, using
// their own local flag names. This is the single place that maps a cell's boolean state flags to
// the CSS class names those views already render (kept verbatim — Playwright asserts on several
// of them, e.g. `.pending-edit`, `.cell-null` — so this is a dedupe of the mapping, never a
// rename). A view passes only the flags it has; anything omitted or false is left off the result.
export interface CellClassFlags {
  /** Right-aligned (numeric) column. */
  alignRight?: boolean;
  /** The cell's value is SQL/Redis NULL — mirrors the tabular cell's own null styling. */
  isNull?: boolean;
  /** Part of the current selection (single cell, range, row, or column). */
  selected?: boolean;
  /** Matched by an in-progress search, but not the current match. */
  searchMatch?: boolean;
  /** The current (focused) search match. */
  searchMatchCurrent?: boolean;
  /** Has a staged, uncommitted edit overlaid on the real value. */
  pendingEdit?: boolean;
  /** References another table via a foreign key. */
  fk?: boolean;
  /** Has a hover-revealed navigation affordance (FK jump / referenced-by). */
  hasNav?: boolean;
}

const FLAG_CLASS_NAMES: { [K in keyof CellClassFlags]-?: string } = {
  alignRight: 'align-right',
  isNull: 'null',
  selected: 'selected',
  searchMatch: 'search-match',
  searchMatchCurrent: 'search-match-current',
  pendingEdit: 'pending-edit',
  fk: 'fk',
  hasNav: 'has-nav',
};

/** Builds a `:class` object from a cell's state flags — see {@link CellClassFlags}. */
function cellClass(flags: CellClassFlags): Record<string, boolean> {
  const out: Record<string, boolean> = {};
  for (const key of Object.keys(FLAG_CLASS_NAMES) as (keyof CellClassFlags)[]) {
    if (flags[key]) out[FLAG_CLASS_NAMES[key]] = true;
  }
  return out;
}

/** {@link cellClass}'s own keys, as a plain array — the shape `setCellCssStyles` (a lone class
 *  name per keyed layer, never a `:class` object) and a search/pending-edit one-off classname
 *  lookup both want. */
export function classesFrom(flags: CellClassFlags): string[] {
  return Object.keys(cellClass(flags));
}
