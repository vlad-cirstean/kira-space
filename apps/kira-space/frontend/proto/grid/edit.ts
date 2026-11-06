import {
  type CellAddress,
  type InlineEditorGrid,
  type InlineEditorOptions,
  type InlineEditorShape,
  InlineInputEditorBase,
} from './cheetahTypes';
import { vetoReason } from './pending';
import type { RowRecord } from './records';
import type { ProtoState } from './state';

/** The library's own class on the inline editor's `<input>`. */
export const EDITOR_CLASS = 'cheetah-grid__inline-input';

/** Cheetah's inline editor with Studio's rules on top: opens on the displayed text (NULL as
 *  empty), remembers it so Escape can restore, and refuses vetoed cells through `readOnly`. */
class ProtoEditor extends InlineInputEditorBase {
  constructor(
    private readonly state: ProtoState,
    private readonly pageCol: number,
    options: InlineEditorOptions,
  ) {
    super(options);
  }

  clone(): InlineEditorShape {
    return createEditor(this.state, this.pageCol);
  }

  onOpenCellInternal(grid: InlineEditorGrid, cell: CellAddress): void {
    grid.doGetCellValue(cell.col, cell.row, (value) => {
      const view = value as { text: string; isNull: boolean };
      this.onInputCellInternal(grid, cell, view.isNull ? '' : view.text);
    });
  }

  onInputCellInternal(grid: InlineEditorGrid, cell: CellAddress, value: string): void {
    grid.doGetCellValue(cell.col, cell.row, (current) => {
      const view = current as { text: string; isNull: boolean };
      this.state.editing = {
        pageRow: this.pendingRow(grid, cell),
        col: this.pageCol,
        original: view.isNull ? '' : view.text,
      };
    });
    super.onInputCellInternal(grid, cell, value);
  }

  private pendingRow(grid: InlineEditorGrid, cell: CellAddress): number {
    const record = (grid as unknown as { getRowRecord(row: number): RowRecord }).getRowRecord(
      cell.row,
    );
    return record.row;
  }
}

export function createEditor(state: ProtoState, pageCol: number): InlineEditorShape {
  return new ProtoEditor(state, pageCol, {
    readOnly: (record) => {
      const reason = vetoReason(state, (record as RowRecord).row, pageCol);
      if (reason) state.vetoReason = reason;
      return reason !== null;
    },
  });
}
