import { createSharedComposable } from '@vueuse/core';
import { useScriptRuns, useSeenRuns } from '@workbench/automations/runs/runsQueries';
import { computed } from 'vue';
import { chipRunsByTask } from './automationRuns';

/** One runs query and one seen list for every chip on the board, not one pair per chip. */
export const useChipRuns = createSharedComposable(() => {
  const { data } = useScriptRuns();
  const { seen, markSeen } = useSeenRuns();
  const byTask = computed(() => chipRunsByTask(data.value ?? [], seen.value));
  return { byTask, markSeen };
});
