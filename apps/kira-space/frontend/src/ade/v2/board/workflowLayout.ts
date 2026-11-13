import dagre from '@dagrejs/dagre';
import type { Workflow, WorkflowStep } from '../wire';
import {
  aggregateEdges,
  END_NODE_ID,
  type Graph,
  type GraphEdge,
  type GraphNode,
  NODE_H,
  NODE_W,
  type Point,
  RANK_SEP,
  resultsOf,
  routeIndex,
  stepNodeId,
} from './workflowGraph';

// The auto-layout of the workflow canvas. Split from `workflowGraph.ts` so dagre loads with the
// graph editor's chunk, not with every page that edits a workflow as data.

const NODE_SEP = 56;
const EDGE_SEP = 24;
const PAD = 20;
const LOOP_LANE = 20;
const SPINE_WEIGHT = 8;

/** The main success path: from the start step, the first `ok` forward result each step; ends at `end`. */
function spineIds(steps: readonly WorkflowStep[]): Set<string> {
  const out = new Set<string>();
  let i = 0;
  while (i >= 0 && i < steps.length) {
    const step = steps[i] as WorkflowStep;
    if (out.has(stepNodeId(step.id))) break;
    out.add(stepNodeId(step.id));
    const r = resultsOf(step).find((x) => x.ok && routeIndex(steps, i, x.next) > i);
    i = r ? routeIndex(steps, i, r.next) : -1;
  }
  if (i === steps.length) out.add(END_NODE_ID);
  return out;
}

/** Lane per loop edge: intervals over the step rows, inner loops nearest the nodes. */
function loopLanes(
  loops: readonly GraphEdge[],
  rowOf: (nodeId: string) => number,
): { lanes: Map<string, number>; count: number } {
  const span = (e: GraphEdge): [number, number] => [rowOf(e.target), rowOf(e.source)];
  const sorted = [...loops].sort((a, b) => {
    const [a0, a1] = span(a);
    const [b0, b1] = span(b);
    return a1 - a0 - (b1 - b0) || a1 - b1;
  });
  const taken: [number, number][][] = [];
  const lanes = new Map<string, number>();
  for (const e of sorted) {
    const [lo, hi] = span(e);
    let k = taken.findIndex((l) => l.every(([a, b]) => hi < a || lo > b));
    if (k < 0) {
      taken.push([]);
      k = taken.length - 1;
    }
    (taken[k] as [number, number][]).push([lo, hi]);
    lanes.set(e.id, k);
  }
  return { lanes, count: taken.length };
}

/**
 * Dagre centres a parent between its children, which bends the main path. Pin each spine node on the
 * start step's column and push the other nodes and edge waypoints of its rank outward to keep their gaps.
 */
function snapSpine(
  node: (id: string) => Point,
  ids: readonly string[],
  spine: ReadonlySet<string>,
  waypoints: Point[],
): void {
  interface Item {
    x: number;
    node: boolean;
    target: { x: number };
  }
  const column = node(ids[0] as string).x;
  const layers = new Map<number, { items: Item[]; pin?: Item }>();
  const layer = (y: number): { items: Item[]; pin?: Item } => {
    const k = Math.round(y * 10);
    const l = layers.get(k) ?? { items: [] };
    layers.set(k, l);
    return l;
  };
  for (const id of ids) {
    const n = node(id);
    const item: Item = { x: n.x, node: true, target: n };
    const l = layer(n.y);
    l.items.push(item);
    if (spine.has(id)) l.pin = item;
  }
  for (const q of waypoints) layer(q.y).items.push({ x: q.x, node: false, target: q });
  const gap = (a: Item, b: Item): number =>
    (a.node ? NODE_W / 2 : 0) +
    (b.node ? NODE_W / 2 : 0) +
    (a.node && b.node ? NODE_SEP : EDGE_SEP);
  for (const { items, pin } of layers.values()) {
    if (!pin) continue;
    items.sort((a, b) => a.x - b.x);
    const at = items.indexOf(pin);
    pin.target.x = column;
    let prev = pin;
    let px = column;
    for (const it of items.slice(at + 1)) {
      px = Math.max(it.x, px + gap(prev, it));
      it.target.x = px;
      prev = it;
    }
    prev = pin;
    px = column;
    for (const it of items.slice(0, at).reverse()) {
      px = Math.min(it.x, px - gap(it, prev));
      it.target.x = px;
      prev = it;
    }
  }
}

