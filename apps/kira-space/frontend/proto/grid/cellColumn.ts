import { categoryForTypeClass } from '../../src/theme/icons';
import { alignmentFor } from '../../src/views/shared/page/columns';
import { type CellContext, ColumnBase, type DrawCellInfo, type Rect } from './cheetahTypes';
import { railOf } from './pending';
import { EDGE_BOTTOM, EDGE_LEFT, EDGE_RIGHT, EDGE_TOP, edgesOf, rowSelected } from './selection';
import { HEADER_ROWS, type ProtoState, type ProtoView, pageRowOf } from './state';
import type { Palette } from './theme';

// Draw geometry, in CSS px. NAV_RESERVE is the left strip a FK/PK glyph owns (C11 T9 asserts the
// button sits within 24 px of the cell's left edge).
const PAD_X = 8;
const NAV_RESERVE = 24;
const ELLIPSIS = '…';

export type NavKind = 'fk' | 'pk';

type CategoryColor = 'number' | 'keyword' | 'control' | null;

const CATEGORY_COLOR: Partial<Record<ReturnType<typeof categoryForTypeClass>, CategoryColor>> = {
  numeric: 'number',
  boolean: 'keyword',
  datetime: 'control',
  string: null,
  other: null,
};

function categoryColor(p: Palette, kind: CategoryColor): string {
  if (kind === 'number') return p.number;
  if (kind === 'keyword') return p.keyword;
  if (kind === 'control') return p.control;
  return p.fg;
}

/** Body cell: plain text, NULL, empty, truncated and masked markers, type colour, right-align,
 *  FK/PK glyph. Backgrounds (selection, search, hover) come from the theme's `cellBg`. */
export class CellColumn extends ColumnBase {
  private readonly colorKind: CategoryColor;
  private readonly right: boolean;

  constructor(
    private readonly state: ProtoState,
    readonly colIndex: number,
    readonly nav: NavKind | null,
  ) {
    super();
    const descriptor = state.data.columns[colIndex];
    if (!descriptor) throw new Error(`no column ${colIndex}`);
    this.colorKind = CATEGORY_COLOR[categoryForTypeClass(descriptor.typeClass)] ?? null;
    this.right = alignmentFor(descriptor) === 'right';
  }

  clone(): CellColumn {
    return new CellColumn(this.state, this.colIndex, this.nav);
  }

  drawInternal(
    value: unknown,
    context: CellContext,
    _style: unknown,
    _helper: unknown,
    _grid: unknown,
    info: DrawCellInfo,
  ): void {
    const state = this.state;
    const p = state.palette;
    const view = value as ProtoView;
    const pageRow = pageRowOf(state, context.row - HEADER_ROWS);
    info.drawCellBase();
    if (view.staged) fillTint(context, p.warn);
    const deleted = state.deleted.size > 0 && state.deleted.has(pageRow);

    let text = view.text;
    let font = p.font;
    let color = categoryColor(p, this.colorKind);
    if (deleted) color = p.fgSubtle;
    if (view.isNull) {
      text = 'NULL';
      font = p.fontItalic;
      color = p.fgSubtle;
    } else if (view.masked) {
      color = p.fgMuted;
    }

    const rect = context.getRect();
    const ctx = context.getContext();
    ctx.save();
    ctx.beginPath();
    ctx.rect(rect.left, rect.top, rect.width, rect.height);
    ctx.clip();
    ctx.font = font;
    ctx.textBaseline = 'middle';
    const midY = rect.top + rect.height / 2;
    const navPad = this.nav && !view.isNull ? NAV_RESERVE : 0;
    ctx.fillStyle = color;
    if (this.right) {
      ctx.textAlign = 'right';
      ctx.fillText(text, rect.right - PAD_X, midY);
    } else {
      ctx.textAlign = 'left';
      const x = rect.left + PAD_X + navPad;
      ctx.fillText(text, x, midY);
      if (view.truncated) {
        ctx.fillStyle = p.fgMuted;
        ctx.fillText(ELLIPSIS, x + ctx.measureText(text).width + 4, midY);
      }
    }
    if (deleted && !view.isNull) {
      const width = ctx.measureText(text).width;
      const x = this.right ? rect.right - PAD_X - width : rect.left + PAD_X + navPad;
      ctx.fillStyle = p.fgSubtle;
      ctx.fillRect(x, midY, width, 1);
    }
    if (navPad > 0) drawNavGlyph(ctx, rect.left + 6, midY, this.nav as NavKind, p.fgMuted);
    const edges = edgesOf(state.selection.sel, context.row - HEADER_ROWS, context.col - 1);
    if (edges !== 0) drawEdges(ctx, rect, edges, p.focus);
    ctx.restore();
  }
}

