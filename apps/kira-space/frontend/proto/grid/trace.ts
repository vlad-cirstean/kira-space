import type { ListGrid } from 'cheetah-grid';
import { HEADER_ROWS } from './state';

// Late-data trace for the prototype pages. DOM grids can scroll ahead of their mounted
// rows (SlickGrid's uncoveredPx); a canvas grid cannot, so Cheetah's analogue is how far its
// painted content lags the scroll offset (lagPx) and, as pixel truth, how many visible rows hold
// no ink (blankRows). Playwright-only and the HUD's Trace button; compiled out of release builds.

interface TraceFrame {
  t: number;
  scrollTop: number;
  pxPerFrame: number;
  /** Cheetah: |scrollTop - drawn top|. Slick: uncoveredPx, the mounted band vs the viewport. */
  gapPx: number;
  /** Rows without ink in the visible band; null when not sampled. */
  blankRows: number | null;
}

export interface TraceResult {
  frames: TraceFrame[];
}

interface TraceSource {
  scroller(): HTMLElement | null;
  gapPx(scroller: HTMLElement): number;
  blankRows?(): number;
}

export interface ProtoTrace {
  /** `blank` samples blankRows each frame; its pixel readback distorts timing, so frame-cost
   *  passes leave it off. */
  start(options?: { blank?: boolean }): void;
  stop(): TraceResult;
}

declare global {
  interface Window {
    __kiraProtoTrace?: ProtoTrace;
  }
}

export function createTrace(source: TraceSource): ProtoTrace {
  let frames: TraceFrame[] = [];
  let running = false;
  return {
    start(options) {
      const scroller = source.scroller();
      if (!scroller) throw new Error('trace: no scroller');
      frames = [];
      running = true;
      let last = scroller.scrollTop;
      const tick = (t: number): void => {
        if (!running) return;
        const scrollTop = scroller.scrollTop;
        frames.push({
          t,
          scrollTop,
          pxPerFrame: Math.abs(scrollTop - last),
          gapPx: source.gapPx(scroller),
          blankRows: options?.blank && source.blankRows ? source.blankRows() : null,
        });
        last = scrollTop;
        requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
    },
    stop() {
      running = false;
      return { frames };
    },
  };
}

export function cheetahSource(grid: ListGrid<unknown>): TraceSource {
  const root = grid.getElement();
  const scroller = (): HTMLElement | null => root.querySelector<HTMLElement>('.grid-scrollable');
  // Runs right after the grid's own scroll redraw, so it records the offset the canvas holds.
  let drawnTop = 0;
  grid.listen('scroll', () => {
    drawnTop = scroller()?.scrollTop ?? 0;
  });
  return {
    scroller,
    gapPx: (el) => Math.abs(el.scrollTop - drawnTop),
    blankRows() {
      const canvas = root.querySelector<HTMLCanvasElement>('canvas');
      const ctx = canvas?.getContext('2d', { willReadFrequently: true });
      if (!canvas || !ctx) return 0;
      const scale = canvas.width / canvas.clientWidth;
      const first = grid.topRow;
      const last = Math.min(grid.rowCount - 1, first + grid.visibleRowCount);
      let blank = 0;
      for (let row = Math.max(first, HEADER_ROWS); row <= last; row++) {
        // The `id` column's right-aligned digits: a 8 px strip just inside the cell padding.
        const rect = grid.getCellRelativeRect(1, row);
        if (rect.top < 0 || rect.top + rect.height > canvas.clientHeight) continue;
        const x = Math.round((rect.right - 16) * scale);
        const y = Math.round(rect.top * scale);
        const w = Math.round(8 * scale);
        const h = Math.round(rect.height * scale);
        const px = ctx.getImageData(x, y, w, h).data;
        let inked = false;
        for (let i = 4; i < px.length; i += 4) {
          if (px[i] !== px[0] || px[i + 1] !== px[1] || px[i + 2] !== px[2]) {
            inked = true;
            break;
          }
        }
        if (!inked) blank++;
      }
      return blank;
    },
  };
}

const SLICK_VIEWPORT = '.slick-viewport-top.slick-viewport-right';

export function slickSource(root: HTMLElement, rowHeight: number): TraceSource {
  const scroller = (): HTMLElement | null => root.querySelector<HTMLElement>(SLICK_VIEWPORT);
  return {
    scroller,
    gapPx(el) {
      let top = Number.POSITIVE_INFINITY;
      let bottom = 0;
      for (const row of el.querySelectorAll<HTMLElement>('.slick-row')) {
        const at = Number.parseFloat(row.style.top);
        top = Math.min(top, at);
        bottom = Math.max(bottom, at + rowHeight);
      }
      if (!Number.isFinite(top)) return el.clientHeight;
      return Math.max(0, top - el.scrollTop) + Math.max(0, el.scrollTop + el.clientHeight - bottom);
    },
  };
}
