import { t } from '@theme/i18n';
import type { Settings } from '../../../state/settingsDomain';
import type { Board, Branch, HistoryEntry, Task } from '../wire';
import { type BranchGraph, buildBranchGraph, isDraft, shortBranchName } from './branchGraph';
import {
  buildCalendar,
  type Calendar,
  dateParts,
  dayLabel,
  isDayOff,
  isoToDays,
  isoToOffset,
  isWeekend,
  LATER,
  monthLabel,
  nextWork,
  parseEst,
  spanDays,
} from './calendar';
import { shelfOf } from './shelf';

// Pure port of mockup v2 `renderVals()` lines 1527-1572 (task order) and 1864-1985 (first-10 cap,
// hours, bands) on the wire types. No clock read: `today` and `localDayOf` are inputs.

/** SPEC2 §4: only the first 10 work items are shown until the user loads the rest. */
export const ITEM_LIMIT = 10;
/** P401: day bands shown from today; work dated further out sits in the tail band. */
export const WINDOW_DAYS = 7;

const bandKey = (e: TimelineEntry): number =>
  e.day === LATER || e.day >= WINDOW_DAYS ? LATER : e.day;

type TimelineBoard = Pick<Board, 'tasks' | 'branches' | 'plan' | 'pairs' | 'history'>;

export interface TimelineInput {
  board: TimelineBoard;
  settings: Settings['ade'];
  /** Local `YYYY-MM-DD`. */
  today: string;
  /** An epoch ms as its own local `YYYY-MM-DD` (history `archivedAt`). */
  localDayOf: (ms: number) => string;
  /** Display name of a repo; orders a task's branch rows. */
  repoLabel: (codeRepoId: string) => string;
  showAllItems: boolean;
  showHistory: boolean;
  /** Repos hidden by the repo filter; a task is visible when it has no branch or one in a shown repo. */
  hiddenRepoIds?: ReadonlySet<string>;
}

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

interface TimelineSpan {
  taskId: string;
  /** 1-based index of this day inside the task's span. */
  dayNumber: number;
  dayCount: number;
  merges: boolean;
  startDay: number;
}

interface TimelineHistoryItem {
  taskId: string;
  title: string;
  day: number;
  codeRepoIds: string[];
  mergedAt: number | null;
}

export interface TimelineBand {
  key: number;
  isLater: boolean;
  label: string;
  /** Planned hours rounded to 0.1, 0 = none. */
  hours: number;
  taskIds: string[];
  spans: TimelineSpan[];
  history: TimelineHistoryItem[];
  isToday: boolean;
  isPast: boolean;
  weekend: boolean;
  dayOff: boolean;
  isMonday: boolean;
  overdueTaskIds: string[];
  overflowTaskIds: string[];
  /** Hours over the workday cap, 0 when not over. */
  overflowHours: number;
  /** Day the overflow moves to; `LATER` for the Later band. */
  overflowMoveTo: number;
}

