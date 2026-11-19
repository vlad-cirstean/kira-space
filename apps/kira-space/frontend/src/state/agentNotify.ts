import type { ScriptRun } from '@shared/domain/scriptRuns';
import { useDocumentVisibility, useWindowFocus, watchDebounced } from '@vueuse/core';
import { isLiveTab, keyOf, placeRunTab } from '@workbench/automations/scriptTabs';
import { TIMING } from '@workbench/behaviour/timing';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { queryClient } from '@workbench/state/queryClient';
import { computed } from 'vue';
import { useAdeBoardUiStore } from '../ade/v2/state/adeBoardUi';
import { control } from '../bridge/control';
import { useModeStore } from './mode';
import { useTabsStore } from './tabs';
import { useTerminalsStore } from './terminals';
import { GENERAL_WORKSPACE, useWorkspaceStore, visibleWorkspace } from './workspace';

// P238: tells Go what this window shows (so a note for the tab you look at is suppressed) and
// answers a notification click by showing the terminal tab or ADE task it names. Installed once
// per window; the window is the lifetime, so nothing is torn down. No store: nothing is shared.
export function installAgentNotifyFocus(): void {
  const mode = useModeStore();
  const tabs = useTabsStore();
  const terminals = useTerminalsStore();
  const board = useAdeBoardUiStore();
  const focused = useWindowFocus();
  const visibility = useDocumentVisibility();

  const activeTerminalId = computed(() => {
    if (mode.active === 'ade') return '';
    const id = tabs.activeIdByWorkspace[visibleWorkspace()];
    return id && tabs.tabs.some((t) => t.id === id && t.kind === 'terminal') ? id : '';
  });

  const activeScriptRunId = computed(() => {
    if (mode.active !== 'automations') return '';
    const id = tabs.activeIdByWorkspace[visibleWorkspace()];
    const tab = tabs.tabs.find((t) => t.id === id);
    return tab?.kind === 'script-run' ? tab.state.runId : '';
  });

  watchDebounced(
    () => ({
      focused: focused.value && visibility.value === 'visible',
      module: mode.active as string,
      activeTerminalId: activeTerminalId.value,
      adeTaskId: mode.active === 'ade' ? (board.selectedTaskId ?? '') : '',
      activeScriptRunId: activeScriptRunId.value,
    }),
    (state) => {
      fireAndForget(control.agentNotifyReportFocus(state), 'agentNotify');
    },
    { debounce: TIMING.persist, immediate: true, deep: true },
  );

  control.onAgentRevealTerminal(({ terminalId }) => {
    const tab = tabs.tabs.find((t) => t.id === terminalId);
    if (!tab) return;
    const key = tab.workspaceId ?? GENERAL_WORKSPACE;
    if (key === 'automations') {
      mode.setMode('automations');
    } else {
      useWorkspaceStore().activateWorkspace(key);
      mode.setMode('git');
    }
    tabs.activateTab(terminalId);
  });

  control.onAgentRevealScriptRun(({ runId, label }) => {
    mode.setMode('automations');
    const cached = queryClient.getQueryData<ScriptRun[]>(['scriptRuns', 'list']);
    const name = cached?.find((r) => r.id === runId)?.scriptName ?? label;
    const run = cached?.find((r) => r.id === runId);
    const state = {
      runId,
      label: name,
      ...(run ? keyOf(run) : { scriptId: '', taskId: '', branchId: '' }),
    };
    const { activate, replace } = placeRunTab(tabs.tabsForWorkspace('automations'), state, (tab) =>
      isLiveTab(tab, (id) => terminals.terminalSession(id)?.status),
    );
    if (activate !== undefined) tabs.activateTab(activate);
    else
      tabs.openTab('script-run', null, runId, () => state, {
        reuse: true,
        workspaceId: 'automations',
        replace,
      });
  });

  control.onAgentRevealTask(({ taskId }) => {
    mode.setMode('ade');
    board.openTask(taskId);
  });
}
