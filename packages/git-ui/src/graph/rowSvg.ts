/**
 * "The graph column is a column, not an overlay. Its formatter returns one
 * small `<svg>` holding only the slice of the graph that passes through *that row's* height".
 * Pure, no Vue, no library, no DOM query beyond the element construction itself — no
 * component, no lifecycle, no `devicePixelRatio` handling, no resize path, no repaint coalescing
 * and no theme subscription, all of which existed only to keep a second drawing surface aligned
 * with rows it was not part of. What remains is a pure function from a
 * row's data to an element.
 *
 * Split, like the `refBadges.ts`, into a pure planning half (`planEdgePaths`/`planNode`, unit-
 * tested directly in `tests/unit/ui/rowSvg.spec.ts`) and a DOM-construction half
 * (`buildRowSvg`/its private helpers) exercised by the Playwright pass — this repo has no
 * jsdom/happy-dom wired into `bun:test`.
 *
 * `GEOMETRY` itself lives in `./geometry.ts`, one level below both this file and the
 * `columns.ts` — see that file's own doc comment for why.
 */
import { type DecorationRef, UNRESOLVED_ROW } from '@kira/git-core';
import { GEOMETRY } from './geometry.ts';
import type { EdgeSegment } from './layoutStore.ts';
import { laneClass, type NodeKind } from './palette.ts';

const SVG_NS = 'http://www.w3.org/2000/svg';

/**
 * One row's complete input to `buildRowSvg`: the reused `segments` buffer and how many of its
 * entries are valid (`segmentCount`) — mirroring `LayoutStore.segmentsInRow`'s own "write by
 * index, return a count, never reallocate" contract — plus this row's own lane/colour/shape.
 * `lane` is `undefined` for a row whose layout has not arrived yet (text lands before lanes
 * do); `buildRowSvg` then draws an empty, correctly-sized cell rather than guessing a lane.
 */
export interface RowSlice {
  readonly row: number;
  readonly lane: number | undefined;
  readonly color: number;
  readonly laneCount: number;
  readonly nodeKind: NodeKind;
  readonly segments: readonly EdgeSegment[];
  readonly segmentCount: number;
  /** Whether this row is HEAD (or a branch `ref.isHead`, detached-HEAD included) —
   *  `graphColumn.ts`'s `readSlice` computes this the identical way `columns.ts`'s `rowMetadata`
   *  already does for the row-bold indicator (`isHeadDecoration`, below), never a second
   *  heuristic. Drives `planNode`'s own HEAD ring; does not affect `nodeKind`. */
  readonly isHead: boolean;
}

/** `dataContext.decoration.some(...)` — the single source for "is this row a stash", shared with
 *  `columns.ts`'s `rowMetadata` and `refBadges.ts`'s badge, never a second heuristic. */
export function isStashRow(decorations: readonly DecorationRef[]): boolean {
  return decorations.some((ref) => ref.kind === 'stash');
}

/** Lives in this shared module, so `graphColumn.ts`'s `readSlice` and `columns.ts`'s `rowMetadata`
 *  both import the one function rather than each defining their own
 *  — mirroring `isStashRow`'s own already-established crossing of this exact boundary. */
export function isHeadDecoration(ref: DecorationRef): boolean {
  return ref.kind === 'head' || (ref.kind === 'branch' && ref.isHead);
}

/** Rounds to 2 decimal places and drops a trailing `.00` — keeps a row's `d` attribute short
 *  (thousands of these exist on screen at once, one per rendered cell) and keeps test-expected
 *  strings predictable without depending on floating-point's exact last-bit representation. */
function fmt(value: number): string {
  return String(Math.round(value * 100) / 100);
}

/** The centre-x of a lane's own column within the gutter, clamped to the twelfth lane
 *  (`GEOMETRY.maxLanes`) rather than growing the gutter without bound — a fifty-lane repository
 *  is real (the `fan(50, …)` fixture), and letting the graph eat a panel that is short and wide
 *  is the worse failure ("the message column is what the user reads"). */
export function laneX(lane: number): number {
  const clamped = Math.min(lane, GEOMETRY.maxLanes - 1);
  return GEOMETRY.padLeft + clamped * GEOMETRY.laneWidth + GEOMETRY.laneWidth / 2;
}

