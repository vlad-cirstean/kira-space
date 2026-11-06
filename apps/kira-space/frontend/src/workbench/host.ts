import { t } from '@theme/i18n';
import type { TabIconRender, WorkbenchHost } from '@workbench/host';
import { computed } from 'vue';
import { fileIconStyle } from '../repo/fileIcon';
import type { TabRecord } from '../state/tabDomain';
import { TAB_KINDS } from '../state/tabKinds';
import { useTabsStore } from '../state/tabs';
import { visibleWorkspace, type WorkspaceKey } from '../state/workspace';
import { useTabIncognitoStore } from '../studio/state/tabIncognito';
import { TAB_VIEWS } from './tabViews';

// The WorkbenchHost instance App.vue provides once. `tabBadge` shows a script terminal's run
// state; `tabIndicator` the incognito eye of a database or API tab.
export function createWorkbenchHost(): WorkbenchHost<WorkspaceKey, TabRecord> {
  const tabsStore = useTabsStore();
  const tabIncognitoStore = useTabIncognitoStore();

  return {
    // P128 §2.6: whichever module is active — the active repo (or GENERAL_WORKSPACE) in `git`
    // mode, else the module's own id (state/workspace.ts's own `visibleWorkspace`).
    activeWorkspace: computed(() => visibleWorkspace()),
    tabs: tabsStore,
    kinds: TAB_KINDS,
    views: TAB_VIEWS,
    // A tab kind's icon() returns either a codicon name or a `{ filePath }` marker (only
    // 'repo-file', state/tabKinds.ts's own doc comment) — resolved here into the two disjoint
    // render shapes the shared TabStrip.vue needs, the same split RepoTreeRow.vue's own
    // fileIconStyle call site uses for the identical marker.
    iconFor(tab): TabIconRender {
      const icon = TAB_KINDS[tab.kind].icon(tab);
      return typeof icon === 'string'
        ? { codicon: icon }
        : { fileStyle: fileIconStyle(icon.filePath) };
    },
    railColorFor(tab) {
      return TAB_KINDS[tab.kind].railColor(tab);
    },
    tabBadge(tab) {
      return TAB_KINDS[tab.kind].badge?.(tab) ?? null;
    },
    // The incognito eye glyph, uniform across every database and API kind.
    tabIndicator(tab) {
      return tabIncognitoStore.isIncognito(tab.id)
        ? { icon: 'eye-closed', tooltip: t('studio.tab.incognito.tooltip') }
        : null;
    },
  };
}
