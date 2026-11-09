import type { AgentActivity } from '@shared/domain/agent';
import { t } from '@theme/i18n';
import { createSharedComposable, useIntervalFn } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { computed, ref, watch } from 'vue';
import { useRepos } from '../../../repo/state/reposQueries';
import { useCodeReposStore } from '../../../state/coderepos';
import { useSettingsStore } from '../../../state/settings';
import { useAgentSessionsStore } from '../../state';
import { withTuiActivity } from '../activity';
import {
  type BranchTag,
  branchTag,
  type TaskAction,
  type TaskCell,
  taskCell,
} from '../board/actions';
import { type BaseMarker, baseMarker } from '../board/baseMarker';
import { isDraft } from '../board/branchGraph';
import {
  buildCalendar,
  type Calendar,
  dateParts,
  dayLabel,
  LATER,
  monthLabel,
} from '../board/calendar';
import { isWorkKind } from '../board/kind';
import { type BranchLine2, branchLine2, taskTitle } from '../board/labels';
import { buildNeedsYou, type NeedsItem, type NeedsKind } from '../board/needsYou';
import {
  type BranchProgress,
  type BranchSegState,
  branchProgress,
  buildTaskProgress,
  type TaskProgress,
} from '../board/progress';
import { shelfOf, type TaskShelf } from '../board/shelf';
import { blocked as isBlocked, statusOf } from '../board/status';
import { buildStepBlocks, type StepBlock } from '../board/stepBlocks';
import { type TaskSessions, taskSessionsOf } from '../board/taskSessions';
import {
  buildTimeline,
  type Ripple,
  rippleOf,
  type TimelineEntry,
  type TimelineView,
  WINDOW_DAYS,
} from '../board/timeline';
import { localIso, localIsoOfMs } from '../localDay';
import { taskColor } from '../palette';
import { useBoard, useChangeRequests, useSessions, useWorkflows } from '../queries';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import type { Board, Branch, Session, Task, TaskStatusDef, Workflow } from '../wire';
import { type AttachFact, attachOf } from './attach';

/** Needs-you kinds that put a `!` on a card and row: a waiting question, a stuck run. */
const ASKS: ReadonlySet<NeedsKind> = new Set(['question', 'stuck run']);

export interface BranchRowModel {
  id: string;
  depth: number;
  branch: Branch;
  repo: string;
  base: BaseMarker | null;
  draft: boolean;
  /** Branch name, `New branch` until created. */
  name: string;
  isReview: boolean;
  /** Second line: `no branch yet · from main`, or a review item's PR title. */
  context: string;
  tag: BranchTag;
  /** Own progress line while the task is in an agent or script step. */
  prog: { segs: BranchSegState[]; label: string; tone: BranchProgress['tone']; tip: string } | null;
  ripple: boolean;
  /** Merged-into and deployed chips of the second line, nothing for a branch not in any target. */
  chips: Pick<BranchLine2, 'merged' | 'deployed' | 'divider'>;
  /** Tooltip of the `!` circle; `''` when the branch needs nothing from the user. */
  attention: string;
  /** What a click on the `!` does. */
  attentionItem: NeedsItem | null;
  /** Detached or shared glyph and its tooltip; `null` for a row that is neither. */
  attach: AttachFact | null;
}

/** A live child of an orchestrator, one row of its card. */
interface ChildModel {
  id: string;
  title: string;
  status: TaskStatusDef;
  /** Step label of the child's progress, `Done` once finished. */
  step: string;
  finished: boolean;
}

/** The orchestrator a task was spawned by. A parent off the board is archived. */
export interface ParentModel {
  id: string;
  title: string;
  archived: boolean;
}

export interface CardModel {
  task: Task;
  entry: TimelineEntry;
  color: string;
  title: string;
  /** Title with no custom name set: the Name field placeholder. */
  defaultTitle: string;
  progress: TaskProgress;
  /** The task's workflow file, `null` when it has none or the file is gone. */
  workflow: Workflow | null;
  /** Panel Workflow block: one per step; empty for a review or parked task. */
  blocks: StepBlock[];
  status: TaskStatusDef;
  /** A stuck or failed run or setup, shown beside the status. */
  blocked: boolean;
  tag: TaskCell['tag'] | null;
  /** Task step action; which kinds render is `useTaskAction`'s call. */
  action: TaskAction | null;
  /** `3d → Mon 28 · PAY-102 · api · web-app`. */
  meta: string;
  attention: string;
  attentionItem: NeedsItem | null;
  /** The task's agent sessions as one state, `null` when it has none. */
  sessions: TaskSessions | null;
  rows: BranchRowModel[];
  review: boolean;
  parked: boolean;
  orchestrator: boolean;
  /** The spawning orchestrator; `null` for a task nobody spawned. */
  parent: ParentModel | null;
  /** Live children of an orchestrator, in board order. */
  children: ChildModel[];
  /** Ids of archived children (History), newest first. */
  archivedChildren: string[];
  /** Plan timeline, or the Later or Backlog shelf. */
  shelf: TaskShelf;
  allMerged: boolean;
  selected: boolean;
  ripple: boolean;
}

