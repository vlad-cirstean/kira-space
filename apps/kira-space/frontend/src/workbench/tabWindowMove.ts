import type { AppMode } from '@shared/domain/mode';
import { useQuery, useQueryClient } from '@tanstack/vue-query';
import type { TabMoveEntry } from '@workbench/bridge/createCoreControl';
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
const MAX_TERMINAL_SNAPSHOT_BYTES = 4 * 1024 * 1024;
const MAX_MOVE_SNAPSHOT_BYTES = 16 * 1024 * 1024;
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

  // Several tabs of one move share the bridge's snapshot budget: 4 MiB each, 16 MiB in all.
  const snapshotBudget = (count: number): number =>
    Math.min(MAX_TERMINAL_SNAPSHOT_BYTES, Math.floor(MAX_MOVE_SNAPSHOT_BYTES / Math.max(count, 1)));
  // A second move while one runs is ignored, so a double drop never holds a terminal twice.
  let moving = false;

  async function releaseHeld(ids: readonly string[]): Promise<void> {
    for (const id of ids) {
      await terminalsStore
        .releaseTerminalMove(id)
        .catch((e: unknown) => logError('tabs', 'terminal release failed', e));
    }
  }

  async function move(
    tabs: readonly TabRecord[],
    to: Parameters<TabWindowMove<TabRecord>['move']>[1],
  ): Promise<void> {
    if (moving) return;
    const list = tabs.filter(movable);
    if (list.length === 0) return;
    moving = true;
    try {
      await moveList(list, to);
    } finally {
      moving = false;
    }
  }

  async function moveList(
    list: readonly TabRecord[],
    to: Parameters<TabWindowMove<TabRecord>['move']>[1],
  ): Promise<void> {
    const ids = list.map((tab) => tab.id);
    if (!(await tabsStore.confirmRelease(ids))) return;
    await tabsStore.settleSaves();
    const entries: TabMoveEntry<TabRecord>[] = [];
    for (const tab of list) {
      const record = tabsStore.serializeTab(tab.id);
      if (record) entries.push({ tab: record, incognito: incognito.isIncognito(tab.id) });
    }
    if (entries.length === 0) return;
    const terminalIds = entries.filter((e) => e.tab.kind === 'terminal').map((e) => e.tab.id);
    const held: string[] = [];
    try {
      const prepared = await Promise.allSettled(
        terminalIds.map((id) =>
          terminalsStore.prepareTerminalMove(id, { maxBytes: snapshotBudget(terminalIds.length) }),
        ),
      );
      let failure: unknown;
      prepared.forEach((outcome, i) => {
        if (outcome.status === 'rejected') {
          failure ??= outcome.reason;
          return;
        }
        if (outcome.value.live) held.push(terminalIds[i]);
        const entry = entries.find((e) => e.tab.id === terminalIds[i]);
        if (entry) entry.terminal = outcome.value.payload;
      });
      if (failure !== undefined) throw failure;
      const result = await control.tabsMove({ tabs: entries, to });
      if (result.windowKey === windowKey) {
        await releaseHeld(held);
        return;
      }
    } catch (err) {
      logError('tabs', 'move failed', err);
      await releaseHeld(held);
      return;
    }
    for (const id of terminalIds) terminalsStore.detachTerminalSession(id);
    tabsStore.dropTabs(entries.map((e) => e.tab.id));
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
    let focused: TabRecord | null = null;
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
      const record = tabsStore.receiveTab(tab, {
        incognito: entry.incognito,
        activate: entry.focus,
      });
      if (entry.focus) focused = record;
    }
    if (!focused) return;
    modeStore.setMode(
      (focused.kind in STUDIO_TAB_KIND_MODE
        ? STUDIO_TAB_KIND_MODE[focused.kind as StudioTabKind]
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
