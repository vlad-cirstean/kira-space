import { beforeAll, describe, expect, test } from 'bun:test';
import { useClaudeAgentText } from '@workbench/testing/unit/claudeAgentText';
import { branchTag, taskCell } from '../../frontend/src/ade/v2/board/actions';
import { baseMarker } from '../../frontend/src/ade/v2/board/baseMarker';
import { buildBranchGraph } from '../../frontend/src/ade/v2/board/branchGraph';
import { branchLine2 } from '../../frontend/src/ade/v2/board/labels';
import { buildNeedsYou, NEEDS_RANK } from '../../frontend/src/ade/v2/board/needsYou';
import { buildTaskProgress, type TaskProgress } from '../../frontend/src/ade/v2/board/progress';
import { shelfOf } from '../../frontend/src/ade/v2/board/shelf';
import { blocked as isBlocked, statusOf } from '../../frontend/src/ade/v2/board/status';
import { buildTimeline, type TimelineView } from '../../frontend/src/ade/v2/board/timeline';
import type {
  Board,
  Branch,
  ChangeRequestsResult,
  Pair,
  ReposResult,
  SessionsResult,
  Workflow,
  WorkflowsResult,
} from '../../frontend/src/ade/v2/wire';
import {
  DEFAULT_STATUSES,
  loadFixture,
  mkBranch,
  mkPlan,
  mkRun,
  mkStep,
  mkTask,
} from './support/adeV2Fixtures';
import { type MockupComponent, runMockupV2 } from './support/mockupV2Oracle';

// Parity: the board logic over the P143 fixtures equals the mockup's own `renderVals()` for status,
// step label and percent, task action, branch tag and actions, base marker, day bands and the
// Needs-you list. The fixtures describe the mockup's default data, with four input normalizations
// the mockup derives itself and the wire carries as backend facts:
//   1. a running headless run whose session waits for input is `stuck` (the mockup reads the session);
//   2. `pairs` are recomputed from each branch's file list, and a pair conflicts when one side is a
//      review item (the mockup's file-overlap rule; the backend uses git merge-tree);
//   3. `now` is 3m 40s after the `preparing` worktree setup started;
//   4. the fixture's `tk01` session (a take-over the mockup only creates on click) is dropped.
// Deliberate divergences (asserted in DIVERGENCE, never skipped): CI failing is dropped (D9); the 7-day window folds later day bands into Further out (P401); undated tasks leave the plan for the Later tab (P405).

const NOW = 1790067000000 + 220_000;
const TODAY = '2026-09-22';
const SETTINGS = {
  historyDays: 14,
  offDays: [] as string[],
  workWeekendDays: [] as string[],
  workdayHours: 6,
  spanDayShare: 0.5,
  headlessSettingSources: 'all' as const,
};

// The product words differ from the mockup's only by sentence case (P265): compare lowercased.
// Ours dropped the mockup's symbol glyphs from labels (colour and codicons carry them).
const REWORD: [RegExp, string][] = [
  [/^↓(\d+) (.+)$/, '$1 behind $2'],
  [/^↻ (.+)$/, 'after $1'],
  [/^⏳ (.+)$/, 'waits for $1'],
  [/^▶ (?!run$|start$)(.+)$/i, 'start $1'],
];
const plain = (v: string): string => {
  let r = v;
  for (const [re, to] of REWORD) r = r.replace(re, to);
  return r
    .replace(/[←-⯿]️?/gu, '')
    .replace(/\s+/g, ' ')
    .trim();
};
const lc = (v: string | undefined) => (v === undefined ? v : plain(v).toLowerCase());
const lcAll = (v: readonly string[]) => v.map((x) => plain(x).toLowerCase());

/** Branch id -> what the new logic shows instead of the mockup, with the reason. */
const DIVERGENCE: Record<string, { mockup: string; ours: string; why: string }> = {
  b_deps: { mockup: 'CI failing', ours: 'Clean', why: 'D9: no CI badge' },
};

