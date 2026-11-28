import { describe, expect, test } from 'bun:test';
import { LATER } from '../../frontend/src/ade/v2/board/calendar';
import { buildTimeline, type TimelineInput } from '../../frontend/src/ade/v2/board/timeline';
import type { Board } from '../../frontend/src/ade/v2/wire';
import {
  DEFAULT_STATUS_DEFAULTS,
  DEFAULT_STATUSES,
  mkBranch,
  mkPlan,
  mkTask,
} from './support/adeV2Fixtures';

// 2026-09-22 is a Tuesday: offsets 4 and 5 are Sat/Sun, 6 is Mon.
const TODAY = '2026-09-22';
const SETTINGS = {
  historyDays: 14,
  offDays: [] as string[],
  workWeekendDays: [] as string[],
  workdayHours: 6,
  spanDayShare: 0.5,
  headlessSettingSources: 'all' as const,
};

function iso(offset: number): string {
  const d = new Date(Date.UTC(2026, 8, 22 + offset));
  return d.toISOString().slice(0, 10);
}

interface TaskSpec {
  id: string;
  day?: number | null;
  est?: string;
  kind?: 'task' | 'review' | 'parked';
  merged?: boolean;
}

function board(specs: TaskSpec[], extra: Partial<Board> = {}): Board {
  const tasks = specs.map((s) =>
    mkTask({ id: s.id, kind: s.kind ?? 'task', est: s.est ?? '', branchIds: [`b_${s.id}`] }),
  );
  const branches = specs.map((s) =>
    mkBranch({
      id: `b_${s.id}`,
      taskId: s.id,
      kind: s.kind === 'review' ? 'review' : s.kind === 'parked' ? 'parked' : 'mine',
      mergedIntoMain: s.merged ?? false,
    }),
  );
  const day: Record<string, string> = {};
  for (const s of specs) if (s.day !== undefined && s.day !== null) day[s.id] = iso(s.day);
  return {
    tasks,
    branches,
    plan: mkPlan({ day, order: specs.map((s) => s.id) }),
    pairs: [],
    statuses: DEFAULT_STATUSES,
    statusDefaults: DEFAULT_STATUS_DEFAULTS,
    history: [],
    repos: [],
    worktreeBasePath: '',
    autofetchMinutes: 0,
    ...extra,
  };
}

function view(b: Board, over: Partial<TimelineInput> = {}, settings = SETTINGS) {
  return buildTimeline({
    board: b,
    settings,
    today: TODAY,
    localDayOf: (ms) => new Date(ms).toISOString().slice(0, 10),
    repoLabel: (id) => id,
    showAllItems: false,
    showHistory: false,
    ...over,
  });
}

describe('merge order', () => {
  test('sorts by merge day, then start day, then plan position; parked is not numbered', () => {
    const v = view(
      board([
        { id: 'late', day: 1, est: '1h' }, // end 1, start 1, pos 0
        { id: 'span', day: 0, est: '2d' }, // end 1, start 0, pos 1
        { id: 'park', kind: 'parked', day: 0, est: '1h' },
        { id: 'tie', day: 1, est: '1h' }, // end 1, start 1, pos 3
      ]),
    );
    expect(v.seq.map((e) => e.taskId)).toEqual(['park', 'span', 'late', 'tie']);
    expect(v.mergeOrder.get('park')).toBeUndefined();
    expect([...v.mergeOrder]).toEqual([
      ['span', 1],
      ['late', 2],
      ['tie', 3],
    ]);
  });

  test('a review item sits right above the first task that builds on it', () => {
    const b = board([
      { id: 'a', day: 0, est: '1h' },
      { id: 'b', day: 1, est: '1h' },
      { id: 'rev', kind: 'review' },
    ]);
    const stacked = b.branches.find((x) => x.id === 'b_b');
    if (stacked) stacked.baseBranchId = 'b_rev';
    const v = view(b);
    expect(v.seq.map((e) => e.taskId)).toEqual(['a', 'rev', 'b']);
    expect(v.entries.get('rev')?.day).toBe(1);
  });

  test('a review item with a plan day sits on that day and places nothing', () => {
    const b = board([
      { id: 'a', day: 0, est: '1h' },
      { id: 'b', day: 1, est: '1h' },
      { id: 'rev', kind: 'review', day: 2 },
    ]);
    const stacked = b.branches.find((x) => x.id === 'b_b');
    if (stacked) stacked.baseBranchId = 'b_rev';
    const v = view(b);
    expect(v.seq.map((e) => e.taskId)).toEqual(['a', 'b', 'rev']);
    expect(v.entries.get('rev')?.day).toBe(2);
    expect(v.entries.get('rev')?.auto).toBe(false);
  });

  test('an unplaced review item goes to Later', () => {
    const v = view(
      board([
        { id: 'a', day: 0 },
        { id: 'rev', kind: 'review' },
      ]),
    );
    expect(v.entries.get('rev')?.day).toBe(LATER);
    expect(v.seq.at(-1)?.taskId).toBe('rev');
  });
});