type NeedsYou = ReturnType<typeof buildNeedsYou>;

interface Ctx {
  board: Board;
  view: TimelineView;
  cal: Calendar;
  progress: ReadonlyMap<string, TaskProgress>;
  sessions: readonly Session[];
  workflows: ReadonlyMap<string, Workflow>;
  needs: NeedsYou;
  ripple: Ripple | null;
  selectedId: string | null;
  mainName: ReadonlyMap<string, string>;
  prTitle: (branchId: string) => string;
  repoLabel: (codeRepoId: string) => string;
  nowMs: number;
}

function buildRow(c: Ctx, task: Task, bid: string, depth: number): BranchRowModel {
  const graph = c.view.graph;
  const br = graph.byBranch.get(bid) as Branch;
  const draft = isDraft(br);
  const parent = graph.parentOf.get(bid);
  const parentName = parent === undefined ? undefined : graph.byBranch.get(parent)?.name;
  const attn = c.needs.items.find((n) => ASKS.has(n.kind) && n.branchId === bid);
  const main = c.mainName.get(br.codeRepoId) ?? '';
  const taskProgress = c.progress.get(task.id);
  const prog = taskProgress ? branchProgress(taskProgress, bid) : null;
  const line2 = branchLine2(br, graph, c.prTitle(bid));
  let context = '';
  if (draft)
    context = t('space.ade.baseMarker.draft.label', { base: parentName || br.base || 'main' });
  else if (br.kind === 'review') context = c.prTitle(bid);
  return {
    id: bid,
    depth: task.kind === 'review' ? 0 : depth + 1,
    branch: br,
    repo: c.repoLabel(br.codeRepoId),
    base: baseMarker(br, graph, main),
    draft,
    name: draft ? t('common.newBranch.label') : br.name,
    isReview: br.kind === 'review',
    context,
    tag: branchTag({
      branch: br,
      graph,
      plan: c.board.plan,
      after: c.view.after,
      nowMs: c.nowMs,
      hadSession: c.sessions.some((x) => x.branchId === bid),
      mainName: main,
      runs: task.runs,
    }),
    prog: prog ? { segs: prog.segments, label: prog.label, tone: prog.tone, tip: prog.tip } : null,
    ripple: c.ripple?.branchIds.includes(bid) ?? false,
    chips: { merged: line2.merged, deployed: line2.deployed, divider: line2.divider },
    attention: attn?.what ?? '',
    attentionItem: attn ?? null,
    attach: attachOf(br, (id) => {
      const other = graph.byTask.get(id);
      return other ? titleOfTask(c, other) : t('space.ade.board.newTask.label');
    }),
  };
}

function cardMeta(c: Ctx, entry: TimelineEntry, task: Task, branches: Branch[]): string {
  const repoNames: string[] = [];
  for (const br of branches) {
    const label = c.repoLabel(br.codeRepoId);
    if (!repoNames.includes(label)) repoNames.push(label);
  }
  const jira = task.jira?.key;
  const dated = entry.day !== LATER && entry.day >= WINDOW_DAYS;
  return (
    (dated
      ? `${dayLabel(c.cal, entry.day)} ${monthLabel(dateParts(c.cal, entry.day).month)} · `
      : '') +
    (entry.days.length > 1 ? `${entry.span}d → ${dayLabel(c.cal, entry.end)} · ` : '') +
    (jira ? `${jira} · ` : '') +
    repoNames.join(' · ')
  );
}

function titleOfTask(c: Ctx, task: Task): string {
  const first = task.branchIds.map((bid) => c.view.graph.byBranch.get(bid)).find(Boolean);
  return taskTitle(task, first, first ? c.prTitle(first.id) : '');
}

