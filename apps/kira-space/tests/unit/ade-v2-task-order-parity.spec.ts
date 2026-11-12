import { describe, expect, test } from 'bun:test';
import { readFileSync, writeFileSync } from 'node:fs';
import { taskListOrder } from '../../frontend/src/ade/v2/board/taskOrder';
import type { Board } from '../../frontend/src/ade/v2/wire';
import { mkBranch, mkPlan, mkTask } from './support/adeV2Fixtures';

// Golden parity with the Go port `internal/ade/taskorder.go`: `cases.json` holds boards and the order
// this TS function gives them; `taskorder_test.go` reads the same file. After changing the task list
// order, regenerate with
//   UPDATE_TASK_ORDER=1 bun test apps/kira-space/tests/unit/ade-v2-task-order-parity.spec.ts
// then port the change to Go until `go test ./apps/kira-space/internal/ade -run TestTaskListOrder` passes.

const CASES = new URL('../../internal/ade/testdata/taskorder/cases.json', import.meta.url);
// 2026-09-22 is a Tuesday: offsets 4 and 5 are Sat/Sun, 6 is Mon.
const TODAY = '2026-09-22';

interface WireTask {
  id: string;
  kind: 'task' | 'review' | 'parked';
  est: string;
  branchIds: string[];
}
interface WireBranch {
  id: string;
  taskId: string;
  codeRepoId: string;
  name: string;
  kind: 'mine' | 'review' | 'parked';
  baseBranchId: string;
  mergedIntoMain: boolean;
}
interface WirePlan {
  day: Record<string, string>;
  order: string[];
  queuedAfter: Record<string, string>;
  backlog: string[];
}
interface WirePair {
  a: string;
  b: string;
  shared: string[];
  conflicts: string[];
}
interface Settings {
  workdayHours: number;
  spanDayShare: number;
  offDays: string[];
  workWeekendDays: string[];
}
interface Case {
  name: string;
  today: string;
  settings: Settings;
  board: { tasks: WireTask[]; branches: WireBranch[]; plan: WirePlan; pairs: WirePair[] };
  order: string[];
}

function iso(offset: number): string {
  return new Date(Date.UTC(2026, 8, 22 + offset)).toISOString().slice(0, 10);
}

const SETTINGS: Settings = { workdayHours: 6, spanDayShare: 0.5, offDays: [], workWeekendDays: [] };

function task(id: string, over: Partial<WireTask> = {}): WireTask {
  return { id, kind: 'task', est: '', branchIds: [], ...over };
}
function branch(id: string, taskId: string, over: Partial<WireBranch> = {}): WireBranch {
  return {
    id,
    taskId,
    codeRepoId: 'repo-web',
    name: `feat/${id}`,
    kind: 'mine',
    baseBranchId: '',
    mergedIntoMain: false,
    ...over,
  };
}
function plan(over: Partial<WirePlan> = {}): WirePlan {
  return { day: {}, order: [], queuedAfter: {}, backlog: [], ...over };
}
/** Plan days given as offsets from `TODAY`. */
function days(offsets: Record<string, number>): Record<string, string> {
  return Object.fromEntries(Object.entries(offsets).map(([id, k]) => [id, iso(k)]));
}

type Draft = Omit<Case, 'order'>;

function draft(
  name: string,
  tasks: WireTask[],
  p: WirePlan,
  extra: {
    branches?: WireBranch[];
    pairs?: WirePair[];
    settings?: Partial<Settings>;
  } = {},
): Draft {
  return {
    name,
    today: TODAY,
    settings: { ...SETTINGS, ...extra.settings },
    board: { tasks, branches: extra.branches ?? [], plan: p, pairs: extra.pairs ?? [] },
  };
}

// A review item `R` (branch `rb`) and two mine tasks `T1`, `T2` (branches `b1`, `b2`).
function reviewBoard(
  extra: {
    queuedAfter?: Record<string, string>;
    base?: string;
    pairs?: WirePair[];
    b1?: Partial<WireBranch>;
    rb?: Partial<WireBranch>;
  },
  name: string,
): Draft {
  return draft(
    name,
    [
      task('T1', { branchIds: ['b1'] }),
      task('T2', { branchIds: ['b2'] }),
      task('R', { kind: 'review', branchIds: ['rb'] }),
    ],
    plan({
      day: days({ T1: 0, T2: 1 }),
      order: ['T1', 'T2', 'R'],
      queuedAfter: extra.queuedAfter ?? {},
    }),
    {
      branches: [
        branch('b1', 'T1', extra.b1),
        branch('b2', 'T2', { baseBranchId: extra.base ?? '' }),
        branch('rb', 'R', { kind: 'review', ...extra.rb }),
      ],
      pairs: extra.pairs,
    },
  );
}

