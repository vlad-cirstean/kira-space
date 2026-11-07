import { usePersistedState } from '@workbench/behaviour/persisted';
import { defineStore } from 'pinia';
import { ref } from 'vue';
import type { OpenSessionEvent } from '../wire';
import { useAdePanelTabsStore } from './adePanelTabs';

// Board view state: which page is open and what the panel shows. Workflows, Agents, Repositories
// and the Add popover keep their own state in `adeWorkflowsUi`, `adeAgentsUi`, `adeReposUi` and
// `adeAddUi`. The three Plan toggles persist
// per window profile; selection and the last refresh summaries are in memory only. Panel tabs
// and the picked session live in `adePanelTabs`.
export type AdeView = 'plan' | 'later' | 'needs' | 'agents' | 'workflows' | 'repos';

export const useAdeBoardUiStore = defineStore('adeBoardUi', () => {
  const tabs = useAdePanelTabsStore();
  const view = ref<AdeView>('plan');
  const selectedTaskId = ref<string | null>(null);
  const selectedBranchId = ref<string | null>(null);
  /** An explicit open asked the Sessions tab to show this session, even if stopped. */
  const revealSessionId = ref<string | null>(null);
  /** Days of history shown beyond the settings window after a Go to; in memory only. */
  const historyReach = ref<number | null>(null);
  /** A See error click asked the open branch panel to scroll to its Worktree setup block. */
  const focusSetup = ref(false);
  /** A See log click asked the open branch panel to open its Last rebase log. */
  const focusRebaseLog = ref(false);
  /** Task whose agent Run dialog is open. */
  const runTaskId = ref<string | null>(null);
  /** Last failed step action per task, shown in the panel header until the next attempt. */
  const actionError = ref<Record<string, string>>({});
  /** Per repo: what the last fetch changed, shown on its chip until the next fetch. */
  const refreshSummary = ref<Record<string, string>>({});
  /** The Needs page's All sessions list is open; the status bar's Show all sessions sets it. */
  const showAllSessions = ref(false);
  const showAllItems = usePersistedState('kira.ade.showAllItems', false);
  const showHistory = usePersistedState('kira.ade.showHistory', false);
  const hiddenRepoIds = usePersistedState<string[]>('kira.ade.hiddenRepoIds', []);

  function select(taskId: string | null): void {
    selectedTaskId.value = taskId;
    selectedBranchId.value = null;
  }

  function selectBranch(taskId: string, branchId: string): void {
    selectedTaskId.value = taskId;
    selectedBranchId.value = branchId;
  }

  /** Opens the Plan with `taskId` selected. */
  function openTask(taskId: string): void {
    view.value = 'plan';
    select(taskId);
  }

  /** Selects the task or branch of a session and opens its Sessions tab on it. */
  function openSession(e: OpenSessionEvent): void {
    view.value = 'plan';
    if (e.branchId) selectBranch(e.taskId, e.branchId);
    else select(e.taskId);
    if (e.branchId) tabs.setBranchTab(e.branchId, 'sessions');
    else tabs.setTaskTab(e.taskId, 'sessions');
    tabs.selectSession(e.taskId, e.sessionId);
    revealSessionId.value = e.sessionId;
  }

  /** Opens the Plan on the task's Sessions tab. */
  function openSessions(taskId: string): void {
    if (view.value !== 'later') view.value = 'plan';
    select(taskId);
    tabs.setTaskTab(taskId, 'sessions');
  }

  function toggleRepo(codeRepoId: string): void {
    hiddenRepoIds.value = hiddenRepoIds.value.includes(codeRepoId)
      ? hiddenRepoIds.value.filter((id) => id !== codeRepoId)
      : [...hiddenRepoIds.value, codeRepoId];
  }

  return {
    view,
    selectedTaskId,
    selectedBranchId,
    revealSessionId,
    historyReach,
    runTaskId,
    focusSetup,
    focusRebaseLog,
    actionError,
    refreshSummary,
    showAllSessions,
    showAllItems,
    showHistory,
    hiddenRepoIds,
    select,
    selectBranch,
    openTask,
    openSession,
    openSessions,
    toggleRepo,
  };
});