/**
 * One segment's SVG path-data fragment for this row, in this row's own coordinates (y=0 is the
 * row's top, y=rowHeight is its bottom) — never the edge's full extent, per §5.3's "every segment
 * a row must draw is expressible in that row's own coordinates".
 *
 * P225: the shape comes from the segment's three lanes, never from its kind. `fromLane` is the
 * source node's lane, `runLane` the lane the edge occupies on pass-through rows, `toLane` the
 * target node's lane (`lanes.ts`; convergence patches `toLane` only). Straight edges have all
 * three equal; a branch-out has `runLane === toLane`; a straight-then-converge edge has
 * `fromLane === runLane`; a branch-out that later converges has all three distinct. Cases are
 * decided by comparing `row` against the segment's own `fromRow`/`toRow` (never a second
 * computation of "is this row special" — `LayoutStore.coversRow` already decided this segment
 * belongs to `row` at all):
 * - `row === fromRow`: the commit's own row. A bezier (or, when the lane does not change, an
 *   equivalent straight run) from the node's centre down to the bottom of this row,
 *   `fromLane -> runLane` — "the transition happens entirely within the row" (§5.3). Overdrawn
 *   by `GEOMETRY.overdraw` past the row's bottom only; the top is the node itself, not a row
 *   boundary, so it gets none.
 * - `row === toRow` (and `toRow` is resolved): the parent's own row. From the top of this row
 *   down to the node's centre, `runLane -> toLane` (a bend when a convergence put the node in
 *   another lane) — overdrawn past the top only, for the same reason in reverse.
 * - Otherwise (a pass-through row, or an edge whose `toRow` is still `UNRESOLVED_ROW` — "runs to
 *   the bottom of its row and stops", which for a query bounded to `[0, rowCount)` it already
 *   does): a full-height run in `runLane`, overdrawn at both ends — two adjacent rows' runs must
 *   meet across a fractional `devicePixelRatio` without a hairline seam (§5.3's fifth decision).
 *
 * `nodeCenterY` is the node's own y (on the subject line, see `planNode`); row-boundary values
 * (`-overdraw`, `rowHeight + overdraw`) are the row's top/bottom, not the node.
 */
export function edgeCommand(
  segment: EdgeSegment,
  row: number,
  rowHeight: number,
  nodeCenterY: number,
): string {
  const overdraw = GEOMETRY.overdraw;

  if (row === segment.fromRow) {
    return bendCommand(
      laneX(segment.fromLane),
      laneX(segment.runLane),
      nodeCenterY,
      rowHeight + overdraw,
    );
  }

  const isEnd = segment.toRow !== UNRESOLVED_ROW && row === segment.toRow;
  if (isEnd) {
    return bendCommand(laneX(segment.runLane), laneX(segment.toLane), -overdraw, nodeCenterY);
  }
  const x = laneX(segment.runLane);
  return `M${fmt(x)},${fmt(-overdraw)} V${fmt(rowHeight + overdraw)}`;
}

/** A vertical run at `xFrom` when the lane does not change, else a bezier from `(xFrom, yStart)`
 *  to `(xTo, yEnd)` with the transition spread across the whole span. */
function bendCommand(xFrom: number, xTo: number, yStart: number, yEnd: number): string {
  if (xFrom === xTo) return `M${fmt(xFrom)},${fmt(yStart)} V${fmt(yEnd)}`;
  const midY = (yStart + yEnd) / 2;
  return (
    `M${fmt(xFrom)},${fmt(yStart)} ` +
    `C${fmt(xFrom)},${fmt(midY)} ${fmt(xTo)},${fmt(midY)} ${fmt(xTo)},${fmt(yEnd)}`
  );
}

interface EdgePathPlan {
  readonly color: number;
  readonly d: string;
}

/** "one `<path>` per lane colour present in the row, not one per segment"
 *  — every covering segment's own command (`edgeCommand`) is concatenated into the one path its
 *  colour owns, which is what holds a row to ~4 elements typical rather than one per segment.
 *  Two *different* lanes sharing a colour (legal once `laneCount` exceeds the palette size) are
 *  concatenated into the same path too — same visual result, one fewer element, and nothing reads
 *  lane identity back out of an already-drawn path. */
function planEdgePaths(
  slice: RowSlice,
  rowHeight: number,
  nodeCenterY: number,
): readonly EdgePathPlan[] {
  const commandsByColor = new Map<number, string[]>();
  for (let i = 0; i < slice.segmentCount; i++) {
    const segment = slice.segments[i] as EdgeSegment;
    const command = edgeCommand(segment, slice.row, rowHeight, nodeCenterY);
    const existing = commandsByColor.get(segment.color);
    if (existing) existing.push(command);
    else commandsByColor.set(segment.color, [command]);
  }
  return Array.from(commandsByColor, ([color, commands]) => ({ color, d: commands.join(' ') }));
}