function parentOf(c: Ctx, task: Task): ParentModel | null {
  if (!task.parentId) return null;
  const live = c.view.graph.byTask.get(task.parentId);
  if (live) return { id: live.id, title: titleOfTask(c, live), archived: false };
  return {
    id: task.parentId,
    title: task.parentTitle || t('space.ade.board.newTask.label'),
    archived: task.parentArchived ?? true,
  };
}

function childrenOf(c: Ctx, id: string): ChildModel[] {
  return c.board.tasks
    .filter((x) => x.parentId === id)
    .map((x) => {
      const progress = c.progress.get(x.id) as TaskProgress;
      return {
        id: x.id,
        title: titleOfTask(c, x),
        status: statusOf(x, c.board.statuses),
        step: progress.hasWorkflow ? progress.label : '',
        finished: progress.finished,
      };
    });
}

function buildCard(c: Ctx, id: string): CardModel | null {
  const graph = c.view.graph;
  const entry = c.view.entries.get(id);
  const task = graph.byTask.get(id);
  if (!entry || !task) return null;
  const progress = c.progress.get(id) as TaskProgress;
  const branches = task.branchIds.flatMap((bid) => graph.byBranch.get(bid) ?? []);
  const sessions = c.sessions.filter((x) => x.taskId === id);
  const status = statusOf(task, c.board.statuses);
  const blocked = isBlocked(task, progress, branches);
  const cell = taskCell({ task, progress, status, blocked, branches, sessions });
  const workflow = task.workflow ?? c.workflows.get(task.workflowId) ?? null;
  const hidden = !isWorkKind(task.kind);
  const first = branches[0];
  const mine = branches.filter((br) => br.kind === 'mine');
  const needsOf = c.needs.items.find((n) => ASKS.has(n.kind) && n.taskId === id);
  return {
    task,
    entry,
    color: taskColor(task.color),
    title: taskTitle(task, first, first ? c.prTitle(first.id) : ''),
    defaultTitle: taskTitle({ ...task, title: '' }, first, first ? c.prTitle(first.id) : ''),
    progress,
    workflow,
    blocks: hidden ? [] : buildStepBlocks(progress, workflow),
    status,
    blocked,
    tag: cell?.tag ?? null,
    action: cell?.action ?? null,
    meta: cardMeta(c, entry, task, branches),
    attention: needsOf?.what ?? '',
    attentionItem: needsOf ?? null,
    sessions: taskSessionsOf(sessions),
    rows: c.view.branchRows(id).map(({ id: bid, depth }) => buildRow(c, task, bid, depth)),
    review: task.kind === 'review',
    parked: task.kind === 'parked',
    orchestrator: task.kind === 'orchestrator',
    parent: parentOf(c, task),
    children: childrenOf(c, id),
    archivedChildren: c.board.history.filter((h) => h.parentId === id).map((h) => h.taskId),
    shelf: shelfOf(c.board, id),
    allMerged: mine.length > 0 && mine.every((br) => br.mergedIntoMain),
    selected: c.selectedId === id,
    ripple: c.ripple?.taskIds.includes(id) ?? false,
  };
}