/** P469: the mockup's "Done ›" step action reads "Next stage ›"; asserted below, never skipped. */
const ACTION_DIVERGENCE = { mockup: 'done ›', ours: 'next stage ›' };

/** P401: 7-day window; the mockup's 14-day bands after day 6 fold into its Later band. */
const WINDOW_DIVERGENCE = { windowDays: 7, why: 'P401: 7-day window; later days fold into Later' };

/** P405: the tail band is "Further out" and holds dated work only; undated tasks sit on the Later tab. */
const TAIL_LABEL = 'Further out';

let board: Board;
let sessions: SessionsResult;
let workflows: Workflow[];
let nicks: Map<string, string>;
let oracle: MockupComponent;
let timeline: TimelineView;
let progress: Map<string, TaskProgress>;

/** The pairs the mockup oracle derives from the captures' shared files (the wire no longer carries files). */
const ORACLE_PAIRS: Pair[] = [
  {
    a: 'b_bill',
    b: 'b_sara',
    shared: ['src/payments/client.ts', 'src/payments/invoice.ts'],
    conflicts: ['src/payments/client.ts', 'src/payments/invoice.ts'],
  },
  { a: 'b_billdash', b: 'b_notif', shared: ['src/ui/nav/Sidebar.tsx'], conflicts: [] },
  {
    a: 'b_li',
    b: 'b_search',
    shared: ['src/search/schema.ts'],
    conflicts: ['src/search/schema.ts'],
  },
  { a: 'b_search', b: 'b_deps', shared: ['package.json'], conflicts: [] },
];

function normalizeBoard(raw: Board, sess: SessionsResult): Board {
  const runs = (t: Board['tasks'][number]) =>
    t.runs.map((r) => {
      const stuck = sess.sessions.some(
        (s) =>
          s.mode === 'headless' &&
          s.state === 'running' &&
          s.activity === 'input' &&
          s.taskId === t.id &&
          s.branchId === r.branchId &&
          s.stepId === r.stepId,
      );
      return r.state === 'running' && stuck ? { ...r, state: 'stuck' as const } : r;
    });
  return { ...raw, tasks: raw.tasks.map((t) => ({ ...t, runs: runs(t) })), pairs: ORACLE_PAIRS };
}

beforeAll(() => {
  useClaudeAgentText();
  const rawSessions = loadFixture<SessionsResult>('sessions');
  sessions = { sessions: rawSessions.sessions.filter((s) => s.id !== 'tk01') };
  board = normalizeBoard(loadFixture<Board>('board'), sessions);
  workflows = loadFixture<WorkflowsResult>('workflows').workflows.flatMap((w) =>
    w.workflow && w.error === null ? [w.workflow] : [],
  );
  nicks = new Map(
    loadFixture<ReposResult>('repos').repos.map((r) => [r.codeRepoId, r.nickname || r.name]),
  );
  oracle = runMockupV2();
  const nick = (id: string) => nicks.get(id) ?? id;
  timeline = buildTimeline({
    board,
    settings: SETTINGS,
    today: TODAY,
    localDayOf: (ms) => new Date(ms).toISOString().slice(0, 10),
    repoLabel: nick,
    showAllItems: true,
    showHistory: false,
  });
  const byBranch = new Map(board.branches.map((b) => [b.id, b]));
  progress = new Map(
    board.tasks.map((t) => [
      t.id,
      buildTaskProgress({
        task: t,
        workflow: workflows.find((w) => w.id === t.workflowId) ?? null,
        branch: (id) => byBranch.get(id),
        repoNick: nick,
      }),
    ]),
  );
});

const nick = (id: string) => nicks.get(id) ?? id;
const hoursOf = (sub: string): number => Number.parseFloat(sub) || 0;
const estHours = (est: string): number => {
  const m = /^(\d+(?:\.\d+)?)(d|h)$/.exec(est);
  return m ? Number(m[1]) * (m[2] === 'd' ? SETTINGS.workdayHours : 1) : 0;
};

