import { t } from '@theme/i18n';
import { errorText, toCodedError } from '@workbench/bridge/codedError';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed } from 'vue';
import { useSetTaskWorkflow, useWorkflows } from '../queries';

function lossCounts(err: unknown): { runs: number; logs: number } {
  const d = toCodedError(err).details as { runs?: number; logs?: number } | undefined;
  return { runs: d?.runs ?? 0, logs: d?.logs ?? 0 };
}

// Workflow switch shared by the Task tab picker and the task menu. `switchTo` resolves false when
// the user declines the loss confirmation; any other failure throws.
export function useSwitchWorkflow() {
  const workflows = useWorkflows();
  const setWorkflow = useSetTaskWorkflow();
  const confirm = useConfirmDialogStore();

  const options = computed(() =>
    (workflows.data.value?.workflows ?? []).flatMap((e) =>
      e.workflow ? [{ id: e.workflow.id, name: e.workflow.name, file: e.fileName }] : [],
    ),
  );

  async function switchTo(taskId: string, workflowId: string): Promise<boolean> {
    const args = { taskId, workflowId };
    try {
      await setWorkflow.mutateAsync(args);
      return true;
    } catch (err) {
      // E_CONFLICT is the unconfirmed switch: its text names the runs and logs the switch deletes.
      if ((err as { code?: string }).code !== 'E_CONFLICT') throw err;
      // The confirmation covers the counts the dialog names; the backend refuses again if the task
      // holds more by the time it arrives (a run finished meanwhile).
      const { runs, logs } = lossCounts(err);
      return confirm.confirm({
        title: t('space.ade.workflowBlock.switch.title'),
        message: errorText(err),
        confirmLabel: t('space.ade.workflowBlock.switch.confirm.label'),
        tone: 'danger',
        run: () =>
          setWorkflow.mutateAsync({
            ...args,
            confirmed: true,
            confirmedRuns: runs,
            confirmedLogs: logs,
          }),
      });
    }
  }

  return { options, switchTo };
}
