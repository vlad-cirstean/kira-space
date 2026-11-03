import { t } from '@theme/i18n';
import { useAgentInfo, useAgentList } from '@workbench/agent/agentInfo';
import { errorText } from '@workbench/bridge/codedError';
import type { MenuItem } from '@workbench/state/contextMenu';
import { computed, type MaybeRefOrGetter, toValue } from 'vue';
import { openRepoTerminalTab } from '../../../state/repoTabs';
import { type Gate, type LaunchTarget, launchTargets } from '../board/taskLaunch';
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
  const { info } = useAgentInfo();
  const { list } = useAgentList();

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

  const usable = computed(() => targets.value.some((x) => x.worktree !== ''));
  // Two or more installed providers: one entry per provider; one keeps the plain menu.
  const providers = computed(() => list.value.filter((e) => e.available));

  const where = (x: LaunchTarget): string =>
    t('common.dotPair.label', { first: x.repo, second: x.branch });
  const agentLabelOf = (product: string): string =>
    t('space.ade.taskLaunch.agent.label', { agent: product });
  const hint = (g: Gate): string | undefined => (g.reason ? t(g.reason) : undefined);

  function terminalItem(x: LaunchTarget, label: string): MenuItem {
    return {
      type: 'item',
      id: `ade-task-launch-terminal-${x.branchId}`,
      label,
      icon: 'terminal-bash',
      disabled: x.terminal.disabled,
      hint: hint(x.terminal),
      run: () => openTerminal(x),
    };
  }

  function agentItem(x: LaunchTarget, label: string, provider?: string): MenuItem {
    return {
      type: 'item',
      id: provider
        ? `ade-task-launch-agent-${provider}-${x.branchId}`
        : `ade-task-launch-agent-${x.branchId}`,
      label,
      icon: 'robot',
      disabled: x.agent.disabled,
      hint: hint(x.agent),
      run: () => startAgent(x, provider ?? ''),
    };
  }

  /** Built at open time, so the gates are fresh. */
  function items(): MenuItem[] {
    const rows = targets.value;
    const choose = providers.value.length > 1;
    const first = rows[0];
    if (rows.length > 1) {
      const terminals = rows.map((x) => terminalItem(x, where(x)));
      const agents: MenuItem[] = choose
        ? providers.value.map((p) => ({
            type: 'submenu',
            id: `ade-task-launch-provider-${p.id}`,
            label: agentLabelOf(p.productLabel),
            icon: 'robot',
            items: rows.map((x) => agentItem(x, where(x), p.id)),
          }))
        : [
            { type: 'label', label: agentLabelOf(info.value.productLabel) },
            ...rows.map((x) => agentItem(x, where(x))),
          ];
      return [
        { type: 'label', label: t('common.module.terminal.label') },
        ...terminals,
        { type: 'separator' },
        ...agents,
      ];
    }
    if (!first) return [];
    const terminal = terminalItem(first, t('common.module.terminal.label'));
    return choose
      ? [
          terminal,
          ...providers.value.map((p) => agentItem(first, agentLabelOf(p.productLabel), p.id)),
        ]
      : [terminal, agentItem(first, agentLabelOf(info.value.productLabel))];
  }

  return { usable, pending: start.isPending, items };
}
