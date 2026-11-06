import type { NavKind } from './cellColumn';
import { type CellContext, type DrawCellInfo, HeaderBase } from './cheetahTypes';
import type { ProtoState } from './state';

const PAD_X = 8;
/** Right strip that cycles sort; the rest of the header selects the column. */
export const SORT_ZONE = 14;
const BADGE_GAP = 4;

/** Header cell: name, PK/FK key badge, sort arrow with the multi-sort order number. */
export class HeaderColumn extends HeaderBase {
  constructor(
    private readonly state: ProtoState,
    readonly pageCol: number,
    readonly key: NavKind | null,
  ) {
    super();
  }

  clone(): HeaderColumn {
    return new HeaderColumn(this.state, this.pageCol, this.key);
  }

  drawInternal(
    value: unknown,
    context: CellContext,
    _style: unknown,
    _helper: unknown,
    _grid: unknown,
    _info: DrawCellInfo,
  ): void {
    const { state } = this;
    const p = state.palette;
    const rect = context.getRect();
    const ctx = context.getContext();
    const midY = rect.top + rect.height / 2;
    const order = state.sorts.findIndex((term) => term.col === this.pageCol);
    const term = order < 0 ? null : state.sorts[order];
    const hovered = state.hoverHeader === this.pageCol;

    ctx.save();
    ctx.beginPath();
    ctx.rect(rect.left, rect.top, rect.width, rect.height);
    ctx.clip();
    ctx.textBaseline = 'middle';

    let right = rect.right - SORT_ZONE;
    if (term || hovered) {
      drawArrow(
        ctx,
        rect.right - SORT_ZONE / 2 - 1,
        midY,
        term?.dir ?? 'asc',
        term ? p.fg : p.fgSubtle,
      );
    }
    if (term && state.sorts.length > 1) {
      ctx.font = p.fontBold;
      ctx.fillStyle = p.fgMuted;
      ctx.textAlign = 'right';
      ctx.fillText(String(order + 1), right, midY);
      right -= ctx.measureText(String(order + 1)).width + BADGE_GAP;
    }
    if (this.key) {
      const label = this.key === 'pk' ? 'PK' : 'FK';
      ctx.font = `600 ${Math.max(9, p.fontSize - 3)}px ${p.font.split('px ')[1] ?? 'monospace'}`;
      ctx.fillStyle = p.fgSubtle;
      ctx.textAlign = 'right';
      ctx.fillText(label, right - BADGE_GAP, midY);
      right -= ctx.measureText(label).width + BADGE_GAP * 2;
    }
    ctx.beginPath();
    ctx.rect(rect.left, rect.top, Math.max(0, right - rect.left), rect.height);
    ctx.clip();
    ctx.font = p.fontBold;
    ctx.fillStyle = p.fg;
    ctx.textAlign = 'left';
    ctx.fillText(String(value), rect.left + PAD_X, midY);
    ctx.restore();
  }
}

function drawArrow(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  dir: 'asc' | 'desc',
  color: string,
): void {
  const half = 4;
  ctx.fillStyle = color;
  ctx.beginPath();
  if (dir === 'asc') {
    ctx.moveTo(x - half, y + 2);
    ctx.lineTo(x + half, y + 2);
    ctx.lineTo(x, y - 3);
  } else {
    ctx.moveTo(x - half, y - 2);
    ctx.lineTo(x + half, y - 2);
    ctx.lineTo(x, y + 3);
  }
  ctx.closePath();
  ctx.fill();
}