/**
 * Steps ranked top to bottom, the main path on one column, loop edges in a left gutter that does not
 * rank; one End node closes the workflow.
 */
export function layoutWorkflow(wf: Workflow): Graph {
  const steps = wf.steps;
  const g = new dagre.graphlib.Graph();
  g.setGraph({
    rankdir: 'TB',
    ranksep: RANK_SEP,
    nodesep: NODE_SEP,
    edgesep: EDGE_SEP,
    marginx: 0,
    marginy: 0,
  });
  g.setDefaultEdgeLabel(() => ({}));
  const ids = [...steps.map((s) => stepNodeId(s.id)), END_NODE_ID];
  for (const id of ids) g.setNode(id, { width: NODE_W, height: NODE_H });
  const spine = spineIds(steps);
  const edges = steps.flatMap((_, i) => aggregateEdges(steps, i));
  for (const e of edges) {
    if (e.loop) continue;
    e.spine = spine.has(e.source) && spine.has(e.target);
    g.setEdge(e.source, e.target, { weight: e.spine ? SPINE_WEIGHT : 1, minlen: 1 });
  }
  dagre.layout(g);

  const forward = edges.filter((e) => !e.loop);
  const route = (e: GraphEdge): Point[] => {
    const src = g.node(e.source);
    const dst = g.node(e.target);
    return (g.edge(e.source, e.target)?.points ?? []).filter(
      (q: Point) => q.y > src.y + NODE_H / 2 + 0.5 && q.y < dst.y - NODE_H / 2 - 0.5,
    );
  };
  const via = new Map(forward.map((e) => [e.id, route(e)]));
  snapSpine(
    (id) => g.node(id),
    ids,
    spine,
    forward.flatMap((e) => via.get(e.id) ?? []),
  );

  const centre = (id: string): Point => {
    const p = g.node(id);
    return { x: p.x, y: p.y };
  };
  const pts = [...via.values()].flat();
  const minX = Math.min(...ids.map((id) => centre(id).x - NODE_W / 2), ...pts.map((p) => p.x));
  const maxX = Math.max(...ids.map((id) => centre(id).x + NODE_W / 2), ...pts.map((p) => p.x));
  const maxY = Math.max(...ids.map((id) => centre(id).y + NODE_H / 2));
  const rowOf = (id: string): number => (id === END_NODE_ID ? steps.length : ids.indexOf(id));
  const { lanes, count } = loopLanes(
    edges.filter((e) => e.loop),
    rowOf,
  );
  const ox = PAD + count * LOOP_LANE - minX;
  const nodes: GraphNode[] = ids.map((id) => {
    const c = centre(id);
    return {
      id,
      kind: id === END_NODE_ID ? 'end' : 'step',
      stepId: id === END_NODE_ID ? undefined : steps[ids.indexOf(id)]?.id,
      x: ox + c.x - NODE_W / 2,
      y: PAD + c.y - NODE_H / 2,
      w: NODE_W,
      h: NODE_H,
    };
  });
  for (const e of edges) {
    if (e.loop) {
      const lane = lanes.get(e.id);
      e.lane = lane;
      e.laneX = lane === undefined ? undefined : PAD + count * LOOP_LANE - (lane + 1) * LOOP_LANE;
    } else {
      const p = via.get(e.id) ?? [];
      if (p.length > 0) e.points = p.map((q: Point) => ({ x: ox + q.x, y: PAD + q.y }));
    }
  }
  return {
    nodes,
    edges,
    width: PAD * 2 + count * LOOP_LANE + (maxX - minX),
    height: PAD * 2 + maxY,
  };
}

/**
 * Every workflow field `layoutWorkflow` reads, as one string: the step order and ids, whether a step
 * is manual (its one implicit result), and each result's id, success flag and route. Two workflows
 * with equal keys lay out identically, so prompt, name and setting edits skip the layout.
 */
export function topologyKey(wf: Workflow): string {
  return JSON.stringify(
    wf.steps.map((s) => [
      s.id,
      s.kind === 'manual' ? 1 : 0,
      s.kind === 'manual' ? [] : s.results.map((r) => [r.id, r.ok ? 1 : 0, r.next]),
    ]),
  );
}
