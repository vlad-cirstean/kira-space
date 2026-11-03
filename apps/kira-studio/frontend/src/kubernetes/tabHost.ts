import type { TabStripHost } from '@workbench/host';
import { computed } from 'vue';
import { kindIcon } from './kinds';
import type { KubeTab, useKubeTabsStore } from './tabs';

/** The module's own tab strip: one tab per resource, so no duplicate action. */
export function createKubeTabHost(
  store: ReturnType<typeof useKubeTabsStore>,
): TabStripHost<'kubernetes', KubeTab> {
  return {
    activeWorkspace: computed(() => 'kubernetes' as const),
    tabs: {
      get activeIdByWorkspace() {
        return { kubernetes: store.activeId };
      },
      tabsForWorkspace: () => store.tabs,
      activateTab: store.activate,
      isPreview: (id) => store.previewId === id,
      promoteTab: store.promote,
      closeTab: store.close,
      closeOthers: store.closeOthers,
      closeToTheRight: store.closeToTheRight,
      closeAll: () => store.closeAll(),
      moveTab: store.move,
    },
    kinds: {
      resource: { title: store.titleOf, menuExtras: () => [] },
    },
    iconFor: (tab) => ({ codicon: kindIcon(tab.state.ref.kind) }),
    railColorFor: () => undefined,
  };
}