/** The oracle bands with every day band past the window folded into Later (see WINDOW_DIVERGENCE). */
function windowedBands(): MockupComponent[] {
  const dayOf = (b: MockupComponent): number | null => {
    const m = /^aq-day-(\d+)$/.exec(b.domId);
    return m ? Number(m[1]) : null;
  };
  const isFolded = (b: MockupComponent) => (dayOf(b) ?? -1) >= WINDOW_DIVERGENCE.windowDays;
  const folded = oracle.bands.filter(isFolded);
  expect(folded.length).toBeGreaterThan(0);
  const kept = oracle.bands.filter((b: MockupComponent) => !isFolded(b));
  return kept.flatMap((b: MockupComponent) => {
    if (!b.isLater) return [b];
    const all = [...folded.flatMap((x: MockupComponent) => x.tasks), ...b.tasks];
    const tasks = all.filter((x: MockupComponent) => shelfOf(board, x.id) === 'plan');
    if (!tasks.length) return [];
    const shelved = all.filter((x: MockupComponent) => !tasks.includes(x));
    const hours =
      [...folded, b].reduce((n, x) => n + hoursOf(x.sub), 0) -
      shelved.reduce((n: number, x: MockupComponent) => n + estHours(x._t.est), 0);
    return [{ ...b, label: TAIL_LABEL, tasks, sub: hours ? `${hours}h` : '' }];
  });
}

const boxes = (): MockupComponent[] => oracle.bands.flatMap((b: MockupComponent) => b.tasks);
const boxOf = (id: string): MockupComponent => boxes().find((b) => b.id === id);

describe('timeline', () => {
  test('bands list the same days, labels and hours', () => {
    const ours = timeline.bands.map((b) => [b.label, b.hours ? `${b.hours}h` : '']);
    const theirs = windowedBands().map((b: MockupComponent) => [
      b.label,
      b.sub === 'off' ? '' : b.sub,
    ]);
    expect(ours).toEqual(theirs);
  });

  test('tasks fall in the same order within every band', () => {
    expect(timeline.bands.flatMap((b) => b.taskIds)).toEqual(
      windowedBands()
        .flatMap((b: MockupComponent) => b.tasks)
        .map((b: MockupComponent) => b.id),
    );
  });

  test('overflow and overdue flags agree', () => {
    const ours = timeline.bands.map((b) => [
      b.overflowTaskIds.length > 0,
      b.overdueTaskIds.length > 0,
    ]);
    const theirs = windowedBands().map((b: MockupComponent) => [b.isOver, b.isOverdue]);
    expect(ours).toEqual(theirs);
  });
});

describe('per task', () => {
  const tasks = () => board.tasks.filter((t) => t.kind !== 'review');

  const cellOf = (taskId: string) => {
    const t = board.tasks.find((x) => x.id === taskId);
    if (!t) throw new Error(taskId);
    const branches = t.branchIds.flatMap((id) => board.branches.filter((b) => b.id === id));
    const p = progress.get(t.id) as TaskProgress;
    const status = statusOf(t, board.statuses);
    const blocked = isBlocked(t, p, branches);
    return {
      status: blocked ? 'Blocked' : status.name,
      cell: taskCell({
        task: t,
        progress: p,
        status,
        blocked,
        branches,
        sessions: sessions.sessions.filter((s) => s.taskId === t.id),
      }),
    };
  };

  test('status chip and step action equal the mockup', () => {
    for (const t of tasks()) {
      const box = boxOf(t.id);
      const { cell } = cellOf(t.id);
      expect([t.id, lc(cell?.tag.label)]).toEqual([t.id, lc(box.cells[0].tag)]);
      const expected = lcAll(box.cells[0].btns.map((b: MockupComponent) => b.label)).map((l) =>
        l === ACTION_DIVERGENCE.mockup ? ACTION_DIVERGENCE.ours : l,
      );
      expect([t.id, lcAll(cell?.action ? [cell.action.label] : [])]).toEqual([t.id, expected]);
    }
  });

  test('derived status equals the mockup row status', () => {
    for (const t of tasks().filter((x) => x.kind === 'task')) {
      expect([t.id, cellOf(t.id).status]).toEqual([t.id, boxOf(t.id).rows[0].status]);
    }
  });

  // The mockup groups steps into stages; the flat model counts steps, so only show-bar parity holds.
  test('bar visibility equals the mockup and the label names the current step', () => {
    for (const t of tasks().filter((x) => x.kind === 'task')) {
      const p = progress.get(t.id) as TaskProgress;
      expect([t.id, p.showBar]).toEqual([t.id, boxOf(t.id).rows[0].phase.showBar]);
      expect(p.segments.length).toBe(p.steps.length);
      expect([t.id, p.label.startsWith(p.step?.name ?? 'Done')]).toEqual([t.id, true]);
    }
  });
});

