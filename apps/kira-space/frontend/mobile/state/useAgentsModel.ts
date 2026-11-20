import { ACTIVITY_RANK, withTuiActivity } from '@ade/activity';
import { type BranchTag, branchTag, type TaskAction, tagLabel, taskCell } from '@ade/board/actions';
import { buildBranchGraph, isDraft } from '@ade/board/branchGraph';
import { taskTitle } from '@ade/board/labels';
import { buildNeedsYou } from '@ade/board/needsYou';
import { buildTaskProgress, type TaskProgress } from '@ade/board/progress';
import { blocked as isBlocked, statusOf } from '@ade/board/status';
import { buildStepBlocks, type StepBlock } from '@ade/board/stepBlocks';
import { type StepMoves, stepMoves } from '@ade/board/stepMoves';
import { type TaskSessions, taskSessionsOf } from '@ade/board/taskSessions';
import { localIso } from '@ade/localDay';
import { taskColor } from '@ade/palette';
import {
  useBoard,
  useChangeRequests,
  useRepoNames,
  useSessions,
  useWorkflows,
} from '@ade/readQueries';
import { runStateLabel } from '@ade/runState';
import {
  byRunningOrder,
  type SessionLookup,
  type SessionView,
  sessionView,
} from '@ade/sessions/sessionView';
import { agentsSummary } from '@ade/status/agentsSummary';
import type {
  Branch,
  ChangeRequest,
  RepoState,
  Run,
  Session,
  Task,
  TaskStatusDef,
  Workflow,
} from '@ade/wire';
import { t } from '@theme/i18n';
import { createSharedComposable, useIntervalFn } from '@vueuse/core';
import { useAgentInfo } from '@workbench/agent/agentInfo';
import { formatWeekdayDate } from '@workbench/behaviour/format';
import { compareNatural } from '@workbench/behaviour/match';
import { TIMING } from '@workbench/behaviour/timing';
import { computed, ref } from 'vue';
import { useAgentSessionsStore } from './agentSessions';

/** One branch of a task as the phone's expanded card lists it. */
export interface PhoneBranch {
  id: string;
  name: string;
  repo: string;
  tag: BranchTag;
  /** The tag's label with a running setup's elapsed time applied. */
  tagText: string;
  ahead: number;
  behind: number;
  dirty: number;
  pr: ChangeRequest | null;
}

/** The task's newest step run, for the card's run line. */
export interface PhoneRunLine {
  state: Run['state'];
  label: string;
  startedAt: number | null;
  finishedAt: number | null;
  /** Why a failed or stuck run ended; '' otherwise. */
  reason: string;
}

export interface PlanCard {
  task: Task;
  title: string;
  color: string;
  progress: TaskProgress;
  status: TaskStatusDef;
  /** A stuck or failed run or setup, shown beside the status. */
  blocked: boolean;
  blocks: StepBlock[];
  /** Needs-you headline for this task, '' when none. */
  attention: string;
  /** Repo nicknames of the task's branches. */
  repos: string[];
  /** The desktop's step action for the task (Run, Step session, Take over, ...), if any. */
  action: TaskAction | null;
  /** Where the step buttons can go from here. */
  moves: StepMoves;
  /** Running interactive Claude Code sessions of the task. */
  tuiSessions: Session[];
  /** One state for the task's agent sessions; null when it has none. */
  sessions: TaskSessions | null;
  /** Every session of the task, running first. */
  sessionViews: SessionView[];
  run: PhoneRunLine | null;
  jira: string;
  /** An agent waits on a branch request for this task. */
  branchRequest: boolean;
  branches: PhoneBranch[];
}

export interface PlanGroup {
  key: string;
  label: string;
  cards: PlanCard[];
}

const DAY_MS = 86_400_000;

function dayLabel(iso: string, today: string): string {
  if (iso < today) return t('space.common.day.earlier.label');
  if (iso === today) return t('space.common.day.today.label');
  const date = new Date(`${iso}T00:00:00`);
  if (localIso(new Date(date.getTime() - DAY_MS)) === today)
    return t('space.common.day.tomorrow.label');
  return formatWeekdayDate(date);
}

/** Newest step run of a task: rebase and review runs sit outside its workflow. */
function runLine(runs: readonly Run[]): PhoneRunLine | null {
  const step = runs.filter((r) => r.purpose === '');
  const newest = step.reduce<Run | null>(
    (best, r) => (!best || (r.startedAt ?? 0) >= (best.startedAt ?? 0) ? r : best),
    null,
  );
  if (!newest) return null;
  const failed = newest.state === 'failed' || newest.state === 'stuck';
  return {
    state: newest.state,
    label: runStateLabel(newest.state),
    startedAt: newest.startedAt,
    finishedAt: newest.finishedAt,
    reason: failed ? (newest.outcome?.reason ?? '') : '',
  };
}

