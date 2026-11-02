import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { computed, type MaybeRefOrGetter, toValue } from 'vue';
import { openRepoTerminalTab } from '../../../state/repoTabs';
import { type LaunchTarget, launchTargets } from '../board/taskLaunch';
import { useLaunchOpener } from '../dialog/useLaunchOpener';
import { usePlanModel } from '../plan/usePlanModel';
import { useStartAdHoc } from '../queries';
import { useAdeBoardUiStore } from '../state/adeBoardUi';

/** The task plus menu: a terminal in the Git module, or an agent session with no first message,
 *  in a named branch's worktree. `branchId` narrows it to one branch. */
export function useTaskLaunch(
  taskId: MaybeRefOrGetter<string>,
  branchId?: MaybeRefOrGetter<string | undefined>,
) {
  const ui = useAdeBoardUiStore();
  const { model, sessions } = usePlanModel();
  const opener = useLaunchOpener();
  const start = useStartAdHoc();

  const card = computed(() => model.value?.cardFor(toValue(taskId)) ?? null);
  const targets = computed(() => {
    const only = branchId ? toValue(branchId) : undefined;
    const rows = (card.value?.rows ?? []).filter((r) => only === undefined || r.id === only);
    return launchTargets(rows, sessions.value);
  });

  function openTerminal(target: LaunchTarget): void {
    openRepoTerminalTab(target.codeRepoId, target.worktree, {
      command: '',
      kind: 'shell',
      color: 'none',
      label: t('common.dotPair.label', { first: card.value?.title ?? '', second: target.branch }),
    });
  }

  /** `provider` "" runs the Agent setting's provider. */
  async function startAgent(target: LaunchTarget, provider = ''): Promise<void> {
    const id = toValue(taskId);
    delete ui.actionError[id];
    try {
      const launch = await start.mutateAsync({ branchId: target.branchId, provider });
      await opener.open(launch);
      ui.openSession({ taskId: id, branchId: target.branchId, sessionId: launch.sessionId });
    } catch (err) {
      ui.select(id);
      ui.actionError[id] = errorText(err);
    }
  }

  return { targets, pending: start.isPending, openTerminal, startAgent };
}