const conflictPair: WirePair = { a: 'b1', b: 'rb', shared: ['f.go'], conflicts: ['f.go'] };

function handCases(): Draft[] {
  return [
    draft(
      'span across a weekend',
      [task('A', { est: '3d' }), task('B', { est: '1d' }), task('C', { est: '1d' })],
      plan({ day: days({ A: 2, B: 3, C: 6 }), order: ['A', 'B', 'C'] }),
    ),
    draft(
      'span across an off day',
      [task('A', { est: '2d' }), task('B', { est: '1d' }), task('C', { est: '1d' })],
      plan({ day: days({ A: 0, B: 1, C: 2 }), order: ['C', 'B', 'A'] }),
      { settings: { offDays: [iso(1)] } },
    ),
    draft(
      'span across a work weekend day',
      [task('A', { est: '2d' }), task('B', { est: '1d' }), task('C', { est: '1d' })],
      plan({ day: days({ A: 3, B: 4, C: 5 }), order: ['A', 'B', 'C'] }),
      { settings: { workWeekendDays: [iso(4)] } },
    ),
    draft(
      'each est unit',
      [
        task('m', { est: '30m' }),
        task('h', { est: '12h' }),
        task('d', { est: '2d' }),
        task('w', { est: '1w' }),
        task('dec', { est: '1.5d' }),
      ],
      plan({ day: days({ m: 0, h: 0, d: 0, w: 0, dec: 0 }), order: ['w', 'dec', 'd', 'h', 'm'] }),
    ),
    draft(
      'unparseable estimates span one day',
      [
        task('soon', { est: 'soon' }),
        task('empty', { est: '' }),
        task('words', { est: '3 days' }),
        task('neg', { est: '-2d' }),
        task('two', { est: '2d' }),
      ],
      plan({
        day: days({ soon: 1, empty: 1, words: 1, neg: 1, two: 0 }),
        order: ['two', 'neg', 'words', 'empty', 'soon'],
      }),
    ),
    draft(
      'fractional hours',
      [
        task('a', { est: '7.5h' }),
        task('b', { est: '6h' }),
        task('c', { est: '6.0000000001h' }),
        task('d', { est: '12.5h' }),
      ],
      plan({ day: days({ a: 0, b: 0, c: 0, d: 0 }), order: ['d', 'c', 'b', 'a'] }),
    ),
    draft(
      'estimate over 60 days',
      [
        task('huge', { est: '99d' }),
        task('weeks', { est: '20w' }),
        task('inf', { est: `1${'0'.repeat(400)}d` }),
        task('late', { est: '1d' }),
      ],
      plan({
        day: days({ huge: 0, weeks: 0, inf: 0, late: 30 }),
        order: ['late', 'inf', 'weeks', 'huge'],
      }),
    ),
    draft(
      'estimate with unicode spaces',
      [
        task('nbsp', { est: ' 2d　' }),
        task('inner', { est: '2 d' }),
        task('bom', { est: '﻿3d' }),
        task('zwsp', { est: '2​d' }),
        task('upper', { est: '2D' }),
        task('ascii', { est: ' \t1w\n' }),
        task('sep', { est: '2 d' }),
      ],
      plan({
        day: days({ nbsp: 0, inner: 0, bom: 0, zwsp: 0, upper: 0, ascii: 0, sep: 0 }),
        order: ['ascii', 'sep', 'upper', 'zwsp', 'bom', 'inner', 'nbsp'],
      }),
    ),
    draft(
      'ties on end, then day, then pos',
      [
        task('a', { est: '1d' }),
        task('b', { est: '1d' }),
        task('c', { est: '2d' }),
        task('d', { est: '1d' }),
      ],
      plan({ day: days({ a: 1, b: 1, c: 0, d: 1 }), order: ['d', 'b', 'c', 'a'] }),
    ),
    draft(
      'pos absent sorts last, equal keys stay in board order',
      [task('x'), task('y'), task('z'), task('w')],
      plan({ day: days({ x: 1, y: 1, z: 1, w: 1 }), order: ['w'] }),
    ),
    draft(
      'pinned review with a day',
      [task('A'), task('R', { kind: 'review', branchIds: ['rb'] }), task('B')],
      plan({ day: days({ A: 0, R: 0, B: 1 }), order: ['B', 'R', 'A'] }),
      { branches: [branch('rb', 'R', { kind: 'review' })] },
    ),
    reviewBoard({ queuedAfter: { b2: 'rb' } }, 'review placed by queuedAfter ancestry'),
    reviewBoard({ base: 'rb' }, 'review placed by baseBranchId ancestry'),
    reviewBoard({ pairs: [conflictPair] }, 'review placed by pair conflict'),
    reviewBoard(
      { pairs: [conflictPair], b1: { mergedIntoMain: true } },
      'conflict ignored for a merged branch',
    ),
    reviewBoard({ pairs: [conflictPair], b1: { name: '' } }, 'conflict ignored for a draft'),
    reviewBoard(
      { pairs: [conflictPair], rb: { codeRepoId: 'repo-api' } },
      'conflict ignored for another repo',
    ),
    reviewBoard(
      { pairs: [conflictPair], queuedAfter: { b1: 'rb' } },
      'conflict ignored for an ancestor, placed by ancestry',
    ),
    draft(
      'review placed through a deep ancestor chain, with a queue cycle elsewhere',
      [
        task('T0', { branchIds: ['b0'] }),
        task('T4', { branchIds: ['b4'] }),
        task('T5', { branchIds: ['b5'] }),
        task('T1', { branchIds: ['b1'] }),
        task('T2', { branchIds: ['b2'] }),
        task('R', { kind: 'review', branchIds: ['rb'] }),
      ],
      plan({
        day: days({ T0: 0, T4: 1, T5: 2, T1: 3, T2: 4 }),
        order: ['T0', 'T4', 'T5', 'T1', 'T2', 'R'],
        queuedAfter: { b4: 'b5', b5: 'b4', b1: 'b2' },
      }),
      {
        branches: [
          branch('b0', 'T0'),
          branch('b4', 'T4'),
          branch('b5', 'T5'),
          branch('b1', 'T1'),
          branch('b2', 'T2', { baseBranchId: 'rb' }),
          branch('rb', 'R', { kind: 'review' }),
        ],
      },
    ),
    draft(
      'an unplaced review goes last',
      [task('A'), task('R', { kind: 'review', branchIds: ['rb'] }), task('B')],
      plan({ day: days({ A: 1, B: 0 }), order: ['R', 'A', 'B'] }),
      { branches: [branch('rb', 'R', { kind: 'review' })] },
    ),
    draft(
      'a review without a branch is never placed on a task',
      [task('A', { branchIds: ['b1'] }), task('R', { kind: 'review' })],
      plan({ day: days({ A: 0 }), order: ['A', 'R'] }),
      { branches: [branch('b1', 'A')] },
    ),
    draft(
      'Later and Backlog shelves',
      [
        task('plan1'),
        task('later1'),
        task('later2'),
        task('back1'),
        task('back2'),
        task('dated-back'),
        task('parked', { kind: 'parked' }),
      ],
      plan({
        day: days({ plan1: 0, 'dated-back': 0 }),
        order: ['back2', 'later2', 'plan1', 'dated-back', 'later1', 'back1', 'parked'],
        backlog: ['back2', 'back1', 'dated-back'],
      }),
    ),
    draft(
      'a task missing from plan.order is appended in board order',
      [task('a'), task('b'), task('c'), task('d')],
      plan({ day: days({ d: 0 }), order: ['b'] }),
    ),
    draft(
      'past days sort first',
      [task('today'), task('past', { est: '2d' }), task('way', { est: '1d' })],
      plan({ day: days({ today: 0, past: -3, way: -10 }), order: ['today', 'past', 'way'] }),
    ),
    draft(
      'an unplaced review that sits in Backlog appears twice',
      [task('A'), task('R', { kind: 'review', branchIds: ['rb'] })],
      plan({ day: days({ A: 0 }), order: ['A', 'R'], backlog: ['R'] }),
      { branches: [branch('rb', 'R', { kind: 'review' })] },
    ),
    {
      ...draft('empty board', [], plan()),
    },
  ];
}

