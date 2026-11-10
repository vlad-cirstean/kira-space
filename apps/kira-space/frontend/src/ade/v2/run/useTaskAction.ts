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
import { useRepoGate } from '../state/useRepoGate';

/** The task's rendered step action and the call behind it. */
export function useTaskAction(card: MaybeRefOrGetter<CardModel>) {
  const ui = useAdeBoardUiStore();
  const dialogs = useAdeDialogsStore();
  const takeOver = useAdeTakeOverStore();
  const start = useStartRun();
  const approve = useApprove();
  const retry = useRetryRun();
  const stage = useAdeStageDialogStore();
  const { gated } = useRepoGate();

  const action = computed<TaskAction | null>(() => toValue(card).action);

  async function run(): Promise<void> {
    const c = toValue(card);
    const a = action.value;
    const step = c.progress.step;
    if (!a) return;
    const taskId = c.task.id;
    delete ui.actionError[taskId];
    if (a.kind === 'archive') return dialogs.archive(taskId);
    if (a.kind === 'nextPass') return stage.openPass(taskId);
    if (a.kind === 'takeOver' || a.kind === 'resume') return takeOver.request(a.sessionId ?? '');
    try {
      if (a.kind === 'session') {
        await gated(taskId, step?.needsRepo ?? false, async () => dialogs.session(taskId));
        return;
      }
      if (!step) return;
      if (a.kind === 'run') {
        await gated(taskId, step.needsRepo, async () => {
          if (step.kind === 'agent') ui.runTaskId = taskId;
          else await start.mutateAsync({ taskId, branchNames: {}, message: '' });
        });
      } else if (a.kind === 'approve') {
        await gated(taskId, step.needsRepo, async () => {
          await approve.mutateAsync({ taskId, stepId: a.stepId ?? '' });
        });
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

  async function startNew(): Promise<void> {
    const c = toValue(card);
    const taskId = c.task.id;
    delete ui.actionError[taskId];
    try {
      await gated(taskId, c.progress.step?.needsRepo ?? false, async () => dialogs.session(taskId));
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

  return { action, perform, startNew, busy };
}