const flat = (parts: readonly (string | { value: string })[]): string =>
  parts.map((p) => (typeof p === 'string' ? p : p.value)).join('');

describe('per branch', () => {
  const byBranch = () => new Map(board.branches.map((b) => [b.id, b]));

  /** Mockup row/cell for a branch, matched by display name and repo (draft rows are `new branch`). */
  function oracleRow(taskId: string, b: Branch): { row: MockupComponent; cell: MockupComponent } {
    const box = boxOf(taskId);
    const offset = box.rows[0].isTask ? 1 : 0;
    const rows: MockupComponent[] = box.rows.slice(offset);
    const hits = rows
      .map((row, i) => ({ row, cell: box.cells[offset + i] }))
      .filter(
        ({ row }) => row.label === (b.name || 'new branch') && row.repo === nick(b.codeRepoId),
      );
    expect(hits).toHaveLength(1);
    return hits[0] as { row: MockupComponent; cell: MockupComponent };
  }

  test('tag, tone-independent label and actions equal the mockup', () => {
    for (const t of board.tasks) {
      for (const id of t.branchIds) {
        const b = byBranch().get(id) as Branch;
        const { cell } = oracleRow(t.id, b);
        const ours = branchTag({
          branch: b,
          graph: timeline.graph,
          plan: board.plan,
          after: timeline.after,
          runs: [],
          nowMs: NOW,
          hadSession: sessions.sessions.some((s) => s.branchId === b.id),
          mainName: 'main',
        });
        const expected = DIVERGENCE[b.id];
        if (expected) {
          expect([b.id, cell.tag]).toEqual([b.id, expected.mockup]);
          expect([b.id, ours.label]).toEqual([b.id, expected.ours]);
          continue;
        }
        expect([b.id, lc(ours.label)]).toEqual([b.id, lc(cell.tag)]);
        expect([b.id, lcAll(ours.actions.map((a) => a.label))]).toEqual([
          b.id,
          lcAll(cell.btns.map((x: MockupComponent) => x.label)),
        ]);
      }
    }
  });

  test('rows come in the same order', () => {
    for (const t of board.tasks) {
      const box = boxOf(t.id);
      const rows: MockupComponent[] = box.rows.slice(box.rows[0].isTask ? 1 : 0);
      const ours = timeline
        .branchRows(t.id)
        .map((r) => byBranch().get(r.id) as Branch)
        .map((b) => `${b.name || 'new branch'}|${nick(b.codeRepoId)}`);
      expect([t.id, ours]).toEqual([t.id, rows.map((r) => `${r.label}|${r.repo}`)]);
    }
  });

  test('base marker label and tone equal the mockup', () => {
    for (const t of board.tasks) {
      for (const id of t.branchIds) {
        const b = byBranch().get(id) as Branch;
        const { row } = oracleRow(t.id, b);
        const m = baseMarker(b, timeline.graph, 'main');
        expect([b.id, flat(m?.label ?? [])]).toEqual([b.id, row.baseLabel]);
        if (m) {
          expect([b.id, m.tone === 'blue']).toEqual([b.id, row.baseStyle.includes('#93b6ff')]);
          expect([b.id, lc(flat(m.tip))]).toEqual([b.id, lc(row.baseTip)]);
        }
      }
    }
  });
});

