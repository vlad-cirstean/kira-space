import cheetahGrid, { type ListGrid } from 'cheetah-grid';
import { categoryForTypeClass } from '../../src/theme/icons';
import { alignmentFor } from '../../src/views/shared/page/columns';
import { EDITOR_CLASS } from './edit';
import type { KiraGridProtoHook } from './hookTypes';

type HookCellState = ReturnType<KiraGridProtoHook['cellState']>;

import { railOf } from './pending';
import { markOf } from './search';
import { selectedCols, selectedRows } from './selection';
import { HEADER_ROWS, type ProtoState, pageRowOf, recordOf } from './state';

export interface HookDeps {
  grid: ListGrid<unknown>;
  state: ProtoState;
  /** Nav kind of a page column. */
  navOf(pageCol: number): 'fk' | 'pk' | null;
}

const NAV_HIT = 24;

/** Playwright-only. Reached through `if (__KIRA_DEBUG_HOOKS__)` so a release build drops it. */
export function installDebugHook({ grid, state, navOf }: HookDeps): void {
  // cheetah-grid-playwright resolves cells through `window.cheetahGrid`, the same module instance
  // the grid was built from.
  (window as unknown as { cheetahGrid: unknown }).cheetahGrid = cheetahGrid;
  const columnAt = (name: string): number => {
    const index = state.data.columns.findIndex((c) => c.name === name);
    if (index < 0) throw new Error(`no column ${name}`);
    return index;
  };
  const nameOf = (pageCol: number): string => state.data.columns[pageCol]?.name as string;

  const hook: KiraGridProtoHook = {
    recordIndex(pageRow) {
      const at = recordOf(state, pageRow);
      return at < 0 ? null : at;
    },
    pageRow: (recordIndex) => pageRowOf(state, recordIndex),
    cellState(pageRow, column): HookCellState {
      const pageCol = columnAt(column);
      const descriptor = state.data.columns[pageCol];
      if (!descriptor) throw new Error(`no column ${column}`);
      const view = state.viewAt(pageRow, pageCol);
      const nav = navOf(pageCol);
      let navRect: HookCellState['navRect'] = null;
      const record = recordOf(state, pageRow);
      const displayCol = state.order.indexOf(pageCol);
      if (nav && record >= 0 && displayCol >= 0) {
        const origin = grid.getElement().getBoundingClientRect();
        const cell = grid.getCellRelativeRect(displayCol + 1, record + HEADER_ROWS);
        navRect = {
          x: origin.left + cell.left,
          y: origin.top + cell.top,
          width: NAV_HIT,
          height: cell.height,
        };
      }
      return {
        text: view.isNull ? 'NULL' : view.text,
        isNull: view.isNull,
        truncated: view.truncated,
        masked: view.masked === true,
        staged: view.staged === true,
        search: markOf(state, pageRow, pageCol),
        category: categoryForTypeClass(descriptor.typeClass),
        align: alignmentFor(descriptor),
        nav,
        navRect,
      };
    },
    selection() {
      const { sel } = state.selection;
      const rows = selectedRows(sel, state.rowCount()).map((r) => pageRowOf(state, r));
      const columns = selectedCols(sel, state.order.length).map((c) =>
        nameOf(state.order[c] as number),
      );
      const active = sel && 'row' in sel ? sel : null;
      return {
        kind: sel?.kind ?? null,
        rows,
        columns,
        active: active
          ? {
              row: pageRowOf(state, active.row),
              column: nameOf(state.order[active.col] as number),
            }
          : null,
      };
    },
    editor() {
      const input = grid.getElement().querySelector<HTMLInputElement>(`input.${EDITOR_CLASS}`);
      const editing = state.editing;
      return {
        open: input !== null,
        pageRow: input && editing ? editing.pageRow : null,
        column: input && editing ? nameOf(editing.col) : null,
        value: input?.value ?? null,
        vetoReason: state.vetoReason,
      };
    },
    header(column) {
      const pageCol = columnAt(column);
      const at = state.sorts.findIndex((term) => term.col === pageCol);
      const key = navOf(pageCol);
      return {
        sort: at < 0 ? null : (state.sorts[at]?.dir ?? null),
        sortOrder: at < 0 ? null : at + 1,
        key: key === 'pk' ? 'PK' : key === 'fk' ? 'FK' : null,
      };
    },
    rowState(pageRow) {
      const rail = railOf(state, pageRow);
      const record = recordOf(state, pageRow);
      return {
        inserted: rail === 'inserted',
        deleted: rail === 'deleted',
        dirty: state.dirty.has(pageRow),
        hovered: state.hoverRow === pageRow,
        gutterLabel: rail === 'inserted' ? '+' : String(state.rowNumberBase + record + 1),
      };
    },
    insertRowCount: () => state.inserts.length,
    gridCol(column) {
      const at = state.order.indexOf(columnAt(column));
      return at < 0 ? null : at + 1;
    },
  };
  window.__kiraGridProto = hook;
}