/** The tabs' data, derived from the cached read queries. One instance serves every screen. */
export const useAgentsModel = createSharedComposable(() => {
  const board = useBoard();
  const sessionsQuery = useSessions();
  const workflowsQuery = useWorkflows();
  const repoNames = useRepoNames();
  const agent = useAgentSessionsStore();
  const changeRequests = useChangeRequests();
  const { info } = useAgentInfo();

  const nowMs = ref(Date.now());
  useIntervalFn(() => {
    nowMs.value = Date.now();
  }, TIMING.pollSlow);

  const sessions = computed(() =>
    withTuiActivity(sessionsQuery.data.value?.sessions ?? [], agent.activity),
  );

  function repoLabel(codeRepoId: string): string {
    const repo = repoNames.data.value?.repos.find((r) => r.codeRepoId === codeRepoId);
    return repo?.nickname || repo?.name || codeRepoId;
  }

  const model = computed(() => {
    const b = board.data.value;
    if (!b) return null;
    const workflows = new Map<string, Workflow>();
    for (const entry of workflowsQuery.data.value?.workflows ?? []) {
      if (entry.workflow) workflows.set(entry.workflow.id, entry.workflow);
    }
    const branchById = new Map<string, Branch>(b.branches.map((br) => [br.id, br]));
    const input = (task: Task) => ({
      task,
      workflow: task.workflow ?? workflows.get(task.workflowId) ?? null,
      branch: (id: string) => branchById.get(id),
      repoNick: repoLabel,
    });
    const progress = new Map(b.tasks.map((t) => [t.id, buildTaskProgress(input(t))]));
    const graph = buildBranchGraph(b);
    const lookup: SessionLookup = {
      task: (id) => b.tasks.find((x) => x.id === id),
      branch: (id) => branchById.get(id),
      repoLabel,
      workflow: (id) => workflows.get(id),
      headlessLabel: info.value.headlessLabel,
    };
    const mainName = new Map(b.repos.map((r) => [r.codeRepoId, r.mainName]));
    const prs = changeRequests.data.value?.branches ?? {};
    const needs = buildNeedsYou({
      board: b,
      sessions: sessions.value,
      progress,
      repoNick: repoLabel,
      nowMs: nowMs.value,
    });

    const cards = new Map<string, PlanCard>();
    for (const task of b.tasks) {
      const taskProgress = progress.get(task.id) as TaskProgress;
      const branches = task.branchIds.flatMap((id) => branchById.get(id) ?? []);
      const i = input(task);
      const status = statusOf(task, b.statuses);
      const blocked = isBlocked(task, taskProgress, branches);
      const taskSessions = sessions.value.filter((s) => s.taskId === task.id);
      const blocks = buildStepBlocks(taskProgress, i.workflow);
      const ownSessions = [...taskSessions].sort(byRunningOrder);
      cards.set(task.id, {
        task,
        title: taskTitle(task, branches[0], ''),
        color: taskColor(task.color),
        progress: taskProgress,
        status,
        blocked,
        blocks,
        attention: needs.items.find((n) => n.taskId === task.id)?.what ?? '',
        repos: [...new Set(branches.map((br) => repoLabel(br.codeRepoId)))],
        action:
          taskCell({
            task,
            progress: taskProgress,
            status,
            blocked,
            branches,
            sessions: taskSessions,
          })?.action ?? null,
        moves: stepMoves(
          blocks,
          taskProgress.finished ? 'done' : (taskProgress.step?.id ?? task.entryStepId),
          task.runs,
        ),
        tuiSessions: taskSessions.filter((s) => s.mode === 'tui' && s.state === 'running'),
        sessions: taskSessionsOf(taskSessions),
        sessionViews: ownSessions.map((s) => sessionView(s, lookup)),
        run: runLine(task.runs),
        jira: task.jira?.key ?? '',
        branchRequest: task.branchRequestId !== '',
        branches: branches.map((br) => {
          const tag = branchTag({
            branch: br,
            graph,
            plan: b.plan,
            // The shared-files tag needs the timeline's merge order, which the phone has no view of.
            after: new Map(),
            nowMs: nowMs.value,
            hadSession: sessions.value.some((x) => x.branchId === br.id),
            mainName: mainName.get(br.codeRepoId) ?? '',
            runs: task.runs,
          });
          return {
            id: br.id,
            name: isDraft(br) ? t('common.newBranch.label') : br.name,
            repo: repoLabel(br.codeRepoId),
            tag,
            tagText: tagLabel(tag, nowMs.value),
            ahead: br.ahead,
            behind: br.behind,
            dirty: br.dirty.length,
            pr: prs[br.id] ?? null,
          };
        }),
      });
    }

    // Plan order within a day; tasks with no day sit under Later.
    const today = localIso(new Date(nowMs.value));
    const rank = new Map(b.plan.order.map((id, k) => [id, k]));
    const ordered = [...cards.values()].sort(
      (a, c) => (rank.get(a.task.id) ?? 1e9) - (rank.get(c.task.id) ?? 1e9),
    );
    const groups = new Map<string, PlanGroup>();
    for (const card of ordered) {
      const day = b.plan.day[card.task.id] ?? '';
      const group = groups.get(day) ?? {
        key: day || 'later',
        label: day ? dayLabel(day, today) : t('common.later.label'),
        cards: [],
      };
      group.cards.push(card);
      groups.set(day, group);
    }
    const plan = [...groups.entries()]
      .sort(([a], [c]) => (a === '' ? 1 : c === '' ? -1 : compareNatural(a, c)))
      .map(([, g]) => g);

    // Every running session and every finished one not yet viewed: the Activity tab's agent list.
    const live = sessions.value
      .filter((s) => s.state === 'running' || s.unseen)
      .map((s) => sessionView(s, lookup));
    const agents = {
      summary: agentsSummary(sessions.value),
      views: live.sort(
        (a, c) =>
          ACTIVITY_RANK[a.kind] - ACTIVITY_RANK[c.kind] || byRunningOrder(a.session, c.session),
      ),
    };
    const repoStates: RepoState[] = b.repos;

    return { needs, cards, plan, today, agents, repoStates };
  });

  return { model, repoLabel, boardQuery: board, nowMs };
});