export interface TimelineView {
  /** Merge order incl. review items, before the first-10 cap and the repo filter. */
  seq: TimelineEntry[];
  entries: ReadonlyMap<string, TimelineEntry>;
  shownTaskIds: string[];
  hiddenCount: number;
  /** `showAllItems` is on and there is something to collapse. */
  canCollapse: boolean;
  /** 1-based merge number among planned (non-parked, non-review) tasks, in `seq` order. */
  mergeOrder: ReadonlyMap<string, number>;
  /** Branch -> earlier-merging branch of another task it shares files with (the `↻` rebase). */
  after: ReadonlyMap<string, { id: string; file: string }>;
  /** Branch rows of a task in display order: repo, then stacked children indented. */
  branchRows(taskId: string): { id: string; depth: number }[];
  bands: TimelineBand[];
  /** Every archived task, newest first, with its day offset (`< 0` = before today). */
  history: TimelineHistoryItem[];
  historyCount: number;
  historyTip: string;
  moreButtonLabel: string;
  graph: BranchGraph;
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
  input: TimelineInput,
  graph: BranchGraph,
  cal: Calendar,
): TimelineEntry[] {
  const { board, settings, today } = input;
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

function makeBranchRows(
  graph: BranchGraph,
  repoLabel: (id: string) => string,
): (taskId: string) => { id: string; depth: number }[] {
  return (taskId) => {
    const t = graph.byTask.get(taskId);
    if (!t) return [];
    const inTask = (id: string): boolean => graph.byBranch.get(id)?.taskId === taskId;
    const label = (id: string): string => repoLabel(graph.byBranch.get(id)?.codeRepoId ?? '');
    const sorted = [...t.branchIds].sort((a, b) =>
      label(a) < label(b) ? -1 : label(a) > label(b) ? 1 : 0,
    );
    const out: { id: string; depth: number }[] = [];
    const emitted = new Set<string>();
    const walk = (id: string, depth: number): void => {
      if (emitted.has(id)) return;
      emitted.add(id);
      out.push({ id, depth });
      for (const c of graph.kids.get(id) ?? []) if (inTask(c)) walk(c, depth + 1);
    };
    for (const id of sorted) {
      const p = graph.parentOf.get(id);
      if (p === undefined || !inTask(p)) walk(id, 0);
    }
    // A base/queue cycle leaves no root: render its rows anyway.
    for (const id of sorted) walk(id, 0);
    return out;
  };
}

function isLiveMine(b: Branch | undefined): b is Branch {
  return b?.kind === 'mine' && !isDraft(b) && !b.mergedIntoMain;
}

/** Shares files with mine work (another task, same repo) that merges earlier: must rebase after it. */
function computeAfter(
  seq: readonly TimelineEntry[],
  branchRows: (taskId: string) => { id: string }[],
  graph: BranchGraph,
): Map<string, { id: string; file: string }> {
  const after = new Map<string, { id: string; file: string }>();
  const order: string[] = [];
  for (const e of seq) for (const r of branchRows(e.taskId)) order.push(r.id);
  order.forEach((bid, i) => {
    const b = graph.byBranch.get(bid);
    if (!isLiveMine(b) || graph.parentOf.has(bid)) return;
    for (let j = i - 1; j >= 0; j--) {
      const a = graph.byBranch.get(order[j] as string);
      if (!isLiveMine(a) || a.codeRepoId !== b.codeRepoId || a.taskId === b.taskId) continue;
      const sh = graph.shared(a.id, bid);
      if (sh.length) {
        after.set(bid, { id: a.id, file: (sh[0] as string).split('/').pop() as string });
        break;
      }
    }
  });
  return after;
}

interface Hours {
  on: ReadonlyMap<number, number>;
  overflowOf(dk: number): string[];
}

function buildHours(seq: readonly TimelineEntry[], cap: number): Hours {
  const on = new Map<number, number>();
  const taskHours = new Map<string, number>();
  const add = (k: number, h: number): void => {
    on.set(k, (on.get(k) ?? 0) + h);
  };
  for (const e of seq) {
    if (e.kind === 'review' || !e.est) continue;
    if (e.est.days > 1 && e.day !== LATER) {
      for (const d of e.days) add(d, e.est.hours / e.est.days);
      taskHours.set(e.taskId, e.est.hours / e.est.days);
    } else {
      add(e.day, e.est.hours);
      taskHours.set(e.taskId, e.est.hours);
    }
  }
  const overflowOf = (dk: number): string[] => {
    const total = on.get(dk) ?? 0;
    if (dk === LATER || total <= cap + 0.01) return [];
    const starts = seq.filter((e) => e.day === dk && e.kind !== 'review' && !e.done);
    const out: string[] = [];
    let over = total - cap;
    for (let i = starts.length - 1; i >= 0 && over > 0.01; i--) {
      const e = starts[i] as TimelineEntry;
      out.unshift(e.taskId);
      over -= taskHours.get(e.taskId) ?? 0;
    }
    return out;
  };
  return { on, overflowOf };
}

interface BandCtx {
  cal: Calendar;
  cap: number;
  seq: readonly TimelineEntry[];
  entries: ReadonlyMap<string, TimelineEntry>;
  shown: ReadonlySet<string>;
  shownTaskIds: readonly string[];
  hours: Hours;
  history: readonly TimelineHistoryItem[];
  showHistory: boolean;
}

function spansOn(ctx: BandCtx, dk: number): TimelineSpan[] {
  const out: TimelineSpan[] = [];
  for (const e of ctx.seq) {
    if (e.days.length < 2 || !ctx.shown.has(e.taskId)) continue;
    const idx = e.days.indexOf(dk);
    if (idx <= 0) continue;
    out.push({
      taskId: e.taskId,
      dayNumber: idx + 1,
      dayCount: e.days.length,
      merges: idx === e.days.length - 1,
      startDay: e.day,
    });
  }
  return out;
}

function buildBand(ctx: BandCtx, dk: number, label: string): TimelineBand {
  const { cal, cap } = ctx;
  const isLater = dk === LATER;
  let rawHours = ctx.hours.on.get(dk) ?? 0;
  if (isLater)
    for (const [k, h] of ctx.hours.on) if (k >= WINDOW_DAYS && k !== LATER) rawHours += h;
  const hours = Math.round(rawHours * 10) / 10;
  const isPast = dk < 0 && !isLater;
  const weekend = !isLater && isWeekend(cal, dk) && !cal.workWeekend.has(dk);
  const dayOff = !isLater && isDayOff(cal, dk);
  const overdue = isPast
    ? ctx.seq.filter((e) => e.day === dk && !e.auto && e.end < 0 && !e.done).map((e) => e.taskId)
    : [];
  const overflow = isPast || weekend || dayOff ? [] : ctx.hours.overflowOf(dk);
  return {
    key: dk,
    isLater,
    label,
    hours,
    taskIds: ctx.shownTaskIds.filter((id) => bandKey(ctx.entries.get(id) as TimelineEntry) === dk),
    spans: spansOn(ctx, dk),
    history: ctx.showHistory ? ctx.history.filter((h) => h.day === dk) : [],
    isToday: dk === 0,
    isPast,
    weekend,
    dayOff,
    isMonday: !isLater && dateParts(cal, dk).dow === 1,
    overdueTaskIds: overdue,
    overflowTaskIds: overflow,
    overflowHours: overflow.length ? Math.round((hours - cap) * 10) / 10 : 0,
    overflowMoveTo: isLater ? LATER : nextWork(cal, dk),
  };
}

/** Band label with the month suffix on the first day of a new month, as the mockup renders it. */
function bandLabels(cal: Calendar, dayKeys: readonly number[]): string[] {
  let prevMonth: number | null = null;
  return dayKeys.map((dk) => {
    if (dk === LATER) return t('space.ade.plan.band.further.label');
    let label = dayLabel(cal, dk);
    if (dk === 0) {
      prevMonth = dateParts(cal, 0).month;
    } else if (dk !== LATER) {
      const p = dateParts(cal, dk);
      if (prevMonth !== null && p.month !== prevMonth) label += ` ${monthLabel(p.month)}`;
      prevMonth = p.month;
    }
    return label;
  });
}

function collectDayKeys(
  input: TimelineInput,
  seq: readonly TimelineEntry[],
  shown: ReadonlySet<string>,
  hiddenCount: number,
  lastShownDay: number,
): number[] {
  const { settings, today } = input;
  const keys = new Set<number>();
  if (input.showHistory) for (let h = -settings.historyDays; h < 0; h++) keys.add(h);
  for (let k = 0; k < WINDOW_DAYS; k++) {
    if (input.showAllItems || !hiddenCount || k <= lastShownDay) keys.add(k);
  }
  for (const e of seq) {
    if (!shown.has(e.taskId)) continue;
    for (const d of e.days) if (d < WINDOW_DAYS) keys.add(d);
  }
  for (const iso of settings.offDays) {
    const k = isoToOffset(today, iso);
    if (k >= 0 && k < WINDOW_DAYS) keys.add(k);
  }
  const dayKeys = [...keys].sort((a, b) => a - b);
  if (seq.some((e) => shown.has(e.taskId) && bandKey(e) === LATER)) dayKeys.push(LATER);
  return dayKeys;
}

export function buildTimeline(input: TimelineInput): TimelineView {
  const { board, settings, today } = input;
  const graph = buildBranchGraph(board);
  const cal = buildCalendar(today, settings);

  const mineAll = buildMineEntries(input, graph, cal);
  // Shelved tasks (Later, Backlog) leave the Plan sequence but keep an entry for the panel and menu.
  const onPlan = (e: TimelineEntry): boolean => shelfOf(board, e.taskId) === 'plan';
  const mine = mineAll.filter(onPlan);
  const reviews = board.tasks.filter(
    (t) => t.kind === 'review' && board.plan.day[t.id] === undefined,
  );
  const seq = placeReviews(mine, reviews, graph);
  const entries = new Map<string, TimelineEntry>();
  for (const e of mineAll) entries.set(e.taskId, e);
  for (const e of seq) entries.set(e.taskId, e);

  const mergeOrder = new Map<string, number>();
  for (const e of seq) if (e.kind === 'task') mergeOrder.set(e.taskId, mergeOrder.size + 1);

  const branchRows = makeBranchRows(graph, input.repoLabel);
  const after = computeAfter(seq, branchRows, graph);

  // First-10 cap and the repo filter. Only visible items count toward the cap.
  const hidden = input.hiddenRepoIds;
  const visSeq = seq.filter((e) => {
    const ids = (graph.byTask.get(e.taskId) as Task).branchIds;
    if (!hidden?.size || ids.length === 0) return true;
    return ids.some((id) => !hidden.has(graph.byBranch.get(id)?.codeRepoId ?? ''));
  });
  const shownTaskIds = visSeq
    .filter((_, i) => input.showAllItems || i < ITEM_LIMIT)
    .map((e) => e.taskId);
  const shown = new Set(shownTaskIds);
  const hiddenCount = visSeq.length - shown.size;
  const lastShownDay = Math.max(
    -1e9,
    ...visSeq.filter((e) => shown.has(e.taskId) && e.day !== LATER).map((e) => e.end),
  );

  const dayKeys = collectDayKeys(input, seq, shown, hiddenCount, lastShownDay);
  const history = historyItems(board.history, today, input.localDayOf);
  const ctx: BandCtx = {
    cal,
    cap: settings.workdayHours,
    seq,
    entries,
    shown,
    shownTaskIds,
    hours: buildHours(seq, settings.workdayHours),
    history,
    showHistory: input.showHistory,
  };
  const labels = bandLabels(cal, dayKeys);
  const bands = dayKeys.map((dk, i) => buildBand(ctx, dk, labels[i] as string));

  const historyCount = history.filter((h) => h.day >= -settings.historyDays).length;
  return {
    seq,
    entries,
    shownTaskIds,
    hiddenCount,
    canCollapse: input.showAllItems && visSeq.length > ITEM_LIMIT,
    mergeOrder,
    after,
    branchRows,
    bands,
    history,
    historyCount,
    historyTip: historyLabel(historyCount, settings.historyDays),
    moreButtonLabel: t('space.ade.plan.view.loadAll.label', { count: hiddenCount }),
    graph,
  };
}

function historyLabel(count: number, days: number): string {
  const tasks = t('space.ade.plan.view.archivedTasks.label', { count });
  return days % 7 === 0
    ? t('space.ade.plan.view.historyWeeks.tooltip', { tasks, count: days / 7 })
    : t('space.ade.plan.view.historyDays.tooltip', { tasks, count: days });
}

function historyItems(
  history: readonly HistoryEntry[],
  today: string,
  localDayOf: (ms: number) => string,
): TimelineHistoryItem[] {
  const todayDays = isoToDays(today);
  return history.map((h) => ({
    taskId: h.taskId,
    title: h.title,
    day: isoToDays(localDayOf(h.archivedAt)) - todayDays,
    codeRepoIds: h.codeRepoIds,
    mergedAt: h.mergedAt,
  }));
}

export interface Ripple {
  branchIds: string[];
  taskIds: string[];
  /** `—` when the selection is not a `mine` task/branch; else `rebase a, b` or `nothing to rebase`. */
  text: string;
  tone: 'amber' | 'grey';
}

/** Selection ripple and the "On merge" line: branches that must rebase when the selection merges. */
export function rippleOf(
  view: Pick<TimelineView, 'after' | 'graph'>,
  selection: { kind: 'task' | 'branch'; id: string },
): Ripple {
  const { graph, after } = view;
  const roots: string[] = [];
  if (selection.kind === 'task') {
    const t = graph.byTask.get(selection.id);
    if (t?.kind !== 'task') return { branchIds: [], taskIds: [], text: '—', tone: 'grey' };
    for (const id of t.branchIds) if (graph.byBranch.get(id)?.kind === 'mine') roots.push(id);
  } else {
    const b = graph.byBranch.get(selection.id);
    if (b?.kind !== 'mine') return { branchIds: [], taskIds: [], text: '—', tone: 'grey' };
    roots.push(selection.id);
  }
  const rootSet = new Set(roots);
  const out: string[] = [];
  const add = (id: string): void => {
    if (!rootSet.has(id) && !out.includes(id)) out.push(id);
  };
  const seen = new Set<string>();
  const down = (id: string): void => {
    if (seen.has(id)) return;
    seen.add(id);
    for (const c of graph.kids.get(id) ?? []) {
      if (graph.byBranch.get(c)?.kind !== 'mine') continue;
      add(c);
      down(c);
    }
  };
  for (const r of roots) down(r);
  for (const [bid, a] of after) {
    if (!rootSet.has(a.id)) continue;
    add(bid);
    down(bid);
  }
  const names = out.map((id) => shortBranchName(graph.byBranch.get(id)?.name ?? id));
  const taskIds: string[] = [];
  for (const id of out) {
    const tid = graph.byBranch.get(id)?.taskId;
    if (tid !== undefined && !taskIds.includes(tid)) taskIds.push(tid);
  }
  return {
    branchIds: out,
    taskIds,
    text: out.length
      ? t('space.ade.board.rebaseList.label', { names: names.join(', ') })
      : t('space.ade.board.nothingToRebase.label'),
    tone: out.length ? 'amber' : 'grey',
  };
}
