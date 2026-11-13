import { describe, expect, test } from 'bun:test';
import type { TaskProgress } from '../../frontend/src/ade/v2/board/progress';
import {
  cachedProgress,
  type ProgressCache,
  reuseModels,
} from '../../frontend/src/ade/v2/plan/planModelMemo';
import type { Task, Workflow } from '../../frontend/src/ade/v2/wire';
import { mkTask } from './support/adeV2Fixtures';

// Cache invalidation across interacting inputs: unchanged cards keep their identity, a changed one is
// the only one replaced, progress rebuilds only when its task, workflow or branch facts change.

interface Card {
  id: string;
  title: string;
  rows: { id: string; tag: string }[];
}

const card = (id: string, title = id, tag = 'open'): Card => ({
  id,
  title,
  rows: [{ id: `${id}-r`, tag }],
});

describe('reuseModels', () => {
  test('an equal rebuild keeps the previous object', () => {
    const first = reuseModels(new Map(), ['a', 'b'], (id) => card(id));
    const second = reuseModels(first, ['a', 'b'], (id) => card(id));
    expect(second.get('a')).toBe(first.get('a') as Card);
    expect(second.get('b')).toBe(first.get('b') as Card);
  });

  test('a changed field replaces only that card and shares its untouched parts', () => {
    const first = reuseModels(new Map(), ['a', 'b'], (id) => card(id));
    const second = reuseModels(first, ['a', 'b'], (id) => card(id, id === 'a' ? 'renamed' : id));
    expect(second.get('b')).toBe(first.get('b') as Card);
    expect(second.get('a')).not.toBe(first.get('a') as Card);
    expect(second.get('a')?.title).toBe('renamed');
    expect(second.get('a')?.rows).toBe((first.get('a') as Card).rows);
  });

  test('a nested change rebuilds that card alone', () => {
    const first = reuseModels(new Map(), ['a', 'b'], (id) => card(id));
    const second = reuseModels(first, ['a', 'b'], (id) =>
      card(id, id, id === 'b' ? 'merged' : 'open'),
    );
    expect(second.get('a')).toBe(first.get('a') as Card);
    expect(second.get('b')?.rows[0]?.tag).toBe('merged');
  });

  test('an id no longer listed or built drops out', () => {
    const first = reuseModels(new Map(), ['a', 'b'], (id) => card(id));
    const second = reuseModels(first, ['a', 'b', 'c'], (id) => (id === 'b' ? null : card(id)));
    expect([...second.keys()]).toEqual(['a', 'c']);
  });
});

describe('cachedProgress', () => {
  const task: Task = mkTask({ id: 't1' });
  const wf = { id: 'wf', name: 'Feature', steps: [] } as unknown as Workflow;
  const progress = (label: string) => ({ label }) as unknown as TaskProgress;

  function run(cache: ProgressCache, t: Task, w: Workflow | null, facts: string, label: string) {
    let built = 0;
    const out = cachedProgress(cache, t, w, facts, () => {
      built++;
      return progress(label);
    });
    return { out, built };
  }

  test('hits on the same task, workflow and facts; misses when any changes', () => {
    const cache: ProgressCache = new WeakMap();
    const first = run(cache, task, wf, 'b1', 'one');
    expect(first.built).toBe(1);
    const hit = run(cache, task, wf, 'b1', 'two');
    expect(hit.built).toBe(0);
    expect(hit.out).toBe(first.out);
    expect(run(cache, task, { ...wf }, 'b1', 'three').built).toBe(1);
    expect(run(cache, task, wf, 'b2', 'four').built).toBe(1);
    expect(run(cache, { ...task }, wf, 'b2', 'five').built).toBe(1);
  });
});