function buildPlanModel() {
  const board = useBoard();
  const changeRequests = useChangeRequests();
  const sessionsQuery = useSessions();
  const agentStore = useAgentSessionsStore();
  const workflows = useWorkflows();
  const repos = useRepos();
  const settingsStore = useSettingsStore();
  const codeReposStore = useCodeReposStore();
  const ui = useAdeBoardUiStore();

  // The model reads the minute clock; `liveNow` also ticks per second while a setup runs, for the
  // `⚙ preparing 3m 40s` label alone, so a running setup never rebuilds the whole model.
  const now = ref(new Date());
  const liveNow = ref(now.value);
  useIntervalFn(() => {
    now.value = new Date();
    liveNow.value = now.value;
  }, TIMING.clockTickSlow);
  const setupRunning = computed(
    () => board.data.value?.branches.some((b) => b.setup?.state === 'running') ?? false,
  );
  const secondTick = useIntervalFn(
    () => {
      liveNow.value = new Date();
    },
    TIMING.clockTick,
    { immediate: false },
  );
  watch(
    setupRunning,
    (running) => {
      if (running) secondTick.resume();
      else secondTick.pause();
    },
    { immediate: true },
  );
  const today = computed(() => localIso(now.value));
  /** Sessions with the TUI activity the server does not report, merged in from the agent hooks. */
  // Only the phase is read, but the agent store writes a new activity on every hook event. A key
  // string compares by value, so `sessions` (and the model behind it) rebuilds on a phase change only.
  const phaseKey = computed(() =>
    (sessionsQuery.data.value?.sessions ?? [])
      .filter((s) => s.mode === 'tui' && s.state === 'running')
      .map((s) => `${s.terminalId}\t${agentStore.activity.get(s.terminalId)?.phase ?? ''}`)
      .join('\n'),
  );
  const sessions = computed(() => {
    const phases = new Map<string, Pick<AgentActivity, 'phase'>>();
    for (const line of phaseKey.value.split('\n')) {
      const [terminalId, phase] = line.split('\t');
      if (terminalId && phase) phases.set(terminalId, { phase: phase as AgentActivity['phase'] });
    }
    return withTuiActivity(sessionsQuery.data.value?.sessions ?? [], phases);
  });

  function repoLabel(codeRepoId: string): string {
    const cfg = repos.data.value?.repos.find((r) => r.codeRepoId === codeRepoId);
    return (
      cfg?.nickname || cfg?.name || codeReposStore.codeRepoRecord(codeRepoId)?.name || codeRepoId
    );
  }

  const planSettings = computed(() => ({
    ...settingsStore.ade,
    historyDays: ui.historyReach ?? settingsStore.ade.historyDays,
  }));

  const model = computed(() => {
    const b = board.data.value;
    if (!b) return null;
    const view = buildTimeline({
      board: b,
      settings: planSettings.value,
      today: today.value,
      localDayOf: localIsoOfMs,
      repoLabel,
      showAllItems: ui.showAllItems,
      showHistory: ui.showHistory,
      hiddenRepoIds: new Set(ui.hiddenRepoIds),
    });
    const graph = view.graph;
    const wf = new Map<string, Workflow>();
    for (const e of workflows.data.value?.workflows ?? []) {
      if (e.workflow) wf.set(e.workflow.id, e.workflow);
    }
    const progress = new Map<string, TaskProgress>();
    for (const t of b.tasks) {
      progress.set(
        t.id,
        buildTaskProgress({
          task: t,
          workflow: t.workflow ?? wf.get(t.workflowId) ?? null,
          branch: (id) => graph.byBranch.get(id),
          repoNick: repoLabel,
        }),
      );
    }
    const nowMs = now.value.getTime();
    const selectedId = ui.selectedTaskId;
    const ctx: Ctx = {
      board: b,
      view,
      cal: buildCalendar(today.value, planSettings.value),
      progress,
      sessions: sessions.value,
      workflows: wf,
      needs: buildNeedsYou({
        board: b,
        sessions: sessions.value,
        progress,
        repoNick: repoLabel,
        nowMs,
      }),
      ripple: selectedId ? rippleOf(view, { kind: 'task', id: selectedId }) : null,
      selectedId,
      mainName: new Map(b.repos.map((r) => [r.codeRepoId, r.mainName])),
      prTitle: (id) => changeRequests.data.value?.branches[id]?.title ?? '',
      repoLabel,
      nowMs,
    };
    const cards = new Map<string, CardModel>();
    for (const id of view.shownTaskIds) {
      const card = buildCard(ctx, id);
      if (card) cards.set(id, card);
    }
    // The panel opens tasks the Plan hides (first-10 cap, repo filter), so it builds them on demand.
    const extra = new Map<string, CardModel | null>();
    const cardFor = (id: string): CardModel | null => {
      const shown = cards.get(id);
      if (shown) return shown;
      if (!extra.has(id)) extra.set(id, buildCard(ctx, id));
      return extra.get(id) ?? null;
    };
    return {
      board: b,
      view,
      cards,
      cardFor,
      ripple: ctx.ripple,
      cal: ctx.cal,
      needs: ctx.needs,
      workflows: wf,
    };
  });

  return {
    model,
    sessions,
    today,
    now,
    liveNow,
    repoLabel,
    settings: planSettings,
    boardQuery: board,
  };
}

/** Everything the Plan and the panel render, derived from the four cached queries and the view
 *  state. One instance serves every caller, so the panel never recomputes the Plan's model. */
export const usePlanModel = createSharedComposable(buildPlanModel);

export type PlanModel = NonNullable<ReturnType<typeof buildPlanModel>['model']['value']>;