export interface NodeShapePlan {
  readonly cx: number;
  readonly cy: number;
  readonly r: number;
  readonly color: number;
  /** A filled dot (ordinary/merge's inner dot) vs. an unfilled ring (merge's outer ring, the
   *  whole of a stash's shape) — the DOM layer sets `fill: none` on an unfilled shape via inline
   *  style, never via a CSS class, because the lane class itself sets `fill` (to draw the ordinary
   *  dot at all) and only an inline style reliably wins that cascade. */
  readonly filled: boolean;
  readonly dashed: boolean;
  /** The HEAD ring — an unfilled ring in `--kira-focus` (the same token the existing
   *  branch-badge dot already uses), drawn in addition to whichever shapes `nodeKind` itself
   *  already returns (stash/merge precedence untouched). `true` only for this one shape; every
   *  other `NodeShapePlan` this module produces leaves it `undefined`, which `buildNodeElement`
   *  treats identically to `false`. */
  readonly isHeadRing?: boolean;
  /** G-UX (item 1): the HEAD halo — a soft, filled, low-opacity disc behind the checked-out
   *  commit's node, additive alongside `isHeadRing` (both are only ever present together, since
   *  both come from the same `slice.isHead` check in `planNode`). `buildRowSvg` paints every
   *  `isHeadHalo` shape BEFORE the row's own edges, unlike every other shape here (including
   *  `isHeadRing`), which paints on top of them — the one place paint order depends on this flag. */
  readonly isHeadHalo?: boolean;
}

/** The three node shapes: filled circle (ordinary), filled circle plus an unfilled ring (merge —
 *  `store.parentsOf(row).length > 1`), unfilled dashed ring alone (stash — `decorationAt(row)`
 *  carries the `stash` kind). Empty for a row with no layout yet (`slice.lane === undefined`) —
 *  nothing to draw, not a guessed lane.
 *
 *  `cy` is `nodeCenterY`, not `rowHeight / 2`: a badge line makes the row taller without moving the
 *  subject. The caller (`graphColumn.ts`) passes `rowHeight - compactRowHeight / 2`, the subject
 *  line's centre in both regimes. */
export function planNode(slice: RowSlice, nodeCenterY: number): readonly NodeShapePlan[] {
  if (slice.lane === undefined) return [];
  const cx = laneX(slice.lane);
  const cy = nodeCenterY;
  const color = slice.color;

  // Three small stacked dots in the group's lane colour, checked before the HEAD
  // ring/halo logic below — a placeholder is never HEAD (`graphColumn.ts`'s `readSlice` always
  // sets `isHead: false` for a collapsed entry), so this returns before that computation rather
  // than relying on the flag alone to keep it empty.
  if (slice.nodeKind === 'collapsed') {
    const { collapsedDotRadius: r, collapsedDotGap: gap } = GEOMETRY;
    return [
      { cx, cy: cy - gap, r, color, filled: true, dashed: false },
      { cx, cy, r, color, filled: true, dashed: false },
      { cx, cy: cy + gap, r, color, filled: true, dashed: false },
    ];
  }

  // The HEAD ring is additive — appended to whichever shapes this kind already
  // returns, never replacing them. Built once, appended at every return below. Its own
  // `headRingRadius` (not `mergeRadius`, which a merge-at-HEAD's own merge ring already uses at
  // the identical centre) is what keeps a merge commit at HEAD showing both rings distinctly —
  // see `geometry.ts`'s own doc comment.
  const headRing: NodeShapePlan[] = slice.isHead
    ? [
        {
          cx,
          cy,
          r: GEOMETRY.headRingRadius,
          color,
          filled: false,
          dashed: false,
          isHeadRing: true,
        },
      ]
    : [];
  // The halo, same additive shape as headRing above and always present alongside
  // it (both gated on the identical `slice.isHead`) — kept as its own array, not folded into
  // headRing, so `buildRowSvg` can paint it in a different position (under the row's own edges)
  // without needing to inspect each shape's flags to decide where it goes.
  const headHalo: NodeShapePlan[] = slice.isHead
    ? [
        {
          cx,
          cy,
          r: GEOMETRY.headHaloRadius,
          color,
          filled: true,
          dashed: false,
          isHeadHalo: true,
        },
      ]
    : [];

  if (slice.nodeKind === 'stash') {
    return [
      ...headHalo,
      { cx, cy, r: GEOMETRY.nodeRadius, color, filled: false, dashed: true },
      ...headRing,
    ];
  }

  const dot: NodeShapePlan = { cx, cy, r: GEOMETRY.nodeRadius, color, filled: true, dashed: false };
  if (slice.nodeKind === 'merge') {
    const ring: NodeShapePlan = {
      cx,
      cy,
      r: GEOMETRY.mergeRadius,
      color,
      filled: false,
      dashed: false,
    };
    return [...headHalo, dot, ring, ...headRing];
  }
  return [...headHalo, dot, ...headRing];
}