describe('spans over weekends and days off', () => {
  test('3d starting Thursday skips the weekend', () => {
    const v = view(board([{ id: 'a', day: 2, est: '3d' }]));
    expect(v.entries.get('a')?.days).toEqual([2, 3, 6]);
    expect(v.entries.get('a')?.end).toBe(6);
  });

  test('a day off pushes the span, a worked weekend joins it', () => {
    const off = view(
      board([{ id: 'a', day: 2, est: '3d' }]),
      {},
      { ...SETTINGS, offDays: [iso(6)] },
    );
    expect(off.entries.get('a')?.days).toEqual([2, 3, 7]);
    const work = view(
      board([{ id: 'a', day: 2, est: '3d' }]),
      {},
      { ...SETTINGS, workWeekendDays: [iso(4)] },
    );
    expect(work.entries.get('a')?.days).toEqual([2, 3, 4]);
  });

  test('later days of a span get a continuation entry, the last one merges', () => {
    const v = view(board([{ id: 'a', day: 2, est: '3d' }]));
    const spans = v.bands.flatMap((x) => x.spans);
    expect(spans.map((s) => [s.dayNumber, s.merges])).toEqual([
      [2, false],
      [3, true],
    ]);
  });
});

describe('capacity and overdue', () => {
  test('hours over the workday cap flag the latest-starting tasks', () => {
    const v = view(
      board([
        { id: 'a', day: 1, est: '4h' },
        { id: 'b', day: 1, est: '4h' },
      ]),
    );
    const band = v.bands.find((x) => x.key === 1);
    expect(band?.hours).toBe(8);
    expect(band?.overflowTaskIds).toEqual(['b']);
    expect(band?.overflowHours).toBe(2);
    expect(band?.overflowMoveTo).toBe(2);
  });

  test('a multi-day task spreads its hours; no overflow within the cap', () => {
    const v = view(board([{ id: 'a', day: 0, est: '2d' }]));
    expect(v.bands.find((x) => x.key === 0)?.hours).toBe(3);
    expect(v.bands.find((x) => x.key === 1)?.hours).toBe(3);
    expect(v.bands.every((x) => x.overflowTaskIds.length === 0)).toBe(true);
  });

  test('unmerged tasks that ended in the past are overdue; merged ones are not', () => {
    const v = view(
      board([
        { id: 'a', day: -2, est: '1h' },
        { id: 'b', day: -2, est: '1h', merged: true },
        { id: 'r1', kind: 'review', day: -2 },
        { id: 'r2', kind: 'review', day: -2, merged: true },
      ]),
      { showHistory: true },
    );
    const band = v.bands.find((x) => x.key === -2);
    expect(band?.isPast).toBe(true);
    expect(band?.overdueTaskIds).toEqual(['a', 'r1']);
  });

  test('unplanned tasks leave the plan: no tail band, no band entry', () => {
    const v = view(board([{ id: 'a' }, { id: 'b', day: 0 }]));
    expect(v.bands.some((b) => b.isLater)).toBe(false);
    expect(v.bands.flatMap((b) => b.taskIds)).toEqual(['b']);
    expect(v.entries.get('a')?.day).toBe(LATER);
  });
});

describe('first-10 cap', () => {
  const many = (n: number, extra: TaskSpec[] = []) =>
    board([
      ...Array.from({ length: n }, (_, i) => ({ id: `t${i}`, day: Math.floor(i / 2), est: '1h' })),
      ...extra,
    ]);

  test('exactly 10 items show everything and no button', () => {
    const v = view(many(10));
    expect(v.hiddenCount).toBe(0);
    expect(v.shownTaskIds).toHaveLength(10);
    expect(v.canCollapse).toBe(false);
  });

  test('11 items hide the last, hide days after the 10th, and drop Later', () => {
    const v = view(many(11));
    expect(v.hiddenCount).toBe(1);
    expect(v.moreButtonLabel).toBe('Load all items · 1 more');
    expect(v.shownTaskIds).not.toContain('t10');
    const keys = v.bands.map((x) => x.key);
    expect(keys).toContain(4);
    expect(keys).not.toContain(5);
    expect(keys).not.toContain(LATER);
  });

  test('showing all items restores the days and offers to collapse', () => {
    const v = view(many(11), { showAllItems: true });
    expect(v.hiddenCount).toBe(0);
    expect(v.canCollapse).toBe(true);
    expect(v.bands.map((x) => x.key)).toContain(5);
  });

  test('a review item counts toward the cap: it can take the 10th slot', () => {
    const b = many(10, [{ id: 'rev', kind: 'review' }]);
    const stacked = b.branches.find((x) => x.id === 'b_t9');
    if (stacked) stacked.baseBranchId = 'b_rev';
    const v = view(b);
    // rev is placed right above t9, so seq is t0..t8, rev, t9 and t9 falls past the cap.
    expect(v.seq.map((e) => e.taskId).indexOf('rev')).toBe(9);
    expect(v.shownTaskIds).toContain('rev');
    expect(v.shownTaskIds).not.toContain('t9');
    expect(v.hiddenCount).toBe(1);
  });
});

