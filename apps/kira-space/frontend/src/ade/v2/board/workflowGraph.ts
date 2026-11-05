import dagre from '@dagrejs/dagre';
import type { StepKind, StepResult, Workflow, WorkflowStep } from '../wire';
import { defaultRoute } from './stepResults';
import { newStep } from './workflowForm';

// Pure model behind the graph workflow editor: the workflow as nodes and edges, the auto-layout,
// and every edit the canvas and the inspector make. All edits return a new workflow. Routes live in
// `StepResult.next`; a loop edge is a result with `loop`, which the parser sets exactly when
// the target is the step itself or an earlier one. The return limit lives on the step
// (`maxReturns`). A manual step has no results: its one implicit `done` goes to the next step.

type EdgeTone = 'ok' | 'fail' | 'neutral';

export const NODE_W = 256;
export const NODE_H = 104;
export const RANK_SEP = 64;
const NODE_SEP = 56;
const EDGE_SEP = 24;
const PAD = 20;
const LOOP_LANE = 20;
const SPINE_WEIGHT = 8;

const END_NODE_ID = 'end';
const stepNodeId = (stepId: string): string => `step:${stepId}`;
export const resultHandleId = (resultId: string): string => `r:${resultId}`;

export interface GraphNode {
  id: string;
  kind: 'step' | 'end';
  stepId?: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface Point {
  x: number;
  y: number;
}

export interface GraphEdge {
  id: string;
  source: string;
  target: string;
  /** Handle of the first result on the edge; the implicit edge of a manual step has none. */
  sourceHandle?: string;
  stepId?: string;
  results: string[];
  tone: EdgeTone;
  loop: boolean;
  /** On the main success path: drawn straight. */
  spine?: boolean;
  /** Absolute route of a forward edge that skips ranks. */
  points?: Point[];
  /** Loop edge: gutter lane index and its absolute x. */
  lane?: number;
  laneX?: number;
}

export interface Graph {
  nodes: GraphNode[];
  edges: GraphEdge[];
  width: number;
  height: number;
}

const clone = <T>(v: T): T => JSON.parse(JSON.stringify(v)) as T;

const NEXT_RESULT: StepResult = {
  id: 'done',
  ok: true,
  description: '',
  next: 'next',
  loop: false,
};

/** The results that route a step: a manual step has one implicit `done` to the next step. */
const resultsOf = (step: WorkflowStep): readonly StepResult[] =>
  step.kind === 'manual' ? [NEXT_RESULT] : step.results;

/** Index of the step a result goes to; `steps.length` for the end of the workflow, -1 for stop. */
function routeIndex(steps: readonly WorkflowStep[], i: number, next: string): number {
  if (next === 'next') return i + 1;
  if (next === 'end') return steps.length;
  if (next === 'stop') return -1;
  return steps.findIndex((s) => s.id === next);
}

/** The step's results grouped by where they go: one edge per target, toned by the results on it. */
export function aggregateEdges(steps: readonly WorkflowStep[], i: number): GraphEdge[] {
  const step = steps[i];
  if (!step) return [];
  const manual = step.kind === 'manual';
  const byTarget = new Map<number, StepResult[]>();
  for (const r of resultsOf(step)) {
    const t = routeIndex(steps, i, r.next);
    if (t < 0) continue;
    byTarget.set(t, [...(byTarget.get(t) ?? []), r]);
  }
  return [...byTarget].map(([t, rs]) => {
    const target = steps[t];
    const oks = rs.filter((r) => r.ok).length;
    return {
      id: `e:${step.id}:${t}`,
      source: stepNodeId(step.id),
      target: target ? stepNodeId(target.id) : END_NODE_ID,
      sourceHandle: manual ? undefined : resultHandleId((rs[0] as StepResult).id),
      stepId: step.id,
      results: manual ? [] : rs.map((r) => r.id),
      tone: manual || oks === rs.length ? 'ok' : oks === 0 ? 'fail' : 'neutral',
      loop: t <= i,
    } satisfies GraphEdge;
  });
}

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

/** Every `next` made explicit: `next` becomes the following step's id, or `end` after the last. */
function expand(steps: WorkflowStep[]): void {
  steps.forEach((s, i) => {
    for (const r of s.results) if (r.next === 'next') r.next = steps[i + 1]?.id ?? 'end';
  });
}

function reclassify(steps: WorkflowStep[]): void {
  const at = new Map(steps.map((s, i) => [s.id, i]));
  steps.forEach((s, i) => {
    for (const r of s.results) {
      const t = at.get(r.next);
      r.loop = t !== undefined && t <= i;
    }
  });
}

/** Back to the short form: a route to the following step is `next`; `end` after the last step too. */
function collapse(steps: WorkflowStep[]): void {
  steps.forEach((s, i) => {
    for (const r of s.results) {
      const following = steps[i + 1]?.id;
      if (
        (following !== undefined && r.next === following) ||
        (following === undefined && r.next === 'end')
      )
        r.next = 'next';
    }
  });
}

/** Whether a forward path (`next` chain and forward ids, never a loop) leads from `from` to `to`. */
function reaches(steps: readonly WorkflowStep[], from: string, to: string): boolean {
  const seen = new Set<string>();
  const walk = (id: string): boolean => {
    if (id === to) return true;
    if (seen.has(id)) return false;
    seen.add(id);
    const i = steps.findIndex((s) => s.id === id);
    const s = steps[i];
    if (!s) return false;
    return resultsOf(s).some((r) => {
      if (r.loop || r.next === 'end' || r.next === 'stop') return false;
      const t = r.next === 'next' ? steps[i + 1]?.id : r.next;
      return t !== undefined && walk(t);
    });
  };
  return walk(from);
}

/**
 * Order the steps: the first step stays the start, the rest follow a stable topological order of the
 * forward routes, so a forward route always points later. Loop routes are re-derived from the result.
 */
export function normalizeOrder(steps: readonly WorkflowStep[]): WorkflowStep[] {
  const out = clone([...steps]);
  expand(out);
  const start = out[0];
  if (!start) return out;
  const ids = new Set(out.map((s) => s.id));
  const incoming = new Map<string, Set<string>>(out.map((s) => [s.id, new Set()]));
  for (const s of out)
    for (const r of s.results)
      if (!r.loop && ids.has(r.next) && r.next !== start.id && r.next !== s.id)
        incoming.get(r.next)?.add(s.id);
  const order: WorkflowStep[] = [start];
  const done = new Set([start.id]);
  const rest = out.slice(1);
  while (rest.length > 0) {
    const k = rest.findIndex((s) => [...(incoming.get(s.id) ?? [])].every((p) => done.has(p)));
    const [pick] = rest.splice(Math.max(k, 0), 1) as [WorkflowStep];
    order.push(pick);
    done.add(pick.id);
  }
  reclassify(order);
  collapse(order);
  return order;
}

function mapSteps(wf: Workflow, f: (steps: WorkflowStep[]) => WorkflowStep[]): Workflow {
  return { ...wf, steps: f(clone(wf.steps)) };
}

/**
 * Point a result at `target` (`next`, `end`, `stop` or a step id). A step the target already leads
 * back to makes a loop edge; any other earlier target is moved after the source.
 */
export function setRoute(wf: Workflow, stepId: string, resultId: string, target: string): Workflow {
  return mapSteps(wf, (steps) => {
    const step = steps.find((s) => s.id === stepId);
    const r = step?.results.find((x) => x.id === resultId);
    if (!step || !r) return steps;
    const si = steps.indexOf(step);
    const ti = steps.findIndex((s) => s.id === target);
    r.next = target;
    r.loop = ti >= 0 && ti <= si && (ti === si || reaches(steps, target, stepId));
    return normalizeOrder(steps);
  });
}

/** The route a result falls back to once its edge is removed. */
export function clearRoute(wf: Workflow, stepId: string, resultIds: string[]): Workflow {
  return mapSteps(wf, (steps) => {
    const step = steps.find((s) => s.id === stepId);
    if (!step) return steps;
    for (const r of step.results)
      if (resultIds.includes(r.id)) {
        r.next = defaultRoute(r.ok);
        r.loop = false;
      }
    return normalizeOrder(steps);
  });
}

function freeResultId(results: readonly StepResult[]): string {
  const taken = new Set(results.map((r) => r.id));
  let n = results.length + 1;
  while (taken.has(`result-${n}`)) n += 1;
  return `result-${n}`;
}

export function addResult(wf: Workflow, stepId: string): Workflow {
  return mapSteps(wf, (steps) => {
    const step = steps.find((s) => s.id === stepId);
    if (step && step.results.length < 12)
      step.results.push({
        id: freeResultId(step.results),
        ok: true,
        description: '',
        next: 'next',
        loop: false,
      });
    return steps;
  });
}

export function removeResult(wf: Workflow, stepId: string, resultId: string): Workflow {
  return mapSteps(wf, (steps) => {
    const step = steps.find((s) => s.id === stepId);
    if (step && step.results.length > 1)
      step.results = step.results.filter((r) => r.id !== resultId);
    return steps;
  });
}

/** Replaces one result's fields; a route change goes through `setRoute`. */
export function patchResult(
  wf: Workflow,
  stepId: string,
  resultId: string,
  patch: Partial<Pick<StepResult, 'id' | 'ok' | 'description'>>,
): Workflow {
  return mapSteps(wf, (steps) => {
    const r = steps.find((s) => s.id === stepId)?.results.find((x) => x.id === resultId);
    if (!r) return steps;
    Object.assign(r, patch);
    if (patch.ok !== undefined && r.next === defaultRoute(!patch.ok))
      r.next = defaultRoute(patch.ok);
    return steps;
  });
}

/** A new step of `kind` at the end of the workflow, or right after `afterStepId`. */
export function addStep(
  wf: Workflow,
  kind: StepKind,
  afterStepId?: string,
): { wf: Workflow; stepId: string } {
  return insertStep(wf, (taken) => newStep(kind, taken), afterStepId);
}

/** The step `make` builds (given the taken ids) at the end of the workflow, or right after `afterStepId`. */
export function insertStep(
  wf: Workflow,
  make: (taken: readonly string[]) => WorkflowStep,
  afterStepId?: string,
): { wf: Workflow; stepId: string } {
  let stepId = '';
  const next = mapSteps(wf, (steps) => {
    const fresh = make(steps.map((s) => s.id));
    stepId = fresh.id;
    const at = afterStepId ? steps.findIndex((s) => s.id === afterStepId) + 1 : steps.length;
    steps.splice(at, 0, fresh);
    return normalizeOrder(steps);
  });
  return { wf: next, stepId };
}

/** Removes a step; routes that went to it fall back to their default. */
export function removeStep(wf: Workflow, stepId: string): Workflow {
  return mapSteps(wf, (steps) => {
    const out = steps.filter((s) => s.id !== stepId);
    for (const s of out)
      for (const r of s.results)
        if (r.next === stepId) {
          r.next = defaultRoute(r.ok);
          r.loop = false;
        }
    return normalizeOrder(out);
  });
}

/** Clears a review step's `implementer` once it no longer names an earlier agent prompt step (the
 *  save would refuse it); '' means the nearest one. */
export function clearStaleImplementers(wf: Workflow): Workflow {
  const stale = (steps: readonly WorkflowStep[], i: number): boolean => {
    const id = steps[i]?.review?.implementer;
    if (!id) return false;
    const t = steps.findIndex((x) => x.id === id);
    const target = steps[t];
    return !target || t >= i || target.kind !== 'agent' || target.smartScript !== '';
  };
  if (!wf.steps.some((_, i) => stale(wf.steps, i))) return wf;
  return mapSteps(wf, (steps) => {
    steps.forEach((s, i) => {
      if (s.review && stale(steps, i)) s.review.implementer = '';
    });
    return steps;
  });
}

/** Makes a step the first one, the workflow's start. */
export function setStart(wf: Workflow, stepId: string): Workflow {
  return mapSteps(wf, (steps) => {
    const at = steps.findIndex((s) => s.id === stepId);
    if (at <= 0) return steps;
    expand(steps);
    const [step] = steps.splice(at, 1) as [WorkflowStep];
    return normalizeOrder([step, ...steps]);
  });
}

/** Whether the step can swap places with its neighbour: not when a result routes to that neighbour
 *  by id, which would turn a forward route into a return. */
export function canMove(wf: Workflow, stepId: string, delta: -1 | 1): boolean {
  const i = wf.steps.findIndex((s) => s.id === stepId);
  const j = i + delta;
  if (i < 0 || j < 0 || j >= wf.steps.length) return false;
  const lo = wf.steps[Math.min(i, j)] as WorkflowStep;
  const hi = wf.steps[Math.max(i, j)] as WorkflowStep;
  return !lo.results.some((r) => !r.loop && r.next === hi.id);
}

/** Swaps the step with its neighbour; `next` keeps meaning "the following step". */
export function moveStep(wf: Workflow, stepId: string, delta: -1 | 1): Workflow {
  if (!canMove(wf, stepId, delta)) return wf;
  return mapSteps(wf, (steps) => {
    const i = steps.findIndex((s) => s.id === stepId);
    const [s] = steps.splice(i, 1) as [WorkflowStep];
    steps.splice(i + delta, 0, s);
    reclassify(steps);
    return steps;
  });
}

/** What a step node shows. */
export interface StepNodeData {
  step: WorkflowStep;
  /** 0-based position in the workflow. */
  index: number;
  start: boolean;
  selected: boolean;
  canUp: boolean;
  canDown: boolean;
  /** Removing the step leaves the workflow with at least one step. */
  canRemove: boolean;
  /** Skipping the step leaves at least one step that runs. */
  canSkip: boolean;
  w: number;
  h: number;
}
export interface EdgeData {
  tone: EdgeTone;
  loop: boolean;
  results: string[];
  selected: boolean;
  spine: boolean;
  points?: Point[];
  lane?: number;
  laneX?: number;
  /** The implicit edge of a manual step: no label, never editable. */
  fixed: boolean;
}