describe('branch second line', () => {
  test('context, merged and deployed chips equal the mockup', () => {
    const prs = loadFixture<ChangeRequestsResult>('change-requests');
    for (const t of board.tasks) {
      for (const b of t.branchIds.flatMap((id) => board.branches.filter((x) => x.id === id))) {
        const box = boxOf(t.id);
        const offset = box.rows[0].isTask ? 1 : 0;
        const i = box.rows
          .slice(offset)
          .findIndex(
            (r: MockupComponent) =>
              r.label === (b.name || 'new branch') && r.repo === nick(b.codeRepoId),
          );
        const row = box.rows[offset + i];
        const line = branchLine2(b, timeline.graph, prs.branches[b.id]?.title ?? '');
        // Re-merge is out of scope (P146 R5): the mockup's `Right-click to re-merge.` tip suffix is dropped.
        const pick = (c: { label: string; tip: string }) => [
          c.label,
          c.tip
            .replace('. Right-click to re-merge.', '')
            .replace(
              /^stale in (\S+): has changes not in \S+$/i,
              'stale in $1, missing changes of this branch',
            )
            .replace(/^(\S+): deployed sha unknown$/i, 'deployed sha of $1 unknown')
            .toLowerCase(),
        ];
        expect([b.id, lc(line.context)]).toEqual([b.id, lc(row.hasSub ? row.sub : '')]);
        expect([b.id, line.merged.map(pick)]).toEqual([b.id, row.into.map(pick)]);
        expect([b.id, line.deployed.map(pick)]).toEqual([b.id, row.deploys.map(pick)]);
      }
    }
  });
});

describe('Needs you', () => {
  const ours = () =>
    buildNeedsYou({
      board,
      sessions: sessions.sessions,
      progress,
      repoNick: nick,
      nowMs: NOW,
    });

  test('same items (kind, text, action, scope) as the mockup', () => {
    const key = (kind: string, what: string, action: string, scope: string) =>
      [kind, what, action, scope].join('|');
    // The product words differ from the mockup's by the P265 style rules: curly quotes, a capitalised scope,
    // and the catalog's "Each repository" for the mockup's "each repo".
    const curly = (s: string) => s.replace(/"([^"]*)"/g, '“$1”');
    const theirs = oracle.needs.items
      .map((n: MockupComponent) =>
        key(
          n.kind,
          curly(n.what),
          n.btn,
          (n.repo ?? '').toLowerCase().replace(/^each repo$/, 'each repository'),
        ),
      )
      .sort();
    const mine = ours()
      .items.map((n) => key(n.kind, n.what, n.action, n.scope.toLowerCase()))
      .sort();
    expect(mine).toEqual(theirs);
  });

  test('ordered by kind rank as SPEC2 §11 lists them, then oldest first', () => {
    const items = ours().items;
    const ranks = items.map((n) => NEEDS_RANK.indexOf(n.kind));
    expect(ranks).toEqual([...ranks].sort((a, b) => a - b));
    const stuck = items.filter((n) => n.kind === 'stuck run');
    expect(stuck.map((n) => n.ageMs)).toEqual(
      [...stuck.map((n) => n.ageMs)].sort((a, b) => (b ?? 0) - (a ?? 0)),
    );
  });

  test('badge counts the items; footer drops the CI count', () => {
    const n = ours();
    expect(n.badge).toBe(oracle.needs.items.length);
    const singular = (s: string) =>
      s.replace(/\b1 (background run|interactive session)s\b/g, '1 $1');
    expect(n.footer).toBe(singular(oracle.needs.load.replace(/ · \d+ waiting on CI/, '')));
  });
});