describe('7-day window', () => {
  const keysOf = (v: ReturnType<typeof view>) => v.bands.map((x) => x.key);

  test('an empty board has bands 0..6 and no tail band', () => {
    expect(keysOf(view(board([])))).toEqual([0, 1, 2, 3, 4, 5, 6]);
  });

  test('work after the window sits in Further out with its date; unplanned work stays off', () => {
    const v = view(board([{ id: 'free' }, { id: 'far', day: 7, est: '1h' }]));
    expect(v.entries.get('far')?.day).toBe(7);
    expect(keysOf(v)).toEqual([0, 1, 2, 3, 4, 5, 6, LATER]);
    expect(v.bands.find((x) => x.isLater)?.taskIds).toEqual(['far']);
  });

  test('a span running past the window gets continuation rows in the window only', () => {
    const v = view(board([{ id: 'a', day: 3, est: '3d' }]));
    expect(v.entries.get('a')?.days).toEqual([3, 6, 7]);
    expect(keysOf(v)).toEqual([0, 1, 2, 3, 4, 5, 6]);
    expect(v.bands.flatMap((x) => x.spans.map((s) => [x.key, s.dayNumber]))).toEqual([[6, 2]]);
  });

  test('Further out hours count work past the window only', () => {
    const v = view(
      board([
        { id: 'far', day: 8, est: '3h' },
        { id: 'free', est: '1h' },
      ]),
    );
    expect(v.bands.find((x) => x.isLater)?.hours).toBe(3);
  });

  test('a day off past the window adds no band', () => {
    const v = view(board([]), {}, { ...SETTINGS, offDays: [iso(9)] });
    expect(keysOf(v)).toEqual([0, 1, 2, 3, 4, 5, 6]);
  });

  test('the next day pulls the day-7 task from Further out into band 6', () => {
    const b = board([{ id: 'far', day: 7, est: '1h' }]);
    expect(view(b).bands.find((x) => x.key === 6)?.taskIds).toEqual([]);
    const next = view(b, { today: iso(1) });
    expect(next.bands.find((x) => x.key === 6)?.taskIds).toEqual(['far']);
    expect(next.bands.find((x) => x.isLater)).toBeUndefined();
  });
});

describe('history', () => {
  test('counts archived tasks inside the window and shows them on their day', () => {
    const noon = (offset: number) => Date.UTC(2026, 8, 22 + offset, 12);
    const v = view(
      board([{ id: 'a', day: 0 }], {
        history: [
          { taskId: 'h1', title: 'one', codeRepoIds: [], mergedAt: null, archivedAt: noon(-1) },
          { taskId: 'h2', title: 'two', codeRepoIds: [], mergedAt: null, archivedAt: noon(-20) },
        ],
      }),
      { showHistory: true },
    );
    expect(v.historyCount).toBe(1);
    expect(v.historyTip).toBe('1 archived task in the last 2 weeks');
    expect(v.bands.find((x) => x.key === -1)?.history.map((h) => h.taskId)).toEqual(['h1']);
  });
});

describe('base cycle', () => {
  test('a base cycle neither recurses without end nor drops its rows', () => {
    const b = board([
      { id: 'a', day: 0 },
      { id: 'kid', day: 1 },
    ]);
    for (const [id, base] of [
      ['b_a', 'b_kid'],
      ['b_kid', 'b_a'],
    ] as const) {
      const br = b.branches.find((x) => x.id === id);
      if (br) br.baseBranchId = base;
    }
    const v = view(b);
    expect(v.branchRows('a').map((r) => r.id)).toEqual(['b_a']);
    expect(v.branchRows('kid').map((r) => r.id)).toEqual(['b_kid']);
  });
});
