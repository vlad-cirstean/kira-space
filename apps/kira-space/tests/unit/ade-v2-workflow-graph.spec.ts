import { describe, expect, test } from 'bun:test';
import { defaultResults } from '../../frontend/src/ade/v2/board/stepResults';
import {
  addStep,
  aggregateEdges,
  canMove,
  clearRoute,
  type Graph,
  type GraphNode,
  moveStep,
  NODE_H,
  NODE_W,
  normalizeOrder,
  removeStep,
  setRoute,
  setStart,
} from '../../frontend/src/ade/v2/board/workflowGraph';
import { layoutWorkflow } from '../../frontend/src/ade/v2/board/workflowLayout';
import type { StepResult, Workflow, WorkflowStep } from '../../frontend/src/ade/v2/wire';
import { mkManual, mkStep } from './support/adeV2Fixtures';

const res = (id: string, ok: boolean, next = ok ? 'next' : 'stop', loop = false): StepResult => ({
  id,
  ok,
  description: '',
  next,
  loop,
});
const step = (id: string, results: StepResult[] = defaultResults()): WorkflowStep =>
  mkStep({ id, results, runsOn: 'once' });
const wfOf = (steps: WorkflowStep[]): Workflow => ({
  id: 'w',
  name: 'w',
  steps,
});
const next = (wf: Workflow, id: string, result: string): string =>
  wf.steps.find((s) => s.id === id)?.results.find((r) => r.id === result)?.next ?? '';
const order = (wf: Workflow): string[] => wf.steps.map((s) => s.id);

describe('aggregateEdges', () => {
  test('groups results by target and tones the edge by their ok flags', () => {
    const steps = [
      step('a', [
        res('good', true, 'b'),
        res('fine', true, 'b'),
        res('bad', false, 'b'),
        res('stop', false),
      ]),
      step('b'),
    ];
    const [e, ...rest] = aggregateEdges(steps, 0);
    expect(rest).toHaveLength(0);
    expect(e).toMatchObject({ tone: 'neutral', results: ['good', 'fine', 'bad'], loop: false });
  });

  test('all ok is ok, all not ok is fail; stop draws no edge', () => {
    const steps = [step('a', [res('p', true, 'next'), res('q', false, 'end')]), step('b')];
    expect(aggregateEdges(steps, 0).map((e) => [e.target, e.tone])).toEqual([
      ['step:b', 'ok'],
      ['end', 'fail'],
    ]);
  });

  test('an earlier or own target is a loop edge', () => {
    const steps = [
      step('a'),
      step('b', [res('ok', true), res('again', false, 'a', true), res('self', false, 'b', true)]),
    ];
    expect(
      aggregateEdges(steps, 1)
        .filter((e) => e.loop)
        .map((e) => e.target),
    ).toEqual(['step:a', 'step:b']);
  });

  test('a manual step has one implicit edge to the next step and no handle', () => {
    const [e, ...rest] = aggregateEdges([mkManual({ id: 'm' }), step('b')], 0);
    expect(rest).toHaveLength(0);
    expect(e).toMatchObject({ target: 'step:b', tone: 'ok', loop: false, results: [] });
    expect(e?.sourceHandle).toBeUndefined();
  });
});

describe('setRoute', () => {
  test('an earlier target already reached forward becomes a loop edge', () => {
    const wf = setRoute(wfOf([step('a'), step('b')]), 'b', 'failed', 'a');
    expect(order(wf)).toEqual(['a', 'b']);
    const r = wf.steps[1]?.results[1];
    expect([r?.next, r?.loop]).toEqual(['a', true]);
  });

  test('an earlier target that cannot reach the source moves after it', () => {
    const base = wfOf([
      step('a', [res('done', true, 'end'), res('failed', false)]),
      step('b', [res('done', true, 'end'), res('failed', false)]),
      step('c'),
    ]);
    const wf = setRoute(base, 'c', 'done', 'b');
    expect(order(wf)).toEqual(['a', 'c', 'b']);
    expect(next(wf, 'c', 'done')).toBe('next');
    expect(wf.steps[1]?.results[0]?.loop).toBe(false);
  });

  test('a forward target past the next step keeps its id; the following step collapses to next', () => {
    const wf = setRoute(wfOf([step('a'), step('b'), step('c')]), 'a', 'done', 'c');
    expect(next(wf, 'a', 'done')).toBe('c');
    expect(next(setRoute(wf, 'a', 'done', 'b'), 'a', 'done')).toBe('next');
  });

  test('self route is a loop edge', () => {
    const wf = setRoute(wfOf([step('a')]), 'a', 'failed', 'a');
    expect(wf.steps[0]?.results[1]).toMatchObject({ next: 'a', loop: true });
  });
});

describe('normalizeOrder', () => {
  test('keeps the start first and puts forward targets after their sources, stable otherwise', () => {
    const steps = [
      step('a', [res('done', true, 'c'), res('failed', false)]),
      step('b', [res('done', true, 'end'), res('failed', false)]),
      step('c', [res('done', true, 'b'), res('failed', false)]),
    ];
    expect(normalizeOrder(steps).map((s) => s.id)).toEqual(['a', 'c', 'b']);
  });

  test('does not mutate its input and keeps the meaning of next', () => {
    const steps = [step('a'), step('b'), step('c')];
    const before = JSON.stringify(steps);
    const out = normalizeOrder(steps);
    expect(JSON.stringify(steps)).toBe(before);
    expect(out.map((s) => s.results[0]?.next)).toEqual(['next', 'next', 'next']);
  });
});

