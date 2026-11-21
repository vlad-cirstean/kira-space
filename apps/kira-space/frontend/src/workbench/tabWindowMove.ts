import type { AppMode } from '@shared/domain/mode';
import { useQuery, useQueryClient } from '@tanstack/vue-query';
import type { TabTerminalPayload } from '@workbench/bridge/createCoreControl';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { logError } from '@workbench/bridge/log';
import { windowKey } from '@workbench/bridge/rpc';
import type { TabWindowMove } from '@workbench/host';
import { computed, onScopeDispose } from 'vue';
import { control } from '../bridge/control';
import { useModeStore } from '../state/mode';
import { useOpenedFilesWindowStore } from '../state/openedFilesWindow';
import { STUDIO_TAB_KIND_MODE, type StudioTabKind, type TabRecord } from '../state/tabDomain';
import { useTabsStore } from '../state/tabs';
import { useTerminalsStore } from '../state/terminals';
import { useTabIncognitoStore } from '../studio/state/tabIncognito';

const AUTOMATIONS = 'automations';
const capsKey = ['tabs', 'caps'] as const;
const targetsKey = ['tabs', 'targets', windowKey] as const;

// Mirrors Go model.IsMovableTab.
function movable(tab: TabRecord): boolean {
  if (useOpenedFilesWindowStore().active) return false;
  if (tab.kind in STUDIO_TAB_KIND_MODE || tab.kind === 'script-run') return true;
  if (tab.kind !== 'terminal' || tab.workspaceId !== AUTOMATIONS) return false;
  return useTerminalsStore().terminalSession(tab.id)?.status !== 'starting';
}

/** Created once, in App.vue's setup (the queries need a component scope). */
export function createTabWindowMove(): TabWindowMove<TabRecord> {
  const tabsStore = useTabsStore();
  const terminalsStore = useTerminalsStore();
  const incognito = useTabIncognitoStore();
  const queryClient = useQueryClient();

  const caps = useQuery({ queryKey: capsKey, queryFn: () => control.tabsCapabilities() });
  const targets = useQuery({ queryKey: targetsKey, queryFn: () => control.tabsTargets() });
  onScopeDispose(
    control.onWindowsChanged(() => void queryClient.invalidateQueries({ queryKey: targetsKey })),
  );

  async function move(
    tab: TabRecord,
    to: Parameters<TabWindowMove<TabRecord>['move']>[1],
  ): Promise<void> {
    if (!(await tabsStore.confirmRelease(tab.id))) return;
    await tabsStore.settleSaves();
    const record = tabsStore.serializeTab(tab.id);
    if (!record) return;
    const isTerminal = tab.kind === 'terminal';
    let held = false;
    try {
      let terminal: TabTerminalPayload | undefined;
      if (isTerminal) {
        const prepared = await terminalsStore.prepareTerminalMove(tab.id);
        held = prepared.live;
        terminal = prepared.payload;
      }
      const result = await control.tabsMove({
        tab: record,
        incognito: incognito.isIncognito(tab.id),
        terminal,
        to,
      });
      if (result.windowKey === windowKey) {
        if (held) await terminalsStore.releaseTerminalMove(tab.id);
        return;
      }
    } catch (err) {
      logError('tabs', 'move failed', err);
      if (held) {
        await terminalsStore
          .releaseTerminalMove(tab.id)
          .catch((e: unknown) => logError('tabs', 'terminal release failed', e));
      }
      return;
    }
    if (isTerminal) terminalsStore.detachTerminalSession(tab.id);
    tabsStore.removeMovedTab(tab.id);
    if (tabsStore.tabs.length === 0) fireAndForget(control.tabsCloseEmptyWindow(), 'window');
  }

  return {
    canMove: movable,
    dragOut: computed(() => caps.data.value?.dragOut ?? false),
    targets: computed(() => targets.data.value ?? []),
    move,
  };
}

/** Pulls the tabs other windows moved here: once at boot, then on every nudge. */
export function installTabInbox(): void {
  const tabsStore = useTabsStore();
  const terminalsStore = useTerminalsStore();
  const modeStore = useModeStore();
  let pulling: Promise<void> = Promise.resolve();

  async function pullOnce(): Promise<void> {
    const entries = await control.tabsInbound();
    let last: TabRecord | null = null;
    for (const entry of entries) {
      const tab = entry.tab;
      if (entry.terminal && tab.kind === 'terminal') {
        await terminalsStore.adoptTerminalSession(
          {
            tabId: tab.id,
            codeRepoId: tab.state.codeRepoId,
            cwd: tab.state.cwd,
            command: tab.state.command,
            launchKind: tab.state.launchKind,
          },
          entry.terminal,
        );
      }
      last = tabsStore.receiveTab(tab, { incognito: entry.incognito });
    }
    if (!last) return;
    modeStore.setMode(
      (last.kind in STUDIO_TAB_KIND_MODE
        ? STUDIO_TAB_KIND_MODE[last.kind as StudioTabKind]
        : AUTOMATIONS) as AppMode,
    );
  }

  // One pull at a time, so two nudges never race on the same inbox.
  function pull(): void {
    pulling = pulling.then(pullOnce).catch((err: unknown) => logError('tabs', 'inbox failed', err));
  }

  control.onTabsInbound(pull);
  pull();
}
