import { defineStore } from 'pinia';
import { ref } from 'vue';

export type TaskPanelTab = 'main' | 'details' | 'sessions';

// Last view of each task and branch panel: the tab, the session picked in a task's Sessions tab and
// whether a task's Details timeline is open. In memory only; entries of tasks and branches that left
// the board are pruned.
export const useAdePanelTabsStore = defineStore('adePanelTabs', () => {
  const taskTabs = ref<Record<string, TaskPanelTab>>({});
  const timelines = ref<Record<string, boolean>>({});
  const branchTabs = ref<Record<string, string>>({});
  /** Per task: the session last picked in its task or branch Sessions tab. */
  const sessionIds = ref<Record<string, string>>({});

  const taskTab = (taskId: string): TaskPanelTab => taskTabs.value[taskId] ?? 'main';
  const timelineOpen = (taskId: string): boolean => timelines.value[taskId] ?? false;
  const branchTab = (branchId: string): string => branchTabs.value[branchId] ?? 'details';
  const sessionOf = (taskId: string): string | null => sessionIds.value[taskId] ?? null;

  function setTaskTab(taskId: string, tab: TaskPanelTab): void {
    taskTabs.value[taskId] = tab;
  }
  function setTimelineOpen(taskId: string, open: boolean): void {
    timelines.value[taskId] = open;
  }
  function setBranchTab(branchId: string, tab: string): void {
    branchTabs.value[branchId] = tab;
  }
  function selectSession(taskId: string, sessionId: string): void {
    sessionIds.value[taskId] = sessionId;
  }

  function dropMissing(rec: Record<string, unknown>, keep: ReadonlySet<string>): void {
    for (const k of Object.keys(rec)) if (!keep.has(k)) delete rec[k];
  }

  /** Drops entries of tasks and branches no longer on the board (archived, removed). */
  function prune(taskIds: ReadonlySet<string>, branchIds: ReadonlySet<string>): void {
    dropMissing(taskTabs.value, taskIds);
    dropMissing(timelines.value, taskIds);
    dropMissing(sessionIds.value, taskIds);
    dropMissing(branchTabs.value, branchIds);
  }

  return {
    taskTabs,
    timelines,
    branchTabs,
    sessionIds,
    taskTab,
    timelineOpen,
    branchTab,
    sessionOf,
    setTaskTab,
    setTimelineOpen,
    setBranchTab,
    selectSession,
    prune,
  };
});