function buildPathElement(plan: EdgePathPlan): SVGPathElement {
  const path = document.createElementNS(SVG_NS, 'path');
  path.setAttribute('class', laneClass(plan.color));
  path.setAttribute('d', plan.d);
  path.setAttribute('stroke-width', String(GEOMETRY.strokeWidth));
  // the lane class sets `fill` too (so a node's dot can use the same class) — any stylesheet rule
  // beats a presentation attribute, so only an inline style reliably makes this a line, not a
  // filled shape auto-closed at its own start/end point.
  path.style.fill = 'none';
  return path;
}

function buildNodeElement(plan: NodeShapePlan): SVGCircleElement {
  const circle = document.createElementNS(SVG_NS, 'circle');
  circle.setAttribute('cx', fmt(plan.cx));
  circle.setAttribute('cy', fmt(plan.cy));
  circle.setAttribute('r', fmt(plan.r));

  // G-UX (item 1): the HEAD halo, like the HEAD ring below, is never lane-coloured —
  // `fill-focus/15` paints it in `--kira-focus` at low opacity.
  if (plan.isHeadHalo) {
    circle.setAttribute('class', 'fill-focus/15');
    return circle;
  }

  // G19 D1: the HEAD ring is never lane-coloured — `stroke-focus` paints
  // it in `--kira-focus`, the same token the existing branch-badge dot already uses, so it
  // never takes `laneClass`, which is about this row's own lane colour.
  if (plan.isHeadRing) {
    circle.setAttribute('class', 'stroke-focus');
    circle.setAttribute('stroke-width', String(GEOMETRY.headRingStrokeWidth));
    circle.style.fill = 'none';
    return circle;
  }

  circle.setAttribute('class', laneClass(plan.color));
  if (!plan.filled) {
    circle.setAttribute('stroke-width', String(GEOMETRY.strokeWidth));
    circle.style.fill = 'none';
  }
  if (plan.dashed) {
    circle.setAttribute('stroke-dasharray', `${GEOMETRY.strokeWidth} ${GEOMETRY.strokeWidth}`);
  }
  return circle;
}

/** Builds one row's `<svg>` — sized to `columnWidth()` (the column's actual,
 *  user-resizable width, `CommitGrid.vue`'s own `widths.value.graph`, not derived from
 *  `slice.laneCount` — a narrowed column must clip its lanes, not overflow into the message
 *  column) and `rowHeight` (this row's own real height, so lanes and rows cannot drift). Lane
 *  x-coordinates
 *  (`laneX`, `planEdgePaths`/`planNode`) are unchanged — only the box around the drawing is the
 *  column's; the `clip-path` utility on `graph-svg` cuts a lane at the column's right edge
 *  (`overflow: hidden` cannot: one non-visible axis forces the other to `auto`) while letting
 *  `GEOMETRY.overdraw`'s vertical bleed through.
 *
 *  `nodeCenterY` is this row's node-anchor y (see `planNode`). */
export function buildRowSvg(
  slice: RowSlice,
  rowHeight: number,
  nodeCenterY: number,
  width: number,
): SVGSVGElement {
  const svg = document.createElementNS(SVG_NS, 'svg');
  svg.setAttribute('data-testid', 'graph-svg');
  svg.setAttribute('class', 'block overflow-visible [clip-path:inset(-2px_0)]');
  svg.setAttribute('width', String(width));
  svg.setAttribute('height', String(rowHeight));
  svg.setAttribute('viewBox', `0 0 ${width} ${rowHeight}`);

  // G-UX (item 1): the HEAD halo paints UNDER the row's own edges (a soft backdrop the edges
  // still read clearly on top of); every other node shape — including the HEAD ring itself —
  // paints on top of the edges, as before.
  const nodes = planNode(slice, nodeCenterY);
  for (const plan of nodes) if (plan.isHeadHalo) svg.appendChild(buildNodeElement(plan));
  for (const plan of planEdgePaths(slice, rowHeight, nodeCenterY))
    svg.appendChild(buildPathElement(plan));
  for (const plan of nodes) if (!plan.isHeadHalo) svg.appendChild(buildNodeElement(plan));

  return svg;
}
