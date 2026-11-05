import { type ComputedRef, computed } from 'vue';
import { automationOp, isLiveRun, runStartedAt } from '../automations/runs/runOps';
import { useScriptRuns } from '../automations/runs/runsQueries';
import {
  canStopRun,
  triggerLabel,
  useStopAutomationRun,
} from '../automations/runs/useAutomationOps';

/** One row of the status bar running-ops widget; an app maps its own op records onto it. */
export interface RunningOpItem {
  id: string;
  label: string;
  detail: string;
  /** Epoch milliseconds. */
  startedAt: number;
  canStop: boolean;
  stop(): void;
}

/** Live Automations runs as widget rows, the same set the Operations panel counts as running. */
export function useRunningAutomationItems(): ComputedRef<RunningOpItem[]> {
  const { data } = useScriptRuns();
  const stopRun = useStopAutomationRun();
  return computed(() =>
    (data.value ?? []).filter(isLiveRun).map((run) => ({
      id: automationOp(run).id,
      label: run.scriptName,
      detail: run.taskTitle || triggerLabel(run),
      startedAt: runStartedAt(run),
      canStop: canStopRun(run),
      stop: () => stopRun(run),
    })),
  );
}