function fillTint(context: CellContext, color: string): void {
  const rect = context.getRect();
  const ctx = context.getContext();
  ctx.save();
  ctx.globalAlpha = 0.18;
  ctx.fillStyle = color;
  ctx.fillRect(rect.left, rect.top, rect.width, rect.height);
  ctx.restore();
}

function drawEdges(ctx: CanvasRenderingContext2D, rect: Rect, edges: number, color: string): void {
  ctx.strokeStyle = color;
  ctx.lineWidth = 1;
  ctx.beginPath();
  if (edges & EDGE_TOP) {
    ctx.moveTo(rect.left, rect.top + 0.5);
    ctx.lineTo(rect.right, rect.top + 0.5);
  }
  if (edges & EDGE_BOTTOM) {
    ctx.moveTo(rect.left, rect.bottom - 0.5);
    ctx.lineTo(rect.right, rect.bottom - 0.5);
  }
  if (edges & EDGE_LEFT) {
    ctx.moveTo(rect.left + 0.5, rect.top);
    ctx.lineTo(rect.left + 0.5, rect.bottom);
  }
  if (edges & EDGE_RIGHT) {
    ctx.moveTo(rect.right - 1.5, rect.top);
    ctx.lineTo(rect.right - 1.5, rect.bottom);
  }
  ctx.stroke();
}

function drawNavGlyph(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  kind: NavKind,
  color: string,
): void {
  ctx.strokeStyle = color;
  ctx.lineWidth = 1.25;
  ctx.beginPath();
  if (kind === 'fk') {
    ctx.moveTo(x, y);
    ctx.lineTo(x + 9, y);
    ctx.moveTo(x + 6, y - 3);
    ctx.lineTo(x + 9, y);
    ctx.lineTo(x + 6, y + 3);
  } else {
    ctx.moveTo(x + 9, y);
    ctx.lineTo(x, y);
    ctx.moveTo(x + 3, y - 3);
    ctx.lineTo(x, y);
    ctx.lineTo(x + 3, y + 3);
  }
  ctx.stroke();
}

/** Row-number gutter cell. */
export class GutterColumn extends ColumnBase {
  constructor(private readonly state: ProtoState) {
    super();
  }

  clone(): GutterColumn {
    return new GutterColumn(this.state);
  }

  drawInternal(
    value: unknown,
    context: CellContext,
    _style: unknown,
    _helper: unknown,
    _grid: unknown,
    info: DrawCellInfo,
  ): void {
    const p = this.state.palette;
    const selected = rowSelected(this.state.selection.sel, context.row - HEADER_ROWS);
    info.drawCellBg({ bgColor: selected ? p.select : p.bgChrome });
    info.drawCellBorder();
    const rect = context.getRect();
    const ctx = context.getContext();
    ctx.save();
    ctx.beginPath();
    ctx.rect(rect.left, rect.top, rect.width, rect.height);
    ctx.clip();
    ctx.font = p.font;
    ctx.fillStyle = p.fgSubtle;
    ctx.textAlign = 'right';
    ctx.textBaseline = 'middle';
    const pageRow = pageRowOf(this.state, context.row - HEADER_ROWS);
    const rail = railOf(this.state, pageRow);
    ctx.fillText(
      rail === 'inserted' ? '+' : String(value),
      rect.right - PAD_X,
      rect.top + rect.height / 2,
    );
    if (rail) {
      ctx.fillStyle = rail === 'inserted' ? p.ok : rail === 'deleted' ? p.error : p.warn;
      ctx.fillRect(rect.left, rect.top, 3, rect.height);
    }
    ctx.restore();
  }
}
