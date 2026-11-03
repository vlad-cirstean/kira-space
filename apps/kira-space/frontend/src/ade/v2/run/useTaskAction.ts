import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { errorText } from '@workbench/bridge/codedError';
import { computed, type MaybeRefOrGetter, toValue } from 'vue';
import type { TaskAction } from '../board/actions';
import type { CardModel } from '../plan/usePlanModel';
import { useApprove, useRetryRun, useStartRun } from '../queries';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeDialogsStore } from '../state/adeDialogs';
import { useAdeStageDialogStore } from '../state/adeStageDialog';
import { useAdeTakeOverStore } from '../state/adeTakeOver';

/** The task's rendered step action and the call behind it. */
export function useTaskAction(card: MaybeRefOrGetter<CardModel>) {
  const ui = useAdeBoardUiStore();
  const dialogs = useAdeDialogsStore();
  const takeOver = useAdeTakeOverStore();
  const start = useStartRun();
  const approve = useApprove();
  const retry = useRetryRun();
  const stage = useAdeStageDialogStore();

  const action = computed<TaskAction | null>(() => toValue(card).action);

  async function run(): Promise<void> {
    const c = toValue(card);
    const a = action.value;
    const step = c.progress.step;
    if (!a) return;
    const taskId = c.task.id;
    delete ui.actionError[taskId];
    if (a.kind === 'archive') return dialogs.archive(taskId);
    if (a.kind === 'session') return dialogs.session(taskId);
    if (a.kind === 'takeOver') return takeOver.request(a.sessionId ?? '');
    if (!step) return;
    try {
      if (a.kind === 'run') {
        if (step.kind === 'agent') ui.runTaskId = taskId;
        else await start.mutateAsync({ taskId, branchNames: {}, message: '' });
      } else if (a.kind === 'approve') {
        await approve.mutateAsync({ taskId, stepId: a.stepId ?? '' });
      } else if (a.kind === 'retry') {
        const runs = c.progress.steps
          .find((s) => s.id === a.stepId)
          ?.runs.filter((r) => r.state === 'failed' || r.state === 'stuck');
        await Promise.all((runs ?? []).map((r) => retry.mutateAsync({ runId: r.runId })));
      } else if (a.kind === 'advance' || a.kind === 'finish') {
        stage.open({ taskId });
      }
    } catch (err) {
      ui.actionError[taskId] = errorText(err);
      ui.select(taskId);
    }
  }

  /** True while a call is in flight: a repeat click must not launch the step twice. */
  const guarded = useAsyncAction(run);
  const busy = guarded.pending;
  async function perform(): Promise<void> {
    if (!busy.value) await guarded.run();
  }

  return { action, perform, busy };
}
