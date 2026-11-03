import type { AppMode } from '@shared/domain/mode';
import { createModeStore } from '@workbench/state/createModeStore';
import { type ComputedRef, computed } from 'vue';
import { control } from '../bridge/control';
import { STUDIO_TAB_KIND_MODE, type TabRecord } from './tabDomain';
import { useTabsStore } from './tabs';

// The mode store's own generic mechanics (hydrate/setMode/the debounced writer/flush-
// before-close) hoisted to packages/workbench/src/state/createModeStore.ts, shared with Kira
// Space's own copy of this file. `activeTab` is Studio's own extra — a derived view over the one
// tab list, not a second state tree — passed through `extend` the same way
// createKeepAwakeStore.ts's own callers do.
export const useModeStore = createModeStore<AppMode, { activeTab: ComputedRef<TabRecord | null> }>(
  control,
  'studio',
  ({ state }) => ({
    activeTab: computed<TabRecord | null>(() => {
      const tabsStore = useTabsStore();
      const id = tabsStore.activeIdByWorkspace[state.active];
      return tabsStore.tabs.find((t) => t.id === id) ?? null;
    }),
  }),
);

// This one function is what replaces the mode filter at every read site tabsState used to scope
// by mode alone.
export function workspaceKeyOf(tab: Pick<TabRecord, 'kind' | 'workspaceId'>): AppMode {
  return (tab.workspaceId as AppMode | null) ?? (STUDIO_TAB_KIND_MODE[tab.kind] as AppMode);
}
