import { columns, data, headers } from 'cheetah-grid';

// cheetah-grid 2.2.0's `main.d.mts` references `columns_d_exports`, `headers_d_exports` and
// `data_d_exports` without declaring them, so `columns`, `headers` and `data` resolve to `any`.
// These are the minimal shapes this prototype relies on, checked at runtime by the specs.

export interface Rect {
  left: number;
  top: number;
  right: number;
  bottom: number;
  width: number;
  height: number;
}

export interface CellContext {
  readonly col: number;
  readonly row: number;
  getContext(): CanvasRenderingContext2D;
  getRect(): Rect;
}

export interface DrawCellInfo {
  getRecord(): unknown;
  drawCellBase(arg?: { bgColor?: string }): void;
  drawCellBg(arg?: { bgColor?: string }): void;
  drawCellBorder(): void;
}

export interface CellDrawer {
  clone(): CellDrawer;
  drawInternal(
    value: unknown,
    context: CellContext,
    style: unknown,
    helper: unknown,
    grid: unknown,
    info: DrawCellInfo,
  ): void;
}

export interface RecordSource<T> {
  length: number;
  get(index: number): T;
}

export interface CheetahDataSource {
  length: number;
  dispose(): void;
}

export const ColumnBase: new () => CellDrawer = columns.type.Column;
export const HeaderBase: new () => CellDrawer = headers.type.BaseHeader;
export const DataSource: new (source: RecordSource<unknown>) => CheetahDataSource = data.DataSource;

export interface CellAddress {
  col: number;
  row: number;
}

export interface InlineEditorGrid {
  doGetCellValue(col: number, row: number, callback: (value: unknown) => void): boolean;
}

export interface InlineEditorOptions {
  readOnly?: (record: unknown) => boolean;
  classList?: string[];
}

export interface InlineEditorShape {
  onInputCellInternal(grid: InlineEditorGrid, cell: CellAddress, value: string): void;
  onOpenCellInternal(grid: InlineEditorGrid, cell: CellAddress): void;
  clone(): InlineEditorShape;
}

export const InlineInputEditorBase: new (options?: InlineEditorOptions) => InlineEditorShape =
  columns.action.InlineInputEditor;