describe('edits', () => {
  test('setStart turns the routes that led into the new start into loop edges', () => {
    const wf = setStart(wfOf([step('a'), step('b')]), 'b');
    expect(order(wf)).toEqual(['b', 'a']);
    expect(wf.steps[0]?.results[0]?.next).toBe('end');
    expect(wf.steps[1]?.results[0]).toMatchObject({ next: 'b', loop: true });
  });

  test('removeStep resets routes into it to their defaults', () => {
    const base = wfOf([
      step('a', [res('done', true, 'c'), res('failed', false, 'b', true)]),
      step('b'),
      step('c'),
    ]);
    const wf = removeStep(base, 'b');
    expect(next(wf, 'a', 'failed')).toBe('stop');
    expect(wf.steps[0]?.results[1]?.loop).toBe(false);
  });

  test('clearRoute restores the default of each result', () => {
    const base = wfOf([step('a', [res('done', true, 'end'), res('failed', false, 'a', true)])]);
    const wf = clearRoute(base, 'a', ['done', 'failed']);
    expect(wf.steps[0]?.results.map((r) => [r.next, r.loop])).toEqual([
      ['next', false],
      ['stop', false],
    ]);
  });

  test('addStep inserts a step of the kind after the given step with that kind defaults', () => {
    const { wf, stepId } = addStep(wfOf([step('a'), step('b')]), 'review', 'a');
    expect(order(wf)[1]).toBe(stepId);
    expect(wf.steps[1]).toMatchObject({ kind: 'review', maxReturns: 0 });
    expect(wf.steps[1]?.results.map((r) => r.id)).toEqual(['approved', 'rejected']);
  });

  test('a manual step added has no results', () => {
    const { wf } = addStep(wfOf([step('a')]), 'manual');
    expect(wf.steps[1]).toMatchObject({ kind: 'manual', results: [] });
  });

  test('moveStep swaps neighbours, except across an explicit route that would turn into a return', () => {
    const wf = wfOf([step('a'), step('b'), step('c')]);
    expect(order(moveStep(wf, 'b', -1))).toEqual(['b', 'a', 'c']);
    expect(moveStep(wf, 'a', -1)).toBe(wf);
    const routed = wfOf([step('a', [res('done', true, 'b'), res('failed', false)]), step('b')]);
    expect(canMove(routed, 'b', -1)).toBe(false);
    expect(moveStep(routed, 'b', -1)).toBe(routed);
  });
});

describe('layoutWorkflow', () => {
  const at = (g: Graph, id: string): GraphNode => g.nodes.find((x) => x.id === id) as GraphNode;
  const cx = (g: Graph, id: string): number => at(g, id).x + NODE_W / 2;
  const layout = (steps: WorkflowStep[]): Graph => layoutWorkflow(wfOf(steps));

  test('steps and the end node share one size and one spine column, top to bottom', () => {
    const g = layout([step('a'), step('b')]);
    for (const n of g.nodes) expect([n.w, n.h]).toEqual([NODE_W, NODE_H]);
    expect(new Set(['step:a', 'step:b', 'end'].map((id) => cx(g, id))).size).toBe(1);
    expect(at(g, 'step:a').y).toBeLessThan(at(g, 'step:b').y);
    expect(at(g, 'step:b').y).toBeLessThan(at(g, 'end').y);
    expect(g.height).toBeGreaterThan(0);
  });

  test('a diamond fans out: branches share a rank, the join stays on the spine, nothing points up', () => {
    const g = layout([
      step('a', [res('left', true, 'b'), res('right', false, 'c')]),
      step('b', [res('done', true, 'd'), res('failed', false)]),
      step('c', [res('done', true, 'd'), res('failed', false)]),
      step('d'),
    ]);
    const p = (id: string): GraphNode => at(g, `step:${id}`);
    expect(p('b').y).toBe(p('c').y);
    expect(p('b').x).not.toBe(p('c').x);
    expect(p('d').y).toBeGreaterThan(p('b').y);
    expect(cx(g, 'step:a')).toBe(cx(g, 'step:b'));
    expect(cx(g, 'step:b')).toBe(cx(g, 'step:d'));
    for (const e of g.edges.filter((x) => !x.loop && x.results.length > 0))
      expect(at(g, e.target).y).toBeGreaterThan(at(g, e.source).y);
  });

  test('a forward edge that skips a rank carries a route around the nodes between', () => {
    const g = layout([step('a', [res('done', true), res('skip', false, 'end')]), step('b')]);
    const skip = g.edges.find((e) => e.target === 'end' && e.source === 'step:a');
    expect(skip?.points?.length).toBeGreaterThan(0);
  });

  test('nested and overlapping loops get distinct lanes, disjoint loops share one, no loop has a route', () => {
    const g = layout([
      step('a', [res('done', true), res('redo', false, 'a', true)]),
      step('b', [res('done', true), res('redo', false, 'b', true)]),
      step('c', [res('done', true), res('redo', false, 'b', true)]),
      step('d', [res('done', true), res('redo', false, 'b', true), res('all', false, 'a', true)]),
    ]);
    const lane = (source: string, target: string): number | undefined =>
      g.edges.find((e) => e.loop && e.source === `step:${source}` && e.target === `step:${target}`)
        ?.lane;
    expect(lane('a', 'a')).toBe(lane('b', 'b'));
    expect(lane('c', 'b')).not.toBe(lane('b', 'b'));
    expect(lane('d', 'b')).not.toBe(lane('c', 'b'));
    expect(lane('d', 'a')).not.toBe(lane('d', 'b'));
    expect(lane('d', 'a')).toBeDefined();
    const loops = g.edges.filter((e) => e.loop);
    expect(loops.every((e) => e.points === undefined && e.laneX !== undefined)).toBe(true);
    const left = Math.min(...g.nodes.filter((n) => n.kind === 'step').map((n) => n.x));
    expect(loops.every((e) => (e.laneX ?? left) < left)).toBe(true);
  });
});