// mulberry32: a small seeded PRNG, so the random boards are the same on every regeneration.
function prng(seed: number): () => number {
  let a = seed;
  return () => {
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const EST_POOL = [
  '',
  '30m',
  '1h',
  '4h',
  '7.5h',
  '1d',
  '2d',
  '3d',
  '1.5d',
  '1w',
  '2w',
  '99d',
  'soon',
  '2 d',
  ' 2d',
  '3 days',
  '0d',
  '0h',
];

function randomCase(i: number): Draft {
  const rnd = prng(1000 + i);
  const int = (lo: number, hi: number): number => lo + Math.floor(rnd() * (hi - lo + 1));
  const pick = <T>(xs: readonly T[]): T => xs[int(0, xs.length - 1)] as T;
  const n = int(1, 12);
  const tasks: WireTask[] = [];
  const branches: WireBranch[] = [];
  const dayMap: Record<string, number> = {};
  for (let k = 0; k < n; k++) {
    const r = rnd();
    const kind = r < 0.78 ? 'task' : r < 0.93 ? 'review' : 'parked';
    const t = task(`t${k}`, { kind, est: pick(EST_POOL) });
    const nb = kind === 'review' ? int(0, 1) : int(0, 2);
    for (let j = 0; j < nb; j++) {
      const id = `b${k}-${j}`;
      t.branchIds.push(id);
      branches.push(
        branch(id, t.id, {
          kind: kind === 'task' ? 'mine' : kind,
          codeRepoId: pick(['repo-web', 'repo-api']),
          name: rnd() < 0.15 ? '' : `feat/${id}`,
          mergedIntoMain: rnd() < 0.15,
        }),
      );
    }
    tasks.push(t);
    if (rnd() < 0.7 || (kind === 'review' && rnd() < 0.5)) dayMap[t.id] = int(-20, 20);
  }
  for (const b of branches) if (rnd() < 0.3) b.baseBranchId = pick(branches).id;
  const queuedAfter: Record<string, string> = {};
  for (const b of branches) if (rnd() < 0.2) queuedAfter[b.id] = pick(branches).id;
  const pairs: WirePair[] = [];
  for (let k = int(0, 6); k > 0 && branches.length > 1; k--) {
    const files = rnd() < 0.7 ? ['x.go'] : [];
    pairs.push({ a: pick(branches).id, b: pick(branches).id, shared: files, conflicts: files });
  }
  const ids = tasks.map((t) => t.id).filter(() => rnd() < 0.8);
  for (let k = ids.length - 1; k > 0; k--) {
    const j = int(0, k);
    [ids[k], ids[j]] = [ids[j] as string, ids[k] as string];
  }
  const offDays = Array.from({ length: int(0, 3) }, () => iso(int(-3, 14)));
  const workWeekendDays = Array.from({ length: int(0, 2) }, () => iso(int(-3, 14)));
  return draft(
    `random ${i}`,
    tasks,
    plan({
      day: days(dayMap),
      order: ids,
      queuedAfter,
      backlog: tasks.map((t) => t.id).filter(() => rnd() < 0.1),
    }),
    {
      branches,
      pairs,
      settings: { workdayHours: pick([4, 6, 8]), offDays, workWeekendDays },
    },
  );
}

function orderOf(c: Draft): string[] {
  const { tasks, branches, plan: p, pairs } = c.board;
  const board = {
    tasks: tasks.map((t) => mkTask({ ...t })),
    branches: branches.map((b) => mkBranch({ ...b })),
    plan: mkPlan({ ...p }),
    pairs,
  } as Pick<Board, 'tasks' | 'branches' | 'plan' | 'pairs'>;
  const ade = { historyDays: 14, headlessSettingSources: 'all' as const, ...c.settings };
  return taskListOrder(board, ade, c.today);
}

if (process.env.UPDATE_TASK_ORDER === '1') {
  const drafts = [...handCases(), ...Array.from({ length: 200 }, (_, i) => randomCase(i))];
  const cases: Case[] = drafts.map((c) => ({ ...c, order: orderOf(c) }));
  writeFileSync(CASES, `[\n${cases.map((c) => JSON.stringify(c)).join(',\n')}\n]\n`);
}

describe('taskListOrder golden cases (shared with the Go port)', () => {
  const cases = JSON.parse(readFileSync(CASES, 'utf8')) as Case[];
  test('the file holds the hand-written and random cases', () => {
    expect(cases.length).toBeGreaterThanOrEqual(220);
  });
  for (const c of cases) {
    test(c.name, () => {
      expect(orderOf(c)).toEqual(c.order);
    });
  }
});