// The mockup has no rebase-conflict check (D1), so these states have no oracle: fixed expectations.
describe('rebase-conflict check states', () => {
  const tagFor = (over: Partial<Branch>) => {
    const b = mkBranch({ id: 'b', taskId: 't', setup: null, ...over });
    const graph = buildBranchGraph({
      tasks: [mkTask({ id: 't', branchIds: ['b'] })],
      branches: [b],
      plan: mkPlan(),
      pairs: [],
    });
    const t = branchTag({
      branch: b,
      graph,
      plan: mkPlan(),
      after: new Map(),
      runs: [],
      nowMs: 0,
      hadSession: true,
      mainName: 'main',
    });
    return [t.label, t.actions.map((a) => a.label), t.tip];
  };

  test('done with paths is a conflict with Rebase, done without is clean', () => {
    expect(tagFor({ conflictsIfRebased: ['a.ts', 'b.ts'] })).toEqual([
      'Conflict',
      ['Rebase'],
      'Conflicts with main if rebased: a.ts, b.ts',
    ]);
    expect(tagFor({})[0]).toBe('Clean');
  });

  test('checking and failed never read as clean', () => {
    expect(tagFor({ conflictCheck: 'checking' })[0]).toBe('Checking…');
    expect(
      tagFor({ conflictCheck: 'failed', conflictCheckReason: 'git 2.30 is older than 2.38' }),
    ).toEqual(['Conflict check failed', [], 'git 2.30 is older than 2.38']);
  });
});

describe('script step recovery', () => {
  const actionFor = (state: 'failed' | 'stuck') => {
    const script = mkStep({
      id: 'release',
      kind: 'script',
      command: 'make release',
      runsOn: 'each repo',
    });
    const b = mkBranch({ id: 'b', taskId: 't', setup: null });
    const t = mkTask({
      id: 't',
      branchIds: ['b'],
      workflowId: 'w',
      entryStepId: 'release',
      runs: [mkRun({ taskId: 't', stepId: 'release', branchId: 'b', state })],
    });
    const p = buildTaskProgress({
      task: t,
      workflow: { id: 'w', name: 'w', steps: [script] },
      branch: () => b,
      repoNick: (id) => id,
    });
    const status = statusOf(t, DEFAULT_STATUSES);
    const blocked = isBlocked(t, p, [b]);
    return taskCell({ task: t, progress: p, status, blocked, branches: [b], sessions: [] })?.action;
  };

  test('a stuck script run offers Retry like a failed one (R23)', () => {
    expect(actionFor('failed')?.kind).toBe('retry');
    expect(actionFor('stuck')?.kind).toBe('retry');
  });
});

describe('Needs you extras (R16)', () => {
  const needsFor = (sessionId: string, finishedAt: number | null) => {
    const script = mkStep({
      id: 'release',
      kind: 'script',
      command: 'make release',
      runsOn: 'each repo',
    });
    const b = mkBranch({ id: 'b', taskId: 't', setup: null });
    const t = mkTask({
      id: 't',
      branchIds: ['b'],
      workflowId: 'w',
      entryStepId: 'release',
      runs: [
        mkRun({
          id: 'run-1',
          taskId: 't',
          stepId: 'release',
          branchId: 'b',
          state: 'stuck',
          note: 'timed out after 10m',
          sessionId,
          finishedAt,
        }),
      ],
    });
    const p = buildTaskProgress({
      task: t,
      workflow: { id: 'w', name: 'w', steps: [script] },
      branch: () => b,
      repoNick: (id) => id,
    });
    return buildNeedsYou({
      board: { tasks: [t], branches: [b] },
      sessions: [],
      progress: new Map([['t', p]]),
      repoNick: (id) => id,
      nowMs: 10_000,
    }).items.filter((n) => n.kind === 'stuck run');
  };

  test('a stuck script run is retried, carries its run id and note, and ages from its finish', () => {
    const [n] = needsFor('', 4_000);
    expect(n?.action).toBe('Retry');
    expect(n?.runIds).toEqual(['run-1']);
    expect(n?.detail).toBe('timed out after 10m');
    expect(n?.ageMs).toBe(6_000);
  });

  test('a stuck run with a session is taken over', () => {
    const [n] = needsFor('s1', 4_000);
    expect(n?.action).toBe('Take over');
    expect(n?.sessionId).toBe('s1');
  });
});
