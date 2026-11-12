import type { Settings } from '../../../state/settingsDomain';
import type { Task } from '../wire';
import { type BranchGraph, buildBranchGraph, type GraphInput } from './branchGraph';
import { buildCalendar, type Calendar, isoToOffset, LATER, parseEst, spanDays } from './calendar';
import { shelfIds, shelfOf } from './shelf';

// The task list order: Plan sequence (merge order), then Later, then Backlog. `buildTimeline` and
// the Go port `internal/ade/taskorder.go` compute it from the same rules; see
// `tests/unit/ade-v2-task-order-parity.spec.ts`.

export interface TimelineEntry {
  taskId: string;
  kind: Task['kind'];
  /** Start day offset from today, `LATER` when undated (shelved, or an unplaced review item). A review item with no plan day takes its placer's day. */
  day: number;
  /** Merge day (last working day of the span). */
  end: number;
  days: number[];
  span: number;
  /** Position in `plan.order`, 999 when absent. */
  pos: number;
  est: { hours: number; days: number } | null;
  /** Every `mine` branch merged. */
  done: boolean;
  /** A review item placed by the merge-order rule: it has no plan day. */
  auto: boolean;
}

/** Done when every branch of the task's own kind (`mine`, or `review` for a review item) merged into main. */
function isDoneTask(t: Task, graph: BranchGraph, kind: 'mine' | 'review' = 'mine'): boolean {
  const own = t.branchIds.filter((id) => graph.byBranch.get(id)?.kind === kind);
  return own.length > 0 && own.every((id) => graph.byBranch.get(id)?.mergedIntoMain === true);
}

function reviewEntry(taskId: string, day: number, end: number): TimelineEntry {
  return {
    taskId,
    kind: 'review',
    day,
    end,
    days: [day],
    span: 1,
    pos: 999,
    est: null,
    done: false,
    auto: true,
  };
}

/** Planned tasks and pinned review items (a review with a plan day) sorted by (merge day, start day, plan position). */
function buildMineEntries(
  board: GraphInput,
  settings: Settings['ade'],
  today: string,
  graph: BranchGraph,
  cal: Calendar,
): TimelineEntry[] {
  const out: TimelineEntry[] = [];
  for (const t of board.tasks) {
    const iso = board.plan.day[t.id];
    const review = t.kind === 'review';
    if (review && iso === undefined) continue;
    const day = iso === undefined ? LATER : isoToOffset(today, iso);
    const p = board.plan.order.indexOf(t.id);
    const est = review ? null : parseEst(t.est, settings.workdayHours, settings.spanDayShare);
    const span = day === LATER ? 1 : (est?.days ?? 1);
    const days = day === LATER ? [LATER] : spanDays(cal, day, span);
    out.push({
      taskId: t.id,
      kind: t.kind,
      day,
      end: days.at(-1) as number,
      days,
      span,
      pos: p < 0 ? 999 : p,
      est,
      done: isDoneTask(t, graph, review ? 'review' : 'mine'),
      auto: false,
    });
  }
  return out.sort((a, b) => a.end - b.end || a.day - b.day || a.pos - b.pos);
}

function buildsOnOrConflicts(t: Task, rb: string, graph: BranchGraph): boolean {
  return t.branchIds.some(
    (bid) =>
      graph.ancestors(bid).includes(rb) ||
      (graph.conflicts.get(bid) ?? []).some((c) => c.with === rb),
  );
}

/** Merge order: a review item sits right above the first task that builds on it or conflicts with it. */
function placeReviews(
  mine: TimelineEntry[],
  reviews: readonly Task[],
  graph: BranchGraph,
): TimelineEntry[] {
  const seq: TimelineEntry[] = [];
  const placed = new Set<string>();
  for (const me of mine) {
    if (me.kind === 'review') {
      seq.push(me);
      continue;
    }
    const t = graph.byTask.get(me.taskId) as Task;
    for (const r of reviews) {
      const rb = r.branchIds[0];
      if (placed.has(r.id) || rb === undefined || !buildsOnOrConflicts(t, rb, graph)) continue;
      placed.add(r.id);
      seq.push(reviewEntry(r.id, me.day, me.end));
    }
    seq.push(me);
  }
  for (const r of reviews) if (!placed.has(r.id)) seq.push(reviewEntry(r.id, LATER, LATER));
  return seq;
}

export interface PlanSequence {
  seq: TimelineEntry[];
  /** Every planned task and pinned review item, shelved ones included. */
  mineAll: TimelineEntry[];
  graph: BranchGraph;
  cal: Calendar;
}

/** Merge order incl. review items, before the first-10 cap and the repo filter. */
export function planSequence(
  board: GraphInput,
  settings: Settings['ade'],
  today: string,
): PlanSequence {
  const graph = buildBranchGraph(board);
  const cal = buildCalendar(today, settings);
  const mineAll = buildMineEntries(board, settings, today, graph, cal);
  // Shelved tasks (Later, Backlog) leave the Plan sequence but keep an entry for the panel and menu.
  const onPlan = (e: TimelineEntry): boolean => shelfOf(board, e.taskId) === 'plan';
  const mine = mineAll.filter(onPlan);
  const reviews = board.tasks.filter(
    (t) => t.kind === 'review' && board.plan.day[t.id] === undefined,
  );
  return { seq: placeReviews(mine, reviews, graph), mineAll, graph, cal };
}

/** Every task id in task list order: the Plan sequence, then Later, then Backlog. */
export function taskListOrder(
  board: GraphInput,
  settings: Settings['ade'],
  today: string,
): string[] {
  const { seq } = planSequence(board, settings, today);
  return [...seq.map((e) => e.taskId), ...shelfIds(board, 'later'), ...shelfIds(board, 'backlog')];
}
